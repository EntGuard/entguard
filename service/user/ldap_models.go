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
	"time"

	sqlc "github.com/entguard/entguard/db"
	"github.com/entguard/entguard/pkg/crypto/aesgcm"
	"github.com/entguard/entguard/pkg/db"
	"github.com/entguard/entguard/service/api/v1"
)

type LdapConfig struct {
	ID       string
	Priority int
	Host     string
	Port     int

	BaseDN string

	BindDN          string
	BindPWEncrypted aesgcm.Ciphertext

	UserListFilter string

	UsernameAttr string
	UIDAttr      string

	UseTLS bool
	CACert []byte
	FQDN   string

	TemplateID int
}

func (lc *LdapConfig) ToAPILdapConfigGet() *api.LdapConfigGet {
	return &api.LdapConfigGet{
		ID:             lc.ID,
		Priority:       lc.Priority,
		Host:           lc.Host,
		Port:           lc.Port,
		BaseDN:         lc.BaseDN,
		BindDN:         lc.BindDN,
		UserListFilter: lc.UserListFilter,
		UsernameAttr:   lc.UsernameAttr,
		UIDAttr:        lc.UIDAttr,
		UseTLS:         lc.UseTLS,
		FQDN:           lc.FQDN,
		CACert:         string(lc.CACert),
		TemplateID:     lc.TemplateID,
	}
}

func DBAllRowsToLdapConfig(lc sqlc.GetAllLdapConfigsRow) *LdapConfig {
	return &LdapConfig{
		ID:              lc.ID,
		Priority:        lc.Priority,
		Host:            lc.Host,
		Port:            lc.Port,
		BaseDN:          lc.BaseDn,
		BindDN:          lc.BindDn,
		BindPWEncrypted: lc.BindPwEncrypted,
		UserListFilter:  lc.UserListFilter,
		UsernameAttr:    lc.UsernameAttribute,
		UIDAttr:         lc.UidAttribute,
		UseTLS:          lc.UseTls,
		FQDN:            lc.Fqdn,
		CACert:          lc.CaCert,
		TemplateID:      lc.TemplateID,
	}
}

func DBRowToLdapConfig(lc sqlc.GetLdapConfigByIDRow, id string) *LdapConfig {
	return &LdapConfig{
		ID:              id,
		Priority:        lc.Priority,
		Host:            lc.Host,
		Port:            lc.Port,
		BaseDN:          lc.BaseDn,
		BindDN:          lc.BindDn,
		BindPWEncrypted: lc.BindPwEncrypted,
		UserListFilter:  lc.UserListFilter,
		UsernameAttr:    lc.UsernameAttribute,
		UIDAttr:         lc.UidAttribute,
		UseTLS:          lc.UseTls,
		FQDN:            lc.Fqdn,
		CACert:          lc.CaCert,
		TemplateID:      lc.TemplateID,
	}
}

func DBInsertParamsToLdapConfig(lc sqlc.InsertLdapConfigParams, id string) *LdapConfig {
	return &LdapConfig{
		ID:              id,
		Priority:        lc.Priority,
		Host:            lc.Host,
		Port:            lc.Port,
		BaseDN:          lc.BaseDn,
		BindDN:          lc.BindDn,
		BindPWEncrypted: lc.BindPwEncrypted,
		UserListFilter:  lc.UserListFilter,
		UsernameAttr:    lc.UsernameAttribute,
		UIDAttr:         lc.UidAttribute,
		UseTLS:          lc.UseTls,
		FQDN:            lc.Fqdn,
		CACert:          lc.CaCert,
		TemplateID:      db.IntFromPtr(lc.TemplateID),
	}
}

func (lc *LdapConfig) ToDBInsertParams() sqlc.InsertLdapConfigParams {
	return sqlc.InsertLdapConfigParams{
		Priority:          lc.Priority,
		Host:              lc.Host,
		Port:              lc.Port,
		BaseDn:            lc.BaseDN,
		BindDn:            lc.BindDN,
		BindPwEncrypted:   lc.BindPWEncrypted,
		UserListFilter:    lc.UserListFilter,
		UsernameAttribute: lc.UsernameAttr,
		UidAttribute:      lc.UIDAttr,
		UseTls:            lc.UseTLS,
		Fqdn:              lc.FQDN,
		CaCert:            lc.CACert,
		TemplateID:        db.IntToPtr(lc.TemplateID),
	}
}

func (lc *LdapConfig) ToDBUpdateParams() sqlc.UpdateLdapConfigParams {
	return sqlc.UpdateLdapConfigParams{
		Priority:          lc.Priority,
		Host:              lc.Host,
		Port:              lc.Port,
		BaseDn:            lc.BaseDN,
		BindDn:            lc.BindDN,
		BindPwEncrypted:   lc.BindPWEncrypted,
		UserListFilter:    lc.UserListFilter,
		UsernameAttribute: lc.UsernameAttr,
		UidAttribute:      lc.UIDAttr,
		UseTls:            lc.UseTLS,
		Fqdn:              lc.FQDN,
		CaCert:            lc.CACert,
		TemplateID:        db.IntToPtr(lc.TemplateID),
		ID:                lc.ID,
	}
}

func ToAPILdapTemplate(tl sqlc.LdapTemplate, aps []sqlc.AddressPool) *api.LdapTemplate {
	var addressPools []*api.AddressPool
	for _, p := range aps {
		addressPools = append(addressPools, ConvertDBToAPIPool(p))
	}

	result := &api.LdapTemplate{
		ID:            tl.ID,
		Name:          tl.Name,
		IsAdmin:       tl.IsAdmin,
		MFAType:       string(tl.MfaAuth),
		InterfaceName: tl.InterfaceName,
		AddressPools:  addressPools,
		DNS:           tl.Dns,
	}
	if tl.Filter != nil {
		result.Filter = *tl.Filter
	}
	if tl.ListenPort != nil {
		result.ListenPort = *tl.ListenPort
	}
	if tl.Mtu != nil {
		result.MTU = *tl.Mtu
	}

	return result
}

type LdapTemplateServer struct {
	ServerName           string
	ServerID             int
	OldServerID          int
	LdapTemplateID       int
	ClientSideAllowedIPs []string
	UsePresharedKey      bool
}

func (m *LdapTemplateServer) ToAPI() *api.LdapTemplateServer {
	return &api.LdapTemplateServer{
		ServerID:             m.ServerID,
		ServerName:           m.ServerName,
		ClientSideAllowedIPs: m.ClientSideAllowedIPs,
		UsePresharedKey:      m.UsePresharedKey,
	}
}

type LdapRelation struct {
	ID        int
	UserID    int
	LdapID    string
	UserUID   string
	UserDN    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type LdapUserAuth struct {
	Priority int
	Host     string
	Port     int
	UserDN   string

	UseTLS bool
	CACert []byte
	FQDN   string
}

type LdapUserPriority struct {
	Priority int
	UserID   int
	LdapID   string
}
