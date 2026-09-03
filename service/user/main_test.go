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

package user_test

import (
	"context"
	"testing"

	log "github.com/sirupsen/logrus"

	sqlc "github.com/entguard/entguard/db"
	"github.com/entguard/entguard/pkg/db"
	"github.com/entguard/entguard/pkg/test"
	"github.com/entguard/entguard/service/user"
	"github.com/entguard/entguard/service/vcm"
)

var (
	q         *user.PostgresQuerier
	sqlcq     *sqlc.Queries
	vcManager vcm.VPNConfigManager
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

		manager, err = test.NewSamplePostgresManager(q, logger, user.NewLdapService(), vcManager, sqlcq)
		if err != nil {
			logger.Fatal("NewSamplePostgresManager: ", err)
		}

		if err := test.TruncateAllTables(ctx, q.GetDbConnection(ctx)); err != nil {
			logger.Fatal(err)
		}
	}
	m.Run()
}
