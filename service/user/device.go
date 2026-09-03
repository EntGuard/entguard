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
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"

	sqlc "github.com/entguard/entguard/db"
	"github.com/entguard/entguard/pkg/crypto/aesgcm"
	"github.com/entguard/entguard/pkg/crypto/keys"
	"github.com/entguard/entguard/pkg/db"
	"github.com/entguard/entguard/pkg/device"
	"github.com/entguard/entguard/pkg/ip"
	"github.com/entguard/entguard/pkg/nanoid"
	"github.com/entguard/entguard/service/api/v1"
	"github.com/entguard/entguard/service/vcm/drivers/postgres"
)

const (
	DummyExternalDeviceID = "000000000000000000000"
)

func (m *PostgresManager) GetDevice(ctx context.Context, id int) (*api.Device, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	device, err := m.SQLcQ.GetDeviceByID(queryCtx, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("GetDeviceByID: %w", err)
	}

	addresses, err := m.SQLcQ.GetDeviceWireGuardIfaceAddrs(queryCtx, device.ID)
	if err != nil {
		return nil, fmt.Errorf("GetDeviceWireGuardIfaceAddrs: %w", err)
	}
	apiDevice, err := ConvertDBToAPIDevice(m.EncryptionKey, device, addresses)
	if err != nil {
		return nil, err
	}

	return apiDevice, nil
}

func (m *PostgresManager) GetDeviceByExternalIDAndUserID(ctx context.Context, externalDeviceID string, userID int) (*api.Device, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	device, err := m.SQLcQ.GetDeviceByExternalIDAndUserID(queryCtx, sqlc.GetDeviceByExternalIDAndUserIDParams{
		ExternalDeviceID: externalDeviceID,
		UserID:           userID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("GetDeviceByExternalIDAndUserID: %w", err)
	}

	addresses, err := m.SQLcQ.GetDeviceWireGuardIfaceAddrs(queryCtx, device.ID)
	if err != nil {
		return nil, fmt.Errorf("GetDeviceWireGuardIfaceAddrs: %w", err)
	}
	apiDevice, err := ConvertDBToAPIDevice(m.EncryptionKey, device, addresses)
	if err != nil {
		return nil, err
	}

	return apiDevice, nil
}

func (m *PostgresManager) GetDevicesByTemplateID(ctx context.Context, deviceTemplateID int) ([]*api.Device, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := m.SQLcQ.DeviceTemplateExists(queryCtx, deviceTemplateID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("DeviceTemplateExists: %w", err)
	}
	devices, err := m.SQLcQ.GetDevicesByTemplateID(queryCtx, deviceTemplateID)
	if err != nil {
		return nil, fmt.Errorf("GetDevicesByTemplateID: %w", err)
	}

	deviceList := []*api.Device{}
	for _, d := range devices {
		addresses, err := m.SQLcQ.GetDeviceWireGuardIfaceAddrs(queryCtx, d.ID)
		if err != nil {
			return nil, fmt.Errorf("GetDeviceWireGuardIfaceAddrs: %w", err)
		}
		apiDevice, err := ConvertDBToAPIDevice(m.EncryptionKey, d, addresses)
		if err != nil {
			return nil, err
		}

		deviceList = append(deviceList, apiDevice)
	}
	return deviceList, nil
}

func (m *PostgresManager) GetDevicesByUserID(ctx context.Context, userID int) ([]*api.Device, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	exists, err := m.SQLcQ.UserExists(queryCtx, userID)
	if err != nil {
		return nil, fmt.Errorf("UserExists: %w", err)
	}
	if !exists {
		return nil, ErrNotFound
	}

	devices, err := m.SQLcQ.GetDevicesByUserID(queryCtx, userID)
	if err != nil {
		return nil, fmt.Errorf("GetDevicesByUserID: %w", err)
	}

	deviceList := []*api.Device{}
	for _, d := range devices {
		addresses, err := m.SQLcQ.GetDeviceWireGuardIfaceAddrs(queryCtx, d.ID)
		if err != nil {
			return nil, fmt.Errorf("GetDeviceWireGuardIfaceAddrs: %w", err)
		}
		apiDevice, err := ConvertDBToAPIDevice(m.EncryptionKey, d, addresses)
		if err != nil {
			return nil, err
		}

		deviceList = append(deviceList, apiDevice)
	}
	return deviceList, nil
}

func ConvertDBToAPIDevice(encryptionKey []byte, d sqlc.Device, addrs []netip.Prefix) (*api.Device, error) {
	priv, err := aesgcm.OpenString(encryptionKey, d.PrivateKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt private key: %w", err)
	}

	apiDevice := &api.Device{
		ID: d.ID,
		DeviceData: api.DeviceData{
			Description: d.Description,
			PrivateKey:  priv,
			PublicKey:   d.PublicKey,
		},
		ExternalDeviceID:  d.ExternalDeviceID,
		SessionID:         d.SessionID,
		LastTimeConnected: d.LastTimeConnected.Time,
		DeviceInformation: device.ParseInformation(db.StringFromPtr(d.DeviceInformation)),
	}
	for _, a := range addrs {
		apiDevice.Addresses = append(apiDevice.Addresses, a.String())
	}
	return apiDevice, nil
}

// GenerateDeviceData generates keys and addresses for a new device without creating it.
// This is used by the UI to pre-populate the device creation form.
func (m *PostgresManager) GenerateDeviceData(ctx context.Context, deviceTemplateID int) (*api.DeviceData, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := m.SQLcQ.GetDeviceTemplateByID(queryCtx, deviceTemplateID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("GetDeviceTemplateByID: %w", err)
	}

	existingDevices, err := m.SQLcQ.GetDevicesByTemplateID(queryCtx, deviceTemplateID)
	if err != nil {
		return nil, fmt.Errorf("GetDevicesByTemplateID: %w", err)
	}

	if len(existingDevices) >= m.Feats.GetDPULimit() {
		return nil, fmt.Errorf("%w: user already has %d devices (limit: %d)",
			ErrDPULimitExceeded, len(existingDevices), m.Feats.GetDPULimit(),
		)
	}

	prvKey, pubKey, err := keys.GenerateBase64EncodedWGKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	allocatedAddresses, err := m.allocateDeviceAddresses(ctx, deviceTemplateID)
	if err != nil {
		return nil, fmt.Errorf("allocateDeviceAddresses: %w", err)
	}

	var addressStrings []string
	for _, addr := range allocatedAddresses {
		addressStrings = append(addressStrings, addr.String())
	}

	return &api.DeviceData{
		Description: "",
		PrivateKey:  prvKey,
		PublicKey:   pubKey,
		Addresses:   addressStrings,
	}, nil
}

// CreateDeviceWithAutogenData creates a new device with autogenerated keys and addresses.
// This is used by AssignDeviceSlot for automatic device provisioning.
func (m *PostgresManager) CreateDeviceWithAutogenData(ctx context.Context, deviceTemplateID int, externalDeviceID, deviceInformation string) (*api.Device, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	prvKey, pubKey, err := keys.GenerateBase64EncodedWGKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	allocatedAddresses, err := m.allocateDeviceAddresses(ctx, deviceTemplateID)
	if err != nil {
		return nil, fmt.Errorf("allocateDeviceAddresses: %w", err)
	}

	return m.createDeviceInternal(queryCtx, deviceTemplateID, externalDeviceID, deviceInformation, prvKey, pubKey, "", allocatedAddresses)
}

// CreateDevice creates a new device with user-provided keys and addresses.
// This is used by the REST API when users want to specify their own device configuration.
func (m *PostgresManager) CreateDevice(ctx context.Context, deviceTemplateID int, data *api.DeviceData) (*api.Device, error) {
	if data == nil {
		return nil, fmt.Errorf("%w: DeviceData is required", ErrInvalidData)
	}

	if err := validateDeviceData(*data); err != nil {
		return nil, err
	}

	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var allocatedAddresses []netip.Prefix
	for _, addrStr := range data.Addresses {
		addr, err := netip.ParsePrefix(addrStr)
		if err != nil {
			return nil, fmt.Errorf("invalid address %s: %w", addrStr, err)
		}
		allocatedAddresses = append(allocatedAddresses, addr)
	}

	return m.createDeviceInternal(queryCtx, deviceTemplateID, "", "",
		data.PrivateKey, data.PublicKey, data.Description, allocatedAddresses)
}

func (m *PostgresManager) createDeviceInternal(
	ctx context.Context,
	deviceTemplateID int,
	externalDeviceID, deviceInformation string,
	prvKey, pubKey, description string,
	allocatedAddresses []netip.Prefix,
) (*api.Device, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	dt, err := m.SQLcQ.GetDeviceTemplateByID(queryCtx, deviceTemplateID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("GetDeviceTemplateByID: %w", err)
	}

	existingDevices, err := m.SQLcQ.GetDevicesByTemplateID(queryCtx, deviceTemplateID)
	if err != nil {
		return nil, fmt.Errorf("GetDevicesByTemplateID: %w", err)
	}

	if len(existingDevices) >= m.Feats.GetDPULimit() {
		return nil, fmt.Errorf("%w: user already has %d devices (limit: %d)",
			ErrDPULimitExceeded, len(existingDevices), m.Feats.GetDPULimit(),
		)
	}

	prvKeyEnc, err := aesgcm.Seal(m.EncryptionKey, prvKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt private key: %w", err)
	}

	dParams := sqlc.InsertDeviceParams{
		DeviceTemplateID:    deviceTemplateID,
		PrivateKeyEncrypted: prvKeyEnc,
		PublicKey:           pubKey,
		Description:         description,
	}
	if externalDeviceID != "" {
		dParams.ExternalDeviceID = externalDeviceID
	}
	if deviceInformation != "" {
		dParams.DeviceInformation = &deviceInformation
	}

	device, err := m.SQLcQ.InsertDevice(queryCtx, dParams)
	if err != nil {
		return nil, fmt.Errorf("InsertDevice: %w", err)
	}

	for _, addr := range allocatedAddresses {
		_, err := m.SQLcQ.InsertDeviceWireGuardIfaceAddr(queryCtx, sqlc.InsertDeviceWireGuardIfaceAddrParams{
			DeviceID: device.ID,
			Addr:     addr,
		})
		if err != nil {
			return nil, fmt.Errorf("InsertDeviceWireGuardIfaceAddr: %w", err)
		}
	}

	err = m.createAdjacenciesForNewDevice(ctx, dt.UserID, device.ID)
	if err != nil {
		return nil, err
	}

	apiDevice, err := ConvertDBToAPIDevice(m.EncryptionKey, device, allocatedAddresses)
	if err != nil {
		return nil, err
	}

	return apiDevice, nil
}

func (m *PostgresManager) createAdjacenciesForNewDevice(ctx context.Context, userID, deviceID int) error {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	oldAdjacencies, err := m.SQLcQ.GetDeviceWireGuardAdjacencies(queryCtx, deviceID)
	if err != nil {
		return fmt.Errorf("GetDeviceWireGuardAdjacencies for device with ID=%d: %w", deviceID, err)
	}
	if len(oldAdjacencies) != 0 {
		return fmt.Errorf("database is inconsistent, newly created device with ID=%d already had adjacencies", deviceID)
	}

	adjTemplates, err := m.SQLcQ.GetUserAdjacencyTemplates(queryCtx, userID)
	if err != nil {
		return fmt.Errorf("GetUserAdjacencyTemplates for user with ID=%d: %w", userID, err)
	}

	for _, at := range adjTemplates {
		var ips []string
		for _, ip := range at.ClientSideAllowedIps {
			ips = append(ips, ip.String())
		}

		psk := ""
		if at.UsePresharedKey {
			psk, err = keys.GenerateBase64EncodedWGPresharedKey()
			if err != nil {
				return fmt.Errorf("failed to generate preshared key: %w", err)
			}
		}

		err := m.Vcm.CreateAdjacency(ctx, at.ServerID, deviceID, &api.WireGuardAdjacency{
			PresharedKey: psk,
			AllowedIPs:   ips,
			ServerSide:   false,
		})

		if err != nil {
			return fmt.Errorf("CreateAdjacency for serverID=%d and deviceID=%d: %w", at.ServerID, deviceID, err)
		}
	}
	return nil
}

func (m *PostgresManager) UpdateDevice(ctx context.Context, id int, du api.DeviceData) error {
	if err := validateDeviceData(du); err != nil {
		return err
	}

	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	deviceOld, err := m.SQLcQ.GetDeviceByID(ctx, id)
	if err != nil {
		return fmt.Errorf("GetDeviceByID: %w", err)
	}

	addrsOld, err := m.SQLcQ.GetDeviceWireGuardIfaceAddrs(ctx, id)
	if err != nil {
		return fmt.Errorf("GetDeviceWireGuardIfaceAddrs: %w", err)
	}

	err = db.WithTx(queryCtx, m.DB.GetDbConnection(queryCtx), func(qsqlc *sqlc.Queries) error {
		privEnc, err := aesgcm.Seal(m.EncryptionKey, du.PrivateKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt private key: %w", err)
		}
		_, err = qsqlc.UpdateDevice(queryCtx, sqlc.UpdateDeviceParams{
			ID:                  id,
			Description:         du.Description,
			PrivateKeyEncrypted: privEnc,
			PublicKey:           du.PublicKey,
		})
		if err != nil {
			return fmt.Errorf("UpdateDevice: %w", err)
		}

		err = qsqlc.DeleteDeviceWireGuardIfaceAddrs(queryCtx, id)
		if err != nil {
			return fmt.Errorf("DeleteDeviceWireGuardIfaceAddrs: %w", err)
		}

		for _, addr := range du.Addresses {
			parsedPrefix, err := netip.ParsePrefix(addr)
			if err != nil {
				return fmt.Errorf("parse address '%s': %w", addr, err)
			}
			_, err = qsqlc.InsertDeviceWireGuardIfaceAddr(queryCtx, sqlc.InsertDeviceWireGuardIfaceAddrParams{
				DeviceID: id,
				Addr:     parsedPrefix,
			})
			if err != nil {
				return fmt.Errorf("InsertDeviceWireGuardIfaceAddr: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}

	if du.PublicKey != deviceOld.PublicKey {
		peers, err := m.SQLcQ.GetDeviceWireGuardAdjacencies(ctx, id)
		if err != nil {
			return fmt.Errorf("GetDeviceWireGuardAdjacencies for ID=%v: %w", id, err)
		}

		for _, p := range peers {
			tags, err := m.SQLcQ.GetServerTags(ctx, db.IntToPtr(p.Serverid))
			if err != nil {
				return fmt.Errorf("can't get server tags: %v", err)
			}

			var ips []string
			for _, ip := range p.ServerSideAllowedIps {
				ips = append(ips, ip.String())
			}

			psk, err := aesgcm.OpenString(m.EncryptionKey, p.PresharedKeyEncrypted)
			if err != nil {
				return fmt.Errorf("failed to decrypt preshared key: %w", err)
			}

			for _, tag := range tags {
				m.Vcm.NotifyWGDelPeerPubKey(tag, deviceOld.PublicKey)
				m.Vcm.NotifyWGAddPeer(tag, du.PublicKey, psk, ips)
			}
		}
	}

	d, err := ConvertDBToAPIDevice(m.EncryptionKey, deviceOld, addrsOld)
	if err != nil {
		return err
	}
	err = m.DeleteInvalidSessionsByDevice(ctx, id,
		&api.DeviceData{
			Addresses:  d.Addresses,
			PublicKey:  d.PublicKey,
			PrivateKey: d.PrivateKey,
		},
		&du)
	if err != nil {
		return fmt.Errorf("DeleteInvalidSessionsByDevice: %w", err)
	}

	return nil
}

func (m *PostgresManager) DeleteDevice(ctx context.Context, id int) error {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	peers, err := m.SQLcQ.GetDeviceWireGuardAdjacencies(queryCtx, id)
	if err != nil {
		return fmt.Errorf("GetDeviceWireGuardAdjacencies: %w", err)
	}

	for _, p := range peers {
		if err := m.SQLcQ.DeleteAdjacency(ctx, p.ID); err != nil {
			m.Logger.
				WithField("deviceID", id).
				WithField("serverID", p.Serverid).
				Info("Failed to remove WireGuard adjacency between device and server")
		}
	}

	n, err := m.SQLcQ.DeleteDevice(queryCtx, id)
	if err != nil {
		return fmt.Errorf("DeleteDevice: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}

	return nil
}

// AssignDeviceSlot updates or creates a device slot based on external_device_id
// following the slot selection algorithm:
//   - Case 1: Device with matching external_device_id exists -> reuse it, update session
//   - Case 2: Free slot exists (empty external_device_id) -> assign it to this external_device_id
//   - Case 3: DPU limit not exhausted -> create new device slot for this external_device_id
//   - Case 4a: DPU limit exhausted, inactive slot exists (empty session_id) -> overwrite oldest inactive
//   - Case 4b: DPU limit exhausted, all slots active -> kick device with oldest timestamp
func (m *PostgresManager) AssignDeviceSlot(ctx context.Context, userID int, session, externalDeviceID, deviceInformation string) (string, error) {
	if len(session) == 0 {
		return "", fmt.Errorf("%w: device's session is empty", ErrInvalidData)
	}
	if userID <= 0 {
		return "", fmt.Errorf("%w: userID is invalid", ErrInvalidData)
	}

	// Backward compatibility: use all-zeros external_device_id for old clients
	if len(externalDeviceID) == 0 {
		externalDeviceID = DummyExternalDeviceID
		m.Logger.WithField("externalDeviceID", externalDeviceID).
			WithField("userID", userID).
			Debug("Using all-zeros external_device_id for old client (backward compatibility)")
	} else if !nanoid.IsValid(externalDeviceID) {
		return "", fmt.Errorf("%w: external_device_id is invalid", ErrInvalidData)
	}

	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	timestamp := db.NewPgTimestamp(time.Now())
	var device sqlc.Device
	var err error

	// Case 1: Try to find device with matching external_device_id for this user
	device, err = m.SQLcQ.GetDeviceByExternalIDAndUserID(queryCtx, sqlc.GetDeviceByExternalIDAndUserIDParams{
		ExternalDeviceID: externalDeviceID,
		UserID:           userID,
	})
	if err == nil {
		device, e := m.SQLcQ.UpdateDeviceSessionAndLastTimeConnected(queryCtx, sqlc.UpdateDeviceSessionAndLastTimeConnectedParams{
			ID:                device.ID,
			SessionID:         session,
			LastTimeConnected: timestamp,
		})
		if e != nil {
			return "", fmt.Errorf("%w: UpdateDeviceSessionAndLastTimeConnected (Case 1: existing device): %w", ErrQueryFailed, e)
		}
		m.Logger.WithField("deviceID", device.ID).
			WithField("externalDeviceID", externalDeviceID).
			WithField("userID", userID).
			Info("Reusing existing device with matching external_device_id")
		return externalDeviceID, nil
	}
	if err != pgx.ErrNoRows {
		return "", fmt.Errorf("%w: GetDeviceByExternalIDAndUserID: %w", ErrQueryFailed, err)
	}

	// Case 2: Try to find device with no external_device_id for this user
	device, err = m.SQLcQ.GetDeviceWithNoExternalIDByUserID(queryCtx, userID)
	if err == nil {
		device, err = m.SQLcQ.UpdateDeviceFromConfiguration(queryCtx, sqlc.UpdateDeviceFromConfigurationParams{
			ID:                device.ID,
			ExternalDeviceID:  externalDeviceID,
			SessionID:         session,
			LastTimeConnected: timestamp,
			DeviceInformation: db.StringToPtr(deviceInformation),
		})
		if err != nil {
			return "", fmt.Errorf("%w: UpdateDeviceFromConfiguration (Case 2: free slot): %w", ErrQueryFailed, err)
		}
		m.Logger.WithField("deviceID", device.ID).
			WithField("externalDeviceID", externalDeviceID).
			WithField("userID", userID).
			Info("Using free device slot (no external_device_id)")
		return externalDeviceID, nil
	}
	if err != pgx.ErrNoRows {
		return "", fmt.Errorf("%w: GetDeviceWithNoExternalIDByUserID: %w", ErrQueryFailed, err)
	}

	// Case 3: Check if DPU limit allows creating a new device
	deviceTemplate, err := m.SQLcQ.GetDeviceTemplateByUserID(queryCtx, userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("%w: GetDeviceTemplateByUserID: user has no device template", ErrNotFound)
		}
		return "", fmt.Errorf("%w: GetDeviceTemplateByUserID: %w", ErrQueryFailed, err)
	}

	existingDevices, err := m.SQLcQ.GetDevicesByTemplateID(queryCtx, deviceTemplate.ID)
	if err != nil {
		return "", fmt.Errorf("%w: GetDevicesByTemplateID: %w", ErrQueryFailed, err)
	}

	// Case 3: DPU not exhausted, create new device
	if len(existingDevices) < m.Feats.GetDPULimit() {
		apiDevice, err := m.CreateDeviceWithAutogenData(ctx, deviceTemplate.ID, externalDeviceID, deviceInformation)
		if err != nil {
			return "", fmt.Errorf("CreateDeviceWithAutogenData: %w", err)
		}

		device, err = m.SQLcQ.UpdateDeviceSessionAndLastTimeConnected(queryCtx, sqlc.UpdateDeviceSessionAndLastTimeConnectedParams{
			ID:                apiDevice.ID,
			SessionID:         session,
			LastTimeConnected: timestamp,
		})
		if err != nil {
			return "", fmt.Errorf("%w: UpdateDeviceSessionAndLastTimeConnected (Case 3: new device): %w", ErrQueryFailed, err)
		}

		m.Logger.WithField("deviceID", device.ID).
			WithField("externalDeviceID", externalDeviceID).
			WithField("userID", userID).
			Info("Created new device slot (DPU limit not exceeded)")
		return externalDeviceID, nil
	}

	// Case 4: DPU exhausted, try to find device with no session_id
	device, err = m.SQLcQ.GetDeviceWithNoSessionIDOrderedByTimestamp(queryCtx, userID)
	if err == nil {
		oldExternalDeviceID := device.ExternalDeviceID
		device, err = m.SQLcQ.UpdateDeviceFromConfiguration(queryCtx, sqlc.UpdateDeviceFromConfigurationParams{
			ID:                device.ID,
			ExternalDeviceID:  externalDeviceID,
			SessionID:         session,
			LastTimeConnected: timestamp,
			DeviceInformation: db.StringToPtr(deviceInformation),
		})
		if err != nil {
			return "", fmt.Errorf("%w: UpdateDeviceFromConfiguration (Case 4a: overwrite inactive): %w", ErrQueryFailed, err)
		}
		m.Logger.WithField("deviceID", device.ID).
			WithField("externalDeviceID", externalDeviceID).
			WithField("oldExternalDeviceID", oldExternalDeviceID).
			WithField("userID", userID).
			Warn("DPU limit exhausted: overwriting device slot with no session_id (oldest first)")
		return externalDeviceID, nil
	}
	if err != pgx.ErrNoRows {
		return "", fmt.Errorf("%w: GetDeviceWithNoSessionIDOrderedByTimestamp: %w", ErrQueryFailed, err)
	}

	// Case 4b: All slots occupied, kick the device with the oldest timestamp
	device, err = m.SQLcQ.GetDeviceWithOldestTimestampByUserID(queryCtx, userID)
	if err != nil {
		return "", fmt.Errorf("%w: GetDeviceWithOldestTimestampByUserID: %w", ErrQueryFailed, err)
	}

	oldExternalDeviceID := device.ExternalDeviceID
	device, err = m.SQLcQ.UpdateDeviceFromConfiguration(queryCtx, sqlc.UpdateDeviceFromConfigurationParams{
		ID:                device.ID,
		ExternalDeviceID:  externalDeviceID,
		SessionID:         session,
		LastTimeConnected: timestamp,
		DeviceInformation: db.StringToPtr(deviceInformation),
	})
	if err != nil {
		return "", fmt.Errorf("%w: UpdateDeviceFromConfiguration (Case 4b: kick oldest): %w", ErrQueryFailed, err)
	}

	m.Logger.WithField("deviceID", device.ID).
		WithField("externalDeviceID", externalDeviceID).
		WithField("oldExternalDeviceID", oldExternalDeviceID).
		WithField("userID", userID).
		Warn("DPU limit exhausted: kicking device with oldest timestamp")
	return externalDeviceID, nil
}

func (m *PostgresManager) allocateDeviceAddresses(ctx context.Context, deviceTemplateID int) ([]netip.Prefix, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	addressPools, err := m.SQLcQ.GetDeviceTemplateAddressPools(queryCtx, deviceTemplateID)
	if err != nil {
		return nil, fmt.Errorf("GetDeviceTemplateAddressPools: %w", err)
	}

	var allocatedAddresses []netip.Prefix

	for _, pool := range addressPools {
		allocatedAddr, err := m.allocateAddressFromPool(ctx, pool)
		if err != nil {
			return nil, fmt.Errorf("allocateAddressFromPool: %w", err)
		}

		allocatedAddresses = append(allocatedAddresses, allocatedAddr)
	}

	return allocatedAddresses, nil
}

func (m *PostgresManager) ResyncDevices(ctx context.Context, deviceTemplateID int) (*api.DeviceResyncResponse, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := m.SQLcQ.DeviceTemplateExists(queryCtx, deviceTemplateID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("DeviceTemplateExists: %w", err)
	}

	addressPools, err := m.SQLcQ.GetDeviceTemplateAddressPools(queryCtx, deviceTemplateID)
	if err != nil {
		return nil, fmt.Errorf("GetDeviceTemplateAddressPools: %w", err)
	}

	devices, err := m.SQLcQ.GetDevicesByTemplateID(queryCtx, deviceTemplateID)
	if err != nil {
		return nil, fmt.Errorf("GetDevicesByTemplateID: %w", err)
	}

	response := &api.DeviceResyncResponse{
		DevicesUpdated: 0,
		DevicesDeleted: 0,
		Errors:         []string{},
	}

	for _, device := range devices {
		updated, err := m.resyncSingleDevice(ctx, device, addressPools)
		if err != nil {
			deviceDesc := fmt.Sprintf("device with ID=%d", device.ID)
			if device.Description != "" {
				deviceDesc = fmt.Sprintf("device with ID=%d (%s)", device.ID, device.Description)
			}

			if errors.Is(err, ErrNoAvailableAddressInPool) {
				m.Logger.WithField("deviceID", device.ID).
					WithField("deviceTemplateID", deviceTemplateID).
					Warn("Deleting device due to no available addresses during resync")

				if delErr := m.DeleteDevice(ctx, device.ID); delErr != nil {
					response.Errors = append(response.Errors, fmt.Sprintf("delete %s: %v", deviceDesc, delErr))
					m.Logger.WithError(delErr).WithField("deviceID", device.ID).Error("Delete device during resync")
					continue
				}

				response.DevicesDeleted++
				continue
			}

			response.Errors = append(response.Errors, fmt.Sprintf("Failed to resync %s: %v", deviceDesc, err))
			m.Logger.WithError(err).WithField("deviceID", device.ID).Error("Resync device")
			continue
		}

		if updated {
			response.DevicesUpdated++
		}
	}

	return response, nil
}

func (m *PostgresManager) resyncSingleDevice(
	ctx context.Context,
	device sqlc.Device,
	addressPools []sqlc.AddressPool,
) (bool, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	currentAddresses, err := m.SQLcQ.GetDeviceWireGuardIfaceAddrs(queryCtx, device.ID)
	if err != nil {
		return false, fmt.Errorf("GetDeviceWireGuardIfaceAddrs: %w", err)
	}

	// separate valid addresses from invalid ones and track which pools are covered
	addressesToKeep, poolsCovered := m.categorizeDeviceAddresses(device.ID, currentAddresses, addressPools)

	// allocate new addresses for pools that don't have an address yet
	addressesToAdd, err := m.allocateAddressesForPools(ctx, addressPools, poolsCovered)
	if err != nil {
		return false, fmt.Errorf("allocateAddressesForPools: %w", err)
	}

	// only update if there are changes
	if !hasAddressChanges(currentAddresses, addressesToKeep, addressesToAdd) {
		return false, nil
	}

	if err := m.updateDeviceAddresses(queryCtx, device.ID, addressesToKeep, addressesToAdd); err != nil {
		return false, fmt.Errorf("updateDeviceAddresses: %w", err)
	}

	m.Logger.WithField("deviceID", device.ID).
		WithField("addressesKept", len(addressesToKeep)).
		WithField("addressesAdded", len(addressesToAdd)).
		Debug("Device addresses resynced")

	return true, nil
}

// categorizeDeviceAddresses separates addresses into those that belong to a pool
// and tracks which pools are already covered by existing addresses
func (m *PostgresManager) categorizeDeviceAddresses(
	deviceID int,
	addresses []netip.Prefix,
	addressPools []sqlc.AddressPool,
) ([]netip.Prefix, map[int]bool) {
	addressesToKeep := []netip.Prefix{}
	poolsCovered := make(map[int]bool)

	for _, addr := range addresses {
		uncoveredPools := filterUncoveredPools(addressPools, poolsCovered)
		poolID := findPoolForAddress(addr, uncoveredPools)

		if poolID == -1 {
			m.Logger.WithField("deviceID", deviceID).
				WithField("address", addr.String()).
				Debug("Removing address that doesn't belong to any uncovered pool")
			continue
		}

		addressesToKeep = append(addressesToKeep, addr)
		poolsCovered[poolID] = true
	}

	return addressesToKeep, poolsCovered
}

// allocateAddressesForPools allocates new addresses for pools that aren't covered yet
func (m *PostgresManager) allocateAddressesForPools(
	ctx context.Context,
	addressPools []sqlc.AddressPool,
	poolsCovered map[int]bool,
) ([]netip.Prefix, error) {
	var addressesToAdd []netip.Prefix

	for _, pool := range addressPools {
		if poolsCovered[pool.ID] {
			continue
		}

		allocatedAddr, err := m.allocateAddressFromPool(ctx, pool)
		if err != nil {
			return nil, fmt.Errorf("allocateAddressFromPool: %w", err)
		}

		addressesToAdd = append(addressesToAdd, allocatedAddr)
	}

	return addressesToAdd, nil
}

// allocateAddressFromPool allocates a single address from the given pool
func (m *PostgresManager) allocateAddressFromPool(ctx context.Context, pool sqlc.AddressPool) (netip.Prefix, error) {
	addressRange, err := ip.AddrRangeFromPrefixes(
		netip.PrefixFrom(pool.StartAddr, pool.NetMask),
		netip.PrefixFrom(pool.EndAddr, pool.NetMask),
	)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("create address range for pool %s: %w", pool.Name, err)
	}

	usedAddrs, err := m.Vcm.QueryUsedAddrsForSubnet(ctx, addressRange.ToContainingSubnet())
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("QueryUsedAddrsForSubnet: %w", err)
	}

	allocatedAddr, err := addressRange.FirstAvailableAddress(usedAddrs)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("%w for poolName=%s, addressRange=%s: %w",
			ErrNoAvailableAddressInPool, pool.Name, addressRange, err)
	}

	return allocatedAddr, nil
}

func (m *PostgresManager) updateDeviceAddresses(
	ctx context.Context,
	deviceID int,
	addressesToKeep []netip.Prefix,
	addressesToAdd []netip.Prefix,
) error {
	allAddresses := append(addressesToKeep, addressesToAdd...)

	err := db.WithTx(ctx, m.DB.GetDbConnection(ctx), func(qsqlc *sqlc.Queries) error {
		if err := qsqlc.DeleteDeviceWireGuardIfaceAddrs(ctx, deviceID); err != nil {
			return fmt.Errorf("DeleteDeviceWireGuardIfaceAddrs: %w", err)
		}

		for _, addr := range allAddresses {
			_, err := qsqlc.InsertDeviceWireGuardIfaceAddr(ctx, sqlc.InsertDeviceWireGuardIfaceAddrParams{
				DeviceID: deviceID,
				Addr:     addr,
			})
			if err != nil {
				return fmt.Errorf("InsertDeviceWireGuardIfaceAddr: %w", err)
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("transaction failed: %w", err)
	}

	return nil
}

// findPoolForAddress returns the pool ID if the address belongs to a pool, -1 otherwise.
// If an address belongs to multiple pools, it will return the ID of the smallest one.
func findPoolForAddress(addr netip.Prefix, addressPools []sqlc.AddressPool) int {
	var candidatePools []sqlc.AddressPool
	for _, pool := range addressPools {
		if addressBelongsToPool(addr, pool) {
			candidatePools = append(candidatePools, pool)
		}
	}
	if len(candidatePools) == 0 {
		return -1
	}
	if len(candidatePools) == 1 {
		return candidatePools[0].ID
	}

	// more than one pool contains the address - find the smallest one.
	// This assumes IPv4 addresses, which is consistent with the rest of the logic(IPv6 addresses are not supported yet).
	var smallestPool sqlc.AddressPool
	var minSize *uint64

	for _, pool := range candidatePools {
		if !pool.StartAddr.Is4() || !pool.EndAddr.Is4() {
			continue
		}
		startBytes := pool.StartAddr.As4()
		endBytes := pool.EndAddr.As4()

		start := uint32(startBytes[0])<<24 | uint32(startBytes[1])<<16 | uint32(startBytes[2])<<8 | uint32(startBytes[3])
		end := uint32(endBytes[0])<<24 | uint32(endBytes[1])<<16 | uint32(endBytes[2])<<8 | uint32(endBytes[3])

		if end < start {
			continue
		}

		size := uint64(end - start)
		if minSize == nil || size < *minSize {
			minSize = &size
			smallestPool = pool
		}
	}

	if minSize == nil {
		return candidatePools[0].ID
	}

	return smallestPool.ID
}

// filterUncoveredPools returns only pools that are not yet covered
func filterUncoveredPools(addressPools []sqlc.AddressPool, poolsCovered map[int]bool) []sqlc.AddressPool {
	var uncoveredPools []sqlc.AddressPool
	for _, pool := range addressPools {
		if !poolsCovered[pool.ID] {
			uncoveredPools = append(uncoveredPools, pool)
		}
	}
	return uncoveredPools
}

// hasAddressChanges checks if there are any changes to apply
func hasAddressChanges(current []netip.Prefix, keep []netip.Prefix, add []netip.Prefix) bool {
	return len(add) > 0 || len(keep) != len(current)
}

func addressBelongsToPool(addr netip.Prefix, pool sqlc.AddressPool) bool {
	addrIP := addr.Addr()

	if addrIP.Compare(pool.StartAddr) < 0 {
		return false
	}
	if addrIP.Compare(pool.EndAddr) > 0 {
		return false
	}

	if addr.Bits() != pool.NetMask {
		return false
	}

	return true
}

// validateDeviceData validates the required fields in DeviceData.
// It checks that public key and private key are non-empty, valid base64-encoded
// 32-byte WireGuard keys, and that at least one address is provided.
func validateDeviceData(data api.DeviceData) error {
	if data.PublicKey == "" {
		return fmt.Errorf("%w: public key cannot be empty", ErrInvalidData)
	}
	if data.PrivateKey == "" {
		return fmt.Errorf("%w: private key cannot be empty", ErrInvalidData)
	}
	if err := postgres.ValidateKeyBase64(data.PublicKey); err != nil {
		return fmt.Errorf("%w: invalid public key: %v", ErrInvalidData, err)
	}
	if err := postgres.ValidateKeyBase64(data.PrivateKey); err != nil {
		return fmt.Errorf("%w: invalid private key: %v", ErrInvalidData, err)
	}
	if len(data.Addresses) == 0 {
		return fmt.Errorf("%w: at least one address must be provided", ErrInvalidData)
	}
	return nil
}
