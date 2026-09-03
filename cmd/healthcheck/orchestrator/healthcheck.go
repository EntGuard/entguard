/*
 * Copyright 2026 PANTHEON.tech s.r.o.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package orchestrator

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/entguard/entguard/pkg/limiter"
	config "github.com/entguard/entguard/pkg/server-config"
	pbapi "github.com/entguard/entguard/proto/v2"
)

type Healthcheck struct {
	cfg      *config.Config
	log      *log.Entry
	conn     *grpc.ClientConn
	tlsCfg   *tls.Config
	connErrs chan error
	stop     context.CancelFunc
	sessions *sync.Map
}

func NewHealthcheck(cfg *config.Config, tlsCfg *tls.Config, logger *log.Entry) *Healthcheck {
	return &Healthcheck{
		cfg:      cfg,
		log:      logger,
		connErrs: make(chan error),
		tlsCfg:   tlsCfg,
	}
}

// Connect connects Healthcheck service with Orchestrator.
func (hc *Healthcheck) Connect() error {
	var err error

	hc.log.WithField("TargetRPCAddress", hc.cfg.HCGRPCAddr).
		Info("Connecting to EntGuard orchestrator")
	hc.conn, err = hc.createConnection()
	if err != nil {
		return fmt.Errorf("could not connect to the EntGuard orchestrator: %v", err)
	}

	// activate connection
	hc.conn.Connect()
	time.Sleep(1 * time.Second)
	hc.log.Info("State of RPC connection with Orchestrator: ", hc.conn.GetState().String())

	hc.log.Info("Requesting all sessions")
	rpcClient := pbapi.NewHealthcheckOrchestratorServiceClient(hc.conn)
	err = hc.requestAllSessions(rpcClient)
	if err != nil {
		return fmt.Errorf("could not retrieve all sessions: %v", err)
	}

	hc.log.Info("Subscribing for sessions updates")
	ctx, cancel := context.WithCancel(context.Background())
	hc.stop = cancel
	go hc.subscribeForChanges(ctx, rpcClient, hc.cfg)

	return nil
}

// Disconnect disconnects Healthcheck service from Orchestrator.
func (hc *Healthcheck) Disconnect() {
	if hc.stop != nil {
		hc.log.Info("Stopping processing updates")
		hc.stop()
		hc.stop = nil
	}
	if hc.conn != nil {
		hc.log.Info("Disconnecting from EntGuard orchestrator")
		if err := hc.conn.Close(); err != nil {
			hc.log.WithError(err).Error("Closing gRPC connection")
		}
		hc.conn = nil
	}

	hc.log.Info("Clearing session list")
	hc.sessions = nil
}

func (hc *Healthcheck) ReconnectOnError() { //TODO: add tests when grpc streaming will be implemented
	for streamUpdatesErr := range hc.connErrs {
		hc.log.WithError(streamUpdatesErr).
			Warn("Reconnecting because an error occurred")

		var err error

		for i := 1; i <= hc.cfg.ReconnectAttempts; i++ {
			if i != 1 {
				hc.log.WithField("reconnectPause", hc.cfg.ReconnectPause).
					Debug("Sleeping before next reconnect attempt")
				time.Sleep(hc.cfg.ReconnectPause)
			}

			hc.log.WithField("reconnectAttempt", i).
				Info("Reconnecting...")
			err = hc.reconnectAttempt()
			if err != nil {
				hc.log.WithError(err).
					Info("Reconnect attempt failed")
				continue
			}
			break
		}
		if err != nil {
			hc.log.Error("All reconnecting attempts failed.")

			hc.log.Info("Shutting down.")
			hc.Disconnect()
			os.Exit(1)
		}
	}
}

func (hc *Healthcheck) reconnectAttempt() error {
	// Releasing resources
	hc.Disconnect()

	// Trying to reconnect
	return hc.Connect()
}

func (hc *Healthcheck) IsSessionInList(session string) (bool, error) {
	if hc.sessions == nil {
		return false, errors.New("sessions list is not initialized. Connect to Orchestrator first")
	}

	_, ok := hc.sessions.Load(session)

	return ok, nil
}

// createConnection returns gRPC client connection.
func (hc *Healthcheck) createConnection() (*grpc.ClientConn, error) {
	creds := credentials.NewTLS(hc.tlsCfg)
	return grpc.NewClient(hc.cfg.HCGRPCAddr, grpc.WithTransportCredentials(creds))
}

func (hc *Healthcheck) requestAllSessions(client pbapi.HealthcheckOrchestratorServiceClient) error {
	ctx, cancel := context.WithTimeout(context.Background(), hc.cfg.RPCTimeout)
	defer cancel()
	response, err := client.GetAllSessions(ctx, &pbapi.GetAllSessionsRequest{})
	if err != nil {
		return fmt.Errorf("request for all session IDs: %v", err)
	}
	if response == nil {
		return errors.New("received nil instead of sessions list")
	}

	hc.log.
		WithField("sessions", response.Sessions).
		WithField("length", len(response.Sessions)).
		Debug("Received sessions from Orchestrator service.")

	m := new(sync.Map)
	for _, s := range response.Sessions {
		m.Store(s, nil)
	}

	hc.sessions = m

	return nil
}

func (hc *Healthcheck) subscribeForChanges(ctx context.Context, client pbapi.HealthcheckOrchestratorServiceClient,
	cfg *config.Config) {
	stream, err := client.SubscribeForSessionsUpdates(ctx)
	if err != nil {
		hc.connErrs <- err
		return
	}

	rateLimiter := limiter.NewLimiter(cfg.HCTokensPerSecond, cfg.HCBucketSize)
	notifications := make(chan *pbapi.SubscribeForSessionsUpdatesResponse)
	go func() {
		hc.log.Debug("Waiting for sessions updates...")
		for {
			notification, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				hc.log.WithError(err).
					Error("Failed to receive from stream")
				break
			}
			if rateLimiter.Limit() {
				hc.log.Error("Session updates stream rate limit exceeded! Dropping current sessions update..." +
					"(please recheck ratelimiter configuration if this is normal traffic intensity and not DDOS attack)")
				continue
			}
			notifications <- notification
		}
		close(notifications)
	}()

	err = stream.Send(&pbapi.SubscribeForSessionsUpdatesRequest{Data: &pbapi.SubscribeForSessionsUpdatesRequest_Subscribe{}})
	if err != nil {
		hc.connErrs <- err
		return
	}

	for {
		select {
		case ntf, ok := <-notifications:
			if !ok {
				hc.connErrs <- errors.New("disconnected from update notifications")
				return
			}

			err := hc.handleNotification(ntf)
			code := pbapi.RPCErrorCode_RPC_ERROR_CODE_OK
			if err != nil {
				code = pbapi.RPCErrorCode_RPC_ERROR_CODE_INTERNAL
				hc.log.Error(err)
			}
			e := stream.Send(updateResponse(code, err))
			if e != nil {
				hc.connErrs <- e
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (hc *Healthcheck) handleNotification(ntf *pbapi.SubscribeForSessionsUpdatesResponse) error {
	hc.log.Debug("Updating session list based on orchestrator notification...")
	switch ntf.Operation {
	case pbapi.SubscribeForSessionsUpdatesResponse_OPERATION_UNDEFINED_UNSPECIFIED:
		return fmt.Errorf("session notification type is undefined, ignoring")

	case pbapi.SubscribeForSessionsUpdatesResponse_OPERATION_ADD_SESSION:
		hc.log.WithFields(log.Fields{
			"session": ntf.Session,
		}).Debug("Adding session.")
		hc.sessions.Store(ntf.Session, nil)

	case pbapi.SubscribeForSessionsUpdatesResponse_OPERATION_DELETE_SESSION:
		hc.log.WithField("session", ntf.Session).Debug("Deleting session.")
		_, ok := hc.sessions.Load(ntf.Session)
		if !ok {
			return fmt.Errorf("session=%s not found in session list", ntf.Session)
		}
		hc.sessions.Delete(ntf.Session)

	default:
		return fmt.Errorf("unsupported peer notification type")
	}
	return nil
}

func updateResponse(code pbapi.RPCErrorCode, err error) *pbapi.SubscribeForSessionsUpdatesRequest {
	var msg string
	if err != nil {
		msg = err.Error()
	}
	return &pbapi.SubscribeForSessionsUpdatesRequest{
		Data: &pbapi.SubscribeForSessionsUpdatesRequest_Response{
			Response: &pbapi.RPCStatus{
				Code:    code,
				Message: msg,
			},
		},
	}
}
