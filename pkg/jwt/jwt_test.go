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

package jwt

import (
	"testing"
	"time"

	tkn "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestJWT(t *testing.T) {
	const infinity = time.Hour
	now := time.Now()
	hourAgo := now.Add(-time.Hour)

	cases := []struct {
		name           string
		userName       string
		tokenOverride  string
		claimsOverride *tkn.RegisteredClaims
		expectFailure  bool
	}{
		{
			name:     "valid token for given user name",
			userName: "Bob",
		},
		{
			name:     "valid token for different username",
			userName: "Alice",
		},
		{
			name:          "invalid token content",
			tokenOverride: "bad token content",
			expectFailure: true,
		},
		{
			// this is more a test that the implementation's claims structure and values didn't change, so
			// that later failure tests test failure correctly
			name:     "valid token with claims override",
			userName: "Bob",
			claimsOverride: &tkn.RegisteredClaims{
				ExpiresAt: tkn.NewNumericDate(now.Add(time.Minute)),
				IssuedAt:  tkn.NewNumericDate(now),
				NotBefore: tkn.NewNumericDate(now),
				Subject:   "Bob",
				Issuer:    "urn:entguard:orchestrator",
				Audience:  tkn.ClaimStrings{"urn:entguard:orchestrator:api"},
			},
		},
		{
			name:           "missing claims in token",
			claimsOverride: &tkn.RegisteredClaims{},
			expectFailure:  true,
		},
		{
			name:     "expired token",
			userName: "Bob",
			claimsOverride: &tkn.RegisteredClaims{
				ExpiresAt: tkn.NewNumericDate(hourAgo.Add(time.Minute)),
				IssuedAt:  tkn.NewNumericDate(hourAgo),
				NotBefore: tkn.NewNumericDate(hourAgo),
				Subject:   "Bob",
				Issuer:    "urn:entguard:orchestrator",
				Audience:  tkn.ClaimStrings{"urn:entguard:orchestrator:api"},
			},
			expectFailure: true,
		},
		{
			name:     "bad issuer",
			userName: "Bob",
			claimsOverride: &tkn.RegisteredClaims{
				ExpiresAt: tkn.NewNumericDate(now.Add(time.Minute)),
				IssuedAt:  tkn.NewNumericDate(now),
				NotBefore: tkn.NewNumericDate(now),
				Subject:   "Bob",
				Issuer:    "bad issuer",
				Audience:  tkn.ClaimStrings{"urn:entguard:orchestrator:api"},
			},
			expectFailure: true,
		},
		{
			name:     "bad audience",
			userName: "Bob",
			claimsOverride: &tkn.RegisteredClaims{
				ExpiresAt: tkn.NewNumericDate(now.Add(time.Minute)),
				IssuedAt:  tkn.NewNumericDate(now),
				NotBefore: tkn.NewNumericDate(now),
				Subject:   "Bob",
				Issuer:    "urn:entguard:orchestrator",
				Audience:  tkn.ClaimStrings{"bad audience"},
			},
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider, err := NewTokenProvider(infinity)
			require.NoError(t, err, "failed to create JWT provider")

			token := tc.tokenOverride
			if tc.tokenOverride == "" {
				var creationErr error
				if tc.claimsOverride != nil {
					token, creationErr = provider.createToken(tc.claimsOverride)
					require.NoError(t, creationErr, "failed to create JWT for claims %+v", tc.claimsOverride)
				} else {
					token, creationErr = provider.CreateToken(tc.userName)
					require.NoError(t, creationErr, "failed to create JWT for %s", tc.userName)
				}
			}

			gotUserName, err := provider.ValidateToken(token)

			if tc.expectFailure {
				require.Error(t, err, "expected error in p.Validate(%s)", token)
				return
			}
			require.NoError(t, err, "expected nil error in p.Validate(%s)", token)
			require.Equal(t, tc.userName, gotUserName, "bad user name")
		})
	}
}
