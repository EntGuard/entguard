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

package rpc_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/dgryski/dgoogauth"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	sqlc "github.com/entguard/entguard/db"
	"github.com/entguard/entguard/pkg/mfa"
	tm "github.com/entguard/entguard/pkg/mfa/test"
	"github.com/entguard/entguard/pkg/test"
	"github.com/entguard/entguard/service/rpc"
)

func TestGetCredentialsFromHeader(t *testing.T) {
	failure := func(authHeader string) {
		t.Helper()
		_, _, err := rpc.GetCredentialsFromHeader(authHeader)
		require.Error(t, err, fmt.Sprintf("expected non-nil error in getCredentialsFromHeader(%v)", authHeader))
	}
	success := func(authHeader string) {
		t.Helper()
		_, _, err := rpc.GetCredentialsFromHeader(authHeader)
		require.NoError(t, err, fmt.Sprintf("expected nil error in getCredentialsFromHeader(%v)", authHeader))
	}

	const validPrefix = "Basic "
	const invalidPrefix = "Invalid"

	validCreds := base64.StdEncoding.EncodeToString([]byte("username:password"))
	missingName := base64.StdEncoding.EncodeToString([]byte(":password"))
	missingPassword := base64.StdEncoding.EncodeToString([]byte("username:"))
	missingDeleimiter := base64.StdEncoding.EncodeToString([]byte("namepass"))
	empty := base64.StdEncoding.EncodeToString([]byte{})

	success(validPrefix + validCreds)

	failure(invalidPrefix + validCreds)
	failure(validPrefix + missingName)
	failure(validPrefix + missingPassword)
	failure(validPrefix + missingDeleimiter)
	failure(validPrefix + empty)
}

func TestExtractHeader(t *testing.T) {
	failure := func(ctx context.Context, header string) {
		t.Helper()
		_, err := rpc.ExtractHeader(ctx, header)
		require.Error(t, err, fmt.Sprintf("expected non-nil error in extractHeader(%v, %v)", ctx, header))
	}
	success := func(ctx context.Context, header string) {
		t.Helper()
		_, err := rpc.ExtractHeader(ctx, header)
		require.NoError(t, err, fmt.Sprintf("expected nil error in extractHeader(%v, %v)", ctx, header))
	}

	const missingHeader = "missing"
	const data = "data"

	md := metadata.New(map[string]string{rpc.AuthHeaderKey: data})

	ctxWithHeader := metadata.NewIncomingContext(context.Background(), md)
	ctxWithoutHeader := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{}))
	ctxWithTwoHeaders := metadata.NewIncomingContext(context.Background(), metadata.Join(md, md))

	success(ctxWithHeader, rpc.AuthHeaderKey)
	failure(ctxWithHeader, missingHeader)
	failure(ctxWithoutHeader, rpc.AuthHeaderKey)
	failure(ctxWithTwoHeaders, rpc.AuthHeaderKey)
}

func TestIntegrationBasicAuth(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	u := test.InsertSampleUser(t, ctx, q, "user1").ToAPI()

	cases := []struct {
		name               string
		userName, password string
		data               string
		want               *rpc.UserMetadata
		expectFailure      bool
	}{
		{
			name:     "context with valid header",
			userName: u.Username,
			password: test.TestUserPassword,
			want: &rpc.UserMetadata{
				UserID:       u.ID,
				MFAType:      u.MFAType,
				Notification: u.Notification,
			},
			expectFailure: false,
		},
		{
			name:          "wrong user password",
			userName:      u.Username,
			password:      "notuserpassword",
			expectFailure: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// create context with header for grpc client
			creds := fmt.Sprintf("%s:%s", tc.userName, tc.password)
			data := fmt.Sprint("Basic ", base64.StdEncoding.EncodeToString([]byte(creds)))
			md := metadata.New(map[string]string{rpc.AuthHeaderKey: data})
			ctxWithHeader := metadata.NewIncomingContext(ctx, md)

			ctx, err := vpnCServer.BasicAuth(ctxWithHeader)

			if tc.expectFailure {
				require.Error(t, err)
				require.Nil(t, ctx)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, ctx)
			um, ok := rpc.GetUserMetadata(ctx)
			require.True(t, ok)
			require.Equal(t, tc.want, um)
		})
	}
}

func TestIntegrationMfaAuth(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	u := test.InsertSampleUser(t, ctx, q, "user1").ToAPI()
	err := q.InsertMFACerts(ctx, []byte(tm.CA), "ca")
	require.NoError(t, err)
	err = q.InsertMFACerts(ctx, []byte(tm.CRL), "crl")
	require.NoError(t, err)

	secret := "F2I74CLERIDNY47A225TVHQXILJROLYX"
	_, err = sqlcq.InsertUserTOTP(ctx, sqlc.InsertUserTOTPParams{
		KeyEncrypted: test.Seal(secret),
		UserID:       &u.ID,
	})
	require.NoError(t, err)
	validCode := dgoogauth.ComputeCode(secret, time.Now().UTC().Unix()/30)

	meta := &rpc.UserMetadata{
		UserID:       u.ID,
		MFAType:      u.MFAType,
		Notification: u.Notification,
	}

	cases := []struct {
		name          string
		mfaType       mfa.AuthenticatorType
		mfaData       string
		expectFailure bool
	}{
		{
			name:          "authenticate with CERT type",
			mfaType:       mfa.CERT,
			mfaData:       tm.ValidCert,
			expectFailure: false,
		},
		{
			name:          "authenticate with TOTP type",
			mfaType:       mfa.TOTP,
			mfaData:       fmt.Sprintf("%06d", validCode),
			expectFailure: false,
		},
		{
			name:          "bad MFA",
			mfaType:       mfa.TOTP,
			mfaData:       "",
			expectFailure: true,
		},
		{
			name:          "invalid code for TOTP type",
			mfaType:       mfa.TOTP,
			mfaData:       "1111111111111",
			expectFailure: true,
		},
		{
			name:          "invalid certificate for CERT type",
			mfaType:       mfa.CERT,
			mfaData:       tm.RevokedCert,
			expectFailure: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := fmt.Sprintf("%s %s", tc.mfaType, base64.StdEncoding.EncodeToString([]byte(tc.mfaData)))
			md := metadata.New(map[string]string{rpc.MfaHeaderKey: data})
			ctxWithHeader := metadata.NewIncomingContext(context.Background(), md)
			ctx := rpc.SetUserMetadata(ctxWithHeader, meta)

			ctx, err := vpnCServer.MfaAuth(ctx)

			if tc.expectFailure {
				require.Error(t, err)
				require.ErrorContains(t, err, "Unauthenticated")
				require.Nil(t, ctx)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, ctx)
			md, ok := metadata.FromIncomingContext(ctx)
			require.True(t, ok)
			require.Empty(t, md[rpc.MfaHeaderKey])
		})
	}
}
