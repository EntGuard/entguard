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

package conc_test

import (
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/entguard/entguard/pkg/conc"

	"github.com/stretchr/testify/require"
)

func TestNewMap(t *testing.T) {
	m := conc.NewMap[int, string]()
	require.NotNil(t, m)
	require.Equal(t, 0, m.Len())
}

func TestSet(t *testing.T) {
	m := conc.NewMap[int, string]()
	m.Set(1, "red")
	m.Set(2, "green")
	m.Set(3, "blue")
	require.Equal(t, 3, m.Len())
}

func TestGet(t *testing.T) {
	m := conc.NewMap[int, string]()
	val, ok := m.Get(1)
	require.False(t, ok)
	require.Empty(t, val)

	m.Set(1, "red")
	val, ok = m.Get(1)
	require.True(t, ok)
	require.Equal(t, "red", val)
}

func TestDel(t *testing.T) {
	m := conc.NewMap[int, string]()
	m.Set(1, "red")
	m.Del(2)
	require.Equal(t, 1, m.Len())
	m.Del(1)
	require.Equal(t, 0, m.Len())
}

func TestClone(t *testing.T) {
	m := conc.NewMap[int, string]()
	m.Set(1, "red")
	m.Set(2, "green")
	m.Set(3, "blue")
	wantLen := m.Len()

	clone := m.Clone()
	require.Equal(t, wantLen, len(clone))

	m.Set(4, "magenta")
	_, ok := clone[4]
	require.False(t, ok)
	require.Equal(t, wantLen, len(clone))

	m.Del(3)
	val, ok := clone[3]
	require.True(t, ok)
	require.Equal(t, "blue", val)
	require.Equal(t, wantLen, len(clone))

	m.Set(2, "orange")
	val, ok = clone[2]
	require.True(t, ok)
	require.Equal(t, "green", val)
	require.Equal(t, wantLen, len(clone))

	clone[5] = "black"
	_, ok = m.Get(5)
	require.False(t, ok)
	require.Equal(t, 3, m.Len())
}

func TestConcurrentOperations(t *testing.T) {
	concurrencyCount := 5
	iterations := concurrencyCount * 100

	var wg sync.WaitGroup
	m := conc.NewMap[int, string]()

	for c := range concurrencyCount {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			for i := 0; i < iterations; i = i + 5 {
				m.Set(i+offset, strconv.Itoa(i+offset))
				val, ok := m.Get(i + offset)
				require.True(t, ok)
				require.Equal(t, strconv.Itoa(i+offset), val)
			}
		}(c)
	}
	wg.Wait()

	require.Equal(t, iterations, m.Len())
	clone := m.Clone()

	keys := make([]int, 0, len(clone))
	for k := range clone {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for i := 0; i < iterations; i++ {
		require.Equal(t, i, keys[i])
		require.Equal(t, strconv.Itoa(i), clone[i])
	}

	for c := range concurrencyCount {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			for i := 0; i < iterations; i = i + 5 {
				m.Del(i + offset)
				val, ok := m.Get(i + offset)
				require.False(t, ok)
				require.Empty(t, val)
			}
		}(c)
	}
	wg.Wait()

	require.Equal(t, 0, m.Len())
}
