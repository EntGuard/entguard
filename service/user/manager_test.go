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

package user_test

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/dgryski/dgoogauth"
	"github.com/stretchr/testify/require"

	sqlc "github.com/entguard/entguard/db"
	"github.com/entguard/entguard/pkg/mfa"
	mt "github.com/entguard/entguard/pkg/mfa/test"
	"github.com/entguard/entguard/pkg/test"
	"github.com/entguard/entguard/service/api/v1"
	"github.com/entguard/entguard/service/user"
	"github.com/entguard/entguard/service/vcm"
	pg "github.com/entguard/entguard/service/vcm/drivers/postgres"
)

var manager *user.PostgresManager

func TestValidatePassword(t *testing.T) {
	failure := func(pwd string, minLen int, upper, lower, digit, symbol bool) {
		t.Helper()
		err := user.ValidatePassword(pwd, minLen, upper, lower, digit, symbol)
		require.Error(t, err, fmt.Sprintf("expected non-nil error in ValidatePassword(%s)", pwd))
	}
	success := func(pwd string, minLen int, upper, lower, digit, symbol bool) {
		t.Helper()
		err := user.ValidatePassword(pwd, minLen, upper, lower, digit, symbol)
		require.NoError(t, err, fmt.Sprintf("expected nil error in ValidatePassword(%s)", pwd))
	}

	failure("1", 2, false, false, false, false)
	failure("1*a", 2, true, false, false, false)
	failure("1*A", 2, false, true, false, false)
	failure("*aA", 2, false, false, true, false)
	failure("1aA", 2, false, false, false, true)

	failure("1hhhhA", 6, true, false, true, true)

	success("1", 1, false, false, true, false)
	success("!!!!v777", 8, false, true, true, true)
	success("44*aA", 4, true, true, true, true)

	// security recommendations: minimum length at least 8 characters, strongly
	// recommended 15 characters; no limit on character composition, must allow
	// at least 64 characters (in our implementation we use bcrypt which does
	// not allow more than 72 characters)
	failure("12345678901234", 15, false, false, false, false)
	success("123456789012345", 15, false, false, false, false)
	success(strings.Repeat("12345678", 8), 15, false, false, false, false) // 8*8=64 chars
	success("lowercaselettersonly", 15, false, false, false, false)

	// special characters
	success(" password with spaces ", 15, false, false, false, false)
	success("ášďÁŠĎôÔäÄñÑ([{<`'\"\\世界", 15, false, false, false, false)
}

func TestIntegrationGetAllUsers(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	timestamp0 := time.Now()
	user1 := test.InsertSampleUser(t, ctx, q, "user1").ToAPI()
	timestamp1 := time.Now()
	user2 := test.InsertSampleUser(t, ctx, q, "user2").ToAPI()
	timestamp2 := time.Now()

	retrievedUsers, err := manager.GetAllUsers(context.Background())
	require.NoError(t, err)

	expectedUsers := &api.UserList{
		Users: []api.User{user1, user2},
	}

	test.RequireTimeBetween(t, retrievedUsers.Users[0].UpdatedAt, timestamp0, timestamp1)
	expectedUsers.Users[0].UpdatedAt = retrievedUsers.Users[0].UpdatedAt
	test.RequireTimeBetween(t, retrievedUsers.Users[1].UpdatedAt, timestamp1, timestamp2)
	expectedUsers.Users[1].UpdatedAt = retrievedUsers.Users[1].UpdatedAt
	require.Equal(t, expectedUsers, retrievedUsers)
}

func TestIntegrationGetUserByNameAndPassword(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	u1 := test.InsertSampleUser(t, ctx, q, "user1").ToAPI()

	// add LDAP user and LDAP configs
	tmpl := test.InsertSampleLdapTemplate(t, ctx, manager, "template", nil)
	ldap := insertSampleLdapConfig(t, ctx, q, 2, tmpl.ID)
	u := test.NewSampleUser(t, "user2")
	u.MarkAsLDAP()
	err := q.InsertUser(ctx, u)
	require.NoError(t, err)
	u2 := u.ToAPI()
	insertSampleLdapRelation(t, ctx, q, u2.ID, ldap.ID)

	// mock LDAP service to call VerifyConnection function
	ls := user.NewLdapServiceMock()
	ls.AddUser(ldap.Host, u2.Username)
	manager.LdapService = ls

	cases := []struct {
		name               string
		userName, password string
		want               api.User
		expectFailure      bool
	}{
		{
			name:          "get user with valid password",
			userName:      u1.Username,
			password:      "testpassword",
			want:          u1,
			expectFailure: false,
		},
		{
			name:          "get user with invalid password",
			userName:      u1.Username,
			password:      "notexistingpassword",
			expectFailure: true,
		},
		{
			name:          "get user with non-existing name",
			userName:      "invalidusername",
			password:      "testpassword",
			expectFailure: true,
		},
		{
			name:          "get LDAP user with valid password",
			userName:      u2.Username,
			password:      "testpassword",
			want:          u2,
			expectFailure: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user, err := manager.GetUserByNameAndPassword(ctx, tc.userName, tc.password)

			if tc.expectFailure {
				require.Error(t, err)
				require.Empty(t, user)
				return
			}
			require.NoError(t, err)
			require.Equal(t, &tc.want, user)
		})
	}
}

func TestIntegrationCreate(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	existingUser := test.InsertSampleUser(t, ctx, q, "user1").ToAPI()
	u := test.NewSampleUser(t, "user2")
	err := u.SetID(2)
	require.NoError(t, err)
	tu := u.ToAPI()

	cases := []struct {
		name                  string
		userName, password    string
		mfaType, notification string
		isAdmin               bool
		want                  api.User
		expectFailure         bool
	}{
		{
			name:          "create user",
			userName:      tu.Username,
			password:      "testpassword123",
			mfaType:       tu.MFAType,
			notification:  tu.Notification,
			isAdmin:       true,
			want:          tu,
			expectFailure: false,
		},
		{
			name:          "create user with invalid password",
			userName:      "newtestuser",
			password:      "lessthan15chrs",
			mfaType:       tu.MFAType,
			notification:  tu.Notification,
			isAdmin:       true,
			expectFailure: true,
		},
		{
			name:          "create user with existing user name",
			userName:      existingUser.Username,
			password:      "anotherpassword",
			mfaType:       tu.MFAType,
			notification:  tu.Notification,
			isAdmin:       true,
			expectFailure: true,
		},
		{
			name:          "create user with non-existing mfa type",
			userName:      "somename",
			password:      "somepassword123",
			mfaType:       "invalidmfa",
			notification:  tu.Notification,
			isAdmin:       true,
			expectFailure: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := &api.UserWithPassword{
				Username:     tc.userName,
				Password:     tc.password,
				IsAdmin:      tc.isAdmin,
				MFAType:      tc.mfaType,
				Notification: tc.notification,
			}
			err := manager.CreateUser(ctx, input)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			// if GetUserByNameAndPassword returns no error, it means that the password matches
			got, err := manager.GetUserByNameAndPassword(ctx, tc.want.Username, tc.password)
			require.NoError(t, err)
			tc.want.UpdatedAt = got.UpdatedAt
			require.Equal(t, tc.want, *got)
		})
	}
}

func TestIntegrationUpdate(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	existingUser := test.InsertSampleUser(t, ctx, q, "testUser").ToAPI()
	u := test.NewSampleUser(t, "testUser")
	err := q.InsertUser(ctx, u)
	require.NoError(t, err)

	err = u.SetName("updatedname")
	require.NoError(t, err)
	u.SetMFAType(mfa.TOTP)
	u.SetAdmin(false)
	err = u.SetNotification("updatednotification")
	require.NoError(t, err)

	tu := u.ToAPI()

	p1 := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool1"))
	test.InsertSampleDeviceTemplate(t, ctx, sqlcq, existingUser.ID, []int{p1.ID})
	test.InsertSampleDeviceTemplate(t, ctx, sqlcq, u.ID, []int{p1.ID})

	cases := []struct {
		name                  string
		id                    int
		userName, password    string
		mfaType, notification string
		isAdmin               bool
		want                  api.User
		expectFailure         bool
	}{
		{
			name:          "update user",
			id:            tu.ID,
			userName:      tu.Username,
			mfaType:       tu.MFAType,
			notification:  tu.Notification,
			isAdmin:       tu.IsAdmin,
			want:          tu,
			expectFailure: false,
		},
		{
			name:          "update user with non-existing id",
			id:            5,
			userName:      tu.Username,
			expectFailure: true,
		},
		{
			name:          "update user to existing user name",
			id:            tu.ID,
			userName:      existingUser.Username,
			mfaType:       tu.MFAType,
			expectFailure: true,
		},
		{
			name:          "update user with non-existing mfa type",
			id:            tu.ID,
			userName:      tu.Username,
			mfaType:       "invalidmfa",
			expectFailure: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := &api.UserUpdate{
				Username:     tc.userName,
				IsAdmin:      tc.isAdmin,
				MFAType:      tc.mfaType,
				Notification: tc.notification,
			}
			updated, err := manager.UpdateUser(ctx, tc.id, input)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			tc.want.UpdatedAt = updated.UpdatedAt
			require.Equal(t, &tc.want, updated)
		})
	}
}

func TestIntegrationUpdatePassword(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	u1 := test.InsertSampleUser(t, ctx, q, "testUser").ToAPI()
	u2 := test.NewSampleUser(t, "testUser")
	u2.MarkAsLDAP()
	err := q.InsertUser(ctx, u2)
	require.NoError(t, err)

	p1 := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool1"))
	test.InsertSampleDeviceTemplate(t, ctx, sqlcq, u1.ID, []int{p1.ID})
	test.InsertSampleDeviceTemplate(t, ctx, sqlcq, u2.ID, []int{p1.ID})

	pw := "updatedpassword"

	cases := []struct {
		name          string
		id            int
		password      string
		expectFailure bool
	}{
		{
			name:          "update user password",
			id:            u1.ID,
			password:      pw,
			expectFailure: false,
		},
		{
			name:          "update user password with non-existing id",
			id:            5,
			password:      pw,
			expectFailure: true,
		},
		{
			name:          "update user with invalid password",
			id:            u1.ID,
			password:      "lessthan15chrs",
			expectFailure: true,
		},
		{
			name:          "update password for LDAP user",
			id:            u2.ToAPI().ID,
			password:      pw,
			expectFailure: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := manager.UpdateUserPassword(ctx, tc.id, tc.password)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			user, err := q.GetUserByID(ctx, tc.id)
			require.NoError(t, err)
			require.True(t, user.ComparePasswords(tc.password))
		})
	}
}

func TestIntegrationDeleteUser(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, context.Background(), q)

	u1 := test.InsertSampleUser(t, ctx, q, "testUser1")
	u2 := test.InsertSampleUser(t, ctx, q, "testUser2")
	apiU1 := u1.ToAPI()
	apiU2 := u2.ToAPI()
	pool := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool"))
	apiPool := user.ConvertDBToAPIPool(*pool)
	test.InsertSampleDeviceTemplateAPI(t, ctx, vcManager, apiU1.ID, []*api.AddressPool{apiPool})
	test.InsertSampleDeviceTemplateAPI(t, ctx, vcManager, apiU2.ID, []*api.AddressPool{apiPool})

	cases := []struct {
		name            string
		id              int
		expectedDeleted api.User
		expectFailure   bool
		expectedError   error
	}{
		{
			name:            "first user",
			id:              apiU1.ID,
			expectedDeleted: apiU1,
			expectFailure:   false,
		},
		{
			name:            "second user",
			id:              apiU2.ID,
			expectedDeleted: apiU2,
			expectFailure:   false,
		},
		{
			name:          "non-existing user",
			id:            5,
			expectFailure: true,
			expectedError: user.ErrUserNotFound,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ub, err := manager.GetAllUsers(ctx)
			require.NoError(t, err)
			usersBefore := ub.Users

			err = manager.DeleteUser(ctx, tc.id)

			ua, e := manager.GetAllUsers(ctx)
			require.NoError(t, e)
			usersAfter := ua.Users

			if tc.expectFailure {
				require.Error(t, err)
				require.Equal(t, len(usersBefore), len(usersAfter))
				return
			}
			require.NoError(t, err)
			require.Contains(t, usersBefore, tc.expectedDeleted)
			require.NotContains(t, usersAfter, tc.expectedDeleted)
			require.Equal(t, len(usersBefore)-1, len(usersAfter))
		})
	}
}

func TestIntegrationGetAllLdapConfigs(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// test case: no LDAP configs to return
	expected := &api.LdapConfigList{
		LdapConfigs: []*api.LdapConfigGet{},
	}
	configList, err := manager.GetAllLdapConfigs(ctx)
	require.NoError(t, err)
	require.NotNil(t, configList)
	require.Len(t, configList.LdapConfigs, 0)
	require.Equal(t, expected, configList)

	// test case: available LDAP configs to return
	temp1 := test.InsertSampleLdapTemplate(t, ctx, manager, "temp1", nil)
	config1 := insertSampleLdapConfig(t, ctx, q, 1, temp1.ID)
	config2 := insertSampleLdapConfig(t, ctx, q, 2, 0)

	expected = &api.LdapConfigList{
		LdapConfigs: []*api.LdapConfigGet{config1.ToAPILdapConfigGet(), config2.ToAPILdapConfigGet()},
	}
	configList, err = manager.GetAllLdapConfigs(ctx)
	require.NoError(t, err)
	require.NotNil(t, configList)
	require.Len(t, configList.LdapConfigs, 2)
	require.Equal(t, expected, configList)
}

func TestIntegrationGetLdapConfig(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	temp1 := test.InsertSampleLdapTemplate(t, ctx, manager, "temp1", nil)
	temp2 := test.InsertSampleLdapTemplate(t, ctx, manager, "temp2", nil)
	config1 := insertSampleLdapConfig(t, ctx, q, 1, temp1.ID)
	_ = insertSampleLdapConfig(t, ctx, q, 2, temp2.ID)

	cases := []struct {
		name           string
		configID       string
		expectedConfig *api.LdapConfigGet
		expectFailure  bool
	}{
		{
			name:           "existing LDAP config",
			configID:       config1.ID,
			expectedConfig: config1.ToAPILdapConfigGet(),
			expectFailure:  false,
		},
		{
			name:          "non-existing LDAP config",
			configID:      "00112233-4455-6677-8899-aabbccddeeff",
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			retrievedConfig, err := manager.GetLdapConfig(ctx, tc.configID)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedConfig, retrievedConfig)
		})
	}
}

func TestIntegrationCreateLdapConfig(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, context.Background(), q)

	conn := q.GetDbConnection(ctx)

	temp1 := test.InsertSampleLdapTemplate(t, ctx, manager, "temp1", nil)
	config1 := insertSampleLdapConfig(t, ctx, q, 100, temp1.ID)

	cases := []struct {
		name          string
		templateID    int
		priority      int
		useTLS        bool
		fqdn          string
		caCert        string
		expectFailure bool
	}{
		// ---- All fields set -------------------------------------------------
		{
			name:          "All fields set",
			templateID:    temp1.ID,
			priority:      1,
			useTLS:        true,
			fqdn:          "testFQDN",
			caCert:        mt.CA,
			expectFailure: false,
		},
		// ---- TLS, FQDN and CACERT combinations ------------------------------
		{
			name:          "TLS, FQDN, no cacert - error: incomplete tls configuration",
			templateID:    temp1.ID,
			priority:      2,
			useTLS:        true,
			fqdn:          "testFQDN",
			caCert:        "",
			expectFailure: true,
		},
		{
			name:          "TLS, no fqdn, CACERT - error: incomplete tls configuration",
			templateID:    temp1.ID,
			priority:      3,
			useTLS:        true,
			fqdn:          "",
			caCert:        mt.CA,
			expectFailure: true,
		},
		{
			name:          "TLS, no fqdn, no cacert - ok",
			templateID:    temp1.ID,
			priority:      4,
			useTLS:        true,
			fqdn:          "",
			caCert:        "",
			expectFailure: false,
		},
		{
			name:          "no tls, FQDN, CACERT - error: fqdn and cacert require tls",
			templateID:    temp1.ID,
			priority:      5,
			useTLS:        false,
			fqdn:          "testFQDN",
			caCert:        mt.CA,
			expectFailure: true,
		},
		{
			name:          "no tls, FQDN, no cacert - error: fqdn requires tls",
			templateID:    temp1.ID,
			priority:      6,
			useTLS:        false,
			fqdn:          "testFQDN",
			caCert:        "",
			expectFailure: true,
		},
		{
			name:          "no tls, no fqdn, CACERT - error: cacert requires tls",
			templateID:    temp1.ID,
			priority:      7,
			useTLS:        false,
			fqdn:          "",
			caCert:        mt.CA,
			expectFailure: true,
		},
		{
			name:          "no tls, no fqdn, no cacert - ok",
			templateID:    temp1.ID,
			priority:      8,
			useTLS:        false,
			fqdn:          "",
			caCert:        "",
			expectFailure: false,
		},
		// ---- other validations ----------------------------------------------
		{
			name:          "too long FQDN",
			templateID:    temp1.ID,
			priority:      9,
			useTLS:        true,
			fqdn:          tooLongFQDN,
			caCert:        mt.CA,
			expectFailure: true,
		},
		{
			name:          "invalid CA CERT",
			templateID:    temp1.ID,
			priority:      10,
			useTLS:        true,
			fqdn:          "testFQDN",
			caCert:        "invalidCaCert",
			expectFailure: true,
		},
		{
			name:          "confict with existing priority",
			templateID:    temp1.ID,
			priority:      config1.Priority,
			useTLS:        true,
			fqdn:          "testFQDN",
			caCert:        mt.CA,
			expectFailure: true,
		},
		{
			name:          "non-existing LDAP template",
			templateID:    5,
			priority:      12,
			useTLS:        true,
			fqdn:          "testFQDN",
			caCert:        mt.CA,
			expectFailure: true,
		},
		{
			name:          "no LDAP template (valid)",
			templateID:    0,
			priority:      13,
			useTLS:        true,
			fqdn:          "testFQDN",
			caCert:        mt.CA,
			expectFailure: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			countBefore := test.CountTableRows(t, conn, "ldaps")

			config := newSampleLdapConfigTLS(tc.priority, tc.useTLS, tc.fqdn, tc.caCert, tc.templateID)
			apiConfig := userLdapConfigToAPILdapConfig(config)
			configGet, err := manager.CreateLdapConfig(ctx, apiConfig)

			countAfter := test.CountTableRows(t, conn, "ldaps")

			if tc.expectFailure {
				require.Error(t, err)
				require.Equal(t, countBefore, countAfter)
				return
			}
			require.NoError(t, err)
			require.Equal(t, countBefore+1, countAfter)

			// check value returned by CreateLdapConfig
			expectedConfigGet := config.ToAPILdapConfigGet()
			expectedConfigGet.ID = configGet.ID
			require.Equal(t, expectedConfigGet, configGet)

			// check actual value inserted into database
			retrievedConfigGet, err := manager.GetLdapConfig(ctx, configGet.ID)
			require.NoError(t, err)
			require.Equal(t, expectedConfigGet, retrievedConfigGet)
		})
	}
}

func TestIntegrationUpdateLdapConfig(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	tempOld := test.InsertSampleLdapTemplate(t, ctx, manager, "tempOld", nil)
	temp1 := test.InsertSampleLdapTemplate(t, ctx, manager, "temp1", nil)
	config1 := insertSampleLdapConfigTLS(t, ctx, q, 100, true, "oldFQDN", "oldCACert", tempOld.ID)
	configNoTLS := insertSampleLdapConfigTLS(t, ctx, q, 101, false, "", "", temp1.ID)

	cases := []struct {
		name             string
		noChangePassword bool
		configID         string
		templateID       int
		priority         int
		useTLS           bool
		fqdn             string
		caCert           string
		expectFailure    bool
	}{
		// ---- All fields set -------------------------------------------------
		{
			name:          "All fields set",
			configID:      config1.ID,
			templateID:    temp1.ID,
			priority:      1,
			useTLS:        true,
			fqdn:          "testFQDN",
			caCert:        mt.CA,
			expectFailure: false,
		},
		// ---- TLS, FQDN and CACERT combinations ------------------------------
		{
			name:          "TLS, FQDN, no cacert - error: incomplete tls configuration",
			configID:      config1.ID,
			templateID:    temp1.ID,
			priority:      2,
			useTLS:        true,
			fqdn:          "testFQDN",
			caCert:        "",
			expectFailure: true,
		},
		{
			name:          "TLS, no fqdn, CACERT - error: incomplete tls configuration",
			configID:      config1.ID,
			templateID:    temp1.ID,
			priority:      3,
			useTLS:        true,
			fqdn:          "",
			caCert:        mt.CA,
			expectFailure: true,
		},
		{
			name:          "TLS, no fqdn, no cacert - ok",
			configID:      config1.ID,
			templateID:    temp1.ID,
			priority:      4,
			useTLS:        true,
			fqdn:          "",
			caCert:        "",
			expectFailure: false,
		},
		{
			name:          "no tls, FQDN, CACERT - error: fqdn and cacert require tls",
			configID:      config1.ID,
			templateID:    temp1.ID,
			priority:      5,
			useTLS:        false,
			fqdn:          "testFQDN",
			caCert:        mt.CA,
			expectFailure: true,
		},
		{
			name:          "no tls, FQDN, no cacert - error: fqdn requires tls",
			configID:      config1.ID,
			templateID:    temp1.ID,
			priority:      6,
			useTLS:        false,
			fqdn:          "testFQDN",
			caCert:        "",
			expectFailure: true,
		},
		{
			name:          "no tls, no fqdn, CACERT - error: cacert requires tls",
			configID:      config1.ID,
			templateID:    temp1.ID,
			priority:      7,
			useTLS:        false,
			fqdn:          "",
			caCert:        mt.CA,
			expectFailure: true,
		},
		{
			name:          "no tls, no fqdn, no cacert - ok",
			configID:      config1.ID,
			templateID:    temp1.ID,
			priority:      8,
			useTLS:        false,
			fqdn:          "",
			caCert:        "",
			expectFailure: false,
		},
		// ---- other validations ----------------------------------------------
		{
			name:          "too long FQDN",
			configID:      config1.ID,
			templateID:    temp1.ID,
			priority:      9,
			useTLS:        true,
			fqdn:          tooLongFQDN,
			caCert:        mt.CA,
			expectFailure: true,
		},
		{
			name:          "invalid CA CERT",
			configID:      config1.ID,
			templateID:    temp1.ID,
			priority:      10,
			useTLS:        true,
			fqdn:          "testFQDN",
			caCert:        "invalidCaCert",
			expectFailure: true,
		},
		{
			name:          "non-existing LDAP template",
			configID:      config1.ID,
			templateID:    5,
			priority:      12,
			useTLS:        true,
			fqdn:          "testFQDN",
			caCert:        mt.CA,
			expectFailure: true,
		},
		{
			name:          "no LDAP template (valid)",
			configID:      config1.ID,
			templateID:    0,
			priority:      13,
			useTLS:        true,
			fqdn:          "testFQDN",
			caCert:        mt.CA,
			expectFailure: false,
		},
		// ---- new test cases for UPDATE tests --------------------------------
		{
			name:          "non-existing LDAP config",
			configID:      "00112233-4455-6677-8899-aabbccddeeff",
			templateID:    temp1.ID,
			priority:      14,
			useTLS:        true,
			fqdn:          "testFQDN",
			caCert:        mt.CA,
			expectFailure: true,
		},
		{
			name:             "not changed password",
			noChangePassword: true,
			configID:         config1.ID,
			templateID:       temp1.ID,
			priority:         15,
			useTLS:           true,
			fqdn:             "testFQDN",
			caCert:           mt.CA,
			expectFailure:    false,
		},
		{
			name:          "config that formerly had no TLS",
			configID:      configNoTLS.ID,
			templateID:    temp1.ID,
			priority:      16,
			useTLS:        true,
			fqdn:          "testFQDN",
			caCert:        mt.CA,
			expectFailure: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// at each test case, reset config1 to old values; otherwise there
			// could be false positives because the LDAP config may have been
			// already updated to new values in previous test case
			_, err := sqlcq.UpdateLdapConfig(ctx, config1.ToDBUpdateParams())
			require.NoError(t, err)

			countBefore := test.CountTableRows(t, conn, "ldaps")

			configUpdate := newSampleLdapConfigTLSUpdate("", tc.priority, tc.useTLS, tc.fqdn, tc.caCert, tc.templateID)
			// configUpdate.ID is now empty to ensure that UpdateLdapConfig uses the ID only from its second argument
			apiConfigUpdate := userLdapConfigToAPILdapConfig(configUpdate)

			expectedPassword := apiConfigUpdate.BindPW
			if tc.noChangePassword {
				apiConfigUpdate.BindPW = ""
				expectedPassword = test.OpenString(config1.BindPWEncrypted)
			}

			err = manager.UpdateLdapConfig(ctx, apiConfigUpdate, tc.configID)

			countAfter := test.CountTableRows(t, conn, "ldaps")
			require.Equal(t, countBefore, countAfter)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			expectedConfigGet := configUpdate.ToAPILdapConfigGet()
			expectedConfigGet.ID = tc.configID
			retrievedConfigGet, err := manager.GetLdapConfig(ctx, tc.configID)
			require.NoError(t, err)
			require.Equal(t, expectedConfigGet, retrievedConfigGet)

			configWithPassword, err := sqlcq.GetLdapConfigByID(ctx, tc.configID)
			require.NoError(t, err)
			require.Equal(t, expectedPassword, test.OpenString(configWithPassword.BindPwEncrypted))
		})
	}
}

func TestIntegrationDeleteLdapConfig(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, context.Background(), q)

	conn := q.GetDbConnection(ctx)

	temp1 := test.InsertSampleLdapTemplate(t, ctx, manager, "temp1", nil)
	config1 := insertSampleLdapConfig(t, ctx, q, 1, temp1.ID)
	_ = insertSampleLdapConfig(t, ctx, q, 2, temp1.ID)

	cases := []struct {
		name            string
		configID        string
		expectedDeleted *api.LdapConfigGet
		expectFailure   bool
	}{
		{
			name:            "existing LDAP config",
			configID:        config1.ID,
			expectedDeleted: config1.ToAPILdapConfigGet(),
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
			configsBefore, err := manager.GetAllLdapConfigs(ctx)
			require.NoError(t, err)

			err = manager.DeleteLdapConfig(ctx, tc.configID)

			countAfter := test.CountTableRows(t, conn, "ldaps")
			configsAfter, e := manager.GetAllLdapConfigs(ctx)
			require.NoError(t, e)

			if tc.expectFailure {
				require.Error(t, err)
				require.Equal(t, countBefore, countAfter)
			} else {
				require.NoError(t, err)
				require.Equal(t, countBefore-1, countAfter)
				require.Contains(t, configsBefore.LdapConfigs, tc.expectedDeleted)
			}
			require.NotContains(t, configsAfter.LdapConfigs, tc.expectedDeleted)
		})
	}
}

func TestIntegrationGetLdapTemplateServers(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	server1 := test.InsertSampleServer(t, ctx, conn, "server1")
	server2 := test.InsertSampleServer(t, ctx, conn, "server2")
	tmpl := test.InsertSampleLdapTemplate(t, ctx, manager, "template1", nil)

	lts1 := &user.LdapTemplateServer{
		ServerName:           server1.Name,
		ServerID:             server1.ID,
		LdapTemplateID:       tmpl.ID,
		ClientSideAllowedIPs: []string{"1.1.0.0/16", "168.10.10.10/32", "186.178.1.1/32"},
		UsePresharedKey:      true,
	}
	err := q.InsertLdapTemplateServer(ctx, lts1)
	require.NoError(t, err)

	lts2 := &user.LdapTemplateServer{
		ServerName:           server2.Name,
		ServerID:             server2.ID,
		LdapTemplateID:       tmpl.ID,
		ClientSideAllowedIPs: []string{"128.0.0.0/7", "186.178.1.1/32"},
		UsePresharedKey:      true,
	}
	err = q.InsertLdapTemplateServer(ctx, lts2)
	require.NoError(t, err)

	ldapList, err := manager.GetLdapTemplateServers(ctx, tmpl.ID)
	require.NoError(t, err)

	templates := ldapList.LdapTemplateServers
	require.Len(t, templates, 2)
	require.Equal(t, lts1.ToAPI(), templates[0])
	require.Equal(t, lts2.ToAPI(), templates[1])
}

func TestIntegrationCreateLdapTemplateServer(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.Conn
	server := test.InsertSampleServer(t, ctx, conn, "server")
	tmpl := test.InsertSampleLdapTemplate(t, ctx, manager, "template", nil)

	cases := []struct {
		name          string
		input         *api.LdapTemplateServer
		errMsg        string
		expectFailure bool
	}{
		{
			name: "create LDAP template server with valid pre shared key - healthcheck automatically added",
			input: &api.LdapTemplateServer{
				ServerName:           server.Name,
				ServerID:             server.ID,
				ClientSideAllowedIPs: []string{"1.1.1.0/24", "192.168.0.0/16"},
				UsePresharedKey:      true,
			},
			expectFailure: false,
		},
		{
			name: "create LDAP template server - healthcheck already in network range",
			input: &api.LdapTemplateServer{
				ServerName:           server.Name,
				ServerID:             server.ID,
				ClientSideAllowedIPs: []string{"186.178.0.0/16"}, // contains healthcheck 186.178.1.1
				UsePresharedKey:      false,
			},
			expectFailure: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeCount := test.CountTableRows(t, conn, "ldap_template_servers")

			err := manager.CreateLdapTemplateServer(ctx, tmpl.ID, tc.input)

			afterCount := test.CountTableRows(t, conn, "ldap_template_servers")
			if tc.expectFailure {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.errMsg)
				require.Equal(t, beforeCount, afterCount)
				return
			}
			require.NoError(t, err)
			templates, err := q.GetLdapTemplateServers(ctx, tmpl.ID)
			require.NoError(t, err)
			require.Equal(t, beforeCount+1, afterCount)
			require.Equal(t, afterCount, len(templates))

			for _, template := range templates {
				allowedPrefixes, err := user.ConvertAllowedIPsStringsToPrefixes(template.ClientSideAllowedIPs)
				require.NoError(t, err)
				require.True(t, vcm.IsHealthcheckAddrInAllowedIPs(allowedPrefixes, server.HealthCheckAddress))
			}
		})
	}
}

func TestIntegrationUpdateLdapTemplateServer(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)
	server1 := test.InsertSampleServer(t, ctx, conn, "server1")
	server2 := test.InsertSampleServer(t, ctx, conn, "server2")
	tmpl := test.InsertSampleLdapTemplate(t, ctx, manager, "template", nil)

	ldapTmplServer1 := &user.LdapTemplateServer{
		ServerName:           server1.Name,
		ServerID:             server1.ID,
		LdapTemplateID:       tmpl.ID,
		ClientSideAllowedIPs: []string{"1.1.1.0/24", "186.178.1.1/32"},
		UsePresharedKey:      true,
	}
	err := q.InsertLdapTemplateServer(ctx, ldapTmplServer1)
	require.NoError(t, err)

	ldapTmplServer2 := &user.LdapTemplateServer{
		ServerName:           server2.Name,
		ServerID:             server2.ID,
		LdapTemplateID:       tmpl.ID,
		ClientSideAllowedIPs: []string{"2.2.2.0/24", "186.178.1.1/32"},
		UsePresharedKey:      true,
	}
	err = q.InsertLdapTemplateServer(ctx, ldapTmplServer2)
	require.NoError(t, err)

	cases := []struct {
		name           string
		ldapTmplServer *user.LdapTemplateServer
		input          *api.LdapTemplateServer
		expectedServer *pg.Server
		errMsg         string
		expectFailure  bool
	}{
		{
			name:           "update LDAP template server with valid pre shared key - healthcheck automatically added",
			ldapTmplServer: ldapTmplServer1,
			input: &api.LdapTemplateServer{
				ServerName:           server2.Name,
				ServerID:             server2.ID,
				ClientSideAllowedIPs: []string{"192.168.0.0/16"},
				UsePresharedKey:      true,
			},
			expectedServer: server2,
			expectFailure:  false,
		},
		{
			name:           "update LDAP template server - healthcheck already in range",
			ldapTmplServer: ldapTmplServer2,
			input: &api.LdapTemplateServer{
				ServerName:           server1.Name,
				ServerID:             server1.ID,
				ClientSideAllowedIPs: []string{"186.178.1.0/24"},
				UsePresharedKey:      false,
			},
			expectedServer: server1,
			expectFailure:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeCount := test.CountTableRows(t, conn, "ldap_template_servers")

			err := manager.UpdateLdapTemplateServer(ctx, tmpl.ID, tc.ldapTmplServer.ServerID, tc.input)

			afterCount := test.CountTableRows(t, conn, "ldap_template_servers")
			require.Equal(t, beforeCount, afterCount)
			if tc.expectFailure {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.errMsg)
				return
			}
			require.NoError(t, err)

			templates, err := q.GetLdapTemplateServers(ctx, tmpl.ID)
			require.NoError(t, err)

			for _, template := range templates {
				if template.ServerID == tc.expectedServer.ID {
					allowedPrefixes, err := user.ConvertAllowedIPsStringsToPrefixes(template.ClientSideAllowedIPs)
					require.NoError(t, err)
					require.True(t, vcm.IsHealthcheckAddrInAllowedIPs(allowedPrefixes, tc.expectedServer.HealthCheckAddress))
				}
			}
		})
	}
}

func TestIntegrationGetAllLdapTemplates(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// test case: no LDAP templates to return
	expected := []*api.LdapTemplate{}
	templateList, err := manager.GetAllLdapTemplates(ctx)
	require.NoError(t, err)
	require.NotNil(t, templateList)
	require.Len(t, templateList, 0)
	require.Equal(t, expected, templateList)

	// test case: available LDAP templates to return
	tmpl1 := test.InsertSampleLdapTemplate(t, ctx, manager, "template1", nil)
	tmpl2 := test.InsertSampleLdapTemplate(t, ctx, manager, "template2", nil)

	expected = []*api.LdapTemplate{tmpl1, tmpl2}
	templateList, err = manager.GetAllLdapTemplates(ctx)
	require.NoError(t, err)
	require.NotNil(t, templateList)
	require.Len(t, templateList, 2)
	require.Equal(t, expected, templateList)
}

func TestIntegrationCreateLdapTemplate(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.Conn
	tmpl1 := test.InsertSampleLdapTemplate(t, ctx, manager, "template1", nil)

	p1Params := test.NewSampleInsertAddressPoolParams("testPool1")
	p1Params.StartAddr = netip.MustParseAddr("10.0.1.1")
	p1Params.EndAddr = netip.MustParseAddr("10.0.1.100")
	p1Params.NetMask = 24
	p1 := test.InsertSampleAddressPool(t, ctx, sqlcq, p1Params)

	p2Params := test.NewSampleInsertAddressPoolParams("testPool2")
	p2Params.StartAddr = netip.MustParseAddr("10.0.2.1")
	p2Params.EndAddr = netip.MustParseAddr("10.0.2.100")
	p2Params.NetMask = 24
	p2 := test.InsertSampleAddressPool(t, ctx, sqlcq, p2Params)

	adp := []*api.AddressPool{user.ConvertDBToAPIPool(*p1), user.ConvertDBToAPIPool(*p2)}

	poolOverlap1, poolOverlap2 := test.SetupOverlappingPools(t, ctx, sqlcq)
	overlapPools := []*api.AddressPool{user.ConvertDBToAPIPool(*poolOverlap1), user.ConvertDBToAPIPool(*poolOverlap2)}

	cases := []struct {
		name          string
		templateName  string
		mfaType       string
		ifaceName     string
		addressPools  []*api.AddressPool
		listenPort    string
		mtu           string
		dns           []string
		expectFailure bool
	}{
		{
			name:          "create LDAP template with allowed mfa type",
			templateName:  "newtemplate",
			mfaType:       "cert",
			ifaceName:     "ifacename",
			addressPools:  adp,
			listenPort:    "51820",
			mtu:           "1420",
			dns:           []string{"8.8.8.8", "8.8.4.4"},
			expectFailure: false,
		},
		{
			name:          "create LDAP template with invalid mfa type",
			templateName:  "addnewtemplate",
			mfaType:       "testmfa",
			ifaceName:     "ifacename",
			addressPools:  adp,
			expectFailure: true,
		},
		{
			name:          "create LDAP template with the same name",
			templateName:  tmpl1.Name,
			mfaType:       "cert",
			ifaceName:     "ifacename",
			addressPools:  adp,
			expectFailure: true,
		},
		{
			name:          "create LDAP template with no interface name",
			templateName:  "newname",
			mfaType:       "cert",
			ifaceName:     "",
			addressPools:  adp,
			expectFailure: true,
		},
		{
			name:          "create LDAP template with empty address pools",
			templateName:  "emptyPoolsTemplate",
			mfaType:       "cert",
			ifaceName:     "iface0",
			listenPort:    "2222",
			mtu:           "9000",
			addressPools:  []*api.AddressPool{},
			expectFailure: true,
		},
		{
			name:          "create LDAP template with single address pool",
			templateName:  "singlePoolTemplate",
			mfaType:       "totp",
			ifaceName:     "iface1",
			addressPools:  []*api.AddressPool{user.ConvertDBToAPIPool(*p1)},
			listenPort:    "2222",
			mtu:           "9000",
			dns:           []string{"1.1.1.1"},
			expectFailure: false,
		},
		{
			name:          "create LDAP template with non-existent address pool",
			templateName:  "invalidPoolTemplate",
			mfaType:       "cert",
			ifaceName:     "iface3",
			listenPort:    "2222",
			mtu:           "9000",
			dns:           []string{"1.1.1.1"},
			addressPools:  []*api.AddressPool{{ID: 99999, Name: "nonexistent"}},
			expectFailure: true,
		},
		{
			name:          "create LDAP template with mixed valid and invalid pools",
			templateName:  "mixedPoolsTemplate",
			mfaType:       "totp",
			ifaceName:     "iface4",
			listenPort:    "2222",
			mtu:           "9000",
			dns:           []string{"1.1.1.1"},
			addressPools:  []*api.AddressPool{user.ConvertDBToAPIPool(*p1), {ID: 99998, Name: "invalid"}},
			expectFailure: true,
		},
		{
			name:          "create LDAP template with overlapping address pools",
			templateName:  "overlapTemplate",
			mfaType:       "cert",
			ifaceName:     "wg-overlap",
			listenPort:    "51820",
			mtu:           "1420",
			dns:           []string{"8.8.8.8"},
			addressPools:  overlapPools,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := test.NewSampleLdapTemplate(tc.templateName, tc.addressPools)
			input.Name = tc.templateName
			input.MFAType = tc.mfaType
			input.InterfaceName = tc.ifaceName
			input.ListenPort = tc.listenPort
			input.MTU = tc.mtu
			input.DNS = tc.dns

			beforeCount := test.CountTableRows(t, conn, "ldap_templates")

			var err error
			input.ID, err = manager.CreateLdapTemplate(ctx, input)

			afterCount := test.CountTableRows(t, conn, "ldap_templates")
			if tc.expectFailure {
				require.Error(t, err)
				require.Equal(t, beforeCount, afterCount)
				return
			}
			require.NoError(t, err)
			require.Equal(t, beforeCount+1, afterCount)

			temp, err := manager.GetLdapTemplate(ctx, input.ID)
			require.NoError(t, err)
			require.Equal(t, input, temp)

			require.Equal(t, len(tc.addressPools), len(temp.AddressPools))
			for i, expectedPool := range tc.addressPools {
				require.Equal(t, expectedPool.ID, temp.AddressPools[i].ID)
				require.Equal(t, expectedPool.Name, temp.AddressPools[i].Name)
			}

			require.Equal(t, tc.ifaceName, temp.InterfaceName)
			require.Equal(t, tc.listenPort, temp.ListenPort)
			require.Equal(t, tc.mtu, temp.MTU)
			require.Equal(t, tc.dns, temp.DNS)
		})
	}
}

func TestIntegrationUpdateLdapTemplate(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.Conn

	p1 := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("updatePool1"))

	basicTemplate := test.InsertSampleLdapTemplate(t, ctx, manager, "basicTemplate", []*api.AddressPool{user.ConvertDBToAPIPool(*p1)})
	interfaceTemplate := test.InsertSampleLdapTemplate(t, ctx, manager, "interfaceTemplate", []*api.AddressPool{user.ConvertDBToAPIPool(*p1)})
	clearIfaceTemplate := test.InsertSampleLdapTemplate(t, ctx, manager, "clearIfaceTemplate", []*api.AddressPool{user.ConvertDBToAPIPool(*p1)})
	emptyIfaceTemplate := test.InsertSampleLdapTemplate(t, ctx, manager, "emptyIfaceTemplate", []*api.AddressPool{user.ConvertDBToAPIPool(*p1)})

	cases := []struct {
		name           string
		templateID     int
		updateTemplate *api.LdapTemplate
		expectFailure  bool
	}{
		{
			name:       "update template basic info",
			templateID: basicTemplate.ID,
			updateTemplate: &api.LdapTemplate{
				ID:            basicTemplate.ID,
				Name:          "updatedTemplate",
				Filter:        "updated filter",
				MFAType:       string(mfa.TOTP),
				IsAdmin:       false,
				InterfaceName: "updatedIface",
				AddressPools:  basicTemplate.AddressPools,
				ListenPort:    "2222",
				MTU:           "1500",
				DNS:           []string{"8.8.8.8"},
			},
		},
		{
			name:       "update template interface settings",
			templateID: interfaceTemplate.ID,
			updateTemplate: &api.LdapTemplate{
				ID:            interfaceTemplate.ID,
				Name:          interfaceTemplate.Name,
				Filter:        interfaceTemplate.Filter,
				MFAType:       string(mfa.CERT),
				IsAdmin:       true,
				InterfaceName: "custom-wg-interface",
				AddressPools:  interfaceTemplate.AddressPools,
				ListenPort:    "51830",
				MTU:           "9000",
				DNS:           []string{"1.1.1.1", "1.0.0.1", "8.8.8.8"},
			},
		},
		{
			name:       "update template clear interface settings",
			templateID: clearIfaceTemplate.ID,
			updateTemplate: &api.LdapTemplate{
				ID:            clearIfaceTemplate.ID,
				Name:          clearIfaceTemplate.Name,
				Filter:        clearIfaceTemplate.Filter,
				MFAType:       string(mfa.CERT),
				IsAdmin:       true,
				InterfaceName: clearIfaceTemplate.InterfaceName,
				AddressPools:  clearIfaceTemplate.AddressPools,
				ListenPort:    "",
				MTU:           "",
				DNS:           []string{},
			},
		},
		{
			name:       "update template with empty interface name",
			templateID: emptyIfaceTemplate.ID,
			updateTemplate: &api.LdapTemplate{
				ID:            emptyIfaceTemplate.ID,
				Name:          emptyIfaceTemplate.Name,
				Filter:        emptyIfaceTemplate.Filter,
				MFAType:       emptyIfaceTemplate.MFAType,
				IsAdmin:       emptyIfaceTemplate.IsAdmin,
				InterfaceName: "",
				AddressPools:  emptyIfaceTemplate.AddressPools,
				ListenPort:    emptyIfaceTemplate.ListenPort,
				MTU:           emptyIfaceTemplate.MTU,
				DNS:           emptyIfaceTemplate.DNS,
			},
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeCount := test.CountTableRows(t, conn, "ldap_templates")

			err := manager.UpdateLdapTemplate(ctx, tc.templateID, tc.updateTemplate)

			afterCount := test.CountTableRows(t, conn, "ldap_templates")

			if tc.expectFailure {
				require.Error(t, err)
				require.Equal(t, beforeCount, afterCount)
				return
			}

			require.NoError(t, err)
			require.Equal(t, beforeCount, afterCount)

			updatedTemplate, err := manager.GetLdapTemplate(ctx, tc.templateID)
			require.NoError(t, err)
			require.Equal(t, tc.updateTemplate, updatedTemplate)
		})
	}
}

func TestIntegrationUpdateLdapTemplateAddressPools(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.Conn

	p1Params := test.NewSampleInsertAddressPoolParams("poolUpdatePool1")
	p1Params.StartAddr = netip.MustParseAddr("10.10.1.1")
	p1Params.EndAddr = netip.MustParseAddr("10.10.1.100")
	p1Params.NetMask = 24
	p1 := test.InsertSampleAddressPool(t, ctx, sqlcq, p1Params)

	p2Params := test.NewSampleInsertAddressPoolParams("poolUpdatePool2")
	p2Params.StartAddr = netip.MustParseAddr("10.10.2.1")
	p2Params.EndAddr = netip.MustParseAddr("10.10.2.100")
	p2Params.NetMask = 24
	p2 := test.InsertSampleAddressPool(t, ctx, sqlcq, p2Params)

	p3Params := test.NewSampleInsertAddressPoolParams("poolUpdatePool3")
	p3Params.StartAddr = netip.MustParseAddr("10.10.3.1")
	p3Params.EndAddr = netip.MustParseAddr("10.10.3.100")
	p3Params.NetMask = 24
	p3 := test.InsertSampleAddressPool(t, ctx, sqlcq, p3Params)

	p4Params := test.NewSampleInsertAddressPoolParams("poolUpdatePool4")
	p4Params.StartAddr = netip.MustParseAddr("10.10.4.1")
	p4Params.EndAddr = netip.MustParseAddr("10.10.4.100")
	p4Params.NetMask = 24
	p4 := test.InsertSampleAddressPool(t, ctx, sqlcq, p4Params)

	poolOverlap1, poolOverlap2 := test.SetupOverlappingPools(t, ctx, sqlcq)
	overlapPools := []*api.AddressPool{user.ConvertDBToAPIPool(*poolOverlap1), user.ConvertDBToAPIPool(*poolOverlap2)}

	addPoolsTemplate := test.InsertSampleLdapTemplate(t, ctx, manager, "addPoolsTemplate", []*api.AddressPool{user.ConvertDBToAPIPool(*p1)})
	removePoolsTemplate := test.InsertSampleLdapTemplate(t, ctx, manager, "removePoolsTemplate", []*api.AddressPool{user.ConvertDBToAPIPool(*p1), user.ConvertDBToAPIPool(*p2), user.ConvertDBToAPIPool(*p3)})
	replacePoolsTemplate := test.InsertSampleLdapTemplate(t, ctx, manager, "replacePoolsTemplate", []*api.AddressPool{user.ConvertDBToAPIPool(*p1), user.ConvertDBToAPIPool(*p2)})
	invalidPoolTemplate := test.InsertSampleLdapTemplate(t, ctx, manager, "invalidPoolTemplate", []*api.AddressPool{user.ConvertDBToAPIPool(*p1)})
	emptyPoolsTemplate := test.InsertSampleLdapTemplate(t, ctx, manager, "emptyPoolsTemplate", []*api.AddressPool{user.ConvertDBToAPIPool(*p1)})

	cases := []struct {
		name           string
		templateID     int
		updateTemplate *api.LdapTemplate
		expectFailure  bool
	}{
		{
			name:       "update template address pools - add pools",
			templateID: addPoolsTemplate.ID,
			updateTemplate: &api.LdapTemplate{
				ID:            addPoolsTemplate.ID,
				Name:          addPoolsTemplate.Name,
				Filter:        addPoolsTemplate.Filter,
				MFAType:       string(mfa.CERT),
				IsAdmin:       true,
				InterfaceName: "wg0",
				AddressPools:  []*api.AddressPool{user.ConvertDBToAPIPool(*p1), user.ConvertDBToAPIPool(*p2), user.ConvertDBToAPIPool(*p3)},
			},
		},
		{
			name:       "update template address pools - remove pools",
			templateID: removePoolsTemplate.ID,
			updateTemplate: &api.LdapTemplate{
				ID:            removePoolsTemplate.ID,
				Name:          removePoolsTemplate.Name,
				Filter:        removePoolsTemplate.Filter,
				MFAType:       string(mfa.CERT),
				IsAdmin:       true,
				InterfaceName: "wg0",
				AddressPools:  []*api.AddressPool{user.ConvertDBToAPIPool(*p2)},
			},
		},
		{
			name:       "update template address pools - replace all pools",
			templateID: replacePoolsTemplate.ID,
			updateTemplate: &api.LdapTemplate{
				ID:            replacePoolsTemplate.ID,
				Name:          replacePoolsTemplate.Name,
				Filter:        replacePoolsTemplate.Filter,
				MFAType:       string(mfa.CERT),
				IsAdmin:       true,
				InterfaceName: "wg0",
				AddressPools:  []*api.AddressPool{user.ConvertDBToAPIPool(*p3), user.ConvertDBToAPIPool(*p4)},
			},
		},
		{
			name:       "update template with invalid address pool",
			templateID: invalidPoolTemplate.ID,
			updateTemplate: &api.LdapTemplate{
				ID:            invalidPoolTemplate.ID,
				Name:          invalidPoolTemplate.Name,
				Filter:        invalidPoolTemplate.Filter,
				MFAType:       string(mfa.CERT),
				IsAdmin:       true,
				InterfaceName: "wg0",
				AddressPools:  []*api.AddressPool{{ID: 99999, Name: "nonexistent"}},
			},
			expectFailure: true,
		},
		{
			name:       "update template with empty address pools",
			templateID: emptyPoolsTemplate.ID,
			updateTemplate: &api.LdapTemplate{
				ID:            emptyPoolsTemplate.ID,
				Name:          emptyPoolsTemplate.Name,
				Filter:        emptyPoolsTemplate.Filter,
				MFAType:       string(mfa.CERT),
				IsAdmin:       true,
				InterfaceName: "wg0",
				AddressPools:  []*api.AddressPool{},
			},
			expectFailure: true,
		},
		{
			name:       "update template with overlapping address pools",
			templateID: addPoolsTemplate.ID,
			updateTemplate: &api.LdapTemplate{
				ID:            addPoolsTemplate.ID,
				Name:          addPoolsTemplate.Name,
				Filter:        addPoolsTemplate.Filter,
				MFAType:       string(mfa.CERT),
				IsAdmin:       true,
				InterfaceName: "wg-overlap",
				AddressPools:  overlapPools,
			},
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeCount := test.CountTableRows(t, conn, "ldap_templates")

			err := manager.UpdateLdapTemplate(ctx, tc.templateID, tc.updateTemplate)

			afterCount := test.CountTableRows(t, conn, "ldap_templates")

			if tc.expectFailure {
				require.Error(t, err)
				require.Equal(t, beforeCount, afterCount)
				return
			}

			require.NoError(t, err)
			require.Equal(t, beforeCount, afterCount)

			updatedTemplate, err := manager.GetLdapTemplate(ctx, tc.templateID)
			require.NoError(t, err)
			require.Equal(t, tc.updateTemplate, updatedTemplate)
		})
	}
}

func TestIntegrationDeleteLdapTemplate(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	tmpl := test.InsertSampleLdapTemplate(t, ctx, manager, "template", nil)

	cases := []struct {
		name          string
		templateID    int
		expectFailure bool
	}{
		{
			name:          "delete LDAP template",
			templateID:    tmpl.ID,
			expectFailure: false,
		},
		{
			name:          "delete nonexisting LDAP template",
			templateID:    0,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeCount := test.CountTableRows(t, conn, "ldap_templates")

			err := manager.DeleteLdapTemplate(ctx, tc.templateID)

			afterCount := test.CountTableRows(t, conn, "ldap_templates")

			if tc.expectFailure {
				require.Error(t, err)
				require.Equal(t, beforeCount, afterCount)
				return
			}
			require.NoError(t, err)
			require.Equal(t, beforeCount-1, afterCount)

			templates, err := sqlcq.GetAllLdapTemplates(ctx)
			require.NoError(t, err)
			require.Len(t, templates, 0)
		})
	}
}

func TestIntegrationMFACreateTOTPSecret(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)
	usr := test.InsertSampleUser(t, ctx, q, "user1").ToAPI()

	p1 := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool1"))
	test.InsertSampleDeviceTemplate(t, ctx, sqlcq, usr.ID, []int{p1.ID})

	cases := []struct {
		name          string
		userID        int
		userUpdatedAt time.Time
		expectFailure bool
	}{
		{
			name:          "create TOTP secret for user",
			userID:        usr.ID,
			userUpdatedAt: usr.UpdatedAt,
			expectFailure: false,
		},
		{
			name:          "invalid user ID",
			userID:        -1000,
			expectFailure: true,
		},
		{
			name:          "nonexisting user ID",
			userID:        usr.ID + 1, // there is no user with higher ID than the last added user
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			countBefore := test.CountTableRows(t, conn, "totp")
			secret, err := manager.MFACreateTOTPSecret(ctx, tc.userID)
			countAfter := test.CountTableRows(t, conn, "totp")

			if tc.expectFailure {
				require.Error(t, err)
				require.Equal(t, countBefore, countAfter)
				return
			}
			require.NoError(t, err)
			require.Equal(t, countBefore+1, countAfter)

			// check returned secret with what is in DB
			dbSecretEncrypted, err := sqlcq.GetUserTOTP(ctx, &tc.userID)
			dbSecret := &mfa.TOTPSecret{
				Key: test.OpenString(dbSecretEncrypted),
			}
			require.NoError(t, err)
			require.Equal(t, secret, dbSecret)

			u, err := sqlcq.GetUserByID(ctx, tc.userID)
			require.NoError(t, err)
			require.True(t, u.UpdatedAt.Valid)
			// TODO fix comparing timestamps
			// require.Greater(t, u.UpdatedAt.Time, tc.userUpdatedAt)
		})
	}
}

func TestIntegrationMFAGetTOTPSecret(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	usr1 := test.InsertSampleUser(t, ctx, q, "user1").ToAPI()
	usr2 := test.InsertSampleUser(t, ctx, q, "user2").ToAPI()
	expectedSecretKey := "testtotpsecretkey"
	_, err := sqlcq.InsertUserTOTP(ctx, sqlc.InsertUserTOTPParams{
		KeyEncrypted: test.Seal(expectedSecretKey),
		UserID:       &usr1.ID,
	})
	require.NoError(t, err)

	cases := []struct {
		name          string
		userID        int
		expectFailure bool
	}{
		{
			name:          "get user's TOTP secret",
			userID:        usr1.ID,
			expectFailure: false,
		},
		{
			name:          "invalid user ID",
			userID:        -1000,
			expectFailure: true,
		},
		{
			name:          "nonexisting user ID",
			userID:        usr2.ID + 1, // there is no user with higher ID than the last added user
			expectFailure: true,
		},
		{
			name:          "get nonexisting TOTP secret for user",
			userID:        usr2.ID,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			secret, err := manager.MFAGetTOTPSecret(ctx, tc.userID)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, secret)
			require.Equal(t, expectedSecretKey, secret.Key)
		})
	}
}

func TestIntegrationMFAUpdateTOTPSecret(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)
	usr := test.InsertSampleUser(t, ctx, q, "user1").ToAPI()
	oldSecretKey := "testtotpsecretkey"
	_, err := sqlcq.InsertUserTOTP(ctx, sqlc.InsertUserTOTPParams{
		KeyEncrypted: test.Seal(oldSecretKey),
		UserID:       &usr.ID,
	})
	require.NoError(t, err)

	p1 := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool1"))
	test.InsertSampleDeviceTemplate(t, ctx, sqlcq, usr.ID, []int{p1.ID})

	cases := []struct {
		name          string
		userID        int
		userUpdatedAt time.Time
		expectFailure bool
	}{
		{
			name:          "update TOTP secret for user (with new random secret)",
			userID:        usr.ID,
			userUpdatedAt: usr.UpdatedAt,
			expectFailure: false,
		},
		{
			name:          "invalid user ID",
			userID:        -1000,
			expectFailure: true,
		},
		{
			name:          "nonexisting user ID",
			userID:        usr.ID + 1, // there is no user with higher ID than the last added user
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			countBefore := test.CountTableRows(t, conn, "totp")
			newSecret, err := manager.MFAUpdateTOTPSecret(ctx, tc.userID)
			countAfter := test.CountTableRows(t, conn, "totp")

			if tc.expectFailure {
				require.Error(t, err)
				require.Equal(t, countBefore, countAfter)
				return
			}
			require.NoError(t, err)
			require.Equal(t, countBefore, countAfter)

			// check that newSecret got updated and that new newSecret is correctly set in the DB
			dbSecretEncrypted, err := sqlcq.GetUserTOTP(ctx, &tc.userID)
			dbSecret := &mfa.TOTPSecret{
				Key: test.OpenString(dbSecretEncrypted),
			}
			require.NoError(t, err)
			require.NotNil(t, newSecret)
			require.NotEqual(t, oldSecretKey, newSecret.Key)
			require.Equal(t, newSecret, dbSecret)

			u, err := sqlcq.GetUserByID(ctx, tc.userID)
			require.NoError(t, err)
			require.True(t, u.UpdatedAt.Valid)
			// TODO fix comparing timestamps
			// require.Greater(t, u.UpdatedAt.Time, tc.userUpdatedAt)
		})
	}
}

func TestIntegrationMFACreateCA(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)
	cases := []struct {
		name                   string
		caCert                 []byte
		want                   *mfa.RawCert
		expectFailure          bool
		expectedFailureMessage string
	}{
		{
			name:   "create MFA CA certificate",
			caCert: []byte(mt.CA),
			want: &mfa.RawCert{
				ID:   1,
				Data: []byte(mt.CA),
			},
			expectFailure: false,
		},
		{
			name:                   "insert MFA CA cert with invalid cert block structure",
			caCert:                 []byte("???"), // can't find the "-----BEGIN" start of certificate block
			expectFailure:          true,
			expectedFailureMessage: "failed to parse x509 certificate in PEM format",
		},
		{
			name: "insert MFA CA cert with bad cert data in first cert block",
			caCert: []byte(
				"\n-----BEGIN CERTIFICATE-----\n" +
					base64.StdEncoding.EncodeToString([]byte("???")) +
					"\n-----END CERTIFICATE-----\n"),
			expectFailure:          true,
			expectedFailureMessage: "failed to parse x509 certificate",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			countBefore := test.CountTableRows(t, conn, "mfa_certs")
			err := manager.MFACreateCA(ctx, tc.caCert)
			countAfter := test.CountTableRows(t, conn, "mfa_certs")

			if tc.expectFailure {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.expectedFailureMessage)
				require.Equal(t, countBefore, countAfter)
				return
			}
			require.NoError(t, err)
			require.Equal(t, countBefore+1, countAfter)

			certsArr, err := q.GetMFACerts(ctx, "ca")
			require.NoError(t, err)
			require.Contains(t, certsArr, tc.want)
		})
	}
}

func TestIntegrationMFACreateCRL(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)
	cases := []struct {
		name                   string
		crlCert                []byte
		expectedCert           *mfa.RawCert
		expectFailure          bool
		expectedFailureMessage string
	}{
		{
			name:    "create MFA CA certificate",
			crlCert: []byte(mt.CRL),
			expectedCert: &mfa.RawCert{
				ID:   1,
				Data: []byte(mt.CRL),
			},
			expectFailure: false,
		},
		{
			name:                   "insert invalid MFA CRL cert",
			crlCert:                []byte("???"),
			expectFailure:          true,
			expectedFailureMessage: "failed to parse CRL",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			countBefore := test.CountTableRows(t, conn, "mfa_certs")
			err := manager.MFACreateCRL(ctx, tc.crlCert)
			countAfter := test.CountTableRows(t, conn, "mfa_certs")

			if tc.expectFailure {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.expectedFailureMessage)
				require.Equal(t, countBefore, countAfter)
				return
			}
			require.NoError(t, err)
			require.Equal(t, countBefore+1, countAfter)

			certsArr, err := q.GetMFACerts(ctx, "crl")
			require.NoError(t, err)
			require.Contains(t, certsArr, tc.expectedCert)
		})
	}
}

func TestIntegrationMFAAuthenticate(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, context.Background(), q)

	u := test.InsertSampleUser(t, ctx, q, "user").ToAPI()
	err := q.InsertMFACerts(ctx, []byte(mt.CA), "ca")
	require.NoError(t, err)
	err = q.InsertMFACerts(ctx, []byte(mt.CRL), "crl")
	require.NoError(t, err)

	secret := "F2I74CLERIDNY47A225TVHQXILJROLYX"
	_, err = sqlcq.InsertUserTOTP(ctx, sqlc.InsertUserTOTPParams{
		KeyEncrypted: test.Seal(secret),
		UserID:       &u.ID,
	})
	require.NoError(t, err)
	validCode := dgoogauth.ComputeCode(secret, time.Now().UTC().Unix()/30)

	cases := []struct {
		name          string
		mfaType       mfa.AuthenticatorType
		creds         any
		userID        int
		expectFailure bool
	}{
		{
			name:    "authenticate with CERT type",
			mfaType: mfa.CERT,
			userID:  u.ID,
			creds: &mfa.CertCredential{
				Cert: []byte(mt.ValidCert),
			},
			expectFailure: false,
		},
		{
			name:    "authenticate with TOTP type",
			mfaType: mfa.TOTP,
			userID:  u.ID,
			creds: &mfa.TOTPCredentials{
				Password: fmt.Sprintf("%06d", validCode),
			},
			expectFailure: false,
		},
		{
			name:    "bad MFA",
			mfaType: mfa.TOTP,
			userID:  u.ID,
			creds: &mfa.CertCredential{
				Cert: []byte(mt.ValidCert),
			},
			expectFailure: true,
		},
		{
			name:    "invalid code for TOTP type",
			mfaType: mfa.TOTP,
			userID:  u.ID,
			creds: &mfa.TOTPCredentials{
				Password: "1111111",
			},
			expectFailure: true,
		},
		{
			name:    "invalid certificate for CERT type",
			mfaType: mfa.CERT,
			userID:  u.ID,
			creds: &mfa.CertCredential{
				Cert: []byte(mt.RevokedCert),
			},
			expectFailure: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := manager.MFAAuthenticate(ctx, tc.userID, tc.mfaType, tc.creds)

			if tc.expectFailure {
				require.Error(t, err)
				require.False(t, result)
				return
			}

			require.NoError(t, err)
			require.True(t, result)
		})
	}
}

func TestIntegrationMFAGetAllCAs(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, context.Background(), q)

	err := manager.MFACreateCA(ctx, []byte(mt.CA))
	require.NoError(t, err)
	err = manager.MFACreateCA(ctx, []byte(mt.CA))
	require.NoError(t, err)
	err = manager.MFACreateCRL(ctx, []byte(mt.CRL))
	require.NoError(t, err)

	wantCA, err := user.ParseCA([]byte(mt.CA))
	require.NoError(t, err)

	cas, err := manager.MFAGetAllCAs(ctx)
	require.NoError(t, err)
	require.Len(t, cas.CAs, 2)
	for i, cert := range cas.CAs {
		require.Equal(t, i+1, cert.ID)
		require.Equal(t, wantCA.Subject.CommonName, cert.CN)
		require.Equal(t, wantCA.NotBefore.String(), cert.NotBefore)
		require.Equal(t, wantCA.NotAfter.String(), cert.NotAfter)
	}
}

func TestIntegrationMFAGetAllCRLs(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	err := manager.MFACreateCRL(ctx, []byte(mt.CRL))
	require.NoError(t, err)
	err = manager.MFACreateCRL(ctx, []byte(mt.CRL))
	require.NoError(t, err)
	err = manager.MFACreateCA(ctx, []byte(mt.CA))
	require.NoError(t, err)

	blockCRL, _ := pem.Decode([]byte(mt.CRL))
	wantCRL, err := x509.ParseRevocationList(blockCRL.Bytes)
	require.NoError(t, err)
	var wantSNs []string
	for _, rc := range wantCRL.RevokedCertificateEntries {
		wantSNs = append(wantSNs, rc.SerialNumber.String())
	}

	crls, err := manager.MFAGetAllCRLs(ctx)
	require.NoError(t, err)
	require.Len(t, crls.CRLs, 2)
	for i, crl := range crls.CRLs {
		require.Equal(t, i+1, crl.ID)
		require.Equal(t, wantCRL.Issuer.String(), crl.Issuer)
		require.Equal(t, wantSNs, crl.RevokedCertificates)
	}
}

func userLdapConfigToAPILdapConfig(lc *user.LdapConfig) *api.LdapConfig {
	return &api.LdapConfig{
		Priority:       lc.Priority,
		Host:           lc.Host,
		Port:           lc.Port,
		BaseDN:         lc.BaseDN,
		BindDN:         lc.BindDN,
		BindPW:         test.OpenString(lc.BindPWEncrypted),
		UserListFilter: lc.UserListFilter,
		UsernameAttr:   lc.UsernameAttr,
		UIDAttr:        lc.UIDAttr,
		UseTLS:         lc.UseTLS,
		FQDN:           lc.FQDN,
		CACert:         string(lc.CACert),
		TemplateID:     lc.TemplateID,
	}
}

const tooLongFQDN = "" +
	"MoreThan255CharsMoreThan255CharsMoreThan255CharsMoreThan255CharsMoreThan255Chars" +
	"MoreThan255CharsMoreThan255CharsMoreThan255CharsMoreThan255CharsMoreThan255Chars" +
	"MoreThan255CharsMoreThan255CharsMoreThan255CharsMoreThan255CharsMoreThan255Chars" +
	"MoreThan255CharsMoreThan255CharsMoreThan255CharsMoreThan255CharsMoreThan255Chars"
