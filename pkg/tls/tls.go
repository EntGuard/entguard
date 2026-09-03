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

package tls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/entguard/entguard/pkg/models"
	config "github.com/entguard/entguard/pkg/server-config"
)

// prepareTLS returns TLS configuration.
func PrepareTLS(scrt *models.Secrets, cfg *config.Config) (*tls.Config, error) {
	cert, err := tls.X509KeyPair([]byte(scrt.Cert), []byte(scrt.Key))
	if err != nil {
		return nil, fmt.Errorf("invalid certificates: %v", err)
	}

	rootCa, err := readRootCA(cfg.CaCert)
	if err != nil {
		return nil, fmt.Errorf("invalid root CA certificates: %v", err)
	}

	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
		RootCAs:      rootCa,
		ServerName:   cfg.CaName,
	}

	return tlsConfig, nil
}

func readRootCA(cert []byte) (*x509.CertPool, error) {
	caCertPool := x509.NewCertPool()
	ok := caCertPool.AppendCertsFromPEM(cert)
	if !ok {
		return nil, errors.New("invalid file content")
	}

	return caCertPool, nil
}
