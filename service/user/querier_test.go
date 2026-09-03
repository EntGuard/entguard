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
	"net/netip"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	sqlc "github.com/entguard/entguard/db"
	"github.com/entguard/entguard/pkg/db"
	"github.com/entguard/entguard/pkg/mfa"
	"github.com/entguard/entguard/pkg/test"
	"github.com/entguard/entguard/service/user"
)

func TestIntegrationGetUserByName(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	timestamp0 := time.Now()
	user1 := test.InsertSampleUser(t, ctx, q, "user1")
	timestamp1 := time.Now()
	_ = test.InsertSampleUser(t, ctx, q, "user2")

	cases := []struct {
		name            string
		username        string
		expectedUser    *user.UserModel
		timestampBefore time.Time
		timestampAfter  time.Time
		expectFailure   bool
	}{
		{
			name:            "existing user",
			username:        "user1",
			expectedUser:    user1,
			timestampBefore: timestamp0,
			timestampAfter:  timestamp1,
			expectFailure:   false,
		},
		{
			name:          "non-existing user",
			username:      "userXXX",
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			retrievedUser, err := q.GetUserByName(ctx, tc.username)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			test.RequireTimeBetween(t, retrievedUser.UpdatedAt, tc.timestampBefore, tc.timestampAfter)
			tc.expectedUser.UpdatedAt = retrievedUser.UpdatedAt
			require.Equal(t, tc.expectedUser, retrievedUser)
		})
	}
}

func TestIntegrationGetUserByID(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	timestamp0 := time.Now()
	user1 := test.InsertSampleUser(t, ctx, q, "user1")
	timestamp1 := time.Now()
	_ = test.InsertSampleUser(t, ctx, q, "user2")

	cases := []struct {
		name            string
		userID          int
		expectedUser    *user.UserModel
		timestampBefore time.Time
		timestampAfter  time.Time
		expectFailure   bool
	}{
		{
			name:            "existing user",
			userID:          user1.ID,
			expectedUser:    user1,
			timestampBefore: timestamp0,
			timestampAfter:  timestamp1,
			expectFailure:   false,
		},
		{
			name:          "non-existing user",
			userID:        5,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			retrievedUser, err := q.GetUserByID(ctx, tc.userID)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			test.RequireTimeBetween(t, retrievedUser.UpdatedAt, tc.timestampBefore, tc.timestampAfter)
			tc.expectedUser.UpdatedAt = retrievedUser.UpdatedAt
			require.Equal(t, tc.expectedUser, retrievedUser)
		})
	}
}

func TestIntegrationInsertUser(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	cases := []struct {
		name     string
		userName string
	}{
		{"first user", "user1"},
		{"second user", "user2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeCount := test.CountTableRows(t, conn, "users")
			beforeTimestamp := time.Now()

			insertedUser := test.NewSampleUser(t, tc.userName)
			err := q.InsertUser(ctx, insertedUser)
			require.NoError(t, err)

			afterTimestamp := time.Now()
			afterCount := test.CountTableRows(t, conn, "users")

			retrievedUser, err := q.GetUserByName(ctx, tc.userName)
			require.NoError(t, err)

			require.Equal(t, afterCount, beforeCount+1)
			test.RequireTimeBetween(t, retrievedUser.UpdatedAt, beforeTimestamp, afterTimestamp)
			insertedUser.UpdatedAt = retrievedUser.UpdatedAt
			require.Equal(t, insertedUser, retrievedUser)
		})
	}
}

func TestIntegrationUpdateUser(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	user1 := test.InsertSampleUser(t, ctx, q, "user1")
	_ = test.InsertSampleUser(t, ctx, q, "user2")

	user5, err := user.NewUserModelWithID(5)
	require.NoError(t, err)

	cases := []struct {
		name          string
		user          *user.UserModel
		newUserName   string
		expectFailure bool
	}{
		{"existing user", user1, "user1Updated", false},
		{"non-existing user", user5, "user5Updated", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeTimestamp := time.Now()
			updatedUser := test.NewSampleUserUpdate(t, tc.user, tc.newUserName)
			err = q.UpdateUser(ctx, updatedUser)
			afterTimestamp := time.Now()

			if tc.expectFailure {
				require.EqualError(t, err, user.ErrUserNotFound.Error())
				return
			}
			require.NoError(t, err)

			retrievedUser, err := q.GetUserByID(ctx, tc.user.ID)
			require.NoError(t, err)

			test.RequireTimeBetween(t, retrievedUser.UpdatedAt, beforeTimestamp, afterTimestamp)
			updatedUser.UpdatedAt = retrievedUser.UpdatedAt
			require.Equal(t, updatedUser, retrievedUser)
		})
	}
}

func TestIntegrationUpdateUserPassword(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	user1 := test.InsertSampleUser(t, ctx, q, "user1")
	_ = test.InsertSampleUser(t, ctx, q, "user2")

	cases := []struct {
		name          string
		userID        int
		password      string
		expectFailure bool
	}{
		{
			name:          "existing user",
			userID:        user1.ID,
			password:      "abc123",
			expectFailure: false,
		},
		{
			name:          "non-existing user",
			userID:        5,
			password:      "abc123",
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := user.NewUserModelWithID(tc.userID)
			require.NoError(t, err)
			err = u.SetPasswordWithHashingCost(tc.password, bcrypt.MinCost)
			require.NoError(t, err)

			beforeTimestamp := time.Now()
			err = q.UpdateUserPassword(ctx, u.ID, u.PasswordHash)
			afterTimestamp := time.Now()

			if tc.expectFailure {
				require.EqualError(t, err, user.ErrUserNotFound.Error())
				return
			}
			require.NoError(t, err)

			retrievedUser, err := q.GetUserByID(ctx, tc.userID)
			require.NoError(t, err)

			test.RequireTimeBetween(t, retrievedUser.UpdatedAt, beforeTimestamp, afterTimestamp)
			require.Equal(t, u.PasswordHash, retrievedUser.PasswordHash)
		})
	}
}

func TestIntegrationCheckUsernameExists(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	_ = test.InsertSampleUser(t, ctx, q, "user1")
	_ = test.InsertSampleUser(t, ctx, q, "user2")

	cases := []struct {
		name           string
		userName       string
		expectedResult bool
	}{
		{"first user", "user1", true},
		{"non-existing user", "userXXX", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exists, err := q.CheckUsernameExists(ctx, tc.userName)
			require.NoError(t, err)
			require.Equal(t, tc.expectedResult, exists)
		})
	}
}

func TestIntegrationCheckUserIDExists(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	user1 := test.InsertSampleUser(t, ctx, q, "user1")
	_ = test.InsertSampleUser(t, ctx, q, "user2")

	cases := []struct {
		name           string
		userID         int
		expectedResult bool
	}{
		{"first user", user1.ID, true},
		{"non-existing user", 5, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exists, err := q.CheckUserIDExists(ctx, tc.userID)
			require.NoError(t, err)
			require.Equal(t, tc.expectedResult, exists)
		})
	}
}

func TestIntegrationCountUsers(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	n, err := q.CountUsers(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, n)

	_ = test.InsertSampleUser(t, ctx, q, "user1")
	n, err = q.CountUsers(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	_ = test.InsertSampleUser(t, ctx, q, "user2")
	n, err = q.CountUsers(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, n)
}

func TestIntegrationVerifyLdapConfigPriority(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	_ = insertSampleLdapConfig(t, ctx, q, 1, 0)
	_ = insertSampleLdapConfig(t, ctx, q, 2, 0)

	cases := []struct {
		name          string
		priority      int
		expectFailure bool
	}{
		{
			name:          "non-existing priority",
			priority:      5,
			expectFailure: false,
		},
		{
			name:          "existing priority",
			priority:      1,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := &user.LdapConfig{Priority: tc.priority}
			err := q.VerifyLdapConfigPriority(ctx, config.Priority)

			if tc.expectFailure {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestIntegrationDeleteLdapConfigQuerier(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	temp1 := test.InsertSampleLdapTemplate(t, ctx, manager, "temp1", nil)
	config1 := insertSampleLdapConfig(t, ctx, q, 1, temp1.ID)
	config2 := insertSampleLdapConfig(t, ctx, q, 2, 0)

	cases := []struct {
		name            string
		configID        string
		expectedDeleted *user.LdapConfig
		expectFailure   bool
	}{
		{
			name:            "LDAP config with LDAP template",
			configID:        config1.ID,
			expectedDeleted: config1,
			expectFailure:   false,
		},
		{
			name:            "LDAP config without LDAP template",
			configID:        config2.ID,
			expectedDeleted: config2,
			expectFailure:   false,
		},
		{
			name:          "non-existing LDAP config",
			configID:      "00112233-4455-6677-8899-aabbccddeeff",
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			countBefore := test.CountTableRows(t, conn, "ldaps")
			configsBeforeDB, err := sqlcq.GetAllLdapConfigs(ctx)
			require.NoError(t, err)

			err = q.DeleteLdapConfig(ctx, tc.configID)

			countAfter := test.CountTableRows(t, conn, "ldaps")
			configsAfterDB, e := sqlcq.GetAllLdapConfigs(ctx)
			require.NoError(t, e)

			configsBefore := make([]*user.LdapConfig, 0, len(configsBeforeDB))
			for _, item := range configsBeforeDB {
				configsBefore = append(configsBefore, user.DBAllRowsToLdapConfig(item))
			}
			configsAfter := make([]*user.LdapConfig, 0, len(configsAfterDB))
			for _, item := range configsAfterDB {
				configsAfter = append(configsAfter, user.DBAllRowsToLdapConfig(item))
			}

			if tc.expectFailure {
				require.Error(t, err)
				require.Equal(t, countBefore, countAfter)
			} else {
				require.NoError(t, err)
				require.Equal(t, countBefore-1, countAfter)
				require.Contains(t, configsBefore, tc.expectedDeleted)
			}
			require.NotContains(t, configsAfter, tc.expectedDeleted)
		})
	}
}

func TestIntegrationGetLdapTemplateServersDB(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	server1 := test.InsertSampleServer(t, ctx, conn, "server1")
	server2 := test.InsertSampleServer(t, ctx, conn, "server2")
	server3 := test.InsertSampleServer(t, ctx, conn, "server3")
	tmpl1 := test.InsertSampleLdapTemplate(t, ctx, manager, "template1", nil)
	tmpl2 := test.InsertSampleLdapTemplate(t, ctx, manager, "template2", nil)
	ldapServerData := []user.LdapTemplateServer{
		{
			ServerName:           server1.Name,
			ServerID:             server1.ID,
			LdapTemplateID:       tmpl1.ID,
			ClientSideAllowedIPs: []string{"1.1.0.0/16", "168.10.10.10/32"},
			UsePresharedKey:      true,
		},
		{
			ServerName:           server2.Name,
			ServerID:             server2.ID,
			LdapTemplateID:       tmpl2.ID,
			ClientSideAllowedIPs: []string{"128.0.0.0/7"},
			UsePresharedKey:      true,
		},
		{
			ServerName:           server3.Name,
			ServerID:             server3.ID,
			LdapTemplateID:       tmpl1.ID,
			ClientSideAllowedIPs: []string{"223.222.221.220/30"},
			UsePresharedKey:      false,
		},
	}
	want := make(map[int][]user.LdapTemplateServer)
	for _, lts := range ldapServerData {
		want[lts.LdapTemplateID] = append(want[lts.LdapTemplateID], lts)
		err := q.InsertLdapTemplateServer(ctx, &lts)
		require.NoError(t, err)
	}

	cases := []struct {
		name          string
		templateID    int
		expectFailure bool
	}{
		{
			name:          "get LDAP template for server by template ID 1",
			templateID:    tmpl1.ID,
			expectFailure: false,
		},
		{
			name:          "get LDAP template for server by template ID 2",
			templateID:    tmpl2.ID,
			expectFailure: false,
		},
		{
			name:          "no LDAP template for server exists with ID",
			templateID:    99,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := q.GetLdapTemplateServers(ctx, tc.templateID)
			if tc.expectFailure {
				require.Empty(t, got)
				return
			}
			require.NoError(t, err)
			require.ElementsMatch(t, got, want[tc.templateID])
		})
	}
}

func TestIntegrationInsertLdapTemplateServer(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)
	server := test.InsertSampleServer(t, ctx, conn, "server")
	test.InsertSampleLdapTemplate(t, ctx, manager, "template", nil)

	cases := []struct {
		name          string
		input         *user.LdapTemplateServer
		expectFailure bool
	}{
		{
			name: "insert valid LDAP template server",
			input: &user.LdapTemplateServer{
				ServerName:           server.Name,
				ServerID:             server.ID,
				OldServerID:          0,
				LdapTemplateID:       1,
				ClientSideAllowedIPs: []string{"1.1.1.0/24", "172.1.1.1/32"},
				UsePresharedKey:      true,
			},
			expectFailure: false,
		},
		{
			name:          "insert invalid (empty) LDAP template server",
			input:         &user.LdapTemplateServer{},
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeCount := test.CountTableRows(t, conn, "ldap_template_servers")
			err := q.InsertLdapTemplateServer(ctx, tc.input)
			afterCount := test.CountTableRows(t, conn, "ldap_template_servers")
			if tc.expectFailure {
				require.Error(t, err)
				require.Equal(t, beforeCount, afterCount)
				return
			}
			require.NoError(t, err)
			got, err := q.GetLdapTemplateServers(ctx, tc.input.LdapTemplateID)
			require.NoError(t, err)
			require.Equal(t, beforeCount+1, afterCount)
			require.Equal(t, len(got), 1)
			require.Equal(t, tc.input, &got[0])
		})
	}
}

func TestIntegrationUpdateLdapTemplateServerDB(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)
	server1 := test.InsertSampleServer(t, ctx, conn, "server1")
	server2 := test.InsertSampleServer(t, ctx, conn, "server2")
	test.InsertSampleLdapTemplate(t, ctx, manager, "template", nil)
	ldapTmplServer := &user.LdapTemplateServer{
		ServerName:           server1.Name,
		ServerID:             server1.ID,
		OldServerID:          0,
		LdapTemplateID:       1,
		ClientSideAllowedIPs: []string{"1.1.1.0/24", "172.1.1.1/32"},
		UsePresharedKey:      true,
	}
	err := q.InsertLdapTemplateServer(ctx, ldapTmplServer)
	require.NoError(t, err)

	cases := []struct {
		name            string
		updateInput     *user.LdapTemplateServer
		wantAfterUpdate *user.LdapTemplateServer
		expectFailure   bool
	}{
		{
			name: "update valid LDAP template server",
			updateInput: &user.LdapTemplateServer{
				ServerID:             server2.ID,
				OldServerID:          server1.ID,
				LdapTemplateID:       ldapTmplServer.LdapTemplateID,
				ClientSideAllowedIPs: []string{"1.1.1.1", "172.1.0.0/16"},
				UsePresharedKey:      true,
			},
			wantAfterUpdate: &user.LdapTemplateServer{
				ServerName:           server2.Name,
				ServerID:             server2.ID,
				LdapTemplateID:       ldapTmplServer.LdapTemplateID,
				ClientSideAllowedIPs: []string{"1.1.1.1/32", "172.1.0.0/16"},
				UsePresharedKey:      true,
			},
			expectFailure: false,
		},
		{
			name: "update nonexisting LDAP template server",
			updateInput: &user.LdapTemplateServer{
				ServerID:             server2.ID,
				OldServerID:          server1.ID,
				LdapTemplateID:       99,
				ClientSideAllowedIPs: []string{"1.1.1.1", "172.1.0.0/16"},
				UsePresharedKey:      true,
			},
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeCount := test.CountTableRows(t, conn, "ldap_template_servers")
			err = q.UpdateLdapTemplateServer(ctx, tc.updateInput)
			afterCount := test.CountTableRows(t, conn, "ldap_template_servers")
			if tc.expectFailure {
				require.Error(t, err)
				require.Equal(t, beforeCount, afterCount)
				return
			}
			require.NoError(t, err)
			got, err := q.GetLdapTemplateServers(ctx, tc.updateInput.LdapTemplateID)
			require.NoError(t, err)
			require.Equal(t, beforeCount, afterCount)
			require.Equal(t, tc.wantAfterUpdate, &got[0])
		})
	}
}

func TestIntegrationDeleteLdapTemplateServer(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)
	server := test.InsertSampleServer(t, ctx, conn, "server1")
	tmpl := test.InsertSampleLdapTemplate(t, ctx, manager, "template", nil)
	ldapTmplServer := &user.LdapTemplateServer{
		ServerName:           server.Name,
		ServerID:             server.ID,
		OldServerID:          0,
		LdapTemplateID:       tmpl.ID,
		ClientSideAllowedIPs: []string{"1.1.1.0/24", "172.1.1.1/32"},
		UsePresharedKey:      true,
	}
	err := q.InsertLdapTemplateServer(ctx, ldapTmplServer)
	require.NoError(t, err)

	cases := []struct {
		name          string
		templateID    int
		serverID      int
		expectFailure bool
	}{
		{
			name:          "delete LDAP template server",
			templateID:    tmpl.ID,
			serverID:      server.ID,
			expectFailure: false,
		},
		{
			name:          "delete nonexisting LDAP template server",
			templateID:    0,
			serverID:      server.ID,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeCount := test.CountTableRows(t, conn, "ldap_template_servers")
			err = q.DeleteLdapTemplateServer(ctx, tc.templateID, tc.serverID)
			afterCount := test.CountTableRows(t, conn, "ldap_template_servers")
			if tc.expectFailure {
				require.Error(t, err)
				require.Equal(t, beforeCount, afterCount)
				return
			}
			require.NoError(t, err)
			got, err := q.GetLdapTemplateServers(ctx, tc.templateID)
			require.NoError(t, err)
			require.Equal(t, beforeCount-1, afterCount)
			require.Equal(t, len(got), 0)
		})
	}
}

func TestIntegrationInsertLDAPUserWithRelation(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)
	user1 := test.InsertSampleUser(t, ctx, q, "testUser")
	user2 := test.InsertSampleUser(t, ctx, q, "testUser")
	ldapConfig := insertSampleLdapConfig(t, ctx, q, 1, 0)

	cases := []struct {
		name          string
		user          *user.UserModel
		relation      *user.LdapRelation
		expectFailure bool
	}{
		{
			name: "insert valid LDAP user with valid relation",
			user: user1,
			relation: &user.LdapRelation{
				UserID:  user1.ToAPI().ID,
				LdapID:  ldapConfig.ID,
				UserUID: "1000",
				UserDN:  "testuserdn",
			},
			expectFailure: false,
		},
		{
			name:          "insert valid LDAP user with empty relation",
			user:          user2,
			relation:      &user.LdapRelation{},
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			userCountBefore := test.CountTableRows(t, conn, "users")
			relationCountBefore := test.CountTableRows(t, conn, "users_to_ldaps")
			err := q.InsertLDAPUserWithRelation(ctx, tc.user, tc.relation)
			userCountAfter := test.CountTableRows(t, conn, "users")
			relationCountAfter := test.CountTableRows(t, conn, "users_to_ldaps")
			if tc.expectFailure {
				require.Error(t, err)
				require.Equal(t, userCountAfter, userCountBefore)
				require.Equal(t, relationCountAfter, relationCountBefore)
				return
			}
			require.NoError(t, err)
			require.Equal(t, userCountAfter, userCountBefore+1)
			require.Equal(t, relationCountAfter, relationCountBefore+1)

			userGot, err := q.GetUserByID(ctx, tc.user.ToAPI().ID)
			require.NoError(t, err)
			userWant := tc.user.ToAPI()
			userWant.Notification = "" // InsertLDAPUserWithRelation does not insert notification
			userWant.UpdatedAt = userGot.ToAPI().UpdatedAt
			require.Equal(t, userWant, userGot.ToAPI())

			relationGot, err := q.GetLdapRelation(ctx, tc.relation.LdapID, tc.relation.UserUID)
			require.NoError(t, err)
			require.Equal(t, tc.relation, relationGot)
		})
	}
}

func TestIntegrationGetLdapRelation(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	ldapConfig := insertSampleLdapConfig(t, ctx, q, 1, 0)
	user1 := test.InsertSampleUser(t, ctx, q, "testUser").ToAPI()
	relation := insertSampleLdapRelation(t, ctx, q, user1.ID, ldapConfig.ID)

	cases := []struct {
		name          string
		ldapConfigID  string
		userUID       string
		wantRelation  *user.LdapRelation
		expectFailure bool
	}{
		{
			name:          "get valid LDAP relation",
			ldapConfigID:  ldapConfig.ID,
			userUID:       relation.UserUID,
			wantRelation:  relation,
			expectFailure: false,
		},
		{
			name:          "no relation exists for LDAP ID",
			ldapConfigID:  "0",
			userUID:       relation.UserUID,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotRelation, err := q.GetLdapRelation(ctx, tc.ldapConfigID, tc.userUID)
			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantRelation, gotRelation)
		})
	}
}

func TestIntegrationInsertLdapRelation(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)
	user1 := test.InsertSampleUser(t, ctx, q, "testUser").ToAPI()
	ldapConfig := insertSampleLdapConfig(t, ctx, q, 1, 0)

	cases := []struct {
		name          string
		relation      *user.LdapRelation
		expectFailure bool
	}{
		{
			name: "insert valid LDAP relation",
			relation: &user.LdapRelation{
				ID:      1,
				UserID:  user1.ID,
				LdapID:  ldapConfig.ID,
				UserUID: "1000",
				UserDN:  "testuserdn",
			},
			expectFailure: false,
		},
		{
			name:          "insert invalid LDAP relation",
			relation:      &user.LdapRelation{},
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			countBefore := test.CountTableRows(t, conn, "users_to_ldaps")
			err := q.InsertLdapRelation(ctx, tc.relation)
			countAfter := test.CountTableRows(t, conn, "users_to_ldaps")
			if tc.expectFailure {
				require.Error(t, err)
				require.Equal(t, countBefore, countAfter)
				return
			}
			require.NoError(t, err)
			require.Equal(t, countBefore+1, countAfter)

			relationGot, err := q.GetLdapRelation(ctx, tc.relation.LdapID, tc.relation.UserUID)
			require.NoError(t, err)
			require.Equal(t, tc.relation, relationGot)
		})
	}
}

func TestIntegrationUpdateLdapRelation(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)
	user1 := test.InsertSampleUser(t, ctx, q, "testUser")
	user2 := test.InsertSampleUser(t, ctx, q, "testUser")
	ldapConfig := insertSampleLdapConfig(t, ctx, q, 1, 0)
	oldRelation := insertSampleLdapRelation(t, ctx, q, user1.ToAPI().ID, ldapConfig.ID)

	cases := []struct {
		name          string
		relation      *user.LdapRelation
		expectFailure bool
	}{
		{
			name: "update LDAP relation",
			relation: &user.LdapRelation{
				ID:        oldRelation.ID,
				UserID:    user2.ToAPI().ID,
				LdapID:    ldapConfig.ID,
				UserUID:   oldRelation.UserUID,
				UserDN:    "updatedtestuserdn",
				CreatedAt: oldRelation.CreatedAt,
				UpdatedAt: oldRelation.UpdatedAt,
			},
			expectFailure: false,
		},
		{
			name: "update nonexisting LDAP relation",
			relation: &user.LdapRelation{
				ID: 99,
			},
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			countBefore := test.CountTableRows(t, conn, "users_to_ldaps")
			err := q.UpdateLdapRelation(ctx, tc.relation)
			countAfter := test.CountTableRows(t, conn, "users_to_ldaps")
			if tc.expectFailure {
				require.Error(t, err)
				require.Equal(t, countBefore, countAfter)
				return
			}
			require.NoError(t, err)
			require.Equal(t, countBefore, countAfter)

			relationGot, err := q.GetLdapRelation(ctx, tc.relation.LdapID, tc.relation.UserUID)
			require.NoError(t, err)
			require.Equal(t, tc.relation, relationGot)
		})
	}
}

func TestIntegrationDeleteLdapRelation(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)
	users := []*user.UserModel{
		test.InsertSampleUser(t, ctx, q, "testUser"),
		test.InsertSampleUser(t, ctx, q, "testUser"),
		test.InsertSampleUser(t, ctx, q, "testUser"),
	}
	ldapConfig := insertSampleLdapConfig(t, ctx, q, 1, 0)
	for i, u := range users {
		iStr := strconv.Itoa(i)
		err := q.InsertLdapRelation(ctx, &user.LdapRelation{
			UserID:    u.ToAPI().ID,
			LdapID:    ldapConfig.ID,
			UserUID:   iStr,
			UserDN:    "testuserdn" + iStr,
			UpdatedAt: time.Date(2025, time.April, 1, 22, i, 0, 0, time.UTC),
		})
		require.NoError(t, err)
	}

	cases := []struct {
		name               string
		deleteBefore       time.Time
		expectDeletedCount int
	}{
		{
			name:               "delete 2 valid LDAP relations",
			deleteBefore:       time.Date(2025, time.April, 1, 22, 1, 30, 0, time.UTC),
			expectDeletedCount: 2,
		},
		{
			name:               "no valid LDAP relation to delete",
			expectDeletedCount: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			countBefore := test.CountTableRows(t, conn, "users_to_ldaps")
			countDeleted, err := q.DeleteLdapRelations(ctx, tc.deleteBefore)
			require.NoError(t, err)
			countAfter := test.CountTableRows(t, conn, "users_to_ldaps")
			require.Equal(t, tc.expectDeletedCount, countDeleted)
			require.Equal(t, tc.expectDeletedCount, countBefore-countAfter)
		})
	}
}

func TestIntegrationGetLdapUsersWithoutRelations(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	user1, err := user.NewUserModelWithName("testuser1")
	require.NoError(t, err)
	user1.MarkAsLDAP()
	err = q.InsertUser(ctx, user1)
	require.NoError(t, err)

	user2, err := user.NewUserModelWithName("testuser2")
	require.NoError(t, err)
	user2.MarkAsLDAP()
	err = q.InsertUser(ctx, user2)
	require.NoError(t, err)

	user3, err := user.NewUserModelWithName("testuser3")
	require.NoError(t, err)
	err = q.InsertUser(ctx, user3)
	require.NoError(t, err)

	ldapConfig := insertSampleLdapConfig(t, ctx, q, 1, 0)
	err = q.InsertLdapRelation(ctx, &user.LdapRelation{
		UserID:  user1.ToAPI().ID,
		LdapID:  ldapConfig.ID,
		UserUID: "1",
		UserDN:  "testuserdn1",
	})
	require.NoError(t, err)
	err = q.InsertLdapRelation(ctx, &user.LdapRelation{
		UserID:  user3.ToAPI().ID,
		LdapID:  ldapConfig.ID,
		UserUID: "3",
		UserDN:  "testuserdn3",
	})
	require.NoError(t, err)

	wantID := user2.ToAPI().ID
	wantUsername := user2.ToAPI().Username

	got, err := q.GetLdapUsersWithoutRelations(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, wantID, got[0].ToAPI().ID)
	require.Equal(t, wantUsername, got[0].ToAPI().Username)
}

func TestIntegrationGetLdapUserAuthsByUserID(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	tmpl := test.InsertSampleLdapTemplate(t, ctx, manager, "template", nil)
	ldap1 := insertSampleLdapConfig(t, ctx, q, 2, tmpl.ID)
	ldap2 := insertSampleLdapConfig(t, ctx, q, 3, tmpl.ID)
	u := test.InsertSampleUser(t, ctx, q, "testUser").ToAPI()
	rel1 := insertSampleLdapRelation(t, ctx, q, u.ID, ldap1.ID)
	rel2 := insertSampleLdapRelation(t, ctx, q, u.ID, ldap2.ID)

	cases := []struct {
		name    string
		userID  int
		want    []*user.LdapUserAuth
		isEmpty bool
	}{
		{
			name:   "get LDAP template by id",
			userID: u.ID,
			want: []*user.LdapUserAuth{
				{
					Priority: ldap1.Priority,
					Host:     ldap1.Host,
					Port:     ldap1.Port,
					UserDN:   rel1.UserDN,
					UseTLS:   ldap1.UseTLS,
					CACert:   ldap1.CACert,
					FQDN:     ldap1.FQDN,
				},
				{
					Priority: ldap2.Priority,
					Host:     ldap2.Host,
					Port:     ldap2.Port,
					UserDN:   rel2.UserDN,
					UseTLS:   ldap2.UseTLS,
					CACert:   ldap2.CACert,
					FQDN:     ldap2.FQDN,
				},
			},
			isEmpty: false,
		},
		{
			name:    "no LDAP template exists with id",
			userID:  5,
			isEmpty: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := q.GetLdapUserAuthsByUserID(ctx, tc.userID)

			if tc.isEmpty {
				require.Empty(t, result)
				return
			}

			require.NoError(t, err)
			require.Len(t, result, 2)
			require.Equal(t, tc.want[0], result[0])
			require.Equal(t, tc.want[1], result[1])
		})
	}
}

func TestIntegrationGetAllLdapUsersPriorities(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	_ = test.InsertSampleUser(t, ctx, q, "user1").ToAPI()
	// no LDAP users
	result, err := q.GetAllLdapUsersPriorities(ctx)
	require.NoError(t, err)
	require.Len(t, result, 0)

	// three LDAP users with two LDAP servers and diff priorities.
	// u2 in two LDAP servers.
	tmpl := test.InsertSampleLdapTemplate(t, ctx, manager, "template", nil)
	ldap1 := insertSampleLdapConfig(t, ctx, q, 2, tmpl.ID)
	ldap2 := insertSampleLdapConfig(t, ctx, q, 3, tmpl.ID)
	u2 := test.InsertSampleUser(t, ctx, q, "user2").ToAPI()
	u3 := test.InsertSampleUser(t, ctx, q, "user3").ToAPI()
	rel1 := insertSampleLdapRelation(t, ctx, q, u2.ID, ldap1.ID)
	rel2 := insertSampleLdapRelation(t, ctx, q, u2.ID, ldap2.ID)
	rel3 := insertSampleLdapRelation(t, ctx, q, u3.ID, ldap2.ID)

	want := []*user.LdapUserPriority{
		{
			Priority: ldap1.Priority,
			UserID:   u2.ID,
			LdapID:   rel1.LdapID,
		},
		{
			Priority: ldap2.Priority,
			UserID:   u2.ID,
			LdapID:   rel2.LdapID,
		},
		{
			Priority: ldap2.Priority,
			UserID:   u3.ID,
			LdapID:   rel3.LdapID,
		},
	}

	result, err = q.GetAllLdapUsersPriorities(ctx)
	require.NoError(t, err)
	require.Len(t, result, 3)
	require.Equal(t, want[0], result[0])
	require.Equal(t, want[1], result[1])
	require.Equal(t, want[2], result[2])
}

func TestIntegrationInsertMFACerts(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)
	cases := []struct {
		name          string
		certs         []byte
		mfaType       string
		want          *mfa.RawCert
		expectFailure bool
	}{
		{
			name:    "insert MFA certs with 'ca' type",
			certs:   []byte("testcerts"),
			mfaType: "ca",
			want: &mfa.RawCert{
				ID:   1,
				Data: []byte("testcerts"),
			},
			expectFailure: false,
		},
		{
			name:    "insert MFA certs with 'crl' type",
			certs:   []byte("testcerts"),
			mfaType: "crl",
			want: &mfa.RawCert{
				ID:   2,
				Data: []byte("testcerts"),
			},
			expectFailure: false,
		},
		{
			name:          "insert MFA certs for nonexisting type",
			certs:         []byte("testcerts"),
			mfaType:       "wrongenumtype",
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			countBefore := test.CountTableRows(t, conn, "mfa_certs")
			err := q.InsertMFACerts(ctx, tc.certs, tc.mfaType)
			countAfter := test.CountTableRows(t, conn, "mfa_certs")

			if tc.expectFailure {
				require.Error(t, err)
				require.Equal(t, countBefore, countAfter)
				return
			}
			require.NoError(t, err)
			require.Equal(t, countBefore+1, countAfter)

			certsArr, err := q.GetMFACerts(ctx, tc.mfaType)
			require.NoError(t, err)
			require.Contains(t, certsArr, tc.want)
		})
	}
}

func TestIntegrationGetMFACerts(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	err := q.InsertMFACerts(ctx, []byte("testcerts"), "ca")
	require.NoError(t, err)
	err = q.InsertMFACerts(ctx, []byte("testcerts"), "ca")
	require.NoError(t, err)

	cases := []struct {
		name          string
		mfaType       string
		want          []*mfa.RawCert
		expectFailure bool
	}{
		{
			name:    "get MFA certs with 'ca' type",
			mfaType: "ca",
			want: []*mfa.RawCert{
				{
					ID:   1,
					Data: []byte("testcerts"),
				},
				{
					ID:   2,
					Data: []byte("testcerts"),
				},
			},
			expectFailure: false,
		},
		{
			name:          "get MFA certs  with 'crl' type",
			mfaType:       "crl",
			want:          []*mfa.RawCert(nil),
			expectFailure: false,
		},
		{
			name:          "get MFA certs with nonexisting type",
			mfaType:       "wrongenumtype",
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			certs, err := q.GetMFACerts(ctx, tc.mfaType)

			if tc.expectFailure {
				require.Error(t, err)
				require.Len(t, certs, 0)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, certs)
		})
	}
}

func TestIntegrationDeleteMFACerts(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	err := q.InsertMFACerts(ctx, []byte("testcerts1"), "ca")
	require.NoError(t, err)
	err = q.InsertMFACerts(ctx, []byte("testcerts2"), "ca")
	require.NoError(t, err)

	cases := []struct {
		name            string
		certsID         int
		wantToBeDeleted *mfa.RawCert
		expectFailure   bool
	}{
		{
			name:    "delete MFA certs with existing ID",
			certsID: 1,
			wantToBeDeleted: &mfa.RawCert{
				ID:   1,
				Data: []byte("testcerts1"),
			},
			expectFailure: false,
		},
		{
			name:          "delete MFA certs with non-existing ID",
			certsID:       5,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			countBefore := test.CountTableRows(t, conn, "mfa_certs")
			err := q.DeleteMFACerts(ctx, tc.certsID)
			countAfter := test.CountTableRows(t, conn, "mfa_certs")

			if tc.expectFailure {
				require.Error(t, err)
				require.Equal(t, countBefore, countAfter)
				return
			}
			require.NoError(t, err)
			require.Equal(t, countBefore-1, countAfter)

			certsArr, err := q.GetMFACerts(ctx, "ca")
			require.NoError(t, err)
			require.NotContains(t, certsArr, tc.wantToBeDeleted)
		})
	}
}

func TestIntegrationGetAddressPoolByID(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	p1 := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool1"))
	_ = test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool2"))

	cases := []struct {
		name          string
		id            int
		expectedPool  *sqlc.AddressPool
		expectFailure bool
	}{
		{
			name:          "get existing pool",
			id:            p1.ID,
			expectedPool:  p1,
			expectFailure: false,
		},
		{
			name:          "get non-existent pool",
			id:            5,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sqlcq.GetAddressPoolByID(ctx, tc.id)
			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedPool, &got)
		})
	}
}

func TestIntegrationGetAddressPoolByName(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	p1 := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool1"))
	_ = test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool2"))

	cases := []struct {
		name          string
		poolName      string
		expectedPool  *sqlc.AddressPool
		expectFailure bool
	}{
		{
			name:          "get existing pool",
			poolName:      p1.Name,
			expectedPool:  p1,
			expectFailure: false,
		},
		{
			name:          "get non-existent pool",
			poolName:      "non-existent",
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sqlcq.GetAddressPoolByName(ctx, tc.poolName)
			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedPool, &got)
		})
	}
}

func TestIntegrationGetAllAddressPools(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	p1 := test.InsertSampleAddressPool(t, ctx, sqlcq, &sqlc.InsertAddressPoolParams{
		Name:        "testPool1",
		StartAddr:   netip.MustParseAddr("10.0.0.9"),
		EndAddr:     netip.MustParseAddr("10.0.0.50"),
		NetMask:     24,
		Description: "test description",
	})
	p2 := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool2"))

	got, err := sqlcq.GetAllAddressPools(ctx)
	require.NoError(t, err)

	require.Equal(t, p2, &got[0])
	require.Equal(t, p1, &got[1])
}

func TestIntegrationInsertAddressPool(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	params := test.NewSampleInsertAddressPoolParams("testPool")

	countBefore := test.CountTableRows(t, conn, "address_pools")
	pool, err := sqlcq.InsertAddressPool(ctx, *params)
	countAfter := test.CountTableRows(t, conn, "address_pools")

	require.NoError(t, err)
	require.Equal(t, countBefore+1, countAfter)

	require.Equal(t, params.Name, pool.Name)
	require.Equal(t, params.StartAddr, pool.StartAddr)
	require.Equal(t, params.EndAddr, pool.EndAddr)
	require.Equal(t, params.NetMask, pool.NetMask)
	require.Equal(t, params.Description, pool.Description)
}

func TestIntegrationUpdateAddressPool(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	p1 := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool1"))
	_ = test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool2"))

	cases := []struct {
		name             string
		updatePoolParams sqlc.UpdateAddressPoolParams
		expectFailure    bool
	}{
		{
			name: "update existing pool",
			updatePoolParams: sqlc.UpdateAddressPoolParams{
				ID:          p1.ID,
				Name:        "updatedPool",
				StartAddr:   netip.MustParseAddr("100.1.1.10"),
				EndAddr:     netip.MustParseAddr("100.1.1.50"),
				NetMask:     18,
				Description: "description",
				UpdatedAt:   db.NewPgTimestamp(time.Now()),
			},
			expectFailure: false,
		},
		{
			name: "update non-existent pool",
			updatePoolParams: sqlc.UpdateAddressPoolParams{
				ID:   5,
				Name: "non-existent",
			},
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			updatedPool, err := sqlcq.UpdateAddressPool(ctx, tc.updatePoolParams)
			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			require.Equal(t, tc.updatePoolParams.Name, updatedPool.Name)
			require.Equal(t, tc.updatePoolParams.StartAddr, updatedPool.StartAddr)
			require.Equal(t, tc.updatePoolParams.EndAddr, updatedPool.EndAddr)
			require.Equal(t, tc.updatePoolParams.NetMask, updatedPool.NetMask)
			require.Equal(t, tc.updatePoolParams.Description, updatedPool.Description)
			require.WithinDuration(t, tc.updatePoolParams.UpdatedAt.Time, updatedPool.UpdatedAt.Time, time.Millisecond)

			got, err := sqlcq.GetAddressPoolByID(ctx, tc.updatePoolParams.ID)
			require.NoError(t, err)
			require.Equal(t, updatedPool, got)
		})
	}
}

func TestIntegrationDeleteAddressPool(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	p1 := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool1"))
	_ = test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool2"))

	cases := []struct {
		name          string
		id            int
		expectFailure bool
	}{
		{
			name:          "delete existing pool",
			id:            p1.ID,
			expectFailure: false,
		},
		{
			name:          "delete non-existent pool",
			id:            5,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			countBefore := test.CountTableRows(t, conn, "address_pools")
			_, err := sqlcq.DeleteAddressPool(ctx, tc.id)
			countAfter := test.CountTableRows(t, conn, "address_pools")

			if tc.expectFailure {
				require.Error(t, err)
				require.Equal(t, countBefore, countAfter)
				return
			}
			require.NoError(t, err)
			require.Equal(t, countBefore-1, countAfter)
		})
	}
}

func insertSampleLdapConfig(t *testing.T, ctx context.Context, q user.Querier, priority, ldapTemplateID int) *user.LdapConfig {
	t.Helper()
	ldapConfig := newSampleLdapConfig(priority, ldapTemplateID)

	ret, err := sqlcq.InsertLdapConfig(ctx, ldapConfig.ToDBInsertParams())
	require.NoError(t, err)

	ldapConfig.ID = ret.ID
	return ldapConfig
}

func insertSampleLdapConfigTLS(t *testing.T, ctx context.Context, q user.Querier, priority int, useTLS bool, fqdn, caCert string, ldapTemplateID int) *user.LdapConfig {
	t.Helper()
	ldapConfig := newSampleLdapConfigTLS(priority, useTLS, fqdn, caCert, ldapTemplateID)

	ret, err := sqlcq.InsertLdapConfig(ctx, ldapConfig.ToDBInsertParams())
	require.NoError(t, err)

	ldapConfig.ID = ret.ID
	return ldapConfig
}

func newSampleLdapConfig(priority, ldapTemplateID int) *user.LdapConfig {
	return &user.LdapConfig{
		Priority:        priority,
		Host:            "testhost",
		Port:            1,
		BaseDN:          "testbasedn",
		BindDN:          "testbinddn",
		BindPWEncrypted: test.Seal("testbindpw"),
		UserListFilter:  "testuserlistfilter",
		UsernameAttr:    "testusernameattr",
		UIDAttr:         "testuidattr",
		UseTLS:          true,
		CACert:          []byte("testcacert"),
		FQDN:            "testfqdn",
		TemplateID:      ldapTemplateID,
	}
}

func newSampleLdapConfigUpdate(configID string, priority, ldapTemplateID int) *user.LdapConfig {
	return &user.LdapConfig{
		ID: configID,

		Priority:        priority,
		Host:            "testhost2",
		Port:            2,
		BaseDN:          "testbasedn2",
		BindDN:          "testbinddn2",
		BindPWEncrypted: test.Seal("testbindpw2"),
		UserListFilter:  "testuserlistfilter2",
		UsernameAttr:    "testusernameattr2",
		UIDAttr:         "testuidattr2",
		UseTLS:          false,
		CACert:          []byte(""),
		FQDN:            "",
		TemplateID:      ldapTemplateID,
	}
}

func newSampleLdapConfigTLS(priority int, useTLS bool, fqdn, caCert string, ldapTemplateID int) *user.LdapConfig {
	config := newSampleLdapConfig(priority, ldapTemplateID)

	config.UseTLS = useTLS
	config.FQDN = fqdn
	config.CACert = []byte(caCert)

	return config
}

func newSampleLdapConfigTLSUpdate(configID string, priority int, useTLS bool, fqdn, caCert string, ldapTemplateID int) *user.LdapConfig {
	config := newSampleLdapConfigUpdate(configID, priority, ldapTemplateID)

	config.UseTLS = useTLS
	config.FQDN = fqdn
	config.CACert = []byte(caCert)

	return config
}

func insertSampleLdapRelation(t *testing.T, ctx context.Context, q user.Querier, userID int, ldapID string) *user.LdapRelation {
	t.Helper()
	relation := &user.LdapRelation{
		UserID:    userID,
		LdapID:    ldapID,
		UserUID:   "1000",
		UserDN:    "testuserdn",
		CreatedAt: time.Date(2025, time.April, 1, 22, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2025, time.April, 1, 22, 0, 0, 0, time.UTC),
	}
	err := q.InsertLdapRelation(ctx, relation)
	require.NoError(t, err)

	return relation
}
