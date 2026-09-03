/*
 * Copyright 2023 PANTHEON.tech s.r.o.
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

package postgres_test

import (
	"fmt"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/entguard/entguard/pkg/test"
	"github.com/entguard/entguard/service/api/v1"
	pg "github.com/entguard/entguard/service/vcm/drivers/postgres"
)

const (
	longServerName        = "more than fifty characters more than fifty characters"
	longServerDescription = `description is more than two hundred fifty characters long
							  description is more than two hundred fifty characters long 
	                          description is more than two hundred fifty characters long
	                          description is more than two hundred fifty characters long`
)

func TestNewServerFromAPI(t *testing.T) {
	input := createTestAPIServer("server1", "172.18.0.1", "test server")

	endpoint, err := netip.ParseAddr(input.Endpoint)
	require.NoError(t, err)

	output := pg.Server{
		Name:        input.Name,
		Endpoint:    endpoint,
		Description: input.Description,
	}

	cases := []struct {
		name      string
		apiServer *api.ServerWithVPNConfig
		expected  pg.Server
		err       error
	}{
		{
			name:      "new server is created",
			apiServer: input,
			expected:  output,
		},
		{
			name:      "no server name",
			apiServer: createTestAPIServer("", input.Endpoint, ""),
			err:       pg.ErrMissingServerName,
		},
		{
			name:      "no server endpoint",
			apiServer: createTestAPIServer(input.Name, "", ""),
			err:       pg.ErrMissingServerEndpoint,
		},
		{
			name:      "server name > 50 char",
			apiServer: createTestAPIServer(longServerName, input.Endpoint, ""),
			err:       pg.ErrTooLongServerName,
		},
		{
			name:      "server description > 250 char",
			apiServer: createTestAPIServer(input.Name, input.Endpoint, longServerDescription),
			err:       pg.ErrTooLongServerDescr,
		},
		{
			name:      "invalid server endpoint",
			apiServer: createTestAPIServer(input.Name, "1", ""),
			err:       pg.ErrInvalidServerEndpoint,
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("Test %d %s", i, c.name), func(t *testing.T) {
			s, err := pg.NewServerFromAPI(c.apiServer)

			if c.err != nil {
				require.Empty(t, s)
				require.Equal(t, c.err, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, c.expected.Name, s.Name)
			require.Equal(t, c.expected.Endpoint, s.Endpoint)
			require.Equal(t, c.expected.Description, s.Description)
		})
	}
}

func TestSetName(t *testing.T) {
	server := pg.Server{}

	cases := []struct {
		name       string
		serverName string
		err        error
	}{
		{
			name:       "set new server name",
			serverName: "testName",
		},
		{
			name:       "no server name",
			serverName: "",
			err:        pg.ErrMissingServerName,
		},
		{
			name:       "server name > 50 char",
			serverName: longServerName,
			err:        pg.ErrTooLongServerName,
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("Test %d %s", i, c.name), func(t *testing.T) {
			snBefore := server.Name

			err := server.SetName(c.serverName)

			if c.err != nil {
				require.Equal(t, c.err, err)
				require.Equal(t, snBefore, server.Name)
				return
			}

			require.NoError(t, err)
			require.Equal(t, c.serverName, server.Name)
		})
	}
}

func TestSetEndpoint(t *testing.T) {
	server := pg.Server{}

	cases := []struct {
		name     string
		endpoint string
		err      error
	}{
		{
			name:     "set new server endpoint",
			endpoint: "1.1.1.1",
		},
		{
			name:     "no server endpoint",
			endpoint: "",
			err:      pg.ErrMissingServerEndpoint,
		},
		{
			name:     "invalid server endpoint",
			endpoint: "1:1",
			err:      pg.ErrInvalidServerEndpoint,
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("Test %d %s", i, c.name), func(t *testing.T) {
			seBefore := server.Endpoint

			err := server.SetEndpoint(c.endpoint)

			if c.err != nil {
				require.Equal(t, c.err, err)
				require.Equal(t, seBefore, server.Endpoint)
				return
			}

			require.NoError(t, err)

			endpoint, err := netip.ParseAddr(c.endpoint)
			require.NoError(t, err)

			require.Equal(t, endpoint, server.Endpoint)
		})
	}
}

func TestSetDescription(t *testing.T) {
	server := pg.Server{}

	cases := []struct {
		name        string
		description string
		err         error
	}{
		{
			name:        "set new server description",
			description: "1.1.1.1",
		},
		{
			name:        "no server description",
			description: "",
		},
		{
			name:        "server description > 250 char",
			description: longServerDescription,
			err:         pg.ErrTooLongServerDescr,
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("Test %d %s", i, c.name), func(t *testing.T) {
			seBefore := server.Description

			err := server.SetDescription(c.description)

			if c.err != nil {
				require.Equal(t, c.err, err)
				require.Equal(t, seBefore, server.Description)
				return
			}

			require.NoError(t, err)
			require.Equal(t, c.description, server.Description)
		})
	}
}

func TestToAPI(t *testing.T) {
	s1 := test.NewSampleServer(t, "server1")
	s1.ID = 1
	s2 := test.NewSampleServerWithoutHC(t, "server2")
	s2.ID = 2

	cases := []struct {
		name   string
		server *pg.Server
	}{
		{
			name:   "server with healthcheck",
			server: s1,
		},
		{
			name:   "server without healthcheck",
			server: s2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sAPI := tc.server.ToAPI()

			apiEndpoint, err := netip.ParseAddr(sAPI.Endpoint)
			require.NoError(t, err)

			var apiHCAddress *netip.Addr
			if sAPI.HealthCheckAddress != "" {
				parsed, err := netip.ParseAddr(sAPI.HealthCheckAddress)
				require.NoError(t, err)
				apiHCAddress = &parsed
			}

			require.Equal(t, tc.server.Name, sAPI.Name)
			require.Equal(t, tc.server.ID, sAPI.ID)
			require.Equal(t, tc.server.Endpoint, apiEndpoint)
			require.Equal(t, tc.server.Description, sAPI.Description)
			require.Equal(t, tc.server.HealthCheckAddress, apiHCAddress)
		})
	}
}

func createTestAPIServer(name, endpoint, descr string) *api.ServerWithVPNConfig {
	return &api.ServerWithVPNConfig{
		Server: api.Server{
			Name:        name,
			Endpoint:    endpoint,
			Description: descr,
		},
		Config: api.ServerWireGuardInterface{
			PrivateKey: "privatekey",
			Addresses:  []string{"1.1.1.7/24", "1.1.1.4/24"},
			ListenPort: "8080",
			DNS:        []string{"test dns1"},
			MTU:        "9000",
		},
	}
}
