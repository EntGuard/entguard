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
	"fmt"
	"net/http"
	"path"
	"path/filepath"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"github.com/entguard/entguard/pkg/conc"
	"github.com/entguard/entguard/pkg/jwt"
	"github.com/entguard/entguard/pkg/limiter"
	"github.com/entguard/entguard/service/api/v1"
	"github.com/entguard/entguard/service/config"
	"github.com/entguard/entguard/service/user"
	"github.com/entguard/entguard/service/vcm"
)

const (
	angularUIDir = "./management-ui/dist/"
)

type TokenProvider interface {
	CreateToken(username string) (token string, err error)
	ValidateToken(token string) (username string, err error)
}

// Server represents the VPN Orchestrator service, and holds all of its dependencies.
type Server struct {
	cfg            *config.RESTServerConfig
	router         *gin.Engine
	userManager    user.Manager
	vcm            vcm.VPNConfigManager
	vpnStatusStore *conc.Map[int, error]
	hcStatusStore  *conc.Map[int, error]
	logger         *log.Logger
	tokenProvider  TokenProvider
}

// NewServer returns a new configured Server.
func NewServer(
	cfg *config.RESTServerConfig, m vcm.VPNConfigManager, um user.Manager, vpnSStatusStore *conc.Map[int, error],
	hcStatusStore *conc.Map[int, error], l *log.Logger,
) (*Server, error) {
	tokenProvider, err := jwt.NewTokenProvider(cfg.TokenExpiration)
	if err != nil {
		return nil, err
	}

	if l.Level != log.DebugLevel {
		gin.SetMode(gin.ReleaseMode)
	}

	s := &Server{
		cfg:            cfg,
		router:         gin.New(),
		userManager:    um,
		vcm:            m,
		vpnStatusStore: vpnSStatusStore,
		hcStatusStore:  hcStatusStore,
		logger:         l,
		tokenProvider:  tokenProvider,
	}

	s.router.Use(gin.Logger())
	s.router.Use(rateLimiterHandlerFunc(cfg.TokensPerSecond, cfg.BucketSize))

	if cfg.AuthType == jwtAuthType {
		s.router.POST(api.GetJWTURL, s.handleGetJWT())
	}

	managementRoutes := s.router.Group("/")

	// Setup authentication for all management routes.
	switch cfg.AuthType {
	case jwtAuthType:
		managementRoutes.Use(s.authenticate(s.jwtAuth))
	case basicAuthType:
		managementRoutes.Use(s.authenticate(s.basicAuth))
	default:
		return nil, fmt.Errorf("authentication of type %q is not supported yet", cfg.AuthType)
	}

	{
		managementRoutes.GET(api.ServersURL, s.handleGetAllServers())
		managementRoutes.POST(api.ServersURL, s.handleCreateServer())

		managementRoutes.GET(api.ServersStatusesURL, s.handleGetAllServersStatuses())

		managementRoutes.GET(api.ServerURL, s.handleGetServer())
		managementRoutes.PATCH(api.ServerURL, s.handleUpdateServer())
		managementRoutes.DELETE(api.ServerURL, s.handleDeleteServer())

		managementRoutes.GET(api.UsersURL, s.handleGetAllUsers())
		managementRoutes.POST(api.UsersURL, s.handleCreateUser())
		managementRoutes.GET(api.UserURL, s.handleGetUser())
		managementRoutes.PATCH(api.UserURL, s.handleUpdateUser())
		managementRoutes.PATCH(api.UserPasswordChangeURL, s.handleUpdateUserPassword())
		managementRoutes.DELETE(api.UserURL, s.handleDeleteUser())

		managementRoutes.GET(api.DeviceTemplateURL, s.handleGetDeviceTemplateByUserID())
		managementRoutes.POST(api.DeviceTemplateURL, s.handleCreateDeviceTemplateForUser())
		managementRoutes.PATCH(api.DeviceTemplateURL, s.handleUpdateDeviceTemplateByUserID())
		managementRoutes.DELETE(api.DeviceTemplateURL, s.handleDeleteDeviceTemplateByUserID())

		managementRoutes.GET(api.ServerVPNConfigURL, s.handleGetServerVPNConfig())
		managementRoutes.PATCH(api.ServerVPNConfigURL, s.handleUpdateServerVPNConfig())

		managementRoutes.GET(api.VPNServerAdjacenciesURL, s.handleGetVPNServerAdjacencies())
		managementRoutes.GET(api.VPNClientAdjacenciesURL, s.handleGetDeviceAdjacenciesByUserID())
		managementRoutes.POST(api.VPNAdjacenciesURL, s.handleCreateVPNAdjacency())
		managementRoutes.PATCH(api.VPNAdjacencyURL, s.handleUpdateVPNAdjacency())
		managementRoutes.DELETE(api.VPNAdjacencyURL, s.handleDeleteVPNAdjacency())

		managementRoutes.GET(api.VPNUserAdjacencyTemplatesURL, s.handleGetVPNUserAdjacencyTemplates())
		managementRoutes.POST(api.VPNAdjacencyTemplatesURL, s.handleCreateVPNAdjacencyTemplate())
		managementRoutes.PATCH(api.VPNAdjacencyTemplateURL, s.handleUpdateVPNAdjacencyTemplate())
		managementRoutes.DELETE(api.VPNAdjacencyTemplateURL, s.handleDeleteVPNAdjacencyTemplate())

		managementRoutes.POST(api.VPNServerSecretsURL, s.handleGetVPNServerSecrets())

		managementRoutes.GET(api.FeaturesURL, s.handleGetPremiumFeatures())

		managementRoutes.GET(api.MFAEnabledAuthTypesURL, s.handleGetMFAEnabledAuthTypes())
		managementRoutes.GET(api.MFATOTPSecretsURL, s.handleGetMFAUserTOTPSecrets())
		managementRoutes.POST(api.MFATOTPSecretsURL, s.handleCreateMFAUserTOTPSecrets())
		managementRoutes.PATCH(api.MFATOTPSecretsURL, s.handleUpdateMFAUserTOTPSecrets())
		managementRoutes.GET(api.MFACertCAsURL, s.handleGetMFACertificateCAs())
		managementRoutes.GET(api.MFACertCRLsURL, s.handleGetMFACertificateCRLs())
		managementRoutes.POST(api.MFACertCAsURL, s.handleAddMFACertificateCA())
		managementRoutes.POST(api.MFACertCRLsURL, s.handleAddMFACertificateCRL())
		managementRoutes.DELETE(api.MFACertCAURL, s.handleDeleteMFACertificateCA())
		managementRoutes.DELETE(api.MFACertCRLURL, s.handleDeleteMFACertificateCRL())

		managementRoutes.GET(api.AddressPoolsURL, s.handleGetAllAddressPools())
		managementRoutes.POST(api.AddressPoolsURL, s.handleCreateAddressPool())
		managementRoutes.GET(api.AddressPoolURL, s.handleGetAddressPool())
		managementRoutes.PATCH(api.AddressPoolURL, s.handleUpdateAddressPool())
		managementRoutes.DELETE(api.AddressPoolURL, s.handleDeleteAddressPool())

		managementRoutes.GET(api.DevicesURL, s.handleGetDevicesByTemplateID())
		managementRoutes.GET(api.DeviceAutogenURL, s.handleGetDeviceAutogenData())
		managementRoutes.POST(api.DevicesURL, s.handleCreateDevice())
		managementRoutes.PATCH(api.DeviceURL, s.handleUpdateDevice())
		managementRoutes.DELETE(api.DeviceURL, s.handleDeleteDevice())
		managementRoutes.POST(api.DeviceTemplateResyncURL, s.handleResyncDevices())
		managementRoutes.POST(api.UserDeviceAdjacenciesResyncURL, s.handleResyncDeviceAdjacencies())
	}

	if s.cfg.Features.LDAP() {
		managementRoutes.GET(api.LdapConfigsURL, s.handleGetAllLdapConfigs())
		managementRoutes.GET(api.LdapConfigURL, s.handleGetLdapConfig())
		managementRoutes.POST(api.LdapConfigsURL, s.handleCreateLdapConfig())
		managementRoutes.PATCH(api.LdapConfigURL, s.handleUpdateLdapConfig())
		managementRoutes.DELETE(api.LdapConfigURL, s.handleDeleteLdapConfig())
		managementRoutes.POST(api.LdapResyncURL, s.handleLdapResync())
		managementRoutes.GET(api.LdapSyncStatusURL, s.handleGetLdapSyncStatus())

		managementRoutes.GET(api.LdapTemplatesURL, s.handleGetAllLdapTemplates())

		managementRoutes.POST(api.LdapTemplatesURL, s.handleCreateLdapTemplate())
		managementRoutes.PATCH(api.LdapTemplateURL, s.handleUpdateLdapTemplate())
		managementRoutes.DELETE(api.LdapTemplateURL, s.handleDeleteLdapTemplate())

		managementRoutes.GET(api.LdapTemplateServersURL, s.handleGetLdapTemplateServers())
		managementRoutes.POST(api.LdapTemplateServersURL, s.handleCreateLdapTemplateServer())

		managementRoutes.PATCH(api.LdapTemplateServerURL, s.handleUpdateLdapTemplateServer())
		managementRoutes.DELETE(api.LdapTemplateServerURL, s.handleDeleteLdapTemplateServer())
	}

	// Serve Angular app.
	// [ Workaround Gin router conflicts. See: https://github.com/gin-gonic/gin/issues/2016 ]
	s.router.NoRoute(func(c *gin.Context) {
		dir, file := path.Split(c.Request.RequestURI)
		ext := filepath.Ext(file)
		if file == "" || ext == "" {
			// Let Angular app router to do the job.
			c.File(angularUIDir + "index.html")
		} else {
			// Serve static files.
			c.File(angularUIDir + path.Join(dir, file))
		}
	})

	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// rateLimiterHandlerFunc creates a middleware(handler function) that restricts overall server traffic based on
// token bucket implementation (token per second, bucket size).
func rateLimiterHandlerFunc(tps float64, bucketSize int) gin.HandlerFunc {
	limiter := limiter.NewLimiter(tps, bucketSize)

	return func(c *gin.Context) {
		if limiter.Limit() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Too many requests. Please try again later.",
			})
			c.Abort() // Stop processing subsequent handlers for this request
			return
		}

		c.Next() // Process the request
	}
}
