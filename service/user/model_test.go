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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/entguard/entguard/pkg/mfa"
	"github.com/entguard/entguard/pkg/test"
	"github.com/entguard/entguard/service/api/v1"
	"github.com/entguard/entguard/service/user"
)

func TestNewUserModelWithName(t *testing.T) {
	cases := []struct {
		name          string
		userName      string
		expectFailure bool
	}{
		{
			name:     "standard name",
			userName: "John",
		},
		{
			name:     "ascii special characters",
			userName: stringWithASCIISpecialCharacters,
		},
		{
			name:     "unicode",
			userName: stringWithUnicode,
		},
		{
			name:          "invalid name",
			userName:      "",
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			userWant := &user.UserModel{
				Name:        tc.userName,
				AuthService: user.InternalUser,
				MfaAuthType: mfa.NONE,
			}

			userGot, err := user.NewUserModelWithName(tc.userName)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, userWant, userGot)
		})
	}
}

func TestNewUserModelWithID(t *testing.T) {
	cases := []struct {
		name          string
		userID        int
		expectFailure bool
	}{
		{
			name:   "valid id 1",
			userID: 1,
		},
		{
			name:   "valid id 2",
			userID: 2,
		},
		{
			name:   "big id",
			userID: 12345,
		},
		{
			name:          "invalid id",
			userID:        0,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			userWant := &user.UserModel{
				ID:          tc.userID,
				AuthService: user.InternalUser,
				MfaAuthType: mfa.NONE,
			}

			userGot, err := user.NewUserModelWithID(tc.userID)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, userWant, userGot)
		})
	}
}

func TestIsInternal(t *testing.T) {
	user1 := test.NewSampleUser(t, "user1")
	err := user1.SetAuthType(user.InternalUser)
	require.NoError(t, err)

	user2 := test.NewSampleUser(t, "user2")
	err = user2.SetAuthType(user.LDAPUser)
	require.NoError(t, err)

	cases := []struct {
		name string
		user *user.UserModel
		want bool
	}{
		{
			name: "internal user",
			user: user1,
			want: true,
		},
		{
			name: "LDAP user",
			user: user2,
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.user.IsInternal()
			require.Equal(t, tc.want, got)
		})
	}
}

func TestIsNotInternal(t *testing.T) {
	user1 := test.NewSampleUser(t, "user1")
	err := user1.SetAuthType(user.InternalUser)
	require.NoError(t, err)

	user2 := test.NewSampleUser(t, "user2")
	err = user2.SetAuthType(user.LDAPUser)
	require.NoError(t, err)

	cases := []struct {
		name string
		user *user.UserModel
		want bool
	}{
		{
			name: "internal user",
			user: user1,
			want: false,
		},
		{
			name: "LDAP user",
			user: user2,
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.user.IsNotInternal()
			require.Equal(t, tc.want, got)
		})
	}
}

func TestIsLDAP(t *testing.T) {
	user1 := test.NewSampleUser(t, "user1")
	err := user1.SetAuthType(user.InternalUser)
	require.NoError(t, err)

	user2 := test.NewSampleUser(t, "user2")
	err = user2.SetAuthType(user.LDAPUser)
	require.NoError(t, err)

	cases := []struct {
		name string
		user *user.UserModel
		want bool
	}{
		{
			name: "internal user",
			user: user1,
			want: false,
		},
		{
			name: "LDAP user",
			user: user2,
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.user.IsLDAP()
			require.Equal(t, tc.want, got)
		})
	}
}

func TestMarkAsLDAP(t *testing.T) {
	user1 := test.NewSampleUser(t, "user1")
	err := user1.SetAuthType(user.InternalUser)
	require.NoError(t, err)

	user2 := test.NewSampleUser(t, "user2")
	err = user2.SetAuthType(user.LDAPUser)
	require.NoError(t, err)

	cases := []struct {
		name string
		user *user.UserModel
	}{
		{
			name: "internal user",
			user: user1,
		},
		{
			name: "LDAP user",
			user: user2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.user.MarkAsLDAP()
			require.Equal(t, user.LDAPUser, tc.user.AuthService)
		})
	}
}

func TestSetID(t *testing.T) {
	u, err := user.NewUserModelWithName("test")
	require.NoError(t, err)

	failure := func(id int) {
		t.Helper()
		err := u.SetID(id)
		require.Error(t, err)
	}
	success := func(id int) {
		t.Helper()
		err := u.SetID(id)
		require.NoError(t, err)
	}

	failure(-1)
	failure(0)
	success(1)
}

func TestSetName(t *testing.T) {
	u, err := user.NewUserModelWithName("test")
	require.NoError(t, err)

	failure := func(username string) {
		t.Helper()
		err := u.SetName(username)
		require.Error(t, err)
	}
	success := func(username string) {
		t.Helper()
		err := u.SetName(username)
		require.NoError(t, err)
	}

	failure("")
	// len("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXY") == 51
	failure("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXY")

	success("foo")
	// len("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWX") == 50
	success("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWX")
}

func TestSetPassword(t *testing.T) {
	u, err := user.NewUserModelWithName("test")
	require.NoError(t, err)

	cases := []struct {
		name          string
		password      string
		expectFailure bool
	}{
		{
			name:          "success",
			password:      "Pa$$w0rd",
			expectFailure: false,
		},
		{
			name:          "empty password",
			password:      "",
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// SetPassword is a wrapper over SetPasswordWithHashingCost with reasonable cost (for production).
			// It would take too long for unit test, so here we instead call SetPasswordWithHashingCost directly.
			err := u.SetPasswordWithHashingCost(tc.password, bcrypt.MinCost)
			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestSetPassword_HashingCostSafety(t *testing.T) {
	u, err := user.NewUserModelWithName("test")
	require.NoError(t, err)

	t1 := time.Now()
	err = u.SetPassword("Pa$$w0rd")
	t2 := time.Now()
	require.NoError(t, err)
	duration := t2.Sub(t1)

	// Verify that production uses safe cost.
	// Recommended value is at least 250 ms, but not too long to annoy users.
	// Here we give it considerable buffer to minimize random test failures
	require.GreaterOrEqual(t, duration, 150*time.Millisecond)
	require.LessOrEqual(t, duration, 5000*time.Millisecond)
}

func TestSetAdmin(t *testing.T) {
	user1 := test.NewSampleUser(t, "user1")
	user1.IsAdmin = false

	user2 := test.NewSampleUser(t, "user2")
	user2.IsAdmin = false

	user3 := test.NewSampleUser(t, "user3")
	user3.IsAdmin = true

	user4 := test.NewSampleUser(t, "user4")
	user4.IsAdmin = true

	cases := []struct {
		name          string
		user          *user.UserModel
		newAdminState bool
	}{
		{
			name:          "not admin to not admin",
			user:          user1,
			newAdminState: false,
		},
		{
			name:          "not admin to admin",
			user:          user2,
			newAdminState: true,
		},
		{
			name:          "admin to not admin",
			user:          user3,
			newAdminState: false,
		},
		{
			name:          "admin to admin",
			user:          user4,
			newAdminState: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.user.SetAdmin(tc.newAdminState)
			require.Equal(t, tc.newAdminState, tc.user.IsAdmin)
		})
	}
}

func TestSetMFAType(t *testing.T) {
	cases := []struct {
		name            string
		mfaType         string
		expectedMFAType string
	}{
		{
			name:            "empty string",
			mfaType:         "",
			expectedMFAType: mfa.NONE,
		},
		{
			name:            "none",
			mfaType:         mfa.NONE,
			expectedMFAType: mfa.NONE,
		},
		{
			name:            "totp",
			mfaType:         mfa.TOTP,
			expectedMFAType: mfa.TOTP,
		},
		{
			name:            "cert",
			mfaType:         mfa.CERT,
			expectedMFAType: mfa.CERT,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := test.NewSampleUser(t, "user")
			u.SetMFAType(tc.mfaType)
			require.Equal(t, tc.expectedMFAType, u.MfaAuthType)
		})
	}
}

func TestSetNotification(t *testing.T) {
	cases := []struct {
		name          string
		notification  string
		expectFailure bool
	}{
		{
			name:         "empty string",
			notification: "",
		},
		{
			name:         "nice notification including spaces",
			notification: "nice notification including spaces",
		},
		{
			name:         "ascii special characters",
			notification: stringWithASCIISpecialCharacters,
		},
		{
			name:         "unicode",
			notification: stringWithUnicode,
		},
		{
			name:          "too long",
			notification:  tooLongNotification,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := test.NewSampleUser(t, "user")
			err := u.SetNotification(tc.notification)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.notification, u.Notification)
		})
	}
}

func TestComparePasswords(t *testing.T) {
	u, err := user.NewUserModelWithName("test")
	require.NoError(t, err)

	cases := []struct {
		name              string
		storedPassword    string
		attemptedPassword string
		expectEqual       bool
	}{
		{
			name:              "correct password",
			storedPassword:    "Pa$$w0rd",
			attemptedPassword: "Pa$$w0rd",
			expectEqual:       true,
		},
		{
			name:              "wrong password",
			storedPassword:    "Pa$$w0rd",
			attemptedPassword: "password",
			expectEqual:       false,
		},
		{
			name:              "empty password",
			storedPassword:    "Pa$$w0rd",
			attemptedPassword: "",
			expectEqual:       false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := u.SetPasswordWithHashingCost(tc.storedPassword, bcrypt.MinCost)
			require.NoError(t, err)
			isEqual := u.ComparePasswords(tc.attemptedPassword)
			require.Equal(t, tc.expectEqual, isEqual)
		})
	}
}

func TestToAPI(t *testing.T) {
	mockTime := time.Now()

	cases := []struct {
		name string
		user *user.UserModel
		want api.User
	}{
		{
			name: "standard values",
			user: &user.UserModel{
				ID:           1,
				Name:         "user1",
				PasswordHash: "mockHash1",
				IsAdmin:      true,
				AuthService:  user.LDAPUser,
				MfaAuthType:  mfa.TOTP,
				Notification: "sampleNotification",
				UpdatedAt:    mockTime,
			},
			want: api.User{
				ID:           1,
				Username:     "user1",
				IsAdmin:      true,
				MFAType:      mfa.TOTP,
				AuthType:     user.LDAPUser,
				Notification: "sampleNotification",
				UpdatedAt:    mockTime,
			},
		},
		{
			name: "unusual values",
			user: &user.UserModel{
				ID:           12345,
				Name:         stringWithUnicode,
				PasswordHash: "mockHash2",
				IsAdmin:      false,
				AuthService:  user.InternalUser,
				MfaAuthType:  mfa.NONE,
				Notification: "",
				UpdatedAt:    mockTime.Add(5 * time.Second),
			},
			want: api.User{
				ID:           12345,
				Username:     stringWithUnicode,
				IsAdmin:      false,
				MFAType:      mfa.NONE,
				AuthType:     user.InternalUser,
				Notification: "",
				UpdatedAt:    mockTime.Add(5 * time.Second),
			},
		},
		{
			name: "unusual values 2",
			user: &user.UserModel{
				ID:           23456,
				Name:         stringWithASCIISpecialCharacters,
				PasswordHash: "mockHash3",
				IsAdmin:      true,
				AuthService:  user.LDAPUser,
				MfaAuthType:  mfa.CERT,
				Notification: stringWithASCIISpecialCharacters,
				UpdatedAt:    mockTime.Add(-5 * time.Second),
			},
			want: api.User{
				ID:           23456,
				Username:     stringWithASCIISpecialCharacters,
				IsAdmin:      true,
				MFAType:      mfa.CERT,
				AuthType:     user.LDAPUser,
				Notification: stringWithASCIISpecialCharacters,
				UpdatedAt:    mockTime.Add(-5 * time.Second),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.user.ToAPI()
			require.Equal(t, tc.want, got)
		})
	}
}

func TestSetAuthType(t *testing.T) {
	cases := []struct {
		name             string
		authType         string
		expectedAuthType string
		expectFailure    bool
	}{
		{
			name:             "empty string",
			authType:         "",
			expectedAuthType: "",
			expectFailure:    true,
		},
		{
			name:             "non-existent auth type",
			authType:         "test-auth-type",
			expectedAuthType: "",
			expectFailure:    true,
		},
		{
			name:             "internal user",
			authType:         user.InternalUser,
			expectedAuthType: user.InternalUser,
		},
		{
			name:             "LDAP user",
			authType:         user.LDAPUser,
			expectedAuthType: user.LDAPUser,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := test.NewSampleUser(t, "user")
			err := u.SetAuthType(tc.authType)
			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedAuthType, u.AuthService)
		})
	}
}

const stringWithASCIISpecialCharacters = "\\asd ASD123/-.?!)(][}{><\"'`&"
const stringWithUnicode = "íýĺŕľňÁŔŘĎČäÄôÔůŮßᴙαΓ张伟"
const tooLongNotification = "MoreThan75CharsMoreThan75CharsMoreThan75CharsMoreThan75CharsMoreThan75CharsMoreThan75Chars"
