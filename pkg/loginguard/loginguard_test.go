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
		lg.RecordFailedAttempt(username, ip)
	}
}

func TestCheck(t *testing.T) {
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
				recordNFailedAttempts(lg, freeAttempts+1, "alice", "1.1.1.1")
				lg.RecordSuccessfulAttempt("alice")
			},
			checks: []check{
				{"alice", "1.1.1.1", baseDelay},
				{"alice", "2.2.2.2", 0},
				{"bob", "1.1.1.1", baseDelay},
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
				require.Equal(t, c.want, lg.CheckWait(c.username, c.ip), "user %q, IP %q", c.username, c.ip)
			}
		})
	}
}

func TestFullMap(t *testing.T) {
	cases := []struct {
		name            string
		fullUsernameMap bool
		fullIPMap       bool
		expired         bool // whether the filled entries are old enough to be removed
		username        string
		ip              string
		warning         string // expected part of the warning, empty means no warning
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
			warning:         "too many usernames",
		},
		{
			name:      "full IP map",
			fullIPMap: true,
			username:  "new user",
			ip:        "9.9.9.9",
			warning:   "too many IP addresses",
		},
		{
			name:            "full both maps",
			fullUsernameMap: true,
			fullIPMap:       true,
			username:        "new user",
			ip:              "9.9.9.9",
			warning:         "brute force", // common part of both messages (which message gets printed first is irrelevant for this test)
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

			lastFail := lg.now()
			if tc.expired {
				lastFail = lg.now().Add(-resetAfter - time.Second)
			}
			if tc.fullUsernameMap {
				for i := range maxEntries {
					lg.users["user"+strconv.Itoa(i)] = entry{failures: 1, lastFail: lastFail}
				}
			}
			if tc.fullIPMap {
				for i := range maxEntries {
					lg.ips["ip"+strconv.Itoa(i)] = entry{failures: 1, lastFail: lastFail}
				}
			}

			lg.RecordFailedAttempt(tc.username, tc.ip)

			if tc.expired {
				require.Len(t, lg.users, 1)
				require.Len(t, lg.ips, 1)
				require.Contains(t, lg.users, tc.username)
				require.Contains(t, lg.ips, tc.ip)
				require.Empty(t, hook.Entries)
				return
			}

			if tc.fullUsernameMap {
				require.Len(t, lg.users, maxEntries)
				require.NotContains(t, lg.users, tc.username)
			} else {
				require.Len(t, lg.users, 1)
			}
			if tc.fullIPMap {
				require.Len(t, lg.ips, maxEntries)
				require.NotContains(t, lg.ips, tc.ip)
			} else {
				require.Len(t, lg.ips, 1)
			}

			if tc.fullUsernameMap || tc.fullIPMap {
				require.GreaterOrEqual(t, len(hook.Entries), 1)
				require.Equal(t, log.WarnLevel, hook.LastEntry().Level)
				require.Contains(t, hook.LastEntry().Message, tc.warning)
			} else {
				require.Empty(t, hook.Entries)
			}

			// If map is full, already tracked keys are still updated and do not cause a warning.
			// (And if the map is not full, the following assertions pass anyway.)
			hook.Reset()
			initialUsernameFailures := lg.users["user0"].failures
			initialIPFailures := lg.ips["ip0"].failures
			lg.RecordFailedAttempt("user0", "ip0")
			require.Equal(t, initialUsernameFailures+1, lg.users["user0"].failures)
			require.Equal(t, initialIPFailures+1, lg.ips["ip0"].failures)
			require.Empty(t, hook.Entries)
		})
	}
}
