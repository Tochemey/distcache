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

package tcp

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetHostPort(t *testing.T) {
	t.Run("Happy Path", func(t *testing.T) {
		address := "localhost:8080"
		host, port, err := GetHostPort(address)
		require.NoError(t, err)
		assert.Equal(t, "127.0.0.1", host)
		assert.Exactly(t, 8080, port)
	})
	t.Run("Invalid address", func(t *testing.T) {
		_, _, err := GetHostPort("127.0.0.1.1.1.1.1.2:8080")
		require.Error(t, err)
	})
}

func TestGetBindIP(t *testing.T) {
	t.Run("Happy Path", func(t *testing.T) {
		address := "localhost:8080"
		ifaces, err := net.Interfaces()
		require.NoError(t, err)
		ifname := ifaces[0].Name
		bindIP, err := GetBindIP(ifname, address)
		require.NoError(t, err)
		assert.Equal(t, "127.0.0.1", bindIP)
	})
}

func TestNewKeepAliveListener(t *testing.T) {
	t.Run("Happy Path", func(t *testing.T) {
		address := "localhost:8080"
		lnr, err := NewKeepAliveListener(address)
		require.NoError(t, err)
		require.NotNil(t, lnr)
	})
}

func TestGetHostPortWildcard(t *testing.T) {
	testCases := []struct {
		name     string
		address  string
		expected string
	}{
		{name: "empty host", address: ":8080", expected: "0.0.0.0"},
		{name: "ipv4 wildcard", address: "0.0.0.0:8080", expected: "0.0.0.0"},
		{name: "ipv6 wildcard", address: "[::]:8080", expected: "::"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			host, port, err := GetHostPort(tc.address)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, host)
			assert.Exactly(t, 8080, port)
			assert.True(t, isWildcard(host))
		})
	}

	assert.False(t, isWildcard("127.0.0.1"))
	assert.False(t, isWildcard("::1"))
}

func TestGetBindIPWildcard(t *testing.T) {
	// every wildcard form must resolve to a concrete, non-wildcard IP
	for _, address := range []string{":8080", "0.0.0.0:8080", "[::]:8080"} {
		bindIP, err := GetBindIP("", address)
		require.NoError(t, err, address)
		require.NotEmpty(t, bindIP, address)
		assert.False(t, isWildcard(bindIP), address)
		assert.NotNil(t, net.ParseIP(bindIP), address)
	}
}
