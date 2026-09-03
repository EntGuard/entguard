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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"github.com/grpc-ecosystem/go-grpc-middleware/ratelimit"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/entguard/entguard/pkg/conc"
	"github.com/entguard/entguard/pkg/limiter"
	pbapiv1 "github.com/entguard/entguard/proto/v1"
	pbapiv2 "github.com/entguard/entguard/proto/v2"
	"github.com/entguard/entguard/service/api/v1"
	"github.com/entguard/entguard/service/config"
	"github.com/entguard/entguard/service/user"
	"github.com/entguard/entguard/service/vcm"
)

var (
	errCfgIsNil = errors.New("config is nil")
)

type Server struct {
	VpnS        *grpc.Server
	VpnC        *grpc.Server
	Healthcheck *grpc.Server
}

func NewServer(
	ctx context.Context, cfg *config.RPCServerConfig, m vcm.VPNConfigManager, u user.Manager,
	vpnSStatusStore *conc.Map[int, error], hcStatusStore *conc.Map[int, error], l *log.Logger,
) (*Server, error) {
	s, err := NewVpnSServer(m, vpnSStatusStore, cfg.Features, l)
	if err != nil {
		return nil, err
	}

	c, err := NewVpnCServer(u, m, l)
	if err != nil {
		return nil, err
	}

	hc, err := NewHealthcheckServer(m, u, hcStatusStore, l)
	if err != nil {
		return nil, err
	}

	cert, err := cfg.GetTLSCert()
	if err != nil {
		return nil, err
	}

	ca := m.GetCACertificate(ctx)
	if ca == nil {
		return nil, errors.New("CA certificate for client auth cannot be nil")
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(ca)

	// VPN Server Service
	vpnsOpts := []grpc.ServerOption{
		grpc.Creds(credentials.NewTLS(&tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{*cert},
			ClientCAs:    caCertPool,
			ClientAuth:   tls.RequireAndVerifyClientCert,
		})),
		grpc.UnaryInterceptor(
			grpc_middleware.ChainUnaryServer(
				ratelimit.UnaryServerInterceptor(limiter.NewLimiter(cfg.TokensPerSecond, cfg.BucketSize)),
				grpc_auth.UnaryServerInterceptor(s.TLSAuth),
			),
		),
		grpc.StreamInterceptor(
			grpc_middleware.ChainStreamServer(
				ratelimit.StreamServerInterceptor(limiter.NewLimiter(cfg.TokensPerSecond, cfg.BucketSize)),
				grpc_auth.StreamServerInterceptor(s.TLSAuth),
			),
		),
	}
	grpcVpnSServer := grpc.NewServer(vpnsOpts...)
	pbapiv1.RegisterVpnSServer(grpcVpnSServer, s)

	// VPN Client Service
	vpncOpts := []grpc.ServerOption{
		grpc.Creds(credentials.NewTLS(&tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{*cert},
		})),
		grpc.UnaryInterceptor(
			grpc_middleware.ChainUnaryServer(
				ratelimit.UnaryServerInterceptor(limiter.NewLimiter(cfg.TokensPerSecond, cfg.BucketSize)),
				grpc_auth.UnaryServerInterceptor(c.BasicAuth),
				grpc_auth.UnaryServerInterceptor(c.MfaAuth),
			),
		),
		grpc.StreamInterceptor(
			grpc_middleware.ChainStreamServer(
				ratelimit.StreamServerInterceptor(limiter.NewLimiter(cfg.TokensPerSecond, cfg.BucketSize)),
				grpc_auth.StreamServerInterceptor(c.BasicAuth),
				grpc_auth.StreamServerInterceptor(c.MfaAuth),
			),
		),
	}
	grpcVpnCServer := grpc.NewServer(vpncOpts...)
	pbapiv1.RegisterVpnCServer(grpcVpnCServer, c)

	// Healthcheck Service
	hcOpts := []grpc.ServerOption{
		grpc.Creds(credentials.NewTLS(&tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{*cert},
			ClientCAs:    caCertPool,
			ClientAuth:   tls.RequireAndVerifyClientCert,
		})),
		grpc.UnaryInterceptor(
			grpc_middleware.ChainUnaryServer(
				ratelimit.UnaryServerInterceptor(limiter.NewLimiter(cfg.TokensPerSecond, cfg.BucketSize)),
			),
		),
		grpc.StreamInterceptor(
			grpc_middleware.ChainStreamServer(
				ratelimit.StreamServerInterceptor(limiter.NewLimiter(cfg.TokensPerSecond, cfg.BucketSize)),
			),
		),
	}
	grpcHealthcheckServer := grpc.NewServer(hcOpts...)
	pbapiv2.RegisterHealthcheckOrchestratorServiceServer(grpcHealthcheckServer, hc)

	server := &Server{
		VpnS:        grpcVpnSServer,
		VpnC:        grpcVpnCServer,
		Healthcheck: grpcHealthcheckServer,
	}

	return server, nil
}

func wireguardConfigToProto(cfg *api.ServerWireGuardInterface, adjacencies []*api.WireGuardPeer, healthcheckPort string, logger *log.Entry) (*pbapiv1.Configuration, error) {
	if cfg == nil {
		return nil, errCfgIsNil
	}

	var wgPeers []*pbapiv1.WireGuardPeer
	for _, p := range adjacencies {

		var healthcheckEndpoint string
		if p.Server != nil && p.Server.HealthCheckAddress != "" && healthcheckPort != "" {
			healthcheckEndpoint = fmt.Sprintf("%s:%s", p.Server.HealthCheckAddress, healthcheckPort)
		}

		logger.Debugf("Creating peer proto with AllowedIPs=%v, OtherSideAllowedIPs=%v", p.Config.AllowedIPs, p.Config.OtherSideAllowedIPs)

		wgPeers = append(wgPeers, &pbapiv1.WireGuardPeer{
			PublicKey:           p.Config.PublicKey,
			PresharedKey:        p.Config.PresharedKey,
			AllowedIps:          p.Config.AllowedIPs,
			Endpoint:            p.Config.Endpoint,
			PersistentKeepalive: p.Config.PersistentKeepalive,
			HealthcheckEndpoint: healthcheckEndpoint,
		})
	}

	vpnConfig := &pbapiv1.Configuration{
		Type: pbapiv1.Configuration_WIREGUARD,
		VpnProto: &pbapiv1.Configuration_Wg{
			Wg: &pbapiv1.WireGuard{
				Name: cfg.Name,
				Iface: &pbapiv1.WireGuardInterface{
					PrivateKey: cfg.PrivateKey,
					Addresses:  cfg.Addresses,
					ListenPort: cfg.ListenPort,
					Dns:        cfg.DNS,
					Mtu:        cfg.MTU,
				},
				Peers: wgPeers,
			},
		},
	}

	return vpnConfig, nil
}
