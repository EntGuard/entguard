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

package postgres_test

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
	"github.com/entguard/entguard/service/user"
	vcm "github.com/entguard/entguard/service/vcm"
	pg "github.com/entguard/entguard/service/vcm/drivers/postgres"
)

const notificationTimeout = 100 * time.Millisecond

var (
	q         *pg.VPNConfigQuerier
	sqlcQ     *sqlc.Queries
	vcManager vcm.VPNConfigManager
	uManager  user.Manager
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
		if err := test.TruncateAllTables(ctx, q.GetDbConnection(ctx)); err != nil {
			logger.Fatal(err)
		}
		sqlcQ = sqlc.New(conn)
		vcManager, err = vcm.NewVCMPostgresBased(ctx, conn, logger, false, test.TestEncryptionKey)
		if err != nil {
			logger.Fatal("NewVCMPostgresBased: ", err)
		}

		pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
		uManager, err = test.NewSamplePostgresManager(pgq, logger, user.NewLdapService(), vcManager, sqlc.New(conn))
		if err != nil {
			logger.Fatal("NewSamplePostgresManager: ", err)
		}
	}
	m.Run()
}

func TestIntegrationGetServerByID(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1")
	server2 := test.InsertSampleServer(t, ctx, conn, "server2", test.WithHealthCheckAddress(nil))

	cases := []struct {
		name           string
		serverID       int
		expectedServer *pg.Server
		expectFailure  bool
	}{
		{
			name:           "get server by id",
			serverID:       server1.ID,
			expectedServer: server1,
			expectFailure:  false,
		},
		{
			name:           "get server without healthcheck address",
			serverID:       server2.ID,
			expectedServer: server2,
			expectFailure:  false,
		},
		{
			name:          "no server exists with id",
			serverID:      5,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, err := q.GetServerByID(ctx, tc.serverID)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expectedServer, server)
		})
	}
}

func TestIntegrationGetServerByName(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1")
	test.InsertSampleServer(t, ctx, conn, "server2")

	cases := []struct {
		name           string
		serverName     string
		expectedServer *pg.Server
		expectFailure  bool
	}{
		{
			name:           "get server by name",
			serverName:     server1.Name,
			expectedServer: server1,
			expectFailure:  false,
		},
		{
			name:          "no server exists with name",
			serverName:    "unknown",
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, err := q.GetServerByName(ctx, tc.serverName)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expectedServer, server)
		})
	}
}

func TestIntegrationGetAllServers(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1")
	server2 := test.InsertSampleServer(t, ctx, conn, "server2")

	servers, err := q.GetAllServers(ctx)
	require.NoError(t, err)

	expectedServers := []*pg.Server{server1, server2}

	require.Len(t, servers, len(expectedServers))
	require.Equal(t, expectedServers, servers)
}

func TestIntegrationCheckServerIDExists(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1")
	test.InsertSampleServer(t, ctx, conn, "server2")

	cases := []struct {
		name     string
		serverID int
		expected bool
	}{
		{
			name:     "existing server id",
			serverID: server1.ID,
			expected: true,
		},
		{
			name:     "no server with id",
			serverID: 5,
			expected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := q.CheckServerIDExists(ctx, tc.serverID)
			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestIntegrationCheckServerNameExists(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1")
	test.InsertSampleServer(t, ctx, conn, "server2")

	cases := []struct {
		name       string
		serverName string
		expected   bool
	}{
		{
			name:       "existing server name",
			serverName: server1.Name,
			expected:   true,
		},
		{
			name:       "no server with name",
			serverName: "unknown",
			expected:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := q.CheckServerNameExists(ctx, tc.serverName)
			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestIntegrationGetReplicaCount(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server := test.InsertSampleServer(t, ctx, conn, "server")
	err := q.IncreaseReplicaCount(ctx, server.ID)
	require.NoError(t, err)

	cases := []struct {
		name          string
		serverID      int
		expected      int
		expectFailure bool
	}{
		{
			name:          "get replica of existing server",
			serverID:      server.ID,
			expected:      1,
			expectFailure: false,
		},
		{
			name:          "no server exists with id",
			serverID:      5,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			count, err := q.GetReplicaCount(ctx, tc.serverID)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expected, count)
		})
	}
}

func TestIntegrationIncreaseReplicaCount(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server := test.InsertSampleServer(t, ctx, conn, "server")

	cases := []struct {
		name          string
		serverID      int
		expectFailure bool
	}{
		{
			name:          "increase replica of existing server",
			serverID:      server.ID,
			expectFailure: false,
		},
		{
			name:          "no server exists with id",
			serverID:      5,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			countBefore, _ := q.GetReplicaCount(ctx, tc.serverID) // skip error check to execute negative test

			err := q.IncreaseReplicaCount(ctx, tc.serverID)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			countAfter, err := q.GetReplicaCount(ctx, tc.serverID)
			require.NoError(t, err)

			require.Equal(t, countAfter, countBefore+1)
		})
	}
}

func TestIntegrationDecreaseReplicaCount(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server := test.InsertSampleServer(t, ctx, conn, "server")
	err := q.IncreaseReplicaCount(ctx, server.ID)
	require.NoError(t, err)

	cases := []struct {
		name          string
		serverID      int
		expectFailure bool
	}{
		{
			name:          "decrease replica of existing server",
			serverID:      server.ID,
			expectFailure: false,
		},
		{
			name:          "no server exists with id",
			serverID:      5,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			countBefore, _ := q.GetReplicaCount(ctx, tc.serverID) // skip error check to execute negative test

			err = q.DecreaseReplicaCount(ctx, tc.serverID)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			countAfter, err := q.GetReplicaCount(ctx, tc.serverID)
			require.NoError(t, err)

			require.Equal(t, countAfter, countBefore-1)
		})
	}
}

func TestIntegrationListenForWGPeerUpdates(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1")
	server2 := test.InsertSampleServer(t, ctx, conn, "server2")

	pool := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("name"))

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user := test.InsertSampleUser(t, ctx, pgq, "testUser").ToAPI()
	dt := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user.ID, []int{pool.ID})

	device := test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt.ID)

	errors := make(chan error, 1)
	var ntf vcm.PeerNotification

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		time.Sleep(notificationTimeout)
	}()

	notifications := q.ListenForWGPeerUpdates(ctx, errors)
	time.Sleep(notificationTimeout)

	// test case for inserting adjacency

	adj := test.InsertSampleAdjacency(t, ctx, sqlcQ, server1.ID, device.ID)

	ntf = vcm.PeerNotification{}
	select {
	case rawNtf := <-notifications:
		err := json.Unmarshal([]byte(rawNtf), &ntf)
		require.NoError(t, err)
	case <-time.After(notificationTimeout):
		require.FailNow(t, "Failed to receive from notifications channel within notificationTimeout")
	}
	require.Equal(t, vcm.PeerNotification{
		OldServerId:        0,
		NewServerId:        server1.ID,
		AllowedIps:         []string{"1.1.1.1/32"}, // adj.ServerSideAllowedIps converted to strings
		AdjacencyId:        adj.ID,
		OldDevicePublicKey: "",
		NewDevicePublicKey: device.PublicKey,
	}, ntf)

	// test case for updating adjacency

	_, err := sqlcQ.UpdateAdjacency(ctx, sqlc.UpdateAdjacencyParams{
		ID:                    adj.ID,
		ServerID:              server2.ID,
		DeviceID:              device.ID,
		PresharedKeyEncrypted: adj.PresharedKeyEncrypted,
		ServerSideAllowedIps:  adj.ServerSideAllowedIps,
		ClientSideAllowedIps:  adj.ClientSideAllowedIps,
	})
	require.NoError(t, err)

	ntf = vcm.PeerNotification{}
	select {
	case rawNtf := <-notifications:
		err := json.Unmarshal([]byte(rawNtf), &ntf)
		require.NoError(t, err)
	case <-time.After(notificationTimeout):
		require.FailNow(t, "Failed to receive from notifications channel within notificationTimeout")
	}
	require.Equal(t, vcm.PeerNotification{
		OldServerId:        server1.ID,
		NewServerId:        server2.ID,
		AllowedIps:         []string{"1.1.1.1/32"}, // adj.ServerSideAllowedIps converted to strings
		AdjacencyId:        adj.ID,
		OldDevicePublicKey: device.PublicKey,
		NewDevicePublicKey: device.PublicKey,
	}, ntf)

	// test case for deleting adjacency

	err = sqlcQ.DeleteAdjacency(ctx, adj.ID)
	require.NoError(t, err)

	ntf = vcm.PeerNotification{}
	select {
	case rawNtf := <-notifications:
		err := json.Unmarshal([]byte(rawNtf), &ntf)
		require.NoError(t, err)
	case <-time.After(notificationTimeout):
		require.FailNow(t, "Failed to receive from notifications channel within notificationTimeout")
	}
	require.Equal(t, vcm.PeerNotification{
		OldServerId:        server2.ID,
		NewServerId:        0,
		AllowedIps:         []string(nil),
		AdjacencyId:        0,
		OldDevicePublicKey: device.PublicKey,
		NewDevicePublicKey: "",
	}, ntf)

	// test case for leftovers

	time.Sleep(notificationTimeout)
	require.Len(t, notifications, 0)

	require.Len(t, errors, 0)
}

func TestIntegrationListenForWGPeerUpdates_deleteDeviceTemplate(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1")
	_ = test.InsertSampleServer(t, ctx, conn, "serverDecoy")

	pool := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("name"))

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user1 := test.InsertSampleUser(t, ctx, pgq, "testUserWithOneDevice").ToAPI()
	user2 := test.InsertSampleUser(t, ctx, pgq, "testUserWithMultipleDevices").ToAPI()
	user3 := test.InsertSampleUser(t, ctx, pgq, "testUserWithNoDevices").ToAPI()
	userDecoy := test.InsertSampleUser(t, ctx, pgq, "testUserDecoy").ToAPI()
	dt1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user1.ID, []int{pool.ID})
	dt2 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user2.ID, []int{pool.ID})
	_ = test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user3.ID, []int{pool.ID})
	dtDecoy := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, userDecoy.ID, []int{pool.ID})

	device1 := test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt1.ID)
	device2a := test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt2.ID)
	device2b := test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt2.ID)
	deviceDecoy := test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dtDecoy.ID)

	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, server1.ID, device1.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, server1.ID, device2a.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, server1.ID, device2b.ID)
	_ = test.InsertSampleAdjacency(t, ctx, sqlcQ, server1.ID, deviceDecoy.ID)

	expectedNTF1 := vcm.PeerNotification{
		OldServerId:        server1.ID,
		NewServerId:        0,
		AllowedIps:         []string(nil),
		AdjacencyId:        0,
		OldDevicePublicKey: device1.PublicKey,
		NewDevicePublicKey: "",
	}
	expectedNTF2a := vcm.PeerNotification{
		OldServerId:        server1.ID,
		NewServerId:        0,
		AllowedIps:         []string(nil),
		AdjacencyId:        0,
		OldDevicePublicKey: device2a.PublicKey,
		NewDevicePublicKey: "",
	}
	expectedNTF2b := vcm.PeerNotification{
		OldServerId:        server1.ID,
		NewServerId:        0,
		AllowedIps:         []string(nil),
		AdjacencyId:        0,
		OldDevicePublicKey: device2b.PublicKey,
		NewDevicePublicKey: "",
	}

	errors := make(chan error, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		time.Sleep(notificationTimeout)
	}()

	notifications := q.ListenForWGPeerUpdates(ctx, errors)
	time.Sleep(notificationTimeout)

	cases := []struct {
		name    string
		userID  int
		expNtfs []vcm.PeerNotification
	}{
		{
			name:    "one device",
			userID:  1,
			expNtfs: []vcm.PeerNotification{expectedNTF1},
		},
		{
			name:    "multiple devices",
			userID:  2,
			expNtfs: []vcm.PeerNotification{expectedNTF2a, expectedNTF2b},
		},
		{
			name:    "no devices",
			userID:  3,
			expNtfs: []vcm.PeerNotification{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := vcManager.DeleteDeviceTemplateByUserID(ctx, tc.userID)
			require.NoError(t, err)

			gotNtfs := make([]vcm.PeerNotification, len(tc.expNtfs))

			for i := 0; i < len(tc.expNtfs); i++ {
				select {
				case rawNtf := <-notifications:
					err = json.Unmarshal([]byte(rawNtf), &gotNtfs[i])
					require.NoError(t, err)
				case <-time.After(notificationTimeout):
					require.FailNow(t, "Failed to receive from notifications channel within notificationTimeout")
				}
			}
			require.ElementsMatch(t, tc.expNtfs, gotNtfs)

			time.Sleep(notificationTimeout)
			require.Len(t, notifications, 0)

			require.Len(t, errors, 0)
		})
	}
}

func TestIntegrationInsertAdjacency(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server := test.InsertSampleServer(t, ctx, conn, "server").ToAPI()

	pool := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("name"))

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user := test.InsertSampleUser(t, ctx, pgq, "testUser").ToAPI()
	dt := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user.ID, []int{pool.ID})

	device := test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt.ID)

	wgc := &pg.WireGuardAdjacencyConfig{
		PresharedKey:         "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
		ServerSideAllowedIPs: []netip.Prefix{netip.MustParsePrefix("1.1.1.1/32")},
		ClientSideAllowedIPs: []netip.Prefix{netip.MustParsePrefix("1.1.1.1/32")},
	}

	cases := []struct {
		name          string
		serverID      int
		deviceID      int
		wgConfig      *pg.WireGuardAdjacencyConfig
		expectFailure bool
	}{
		{
			name:          "insert adjacency",
			serverID:      server.ID,
			deviceID:      device.ID,
			wgConfig:      wgc,
			expectFailure: false,
		},
		{
			name:          "no user exists with id",
			serverID:      server.ID,
			deviceID:      20,
			wgConfig:      wgc,
			expectFailure: true,
		},
		{
			name:          "no server exists with id",
			serverID:      20,
			deviceID:      device.ID,
			wgConfig:      wgc,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adjacenciesCountBefore := test.CountTableRows(t, q.GetDbConnection(ctx), "adjacencies")

			_, err := sqlcQ.InsertAdjacency(ctx, sqlc.InsertAdjacencyParams{
				ServerID:              tc.serverID,
				DeviceID:              tc.deviceID,
				ServerSideAllowedIps:  tc.wgConfig.ServerSideAllowedIPs,
				ClientSideAllowedIps:  tc.wgConfig.ClientSideAllowedIPs,
				PresharedKeyEncrypted: test.Seal(tc.wgConfig.PresharedKey),
			})

			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			adjacenciesCountAfter := test.CountTableRows(t, q.GetDbConnection(ctx), "adjacencies")

			require.Equal(t, adjacenciesCountAfter, adjacenciesCountBefore+1)

			wgps, err := vcManager.GetWireGuardServerAdjacencies(ctx, tc.serverID)
			require.NoError(t, err)
			require.Len(t, wgps, 1)
			require.NotEmpty(t, wgps[0])

			var ssips, csips []string
			for _, ssip := range tc.wgConfig.ServerSideAllowedIPs {
				ssips = append(ssips, ssip.String())
			}
			for _, csip := range tc.wgConfig.ClientSideAllowedIPs {
				csips = append(csips, csip.String())
			}

			require.Equal(t, tc.deviceID, wgps[0].DeviceID)
			require.Equal(t, tc.wgConfig.PresharedKey, wgps[0].Config.PresharedKey)
			require.Equal(t, ssips, wgps[0].Config.AllowedIPs)
			require.Equal(t, csips, wgps[0].Config.OtherSideAllowedIPs)
		})
	}
}

func TestIntegrationUpdateAdjacency(t *testing.T) {
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

	device1 := test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt1.ID)
	device2 := test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt2.ID)

	adj := test.InsertSampleAdjacency(t, ctx, sqlcQ, server1.ID, device1.ID)

	wgc := &pg.WireGuardAdjacencyConfig{
		PresharedKey:         "updated preshared key",
		ServerSideAllowedIPs: append(adj.ServerSideAllowedIps, netip.MustParsePrefix("111.2.2.2/32")),
		ClientSideAllowedIPs: append(adj.ClientSideAllowedIps, netip.MustParsePrefix("111.2.2.2/32")),
	}

	cases := []struct {
		name          string
		adjacencyID   int
		newServerID   int
		newDeviceID   int
		wgConfig      *pg.WireGuardAdjacencyConfig
		expectFailure bool
	}{
		{
			name:          "update adjacency",
			adjacencyID:   adj.ID,
			newServerID:   server2.ID,
			newDeviceID:   device2.ID,
			wgConfig:      wgc,
			expectFailure: false,
		},
		{
			name:          "no adjacency exists with adjacency id",
			adjacencyID:   99999,
			newServerID:   server2.ID,
			newDeviceID:   device1.ID,
			wgConfig:      wgc,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adjacenciesCountBefore := test.CountTableRows(t, q.GetDbConnection(ctx), "adjacencies")

			_, err := sqlcQ.UpdateAdjacency(ctx, sqlc.UpdateAdjacencyParams{
				ID:                    tc.adjacencyID,
				ServerID:              tc.newServerID,
				DeviceID:              tc.newDeviceID,
				PresharedKeyEncrypted: test.Seal(tc.wgConfig.PresharedKey),
				ServerSideAllowedIps:  tc.wgConfig.ServerSideAllowedIPs,
				ClientSideAllowedIps:  tc.wgConfig.ClientSideAllowedIPs,
			})

			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			adjacenciesCountAfter := test.CountTableRows(t, q.GetDbConnection(ctx), "adjacencies")

			require.Equal(t, adjacenciesCountAfter, adjacenciesCountBefore)

			wgps, err := vcManager.GetWireGuardServerAdjacencies(ctx, tc.newServerID)
			require.NoError(t, err)
			require.Len(t, wgps, 1)
			require.NotEmpty(t, wgps[0])

			var ssips, csips []string
			for _, ssip := range tc.wgConfig.ServerSideAllowedIPs {
				ssips = append(ssips, ssip.String())
			}
			for _, csip := range tc.wgConfig.ClientSideAllowedIPs {
				csips = append(csips, csip.String())
			}

			require.Equal(t, tc.newDeviceID, wgps[0].DeviceID)
			require.Equal(t, tc.wgConfig.PresharedKey, wgps[0].Config.PresharedKey)
			require.Equal(t, ssips, wgps[0].Config.AllowedIPs)
			require.Equal(t, csips, wgps[0].Config.OtherSideAllowedIPs)
		})
	}
}

func TestIntegrationDeleteAdjacency(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server := test.InsertSampleServer(t, ctx, conn, "server").ToAPI()

	pool := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("name"))

	pgq := test.VPNConfigQuerierToPostgresQuerier(ctx, q)
	user := test.InsertSampleUser(t, ctx, pgq, "testUser").ToAPI()
	dt := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user.ID, []int{pool.ID})

	device := test.InsertSampleDeviceAutoGenerated(t, ctx, uManager, dt.ID)

	adj := test.InsertSampleAdjacency(t, ctx, sqlcQ, server.ID, device.ID)

	adjacenciesCountBefore := test.CountTableRows(t, q.GetDbConnection(ctx), "adjacencies")

	err := sqlcQ.DeleteAdjacency(ctx, adj.ID)
	require.NoError(t, err)

	adjacenciesCountAfter := test.CountTableRows(t, q.GetDbConnection(ctx), "adjacencies")

	require.Equal(t, adjacenciesCountAfter, adjacenciesCountBefore-1)
}

func TestIntegrationInsertServer(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	cases := []struct {
		name         string
		serverSample *pg.Server
	}{
		{
			name:         "first server",
			serverSample: test.NewSampleServer(t, "server1"),
		},
		{
			name:         "second server",
			serverSample: test.NewSampleServer(t, "server2"),
		},
		{
			name:         "server with empty healthcheck address",
			serverSample: test.NewSampleServerWithoutHC(t, "server3"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeCount := test.CountTableRows(t, conn, "servers")

			wgConfig := test.NewSampleWGServerConfig()
			id, err := vcm.InsertServerWithWireGuardConfig(ctx, conn, test.TestEncryptionKey, tc.serverSample, wgConfig)
			require.NoError(t, err)
			tc.serverSample.ID = id

			afterCount := test.CountTableRows(t, conn, "servers")

			require.Equal(t, afterCount, beforeCount+1)

			retrieved, err := q.GetServerByName(ctx, tc.serverSample.Name)
			require.NoError(t, err)
			require.Equal(t, tc.serverSample, retrieved)
		})
	}
}

func TestIntegrationUpdateServer(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1")
	_ = test.InsertSampleServer(t, ctx, conn, "server2")

	healthAddr := netip.MustParseAddr("1.1.1.1")
	serverUpdate11 := pg.Server{
		ID:                 server1.ID,
		Name:               "serverUpdated",
		Endpoint:           netip.MustParseAddr("172.18.0.2"),
		HealthCheckAddress: &healthAddr,
		Description:        "test server updated",
	}

	expectedServer11 := serverUpdate11

	serverUpdate12 := serverUpdate11
	serverUpdate12.HealthCheckAddress = nil

	serverUpdate5 := serverUpdate11
	serverUpdate5.ID = 5

	cases := []struct {
		name           string
		server         *pg.Server
		expectedServer *pg.Server
		expectFailure  bool
	}{
		{
			name:           "update existing server",
			server:         &serverUpdate11,
			expectedServer: &expectedServer11,
			expectFailure:  false,
		},
		{
			name:           "update existing server with empty healthcheck address",
			server:         &serverUpdate12,
			expectedServer: &serverUpdate12,
			expectFailure:  false,
		},
		{
			name:          "update non-existing server",
			server:        &serverUpdate5,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := q.UpdateServer(ctx, tc.server)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			server, err := q.GetServerByID(ctx, tc.server.ID)
			require.NoError(t, err)
			require.Equal(t, tc.expectedServer, server)
		})
	}
}

func TestIntegrationDeleteServer(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1")
	_ = test.InsertSampleServer(t, ctx, conn, "server2")

	cases := []struct {
		name          string
		serverID      int
		expectFailure bool
	}{
		{
			name:          "existing server",
			serverID:      server1.ID,
			expectFailure: false,
		},
		{
			name:          "non-existing server",
			serverID:      5,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeCount := test.CountTableRows(t, conn, "servers")
			err := q.DeleteServer(ctx, tc.serverID)
			afterCount := test.CountTableRows(t, conn, "servers")

			if tc.expectFailure {
				require.NoError(t, err)
				require.Equal(t, afterCount, beforeCount)
			} else {
				require.NoError(t, err)
				require.Equal(t, afterCount, beforeCount-1)
			}

			servers, err := q.GetAllServers(ctx)
			require.NoError(t, err)
			var ids []int
			for _, s := range servers {
				ids = append(ids, s.ID)
			}
			require.NotContains(t, ids, tc.serverID)
		})
	}
}

const (
	sampleTag1  = "vZsZEIEjVhpyFxWB"
	sampleTag2a = "gkiSTpUjtdOICpdJ"
	sampleTag2b = "YBXwwTkdwfoIGisF"
	sampleTag5  = "AMufGZatubzBUUGM"
)

func TestIntegrationGetServerTags(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1")
	server2 := test.InsertSampleServer(t, ctx, conn, "server2")
	server3 := test.InsertSampleServer(t, ctx, conn, "server3")
	err := q.InsertUsedTag(ctx, sampleTag1, server1.ID)
	require.NoError(t, err)
	err = q.InsertUsedTag(ctx, sampleTag2a, server2.ID)
	require.NoError(t, err)
	err = q.InsertUsedTag(ctx, sampleTag2b, server2.ID)
	require.NoError(t, err)

	cases := []struct {
		name         string
		serverID     int
		expectedTags []string
	}{
		{
			name:         "one tag",
			serverID:     server1.ID,
			expectedTags: []string{sampleTag1},
		},
		{
			name:         "multiple tags",
			serverID:     server2.ID,
			expectedTags: []string{sampleTag2a, sampleTag2b},
		},
		{
			name:         "no tags",
			serverID:     server3.ID,
			expectedTags: []string(nil),
		},
		{
			name:         "non-existing server",
			serverID:     5,
			expectedTags: []string(nil),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tags, err := q.GetServerTags(ctx, tc.serverID)
			require.NoError(t, err)
			require.Len(t, tags, len(tc.expectedTags))
			require.Equal(t, tc.expectedTags, tags)
		})
	}
}

func TestIntegrationGetServerTagsByName(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1")
	server2 := test.InsertSampleServer(t, ctx, conn, "server2")
	_ = test.InsertSampleServer(t, ctx, conn, "server3")
	err := q.InsertUsedTag(ctx, sampleTag1, server1.ID)
	require.NoError(t, err)
	err = q.InsertUsedTag(ctx, sampleTag2a, server2.ID)
	require.NoError(t, err)
	// GetServerTagsByName returns the tags in reverse creation time order, so sleep
	// to make sure the timestamps are different
	time.Sleep(time.Millisecond)
	err = q.InsertUsedTag(ctx, sampleTag2b, server2.ID)
	require.NoError(t, err)

	cases := []struct {
		name         string
		serverName   string
		expectedTags []string
	}{
		{
			name:         "one tag",
			serverName:   "server1",
			expectedTags: []string{sampleTag1},
		},
		{
			name:         "multiple tags",
			serverName:   "server2",
			expectedTags: []string{sampleTag2b, sampleTag2a},
		},
		{
			name:         "no tags",
			serverName:   "server3",
			expectedTags: []string(nil),
		},
		{
			name:         "non-existing server",
			serverName:   "serverXXX",
			expectedTags: []string(nil),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tags, err := q.GetServerTagsByName(ctx, tc.serverName)
			require.NoError(t, err)
			require.Len(t, tags, len(tc.expectedTags))
			require.Equal(t, tc.expectedTags, tags)
		})
	}
}

func TestIntegrationInsertUsedTag(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1")
	server2 := test.InsertSampleServer(t, ctx, conn, "server2")

	cases := []struct {
		name          string
		serverID      int
		tags          []string
		expectFailure bool
	}{
		{
			name:          "one tag",
			serverID:      server1.ID,
			tags:          []string{sampleTag1},
			expectFailure: false,
		},
		{
			name:          "multiple tags",
			serverID:      server2.ID,
			tags:          []string{sampleTag2a, sampleTag2b},
			expectFailure: false,
		},
		{
			name:          "tag for non-existing server",
			serverID:      5,
			tags:          []string{sampleTag5},
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			countBefore := test.CountTableRows(t, conn, "tags")

			for _, tag := range tc.tags {
				err := q.InsertUsedTag(ctx, tag, tc.serverID)
				if tc.expectFailure {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			}

			countAfter := test.CountTableRows(t, conn, "tags")

			if tc.expectFailure {
				require.Equal(t, countAfter, countBefore)
				return
			}
			require.Equal(t, countAfter, countBefore+len(tc.tags))

			retrievedTags, err := q.GetServerTags(ctx, tc.serverID)
			require.NoError(t, err)
			require.Len(t, retrievedTags, len(tc.tags))
			require.Equal(t, tc.tags, retrievedTags)
		})
	}
}

func TestIntegrationDeleteUsedTags(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	server1 := test.InsertSampleServer(t, ctx, conn, "server1")
	server2 := test.InsertSampleServer(t, ctx, conn, "server2")
	err := q.InsertUsedTag(ctx, sampleTag1, server1.ID)
	require.NoError(t, err)
	err = q.InsertUsedTag(ctx, sampleTag2a, server2.ID)
	require.NoError(t, err)
	err = q.InsertUsedTag(ctx, sampleTag2b, server2.ID)
	require.NoError(t, err)

	// make sure the table is non-emtpy, to avoid false passing test
	count := test.CountTableRows(t, conn, "tags")
	require.Equal(t, 3, count)

	err = q.DeleteUsedTags(ctx)
	require.NoError(t, err)

	count = test.CountTableRows(t, conn, "tags")
	require.Equal(t, 0, count)

	// deleting from empty table should not cause error nor modify the table
	err = q.DeleteUsedTags(ctx)
	require.NoError(t, err)

	count = test.CountTableRows(t, conn, "tags")
	require.Equal(t, 0, count)
}
