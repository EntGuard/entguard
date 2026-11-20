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
	freeAttempts           = 3                   // failures allowed before the first delay
	baseDelay              = time.Second         // delay after the first non-free failure, doubled with each next failure
	maxDelay               = 5 * time.Minute     // upper bound of the delay
	resetAfter             = 30 * time.Minute    // failure counter is reset after this time without a failure
	maxEntries             = 100000              // upper bound of tracked keys per map (protection against memory exhaustion)
	knownClientTrustPeriod = 30 * 24 * time.Hour // client is considered as known for this time after its last successful login
	ipv6Bits               = 64                  // IPv6 addresses are tracked by their network prefix of this length
	warnInterval           = time.Minute         // minimal time between two warnings about a full map
)

type ipKey string

// ipToIpKey returns the key under which the IP address is tracked. An IPv6 address is replaced by its network
// prefix, because a single client usually controls a whole prefix and can choose any of its addresses.
// An address that cannot be parsed is returned unchanged.
func ipToIpKey(ip string) ipKey {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ipKey(ip)
	}
	addr = addr.Unmap().WithZone("")

	if addr.Is4() {
		return ipKey(addr.String())
	}

	prefix, err := addr.Prefix(ipv6Bits)
	if err != nil {
		return ipKey(addr.String())
	}
	return ipKey(prefix.String())
}

type entry struct {
	failures     int
	lastFail     time.Time
	blockedUntil time.Time
	lastSuccess  time.Time // used only by entries of known clients
}

type knownClient struct {
	username string
	ipK      ipKey
}

type mapKey interface{ string | ipKey | knownClient }

// LoginGuard tracks failed login attempts per username and per IP address.
//
// DOS prevention: A user who has successfully logged in from an address gets
// tracked as a known client (username-ip pair). Attempts of a known client are
// not limited by the failures of its username (which can be caused by anybody
// who knows the username). They are limited by failures of the client itself
// instead. So an attacker guessing the password of a user from elsewhere does
// not lock out the user at the places where the user has logged in before.
// Known clients are tracked separately for password and MFA attempts, the same
// way as usernames.
type LoginGuard struct {
	mu sync.Mutex

	// userPasswords is keyed on usernames and tracks failed password attempts for the user
	userPasswords map[string]entry
	// userMFAs is also keyed on usernames and tracks failed MFA attempts for the user
	userMFAs map[string]entry
	// ips is keyed on IP addresses and tracks any (password/MFA) failed attempts from the IP
	ips map[ipKey]entry

	// knownClientPasswords tracks known clients and their failed password attempts
	knownClientPasswords map[knownClient]entry
	// knownClientMFAs tracks known clients and their failed MFA attempts
	knownClientMFAs map[knownClient]entry

	now    func() time.Time
	logger *log.Logger
	// following variables are the times of the last warnings about a full map
	lastWarnUsernames    time.Time
	lastWarnIPs          time.Time
	lastWarnKnownClients time.Time
}

const (
	warnUsernames = iota
	warnIPs
	warnKnownClients
)

// New creates a new LoginGuard.
func New(l *log.Logger) *LoginGuard {
	return &LoginGuard{
		userPasswords: make(map[string]entry),
		userMFAs:      make(map[string]entry),
		ips:           make(map[ipKey]entry),

		knownClientPasswords: make(map[knownClient]entry),
		knownClientMFAs:      make(map[knownClient]entry),

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
	ipK := ipToIpKey(ip)

	lg.mu.Lock()
	defer lg.mu.Unlock()

	wait := lg.checkWait(username, ipK, isMFA)
	if wait > 0 {
		return wait
	}

	lg.recordFailedAttempt(username, ipK, isMFA)
	return 0
}

func (lg *LoginGuard) checkWait(username string, ipK ipKey, isMFA bool) time.Duration {
	now := lg.now()
	wait := time.Duration(0)

	isKnown := lg.isClientKnown(username, ipK, isMFA)
	if isKnown && isMFA {
		if entry, ok := lg.knownClientMFAs[knownClient{username, ipK}]; ok {
			wait = max(wait, entry.blockedUntil.Sub(now))
		}
	} else if isKnown {
		if entry, ok := lg.knownClientPasswords[knownClient{username, ipK}]; ok {
			wait = max(wait, entry.blockedUntil.Sub(now))
		}
	} else if isMFA {
		if entry, ok := lg.userMFAs[username]; ok {
			wait = max(wait, entry.blockedUntil.Sub(now))
		}
	} else {
		if entry, ok := lg.userPasswords[username]; ok {
			wait = max(wait, entry.blockedUntil.Sub(now))
		}
	}

	if entry, ok := lg.ips[ipK]; ok {
		wait = max(wait, entry.blockedUntil.Sub(now))
	}

	return wait
}

func (lg *LoginGuard) isClientKnown(username string, ipK ipKey, isMFA bool) bool {
	if isMFA {
		return isClientKnownInMap(lg.knownClientMFAs, knownClient{username, ipK}, lg.now())
	}
	return isClientKnownInMap(lg.knownClientPasswords, knownClient{username, ipK}, lg.now())
}

func isClientKnownInMap(entries map[knownClient]entry, key knownClient, now time.Time) bool {
	entry, ok := entries[key]
	if !ok {
		return false
	}
	if now.Sub(entry.lastSuccess) > knownClientTrustPeriod {
		delete(entries, key)
		return false
	}
	return true
}

func (lg *LoginGuard) recordFailedAttempt(username string, ipK ipKey, isMFA bool) {
	now := lg.now()

	isKnown := lg.isClientKnown(username, ipK, isMFA)
	var mapFull bool
	if isKnown && isMFA {
		mapFull = recordFailedAttemptIntoMap(lg.knownClientMFAs, knownClient{username, ipK}, now)
	} else if isKnown {
		mapFull = recordFailedAttemptIntoMap(lg.knownClientPasswords, knownClient{username, ipK}, now)
	} else if isMFA {
		mapFull = recordFailedAttemptIntoMap(lg.userMFAs, username, now)
	} else {
		mapFull = recordFailedAttemptIntoMap(lg.userPasswords, username, now)
	}
	if mapFull {
		lg.logAttackWarning(warnUsernames)
	}

	if ipK != "" {
		mapFull = recordFailedAttemptIntoMap(lg.ips, ipK, now)
		if mapFull {
			lg.logAttackWarning(warnIPs)
		}
	}
}

func (lg *LoginGuard) logAttackWarning(msg int) {
	switch msg {
	case warnUsernames:
		if lg.now().Sub(lg.lastWarnUsernames) > warnInterval {
			lg.lastWarnUsernames = lg.now()
			lg.logger.Warn("Detected attempts to log in with too many usernames. This likely means " +
				"an ongoing brute force or resource exhaustion attack.")
		}
	case warnIPs:
		if lg.now().Sub(lg.lastWarnIPs) > warnInterval {
			lg.lastWarnIPs = lg.now()
			lg.logger.Warn("Detected attempts to log in from too many IP addresses. This likely means " +
				"an ongoing brute force or resource exhaustion attack.")
		}
	case warnKnownClients:
		if lg.now().Sub(lg.lastWarnKnownClients) > warnInterval {
			lg.lastWarnKnownClients = lg.now()
			lg.logger.Warn("Detected successful logins with too many clients. This likely means " +
				"a rogue insider or an attacker with correctly guessed password is performing resource exhaustion attack.")
		}
	}
}

// RecordSuccessfulAttempt forgets failures of the given user and adds/updates
// the client (username-ip pair) in known clients map.
//
// IP failures are kept, so a successful login does not clear the history of an
// address that is guessing passwords of other users. But IP failures are
// reverted, in order to revert the "failed" attempt that has been recorded by
// CheckWait.
func (lg *LoginGuard) RecordSuccessfulAttempt(username, ip string, isMFA bool) {
	ipK := ipToIpKey(ip)

	lg.mu.Lock()
	defer lg.mu.Unlock()

	isKnown := lg.isClientKnown(username, ipK, isMFA)
	var mapFull bool
	if isMFA {
		if !isKnown {
			delete(lg.userMFAs, username)
		}
		mapFull = addKnownClientIntoMap(lg.knownClientMFAs, knownClient{username, ipK}, lg.now())
	} else {
		if !isKnown {
			delete(lg.userPasswords, username)
		}
		mapFull = addKnownClientIntoMap(lg.knownClientPasswords, knownClient{username, ipK}, lg.now())
	}
	if mapFull {
		lg.logAttackWarning(warnKnownClients)
	}

	revertAttemptInMap(lg.ips, ipK)
}

// RevertAttempt reverts one failed attempt. It should be called when the login
// attempt could not be authenticated because an error happened during
// authentication.
func (lg *LoginGuard) RevertAttempt(username, ip string, isMFA bool) {
	ipK := ipToIpKey(ip)

	lg.mu.Lock()
	defer lg.mu.Unlock()

	isKnown := lg.isClientKnown(username, ipK, isMFA)
	if isKnown && isMFA {
		revertAttemptInMap(lg.knownClientMFAs, knownClient{username, ipK})
	} else if isKnown {
		revertAttemptInMap(lg.knownClientPasswords, knownClient{username, ipK})
	} else if isMFA {
		revertAttemptInMap(lg.userMFAs, username)
	} else {
		revertAttemptInMap(lg.userPasswords, username)
	}

	revertAttemptInMap(lg.ips, ipK)
}

func revertAttemptInMap[T mapKey](entries map[T]entry, key T) {
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
func recordFailedAttemptIntoMap[T mapKey](entries map[T]entry, key T, now time.Time) (mapFull bool) {
	prev, ok := entries[key]
	if !ok {
		oldestRemoved := makeRoomInMap(entries, now)
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
		lastSuccess:  prev.lastSuccess,
	}
	return false
}

// addKnownClientIntoMap adds or updates a known client entry.
// It returns true if some known client is forgotten because the map is full.
func addKnownClientIntoMap(entries map[knownClient]entry, key knownClient, now time.Time) (mapFull bool) {
	if key.ipK == "" {
		return
	}
	oldestRemoved := false

	_, ok := entries[key]
	if !ok {
		oldestRemoved = makeRoomInMap(entries, now)
	}
	entries[key] = entry{
		lastSuccess: now,
	}
	return oldestRemoved
}

func makeRoomInMap[T mapKey](entries map[T]entry, now time.Time) (oldestRemoved bool) {
	_, isKnownClient := any(entries).(map[knownClient]entry)

	if len(entries) >= maxEntries {
		if isKnownClient {
			removeStaleKnownClients(entries, now)
		} else {
			removeExpiredInMap(entries, now)
		}
	}
	if len(entries) >= maxEntries {
		removeOldestInMap(entries)
		return true
	}
	return false
}

func removeExpiredInMap[T mapKey](entries map[T]entry, now time.Time) {
	for k, entry := range entries {
		if now.Sub(entry.lastFail) > resetAfter {
			delete(entries, k)
		}
	}
}

func removeStaleKnownClients[T mapKey](entries map[T]entry, now time.Time) {
	for k, entry := range entries {
		if now.Sub(entry.lastSuccess) > knownClientTrustPeriod {
			delete(entries, k)
		}
	}
}

func removeOldestInMap[T mapKey](entries map[T]entry) {
	if len(entries) == 0 {
		return
	}

	var oldestKey T
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
