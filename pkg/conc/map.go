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

package conc

import "sync"

// Map is a very simple concurrent map implementation.
type Map[K comparable, V any] struct {
	inner map[K]V
	mu    sync.RWMutex
}

func NewMap[K comparable, V any]() *Map[K, V] {
	return &Map[K, V]{inner: make(map[K]V), mu: sync.RWMutex{}}
}

func (m *Map[K, V]) Get(key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	val, ok := m.inner[key]
	return val, ok
}

func (m *Map[K, V]) Set(key K, val V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inner[key] = val
}

func (m *Map[K, V]) Del(key K) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.inner, key)
}

func (m *Map[K, V]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.inner)
}

// Clone() returns map that contains a snapshot of the data inside Map.
// Useful when one needs to repeatedly access the state of a Map for
// example during iteration.
func (m *Map[K, V]) Clone() map[K]V {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make(map[K]V)
	for k, v := range m.inner {
		res[k] = v
	}
	return res
}
