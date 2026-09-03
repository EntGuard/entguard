/*
 * Copyright 2025 PANTHEON.tech s.r.o.
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

package rpc

import (
	"context"
	"errors"
	"fmt"
	"io"

	log "github.com/sirupsen/logrus"

	"github.com/entguard/entguard/pkg/conc"
	pbapi "github.com/entguard/entguard/proto/v2"
	"github.com/entguard/entguard/service/user"
	"github.com/entguard/entguard/service/vcm"
)

const (
	DefaultHealthCheckStatusStoreID = 0 // used to identify default HC in HCStatusStore that is implemented as Map

	hcSCaller = "Healthcheck GRPC server"
)

func NewHealthcheckStatusStore() *conc.Map[int, error] {
	store := conc.NewMap[int, error]()
	store.Set(DefaultHealthCheckStatusStoreID, ErrDisconnected) // setting default value
	return store
}

type HealthcheckServer struct {
	pbapi.UnimplementedHealthcheckOrchestratorServiceServer

	vcm         vcm.VPNConfigManager
	um          user.Manager
	logger      *log.Entry
	statusStore *conc.Map[int, error]
}

func NewHealthcheckServer(m vcm.VPNConfigManager, u user.Manager, hcStatusStore *conc.Map[int, error], l *log.Logger) (*HealthcheckServer, error) {
	logger := l.WithField("reportCaller", hcSCaller)
	srv := &HealthcheckServer{
		vcm:         m,
		um:          u,
		logger:      logger,
		statusStore: hcStatusStore,
	}
	return srv, nil
}

func (hc *HealthcheckServer) GetAllSessions(ctx context.Context, req *pbapi.GetAllSessionsRequest) (*pbapi.GetAllSessionsResponse, error) {
	hc.logger.
		Info("Received request to fetch all sessions for the healthcheck service.")

	sessions, err := hc.um.GetAllDevicesSessions(ctx)
	if err != nil {
		return nil, err
	}

	return &pbapi.GetAllSessionsResponse{
		Sessions: sessions,
	}, nil
}

func (hc *HealthcheckServer) SubscribeForSessionsUpdates(stream pbapi.HealthcheckOrchestratorService_SubscribeForSessionsUpdatesServer) error {
	ctx := stream.Context()

	// Healthcheck service should continuously listen for sessions updates, so if this function
	// exits, consider the healthcheck service disconnected.
	defer func() {
		hc.statusStore.Set(DefaultHealthCheckStatusStoreID, ErrDisconnected)
		hc.logger.
			Info("Sessions changes stream for the healthcheck service has stopped.")
	}()

	req, err := stream.Recv()
	if err == io.EOF {
		hc.logger.
			WithError(err).
			Info("Stopped receiving update responses from healthcheck service.")
		return nil
	}
	if err != nil {
		hc.logger.
			WithError(err).
			Warn("Receiving subscribe request failed.")
		return err
	}
	// First message from the healthcheck service should be a subscribe request of type *pb.SubscribeForSessionsUpdatesRequest_Subscribe.
	// All subsequent messages should be session responses of type *pb.SubscribeForSessionsUpdatesRequest_Response.
	if req.GetSubscribe() == nil {
		err = fmt.Errorf("expected subscribe request, got message of type (%T)", req.GetData())
		hc.logger.
			WithError(err).
			Warn("Received wrong response type from healthcheck service.")
		return err
	}

	// Set default error for healthcheck to nil.
	hc.statusStore.Set(DefaultHealthCheckStatusStoreID, nil)

	notificationsCh := hc.vcm.WatchDeviceSessionUpdates(ctx)
	for {
		select {
		case ntf, ok := <-notificationsCh:
			if !ok {
				hc.logger.
					Info("Notification channel was closed.")
				return nil
			}
			hc.logger.
				Debug("Sending update request to healthcheck service.")
			if err := stream.Send(ntf); err != nil {
				hc.logger.
					WithError(err).
					Warn("Sending notification to healthcheck service failed.")
				return err
			}
			if err := hc.receiveFromHealthcheck(stream); err != nil {
				return err
			}
		case <-stream.Context().Done():
			// Healthcheck service has stopped listening on the other side of the stream.
			return nil
		}
	}
}

func (hc *HealthcheckServer) receiveFromHealthcheck(stream pbapi.HealthcheckOrchestratorService_SubscribeForSessionsUpdatesServer) error {
	resp, err := stream.Recv()
	if err == io.EOF {
		hc.logger.
			WithError(err).
			Info("Stopped receiving update responses from healthcheck service.")
		return nil
	}
	if err != nil {
		hc.logger.
			WithError(err).
			Warn("Receiving response failed for healthcheck service.")
		return err
	}
	hc.logger.
		Debug("Received update response from healthcheck service.")
	switch r := resp.Data.(type) {
	case *pbapi.SubscribeForSessionsUpdatesRequest_Subscribe:
		err := errors.New("healthcheck service is already subscribed")
		hc.logger.
			WithError(err).
			Warn("Got subscribe request from already subscribed and connected healthcheck service.")
		return err
	case *pbapi.SubscribeForSessionsUpdatesRequest_Response:
		if r.Response.Code == pbapi.RPCErrorCode_RPC_ERROR_CODE_OK {
			hc.logger.
				Debug("Healthcheck service update successful.")
			hc.statusStore.Set(DefaultHealthCheckStatusStoreID, nil)
		} else {
			err := fmt.Errorf("healthcheck service update failed with code: %s and message: %s", pbapi.RPCErrorCode_name[int32(r.Response.Code)], r.Response.Message)
			hc.logger.
				WithError(err).
				Warn("Received error response from healthcheck service.")
			hc.statusStore.Set(DefaultHealthCheckStatusStoreID, err)
		}
	default:
		err := fmt.Errorf("healthcheck service replied with unsupported response type (%T)", r)
		hc.logger.
			WithError(err).
			Warn("Received unsupported response type from healthcheck service.")
		return err
	}
	return nil
}
