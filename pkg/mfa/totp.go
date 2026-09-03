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
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"

	"github.com/dgryski/dgoogauth"
)

const (
	secretSize = 20
)

type TOTPAuthenticator struct{}

type TOTPCredentials struct {
	Password string
}

type TOTPSecret struct {
	Key string `json:"totp_key"`
}

func (a *TOTPAuthenticator) CreateUserSecrets() (*TOTPSecret, error) {
	key, err := createTOTPKey()
	if err != nil {
		return nil, err
	}

	return &TOTPSecret{
		Key: key,
	}, nil
}

func (a *TOTPAuthenticator) Authenticate(secret, creds interface{}) (bool, error) {
	totp, ok := creds.(*TOTPCredentials)
	if !ok {
		return false, errors.New("invalid credential type")
	}
	scrt, ok := secret.(*TOTPSecret)
	if !ok {
		return false, errors.New("invalid secret type")
	}

	authenticated, err := (&dgoogauth.OTPConfig{
		Secret:      scrt.Key,
		WindowSize:  3,
		HotpCounter: 0,
	}).Authenticate(totp.Password)
	if err != nil {
		return false, fmt.Errorf("failed to authenticate with TOTP: %v", err)
	}
	return authenticated, nil
}

func createTOTPKey() (string, error) {
	secret := make([]byte, secretSize)
	_, err := rand.Read(secret)
	if err != nil {
		return "", fmt.Errorf("failed to generate user secret: %v", err)
	}
	secretBase32 := base32.StdEncoding.EncodeToString(secret)
	return secretBase32, nil
}
