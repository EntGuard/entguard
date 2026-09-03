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

package certs

import (
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMakeClientCert(t *testing.T) {
	p, err := NewProvider("test", "test-unit")
	require.NoError(t, err, "failed to create certificate provider")

	cases := []struct {
		name          string
		unit          string
		serverTag     string
		expectFailure bool
	}{
		{
			name:          "valid unit and server tag",
			unit:          "test",
			serverTag:     "test-server-tag",
			expectFailure: false,
		},
		{
			name:          "empty unit",
			unit:          "",
			serverTag:     "test-server-tag",
			expectFailure: true,
		},
		{
			name:          "empty server tag",
			unit:          "test",
			serverTag:     "",
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cert, key, err := p.MakeClientCert(tc.unit, tc.serverTag)

			if tc.expectFailure {
				require.Error(t, err)
				require.Empty(t, cert)
				require.Empty(t, key)
				return
			}

			require.NoError(t, err)
			require.NotEmpty(t, cert)
			require.NotEmpty(t, key)
		})
	}
}

// TestMakeClientCert_UsesRegisteredPENOID pins the server tag extension of freshly issued
// certificates to the registered PEN arc. The legacy OID is still accepted on read, so a
// regression on the issuing side would otherwise go unnoticed.
func TestMakeClientCert_UsesRegisteredPENOID(t *testing.T) {
	p, err := NewProvider("test", "test-unit")
	require.NoError(t, err, "failed to create certificate provider")

	const serverTag = "test-server-tag"
	certPEM, _, err := p.MakeClientCert("test", serverTag)
	require.NoError(t, err)

	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block, "issued certificate is not valid PEM")

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	var oids []string
	for _, ext := range cert.Extensions {
		oids = append(oids, ext.Id.String())
	}
	require.Contains(t, oids, ServerTagOID.String(), "server tag must use the registered PEN OID")
	require.NotContains(t, oids, legacyServerTagOID.String(), "issued certificates must not use the legacy OID")

	tag, err := GetServerTag(cert)
	require.NoError(t, err)
	require.Equal(t, serverTag, tag)
}
