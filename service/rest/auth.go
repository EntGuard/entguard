/*
 * Copyright 2021 PANTHEON.tech s.r.o.
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

package rest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/entguard/entguard/pkg/httpconst"
	"github.com/entguard/entguard/service/api/v1"
	"github.com/entguard/entguard/service/user"
)

const (
	basicAuthType = "basic"
	jwtAuthType   = "jwt"
)

var (
	ErrAuthenticationFailed = errors.New("authentication failed")
)

func (s *Server) basicAuth(r *http.Request) (*api.User, error) {
	username, password, ok := r.BasicAuth()
	if !ok {
		return nil, fmt.Errorf("%w: invalid or missing basicAuth", ErrAuthenticationFailed)
	}

	u, err := s.userManager.GetUserByNameAndPassword(r.Context(), username, password)
	if err != nil {
		if err == user.ErrUserNotFound {
			return nil, fmt.Errorf("%w: unknown user name or bad password", ErrAuthenticationFailed)
		}
		return nil, err
	}
	return u, nil
}

func (s *Server) jwtAuth(r *http.Request) (*api.User, error) {
	reqToken := r.Header.Get(httpconst.Authorization)
	if reqToken == "" {
		return nil, fmt.Errorf("%w: missing Authorization header", ErrAuthenticationFailed)
	}

	splitToken := strings.Split(reqToken, "Bearer")
	if len(splitToken) != 2 {
		return nil, fmt.Errorf("%w: bad Authorization header", ErrAuthenticationFailed)
	}

	reqToken = strings.TrimSpace(splitToken[1])

	username, err := s.getUsernameFromToken(reqToken)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid token", ErrAuthenticationFailed)
	}
	u, err := s.userManager.GetUserByName(r.Context(), username)
	if err != nil {
		if err == user.ErrUserNotFound {
			return nil, fmt.Errorf("%w: invalid token", ErrAuthenticationFailed)
		}
		return nil, err
	}
	return u, nil
}

func (s *Server) getUsernameFromToken(token string) (string, error) {
	username, err := s.tokenProvider.ValidateToken(token)
	if err != nil {
		return "", err
	}
	return username, nil
}

func (s *Server) getTokenForUser(ctx context.Context, username, password string) (string, error) {
	u, err := s.userManager.GetUserByNameAndPassword(ctx, username, password)
	if err != nil {
		return "", err
	}
	if !u.IsAdmin {
		return "", user.ErrUserNotFound
	}
	token, err := s.tokenProvider.CreateToken(username)
	if err != nil {
		return "", err
	}
	s.logger.Debug("A new JWT issued")

	return token, nil
}
