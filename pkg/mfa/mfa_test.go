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
	"fmt"
	"testing"
	"time"

	"github.com/dgryski/dgoogauth"
	"github.com/stretchr/testify/require"

	"github.com/entguard/entguard/pkg/mfa"
	"github.com/entguard/entguard/pkg/mfa/test"
)

func TestAuthenticateWithCert(t *testing.T) {
	a := mfa.CERTAuthenticator{}

	failure := func(cert string) {
		t.Helper()
		ok, _ := a.Authenticate(&mfa.Validators{
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
		ok, err := a.Authenticate(&mfa.Validators{
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

	failure := func(secret, password string) {
		t.Helper()
		ok, _ := a.Authenticate(
			&mfa.TOTPSecret{Key: secret},
			&mfa.TOTPCredentials{Password: password},
		)
		if ok {
			t.Fatalf("expected first returned value to be false in TOTP a.Authenticate, password: %s", password)
		}
	}

	success := func(secret, password string) {
		t.Helper()
		ok, err := a.Authenticate(
			&mfa.TOTPSecret{Key: secret},
			&mfa.TOTPCredentials{Password: password},
		)
		if !ok || err != nil {
			t.Fatalf("expected first returned value to be true and nil error in TOTP a.Authenticate: %v, code: %s", err, password)
		}
	}

	someCode := 123456
	failure(secret, fmt.Sprintf("%06d", someCode))

	validCode := dgoogauth.ComputeCode(secret, time.Now().UTC().Unix()/30)
	success(secret, fmt.Sprintf("%06d", validCode))
}

func TestCreateUserSecrets(t *testing.T) {
	a := mfa.TOTPAuthenticator{}

	totpSecret, err := a.CreateUserSecrets()
	require.NoError(t, err)

	require.IsType(t, &mfa.TOTPSecret{}, totpSecret)
	require.NotNil(t, totpSecret)
	require.NotEmpty(t, totpSecret.Key)
}
