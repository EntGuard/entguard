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
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	sqlc "github.com/entguard/entguard/db"
	"github.com/entguard/entguard/pkg/db"
	"github.com/entguard/entguard/pkg/test"
)

func TestIntegrationEnforceDPULimitDeleteDevicesWithoutExternalDeviceID(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	addressPool := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("dpu-test-pool"))
	dpuLimit := manager.Feats.GetDPULimit()
	require.Greater(t, dpuLimit, 0)

	user := test.InsertSampleUser(t, ctx, q, "user-exceed-dpu-no-ext-id")
	deviceTemplate := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user.ID, []int{addressPool.ID})

	// create dpuLimit + 3 devices, some without external_device_id
	devicesCreated := dpuLimit + 3
	devicesWithoutExtID := 3

	for i := 0; i < devicesWithoutExtID; i++ {
		device, err := sqlcq.InsertDevice(ctx, sqlc.InsertDeviceParams{
			DeviceTemplateID:    deviceTemplate.ID,
			PrivateKeyEncrypted: test.Seal("test-private-key"),
			PublicKey:           "test-public-key",
			ExternalDeviceID:    "",
		})
		require.NoError(t, err)

		_, err = sqlcq.UpdateDeviceSessionAndLastTimeConnected(ctx, sqlc.UpdateDeviceSessionAndLastTimeConnectedParams{
			ID:                device.ID,
			SessionID:         "test-session",
			LastTimeConnected: db.NewPgTimestamp(time.Now()),
		})
		require.NoError(t, err)
	}

	for i := devicesWithoutExtID; i < devicesCreated; i++ {
		test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate.ID, nil)
	}

	devicesBefore, err := sqlcq.GetDevicesByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, devicesBefore, devicesCreated)

	err = manager.EnforceDPULimit(ctx)
	require.NoError(t, err)

	// verify devices were deleted
	devicesAfter, err := sqlcq.GetDevicesByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, devicesAfter, dpuLimit)

	// verify that devices without external_device_id were deleted first
	for _, device := range devicesAfter {
		require.NotEmpty(t, device.ExternalDeviceID)
	}
}

func TestIntegrationEnforceDPULimitDeleteDevicesWithoutLastTimeConnected(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	addressPool := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("dpu-test-pool"))
	dpuLimit := manager.Feats.GetDPULimit()
	require.Greater(t, dpuLimit, 0)

	user := test.InsertSampleUser(t, ctx, q, "user-exceed-dpu-no-timestamp")
	deviceTemplate := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user.ID, []int{addressPool.ID})

	// create dpuLimit + 2 devices, some without last_time_connected
	devicesCreated := dpuLimit + 2
	devicesWithoutTimestamp := 2

	for i := 0; i < devicesWithoutTimestamp; i++ {
		device := test.InsertSampleDeviceManual(t, ctx, sqlcq, deviceTemplate.ID)
		_, err := sqlcq.UpdateDeviceSessionAndLastTimeConnected(ctx, sqlc.UpdateDeviceSessionAndLastTimeConnectedParams{
			ID:                device.ID,
			SessionID:         "test-session",
			LastTimeConnected: pgtype.Timestamp{Valid: false},
		})
		require.NoError(t, err)
	}

	for i := devicesWithoutTimestamp; i < devicesCreated; i++ {
		timestamp := time.Now().Add(-time.Duration(i) * time.Hour)
		test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate.ID, &timestamp)
	}

	devicesBefore, err := sqlcq.GetDevicesByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, devicesBefore, devicesCreated)

	err = manager.EnforceDPULimit(ctx)
	require.NoError(t, err)

	// verify devices were deleted
	devicesAfter, err := sqlcq.GetDevicesByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, devicesAfter, dpuLimit)

	// verify that devices without last_time_connected were deleted first
	for _, device := range devicesAfter {
		require.True(t, device.LastTimeConnected.Valid)
	}
}

func TestIntegrationEnforceDPULimitDeleteDevicesWithoutSessionID(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	addressPool := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("dpu-test-pool"))
	dpuLimit := manager.Feats.GetDPULimit()
	require.Greater(t, dpuLimit, 0)

	user := test.InsertSampleUser(t, ctx, q, "user-exceed-dpu-no-session")
	deviceTemplate := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user.ID, []int{addressPool.ID})

	// create dpuLimit + 2 devices, some without session_id
	devicesCreated := dpuLimit + 2
	devicesWithoutSession := 2

	for i := 0; i < devicesWithoutSession; i++ {
		device := test.InsertSampleDeviceManual(t, ctx, sqlcq, deviceTemplate.ID)
		_, err := sqlcq.UpdateDeviceSessionAndLastTimeConnected(ctx, sqlc.UpdateDeviceSessionAndLastTimeConnectedParams{
			ID:                device.ID,
			SessionID:         "",
			LastTimeConnected: db.NewPgTimestamp(time.Now()),
		})
		require.NoError(t, err)
	}

	for i := devicesWithoutSession; i < devicesCreated; i++ {
		timestamp := time.Now()
		test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate.ID, &timestamp)
	}

	devicesBefore, err := sqlcq.GetDevicesByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, devicesBefore, devicesCreated)

	err = manager.EnforceDPULimit(ctx)
	require.NoError(t, err)

	// verify devices were deleted
	devicesAfter, err := sqlcq.GetDevicesByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, devicesAfter, dpuLimit)

	// verify that devices without session_id were deleted
	for _, device := range devicesAfter {
		require.NotEmpty(t, device.SessionID)
	}
}

func TestIntegrationEnforceDPULimitDeleteOldestDevices(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	addressPool := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("dpu-test-pool"))
	dpuLimit := manager.Feats.GetDPULimit()
	require.Greater(t, dpuLimit, 0)

	user := test.InsertSampleUser(t, ctx, q, "user-exceed-dpu-oldest")
	deviceTemplate := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user.ID, []int{addressPool.ID})

	// create dpuLimit + 2 devices, all with proper attributes
	devicesCreated := dpuLimit + 2

	for i := 0; i < devicesCreated; i++ {
		timestamp := time.Now().Add(-time.Duration(i) * time.Hour)
		test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate.ID, &timestamp)
		// add small delay to ensure different created_at timestamps
		time.Sleep(10 * time.Millisecond)
	}

	devicesBefore, err := sqlcq.GetDevicesByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, devicesBefore, devicesCreated)

	err = manager.EnforceDPULimit(ctx)
	require.NoError(t, err)

	// verify devices were deleted
	devicesAfter, err := sqlcq.GetDevicesByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, devicesAfter, dpuLimit)

	// verify that the devices with oldest last_time_connected were deleted.
	// devices were created with decreasing last_time_connected (i=0 is most recent, highest i is oldest),
	// so the ones with the lowest IDs have the most recent connections and should be retained.
	maxExpectedID := devicesBefore[dpuLimit-1].ID
	for _, device := range devicesAfter {
		require.LessOrEqual(t, device.ID, maxExpectedID)
	}
}

func TestIntegrationEnforceDPULimitWithinLimit(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	addressPool := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("dpu-test-pool"))
	dpuLimit := manager.Feats.GetDPULimit()
	require.GreaterOrEqual(t, dpuLimit, 2)

	user := test.InsertSampleUser(t, ctx, q, "user-within-dpu")
	deviceTemplate := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user.ID, []int{addressPool.ID})

	// create devices within DPU limit
	devicesCreated := dpuLimit - 1

	for i := 0; i < devicesCreated; i++ {
		test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate.ID, nil)
	}

	devicesBefore, err := sqlcq.GetDevicesByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, devicesBefore, devicesCreated)

	err = manager.EnforceDPULimit(ctx)
	require.NoError(t, err)

	// no devices were deleted
	devicesAfter, err := sqlcq.GetDevicesByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, devicesAfter, devicesCreated)
}

func TestIntegrationEnforceDPULimitMultipleUsers(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	addressPool := test.InsertSampleAddressPool(t, ctx, sqlcq, test.NewSampleInsertAddressPoolParams("dpu-test-pool"))
	dpuLimit := manager.Feats.GetDPULimit()
	require.GreaterOrEqual(t, dpuLimit, 2)

	user1 := test.InsertSampleUser(t, ctx, q, "multi-user-1")
	deviceTemplate1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user1.ID, []int{addressPool.ID})

	user2 := test.InsertSampleUser(t, ctx, q, "multi-user-2")
	deviceTemplate2 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user2.ID, []int{addressPool.ID})

	user3 := test.InsertSampleUser(t, ctx, q, "multi-user-3")
	deviceTemplate3 := test.InsertSampleDeviceTemplate(t, ctx, sqlcq, user3.ID, []int{addressPool.ID})

	// user 1: exceeds DPU limit
	for i := 0; i < dpuLimit+3; i++ {
		test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate1.ID, nil)
	}

	// user 2: within DPU limit
	withinLimit := dpuLimit - 1
	for i := 0; i < withinLimit; i++ {
		test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate2.ID, nil)
	}

	// user 3: exceeds DPU limit
	for i := 0; i < dpuLimit+2; i++ {
		test.InsertSampleDeviceWithSession(t, ctx, sqlcq, deviceTemplate3.ID, nil)
	}

	err := manager.EnforceDPULimit(ctx)
	require.NoError(t, err)

	// user 1 devices were reduced to DPU limit
	user1Devices, err := sqlcq.GetDevicesByUserID(ctx, user1.ID)
	require.NoError(t, err)
	require.Len(t, user1Devices, dpuLimit)

	// user 2 devices remain unchanged
	user2Devices, err := sqlcq.GetDevicesByUserID(ctx, user2.ID)
	require.NoError(t, err)
	require.Len(t, user2Devices, withinLimit)

	// user 3 devices were reduced to DPU limit
	user3Devices, err := sqlcq.GetDevicesByUserID(ctx, user3.ID)
	require.NoError(t, err)
	require.Len(t, user3Devices, dpuLimit)
}
