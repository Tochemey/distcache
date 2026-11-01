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

package dnssd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConfig(t *testing.T) {
	t.Run("With valid configuration", func(t *testing.T) {
		config := &Config{
			DomainName: "google.com",
		}
		assert.NoError(t, config.Validate())
	})
	t.Run("With invalid configuration", func(t *testing.T) {
		config := &Config{
			DomainName: "",
		}
		err := config.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "DomainName")
	})
	t.Run("With invalid port", func(t *testing.T) {
		assert.Error(t, Config{DomainName: "google.com", Port: -1}.Validate())
		assert.Error(t, Config{DomainName: "google.com", Port: 70000}.Validate())
		assert.NoError(t, Config{DomainName: "google.com", Port: 8080}.Validate())
	})
	t.Run("With invalid timeout", func(t *testing.T) {
		assert.Error(t, Config{DomainName: "google.com", Timeout: -time.Second}.Validate())
	})
	t.Run("With timeout default", func(t *testing.T) {
		assert.Equal(t, DefaultTimeout, Config{}.timeout())
		assert.Equal(t, time.Second, Config{Timeout: time.Second}.timeout())
	})
}
