/*
 * Copyright 2021 PANTHEON.tech s.r.o.
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
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"

	"github.com/entguard/entguard/pkg/mfa"
)

const (
	postgresQuerierCaller = "postgres user querier"
	queryTimeout          = 5 * time.Second
)

const (
	queryUserByUsername     = `SELECT id, password, is_admin, auth_service, mfa_auth, ntf, updated_at FROM users WHERE username=$1;`
	queryUserByID           = `SELECT username, password, is_admin, auth_service, mfa_auth, ntf, updated_at FROM users WHERE id=$1;`
	queryUsernameCheck      = `SELECT 1 FROM users WHERE username=$1 LIMIT 1;`
	queryUserIDCheck        = `SELECT 1 FROM users WHERE id=$1 LIMIT 1;`
	queryInsertUser         = `INSERT INTO users (username, password, is_admin, auth_service, mfa_auth, ntf, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, updated_at;`
	queryUpdateUser         = `UPDATE users SET username=$1, is_admin=$2, mfa_auth=$3, ntf=$4, updated_at=$5 WHERE id=$6;`
	queryUpdateUserPwd      = `UPDATE users SET password=$1, updated_at=$2 WHERE id=$3;`
	queryUpdateUserEditTime = `UPDATE users SET updated_at=$1 WHERE id=$2;`
	queryCountUsers         = `SELECT COUNT(*) FROM users;`

	queryMFACerts      = `SELECT id, cert FROM mfa_certs WHERE cert_type=$1`
	queryInsertMFACert = `INSERT INTO mfa_certs(cert, cert_type) VALUES ($1, $2) RETURNING id`
	queryDeleteMFACert = `DELETE FROM mfa_certs WHERE id=$1;`

	queryLdapConfigVerifyPriority = `
		SELECT 1 FROM ldaps WHERE priority=$1
	`
	queryDeleteLdapTemplate = `
		DELETE FROM ldap_templates WHERE id=$1`
	queryInsertLdapTemplateServer = `
		INSERT INTO ldap_template_servers (
			ldap_template_id, server_id, allowed_ips, use_preshared_key
		)
		VALUES ($1, $2, $3, $4);`
	queryUpdateLdapTemplateServer = `
		UPDATE ldap_template_servers
		SET server_id = $1,
		allowed_ips = $2, use_preshared_key = $3
		WHERE ldap_template_id = $4 AND server_id = $5;`
	queryDeleteLdapTemplateServer = `
		DELETE FROM ldap_template_servers
		WHERE ldap_template_id = $1 AND server_id = $2;`
	queryGetLdapTemplateServers = `
		SELECT
		servers.name, server_id, ldap_template_id, allowed_ips, use_preshared_key
		FROM ldap_template_servers
		LEFT JOIN servers
		ON ldap_template_servers.server_id = servers.id
		WHERE ldap_template_servers.ldap_template_id=$1`
	queryDeleteLdapConfig = `
		DELETE FROM ldaps WHERE id=$1`
	queryLdapUserAuth = `
		SELECT ldaps.priority, ldaps.host, ldaps.port, users_to_ldaps.user_dn, ldaps.use_tls, ldaps.fqdn, ldaps.ca_cert
		FROM users_to_ldaps
		JOIN ldaps ON users_to_ldaps.ldap_id=ldaps.id
		WHERE users_to_ldaps.user_id=$1
		ORDER BY ldaps.priority;`
	queryLdapUsersPriorities = `
		SELECT ldaps.priority, users_to_ldaps.user_id, users_to_ldaps.ldap_id
		FROM users_to_ldaps
		JOIN ldaps ON users_to_ldaps.ldap_id=ldaps.id
		ORDER BY ldaps.priority;`
	queryLdapRelation = `
		SELECT id, user_id, user_dn, created_at, updated_at
		FROM users_to_ldaps
		WHERE ldap_id=$1 AND user_uid=$2`
	queryInsertLdapRelation = `
		INSERT INTO users_to_ldaps (user_id, ldap_id, user_uid, user_dn, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id;`
	queryUpdateLdapRelation = `
		UPDATE users_to_ldaps
		SET user_id=$1, user_dn=$2, updated_at=$3
		WHERE id=$4 AND ldap_id=$5 AND user_uid=$6`
	queryDeleteLdapRelations = `
		DELETE FROM users_to_ldaps
		WHERE updated_at < $1`
	queryLdapUsersWithoutRelations = `
		SELECT id, username
		FROM users
		WHERE id IN (
			SELECT users.id
			FROM users LEFT JOIN users_to_ldaps ON users.id=users_to_ldaps.user_id
			WHERE users.auth_service='ldap'
			GROUP BY users.id HAVING COUNT(users_to_ldaps.id) = 0
		)`
)

type Querier interface {
	GetUserByName(ctx context.Context, name string) (*UserModel, error)
	GetUserByID(ctx context.Context, id int) (*UserModel, error)
	InsertUser(ctx context.Context, u *UserModel) error
	UpdateUser(ctx context.Context, u *UserModel) error
	UpdateUserPassword(ctx context.Context, userID int, passwordHash string) error
	CheckUsernameExists(ctx context.Context, username string) (bool, error)
	CheckUserIDExists(ctx context.Context, id int) (bool, error)
	CountUsers(ctx context.Context) (int, error)

	// LDAP related methods.
	VerifyLdapConfigPriority(ctx context.Context, priority int) error
	DeleteLdapConfig(ctx context.Context, id string) error
	InsertLDAPUserWithRelation(ctx context.Context, u *UserModel, rel *LdapRelation) error
	GetLdapRelation(ctx context.Context, ldapID, userUID string) (*LdapRelation, error)
	InsertLdapRelation(ctx context.Context, rel *LdapRelation) error
	UpdateLdapRelation(ctx context.Context, rel *LdapRelation) error
	DeleteLdapRelations(ctx context.Context, updatedBefore time.Time) (int, error)
	GetLdapUsersWithoutRelations(ctx context.Context) ([]*UserModel, error)
	GetLdapUserAuthsByUserID(ctx context.Context, id int) ([]*LdapUserAuth, error)
	GetAllLdapUsersPriorities(ctx context.Context) ([]*LdapUserPriority, error)

	GetLdapTemplateServers(ctx context.Context, ldapTemplateID int) ([]LdapTemplateServer, error)
	InsertLdapTemplateServer(ctx context.Context, server *LdapTemplateServer) error
	UpdateLdapTemplateServer(ctx context.Context, server *LdapTemplateServer) error
	DeleteLdapTemplateServer(ctx context.Context, templateID, serverID int) error

	GetMFACerts(ctx context.Context, mfaType string) ([]*mfa.RawCert, error)
	InsertMFACerts(ctx context.Context, cert []byte, mfaType string) error
	DeleteMFACerts(ctx context.Context, id int) error
	GetDbConnection(ctx context.Context) *pgxpool.Pool
}

// PostgresQuerier makes calls to the database.
type PostgresQuerier struct {
	Conn   *pgxpool.Pool
	Logger *log.Entry
}

func NewPostgresQuerier(ctx context.Context, conn *pgxpool.Pool, l *log.Logger) Querier {
	return &PostgresQuerier{
		Conn:   conn,
		Logger: l.WithField("reportCaller", postgresQuerierCaller),
	}
}

func (q *PostgresQuerier) GetDbConnection(ctx context.Context) *pgxpool.Pool {
	return q.Conn
}

func (q *PostgresQuerier) GetUserByName(ctx context.Context, name string) (*UserModel, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	u, err := NewUserModelWithName(name)
	if err != nil {
		return nil, err
	}
	err = q.Conn.QueryRow(
		queryCtx, queryUserByUsername, u.Name,
	).Scan(
		&u.ID, &u.PasswordHash, &u.IsAdmin, &u.AuthService, &u.MfaAuthType, &u.Notification, &u.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrUserNotFound
		}
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Search for user by username failed")
		return nil, ErrQueryFailed
	}
	return u, nil
}

func (q *PostgresQuerier) GetUserByID(ctx context.Context, id int) (*UserModel, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	u, err := NewUserModelWithID(id)
	if err != nil {
		return nil, err
	}
	err = q.Conn.QueryRow(
		queryCtx, queryUserByID, u.ID,
	).Scan(
		&u.Name, &u.PasswordHash, &u.IsAdmin, &u.AuthService, &u.MfaAuthType, &u.Notification, &u.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrUserNotFound
		}
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Search for user by ID failed")
		return nil, ErrQueryFailed
	}
	return u, nil
}

func (q *PostgresQuerier) InsertUser(ctx context.Context, u *UserModel) error {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	err := q.Conn.QueryRow(
		queryCtx, queryInsertUser, u.Name, u.PasswordHash, u.IsAdmin, u.AuthService, u.MfaAuthType, u.Notification, time.Now(),
	).Scan(
		&u.ID,
		&u.UpdatedAt,
	)
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Insert user failed")
		return ErrQueryFailed
	}
	return nil
}

func (q *PostgresQuerier) UpdateUser(ctx context.Context, u *UserModel) error {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	ct, err := q.Conn.Exec(
		queryCtx, queryUpdateUser, u.Name, u.IsAdmin, u.MfaAuthType, u.Notification, time.Now(), u.ID,
	)
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Update user failed")
		return ErrQueryFailed
	}
	if ct.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (q *PostgresQuerier) UpdateUserPassword(ctx context.Context, userID int, passwordHash string) error {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	ct, err := q.Conn.Exec(
		queryCtx, queryUpdateUserPwd, passwordHash, time.Now(), userID,
	)
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Update user password failed")
		return ErrQueryFailed
	}
	if ct.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (q *PostgresQuerier) CheckUsernameExists(ctx context.Context, username string) (bool, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var dummy int
	err := q.Conn.QueryRow(
		queryCtx, queryUsernameCheck, username,
	).Scan(
		&dummy,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			// This username is not taken.
			return false, nil
		}
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Check username failed")
		return false, ErrQueryFailed
	}

	return true, nil
}

func (q *PostgresQuerier) CheckUserIDExists(ctx context.Context, userID int) (bool, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var dummy int
	err := q.Conn.QueryRow(
		queryCtx, queryUserIDCheck, userID,
	).Scan(
		&dummy,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Check user ID failed")
		return false, ErrQueryFailed
	}

	return true, nil
}

func (q *PostgresQuerier) CountUsers(ctx context.Context) (int, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var count int
	err := q.Conn.QueryRow(
		queryCtx, queryCountUsers,
	).Scan(
		&count,
	)
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("User count failed")
		return -1, ErrQueryFailed
	}
	return count, nil
}

func (q *PostgresQuerier) VerifyLdapConfigPriority(ctx context.Context, priority int) error {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var val int
	err := q.Conn.QueryRow(
		queryCtx, queryLdapConfigVerifyPriority, priority,
	).Scan(
		&val,
	)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Verify LDAP configuration priority failed")
		return ErrQueryFailed
	}
	return fmt.Errorf("LDAP configuration with the same priority level already exists")
}

func (q *PostgresQuerier) InsertLdapTemplateServer(ctx context.Context, server *LdapTemplateServer) error {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := q.Conn.Exec(
		queryCtx, queryInsertLdapTemplateServer,
		server.LdapTemplateID,
		server.ServerID,
		server.ClientSideAllowedIPs,
		server.UsePresharedKey,
	)
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Insert LDAP template server query failed")
		return ErrQueryFailed
	}
	return nil
}

func (q *PostgresQuerier) UpdateLdapTemplateServer(ctx context.Context, server *LdapTemplateServer) error {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	ct, err := q.Conn.Exec(
		queryCtx, queryUpdateLdapTemplateServer,
		server.ServerID,
		server.ClientSideAllowedIPs,
		server.UsePresharedKey,
		server.LdapTemplateID,
		server.OldServerID,
	)
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Update LDAP template server query failed")
		return ErrQueryFailed
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (q *PostgresQuerier) DeleteLdapTemplateServer(ctx context.Context, templateID, serverID int) error {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	ct, err := q.Conn.Exec(
		queryCtx, queryDeleteLdapTemplateServer,
		templateID,
		serverID,
	)
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Delete LDAP template server query failed")
		return ErrQueryFailed
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (q *PostgresQuerier) GetLdapTemplateServers(ctx context.Context, ldapTemplateID int) ([]LdapTemplateServer, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	rows, err := q.Conn.Query(queryCtx, queryGetLdapTemplateServers, ldapTemplateID)
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Get all LDAP template servers query failed")
		return nil, ErrQueryFailed
	}
	defer rows.Close()

	var (
		ldapTemplateServer  LdapTemplateServer
		ldapTemplateServers []LdapTemplateServer
	)
	for rows.Next() {
		var ips []netip.Prefix
		ldapTemplateServer = LdapTemplateServer{
			ClientSideAllowedIPs: []string{},
		}
		err := rows.Scan(
			&ldapTemplateServer.ServerName,
			&ldapTemplateServer.ServerID,
			&ldapTemplateServer.LdapTemplateID,
			&ips,
			&ldapTemplateServer.UsePresharedKey,
		)
		if err != nil {
			q.Logger.Infof("All LDAP template servers scanning rows failed: %v", err)
			return nil, ErrQueryFailed
		}
		for _, ip := range ips {
			ldapTemplateServer.ClientSideAllowedIPs = append(ldapTemplateServer.ClientSideAllowedIPs, ip.String())
		}

		ldapTemplateServers = append(ldapTemplateServers, ldapTemplateServer)
	}

	if rows.Err() != nil {
		q.Logger.Infof("All LDAP template servers error after scanning rows: %v\n", rows.Err())
		return nil, ErrQueryFailed
	}
	return ldapTemplateServers, nil
}

func (q *PostgresQuerier) DeleteLdapConfig(ctx context.Context, id string) error {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	ct, err := q.Conn.Exec(
		queryCtx, queryDeleteLdapConfig, id,
	)
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Delete LDAP config query failed")
		return ErrQueryFailed
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("ldap configuration %w", ErrNotFound)
	}
	return nil
}

func (q *PostgresQuerier) InsertLDAPUserWithRelation(ctx context.Context, u *UserModel, rel *LdapRelation) (err error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	tx, err := q.Conn.Begin(queryCtx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		rbErr := tx.Rollback(ctx)
		if rbErr != nil && rbErr != pgx.ErrTxClosed {
			err = errors.Join(err, fmt.Errorf("failed to rollback transaction: %w", rbErr))
		}
	}()

	err = tx.QueryRow(
		queryCtx, queryInsertUser, u.Name, u.PasswordHash, u.IsAdmin, u.AuthService, u.MfaAuthType, "", time.Now(),
	).Scan(
		&u.ID,
		&u.UpdatedAt,
	)
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Insert user failed")
		return ErrQueryFailed
	}

	rel.UserID = u.ID

	err = tx.QueryRow(
		queryCtx, queryInsertLdapRelation,
		rel.UserID,
		rel.LdapID,
		// Decode UserUID back when retrieving it.
		base64.StdEncoding.EncodeToString([]byte(rel.UserUID)),
		rel.UserDN,
		rel.CreatedAt,
		rel.UpdatedAt,
	).Scan(&rel.ID)
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Insert LDAP relation failed")
		return ErrQueryFailed
	}

	err = tx.Commit(queryCtx)
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return err
}

func (q *PostgresQuerier) GetLdapRelation(ctx context.Context, ldapID, userUID string) (*LdapRelation, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	rel := &LdapRelation{
		LdapID:  ldapID,
		UserUID: userUID,
	}
	err := q.Conn.QueryRow(
		queryCtx, queryLdapRelation,
		ldapID,
		// Decode UserUID back when retrieving it.
		base64.StdEncoding.EncodeToString([]byte(userUID)),
	).Scan(
		&rel.ID,
		&rel.UserID,
		&rel.UserDN,
		&rel.CreatedAt,
		&rel.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("ldap relation %w", ErrNotFound)
		}
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Search for LDAP relation failed")
		return nil, ErrQueryFailed
	}
	return rel, nil
}

func (q *PostgresQuerier) InsertLdapRelation(ctx context.Context, rel *LdapRelation) error {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	err := q.Conn.QueryRow(
		queryCtx, queryInsertLdapRelation,
		rel.UserID,
		rel.LdapID,
		// Decode UserUID back when retrieving it.
		base64.StdEncoding.EncodeToString([]byte(rel.UserUID)),
		rel.UserDN,
		rel.CreatedAt,
		rel.UpdatedAt,
	).Scan(&rel.ID)
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Insert LDAP relation failed")
		return ErrQueryFailed
	}
	return nil
}

func (q *PostgresQuerier) UpdateLdapRelation(ctx context.Context, rel *LdapRelation) error {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	ct, err := q.Conn.Exec(
		queryCtx, queryUpdateLdapRelation,
		rel.UserID,
		rel.UserDN,
		rel.UpdatedAt,
		rel.ID,
		rel.LdapID,
		// Decode UserUID back when retrieving it.
		base64.StdEncoding.EncodeToString([]byte(rel.UserUID)),
	)
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Update LDAP relation failed")
		return ErrQueryFailed
	}
	if ct.RowsAffected() == 0 {
		q.Logger.Infof("No LDAP relation found for update")
		return nil
	}
	if ct.RowsAffected() > 1 {
		q.Logger.Infof("More than one LDAP relation updated")
		return nil
	}
	return nil
}

func (q *PostgresQuerier) DeleteLdapRelations(ctx context.Context, updatedBefore time.Time) (int, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	ct, err := q.Conn.Exec(
		queryCtx, queryDeleteLdapRelations, updatedBefore,
	)
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("DeleteLdapRelations failed")
		return 0, ErrQueryFailed
	}
	return int(ct.RowsAffected()), nil
}

func (q *PostgresQuerier) GetLdapUsersWithoutRelations(ctx context.Context) ([]*UserModel, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	rows, err := q.Conn.Query(queryCtx, queryLdapUsersWithoutRelations)
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Get LDAP users without relations failed")
		return nil, ErrQueryFailed
	}
	defer rows.Close()

	var (
		u     *UserModel
		users []*UserModel
	)
	for rows.Next() {
		u = &UserModel{}
		err = rows.Scan(&u.ID, &u.Name)
		if err != nil {
			q.Logger.
				WithError(err).
				Warn("LDAP users scanning rows failed")
			return nil, ErrQueryFailed
		}

		users = append(users, u)
	}

	if rows.Err() != nil {
		q.Logger.
			WithError(rows.Err()).
			Warn("LDAP users error after scanning rows")
		return nil, ErrQueryFailed
	}
	return users, nil
}

func (q *PostgresQuerier) GetLdapUserAuthsByUserID(ctx context.Context, id int) ([]*LdapUserAuth, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	rows, err := q.Conn.Query(queryCtx, queryLdapUserAuth, id)
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("All LDAP user auths query failed")
		return nil, ErrQueryFailed
	}
	defer rows.Close()

	var (
		lua  *LdapUserAuth
		luas []*LdapUserAuth
	)
	for rows.Next() {
		lua = &LdapUserAuth{}
		err = rows.Scan(
			&lua.Priority, &lua.Host, &lua.Port, &lua.UserDN, &lua.UseTLS, &lua.FQDN, &lua.CACert,
		)
		if err != nil {
			q.Logger.Infof("All LDAP user auths scanning rows failed: %v", err)
			return nil, ErrQueryFailed
		}

		luas = append(luas, lua)
	}

	if rows.Err() != nil {
		q.Logger.Infof("All LDAP user auths error after scanning rows: %v\n", rows.Err())
		return nil, ErrQueryFailed
	}
	return luas, nil
}

func (q *PostgresQuerier) GetAllLdapUsersPriorities(ctx context.Context) ([]*LdapUserPriority, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	rows, err := q.Conn.Query(queryCtx, queryLdapUsersPriorities)
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("All LDAP users priorities failed")
		return nil, ErrQueryFailed
	}
	defer rows.Close()

	var (
		lup  *LdapUserPriority
		lups []*LdapUserPriority
	)
	for rows.Next() {
		lup = &LdapUserPriority{}
		err = rows.Scan(
			&lup.Priority, &lup.UserID, &lup.LdapID)
		if err != nil {
			q.Logger.Infof("All LDAP users priorities scanning rows failed: %v", err)
			return nil, ErrQueryFailed
		}

		lups = append(lups, lup)
	}

	if rows.Err() != nil {
		q.Logger.Infof("All LDAP users priorities error after scanning rows: %v\n", rows.Err())
		return nil, ErrQueryFailed
	}
	return lups, nil
}

func (q *PostgresQuerier) GetMFACerts(ctx context.Context, mfaType string) ([]*mfa.RawCert, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	rows, err := q.Conn.Query(queryCtx, queryMFACerts, mfaType)
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("MFA CAs query failed")
		return nil, ErrQueryFailed
	}
	defer rows.Close()

	var list []*mfa.RawCert
	for rows.Next() {
		var id int
		var cert []byte
		err = rows.Scan(&id, &cert)
		if err != nil {
			q.Logger.
				WithError(err).
				Warn("MFA CAs scaning failed")
			return nil, ErrQueryFailed
		}

		list = append(list, &mfa.RawCert{
			ID:   id,
			Data: cert,
		})
	}

	if rows.Err() != nil {
		q.Logger.
			WithError(rows.Err()).
			Warn("MFA CAs error after scanning rows")
		return nil, ErrQueryFailed
	}
	return list, nil
}

func (q *PostgresQuerier) InsertMFACerts(ctx context.Context, cert []byte, mfaType string) error {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var id int
	err := q.Conn.QueryRow(
		queryCtx, queryInsertMFACert, cert, mfaType,
	).Scan(
		&id,
	)
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Insert MFA certificate CA failed")
		return ErrQueryFailed
	}
	return nil
}

func (q *PostgresQuerier) DeleteMFACerts(ctx context.Context, id int) error {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	ct, err := q.Conn.Exec(
		queryCtx, queryDeleteMFACert, id,
	)
	if err != nil {
		q.Logger.
			WithError(err).
			WithField("contextDeadline", queryTimeout).
			Warn("Delete MFA CA failed")
		return ErrQueryFailed
	}
	if ct.RowsAffected() == 0 {
		return ErrMFACertNotFound
	}
	return nil
}
