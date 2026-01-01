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

package admin

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfigNormalize(t *testing.T) {
	cfg := Config{
		BasePath: "/admin/",
	}

	normalized := cfg.Normalize()
	require.Equal(t, "/admin", normalized.BasePath)
	require.Equal(t, 5*time.Second, normalized.ReadTimeout)
	require.Equal(t, 10*time.Second, normalized.WriteTimeout)
	require.Equal(t, 30*time.Second, normalized.IdleTimeout)
}

func TestServerNoListenAddr(t *testing.T) {
	server := NewServer(Config{}, nil, nil)

	err := server.Start(context.Background())
	require.NoError(t, err)
	require.NoError(t, server.Shutdown(context.Background()))
}
