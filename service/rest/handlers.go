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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"github.com/entguard/entguard/pkg/models"
	"github.com/entguard/entguard/service/api/v1"
	"github.com/entguard/entguard/service/rpc"
	"github.com/entguard/entguard/service/user"
	"github.com/entguard/entguard/service/vcm"
)

// List of possible errors.
const (
	errUserNotFound               = "user does not exist"
	errUserAcquisition            = "user acquisition failed"
	errUserNotCreated             = "failed to create a user"
	errUserNotUpdated             = "failed to update user"
	errUserNotDeleted             = "failed to delete a user"
	errUsersNotRetrieved          = "failed to retrieve all users"
	errDeviceTemplateDoesNotExist = "Device template does not exist for this user"
	errDeviceTemplateExists       = "Device template already exists for this user"
	errUserSecretsNotMFAFound     = "user MFA secrets were not found"

	errDeviceNotFound    = "device does not exist"
	errDeviceAcquisition = "device acquisition failed"

	errDeviceTemplateAcquisition = "failed to retrieve device template"
	errDeviceTemplateDeletion    = "failed to delete device template"
	errDeviceTemplateUnmarshal   = "failed to unmarshal device template"
	errDeviceTemplateMarshal     = "failed to marshal device template"
	errDeviceTemplateNotCreated  = "failed to create a device template"
	errDeviceTemplateNotUpdated  = "failed to update device template"

	errServerNotFound      = "server does not exist"
	errServerAcquisition   = "server acquisition failed"
	errServersNotRetrieved = "failed to retrieve all servers"
	errServerExists        = "server already exists"
	errServerNotCreated    = "failed to create a server"
	errServerNotDeleted    = "failed to delete a server"
	errServerNotUpdated    = "failed to update a server"

	errServerVPNConfigDoesNotExist = "VPN configuration does not exist for this server"
	errServerVPNConfigAcquisition  = "failed to retrieve server VPN config"
	errServerVPNConfigUnmarshal    = "failed to unmarshal server WireGuard config"
	errServerVPNConfigMarshal      = "failed to marshal server WireGuard config"
	errServerVPNConfigNotUpdated   = "failed to update server WireGuard config"

	errAdjacencyNotFound   = "adjacency does not exist"
	errAdjacencyExists     = "adjacency already exists"
	errAdjacencyNotCreated = "failed to create adjacency"
	errAdjacencyNotUpdated = "failed to update adjacency"
	errAdjacencyNotDeleted = "failed to delete adjacency"
	errAdjacencyInvalid    = "adjacency invalid"

	errCertificateNotGenerated = "failed to generate a certificate"

	errGeneric      = "something went wrong. Please check the logs for details"
	errInvalidJSON  = "invalid JSON body"
	errNonIntegerID = "id should be of integer value"

	errTokenNotCreated = "incorrect username or password"

	errLdapConfigNotFound              = "LDAP configuration not found"
	errLdapTemplateNotFound            = "LDAP template not found"
	errLdapTemplateInterfaceNotUpdated = "failed to update LDAP template interface"
	errLdapSyncInProgress              = "LDAP sync in progress. Please, wait until it ends"
	errLdapSyncNotStarted              = "LDAP sync has not started yet. You could run it manually or wait until the start"

	errCertDataNotFound = "CA certificate info not found"

	errTemplateExists = "template already exists"

	errReplicaCheckFailed = "failed to check if all replicas of server are running"

	// MessageKey values are defined in management-ui/src/assets/i18n/en.json file
	statusPrefix        = "service.statuses."
	statusDisconnected  = "disconnected"
	statusInSync        = "in-sync"
	statusNotConfigured = "not-configured"
	statusConfigError   = "configuration.error"
)

func (s *Server) handleCreateUser() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleCreateUser")
	return func(c *gin.Context) {
		var reqBody api.UserWithPassword
		if err := c.ShouldBindJSON(&reqBody); err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errInvalidJSON, err)})
			return
		}

		if err := s.userManager.CreateUser(c, &reqBody); err != nil {
			if errors.Is(err, user.ErrFeaturesRestriction) ||
				errors.Is(err, user.ErrBadUserData) {
				c.JSON(http.StatusBadRequest,
					api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errUserNotCreated, err)})
				return
			}
			hndlLog.
				WithError(err).
				Warn("Failed to create a new user")
			c.JSON(http.StatusInternalServerError,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errUserNotCreated, err)})
			return
		}

		c.Status(http.StatusOK)
	}
}

func (s *Server) handleGetAllUsers() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetAllUsers")
	return func(c *gin.Context) {
		users, err := s.userManager.GetAllUsers(c)
		if err != nil {
			hndlLog.
				WithError(err).
				Warn("Failed to retrieve all users")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errUsersNotRetrieved})
			return
		}
		c.JSON(http.StatusOK, users)
	}
}

func (s *Server) handleGetUser() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetUser")
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		resp, err := s.userManager.GetUserByID(c, id)
		if err != nil {
			if err == user.ErrUserNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errUserNotFound})
			} else {
				hndlLog.
					WithError(err).
					Warn("Failed to find a user")
				c.JSON(http.StatusInternalServerError,
					api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errUserAcquisition, err)})
			}
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

func (s *Server) handleUpdateUser() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleUpdateUser")
	return func(c *gin.Context) {
		var reqBody api.UserUpdate
		if err := c.ShouldBindJSON(&reqBody); err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errInvalidJSON, err)})
			return
		}

		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		updatedUser, err := s.userManager.UpdateUser(c, id, &reqBody)
		if err != nil {
			if err == user.ErrUserNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errUserNotFound})
				return
			}
			if errors.Is(err, user.ErrBadUserData) {
				c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: err.Error()})
				return
			}
			if errors.Is(err, user.ErrOnlyForIntrUsers) {
				c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: err.Error()})
				return
			}

			hndlLog.
				WithError(err).
				Warn("Failed to update a user")
			c.JSON(http.StatusInternalServerError,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errUserNotUpdated, err)})
			return
		}

		c.JSON(http.StatusOK, updatedUser)
	}
}

func (s *Server) handleUpdateUserPassword() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleUpdateUserPassword")
	return func(c *gin.Context) {
		var reqBody api.UserPasswordUpdate
		err := c.ShouldBindJSON(&reqBody)
		if err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errInvalidJSON, err)})
			return
		}

		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		err = s.userManager.UpdateUserPassword(c, id, reqBody.Password)
		if err != nil {
			if err == user.ErrUserNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errUserNotFound})
				return
			}
			if errors.Is(err, user.ErrBadUserData) {
				c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: err.Error()})
				return
			}
			if errors.Is(err, user.ErrOnlyForIntrUsers) {
				c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: err.Error()})
				return
			}

			hndlLog.
				WithError(err).
				Warn("Failed to update a user password")
			c.JSON(http.StatusInternalServerError,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errUserNotUpdated, err)})
			return
		}

		c.Status(http.StatusOK)
	}
}

func (s *Server) handleDeleteUser() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleDeleteUser")
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		err = s.userManager.DeleteUser(c, id)
		if err != nil {
			if err == user.ErrUserNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errUserNotFound})
			} else {
				hndlLog.
					WithError(err).
					Warn("Failed to delete a user")
				c.JSON(http.StatusInternalServerError,
					api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errUserNotDeleted, err)})
			}
			return
		}

		c.Status(http.StatusOK)
	}
}

func (s *Server) handleGetJWT() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetJWT")
	return func(c *gin.Context) {
		var reqBody api.JWTRequestCreds
		if err := c.ShouldBindJSON(&reqBody); err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errInvalidJSON, err)})
			return
		}

		token, err := s.getTokenForUser(c, reqBody.Username, reqBody.Password)
		if err != nil {
			var status int
			var msg string

			if err == user.ErrUserNotFound {
				status = http.StatusBadRequest
				msg = errTokenNotCreated
			} else {
				hndlLog.WithError(err).Warn("Failed to create a token")
				status = http.StatusInternalServerError
				msg = errGeneric
			}

			c.JSON(status, api.ErrorResponse{Message: msg})
			return
		}

		c.JSON(http.StatusOK, api.JWT{Token: token})
	}
}

func (s *Server) handleCreateDeviceTemplateForUser() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleCreateDeviceTemplateForUser")
	return func(c *gin.Context) {
		var reqBody api.DeviceTemplate
		if err := c.ShouldBindJSON(&reqBody); err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errInvalidJSON, err)})
			return
		}
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		usr, err := s.userManager.GetUserByID(c, id)
		if err != nil {
			if err == user.ErrUserNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errUserNotFound})
				return
			}
			hndlLog.
				WithError(err).
				Error("Finding a user")
			c.JSON(http.StatusInternalServerError,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errUserAcquisition, err)})
			return
		}

		exists, err := s.vcm.CheckDeviceTemplateExistsByUserID(c, usr.ID)
		if err != nil {
			hndlLog.
				WithError(err).
				Error("Checking device template existence")
			c.JSON(http.StatusInternalServerError,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errDeviceTemplateNotCreated, err)})
			return
		}
		if exists {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: errDeviceTemplateExists})
			return
		}

		_, err = s.vcm.CreateDeviceTemplate(c, usr.ID, &reqBody)
		if err != nil {
			hndlLog.
				WithError(err).
				Error("Creating a device template")
			c.JSON(http.StatusInternalServerError,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errDeviceTemplateNotCreated, err)})
			return
		}

		c.Status(http.StatusOK)
	}
}

func (s *Server) handleUpdateDeviceTemplateByUserID() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleUpdateDeviceTemplateByUserID")
	return func(c *gin.Context) {
		var reqBody api.DeviceTemplate
		if err := c.ShouldBindJSON(&reqBody); err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errInvalidJSON, err)})
			return
		}
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		usr, err := s.userManager.GetUserByID(c, id)
		if err != nil {
			if err == user.ErrUserNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errUserNotFound})
				return
			}
			hndlLog.
				WithError(err).
				Warn("Failed to find a user")
			c.JSON(http.StatusInternalServerError,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errUserAcquisition, err)})
			return
		}

		err = s.vcm.UpdateDeviceTemplateByUserID(c, usr.ID, &reqBody)
		if err != nil {
			hndlLog.
				WithError(err).
				Warn("Failed to update device template")
			c.JSON(http.StatusInternalServerError,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errDeviceTemplateNotUpdated, err)})
			return
		}

		c.Status(http.StatusOK)
	}
}

func (s *Server) handleCreateVPNAdjacency() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleCreateVPNAdjacency")
	return func(c *gin.Context) {
		var reqBody api.Adjacency
		if err := c.ShouldBindJSON(&reqBody); err != nil {
			msg := api.ErrorResponse{Message: fmt.Sprintf("%s: %v", errInvalidJSON, err)}
			c.JSON(http.StatusBadRequest, msg)
			return
		}

		_, err := s.userManager.GetDevice(c, reqBody.DeviceID)
		if err != nil {
			if err == user.ErrNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errDeviceNotFound})
				return
			}
			hndlLog.
				WithError(err).
				Error("Failed to find a device")
			msg := fmt.Sprintf("%s: %v", errDeviceAcquisition, err)
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: msg})
			return
		}

		_, err = s.vcm.GetServerByID(c, reqBody.ServerID)
		if err != nil {
			if err == vcm.ErrNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errServerNotFound})
				return
			}
			hndlLog.
				WithError(err).
				Warn("Failed to find a server")
			msg := fmt.Sprintf("%s: %v", errServerAcquisition, err)
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: msg})
			return
		}

		err = s.vcm.CreateAdjacency(c, reqBody.ServerID, reqBody.DeviceID, reqBody.Config)
		if err != nil {
			if errors.Is(err, vcm.ErrInvalidData) {
				msg := fmt.Sprintf("%s: %v", errAdjacencyInvalid, err)
				c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: msg})
			} else if errors.Is(err, vcm.ErrAlreadyExists) {
				c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: errAdjacencyExists})
			} else {
				hndlLog.
					WithError(err).
					Warn("Failed to create adjacency")
				msg := fmt.Sprintf("%s: %v", errAdjacencyNotCreated, err)
				c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: msg})
			}
			return
		}

		c.Status(http.StatusOK)
	}
}

func (s *Server) handleUpdateVPNAdjacency() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleUpdateVPNAdjacency")
	return func(c *gin.Context) {
		var reqBody api.Adjacency
		if err := c.ShouldBindJSON(&reqBody); err != nil {
			msg := api.ErrorResponse{Message: fmt.Sprintf("%s: %v", errInvalidJSON, err)}
			c.JSON(http.StatusBadRequest, msg)
			return
		}

		adjacencyID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}

		_, err = s.userManager.GetDevice(c, reqBody.DeviceID)
		if err != nil {
			if err == user.ErrNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errDeviceNotFound})
				return
			}
			hndlLog.WithError(err).
				WithField("deviceID", reqBody.DeviceID).
				Error("Failed to find a device")
			msg := fmt.Sprintf("%v : %v", errDeviceAcquisition, err)
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: msg})
			return
		}

		_, err = s.vcm.GetServerByID(c, reqBody.ServerID)
		if err != nil {
			if err == vcm.ErrNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errServerNotFound})
				return
			}
			hndlLog.
				WithError(err).
				WithField("serverID", reqBody.ServerID).
				Warn("Failed to find a server")
			msg := fmt.Sprintf("%s: %v", errServerAcquisition, err)
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: msg})
			return
		}

		err = s.vcm.UpdateAdjacency(c, adjacencyID, reqBody.ServerID, reqBody.DeviceID, reqBody.Config)
		if err != nil {
			if errors.Is(err, vcm.ErrInvalidData) {
				msg := fmt.Sprintf("%s: %v", errAdjacencyInvalid, err)
				c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: msg})
			} else if errors.Is(err, vcm.ErrDoesNotExist) {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errAdjacencyNotFound})
			} else if errors.Is(err, vcm.ErrAlreadyExists) {
				c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: errAdjacencyExists})
			} else {
				hndlLog.
					WithError(err).
					Warn("Failed to update adjacency")
				msg := fmt.Sprintf("%s: %v", errAdjacencyNotUpdated, err)
				c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: msg})
			}
			return
		}

		c.Status(http.StatusOK)
	}
}

func (s *Server) handleDeleteVPNAdjacency() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleDeleteVPNAdjacency")
	return func(c *gin.Context) {
		adjacencyID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}

		err = s.vcm.DeleteAdjacency(c, adjacencyID)
		if err != nil {
			if errors.Is(err, vcm.ErrNotFound) {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errAdjacencyNotFound})
			} else {
				hndlLog.
					WithError(err).
					Warn("Failed to delete adjacency")
				msg := fmt.Sprintf("%s: %v", errAdjacencyNotDeleted, err)
				c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: msg})
			}
			return
		}

		c.Status(http.StatusOK)
	}
}

func (s *Server) handleGetVPNServerAdjacencies() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetVPNServerAdjacencies")
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		srv, err := s.vcm.GetServerByID(c, id)
		if err != nil {
			if err == vcm.ErrNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errServerNotFound})
				return
			}
			hndlLog.
				WithError(err).
				Warn("Failed to find a server")
			c.JSON(http.StatusInternalServerError,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errServerAcquisition, err)})
			return
		}

		adjacencies, err := s.vcm.GetWireGuardServerAdjacencies(c, srv.ID)
		if err != nil {
			hndlLog.
				WithError(err).
				WithField("serverID", srv.ID).
				Warn("Failed to retrieve a list of WireGuard adjacencies for server")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		adjs := api.AdjacencyList{
			Adjacencies: make([]api.AdjacencyExtended, 0, len(adjacencies)),
		}
		for _, p := range adjacencies {
			device, err := s.userManager.GetDevice(c, p.DeviceID)
			if err != nil {
				hndlLog.
					WithError(err).
					WithField("deviceID", p.DeviceID).
					Error("Device is present in peers, but could not be found")
				continue
			}
			adjs.Adjacencies = append(adjs.Adjacencies, api.AdjacencyExtended{
				ID:     p.ID,
				Device: device,
				Config: p.Config,
			})
		}

		c.JSON(http.StatusOK, adjs)
	}
}

func (s *Server) handleGetDeviceAdjacenciesByUserID() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetDeviceAdjacenciesByUserID")
	return func(c *gin.Context) {
		userID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}

		adjacencies, err := s.vcm.GetUserDeviceAdjacencies(c, userID)
		if err != nil {
			if errors.Is(err, vcm.ErrDoesNotExist) {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: "User not found"})
				return
			}
			hndlLog.
				WithError(err).
				WithField("userID", userID).
				Error("Retrieving device adjacencies for user")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		adjs := api.AdjacencyList{}
		for _, adj := range adjacencies {
			adjs.Adjacencies = append(adjs.Adjacencies, *adj)
		}

		c.JSON(http.StatusOK, adjs)
	}
}

func (s *Server) handleGetDeviceTemplateByUserID() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetDeviceTemplateByUserID")
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		usr, err := s.userManager.GetUserByID(c, id)
		if err != nil {
			if err == user.ErrUserNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errUserNotFound})
				return
			}
			hndlLog.
				WithError(err).
				Warn("Failed to find a user")
			c.JSON(http.StatusInternalServerError,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errUserAcquisition, err)})
			return
		}

		dt, err := s.vcm.GetDeviceTemplateByUserID(c, usr.ID)
		if err != nil {
			if errors.Is(err, vcm.ErrNotFound) {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errDeviceTemplateDoesNotExist})
			} else {
				hndlLog.
					WithError(err).
					Warn("Error retrieving device template")
				c.JSON(http.StatusInternalServerError,
					api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errDeviceTemplateAcquisition, err)})
			}
			return
		}

		c.JSON(http.StatusOK, dt)
	}
}

func (s *Server) handleDeleteDeviceTemplateByUserID() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleDeleteDeviceTemplateByUserID")
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		usr, err := s.userManager.GetUserByID(c, id)
		if err != nil {
			if err == user.ErrUserNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errUserNotFound})
				return
			}
			hndlLog.
				WithError(err).
				Warn("Failed to find a user")
			c.JSON(http.StatusInternalServerError,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errUserAcquisition, err)})
			return
		}

		dtExists, err := s.vcm.CheckDeviceTemplateExistsByUserID(c, usr.ID)
		if err != nil {
			hndlLog.
				WithError(err).
				Warn("Failed to check device template existence")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		if !dtExists {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errDeviceTemplateDoesNotExist})
			return
		}

		err = s.vcm.DeleteDeviceTemplateByUserID(c, usr.ID)
		if err != nil {
			hndlLog.
				WithError(err).
				Warn("Failed to delete device template")
			c.JSON(http.StatusInternalServerError,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errDeviceTemplateDeletion, err)})
			return
		}

		c.Status(http.StatusOK)
	}
}

func (s *Server) handleGetAllServers() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetAllServers")
	return func(c *gin.Context) {
		servers, err := s.vcm.GetAllServers(c)
		if err != nil {
			hndlLog.
				WithError(err).
				Warn("Failed to retrieve all servers")
			c.JSON(http.StatusInternalServerError,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errServersNotRetrieved, err)})
			return
		}

		c.JSON(http.StatusOK, servers)
	}
}

func (s *Server) handleGetAllServersStatuses() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetAllServersStatuses")
	return func(c *gin.Context) {
		servers, err := s.vcm.GetAllServers(c)
		if err != nil {
			hndlLog.
				WithError(err).
				Warn("Failed to retrieve all servers")
			c.JSON(http.StatusInternalServerError,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errServersNotRetrieved, err)})
			return
		}
		store := s.vpnStatusStore.Clone()
		statuses := &api.ServiceStatuses{
			Statuses: make([]api.ServiceStatus, 0, len(servers.Servers)),
		}
		for _, server := range servers.Servers {
			replicasOK, err := s.vcm.AllReplicasRunning(c, &server)
			if err != nil {
				hndlLog.
					WithError(err).
					Warn("Failed to check if server replicas are running")
				c.JSON(http.StatusInternalServerError,
					api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errReplicaCheckFailed, err)})
				return
			}

			status := api.ServiceStatus{
				ID:   server.ID,
				Name: server.Name,
				VPNServer: api.Status{
					Status:      api.ServiceStatusDefault,
					MessageKey:  "",
					MessageArgs: nil,
				},
			}

			statusErr, ok := store[server.ID]
			if ok {
				switch statusErr {
				case nil:
					if replicasOK {
						status.VPNServer.Status = api.ServiceStatusGreen
						status.VPNServer.MessageKey = statusPrefix + statusInSync
					} else {
						status.VPNServer.Status = api.ServiceStatusRed
						status.VPNServer.MessageKey = statusPrefix + statusConfigError
						status.VPNServer.MessageArgs = []string{"kubernetes replica count mismatch"}
					}
				case rpc.ErrDisconnected:
					status.VPNServer.Status = api.ServiceStatusRed
					status.VPNServer.MessageKey = statusPrefix + statusDisconnected
				default:
					status.VPNServer.Status = api.ServiceStatusRed
					status.VPNServer.MessageKey = statusPrefix + statusConfigError
					status.VPNServer.MessageArgs = []string{statusErr.Error()}
				}
			} else {
				status.VPNServer.Status = api.ServiceStatusRed
				status.VPNServer.MessageKey = statusPrefix + statusDisconnected
			}

			status.Healthcheck = s.setHealthcheckStatus(server.HealthCheckAddress)

			statuses.Statuses = append(statuses.Statuses, status)
		}
		c.JSON(http.StatusOK, statuses)
	}
}

func (s *Server) setHealthcheckStatus(hcAddr string) api.Status {
	status := api.Status{
		Status:      api.ServiceStatusGray,
		MessageKey:  statusPrefix + statusNotConfigured,
		MessageArgs: nil,
	}

	if hcAddr == "" {
		return status
	}

	hcErr, found := s.hcStatusStore.Get(rpc.DefaultHealthCheckStatusStoreID)
	if !found {
		hcErr = rpc.ErrDisconnected
		s.logger.Warnf("No healthcheck status in store. Using default status(%v).", hcErr.Error())
	}
	switch hcErr {
	case nil:
		status.Status = api.ServiceStatusGreen
		status.MessageKey = statusPrefix + statusInSync
	case rpc.ErrDisconnected:
		status.Status = api.ServiceStatusRed
		status.MessageKey = statusPrefix + statusDisconnected
	default:
		status.Status = api.ServiceStatusRed
		status.MessageKey = statusPrefix + statusConfigError
		status.MessageArgs = []string{hcErr.Error()}
	}

	return status
}

func (s *Server) handleGetServer() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetServer")
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		resp, err := s.vcm.GetServerByID(c, id)
		if err != nil {
			if err == vcm.ErrNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errServerNotFound})
			} else {
				hndlLog.
					WithError(err).
					Warn("Failed to find a server")
				c.JSON(http.StatusInternalServerError,
					api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errServerAcquisition, err)})
			}
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

func (s *Server) handleCreateServer() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleCreateServer")
	return func(c *gin.Context) {
		var reqBody api.ServerWithVPNConfig
		if err := c.ShouldBindJSON(&reqBody); err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errInvalidJSON, err)})
			return
		}
		if err := s.vcm.CreateServer(c, &reqBody); err != nil {
			switch {
			case errors.Is(err, vcm.ErrAlreadyExists):
				c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: errServerExists})
			case errors.Is(err, vcm.ErrInvalidData):
				c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: err.Error()})
			default:
				hndlLog.WithError(err).Warn("Failed to create server")
				c.JSON(http.StatusInternalServerError,
					api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errServerNotCreated, err)})
			}
			return
		}
		c.Status(http.StatusOK)
	}
}

func (s *Server) handleUpdateServer() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleUpdateServer")
	return func(c *gin.Context) {
		var reqBody api.ServerUpdate
		if err := c.ShouldBindJSON(&reqBody); err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errInvalidJSON, err)})
			return
		}

		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		updatedServer, err := s.vcm.UpdateServer(c, id, &reqBody)
		if err != nil {
			if err == vcm.ErrNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errServerNotFound})
				return
			}
			hndlLog.
				WithError(err).
				Warn("Failed to update server")

			c.JSON(http.StatusInternalServerError,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errServerNotUpdated, err)})
			return
		}

		c.JSON(http.StatusOK, updatedServer)
	}
}

func (s *Server) handleDeleteServer() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleDeleteServer")
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		if err := s.vcm.DeleteServer(c, id); err != nil {
			if err == vcm.ErrNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errServerNotFound})
			} else {
				hndlLog.
					WithError(err).
					Warn("Failed to delete a server")
				c.JSON(http.StatusInternalServerError,
					api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errServerNotDeleted, err)})
			}
			return
		}
		s.vpnStatusStore.Del(id)
		c.Status(http.StatusOK)
	}
}

func (s *Server) handleGetServerVPNConfig() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetServerVPNConfig")
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		srv, err := s.vcm.GetServerByID(c, id)
		if err != nil {
			if err == vcm.ErrNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errServerNotFound})
				return
			}
			hndlLog.
				WithError(err).
				Warn("Failed to find a server")
			c.JSON(http.StatusInternalServerError,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errServerAcquisition, err)})
			return
		}

		cfg, err := s.vcm.GetWireGuardServerConfig(c, srv.ID)
		if err != nil {
			if errors.Is(err, vcm.ErrNotFound) {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errServerVPNConfigDoesNotExist})
			} else {
				hndlLog.
					WithError(err).
					Warn("Error retrieving server VPN config")
				c.JSON(http.StatusInternalServerError,
					api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errServerVPNConfigAcquisition, err)})
			}
			return
		}

		c.JSON(http.StatusOK, cfg)

	}
}

func (s *Server) handleUpdateServerVPNConfig() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleUpdateServerVPNConfig")
	return func(c *gin.Context) {
		var reqBody api.ServerWireGuardInterface
		if err := c.ShouldBindJSON(&reqBody); err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errInvalidJSON, err)})
			return
		}
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: errNonIntegerID})
			return
		}
		srv, err := s.vcm.GetServerByID(c, id)
		if err != nil {
			if err == vcm.ErrNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errServerNotFound})
				return
			}
			hndlLog.
				WithError(err).
				Warn("Failed to find the server")
			c.JSON(http.StatusInternalServerError,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errServerAcquisition, err)})
			return
		}

		err = s.vcm.UpdateWireGuardServerConfig(c, srv.ID, &reqBody)
		if err != nil {
			hndlLog.
				WithError(err).
				Warn("Failed to update server WireGuard config")
			c.JSON(http.StatusInternalServerError,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errServerVPNConfigNotUpdated, err)})
			return
		}

		c.Status(http.StatusOK)
	}
}

func (s *Server) handleGetVPNServerSecrets() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetVPNServerSecrets")
	return func(c *gin.Context) {
		var reqBody api.VPNServerSecretsRequest
		if err := c.ShouldBindJSON(&reqBody); err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errInvalidJSON, err)})
			return
		}

		cert, key, err := s.vcm.GetCertForServer(c, reqBody.ServerName)
		if err != nil {
			if err == vcm.ErrNotFound {
				c.JSON(http.StatusNotFound,
					api.ErrorResponse{Message: errServerNotFound})
				return
			}
			hndlLog.
				WithError(err).
				Warn("Failed to create a client certificate")
			c.JSON(http.StatusInternalServerError,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errCertificateNotGenerated, err)})
			return
		}

		resp := &models.Secrets{
			Cert: string(cert),
			Key:  string(key),
		}
		c.JSON(http.StatusOK, resp)
	}
}

func (s *Server) handleGetPremiumFeatures() gin.HandlerFunc {
	return func(c *gin.Context) {
		features := &api.FeaturesGet{
			LDAP: s.cfg.Features.LDAP(),
		}
		c.JSON(http.StatusOK, features)
	}
}

func (s *Server) handleGetAllLdapConfigs() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetAllLdapConfigs")
	return func(c *gin.Context) {
		configList, err := s.userManager.GetAllLdapConfigs(c)
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to get list of LDAP configurations")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		c.JSON(http.StatusOK, configList)
	}
}

func (s *Server) handleGetLdapConfig() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetLdapConfig")
	return func(c *gin.Context) {
		config, err := s.userManager.GetLdapConfig(c, c.Param("id"))
		if errors.Is(err, user.ErrNotFound) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errLdapConfigNotFound})
			return
		}
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to get LDAP configuration")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		c.JSON(http.StatusOK, config)
	}
}

func (s *Server) handleCreateLdapConfig() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleCreateLdapConfig")
	return func(c *gin.Context) {
		var reqBody api.LdapConfig
		err := c.ShouldBindJSON(&reqBody)
		if err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("%s: %v", errInvalidJSON, err)})
			return
		}
		config, err := s.userManager.CreateLdapConfig(c, &reqBody)
		if errors.Is(err, user.ErrInvalidData) {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: err.Error()})
			return
		}
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to create LDAP configuration")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		c.JSON(http.StatusOK, config)
	}
}

func (s *Server) handleUpdateLdapConfig() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleUpdateLdapConfig")
	return func(c *gin.Context) {
		var reqBody api.LdapConfig
		err := c.ShouldBindJSON(&reqBody)
		if err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("%s: %v", errInvalidJSON, err)})
			return
		}
		err = s.userManager.UpdateLdapConfig(c, &reqBody, c.Param("id"))
		if errors.Is(err, user.ErrNotFound) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errLdapConfigNotFound})
			return
		}
		if errors.Is(err, user.ErrInvalidData) {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: err.Error()})
			return
		}
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to update LDAP configuration")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		c.Status(http.StatusOK)
	}
}

func (s *Server) handleDeleteLdapConfig() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleDeleteLdapConfig")
	return func(c *gin.Context) {
		err := s.userManager.DeleteLdapConfig(c, c.Param("id"))
		if errors.Is(err, user.ErrNotFound) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errLdapConfigNotFound})
			return
		}
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to delete LDAP configuration")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		c.Status(http.StatusOK)
	}
}

func (s *Server) handleLdapResync() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleLdapResync")
	return func(c *gin.Context) {
		err := s.userManager.Resync(c)
		if err == user.ErrSyncInProgress {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: errLdapSyncInProgress})
			return
		}
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to resync with LDAPs")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		c.Status(http.StatusOK)
	}
}

func (s *Server) handleGetLdapSyncStatus() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetLdapSyncStatus")
	return func(c *gin.Context) {
		status, err := s.userManager.GetLdapSyncStats(c)
		if err == user.ErrSyncNotStarted {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: errLdapSyncNotStarted})
			return
		}
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to get LDAP sync status")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		c.JSON(http.StatusOK, status)
	}
}

func (s *Server) handleCreateLdapTemplate() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleCreateLdapTemplate")
	return func(c *gin.Context) {
		var reqBody api.LdapTemplate
		err := c.ShouldBindJSON(&reqBody)
		if err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("%s: %v", errInvalidJSON, err)})
			return
		}

		_, err = s.userManager.CreateLdapTemplate(c, &reqBody)
		if errors.Is(err, vcm.ErrAlreadyExists) {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: errTemplateExists})
			return
		}
		if errors.Is(err, user.ErrInvalidData) {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: err.Error()})
			return
		}
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to create LDAP template")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		c.Status(http.StatusOK)
	}
}

func (s *Server) handleUpdateLdapTemplate() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleUpdateLdapTemplate")
	return func(c *gin.Context) {
		var reqBody api.LdapTemplate
		err := c.ShouldBindJSON(&reqBody)
		if err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("%s: %v", errInvalidJSON, err)})
			return
		}
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		err = s.userManager.UpdateLdapTemplate(c, id, &reqBody)
		if errors.Is(err, user.ErrNotFound) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errLdapTemplateNotFound})
			return
		}
		if errors.Is(err, user.ErrInvalidData) {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: err.Error()})
			return
		}
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to update LDAP template")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		c.Status(http.StatusOK)
	}
}

func (s *Server) handleDeleteLdapTemplate() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleDeleteLdapTemplate")
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		err = s.userManager.DeleteLdapTemplate(c, id)
		if errors.Is(err, user.ErrNotFound) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errLdapTemplateNotFound})
			return
		}
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to delete LDAP template")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		c.Status(http.StatusOK)
	}
}

func (s *Server) handleGetAllLdapTemplates() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetAllLdapTemplates")
	return func(c *gin.Context) {
		templateList, err := s.userManager.GetAllLdapTemplates(c)
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to get list of LDAP templates")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		c.JSON(http.StatusOK, templateList)
	}
}

func (s *Server) handleGetLdapTemplateServers() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetLdapTemplateServers")
	return func(c *gin.Context) {
		templateID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		exists, err := s.userManager.CheckLdapTemplateExists(c, templateID)
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to get LDAP template servers")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		if !exists {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errLdapTemplateNotFound})
			return
		}

		configList, err := s.userManager.GetLdapTemplateServers(c, templateID)
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to get list of LDAP configurations")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		c.JSON(http.StatusOK, configList)
	}
}

func (s *Server) handleCreateLdapTemplateServer() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleCreateLdapTemplateServer")
	return func(c *gin.Context) {
		var reqBody api.LdapTemplateServer
		err := c.ShouldBindJSON(&reqBody)
		if err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("%s: %v", errInvalidJSON, err)})
			return
		}

		templateID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		exists, err := s.userManager.CheckLdapTemplateExists(c, templateID)
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to create LDAP template server")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		if !exists {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errLdapTemplateNotFound})
			return
		}

		exists, err = s.vcm.CheckServerExists(c, reqBody.ServerID)
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to create LDAP template server")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		if !exists {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errServerNotFound})
			return
		}

		err = s.userManager.CreateLdapTemplateServer(c, templateID, &reqBody)
		if errors.Is(err, user.ErrInvalidData) {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: err.Error()})
			return
		}
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to create LDAP template server")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		c.Status(http.StatusOK)
	}
}

func (s *Server) handleUpdateLdapTemplateServer() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleUpdateLdapTemplateServer")
	return func(c *gin.Context) {
		var reqBody api.LdapTemplateServer
		err := c.ShouldBindJSON(&reqBody)
		if err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("%s: %v", errInvalidJSON, err)})
			return
		}

		templateID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		exists, err := s.userManager.CheckLdapTemplateExists(c, templateID)
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to update LDAP template server")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		if !exists {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errLdapTemplateNotFound})
			return
		}

		oldServerID, err := strconv.Atoi(c.Param("server-id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		exists, err = s.vcm.CheckServerExists(c, oldServerID)
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to update LDAP template server")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		if !exists {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errServerNotFound})
			return
		}

		err = s.userManager.UpdateLdapTemplateServer(c, templateID, oldServerID, &reqBody)
		if errors.Is(err, user.ErrInvalidData) {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: err.Error()})
			return
		}
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to update LDAP template server")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		c.Status(http.StatusOK)
	}
}

func (s *Server) handleDeleteLdapTemplateServer() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleDeleteLdapTemplateServer")
	return func(c *gin.Context) {
		templateID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		exists, err := s.userManager.CheckLdapTemplateExists(c, templateID)
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to delete LDAP template server")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		if !exists {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errLdapTemplateNotFound})
			return
		}

		serverID, err := strconv.Atoi(c.Param("server-id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		exists, err = s.vcm.CheckServerExists(c, serverID)
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to delete LDAP template server")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		if !exists {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errServerNotFound})
			return
		}

		err = s.userManager.DeleteLdapTemplateServer(c, templateID, serverID)
		if errors.Is(err, user.ErrInvalidData) {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: err.Error()})
			return
		}
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to delete LDAP template server")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		c.Status(http.StatusOK)
	}
}

func (s *Server) handleGetMFAEnabledAuthTypes() gin.HandlerFunc {
	return func(c *gin.Context) {
		et := s.userManager.MFAGetEnabledTypes(c)
		c.JSON(http.StatusOK, et)
	}
}

func (s *Server) handleGetMFAUserTOTPSecrets() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetMFAUserSecrets")
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		scrt, err := s.userManager.MFAGetTOTPSecret(c, id)
		if errors.Is(err, user.ErrNotFound) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errUserNotFound})
			return
		}
		if errors.Is(err, user.ErrUserSecretNotFound) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errUserSecretsNotMFAFound})
			return
		}
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to get user")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		mScrt, err := json.Marshal(scrt)
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to get user MFA secrets")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		c.JSON(http.StatusOK, &api.MFAUserSecrets{
			Secrets: mScrt,
		})
	}
}

func (s *Server) handleCreateMFAUserTOTPSecrets() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetMFAUserSecrets")
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		scrt, err := s.userManager.MFACreateTOTPSecret(c, id)
		if errors.Is(err, user.ErrNotFound) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errUserNotFound})
			return
		}
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to get user")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		mScrt, err := json.Marshal(scrt)
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to get user MFA secrets")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		c.JSON(http.StatusOK, &api.MFAUserSecrets{
			Secrets: mScrt,
		})
	}
}

func (s *Server) handleUpdateMFAUserTOTPSecrets() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetMFAUserSecrets")
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		scrt, err := s.userManager.MFAUpdateTOTPSecret(c, id)
		if errors.Is(err, user.ErrNotFound) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errUserNotFound})
			return
		}
		if errors.Is(err, user.ErrUserSecretNotFound) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errUserSecretsNotMFAFound})
			return
		}
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to get user")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		mScrt, err := json.Marshal(scrt)
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to get user MFA secrets")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		c.JSON(http.StatusOK, &api.MFAUserSecrets{
			Secrets: mScrt,
		})
	}
}

func (s *Server) handleGetMFACertificateCAs() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetMFACertificateCAs")
	return func(c *gin.Context) {
		l, err := s.userManager.MFAGetAllCAs(c)
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to get certificate CAs")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		c.JSON(http.StatusOK, l)
	}
}

func (s *Server) handleGetMFACertificateCRLs() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetMFACertificateCRLs")
	return func(c *gin.Context) {
		l, err := s.userManager.MFAGetAllCRLs(c)
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to get certificate CRLs")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		c.JSON(http.StatusOK, l)
	}
}

func (s *Server) handleAddMFACertificateCA() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleAddMFACertificateCA")
	return func(c *gin.Context) {
		var reqBody api.Cert
		if err := c.ShouldBindJSON(&reqBody); err != nil {
			msg := api.ErrorResponse{Message: fmt.Sprintf("%s: %v", errInvalidJSON, err)}
			c.JSON(http.StatusBadRequest, msg)
			return
		}

		err := s.userManager.MFACreateCA(c, []byte(reqBody.Data))
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to create certificate CA")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		c.Status(http.StatusOK)
	}
}

func (s *Server) handleAddMFACertificateCRL() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleAddMFACertificateCRL")
	return func(c *gin.Context) {
		var reqBody api.Cert
		if err := c.ShouldBindJSON(&reqBody); err != nil {
			msg := api.ErrorResponse{Message: fmt.Sprintf("%s: %v", errInvalidJSON, err)}
			c.JSON(http.StatusBadRequest, msg)
			return
		}

		err := s.userManager.MFACreateCRL(c, []byte(reqBody.Data))
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to create certificate CRL")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		c.Status(http.StatusOK)
	}
}

func (s *Server) handleDeleteMFACertificateCA() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleDeleteMFACertificateCA")
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		err = s.userManager.MFADeleteCA(c, id)
		if errors.Is(err, user.ErrMFACertNotFound) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errCertDataNotFound})
			return
		}
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to create certificate CA")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		c.Status(http.StatusOK)
	}
}

func (s *Server) handleDeleteMFACertificateCRL() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleDeleteMFACertificateCRL")
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		err = s.userManager.MFADeleteCRL(c, id)
		if errors.Is(err, user.ErrMFACertNotFound) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errCertDataNotFound})
			return
		}
		if err != nil {
			hndlLog.WithError(err).Warn("Failed to create certificate CRL")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		c.Status(http.StatusOK)
	}
}

func (s *Server) handleGetAllAddressPools() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetAllAddressPools")
	return func(c *gin.Context) {
		pools, err := s.userManager.GetAllAddressPools(c)
		if err != nil {
			hndlLog.WithError(err).Error("Get all address pools")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		response := api.AddressPoolListResponse{
			AddressPools: pools,
		}

		c.JSON(http.StatusOK, response)
	}
}

func (s *Server) handleGetAddressPool() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetAddressPool")
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}

		pool, err := s.userManager.GetAddressPoolByID(c, id)
		if err != nil {
			if errors.Is(err, user.ErrNotFound) {
				msg := fmt.Sprintf("Address pool with ID=%d not found", id)
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: msg})
			} else {
				hndlLog.WithError(err).Error("Get address pool with ID=", id)
				c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			}
			return
		}
		c.JSON(http.StatusOK, pool)
	}
}

func (s *Server) handleCreateAddressPool() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleCreateAddressPool")
	return func(c *gin.Context) {
		var req api.AddressPool
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errInvalidJSON, err)})
			return
		}
		req.Name = strings.TrimSpace(req.Name)

		if err := s.userManager.ValidateAddressPool(&req); err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("Address pool with name=%s is invalid: %v", req.Name, err)})
			return
		}

		pool, err := s.userManager.CreateAddressPool(c, &req)
		if err != nil {
			if errors.Is(err, user.ErrAlreadyExists) {
				c.JSON(http.StatusBadRequest,
					api.ErrorResponse{Message: fmt.Sprintf("Address pool with name=%s already exists", req.Name)})
				return
			}
			hndlLog.WithError(err).WithField("AddressPoolInput", req).Error("Create address pool")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		c.JSON(http.StatusCreated, pool)
	}
}

func (s *Server) handleUpdateAddressPool() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleUpdateAddressPool")
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}
		var req api.AddressPool
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errInvalidJSON, err)})
			return
		}

		req.Name = strings.TrimSpace(req.Name)

		if err := s.userManager.ValidateAddressPool(&req); err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("Address pool with name=%s is invalid: %v", req.Name, err)})
			return
		}

		pool, err := s.userManager.UpdateAddressPool(c, id, &req)
		if err != nil {
			if errors.Is(err, user.ErrAlreadyExists) {
				c.JSON(http.StatusBadRequest,
					api.ErrorResponse{Message: fmt.Sprintf("Address pool with name=%s already exists", req.Name)})
				return
			}
			if errors.Is(err, user.ErrNotFound) {
				msg := fmt.Sprintf("Address pool with ID=%d not found", id)
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: msg})
			}
			hndlLog.WithError(err).WithField("AddressPoolInput", req).Error("Update address pool")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}
		c.JSON(http.StatusOK, pool)
	}
}

func (s *Server) handleDeleteAddressPool() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleDeleteAddressPool")
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}

		if err := s.userManager.DeleteAddressPool(c, id); err != nil {
			if errors.Is(err, user.ErrNotFound) {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: "Address pool not found"})
				return
			}
			hndlLog.WithError(err).WithField("ID", id).Error("Delete address pool")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		c.Status(http.StatusNoContent)
	}
}

func (s *Server) handleGetDevicesByTemplateID() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetDevicesByTemplateID")
	return func(c *gin.Context) {
		deviceTemplateID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: errNonIntegerID})
			return
		}

		deviceList, err := s.userManager.GetDevicesByTemplateID(c, deviceTemplateID)
		if err != nil {
			if errors.Is(err, user.ErrNotFound) {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: "Device template not found"})
				return
			}
			hndlLog.WithError(err).WithField("deviceTemplateID", deviceTemplateID).Error("Get devices by template id")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		c.JSON(http.StatusOK, &api.DeviceListResponse{Devices: deviceList})
	}
}

func (s *Server) handleCreateDevice() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleCreateDevice")
	return func(c *gin.Context) {
		deviceTemplateID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: errNonIntegerID})
			return
		}

		var req api.DeviceData
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errInvalidJSON, err)})
			return
		}

		if _, err := s.userManager.CreateDevice(c, deviceTemplateID, &req); err != nil {
			if errors.Is(err, user.ErrNotFound) {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: "Device template not found"})
				return
			}
			if errors.Is(err, user.ErrDPULimitExceeded) {
				c.JSON(http.StatusForbidden, api.ErrorResponse{Message: err.Error()})
				return
			}

			if errors.Is(err, vcm.ErrInvalidData) || strings.Contains(err.Error(), "validation error") {
				c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: err.Error()})
				return
			}
			hndlLog.WithError(err).
				WithField("deviceTemplateID", deviceTemplateID).
				WithField("publicKey", req.PublicKey).
				Error("Create device")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		c.Status(http.StatusCreated)
	}
}

func (s *Server) handleGetDeviceAutogenData() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetDeviceAutogenData")
	return func(c *gin.Context) {
		deviceTemplateID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: errNonIntegerID})
			return
		}

		data, err := s.userManager.GenerateDeviceData(c, deviceTemplateID)
		if err != nil {
			if errors.Is(err, user.ErrNotFound) {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: "Device template not found"})
				return
			}
			if errors.Is(err, user.ErrDPULimitExceeded) {
				c.JSON(http.StatusForbidden, api.ErrorResponse{Message: err.Error()})
				return
			}
			hndlLog.WithError(err).WithField("deviceTemplateID", deviceTemplateID).Error("Generate device data")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		c.JSON(http.StatusOK, data)
	}
}

func (s *Server) handleUpdateDevice() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleUpdateDevice")
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: errNonIntegerID})
			return
		}

		var req api.DeviceData
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest,
				api.ErrorResponse{Message: fmt.Sprintf("%v : %v", errInvalidJSON, err)})
			return
		}

		if err := s.userManager.UpdateDevice(c, id, req); err != nil {
			if errors.Is(err, user.ErrNotFound) {
				msg := fmt.Sprintf("Device with ID=%d not found", id)
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: msg})
				return
			}
			hndlLog.WithError(err).WithField("device id", id).Error("UpdateDevice")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		c.Status(http.StatusCreated)
	}
}

func (s *Server) handleDeleteDevice() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleDeleteDevice")
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: errNonIntegerID})
			return
		}

		if err := s.userManager.DeleteDevice(c, id); err != nil {
			if errors.Is(err, user.ErrNotFound) {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: "Device not found"})
				return
			}
			hndlLog.WithError(err).WithField("device ID", id).Error("Delete device")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		c.Status(http.StatusNoContent)
	}
}

func (s *Server) handleResyncDevices() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleResyncDevices")
	return func(c *gin.Context) {
		deviceTemplateID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: errNonIntegerID})
			return
		}

		response, err := s.userManager.ResyncDevices(c, deviceTemplateID)
		if err != nil {
			if errors.Is(err, user.ErrNotFound) {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: "Device template not found"})
				return
			}
			hndlLog.WithError(err).WithField("deviceTemplateID", deviceTemplateID).Error("Resync devices")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		c.JSON(http.StatusOK, response)
	}
}

func (s *Server) handleResyncDeviceAdjacencies() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleResyncDeviceAdjacencies")
	return func(c *gin.Context) {
		userID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: errNonIntegerID})
			return
		}

		response, err := s.vcm.ResyncDeviceAdjacencies(c, userID)
		if err != nil {
			if errors.Is(err, vcm.ErrDoesNotExist) {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errUserNotFound})
				return
			}
			hndlLog.WithError(err).WithField("userID", userID).Error("Resync device adjacencies")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		c.JSON(http.StatusOK, response)
	}
}

func (s *Server) handleGetVPNUserAdjacencyTemplates() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleGetVPNUserAdjacencyTemplates")
	return func(c *gin.Context) {
		userID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}

		adjTemplates, err := s.vcm.GetUserAdjacencyTemplates(c, userID)
		if err != nil {
			if errors.Is(err, vcm.ErrDoesNotExist) {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errUserNotFound})
				return
			}
			hndlLog.
				WithError(err).
				WithField("userID", userID).
				Error("Retrieving a list of adjacency templates for user")
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: errGeneric})
			return
		}

		adjs := api.AdjacencyTemplateList{}
		for _, at := range adjTemplates {
			adjs.AdjacencyTemplates = append(adjs.AdjacencyTemplates, *at)
		}

		c.JSON(http.StatusOK, adjs)
	}
}

func (s *Server) handleCreateVPNAdjacencyTemplate() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleCreateVPNAdjacencyTemplate")
	return func(c *gin.Context) {
		var reqBody api.AdjacencyTemplate
		if err := c.ShouldBindJSON(&reqBody); err != nil {
			msg := api.ErrorResponse{Message: fmt.Sprintf("%s: %v", errInvalidJSON, err)}
			c.JSON(http.StatusBadRequest, msg)
			return
		}

		usr, err := s.userManager.GetUserByID(c, reqBody.UserID)
		if err != nil {
			if err == user.ErrUserNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errUserNotFound})
				return
			}
			hndlLog.
				WithError(err).
				WithField("userID", reqBody.UserID).
				Error("Cannot find a user")
			msg := fmt.Sprintf("%s: %v", errUserAcquisition, err)
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: msg})
			return
		}

		srv, err := s.vcm.GetServerByID(c, reqBody.ServerID)
		if err != nil {
			if err == vcm.ErrNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errServerNotFound})
				return
			}
			hndlLog.
				WithError(err).
				WithField("serverID", reqBody.ServerID).
				Error("Cannot find a server")
			msg := fmt.Sprintf("%s: %v", errServerAcquisition, err)
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: msg})
			return
		}

		err = s.vcm.CreateAdjacencyTemplate(c, srv, usr.ID, reqBody.TemplateConfig)
		if err != nil {
			if errors.Is(err, vcm.ErrInvalidData) {
				msg := fmt.Sprintf("%s: %v", errAdjacencyInvalid, err)
				c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: msg})
			} else if errors.Is(err, vcm.ErrAlreadyExists) {
				c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: errAdjacencyExists})
			} else {
				hndlLog.
					WithError(err).
					Error("Creating adjacency template")
				msg := fmt.Sprintf("%s: %v", errAdjacencyNotCreated, err)
				c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: msg})
			}
			return
		}

		hndlLog.WithFields(log.Fields{
			"serverID": reqBody.ServerID,
			"userID":   reqBody.UserID,
		}).Info("A new adjacency template added")

		c.Status(http.StatusOK)
	}
}

func (s *Server) handleUpdateVPNAdjacencyTemplate() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleUpdateVPNAdjacencyTemplate")
	return func(c *gin.Context) {
		var reqBody api.AdjacencyTemplate
		if err := c.ShouldBindJSON(&reqBody); err != nil {
			msg := api.ErrorResponse{Message: fmt.Sprintf("%s: %v", errInvalidJSON, err)}
			c.JSON(http.StatusBadRequest, msg)
			return
		}

		adjacencyTemplateID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}

		_, err = s.userManager.GetUserByID(c, reqBody.UserID)
		if err != nil {
			if err == user.ErrUserNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errUserNotFound})
				return
			}
			hndlLog.WithError(err).
				WithField("userID", reqBody.UserID).
				Error("Cannot find a user")
			msg := fmt.Sprintf("%v : %v", errUserAcquisition, err)
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: msg})
			return
		}

		srv, err := s.vcm.GetServerByID(c, reqBody.ServerID)
		if err != nil {
			if err == vcm.ErrNotFound {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errServerNotFound})
				return
			}
			hndlLog.
				WithError(err).
				WithField("serverID", reqBody.ServerID).
				Error("Cannot find a server")
			msg := fmt.Sprintf("%s: %v", errServerAcquisition, err)
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: msg})
			return
		}

		err = s.vcm.UpdateAdjacencyTemplate(c, adjacencyTemplateID, srv, reqBody.UserID, reqBody.TemplateConfig)
		if err != nil {
			if errors.Is(err, vcm.ErrInvalidData) {
				msg := fmt.Sprintf("%s: %v", errAdjacencyInvalid, err)
				c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: msg})
			} else if errors.Is(err, vcm.ErrDoesNotExist) {
				c.JSON(http.StatusBadRequest, api.ErrorResponse{Message: errAdjacencyNotFound})
			} else {
				hndlLog.
					WithError(err).
					Error("Updating adjacency template")
				msg := fmt.Sprintf("%s: %v", errAdjacencyNotUpdated, err)
				c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: msg})
			}
			return
		}

		hndlLog.WithFields(log.Fields{
			"serverID":            reqBody.ServerID,
			"userID":              reqBody.UserID,
			"adjacencyTemplateID": adjacencyTemplateID,
		}).Info("Adjacency template updated")

		c.Status(http.StatusOK)
	}
}

func (s *Server) handleDeleteVPNAdjacencyTemplate() gin.HandlerFunc {
	hndlLog := s.logger.WithField("reportCaller", "handleDeleteVPNAdjacencyTemplate")
	return func(c *gin.Context) {
		adjacencyTemplateID, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, errNonIntegerID)
			return
		}

		err = s.vcm.DeleteAdjacencyTemplate(c, adjacencyTemplateID)
		if err != nil {
			if errors.Is(err, vcm.ErrDoesNotExist) {
				c.JSON(http.StatusNotFound, api.ErrorResponse{Message: errAdjacencyNotFound})
			} else {
				hndlLog.
					WithError(err).
					Error("Deleting adjacency template")
				msg := fmt.Sprintf("%s: %v", errAdjacencyNotDeleted, err)
				c.JSON(http.StatusInternalServerError, api.ErrorResponse{Message: msg})
			}
			return
		}

		hndlLog.WithField("adjacencyTemplateID", adjacencyTemplateID).
			Info("Adjacency template deleted")

		c.Status(http.StatusOK)
	}
}
