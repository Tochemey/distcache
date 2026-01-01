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
	set := newHotKeySet(2)
	set.record("a")
	set.record("a")
	set.record("b")
	set.record("c")

	keys := set.topKeys(3, 1)
	require.Contains(t, keys, "a")
	require.Len(t, keys, 2)

	orderSet := newHotKeySet(3)
	orderSet.record("b")
	orderSet.record("b")
	orderSet.record("a")
	orderSet.record("a")
	ordered := orderSet.topKeys(2, 2)
	require.Equal(t, []string{"a", "b"}, ordered)

	require.Empty(t, orderSet.topKeys(2, 3))
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
