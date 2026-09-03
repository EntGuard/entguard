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

	log "github.com/sirupsen/logrus"

	pbapi "github.com/entguard/entguard/proto/v1"
	"github.com/entguard/entguard/service/session"
	"github.com/entguard/entguard/service/user"
	"github.com/entguard/entguard/service/vcm"
)

const (
	vpnCCaller = "VpnC GRPC server"
)

type VpnCServer struct {
	pbapi.UnimplementedVpnCServer

	um     user.Manager
	vcm    vcm.VPNConfigManager
	logger *log.Entry
}

func NewVpnCServer(u user.Manager, m vcm.VPNConfigManager, l *log.Logger) (*VpnCServer, error) {
	logger := l.WithField("reportCaller", vpnCCaller)
	return &VpnCServer{
		um:     u,
		vcm:    m,
		logger: logger,
	}, nil
}

func (s *VpnCServer) GetConfiguration(ctx context.Context, req *pbapi.ClientConfigurationRequest) (*pbapi.ClientConfigurationResponse, error) {
	md, ok := GetUserMetadata(ctx)
	if !ok {
		return nil, errors.New("failed to acquire user MD")
	}

	session, err := session.GenerateSessionID()
	if err != nil {
		s.logger.WithError(err).WithField("userID", md.UserID).Debug("Failed to generate session")
		return nil, fmt.Errorf("GenerateSessionID: %v", err)
	}

	externalDeviceID := req.GetExternalDeviceId()
	deviceInformation := req.GetDeviceInformation()
	assignedExternalDeviceID, err := s.um.AssignDeviceSlot(ctx, md.UserID, session, externalDeviceID, deviceInformation)
	if err != nil {
		s.logger.WithError(err).
			WithField("userID", md.UserID).
			WithField("externalDeviceID", externalDeviceID).
			Error("Assign device slot")
		return nil, fmt.Errorf("AssignDeviceSlot: %v", err)
	}

	s.logger.WithField("assignedExternalDeviceID", assignedExternalDeviceID).Debug("Retrieving configuration for device")
	device, err := s.um.GetDeviceByExternalIDAndUserID(ctx, assignedExternalDeviceID, md.UserID)
	if err != nil {
		s.logger.WithError(err).
			WithField("userID", md.UserID).
			WithField("externalDeviceID", assignedExternalDeviceID).
			Error("Get device by external ID and user ID")
		return nil, fmt.Errorf("GetDeviceByExternalIDAndUserID: %v", err)
	}
	conf, err := s.getConfiguration(ctx, device.ID)
	if err != nil {
		s.logger.WithError(err).
			WithField("userID", md.UserID).
			WithField("externalDeviceID", assignedExternalDeviceID).
			WithField("deviceID", device.ID).
			Error("Get device configuration")
		return nil, fmt.Errorf("getConfiguration: %v", err)
	}

	s.logger.
		WithField("userID", md.UserID).
		WithField("externalDeviceID", externalDeviceID).
		WithField("sessionID", session).
		Debug("Sending WireGuard configuration for device")

	return &pbapi.ClientConfigurationResponse{
		Configuration: conf,
		SessionId:     session,
	}, nil
}

func (s *VpnCServer) getConfiguration(ctx context.Context, deviceID int) (*pbapi.Configuration, error) {
	var vpnConfig *pbapi.Configuration
	cfg, err := s.vcm.GetDeviceWireGuardConfig(ctx, deviceID)
	if err != nil {
		if errors.Is(err, vcm.ErrNotFound) {
			return nil, fmt.Errorf("device WireGuard config was not found : %v", err)
		}
		return nil, fmt.Errorf("device WireGuard config acquisition failed: %v", err)
	}
	adjacencies, err := s.vcm.GetWireGuardDeviceAdjacencies(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	vpnConfig, err = wireguardConfigToProto(cfg, adjacencies, s.um.GetHealthcheckVpnCPort(), s.logger)
	if err != nil {
		return nil, err
	}
	return vpnConfig, nil
}
