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

import (
	"errors"
	"fmt"
	"strconv"
)

type LdapServiceMock struct {
	// A central storage of all users.
	usersForLDAPs map[string][]*LdapUser
	// Common UID counter.
	uid int
}

type ldapConnMock struct {
	users []*LdapUser
}

func NewLdapServiceMock() *LdapServiceMock {
	return &LdapServiceMock{
		usersForLDAPs: make(map[string][]*LdapUser),
		uid:           0,
	}
}

func (service *LdapServiceMock) AddUser(host, username string) {
	if host == "" {
		return
	}
	service.uid += 1
	u := &LdapUser{
		UID:      strconv.Itoa(service.uid),
		Username: username,
		DN:       fmt.Sprintf("cn=%s,dc=%s,dc=entguard", username, host),
	}
	service.usersForLDAPs[host] = append(service.usersForLDAPs[host], u)
}

func (service *LdapServiceMock) DeleteUser(host, username string) {
	users := service.usersForLDAPs[host]
	for i := range users {
		if users[i].Username == username {
			users = append(users[:i], users[i+1:]...)
			break
		}
	}
	service.usersForLDAPs[host] = users
}

func (service *LdapServiceMock) UpdateUser(host, usernameOld, usernameNew string) {
	for _, u := range service.usersForLDAPs[host] {
		if u.Username == usernameOld {
			u.Username = usernameNew
			u.DN = fmt.Sprintf("cn=%s,dc=%s,dc=entguard", usernameNew, host)
			break
		}
	}
}

func (service *LdapServiceMock) CreateConnection(host string, port int, bindDN, bindPW string, cfg *LDAPSConfig) (LdapConn, error) {
	users, ok := service.usersForLDAPs[host]
	if !ok {
		return nil, errors.New("dummy error")
	}
	return &ldapConnMock{users}, nil
}

func (service *LdapServiceMock) VerifyConnection(host string, port int, bindDN, bindPW string, cfg *LDAPSConfig) error {
	_, ok := service.usersForLDAPs[host]
	if !ok {
		return errors.New("dummy error")
	}
	return nil
}

func (l *ldapConnMock) FetchUsers(opts *FetchUsersOptions) ([]*LdapUser, error) {
	return l.users, nil
}

func (l *ldapConnMock) Close() {}
func (l *ldapConnMock) IsClosed() bool {
	return false
}
