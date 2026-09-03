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

package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"

	"github.com/entguard/entguard/db"
	"github.com/entguard/entguard/service/config"
)

func Connect(ctx context.Context, logger *log.Logger, config *config.PostgresConfig) (*pgxpool.Pool, error) {
	const (
		initConnPeriod   = 3 * time.Second
		initConnAttempts = 3
	)
	var (
		err  error
		conn *pgxpool.Pool
	)
	for i := 1; i <= initConnAttempts; i++ {
		logger.
			WithField("attempt", i).
			Debug("Trying to connect to the database")
		conn, err = pgxpool.New(ctx, config.ToURL())
		if err == nil {
			logger.
				WithField("attempt", i).
				Info("Initial connection with the database established")
			break
		}
		logger.
			WithError(err).
			WithField("attempt", i).
			Debug("Failed to connect")
		if i < initConnAttempts {
			logger.
				WithField("attempt", i).
				WithField("period", initConnPeriod).
				Debug("Next try after period")
			time.Sleep(initConnPeriod)
		}
	}
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, errors.New("connection attempt returned nil connection despite nil error (should not happen)")
	}
	return conn, nil
}

// ConnectOnce is like Connect, but it does not do retries and it does not require logger.
// It is meant for interactive use (for egvpn).
func ConnectOnce(ctx context.Context, config *config.PostgresConfig) (*pgxpool.Pool, error) {
	conn, err := pgxpool.New(ctx, config.ToURL())
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, errors.New("connection attempt returned nil connection despite nil error (should not happen)")
	}
	return conn, nil
}

func NewPgTimestamp(t time.Time) pgtype.Timestamp {
	return pgtype.Timestamp{
		Time:  t.UTC(),
		Valid: true,
	}
}

func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(q *db.Queries) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		rbErr := tx.Rollback(ctx)
		if rbErr != nil && rbErr != pgx.ErrTxClosed {
			err = errors.Join(err, fmt.Errorf("failed to rollback transaction: %w", rbErr))
		}
	}()

	qtx := db.New(tx)
	if err := fn(qtx); err != nil {
		return err
	}

	// Check if the context already has a deadline.
	// If it does, extend the deadline by the specified extension duration
	// to ensure the transaction has enough time to complete.
	commitCtx := ctx
	if deadline, ok := ctx.Deadline(); ok {
		newDeadline := deadline.Add(10 * time.Second)
		c, cancel := context.WithDeadline(ctx, newDeadline)
		defer cancel()
		commitCtx = c
	}

	err = tx.Commit(commitCtx)
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return err
}

func IntToPtr(i int) *int {
	if i == 0 {
		return nil
	}
	return &i
}

func IntFromPtr(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}

func StringToPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func StringFromPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
