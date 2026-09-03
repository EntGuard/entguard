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

package vpn_client

import (
	"fmt"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/grpc-ecosystem/go-grpc-middleware/ratelimit"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	orch "github.com/entguard/entguard/cmd/healthcheck/orchestrator"
	gserver "github.com/entguard/entguard/pkg/grpc"
	"github.com/entguard/entguard/pkg/limiter"
	config "github.com/entguard/entguard/pkg/server-config"
	pbapiv2 "github.com/entguard/entguard/proto/v2"
)

const clientHealthcheckPort = 8084 // TODO hardcoded for now

func RunGRPCServerForVPNClient(logger *log.Entry, hc *orch.Healthcheck, cfg *config.Config, e chan error) {
	logger.
		Info("Starting gRPC server...")

	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(
			grpc_middleware.ChainUnaryServer(
				ratelimit.UnaryServerInterceptor(limiter.NewLimiter(cfg.HCTokensPerSecond, cfg.HCBucketSize)),
			),
		),
		// NOTE: not defining grpc.StreamInterceptor for rate limiter as we are not using streams in proto now
	}
	server := grpc.NewServer(opts...)
	pbapiv2.RegisterHealthcheckClientServiceServer(server, &HealthcheckClient{
		hc:  hc,
		log: logger,
	})

	logger.
		WithField("port", clientHealthcheckPort).
		Info("Listening for Healthcheck pings from VPN client")
	go gserver.RunGRPCServer(server, fmt.Sprintf(":%d", clientHealthcheckPort), e)
}
