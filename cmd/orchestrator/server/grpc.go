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

package server

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/entguard/entguard/pkg/grpc"
	"github.com/entguard/entguard/service/config"
	"github.com/entguard/entguard/service/rpc"
)

func RunRPC(s *rpc.Server, cfg *config.RPCServerConfig, l *log.Logger, e chan error) {
	logger := l.WithField("reportCaller", "RPC API")
	logger.
		Info("Starting RPC...")

	addrS := fmt.Sprintf(":%s", cfg.PortVpnS)
	addrC := fmt.Sprintf(":%s", cfg.PortVpnC)
	addrHealthcheck := fmt.Sprintf(":%s", cfg.PortHealthcheck)

	logger.
		WithField("port", cfg.PortVpnS).
		Info("Listening for VPN server")
	go grpc.RunGRPCServer(s.VpnS, addrS, e)

	logger.
		WithField("port", cfg.PortVpnC).
		Info("Listening for VPN client")
	go grpc.RunGRPCServer(s.VpnC, addrC, e)

	logger.
		WithField("port", cfg.PortHealthcheck).
		Info("Listening for Healthcheck service")
	go grpc.RunGRPCServer(s.Healthcheck, addrHealthcheck, e)
}
