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

package standalone

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tochemey/distcache/discovery"
)

func TestDiscovery(t *testing.T) {
	t.Run("implements discovery.Provider", func(t *testing.T) {
		provider := NewDiscovery()
		require.NotNil(t, provider)
		assert.IsType(t, &Discovery{}, provider)
		var p interface{} = provider
		_, ok := p.(discovery.Provider)
		assert.True(t, ok)
	})

	t.Run("ID returns standalone", func(t *testing.T) {
		provider := NewDiscovery()
		assert.Equal(t, "standalone", provider.ID())
	})

	t.Run("Initialize returns nil", func(t *testing.T) {
		provider := NewDiscovery()
		assert.NoError(t, provider.Initialize())
	})

	t.Run("Register returns nil", func(t *testing.T) {
		provider := NewDiscovery()
		assert.NoError(t, provider.Register())
	})

	t.Run("Deregister returns nil", func(t *testing.T) {
		provider := NewDiscovery()
		assert.NoError(t, provider.Deregister())
	})

	t.Run("DiscoverPeers returns empty list", func(t *testing.T) {
		provider := NewDiscovery()
		require.NoError(t, provider.Initialize())
		require.NoError(t, provider.Register())

		peers, err := provider.DiscoverPeers()
		require.NoError(t, err)
		assert.Empty(t, peers)

		assert.NoError(t, provider.Deregister())
		assert.NoError(t, provider.Close())
	})

	t.Run("Close returns nil", func(t *testing.T) {
		provider := NewDiscovery()
		assert.NoError(t, provider.Close())
	})

	t.Run("full lifecycle succeeds", func(t *testing.T) {
		provider := NewDiscovery()

		require.NoError(t, provider.Initialize())
		require.NoError(t, provider.Register())

		peers, err := provider.DiscoverPeers()
		require.NoError(t, err)
		assert.Empty(t, peers)

		require.NoError(t, provider.Deregister())
		require.NoError(t, provider.Close())
	})
}
