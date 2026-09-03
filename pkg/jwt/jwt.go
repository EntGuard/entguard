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

package jwt

import (
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	tkn "github.com/golang-jwt/jwt/v5"
)

const (
	secretKeyLength   = 64
	defaultSignMethod = "HS256"
	minExpiration     = time.Second * 60
	issuer            = "urn:entguard:orchestrator"
	audience          = "urn:entguard:orchestrator:api"
)

var (
	errInvalidClaimsType    = errors.New("jwt claims type is invalid")
	errInvalidSigningMethod = errors.New("unexpected signing method")
	errInvalidToken         = errors.New("invalid token")
)

type TokenProvider struct {
	secret     []byte
	expiration time.Duration
	signMethod string
}

// NewTokenProvider creates new TokenProvider with expiration period set to expiration.
// Expiration must be greater than 60 seconds.
func NewTokenProvider(expiration time.Duration) (*TokenProvider, error) {
	if expiration < minExpiration {
		return nil, fmt.Errorf("expiration time must at least %v", minExpiration)
	}
	secret, err := generateSecretKey()
	if err != nil {
		return nil, err
	}
	p := &TokenProvider{
		secret:     secret,
		expiration: expiration,
		signMethod: defaultSignMethod,
	}
	return p, nil
}

func (p *TokenProvider) CreateToken(username string) (string, error) {
	now := time.Now()
	claims := &tkn.RegisteredClaims{
		ExpiresAt: tkn.NewNumericDate(now.Add(p.expiration)),
		IssuedAt:  tkn.NewNumericDate(now),
		NotBefore: tkn.NewNumericDate(now),
		Subject:   username,
		Issuer:    issuer,
		Audience:  tkn.ClaimStrings{audience},
	}
	return p.createToken(claims)
}

func (p *TokenProvider) createToken(claims *tkn.RegisteredClaims) (string, error) {
	token := tkn.NewWithClaims(tkn.GetSigningMethod(p.signMethod), claims)
	t, err := token.SignedString(p.secret)
	if err != nil {
		return "", err
	}
	return t, nil
}

func (p *TokenProvider) ValidateToken(token string) (username string, err error) {
	pt, err := tkn.ParseWithClaims(token,
		&tkn.RegisteredClaims{},
		func(token *tkn.Token) (interface{}, error) {
			if _, ok := token.Method.(*tkn.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("%w: %v", errInvalidSigningMethod, token.Header["alg"])
			}
			return p.secret, nil
		},
		tkn.WithExpirationRequired(),
		tkn.WithIssuedAt(),
		tkn.WithNotBeforeRequired(),
		tkn.WithIssuer(issuer),
		tkn.WithAudience(audience),
	)
	if err != nil {
		return "", fmt.Errorf("%w: %v", errInvalidToken, err)
	}

	if !pt.Valid {
		return "", errInvalidToken
	}

	c, ok := pt.Claims.(*tkn.RegisteredClaims)
	if !ok {
		return "", errInvalidClaimsType
	}

	return c.Subject, nil
}

func generateSecretKey() ([]byte, error) {
	key := make([]byte, secretKeyLength)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}
