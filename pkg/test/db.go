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

package test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	sqlc "github.com/entguard/entguard/db"
	"github.com/entguard/entguard/pkg/crypto/aesgcm"
	"github.com/entguard/entguard/pkg/mfa"
	"github.com/entguard/entguard/service/config"
	"github.com/entguard/entguard/service/user"
	"github.com/entguard/entguard/service/vcm"
	pg "github.com/entguard/entguard/service/vcm/drivers/postgres"
)

const (
	unitEnvVar              = "TEST_UNIT"
	integrationEnvVar       = "TEST_INTEGRATION"
	restFuzzEnvVar          = "TEST_FUZZ_REST"
	restHandlersLocalEnvVar = "TEST_LOCAL_REST_HANDLERS"

	restFuzzComposeProject      = "eg-test-fuzz-rest"
	restFuzzOrchestratorService = "testweb"
	restFuzzDBService           = "testdb"

	Localhost      = "127.0.0.1"
	countTableRows = "SELECT COUNT(*)::int FROM %s;"
	countSessions  = "SELECT COUNT(*)::int FROM devices WHERE session_id != '';"
)

var TestEncryptionKey = []byte("1234567890abcdef")

type TestQuerier interface {
	GetDbConnection(ctx context.Context) *pgxpool.Pool
}

func IsTestUnitRun() bool {
	return os.Getenv(unitEnvVar) != ""
}

func IsIntegrationTest() bool {
	return os.Getenv(integrationEnvVar) != ""
}

func IsRESTFuzzTest() bool {
	return os.Getenv(restFuzzEnvVar) != ""
}

func IsRESTHandlersLocalTest() bool {
	return os.Getenv(restHandlersLocalEnvVar) != ""
}

func SkipOrSetupIntegrationTest(t *testing.T, ctx context.Context, q TestQuerier) {
	if !IsIntegrationTest() {
		t.Skipf("skipping integration test: to run this test set %s envvar to 1", integrationEnvVar)
	}

	err := TruncateAllTables(ctx, q.GetDbConnection(ctx))
	require.NoError(t, err)
}

func SkipOrSetupRESTHandlersLocalTest(t *testing.T, ctx context.Context, q TestQuerier) {
	if !IsRESTHandlersLocalTest() {
		t.Skipf("skipping handlers local test: to run this test set %s envvar to 1", restHandlersLocalEnvVar)
	}

	err := TruncateAllTables(ctx, q.GetDbConnection(ctx))
	require.NoError(t, err)
}

func PostgresConfig() *config.PostgresConfig {
	return &config.PostgresConfig{
		Host:         Host(),
		Port:         5433,
		Database:     "entguard_test",
		User:         "entguard_test",
		Password:     "test",
		ConnPoolSize: config.DefaultPostgresConfig().ConnPoolSize,
	}
}

func Host() string {
	host := Localhost
	// This is needed to run tests on CI server
	if h := os.Getenv("DOCKER_HOST"); h != "" {
		h, ok := strings.CutPrefix(h, "tcp://")
		if !ok {
			_ = os.Setenv("DOCKER_HOST", "tcp://"+h)
		}
		host = strings.Split(h, ":")[0]
	}
	return host
}

func TruncateAllTables(ctx context.Context, conn *pgxpool.Pool) error {
	tables, err := getAllTestTables(ctx, conn)
	if err != nil {
		return fmt.Errorf("getAllTestTables: %w", err)
	}

	if len(tables) == 0 {
		return errors.New("no tables to truncate (migrations are not executed?)")
	}

	stmt := "TRUNCATE TABLE " + strings.Join(tables, ", ") + " RESTART IDENTITY CASCADE;"
	if _, err := conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("truncate database tables: %w", err)
	}

	return nil
}

func getAllTestTables(ctx context.Context, conn *pgxpool.Pool) ([]string, error) {
	allTablesQuery := "SELECT tablename FROM pg_tables WHERE schemaname = current_schema()"
	rows, err := conn.Query(context.Background(), allTablesQuery)
	if err != nil {
		return nil, fmt.Errorf("get all table names: %w", err)
	}
	defer rows.Close()

	var tables []string

	for rows.Next() {
		var tableName string
		err := rows.Scan(&tableName)
		if err != nil {
			return nil, fmt.Errorf("scan rows to get all table names: %w", err)
		}

		if tableName == "migrations" {
			continue
		}
		tables = append(tables, tableName)
	}
	if rows.Err() != nil {
		return nil, err
	}
	return tables, nil
}

func CountTableRows(t *testing.T, q *pgxpool.Pool, tableName string) int {
	var count int
	err := q.QueryRow(
		context.Background(),
		fmt.Sprintf(countTableRows, tableName),
	).Scan(&count)
	require.NoError(t, err)

	return count
}

func CountSessions(t *testing.T, q *pgxpool.Pool) int {
	var count int
	err := q.QueryRow(
		context.Background(),
		countSessions,
	).Scan(&count)
	require.NoError(t, err)

	return count
}

func OverwriteDBValue(t *testing.T, q *pgxpool.Pool, tableName string, rowID int, colName string, value any) {
	_, err := q.Exec(
		context.Background(),
		fmt.Sprintf("UPDATE %s SET %s=$1 WHERE id = $2", tableName, colName),
		value, rowID,
	)
	require.NoError(t, err)
}

func VPNConfigQuerierToPostgresQuerier(ctx context.Context, q *pg.VPNConfigQuerier) *user.PostgresQuerier {
	return &user.PostgresQuerier{
		Conn:   q.Conn,
		Logger: q.Logger,
	}
}

func PostgresQueriesToVPNConfigQuerier(ctx context.Context, q *user.PostgresQuerier) *pg.VPNConfigQuerier {
	return &pg.VPNConfigQuerier{
		Conn:   q.Conn,
		Logger: q.Logger,
	}
}

func NewSamplePostgresManager(q user.Querier, l *log.Logger, ldap user.LdapService, vcm vcm.VPNConfigManager, sqlcQuerier *sqlc.Queries) (*user.PostgresManager, error) {
	cfg, err := config.FromEnv()
	if err != nil {
		return nil, err
	}
	cfg.UserManager.MFAEnabledTypes = []mfa.AuthenticatorType{mfa.CERT, mfa.TOTP}
	cfg.Features = &config.FeaturesMock{}

	pm := &user.PostgresManager{
		DB:            q,
		SQLcQ:         sqlcQuerier,
		Cfg:           cfg.UserManager,
		PwdCfg:        cfg.UserManager.PwdCfg,
		Feats:         cfg.Features,
		TotpMfa:       &mfa.TOTPAuthenticator{},
		CertMfa:       &mfa.CERTAuthenticator{},
		LdapResync:    make(chan struct{}),
		LdapStats:     &user.LdapSyncStats{},
		Logger:        l.WithField("reportCaller", "running manager tests"),
		Vcm:           vcm,
		LdapService:   ldap,
		EncryptionKey: TestEncryptionKey,
	}

	return pm, nil
}

// Seal is a convenience wrapper over aesgcm.Seal for use in tests.
// It calls aesgcm.Seal with test.TestEncryptionKey and panics on error.
func Seal(plaintext string) aesgcm.Ciphertext {
	c, err := aesgcm.Seal(TestEncryptionKey, plaintext)
	if err != nil {
		panic(err)
	}
	return c
}

// OpenString is a convenience wrapper over aesgcm.OpenString for use in tests.
// It calls aesgcm.OpenString with test.TestEncryptionKey and panics on error.
func OpenString(ciphertext aesgcm.Ciphertext) string {
	p, err := aesgcm.OpenString(TestEncryptionKey, ciphertext)
	if err != nil {
		panic(err)
	}
	return p
}
