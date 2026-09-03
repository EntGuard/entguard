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

package limiter

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	const (
		bs  = 500
		tps = 10
	)

	l := NewLimiter(tps, bs)

	success := func() {
		t.Helper()

		for i := 0; i < bs; i++ {
			ok := l.Limit()
			if ok {
				t.Fatalf("limiter returned true in the first loop, false expected, iteration (%v)", i)
			}
		}

		time.Sleep(time.Second)

		for i := 0; i < tps; i++ {
			ok := l.Limit()
			if ok {
				t.Fatalf("limiter returned true in the second loop, false expected, iteration (%v)", i)
			}
		}
	}

	failure := func() {
		t.Helper()

		for i := 0; i < bs; i++ {
			l.Limit()
		}

		if ok := l.Limit(); !ok {
			t.Fatal("limiter returned false in after the loop, true expected")
		}
	}

	success()
	failure()
}
