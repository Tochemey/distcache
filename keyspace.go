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

package distcache

import (
	"context"
	"time"
)

// KeySpace defines a logical namespace for storing key-value pairs in a distributed cache.
// It provides metadata about the namespace, including its name, storage limits, and data source.
// Additionally, it allows checking when a specific key is set to expire.
type KeySpace interface {
	// Name returns the name of the namespace.
	// The namespace is used to logically group key-value pairs.
	Name() string

	// MaxBytes returns the maximum number of bytes allocated for this namespace.
	// Once the limit is reached, the cache may evict entries based on its eviction policy.
	MaxBytes() int64

	// DataSource returns the underlying data source for this namespace.
	// This source is used to fetch data in case of a cache miss.
	DataSource() DataSource

	// ExpiresAt returns the expiration time for a given key within the namespace.
	// If the key does not have a predefined expiration time, it may return a zero time.
	//
	// ctx: Context for managing timeouts or cancellations.
	// key: The cache key whose expiration time is being queried.
	ExpiresAt(ctx context.Context, key string) time.Time
}
