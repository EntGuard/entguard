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

package mfa

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
)

type CERTAuthenticator struct{}

type CertCredential struct {
	Cert []byte
}

type Validators struct {
	CAs  [][]byte
	CRLs [][]byte
}

func (a CERTAuthenticator) Authenticate(validators, creds interface{}) (bool, error) {
	t, ok := creds.(*CertCredential)
	if !ok {
		return false, errors.New("invalid credential type")
	}

	v, ok := validators.(*Validators)
	if !ok {
		return false, errors.New("invalid validator type")
	}

	if len(v.CAs) == 0 {
		return false, errors.New("no CA certificates present to verify x509 certificate")
	}

	ca, err := findCAForCert(t.Cert, v.CAs)
	if err != nil {
		return false, err
	}

	crls, err := findCRLsForCA(ca, v.CRLs)
	if err != nil {
		return false, err
	}

	err = checkCertAgainstCRL(t.Cert, crls)
	if err != nil {
		return false, err
	}

	return true, nil
}

func findCAForCert(cert []byte, ca [][]byte) ([]byte, error) {
	block, _ := pem.Decode(cert)
	if block == nil {
		return nil, errors.New("failed to parse x509 certificate in PEM format")
	}
	parsedCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse x509 certificate: %w", err)
	}

	for _, rawCA := range ca {
		roots := x509.NewCertPool()
		ok := roots.AppendCertsFromPEM(rawCA)
		if !ok {
			return nil, errors.New("failed to parse root CA certificate in PEM format")
		}

		if _, err := parsedCert.Verify(x509.VerifyOptions{
			Roots: roots,
		}); err == nil {
			return rawCA, nil
		}
	}

	return nil, errors.New("no root CA certificate found that signed provided x509 certificate")
}

func findCRLsForCA(ca []byte, crls [][]byte) ([][]byte, error) {
	block, _ := pem.Decode(ca)
	if block == nil {
		return nil, errors.New("failed to parse CA certificate in PEM format")
	}
	parsedCA, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	var caCRLs [][]byte
	for _, rawCRL := range crls {
		blockCRL, _ := pem.Decode(rawCRL)
		if blockCRL == nil {
			return nil, errors.New("failed to parse CRL in PEM format")
		}
		l, err := x509.ParseRevocationList(blockCRL.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CRL: %w", err)
		}

		if err := l.CheckSignatureFrom(parsedCA); err == nil {
			caCRLs = append(caCRLs, rawCRL)
		}
	}

	return caCRLs, nil
}

func checkCertAgainstCRL(cert []byte, crls [][]byte) error {
	block, _ := pem.Decode(cert)
	if block == nil {
		return errors.New("failed to parse x509 certificate in PEM format")
	}
	parsedCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse x509 certificate: %w", err)
	}

	for _, rawCRL := range crls {
		blockCRL, _ := pem.Decode(rawCRL)
		if blockCRL == nil {
			return errors.New("failed to parse CRL in PEM format")
		}
		l, err := x509.ParseRevocationList(blockCRL.Bytes)
		if err != nil {
			return fmt.Errorf("failed to parse CRL: %w", err)
		}

		for _, rc := range l.RevokedCertificateEntries {
			if rc.SerialNumber.Cmp(parsedCert.SerialNumber) == 0 {
				return errors.New("x509 certificate has been revoked")
			}
		}
	}

	return nil
}
