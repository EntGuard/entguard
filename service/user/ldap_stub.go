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

package user

import (
	"context"
	"errors"

	"github.com/entguard/entguard/service/api/v1"
)

var ErrLDAPNotSupported = errors.New("LDAP integration is available in the enterprise edition")

// ldapServiceStub is the open-source LDAP client.
//
// LDAP/AD directory integration is an enterprise-only feature. This open-source
// stub client performs no directory I/O; every operation reports that LDAP is
// an enterprise feature.
type ldapServiceStub struct{}

func NewLdapService() LdapService {
	return &ldapServiceStub{}
}

func (*ldapServiceStub) CreateConnection(host string, port int, bindDN, bindPW string, cfg *LDAPSConfig) (LdapConn, error) {
	return nil, ErrLDAPNotSupported
}

func (*ldapServiceStub) VerifyConnection(host string, port int, bindDN, bindPW string, cfg *LDAPSConfig) error {
	return ErrLDAPNotSupported
}

type LdapSyncStats struct {
	// stub
}

func (m *PostgresManager) startLdapSync(ctx context.Context) {
	// no-op
}

func (m *PostgresManager) GetLdapSyncStats(ctx context.Context) (*api.LdapSyncStatus, error) {
	return nil, ErrLDAPNotSupported
}

func (m *PostgresManager) Resync(ctx context.Context) error {
	return ErrLDAPNotSupported
}
