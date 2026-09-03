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
	"testing"

	log "github.com/sirupsen/logrus"

	sqlc "github.com/entguard/entguard/db"
	"github.com/entguard/entguard/pkg/conc"
	"github.com/entguard/entguard/pkg/db"
	"github.com/entguard/entguard/pkg/test"
	"github.com/entguard/entguard/service/config"
	"github.com/entguard/entguard/service/rpc"
	"github.com/entguard/entguard/service/user"
	"github.com/entguard/entguard/service/vcm"
)

var (
	vpnCServer        *rpc.VpnCServer
	vpnSServer        *rpc.VpnSServer
	healthcheckServer *rpc.HealthcheckServer
	q                 *user.PostgresQuerier
	sqlcq             *sqlc.Queries
	uManager          *user.PostgresManager
	vcManager         vcm.VPNConfigManager
)

func TestMain(m *testing.M) {
	if test.IsIntegrationTest() {
		logger := log.New()
		logger.SetLevel(log.DebugLevel)

		ctx := context.Background()

		conn, err := db.Connect(ctx, logger, test.PostgresConfig())
		if err != nil {
			logger.Fatal("db.Connect: ", err)
		}
		q = user.NewPostgresQuerier(ctx, conn, logger).(*user.PostgresQuerier)
		sqlcq = sqlc.New(conn)
		vcManager, err = vcm.NewVCMPostgresBased(ctx, conn, logger, false, test.TestEncryptionKey)
		if err != nil {
			logger.Fatal("NewVCMPostgresBased: ", err)
		}
		vcManager.SetKubernetesClientset(ctx, nil) // do not use kube cluster settings in tests

		uManager, err = test.NewSamplePostgresManager(q, logger, user.NewLdapService(), vcManager, sqlc.New(conn))
		if err != nil {
			logger.Fatal("NewSamplePostgresManager: ", err)
		}

		vpnCServer, err = rpc.NewVpnCServer(uManager, vcManager, logger)
		if err != nil {
			logger.Fatal("NewVpnCServer: ", err)
		}
		// create stub features
		feats, err := config.RetrieveFeatures("", "")
		if err != nil {
			logger.Fatal("RetrieveFeatures: ", err)
		}
		vpnSStatusStore := conc.NewMap[int, error]()
		vpnSServer, err = rpc.NewVpnSServer(vcManager, vpnSStatusStore, feats, logger)
		if err != nil {
			logger.Fatal("NewVpnSServer: ", err)
		}

		hcStatusStore := rpc.NewHealthcheckStatusStore()
		healthcheckServer, err = rpc.NewHealthcheckServer(vcManager, uManager, hcStatusStore, logger)
		if err != nil {
			logger.Fatal("NewHealthcheckServer: ", err)
		}

		if err := test.TruncateAllTables(ctx, q.GetDbConnection(ctx)); err != nil {
			logger.Fatal(err)
		}
	}
	m.Run()
}
