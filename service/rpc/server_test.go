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

package rpc_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/entguard/entguard/pkg/conc"
	"github.com/entguard/entguard/pkg/test"
	"github.com/entguard/entguard/pkg/test/grpc"
	"github.com/entguard/entguard/service/config"
	"github.com/entguard/entguard/service/rpc"
)

func TestIntegrationNewServer(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)
	cfg, err := config.FromEnv()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	cert, err := os.CreateTemp("", "grpc-server-cert.pem")
	require.NoError(t, err)
	defer func() { _ = os.Remove(cert.Name()) }()
	size, err := cert.Write([]byte(grpc.Cert))
	require.NoError(t, err)
	require.NotZero(t, size)
	cfg.RPC.CertFile = cert.Name()

	key, err := os.CreateTemp("", "grpc-server-key.pem")
	require.NoError(t, err)
	defer func() { _ = os.Remove(key.Name()) }()
	keysize, err := key.Write([]byte(grpc.Key))
	require.NoError(t, err)
	require.NotZero(t, keysize)
	cfg.RPC.KeyFile = key.Name()

	vpnSStatusStore := conc.NewMap[int, error]()
	hcStatusStore := rpc.NewHealthcheckStatusStore()
	server, err := rpc.NewServer(ctx, cfg.RPC, vcManager, uManager, vpnSStatusStore, hcStatusStore, q.Logger.Logger)
	require.NoError(t, err)
	require.NotNil(t, server)
	require.NotNil(t, server.VpnC)
	require.NotNil(t, server.VpnS)
	require.NotNil(t, server.Healthcheck)
	defer func() {
		server.VpnC.Stop()
		server.VpnS.Stop()
		server.Healthcheck.Stop()
	}()

	sInfo := server.VpnS.GetServiceInfo()
	require.NotNil(t, sInfo)
	require.Len(t, sInfo, 1)
	require.Len(t, sInfo["proto.VpnS"].Methods, 3)
	require.NotEmpty(t, sInfo["proto.VpnS"].Metadata)

	cInfo := server.VpnC.GetServiceInfo()
	require.NotNil(t, cInfo)
	require.Len(t, cInfo, 1)
	require.Len(t, cInfo["proto.VpnC"].Methods, 1)
	require.NotEmpty(t, cInfo["proto.VpnC"].Metadata)

	hcInfo := server.Healthcheck.GetServiceInfo()
	require.NotNil(t, hcInfo)
	require.Len(t, hcInfo, 1)
	require.Len(t, hcInfo["proto.v2.HealthcheckOrchestratorService"].Methods, 2)
	require.NotEmpty(t, hcInfo["proto.v2.HealthcheckOrchestratorService"].Metadata)
}
