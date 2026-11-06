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

// Package loginguard implements progressive delays after repeated failed login attempts.
package loginguard

import (
	"math"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	freeAttempts = 3                // failures allowed before the first delay
	baseDelay    = time.Second      // delay after the first non-free failure, doubled with each next failure
	maxDelay     = 5 * time.Minute  // upper bound of the delay
	resetAfter   = 30 * time.Minute // failure counter is reset after this time without a failure
	maxEntries   = 100000           // upper bound of tracked keys per map (protection against memory exhaustion)
)

type entry struct {
	failures     int
	lastFail     time.Time
	blockedUntil time.Time
}

// LoginGuard tracks failed login attempts per username and per IP address.
type LoginGuard struct {
	mu    sync.Mutex
	users map[string]entry
	ips   map[string]entry
	now   func() time.Time

	logger *log.Logger
}

// New creates a new LoginGuard.
func New(l *log.Logger) *LoginGuard {
	return &LoginGuard{
		users: make(map[string]entry),
		ips:   make(map[string]entry),
		now:   time.Now,

		logger: l,
	}
}

// CheckWait returns how long the caller must wait before the next login attempt is allowed (0 means allowed).
// It must be called before the credentials are checked.
func (lg *LoginGuard) CheckWait(username, ip string) time.Duration {
	lg.mu.Lock()
	defer lg.mu.Unlock()

	now := lg.now()
	wait := time.Duration(0)
	if entry, ok := lg.users[username]; ok {
		wait = max(wait, entry.blockedUntil.Sub(now))
	}
	if entry, ok := lg.ips[ip]; ok {
		wait = max(wait, entry.blockedUntil.Sub(now))
	}
	return wait
}

// RecordFailedAttempt records a failed login attempt. It must be called only
// for attempts where the credentials have been actually checked (it must NOT be
// called for attempts blocked by CheckWait).
func (lg *LoginGuard) RecordFailedAttempt(username, ip string) {
	lg.mu.Lock()
	defer lg.mu.Unlock()

	now := lg.now()
	bruteForceMsg := "Detected attempts to log in %s. This likely means " +
		"an ongoing brute force or resource exhaustion attack."

	success := recordFailedAttempt(lg.users, username, now)
	if !success {
		lg.logger.Warnf(bruteForceMsg, "with too many usernames")
	}
	if ip != "" {
		success = recordFailedAttempt(lg.ips, ip, now)
		if !success {
			lg.logger.Warnf(bruteForceMsg, "from too many IP addresses")
		}
	}
}

// RecordSuccessfulAttempt forgets failures of the given user. IP failures are kept, so a successful login
// does not clear the history of an address that is guessing passwords of other users.
func (lg *LoginGuard) RecordSuccessfulAttempt(username string) {
	lg.mu.Lock()
	defer lg.mu.Unlock()

	delete(lg.users, username)
}

// recordFailedAttempt records a failure under the given key and sets the delay for the next attempt.
// It returns false if the login attempt could not be recorded because the map is full.
func recordFailedAttempt(entries map[string]entry, key string, now time.Time) (success bool) {
	prev, ok := entries[key]
	if !ok {
		// if map is full, try to make room for the new entry
		if len(entries) >= maxEntries {
			removeExpired(entries, now)
		}
		if len(entries) >= maxEntries {
			return false
		}
		entries[key] = entry{
			failures:     1,
			lastFail:     now,
			blockedUntil: now,
		}
		return true
	}

	failures := prev.failures + 1
	if now.Sub(prev.lastFail) > resetAfter {
		failures = 1
	}
	blockedUntil := now
	if failures > freeAttempts {
		// cap exponent to prevent overflowing time.Duration
		exponent := min(failures-freeAttempts-1, 20)
		newDelay := baseDelay * time.Duration(math.Pow(2, float64(exponent)))
		blockedUntil = now.Add(min(newDelay, maxDelay))
	}
	entries[key] = entry{
		failures:     failures,
		lastFail:     now,
		blockedUntil: blockedUntil,
	}
	return true
}

func removeExpired(entries map[string]entry, now time.Time) {
	for k, entry := range entries {
		if now.Sub(entry.lastFail) > resetAfter {
			delete(entries, k)
		}
	}
}
