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

import "context"

// DataSource defines the interface used by `distcache` to retrieve data
// when a requested entry is not found in the cache. Implementations of this
// interface provide a mechanism to fetch data from an external source, such
// as a database, API, or file system.
type DataSource interface {
	// Fetch retrieves the value associated with the given key from the data source.
	// It is called when a cache miss occurs.
	//
	// Parameters:
	//   - ctx: A context for managing timeouts, cancellations, or deadlines.
	//   - key: The cache key whose value needs to be fetched.
	//
	// Returns:
	//   - A byte slice containing the fetched data.
	//   - An error if the data retrieval fails.
	Fetch(ctx context.Context, key string) ([]byte, error)
}
