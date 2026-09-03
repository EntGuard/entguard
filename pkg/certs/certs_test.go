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

package certs

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetServerTag(t *testing.T) {
	validTagStr := "my-server-tag"
	validTagValue, err := asn1.Marshal(asn1.RawValue{
		Class: asn1.ClassUniversal,
		Tag:   asn1.TagUTF8String,
		Bytes: []byte(validTagStr),
	})
	require.NoError(t, err)

	otherOID := asn1.ObjectIdentifier{1, 2, 3, 4}
	otherValue, err := asn1.Marshal(asn1.RawValue{
		Class: asn1.ClassUniversal,
		Tag:   asn1.TagUTF8String,
		Bytes: []byte("other"),
	})
	require.NoError(t, err)

	wrongTagTypeValue, err := asn1.Marshal(asn1.RawValue{
		Class: asn1.ClassUniversal,
		Tag:   asn1.TagPrintableString,
		Bytes: []byte(validTagStr),
	})
	require.NoError(t, err)

	cases := []struct {
		name        string
		cert        *x509.Certificate
		expectedTag string
		expectError bool
	}{
		{
			name: "valid extension present",
			cert: &x509.Certificate{
				Extensions: []pkix.Extension{
					{Id: ServerTagOID, Value: validTagValue},
				},
			},
			expectedTag: validTagStr,
		},
		{
			name: "correct extension among multiple extensions",
			cert: &x509.Certificate{
				Extensions: []pkix.Extension{
					{Id: otherOID, Value: otherValue},
					{Id: ServerTagOID, Value: validTagValue},
				},
			},
			expectedTag: validTagStr,
		},
		{
			name: "legacy OID extension present",
			cert: &x509.Certificate{
				Extensions: []pkix.Extension{
					{Id: legacyServerTagOID, Value: validTagValue},
				},
			},
			expectedTag: validTagStr,
		},
		{
			name: "extension not present",
			cert: &x509.Certificate{
				Extensions: []pkix.Extension{
					{Id: otherOID, Value: otherValue},
				},
			},
			expectError: true,
		},
		{
			name:        "no extensions",
			cert:        &x509.Certificate{},
			expectError: true,
		},
		{
			name: "extension with invalid ASN1 value",
			cert: &x509.Certificate{
				Extensions: []pkix.Extension{
					{Id: ServerTagOID, Value: []byte{0xFF, 0xFF}},
				},
			},
			expectError: true,
		},
		{
			name: "extension with wrong ASN1 type",
			cert: &x509.Certificate{
				Extensions: []pkix.Extension{
					{Id: ServerTagOID, Value: wrongTagTypeValue},
				},
			},
			expectError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tag, err := GetServerTag(tc.cert)
			if tc.expectError {
				require.Error(t, err)
				require.Empty(t, tag)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedTag, tag)
		})
	}
}
