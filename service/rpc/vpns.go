/*
 * Copyright 2020 PANTHEON.tech s.r.o.
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
	"net"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/entguard/entguard/pkg/certs"
	"github.com/entguard/entguard/pkg/conc"
	pbapi "github.com/entguard/entguard/proto/v1"
	"github.com/entguard/entguard/service/config"
	"github.com/entguard/entguard/service/vcm"
)

const (
	vpnSCaller = "VpnS GRPC server"

	telemetrySocketPath = "/run/entguard/sockets/socket"
)

var (
	ErrDisconnected = errors.New("server disconnected")
	ErrNameNotFound = errors.New("failed to acquire server name")
)

type VpnSServer struct {
	pbapi.UnimplementedVpnSServer

	vcm    vcm.VPNConfigManager
	logger *log.Entry

	statusStore *conc.Map[int, error]
	feats       config.FeaturesData
}

func NewVpnSServer(m vcm.VPNConfigManager, s *conc.Map[int, error], feats config.FeaturesData, l *log.Logger) (*VpnSServer, error) {
	logger := l.WithField("reportCaller", vpnSCaller)
	srv := &VpnSServer{
		vcm:         m,
		statusStore: s,
		logger:      logger,
		feats:       feats,
	}
	return srv, nil
}

func (s *VpnSServer) GetConfiguration(ctx context.Context, req *pbapi.Empty) (*pbapi.Configuration, error) {
	md, ok := GetServerMetadata(ctx)
	if !ok {
		return nil, errors.New("failed to acquire server name")
	}

	s.logger.
		WithField("serverName", md.ServerName).
		Info("Received request for configuration for the server.")
	vpnServer, err := s.vcm.GetServerByName(ctx, md.ServerName)
	if err != nil {
		return nil, err
	}

	var vpnConfig *pbapi.Configuration
	cfg, err := s.vcm.GetWireGuardServerConfig(ctx, vpnServer.ID)
	if err != nil {
		return nil, err
	}
	adjacencies, err := s.vcm.GetWireGuardServerAdjacencies(ctx, vpnServer.ID)
	if err != nil {
		return nil, err
	}
	vpnConfig, err = wireguardConfigToProto(cfg, adjacencies, "", s.logger)
	if err != nil {
		return nil, err
	}

	vpnConfig.TelemetryEnabled = s.feats.Telemetry()

	return vpnConfig, nil
}

func (s *VpnSServer) SubscribeWG(stream pbapi.VpnS_SubscribeWGServer) error {
	ctx := stream.Context()
	md, ok := GetServerMetadata(ctx)
	if !ok {
		return ErrNameNotFound
	}

	s.logger.
		WithField("serverName", md.ServerName).
		Info("Received request for configuration changes stream for the server.")
	vpnS, err := s.vcm.GetServerByName(ctx, md.ServerName)
	if err != nil {
		return err
	}

	// Server should continuously listen to config updates, so if this function
	// exits, consider the server disconnected.
	defer func() {
		s.statusStore.Set(vpnS.ID, ErrDisconnected)
	}()

	req, err := stream.Recv()
	if err == io.EOF {
		s.logger.
			WithError(err).
			Info("Stopped receiving update responses from server.")
		return nil
	}
	if err != nil {
		s.logger.
			WithError(err).
			Warn("Receiving subscribe request failed.")
		return err
	}
	// First message from the server should be a subscribe request of type *pb.SubscribeWGRequest_Subscribe.
	// All subsequent messages should be configuration responses of type *pb.SubscribeWGRequest_Response.
	if req.GetSubscribe() == nil {
		err = fmt.Errorf("expected subscribe request from server with ID %d, instead got message of type (%T)", vpnS.ID, req.GetData())
		s.logger.
			WithError(err).
			Warn("Received wrong response type from server.")
		return err
	}

	// SubscribeWG got called by the server, that means the initial call to
	// GetConfiguration function succeeded and server applied the configuration
	// correctly. So set the last err for this server to nil.
	s.statusStore.Set(vpnS.ID, nil)
	ch := make(chan *pbapi.WireGuardNotification)
	cancel, err := s.vcm.WatchWireGuardConfig(ctx, ch, vpnS.ID, md.ServerTag)
	if err != nil {
		return err
	}
	defer func() {
		cancel()
		s.logger.
			WithField("serverName", md.ServerName).
			Info("Configuration changes stream for the server has stopped.")
	}()

	for {
		select {
		case ntf, ok := <-ch:
			if !ok {
				s.logger.
					Info("Notification channel was closed.")
				return nil
			}
			s.logger.
				Info("Sending update request to server.")
			if err := stream.Send(ntf); err != nil {
				s.logger.
					WithError(err).
					Warn("Sending notification failed.")
				return err
			}
			if err := s.receiveFromServer(vpnS.ID, stream); err != nil {
				return err
			}
		case <-stream.Context().Done():
			// VpnS has stopped listening on the other side of the stream.
			return nil
		}
	}
}

func (s *VpnSServer) SendTelemetry(stream pbapi.VpnS_SendTelemetryServer) error {
	md, ok := GetServerMetadata(stream.Context())
	if !ok {
		return ErrNameNotFound
	}
	s.logger.
		WithField("serverName", md.ServerName).
		Info("Starting receiving telemetry data from server.")

	addr, err := net.ResolveUnixAddr("unix", telemetrySocketPath)
	if err != nil {
		return err
	}

	conn, err := net.DialUnix("unix", nil, addr)
	if err != nil {
		return err
	}

	defer func() { _ = conn.Close() }()

	for {
		telemetry, err := stream.Recv()
		if err == io.EOF {
			s.logger.Info("Stopped receiving telemetry data from server.")
			return stream.SendAndClose(&pbapi.Empty{})
		}
		if err != nil {
			return err
		}

		_, err = conn.Write(telemetry.Data)
		if err != nil {
			return err
		}
	}
}

// TLSAuth tries to authenticate server by using provided certificate,
// on success it appends servername to context MD and returns new context, on fail it returns an error.
func (s *VpnSServer) TLSAuth(ctx context.Context) (context.Context, error) {
	s.logger.Debug("TLS auth started")
	p, ok := peer.FromContext(ctx)
	if !ok {
		return ctx, status.Error(codes.Unauthenticated, "no peer found")
	}
	tlsAuth, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return ctx, status.Error(codes.Unauthenticated, "unexpected peer transport credentials")
	}
	if len(tlsAuth.State.VerifiedChains) == 0 || len(tlsAuth.State.VerifiedChains[0]) == 0 {
		return ctx, status.Error(codes.Unauthenticated, "could not verify peer certificate")
	}
	certificate := tlsAuth.State.VerifiedChains[0][0]
	if len(certificate.Subject.OrganizationalUnit) == 0 {
		return ctx, status.Error(codes.Unauthenticated, "could not get certificate's subject's organization units")
	}
	name := certificate.Subject.OrganizationalUnit[0]
	tag, err := certs.GetServerTag(certificate)
	if err != nil {
		return ctx, status.Error(codes.Unauthenticated, "could not extract server tag from certificate")
	}

	verified := s.vcm.VerifyServerTag(ctx, tag, name)
	if !verified {
		return ctx, status.Error(codes.Unauthenticated, "could not verify server tag")
	}

	serverMD := &ServerMetadata{
		ServerName: name,
		ServerTag:  tag,
	}
	newCtx := SetServerMetadata(ctx, serverMD)

	s.logger.WithField("serverName", name).Debug("TLS auth completed")
	return newCtx, nil
}

func (s *VpnSServer) receiveFromServer(serverID int, stream pbapi.VpnS_SubscribeWGServer) error {
	resp, err := stream.Recv()
	if err == io.EOF {
		s.logger.
			WithError(err).
			Info("Stopped receiving update responses from server.")
		return nil
	}
	if err != nil {
		s.logger.
			WithError(err).
			Warn("Receiving response failed.")
		return err
	}
	s.logger.
		Info("Received update response from server.")
	switch r := resp.Data.(type) {
	case *pbapi.SubscribeWGRequest_Subscribe:
		err := fmt.Errorf("server %d is already subscribed", serverID)
		s.logger.
			WithError(err).
			Warn("Got subscribe request from already subscribed and connected server.")
		return err
	case *pbapi.SubscribeWGRequest_Response:
		if r.Response.Code == pbapi.RPCErrorCode_OK {
			s.logger.
				Info("Server update successful.")
			s.statusStore.Set(serverID, nil)
		} else {
			err := fmt.Errorf("server update failed with code: %s and message: %s", pbapi.RPCErrorCode_name[int32(r.Response.Code)], r.Response.Message)
			s.logger.
				WithError(err).
				Warn("Received error response from server.")
			s.statusStore.Set(serverID, err)
		}
	default:
		err := fmt.Errorf("server replied with unsupported response type (%T)", r)
		s.logger.
			WithError(err).
			Warn("Received unsupported response type from server.")
		return err
	}
	return nil
}

type serverMDKey struct{}

// ServerMetadata contains metadata about a server.
type ServerMetadata struct {
	ServerName string
	ServerTag  string
}

// GetServerMetadata can be used to extract server metadata stored in a context.
func GetServerMetadata(ctx context.Context) (*ServerMetadata, bool) {
	serverMD := ctx.Value(serverMDKey{})

	switch md := serverMD.(type) {
	case *ServerMetadata:
		return md, true
	default:
		return nil, false
	}
}

func SetServerMetadata(ctx context.Context, serverMD *ServerMetadata) context.Context {
	return context.WithValue(ctx, serverMDKey{}, serverMD)
}
