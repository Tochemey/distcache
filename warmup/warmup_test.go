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

package warmup

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHotKeyTracker(t *testing.T) {
	tracker := NewTracker(2)
	tracker.Record("ks", "a")
	tracker.Record("ks", "a")
	tracker.Record("ks", "b")
	tracker.Record("ks", "c")

	keys := tracker.TopKeys("ks", 2, 1)
	require.Len(t, keys, 2)
	require.Contains(t, keys, "a")
}

func TestTrackerTopKeysUnknownKeyspace(t *testing.T) {
	tracker := NewTracker(2)
	require.Nil(t, tracker.TopKeys("missing", 1, 1))
}

func TestWarmupConfigNormalize(t *testing.T) {
	normalized := Config{}.Normalize()
	require.Equal(t, 100, normalized.MaxHotKeys)
	require.Equal(t, uint64(1), normalized.MinHits)
	require.Equal(t, 4, normalized.Concurrency)
	require.Equal(t, 2*time.Second, normalized.Timeout)
	require.True(t, normalized.WarmOnJoin)
	require.True(t, normalized.WarmOnLeave)

	normalized = Config{WarmOnJoin: true}.Normalize()
	require.True(t, normalized.WarmOnJoin)
	require.False(t, normalized.WarmOnLeave)
}

func TestHotKeySetOrderingAndEviction(t *testing.T) {
	// The set may hold up to 2*maxKeys entries between eviction passes; the
	// pass triggers once that bound is exceeded and keeps the top maxKeys.
	set := newHotKeySet(2)
	set.record("a")
	set.record("a")
	set.record("a")
	set.record("b")
	set.record("b")
	set.record("c")
	set.record("d")
	require.Len(t, set.counts, 4)

	// fifth distinct key exceeds 2*maxKeys and triggers the eviction pass
	set.record("e")
	require.Len(t, set.counts, 2)
	require.Equal(t, []string{"a", "b"}, set.topKeys(3, 1))
	// survivors have had their counts halved so stale keys age out
	require.Equal(t, uint64(1), set.counts["a"])
	require.Equal(t, uint64(1), set.counts["b"])

	orderSet := newHotKeySet(3)
	orderSet.record("b")
	orderSet.record("b")
	orderSet.record("a")
	orderSet.record("a")
	ordered := orderSet.topKeys(2, 2)
	require.Equal(t, []string{"a", "b"}, ordered)

	require.Empty(t, orderSet.topKeys(2, 3))
}

func TestHotKeySetEmptyKeyEvictable(t *testing.T) {
	set := newHotKeySet(1)
	set.record("")
	set.record("hot")
	set.record("hot")
	set.record("other")
	require.Len(t, set.counts, 1)
	_, found := set.counts[""]
	require.False(t, found)
	require.Equal(t, []string{"hot"}, set.topKeys(1, 1))
}

func TestHotKeySetDecay(t *testing.T) {
	// A key that was hot early must eventually be displaced by keys that are
	// hot now, even though its historical count was much larger.
	set := newHotKeySet(2)
	for range 64 {
		set.record("old")
	}
	for i := range 1000 {
		set.record(fmt.Sprintf("new-%d", i%3))
	}
	keys := set.topKeys(2, 1)
	require.Len(t, keys, 2)
	require.NotContains(t, keys, "old")
}

func TestTrackerConcurrentRecord(t *testing.T) {
	const (
		goroutines = 16
		perRoutine = 500
	)
	tracker := NewTracker(10)

	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perRoutine {
				tracker.Record("ks", "hot")
				tracker.Record(fmt.Sprintf("ks-%d", g%3), fmt.Sprintf("k%d", i%5))
			}
		}()
	}
	wg.Wait()

	set, ok := tracker.keyspaces.Get("ks")
	require.True(t, ok)
	// "hot" is the only key in "ks" so it is never evicted and no count is lost
	require.Equal(t, uint64(goroutines*perRoutine), set.counts["hot"])
	require.Equal(t, []string{"hot"}, tracker.TopKeys("ks", 1, 1))

	for i := range 3 {
		keys := tracker.TopKeys(fmt.Sprintf("ks-%d", i), 10, 1)
		require.Len(t, keys, 5)
	}
}

func BenchmarkHotKeySetRecordHighCardinality(b *testing.B) {
	keys := make([]string, 1<<16)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%d", i)
	}
	set := newHotKeySet(100)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		set.record(keys[i&(len(keys)-1)])
	}
}

func BenchmarkTrackerRecordParallel(b *testing.B) {
	keys := make([]string, 1<<12)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%d", i)
	}
	tracker := NewTracker(100)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			tracker.Record("ks", keys[i&(len(keys)-1)])
			i++
		}
	})
}

func TestHotKeySetTopKeysLimit(t *testing.T) {
	set := newHotKeySet(10)
	set.record("a")
	set.record("b")
	set.record("c")

	require.Nil(t, set.topKeys(0, 1))

	keys := set.topKeys(2, 1)
	require.Equal(t, []string{"a", "b"}, keys)
}
