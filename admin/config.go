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
	"crypto/tls"
	"strings"
	"time"
)

// Config configures the diagnostic HTTP server.
type Config struct {
	// ListenAddr specifies the address to bind for diagnostics.
	ListenAddr string
	// BasePath is the URL prefix for diagnostic endpoints.
	BasePath string
	// ReadTimeout bounds the maximum time to read request data.
	ReadTimeout time.Duration
	// WriteTimeout bounds the maximum time to write responses.
	WriteTimeout time.Duration
	// IdleTimeout is the maximum amount of time to wait for the next request.
	IdleTimeout time.Duration
	// TLSConfig enables HTTPS when non-nil.
	TLSConfig *tls.Config
}

// Normalize returns a configuration with defaults applied.
func (c Config) Normalize() Config {
	config := c
	if config.BasePath == "" {
		config.BasePath = defaultAdminBasePath
	}

	config.BasePath = "/" + strings.Trim(config.BasePath, "/")
	if config.ReadTimeout <= 0 {
		config.ReadTimeout = 5 * time.Second
	}

	if config.WriteTimeout <= 0 {
		config.WriteTimeout = 10 * time.Second
	}

	if config.IdleTimeout <= 0 {
		config.IdleTimeout = 30 * time.Second
	}

	return config
}
