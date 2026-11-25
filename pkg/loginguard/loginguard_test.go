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

package loginguard

import (
	"strconv"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"
)

func newTestLoginGuard(l *log.Logger) *LoginGuard {
	lg := New(l)

	now := time.Unix(1000000, 0)
	lg.now = func() time.Time { return now }
	return lg
}

func advanceTime(lg *LoginGuard, d time.Duration) {
	now := lg.now().Add(d)
	lg.now = func() time.Time { return now }
}

func recordNFailedAttempts(lg *LoginGuard, n int, username, ip string) {
	for range n {
		lg.recordFailedAttempt(username, ipToIpKey(ip), false)
	}
}

func recordNFailedAttemptsMFA(lg *LoginGuard, n int, username, ip string) {
	for range n {
		lg.recordFailedAttempt(username, ipToIpKey(ip), true)
	}
}

// successfulLogin performs a successful login the same way as the callers of LoginGuard do
func successfulLogin(t *testing.T, lg *LoginGuard, username, ip string, isMFA bool) {
	t.Helper()
	wait := lg.CheckWaitAndRecordFailedAttempt(username, ip, isMFA)
	require.Zero(t, wait, "login of %q from %q must not be delayed", username, ip)
	lg.RecordSuccessfulAttempt(username, ip, isMFA)
}

// failedLoginFromDistinctIPs performs n failed logins the same way as the callers of LoginGuard do, each from a different
// IP address, so that only the username counter (not the IP counter) causes delays
func failedLoginFromDistinctIPs(lg *LoginGuard, n int, username string, isMFA bool) {
	for i := range n {
		lg.CheckWaitAndRecordFailedAttempt(username, "1.2.3."+strconv.Itoa(i), isMFA)
	}
}

func TestCheckWait(t *testing.T) {
	type check struct {
		username string
		ip       string
		want     time.Duration
	}

	cases := []struct {
		name   string
		setup  func(lg *LoginGuard)
		checks []check
	}{
		{
			name:   "no failures",
			setup:  func(lg *LoginGuard) {},
			checks: []check{{"alice", "1.1.1.1", 0}},
		},
		{
			name: "free attempts are not delayed",
			setup: func(lg *LoginGuard) {
				recordNFailedAttempts(lg, freeAttempts, "alice", "1.1.1.1")
			},
			checks: []check{{"alice", "1.1.1.1", 0}},
		},
		{
			name: "first delay",
			setup: func(lg *LoginGuard) {
				recordNFailedAttempts(lg, freeAttempts+1, "alice", "1.1.1.1")
			},
			checks: []check{{"alice", "1.1.1.1", baseDelay}},
		},
		{
			name: "delay doubles with each failure",
			setup: func(lg *LoginGuard) {
				recordNFailedAttempts(lg, freeAttempts+5, "alice", "1.1.1.1")
			},
			checks: []check{{"alice", "1.1.1.1", 16 * baseDelay}},
		},
		{
			name: "delay is capped",
			setup: func(lg *LoginGuard) {
				recordNFailedAttempts(lg, 200, "alice", "1.1.1.1")
			},
			checks: []check{{"alice", "1.1.1.1", maxDelay}},
		},
		{
			name: "remaining delay decreases as time passes",
			setup: func(lg *LoginGuard) {
				recordNFailedAttempts(lg, freeAttempts+4, "alice", "1.1.1.1")
				advanceTime(lg, 5*baseDelay)
			},
			checks: []check{{"alice", "1.1.1.1", 3 * baseDelay}},
		},
		{
			name: "remaining delay ended after waiting",
			setup: func(lg *LoginGuard) {
				recordNFailedAttempts(lg, freeAttempts+4, "alice", "1.1.1.1")
				advanceTime(lg, 10*baseDelay)
			},
			checks: []check{{"alice", "1.1.1.1", 0}},
		},
		{
			name: "new free attempts granted after idle time",
			setup: func(lg *LoginGuard) {
				recordNFailedAttempts(lg, freeAttempts+4, "alice", "1.1.1.1")
				advanceTime(lg, resetAfter+time.Second)
				recordNFailedAttempts(lg, freeAttempts, "alice", "1.1.1.1")
			},
			checks: []check{{"alice", "1.1.1.1", 0}},
		},
		{
			name: "failures from an IP preserved for different users (single node attack)",
			setup: func(lg *LoginGuard) {
				recordNFailedAttempts(lg, freeAttempts+1, "alice", "1.1.1.1")
			},
			checks: []check{{"bob", "1.1.1.1", baseDelay}},
		},
		{
			name: "failures of a user preserved for different IPs (distributed attack)",
			setup: func(lg *LoginGuard) {
				recordNFailedAttempts(lg, freeAttempts+1, "alice", "1.1.1.1")
			},
			checks: []check{{"alice", "2.2.2.2", baseDelay}},
		},
		{
			name: "unaffected user and IP",
			setup: func(lg *LoginGuard) {
				recordNFailedAttempts(lg, freeAttempts+1, "alice", "1.1.1.1")
			},
			checks: []check{{"bob", "2.2.2.2", 0}},
		},
		{
			name: "longer of user and IP delay wins",
			setup: func(lg *LoginGuard) {
				recordNFailedAttempts(lg, freeAttempts+1, "alice", "1.1.1.1")
				recordNFailedAttempts(lg, 2, "alice", "2.2.2.2")

				recordNFailedAttempts(lg, freeAttempts+1, "bob", "10.10.10.10")
				recordNFailedAttempts(lg, 1, "charlie", "10.10.10.10")
			},
			checks: []check{
				{"alice", "1.1.1.1", 4 * baseDelay},
				{"alice", "2.2.2.2", 4 * baseDelay},
				{"bob", "10.10.10.10", 2 * baseDelay},
				{"charlie", "10.10.10.10", 2 * baseDelay},
			},
		},
		{
			name: "empty IP is not tracked",
			setup: func(lg *LoginGuard) {
				recordNFailedAttempts(lg, freeAttempts+1, "alice", "")
			},
			checks: []check{
				{"alice", "", baseDelay},
				{"bob", "", 0},
			},
		},
		{
			name: "success clears user and keeps IP",
			setup: func(lg *LoginGuard) {
				recordNFailedAttempts(lg, freeAttempts+3, "alice", "1.1.1.1")
				lg.RecordSuccessfulAttempt("alice", "1.1.1.1", false)
				// RecordSuccessfulAttempt undoes one failed attempt for the IP, so let's restore it
				recordNFailedAttempts(lg, 1, "xxx", "1.1.1.1")
			},
			checks: []check{
				{"alice", "1.1.1.1", 4 * baseDelay},
				{"alice", "2.2.2.2", 0},
				{"bob", "1.1.1.1", 4 * baseDelay},
			},
		},
		{
			name: "case sensitivity and unicode characters in usernames",
			setup: func(lg *LoginGuard) {
				recordNFailedAttempts(lg, freeAttempts+1, "alice", "")
				recordNFailedAttempts(lg, freeAttempts+1, "kŕdeľ ďatľov učí koňa žrať kôru", "")
				recordNFailedAttempts(lg, freeAttempts+1, "用户", "")
				recordNFailedAttempts(lg, freeAttempts+1, "👩‍💻", "")
				recordNFailedAttempts(lg, freeAttempts+1, "", "") // empty username
				recordNFailedAttempts(lg, freeAttempts+1, ")]}-_.,!?\"`'\\", "")
			},
			checks: []check{
				{"alice", "1.1.1.1", baseDelay},
				{"Alice", "1.1.1.1", 0},
				{"kŕdeľ ďatľov učí koňa žrať kôru", "1.1.1.1", baseDelay},
				{"krdel datlov uci kona zrat koru", "1.1.1.1", 0},
				{"用户", "1.1.1.1", baseDelay},
				{"用戶", "1.1.1.1", 0},
				{"👩‍💻", "1.1.1.1", baseDelay},
				{"👩", "1.1.1.1", 0},
				{"", "1.1.1.1", baseDelay},
				{")]}-_.,!?\"`'\\", "1.1.1.1", baseDelay},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lg := newTestLoginGuard(log.New())
			tc.setup(lg)
			for _, c := range tc.checks {
				require.Equal(t, c.want, lg.checkWait(c.username, ipToIpKey(c.ip), false), "user %q, IP %q", c.username, c.ip)
			}
		})
	}
}

func TestCheckWaitAndRecordFailedAttempt(t *testing.T) {
	lg := newTestLoginGuard(log.New())

	require.Equal(t, 0, lg.userPasswords["alice"].failures)
	require.Equal(t, 0, lg.userMFAs["alice"].failures)
	require.Equal(t, 0, lg.ips["1.1.1.1"].failures)

	// add password failure
	wait := lg.CheckWaitAndRecordFailedAttempt("alice", "1.1.1.1", false)
	require.Zero(t, wait)
	require.Equal(t, 1, lg.userPasswords["alice"].failures)
	require.Equal(t, 0, lg.userMFAs["alice"].failures)
	require.Equal(t, 1, lg.ips["1.1.1.1"].failures)

	// add MFA failure
	wait = lg.CheckWaitAndRecordFailedAttempt("alice", "1.1.1.1", true)
	require.Zero(t, wait)
	require.Equal(t, 1, lg.userPasswords["alice"].failures)
	require.Equal(t, 1, lg.userMFAs["alice"].failures)
	require.Equal(t, 2, lg.ips["1.1.1.1"].failures)

	recordNFailedAttempts(lg, freeAttempts, "alice", "1.1.1.1")
	require.Equal(t, freeAttempts+1, lg.userPasswords["alice"].failures)
	require.Equal(t, freeAttempts+2, lg.ips["1.1.1.1"].failures)

	// delayed attempt is rejected and not recorded
	wait = lg.CheckWaitAndRecordFailedAttempt("alice", "1.1.1.1", false)
	require.Equal(t, 2*baseDelay, wait)
	require.Equal(t, freeAttempts+1, lg.userPasswords["alice"].failures)
	require.Equal(t, freeAttempts+2, lg.ips["1.1.1.1"].failures)
}

func TestFullMap(t *testing.T) {
	cases := []struct {
		name            string
		fullUsernameMap bool
		fullIPMap       bool
		expired         bool // whether the filled entries are old enough to be removed
		username        string
		ip              string
		warnings        []string // expected parts of the logged warnings, in order
	}{
		{
			name:     "empty maps",
			username: "new user",
			ip:       "9.9.9.9",
		},
		{
			name:            "full username map",
			fullUsernameMap: true,
			username:        "new user",
			ip:              "9.9.9.9",
			warnings:        []string{"too many usernames"},
		},
		{
			name:      "full IP map",
			fullIPMap: true,
			username:  "new user",
			ip:        "9.9.9.9",
			warnings:  []string{"too many IP addresses"},
		},
		{
			name:            "full both maps",
			fullUsernameMap: true,
			fullIPMap:       true,
			username:        "new user",
			ip:              "9.9.9.9",
			warnings:        []string{"too many usernames", "too many IP addresses"},
		},
		{
			name:            "full maps with expired entries",
			fullUsernameMap: true,
			fullIPMap:       true,
			expired:         true,
			username:        "new user",
			ip:              "9.9.9.9",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger, hook := logtest.NewNullLogger()
			lg := newTestLoginGuard(logger)

			// All entries have the same time, except entry 5, which is the oldest one.
			lastFail := lg.now()
			if tc.expired {
				lastFail = lg.now().Add(-resetAfter - time.Second)
			}
			oldestLastFail := lastFail.Add(-time.Second)

			if tc.fullUsernameMap {
				for i := range maxEntries {
					lg.userPasswords["user"+strconv.Itoa(i)] = entry{failures: 1, lastFail: lastFail}
				}
				lg.userPasswords["user5"] = entry{failures: 1, lastFail: oldestLastFail}
			}
			if tc.fullIPMap {
				for i := range maxEntries {
					lg.ips[ipKey("ip"+strconv.Itoa(i))] = entry{failures: 1, lastFail: lastFail}
				}
				lg.ips["ip5"] = entry{failures: 1, lastFail: oldestLastFail}
			}

			newIPKey := ipToIpKey(tc.ip)
			lg.recordFailedAttempt(tc.username, newIPKey, false)

			// the new attempt is always recorded
			require.Contains(t, lg.userPasswords, tc.username)
			require.Contains(t, lg.ips, newIPKey)

			// in all test cases the oldest attempt is not persisted (either it expired or it get evicted due to full map)
			require.NotContains(t, lg.userPasswords, "user5")
			require.NotContains(t, lg.ips, ipKey("ip5"))

			if tc.fullUsernameMap && !tc.expired {
				require.Len(t, lg.userPasswords, maxEntries)
			} else {
				require.Len(t, lg.userPasswords, 1)
			}
			if tc.fullIPMap && !tc.expired {
				require.Len(t, lg.ips, maxEntries)
			} else {
				require.Len(t, lg.ips, 1)
			}

			require.Len(t, hook.Entries, len(tc.warnings))
			for i, w := range tc.warnings {
				require.Equal(t, log.WarnLevel, hook.Entries[i].Level)
				require.Contains(t, hook.Entries[i].Message, w)
			}

			// If map is full, already tracked keys are updated without evicting anything.
			// (And if the map is not full, the following assertions pass anyway.)
			//
			// Eviction causes a warning. We will rely on this to determine whether eviction happened.
			hook.Reset()
			advanceTime(lg, warnInterval+time.Second)

			initialUsernameFailures := lg.userPasswords["user1"].failures
			initialIPFailures := lg.ips["ip1"].failures
			lg.recordFailedAttempt("user1", "ip1", false)
			require.Equal(t, initialUsernameFailures+1, lg.userPasswords["user1"].failures)
			require.Equal(t, initialIPFailures+1, lg.ips["ip1"].failures)
			require.Empty(t, hook.Entries)

			// Test hardening: since we rely on that warning, see whether the warning really works
			lg.recordFailedAttempt("userXXX", "ipXXX", false)
			if (tc.fullUsernameMap || tc.fullIPMap) && !tc.expired {
				require.NotEmpty(t, hook.Entries)
			}
		})
	}
}

func TestFullKnownClientMap(t *testing.T) {
	cases := []struct {
		name  string
		stale bool // whether the filled known clients are older than knownClientTrustPeriod
	}{
		{
			name: "full map evicts a known client",
		},
		{
			name:  "full map with stale known clients",
			stale: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger, hook := logtest.NewNullLogger()
			lg := newTestLoginGuard(logger)

			// The filled known clients are just inside or just outside knownClientTrustPeriod, so that the test
			// fails if staleness is determined by a different period (e.g. resetAfter).
			lastSuccess := lg.now().Add(-knownClientTrustPeriod + time.Second)
			if tc.stale {
				lastSuccess = lg.now().Add(-knownClientTrustPeriod - time.Second)
			}
			for i := range maxEntries {
				lg.knownClientPasswords[knownClient{"user" + strconv.Itoa(i), "1.1.1.1"}] = entry{lastSuccess: lastSuccess}
			}

			successfulLogin(t, lg, "alice", "2.2.2.2", false)

			require.Contains(t, lg.knownClientPasswords, knownClient{"alice", "2.2.2.2"})
			if tc.stale {
				require.Len(t, lg.knownClientPasswords, 1)
				require.Empty(t, hook.Entries)
				return
			}
			require.Len(t, lg.knownClientPasswords, maxEntries)
			require.Len(t, hook.Entries, 1)
			require.Contains(t, hook.LastEntry().Message, "too many clients")
		})
	}
}

func TestFullMapWarningIsRateLimited(t *testing.T) {
	logger, hook := logtest.NewNullLogger()
	lg := newTestLoginGuard(logger)
	for i := range maxEntries {
		lg.userPasswords["user"+strconv.Itoa(i)] = entry{failures: 1, lastFail: lg.now()}
	}

	lg.recordFailedAttempt("new1", "", false)
	require.Len(t, hook.Entries, 1)

	lg.recordFailedAttempt("new2", "", false)
	require.Len(t, hook.Entries, 1, "no new warning within the warning interval")

	advanceTime(lg, warnInterval+time.Second)
	lg.recordFailedAttempt("new3", "", false)
	require.Len(t, hook.Entries, 2, "new warning after the warning interval")
}

func TestSuccessDoesNotCountAsIPFailure(t *testing.T) {
	lg := newTestLoginGuard(log.New())

	// many users behind a single address (e.g. NAT), each logging in repeatedly, with and without MFA
	for i := range (freeAttempts + 2) * 4 {
		username := "user" + strconv.Itoa(i%4)
		successfulLogin(t, lg, username, "1.1.1.1", false)
		successfulLogin(t, lg, username, "1.1.1.1", true)
	}

	require.Zero(t, lg.ips["1.1.1.1"].failures)
	wait := lg.CheckWaitAndRecordFailedAttempt("bob", "1.1.1.1", false)
	require.Zero(t, wait)
}

func TestRevertAttempt(t *testing.T) {
	cases := []struct {
		name  string
		isMFA bool
	}{
		{
			name:  "password",
			isMFA: false,
		},
		{
			name:  "MFA",
			isMFA: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lg := newTestLoginGuard(log.New())

			expectedTrackedMap := lg.userPasswords
			expectedUntrackedMap := lg.userMFAs
			if !tc.isMFA {
				recordNFailedAttempts(lg, freeAttempts, "alice", "1.1.1.1")
			} else {
				expectedTrackedMap = lg.userMFAs
				expectedUntrackedMap = lg.userPasswords

				recordNFailedAttemptsMFA(lg, freeAttempts, "alice", "1.1.1.1")
			}

			// this attempt sets a delay, which must be cleared by the revert
			wait := lg.CheckWaitAndRecordFailedAttempt("alice", "1.1.1.1", tc.isMFA)
			require.Zero(t, wait)
			lg.RevertAttempt("alice", "1.1.1.1", tc.isMFA)

			require.Equal(t, freeAttempts, expectedTrackedMap["alice"].failures)
			require.Equal(t, 0, expectedUntrackedMap["alice"].failures)
			require.Equal(t, freeAttempts, lg.ips["1.1.1.1"].failures)
			wait = lg.CheckWaitAndRecordFailedAttempt("alice", "1.1.1.1", tc.isMFA)
			require.Zero(t, wait)
		})
	}
}

func TestPasswordAndMFAFailuresAreSeparate(t *testing.T) {
	t.Run("password failures do not delay MFA", func(t *testing.T) {
		lg := newTestLoginGuard(log.New())
		failedLoginFromDistinctIPs(lg, freeAttempts+1, "alice", false)
		require.Equal(t, baseDelay, lg.checkWait("alice", "1.1.1.1", false))
		require.Zero(t, lg.checkWait("alice", "1.1.1.1", true))
	})

	t.Run("MFA failures do not delay password", func(t *testing.T) {
		lg := newTestLoginGuard(log.New())
		failedLoginFromDistinctIPs(lg, freeAttempts+1, "alice", true)
		require.Zero(t, lg.checkWait("alice", "1.1.1.1", false))
		require.Equal(t, baseDelay, lg.checkWait("alice", "1.1.1.1", true))
	})

	t.Run("password success clears password failures and does not clear MFA failures", func(t *testing.T) {
		lg := newTestLoginGuard(log.New())
		failedLoginFromDistinctIPs(lg, freeAttempts+1, "alice", false)
		failedLoginFromDistinctIPs(lg, freeAttempts+1, "alice", true)
		advanceTime(lg, baseDelay+time.Second)
		successfulLogin(t, lg, "alice", "1.1.1.1", false)
		require.NotContains(t, lg.userPasswords, "alice")

		failedLoginFromDistinctIPs(lg, 1, "alice", false)
		require.Zero(t, lg.checkWait("alice", "2.2.2.2", false))
		failedLoginFromDistinctIPs(lg, 1, "alice", true)
		require.Equal(t, 2*baseDelay, lg.checkWait("alice", "2.2.2.2", true))
	})

	t.Run("MFA success clears MFA failures and does not clear password failures", func(t *testing.T) {
		lg := newTestLoginGuard(log.New())
		failedLoginFromDistinctIPs(lg, freeAttempts+1, "alice", false)
		failedLoginFromDistinctIPs(lg, freeAttempts+1, "alice", true)
		advanceTime(lg, baseDelay+time.Second)
		successfulLogin(t, lg, "alice", "1.1.1.1", true)
		require.NotContains(t, lg.userMFAs, "alice")

		failedLoginFromDistinctIPs(lg, 1, "alice", true)
		require.Zero(t, lg.checkWait("alice", "2.2.2.2", true))
		failedLoginFromDistinctIPs(lg, 1, "alice", false)
		require.Equal(t, 2*baseDelay, lg.checkWait("alice", "2.2.2.2", false))
	})
}

func TestIPToIPKey(t *testing.T) {
	cases := []struct {
		ip   string
		want ipKey
	}{
		{"1.2.3.4", "1.2.3.4"},
		{"::ffff:1.2.3.4", "1.2.3.4"}, // IPv4-mapped IPv6 address is tracked as the IPv4 address
		{"2001:db8:1:2:aaaa::1", "2001:db8:1:2::/64"},
		{"2001:db8:1:2:ffff::9", "2001:db8:1:2::/64"},
		{"2001:db8:1:3::1", "2001:db8:1:3::/64"},
		{"fe80::1%eth0", "fe80::/64"},
		{"::1", "::/64"},
		{"", ""},
		{"not an IP", "not an IP"},
	}
	for _, c := range cases {
		require.Equal(t, c.want, ipToIpKey(c.ip), "IP %q", c.ip)
	}
}

func TestIPv6PrefixSharesDelay(t *testing.T) {
	lg := newTestLoginGuard(log.New())

	recordNFailedAttempts(lg, freeAttempts+1, "alice", "2001:db8:1:2::1")
	wait := lg.CheckWaitAndRecordFailedAttempt("bob", "2001:db8:1:2:ffff::9", false)
	require.Equal(t, baseDelay, wait, "same /64")
	wait = lg.CheckWaitAndRecordFailedAttempt("bob", "2001:db8:1:3::1", false)
	require.Zero(t, wait, "different /64")

	recordNFailedAttempts(lg, freeAttempts+1, "charlie", "1.2.3.4")
	wait = lg.CheckWaitAndRecordFailedAttempt("dave", "::ffff:1.2.3.4", false)
	require.Equal(t, baseDelay, wait, "IPv4-mapped address")
}

func TestKnownClient(t *testing.T) {
	const home = "1.1.1.1"

	t.Run("attacker cannot lock out a known client", func(t *testing.T) {
		lg := newTestLoginGuard(log.New())
		successfulLogin(t, lg, "alice", home, false)

		failedLoginFromDistinctIPs(lg, freeAttempts+5, "alice", false)
		wait := lg.CheckWaitAndRecordFailedAttempt("alice", "8.8.8.8", false)
		require.Equal(t, baseDelay, wait, "attacker is delayed")
		wait = lg.CheckWaitAndRecordFailedAttempt("alice", home, false)
		require.Zero(t, wait)

		successfulLogin(t, lg, "alice", home, false)
		wait = lg.CheckWaitAndRecordFailedAttempt("alice", "8.8.8.8", false)
		require.Equal(t, baseDelay, wait,
			"success of a known client must not clear the username failures caused by the attacker")
	})

	t.Run("known client is delayed by its own failures", func(t *testing.T) {
		lg := newTestLoginGuard(log.New())
		successfulLogin(t, lg, "alice", home, false)

		recordNFailedAttempts(lg, freeAttempts+1, "alice", home)
		require.Equal(t, freeAttempts+1, lg.knownClientPasswords[knownClient{"alice", home}].failures)
		require.NotContains(t, lg.userPasswords, "alice", "failures of a known client are not counted for the username")
		wait := lg.CheckWaitAndRecordFailedAttempt("alice", home, false)
		require.Equal(t, baseDelay, wait)
	})

	t.Run("known client expires", func(t *testing.T) {
		lg := newTestLoginGuard(log.New())
		successfulLogin(t, lg, "alice", home, false)

		advanceTime(lg, knownClientTrustPeriod+time.Second)
		failedLoginFromDistinctIPs(lg, freeAttempts+1, "alice", false)
		wait := lg.CheckWaitAndRecordFailedAttempt("alice", home, false)
		require.Equal(t, baseDelay, wait)
		require.NotContains(t, lg.knownClientPasswords, knownClient{"alice", home})
	})

	t.Run("known client does not expire within the trust period", func(t *testing.T) {
		lg := newTestLoginGuard(log.New())
		successfulLogin(t, lg, "alice", home, false)

		advanceTime(lg, knownClientTrustPeriod-time.Second)
		failedLoginFromDistinctIPs(lg, freeAttempts+1, "alice", false)
		wait := lg.CheckWaitAndRecordFailedAttempt("alice", home, false)
		require.Zero(t, wait)
	})

	t.Run("known client is per IP", func(t *testing.T) {
		lg := newTestLoginGuard(log.New())
		successfulLogin(t, lg, "alice", home, false)

		failedLoginFromDistinctIPs(lg, freeAttempts+1, "alice", false)
		wait := lg.CheckWaitAndRecordFailedAttempt("alice", "2.2.2.2", false)
		require.Equal(t, baseDelay, wait)
	})

	t.Run("password login does not make the client known for MFA", func(t *testing.T) {
		lg := newTestLoginGuard(log.New())
		successfulLogin(t, lg, "alice", home, false)

		failedLoginFromDistinctIPs(lg, freeAttempts+1, "alice", true)
		wait := lg.CheckWaitAndRecordFailedAttempt("alice", home, true)
		require.Equal(t, baseDelay, wait)
	})

	t.Run("MFA known client is not locked out by MFA failures", func(t *testing.T) {
		lg := newTestLoginGuard(log.New())
		successfulLogin(t, lg, "alice", home, true)

		failedLoginFromDistinctIPs(lg, freeAttempts+1, "alice", true)
		wait := lg.CheckWaitAndRecordFailedAttempt("alice", home, true)
		require.Zero(t, wait)
	})

	t.Run("client with unknown IP is not tracked", func(t *testing.T) {
		lg := newTestLoginGuard(log.New())
		successfulLogin(t, lg, "alice", "", false)
		require.Empty(t, lg.knownClientPasswords)
	})
}
