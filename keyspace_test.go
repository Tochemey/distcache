/*
 * MIT License
 *
 * Copyright (c) 2025 Arsene Tochemey Gandote
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package distcache

import (
	"context"
	"sync"
	"time"
)

type MockKeySpace struct {
	*sync.RWMutex
	name       string
	maxBytes   int64
	dataSource DataSource
}

var _ KeySpace = (*MockKeySpace)(nil)

func NewMockKeySpace(name string, maxBytes int64, source DataSource) *MockKeySpace {
	return &MockKeySpace{
		RWMutex:    &sync.RWMutex{},
		name:       name,
		maxBytes:   maxBytes,
		dataSource: source,
	}
}

func (x *MockKeySpace) Name() string {
	x.RLock()
	defer x.RUnlock()
	return x.name
}

func (x *MockKeySpace) MaxBytes() int64 {
	x.RLock()
	defer x.RUnlock()
	return x.maxBytes
}

func (x *MockKeySpace) DataSource() DataSource {
	x.RLock()
	defer x.RUnlock()
	return x.dataSource
}

func (x *MockKeySpace) ExpiresAt(ctx context.Context, key string) time.Time {
	x.Lock()
	defer x.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if _, err := x.dataSource.Fetch(ctx, key); err != nil {
		return time.Now().Add(time.Second)
	}

	return time.Time{}
}
