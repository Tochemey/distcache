// MIT License
//
// Copyright (c) 2025-2026 Arsene Tochemey Gandote
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

// Package warmup provides hot key tracking and configuration for cache warmups.
package warmup

import (
	"sort"
	"sync"
	"time"

	"github.com/tochemey/distcache/internal/syncmap"
)

// Config controls key prefetch behavior on cluster topology changes.
//
// Hot keys are tracked using a bounded frequency map. When a join/leave event
// occurs, distcache prefetches:
//   - Explicit WarmKeys configured on the keyspace, plus
//   - The top N hot keys (MaxHotKeys) whose hit counts are at least MinHits.
//
// The algorithm is O(n) with respect to tracked keys per keyspace and is
// invoked only on topology changes.
type Config struct {
	// MaxHotKeys bounds the number of hot keys considered per keyspace.
	MaxHotKeys int
	// MinHits is the minimum access count for a key to be considered hot.
	MinHits uint64
	// Concurrency controls the number of concurrent prefetch requests.
	Concurrency int
	// Timeout bounds the per-key prefetch duration.
	Timeout time.Duration
	// WarmOnJoin enables warm-up on cluster join events.
	WarmOnJoin bool
	// WarmOnLeave enables warm-up on cluster leave events.
	WarmOnLeave bool
}

// Normalize returns a configuration with defaults applied.
func (c Config) Normalize() Config {
	config := c
	if config.MaxHotKeys <= 0 {
		config.MaxHotKeys = 100
	}

	if config.MinHits == 0 {
		config.MinHits = 1
	}

	if config.Concurrency <= 0 {
		config.Concurrency = 4
	}

	if config.Timeout <= 0 {
		config.Timeout = 2 * time.Second
	}

	if !config.WarmOnJoin && !config.WarmOnLeave {
		config.WarmOnJoin = true
		config.WarmOnLeave = true
	}
	return config
}

// Tracker records key access frequency for warm-up decisions.
type Tracker struct {
	maxKeys   int
	keyspaces *syncmap.Map[string, *hotKeySet]
}

// NewTracker constructs a Tracker with the provided key cap.
func NewTracker(maxKeys int) *Tracker {
	return &Tracker{
		maxKeys:   maxKeys,
		keyspaces: syncmap.New[string, *hotKeySet](),
	}
}

// Record increments the hit count for a key within the given keyspace.
func (t *Tracker) Record(keyspace string, key string) {
	set, ok := t.keyspaces.Get(keyspace)
	if !ok {
		set = newHotKeySet(t.maxKeys)
		t.keyspaces.Set(keyspace, set)
	}
	set.record(key)
}

// TopKeys returns the most frequently accessed keys for a keyspace.
func (t *Tracker) TopKeys(keyspace string, limit int, minHits uint64) []string {
	set, ok := t.keyspaces.Get(keyspace)
	if !ok {
		return nil
	}
	return set.topKeys(limit, minHits)
}

type hotKeySet struct {
	mu      sync.Mutex
	maxKeys int
	counts  map[string]uint64
}

func newHotKeySet(maxKeys int) *hotKeySet {
	return &hotKeySet{
		maxKeys: maxKeys,
		counts:  make(map[string]uint64),
	}
}

func (s *hotKeySet) record(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.counts[key]++
	if len(s.counts) <= s.maxKeys {
		return
	}

	var minKey string
	var minCount uint64
	first := true
	for k, count := range s.counts {
		if first || count < minCount {
			minKey = k
			minCount = count
			first = false
		}
	}
	if minKey != "" {
		delete(s.counts, minKey)
	}
}

func (s *hotKeySet) topKeys(limit int, minHits uint64) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		return nil
	}

	type keyCount struct {
		key   string
		count uint64
	}

	entries := make([]keyCount, 0, len(s.counts))
	for k, count := range s.counts {
		if count < minHits {
			continue
		}
		entries = append(entries, keyCount{key: k, count: count})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count == entries[j].count {
			return entries[i].key < entries[j].key
		}
		return entries[i].count > entries[j].count
	})

	if len(entries) > limit {
		entries = entries[:limit]
	}

	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		keys = append(keys, entry.key)
	}
	return keys
}
