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

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"
)

const (
	serverIDCheckQuery   = `SELECT 1 FROM servers WHERE id=$1 LIMIT 1;`
	serverNameCheckQuery = `SELECT 1 FROM servers WHERE name=$1 LIMIT 1;`
	allServersQuery      = `SELECT id, name, endpoint, description, healthcheck_address FROM servers`
	getServerByIDQuery   = `SELECT name, endpoint, description, healthcheck_address FROM servers WHERE id=$1`
	getServerByNameQuery = `SELECT id, endpoint, description, healthcheck_address FROM servers WHERE name=$1`
	updateServerQuery    = `UPDATE servers SET name=$1, endpoint=$2, description=$3, healthcheck_address=$4 WHERE id=$5`
	deleteServerQuery    = `DELETE FROM servers WHERE id=$1`

	getServerReplicas      = `SELECT running_replicas FROM servers WHERE id=$1`
	increaseServerReplicas = `UPDATE servers SET running_replicas = running_replicas + 1 WHERE id=$1`
	decreaseServerReplicas = `UPDATE servers SET running_replicas = running_replicas - 1 WHERE running_replicas > 0 AND id=$1`

	getServerTagsQuery   = `SELECT tag FROM tags WHERE server_id=$1`
	insertServerTagQuery = `INSERT INTO tags (tag, created, server_id) VALUES ($1, $2, $3) RETURNING id;`
	getServerTagsByName  = `SELECT tag FROM tags WHERE server_id IN (SELECT id FROM servers WHERE name=$1) ORDER BY created DESC`
	deleteTagsQuery      = `TRUNCATE TABLE tags`
)

const (
	caller          = "VCM PostgreSQL querier"
	QueryTimeout    = 5 * time.Second
	unlistenTimeout = 1 * time.Second
)

// VPNConfigQuerier makes VPN configuration related queries to PostgreSQL.
type VPNConfigQuerier struct {
	Conn *pgxpool.Pool
	// SqlcQ  *sqlc.Queries
	Logger *log.Entry
}

func NewQuerier(ctx context.Context, conn *pgxpool.Pool, l *log.Logger) *VPNConfigQuerier {
	// sqlcQ := sqlc.New(conn)
	return &VPNConfigQuerier{
		Conn:   conn,
		Logger: l.WithField("reportCaller", caller),
		// SqlcQ:  sqlcQ,
	}
}

func (q *VPNConfigQuerier) GetDbConnection(ctx context.Context) *pgxpool.Pool {
	return q.Conn
}

// CheckServerIDExists returns true and no error if a server with given ID already exists.
func (q *VPNConfigQuerier) CheckServerIDExists(ctx context.Context, id int) (bool, error) {
	queryCtx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	var val int
	err := q.Conn.QueryRow(
		queryCtx, serverIDCheckQuery, id,
	).Scan(&val)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	return true, nil
}

// CheckServerNameExists returns true and no error if a server with given name already exists.
func (q *VPNConfigQuerier) CheckServerNameExists(ctx context.Context, name string) (bool, error) {
	queryCtx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	var val int
	err := q.Conn.QueryRow(
		queryCtx, serverNameCheckQuery, name,
	).Scan(&val)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	return true, nil
}

// ListenForWGPeerUpdates sets up and starts listening for database notifications informing about changes
// made to the 'adjacencies' table. See 'trigger_wg_peers_update' trigger in migration scripts for more details.
func (q *VPNConfigQuerier) ListenForWGPeerUpdates(ctx context.Context, fails chan error) (rawNotifications <-chan string) {
	return q.listenForDBNotification("peer_inserted", ctx, fails)
}

// ListenForWGConfigsUpdates sets up and starts listening for database notifications informing about changes
// made to the 'server_wg_configs' table. See 'trigger_server_wg_configs_update' trigger in 20250729145056-v1.12.0-split-wg-configs.sql for more details.
func (q *VPNConfigQuerier) ListenForWGConfigsUpdates(ctx context.Context, fails chan error) (rawNotifications <-chan string) {
	return q.listenForDBNotification("server_wg_configs_updated", ctx, fails)
}

// ListenForDevicesSessionsUpdates sets up and starts listening for database notifications informing about changes
// made to the 'devices' table 'session_id' column. See 'trigger_sessions_update' trigger in 20251013121636-v1.12.0-update-session-trigger-to-listen-device-table.sql for more details.
func (q *VPNConfigQuerier) ListenForDevicesSessionsUpdates(ctx context.Context, fails chan error) (rawNotifications <-chan string) {
	return q.listenForDBNotification("session_updated", ctx, fails)
}

func (q *VPNConfigQuerier) listenForDBNotification(notificationDBChannelName string, ctx context.Context, fails chan error) <-chan string {
	rawNotifications := make(chan string)
	go func() {
		defer close(rawNotifications)

		conn, err := q.Conn.Acquire(ctx)
		if err != nil {
			fails <- fmt.Errorf("can't acquire database connection pool: %v", err)
		}
		defer conn.Release()

		queryCtx, cancel := context.WithTimeout(ctx, QueryTimeout)
		defer cancel()

		_, err = conn.Exec(queryCtx, "listen "+notificationDBChannelName)
		if err != nil {
			fails <- fmt.Errorf("can't execute 'listen %s' query: %v", notificationDBChannelName, err)
		}
		defer func() {
			unlistenCtx, unlistenCancel := context.WithTimeout(context.Background(), unlistenTimeout)

			_, err = conn.Exec(unlistenCtx, "unlisten "+notificationDBChannelName)
			if err != nil {
				q.Logger.Errorf("can't execute 'unlisten %s' query: %v", notificationDBChannelName, err)
			}
			unlistenCancel()
		}()

		for {
			if e := ctx.Err(); e != nil {
				fails <- e
				return
			}

			notification, err := conn.Conn().WaitForNotification(ctx)
			if err == nil {
				rawNotifications <- notification.Payload
			} else {
				fails <- fmt.Errorf("waiting for database notification failed: %v", err)
				return
			}
		}
	}()
	return rawNotifications
}

// GetAllServers returns list of servers.
func (q *VPNConfigQuerier) GetAllServers(ctx context.Context) ([]*Server, error) {
	queryCtx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	rows, err := q.Conn.Query(queryCtx, allServersQuery)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	defer rows.Close()

	var (
		srvr  *Server
		srvrs []*Server
	)
	for rows.Next() {
		srvr = &Server{}
		err := rows.Scan(
			&srvr.ID,
			&srvr.Name,
			&srvr.Endpoint,
			&srvr.Description,
			&srvr.HealthCheckAddress,
		)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
		}
		srvrs = append(srvrs, srvr)
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("%w: %v", ErrQueryFailed, rows.Err())
	}
	return srvrs, nil
}

// GetReplicaCount returns the number of currently running replicas of the given server.
func (q *VPNConfigQuerier) GetReplicaCount(ctx context.Context, serverID int) (int, error) {
	queryCtx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	var count int
	err := q.Conn.QueryRow(queryCtx, getServerReplicas, serverID).Scan(&count)
	if err == pgx.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	return count, nil
}

// IncreaseReplicaCount increases the current replica count of the given server by 1.
func (q *VPNConfigQuerier) IncreaseReplicaCount(ctx context.Context, serverID int) error {
	queryCtx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	ct, err := q.Conn.Exec(
		queryCtx, increaseServerReplicas, serverID)

	if err != nil {
		return fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DecreaseReplicaCount decreases the current replica count of the given server by 1.
func (q *VPNConfigQuerier) DecreaseReplicaCount(ctx context.Context, serverID int) error {
	queryCtx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	ct, err := q.Conn.Exec(
		queryCtx, decreaseServerReplicas, serverID)

	if err != nil {
		return fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetServerByID returns server by its ID.
func (q *VPNConfigQuerier) GetServerByID(ctx context.Context, id int) (*Server, error) {
	queryCtx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	srvr := &Server{
		ID: id,
	}
	err := q.Conn.QueryRow(queryCtx, getServerByIDQuery, id).Scan(
		&srvr.Name,
		&srvr.Endpoint,
		&srvr.Description,
		&srvr.HealthCheckAddress,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	return srvr, nil
}

// GetServerByName returns server by its name.
func (q *VPNConfigQuerier) GetServerByName(ctx context.Context, name string) (*Server, error) {
	queryCtx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	srvr := &Server{
		Name: name,
	}
	err := q.Conn.QueryRow(queryCtx, getServerByNameQuery, name).Scan(
		&srvr.ID,
		&srvr.Endpoint,
		&srvr.Description,
		&srvr.HealthCheckAddress,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	return srvr, nil
}

// UpdateServer modifies the server in the database.
func (q *VPNConfigQuerier) UpdateServer(ctx context.Context, s *Server) error {
	queryCtx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	ct, err := q.Conn.Exec(
		queryCtx, updateServerQuery,
		s.Name, s.Endpoint, s.Description, s.HealthCheckAddress, s.ID)

	if err != nil {
		return fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetServerTags returns all tags that belong to server with specified ID. The tag is unique identification of server
// instance for given server ID (serverID is unique ID of server configuration that can be applied to multiple
// server instances(HA setup) that are uniquely identified by tag).
func (q *VPNConfigQuerier) GetServerTags(ctx context.Context, serverID int) ([]string, error) {
	queryCtx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	rows, err := q.Conn.Query(queryCtx, getServerTagsQuery, serverID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	defer rows.Close()

	var (
		tag  string
		tags []string
	)
	for rows.Next() {
		err := rows.Scan(&tag)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
		}
		tags = append(tags, tag)
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("%w: %v", ErrQueryFailed, rows.Err())
	}
	return tags, nil
}

// InsertUsedTag inserts tag to tags table.
func (q *VPNConfigQuerier) InsertUsedTag(ctx context.Context, tag string, serverID int) error {
	queryCtx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	var id int
	err := q.Conn.QueryRow(
		queryCtx, insertServerTagQuery, tag, time.Now(), serverID,
	).Scan(&id)

	if err != nil {
		return fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	return nil
}

// GetServerTagsByName returns all tags that belong to the server with the specified name,
// ordered by creation time descending.
func (q *VPNConfigQuerier) GetServerTagsByName(ctx context.Context, serverName string) ([]string, error) {
	queryCtx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	rows, err := q.Conn.Query(queryCtx, getServerTagsByName, serverName)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	defer rows.Close()

	var (
		tag  string
		tags []string
	)
	for rows.Next() {
		err := rows.Scan(&tag)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
		}
		tags = append(tags, tag)
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("%w: %v", ErrQueryFailed, rows.Err())
	}

	return tags, nil
}

// DeleteUsedTags truncates tags table.
func (q *VPNConfigQuerier) DeleteUsedTags(ctx context.Context) error {
	queryCtx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	_, err := q.Conn.Exec(queryCtx, deleteTagsQuery)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	return nil
}

// DeleteServer deletes server from the database.
func (q *VPNConfigQuerier) DeleteServer(ctx context.Context, id int) error {
	queryCtx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	_, err := q.Conn.Exec(queryCtx, deleteServerQuery, id)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	return nil
}
