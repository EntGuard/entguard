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

package user

import (
	"context"
	"fmt"
	"sort"

	sqlc "github.com/entguard/entguard/db"
)

// EnforceDPULimit checks all users and ensures their device count does not exceed the DPU limit.
// For users exceeding the limit, it deletes excess devices based on the following priority:
//
//  1. Devices without external_device_id (all have same priority)
//  2. Devices without last_time_connected (all have same priority)
//  3. Devices without session_id (prioritized by oldest last_time_connected)
//  4. Any device (prioritized by oldest last_time_connected)
func (m *PostgresManager) EnforceDPULimit(ctx context.Context) error {
	dpuLimit := m.Feats.GetDPULimit()
	m.Logger.WithField("dpuLimit", dpuLimit).Debug("Enforcing DPU limit on startup")

	users, err := m.SQLcQ.GetAllUsers(ctx)
	if err != nil {
		return fmt.Errorf("get all users: %w", err)
	}

	totalDevicesDeleted := 0
	usersAffected := 0

	for _, user := range users {
		queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
		defer cancel()

		devices, err := m.SQLcQ.GetDevicesByUserID(queryCtx, user.ID)
		if err != nil {
			m.Logger.
				WithError(err).
				WithField("userID", user.ID).
				WithField("username", user.Username).
				Error("Get devices for user during DPU enforcement")
			continue
		}

		// check if user exceeds DPU limit
		deviceCount := len(devices)
		if deviceCount <= dpuLimit {
			continue
		}

		// calculate how many devices to delete
		excessCount := deviceCount - dpuLimit
		m.Logger.
			WithField("userID", user.ID).
			WithField("username", user.Username).
			WithField("deviceCount", deviceCount).
			WithField("dpuLimit", dpuLimit).
			WithField("excessCount", excessCount).
			Warn("User exceeds DPU limit, deleting excess devices")

		sortedDevices := prioritizeDevicesForDeletion(devices)

		// delete excess devices
		thisUserAffected := false
		for i := 0; i < excessCount && i < len(sortedDevices); i++ {
			deviceID := sortedDevices[i].ID
			err := m.DeleteDevice(ctx, deviceID)
			if err != nil {
				m.Logger.
					WithError(err).
					WithField("deviceID", deviceID).
					WithField("userID", user.ID).
					WithField("username", user.Username).
					Error("Delete excess device during DPU enforcement")
				continue
			}

			m.Logger.
				WithField("deviceID", deviceID).
				WithField("externalDeviceID", sortedDevices[i].ExternalDeviceID).
				WithField("userID", user.ID).
				WithField("username", user.Username).
				Info("Deleted excess device during DPU enforcement")

			totalDevicesDeleted++
			thisUserAffected = true
		}

		if thisUserAffected {
			usersAffected++
		}
	}

	m.Logger.
		WithField("totalDevicesDeleted", totalDevicesDeleted).
		WithField("usersAffected", usersAffected).
		Debug("DPU limit enforcement completed")

	return nil
}

func prioritizeDevicesForDeletion(devices []sqlc.Device) []sqlc.Device {
	sortedDevices := make([]sqlc.Device, len(devices))
	copy(sortedDevices, devices)

	sort.Slice(sortedDevices, func(i, j int) bool {
		devI := sortedDevices[i]
		devJ := sortedDevices[j]

		// priority 1: devices without external_device_id
		hasExtIDI := devI.ExternalDeviceID != ""
		hasExtIDJ := devJ.ExternalDeviceID != ""
		if !hasExtIDI && hasExtIDJ {
			return true // i has no external_device_id, should be deleted first
		}
		if hasExtIDI && !hasExtIDJ {
			return false // j has no external_device_id, should be deleted first
		}
		// if both have or both don't have external_device_id, continue to next priority

		// priority 2: devices without last_time_connected
		hasLastTimeI := devI.LastTimeConnected.Valid
		hasLastTimeJ := devJ.LastTimeConnected.Valid
		if !hasLastTimeI && hasLastTimeJ {
			return true // i has no last_time_connected, should be deleted first
		}
		if hasLastTimeI && !hasLastTimeJ {
			return false // j has no last_time_connected, should be deleted first
		}
		// if both have or both don't have last_time_connected, continue to next priority

		// priority 3: devices without session_id
		hasSessionI := devI.SessionID != ""
		hasSessionJ := devJ.SessionID != ""
		if !hasSessionI && !hasSessionJ {
			// both have no session_id: sort by oldest last_time_connected
			if devI.LastTimeConnected.Valid && devJ.LastTimeConnected.Valid {
				return devI.LastTimeConnected.Time.Before(devJ.LastTimeConnected.Time)
			}
			return false
		}
		if !hasSessionI && hasSessionJ {
			return true // i has no session_id, should be deleted first
		}
		if hasSessionI && !hasSessionJ {
			return false // j has no session_id, should be deleted first
		}

		// priority 4: any device - sort by oldest last_time_connected
		if devI.LastTimeConnected.Valid && devJ.LastTimeConnected.Valid {
			return devI.LastTimeConnected.Time.Before(devJ.LastTimeConnected.Time)
		}

		// fallback: if last_time_connected is not valid, use device ID
		return devI.ID < devJ.ID
	})

	return sortedDevices
}
