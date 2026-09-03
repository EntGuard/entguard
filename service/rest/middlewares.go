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

package rest

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/entguard/entguard/service/api/v1"
)

func (s *Server) authenticate(authFunc func(r *http.Request) (*api.User, error)) gin.HandlerFunc {
	mwLog := s.logger.WithField("reportCaller", "authenticate middleware")
	return func(c *gin.Context) {
		user, err := authFunc(c.Request)
		if err != nil {
			if errors.Is(err, ErrAuthenticationFailed) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, api.ErrorResponse{Message: err.Error()})
				return
			}
			mwLog.
				WithError(err).
				Info("Authentication failed")
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		if !user.IsAdmin {
			mwLog.
				WithField("username", user.Username).
				Info("Non-admin user tried to access management REST API.")
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		c.Next()
	}
}
