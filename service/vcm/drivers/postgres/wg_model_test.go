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

package postgres_test

import (
	"fmt"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	sqlc "github.com/entguard/entguard/db"
	"github.com/entguard/entguard/service/api/v1"
	pg "github.com/entguard/entguard/service/vcm/drivers/postgres"
)

func TestNewWireGuardConfigFromServerAPI(t *testing.T) {
	cases := []struct {
		name          string
		wgI           *api.ServerWireGuardInterface
		expectFailure bool
	}{
		{
			name: "success",
			wgI: &api.ServerWireGuardInterface{
				Name:       "eg0",
				PrivateKey: "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				PublicKey:  "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				Addresses:  []string{"0.0.0.0/22"},
				ListenPort: "77",
			},
			expectFailure: false,
		},
		{
			name: "missing name",
			wgI: &api.ServerWireGuardInterface{
				PrivateKey: "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				PublicKey:  "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				Addresses:  []string{"0.0.0.0/22"},
				ListenPort: "77",
			},
			expectFailure: true,
		},
		{
			name: "tab in the name",
			wgI: &api.ServerWireGuardInterface{
				Name:       "some	eryery",
				PrivateKey: "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				PublicKey:  "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				Addresses:  []string{"0.0.0.0/22"},
				ListenPort: "77",
			},
			expectFailure: true,
		},
		{
			name: "name length is 16",
			wgI: &api.ServerWireGuardInterface{
				Name:       "1616161616161616",
				PrivateKey: "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				PublicKey:  "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				Addresses:  []string{"0.0.0.0/22"},
				ListenPort: "77",
			},
			expectFailure: true,
		},
		{
			name: "name length is 15 - success",
			wgI: &api.ServerWireGuardInterface{
				Name:       "151515151515151",
				PrivateKey: "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				PublicKey:  "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				Addresses:  []string{"0.0.0.0/22"},
				ListenPort: "77",
			},
			expectFailure: false,
		},
		{
			name: "missing public key",
			wgI: &api.ServerWireGuardInterface{
				Name:       "eg0",
				PrivateKey: "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				Addresses:  []string{"0.0.0.0/22"},
				ListenPort: "77",
			},
			expectFailure: true,
		},
		{
			name: "public key length is bigger than 32",
			wgI: &api.ServerWireGuardInterface{
				Name:       "eg0",
				PrivateKey: "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				PublicKey:  "YXNkZmFzZGZhc2RmYXNkZmFzZGZhc2RmYXNkZmFzZGZo",
				Addresses:  []string{"0.0.0.0/22"},
				ListenPort: "77",
			},
			expectFailure: true,
		},
		{
			name: "invalid base64",
			wgI: &api.ServerWireGuardInterface{
				Name:       "eg0",
				PrivateKey: "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				PublicKey:  "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkvTIql5XCUM",
				Addresses:  []string{"0.0.0.0/22"},
				ListenPort: "77",
			},
			expectFailure: true,
		},
		{
			name: "missing private key",
			wgI: &api.ServerWireGuardInterface{
				Name:       "eg0",
				PublicKey:  "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				Addresses:  []string{"0.0.0.0/22"},
				ListenPort: "77",
			},
			expectFailure: true,
		},
		{
			name: "private key has `-` in it",
			wgI: &api.ServerWireGuardInterface{
				Name:       "eg0",
				PrivateKey: "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql-XCUM=",
				PublicKey:  "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				Addresses:  []string{"0.0.0.0/22"},
				ListenPort: "77",
			},
			expectFailure: true,
		},
		{
			name: "private key has `+` in it - success",
			wgI: &api.ServerWireGuardInterface{
				Name:       "eg0",
				PrivateKey: "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql+XCUM=",
				PublicKey:  "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				Addresses:  []string{"0.0.0.0/22"},
				ListenPort: "77",
			},
			expectFailure: false,
		},
		{
			name: "missing addresses",
			wgI: &api.ServerWireGuardInterface{
				Name:       "eg0",
				PrivateKey: "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				PublicKey:  "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				ListenPort: "77",
			},
			expectFailure: true,
		},
		{
			name: "empty addresses",
			wgI: &api.ServerWireGuardInterface{
				Name:       "eg0",
				PrivateKey: "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				PublicKey:  "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				Addresses:  []string{""},
				ListenPort: "77",
			},
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := pg.NewWireGuardConfigFromAPI(tc.wgI)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestNewDeviceTemplateFromAPI(t *testing.T) {
	ap := api.AddressPool{
		ID:          1,
		Name:        "address pool name",
		Description: "address pool desc",
		StartAddr:   "0.0.0.0",
		EndAddr:     "0.0.3.255",
		NetMask:     22,
	}

	cases := []struct {
		name           string
		deviceTemplate *api.DeviceTemplate
		expectFailure  bool
	}{
		{
			name: "success",
			deviceTemplate: &api.DeviceTemplate{
				InterfaceName: "eg0",
				AddressPools:  []*api.AddressPool{&ap},
				ListenPort:    "77",
			},
			expectFailure: false,
		},
		{
			name: "missing interface name",
			deviceTemplate: &api.DeviceTemplate{
				AddressPools: []*api.AddressPool{&ap},
				ListenPort:   "77",
			},
			expectFailure: true,
		},
		{
			name: "tab in the name",
			deviceTemplate: &api.DeviceTemplate{
				InterfaceName: "some	eryery",
				AddressPools:  []*api.AddressPool{&ap},
				ListenPort:    "77",
			},
			expectFailure: true,
		},
		{
			name: "name length is 16",
			deviceTemplate: &api.DeviceTemplate{
				InterfaceName: "1616161616161616",
				AddressPools:  []*api.AddressPool{&ap},
				ListenPort:    "77",
			},
			expectFailure: true,
		},
		{
			name: "name length is 15 - success",
			deviceTemplate: &api.DeviceTemplate{
				InterfaceName: "151515151515151",
				AddressPools:  []*api.AddressPool{&ap},
				ListenPort:    "77",
			},
			expectFailure: false,
		},
		{
			name: "missing address pools",
			deviceTemplate: &api.DeviceTemplate{
				InterfaceName: "eg0",
				ListenPort:    "77",
			},
			expectFailure: true,
		},
		{
			name: "empty address pools",
			deviceTemplate: &api.DeviceTemplate{
				InterfaceName: "eg0",
				AddressPools:  []*api.AddressPool{},
				ListenPort:    "77",
			},
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := pg.NewDeviceTemplateFromAPI(tc.deviceTemplate)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestNewWireGuardAdjacencyConfigFromAPI(t *testing.T) {
	cases := []struct {
		name          string
		adjacency     *api.WireGuardAdjacency
		want          *pg.WireGuardAdjacencyConfig
		expectFailure bool
	}{
		{
			name: "adjacency with nil allowed IPs list",
			adjacency: &api.WireGuardAdjacency{
				ServerSide:   false,
				PresharedKey: "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				AllowedIPs:   nil,
			},
			expectFailure: true,
		},
		{
			name: "adjacency with empty allowed IPs list",
			adjacency: &api.WireGuardAdjacency{
				ServerSide:   false,
				PresharedKey: "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				AllowedIPs:   []string{},
			},
			expectFailure: true,
		},
		{
			name: "allowed IPs list with bad address",
			adjacency: &api.WireGuardAdjacency{
				ServerSide:   false,
				PresharedKey: "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				AllowedIPs:   []string{"1.1.1.1/10", "bad address"},
			},
			expectFailure: true,
		},
		{
			name: "adjacency with valid allowed IPs list",
			adjacency: &api.WireGuardAdjacency{
				ServerSide:   false,
				PresharedKey: "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				AllowedIPs:   []string{"1.1.1.1/32", "1.1.1.1/0", "1.1.1.1"},
			},
			want: &pg.WireGuardAdjacencyConfig{
				PresharedKey: "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				ClientSideAllowedIPs: []netip.Prefix{
					netip.MustParsePrefix("1.1.1.1/32"),
					netip.MustParsePrefix("0.0.0.0/0"),
				},
			},
			expectFailure: false,
		},
		{
			name: "server side is activated without other side allowed IPs list",
			adjacency: &api.WireGuardAdjacency{
				ServerSide:   true,
				PresharedKey: "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				AllowedIPs:   []string{"1.1.1.1/32", "1.1.1.1/0", "1.1.1.1"},
			},
			expectFailure: true,
		},
		{
			name: "server side is activated with empty other side allowed IPs list",
			adjacency: &api.WireGuardAdjacency{
				ServerSide:          true,
				PresharedKey:        "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				AllowedIPs:          []string{"1.1.1.1/32", "1.1.1.1/0", "1.1.1.1"},
				OtherSideAllowedIPs: []string{},
			},
			expectFailure: true,
		},
		{
			name: "server side is activated with bad address in other side allowed IPs list",
			adjacency: &api.WireGuardAdjacency{
				ServerSide:          true,
				PresharedKey:        "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				AllowedIPs:          []string{"1.1.1.1/32", "1.1.1.1/0", "1.1.1.1"},
				OtherSideAllowedIPs: []string{"1.1.1.1/10", "bad address"},
			},
			expectFailure: true,
		},
		{
			name: "server side is activated with valid addresses in other side allowed IPs list",
			adjacency: &api.WireGuardAdjacency{
				ServerSide:          true,
				PresharedKey:        "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				AllowedIPs:          []string{"1.1.1.1/32", "1.1.1.1/0", "1.1.1.1"},
				OtherSideAllowedIPs: []string{"1.1.1.1/32", "1.1.1.1/0", "1.1.1.1"},
			},
			want: &pg.WireGuardAdjacencyConfig{
				PresharedKey: "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				ServerSideAllowedIPs: []netip.Prefix{
					netip.MustParsePrefix("1.1.1.1/32"),
					netip.MustParsePrefix("0.0.0.0/0"),
				},
				ClientSideAllowedIPs: []netip.Prefix{
					netip.MustParsePrefix("1.1.1.1/32"),
					netip.MustParsePrefix("0.0.0.0/0"),
				},
			},
			expectFailure: false,
		},
		{
			name: "server side is not activated but with valid addresses in other side allowed IPs list",
			adjacency: &api.WireGuardAdjacency{
				ServerSide:          false,
				PresharedKey:        "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				AllowedIPs:          []string{"1.1.1.1/32", "1.1.1.1/0", "1.1.1.1"},
				OtherSideAllowedIPs: []string{"1.1.1.1/32", "1.1.1.1/0", "1.1.1.1"},
			},
			want: &pg.WireGuardAdjacencyConfig{
				PresharedKey: "bg4ZVkvs0YAfbLol3E4z745FT0lI15HkNvTIql5XCUM=",
				ClientSideAllowedIPs: []netip.Prefix{
					netip.MustParsePrefix("1.1.1.1/32"),
					netip.MustParsePrefix("0.0.0.0/0"),
				},
			},
			expectFailure: false,
		},
		{
			name: "adjacency without preshared key",
			adjacency: &api.WireGuardAdjacency{
				ServerSide: false,
				AllowedIPs: []string{"1.1.1.1/32", "1.1.1.1/0", "1.1.1.1"},
			},
			want: &pg.WireGuardAdjacencyConfig{
				ClientSideAllowedIPs: []netip.Prefix{
					netip.MustParsePrefix("1.1.1.1/32"),
					netip.MustParsePrefix("0.0.0.0/0"),
				},
			},
			expectFailure: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := pg.NewWireGuardAdjacencyConfigFromAPI(tc.adjacency)
			if tc.expectFailure {
				require.Error(t, err)
				require.Nil(t, result)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, tc.want, result)
		})
	}
}

func TestNewWireGuardAdjacencyTemplateConfigFromAPI(t *testing.T) {
	cases := []struct {
		name              string
		adjacencyTemplate *api.WireGuardAdjacencyTemplate
		want              *pg.WireGuardAdjacencyTemplateConfig
		expectFailure     bool
	}{
		{
			name: "adjacency with nil allowed IPs list",
			adjacencyTemplate: &api.WireGuardAdjacencyTemplate{
				UsePresharedKey:      true,
				ClientSideAllowedIPs: nil,
			},
			expectFailure: true,
		},
		{
			name: "adjacency with empty allowed IPs list",
			adjacencyTemplate: &api.WireGuardAdjacencyTemplate{
				UsePresharedKey:      true,
				ClientSideAllowedIPs: []string{},
			},
			expectFailure: true,
		},
		{
			name: "allowed IPs list with bad address",
			adjacencyTemplate: &api.WireGuardAdjacencyTemplate{
				UsePresharedKey:      true,
				ClientSideAllowedIPs: []string{"1.1.1.1/10", "bad address"},
			},
			expectFailure: true,
		},
		{
			name: "adjacency with valid allowed IPs list",
			adjacencyTemplate: &api.WireGuardAdjacencyTemplate{
				UsePresharedKey:      true,
				ClientSideAllowedIPs: []string{"1.1.1.1/32", "1.1.1.1/0", "1.1.1.1"},
			},
			want: &pg.WireGuardAdjacencyTemplateConfig{
				UsePresharedKey: true,
				ClientSideAllowedIPs: []netip.Prefix{
					netip.MustParsePrefix("1.1.1.1/32"),
					netip.MustParsePrefix("0.0.0.0/0"),
				},
			},
			expectFailure: false,
		},
		{
			name: "adjacency without preshared key",
			adjacencyTemplate: &api.WireGuardAdjacencyTemplate{
				UsePresharedKey:      false,
				ClientSideAllowedIPs: []string{"1.1.1.1/32", "1.1.1.1/0", "1.1.1.1"},
			},
			want: &pg.WireGuardAdjacencyTemplateConfig{
				UsePresharedKey: false,
				ClientSideAllowedIPs: []netip.Prefix{
					netip.MustParsePrefix("1.1.1.1/32"),
					netip.MustParsePrefix("0.0.0.0/0"),
				},
			},
			expectFailure: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := pg.NewWireGuardAdjacencyTemplateConfigFromAPI(tc.adjacencyTemplate)
			if tc.expectFailure {
				require.Error(t, err)
				require.Nil(t, result)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, tc.want, result)
		})
	}
}

func TestParseIPNet(t *testing.T) {
	cases := []struct {
		Name          string
		Input         string
		Want          string
		ExpectFailure bool
	}{
		{"IPv4 (no action)", "10.10.10.0/24", "10.10.10.0/24", false},
		{"IPv4 (returns network)", "10.10.10.5/24", "10.10.10.0/24", false},
		{"IPv4 (add /32 mask)", "10.10.10.5", "10.10.10.5/32", false},
		{"IPv6 (no action)", "2001:db8::/32", "2001:db8::/32", false},
		{"IPv6 (returns network)", "2001:db8:5a3::/32", "2001:db8::/32", false},
		{"IPv6 (add /128 mask)", "2001:db8::2021", "2001:db8::2021/128", false},
		{"empty string", "", "", true},
		{"not an IP", "not an IP", "", true},
		{"invalid IPv4", "10.10.10.789", "", true},
		{"invalid IPv6", "2001:yyy::/32", "", true},
	}

	for _, td := range cases {
		t.Run(td.Name, func(t *testing.T) {
			parseResult, err := pg.ParseIPNet(td.Input)
			if td.ExpectFailure {
				require.Error(t, err, fmt.Sprintf("ParseIPNet(%s)", td.Input))
				return
			}
			require.NoError(t, err, fmt.Sprintf("ParseIPNet(%s)", td.Input))
			require.Equal(t, td.Want, parseResult.String())
		})
	}
}

func TestWgcToAPI(t *testing.T) {
	wgc := &pg.WireGuardConfig{
		InterfaceName:       "egtest",
		PrivateKey:          "privatekey",
		PublicKey:           "publickey",
		Addresses:           []string{"1.1.1.1/24", "1.2.3.4/18"},
		ListenPort:          -1,
		DNS:                 []string{"test dns1"},
		MTU:                 -1,
		PersistentKeepalive: -1,
	}

	wgI := wgc.ToAPI()
	require.Equal(t, wgc.InterfaceName, wgI.Name)
	require.Equal(t, wgc.PrivateKey, wgI.PrivateKey)
	require.Equal(t, wgc.PublicKey, wgI.PublicKey)
	require.Equal(t, wgc.Addresses, wgI.Addresses)
	require.Equal(t, wgc.DNS, wgI.DNS)

	require.Empty(t, wgI.ListenPort)
	require.Empty(t, wgI.MTU)
	require.Empty(t, wgI.PersistentKeepalive)

	wgc.ListenPort = 9090
	wgc.MTU = 100
	wgc.PersistentKeepalive = 60
	wgI = wgc.ToAPI()

	require.Equal(t, fmt.Sprint(wgc.ListenPort), wgI.ListenPort)
	require.Equal(t, fmt.Sprint(wgc.MTU), wgI.MTU)
	require.Equal(t, fmt.Sprint(wgc.PersistentKeepalive), wgI.PersistentKeepalive)
}

func TestDtToAPI(t *testing.T) {
	dt := &pg.DeviceTemplate{
		ID:            5,
		InterfaceName: "egtest",
		AddressPools: []*sqlc.AddressPool{
			{
				ID:          20,
				Name:        "name1",
				Description: "desc1",
				StartAddr:   netip.MustParseAddr("1.1.0.0"),
				EndAddr:     netip.MustParseAddr("1.1.255.255"),
				NetMask:     16,
			},
			{
				ID:          30,
				Name:        "name2",
				Description: "",
				StartAddr:   netip.MustParseAddr("1.2.3.10"),
				EndAddr:     netip.MustParseAddr("1.2.3.99"),
				NetMask:     24,
			},
		},
		ListenPort: -1,
		DNS:        []string{"test dns1"},
		MTU:        -1,
	}

	apiDT := dt.ToAPI()
	require.Equal(t, dt.ID, apiDT.ID)
	require.Equal(t, dt.InterfaceName, apiDT.InterfaceName)
	require.Equal(t, dt.DNS, apiDT.DNS)

	require.NotEmpty(t, apiDT.AddressPools)
	require.Len(t, apiDT.AddressPools, len(dt.AddressPools))
	for i := range apiDT.AddressPools {
		require.Equal(t, apiDT.AddressPools[i].ID, dt.AddressPools[i].ID)
		require.Equal(t, apiDT.AddressPools[i].Name, dt.AddressPools[i].Name)
		require.Equal(t, apiDT.AddressPools[i].Description, dt.AddressPools[i].Description)
		require.Equal(t, apiDT.AddressPools[i].StartAddr, dt.AddressPools[i].StartAddr.String())
		require.Equal(t, apiDT.AddressPools[i].EndAddr, dt.AddressPools[i].EndAddr.String())
		require.Equal(t, apiDT.AddressPools[i].NetMask, dt.AddressPools[i].NetMask)
	}

	require.Empty(t, apiDT.ListenPort)
	require.Empty(t, apiDT.MTU)

	dt.ListenPort = 9090
	dt.MTU = 100
	apiDT = dt.ToAPI()

	require.Equal(t, fmt.Sprint(dt.ListenPort), apiDT.ListenPort)
	require.Equal(t, fmt.Sprint(dt.MTU), apiDT.MTU)
}

func TestWgaToAPI(t *testing.T) {
	wgp := &pg.WireGuardAdjacency{
		PublicKey:           "publickey",
		PresharedKey:        "presharedkey",
		AllowedIPs:          []string{"1.1.1.1/24", "1.2.3.4/18"},
		ListenPort:          -1,
		PersistentKeepalive: -1,
		OtherSideAllowedIPs: []string{"111.1.1.1/24"},
	}

	wgpApi := wgp.ToAPI()
	require.Equal(t, wgp.PresharedKey, wgpApi.Config.PresharedKey)
	require.Equal(t, wgp.PublicKey, wgpApi.Config.PublicKey)
	require.Equal(t, wgp.AllowedIPs, wgpApi.Config.AllowedIPs)
	require.Equal(t, wgp.OtherSideAllowedIPs, wgpApi.Config.OtherSideAllowedIPs)

	require.Empty(t, wgpApi.Config.Endpoint)
	require.Empty(t, wgpApi.Config.PersistentKeepalive)
	require.Empty(t, wgpApi.Server)

	wgp.PersistentKeepalive = 60
	wgpApi = wgp.ToAPI()
	require.Equal(t, fmt.Sprint(wgp.PersistentKeepalive), wgpApi.Config.PersistentKeepalive)

	// Port is set but missing server endpoint
	wgp.ListenPort = 9090
	wgpApi = wgp.ToAPI()

	require.Empty(t, wgpApi.Config.Endpoint)

	// Server endpoint and port are set
	wgp.Server = &pg.Server{
		Endpoint: netip.MustParseAddr("172.18.0.1"),
	}
	wgpApi = wgp.ToAPI()

	require.NotEmpty(t, wgpApi.Server)
	require.Equal(t, "172.18.0.1:9090", wgpApi.Config.Endpoint)

	// Server endpoint is set but missing port
	wgp.ListenPort = -1
	wgpApi = wgp.ToAPI()

	require.Empty(t, wgpApi.Config.Endpoint)
}
