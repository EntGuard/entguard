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

package addresspools

import (
	"fmt"

	sqlc "github.com/entguard/entguard/db"
)

// VerifyPoolsNotOverlap checks if any two address pools in the list overlap.
func VerifyPoolsNotOverlap(pools []sqlc.AddressPool) error {
	for i := 0; i < len(pools); i++ {
		for j := i + 1; j < len(pools); j++ {
			if addressPoolsOverlap(pools[i], pools[j]) {
				return fmt.Errorf("address pools '%s' and '%s' overlap", pools[i].Name, pools[j].Name)
			}
		}
	}
	return nil
}

// addressPoolsOverlap checks if two address pools overlap.
// Pools overlap if their IP ranges intersect.
func addressPoolsOverlap(pool1, pool2 sqlc.AddressPool) bool {
	return pool1.StartAddr.Compare(pool2.EndAddr) <= 0 && pool2.StartAddr.Compare(pool1.EndAddr) <= 0
}
