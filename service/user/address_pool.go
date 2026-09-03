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
	"github.com/entguard/entguard/pkg/db"
	"github.com/entguard/entguard/service/api/v1"
)

const (
	maxIP4Mask = 32
	maxIP6Mask = 128
)

func (m *PostgresManager) GetAllAddressPools(ctx context.Context) ([]*api.AddressPool, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	dbPools, err := m.SQLcQ.GetAllAddressPools(queryCtx)
	if err != nil {
		return nil, err
	}
	var pools []*api.AddressPool
	for _, p := range dbPools {
		pools = append(pools, ConvertDBToAPIPool(p))
	}
	return pools, nil
}

func (m *PostgresManager) GetAddressPoolByID(ctx context.Context, id int) (*api.AddressPool, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	p, err := m.SQLcQ.GetAddressPoolByID(queryCtx, id)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return ConvertDBToAPIPool(p), nil
}

func (m *PostgresManager) CreateAddressPool(ctx context.Context, pool *api.AddressPool) (*api.AddressPool, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := m.SQLcQ.GetAddressPoolByName(queryCtx, pool.Name)
	if !errors.Is(err, pgx.ErrNoRows) {
		if err != nil {
			return nil, fmt.Errorf("GetAddressPoolByName: %w", err)
		}
		return nil, ErrAlreadyExists
	}

	start, err := netip.ParseAddr(pool.StartAddr)
	if err != nil {
		return nil, err
	}
	end, err := netip.ParseAddr(pool.EndAddr)
	if err != nil {
		return nil, err
	}
	p, err := m.SQLcQ.InsertAddressPool(queryCtx, sqlc.InsertAddressPoolParams{
		Name:        pool.Name,
		StartAddr:   start,
		EndAddr:     end,
		NetMask:     pool.NetMask,
		Description: pool.Description,
	})
	if err != nil {
		return nil, err
	}
	return ConvertDBToAPIPool(p), nil
}

func (m *PostgresManager) UpdateAddressPool(ctx context.Context, id int, pool *api.AddressPool) (*api.AddressPool, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	existing, err := m.SQLcQ.GetAddressPoolByID(queryCtx, id)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetAddressPoolByID: %w", err)
	}
	if existing.Name != pool.Name {
		_, err = m.SQLcQ.GetAddressPoolByName(queryCtx, pool.Name)
		if !errors.Is(err, pgx.ErrNoRows) {
			if err != nil {
				return nil, fmt.Errorf("GetAddressPoolByName: %w", err)
			}
			return nil, ErrAlreadyExists
		}
	}

	start, err := netip.ParseAddr(pool.StartAddr)
	if err != nil {
		return nil, err
	}
	end, err := netip.ParseAddr(pool.EndAddr)
	if err != nil {
		return nil, err
	}
	p, err := m.SQLcQ.UpdateAddressPool(queryCtx, sqlc.UpdateAddressPoolParams{
		ID:          id,
		Name:        pool.Name,
		StartAddr:   start,
		EndAddr:     end,
		NetMask:     pool.NetMask,
		Description: pool.Description,
		UpdatedAt:   db.NewPgTimestamp(time.Now()),
	})
	if err != nil {
		return nil, err
	}
	return ConvertDBToAPIPool(p), nil
}

func (m *PostgresManager) DeleteAddressPool(ctx context.Context, id int) error {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := m.SQLcQ.DeleteAddressPool(queryCtx, id)
	if err == pgx.ErrNoRows {
		return ErrNotFound
	}
	return err
}

func (m *PostgresManager) ValidateAddressPool(p *api.AddressPool) error {
	if p.Name == "" {
		return fmt.Errorf("name cannot be empty")
	}

	start, err := netip.ParseAddr(p.StartAddr)
	if err != nil {
		return fmt.Errorf("parse start address: %w", err)
	}

	end, err := netip.ParseAddr(p.EndAddr)
	if err != nil {
		return fmt.Errorf("parse end address: %w", err)
	}

	if start.Compare(end) > 0 {
		return fmt.Errorf("start address=%s must be less than or equal to end address=%s",
			p.StartAddr, p.EndAddr)
	}

	maxMask := maxIP4Mask
	if start.Is6() || end.Is6() {
		maxMask = maxIP6Mask
	}

	if p.NetMask < 0 || p.NetMask > maxMask {
		return fmt.Errorf("invalid network mask=%d", p.NetMask)
	}

	prefix := netip.PrefixFrom(start, p.NetMask)
	if !prefix.Contains(start) || !prefix.Contains(end) {
		return fmt.Errorf("start address and end address must be within subnet %s", prefix.String())
	}

	return nil
}

func ConvertDBToAPIPool(p sqlc.AddressPool) *api.AddressPool {
	return &api.AddressPool{
		ID:          p.ID,
		Name:        p.Name,
		StartAddr:   p.StartAddr.String(),
		EndAddr:     p.EndAddr.String(),
		NetMask:     p.NetMask,
		Description: p.Description,
	}
}
