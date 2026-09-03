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

package user_test

import (
	"context"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/entguard/entguard/db"
	"github.com/entguard/entguard/pkg/test"
	"github.com/entguard/entguard/service/api/v1"
	"github.com/entguard/entguard/service/user"
)

func TestValidateAddressPool(t *testing.T) {
	cases := []struct {
		name          string
		pool          *api.AddressPool
		expectFailure bool
	}{
		{
			name: "valid pool IPv4",
			pool: &api.AddressPool{
				Name:        "validIPv4",
				StartAddr:   "10.0.0.1",
				EndAddr:     "10.0.0.10",
				NetMask:     24,
				Description: "A valid IPv4 address pool",
			},
		},
		{
			name: "empty name",
			pool: &api.AddressPool{
				Name:      "",
				StartAddr: "10.0.0.1",
				EndAddr:   "10.0.0.10",
				NetMask:   24,
			},
			expectFailure: true,
		},
		{
			name: "invalid start IP",
			pool: &api.AddressPool{
				Name:      "badStart",
				StartAddr: "bad_ip",
				EndAddr:   "10.0.0.10",
				NetMask:   24,
			},
			expectFailure: true,
		},
		{
			name: "start > end",
			pool: &api.AddressPool{
				Name:      "start>End",
				StartAddr: "10.0.0.50",
				EndAddr:   "10.0.0.10",
				NetMask:   24,
			},
			expectFailure: true,
		},
		{
			name: "mask too big",
			pool: &api.AddressPool{
				Name:      "tooBigMask",
				StartAddr: "10.0.0.1",
				EndAddr:   "10.0.0.10",
				NetMask:   33,
			},
			expectFailure: true,
		},
		{
			name: "IPs outside subnet",
			pool: &api.AddressPool{
				Name:      "outOfSubnet",
				StartAddr: "10.0.0.1",
				EndAddr:   "10.0.1.1",
				NetMask:   24,
			},
			expectFailure: true,
		},
		{
			name: "valid pool IPv6",
			pool: &api.AddressPool{
				Name:        "validIPv6",
				StartAddr:   "2001:db8::1",
				EndAddr:     "2001:db8::2",
				NetMask:     64,
				Description: "A valid IPv6 address pool",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := manager.ValidateAddressPool(tc.pool)
			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestIntegrationCreateAddressPool(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	p := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool"))
	basePool := user.ConvertDBToAPIPool(*p)

	// create a pool for overlap testing
	_ = test.InsertSampleAddressPool(t, ctx, sqlcq, &db.InsertAddressPoolParams{
		Name:      "overlap-base",
		StartAddr: netip.MustParseAddr("10.200.0.1"),
		EndAddr:   netip.MustParseAddr("10.200.0.100"),
		NetMask:   24,
	})

	cases := []struct {
		name          string
		input         *api.AddressPool
		expectFailure bool
	}{
		{
			name: "insert new valid pool",
			input: &api.AddressPool{
				Name:        "pool2",
				StartAddr:   "10.2.2.1",
				EndAddr:     "10.2.2.100",
				NetMask:     24,
				Description: "A new valid pool",
			},
			expectFailure: false,
		},
		{
			name:          "duplicate name",
			input:         basePool,
			expectFailure: true,
		},
		{
			name: "overlapping pools allowed in pool creation",
			input: &api.AddressPool{
				Name:        "overlap-allowed",
				StartAddr:   "10.200.0.50",
				EndAddr:     "10.200.0.150",
				NetMask:     24,
				Description: "Overlapping with overlap-base pool",
			},
			expectFailure: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool, err := manager.CreateAddressPool(ctx, tc.input)

			if tc.expectFailure {
				require.Error(t, err)
				require.Nil(t, pool)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, pool)
			require.Equal(t, tc.input.Name, pool.Name)
			require.Equal(t, tc.input.StartAddr, pool.StartAddr)
			require.Equal(t, tc.input.EndAddr, pool.EndAddr)
			require.Equal(t, tc.input.NetMask, pool.NetMask)
			require.Equal(t, tc.input.Description, pool.Description)
			require.NotZero(t, pool.ID)
		})
	}
}

func TestIntegrationUpdateAddressPool_ServiceLayer(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	p1 := test.InsertSampleAddressPool(t, ctx, sqlcq, &db.InsertAddressPoolParams{
		Name:      "testPool1",
		StartAddr: netip.MustParseAddr("10.0.0.1"),
		EndAddr:   netip.MustParseAddr("10.0.0.100"),
		NetMask:   24,
	})

	_ = test.InsertSampleAddressPool(t, ctx, sqlcq, &db.InsertAddressPoolParams{
		Name:      "testPool2",
		StartAddr: netip.MustParseAddr("10.1.1.1"),
		EndAddr:   netip.MustParseAddr("10.1.1.100"),
		NetMask:   24,
	})

	cases := []struct {
		name          string
		id            int
		input         *api.AddressPool
		expectFailure bool
	}{
		{
			name: "successful update with same name",
			id:   p1.ID,
			input: &api.AddressPool{
				Name:      "testPool1",
				StartAddr: "10.0.0.10",
				EndAddr:   "10.0.0.200",
				NetMask:   24,
			},
			expectFailure: false,
		},
		{
			name: "successful update, all fields",
			id:   p1.ID,
			input: &api.AddressPool{
				Name:        "updatedPool",
				StartAddr:   "10.0.0.10",
				EndAddr:     "10.0.0.200",
				NetMask:     18,
				Description: "Updated description",
			},
			expectFailure: false,
		},
		{
			name: "update non-existing pool",
			id:   5,
			input: &api.AddressPool{
				Name:      "nonexistent",
				StartAddr: "10.0.0.10",
				EndAddr:   "10.0.0.20",
				NetMask:   24,
			},
			expectFailure: true,
		},
		{
			name: "duplicate name",
			id:   p1.ID,
			input: &api.AddressPool{
				Name:      "testPool2",
				StartAddr: "10.0.0.10",
				EndAddr:   "10.0.0.20",
				NetMask:   24,
			},
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool, err := manager.UpdateAddressPool(ctx, tc.id, tc.input)

			if tc.expectFailure {
				require.Error(t, err)
				require.Nil(t, pool)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, pool)
			require.Equal(t, tc.input.Name, pool.Name)
			require.Equal(t, tc.input.StartAddr, pool.StartAddr)
			require.Equal(t, tc.input.EndAddr, pool.EndAddr)
			require.Equal(t, tc.input.NetMask, pool.NetMask)
			require.Equal(t, tc.input.Description, pool.Description)

			dbPool, err := sqlcq.GetAddressPoolByID(ctx, tc.id)
			require.NoError(t, err)
			require.Equal(t, tc.input.Name, dbPool.Name)
			require.Equal(t, netip.MustParseAddr(tc.input.StartAddr), dbPool.StartAddr)
			require.Equal(t, netip.MustParseAddr(tc.input.EndAddr), dbPool.EndAddr)
			require.Equal(t, tc.input.NetMask, dbPool.NetMask)
			require.Equal(t, tc.input.Description, dbPool.Description)
		})
	}
}

func TestIntegrationGetAddressPoolByID_ServiceLayer(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	p1 := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool1"))
	_ = test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool2"))

	cases := []struct {
		name          string
		id            int
		dbPool        *db.AddressPool
		expectFailure bool
	}{
		{
			name:          "get existing pool",
			id:            p1.ID,
			dbPool:        p1,
			expectFailure: false,
		},
		{
			name:          "get non-existent pool",
			id:            5,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool, err := manager.GetAddressPoolByID(ctx, tc.id)

			if tc.expectFailure {
				require.Error(t, err)
				require.Nil(t, pool)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, pool)
			require.Equal(t, tc.dbPool.Name, pool.Name)
			require.Equal(t, tc.dbPool.StartAddr.String(), pool.StartAddr)
			require.Equal(t, tc.dbPool.EndAddr.String(), pool.EndAddr)
			require.Equal(t, tc.dbPool.NetMask, pool.NetMask)
			require.Equal(t, tc.dbPool.ID, pool.ID)
			require.Equal(t, tc.dbPool.Description, pool.Description)
		})
	}
}

func TestIntegrationGetAllAddressPools_ServiceLayer(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	p1 := test.InsertSampleAddressPool(t, ctx, sqlcq, &db.InsertAddressPoolParams{
		Name:        "zPool",
		StartAddr:   netip.MustParseAddr("10.0.1.9"),
		EndAddr:     netip.MustParseAddr("10.0.1.100"),
		NetMask:     24,
		Description: "A pool with a name starting with z",
	})
	p2 := test.InsertSampleAddressPool(t, ctx, sqlcq, &db.InsertAddressPoolParams{
		Name:        "aPool",
		StartAddr:   netip.MustParseAddr("10.0.0.1"),
		EndAddr:     netip.MustParseAddr("10.0.0.100"),
		NetMask:     24,
		Description: "",
	})

	pools, err := manager.GetAllAddressPools(ctx)
	require.NoError(t, err)
	require.NotNil(t, pools)
	require.Equal(t, len(pools), 2)

	// sorted by StartAddr
	require.Equal(t, p2.Name, pools[0].Name)
	require.Equal(t, p2.StartAddr.String(), pools[0].StartAddr)
	require.Equal(t, p2.EndAddr.String(), pools[0].EndAddr)
	require.Equal(t, p2.NetMask, pools[0].NetMask)
	require.Equal(t, p2.Description, pools[0].Description)

	require.Equal(t, p1.Name, pools[1].Name)
	require.Equal(t, p1.StartAddr.String(), pools[1].StartAddr)
	require.Equal(t, p1.EndAddr.String(), pools[1].EndAddr)
	require.Equal(t, p1.NetMask, pools[1].NetMask)
	require.Equal(t, p1.Description, pools[1].Description)
}

func TestIntegrationDeleteAddressPool_ServiceLayer(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	p := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("testPool"))

	cases := []struct {
		name          string
		id            int
		expectFailure bool
	}{
		{
			name:          "delete existing pool",
			id:            p.ID,
			expectFailure: false,
		},
		{
			name:          "delete non-existing pool",
			id:            5,
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := manager.DeleteAddressPool(ctx, tc.id)

			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			_, err = manager.GetAddressPoolByID(ctx, tc.id)
			require.Error(t, err)
		})
	}
}
