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

package vcm_test

import (
	"context"
	"encoding/json"
	"net/netip"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	sqlc "github.com/entguard/entguard/db"
	"github.com/entguard/entguard/pkg/db"
	"github.com/entguard/entguard/pkg/test"
	"github.com/entguard/entguard/service/api/v1"
	"github.com/entguard/entguard/service/session"
	"github.com/entguard/entguard/service/user"
	"github.com/entguard/entguard/service/vcm"
	pg "github.com/entguard/entguard/service/vcm/drivers/postgres"
)

const notificationTimeout = 100 * time.Millisecond

var (
	vcManager vcm.VPNConfigManager
	uManager  user.Manager
	q         *pg.VPNConfigQuerier
	sqlcQ     *sqlc.Queries
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

		q = pg.NewQuerier(ctx, conn, logger)

		sqlcQ = sqlc.New(conn)

		vcManager, err = vcm.NewVCMPostgresBased(ctx, conn, logger, false, test.TestEncryptionKey)
		if err != nil {
			logger.Fatal("NewVCMPostgresBased: ", err)
		}
		vcManager.SetKubernetesClientset(ctx, nil) // do not use kube cluster settings in tests

		pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
		uManager, err = test.NewSamplePostgresManager(pgq, logger, user.NewLdapService(), vcManager, sqlc.New(conn))
		if err != nil {
			logger.Fatal("NewSamplePostgresManager: ", err)
		}

		if err := test.TruncateAllTables(ctx, q.GetDbConnection(ctx)); err != nil {
			logger.Fatal(err)
		}
	}
	m.Run()
}

func TestIntegrationCheckDeviceTemplateExistsByUserID(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user1 := test.InsertSampleUser(t, ctx, pgq, "testUser").ToAPI()
	user2 := test.InsertSampleUser(t, ctx, pgq, "testUser").ToAPI()
	pool := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("testPool"))
	apiPool := pg.ConvertDBToAPIPool(pool)
	test.InsertSampleDeviceTemplateAPI(t, ctx, vcManager, user1.ID, []*api.AddressPool{apiPool})

	cases := []struct {
		name     string
		userID   int
		expected bool
	}{
		{
			name:     "device template exists",
			userID:   user1.ID,
			expected: true,
		},
		{
			name:     "no device template exists with user id",
			userID:   user2.ID,
			expected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := vcManager.CheckDeviceTemplateExistsByUserID(ctx, tc.userID)
			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestIntegrationGetDeviceTemplateByUserID(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	p1Params := test.NewSampleInsertAddressPoolParams("testPool1")
	p1Params.StartAddr = netip.MustParseAddr("10.1.1.1")
	p1Params.EndAddr = netip.MustParseAddr("10.1.1.100")
	p1Params.NetMask = 24
	p1 := test.InsertSampleAddressPool(t, ctx, sqlcQ, p1Params)

	p2Params := test.NewSampleInsertAddressPoolParams("testPool2")
	p2Params.StartAddr = netip.MustParseAddr("10.1.2.1")
	p2Params.EndAddr = netip.MustParseAddr("10.1.2.100")
	p2Params.NetMask = 24
	p2 := test.InsertSampleAddressPool(t, ctx, sqlcQ, p2Params)

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user1 := test.InsertSampleUser(t, ctx, pgq, "testUser").ToAPI()
	user2 := test.InsertSampleUser(t, ctx, pgq, "testUser").ToAPI()
	dt := test.InsertSampleDeviceTemplateAPI(
		t, ctx, vcManager, user1.ID, []*api.AddressPool{pg.ConvertDBToAPIPool(p1), pg.ConvertDBToAPIPool(p2)},
	)

	cases := []struct {
		name                   string
		userID                 int
		expectedDeviceTemplate *api.DeviceTemplate
		expectFailure          bool
	}{
		{
			name:                   "get device template",
			userID:                 user1.ID,
			expectedDeviceTemplate: dt,
			expectFailure:          false,
		},
		{
			name:          "no device template exists with user id",
			userID:        user2.ID,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := vcManager.GetDeviceTemplateByUserID(ctx, tc.userID)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expectedDeviceTemplate, got)
		})
	}
}

func TestIntegrationCreateDeviceTemplate(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	p1Params := test.NewSampleInsertAddressPoolParams("testPool1")
	p1Params.StartAddr = netip.MustParseAddr("10.2.1.1")
	p1Params.EndAddr = netip.MustParseAddr("10.2.1.100")
	p1Params.NetMask = 24
	p1 := test.InsertSampleAddressPool(t, ctx, sqlcQ, p1Params)

	p2Params := test.NewSampleInsertAddressPoolParams("testPool2")
	p2Params.StartAddr = netip.MustParseAddr("10.2.2.1")
	p2Params.EndAddr = netip.MustParseAddr("10.2.2.100")
	p2Params.NetMask = 24
	p2 := test.InsertSampleAddressPool(t, ctx, sqlcQ, p2Params)

	overlapPool1, overlapPool2 := test.SetupOverlappingPools(t, ctx, sqlcQ)

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user := test.InsertSampleUser(t, ctx, pgq, "testUser").ToAPI()
	cases := []struct {
		name           string
		userID         int
		deviceTemplate *api.DeviceTemplate
		expectFailure  bool
	}{
		{
			name:   "insert device template",
			userID: user.ID,
			deviceTemplate: &api.DeviceTemplate{
				InterfaceName: "egtest",
				AddressPools: []*api.AddressPool{
					pg.ConvertDBToAPIPool(p1),
					pg.ConvertDBToAPIPool(p2),
				},
				ListenPort: "8080",
				DNS:        []string{"test dns1"},
				MTU:        "9000",
			},
			expectFailure: false,
		},
		{
			name:   "no user exists with id",
			userID: 6,
			deviceTemplate: &api.DeviceTemplate{
				InterfaceName: "egtest",
				AddressPools: []*api.AddressPool{
					pg.ConvertDBToAPIPool(p1),
					pg.ConvertDBToAPIPool(p2),
				},
				ListenPort: "8080",
				DNS:        []string{"test dns1"},
				MTU:        "9000",
			},
			expectFailure: true,
		},
		{
			name:   "create with overlapping address pools",
			userID: user.ID,
			deviceTemplate: &api.DeviceTemplate{
				InterfaceName: "egtest-overlap",
				AddressPools: []*api.AddressPool{
					pg.ConvertDBToAPIPool(overlapPool1),
					pg.ConvertDBToAPIPool(overlapPool2),
				},
				ListenPort: "8080",
				DNS:        []string{"test dns1"},
				MTU:        "9000",
			},
			expectFailure: true,
		},
	}

	getCounts := func() []int {
		return countTablesRows(t, ctx, "device_templates", "device_template_address_pools", "device_template_wg_iface_dnss")
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			counts := getCounts()
			wgConfigCountBefore, ifaceAddrPoolsCountBefore, ifaceDnsCountBefore := counts[0], counts[1], counts[2]

			id, err := vcManager.CreateDeviceTemplate(ctx, tc.userID, tc.deviceTemplate)

			counts = getCounts()
			wgConfigCountAfter, ifaceAddrPoolsCountAfter, ifaceDnsCountAfter := counts[0], counts[1], counts[2]

			if tc.expectFailure {
				require.Error(t, err)
				require.Equal(t, wgConfigCountAfter, wgConfigCountBefore)
				require.Equal(t, ifaceAddrPoolsCountAfter, ifaceAddrPoolsCountBefore)
				require.Equal(t, ifaceDnsCountAfter, ifaceDnsCountBefore)
				return
			}
			require.NoError(t, err)

			require.Equal(t, wgConfigCountAfter, wgConfigCountBefore+1)
			require.Equal(t, ifaceAddrPoolsCountAfter, ifaceAddrPoolsCountBefore+len(tc.deviceTemplate.AddressPools))
			require.Equal(t, ifaceDnsCountAfter, ifaceDnsCountBefore+len(tc.deviceTemplate.DNS))

			got, err := vcManager.GetDeviceTemplateByUserID(ctx, tc.userID)
			require.NoError(t, err)
			tc.deviceTemplate.ID = id
			require.Equal(t, tc.deviceTemplate, got)
		})
	}
}

func TestIntegrationUpdateDeviceTemplateByUserID(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	p1Params := test.NewSampleInsertAddressPoolParams("testPool1")
	p1Params.StartAddr = netip.MustParseAddr("10.3.1.1")
	p1Params.EndAddr = netip.MustParseAddr("10.3.1.100")
	p1Params.NetMask = 24
	p1 := test.InsertSampleAddressPool(t, ctx, sqlcQ, p1Params)

	p2Params := test.NewSampleInsertAddressPoolParams("testPool2")
	p2Params.StartAddr = netip.MustParseAddr("10.3.2.1")
	p2Params.EndAddr = netip.MustParseAddr("10.3.2.100")
	p2Params.NetMask = 24
	p2 := test.InsertSampleAddressPool(t, ctx, sqlcQ, p2Params)

	p3Params := test.NewSampleInsertAddressPoolParams("testPool3")
	p3Params.StartAddr = netip.MustParseAddr("10.3.3.1")
	p3Params.EndAddr = netip.MustParseAddr("10.3.3.100")
	p3Params.NetMask = 24
	p3 := test.InsertSampleAddressPool(t, ctx, sqlcQ, p3Params)

	p4Params := test.NewSampleInsertAddressPoolParams("testPool4")
	p4Params.StartAddr = netip.MustParseAddr("10.3.4.1")
	p4Params.EndAddr = netip.MustParseAddr("10.3.4.100")
	p4Params.NetMask = 24
	p4 := test.InsertSampleAddressPool(t, ctx, sqlcQ, p4Params)

	// Create overlapping pools for validation test
	overlapPool1, overlapPool2 := test.SetupOverlappingPools(t, ctx, sqlcQ)

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user1 := test.InsertSampleUser(t, ctx, pgq, "testUser").ToAPI()
	user2 := test.InsertSampleUser(t, ctx, pgq, "testUser").ToAPI()
	test.InsertSampleDeviceTemplateAPI(
		t, ctx, vcManager, user1.ID, []*api.AddressPool{pg.ConvertDBToAPIPool(p1), pg.ConvertDBToAPIPool(p2)})

	dt := &api.DeviceTemplate{
		InterfaceName: "updatedName",
		AddressPools: []*api.AddressPool{
			pg.ConvertDBToAPIPool(p3),
			pg.ConvertDBToAPIPool(p4),
		},
		ListenPort: "9090",
		DNS:        []string{"updated test dns1", "new test dns2"},
		MTU:        "8000",
	}

	getCounts := func() []int {
		return countTablesRows(t, ctx, "device_templates", "device_template_address_pools", "device_template_wg_iface_dnss")
	}

	cases := []struct {
		name           string
		userID         int
		deviceTemplate *api.DeviceTemplate
		expectFailure  bool
	}{
		{
			name:           "update device template",
			userID:         user1.ID,
			deviceTemplate: dt,
			expectFailure:  false,
		},
		{
			name:           "no device template exists with user id",
			userID:         user2.ID,
			deviceTemplate: dt,
			expectFailure:  true,
		},
		{
			name:   "update with overlapping address pools",
			userID: user1.ID,
			deviceTemplate: &api.DeviceTemplate{
				InterfaceName: "egtest-overlap",
				AddressPools: []*api.AddressPool{
					pg.ConvertDBToAPIPool(overlapPool1),
					pg.ConvertDBToAPIPool(overlapPool2),
				},
				ListenPort: "8080",
				DNS:        []string{"test dns1"},
				MTU:        "9000",
			},
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			counts := getCounts()
			wgConfigCountBefore, ifaceAddrPoolsCountBefore, ifaceDnsCountBefore := counts[0], counts[1], counts[2]

			err := vcManager.UpdateDeviceTemplateByUserID(ctx, tc.userID, tc.deviceTemplate)

			counts = getCounts()
			wgConfigCountAfter, ifaceAddrPoolsCountAfter, ifaceDnsCountAfter := counts[0], counts[1], counts[2]

			if tc.expectFailure {
				require.Error(t, err)

				require.Equal(t, wgConfigCountAfter, wgConfigCountBefore)
				require.Equal(t, ifaceAddrPoolsCountAfter, ifaceAddrPoolsCountBefore)
				require.Equal(t, ifaceDnsCountAfter, ifaceDnsCountBefore)
				return
			}
			require.NoError(t, err)

			require.Equal(t, wgConfigCountAfter, wgConfigCountBefore)
			require.Equal(t, ifaceAddrPoolsCountAfter, ifaceAddrPoolsCountBefore)
			require.Equal(t, ifaceDnsCountAfter, ifaceDnsCountBefore+1)

			got, err := vcManager.GetDeviceTemplateByUserID(ctx, tc.userID)
			require.NoError(t, err)
			got.ID = 0
			require.Equal(t, tc.deviceTemplate, got)
		})
	}
}

func TestIntegrationGetWireGuardDeviceAdjacencies(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	ss := make(map[int]api.Server)
	server1 := test.InsertSampleServer(t, ctx, conn, "server1").ToAPI()
	server2 := test.InsertSampleServer(t, ctx, conn, "server2", test.WithHealthCheckAddress(nil)).ToAPI()
	ss[server1.ID] = server1
	ss[server2.ID] = server2

	var err error
	scs := make(map[int]*pg.WireGuardConfig)
	scs[server1.ID], err = vcm.GetWireGuardServerConfig(ctx, sqlcQ, test.TestEncryptionKey, server1.ID)
	require.NoError(t, err)
	scs[server2.ID], err = vcm.GetWireGuardServerConfig(ctx, sqlcQ, test.TestEncryptionKey, server2.ID)
	require.NoError(t, err)

	pool := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("name"))

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user1 := test.InsertSampleUser(t, ctx, pgq, "testUser1").ToAPI()
	user2 := test.InsertSampleUser(t, ctx, pgq, "testUser2").ToAPI()

	dt1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user1.ID, []int{pool.ID})
	dt2 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user2.ID, []int{pool.ID})

	device1 := test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt1.ID)
	device2 := test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt2.ID)

	adjs := make(map[int]*sqlc.Adjacency)
	adjs[server1.ID] = test.InsertSampleAdjacency(t, ctx, sqlcQ, server1.ID, device1.ID)
	adjs[server2.ID] = test.InsertSampleAdjacency(t, ctx, sqlcQ, server2.ID, device1.ID)

	cases := []struct {
		name              string
		deviceID          int
		servers           map[int]api.Server
		adjacencies       map[int]*sqlc.Adjacency
		serverConfigs     map[int]*pg.WireGuardConfig
		expectEmptyResult bool
	}{
		{
			name:              "get wg device peers",
			deviceID:          device1.ID,
			servers:           ss,
			adjacencies:       adjs,
			serverConfigs:     scs,
			expectEmptyResult: false,
		},
		{
			name:              "no wg device peer exists with device id",
			deviceID:          device2.ID,
			expectEmptyResult: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wgps, err := vcManager.GetWireGuardDeviceAdjacencies(ctx, tc.deviceID)
			require.NoError(t, err)

			if tc.expectEmptyResult {
				require.Len(t, wgps, 0)
				return
			}

			require.Len(t, wgps, len(tc.adjacencies))

			for _, p := range wgps {
				var ssips, csips []string
				for _, ssip := range tc.adjacencies[p.Server.ID].ServerSideAllowedIps {
					ssips = append(ssips, ssip.String())
				}
				for _, csip := range tc.adjacencies[p.Server.ID].ClientSideAllowedIps {
					csips = append(csips, csip.String())
				}

				require.Equal(t, test.OpenString(tc.adjacencies[p.Server.ID].PresharedKeyEncrypted), p.Config.PresharedKey)
				require.Equal(t, ssips, p.Config.OtherSideAllowedIPs)
				require.Equal(t, csips, p.Config.AllowedIPs)
				require.Equal(t, tc.serverConfigs[p.Server.ID].PublicKey, p.Config.PublicKey)
				require.Empty(t, p.Config.PersistentKeepalive)
				require.Equal(t, tc.servers[p.Server.ID].ID, p.Server.ID)
				require.Equal(t, tc.servers[p.Server.ID].Description, p.Server.Description)
				require.Equal(t, tc.servers[p.Server.ID].Endpoint, p.Server.Endpoint)
				require.Equal(t, tc.servers[p.Server.ID].Name, p.Server.Name)
				if tc.servers[p.Server.ID].HealthCheckAddress != "" {
					require.Equal(t, tc.servers[p.Server.ID].HealthCheckAddress, p.Server.HealthCheckAddress)
				} else {
					require.Empty(t, p.Server.HealthCheckAddress)
				}
			}
		})
	}
}

func countTablesRows(t *testing.T, ctx context.Context, tables ...string) []int {
	counts := make([]int, 0, len(tables))
	for _, table := range tables {
		c := test.CountTableRows(t, q.GetDbConnection(ctx), table)
		counts = append(counts, c)
	}
	return counts
}

func TestIntegrationGetWireGuardServerAdjacencies(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server").ToAPI()
	server2 := test.InsertSampleServer(t, ctx, conn, "server").ToAPI()

	pool := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("name"))

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user1 := test.InsertSampleUser(t, ctx, pgq, "testUser").ToAPI()
	user2 := test.InsertSampleUser(t, ctx, pgq, "testUser").ToAPI()

	dt1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user1.ID, []int{pool.ID})
	dt2 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user2.ID, []int{pool.ID})

	device1a := test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt1.ID)
	device1b := test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt1.ID)
	device2 := test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt2.ID)
	devicePublicKeys := make(map[int]string)
	devicePublicKeys[device1a.ID] = device1a.PublicKey
	devicePublicKeys[device1b.ID] = device1b.PublicKey
	devicePublicKeys[device2.ID] = device2.PublicKey

	adjs := make(map[int]*sqlc.Adjacency)
	adjs[device1a.ID] = test.InsertSampleAdjacency(t, ctx, sqlcQ, server1.ID, device1a.ID)
	adjs[device1b.ID] = test.InsertSampleAdjacency(t, ctx, sqlcQ, server1.ID, device1b.ID)
	adjs[device2.ID] = test.InsertSampleAdjacency(t, ctx, sqlcQ, server1.ID, device2.ID)

	cases := []struct {
		name              string
		serverID          int
		devicePublicKeys  map[int]string
		adjacencies       map[int]*sqlc.Adjacency
		expectEmptyResult bool
	}{
		{
			name:              "get wg server peer",
			serverID:          server1.ID,
			devicePublicKeys:  devicePublicKeys,
			adjacencies:       adjs,
			expectEmptyResult: false,
		},
		{
			name:              "no server peer exists with server id",
			serverID:          server2.ID,
			expectEmptyResult: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wgps, err := vcManager.GetWireGuardServerAdjacencies(ctx, tc.serverID)

			if tc.expectEmptyResult {
				require.Len(t, wgps, 0)
				return
			}

			require.NoError(t, err)
			require.Len(t, wgps, len(tc.adjacencies))

			for _, p := range wgps {
				var ssips, csips []string
				for _, ssip := range tc.adjacencies[p.DeviceID].ServerSideAllowedIps {
					ssips = append(ssips, ssip.String())
				}
				for _, csip := range tc.adjacencies[p.DeviceID].ClientSideAllowedIps {
					csips = append(csips, csip.String())
				}

				require.Equal(t, test.OpenString(tc.adjacencies[p.DeviceID].PresharedKeyEncrypted), p.Config.PresharedKey)
				require.Equal(t, ssips, p.Config.OtherSideAllowedIPs)
				require.Equal(t, csips, p.Config.AllowedIPs)
				require.Equal(t, tc.devicePublicKeys[p.DeviceID], p.Config.PublicKey)
				require.Empty(t, p.Config.Endpoint)
				require.Empty(t, p.Config.PersistentKeepalive)
			}
		})
	}
}

func TestIntegrationDeleteDeviceTemplateByUserID(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user := test.InsertSampleUser(t, ctx, pgq, "testUser").ToAPI()
	pool := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("testPool"))
	apiPool := pg.ConvertDBToAPIPool(pool)
	_ = test.InsertSampleDeviceTemplateAPI(t, ctx, vcManager, user.ID, []*api.AddressPool{apiPool})

	dt, err := sqlcQ.GetDeviceTemplateByUserID(ctx, user.ID)
	require.NoError(t, err)
	_ = test.InsertSampleDeviceManual(t, ctx, sqlcQ, dt.ID)
	_ = test.InsertSampleDeviceManual(t, ctx, sqlcQ, dt.ID)

	wgConfigCountBefore := test.CountTableRows(t, q.GetDbConnection(ctx), "device_templates")
	deviceCountBefore := test.CountTableRows(t, q.GetDbConnection(ctx), "devices")

	err = vcManager.DeleteDeviceTemplateByUserID(ctx, user.ID)
	require.NoError(t, err)

	wgConfigCountAfter := test.CountTableRows(t, q.GetDbConnection(ctx), "device_templates")
	deviceCountAfter := test.CountTableRows(t, q.GetDbConnection(ctx), "devices")

	require.Equal(t, wgConfigCountAfter, wgConfigCountBefore-1)
	require.Equal(t, deviceCountAfter, deviceCountBefore-2)
}

func TestIntegrationGetDeviceWireGuardConfig(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	p1 := test.InsertSampleAddressPool(t, ctx, sqlcQ, &sqlc.InsertAddressPoolParams{
		Name:        "testPool1",
		Description: "desc1",
		StartAddr:   netip.MustParseAddr("10.20.30.40"),
		EndAddr:     netip.MustParseAddr("10.20.30.99"),
		NetMask:     24,
	})
	p2 := test.InsertSampleAddressPool(t, ctx, sqlcQ, &sqlc.InsertAddressPoolParams{
		Name:        "testPool2",
		Description: "desc2",
		StartAddr:   netip.MustParseAddr("1.1.1.1"),
		EndAddr:     netip.MustParseAddr("1.1.255.255"),
		NetMask:     16,
	})
	_ = test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("testPool3"))

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user1 := test.InsertSampleUser(t, ctx, pgq, "testUser").ToAPI()
	user2 := test.InsertSampleUser(t, ctx, pgq, "testUser").ToAPI()
	dt1 := test.InsertSampleDeviceTemplateAPI(t, ctx, vcManager, user1.ID, []*api.AddressPool{
		user.ConvertDBToAPIPool(*p1), user.ConvertDBToAPIPool(*p2)},
	)
	dt2 := test.InsertSampleDeviceTemplateAPI(t, ctx, vcManager, user2.ID, []*api.AddressPool{
		user.ConvertDBToAPIPool(*p1)},
	)

	device := test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt1.ID)
	test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt1.ID)
	test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt2.ID)

	cases := []struct {
		name             string
		deviceID         int
		expectedWGConfig *api.ServerWireGuardInterface
		expectFailure    bool
	}{
		{
			name:     "get device wg config",
			deviceID: device.ID,
			expectedWGConfig: &api.ServerWireGuardInterface{
				Name:                dt1.InterfaceName,
				PrivateKey:          device.PrivateKey,
				PublicKey:           device.PublicKey,
				Addresses:           []string{"10.20.30.40/24", "1.1.1.1/16"},
				ListenPort:          dt1.ListenPort,
				DNS:                 dt1.DNS,
				MTU:                 dt1.MTU,
				PersistentKeepalive: "",
			},
			expectFailure: false,
		},
		{
			name:          "no device exists with id",
			deviceID:      999,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wgc, err := vcManager.GetDeviceWireGuardConfig(ctx, tc.deviceID)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expectedWGConfig, wgc)
		})
	}
}

func TestIntegrationGetWireGuardServerConfig(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	var err error
	server := test.NewSampleServer(t, "server")
	config := test.NewSampleWGServerConfig()
	server.ID, err = vcm.InsertServerWithWireGuardConfig(ctx, conn, test.TestEncryptionKey, server, config)
	require.NoError(t, err)

	cases := []struct {
		name             string
		serverID         int
		expectedWGConfig *api.ServerWireGuardInterface
		expectFailure    bool
	}{
		{
			name:             "get wg server config",
			serverID:         server.ID,
			expectedWGConfig: config.ToAPI(),
			expectFailure:    false,
		},
		{
			name:          "no config exists with server id",
			serverID:      2,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wgc, err := vcManager.GetWireGuardServerConfig(ctx, tc.serverID)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expectedWGConfig, wgc)
		})
	}
}

func TestIntegrationUpdateWireGuardServerConfig(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1").ToAPI()

	wgc := &api.ServerWireGuardInterface{
		Name:                "updatedName",
		PrivateKey:          "Bl+OePMXvLrymZRtCxzmwaw6IkCe2Wel00qE5iUUhi0=",
		PublicKey:           "Bl+OePMXvLrymZRtCxzmwaw6IkCe2Wel00qE5iUUhi0=",
		Addresses:           []string{"1.1.1.1/24", "1.2.3.4/18", "1.2.3.4/18"},
		ListenPort:          "9090",
		DNS:                 []string{"updated test dns1", "new test dns2"},
		MTU:                 "8000",
		PersistentKeepalive: "30",
	}

	getCounts := func() []int {
		return countTablesRows(t, ctx, "server_wg_configs", "server_wg_iface_addresses", "server_wg_iface_dnss")
	}

	cases := []struct {
		name          string
		serverID      int
		newWGConfig   *api.ServerWireGuardInterface
		expectFailure bool
	}{
		{
			name:          "update wg server config",
			serverID:      server1.ID,
			newWGConfig:   wgc,
			expectFailure: false,
		},
		{
			name:          "no wg server config exists with server id",
			serverID:      2,
			newWGConfig:   wgc,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			counts := getCounts()
			wgConfigCountBefore, ifaceAddrsCountBefore, ifaceDnsCountBefore := counts[0], counts[1], counts[2]

			err := vcManager.UpdateWireGuardServerConfig(ctx, tc.serverID, tc.newWGConfig)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			counts = getCounts()
			wgConfigCountAfter, ifaceAddrsCountAfter, ifaceDnsCountAfter := counts[0], counts[1], counts[2]

			require.Equal(t, wgConfigCountAfter, wgConfigCountBefore)
			require.Equal(t, ifaceAddrsCountAfter, ifaceAddrsCountBefore+1)
			require.Equal(t, ifaceDnsCountAfter, ifaceDnsCountBefore+1)

			c, err := vcManager.GetWireGuardServerConfig(ctx, tc.serverID)
			require.NoError(t, err)
			require.Equal(t, tc.newWGConfig, c)
		})
	}
}

func TestIntegrationQueryUsedAddrsForSubnet(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server := test.InsertSampleServer(t, ctx, conn, "server1").ToAPI()
	config, err := vcManager.GetWireGuardServerConfig(ctx, server.ID)
	require.NoError(t, err)

	cases := []struct {
		name            string
		subnet          netip.Prefix
		expectedAddrLen int
	}{
		{
			name:            "get addresses",
			subnet:          netip.MustParsePrefix("1.1.0.0/16"),
			expectedAddrLen: 2,
		},
		{
			name:            "different subnet",
			subnet:          netip.MustParsePrefix("1.2.3.0/24"),
			expectedAddrLen: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addrs, err := vcManager.QueryUsedAddrsForSubnet(ctx, tc.subnet)
			require.NoError(t, err)
			require.Len(t, addrs, tc.expectedAddrLen)

			if tc.expectedAddrLen == 0 {
				return
			}

			require.Contains(t, config.Addresses[0], addrs[0].String())
			require.Contains(t, config.Addresses[1], addrs[1].String())
		})
	}
}

func TestIsHealthcheckAddrInAllowedIPs(t *testing.T) {

	hcAddr, err := netip.ParseAddr("192.168.0.5")
	require.NoError(t, err)

	cases := []struct {
		name       string
		allowedIPs []netip.Prefix
		hcAddr     *netip.Addr
		want       bool
	}{
		{
			name:   "hc address explicitly in the list",
			hcAddr: &hcAddr,
			allowedIPs: []netip.Prefix{
				netip.MustParsePrefix("1.1.1.2/24"),
				netip.MustParsePrefix("192.168.0.5/32"),
			},
			want: true,
		},
		{
			name:       "hc address included into allowed network from the list",
			hcAddr:     &hcAddr,
			allowedIPs: []netip.Prefix{netip.MustParsePrefix("192.168.0.0/24")},
			want:       true,
		},
		{
			name:   "hc address is not in the list",
			hcAddr: &hcAddr,
			allowedIPs: []netip.Prefix{
				netip.MustParsePrefix("0.0.0.0/24"),
				netip.MustParsePrefix("1.1.1.2/24"),
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := vcm.IsHealthcheckAddrInAllowedIPs(tc.allowedIPs, tc.hcAddr)
			require.Equal(t, tc.want, result)
		})
	}
}

func TestEnsureHealthcheckAddrInAllowedIPs(t *testing.T) {
	hcAddr := netip.MustParseAddr("192.168.0.5")

	cases := []struct {
		name           string
		allowedIPs     []netip.Prefix
		hcAddr         *netip.Addr
		wantModified   bool
		wantContainsHC bool
	}{
		{
			name:           "hc address is nil",
			hcAddr:         nil,
			allowedIPs:     []netip.Prefix{netip.MustParsePrefix("1.1.1.2/24")},
			wantModified:   false,
			wantContainsHC: false,
		},
		{
			name:           "hc address explicitly in the list",
			hcAddr:         &hcAddr,
			allowedIPs:     []netip.Prefix{netip.MustParsePrefix("1.1.1.2/24"), netip.MustParsePrefix("192.168.0.5/32")},
			wantModified:   false,
			wantContainsHC: true,
		},
		{
			name:           "hc address included in allowed network from the list",
			hcAddr:         &hcAddr,
			allowedIPs:     []netip.Prefix{netip.MustParsePrefix("192.168.0.0/24")},
			wantModified:   false,
			wantContainsHC: true,
		},
		{
			name:           "hc address not in the list - should be added",
			hcAddr:         &hcAddr,
			allowedIPs:     []netip.Prefix{netip.MustParsePrefix("0.0.0.0/24"), netip.MustParsePrefix("1.1.1.2/24")},
			wantModified:   true,
			wantContainsHC: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, modified := vcm.EnsureHealthcheckAddrInAllowedIPs(tc.allowedIPs, tc.hcAddr)
			require.Equal(t, tc.wantModified, modified)

			if tc.wantContainsHC {
				contains := vcm.IsHealthcheckAddrInAllowedIPs(result, tc.hcAddr)
				require.True(t, contains)
			}

			if !tc.wantModified {
				require.Equal(t, len(tc.allowedIPs), len(result))
			} else {
				require.Equal(t, len(tc.allowedIPs)+1, len(result))
			}
		})
	}
}

func TestRemoveHealthcheckAddrFromAllowedIPs(t *testing.T) {
	hcAddr := netip.MustParseAddr("192.168.0.5")
	hcAddrIPv6 := netip.MustParseAddr("2001:db8::1")

	cases := []struct {
		name         string
		allowedIPs   []netip.Prefix
		hcAddr       *netip.Addr
		wantModified bool
		wantLen      int
	}{
		{
			name:         "hc address is nil",
			hcAddr:       nil,
			allowedIPs:   []netip.Prefix{netip.MustParsePrefix("1.1.1.2/24")},
			wantModified: false,
			wantLen:      1,
		},
		{
			name:         "hc address not in the list",
			hcAddr:       &hcAddr,
			allowedIPs:   []netip.Prefix{netip.MustParsePrefix("1.1.1.2/24"), netip.MustParsePrefix("10.0.0.0/8")},
			wantModified: false,
			wantLen:      2,
		},
		{
			name:         "hc address explicitly in the list - should be removed",
			hcAddr:       &hcAddr,
			allowedIPs:   []netip.Prefix{netip.MustParsePrefix("1.1.1.2/24"), netip.MustParsePrefix("192.168.0.5/32"), netip.MustParsePrefix("10.0.0.0/8")},
			wantModified: true,
			wantLen:      2,
		},
		{
			name:         "only hc address in the list - should be removed leaving empty",
			hcAddr:       &hcAddr,
			allowedIPs:   []netip.Prefix{netip.MustParsePrefix("192.168.0.5/32")},
			wantModified: true,
			wantLen:      0,
		},
		{
			name:         "hc address in broader network - should not be removed",
			hcAddr:       &hcAddr,
			allowedIPs:   []netip.Prefix{netip.MustParsePrefix("192.168.0.0/24")},
			wantModified: false,
			wantLen:      1,
		},
		{
			name:         "IPv6 hc address explicitly in the list - should be removed",
			hcAddr:       &hcAddrIPv6,
			allowedIPs:   []netip.Prefix{netip.MustParsePrefix("1.1.1.2/24"), netip.MustParsePrefix("2001:db8::1/128")},
			wantModified: true,
			wantLen:      1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, modified := vcm.RemoveHealthcheckAddrFromAllowedIPs(tc.allowedIPs, tc.hcAddr)
			require.Equal(t, tc.wantModified, modified)
			require.Equal(t, tc.wantLen, len(result))

			if tc.wantModified {
				contains := vcm.IsHealthcheckAddrInAllowedIPs(result, tc.hcAddr)
				require.False(t, contains)
			}
		})
	}
}

func TestIntegrationDeleteInvalidSessionsByDeviceTemplate(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	conn := q.GetDbConnection(ctx)

	user1 := test.InsertSampleUser(t, ctx, pgq, "user1")
	user2 := test.InsertSampleUser(t, ctx, pgq, "user2")
	user3 := test.InsertSampleUser(t, ctx, pgq, "user3")
	user4 := test.InsertSampleUser(t, ctx, pgq, "user4")
	user5 := test.InsertSampleUser(t, ctx, pgq, "user5")
	user6 := test.InsertSampleUser(t, ctx, pgq, "user6")

	addressPool1 := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("testPool1"))
	addressPool2 := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("testPool2"))

	// create separate templates for each test case to avoid session restoration
	templateNoChange := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user1.ID, []int{addressPool1.ID})
	templateListenPort := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user2.ID, []int{addressPool1.ID})
	templateMTU := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user3.ID, []int{addressPool1.ID})
	templateAddressPool := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user4.ID, []int{addressPool1.ID})
	templateDNS := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user5.ID, []int{addressPool1.ID})
	templateIfaceName := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user6.ID, []int{addressPool1.ID})

	d1NoChange, s1NoChange := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, templateNoChange.ID, nil)
	d2NoChange, s2NoChange := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, templateNoChange.ID, nil)
	d3NoChange, s3NoChange := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, templateNoChange.ID, nil)

	d1ListenPort, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, templateListenPort.ID, nil)
	d2ListenPort, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, templateListenPort.ID, nil)
	d3ListenPort, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, templateListenPort.ID, nil)

	d1MTU, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, templateMTU.ID, nil)
	d2MTU, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, templateMTU.ID, nil)
	d3MTU, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, templateMTU.ID, nil)

	d1AddressPool, s1AP := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, templateAddressPool.ID, nil)
	d2AddressPool, s2AP := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, templateAddressPool.ID, nil)
	d3AddressPool, s3AP := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, templateAddressPool.ID, nil)

	d1DNS, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, templateDNS.ID, nil)
	d2DNS, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, templateDNS.ID, nil)
	d3DNS, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, templateDNS.ID, nil)

	d1IfaceName, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, templateIfaceName.ID, nil)
	d2IfaceName, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, templateIfaceName.ID, nil)
	d3IfaceName, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, templateIfaceName.ID, nil)

	dtOld := api.DeviceTemplate{
		InterfaceName: "egtest",
		ListenPort:    "8080",
		DNS:           []string{"test dns1"},
		MTU:           "9000",
		AddressPools: []*api.AddressPool{
			{ID: addressPool1.ID},
		},
	}

	dtNewListenPort := dtOld
	dtNewListenPort.ListenPort = "9090"

	dtNewMTU := dtOld
	dtNewMTU.MTU = "8000"

	dtNewAddressPool := dtOld
	dtNewAddressPool.AddressPools = []*api.AddressPool{
		{ID: addressPool2.ID},
	}

	dtNewDNS := dtOld
	dtNewDNS.DNS = append(dtNewDNS.DNS, "test dns2")

	dtNewIfaceName := dtOld
	dtNewIfaceName.InterfaceName = "newname"

	cases := []struct {
		name               string
		templateID         int
		deviceIDs          []int
		oldDT, newDT       *api.DeviceTemplate
		expectedSessions   []string
		expectedNumDeleted int
	}{
		{
			name:               "nothing changed - keep",
			templateID:         templateNoChange.ID,
			deviceIDs:          []int{d1NoChange.ID, d2NoChange.ID, d3NoChange.ID},
			expectedSessions:   []string{s1NoChange, s2NoChange, s3NoChange},
			oldDT:              &dtOld,
			newDT:              &dtOld,
			expectedNumDeleted: 0,
		},
		{
			name:               "ListenPort changed - delete",
			templateID:         templateListenPort.ID,
			deviceIDs:          []int{d1ListenPort.ID, d2ListenPort.ID, d3ListenPort.ID},
			expectedSessions:   []string{"", "", ""},
			oldDT:              &dtOld,
			newDT:              &dtNewListenPort,
			expectedNumDeleted: 3,
		},
		{
			name:               "MTU changed - delete",
			templateID:         templateMTU.ID,
			deviceIDs:          []int{d1MTU.ID, d2MTU.ID, d3MTU.ID},
			expectedSessions:   []string{"", "", ""},
			oldDT:              &dtOld,
			newDT:              &dtNewMTU,
			expectedNumDeleted: 3,
		},
		{
			name:               "AddressPools changed - keep",
			templateID:         templateAddressPool.ID,
			deviceIDs:          []int{d1AddressPool.ID, d2AddressPool.ID, d3AddressPool.ID},
			expectedSessions:   []string{s1AP, s2AP, s3AP},
			oldDT:              &dtOld,
			newDT:              &dtNewAddressPool,
			expectedNumDeleted: 0,
		},
		{
			name:               "DNS changed - delete",
			templateID:         templateDNS.ID,
			deviceIDs:          []int{d1DNS.ID, d2DNS.ID, d3DNS.ID},
			expectedSessions:   []string{"", "", ""},
			oldDT:              &dtOld,
			newDT:              &dtNewDNS,
			expectedNumDeleted: 3,
		},
		{
			name:               "Interface name changed - delete",
			templateID:         templateIfaceName.ID,
			deviceIDs:          []int{d1IfaceName.ID, d2IfaceName.ID, d3IfaceName.ID},
			expectedSessions:   []string{"", "", ""},
			oldDT:              &dtOld,
			newDT:              &dtNewIfaceName,
			expectedNumDeleted: 3,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := test.CountSessions(t, conn)

			err := vcManager.DeleteInvalidSessionsByDeviceTemplate(ctx, tc.templateID, tc.oldDT, tc.newDT)
			require.NoError(t, err)

			after := test.CountSessions(t, conn)
			require.Equal(t, before-tc.expectedNumDeleted, after)

			for i, deviceID := range tc.deviceIDs {
				device, err := sqlcQ.GetDeviceByID(ctx, deviceID)
				require.NoError(t, err)

				if tc.expectedNumDeleted != 0 {
					require.Empty(t, device.SessionID)
					continue
				}
				require.Equal(t, tc.expectedSessions[i], device.SessionID)
				require.True(t, device.LastTimeConnected.Valid)
			}
		})
	}
}

func TestIntegrationDeleteInvalidSessionsByAdjacency(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	conn := q.GetDbConnection(ctx)

	server1 := test.InsertSampleServer(t, ctx, conn, "server1")
	server2 := test.InsertSampleServer(t, ctx, conn, "server2")

	user1 := test.InsertSampleUser(t, ctx, pgq, "user1")
	user2 := test.InsertSampleUser(t, ctx, pgq, "user2")
	user3 := test.InsertSampleUser(t, ctx, pgq, "user3")
	user4 := test.InsertSampleUser(t, ctx, pgq, "user4")
	user5 := test.InsertSampleUser(t, ctx, pgq, "user5")
	user6 := test.InsertSampleUser(t, ctx, pgq, "user6")
	user7 := test.InsertSampleUser(t, ctx, pgq, "user7")

	addressPool := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("testPool"))

	deviceTemplate1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user1.ID, []int{addressPool.ID})
	deviceTemplate2 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user2.ID, []int{addressPool.ID})
	deviceTemplate3 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user3.ID, []int{addressPool.ID})
	deviceTemplate4 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user4.ID, []int{addressPool.ID})
	deviceTemplate5 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user5.ID, []int{addressPool.ID})
	deviceTemplate6 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user6.ID, []int{addressPool.ID})
	deviceTemplate7 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user7.ID, []int{addressPool.ID})

	d1, s1 := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate1.ID, nil)
	d2, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate2.ID, nil)
	d3a, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate3.ID, nil)
	d3b, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate3.ID, nil)
	d3Decoy, s3Decoy := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate3.ID, nil)
	d4, s4 := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate4.ID, nil)
	d5, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate5.ID, nil)
	d6, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate6.ID, nil)
	d7, s7 := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate7.ID, nil)

	adj1Old := test.InsertSampleAdjacency(t, ctx, sqlcQ, server1.ID, d1.ID)
	adj2Old := test.InsertSampleAdjacency(t, ctx, sqlcQ, server1.ID, d2.ID)
	adj3Old := test.InsertSampleAdjacency(t, ctx, sqlcQ, server1.ID, d3a.ID)
	adj4Old := test.InsertSampleAdjacency(t, ctx, sqlcQ, server1.ID, d4.ID,
		test.WithSSAIPsAdj([]netip.Prefix{netip.MustParsePrefix("1.2.3.0/24")}))
	adj5Old := test.InsertSampleAdjacency(t, ctx, sqlcQ, server1.ID, d5.ID,
		test.WithCSAIPsAdj([]netip.Prefix{netip.MustParsePrefix("5.6.7.0/24")}))
	adj6Old := test.InsertSampleAdjacency(t, ctx, sqlcQ, server1.ID, d6.ID, test.WithPresharedKeyAdj("pskToBeChanged"))
	adj7Old := test.InsertSampleAdjacency(t, ctx, sqlcQ, server1.ID, d7.ID, test.WithPresharedKeyAdj("pskToStayTheSame"))

	adj2NewServer := *adj2Old
	adj2NewServer.ServerID = server2.ID

	adj3NewDevice := *adj3Old
	adj3NewDevice.DeviceID = d3b.ID

	adj4NewSSAIPs := *adj4Old
	adj4NewSSAIPs.ServerSideAllowedIps = []netip.Prefix{netip.MustParsePrefix("10.20.30.0/24")}

	adj5NewCSAIPs := *adj5Old
	adj5NewCSAIPs.ClientSideAllowedIps = []netip.Prefix{netip.MustParsePrefix("50.60.70.0/24")}

	adj6NewPresharedKey := *adj6Old
	adj6NewPresharedKey.PresharedKeyEncrypted = test.Seal("newpsk")

	adj7NewSamePresharedKey := *adj7Old
	adj7NewSamePresharedKey.PresharedKeyEncrypted = test.Seal("pskToStayTheSame")

	cases := []struct {
		name               string
		oldAdj, newAdj     *sqlc.Adjacency
		deviceIDs          []int
		expectedSessions   []string
		expectedNumDeleted int
	}{
		{
			name:               "nothing changed - keep",
			oldAdj:             adj1Old,
			newAdj:             adj1Old,
			deviceIDs:          []int{d1.ID},
			expectedSessions:   []string{s1},
			expectedNumDeleted: 0,
		},
		{
			name:               "adjacency moved to different server - delete",
			oldAdj:             adj2Old,
			newAdj:             &adj2NewServer,
			deviceIDs:          []int{d2.ID},
			expectedSessions:   []string{""},
			expectedNumDeleted: 1,
		},
		{
			name:               "adjacency moved to different device - delete sessions for both devices",
			oldAdj:             adj3Old,
			newAdj:             &adj3NewDevice,
			deviceIDs:          []int{d3a.ID, d3b.ID, d3Decoy.ID},
			expectedSessions:   []string{"", "", s3Decoy},
			expectedNumDeleted: 2,
		},
		{
			name:               "server side allowed IPs changed - keep",
			oldAdj:             adj4Old,
			newAdj:             &adj4NewSSAIPs,
			deviceIDs:          []int{d4.ID},
			expectedSessions:   []string{s4},
			expectedNumDeleted: 0,
		},
		{
			name:               "client side allowed IPs changed - delete",
			oldAdj:             adj5Old,
			newAdj:             &adj5NewCSAIPs,
			deviceIDs:          []int{d5.ID},
			expectedSessions:   []string{""},
			expectedNumDeleted: 1,
		},
		{
			name:               "preshared key changed - delete",
			oldAdj:             adj6Old,
			newAdj:             &adj6NewPresharedKey,
			deviceIDs:          []int{d6.ID},
			expectedSessions:   []string{""},
			expectedNumDeleted: 1,
		},
		{
			// test case against potential bug if code accidentaly compares pointers instead of values
			name:               "preshared key not changed, but different pointer - keep",
			oldAdj:             adj7Old,
			newAdj:             &adj7NewSamePresharedKey,
			deviceIDs:          []int{d7.ID},
			expectedSessions:   []string{s7},
			expectedNumDeleted: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := test.CountSessions(t, conn)

			err := vcManager.DeleteInvalidSessionsByAdjacency(ctx, tc.oldAdj, tc.newAdj)
			require.NoError(t, err)

			after := test.CountSessions(t, conn)
			require.Equal(t, before-tc.expectedNumDeleted, after)

			for i, deviceID := range tc.deviceIDs {
				device, err := sqlcQ.GetDeviceByID(ctx, deviceID)
				require.NoError(t, err)
				require.Equal(t, tc.expectedSessions[i], device.SessionID)
			}
		})
	}
}

func TestIntegrationDeleteInvalidSessionsByServer(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	conn := q.GetDbConnection(ctx)

	server1NoChange := test.InsertSampleServer(t, ctx, conn, "server1NoChange")
	server2Name := test.InsertSampleServer(t, ctx, conn, "server2Name")
	server3Endpoint := test.InsertSampleServer(t, ctx, conn, "server3Endpoint", test.WithEndpoint(netip.MustParseAddr("1.2.3.4")))
	hcOld := netip.MustParseAddr("5.6.7.8")
	server4HC := test.InsertSampleServer(t, ctx, conn, "server4HC", test.WithHealthCheckAddress(&hcOld))
	server5Description := test.InsertSampleServer(t, ctx, conn, "server5Description", test.WithDescription("some description"))

	serverMulti := test.InsertSampleServer(t, ctx, conn, "serverMulti", test.WithEndpoint(netip.MustParseAddr("100.0.0.0")))

	serverDecoy := test.InsertSampleServer(t, ctx, conn, "serverDecoy")

	user1 := test.InsertSampleUser(t, ctx, pgq, "user1")
	user2 := test.InsertSampleUser(t, ctx, pgq, "user2")
	user3 := test.InsertSampleUser(t, ctx, pgq, "user3")
	user4 := test.InsertSampleUser(t, ctx, pgq, "user4")
	user5 := test.InsertSampleUser(t, ctx, pgq, "user5")
	userMulti6 := test.InsertSampleUser(t, ctx, pgq, "userMulti6")
	userMulti7 := test.InsertSampleUser(t, ctx, pgq, "userMulti7")

	addressPool := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("testPool"))

	deviceTemplate1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user1.ID, []int{addressPool.ID})
	deviceTemplate2 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user2.ID, []int{addressPool.ID})
	deviceTemplate3 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user3.ID, []int{addressPool.ID})
	deviceTemplate4 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user4.ID, []int{addressPool.ID})
	deviceTemplate5 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user5.ID, []int{addressPool.ID})
	deviceTemplateMulti6 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, userMulti6.ID, []int{addressPool.ID})
	deviceTemplateMulti7 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, userMulti7.ID, []int{addressPool.ID})

	d1, s1 := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate1.ID, nil)
	d2, s2 := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate2.ID, nil)
	d3, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate3.ID, nil)
	d4, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate4.ID, nil)
	d5, s5 := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate5.ID, nil)
	d6real, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplateMulti6.ID, nil)
	d6decoy, s6decoy := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplateMulti6.ID, nil)
	d6both, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplateMulti6.ID, nil)
	d6none, s6none := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplateMulti6.ID, nil)
	d7real, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplateMulti7.ID, nil)
	d7decoy, s7decoy := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplateMulti7.ID, nil)
	d7both, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplateMulti7.ID, nil)
	d7none, s7none := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplateMulti7.ID, nil)

	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, server1NoChange.ID, d1.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, server2Name.ID, d2.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, server3Endpoint.ID, d3.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, server4HC.ID, d4.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, server5Description.ID, d5.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, serverMulti.ID, d6real.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, serverDecoy.ID, d6decoy.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, serverMulti.ID, d6both.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, serverDecoy.ID, d6both.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, serverMulti.ID, d7real.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, serverDecoy.ID, d7decoy.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, serverMulti.ID, d7both.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, serverDecoy.ID, d7both.ID)

	server2New := *server2Name
	server2New.Name = "newname"

	server3New := *server3Endpoint
	server3New.Endpoint = netip.MustParseAddr("10.20.30.40")

	server4New := *server4HC
	hcNew := netip.MustParseAddr("50.60.70.80")
	server4New.HealthCheckAddress = &hcNew

	server5New := *server5Description
	server5New.Description = "new desc"

	serverMultiNew := *serverMulti
	serverMultiNew.Endpoint = netip.MustParseAddr("200.0.0.0")

	cases := []struct {
		name                 string
		oldServer, newServer *pg.Server
		deviceIDs            []int
		expectedSessions     []string
		expectedNumDeleted   int
	}{
		{
			name:               "nothing changed - keep",
			oldServer:          server1NoChange,
			newServer:          server1NoChange,
			deviceIDs:          []int{d1.ID},
			expectedSessions:   []string{s1},
			expectedNumDeleted: 0,
		},
		{
			name:               "name changed - keep",
			oldServer:          server2Name,
			newServer:          &server2New,
			deviceIDs:          []int{d2.ID},
			expectedSessions:   []string{s2},
			expectedNumDeleted: 0,
		},
		{
			name:               "endpoint changed - delete",
			oldServer:          server3Endpoint,
			newServer:          &server3New,
			deviceIDs:          []int{d3.ID},
			expectedSessions:   []string{""},
			expectedNumDeleted: 1,
		},
		{
			name:               "healthcheck addreses changed - delete",
			oldServer:          server4HC,
			newServer:          &server4New,
			deviceIDs:          []int{d4.ID},
			expectedSessions:   []string{""},
			expectedNumDeleted: 1,
		},
		{
			name:               "description changed - keep",
			oldServer:          server5Description,
			newServer:          &server5New,
			deviceIDs:          []int{d5.ID},
			expectedSessions:   []string{s5},
			expectedNumDeleted: 0,
		},
		{
			name:               "delete multiple sessions of devices across multiple users",
			oldServer:          serverMulti,
			newServer:          &serverMultiNew,
			deviceIDs:          []int{d6real.ID, d6decoy.ID, d6both.ID, d6none.ID, d7real.ID, d7decoy.ID, d7both.ID, d7none.ID},
			expectedSessions:   []string{"", s6decoy, "", s6none, "", s7decoy, "", s7none},
			expectedNumDeleted: 4,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := test.CountSessions(t, conn)

			err := vcManager.DeleteInvalidSessionsByServer(ctx, tc.oldServer, tc.newServer)
			require.NoError(t, err)

			after := test.CountSessions(t, conn)
			require.Equal(t, before-tc.expectedNumDeleted, after)

			for i, deviceID := range tc.deviceIDs {
				device, err := sqlcQ.GetDeviceByID(ctx, deviceID)
				require.NoError(t, err)
				require.Equal(t, tc.expectedSessions[i], device.SessionID)
			}
		})
	}
}

func TestIntegrationDeleteInvalidSessionsByServerWGConfig(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	conn := q.GetDbConnection(ctx)

	server1NoChange, wgc1NoChange := test.InsertSampleServerReturnWithWGC(
		t, ctx, conn, "server1NoChange",
	)
	server2InterfaceName, wgc2InterfaceName := test.InsertSampleServerReturnWithWGC(
		t, ctx, conn, "server2InterfaceName", test.WithInterfaceNameServerWGC("eg0"),
	)
	server3PublicKey, wgc3PublicKey := test.InsertSampleServerReturnWithWGC(
		t, ctx, conn, "server3PublicKey", test.WithPublicKeyServerWGC("pubkey"),
	)
	server4PrivateKey, wgc4PrivateKey := test.InsertSampleServerReturnWithWGC(
		t, ctx, conn, "server4PrivateKey", test.WithPrivateKeyServerWGC("privkey"),
	)
	server5IfaceAddrs, wgc5IfaceAddrs := test.InsertSampleServerReturnWithWGC(
		t, ctx, conn, "server5IfaceAddrs", test.WithAddressesServerWGC([]string{"1.1.1.1/32"}),
	)
	server6ListenPort, wgc6ListenPort := test.InsertSampleServerReturnWithWGC(
		t, ctx, conn, "server6ListenPort", test.WithListenPortServerWGC(9876),
	)
	server7MTU, wgc7MTU := test.InsertSampleServerReturnWithWGC(
		t, ctx, conn, "server7MTU", test.WithMTUServerWGC(1350),
	)
	server8PersistentKeepalive, wgc8PersistentKeepalive := test.InsertSampleServerReturnWithWGC(
		t, ctx, conn, "server8PersistentKeepalive", test.WithPersistentKeepaliveServerWGC(20),
	)

	serverMulti, wgcMulti := test.InsertSampleServerReturnWithWGC(t, ctx, conn, "serverMulti", test.WithPublicKeyServerWGC("aaa"))

	serverDecoy, _ := test.InsertSampleServerReturnWithWGC(t, ctx, conn, "serverDecoy")

	user1 := test.InsertSampleUser(t, ctx, pgq, "user1")
	user2 := test.InsertSampleUser(t, ctx, pgq, "user2")
	user3 := test.InsertSampleUser(t, ctx, pgq, "user3")
	user4 := test.InsertSampleUser(t, ctx, pgq, "user4")
	user5 := test.InsertSampleUser(t, ctx, pgq, "user5")
	user6 := test.InsertSampleUser(t, ctx, pgq, "user6")
	user7 := test.InsertSampleUser(t, ctx, pgq, "user7")
	user8 := test.InsertSampleUser(t, ctx, pgq, "user8")
	userMulti9 := test.InsertSampleUser(t, ctx, pgq, "userMulti9")
	userMulti10 := test.InsertSampleUser(t, ctx, pgq, "userMulti10")

	addressPool := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("testPool"))

	deviceTemplate1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user1.ID, []int{addressPool.ID})
	deviceTemplate2 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user2.ID, []int{addressPool.ID})
	deviceTemplate3 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user3.ID, []int{addressPool.ID})
	deviceTemplate4 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user4.ID, []int{addressPool.ID})
	deviceTemplate5 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user5.ID, []int{addressPool.ID})
	deviceTemplate6 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user6.ID, []int{addressPool.ID})
	deviceTemplate7 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user7.ID, []int{addressPool.ID})
	deviceTemplate8 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user8.ID, []int{addressPool.ID})
	deviceTemplateMulti9 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, userMulti9.ID, []int{addressPool.ID})
	deviceTemplateMulti10 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, userMulti10.ID, []int{addressPool.ID})

	d1, s1 := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate1.ID, nil)
	d2, s2 := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate2.ID, nil)
	d3, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate3.ID, nil)
	d4, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate4.ID, nil)
	d5, s5 := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate5.ID, nil)
	d6, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate6.ID, nil)
	d7, s7 := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate7.ID, nil)
	d8, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate8.ID, nil)
	d9real, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplateMulti9.ID, nil)
	d9decoy, s9decoy := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplateMulti9.ID, nil)
	d9both, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplateMulti9.ID, nil)
	d9none, s9none := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplateMulti9.ID, nil)
	d10real, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplateMulti10.ID, nil)
	d10decoy, s10decoy := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplateMulti10.ID, nil)
	d10both, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplateMulti10.ID, nil)
	d10none, s10none := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplateMulti10.ID, nil)

	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, server1NoChange.ID, d1.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, server2InterfaceName.ID, d2.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, server3PublicKey.ID, d3.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, server4PrivateKey.ID, d4.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, server5IfaceAddrs.ID, d5.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, server6ListenPort.ID, d6.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, server7MTU.ID, d7.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, server8PersistentKeepalive.ID, d8.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, serverMulti.ID, d9real.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, serverDecoy.ID, d9decoy.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, serverMulti.ID, d9both.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, serverDecoy.ID, d9both.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, serverMulti.ID, d10real.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, serverDecoy.ID, d10decoy.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, serverMulti.ID, d10both.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, serverDecoy.ID, d10both.ID)

	wgc2New := *wgc2InterfaceName
	wgc2New.InterfaceName = "eg1"

	wgc3New := *wgc3PublicKey
	wgc3New.PublicKey = "newpubkey"

	wgc4New := *wgc4PrivateKey
	wgc4New.PrivateKey = "newprivkey"

	wgc5New := *wgc5IfaceAddrs
	wgc5New.Addresses = []string{"2.2.2.2/32"}

	wgc6New := *wgc6ListenPort
	wgc6New.ListenPort = 9877

	wgc7New := *wgc7MTU
	wgc7New.MTU = 1351

	wgc8New := *wgc8PersistentKeepalive
	wgc8New.PersistentKeepalive = 21

	wgcMultiNew := *wgcMulti
	wgcMultiNew.PublicKey = "bbb"

	cases := []struct {
		name                       string
		serverID                   int
		oldServerWGC, newServerWGC *pg.WireGuardConfig
		deviceIDs                  []int
		expectedSessions           []string
		expectedNumDeleted         int
	}{
		{
			name:               "nothing changed - keep",
			serverID:           server1NoChange.ID,
			oldServerWGC:       wgc1NoChange,
			newServerWGC:       wgc1NoChange,
			deviceIDs:          []int{d1.ID},
			expectedSessions:   []string{s1},
			expectedNumDeleted: 0,
		},
		{
			name:               "Interface Name changed - keep",
			serverID:           server2InterfaceName.ID,
			oldServerWGC:       wgc2InterfaceName,
			newServerWGC:       &wgc2New,
			deviceIDs:          []int{d2.ID},
			expectedSessions:   []string{s2},
			expectedNumDeleted: 0,
		},
		{
			name:               "Public Key changed - delete",
			serverID:           server3PublicKey.ID,
			oldServerWGC:       wgc3PublicKey,
			newServerWGC:       &wgc3New,
			deviceIDs:          []int{d3.ID},
			expectedSessions:   []string{""},
			expectedNumDeleted: 1,
		},
		{
			name:               "Private Key changed - delete",
			serverID:           server4PrivateKey.ID,
			oldServerWGC:       wgc4PrivateKey,
			newServerWGC:       &wgc4New,
			deviceIDs:          []int{d4.ID},
			expectedSessions:   []string{""},
			expectedNumDeleted: 1,
		},
		{
			name:               "Interface Addresses changed - keep",
			serverID:           server5IfaceAddrs.ID,
			oldServerWGC:       wgc5IfaceAddrs,
			newServerWGC:       &wgc5New,
			deviceIDs:          []int{d5.ID},
			expectedSessions:   []string{s5},
			expectedNumDeleted: 0,
		},
		{
			name:               "Listen Port changed - delete",
			serverID:           server6ListenPort.ID,
			oldServerWGC:       wgc6ListenPort,
			newServerWGC:       &wgc6New,
			deviceIDs:          []int{d6.ID},
			expectedSessions:   []string{""},
			expectedNumDeleted: 1,
		},
		{
			name:               "MTU changed - keep",
			serverID:           server7MTU.ID,
			oldServerWGC:       wgc7MTU,
			newServerWGC:       &wgc7New,
			deviceIDs:          []int{d7.ID},
			expectedSessions:   []string{s7},
			expectedNumDeleted: 0,
		},
		{
			name:               "Persistent Keepalive changed - delete",
			serverID:           server8PersistentKeepalive.ID,
			oldServerWGC:       wgc8PersistentKeepalive,
			newServerWGC:       &wgc8New,
			deviceIDs:          []int{d8.ID},
			expectedSessions:   []string{""},
			expectedNumDeleted: 1,
		},
		{
			name:               "delete multiple sessions of devices across multiple users",
			serverID:           serverMulti.ID,
			oldServerWGC:       wgcMulti,
			newServerWGC:       &wgcMultiNew,
			deviceIDs:          []int{d9real.ID, d9decoy.ID, d9both.ID, d9none.ID, d10real.ID, d10decoy.ID, d10both.ID, d10none.ID},
			expectedSessions:   []string{"", s9decoy, "", s9none, "", s10decoy, "", s10none},
			expectedNumDeleted: 4,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := test.CountSessions(t, conn)

			err := vcManager.DeleteInvalidSessionsByServerWGConfig(ctx, tc.serverID, tc.oldServerWGC, tc.newServerWGC)
			require.NoError(t, err)

			after := test.CountSessions(t, conn)
			require.Equal(t, before-tc.expectedNumDeleted, after)

			for i, deviceID := range tc.deviceIDs {
				device, err := sqlcQ.GetDeviceByID(ctx, deviceID)
				require.NoError(t, err)
				require.Equal(t, tc.expectedSessions[i], device.SessionID)
			}
		})
	}
}

func TestIntegrationListenForDeviceSessionsUpdates(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user1 := test.InsertSampleUser(t, ctx, pgq, "user1")
	user2 := test.InsertSampleUser(t, ctx, pgq, "user2")
	addressPool := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("testPool"))
	deviceTemplate1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user1.ID, []int{addressPool.ID})
	deviceTemplate2 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user2.ID, []int{addressPool.ID})

	device1 := test.InsertSampleDeviceManual(t, ctx, sqlcQ, deviceTemplate1.ID)
	device2 := test.InsertSampleDeviceManual(t, ctx, sqlcQ, deviceTemplate2.ID)

	session1, err := session.GenerateSessionID()
	require.NoError(t, err)
	session2, err := session.GenerateSessionID()
	require.NoError(t, err)
	session3, err := session.GenerateSessionID()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		time.Sleep(notificationTimeout)
	}()
	errors := make(chan error, 1)

	notifications := q.ListenForDevicesSessionsUpdates(ctx, errors)
	time.Sleep(notificationTimeout)

	cases := []struct {
		name             string
		deviceID         int
		session          string
		expectOldSession string
		expectNewSession string
	}{
		{
			name:             "update device1 with session",
			deviceID:         device1.ID,
			session:          session1,
			expectOldSession: "",
			expectNewSession: session1,
		},
		{
			name:             "update device2 with session",
			deviceID:         device2.ID,
			session:          session2,
			expectOldSession: "",
			expectNewSession: session2,
		},
		{
			name:             "update device1 with new session",
			deviceID:         device1.ID,
			session:          session3,
			expectOldSession: session1,
			expectNewSession: session3,
		},
		{
			name:             "delete session for device1",
			deviceID:         device1.ID,
			session:          "",
			expectOldSession: session3,
			expectNewSession: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err = sqlcQ.UpdateDeviceSessionAndLastTimeConnected(ctx, sqlc.UpdateDeviceSessionAndLastTimeConnectedParams{
				ID:                tc.deviceID,
				SessionID:         tc.session,
				LastTimeConnected: db.NewPgTimestamp(time.Now()),
			})
			require.NoError(t, err)

			select {
			case rawNotification := <-notifications:
				require.NotEmpty(t, rawNotification)

				var nft vcm.DeviceSessionNotification
				err = json.Unmarshal([]byte(rawNotification), &nft)
				require.NoError(t, err)
				require.NotEmpty(t, nft)
				require.Equal(t, tc.expectNewSession, nft.NewSession)
				require.Equal(t, tc.expectOldSession, nft.OldSession)
			case <-time.After(notificationTimeout):
				require.FailNow(t, "Failed to receive from notifications channel within notificationTimeout")
			}

			time.Sleep(notificationTimeout)
			require.Len(t, notifications, 0)

			require.Len(t, errors, 0)
		})
	}
}

func TestIntegrationSessionUpdateNotificationWhenDeleteDevice(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user := test.InsertSampleUser(t, ctx, pgq, "user")
	addressPool := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("testPool"))
	deviceTemplate := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user.ID, []int{addressPool.ID})

	device1, session1 := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate.ID, nil)
	device2 := test.InsertSampleDeviceManual(t, ctx, sqlcQ, deviceTemplate.ID)

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		time.Sleep(notificationTimeout)
	}()
	errors := make(chan error, 1)

	time.Sleep(notificationTimeout)
	notifications := q.ListenForDevicesSessionsUpdates(ctx, errors)
	time.Sleep(notificationTimeout)

	cases := []struct {
		name             string
		deviceID         int
		expectOldSession string
		expectNewSession string
	}{
		{
			name:             "delete device with session",
			deviceID:         device1.ID,
			expectOldSession: session1,
			expectNewSession: "",
		},
		{
			name:             "delete device without session",
			deviceID:         device2.ID,
			expectOldSession: "",
			expectNewSession: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := sqlcQ.DeleteDevice(ctx, tc.deviceID)
			require.NoError(t, err)

			select {
			case rawNotification := <-notifications:
				require.NotEmpty(t, rawNotification)

				var nft vcm.DeviceSessionNotification
				err = json.Unmarshal([]byte(rawNotification), &nft)
				require.NoError(t, err)
				require.NotEmpty(t, nft)
				require.Equal(t, tc.expectNewSession, nft.NewSession)
				require.Equal(t, tc.expectOldSession, nft.OldSession)
			case <-time.After(notificationTimeout):
				require.FailNow(t, "Failed to receive from notifications channel within notificationTimeout")
			}

			time.Sleep(notificationTimeout)
			require.Len(t, notifications, 0)

			require.Len(t, errors, 0)
		})
	}
}

func TestIntegrationIsHealthCheckEnabledForDevice(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)

	user1 := test.InsertSampleUser(t, ctx, pgq, "testUser1")
	addressPool1 := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("pool1"))
	deviceTemplate1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user1.ID, []int{addressPool1.ID})
	device1 := test.InsertSampleDeviceManual(t, ctx, sqlcQ, deviceTemplate1.ID)

	server1a := test.InsertSampleServer(t, ctx, conn, "server1a")
	server1b := test.InsertSampleServer(t, ctx, conn, "server1b")
	test.InsertSampleAdjacency(t, ctx, sqlcQ, server1a.ID, device1.ID)
	test.InsertSampleAdjacency(t, ctx, sqlcQ, server1b.ID, device1.ID)

	user2 := test.InsertSampleUser(t, ctx, pgq, "testUser2")
	addressPool2 := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("pool2"))
	deviceTemplate2 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user2.ID, []int{addressPool2.ID})
	device2 := test.InsertSampleDeviceManual(t, ctx, sqlcQ, deviceTemplate2.ID)

	server2a := test.InsertSampleServer(t, ctx, conn, "server2a", test.WithHealthCheckAddress(nil))
	server2b := test.InsertSampleServer(t, ctx, conn, "server2b", test.WithHealthCheckAddress(nil))
	test.InsertSampleAdjacency(t, ctx, sqlcQ, server2a.ID, device2.ID)
	test.InsertSampleAdjacency(t, ctx, sqlcQ, server2b.ID, device2.ID)

	user3 := test.InsertSampleUser(t, ctx, pgq, "testUser3")
	addressPool3 := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("pool3"))
	deviceTemplate3 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user3.ID, []int{addressPool3.ID})
	device3 := test.InsertSampleDeviceManual(t, ctx, sqlcQ, deviceTemplate3.ID)

	server3a := test.InsertSampleServer(t, ctx, conn, "server3a")
	server3b := test.InsertSampleServer(t, ctx, conn, "server3b", test.WithHealthCheckAddress(nil))
	test.InsertSampleAdjacency(t, ctx, sqlcQ, server3a.ID, device3.ID)
	test.InsertSampleAdjacency(t, ctx, sqlcQ, server3b.ID, device3.ID)

	cases := []struct {
		name     string
		deviceID int
		expected bool
	}{
		{
			name:     "healthcheck service is enabled. All servers with healthcheck address",
			deviceID: device1.ID,
			expected: true,
		},
		{
			name:     "healthcheck service is enabled. One of servers without healthcheck address",
			deviceID: device3.ID,
			expected: true,
		},
		{
			name:     "healthcheck service is disabled",
			deviceID: device2.ID,
			expected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := sqlcQ.IsHealthCheckEnabledForDevice(ctx, tc.deviceID)

			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestIntegrationGetUserAdjacencyTemplates(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1a := test.InsertSampleServer(t, ctx, conn, "server1a")
	server1b := test.InsertSampleServer(t, ctx, conn, "server1b")
	server2 := test.InsertSampleServer(t, ctx, conn, "server2")

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user1 := test.InsertSampleUser(t, ctx, pgq, "testUser1").ToAPI()
	user2 := test.InsertSampleUser(t, ctx, pgq, "testUser2").ToAPI()
	user3 := test.InsertSampleUser(t, ctx, pgq, "testUser3").ToAPI()

	adjTemplates := make(map[int]*sqlc.AdjacencyTemplate)
	adjTemplates[server1a.ID] = test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, server1a.ID, user1.ID)
	adjTemplates[server1b.ID] = test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, server1b.ID, user1.ID)
	adjTemplates[server2.ID] = test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, server2.ID, user2.ID)

	buildExpectedAdjacency := func(adjTemplate *sqlc.AdjacencyTemplate, server *pg.Server) *api.AdjacencyTemplateExtended {
		var allowedIPs []string
		for _, ip := range adjTemplate.ClientSideAllowedIps {
			allowedIPs = append(allowedIPs, ip.String())
		}

		config := &api.WireGuardAdjacencyTemplate{
			UsePresharedKey:      adjTemplate.UsePresharedKey,
			ClientSideAllowedIPs: allowedIPs,
		}

		serverAPI := &api.Server{
			ID:          server.ID,
			Name:        server.Name,
			Endpoint:    server.Endpoint.String(),
			Description: server.Description,
		}
		if server.HealthCheckAddress != nil {
			serverAPI.HealthCheckAddress = server.HealthCheckAddress.String()
		}

		return &api.AdjacencyTemplateExtended{
			ID:             adjTemplate.ID,
			Server:         serverAPI,
			TemplateConfig: config,
		}
	}

	cases := []struct {
		name              string
		userID            int
		expectedPeers     []*api.AdjacencyTemplateExtended
		expectEmptyResult bool
		expectFailure     bool
		expectedError     error
	}{
		{
			name:   "user with multiple adjacency templates",
			userID: user1.ID,
			expectedPeers: []*api.AdjacencyTemplateExtended{
				buildExpectedAdjacency(adjTemplates[server1a.ID], server1a),
				buildExpectedAdjacency(adjTemplates[server1b.ID], server1b),
			},
			expectEmptyResult: false,
		},
		{
			name:   "user with one adjacency template",
			userID: user2.ID,
			expectedPeers: []*api.AdjacencyTemplateExtended{
				buildExpectedAdjacency(adjTemplates[server2.ID], server2),
			},
			expectEmptyResult: false,
		},
		{
			name:              "user with no adjacency templates",
			userID:            user3.ID,
			expectedPeers:     []*api.AdjacencyTemplateExtended{},
			expectEmptyResult: true,
		},
		{
			name:              "user does not exist",
			userID:            999,
			expectedPeers:     []*api.AdjacencyTemplateExtended{},
			expectEmptyResult: true,
			expectFailure:     true,
			expectedError:     vcm.ErrDoesNotExist,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := vcManager.GetUserAdjacencyTemplates(ctx, tc.userID)

			if tc.expectFailure {
				require.Error(t, err)
				if tc.expectedError != nil {
					require.ErrorIs(t, err, tc.expectedError)
				}
				return
			}

			require.NoError(t, err)

			if tc.expectEmptyResult {
				require.Len(t, result, 0)
				return
			}
			require.Len(t, result, len(tc.expectedPeers))

			actualAdjsByServerID := make(map[int]*api.AdjacencyTemplateExtended)
			for _, adj := range result {
				actualAdjsByServerID[adj.Server.ID] = adj
			}

			for _, expectedAdj := range tc.expectedPeers {
				actualAdj, found := actualAdjsByServerID[expectedAdj.Server.ID]
				require.True(t, found)
				require.Equal(t, expectedAdj, actualAdj)
			}
		})
	}
}

func TestIntegrationCreateAdjacencyTemplate(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1").ToAPI()
	server2 := test.InsertSampleServer(t, ctx, conn, "server2").ToAPI()

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user1 := test.InsertSampleUser(t, ctx, pgq, "testUser1").ToAPI()
	user2 := test.InsertSampleUser(t, ctx, pgq, "testUser2").ToAPI()

	test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, server1.ID, user1.ID)

	countAdjacencyTemplates := func() int {
		return test.CountTableRows(t, q.GetDbConnection(ctx), "adjacency_templates")
	}

	cases := []struct {
		name             string
		server           *api.Server
		user             *api.User
		wgAdj            *api.WireGuardAdjacencyTemplate
		expectedAdjTempl *api.AdjacencyTemplateExtended
		expectFailure    bool
		expectedError    error
	}{
		{
			name:             "successfully create adjacency template",
			server:           &server1,
			user:             &user2,
			wgAdj:            test.NewSampleAPIAdjacencyTemplate([]string{"10.0.0.0/8"}),
			expectedAdjTempl: test.NewSampleAdjacencyTemplateExtended(t, []string{"10.0.0.0/8", server1.HealthCheckAddress + "/32"}, true, &server1),
			expectFailure:    false,
		},
		{
			name:             "successfully create adjacency template for different server",
			server:           &server2,
			user:             &user1,
			wgAdj:            test.NewSampleAPIAdjacencyTemplate([]string{"192.168.0.0/16"}),
			expectedAdjTempl: test.NewSampleAdjacencyTemplateExtended(t, []string{"192.168.0.0/16", server2.HealthCheckAddress + "/32"}, true, &server2),
			expectFailure:    false,
		},
		{
			name:          "fail when adjacency template already exists",
			server:        &server1,
			user:          &user1,
			wgAdj:         test.NewSampleAPIAdjacencyTemplate([]string{"172.16.0.0/12"}),
			expectFailure: true,
			expectedError: vcm.ErrAlreadyExists,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			countBefore := countAdjacencyTemplates()

			err := vcManager.CreateAdjacencyTemplate(ctx, tc.server, tc.user.ID, tc.wgAdj)

			if tc.expectFailure {
				require.Error(t, err)
				if tc.expectedError != nil {
					require.ErrorIs(t, err, tc.expectedError)
				}
				countAfter := countAdjacencyTemplates()
				require.Equal(t, countBefore, countAfter)
				return
			}

			require.NoError(t, err)
			countAfter := countAdjacencyTemplates()
			require.Equal(t, countBefore+1, countAfter)

			adjTemplates, err := vcManager.GetUserAdjacencyTemplates(ctx, tc.user.ID)
			require.NoError(t, err)

			var actualAdj *api.AdjacencyTemplateExtended
			for _, tmpl := range adjTemplates {
				if tmpl.Server.ID == tc.server.ID {
					actualAdj = tmpl
					break
				}
			}
			require.NotNil(t, actualAdj)

			require.Equal(t, tc.expectedAdjTempl.Server, actualAdj.Server)
			require.Equal(t, tc.expectedAdjTempl.TemplateConfig, actualAdj.TemplateConfig)
		})
	}
}

func TestIntegrationUpdateAdjacencyTemplate(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1a := test.InsertSampleServer(t, ctx, conn, "server1a").ToAPI()
	server1b := test.InsertSampleServer(t, ctx, conn, "server1b").ToAPI()
	server2 := test.InsertSampleServer(t, ctx, conn, "server2").ToAPI()

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user1 := test.InsertSampleUser(t, ctx, pgq, "testUser1").ToAPI()
	user2 := test.InsertSampleUser(t, ctx, pgq, "testUser2").ToAPI()

	adjTemplate1 := test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, server1a.ID, user1.ID)
	adjTemplate2 := test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, server2.ID, user2.ID)
	_ = test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, server1b.ID, user1.ID) // user1 already has template for server1b

	countAdjacencyTemplates := func() int {
		return test.CountTableRows(t, q.GetDbConnection(ctx), "adjacency_templates")
	}

	cases := []struct {
		name             string
		adjTemplate      *sqlc.AdjacencyTemplate
		server           *api.Server
		wgAdj            *api.WireGuardAdjacencyTemplate
		expectedAdjTempl *api.AdjacencyTemplateExtended
		expectFailure    bool
		expectedError    error
	}{
		{
			name:             "successfully update adjacency template",
			adjTemplate:      adjTemplate1,
			server:           &server1a,
			wgAdj:            test.NewSampleAPIAdjacencyTemplate([]string{"172.16.0.0/12"}),
			expectedAdjTempl: test.NewSampleAdjacencyTemplateExtended(t, []string{"172.16.0.0/12", server1a.HealthCheckAddress + "/32"}, true, &server1a),
			expectFailure:    false,
		},
		{
			name:             "successfully update adjacency template to different server",
			adjTemplate:      adjTemplate2,
			server:           &server1a,
			wgAdj:            test.NewSampleAPIAdjacencyTemplate([]string{"192.168.100.0/24"}),
			expectedAdjTempl: test.NewSampleAdjacencyTemplateExtended(t, []string{"192.168.100.0/24", server1a.HealthCheckAddress + "/32"}, true, &server1a),
			expectFailure:    false,
		},
		{
			name: "fail when adjacency template does not exist",
			adjTemplate: &sqlc.AdjacencyTemplate{
				ID:     99999,
				UserID: user1.ID,
			},
			server:        &server1a,
			wgAdj:         test.NewSampleAPIAdjacencyTemplate([]string{"10.0.0.0/8"}),
			expectFailure: true,
			expectedError: vcm.ErrDoesNotExist,
		},
		{
			name:          "fail when updating would create duplicate adjacency template",
			adjTemplate:   adjTemplate1,
			server:        &server1b,
			wgAdj:         test.NewSampleAPIAdjacencyTemplate([]string{"10.0.0.0/8"}),
			expectFailure: true,
			expectedError: vcm.ErrAlreadyExists,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			countBefore := countAdjacencyTemplates()

			err := vcManager.UpdateAdjacencyTemplate(ctx, tc.adjTemplate.ID, tc.server, tc.adjTemplate.UserID, tc.wgAdj)

			if tc.expectFailure {
				require.Error(t, err)
				if tc.expectedError != nil {
					require.ErrorIs(t, err, tc.expectedError)
				}
				countAfter := countAdjacencyTemplates()
				require.Equal(t, countBefore, countAfter)
				return
			}

			require.NoError(t, err)
			countAfter := countAdjacencyTemplates()
			require.Equal(t, countBefore, countAfter)

			adjTemplates, err := vcManager.GetUserAdjacencyTemplates(ctx, tc.adjTemplate.UserID)
			require.NoError(t, err)

			var actualAdj *api.AdjacencyTemplateExtended
			for _, tmpl := range adjTemplates {
				if tmpl.ID == tc.adjTemplate.ID {
					actualAdj = tmpl
					break
				}
			}
			require.NotNil(t, actualAdj)

			require.Equal(t, tc.adjTemplate.ID, actualAdj.ID)
			require.Equal(t, tc.expectedAdjTempl.Server, actualAdj.Server)
			require.Equal(t, tc.expectedAdjTempl.TemplateConfig, actualAdj.TemplateConfig)
		})
	}
}

func TestIntegrationDeleteAdjacencyTemplate(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1").ToAPI()
	server2 := test.InsertSampleServer(t, ctx, conn, "server2").ToAPI()

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user1 := test.InsertSampleUser(t, ctx, pgq, "testUser1").ToAPI()

	adjTemplate1 := test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, server1.ID, user1.ID)
	_ = test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, server2.ID, user1.ID)

	countAdjacencyTemplates := func() int {
		return test.CountTableRows(t, q.GetDbConnection(ctx), "adjacency_templates")
	}

	cases := []struct {
		name                string
		adjacencyTemplateID int
		expectFailure       bool
		expectedError       error
	}{
		{
			name:                "successfully delete adjacency template",
			adjacencyTemplateID: adjTemplate1.ID,
			expectFailure:       false,
		},
		{
			name:                "fail when adjacency template does not exist",
			adjacencyTemplateID: 99999,
			expectFailure:       true,
			expectedError:       vcm.ErrDoesNotExist,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			countBefore := countAdjacencyTemplates()

			err := vcManager.DeleteAdjacencyTemplate(ctx, tc.adjacencyTemplateID)

			if tc.expectFailure {
				require.Error(t, err)
				if tc.expectedError != nil {
					require.ErrorIs(t, err, tc.expectedError)
				}
				require.Equal(t, countBefore, countAdjacencyTemplates())
				return
			}

			require.NoError(t, err)
			require.Equal(t, countBefore-1, countAdjacencyTemplates())

			exists, err := sqlcQ.AdjacencyTemplateExistsByID(ctx, tc.adjacencyTemplateID)
			require.NoError(t, err)
			require.False(t, exists)
		})
	}
}

func TestIntegrationUpdateServerWithHealthcheckUpdatesAdjacencies(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	server := test.InsertSampleServer(t, ctx, conn, "server-test-hc", test.WithHealthCheckAddress(nil)).ToAPI()

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user1 := test.InsertSampleUser(t, ctx, pgq, "user-hc-test").ToAPI()
	pool := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("hc-pool"))
	dt := test.InsertSampleDeviceTemplateAPI(t, ctx, vcManager, user1.ID, []*api.AddressPool{user.ConvertDBToAPIPool(*pool)})
	device := test.InsertSampleDeviceAutoGeneratedNoExternalID(t, ctx, uManager, dt.ID)

	// create adjacency without healthcheck in allowed IPs
	_, err := sqlcQ.InsertAdjacency(ctx, sqlc.InsertAdjacencyParams{
		ServerID:              server.ID,
		DeviceID:              device.ID,
		ServerSideAllowedIps:  []netip.Prefix{netip.MustParsePrefix("1.1.1.1/32")},
		ClientSideAllowedIps:  []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")},
		PresharedKeyEncrypted: test.Seal(""),
	})
	require.NoError(t, err)

	// create adjacency template without healthcheck
	_, err = sqlcQ.InsertAdjacencyTemplate(ctx, sqlc.InsertAdjacencyTemplateParams{
		ServerID:             server.ID,
		UserID:               user1.ID,
		ClientSideAllowedIps: []netip.Prefix{netip.MustParsePrefix("192.168.0.0/16")},
		UsePresharedKey:      false,
	})
	require.NoError(t, err)

	// create ldap template and ldap template server without healthcheck
	ldapTemplate := test.InsertSampleLdapTemplate(t, ctx, uManager.(*user.PostgresManager), "ldap-template-hc-test", nil)
	_, err = sqlcQ.InsertLdapTemplateServer(ctx, sqlc.InsertLdapTemplateServerParams{
		LdapTemplateID:  ldapTemplate.ID,
		ServerID:        &server.ID,
		AllowedIps:      []netip.Prefix{netip.MustParsePrefix("172.16.0.0/16")},
		UsePresharedKey: false,
	})
	require.NoError(t, err)

	// update server to add healthcheck address
	healthcheckAddr := "172.16.0.1"
	updatedServer, err := vcManager.UpdateServer(ctx, server.ID, &api.ServerUpdate{
		Name:               server.Name,
		Endpoint:           server.Endpoint,
		Description:        server.Description,
		HealthCheckAddress: healthcheckAddr,
	})
	require.NoError(t, err)
	require.Equal(t, healthcheckAddr, updatedServer.HealthCheckAddress)

	//  adjacency was updated with healthcheck address
	adjacencies, err := sqlcQ.GetAdjacenciesByServerID(ctx, server.ID)
	require.NoError(t, err)
	require.Len(t, adjacencies, 1)

	healthcheckAddrParsed, err := netip.ParseAddr(healthcheckAddr)
	require.NoError(t, err)
	require.True(t, vcm.IsHealthcheckAddrInAllowedIPs(adjacencies[0].ClientSideAllowedIps, &healthcheckAddrParsed))

	//  adjacency template was updated with healthcheck address
	adjTemplates, err := sqlcQ.GetAdjacencyTemplatesByServerID(ctx, server.ID)
	require.NoError(t, err)
	require.Len(t, adjTemplates, 1)

	require.True(t, vcm.IsHealthcheckAddrInAllowedIPs(adjTemplates[0].ClientSideAllowedIps, &healthcheckAddrParsed))

	//  ldap template server was updated with healthcheck address
	ldapTemplateServers, err := sqlcQ.GetLdapTemplateServersByServerID(ctx, &server.ID)
	require.NoError(t, err)
	require.Len(t, ldapTemplateServers, 1)

	require.True(t, vcm.IsHealthcheckAddrInAllowedIPs(ldapTemplateServers[0].AllowedIps, &healthcheckAddrParsed))
}

func TestIntegrationUpdateServerRemovingHealthcheckRemovesFromAdjacencies(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	healthcheckAddr := "172.16.0.1"
	server, _, adj, adjTemplate, ldapTemplateServerBefore, hcPrefix := setupServerWithHealthcheckAndAdjacencies(
		t, ctx, "server-test-hc-removal", healthcheckAddr,
	)

	healthcheckAddrParsed, err := netip.ParseAddr(healthcheckAddr)
	require.NoError(t, err)

	var adjTemplates []sqlc.AdjacencyTemplate
	adjTemplates, err = sqlcQ.GetAdjacencyTemplatesByServerID(ctx, server.ID)
	require.NoError(t, err)
	require.Len(t, adjTemplates, 1)
	require.True(t, vcm.IsHealthcheckAddrInAllowedIPs(adjTemplates[0].ClientSideAllowedIps, &healthcheckAddrParsed))

	ldapTemplateServers, err := sqlcQ.GetLdapTemplateServersByServerID(ctx, &server.ID)
	require.NoError(t, err)
	require.Len(t, ldapTemplateServers, 1)
	require.True(t, vcm.IsHealthcheckAddrInAllowedIPs(ldapTemplateServers[0].AllowedIps, &healthcheckAddrParsed))

	// update server to remove healthcheck address
	updatedServer, err := vcManager.UpdateServer(ctx, server.ID, &api.ServerUpdate{
		Name:               server.Name,
		Endpoint:           server.Endpoint,
		Description:        server.Description,
		HealthCheckAddress: "",
	})
	require.NoError(t, err)
	require.Empty(t, updatedServer.HealthCheckAddress)

	// verify healthcheck was removed from adjacency
	adjacencies, err := sqlcQ.GetAdjacenciesByServerID(ctx, server.ID)
	require.NoError(t, err)
	require.Len(t, adjacencies, 1)
	hcPrefixRemoved := true
	for _, prefix := range adjacencies[0].ClientSideAllowedIps {
		if prefix == hcPrefix {
			hcPrefixRemoved = false
			break
		}
	}
	require.True(t, hcPrefixRemoved, "healthcheck /32 prefix should be removed from adjacency")
	require.Equal(t, len(adj.ClientSideAllowedIps)-1, len(adjacencies[0].ClientSideAllowedIps)) // healthcheck was removed

	// verify healthcheck was removed from adjacency template
	adjTemplates, err = sqlcQ.GetAdjacencyTemplatesByServerID(ctx, server.ID)
	require.NoError(t, err)
	require.Len(t, adjTemplates, 1)
	hcPrefixRemoved = true
	for _, prefix := range adjTemplates[0].ClientSideAllowedIps {
		if prefix == hcPrefix {
			hcPrefixRemoved = false
			break
		}
	}
	require.True(t, hcPrefixRemoved, "healthcheck /32 prefix should be removed from adjacency template")
	require.Equal(t, len(adjTemplate.ClientSideAllowedIps)-1, len(adjTemplates[0].ClientSideAllowedIps)) // healthcheck was removed

	// verify healthcheck was removed from ldap template server
	ldapTemplateServers, err = sqlcQ.GetLdapTemplateServersByServerID(ctx, &server.ID)
	require.NoError(t, err)
	require.Len(t, ldapTemplateServers, 1)
	hcPrefixRemoved = true
	for _, prefix := range ldapTemplateServers[0].AllowedIps {
		if prefix == hcPrefix {
			hcPrefixRemoved = false
			break
		}
	}
	require.True(t, hcPrefixRemoved, "healthcheck /32 prefix should be removed from LDAP template server")
	require.Equal(t, len(ldapTemplateServerBefore.AllowedIps)-1, len(ldapTemplateServers[0].AllowedIps)) // healthcheck was removed
}

func TestIntegrationUpdateServerChangingHealthcheckReplacesInAdjacencies(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	oldHealthcheckAddr := "186.178.1.1"
	server, _, adj, adjTemplate, ldapTemplateServerBefore, oldHcPrefix := setupServerWithHealthcheckAndAdjacencies(
		t, ctx, "server-test-hc-change", oldHealthcheckAddr,
	)

	oldHealthcheckAddrParsed, err := netip.ParseAddr(oldHealthcheckAddr)
	require.NoError(t, err)

	var adjTemplates []sqlc.AdjacencyTemplate
	adjTemplates, err = sqlcQ.GetAdjacencyTemplatesByServerID(ctx, server.ID)
	require.NoError(t, err)
	require.Len(t, adjTemplates, 1)
	require.True(t, vcm.IsHealthcheckAddrInAllowedIPs(adjTemplates[0].ClientSideAllowedIps, &oldHealthcheckAddrParsed))

	var ldapTemplateServers []sqlc.GetLdapTemplateServersByServerIDRow
	ldapTemplateServers, err = sqlcQ.GetLdapTemplateServersByServerID(ctx, &server.ID)
	require.NoError(t, err)
	require.Len(t, ldapTemplateServers, 1)
	require.True(t, vcm.IsHealthcheckAddrInAllowedIPs(ldapTemplateServers[0].AllowedIps, &oldHealthcheckAddrParsed))

	// update server to new healthcheck address
	newHealthcheckAddr := "186.178.1.2"
	updatedServer, err := vcManager.UpdateServer(ctx, server.ID, &api.ServerUpdate{
		Name:               server.Name,
		Endpoint:           server.Endpoint,
		Description:        server.Description,
		HealthCheckAddress: newHealthcheckAddr,
	})
	require.NoError(t, err)
	require.Equal(t, newHealthcheckAddr, updatedServer.HealthCheckAddress)

	// parse new healthcheck address
	newHealthcheckAddrParsed, err := netip.ParseAddr(newHealthcheckAddr)
	require.NoError(t, err)
	newHcPrefix := netip.PrefixFrom(newHealthcheckAddrParsed, 32)

	// verify old healthcheck was removed and new one added to adjacency
	adjacencies, err := sqlcQ.GetAdjacenciesByServerID(ctx, server.ID)
	require.NoError(t, err)
	require.Len(t, adjacencies, 1)

	// old healthcheck prefix should not be present
	oldHcPrefixPresent := false
	for _, prefix := range adjacencies[0].ClientSideAllowedIps {
		if prefix == oldHcPrefix {
			oldHcPrefixPresent = true
			break
		}
	}
	require.False(t, oldHcPrefixPresent, "old healthcheck /32 prefix should be removed from adjacency")

	// new healthcheck prefix should be present
	newHcPrefixPresent := false
	for _, prefix := range adjacencies[0].ClientSideAllowedIps {
		if prefix == newHcPrefix {
			newHcPrefixPresent = true
			break
		}
	}
	require.True(t, newHcPrefixPresent, "new healthcheck /32 prefix should be added to adjacency")
	require.Equal(t, len(adj.ClientSideAllowedIps), len(adjacencies[0].ClientSideAllowedIps)) // same count: removed old, added new

	// verify old healthcheck was removed and new one added to adjacency template
	adjTemplates, err = sqlcQ.GetAdjacencyTemplatesByServerID(ctx, server.ID)
	require.NoError(t, err)
	require.Len(t, adjTemplates, 1)

	oldHcPrefixPresent = false
	for _, prefix := range adjTemplates[0].ClientSideAllowedIps {
		if prefix == oldHcPrefix {
			oldHcPrefixPresent = true
			break
		}
	}
	require.False(t, oldHcPrefixPresent, "old healthcheck /32 prefix should be removed from adjacency template")

	newHcPrefixPresent = false
	for _, prefix := range adjTemplates[0].ClientSideAllowedIps {
		if prefix == newHcPrefix {
			newHcPrefixPresent = true
			break
		}
	}
	require.True(t, newHcPrefixPresent, "new healthcheck /32 prefix should be added to adjacency template")
	require.Equal(t, len(adjTemplate.ClientSideAllowedIps), len(adjTemplates[0].ClientSideAllowedIps)) // same count

	// verify old healthcheck was removed and new one added to ldap template server
	ldapTemplateServers, err = sqlcQ.GetLdapTemplateServersByServerID(ctx, &server.ID)
	require.NoError(t, err)
	require.Len(t, ldapTemplateServers, 1)
	t.Logf("LDAP server allowed IPs after update: %v", ldapTemplateServers[0].AllowedIps)
	t.Logf("Old HC prefix: %v, New HC prefix: %v", oldHcPrefix, newHcPrefix)

	oldHcPrefixPresent = false
	for _, prefix := range ldapTemplateServers[0].AllowedIps {
		if prefix == oldHcPrefix {
			oldHcPrefixPresent = true
			break
		}
	}
	require.False(t, oldHcPrefixPresent, "old healthcheck /32 prefix should be removed from LDAP template server")

	newHcPrefixPresent = false
	for _, prefix := range ldapTemplateServers[0].AllowedIps {
		if prefix == newHcPrefix {
			newHcPrefixPresent = true
			break
		}
	}
	require.True(t, newHcPrefixPresent, "new healthcheck /32 prefix should be added to LDAP template server")
	require.Equal(t, len(ldapTemplateServerBefore.AllowedIps), len(ldapTemplateServers[0].AllowedIps)) // same count
}

func TestIntegrationResyncDeviceAdjacencies_DeleteWithoutTemplate(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1").ToAPI()
	server2 := test.InsertSampleServer(t, ctx, conn, "server2").ToAPI()

	pool := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("resync-pool"))

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user1 := test.InsertSampleUser(t, ctx, pgq, "resync-user1").ToAPI()
	dt1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user1.ID, []int{pool.ID})
	device1 := test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt1.ID)

	test.InsertSampleAdjacency(t, ctx, sqlcQ, server1.ID, device1.ID)
	test.InsertSampleAdjacency(t, ctx, sqlcQ, server2.ID, device1.ID)

	// resync
	response, err := vcManager.ResyncDeviceAdjacencies(ctx, user1.ID)
	require.NoError(t, err)
	require.Equal(t, 0, response.AdjacenciesCreated)
	require.Equal(t, 0, response.AdjacenciesUpdated)
	require.Equal(t, 2, response.AdjacenciesDeleted)
	require.Empty(t, response.Errors)

	adjacenciesCount := test.CountTableRows(t, q.GetDbConnection(ctx), "adjacencies")
	require.Equal(t, 0, adjacenciesCount)
}

func TestIntegrationResyncDeviceAdjacencies_CreateFromTemplates(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1").ToAPI()
	server2 := test.InsertSampleServer(t, ctx, conn, "server2").ToAPI()

	pool := test.InsertSampleAddressPool(t, ctx, sqlcQ, &sqlc.InsertAddressPoolParams{
		Name:        "resync-pool",
		StartAddr:   netip.MustParseAddr("1.1.1.10"),
		EndAddr:     netip.MustParseAddr("1.1.1.100"),
		NetMask:     24,
		Description: "test pool matching server network",
	})

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user1 := test.InsertSampleUser(t, ctx, pgq, "resync-user1").ToAPI()
	dt1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user1.ID, []int{pool.ID})
	_ = test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt1.ID)
	_ = test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt1.ID)

	test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, server1.ID, user1.ID)
	test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, server2.ID, user1.ID)

	// resync
	response, err := vcManager.ResyncDeviceAdjacencies(ctx, user1.ID)
	require.NoError(t, err)
	require.Equal(t, 4, response.AdjacenciesCreated) // 2 devices × 2 servers
	require.Equal(t, 0, response.AdjacenciesUpdated)
	require.Equal(t, 0, response.AdjacenciesDeleted)
	require.Empty(t, response.Errors)

	adjacenciesCount := test.CountTableRows(t, q.GetDbConnection(ctx), "adjacencies")
	require.Equal(t, 4, adjacenciesCount)
}

func TestIntegrationResyncDeviceAdjacencies_UpdateFromTemplates(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1").ToAPI()
	server2 := test.InsertSampleServer(t, ctx, conn, "server2").ToAPI()

	pool := test.InsertSampleAddressPool(t, ctx, sqlcQ, &sqlc.InsertAddressPoolParams{
		Name:        "resync-pool",
		StartAddr:   netip.MustParseAddr("1.1.1.10"),
		EndAddr:     netip.MustParseAddr("1.1.1.100"),
		NetMask:     24,
		Description: "test pool matching server network",
	})

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user1 := test.InsertSampleUser(t, ctx, pgq, "resync-user1").ToAPI()
	dt1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user1.ID, []int{pool.ID})
	deviceForNoPsk := test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt1.ID)
	deviceForWithPsk := test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt1.ID)
	deviceForAIPs := test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt1.ID)

	tmplNoUsePsk := test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, server1.ID, user1.ID, test.WithUsePresharedKeyAdjTmpl(false))
	tmplWithUsePsk := test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, server2.ID, user1.ID, test.WithUsePresharedKeyAdjTmpl(true))

	// create initial adjacencies
	_, err := vcManager.ResyncDeviceAdjacencies(ctx, user1.ID)
	require.NoError(t, err)
	adjacenciesCountBefore := test.CountTableRows(t, q.GetDbConnection(ctx), "adjacencies")
	require.Equal(t, 6, adjacenciesCountBefore)
	adjs, err := sqlcQ.GetAdjacenciesByServerID(ctx, server1.ID)
	require.NoError(t, err)
	require.Len(t, adjs, 3)
	adjsServer1 := make(map[int]sqlc.Adjacency)
	for _, a := range adjs {
		adjsServer1[a.DeviceID] = a
	}
	adjs, err = sqlcQ.GetAdjacenciesByServerID(ctx, server2.ID)
	require.NoError(t, err)
	require.Len(t, adjs, 3)
	adjsServer2 := make(map[int]sqlc.Adjacency)
	for _, a := range adjs {
		adjsServer2[a.DeviceID] = a
	}

	adjsS1DForNoPsk := adjsServer1[deviceForNoPsk.ID]
	adjsS1DForWithPsk := adjsServer1[deviceForWithPsk.ID]
	adjsS1DForAIPs := adjsServer1[deviceForAIPs.ID]
	adjsS2DForNoPsk := adjsServer2[deviceForNoPsk.ID]
	adjsS2DForWithPsk := adjsServer2[deviceForWithPsk.ID]
	adjsS2DForAIPs := adjsServer2[deviceForAIPs.ID]

	psk1 := "dLCynDyWqkurpR9+dhCyhH7u+T5xRd/5bmlWNJiSIQc="
	psk2 := "12mJJStgR5tYDPZmS9QV0Av1JzLNllWHkhP/piel/3Y="
	// include healthcheck address (186.178.1.1) in allowed IPs so adjacency can be updated
	newClientIPs := []netip.Prefix{netip.MustParsePrefix("100.200.100.200/32"), netip.MustParsePrefix("186.178.1.0/24")}

	adjsS1DForNoPsk.PresharedKeyEncrypted = test.Seal("")
	adjsS1DForWithPsk.PresharedKeyEncrypted = test.Seal(psk1)
	// include healthcheck address (186.178.1.1) in allowed IPs so adjacency can be updated
	adjsS1DForAIPs.ClientSideAllowedIps = newClientIPs

	adjsS2DForNoPsk.PresharedKeyEncrypted = test.Seal("")
	adjsS2DForWithPsk.PresharedKeyEncrypted = test.Seal(psk2)
	adjsS2DForAIPs.ClientSideAllowedIps = newClientIPs

	for _, a := range []sqlc.Adjacency{adjsS1DForNoPsk, adjsS1DForWithPsk, adjsS1DForAIPs, adjsS2DForNoPsk, adjsS2DForWithPsk, adjsS2DForAIPs} {
		_, err = sqlcQ.UpdateAdjacency(ctx, sqlc.UpdateAdjacencyParams{
			ID:                    a.ID,
			ServerID:              a.ServerID,
			DeviceID:              a.DeviceID,
			PresharedKeyEncrypted: a.PresharedKeyEncrypted,
			ServerSideAllowedIps:  a.ServerSideAllowedIps,
			ClientSideAllowedIps:  a.ClientSideAllowedIps,
		})
		require.NoError(t, err)
	}

	// resync
	response, err := vcManager.ResyncDeviceAdjacencies(ctx, user1.ID)
	require.NoError(t, err)
	require.Equal(t, 0, response.AdjacenciesCreated)
	require.Equal(t, 4, response.AdjacenciesUpdated) // 2 × updated IPs, 2 × updated psk (remaining 2 adjacencies not changed)
	require.Equal(t, 0, response.AdjacenciesDeleted)
	require.Empty(t, response.Errors)

	adjacenciesCountAfter := test.CountTableRows(t, q.GetDbConnection(ctx), "adjacencies")
	require.Equal(t, 6, adjacenciesCountAfter)

	adjsGot, err := sqlcQ.GetAdjacenciesByServerID(ctx, server1.ID)
	require.NoError(t, err)
	require.Len(t, adjsGot, 3)
	adjsGotServer1 := make(map[int]sqlc.Adjacency)
	for _, a := range adjsGot {
		adjsGotServer1[a.DeviceID] = a
	}
	adjsGot, err = sqlcQ.GetAdjacenciesByServerID(ctx, server2.ID)
	require.NoError(t, err)
	require.Len(t, adjsGot, 3)
	adjsGotServer2 := make(map[int]sqlc.Adjacency)
	for _, a := range adjsGot {
		adjsGotServer2[a.DeviceID] = a
	}

	adjsGotS1DForNoPsk := adjsGotServer1[deviceForNoPsk.ID]
	adjsGotS1DForWithPsk := adjsGotServer1[deviceForWithPsk.ID]
	adjsGotS1DForAIPs := adjsGotServer1[deviceForAIPs.ID]
	adjsGotS2DForNoPsk := adjsGotServer2[deviceForNoPsk.ID]
	adjsGotS2DForWithPsk := adjsGotServer2[deviceForWithPsk.ID]
	adjsGotS2DForAIPs := adjsGotServer2[deviceForAIPs.ID]

	require.Equal(t, tmplNoUsePsk.UsePresharedKey, test.OpenString(adjsGotS1DForNoPsk.PresharedKeyEncrypted) != "")
	require.Equal(t, tmplNoUsePsk.UsePresharedKey, test.OpenString(adjsGotS1DForWithPsk.PresharedKeyEncrypted) != "")
	require.Equal(t, tmplNoUsePsk.ClientSideAllowedIps, adjsGotS1DForAIPs.ClientSideAllowedIps)
	require.Equal(t, tmplWithUsePsk.UsePresharedKey, test.OpenString(adjsGotS2DForNoPsk.PresharedKeyEncrypted) != "")
	require.Equal(t, tmplWithUsePsk.UsePresharedKey, test.OpenString(adjsGotS2DForWithPsk.PresharedKeyEncrypted) != "")
	require.Equal(t, tmplWithUsePsk.ClientSideAllowedIps, adjsGotS2DForAIPs.ClientSideAllowedIps)

	require.Equal(t, psk2, test.OpenString(adjsGotS2DForWithPsk.PresharedKeyEncrypted))
	require.NotEqual(t, psk1, test.OpenString(adjsGotS2DForNoPsk.PresharedKeyEncrypted))
	require.NotEqual(t, psk2, test.OpenString(adjsGotS2DForNoPsk.PresharedKeyEncrypted))
	require.Equal(t, test.OpenString(adjsS2DForAIPs.PresharedKeyEncrypted),
		test.OpenString(adjsGotS2DForAIPs.PresharedKeyEncrypted))
}

func TestIntegrationResyncDeviceAdjacencies_MixedOperations(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1").ToAPI()
	server2 := test.InsertSampleServer(t, ctx, conn, "server2").ToAPI()
	server3 := test.InsertSampleServer(t, ctx, conn, "server3").ToAPI()

	pool := test.InsertSampleAddressPool(t, ctx, sqlcQ, &sqlc.InsertAddressPoolParams{
		Name:        "resync-pool",
		StartAddr:   netip.MustParseAddr("1.1.1.10"),
		EndAddr:     netip.MustParseAddr("1.1.1.100"),
		NetMask:     24,
		Description: "test pool matching server network",
	})

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user1 := test.InsertSampleUser(t, ctx, pgq, "resync-user1").ToAPI()
	dt1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user1.ID, []int{pool.ID})
	_ = test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt1.ID)
	_ = test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt1.ID)

	test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, server1.ID, user1.ID)
	test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, server2.ID, user1.ID)

	// create initial adjacencies
	_, err := vcManager.ResyncDeviceAdjacencies(ctx, user1.ID)
	require.NoError(t, err)

	adjTemplates, err := sqlcQ.GetUserAdjacencyTemplates(ctx, user1.ID)
	require.NoError(t, err)
	require.Len(t, adjTemplates, 2)

	err = sqlcQ.DeleteAdjacencyTemplate(ctx, adjTemplates[0].ID)
	require.NoError(t, err)

	test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, server3.ID, user1.ID)

	remainingTemplates, err := sqlcQ.GetUserAdjacencyTemplates(ctx, user1.ID)
	require.NoError(t, err)
	require.Len(t, remainingTemplates, 2)

	// include healthcheck address (186.178.1.1) in allowed IPs so adjacency can be updated
	newClientIPs := []netip.Prefix{netip.MustParsePrefix("186.178.1.0/24"), netip.MustParsePrefix("192.168.100.0/24")}
	for _, tmpl := range remainingTemplates {
		if tmpl.ServerID != server3.ID {
			_, err = sqlcQ.UpdateAdjacencyTemplate(ctx, sqlc.UpdateAdjacencyTemplateParams{
				ID:                   tmpl.ID,
				ServerID:             tmpl.ServerID,
				ClientSideAllowedIps: newClientIPs,
				UsePresharedKey:      false,
			})
			require.NoError(t, err)
			break
		}
	}

	adjacenciesCountBefore := test.CountTableRows(t, q.GetDbConnection(ctx), "adjacencies")
	require.Equal(t, 4, adjacenciesCountBefore) // 2 devices × 2 initial templates

	// resync
	response, err := vcManager.ResyncDeviceAdjacencies(ctx, user1.ID)
	require.NoError(t, err)
	require.Equal(t, 2, response.AdjacenciesCreated) // 2 devices × 1 new server3 template
	require.Equal(t, 2, response.AdjacenciesUpdated) // 2 devices × 1 updated template
	require.Equal(t, 2, response.AdjacenciesDeleted) // 2 devices × 1 deleted template
	require.Empty(t, response.Errors)

	adjacenciesCountAfter := test.CountTableRows(t, q.GetDbConnection(ctx), "adjacencies")
	require.Equal(t, 4, adjacenciesCountAfter)
}

func TestIntegrationResyncDeviceAdjacencies_EdgeCases(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1").ToAPI()
	server2 := test.InsertSampleServer(t, ctx, conn, "server2").ToAPI()

	pool := test.InsertSampleAddressPool(t, ctx, sqlcQ, &sqlc.InsertAddressPoolParams{
		Name:        "resync-pool",
		StartAddr:   netip.MustParseAddr("1.1.1.10"),
		EndAddr:     netip.MustParseAddr("1.1.1.100"),
		NetMask:     24,
		Description: "test pool matching server network",
	})

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)

	// user with matching adjacencies - create adjacencies that will match templates exactly
	userNoChanges := test.InsertSampleUser(t, ctx, pgq, "user-no-changes").ToAPI()
	dt1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, userNoChanges.ID, []int{pool.ID})
	_ = test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt1.ID)
	_ = test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt1.ID)
	test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, server1.ID, userNoChanges.ID)
	test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, server2.ID, userNoChanges.ID)
	// first resync creates adjacencies
	response, err := vcManager.ResyncDeviceAdjacencies(ctx, userNoChanges.ID)
	require.NoError(t, err)
	require.Equal(t, 4, response.AdjacenciesCreated) // 2 devices × 2 templates

	// user with no devices
	userNoDevices := test.InsertSampleUser(t, ctx, pgq, "user-no-devices").ToAPI()
	test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, server1.ID, userNoDevices.ID)

	cases := []struct {
		name            string
		userID          int
		expectedCreated int
		expectedUpdated int
		expectedDeleted int
		expectError     bool
		expectedError   error
	}{
		{
			name:            "no changes when adjacencies match templates",
			userID:          userNoChanges.ID,
			expectedCreated: 0,
			expectedUpdated: 0,
			expectedDeleted: 0,
			expectError:     false,
		},
		{
			name:          "error when user does not exist",
			userID:        99999,
			expectError:   true,
			expectedError: vcm.ErrDoesNotExist,
		},
		{
			name:            "handle user with no devices",
			userID:          userNoDevices.ID,
			expectedCreated: 0,
			expectedUpdated: 0,
			expectedDeleted: 0,
			expectError:     false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response, err := vcManager.ResyncDeviceAdjacencies(ctx, tc.userID)

			if tc.expectError {
				require.Error(t, err)
				require.Nil(t, response)
				if tc.expectedError != nil {
					require.ErrorIs(t, err, tc.expectedError)
				}
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expectedCreated, response.AdjacenciesCreated)
			require.Equal(t, tc.expectedUpdated, response.AdjacenciesUpdated)
			require.Equal(t, tc.expectedDeleted, response.AdjacenciesDeleted)
		})
	}
}

// setupServerWithHealthcheckAndAdjacencies creates a server with healthcheck and associated adjacencies for testing
func setupServerWithHealthcheckAndAdjacencies(
	t *testing.T,
	ctx context.Context,
	serverName string,
	healthcheckAddr string,
) (
	server *api.Server,
	deviceID int,
	adj sqlc.Adjacency,
	adjTemplate sqlc.AdjacencyTemplate,
	ldapTemplateServer sqlc.LdapTemplateServer,
	hcPrefix netip.Prefix,
) {
	t.Helper()

	// parse healthcheck address and create prefix
	healthcheckAddrParsed, err := netip.ParseAddr(healthcheckAddr)
	require.NoError(t, err)
	hcPrefix = netip.PrefixFrom(healthcheckAddrParsed, 32)

	conn := q.GetDbConnection(ctx)

	apiServer := test.InsertSampleServer(t, ctx, conn, serverName, test.WithHealthCheckAddress(&healthcheckAddrParsed)).ToAPI()
	server = &apiServer

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user1 := test.InsertSampleUser(t, ctx, pgq, serverName+"-user").ToAPI()
	pool := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams(serverName+"-pool"))
	dt := test.InsertSampleDeviceTemplateAPI(t, ctx, vcManager, user1.ID, []*api.AddressPool{user.ConvertDBToAPIPool(*pool)})
	device := test.InsertSampleDeviceAutoGeneratedNoExternalID(t, ctx, uManager, dt.ID)
	deviceID = device.ID

	// create adjacency with healthcheck in allowed IPs
	adj, err = sqlcQ.InsertAdjacency(ctx, sqlc.InsertAdjacencyParams{
		ServerID:              server.ID,
		DeviceID:              deviceID,
		ServerSideAllowedIps:  []netip.Prefix{netip.MustParsePrefix("1.1.1.1/32")},
		ClientSideAllowedIps:  []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), hcPrefix},
		PresharedKeyEncrypted: test.Seal(""),
	})
	require.NoError(t, err)

	// create adjacency template with healthcheck
	adjTemplate, err = sqlcQ.InsertAdjacencyTemplate(ctx, sqlc.InsertAdjacencyTemplateParams{
		ServerID:             server.ID,
		UserID:               user1.ID,
		ClientSideAllowedIps: []netip.Prefix{netip.MustParsePrefix("192.168.0.0/16"), hcPrefix},
		UsePresharedKey:      false,
	})
	require.NoError(t, err)

	// create ldap template and ldap template server with healthcheck
	ldapTemplate := test.InsertSampleLdapTemplate(t, ctx, uManager.(*user.PostgresManager), serverName+"-ldap-template", nil)
	ldapTemplateServer, err = sqlcQ.InsertLdapTemplateServer(ctx, sqlc.InsertLdapTemplateServerParams{
		LdapTemplateID:  ldapTemplate.ID,
		ServerID:        &server.ID,
		AllowedIps:      []netip.Prefix{netip.MustParsePrefix("172.16.0.0/16"), hcPrefix},
		UsePresharedKey: false,
	})
	require.NoError(t, err)

	return server, deviceID, adj, adjTemplate, ldapTemplateServer, hcPrefix
}
