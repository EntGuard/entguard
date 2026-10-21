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

package mfa_test

import (
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"

	"github.com/entguard/entguard/pkg/mfa"
	"github.com/entguard/entguard/pkg/mfa/test"
)

func TestAuthenticateWithCert(t *testing.T) {
	a := mfa.CERTAuthenticator{}

	failure := func(cert string) {
		t.Helper()
		ok, _ := a.Authenticate(&mfa.CertValidators{
			CAs:  [][]byte{[]byte(test.CA)},
			CRLs: [][]byte{[]byte(test.CRL)},
		}, &mfa.CertCredential{
			Cert: []byte(cert),
		})
		if ok {
			t.Fatal("expected first returned value to be false in CERT a.Authenticate")
		}
	}

	success := func(cert string) {
		t.Helper()
		ok, err := a.Authenticate(&mfa.CertValidators{
			CAs:  [][]byte{[]byte(test.CA)},
			CRLs: [][]byte{[]byte(test.CRL)},
		}, &mfa.CertCredential{
			Cert: []byte(cert),
		})
		if !ok || err != nil {
			t.Fatalf("expected first returned value to be true and nil error in CERT a.Authenticate: %v", err)
		}
	}

	failure(test.RevokedCert)
	success(test.ValidCert)
}

func TestAuthenticateWithTOTP(t *testing.T) {
	a := mfa.TOTPAuthenticator{}
	secret := "F2I74CLERIDNY47A225TVHQXILJROLYX"

	currentTime := time.Now().UTC()
	currentCounter := mfa.TimeToCounter(currentTime)

	currentCode, err := totp.GenerateCode(secret, currentTime)
	require.NoError(t, err)
	validEarlyCode, err := totp.GenerateCode(secret, currentTime.Add(-1*mfa.Period*time.Second))
	require.NoError(t, err)
	validLateCode, err := totp.GenerateCode(secret, currentTime.Add(1*mfa.Period*time.Second))
	require.NoError(t, err)
	tooEarlyCode, err := totp.GenerateCode(secret, currentTime.Add(-2*mfa.Period*time.Second))
	require.NoError(t, err)
	tooLateCode, err := totp.GenerateCode(secret, currentTime.Add(2*mfa.Period*time.Second))
	require.NoError(t, err)

	cases := []struct {
		name                string
		secret              string
		lastUsedCounter     int64
		password            string
		expectedCounter     int64
		expectAuthenticated bool
		expectReplay        bool
	}{
		{
			name:                "valid current code",
			secret:              secret,
			lastUsedCounter:     0,
			password:            currentCode,
			expectedCounter:     currentCounter,
			expectAuthenticated: true,
		},
		{
			name:                "valid early code",
			secret:              secret,
			lastUsedCounter:     0,
			password:            validEarlyCode,
			expectedCounter:     currentCounter - 1,
			expectAuthenticated: true,
		},
		{
			name:                "valid late code",
			secret:              secret,
			lastUsedCounter:     0,
			password:            validLateCode,
			expectedCounter:     currentCounter + 1,
			expectAuthenticated: true,
		},
		{
			name:                "invalid code - too early",
			secret:              secret,
			lastUsedCounter:     0,
			password:            tooEarlyCode,
			expectAuthenticated: false,
		},
		{
			name:                "invalid code - too late",
			secret:              secret,
			lastUsedCounter:     0,
			password:            tooLateCode,
			expectAuthenticated: false,
		},
		{
			name:                "valid new unique code (no replay)",
			secret:              secret,
			lastUsedCounter:     currentCounter - 1,
			password:            currentCode,
			expectedCounter:     currentCounter,
			expectAuthenticated: true,
		},
		{
			name:                "invalid - code replayed",
			secret:              secret,
			lastUsedCounter:     currentCounter,
			password:            currentCode,
			expectedCounter:     currentCounter,
			expectAuthenticated: false,
			expectReplay:        true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, gotCounter, err := a.Authenticate(
				&mfa.TOTPValidators{
					Key:             tc.secret,
					LastUsedCounter: tc.lastUsedCounter,
				},
				&mfa.TOTPCredentials{Password: tc.password},
			)
			if !tc.expectAuthenticated {
				require.False(t, ok)
				if tc.expectReplay {
					require.ErrorIs(t, err, mfa.ErrTOTPReplay)
				} else {
					require.NoError(t, err)
				}
				return
			}
			require.NoError(t, err)
			require.True(t, ok)
			require.Equal(t, tc.expectedCounter, gotCounter)
		})
	}
}

func TestCreateTOTPKey(t *testing.T) {
	a := mfa.TOTPAuthenticator{}

	key, err := a.CreateTOTPKey()
	require.NoError(t, err)
	require.NotEmpty(t, key)
}
