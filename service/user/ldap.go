/*
 * Copyright 2021 PANTHEON.tech s.r.o.
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

type LdapService interface {
	CreateConnection(host string, port int, bindDN, bindPW string, cfg *LDAPSConfig) (LdapConn, error)
	VerifyConnection(host string, port int, bindDN, bindPW string, cfg *LDAPSConfig) error
}

type LdapConn interface {
	FetchUsers(opts *FetchUsersOptions) ([]*LdapUser, error)
	Close()
	IsClosed() bool
}

type LdapUser struct {
	UID      string // Unique ID retrieved from LDAP server.
	Username string // Username retrieved from LDAP server.
	DN       string // DN retrieved from LDAP server.
}

type FetchUsersOptions struct {
	BaseDN       string
	Filter       string
	UIDAttr      string
	UsernameAttr string
}

type LDAPSConfig struct {
	UseTLS bool
	FQDN   string
	CACert []byte
}
