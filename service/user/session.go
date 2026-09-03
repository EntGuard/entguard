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

package user

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	log "github.com/sirupsen/logrus"

	"github.com/entguard/entguard/pkg/db"
	"github.com/entguard/entguard/pkg/mfa"
	"github.com/entguard/entguard/service/api/v1"
)

const (
	zeroDuration = 0 * time.Second

	initialSessionExpCheck = 5 * time.Second
)

// GetAllDevicesSessions returns all active session IDs
func (m *PostgresManager) GetAllDevicesSessions(ctx context.Context) ([]string, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	sessions, err := m.SQLcQ.GetAllSessions(queryCtx)
	if err != nil {
		return nil, fmt.Errorf("%w: GetAllSessions: %v", ErrQueryFailed, err)
	}
	return sessions, nil
}

// ---- Periodic invalidation of old sessions

func (m *PostgresManager) startSessionInvalidation(ctx context.Context) {
	if m.Cfg.SessionExpiration == zeroDuration {
		m.Logger.Info("Session expiration check is disabled")
		return
	}

	syncLog := m.Logger.WithField("reportCaller", "session exp check")

	syncLog.Debugf("Session expiration check will start in %v", initialSessionExpCheck)
	ticker := time.NewTicker(initialSessionExpCheck)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.RunSessionInvalidation(ctx, syncLog)
			ticker.Reset(m.Cfg.SessionExpCheckInterval)
		}
	}
}

func (m *PostgresManager) RunSessionInvalidation(ctx context.Context, syncLog *log.Entry) {
	startTime := time.Now()
	syncLog.Debug("Checking for expired sessions")

	numSessionsInvalidated, err := m.InvalidateOldDevicesSessions(ctx, time.Now().Add(-m.Cfg.SessionExpiration))

	endTime := time.Now()

	if err != nil {
		syncLog.
			WithError(err).
			WithField("checkDuration", endTime.Sub(startTime)).
			Error("Failed to check for or delete expired sessions")
		return
	}
	syncLog.
		WithField("sessionsDeleted", numSessionsInvalidated).
		WithField("checkDuration", endTime.Sub(startTime)).
		Debug("Check for expired sessions completed")
}

// InvalidateOldDevicesSessions invalidates all device sessions older than the given threshold
func (m *PostgresManager) InvalidateOldDevicesSessions(ctx context.Context, threshold time.Time) (int, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	numSessionsInvalidated, err := m.SQLcQ.InvalidateOldDeviceSessions(queryCtx, db.NewPgTimestamp(threshold))
	if err != nil {
		return 0, fmt.Errorf("%w: InvalidateOldDeviceSessions: %w", ErrQueryFailed, err)
	}

	return int(numSessionsInvalidated), nil
}

// ---- Invalidation of sessions based on configuration change

// When this attribute changes, should the session be invalidated?
//
// - User screen:
//     - User section:
//         - Username - yes
//         - Password - yes
//         - Is admin - no
//         - MFA type - yes
//         - Notification - yes
//         - Auth type - yes (local/LDAP; this can not be changed in the UI, but may possibly change during LDAP sync)
//     - TOTP secret (generate new TOTP secret) - yes, but only if MFA type is set to 'TOTP'; otherwise no
//     - Device template section:
//         - Interface name - yes
//         - Address pools - no (if changing address pool would cause changes to device addresses, then the session will
//           be invalidated based on the addresses change, so we do not need additional check based on address pools)
//         - Listen port - yes
//         - DNS - yes
//         - MTU - yes
//     - Device section:
//         - Description - no
//         - Private & public key - yes
//         - Addresses - yes
//     - Adjacency template section:
//         - (None of the attributes should invalidate session. If a change to the adjacency template would cause a change
//           to the actual device adjacency, then the session will be invalidated based on the change to the adjacency.)
//     - Adjacency section: (keep in mind that adjacencies can also be edited from Server screen)
//         - Server - yes
//         - Client side allowed IPs - yes
//         - Server side allowed IPs - no
//         - Preshared key - yes
//         - (also invalidate session if a new adjacency is created for the device or if some existing adjacency is deleted)
//
// - Server screen (only if the device has adjacency with the server):
//     - Server section:
//         - Server name - no
//         - Endpoint - yes
//         - Healthcheck address - yes
//         - Description - no
//     - Server interface section:
//         - Interface name - no
//         - Private & public key - yes
//         - Addresses - no
//         - Listen port - yes
//         - MTU - no
//         - Persistent keepalive - yes
//
// - LDAP screen - no (any relevant changes to LDAP configs / templates will
// result in changes to device attributes after LDAP sync completes. Then
// session will be invalidated based on device attributes, so we do not need to
// do additional check here.)
//
// - MFA certificates screen - yes, but only if MFA type is set to ‘Cert’;
// otherwise no (any change to CA or CRL should invalidate sessions for all
// devices with MFA type ‘Cert’)
//
// - Address pools screen - no (if changing address pools would cause changes to
// device addresses, then the session will be invalidated based on the device
// addresses change, so we do not need additional check based on address pools)

// DeleteInvalidSessionsByDevice invalidates a device session if certain fields changed
func (m *PostgresManager) DeleteInvalidSessionsByDevice(ctx context.Context, deviceID int, old, new *api.DeviceData) error {
	if new.PrivateKey != old.PrivateKey ||
		new.PublicKey != old.PublicKey ||
		!reflect.DeepEqual(new.Addresses, old.Addresses) {

		if err := m.deleteDeviceSession(ctx, deviceID); err != nil {
			return err
		}
	}

	return nil
}

func (m *PostgresManager) DeleteInvalidSessionsByUser(ctx context.Context, old, new *UserModel) error {
	if new.Name != old.Name ||
		new.AuthService != old.AuthService ||
		new.MfaAuthType != old.MfaAuthType ||
		new.Notification != old.Notification {

		if err := m.deleteUserDeviceSessions(ctx, new.ID); err != nil {
			return err
		}
	}
	return nil
}

func (m *PostgresManager) DeleteInvalidSessionsByUserPassword(ctx context.Context, user *UserModel) error {
	// Do not try to compare new password with old password, delete sessions anyway
	if err := m.deleteUserDeviceSessions(ctx, user.ID); err != nil {
		return err
	}
	return nil
}

func (m *PostgresManager) DeleteInvalidSessionsByUserTOTP(ctx context.Context, user *UserModel) error {
	// Changing TOTP secret affects the user only if his MFA type is TOTP
	if user.MfaAuthType == mfa.TOTP {

		// Do not try to compare the TOTP secret, assume it was changed
		if err := m.deleteUserDeviceSessions(ctx, user.ID); err != nil {
			return err
		}
	}
	return nil
}

const (
	InsertCert = "INSERT"
	DeleteCert = "DELETE"
)

func (m *PostgresManager) DeleteInvalidSessionsByMFACert(ctx context.Context, operation string, certType string) error {
	if operation == InsertCert && certType == mfa.CertTypeCA {
		// no valid certificate removed or revoked - all sessions remain valid
	} else if operation == InsertCert && certType == mfa.CertTypeCRL {
		// revoked valid certificates
		if err := m.deleteSessionsOfAllUsersWithCertMFA(ctx); err != nil {
			return err
		}
	} else if operation == DeleteCert && certType == mfa.CertTypeCA {
		// removed valid certificate
		if err := m.deleteSessionsOfAllUsersWithCertMFA(ctx); err != nil {
			return err
		}
	} else if operation == DeleteCert && certType == mfa.CertTypeCRL {
		// no valid certificate removed or revoked - all sessions remain valid
	} else {
		return errors.New("internal error in DeleteInvalidSessionsByMFACert")
	}

	return nil
}

// deleteDeviceSession clears the session for a specific device
func (m *PostgresManager) deleteDeviceSession(ctx context.Context, id int) error {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := m.SQLcQ.InvalidateDeviceSession(queryCtx, id)
	if err != nil {
		return fmt.Errorf("%w: InvalidateDeviceSession: %v", ErrQueryFailed, err)
	}
	return nil
}

func (m *PostgresManager) deleteUserDeviceSessions(ctx context.Context, userID int) error {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	dt, err := m.SQLcQ.GetDeviceTemplateByUserID(queryCtx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // no device template, nothing to invalidate
		}
		return err
	}

	if _, err := m.SQLcQ.InvalidateTemplateDeviceSessions(queryCtx, dt.ID); err != nil {
		return err
	}
	return nil
}

func (m *PostgresManager) deleteSessionsOfAllUsersWithCertMFA(ctx context.Context) error {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	if _, err := m.SQLcQ.InvalidateAllDeviceSessionsWithCert(queryCtx); err != nil {
		return err
	}
	return nil
}
