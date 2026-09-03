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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sqlc "github.com/entguard/entguard/db"
	"github.com/entguard/entguard/pkg/db"
	"github.com/entguard/entguard/pkg/mfa"
	"github.com/entguard/entguard/pkg/test"
	"github.com/entguard/entguard/service/api/v1"
	"github.com/entguard/entguard/service/session"
	"github.com/entguard/entguard/service/user"
)

func TestIntegrationInvalidateOldDeviceSessions(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	threshold := time.Now().Add(-5 * time.Hour)
	expired1 := time.Now().Add(-6 * time.Hour)
	expired2 := time.Now().Add(-7 * time.Hour)
	valid1 := time.Now().Add(-4 * time.Hour)
	valid2 := time.Now().Add(-3 * time.Hour)

	user1 := test.InsertSampleUser(t, ctx, q, "testUser1")
	user2 := test.InsertSampleUser(t, ctx, q, "testUser2")
	addressPool := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool"))
	deviceTemplate1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user1.ID, []int{addressPool.ID})
	deviceTemplate2 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user2.ID, []int{addressPool.ID})

	deviceExpired1, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate1.ID, &expired1)
	deviceValid1, s1 := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate1.ID, &valid1)
	deviceNoSession1 := test.InsertSampleDeviceManual(t, ctx, sqlcq, deviceTemplate1.ID)
	deviceExpired2, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate2.ID, &expired2)
	deviceValid2, s2 := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate2.ID, &valid2)
	deviceNoSession2 := test.InsertSampleDeviceManual(t, ctx, sqlcq, deviceTemplate2.ID)

	n, err := manager.InvalidateOldDevicesSessions(ctx, threshold)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	checkDevice := func(deviceID int, expectedSessionID string, expectedSessionTime time.Time) {
		t.Helper()
		device, err := sqlcq.GetDeviceByID(ctx, deviceID)
		require.NoError(t, err)
		require.Equal(t, expectedSessionID, device.SessionID)

		if expectedSessionTime.IsZero() {
			// device never had a session, last_time_connected should be NULL
			return
		}

		// device has or had a session, validate timestamp
		require.True(t, device.LastTimeConnected.Valid)
		expectedTimeUTC := expectedSessionTime.UTC()
		before := expectedTimeUTC.Add(-1 * time.Second)
		after := expectedTimeUTC.Add(1 * time.Second)
		test.RequireTimeBetween(t, device.LastTimeConnected.Time, before, after)
	}

	checkDevice(deviceValid1.ID, s1, valid1)
	checkDevice(deviceValid2.ID, s2, valid2)
	checkDevice(deviceExpired1.ID, "", expired1)
	checkDevice(deviceExpired2.ID, "", expired2)
	checkDevice(deviceNoSession1.ID, "", time.Time{})
	checkDevice(deviceNoSession2.ID, "", time.Time{})
}

func TestIntegrationGetAllDevicesSessions(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	user1 := test.InsertSampleUser(t, ctx, q, "testUser1")
	user2 := test.InsertSampleUser(t, ctx, q, "testUser2")
	addressPool := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool"))
	deviceTemplate1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user1.ID, []int{addressPool.ID})
	deviceTemplate2 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user2.ID, []int{addressPool.ID})

	_, session1 := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate1.ID, nil)
	require.NotEmpty(t, session1)
	_, session2 := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate1.ID, nil)
	require.NotEmpty(t, session2)
	_, session3 := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate2.ID, nil)
	require.NotEmpty(t, session3)

	device4NoSession := test.InsertSampleDeviceManual(t, ctx, sqlcq, deviceTemplate1.ID)
	device5NoSession := test.InsertSampleDeviceManual(t, ctx, sqlcq, deviceTemplate2.ID)

	sessions, err := manager.GetAllDevicesSessions(ctx)
	require.NoError(t, err)
	require.NotNil(t, sessions)

	require.Len(t, sessions, 3)

	require.Contains(t, sessions, session1)
	require.Contains(t, sessions, session2)
	require.Contains(t, sessions, session3)

	require.NotContains(t, sessions, device4NoSession.SessionID)
	require.NotContains(t, sessions, device5NoSession.SessionID)

	sessionSet := make(map[string]bool)
	for _, s := range sessions {
		require.False(t, sessionSet[s], "duplicate session found: %s", s)
		sessionSet[s] = true
	}
}

func TestIntegrationDeleteInvalidSessionsByDevice(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	user1 := test.InsertSampleUser(t, ctx, q, "user1")
	user2 := test.InsertSampleUser(t, ctx, q, "user2")
	user3 := test.InsertSampleUser(t, ctx, q, "user3")
	user4 := test.InsertSampleUser(t, ctx, q, "user4")
	user5 := test.InsertSampleUser(t, ctx, q, "user5")

	addressPool := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool"))
	deviceTemplate1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user1.ID, []int{addressPool.ID})
	deviceTemplate2 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user2.ID, []int{addressPool.ID})
	deviceTemplate3 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user3.ID, []int{addressPool.ID})
	deviceTemplate4 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user4.ID, []int{addressPool.ID})
	deviceTemplate5 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user5.ID, []int{addressPool.ID})

	d1, s1 := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate1.ID, nil)
	d2, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate2.ID, nil)
	d3, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate3.ID, nil)
	d4, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate4.ID, nil)
	d5, s5 := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate5.ID, nil)

	deviceOld := api.DeviceData{
		Description: "test device",
		PrivateKey:  "privatekey",
		PublicKey:   "publickey",
		Addresses:   []string{"1.1.1.1/24", "1.2.3.4/18"},
	}

	deviceNewPrivateKey := deviceOld
	deviceNewPrivateKey.PrivateKey = "notequalprivatekey"

	deviceNewPublicKey := deviceOld
	deviceNewPublicKey.PublicKey = "notequalpublickey"

	deviceNewAddress := deviceOld
	deviceNewAddress.Addresses = append(deviceNewAddress.Addresses, "10.10.10.1/24")

	deviceNewDescription := deviceOld
	deviceNewDescription.Description = "newdescription"

	cases := []struct {
		name                 string
		deviceID             int
		oldDevice, newDevice *api.DeviceData
		expectedSession      string
		expectedNumDeleted   int
	}{
		{
			name:            "nothing changed - keep",
			deviceID:        d1.ID,
			oldDevice:       &deviceOld,
			newDevice:       &deviceOld,
			expectedSession: s1,
		},
		{
			name:               "private key is updated - delete",
			deviceID:           d2.ID,
			oldDevice:          &deviceOld,
			newDevice:          &deviceNewPrivateKey,
			expectedSession:    "",
			expectedNumDeleted: 1,
		},
		{
			name:               "public key is updated - delete",
			deviceID:           d3.ID,
			oldDevice:          &deviceOld,
			newDevice:          &deviceNewPublicKey,
			expectedSession:    "",
			expectedNumDeleted: 1,
		},
		{
			name:               "addresses are updated - delete",
			deviceID:           d4.ID,
			oldDevice:          &deviceOld,
			newDevice:          &deviceNewAddress,
			expectedSession:    "",
			expectedNumDeleted: 1,
		},
		{
			name:            "description is updated - keep",
			deviceID:        d5.ID,
			oldDevice:       &deviceOld,
			newDevice:       &deviceNewDescription,
			expectedSession: s5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := test.CountSessions(t, conn)

			err := manager.DeleteInvalidSessionsByDevice(ctx, tc.deviceID, tc.oldDevice, tc.newDevice)
			require.NoError(t, err)

			after := test.CountSessions(t, conn)
			require.Equal(t, before-tc.expectedNumDeleted, after)

			device, err := sqlcq.GetDeviceByID(ctx, tc.deviceID)
			require.NoError(t, err)
			require.Equal(t, tc.expectedSession, device.SessionID)
		})
	}
}

func TestIntegrationDeleteInvalidSessionsByUser(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	user1 := test.InsertSampleUser(t, ctx, q, "user1")
	user2 := test.InsertSampleUser(t, ctx, q, "user2")
	user3 := test.InsertSampleUser(t, ctx, q, "user3")
	user4 := test.InsertSampleUser(t, ctx, q, "user4")
	user5 := test.InsertSampleUser(t, ctx, q, "user5")
	user6 := test.InsertSampleUser(t, ctx, q, "user6")

	addressPool := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool"))
	deviceTemplate1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user1.ID, []int{addressPool.ID})
	deviceTemplate2 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user2.ID, []int{addressPool.ID})
	deviceTemplate3 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user3.ID, []int{addressPool.ID})
	deviceTemplate4 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user4.ID, []int{addressPool.ID})
	deviceTemplate5 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user5.ID, []int{addressPool.ID})
	deviceTemplate6 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user6.ID, []int{addressPool.ID})

	d1a, s1a := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate1.ID, nil)
	d1b, s1b := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate1.ID, nil)
	d2a, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate2.ID, nil)
	d2b, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate2.ID, nil)
	d3a, s3a := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate3.ID, nil)
	d3b, s3b := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate3.ID, nil)
	d4a, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate4.ID, nil)
	d4b, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate4.ID, nil)
	d5a, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate5.ID, nil)
	d5b, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate5.ID, nil)
	d6a, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate6.ID, nil)
	d6b, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate6.ID, nil)

	userOld := user.UserModel{
		ID:           user1.ID,
		Name:         "username",
		IsAdmin:      false,
		AuthService:  user.InternalUser,
		MfaAuthType:  mfa.NONE,
		Notification: "ntf",
	}

	userNewName := userOld
	userNewName.ID = user2.ID
	userNewName.Name = "newname"

	userNewIsAdmin := userOld
	userNewIsAdmin.ID = user3.ID
	userNewIsAdmin.IsAdmin = true

	userNewAuthService := userOld
	userNewAuthService.ID = user4.ID
	userNewAuthService.AuthService = user.LDAPUser

	userNewMfaAuthType := userOld
	userNewMfaAuthType.ID = user5.ID
	userNewMfaAuthType.MfaAuthType = mfa.TOTP

	userNewNotification := userOld
	userNewNotification.ID = user6.ID
	userNewNotification.Notification = "new ntf"

	cases := []struct {
		name               string
		oldUser, newUser   *user.UserModel
		deviceIDs          []int
		expectedSessions   []string
		expectedNumDeleted int
	}{
		{
			name:               "nothing changed - keep",
			oldUser:            &userOld,
			newUser:            &userOld,
			deviceIDs:          []int{d1a.ID, d1b.ID},
			expectedSessions:   []string{s1a, s1b},
			expectedNumDeleted: 0,
		},
		{
			name:               "name is updated - delete",
			oldUser:            &userOld,
			newUser:            &userNewName,
			deviceIDs:          []int{d2a.ID, d2b.ID},
			expectedSessions:   []string{"", ""},
			expectedNumDeleted: 2,
		},
		{
			name:               "IsAdmin is updated - keep",
			oldUser:            &userOld,
			newUser:            &userNewIsAdmin,
			deviceIDs:          []int{d3a.ID, d3b.ID},
			expectedSessions:   []string{s3a, s3b},
			expectedNumDeleted: 0,
		},
		{
			name:               "auth service is updated - delete",
			oldUser:            &userOld,
			newUser:            &userNewAuthService,
			deviceIDs:          []int{d4a.ID, d4b.ID},
			expectedSessions:   []string{"", ""},
			expectedNumDeleted: 2,
		},
		{
			name:               "MFA type is updated - delete",
			oldUser:            &userOld,
			newUser:            &userNewMfaAuthType,
			deviceIDs:          []int{d5a.ID, d5b.ID},
			expectedSessions:   []string{"", ""},
			expectedNumDeleted: 2,
		},
		{
			name:               "Notification is updated - delete",
			oldUser:            &userOld,
			newUser:            &userNewNotification,
			deviceIDs:          []int{d6a.ID, d6b.ID},
			expectedSessions:   []string{"", ""},
			expectedNumDeleted: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := test.CountSessions(t, conn)

			err := manager.DeleteInvalidSessionsByUser(ctx, tc.oldUser, tc.newUser)
			require.NoError(t, err)

			after := test.CountSessions(t, conn)
			require.Equal(t, before-tc.expectedNumDeleted, after)

			for i, deviceID := range tc.deviceIDs {
				device, err := sqlcq.GetDeviceByID(ctx, deviceID)
				require.NoError(t, err)
				require.Equal(t, tc.expectedSessions[i], device.SessionID)
			}
		})
	}
}

func TestIntegrationDeleteInvalidSessionsByUserPassword(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	user1 := test.InsertSampleUser(t, ctx, q, "user1")
	userDecoy := test.InsertSampleUser(t, ctx, q, "userDecoy")

	addressPool := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool"))
	deviceTemplate1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user1.ID, []int{addressPool.ID})
	deviceTemplateDecoy := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, userDecoy.ID, []int{addressPool.ID})

	d1a, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate1.ID, nil)
	d1b, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate1.ID, nil)
	_, _ = test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplateDecoy.ID, nil)
	_, _ = test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplateDecoy.ID, nil)

	cases := []struct {
		name               string
		User               *user.UserModel
		deviceIDs          []int
		expectedSessions   []string
		expectedNumDeleted int
	}{
		{
			name:               "password is updated - delete sessions",
			User:               user1,
			deviceIDs:          []int{d1a.ID, d1b.ID},
			expectedSessions:   []string{"", ""},
			expectedNumDeleted: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := test.CountSessions(t, conn)

			// Pretend that function for password change has been called
			// (otherwise we would not call `DeleteInvalidSessionsByUserPassword`)
			err := manager.DeleteInvalidSessionsByUserPassword(ctx, tc.User)
			require.NoError(t, err)

			after := test.CountSessions(t, conn)
			require.Equal(t, before-tc.expectedNumDeleted, after)

			for i, deviceID := range tc.deviceIDs {
				device, err := sqlcq.GetDeviceByID(ctx, deviceID)
				require.NoError(t, err)
				require.Equal(t, tc.expectedSessions[i], device.SessionID)
			}
		})
	}
}

func TestIntegrationDeleteInvalidSessionsByUserTOTP(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	userTOTP := test.InsertSampleUser(t, ctx, q, "userWithTOTP", test.WithMfaType(mfa.TOTP))
	userDecoy := test.InsertSampleUser(t, ctx, q, "anotherUserWithTOTP", test.WithMfaType(mfa.TOTP))
	userCERT := test.InsertSampleUser(t, ctx, q, "userWithCert", test.WithMfaType(mfa.CERT))
	userNONE := test.InsertSampleUser(t, ctx, q, "userWithNone", test.WithMfaType(mfa.NONE))

	addressPool := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool"))
	deviceTemplate1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, userTOTP.ID, []int{addressPool.ID})
	deviceTemplate2 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, userDecoy.ID, []int{addressPool.ID})
	deviceTemplate3 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, userCERT.ID, []int{addressPool.ID})
	deviceTemplate4 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, userNONE.ID, []int{addressPool.ID})

	d1a, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate1.ID, nil)
	d1b, _ := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate1.ID, nil)
	_, _ = test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate2.ID, nil)
	_, _ = test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate2.ID, nil)
	d3a, s3a := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate3.ID, nil)
	d3b, s3b := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate3.ID, nil)
	d4a, s4a := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate4.ID, nil)
	d4b, s4b := test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate4.ID, nil)

	cases := []struct {
		name               string
		User               *user.UserModel
		deviceIDs          []int
		expectedSessions   []string
		expectedNumDeleted int
	}{
		{
			name:               "TOTP secret changed for user with TOTP - delete",
			User:               userTOTP,
			deviceIDs:          []int{d1a.ID, d1b.ID},
			expectedSessions:   []string{"", ""},
			expectedNumDeleted: 2,
		},
		{
			name:               "TOTP secret changed for user with CERT - keep",
			User:               userCERT,
			deviceIDs:          []int{d3a.ID, d3b.ID},
			expectedSessions:   []string{s3a, s3b},
			expectedNumDeleted: 0,
		},
		{
			name:               "TOTP secret changed for user with none MFA - keep",
			User:               userNONE,
			deviceIDs:          []int{d4a.ID, d4b.ID},
			expectedSessions:   []string{s4a, s4b},
			expectedNumDeleted: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := test.CountSessions(t, conn)

			// Pretend that function for regenerating TOTP secret has been called
			// (otherwise we would not call `DeleteInvalidSessionsByUserTOTP`)
			err := manager.DeleteInvalidSessionsByUserTOTP(ctx, tc.User)
			require.NoError(t, err)

			after := test.CountSessions(t, conn)
			require.Equal(t, before-tc.expectedNumDeleted, after)

			for i, deviceID := range tc.deviceIDs {
				device, err := sqlcq.GetDeviceByID(ctx, deviceID)
				require.NoError(t, err)
				require.Equal(t, tc.expectedSessions[i], device.SessionID)
			}
		})
	}
}

func TestIntegrationDeleteInvalidSessionsByMFACert(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	conn := q.GetDbConnection(ctx)

	// preconditions
	userCERT1 := test.InsertSampleUser(t, ctx, q, "userWithCert", test.WithMfaType(mfa.CERT))
	userCERT2 := test.InsertSampleUser(t, ctx, q, "anotherUserWithCert", test.WithMfaType(mfa.CERT))
	userTOTP := test.InsertSampleUser(t, ctx, q, "userWithTOTP", test.WithMfaType(mfa.TOTP))
	userNONE := test.InsertSampleUser(t, ctx, q, "userWithNone", test.WithMfaType(mfa.NONE))

	addressPool := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool"))
	deviceTemplate1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, userCERT1.ID, []int{addressPool.ID})
	deviceTemplate2 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, userCERT2.ID, []int{addressPool.ID})
	deviceTemplate3 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, userTOTP.ID, []int{addressPool.ID})
	deviceTemplate4 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, userNONE.ID, []int{addressPool.ID})

	d1a := test.InsertSampleDeviceManual(t, ctx, sqlcq, deviceTemplate1.ID)
	d1b := test.InsertSampleDeviceManual(t, ctx, sqlcq, deviceTemplate1.ID)
	d2a := test.InsertSampleDeviceManual(t, ctx, sqlcq, deviceTemplate2.ID)
	d2b := test.InsertSampleDeviceManual(t, ctx, sqlcq, deviceTemplate2.ID)
	d3a := test.InsertSampleDeviceManual(t, ctx, sqlcq, deviceTemplate3.ID)
	d3b := test.InsertSampleDeviceManual(t, ctx, sqlcq, deviceTemplate3.ID)
	d4a := test.InsertSampleDeviceManual(t, ctx, sqlcq, deviceTemplate4.ID)
	d4b := test.InsertSampleDeviceManual(t, ctx, sqlcq, deviceTemplate4.ID)

	cases := []struct {
		name                           string
		operation                      string
		certType                       string
		expectedDevicesWithSessions    []*sqlc.Device
		expectedDevicesWithoutSessions []*sqlc.Device
	}{
		{
			name:                           "CA inserted - keep",
			certType:                       mfa.CertTypeCA,
			operation:                      user.InsertCert,
			expectedDevicesWithSessions:    []*sqlc.Device{d1a, d1b, d2a, d2b, d3a, d3b, d4a, d4b},
			expectedDevicesWithoutSessions: []*sqlc.Device{},
		},
		{
			name:                           "CA deleted - delete, but only for users with CERT",
			certType:                       mfa.CertTypeCA,
			operation:                      user.DeleteCert,
			expectedDevicesWithSessions:    []*sqlc.Device{d3a, d3b, d4a, d4b},
			expectedDevicesWithoutSessions: []*sqlc.Device{d1a, d1b, d2a, d2b},
		},
		{
			name:                           "CRL inserted - delete, but only for users with CERT",
			certType:                       mfa.CertTypeCRL,
			operation:                      user.InsertCert,
			expectedDevicesWithSessions:    []*sqlc.Device{d3a, d3b, d4a, d4b},
			expectedDevicesWithoutSessions: []*sqlc.Device{d1a, d1b, d2a, d2b},
		},
		{
			name:                           "CRL deleted - keep",
			certType:                       mfa.CertTypeCRL,
			operation:                      user.DeleteCert,
			expectedDevicesWithSessions:    []*sqlc.Device{d1a, d1b, d2a, d2b, d3a, d3b, d4a, d4b},
			expectedDevicesWithoutSessions: []*sqlc.Device{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sessions := make(map[int]string)
			allExpectedDevices := []*sqlc.Device{}
			allExpectedDevices = append(allExpectedDevices, tc.expectedDevicesWithSessions...)
			allExpectedDevices = append(allExpectedDevices, tc.expectedDevicesWithoutSessions...)
			for _, d := range allExpectedDevices {
				s, err := session.GenerateSessionID()
				require.NoError(t, err)

				_, err = sqlcq.UpdateDeviceSessionAndLastTimeConnected(ctx, sqlc.UpdateDeviceSessionAndLastTimeConnectedParams{
					ID:                d.ID,
					SessionID:         s,
					LastTimeConnected: db.NewPgTimestamp(time.Now()),
				})
				require.NoError(t, err)

				sessions[d.ID] = s
			}

			before := test.CountSessions(t, conn)

			// Pretend that certicates have been inserted/deleted
			// (otherwise we would not call `DeleteInvalidSessionsByMFACert`)
			err := manager.DeleteInvalidSessionsByMFACert(ctx, tc.operation, tc.certType)
			require.NoError(t, err)

			after := test.CountSessions(t, conn)
			require.Equal(t, before-len(tc.expectedDevicesWithoutSessions), after)

			for _, d := range tc.expectedDevicesWithSessions {
				device, err := sqlcq.GetDeviceByID(ctx, d.ID)
				require.NoError(t, err)
				require.Equal(t, sessions[d.ID], device.SessionID)
			}
			for _, d := range tc.expectedDevicesWithoutSessions {
				device, err := sqlcq.GetDeviceByID(ctx, d.ID)
				require.NoError(t, err)
				require.Equal(t, "", device.SessionID)
			}
		})
	}
}
