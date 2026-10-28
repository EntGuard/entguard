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
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

// VerifyPublicServerCert checks the orchestrator's TLS listeners certificate
func VerifyPublicServerCert(certificatePath string) (msg string) {
	return verifyPublicServerCert(certificatePath, x509.SystemCertPool)
}

func verifyPublicServerCert(certificatePath string, rootPool func() (*x509.CertPool, error)) (msg string) {
	certificatePEM, err := os.ReadFile(certificatePath)
	if err != nil {
		return fmt.Sprintf("Failed to read server certificate: %s", err)
	}

	var leaf *x509.Certificate
	intermediates := x509.NewCertPool()

	var block *pem.Block
	rest := certificatePEM

	index := 0
	for len(bytes.TrimSpace(rest)) > 0 {
		block, rest = pem.Decode(rest)
		if block == nil {
			return "Server certificate bundle contains invalid PEM data"
		}
		if block.Type != "CERTIFICATE" {
			return fmt.Sprintf("Unexpected PEM block %q in server certificate bundle", block.Type)
		}

		cert, parseErr := x509.ParseCertificate(block.Bytes)
		if parseErr != nil {
			return fmt.Sprintf("parse server certificate: %s", parseErr)
		}
		if index == 0 {
			leaf = cert
		} else {
			intermediates.AddCert(cert)
		}
		index++
	}
	if leaf == nil {
		return "Server certificate bundle contains no certificates"
	}

	roots, err := rootPool()
	if err != nil {
		return fmt.Sprintf("load system root certificates: %s", err)
	}
	if roots == nil {
		return fmt.Sprint("system root certificate pool is unavailable")
	}

	_, err = leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	if err != nil {
		return fmt.Sprintf("The TLS certificate is not issued by a publicly trusted certificate authority: %s", err)
	}
	return
}
