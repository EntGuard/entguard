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
	"net/netip"
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
	ipv6Bits     = 64               // IPv6 addresses are tracked by their network prefix of this length
	warnInterval = time.Minute      // minimal time between two warnings about a full map
)

// ipToKey returns the key under which the IP address is tracked. An IPv6 address is replaced by its network
// prefix, because a single client usually controls a whole prefix and can choose any of its addresses.
// An address that cannot be parsed is returned unchanged.
func ipToKey(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	addr = addr.Unmap().WithZone("")

	if addr.Is4() {
		return addr.String()
	}

	prefix, err := addr.Prefix(ipv6Bits)
	if err != nil {
		return addr.String()
	}
	return prefix.String()
}

type entry struct {
	failures     int
	lastFail     time.Time
	blockedUntil time.Time
}

// LoginGuard tracks failed login attempts per username and per IP address.
type LoginGuard struct {
	mu sync.Mutex

	// userPasswords is keyed on usernames and tracks failed password attempts for the user
	userPasswords map[string]entry
	// userMFAs is also keyed on usernames and tracks failed MFA attempts for the user
	userMFAs map[string]entry
	// ips is keyed on IP addresses and tracks any (password/MFA) failed attempts from the IP
	ips map[string]entry

	now    func() time.Time
	logger *log.Logger
	// lastWarn is the time of the last warning about a full map
	lastWarn time.Time
}

// New creates a new LoginGuard.
func New(l *log.Logger) *LoginGuard {
	return &LoginGuard{
		userPasswords: make(map[string]entry),
		userMFAs:      make(map[string]entry),
		ips:           make(map[string]entry),

		now:    time.Now,
		logger: l,
	}
}

// CheckWait returns how long the caller must wait before the next login attempt
// is allowed (0 means allowed). It must be called before the credentials are
// checked.
//
// CheckWait also records the attempt as failed, before the actual
// authentication. It performs the check-and-record atomically. This provides
// protection against parallel login requests.
//
// After authenticaion, the caller should call RecordSuccessfulAttempt (after
// successful authentication), RevertAttempt (after authentication error) or
// nothing (after failed authentication) (if authentication fails, the function
// recordFailedAttempt had already been called beforehand by CheckWait).
//
// isMFA must be false for password attempts and true for MFA attempts.
func (lg *LoginGuard) CheckWait(username, ip string, isMFA bool) time.Duration {
	ipKey := ipToKey(ip)

	lg.mu.Lock()
	defer lg.mu.Unlock()

	wait := lg.checkWait(username, ipKey, isMFA)
	if wait > 0 {
		return wait
	}

	lg.recordFailedAttempt(username, ipKey, isMFA)
	return 0
}

func (lg *LoginGuard) checkWait(username, ipKey string, isMFA bool) time.Duration {
	now := lg.now()
	wait := time.Duration(0)

	if isMFA {
		if entry, ok := lg.userMFAs[username]; ok {
			wait = max(wait, entry.blockedUntil.Sub(now))
		}
	} else {
		if entry, ok := lg.userPasswords[username]; ok {
			wait = max(wait, entry.blockedUntil.Sub(now))
		}
	}

	if entry, ok := lg.ips[ipKey]; ok {
		wait = max(wait, entry.blockedUntil.Sub(now))
	}

	return wait
}

func (lg *LoginGuard) recordFailedAttempt(username, ipKey string, isMFA bool) {
	now := lg.now()

	var mapFull bool
	if isMFA {
		mapFull = recordFailedAttemptIntoMap(lg.userMFAs, username, now)
	} else {
		mapFull = recordFailedAttemptIntoMap(lg.userPasswords, username, now)
	}
	if mapFull {
		lg.logAttackWarning("with too many usernames")
	}

	if ipKey != "" {
		mapFull = recordFailedAttemptIntoMap(lg.ips, ipKey, now)
		if mapFull {
			lg.logAttackWarning("from too many IP addresses")
		}
	}
}

func (lg *LoginGuard) logAttackWarning(msg string) {
	if lg.lastWarn.Add(warnInterval).Before(lg.now()) {
		lg.lastWarn = lg.now()
		lg.logger.Warn("Detected attempts to log in " + msg + ". This likely means " +
			"an ongoing brute force or resource exhaustion attack.")
	}
}

// RecordSuccessfulAttempt forgets failures of the given user. IP failures are
// kept, so a successful login does not clear the history of an address that is
// guessing passwords of other users. But IP failures are reverted, in order to
// revert the "failed" attempt that has been recorded by CheckWait.
func (lg *LoginGuard) RecordSuccessfulAttempt(username, ip string, isMFA bool) {
	ipKey := ipToKey(ip)

	lg.mu.Lock()
	defer lg.mu.Unlock()

	if isMFA {
		delete(lg.userMFAs, username)
	} else {
		delete(lg.userPasswords, username)
	}

	revertAttemptInMap(lg.ips, ipKey)
}

// RevertAttempt reverts one failed attempt. It should be called when the login
// attempt could not be authenticated because an error happened during
// authentication.
func (lg *LoginGuard) RevertAttempt(username, ip string, isMFA bool) {
	ipKey := ipToKey(ip)

	lg.mu.Lock()
	defer lg.mu.Unlock()

	if isMFA {
		revertAttemptInMap(lg.userMFAs, username)
	} else {
		revertAttemptInMap(lg.userPasswords, username)
	}

	revertAttemptInMap(lg.ips, ipKey)
}

func revertAttemptInMap(entries map[string]entry, key string) {
	entry, ok := entries[key]
	if ok {
		if entry.failures >= 1 {
			entry.failures--
		}
		// There is a small chance that this gets run after a parallel
		// unsuccessful attempt. However the security implication is minimal and
		// it is not worth overengineering around this
		entry.blockedUntil = time.Time{}
		entries[key] = entry
	}
}

// recordFailedAttemptIntoMap records a failure under the given key and sets the delay for the next attempt.
// It returns true if the oldest attempt is forgotten because the map is full.
func recordFailedAttemptIntoMap(entries map[string]entry, key string, now time.Time) (mapFull bool) {
	prev, ok := entries[key]
	if !ok {
		oldestRemoved := false
		// if map is full, try to make room for the new entry
		if len(entries) >= maxEntries {
			removeExpiredInMap(entries, now)
		}
		// if map is still full, remove the oldest entry
		if len(entries) >= maxEntries {
			removeOldestInMap(entries)
			oldestRemoved = true
		}
		entries[key] = entry{
			failures:     1,
			lastFail:     now,
			blockedUntil: now,
		}
		return oldestRemoved
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
	return false
}

func removeExpiredInMap(entries map[string]entry, now time.Time) {
	for k, entry := range entries {
		if now.Sub(entry.lastFail) > resetAfter {
			delete(entries, k)
		}
	}
}

func removeOldestInMap(entries map[string]entry) {
	if len(entries) == 0 {
		return
	}

	var oldestKey string
	var oldestValue entry

	// set initial values, so in the following "real" cycle we do not have to deal with empty values
	for k, entry := range entries {
		oldestKey = k
		oldestValue = entry
		break
	}

	for k, entry := range entries {
		if entry.lastFail.Before(oldestValue.lastFail) {
			oldestKey = k
			oldestValue = entry
		}
	}
	delete(entries, oldestKey)
}
