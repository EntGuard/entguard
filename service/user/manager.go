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

package user

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"

	sqlc "github.com/entguard/entguard/db"
	"github.com/entguard/entguard/pkg/addresspools"
	"github.com/entguard/entguard/pkg/crypto/aesgcm"
	"github.com/entguard/entguard/pkg/db"
	"github.com/entguard/entguard/pkg/mfa"
	"github.com/entguard/entguard/service/api/v1"
	"github.com/entguard/entguard/service/config"
	"github.com/entguard/entguard/service/vcm"
	"github.com/entguard/entguard/service/vcm/drivers/postgres"
)

const (
	FQDNMaxSize = 255

	managerCaller = "user manager"
)

type Manager interface {
	GetAllUsers(ctx context.Context) (*api.UserList, error)
	GetUserByName(ctx context.Context, username string) (*api.User, error)
	GetUserByID(ctx context.Context, id int) (*api.User, error)
	GetUserByNameAndPassword(ctx context.Context, username, password string) (*api.User, error)
	CreateUser(ctx context.Context, u *api.UserWithPassword) error
	UpdateUser(ctx context.Context, id int, u *api.UserUpdate) (*api.User, error)
	UpdateUserPassword(ctx context.Context, id int, password string) error
	DeleteUser(ctx context.Context, id int) error

	// LDAP management
	GetAllLdapConfigs(ctx context.Context) (*api.LdapConfigList, error)
	GetLdapConfig(ctx context.Context, id string) (*api.LdapConfigGet, error)
	CreateLdapConfig(ctx context.Context, lc *api.LdapConfig) (*api.LdapConfigGet, error)
	UpdateLdapConfig(ctx context.Context, lc *api.LdapConfig, id string) error
	DeleteLdapConfig(ctx context.Context, id string) error

	GetAllLdapTemplates(ctx context.Context) ([]*api.LdapTemplate, error)
	CheckLdapTemplateExists(ctx context.Context, id int) (bool, error)
	GetLdapTemplate(ctx context.Context, id int) (*api.LdapTemplate, error)
	CreateLdapTemplate(ctx context.Context, lt *api.LdapTemplate) (int, error)
	UpdateLdapTemplate(ctx context.Context, id int, lt *api.LdapTemplate) error
	DeleteLdapTemplate(ctx context.Context, id int) error

	GetLdapTemplateServers(ctx context.Context, templateID int) (*api.LdapTemplateServerList, error)
	CreateLdapTemplateServer(ctx context.Context, templateID int, lts *api.LdapTemplateServer) error
	UpdateLdapTemplateServer(ctx context.Context, templateID int, oldServerID int, lts *api.LdapTemplateServer) error
	DeleteLdapTemplateServer(ctx context.Context, templateID int, serverID int) error

	GetLdapSyncStats(ctx context.Context) (*api.LdapSyncStatus, error)
	Resync(ctx context.Context) error

	// MFA management
	MFAGetEnabledTypes(ctx context.Context) *api.MfaEnabledTypes
	MFAAuthenticate(ctx context.Context, userID int, mfaType mfa.AuthenticatorType, creds interface{}) (bool, error)

	MFACreateTOTPSecret(ctx context.Context, userID int) (*mfa.TOTPSecret, error)
	MFAUpdateTOTPSecret(ctx context.Context, userID int) (*mfa.TOTPSecret, error)
	MFAGetTOTPSecret(ctx context.Context, userID int) (*mfa.TOTPSecret, error)

	MFAGetAllCAs(ctx context.Context) (*api.MfaCAList, error)
	MFACreateCA(ctx context.Context, ca []byte) error
	MFADeleteCA(ctx context.Context, id int) error

	MFAGetAllCRLs(ctx context.Context) (*api.MfaCRLList, error)
	MFACreateCRL(ctx context.Context, crl []byte) error
	MFADeleteCRL(ctx context.Context, id int) error

	GetHealthcheckVpnCPort() string

	// Address Pools
	GetAllAddressPools(ctx context.Context) ([]*api.AddressPool, error)
	GetAddressPoolByID(ctx context.Context, id int) (*api.AddressPool, error)
	CreateAddressPool(ctx context.Context, pool *api.AddressPool) (*api.AddressPool, error)
	UpdateAddressPool(ctx context.Context, id int, pool *api.AddressPool) (*api.AddressPool, error)
	DeleteAddressPool(ctx context.Context, id int) error
	ValidateAddressPool(p *api.AddressPool) error

	// Devices
	GetDevice(ctx context.Context, id int) (*api.Device, error)
	GetDeviceByExternalIDAndUserID(ctx context.Context, externalDeviceID string, userID int) (*api.Device, error)
	GetDevicesByTemplateID(ctx context.Context, deviceTemplateID int) ([]*api.Device, error)
	GetDevicesByUserID(ctx context.Context, userID int) ([]*api.Device, error)
	GenerateDeviceData(ctx context.Context, deviceTemplateID int) (*api.DeviceData, error)
	CreateDeviceWithAutogenData(ctx context.Context, deviceTemplateID int, externalDeviceID, deviceInformation string) (*api.Device, error)
	CreateDevice(ctx context.Context, deviceTemplateID int, data *api.DeviceData) (*api.Device, error)
	UpdateDevice(ctx context.Context, id int, du api.DeviceData) error
	DeleteDevice(ctx context.Context, id int) error
	ResyncDevices(ctx context.Context, deviceTemplateID int) (*api.DeviceResyncResponse, error)
	AssignDeviceSlot(ctx context.Context, userID int, session, externalDeviceID, deviceInformation string) (string, error)

	// Sessions
	GetAllDevicesSessions(ctx context.Context) ([]string, error)
	InvalidateOldDevicesSessions(ctx context.Context, threshold time.Time) (int, error)
	DeleteInvalidSessionsByDevice(ctx context.Context, deviceID int, old, new *api.DeviceData) error

	EnforceDPULimit(ctx context.Context) error
}

type PostgresManager struct {
	DB          Querier
	SQLcQ       *sqlc.Queries
	LdapService LdapService
	Cfg         *config.UserManagementConfig
	PwdCfg      *config.PasswordConfig
	Feats       config.FeaturesData

	CertMfa *mfa.CERTAuthenticator
	TotpMfa *mfa.TOTPAuthenticator

	LdapStats *LdapSyncStats
	Mtx       sync.Mutex

	LdapResync chan struct{}
	Logger     *log.Entry

	Vcm vcm.VPNConfigManager

	EncryptionKey []byte
}

func NewPostgresManager(ctx context.Context, cfg *config.UserManagementConfig, vcm vcm.VPNConfigManager, conn *pgxpool.Pool, l *log.Logger, encryptionKey []byte) (Manager, error) {
	dbQuerier := NewPostgresQuerier(ctx, conn, l)
	sqlcQuerier := sqlc.New(conn)
	m := &PostgresManager{
		DB:            dbQuerier,
		SQLcQ:         sqlcQuerier,
		Cfg:           cfg,
		PwdCfg:        cfg.PwdCfg,
		Feats:         cfg.Features,
		TotpMfa:       &mfa.TOTPAuthenticator{},
		CertMfa:       &mfa.CERTAuthenticator{},
		LdapResync:    make(chan struct{}),
		LdapStats:     &LdapSyncStats{},
		Logger:        l.WithField("reportCaller", managerCaller),
		Vcm:           vcm,
		EncryptionKey: encryptionKey,
	}

	go m.startLdapSync(ctx)
	go m.startSessionInvalidation(ctx)

	return m, nil
}

func (m *PostgresManager) GetAllUsers(ctx context.Context) (*api.UserList, error) {
	users, err := m.SQLcQ.GetAllUsers(ctx)
	if err != nil {
		return nil, err
	}

	ul := &api.UserList{}
	for _, u := range users {
		ul.Users = append(ul.Users, ConvertDBToAPIUser(u))
	}
	return ul, nil
}

func (m *PostgresManager) GetUserByName(ctx context.Context, username string) (*api.User, error) {
	u, err := m.DB.GetUserByName(ctx, username)
	if err != nil {
		return nil, err
	}
	apiUser := u.ToAPI()
	return &apiUser, nil
}

func (m *PostgresManager) GetUserByID(ctx context.Context, id int) (*api.User, error) {
	u, err := m.DB.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	apiUser := u.ToAPI()
	return &apiUser, nil
}

func (m *PostgresManager) GetUserByNameAndPassword(ctx context.Context, username, password string) (*api.User, error) {
	u, err := m.DB.GetUserByName(ctx, username)
	if err != nil {
		return nil, err
	}

	if u.IsInternal() {
		ok := u.ComparePasswords(password)
		if !ok {
			return nil, ErrUserNotFound
		}
	} else if u.IsLDAP() {
		ldapAuths, err := m.DB.GetLdapUserAuthsByUserID(ctx, u.ID)
		if err != nil {
			return nil, ErrUserNotFound
		}

		m.Logger.Infof("Trying to authenticate user against %d LDAP servers", len(ldapAuths))
		var isLDAPSuccess bool
		for _, la := range ldapAuths {
			cfg := LDAPSConfig{
				UseTLS: la.UseTLS,
				FQDN:   la.FQDN,
				CACert: la.CACert,
			}
			err = m.LdapService.VerifyConnection(la.Host, la.Port, la.UserDN, password, &cfg)
			if err == nil {
				isLDAPSuccess = true
				break
			}
			m.Logger.WithError(err).Info("Failed to connect to LDAP server with address: ",
				net.JoinHostPort(la.Host, strconv.Itoa(la.Port)))
		}
		if !isLDAPSuccess {
			return nil, ErrUserNotFound
		}
	}

	apiUser := u.ToAPI()
	return &apiUser, nil
}

func (m *PostgresManager) CreateUser(ctx context.Context, apiUser *api.UserWithPassword) error {
	err := m.ensureUserCanBeAdded(ctx)
	if err != nil {
		return err
	}
	exists, err := m.DB.CheckUsernameExists(ctx, apiUser.Username)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%w: user with that username already exists", ErrBadUserData)
	}

	u, err := NewUserModelWithName(apiUser.Username)
	if err != nil {
		return err
	}

	err = ValidatePassword(apiUser.Password, m.PwdCfg.MinLength, m.PwdCfg.Uppercase, m.PwdCfg.Lowercase, m.PwdCfg.Digit, m.PwdCfg.Symbol)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadUserData, err)
	}

	err = u.SetPassword(apiUser.Password)
	if err != nil {
		return err
	}
	u.SetAdmin(apiUser.IsAdmin)

	err = m.ensureMFACanBeSet(mfa.AuthenticatorType(apiUser.MFAType))
	if err != nil {
		return fmt.Errorf("%w: %v, suggested type: %s", ErrBadUserData, err, u.MfaAuthType)
	}
	u.SetMFAType(apiUser.MFAType)

	err = u.SetNotification(apiUser.Notification)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadUserData, err)
	}

	if err := m.DB.InsertUser(ctx, u); err != nil {
		return err
	}
	m.Logger.
		Info("User created.")
	return nil
}

func (m *PostgresManager) UpdateUser(ctx context.Context, id int, apiUser *api.UserUpdate) (*api.User, error) {
	u, err := m.DB.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}

	uOld := *u

	if u.Name != apiUser.Username {
		if u.IsNotInternal() {
			return nil, fmt.Errorf("%w: username cannot be changed", ErrOnlyForIntrUsers)
		}
		exists, err := m.DB.CheckUsernameExists(ctx, apiUser.Username)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, fmt.Errorf("%w: user with that username already exists", ErrBadUserData)
		}
		err = u.SetName(apiUser.Username)
		if err != nil {
			return nil, err
		}
	}

	u.SetAdmin(apiUser.IsAdmin)

	err = m.ensureMFACanBeSet(mfa.AuthenticatorType(apiUser.MFAType))
	if err != nil {
		return nil, fmt.Errorf("%w: %v, suggested type: %s", ErrBadUserData, err, u.MfaAuthType)
	}
	u.SetMFAType(apiUser.MFAType)

	err = u.SetNotification(apiUser.Notification)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadUserData, err)
	}

	err = m.DB.UpdateUser(ctx, u)
	if err != nil {
		return nil, err
	}

	m.Logger.Info("User updated")

	if err := m.DeleteInvalidSessionsByUser(ctx, &uOld, u); err != nil {
		return nil, err
	}

	updatedAPIUser := u.ToAPI()
	return &updatedAPIUser, nil
}

func (m *PostgresManager) UpdateUserPassword(ctx context.Context, id int, password string) error {
	u, err := m.DB.GetUserByID(ctx, id)
	if err != nil {
		return err
	}
	if u.IsNotInternal() {
		return fmt.Errorf("%w: password cannot be changed", ErrOnlyForIntrUsers)
	}
	err = ValidatePassword(password, m.PwdCfg.MinLength, m.PwdCfg.Uppercase, m.PwdCfg.Lowercase, m.PwdCfg.Digit, m.PwdCfg.Symbol)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadUserData, err)
	}
	err = u.SetPassword(password)
	if err != nil {
		return err
	}
	err = m.DB.UpdateUserPassword(ctx, u.ID, u.PasswordHash)
	if err != nil {
		return err
	}
	m.Logger.Info("User password updated")

	if err := m.DeleteInvalidSessionsByUserPassword(ctx, u); err != nil {
		return err
	}

	return nil
}

func (m *PostgresManager) DeleteUser(ctx context.Context, id int) error {
	u, err := NewUserModelWithID(id)
	if err != nil {
		return err
	}
	err = m.Vcm.DeleteDeviceTemplateByUserID(ctx, u.ID)
	if err != nil {
		return err
	}
	n, err := m.SQLcQ.DeleteUser(ctx, u.ID)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrUserNotFound
	}
	m.Logger.Infof("User %d is deleted", u.ID)
	return nil
}

func (m *PostgresManager) GetAllLdapConfigs(ctx context.Context) (*api.LdapConfigList, error) {
	allLdapsDB, err := m.SQLcQ.GetAllLdapConfigs(ctx)
	if err != nil {
		return nil, err
	}
	configList := &api.LdapConfigList{
		LdapConfigs: make([]*api.LdapConfigGet, 0, len(allLdapsDB)),
	}
	for _, item := range allLdapsDB {
		configList.LdapConfigs = append(configList.LdapConfigs, DBAllRowsToLdapConfig(item).ToAPILdapConfigGet())
	}
	return configList, nil
}

func (m *PostgresManager) GetLdapConfig(ctx context.Context, id string) (*api.LdapConfigGet, error) {
	lcDB, err := m.SQLcQ.GetLdapConfigByID(ctx, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("ldap configuration %w", ErrNotFound)
		}
		return nil, err
	}

	return DBRowToLdapConfig(lcDB, id).ToAPILdapConfigGet(), nil
}

func (m *PostgresManager) CreateLdapConfig(ctx context.Context, lc *api.LdapConfig) (*api.LdapConfigGet, error) {
	err := validateLdapConfig(lc)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidData, err)
	}
	err = m.DB.VerifyLdapConfigPriority(ctx, lc.Priority)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidData, err)
	}

	bindPWEnc, err := aesgcm.Seal(m.EncryptionKey, lc.BindPW)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt LDAP bind password: %v", err)
	}
	ldapConfig := sqlc.InsertLdapConfigParams{
		Priority:          lc.Priority,
		Host:              lc.Host,
		Port:              lc.Port,
		BaseDn:            lc.BaseDN,
		BindDn:            lc.BindDN,
		BindPwEncrypted:   bindPWEnc,
		UserListFilter:    lc.UserListFilter,
		UsernameAttribute: lc.UsernameAttr,
		UidAttribute:      lc.UIDAttr,
		UseTls:            lc.UseTLS,
		Fqdn:              lc.FQDN,
		CaCert:            []byte(lc.CACert),
		TemplateID:        db.IntToPtr(lc.TemplateID),
	}
	ret, err := m.SQLcQ.InsertLdapConfig(ctx, ldapConfig)
	if err != nil {
		return nil, err
	}
	return DBInsertParamsToLdapConfig(ldapConfig, ret.ID).ToAPILdapConfigGet(), nil
}

func (m *PostgresManager) UpdateLdapConfig(ctx context.Context, lc *api.LdapConfig, id string) error {
	err := validateLdapConfig(lc)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidData, err)
	}

	oldLC, err := m.SQLcQ.GetLdapConfigByID(ctx, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("ldap configuration %w", ErrNotFound)
		}
		return err
	}

	if lc.Priority != oldLC.Priority {
		err = m.DB.VerifyLdapConfigPriority(ctx, lc.Priority)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidData, err)
		}
	}

	passwordEnc := oldLC.BindPwEncrypted
	if len(lc.BindPW) != 0 {
		passwordEnc, err = aesgcm.Seal(m.EncryptionKey, lc.BindPW)
		if err != nil {
			return fmt.Errorf("failed to encrypt LDAP bind password: %v", err)
		}
	}

	ldapConfig := sqlc.UpdateLdapConfigParams{
		Priority:          lc.Priority,
		Host:              lc.Host,
		Port:              lc.Port,
		BaseDn:            lc.BaseDN,
		BindDn:            lc.BindDN,
		BindPwEncrypted:   passwordEnc,
		UserListFilter:    lc.UserListFilter,
		UsernameAttribute: lc.UsernameAttr,
		UidAttribute:      lc.UIDAttr,
		UseTls:            lc.UseTLS,
		Fqdn:              lc.FQDN,
		CaCert:            []byte(lc.CACert),
		TemplateID:        db.IntToPtr(lc.TemplateID),
		ID:                id,
	}
	_, err = m.SQLcQ.UpdateLdapConfig(ctx, ldapConfig)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("ldap configuration %w", ErrNotFound)
		}
		return err
	}
	return nil
}

func (m *PostgresManager) DeleteLdapConfig(ctx context.Context, id string) error {
	return m.DB.DeleteLdapConfig(ctx, id)
}

func (m *PostgresManager) CheckLdapTemplateExists(ctx context.Context, id int) (bool, error) {
	exists, err := m.SQLcQ.LdapTemplateIDCheck(ctx, id)
	if err != nil {
		return false, err
	}

	return exists, nil
}

func (m *PostgresManager) GetAllLdapTemplates(ctx context.Context) ([]*api.LdapTemplate, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	allTemplates, err := m.SQLcQ.GetAllLdapTemplates(queryCtx)
	if err != nil {
		return nil, fmt.Errorf("%w: GetAllLdapTemplates: %w", ErrQueryFailed, err)
	}

	ldapTemplates := make([]*api.LdapTemplate, 0, len(allTemplates))
	for _, lt := range allTemplates {
		ads, err := m.SQLcQ.GetLdapTemplateAddressPools(queryCtx, lt.ID)
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil, ErrNotFound
			}
			return nil, fmt.Errorf("%w: GetLdapTemplateAddressPools: %w", ErrQueryFailed, err)
		}

		ldapTemplates = append(ldapTemplates, ToAPILdapTemplate(lt, ads))
	}

	return ldapTemplates, nil
}

func (m *PostgresManager) GetLdapTemplate(ctx context.Context, id int) (*api.LdapTemplate, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	lt, err := m.SQLcQ.GetLdapTemplateByID(queryCtx, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: GetLdapTemplateByID: %w", ErrQueryFailed, err)
	}

	ads, err := m.SQLcQ.GetLdapTemplateAddressPools(queryCtx, lt.ID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: GetLdapTemplateAddressPools: %w", ErrQueryFailed, err)
	}

	return ToAPILdapTemplate(lt, ads), nil
}

func (m *PostgresManager) CreateLdapTemplate(ctx context.Context, lt *api.LdapTemplate) (int, error) {
	if lt.InterfaceName == "" {
		return 0, fmt.Errorf("%w: %s", ErrInvalidData, "interface name can not be empty")
	}
	if len(lt.AddressPools) == 0 {
		return 0, fmt.Errorf("%w: %s", ErrInvalidData, "address pools can not be empty")
	}

	err := m.ensureMFACanBeSet(mfa.AuthenticatorType(lt.MFAType))
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidData, err)
	}

	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	exists, err := m.SQLcQ.LdapTemplateNameCheck(queryCtx, lt.Name)
	if err != nil {
		return 0, err
	}
	if exists {
		return 0, vcm.ErrAlreadyExists
	}

	pools := make([]sqlc.AddressPool, 0, len(lt.AddressPools))
	for _, ap := range lt.AddressPools {
		pool, err := m.SQLcQ.GetAddressPoolByID(queryCtx, ap.ID)
		if err != nil {
			if err == pgx.ErrNoRows {
				return 0, fmt.Errorf("%w: address pool with ID %d does not exist", ErrInvalidData, ap.ID)
			}
			return 0, fmt.Errorf("%w: GetAddressPoolByID: %v", ErrQueryFailed, err)
		}
		pools = append(pools, pool)
	}

	if len(pools) > 1 {
		if err := addresspools.VerifyPoolsNotOverlap(pools); err != nil {
			return 0, fmt.Errorf("%w: VerifyPoolsNotOverlap: %v", ErrInvalidData, err)
		}
	}

	MFAType := lt.MFAType
	if len(lt.MFAType) == 0 {
		MFAType = string(mfa.NONE)
	}

	insertParam := sqlc.InsertLdapTemplateParams{
		Name:          lt.Name,
		InterfaceName: lt.InterfaceName,
		IsAdmin:       lt.IsAdmin,
		Dns:           lt.DNS,
		MfaAuth:       sqlc.MfaAuthType(MFAType),
	}
	if lt.Filter != "" {
		insertParam.Filter = &lt.Filter
	}
	if lt.ListenPort != "" {
		insertParam.ListenPort = &lt.ListenPort
	}
	if lt.MTU != "" {
		insertParam.Mtu = &lt.MTU
	}

	result, err := m.SQLcQ.InsertLdapTemplate(queryCtx, insertParam)
	if err != nil {
		return 0, fmt.Errorf("%w: InsertLdapTemplate: %v", ErrQueryFailed, err)
	}

	for _, ap := range lt.AddressPools {
		err = m.SQLcQ.InsertLdapTemplateAddressPool(queryCtx, sqlc.InsertLdapTemplateAddressPoolParams{
			LdapTemplateID: result.ID,
			AddressPoolID:  ap.ID,
		})
		if err != nil {
			return 0, fmt.Errorf("%w: InsertLdapTemplateAddressPool: %v", ErrQueryFailed, err)
		}
	}
	return result.ID, nil
}

func (m *PostgresManager) UpdateLdapTemplate(ctx context.Context, tmplID int, lt *api.LdapTemplate) error {
	if lt.InterfaceName == "" {
		return fmt.Errorf("%w: interface name can not be empty", ErrInvalidData)
	}
	if len(lt.AddressPools) == 0 {
		return fmt.Errorf("%w: %s", ErrInvalidData, "address pools can not be empty")
	}

	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	currentTemplate, err := m.SQLcQ.GetLdapTemplateByID(queryCtx, tmplID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrNotFound
		}
		return fmt.Errorf("%w: GetLdapTemplateByID: %w", ErrQueryFailed, err)
	}

	if currentTemplate.Name != lt.Name {
		nameExists, err := m.SQLcQ.LdapTemplateNameCheck(queryCtx, lt.Name)
		if err != nil {
			return fmt.Errorf("%w: TemplateNameCheck: %w", ErrQueryFailed, err)
		}
		if nameExists {
			return vcm.ErrAlreadyExists
		}
	}

	pools := make([]sqlc.AddressPool, 0, len(lt.AddressPools))
	for _, ap := range lt.AddressPools {
		pool, err := m.SQLcQ.GetAddressPoolByID(queryCtx, ap.ID)
		if err != nil {
			if err == pgx.ErrNoRows {
				return fmt.Errorf("%w: address pool with ID %d does not exist", ErrInvalidData, ap.ID)
			}
			return fmt.Errorf("%w: GetAddressPoolByID: %v", ErrQueryFailed, err)
		}
		pools = append(pools, pool)
	}

	if len(pools) > 1 {
		if err := addresspools.VerifyPoolsNotOverlap(pools); err != nil {
			return fmt.Errorf("%w: VerifyPoolsNotOverlap: %v", ErrInvalidData, err)
		}
	}

	MFAType := lt.MFAType
	if len(lt.MFAType) == 0 {
		MFAType = string(currentTemplate.MfaAuth)
	}

	updateParams := sqlc.UpdateLdapTemplateParams{
		ID:            tmplID,
		Name:          lt.Name,
		InterfaceName: lt.InterfaceName,
		IsAdmin:       lt.IsAdmin,
		MfaAuth:       sqlc.MfaAuthType(MFAType),
		Dns:           lt.DNS,
	}
	if lt.Filter != "" {
		updateParams.Filter = &lt.Filter
	}
	if lt.ListenPort != "" {
		updateParams.ListenPort = &lt.ListenPort
	}
	if lt.MTU != "" {
		updateParams.Mtu = &lt.MTU
	}

	_, err = m.SQLcQ.UpdateLdapTemplate(queryCtx, updateParams)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrNotFound
		}
		return fmt.Errorf("%w: UpdateLdapTemplate: %w", ErrQueryFailed, err)
	}

	err = m.SQLcQ.DeleteLdapTemplateAddressPools(queryCtx, tmplID)
	if err != nil {
		return fmt.Errorf("%w: DeleteLdapTemplateAddressPools: %v", ErrQueryFailed, err)
	}
	for _, ap := range lt.AddressPools {
		err = m.SQLcQ.InsertLdapTemplateAddressPool(queryCtx, sqlc.InsertLdapTemplateAddressPoolParams{
			LdapTemplateID: tmplID,
			AddressPoolID:  ap.ID,
		})
		if err != nil {
			return fmt.Errorf("%w: InsertLdapTemplateAddressPool: %v", ErrQueryFailed, err)
		}
	}

	return nil
}

func (m *PostgresManager) DeleteLdapTemplate(ctx context.Context, id int) error {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	exists, err := m.SQLcQ.LdapTemplateIDCheck(queryCtx, id)
	if err != nil {
		return fmt.Errorf("%w: LdapTemplateIDCheck: %w", ErrQueryFailed, err)
	}
	if !exists {
		return fmt.Errorf("%w: LDAP template with ID %d not found", ErrNotFound, id)
	}
	err = m.SQLcQ.DeleteLdapTemplate(queryCtx, id)
	if err != nil {
		return fmt.Errorf("%w: DeleteLdapTemplate: %w", ErrQueryFailed, err)
	}
	return nil
}

func (m *PostgresManager) GetLdapTemplateServers(ctx context.Context, templateID int) (*api.LdapTemplateServerList, error) {
	servers, err := m.DB.GetLdapTemplateServers(ctx, templateID)
	if err != nil {
		return nil, err
	}
	serverList := &api.LdapTemplateServerList{
		LdapTemplateServers: make([]*api.LdapTemplateServer, 0, len(servers)),
	}
	for _, server := range servers {
		serverList.LdapTemplateServers = append(serverList.LdapTemplateServers,
			&api.LdapTemplateServer{
				ServerID:             server.ServerID,
				ServerName:           server.ServerName,
				ClientSideAllowedIPs: server.ClientSideAllowedIPs,
				UsePresharedKey:      server.UsePresharedKey,
			},
		)
	}
	return serverList, nil
}

// ensureHealthcheckInAllowedIPs ensures that the server's healthcheck address is included
// in the allowed IPs list. If the healthcheck address is already contained in one of the
// allowed IP ranges, the list is returned unchanged. Otherwise, the healthcheck address
// is added as a /32 (IPv4) or /128 (IPv6) prefix.
func (m *PostgresManager) ensureHealthcheckInAllowedIPs(server *api.Server, allowedIPs []string) ([]string, error) {
	if server.HealthCheckAddress == "" {
		return allowedIPs, nil
	}

	allowedIPsPrefixes, err := ConvertAllowedIPsStringsToPrefixes(allowedIPs)
	if err != nil {
		return nil, fmt.Errorf("convert allowed IPs to prefixes: %v", err)
	}

	hcAddr, err := netip.ParseAddr(server.HealthCheckAddress)
	if err != nil {
		return nil, fmt.Errorf("parse healthcheck address '%s': %w", server.HealthCheckAddress, err)
	}

	allowedIPsPrefixes, _ = vcm.EnsureHealthcheckAddrInAllowedIPs(allowedIPsPrefixes, &hcAddr)
	return convertAllowedIPsPrefixesToStrings(allowedIPsPrefixes), nil
}

// validateAndPrepareTemplateServer retrieves the server by ID
// and ensures the server's healthcheck address is present in the allowed IPs list.
// Returns the resolved allowed IPs and the server, or an error.
func (m *PostgresManager) validateAndPrepareTemplateServer(
	ctx context.Context, serverID int, allowedIPs []string,
) (*api.Server, []string, error) {
	server, err := m.Vcm.GetServerByID(ctx, serverID)
	if err != nil {
		return nil, nil, fmt.Errorf("GetServerByID: %v", err)
	}

	resolvedIPs, err := m.ensureHealthcheckInAllowedIPs(server, allowedIPs)
	if err != nil {
		return nil, nil, err
	}

	return server, resolvedIPs, nil
}

func (m *PostgresManager) CreateLdapTemplateServer(ctx context.Context, templateID int, lts *api.LdapTemplateServer) error {
	_, resolvedIPs, err := m.validateAndPrepareTemplateServer(ctx, lts.ServerID, lts.ClientSideAllowedIPs)
	if err != nil {
		return err
	}

	ldapTemplateServer := &LdapTemplateServer{
		LdapTemplateID:       templateID,
		ServerID:             lts.ServerID,
		ClientSideAllowedIPs: resolvedIPs,
		UsePresharedKey:      lts.UsePresharedKey,
	}

	if err := m.DB.InsertLdapTemplateServer(ctx, ldapTemplateServer); err != nil {
		return fmt.Errorf("unable to insert LDAP template server into database: %v", err)
	}

	return nil
}

func (m *PostgresManager) UpdateLdapTemplateServer(ctx context.Context, templateID int, oldServerID int, lts *api.LdapTemplateServer) error {
	_, resolvedIPs, err := m.validateAndPrepareTemplateServer(ctx, lts.ServerID, lts.ClientSideAllowedIPs)
	if err != nil {
		return err
	}

	ldapTemplateServer := &LdapTemplateServer{
		LdapTemplateID:       templateID,
		OldServerID:          oldServerID,
		ServerID:             lts.ServerID,
		ClientSideAllowedIPs: resolvedIPs,
		UsePresharedKey:      lts.UsePresharedKey,
	}

	if err := m.DB.UpdateLdapTemplateServer(ctx, ldapTemplateServer); err != nil {
		return fmt.Errorf("unable to update LDAP template server in database: %v", err)
	}

	return nil
}

func (m *PostgresManager) DeleteLdapTemplateServer(ctx context.Context, templateID int, serverID int) error {
	err := m.DB.DeleteLdapTemplateServer(ctx, templateID, serverID)
	if err != nil {
		return fmt.Errorf("unable to delete LDAP template server from database: %v", err)
	}

	return nil
}

func ConvertAllowedIPsStringsToPrefixes(allowedIPs []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, len(allowedIPs))
	for i, ipStr := range allowedIPs {
		// Use postgres.ParseIPNet (same as the VCM adjacency path) so that:
		// - bare IPs without a prefix length are accepted and auto-completed to /32 or /128
		// - host bits are normalised via .Masked() (e.g. 10.0.0.1/24 → 10.0.0.0/24)
		// Any change to ParseIPNet validation rules will apply consistently here.
		prefix, err := postgres.ParseIPNet(ipStr)
		if err != nil {
			return nil, fmt.Errorf("invalid IP prefix %q: %w", ipStr, err)
		}
		prefixes[i] = prefix
	}
	return prefixes, nil
}

func convertAllowedIPsPrefixesToStrings(prefixes []netip.Prefix) []string {
	strs := make([]string, len(prefixes))
	for i, prefix := range prefixes {
		strs[i] = prefix.String()
	}
	return strs
}

func (m *PostgresManager) MFAGetEnabledTypes(ctx context.Context) *api.MfaEnabledTypes {
	mfaTypes := make([]string, 0, len(m.Cfg.MFAEnabledTypes))
	for _, t := range m.Cfg.MFAEnabledTypes {
		mfaTypes = append(mfaTypes, string(t))
	}
	return &api.MfaEnabledTypes{
		Types: mfaTypes,
	}
}

func (m *PostgresManager) MFAAuthenticate(ctx context.Context, userID int, mfaType mfa.AuthenticatorType, creds any) (bool, error) {
	if mfaType == mfa.NONE {
		return false, ErrNoAuthenticatorForUser
	}

	switch mfaType {
	case mfa.TOTP:
		scrtEncrypted, err := m.SQLcQ.GetUserTOTP(ctx, &userID)
		if err != nil {
			if err == pgx.ErrNoRows {
				return false, ErrUserSecretNotFound
			}
			return false, err
		}
		scrt, err := aesgcm.OpenString(m.EncryptionKey, scrtEncrypted)
		if err != nil {
			return false, err
		}
		success, err := m.TotpMfa.Authenticate(&mfa.TOTPSecret{
			Key: scrt,
		}, creds)
		if err != nil {
			return false, err
		}
		return success, nil
	case mfa.CERT:
		crls, err := m.DB.GetMFACerts(ctx, "crl")
		if err != nil {
			return false, err
		}
		cas, err := m.DB.GetMFACerts(ctx, "ca")
		if err != nil {
			return false, err
		}
		f := func(cs []*mfa.RawCert) [][]byte {
			var rcs [][]byte
			for _, c := range cs {
				rcs = append(rcs, c.Data)
			}
			return rcs
		}

		success, err := m.CertMfa.Authenticate(&mfa.Validators{
			CAs:  f(cas),
			CRLs: f(crls),
		}, creds)
		if err != nil {
			return false, err
		}
		return success, nil
	default:
		return false, ErrAuthenticatorIsNotSupported
	}
}

func (m *PostgresManager) MFACreateTOTPSecret(ctx context.Context, userID int) (*mfa.TOTPSecret, error) {
	u, err := m.DB.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, fmt.Errorf("%w: user with that ID does not exist", ErrBadUserData)
		}
		return nil, fmt.Errorf("GetUserByID: %w", err)
	}

	scrt, err := m.TotpMfa.CreateUserSecrets()
	if err != nil {
		return nil, fmt.Errorf("failed to create secrets: %v", err)
	}

	err = db.WithTx(ctx, m.DB.GetDbConnection(ctx), func(qsqlc *sqlc.Queries) error {
		keyEnc, err := aesgcm.Seal(m.EncryptionKey, scrt.Key)
		if err != nil {
			return fmt.Errorf("failed to encrypt secrets: %v", err)
		}
		_, err = qsqlc.InsertUserTOTP(ctx, sqlc.InsertUserTOTPParams{
			KeyEncrypted: keyEnc,
			UserID:       &userID,
		})
		if err != nil {
			return fmt.Errorf("InsertUserTOTP: %w", err)
		}

		_, err = qsqlc.UpdateUserEditTime(ctx, sqlc.UpdateUserEditTimeParams{
			UpdatedAt: db.NewPgTimestamp(time.Now()),
			ID:        userID,
		})
		if err != nil {
			return fmt.Errorf("UpdateUserEditTime: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}

	if err := m.DeleteInvalidSessionsByUserTOTP(ctx, u); err != nil {
		return nil, err
	}

	return scrt, nil
}

func (m *PostgresManager) MFAUpdateTOTPSecret(ctx context.Context, userID int) (*mfa.TOTPSecret, error) {
	u, err := m.DB.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, fmt.Errorf("%w: user with that ID does not exist", ErrBadUserData)
		}
		return nil, fmt.Errorf("GetUserByID: %w", err)
	}

	scrt, err := m.TotpMfa.CreateUserSecrets()
	if err != nil {
		return nil, fmt.Errorf("failed to create secrets: %v", err)
	}

	err = db.WithTx(ctx, m.DB.GetDbConnection(ctx), func(qsqlc *sqlc.Queries) error {
		keyEnc, err := aesgcm.Seal(m.EncryptionKey, scrt.Key)
		if err != nil {
			return fmt.Errorf("failed to encrypt secrets: %v", err)
		}
		_, err = qsqlc.UpdateUserTOTP(ctx, sqlc.UpdateUserTOTPParams{
			KeyEncrypted: keyEnc,
			UserID:       &userID,
		})
		if err != nil {
			if err == pgx.ErrNoRows {
				return ErrUserSecretNotFound
			}
			return fmt.Errorf("UpdateUserTOTP: %w", err)
		}

		_, err = qsqlc.UpdateUserEditTime(ctx, sqlc.UpdateUserEditTimeParams{
			UpdatedAt: db.NewPgTimestamp(time.Now()),
			ID:        userID,
		})
		if err != nil {
			return fmt.Errorf("UpdateUserEditTime: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}

	if err := m.DeleteInvalidSessionsByUserTOTP(ctx, u); err != nil {
		return nil, err
	}

	return scrt, nil
}

func (m *PostgresManager) MFAGetTOTPSecret(ctx context.Context, userID int) (*mfa.TOTPSecret, error) {
	exists, err := m.DB.CheckUserIDExists(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("%w: user with that ID does not exist", ErrBadUserData)
	}

	scrtEncrypted, err := m.SQLcQ.GetUserTOTP(ctx, &userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrUserSecretNotFound
		}
		return nil, err
	}
	scrt, err := aesgcm.OpenString(m.EncryptionKey, scrtEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secrets: %v", err)
	}
	return &mfa.TOTPSecret{Key: scrt}, nil
}

func (m *PostgresManager) MFAGetAllCAs(ctx context.Context) (*api.MfaCAList, error) {
	certs, err := m.DB.GetMFACerts(ctx, "ca")
	if err != nil {
		return nil, err
	}

	var list api.MfaCAList
	for _, rc := range certs {
		cert, err := ParseCA(rc.Data)
		if err != nil {
			return nil, err
		}

		list.CAs = append(list.CAs, api.MfaCA{
			ID:        rc.ID,
			CN:        cert.Subject.CommonName,
			NotBefore: cert.NotBefore.String(),
			NotAfter:  cert.NotAfter.String(),
		})
	}
	return &list, nil
}

func (m *PostgresManager) MFACreateCA(ctx context.Context, ca []byte) error {
	if _, err := ParseCA(ca); err != nil {
		return err
	}
	if err := m.DB.InsertMFACerts(ctx, ca, mfa.CertTypeCA); err != nil {
		return err
	}
	if err := m.DeleteInvalidSessionsByMFACert(ctx, InsertCert, mfa.CertTypeCA); err != nil {
		return err
	}
	return nil
}

func ParseCA(ca []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(ca))
	if block == nil {
		return nil, errors.New("failed to parse x509 certificate in PEM format")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse x509 certificate: %w", err)
	}
	return cert, nil
}

func (m *PostgresManager) MFADeleteCA(ctx context.Context, id int) error {
	if err := m.DB.DeleteMFACerts(ctx, id); err != nil {
		return err
	}
	if err := m.DeleteInvalidSessionsByMFACert(ctx, DeleteCert, mfa.CertTypeCA); err != nil {
		return err
	}
	return nil
}

func (m *PostgresManager) MFAGetAllCRLs(ctx context.Context) (*api.MfaCRLList, error) {
	crls, err := m.DB.GetMFACerts(ctx, "crl")
	if err != nil {
		return nil, err
	}

	var list api.MfaCRLList
	for _, r := range crls {
		blockCRL, _ := pem.Decode(r.Data)
		if blockCRL == nil {
			return nil, errors.New("failed to parse CRL in PEM format")
		}
		crl, err := x509.ParseRevocationList(blockCRL.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CRL: %w", err)
		}

		var sn []string
		for _, rc := range crl.RevokedCertificateEntries {
			sn = append(sn, rc.SerialNumber.String())
		}

		list.CRLs = append(list.CRLs, api.MfaCRL{
			ID:                  r.ID,
			Issuer:              crl.Issuer.String(),
			RevokedCertificates: sn,
		})
	}
	return &list, nil
}

func (m *PostgresManager) MFACreateCRL(ctx context.Context, crl []byte) error {
	blockCRL, _ := pem.Decode(crl)
	if blockCRL == nil {
		return errors.New("failed to parse CRL in PEM format")
	}
	_, err := x509.ParseRevocationList(blockCRL.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse CRL: %w", err)
	}
	if err := m.DB.InsertMFACerts(ctx, crl, mfa.CertTypeCRL); err != nil {
		return err
	}
	if err := m.DeleteInvalidSessionsByMFACert(ctx, InsertCert, mfa.CertTypeCRL); err != nil {
		return err
	}
	return nil
}

func (m *PostgresManager) MFADeleteCRL(ctx context.Context, id int) error {
	if err := m.DB.DeleteMFACerts(ctx, id); err != nil {
		return err
	}
	if err := m.DeleteInvalidSessionsByMFACert(ctx, DeleteCert, mfa.CertTypeCRL); err != nil {
		return err
	}
	return nil
}

func (m *PostgresManager) ensureUserCanBeAdded(ctx context.Context) error {
	maxQuantity := m.Feats.GetMaxUserQuantity()
	if maxQuantity != 0 {
		quantity, err := m.DB.CountUsers(ctx)
		if err != nil {
			return err
		}
		if quantity >= maxQuantity-1 {
			return fmt.Errorf("%w: max user quantity allowed exceeded", ErrFeaturesRestriction)
		}
	}
	return nil
}

func (m *PostgresManager) ensureMFACanBeSet(mfaType mfa.AuthenticatorType) error {
	if len(mfaType) == 0 {
		return nil
	}

	for _, a := range m.Cfg.MFAEnabledTypes {
		if a == mfaType {
			return nil
		}
	}
	return ErrAuthenticatorIsNotSupported
}

func validateLdapConfig(cfg *api.LdapConfig) error {
	if cfg.UseTLS {
		if len(cfg.FQDN) == 0 && len(cfg.CACert) == 0 {
			return nil
		}

		if len(cfg.FQDN) == 0 {
			return errors.New("FQDN is not provided")
		}

		if len(cfg.FQDN) > FQDNMaxSize {
			return errors.New("FQDN exceeds its maximum length")
		}

		if len(cfg.CACert) == 0 {
			return errors.New("the CA certificate is not provided")
		}
		caCertPool := x509.NewCertPool()
		ok := caCertPool.AppendCertsFromPEM([]byte(cfg.CACert))
		if !ok {
			return errors.New("invalid CA certificate")
		}
	}

	if !cfg.UseTLS && (len(cfg.CACert) != 0 || len(cfg.FQDN) != 0) {
		return errors.New("enable TLS to add FQDN and CA certificate")
	}
	return nil
}

func ValidatePassword(pwd string, minLen int, upper, lower, digit, symbol bool) error {
	if len(pwd) < minLen {
		msg := fmt.Sprintf("expected minimum length: %v, got: %v", minLen, len(pwd))
		return fmt.Errorf("%w: %s", ErrBadPassword, msg)
	}
	var upperPresent, lowerPresent, digitPresent, symbolPresent bool
	for _, c := range pwd {
		switch {
		case unicode.IsDigit(c):
			digitPresent = true
		case unicode.IsUpper(c):
			upperPresent = true
		case unicode.IsLower(c):
			lowerPresent = true
		case !unicode.IsLetter(c):
			symbolPresent = true
		}
	}
	if upper && !upperPresent {
		return fmt.Errorf("%w: uppercase character is not present", ErrBadPassword)
	}
	if lower && !lowerPresent {
		return fmt.Errorf("%w: lowercase character is not present", ErrBadPassword)
	}
	if digit && !digitPresent {
		return fmt.Errorf("%w: digit is not present", ErrBadPassword)
	}
	if symbol && !symbolPresent {
		return fmt.Errorf("%w: symbol character is not present", ErrBadPassword)
	}
	return nil
}

func (m *PostgresManager) GetHealthcheckVpnCPort() string {
	return m.Cfg.HealthcheckRPCPortVpnC
}
