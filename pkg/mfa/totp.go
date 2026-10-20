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
	"math"
	"time"

	"github.com/pquerna/otp/hotp"
)

const (
	secretSize = 20
	Period     = 30
)

// ErrTOTPReplay is returned by Authenticate when a valid code is reused. It is
// meant for server-side logging only and MUST NOT be exposed to clients.
var ErrTOTPReplay = errors.New("attempted replay of TOTP code")

type TOTPAuthenticator struct{}

type TOTPCredentials struct {
	Password string
}

type TOTPValidators struct {
	Key             string
	LastUsedCounter int64
}

// CreateTOTPKey generates a new random base32-encoded TOTP key.
func (a *TOTPAuthenticator) CreateTOTPKey() (string, error) {
	secret := make([]byte, secretSize)
	_, err := rand.Read(secret)
	if err != nil {
		return "", fmt.Errorf("failed to generate user secret: %v", err)
	}
	secretBase32 := base32.StdEncoding.EncodeToString(secret)
	return secretBase32, nil
}

// Authenticate checks user supplied TOTP code (in creds) against TOTP secret
// (in secret.Key). It provides replay protection - it does not authenticate if
// supplied code is valid for the same or earlier period than a previously
// validated period (in secret.LastUsedCounter).
//
// If possiblyAuthenticated is true, Authenticate returns the counter for which
// the TOTP code is valid. Callers MUST save the returned counter to database.
// Then the result must be treated as authenticated if and only if the database
// operation succeeds.
//
// Callers MUST hide returned ErrTOTPReplay.
func (a *TOTPAuthenticator) Authenticate(
	secret, creds interface{},
) (possiblyAuthenticated bool, counter int64, err error) {
	cred, ok := creds.(*TOTPCredentials)
	if !ok {
		return false, 0, errors.New("invalid credential type")
	}
	v, ok := secret.(*TOTPValidators)
	if !ok {
		return false, 0, errors.New("invalid secret type")
	}

	// the TOTP library does not support replay protection nor checking code for
	// custom counters, so we must use directly the underlying HOTP stuff
	currentTime := time.Now().UTC()
	currentCounter := TimeToCounter(currentTime)
	allowedCounters := []int64{currentCounter - 1, currentCounter, currentCounter + 1}

	for _, c := range allowedCounters {
		if hotp.Validate(cred.Password, uint64(c), v.Key) {
			if c <= v.LastUsedCounter {
				return false, 0, ErrTOTPReplay
			}
			return true, c, nil
		}
	}
	return false, 0, nil
}

func TimeToCounter(t time.Time) int64 {
	// copy-pasted from totp.ValidateCustom
	return int64(math.Floor(float64(t.Unix()) / float64(Period)))
}
