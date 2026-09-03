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
	"net/netip"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	sqlc "github.com/entguard/entguard/db"
	"github.com/entguard/entguard/pkg/db"
	"github.com/entguard/entguard/pkg/mfa"
	"github.com/entguard/entguard/pkg/nanoid"
	"github.com/entguard/entguard/service/api/v1"
	"github.com/entguard/entguard/service/session"
	"github.com/entguard/entguard/service/user"
	"github.com/entguard/entguard/service/vcm"
	pg "github.com/entguard/entguard/service/vcm/drivers/postgres"
)

const (
	TestUserName        = "testuser"
	TestUserPassword    = "testpassword"
	testHealthcheckAddr = "186.178.1.1"
)

func NewSampleUser(t *testing.T, name string) *user.UserModel {
	u, err := user.NewUserModelWithName(name)
	require.NoError(t, err)

	err = u.SetPasswordWithHashingCost(TestUserPassword, bcrypt.MinCost)
	require.NoError(t, err)

	u.SetAdmin(true)
	u.SetMFAType(mfa.TOTP)

	err = u.SetAuthType(user.InternalUser)
	require.NoError(t, err)

	err = u.SetNotification("sample notification")
	require.NoError(t, err)

	return u
}

// user options
func WithMfaType(mfaType string) func(user *user.UserModel) {
	return func(user *user.UserModel) {
		user.SetMFAType(mfaType)
	}
}

func InsertSampleUser(t *testing.T, ctx context.Context, q user.Querier, name string, options ...func(user *user.UserModel)) *user.UserModel {
	u := NewSampleUser(t, name)
	for _, f := range options {
		f(u)
	}
	err := q.InsertUser(ctx, u)
	require.NoError(t, err)

	return u
}

func InsertSampleDeviceWithSession(t *testing.T, ctx context.Context, q *sqlc.Queries, deviceTemplateID int, lastTimeConnected *time.Time) (*sqlc.Device, string) {
	device := InsertSampleDeviceManual(t, ctx, q, deviceTemplateID)

	sessionID, err := session.GenerateSessionID()
	require.NoError(t, err)

	if lastTimeConnected == nil {
		t := time.Now()
		lastTimeConnected = &t
	}

	updatedDevice, err := q.UpdateDeviceSessionAndLastTimeConnected(ctx, sqlc.UpdateDeviceSessionAndLastTimeConnectedParams{
		ID:                int(device.ID),
		SessionID:         sessionID,
		LastTimeConnected: db.NewPgTimestamp(*lastTimeConnected),
	})
	require.NoError(t, err)

	return &updatedDevice, sessionID
}

func NewSampleUserUpdate(t *testing.T, u *user.UserModel, name string) *user.UserModel {
	err := u.SetName(name)
	require.NoError(t, err)

	u.SetAdmin(false)

	u.SetMFAType(mfa.CERT)

	err = u.SetNotification("sample notification updated")
	require.NoError(t, err)

	return u
}

func NewSampleServer(t *testing.T, name string) *pg.Server {
	ep, err := netip.ParseAddr("172.18.0.1")
	require.NoError(t, err)

	healthCheckAddr, err := netip.ParseAddr(testHealthcheckAddr)
	require.NoError(t, err)

	return &pg.Server{
		Name:               name,
		Endpoint:           ep,
		Description:        "test server",
		HealthCheckAddress: &healthCheckAddr,
	}
}

func NewSampleServerWithoutHC(t *testing.T, name string) *pg.Server {
	s := NewSampleServer(t, name)
	s.HealthCheckAddress = nil
	return s
}

func NewSampleWGServerConfig() *pg.WireGuardConfig {
	return &pg.WireGuardConfig{
		InterfaceName:       "egtest",
		PrivateKey:          "uBEC+jx8Yvr9Aw+LIzro0/wZfgpV7pDup0xb/2hLAUo=",
		PublicKey:           "uBEC+jx8Yvr9Aw+LIzro0/wZfgpV7pDup0xb/2hLAUo=",
		Addresses:           []string{"1.1.1.0/24", "1.1.7.0/24"},
		ListenPort:          8080,
		DNS:                 []string{"test dns1"},
		MTU:                 9000,
		PersistentKeepalive: -1,
	}
}

// server and server's wg config options
func WithEndpoint(endpoint netip.Addr) func(s *pg.Server, wgc *pg.WireGuardConfig) {
	return func(s *pg.Server, wgc *pg.WireGuardConfig) {
		s.Endpoint = endpoint
	}
}

func WithHealthCheckAddress(hcAddr *netip.Addr) func(s *pg.Server, wgc *pg.WireGuardConfig) {
	return func(s *pg.Server, wgc *pg.WireGuardConfig) {
		if hcAddr == nil {
			s.HealthCheckAddress = nil
			return
		}

		hcAddrCopy := *hcAddr
		s.HealthCheckAddress = &hcAddrCopy
	}
}

func WithDescription(desc string) func(s *pg.Server, wgc *pg.WireGuardConfig) {
	return func(s *pg.Server, wgc *pg.WireGuardConfig) {
		s.Description = desc
	}
}

func WithInterfaceNameServerWGC(ifaceName string) func(s *pg.Server, wgc *pg.WireGuardConfig) {
	return func(s *pg.Server, wgc *pg.WireGuardConfig) {
		wgc.InterfaceName = ifaceName
	}
}

func WithPrivateKeyServerWGC(privkey string) func(s *pg.Server, wgc *pg.WireGuardConfig) {
	return func(s *pg.Server, wgc *pg.WireGuardConfig) {
		wgc.PrivateKey = privkey
	}
}

func WithPublicKeyServerWGC(pubkey string) func(s *pg.Server, wgc *pg.WireGuardConfig) {
	return func(s *pg.Server, wgc *pg.WireGuardConfig) {
		wgc.PublicKey = pubkey
	}
}

func WithAddressesServerWGC(addresses []string) func(s *pg.Server, wgc *pg.WireGuardConfig) {
	return func(s *pg.Server, wgc *pg.WireGuardConfig) {
		wgc.Addresses = []string{}
		wgc.Addresses = append(wgc.Addresses, addresses...)
	}
}

func WithListenPortServerWGC(listenPort int) func(s *pg.Server, wgc *pg.WireGuardConfig) {
	return func(s *pg.Server, wgc *pg.WireGuardConfig) {
		wgc.ListenPort = listenPort
	}
}

func WithMTUServerWGC(mtu int) func(s *pg.Server, wgc *pg.WireGuardConfig) {
	return func(s *pg.Server, wgc *pg.WireGuardConfig) {
		wgc.MTU = mtu
	}
}

func WithPersistentKeepaliveServerWGC(persistentKeepalive int) func(s *pg.Server, wgc *pg.WireGuardConfig) {
	return func(s *pg.Server, wgc *pg.WireGuardConfig) {
		wgc.PersistentKeepalive = persistentKeepalive
	}
}

func InsertSampleServer(
	t *testing.T, ctx context.Context, conn *pgxpool.Pool, name string,
	options ...func(s *pg.Server, wgc *pg.WireGuardConfig),
) *pg.Server {
	t.Helper()

	server, _ := InsertSampleServerReturnWithWGC(t, ctx, conn, name, options...)
	return server
}

func InsertSampleServerReturnWithWGC(
	t *testing.T, ctx context.Context, conn *pgxpool.Pool, name string,
	options ...func(s *pg.Server, wgc *pg.WireGuardConfig),
) (*pg.Server, *pg.WireGuardConfig) {
	t.Helper()

	server := NewSampleServer(t, name)
	wgConfig := NewSampleWGServerConfig()
	for _, f := range options {
		f(server, wgConfig)
	}
	id, err := vcm.InsertServerWithWireGuardConfig(ctx, conn, TestEncryptionKey, server, wgConfig)
	require.NoError(t, err)

	server.ID = id
	return server, wgConfig
}

// RequireTimeBetween asserts that tested is between before and after
// (inclusive). It handles missing timezone information caused by inserting
// golang timestamp into database.
func RequireTimeBetween(t *testing.T, tested, before, after time.Time) {
	t.Helper()

	for _, t := range []*time.Time{&tested, &before, &after} {
		_, offsetSeconds := t.Zone()
		offsetTime := time.Duration(offsetSeconds) * time.Second
		*t = t.Add(offsetTime)
		*t = t.Truncate(time.Microsecond) // Postgres stores timestamps in microsecond precision
	}

	require.WithinRange(t, tested, before, after)
}

// adjacency options
func WithSSAIPsAdj(ssaips []netip.Prefix) func(a *sqlc.InsertAdjacencyParams) {
	return func(a *sqlc.InsertAdjacencyParams) {
		a.ServerSideAllowedIps = []netip.Prefix{}
		a.ServerSideAllowedIps = append(a.ServerSideAllowedIps, ssaips...)
	}
}

func WithCSAIPsAdj(csaips []netip.Prefix) func(a *sqlc.InsertAdjacencyParams) {
	return func(a *sqlc.InsertAdjacencyParams) {
		a.ClientSideAllowedIps = []netip.Prefix{}
		a.ClientSideAllowedIps = append(a.ClientSideAllowedIps, csaips...)
	}
}

// WithPresharedKeyAdj returns option that encrypts psk before assigning it to InsertAdjacencyParams
func WithPresharedKeyAdj(psk string) func(a *sqlc.InsertAdjacencyParams) {
	return func(a *sqlc.InsertAdjacencyParams) {
		a.PresharedKeyEncrypted = Seal(psk)
	}
}

func InsertSampleAdjacency(
	t *testing.T, ctx context.Context, q *sqlc.Queries, serverID, deviceID int, options ...func(a *sqlc.InsertAdjacencyParams),
) *sqlc.Adjacency {
	t.Helper()

	params := &sqlc.InsertAdjacencyParams{
		ServerID:              serverID,
		DeviceID:              deviceID,
		ServerSideAllowedIps:  []netip.Prefix{netip.MustParsePrefix("1.1.1.1/32")},
		ClientSideAllowedIps:  []netip.Prefix{netip.MustParsePrefix("1.1.1.1/32")},
		PresharedKeyEncrypted: Seal("bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM="),
	}
	for _, f := range options {
		f(params)
	}
	adj, err := q.InsertAdjacency(ctx, *params)
	require.NoError(t, err)

	return &adj
}

// adjacency template options
func WithCSAIPsAdjTmpl(csaips []netip.Prefix) func(a *sqlc.InsertAdjacencyTemplateParams) {
	return func(a *sqlc.InsertAdjacencyTemplateParams) {
		a.ClientSideAllowedIps = []netip.Prefix{}
		a.ClientSideAllowedIps = append(a.ClientSideAllowedIps, csaips...)
	}
}

func WithUsePresharedKeyAdjTmpl(usePsk bool) func(a *sqlc.InsertAdjacencyTemplateParams) {
	return func(a *sqlc.InsertAdjacencyTemplateParams) {
		a.UsePresharedKey = usePsk
	}
}

func InsertSampleAdjacencyTemplate(
	t *testing.T, ctx context.Context, q *sqlc.Queries, serverID, userID int, options ...func(a *sqlc.InsertAdjacencyTemplateParams),
) *sqlc.AdjacencyTemplate {
	t.Helper()

	params := &sqlc.InsertAdjacencyTemplateParams{
		ServerID:             serverID,
		UserID:               userID,
		ClientSideAllowedIps: []netip.Prefix{netip.MustParsePrefix("186.178.1.0/24"), netip.MustParsePrefix("192.168.100.0/24")},
		UsePresharedKey:      true,
	}
	for _, f := range options {
		f(params)
	}
	adjTemplate, err := q.InsertAdjacencyTemplate(ctx, *params)
	require.NoError(t, err)

	return &adjTemplate
}

func InsertAdjacencyTemplate(
	t *testing.T, ctx context.Context, q *sqlc.Queries, params *sqlc.InsertAdjacencyTemplateParams,
) *sqlc.AdjacencyTemplate {
	t.Helper()

	adjTemplate, err := q.InsertAdjacencyTemplate(ctx, *params)
	require.NoError(t, err)

	return &adjTemplate
}

func NewSampleAPIAdjacencyTemplate(allowedIPs []string) *api.WireGuardAdjacencyTemplate {
	return &api.WireGuardAdjacencyTemplate{
		UsePresharedKey:      true,
		ClientSideAllowedIPs: allowedIPs,
	}
}

func NewSampleAdjacencyTemplateExtended(t *testing.T, allowedIPs []string, usePresharedKey bool, server *api.Server) *api.AdjacencyTemplateExtended {
	t.Helper()

	config := &api.WireGuardAdjacencyTemplate{
		UsePresharedKey:      usePresharedKey,
		ClientSideAllowedIPs: allowedIPs,
	}

	return &api.AdjacencyTemplateExtended{
		Server:         server,
		TemplateConfig: config,
	}
}

func NewSampleLdapTemplate(name string, adp []*api.AddressPool) *api.LdapTemplate {
	return &api.LdapTemplate{
		Name:          name,
		InterfaceName: "egtest0",
		IsAdmin:       true,
		MFAType:       mfa.CERT,
		ListenPort:    "9090",
		MTU:           "3333",
		DNS:           []string{"test.dns.example.com"},
		Filter:        "(objectClass=person)",
		AddressPools:  adp,
	}
}

func InsertSampleLdapTemplate(t *testing.T, ctx context.Context, m *user.PostgresManager, name string, adp []*api.AddressPool) *api.LdapTemplate {
	t.Helper()

	if adp == nil {
		p1 := InsertSampleAddressPool(t, ctx, m.SQLcQ, &sqlc.InsertAddressPoolParams{
			Name:      "testPool1",
			StartAddr: netip.MustParseAddr("10.0.0.1"),
			EndAddr:   netip.MustParseAddr("10.0.0.100"),
			NetMask:   24,
		})

		p2 := InsertSampleAddressPool(t, ctx, m.SQLcQ, &sqlc.InsertAddressPoolParams{
			Name:      "testPool2",
			StartAddr: netip.MustParseAddr("10.1.1.1"),
			EndAddr:   netip.MustParseAddr("10.1.1.100"),
			NetMask:   24,
		})
		adp = []*api.AddressPool{user.ConvertDBToAPIPool(*p1), user.ConvertDBToAPIPool(*p2)}
	}

	temp := NewSampleLdapTemplate(name, adp)

	id, err := m.CreateLdapTemplate(ctx, temp)
	require.NoError(t, err)

	temp.ID = id

	return temp
}

func InsertSampleLdapTemplateServer(
	t *testing.T, ctx context.Context, q user.Querier, serverName string, serverID, templateID int,
) *user.LdapTemplateServer {
	lts := &user.LdapTemplateServer{
		ServerName:           serverName,
		ServerID:             serverID,
		LdapTemplateID:       templateID,
		ClientSideAllowedIPs: []string{"3.3.3.3/32", testHealthcheckAddr},
		UsePresharedKey:      true,
	}

	err := q.InsertLdapTemplateServer(ctx, lts)
	require.NoError(t, err)

	return lts
}

func InsertSampleAddressPool(t *testing.T, ctx context.Context, q *sqlc.Queries, params *sqlc.InsertAddressPoolParams) *sqlc.AddressPool {
	t.Helper()

	pool, err := q.InsertAddressPool(ctx, *params)
	require.NoError(t, err)

	return &pool
}

func NewSampleInsertAddressPoolParams(name string) *sqlc.InsertAddressPoolParams {
	return &sqlc.InsertAddressPoolParams{
		Name:        name,
		StartAddr:   netip.MustParseAddr("10.0.0.1"),
		EndAddr:     netip.MustParseAddr("10.0.0.100"),
		NetMask:     24,
		Description: "test description",
	}
}

func SetupOverlappingPools(t *testing.T, ctx context.Context, q *sqlc.Queries) (pool1, pool2 *sqlc.AddressPool) {
	t.Helper()

	addressPool1Params := NewSampleInsertAddressPoolParams("overlapping-pool-1-broad")
	addressPool1Params.StartAddr = netip.MustParseAddr("10.200.0.1")
	addressPool1Params.EndAddr = netip.MustParseAddr("10.200.0.100")
	addressPool1Params.NetMask = 24
	pool1 = InsertSampleAddressPool(t, ctx, q, addressPool1Params)

	addressPool2Params := NewSampleInsertAddressPoolParams("overlapping-pool-2-subset")
	addressPool2Params.StartAddr = netip.MustParseAddr("10.200.0.30")
	addressPool2Params.EndAddr = netip.MustParseAddr("10.200.0.39")
	addressPool2Params.NetMask = 24
	pool2 = InsertSampleAddressPool(t, ctx, q, addressPool2Params)

	return pool1, pool2
}

func InsertSampleDeviceManual(t *testing.T, ctx context.Context, q *sqlc.Queries, deviceTemplateID int) *sqlc.Device {
	t.Helper()
	externalDeviceID, err := nanoid.Generate()
	require.NoError(t, err)
	deviceInfo := "OS:iOS;OSV:17.2;OSB:21C66;APPV:3.5.1;DVCE:iPhone 15 Pro"
	params := sqlc.InsertDeviceParams{
		Description:         "device-description-stub",
		DeviceTemplateID:    deviceTemplateID,
		ExternalDeviceID:    externalDeviceID,
		PrivateKeyEncrypted: Seal("uBEC+jx8Yvr9Aw+LIzro0/wZfgpV7pDup0xb/2hLAUo="),
		PublicKey:           "uBEC+jx8Yvr9Aw+LIzro0/wZfgpV8pDup0xb/2hLAUo=",
		DeviceInformation:   &deviceInfo,
	}
	device, err := q.InsertDevice(ctx, params)
	require.NoError(t, err)

	return &device
}

func InsertSampleDeviceAutoGenerated(t *testing.T, ctx context.Context, m user.Manager, deviceTemplateID int) *api.Device {
	t.Helper()
	externalDeviceID, err := nanoid.Generate()
	require.NoError(t, err)
	deviceInformation := "OS:iOS;OSV:17.2;OSB:21C66;APPV:3.5.1;DVCE:iPhone 15 Pro"

	device, err := m.CreateDeviceWithAutogenData(ctx, deviceTemplateID, externalDeviceID, deviceInformation)
	require.NoError(t, err)

	return device
}

func InsertSampleDeviceAutoGeneratedNoExternalID(t *testing.T, ctx context.Context, m user.Manager, deviceTemplateID int) *api.Device {
	t.Helper()
	deviceInformation := "OS:iOS;OSV:17.2;OSB:21C66;APPV:3.5.1;DVCE:iPhone 15 Pro"

	device, err := m.CreateDeviceWithAutogenData(ctx, deviceTemplateID, "", deviceInformation)
	require.NoError(t, err)

	return device
}

// TODO do we need both InsertSampleDeviceTemplateAPI and InsertSampleDeviceTemplate?
func InsertSampleDeviceTemplateAPI(
	t *testing.T, ctx context.Context, m vcm.VPNConfigManager, userID int, addressPools []*api.AddressPool,
) *api.DeviceTemplate {
	t.Helper()
	dt := &api.DeviceTemplate{
		InterfaceName: "egtest",
		AddressPools:  addressPools,
		ListenPort:    "51820",
		DNS:           []string{"test dns1"},
		MTU:           "1420",
	}

	id, err := m.CreateDeviceTemplate(ctx, userID, dt)
	require.NoError(t, err)

	dt.ID = id
	return dt
}

// TODO do we need both InsertSampleDeviceTemplateAPI and InsertSampleDeviceTemplate?
func InsertSampleDeviceTemplate(t *testing.T, ctx context.Context, sqlcQ *sqlc.Queries, userID int, addressPoolIDs []int) *sqlc.DeviceTemplate {
	t.Helper()

	deviceTemplate, err := sqlcQ.InsertDeviceTemplate(ctx, sqlc.InsertDeviceTemplateParams{
		UserID:        userID,
		InterfaceName: "wg0",
		ListenPort:    db.IntToPtr(51820),
		Mtu:           db.IntToPtr(1420),
	})
	require.NoError(t, err)

	for _, poolID := range addressPoolIDs {
		_, err = sqlcQ.InsertDeviceTemplateAddressPool(ctx, sqlc.InsertDeviceTemplateAddressPoolParams{
			DeviceTemplateID: deviceTemplate.ID,
			AddressPoolID:    poolID,
		})
		require.NoError(t, err)
	}

	for _, dns := range []string{"dns1", "dns2"} {
		_, err = sqlcQ.InsertDeviceTemplateWireGuardIfaceDNS(ctx, sqlc.InsertDeviceTemplateWireGuardIfaceDNSParams{
			DeviceTemplateID: deviceTemplate.ID,
			Dns:              dns,
		})
		require.NoError(t, err)
	}

	return &deviceTemplate
}

func InsertDeviceAddress(t *testing.T, ctx context.Context, sqlcQ *sqlc.Queries, deviceID int, addr string) *sqlc.DeviceWgIfaceAddress {
	t.Helper()

	address, err := sqlcQ.InsertDeviceWireGuardIfaceAddr(ctx, sqlc.InsertDeviceWireGuardIfaceAddrParams{
		DeviceID: deviceID,
		Addr:     netip.MustParsePrefix(addr),
	})
	require.NoError(t, err)
	return &address
}
