/*
 * Copyright 2024 PANTHEON.tech s.r.o.
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

package rest_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	composeapi "github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/entguard/entguard/cmd/orchestrator/server"
	sqlc "github.com/entguard/entguard/db"
	"github.com/entguard/entguard/pkg/conc"
	"github.com/entguard/entguard/pkg/db"
	"github.com/entguard/entguard/pkg/httpconst"
	"github.com/entguard/entguard/pkg/mfa"
	"github.com/entguard/entguard/pkg/test"
	"github.com/entguard/entguard/pkg/test/grpc"
	"github.com/entguard/entguard/service/api/v1"
	"github.com/entguard/entguard/service/config"
	"github.com/entguard/entguard/service/rest"
	"github.com/entguard/entguard/service/rpc"
	"github.com/entguard/entguard/service/user"
	"github.com/entguard/entguard/service/vcm"
)

var (
	httpClient    *http.Client
	q             *user.PostgresQuerier
	vcmQ          vcm.VPNConfigManager
	sqlcQ         *sqlc.Queries
	manager       *user.PostgresManager
	composeClient composeapi.Service
	testDir       string
)

func TestMain(m *testing.M) {
	if !test.IsRESTFuzzTest() &&
		!test.IsRESTHandlersLocalTest() {
		m.Run()
		return
	}
	logger := log.New()
	logger.SetLevel(log.DebugLevel)

	ctx := context.Background()

	httpClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	conn, err := db.Connect(ctx, logger, test.PostgresConfig())
	if err != nil {
		logger.Fatal("db.Connect: ", err)
	}
	q = user.NewPostgresQuerier(ctx, conn, logger).(*user.PostgresQuerier)
	if err := test.TruncateAllTables(ctx, q.GetDbConnection(ctx)); err != nil {
		logger.Fatal("TruncateAllTables: ", err)
	}
	exitCode := 0
	if test.IsRESTFuzzTest() {
		dockerCli, err := command.NewDockerCli()
		if err != nil {
			logger.Fatal("NewDockerCli: ", err)
		}
		if err := dockerCli.Initialize(flags.NewClientOptions()); err != nil {
			logger.Fatal("Docker Client initialization: ", err)
		}
		composeClient = compose.NewComposeService(dockerCli)
		testDir, err = os.MkdirTemp("", "eg-fuzz-test")
		if err != nil {
			logger.Fatal("MkdirTemp(eg-fuzz-test): ", err)
		}
		defer func() {
			if exitCode == 0 {
				if err := os.RemoveAll(testDir); err != nil {
					logger.Fatal("RemoveAll(eg-fuzz-test): ", err)
				}
			}
		}()
	}
	if test.IsRESTHandlersLocalTest() {
		cfg, err := config.FromEnv()
		if err != nil {
			logger.Fatal("Read configuration", err)
		}
		vcmQ, err = vcm.NewVCMPostgresBased(ctx, conn, logger, false, test.TestEncryptionKey)
		if err != nil {
			logger.Fatal("NewVCMPostgresBased: ", err)
		}
		vcmQ.SetKubernetesClientset(ctx, nil) // do not use kube cluster settings in tests

		sqlcQ = sqlc.New(conn)

		manager, err = test.NewSamplePostgresManager(q, logger, user.NewLdapService(), vcmQ, sqlcQ)
		if err != nil {
			logger.Fatal("NewSamplePostgresManager: ", err)
		}
		vpnSStatusStore := conc.NewMap[int, error]()
		cfg.REST.Port = test.TestOrchestratorPort
		cfg.REST.Features = &config.FeaturesMock{}
		restTestDir, err := os.MkdirTemp("", "eg-rest-test")
		if err != nil {
			logger.Fatal("MkdirTemp(eg-rest-test): ", err)
		}
		defer func() {
			if exitCode == 0 {
				if err := os.RemoveAll(restTestDir); err != nil {
					logger.Fatal("RemoveAll(eg-rest-test): ", err)
				}
			}
		}()
		cfg.REST.CertFile, err = test.CreateFileWithContentInExistingDir(restTestDir, "orchCert.pem", []byte(grpc.Cert))
		if err != nil {
			logger.Fatal("Can't create orchCert.pem: ", err)
		}
		cfg.REST.KeyFile, err = test.CreateFileWithContentInExistingDir(restTestDir, "orchKey.pem", []byte(grpc.Key))
		if err != nil {
			logger.Fatal("Can't create orchKey.pem: ", err)
		}

		hcStatusStore := rpc.NewHealthcheckStatusStore()
		restServer, err := rest.NewServer(cfg.REST, vcmQ, manager, vpnSStatusStore, hcStatusStore, logger)
		if err != nil {
			logger.Fatal("Create REST service", err)
		}
		fails := make(chan error)
		go server.RunREST(restServer, cfg.REST, logger, fails)

	}
	exitCode = m.Run()
}

func TestFuzzRESTUsersURL(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	jwt, files := test.SkipOrSetupRESTFuzzTest(t, ctx, q, composeClient, testDir)
	url := httpconst.ProtocolHTTPS + test.TestOrchestratorURL + api.UsersURL

	t.Cleanup(func() {
		if t.Failed() {
			_ = test.DumpComposeLogs(ctx, composeClient, files.ComposeLogFile)
		}
		cancel()
	})

	initData := []api.UserWithPassword{
		{
			ID:           123,
			Username:     "Donald",
			Password:     "Donald!Mc#Fu zz2",
			IsAdmin:      false,
			MFAType:      mfa.TOTP,
			Notification: "created testing user Donald",
			UpdatedAt:    time.Now(),
		},
		{
			Username:     "Emily",
			Password:     "em1ly!T3 st Fuzz+?",
			IsAdmin:      true,
			MFAType:      mfa.CERT,
			Notification: "",
		},
		{
			ID:           2098,
			Username:     "Bogus",
			Password:     "passwor@%67dforbogus!Mc#2",
			IsAdmin:      true,
			MFAType:      mfa.TOTP,
			Notification: "creating Bogus user",
		},
	}
	err := test.WriteJSONWordlistToFile(files.WordlistFile, initData, 5000)
	require.NoError(t, err)

	err = test.RunFFUFJob(
		ctx,
		http.MethodPost,
		url,
		[]string{files.WordlistFile},
		jwt,
		files.FFUFLogFile,
		files.FFUFOutputFile,
	)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set(httpconst.Authorization, fmt.Sprintf("Bearer %s", jwt.Token))
	_, err = httpClient.Do(req)
	require.NoError(t, err)
}

func TestRESTHandleCreateServer(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupRESTHandlersLocalTest(t, ctx, q)
	apiUser := test.InsertSampleUser(t, ctx, q, test.TestUserName).ToAPI()

	badServerData := newSampleServerWithConfig()
	badServerData.Endpoint = "12345"
	badServerConfigData := newSampleServerWithConfig()
	badServerConfigData.Config.PublicKey = "abc"

	cases := []struct {
		name          string
		inputServer   *api.ServerWithVPNConfig
		expectFailure bool
	}{
		{
			name:        "server and config are created",
			inputServer: newSampleServerWithConfig(),
		},
		{
			name:          "error with server",
			inputServer:   badServerData,
			expectFailure: true,
		},
		{
			name:          "error with config in server",
			inputServer:   badServerConfigData,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeServerCount := test.CountTableRows(t, q.Conn, "servers")
			beforeConfigCount := test.CountTableRows(t, q.Conn, "server_wg_configs")

			req, err := test.CreateRequest(http.MethodPost, api.ServersURL, tc.inputServer, &apiUser)
			require.NoError(t, err)

			response, err := httpClient.Do(req)
			require.NoError(t, err)

			afterServerCount := test.CountTableRows(t, q.Conn, "servers")
			afterConfigCount := test.CountTableRows(t, q.Conn, "server_wg_configs")

			if tc.expectFailure {
				require.Equal(t, http.StatusBadRequest, response.StatusCode)
				require.Equal(t, beforeServerCount, afterServerCount)
				require.Equal(t, beforeConfigCount, afterConfigCount)
				return
			}
			require.Equal(t, http.StatusOK, response.StatusCode)
			require.Equal(t, beforeServerCount+1, afterServerCount)
			require.Equal(t, beforeConfigCount+1, afterConfigCount)

			serverResult, err := vcmQ.GetServerByName(ctx, "test_server")
			require.NoError(t, err)
			require.Equal(t, tc.inputServer.Endpoint, serverResult.Endpoint)

			configResult, err := vcmQ.GetWireGuardServerConfig(ctx, serverResult.ID)
			require.NoError(t, err)
			require.Equal(t, tc.inputServer.Config.Name, configResult.Name)
			require.Equal(t, tc.inputServer.Config.Addresses[0], configResult.Addresses[0])
		})
	}
}

func TestRESTHandleCreateLdapTemplate(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupRESTHandlersLocalTest(t, ctx, q)
	apiUser := test.InsertSampleUser(t, ctx, q, test.TestUserName).ToAPI()

	p1 := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("testPool1"))
	p2Params := test.NewSampleInsertAddressPoolParams("testPool2")
	p2Params.StartAddr = netip.MustParseAddr("10.1.0.1")
	p2Params.EndAddr = netip.MustParseAddr("10.1.0.100")
	p2 := test.InsertSampleAddressPool(t, ctx, sqlcQ, p2Params)
	adp := []*api.AddressPool{{ID: p1.ID}, {ID: p2.ID}}

	correctTemplateData := test.NewSampleLdapTemplate("testname", adp)

	invalidLdapTemplate := test.NewSampleLdapTemplate("bad1", adp)
	invalidLdapTemplate.MFAType = mfa.NONE

	invalidTemplateIfaceData := test.NewSampleLdapTemplate("bad2", nil)

	cases := []struct {
		name          string
		inputTemplate *api.LdapTemplate
		expectFailure bool
	}{
		{
			name:          "template is created",
			inputTemplate: correctTemplateData,
		},
		{
			name:          "template created with single address pool",
			inputTemplate: test.NewSampleLdapTemplate("single-pool", []*api.AddressPool{{ID: p1.ID}}),
		},
		{
			name:          "error with template",
			inputTemplate: invalidLdapTemplate,
			expectFailure: true,
		},
		{
			name:          "error with interface in template",
			inputTemplate: invalidTemplateIfaceData,
			expectFailure: true,
		},
		{
			name:          "empty address pools",
			inputTemplate: test.NewSampleLdapTemplate("empty-pools", []*api.AddressPool{}),
			expectFailure: true,
		},
		{
			name:          "nil address pools",
			inputTemplate: test.NewSampleLdapTemplate("nil-pools", nil),
			expectFailure: true,
		},
		{
			name:          "non-existing address pool ID",
			inputTemplate: test.NewSampleLdapTemplate("non-existing-pool", []*api.AddressPool{{ID: 9999}}),
			expectFailure: true,
		},
		{
			name:          "mix of valid and invalid address pool IDs",
			inputTemplate: test.NewSampleLdapTemplate("mixed-pools", []*api.AddressPool{{ID: p1.ID}, {ID: 9999}}),
			expectFailure: true,
		},
		{
			name:          "duplicate template name",
			inputTemplate: test.NewSampleLdapTemplate("testname", adp), // same name as first test case
			expectFailure: true,
		},
		{
			name: "empty interface name",
			inputTemplate: &api.LdapTemplate{
				Name:          "empty-interface",
				InterfaceName: "",
				AddressPools:  adp,
				MFAType:       mfa.CERT,
				IsAdmin:       true,
			},
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeCount := test.CountTableRows(t, q.Conn, "ldap_templates")
			req, err := test.CreateRequest(http.MethodPost, api.LdapTemplatesURL, tc.inputTemplate, &apiUser)
			require.NoError(t, err)

			response, err := httpClient.Do(req)
			require.NoError(t, err)

			afterCount := test.CountTableRows(t, q.Conn, "ldap_templates")

			if tc.expectFailure {
				require.Equal(t, http.StatusBadRequest, response.StatusCode)
				require.Equal(t, beforeCount, afterCount)
				return
			}
			require.Equal(t, http.StatusOK, response.StatusCode)
			require.Equal(t, beforeCount+1, afterCount)
		})
	}
}

func TestRESTHandleUpdateLdapTemplate(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupRESTHandlersLocalTest(t, ctx, q)

	apiUser := test.InsertSampleUser(t, ctx, q, test.TestUserName).ToAPI()

	temp := test.InsertSampleLdapTemplate(t, ctx, manager, "template", nil)
	p3 := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("testPool3"))
	p4Params := test.NewSampleInsertAddressPoolParams("testPool4")
	p4Params.StartAddr = netip.MustParseAddr("10.2.0.1")
	p4Params.EndAddr = netip.MustParseAddr("10.2.0.100")
	p4 := test.InsertSampleAddressPool(t, ctx, sqlcQ, p4Params)

	cases := []struct {
		name          string
		inputTemplate *api.LdapTemplate
		templateID    int
		statusCode    int
	}{
		{
			name: "template is updated",
			inputTemplate: &api.LdapTemplate{
				Name:          "updatedName",
				MFAType:       mfa.TOTP,
				IsAdmin:       false,
				Filter:        "updatedFilter",
				InterfaceName: "updatedIface",
				AddressPools: []*api.AddressPool{
					{ID: temp.AddressPools[0].ID},
					{ID: temp.AddressPools[1].ID},
				},
			},
			templateID: temp.ID,
			statusCode: http.StatusOK,
		},
		{
			name: "template updated with different address pools",
			inputTemplate: &api.LdapTemplate{
				Name:          "updatedName2",
				InterfaceName: "updatedIface2",
				AddressPools: []*api.AddressPool{
					{ID: p3.ID},
					{ID: p4.ID},
				},
				MFAType: mfa.TOTP,
			},
			templateID: temp.ID,
			statusCode: http.StatusOK,
		},
		{
			name: "template updated with single address pool",
			inputTemplate: &api.LdapTemplate{
				Name:          "updatedName3",
				InterfaceName: "updatedIface3",
				AddressPools: []*api.AddressPool{
					{ID: temp.AddressPools[0].ID},
				},
				MFAType: mfa.TOTP,
			},
			templateID: temp.ID,
			statusCode: http.StatusOK,
		},
		{
			name: "non existing template id",
			inputTemplate: &api.LdapTemplate{
				Name:          "updatedName",
				InterfaceName: "updatedIface",
				AddressPools: []*api.AddressPool{
					{ID: temp.AddressPools[0].ID},
					{ID: temp.AddressPools[1].ID},
				},
			},
			templateID: 5,
			statusCode: http.StatusNotFound,
		},
		{
			name: "empty name",
			inputTemplate: &api.LdapTemplate{
				Name:          "",
				InterfaceName: "updatedIface",
				AddressPools: []*api.AddressPool{
					{ID: temp.AddressPools[0].ID},
				},
			},
			templateID: temp.ID,
			statusCode: http.StatusBadRequest,
		},
		{
			name: "empty address pools",
			inputTemplate: &api.LdapTemplate{
				Name:          "validName",
				InterfaceName: "validIface",
				AddressPools:  []*api.AddressPool{},
			},
			templateID: temp.ID,
			statusCode: http.StatusBadRequest,
		},
		{
			name: "nil address pools",
			inputTemplate: &api.LdapTemplate{
				Name:          "validName",
				InterfaceName: "validIface",
				AddressPools:  nil,
			},
			templateID: temp.ID,
			statusCode: http.StatusBadRequest,
		},
		{
			name: "non-existing address pool ID",
			inputTemplate: &api.LdapTemplate{
				Name:          "validName",
				InterfaceName: "validIface",
				AddressPools:  []*api.AddressPool{{ID: 9999}},
			},
			templateID: temp.ID,
			statusCode: http.StatusBadRequest,
		},
		{
			name: "mix of valid and invalid address pool IDs",
			inputTemplate: &api.LdapTemplate{
				Name:          "validName",
				InterfaceName: "validIface",
				AddressPools: []*api.AddressPool{
					{ID: temp.AddressPools[0].ID},
					{ID: 9999},
				},
			},
			templateID: temp.ID,
			statusCode: http.StatusBadRequest,
		},
		{
			name: "empty interface name",
			inputTemplate: &api.LdapTemplate{
				Name:          "validName",
				InterfaceName: "",
				AddressPools: []*api.AddressPool{
					{ID: temp.AddressPools[0].ID},
				},
			},
			templateID: temp.ID,
			statusCode: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeCount := test.CountTableRows(t, q.Conn, "ldap_templates")
			req, err := test.CreateRequest(
				http.MethodPatch,
				test.URLWithID(api.LdapTemplateURL, fmt.Sprintf("%d", tc.templateID)),
				tc.inputTemplate,
				&apiUser,
			)
			require.NoError(t, err)

			response, err := httpClient.Do(req)
			require.NoError(t, err)
			require.Equal(t, tc.statusCode, response.StatusCode)

			afterCount := test.CountTableRows(t, q.Conn, "ldap_templates")
			require.Equal(t, beforeCount, afterCount)

			if response.StatusCode == http.StatusBadRequest ||
				response.StatusCode == http.StatusNotFound {
				return
			}

			temps, err := manager.GetAllLdapTemplates(ctx)
			require.NoError(t, err)
			temp := temps[0]
			require.Equal(t, tc.inputTemplate.Name, temp.Name)
			require.Equal(t, tc.inputTemplate.Filter, temp.Filter)
			require.Equal(t, tc.inputTemplate.IsAdmin, temp.IsAdmin)
			require.Equal(t, tc.inputTemplate.MFAType, temp.MFAType)
		})
	}
}

func TestRESTHandleGetAllAddressPools(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupRESTHandlersLocalTest(t, ctx, q)
	apiUser := test.InsertSampleUser(t, ctx, q, test.TestUserName).ToAPI()

	p1 := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("aTestPool"))
	p2 := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("zTestPool"))

	req, err := test.CreateRequest(http.MethodGet, api.AddressPoolsURL, nil, &apiUser)
	require.NoError(t, err)

	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got api.AddressPoolListResponse
	err = json.NewDecoder(resp.Body).Decode(&got)
	require.NoError(t, err)

	require.Equal(t, len(got.AddressPools), 2)

	require.Equal(t, user.ConvertDBToAPIPool(*p1), got.AddressPools[0])
	require.Equal(t, user.ConvertDBToAPIPool(*p2), got.AddressPools[1])
}

func TestRESTHandleGetAddressPool(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupRESTHandlersLocalTest(t, ctx, q)
	apiUser := test.InsertSampleUser(t, ctx, q, test.TestUserName).ToAPI()

	p := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("testPool"))

	cases := []struct {
		name         string
		idParam      string
		expectPool   *api.AddressPool
		expectStatus int
	}{
		{
			name:         "valid ID",
			idParam:      fmt.Sprintf("%d", p.ID),
			expectPool:   user.ConvertDBToAPIPool(*p),
			expectStatus: http.StatusOK,
		},
		{
			name:         "non-integer ID",
			idParam:      "abc",
			expectStatus: http.StatusBadRequest,
		},
		{
			name:         "non-existing ID",
			idParam:      "5",
			expectStatus: http.StatusNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url := test.URLWithID(api.AddressPoolURL, tc.idParam)
			req, err := test.CreateRequest(http.MethodGet, url, nil, &apiUser)
			require.NoError(t, err)

			resp, err := httpClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, tc.expectStatus, resp.StatusCode)

			if tc.expectStatus == http.StatusOK {
				var got api.AddressPool
				err = json.NewDecoder(resp.Body).Decode(&got)
				require.NoError(t, err)
				require.Equal(t, tc.expectPool, &got)
			}
		})
	}
}

func TestRESTHandleCreateAddressPool(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupRESTHandlersLocalTest(t, ctx, q)
	apiUser := test.InsertSampleUser(t, ctx, q, test.TestUserName).ToAPI()

	validPool := &api.AddressPool{
		Name:        "testPool1",
		StartAddr:   "10.10.0.1",
		EndAddr:     "10.10.0.254",
		NetMask:     24,
		Description: "description",
	}

	invalidPool := &api.AddressPool{
		Name:        "testPool2",
		StartAddr:   "10.20.0.1",
		EndAddr:     "not.an.ip",
		NetMask:     24,
		Description: "description",
	}

	_ = test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams(validPool.Name))

	cases := []struct {
		name         string
		inputPool    *api.AddressPool
		expectStatus int
	}{
		{
			name: "valid pool",
			inputPool: &api.AddressPool{
				Name:        "unique-pool",
				StartAddr:   "10.20.0.1",
				EndAddr:     "10.20.0.10",
				NetMask:     24,
				Description: "description",
			},
			expectStatus: http.StatusCreated,
		},
		{
			name:         "invalid pool - bad IP",
			inputPool:    invalidPool,
			expectStatus: http.StatusBadRequest,
		},
		{
			name:         "duplicate pool",
			inputPool:    validPool,
			expectStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeCount := test.CountTableRows(t, q.Conn, "address_pools")
			req, err := test.CreateRequest(http.MethodPost, api.AddressPoolsURL, tc.inputPool, &apiUser)
			require.NoError(t, err)

			resp, err := httpClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			afterCount := test.CountTableRows(t, q.Conn, "address_pools")
			require.Equal(t, tc.expectStatus, resp.StatusCode)

			if tc.expectStatus != http.StatusCreated {
				require.Equal(t, beforeCount, afterCount)
				return
			}
			require.Equal(t, beforeCount+1, afterCount)
		})
	}
}

func TestRESTHandleUpdateAddressPool(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupRESTHandlersLocalTest(t, ctx, q)
	apiUser := test.InsertSampleUser(t, ctx, q, test.TestUserName).ToAPI()

	p1 := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("testPool1"))

	_ = test.InsertSampleAddressPool(t, ctx, sqlcQ, &sqlc.InsertAddressPoolParams{
		Name:        "testPool2",
		StartAddr:   netip.MustParseAddr("10.20.0.1"),
		EndAddr:     netip.MustParseAddr("10.20.0.254"),
		NetMask:     24,
		Description: "description",
	})

	cases := []struct {
		name         string
		idParam      string
		input        *api.AddressPool
		expectStatus int
	}{
		{
			name:    "successful update, all fields",
			idParam: fmt.Sprintf("%d", p1.ID),
			input: &api.AddressPool{
				Name:        "updatedName",
				StartAddr:   "10.30.0.10",
				EndAddr:     "10.30.0.200",
				NetMask:     24,
				Description: "description updated",
			},
			expectStatus: http.StatusOK,
		},
		{
			name:    "invalid ID (non-integer)",
			idParam: "abc",
			input: &api.AddressPool{
				Name:        "invalidID",
				StartAddr:   "10.30.0.10",
				EndAddr:     "10.30.0.200",
				NetMask:     24,
				Description: "description",
			},
			expectStatus: http.StatusBadRequest,
		},
		{
			name:    "invalid IP format",
			idParam: fmt.Sprintf("%d", p1.ID),
			input: &api.AddressPool{
				Name:        "invalidIP",
				StartAddr:   "bad.ip.addr",
				EndAddr:     "10.10.10.10",
				NetMask:     24,
				Description: "description",
			},
			expectStatus: http.StatusBadRequest,
		},
		{
			name:    "name conflict",
			idParam: fmt.Sprintf("%d", p1.ID),
			input: &api.AddressPool{
				Name:        "testPool2",
				StartAddr:   "10.10.10.1",
				EndAddr:     "10.10.10.254",
				NetMask:     24,
				Description: "description",
			},
			expectStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url := test.URLWithID(api.AddressPoolURL, tc.idParam)
			req, err := test.CreateRequest(http.MethodPatch, url, tc.input, &apiUser)
			require.NoError(t, err)

			resp, err := httpClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, tc.expectStatus, resp.StatusCode)

			if tc.expectStatus == http.StatusOK {
				var result api.AddressPool
				err = json.NewDecoder(resp.Body).Decode(&result)
				require.NoError(t, err)
				require.Equal(t, tc.input.Name, result.Name)
				require.Equal(t, tc.input.StartAddr, result.StartAddr)
				require.Equal(t, tc.input.EndAddr, result.EndAddr)
				require.Equal(t, tc.input.NetMask, result.NetMask)
				require.Equal(t, tc.input.Description, result.Description)
			}
		})
	}
}

func TestRESTHandleDeleteAddressPool(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupRESTHandlersLocalTest(t, ctx, q)
	apiUser := test.InsertSampleUser(t, ctx, q, test.TestUserName).ToAPI()

	p := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("testPool"))

	cases := []struct {
		name         string
		idParam      string
		expectStatus int
	}{
		{
			name:         "successful deletion",
			idParam:      fmt.Sprintf("%d", p.ID),
			expectStatus: http.StatusNoContent,
		},
		{
			name:         "non-integer ID",
			idParam:      "abc",
			expectStatus: http.StatusBadRequest,
		},
		{
			name:         "non-existent pool",
			idParam:      "5",
			expectStatus: http.StatusNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeCount := test.CountTableRows(t, q.Conn, "address_pools")

			url := test.URLWithID(api.AddressPoolURL, tc.idParam)
			req, err := test.CreateRequest(http.MethodDelete, url, nil, &apiUser)
			require.NoError(t, err)

			resp, err := httpClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, tc.expectStatus, resp.StatusCode)

			afterCount := test.CountTableRows(t, q.Conn, "address_pools")
			if tc.expectStatus != http.StatusNoContent {
				require.Equal(t, beforeCount, afterCount)
				return
			}
			require.Equal(t, beforeCount-1, afterCount)

		})
	}
}

func newSampleServerWithConfig() *api.ServerWithVPNConfig {
	return &api.ServerWithVPNConfig{
		Server: api.Server{
			Name:     "test_server",
			Endpoint: "127.0.0.1",
		},
		Config: api.ServerWireGuardInterface{
			Name:       "egtest",
			Addresses:  []string{"1.1.1.1/24"},
			ListenPort: "9090",
			PrivateKey: "0FsoFCq1EZb30fKvs8I43iCAof7v1pukGPsSEOAoH1g=",
			PublicKey:  "2SaLuA4nGVf77azT9kmW28V9DW/0aSbMH43GTt7rA3w=",
		},
	}
}

func TestRESTHandleCreateDevice(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupRESTHandlersLocalTest(t, ctx, q)

	apiUser := test.InsertSampleUser(t, ctx, q, test.TestUserName).ToAPI()

	addressPool1Params := test.NewSampleInsertAddressPoolParams("test-pool-1")
	addressPool1Params.StartAddr = netip.MustParseAddr("10.0.0.1")
	addressPool1Params.EndAddr = netip.MustParseAddr("10.0.0.100")
	addressPool1 := test.InsertSampleAddressPool(t, ctx, sqlcQ, addressPool1Params)

	addressPool2Params := test.NewSampleInsertAddressPoolParams("test-pool-2")
	addressPool2Params.StartAddr = netip.MustParseAddr("192.168.1.1")
	addressPool2Params.EndAddr = netip.MustParseAddr("192.168.1.100")
	addressPool2 := test.InsertSampleAddressPool(t, ctx, sqlcQ, addressPool2Params)

	deviceTemplate := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, apiUser.ID, []int{addressPool1.ID, addressPool2.ID})

	cases := []struct {
		name             string
		deviceTemplateID int
		expectStatus     int
		setupFunc        func()
	}{
		{
			name:             "valid device creation",
			deviceTemplateID: deviceTemplate.ID,
			expectStatus:     http.StatusCreated,
		},
		{
			name:             "device template not found",
			deviceTemplateID: 99999,
			expectStatus:     http.StatusNotFound,
		},
		{
			name:             "DPU limit exceeded",
			deviceTemplateID: deviceTemplate.ID,
			expectStatus:     http.StatusForbidden,
			setupFunc: func() {
				for i := 1; i < manager.Feats.GetDPULimit(); i++ { // creates DPULimit - 1 devices(1 device already created in the first test case)
					_, err := manager.CreateDeviceWithAutogenData(ctx, deviceTemplate.ID, "", "")
					require.NoError(t, err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup specific test conditions
			if tc.setupFunc != nil {
				tc.setupFunc()
			}

			beforeDeviceCount := test.CountTableRows(t, q.Conn, "devices")

			url := test.URLWithID(api.DevicesURL, fmt.Sprintf("%d", tc.deviceTemplateID))
			req, err := test.CreateRequest(http.MethodPost, url, nil, &apiUser)
			require.NoError(t, err)

			resp, err := httpClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			afterDeviceCount := test.CountTableRows(t, q.Conn, "devices")
			require.Equal(t, tc.expectStatus, resp.StatusCode)

			if tc.expectStatus != http.StatusCreated {
				require.Equal(t, beforeDeviceCount, afterDeviceCount)
				return
			}

			require.Equal(t, beforeDeviceCount+1, afterDeviceCount)

			devices, err := sqlcQ.GetDevicesByTemplateID(ctx, tc.deviceTemplateID)
			require.NoError(t, err)
			require.Greater(t, len(devices), 0)

			latestDevice := devices[len(devices)-1]
			require.Equal(t, tc.deviceTemplateID, latestDevice.DeviceTemplateID)
			require.NotEmpty(t, test.OpenString(latestDevice.PrivateKeyEncrypted))
			require.NotEmpty(t, latestDevice.PublicKey)

			addrs, err := sqlcQ.GetDeviceWireGuardIfaceAddrs(ctx, latestDevice.ID)
			require.NoError(t, err)
			require.Equal(t, 2, len(addrs)) // Should have addresses from both pools
		})
	}
}

func TestRESTHandleGetDevicesByTemplateID(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupRESTHandlersLocalTest(t, ctx, q)

	addressPool1Params := test.NewSampleInsertAddressPoolParams("get-devices-pool-1")
	addressPool1Params.StartAddr = netip.MustParseAddr("10.50.0.1")
	addressPool1Params.EndAddr = netip.MustParseAddr("10.50.0.100")
	addressPool1 := test.InsertSampleAddressPool(t, ctx, sqlcQ, addressPool1Params)

	addressPool2Params := test.NewSampleInsertAddressPoolParams("get-devices-pool-2")
	addressPool2Params.StartAddr = netip.MustParseAddr("192.168.50.1")
	addressPool2Params.EndAddr = netip.MustParseAddr("192.168.50.100")
	addressPool2 := test.InsertSampleAddressPool(t, ctx, sqlcQ, addressPool2Params)

	u1 := test.InsertSampleUser(t, ctx, q, "user-with-devices")
	dt1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, u1.ID, []int{addressPool1.ID, addressPool2.ID})
	device1 := test.InsertSampleDeviceAutoGenerated(t, ctx, manager, dt1.ID)
	device2 := test.InsertSampleDeviceAutoGenerated(t, ctx, manager, dt1.ID)

	// user without devices
	u2 := test.InsertSampleUser(t, ctx, q, "user-without-devices")
	dt2 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, u2.ID, []int{addressPool1.ID})

	cases := []struct {
		name            string
		user            api.User
		idParam         string
		expectStatus    int
		expectedDevices []*api.Device
	}{
		{
			name:            "get devices for template with devices",
			user:            u1.ToAPI(),
			idParam:         fmt.Sprintf("%d", dt1.ID),
			expectStatus:    http.StatusOK,
			expectedDevices: []*api.Device{device1, device2},
		},
		{
			name:            "get devices for template without devices",
			user:            u2.ToAPI(),
			idParam:         fmt.Sprintf("%d", dt2.ID),
			expectStatus:    http.StatusOK,
			expectedDevices: []*api.Device{},
		},
		{
			name:         "non-integer template ID",
			user:         u1.ToAPI(),
			idParam:      "abc",
			expectStatus: http.StatusBadRequest,
		},
		{
			name:         "non-existing template ID",
			user:         u1.ToAPI(),
			idParam:      "99999",
			expectStatus: http.StatusNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url := test.URLWithID(api.DevicesURL, tc.idParam)
			req, err := test.CreateRequest(http.MethodGet, url, nil, &tc.user)
			require.NoError(t, err)

			resp, err := httpClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, tc.expectStatus, resp.StatusCode)

			if tc.expectStatus == http.StatusOK {
				var deviceListResp api.DeviceListResponse
				err = json.NewDecoder(resp.Body).Decode(&deviceListResp)
				require.NoError(t, err)

				require.Len(t, deviceListResp.Devices, len(tc.expectedDevices))

				for _, expectedDevice := range tc.expectedDevices {
					found := false
					for _, d := range deviceListResp.Devices {
						if d.ID == expectedDevice.ID {
							found = true
							require.Equal(t, expectedDevice.ExternalDeviceID, d.ExternalDeviceID)
							require.Equal(t, expectedDevice.DeviceInformation, d.DeviceInformation)
							require.NotEmpty(t, d.PrivateKey)
							require.NotEmpty(t, d.PublicKey)
							require.Len(t, d.Addresses, 2)
							break
						}
					}
					require.True(t, found)
				}
			}
		})
	}
}

func TestRESTHandleResyncDevices(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupRESTHandlersLocalTest(t, ctx, q)

	addressPool1Params := test.NewSampleInsertAddressPoolParams("resync-pool-1")
	addressPool1Params.StartAddr = netip.MustParseAddr("10.100.0.1")
	addressPool1Params.EndAddr = netip.MustParseAddr("10.100.0.10")
	addressPool1 := test.InsertSampleAddressPool(t, ctx, sqlcQ, addressPool1Params)

	addressPool2Params := test.NewSampleInsertAddressPoolParams("resync-pool-2")
	addressPool2Params.StartAddr = netip.MustParseAddr("192.168.100.1")
	addressPool2Params.EndAddr = netip.MustParseAddr("192.168.100.10")
	addressPool2 := test.InsertSampleAddressPool(t, ctx, sqlcQ, addressPool2Params)

	addressPool3Params := test.NewSampleInsertAddressPoolParams("resync-pool-3-small")
	addressPool3Params.StartAddr = netip.MustParseAddr("172.16.0.1")
	addressPool3Params.EndAddr = netip.MustParseAddr("172.16.0.1")
	addressPool3 := test.InsertSampleAddressPool(t, ctx, sqlcQ, addressPool3Params)

	// devices with correct addresses
	u1 := test.InsertSampleUser(t, ctx, q, "resync-user1")
	dt1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, u1.ID, []int{addressPool1.ID, addressPool2.ID})
	_ = test.InsertSampleDeviceAutoGenerated(t, ctx, manager, dt1.ID)
	_ = test.InsertSampleDeviceAutoGenerated(t, ctx, manager, dt1.ID)

	// device with missing address
	u2 := test.InsertSampleUser(t, ctx, q, "resync-user2")
	dt2 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, u2.ID, []int{addressPool1.ID, addressPool2.ID})
	device2 := test.InsertSampleDeviceManual(t, ctx, sqlcQ, dt2.ID)
	_ = test.InsertDeviceAddress(t, ctx, sqlcQ, device2.ID, "10.100.0.5/24")

	// device with invalid address
	u3 := test.InsertSampleUser(t, ctx, q, "resync-user3")
	dt3 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, u3.ID, []int{addressPool1.ID, addressPool2.ID})
	device3 := test.InsertSampleDeviceManual(t, ctx, sqlcQ, dt3.ID)
	_ = test.InsertDeviceAddress(t, ctx, sqlcQ, device3.ID, "99.99.99.99/24")

	// device deletion due to exhausted pool
	u4 := test.InsertSampleUser(t, ctx, q, "resync-user4")
	dt4 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, u4.ID, []int{addressPool1.ID, addressPool3.ID})
	deviceThatUsesPool3 := test.InsertSampleDeviceManual(t, ctx, sqlcQ, dt4.ID)
	_ = test.InsertDeviceAddress(t, ctx, sqlcQ, deviceThatUsesPool3.ID, "172.16.0.1/24")
	_ = test.InsertDeviceAddress(t, ctx, sqlcQ, deviceThatUsesPool3.ID, "10.100.0.8/24")
	deviceToBeDeleted := test.InsertSampleDeviceManual(t, ctx, sqlcQ, dt4.ID)
	_ = test.InsertDeviceAddress(t, ctx, sqlcQ, deviceToBeDeleted.ID, "99.99.99.99/24")

	cases := []struct {
		name                   string
		user                   api.User
		idParam                string
		expectStatus           int
		expectedDevicesUpdated int
		expectedDevicesDeleted int
		expectedErrorCount     int
	}{
		{
			name:                   "devices with correct addresses - no changes needed",
			user:                   u1.ToAPI(),
			idParam:                fmt.Sprintf("%d", dt1.ID),
			expectStatus:           http.StatusOK,
			expectedDevicesUpdated: 0,
			expectedDevicesDeleted: 0,
			expectedErrorCount:     0,
		},
		{
			name:                   "device with missing address - should allocate",
			user:                   u2.ToAPI(),
			idParam:                fmt.Sprintf("%d", dt2.ID),
			expectStatus:           http.StatusOK,
			expectedDevicesUpdated: 1,
			expectedDevicesDeleted: 0,
			expectedErrorCount:     0,
		},
		{
			name:                   "device with invalid address - should fix",
			user:                   u3.ToAPI(),
			idParam:                fmt.Sprintf("%d", dt3.ID),
			expectStatus:           http.StatusOK,
			expectedDevicesUpdated: 1,
			expectedDevicesDeleted: 0,
			expectedErrorCount:     0,
		},
		{
			name:                   "device deleted due to exhausted pool",
			user:                   u4.ToAPI(),
			idParam:                fmt.Sprintf("%d", dt4.ID),
			expectStatus:           http.StatusOK,
			expectedDevicesUpdated: 0,
			expectedDevicesDeleted: 1,
			expectedErrorCount:     0,
		},
		{
			name:         "invalid ID (non-integer)",
			user:         u1.ToAPI(),
			idParam:      "abc",
			expectStatus: http.StatusBadRequest,
		},
		{
			name:         "device template not found",
			user:         u1.ToAPI(),
			idParam:      "99999",
			expectStatus: http.StatusNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url := test.URLWithID(api.DeviceTemplateResyncURL, tc.idParam)
			req, err := test.CreateRequest(http.MethodPost, url, nil, &tc.user)
			require.NoError(t, err)

			resp, err := httpClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, tc.expectStatus, resp.StatusCode)

			if tc.expectStatus != http.StatusOK {
				return
			}

			var resyncResponse api.DeviceResyncResponse
			err = json.NewDecoder(resp.Body).Decode(&resyncResponse)
			require.NoError(t, err)

			require.Equal(t, tc.expectedDevicesUpdated, resyncResponse.DevicesUpdated)
			require.Equal(t, tc.expectedDevicesDeleted, resyncResponse.DevicesDeleted)
			require.Len(t, resyncResponse.Errors, tc.expectedErrorCount)
		})
	}
}

func TestRESTHandleResyncDeviceAdjacencies(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupRESTHandlersLocalTest(t, ctx, q)

	s1 := test.NewSampleServer(t, "server1")
	wgConfig1 := test.NewSampleWGServerConfig()
	s1WithConfig := &api.ServerWithVPNConfig{
		Server: s1.ToAPI(),
		Config: *wgConfig1.ToAPI(),
	}
	err := vcmQ.CreateServer(ctx, s1WithConfig)
	require.NoError(t, err)

	s2 := test.NewSampleServer(t, "server2")
	wgConfig2 := test.NewSampleWGServerConfig()
	s2WithConfig := &api.ServerWithVPNConfig{
		Server: s2.ToAPI(),
		Config: *wgConfig2.ToAPI(),
	}
	err = vcmQ.CreateServer(ctx, s2WithConfig)
	require.NoError(t, err)

	servers, err := vcmQ.GetAllServers(ctx)
	require.NoError(t, err)
	require.Len(t, servers.Servers, 2)
	s1ID := servers.Servers[0].ID
	s2ID := servers.Servers[1].ID

	pool := test.InsertSampleAddressPool(t, ctx, sqlcQ, &sqlc.InsertAddressPoolParams{
		Name:        "adjacency-resync-pool",
		StartAddr:   netip.MustParseAddr("1.1.1.10"),
		EndAddr:     netip.MustParseAddr("1.1.1.100"),
		NetMask:     24,
		Description: "test pool matching server network",
	})

	// user with devices and templates - should create adjacencies
	u1 := test.InsertSampleUser(t, ctx, q, "user1")
	dt1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, u1.ID, []int{pool.ID})
	_ = test.InsertSampleDeviceAutoGenerated(t, ctx, manager, dt1.ID)
	_ = test.InsertSampleDeviceAutoGenerated(t, ctx, manager, dt1.ID)
	test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, s1ID, u1.ID)
	test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, s2ID, u1.ID)

	// user with device adjacencies but no adjacency templates - should delete adjacencies
	u2 := test.InsertSampleUser(t, ctx, q, "user2")
	dt2 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, u2.ID, []int{pool.ID})
	device2 := test.InsertSampleDeviceAutoGenerated(t, ctx, manager, dt2.ID)
	test.InsertSampleAdjacency(t, ctx, sqlcQ, s1ID, device2.ID)
	test.InsertSampleAdjacency(t, ctx, sqlcQ, s2ID, device2.ID)

	// user with existing adjacencies
	u3 := test.InsertSampleUser(t, ctx, q, "user3")
	dt3 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, u3.ID, []int{pool.ID})
	device3 := test.InsertSampleDeviceAutoGenerated(t, ctx, manager, dt3.ID)
	test.InsertSampleAdjacencyTemplate(t, ctx, sqlcQ, s1ID, u3.ID)
	test.InsertSampleAdjacency(t, ctx, sqlcQ, s1ID, device3.ID)

	// user with device but no adjacency templates and no device adjacencies - should do nothing
	u4 := test.InsertSampleUser(t, ctx, q, "user4")
	dt4 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, u4.ID, []int{pool.ID})
	_ = test.InsertSampleDeviceAutoGenerated(t, ctx, manager, dt4.ID)

	cases := []struct {
		name            string
		user            api.User
		idParam         string
		expectStatus    int
		expectedCreated int
		expectedUpdated int
		expectedDeleted int
		expectErrors    bool
	}{
		{
			name:            "create adjacencies from templates",
			user:            u1.ToAPI(),
			idParam:         fmt.Sprintf("%d", u1.ID),
			expectStatus:    http.StatusOK,
			expectedCreated: 4, // 2 devices × 2 templates
			expectedUpdated: 0,
			expectedDeleted: 0,
			expectErrors:    false,
		},
		{
			name:            "no templates - delete existing adjacencies",
			user:            u2.ToAPI(),
			idParam:         fmt.Sprintf("%d", u2.ID),
			expectStatus:    http.StatusOK,
			expectedCreated: 0,
			expectedUpdated: 0,
			expectedDeleted: 2, // 2 adjacencies without matching templates
			expectErrors:    false,
		},
		{
			name:            "adjacency already exists - gets updated",
			user:            u3.ToAPI(),
			idParam:         fmt.Sprintf("%d", u3.ID),
			expectStatus:    http.StatusOK,
			expectedCreated: 0,
			expectedUpdated: 1, // existing adjacency has wrong IPs, will be updated
			expectedDeleted: 0,
			expectErrors:    false,
		},
		{
			name:            "no templates and no adjacencies - no changes",
			user:            u4.ToAPI(),
			idParam:         fmt.Sprintf("%d", u4.ID),
			expectStatus:    http.StatusOK,
			expectedCreated: 0,
			expectedUpdated: 0,
			expectedDeleted: 0,
			expectErrors:    false,
		},
		{
			name:         "invalid ID (non-integer)",
			user:         u1.ToAPI(),
			idParam:      "abc",
			expectStatus: http.StatusBadRequest,
		},
		{
			name:         "user not found",
			user:         u1.ToAPI(),
			idParam:      "99999",
			expectStatus: http.StatusNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url := test.URLWithID(api.UserDeviceAdjacenciesResyncURL, tc.idParam)
			req, err := test.CreateRequest(http.MethodPost, url, nil, &tc.user)
			require.NoError(t, err)

			resp, err := httpClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, tc.expectStatus, resp.StatusCode)

			var resyncResponse api.DeviceAdjacencyResyncResponse
			err = json.NewDecoder(resp.Body).Decode(&resyncResponse)
			require.NoError(t, err)

			require.Equal(t, tc.expectedCreated, resyncResponse.AdjacenciesCreated)
			require.Equal(t, tc.expectedUpdated, resyncResponse.AdjacenciesUpdated)
			require.Equal(t, tc.expectedDeleted, resyncResponse.AdjacenciesDeleted)

			if tc.expectErrors {
				require.NotEmpty(t, resyncResponse.Errors)
			} else {
				require.Empty(t, resyncResponse.Errors)
			}
		})
	}
}

func TestRESTHandleGetDeviceAdjacenciesByUserID(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupRESTHandlersLocalTest(t, ctx, q)

	addressPoolParams := test.NewSampleInsertAddressPoolParams("adj-pool")
	addressPoolParams.StartAddr = netip.MustParseAddr("10.200.0.1")
	addressPoolParams.EndAddr = netip.MustParseAddr("10.200.0.100")
	addressPool := test.InsertSampleAddressPool(t, ctx, sqlcQ, addressPoolParams)

	s1 := test.NewSampleServer(t, "server1-adj")
	wgConfig1 := test.NewSampleWGServerConfig()
	s1WithConfig := &api.ServerWithVPNConfig{
		Server: s1.ToAPI(),
		Config: *wgConfig1.ToAPI(),
	}
	err := vcmQ.CreateServer(ctx, s1WithConfig)
	require.NoError(t, err)

	s2 := test.NewSampleServer(t, "server2-adj")
	wgConfig2 := test.NewSampleWGServerConfig()
	s2WithConfig := &api.ServerWithVPNConfig{
		Server: s2.ToAPI(),
		Config: *wgConfig2.ToAPI(),
	}
	err = vcmQ.CreateServer(ctx, s2WithConfig)
	require.NoError(t, err)

	servers, err := vcmQ.GetAllServers(ctx)
	require.NoError(t, err)
	require.Len(t, servers.Servers, 2)
	s1ID := servers.Servers[0].ID
	s2ID := servers.Servers[1].ID

	// user with devices and adjacencies
	u1 := test.InsertSampleUser(t, ctx, q, "user-with-adjacencies")
	dt1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, u1.ID, []int{addressPool.ID})
	device1 := test.InsertSampleDeviceAutoGenerated(t, ctx, manager, dt1.ID)
	device2 := test.InsertSampleDeviceAutoGenerated(t, ctx, manager, dt1.ID)
	adj1 := test.InsertSampleAdjacency(t, ctx, sqlcQ, s1ID, device1.ID)
	adj2 := test.InsertSampleAdjacency(t, ctx, sqlcQ, s2ID, device1.ID)
	adj3 := test.InsertSampleAdjacency(t, ctx, sqlcQ, s1ID, device2.ID)

	// expected adjacencies for user1
	config1 := &api.WireGuardPeerConfig{
		PublicKey:           wgConfig1.PublicKey,
		PresharedKey:        test.OpenString(adj1.PresharedKeyEncrypted),
		AllowedIPs:          []string{"1.1.1.1/32"},
		OtherSideAllowedIPs: []string{"1.1.1.1/32"},
		Endpoint:            fmt.Sprintf("%s:%d", servers.Servers[0].Endpoint, wgConfig1.ListenPort),
	}
	config2 := &api.WireGuardPeerConfig{
		PublicKey:           wgConfig2.PublicKey,
		PresharedKey:        test.OpenString(adj2.PresharedKeyEncrypted),
		AllowedIPs:          []string{"1.1.1.1/32"},
		OtherSideAllowedIPs: []string{"1.1.1.1/32"},
		Endpoint:            fmt.Sprintf("%s:%d", servers.Servers[1].Endpoint, wgConfig2.ListenPort),
	}
	config3 := &api.WireGuardPeerConfig{
		PublicKey:           wgConfig1.PublicKey,
		PresharedKey:        test.OpenString(adj3.PresharedKeyEncrypted),
		AllowedIPs:          []string{"1.1.1.1/32"},
		OtherSideAllowedIPs: []string{"1.1.1.1/32"},
		Endpoint:            fmt.Sprintf("%s:%d", servers.Servers[0].Endpoint, wgConfig1.ListenPort),
	}
	expectedAdj1 := api.AdjacencyExtended{
		ID:     adj1.ID,
		Server: &servers.Servers[0],
		Device: device1,
		Config: config1,
	}
	expectedAdj2 := api.AdjacencyExtended{
		ID:     adj2.ID,
		Server: &servers.Servers[1],
		Device: device1,
		Config: config2,
	}
	expectedAdj3 := api.AdjacencyExtended{
		ID:     adj3.ID,
		Server: &servers.Servers[0],
		Device: device2,
		Config: config3,
	}

	// user with devices but no adjacencies
	u2 := test.InsertSampleUser(t, ctx, q, "user-no-adjacencies")
	dt2 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, u2.ID, []int{addressPool.ID})
	_ = test.InsertSampleDeviceAutoGenerated(t, ctx, manager, dt2.ID)

	// user with no devices
	u3 := test.InsertSampleUser(t, ctx, q, "user-no-devices")
	_ = test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, u3.ID, []int{addressPool.ID})

	cases := []struct {
		name                string
		user                api.User
		idParam             string
		expectStatus        int
		expectedAdjacencies []api.AdjacencyExtended
	}{
		{
			name:                "get adjacencies for user with devices and adjacencies",
			user:                u1.ToAPI(),
			idParam:             fmt.Sprintf("%d", u1.ID),
			expectStatus:        http.StatusOK,
			expectedAdjacencies: []api.AdjacencyExtended{expectedAdj1, expectedAdj2, expectedAdj3}, // 2 adjacencies for device1, 1 for device2
		},
		{
			name:                "get adjacencies for user with devices but no adjacencies",
			user:                u2.ToAPI(),
			idParam:             fmt.Sprintf("%d", u2.ID),
			expectStatus:        http.StatusOK,
			expectedAdjacencies: []api.AdjacencyExtended{},
		},
		{
			name:                "get adjacencies for user with no devices",
			user:                u3.ToAPI(),
			idParam:             fmt.Sprintf("%d", u3.ID),
			expectStatus:        http.StatusOK,
			expectedAdjacencies: []api.AdjacencyExtended{},
		},
		{
			name:         "invalid ID (non-integer)",
			user:         u1.ToAPI(),
			idParam:      "abc",
			expectStatus: http.StatusBadRequest,
		},
		{
			name:         "user not found",
			user:         u1.ToAPI(),
			idParam:      "99999",
			expectStatus: http.StatusNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url := test.URLWithID(api.VPNClientAdjacenciesURL, tc.idParam)
			req, err := test.CreateRequest(http.MethodGet, url, nil, &tc.user)
			require.NoError(t, err)

			resp, err := httpClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, tc.expectStatus, resp.StatusCode)

			if tc.expectStatus == http.StatusOK {
				var adjListResp api.AdjacencyList
				err = json.NewDecoder(resp.Body).Decode(&adjListResp)
				require.NoError(t, err)
				require.Len(t, adjListResp.Adjacencies, len(tc.expectedAdjacencies))

				if len(tc.expectedAdjacencies) > 0 {
					require.Equal(t, tc.expectedAdjacencies, adjListResp.Adjacencies)
				}
			}
		})
	}
}
