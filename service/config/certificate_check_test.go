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

package config

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVerifyPublicServerCertErrors(t *testing.T) {

	untrustedCert, _ := generateUntrustedServerCert(t)
	trustedCert, trustedRoots := generateTrustedServerCert(t)

	tests := []struct {
		name        string
		certificate []byte
		want        string
	}{
		{
			name: "missing cert file",
			want: "Failed to read server certificate",
		},
		{
			name: "empty bundle",
			want: "contains no certificates",
		},
		{
			name:        "invalid PEM",
			certificate: []byte("not a PEM certificate"),
			want:        "invalid PEM data",
		},
		{
			name:        "unexpected PEM block",
			certificate: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("key")}),
			want:        "Unexpected PEM block",
		},
		{
			name:        "invalid certificate DER",
			certificate: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("not a certificate")}),
			want:        "parse server certificate",
		},
		{
			name:        "untrusted self-signed certificate",
			certificate: untrustedCert,
			want:        "not issued by a publicly trusted certificate authority",
		},
		{
			name:        "trusted self-signed certificate",
			certificate: trustedCert,
			want:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			certificatePath := filepath.Join(t.TempDir(), "server-cert.pem")
			if tt.name != "missing cert file" {
				if err := os.WriteFile(certificatePath, tt.certificate, 0o600); err != nil {
					t.Fatalf("write certificate fixture: %v", err)
				}
			}
			if tt.name == "trusted self-signed certificate" {
				got := verifyPublicServerCert(certificatePath, func() (*x509.CertPool, error) { return trustedRoots, nil })
				if got != "" {
					t.Fatalf("VerifyPublicServerCert() = %q, want message containing %q", got, tt.want)
				}
			} else {
				got := verifyPublicServerCert(certificatePath, x509.SystemCertPool)
				if !strings.Contains(got, tt.want) {
					t.Fatalf("VerifyPublicServerCert() = %q, want message containing %q", got, tt.want)
				}
			}
		})
	}
}

func generateTrustedServerCert(t *testing.T) ([]byte, *x509.CertPool) {
	now := time.Now()
	certificateDER, privateKey := selfSignedCertificatePEM(now, t)
	caCertificate, err := x509.ParseCertificate(certificateDER)
	if err != nil {
		t.Fatalf("parse test CA certificate: %v", err)
	}

	serverPublicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(11),
		Subject:               pkix.Name{CommonName: "orchestrator.example.test"},
		DNSNames:              []string{"orchestrator.example.test"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCertificate, serverPublicKey, privateKey)
	if err != nil {
		t.Fatalf("create test server certificate: %v", err)
	}

	roots := x509.NewCertPool()
	roots.AddCert(caCertificate)
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})
	return certificatePEM, roots
}

func generateUntrustedServerCert(t *testing.T) ([]byte, *x509.CertPool) {
	certificateDER, _ := selfSignedCertificatePEM(time.Now(), t)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER}), x509.NewCertPool()
}

func selfSignedCertificatePEM(now time.Time, t *testing.T) ([]byte, ed25519.PrivateKey) {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate test key: %v", err)
	}

	certificateTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certificateDER, err := x509.CreateCertificate(
		rand.Reader,
		certificateTemplate,
		certificateTemplate,
		publicKey,
		privateKey,
	)
	if err != nil {
		t.Fatalf("create self-signed test certificate: %v", err)
	}

	return certificateDER, privateKey
}
