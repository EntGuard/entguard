/*
 * Copyright 2020 PANTHEON.tech s.r.o.
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
	"crypto"
	"crypto/x509"
	"errors"
	"fmt"
)

// Provider operates certificates.
type Provider struct {
	CACert *x509.Certificate
	CAKey  crypto.PrivateKey

	Orgz string
}

// NewProvider returns Provider with initialized rootCA certificate.
func NewProvider(organization, organizationalUnit string) (*Provider, error) {
	if organization == "" {
		return nil, errors.New("organization cannot be empty")
	}
	if organizationalUnit == "" {
		return nil, errors.New("organizational unit cannot be empty")
	}

	cert, key, err := newRootCA(organization, organizationalUnit)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new RootCA: %v", err)
	}

	m := &Provider{
		CACert: cert,
		CAKey:  key,
		Orgz:   organization,
	}

	return m, nil
}

// MakeClientCert returns a TLS key pair for client.
func (m *Provider) MakeClientCert(orgzUnit, serverTag string) (cert []byte, key []byte, e error) {
	if orgzUnit == "" {
		return nil, nil, errors.New("organizational unit cannot be empty")
	}
	if serverTag == "" {
		return nil, nil, errors.New("server tag cannot be empty")
	}

	cert, key, e = newClientCert(m.Orgz, orgzUnit, serverTag, m.CACert, m.CAKey)
	if e != nil {
		return nil, nil, fmt.Errorf("could not create client certificates: %v", e)
	}
	return
}
