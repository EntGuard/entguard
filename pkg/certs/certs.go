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
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"time"
)

/*
 *
 * Inspired by https://github.com/FiloSottile/mkcert
 *
 */

const (
	rootCABitSize     = 3072
	clientCertBitSize = 2048

	// infinity sets amount of years for certs validity.
	infinity = 1000
)

// ServerTagOID is the OID used to store the server tag in the X.509 certificate Extensions field.
// The OID is structured as follows:
//   - 1.3.6.1.4.1       — standard IANA Private Enterprise Numbers (PEN) arc
//   - 65463             — PEN registered to PANTHEON.tech s.r.o.
//   - 1                 — product arc for EntGuard
//   - 1                 — attribute identifier for the server tag
var ServerTagOID = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 65463, 1, 1}

// legacyServerTagOID is the OID that carried the server tag before PANTHEON.tech had a
// registered PEN, when 53285 was picked arbitrarily under the PEN arc. Client certificates
// are issued with an effectively unlimited lifetime and are installed on the VPN servers,
// so certificates bearing the legacy OID never rotate out on their own. GetServerTag keeps
// accepting them so that upgrading the orchestrator does not reject servers that still
// present a certificate issued before the change; issuance always uses ServerTagOID.
var legacyServerTagOID = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 53285, 1, 1}

func newRootCA(orgz, orgzUnit string) (*x509.Certificate, crypto.PrivateKey, error) {
	pub, priv, err := generateKeyPair(rootCABitSize)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate the CA key: %v", err)
	}

	spkiASN1, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode public key: %v", err)
	}

	var spki struct {
		Algorithm        pkix.AlgorithmIdentifier
		SubjectPublicKey asn1.BitString
	}

	_, err = asn1.Unmarshal(spkiASN1, &spki)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode public key: %v", err)
	}

	skid := sha1.Sum(spki.SubjectPublicKey.Bytes)

	tpl := &x509.Certificate{
		SerialNumber: randomSerialNumber(),
		Subject: pkix.Name{
			Organization:       []string{orgz},
			OrganizationalUnit: []string{orgzUnit},
		},
		SubjectKeyId: skid[:],

		NotAfter:  time.Now().AddDate(infinity, 0, 0),
		NotBefore: time.Now(),

		KeyUsage: x509.KeyUsageCertSign,

		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, tpl, tpl, pub, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate CA certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA certificate: %v", err)
	}

	return cert, priv, nil
}

// GetServerTag extracts the server tag from the Extensions field of an X.509 certificate
func GetServerTag(cert *x509.Certificate) (string, error) {
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(ServerTagOID) || ext.Id.Equal(legacyServerTagOID) {
			var raw asn1.RawValue
			rest, err := asn1.Unmarshal(ext.Value, &raw)
			if err != nil {
				return "", fmt.Errorf("decoding server tag extension: %v", err)
			}
			if len(rest) != 0 {
				return "", fmt.Errorf("unexpected trailing bytes in server tag extension")
			}
			if raw.Class != asn1.ClassUniversal || raw.Tag != asn1.TagUTF8String {
				return "", fmt.Errorf("unexpected ASN.1 type in server tag extension")
			}
			return string(raw.Bytes), nil
		}
	}
	return "", fmt.Errorf("server tag extension not found in certificate")
}

func newClientCert(orgz, orgzUnit, serverTag string, rootCA *x509.Certificate, rootCAKey crypto.PrivateKey) (cert []byte, key []byte, e error) {
	pub, priv, err := generateKeyPair(clientCertBitSize)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate certificate key: %v", err)
	}

	tagValue, err := asn1.Marshal(asn1.RawValue{
		Class: asn1.ClassUniversal,
		Tag:   asn1.TagUTF8String,
		Bytes: []byte(serverTag),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode server tag: %v", err)
	}

	tpl := &x509.Certificate{
		SerialNumber: randomSerialNumber(),
		Subject: pkix.Name{
			Organization:       []string{orgz},
			OrganizationalUnit: []string{orgzUnit},
		},

		ExtraExtensions: []pkix.Extension{
			{
				Id:    ServerTagOID,
				Value: tagValue,
			},
		},

		NotAfter:  time.Now().AddDate(infinity, 0, 0),
		NotBefore: time.Now(),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,

		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageCodeSigning,
		},
	}

	cert, err = x509.CreateCertificate(rand.Reader, tpl, rootCA, pub, rootCAKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate certificate: %v", err)
	}

	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode certificate key: %v", err)
	}

	encodedKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})
	encodedCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert})

	return encodedCert, encodedKey, nil
}

func generateKeyPair(size int) (crypto.PublicKey, crypto.PrivateKey, error) {
	var priv crypto.PrivateKey
	var err error

	priv, err = rsa.GenerateKey(rand.Reader, size)
	if err != nil {
		return nil, nil, err
	}

	pub := priv.(crypto.Signer).Public()

	return pub, priv, nil
}

func randomSerialNumber() *big.Int {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		log.Printf("failed to generate serial number: %v /n", err)
		return big.NewInt(0)
	}
	return serialNumber
}
