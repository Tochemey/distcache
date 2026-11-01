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

package syncmap

import (
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAndSet(t *testing.T) {
	sm := New[int, string]()
	sm.Set(1, "one")
	sm.Set(2, "two")
	assert.Exactly(t, 2, sm.Len())
}

func TestGet(t *testing.T) {
	sm := New[int, string]()
	sm.Set(1, "one")

	val, ok := sm.Get(1)
	require.True(t, ok)
	require.Equal(t, "one", val)

	_, ok = sm.Get(2)
	require.False(t, ok)
}

func TestDelete(t *testing.T) {
	sm := New[int, string]()
	sm.Set(1, "one")
	sm.Delete(1)
	_, ok := sm.Get(1)
	require.False(t, ok)
	sm.Delete(2) // just make sure this doesn't panic
}

func TestLen(t *testing.T) {
	sm := New[int, string]()
	sm.Set(1, "one")
	sm.Set(2, "two")
	sm.Set(3, "three")
	sm.Delete(2)
	assert.Exactly(t, 2, sm.Len())
}

func TestForEach(t *testing.T) {
	sm := New[int, string]()
	sm.Set(1, "one")
	sm.Set(2, "two")

	keys := make([]int, 0)
	sm.Range(func(k int, v string) { // nolint
		keys = append(keys, k)
	})

	assert.Exactly(t, 2, len(keys))
	// Check if keys 1 and 2 are present
	if !slices.Contains(keys, 1) || !slices.Contains(keys, 2) {
		t.Errorf("Expected keys 1 and 2, got %v", keys)
	}
}

func TestKeys(t *testing.T) {
	sm := New[int, string]()
	sm.Set(1, "one")
	sm.Set(2, "two")
	keys := sm.Keys()
	assert.Exactly(t, 2, len(keys))
	slices.Sort(keys)
	assert.ElementsMatch(t, []int{1, 2}, keys)
}

func TestGetOrSet(t *testing.T) {
	sm := New[string, int]()

	// first call stores the value
	actual, loaded := sm.GetOrSet("a", 1)
	require.False(t, loaded)
	require.Equal(t, 1, actual)

	// second call returns the existing value and ignores the new one
	actual, loaded = sm.GetOrSet("a", 2)
	require.True(t, loaded)
	require.Equal(t, 1, actual)

	val, ok := sm.Get("a")
	require.True(t, ok)
	require.Equal(t, 1, val)
	assert.Exactly(t, 1, sm.Len())
}

func TestGetOrSetConcurrent(t *testing.T) {
	sm := New[string, int]()
	const workers = 50

	var wg sync.WaitGroup
	var stored atomic.Int32
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, loaded := sm.GetOrSet("key", i); !loaded {
				stored.Add(1)
			}
		}(i)
	}
	wg.Wait()

	// exactly one goroutine must have won the store
	assert.Equal(t, int32(1), stored.Load())
	assert.Exactly(t, 1, sm.Len())
}
