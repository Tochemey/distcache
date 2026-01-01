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
	"encoding/json"
	"errors"
	"time"

	"github.com/tochemey/distcache/internal/syncmap"
)

type User struct {
	ID   string
	Name string
	Age  int
}

type MockDataSource struct {
	store *syncmap.Map[string, User]
}

var _ DataSource = (*MockDataSource)(nil)

func NewMockDataSource() *MockDataSource {
	return &MockDataSource{
		store: syncmap.New[string, User](),
	}
}

func (x *MockDataSource) Insert(ctx context.Context, users []*User) error {
	_, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	for _, user := range users {
		x.store.Set(user.ID, *user)
	}
	return nil
}

func (x *MockDataSource) Fetch(ctx context.Context, key string) ([]byte, error) {
	_, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	user, ok := x.store.Get(key)
	if !ok {
		return nil, errors.New("not found")
	}

	bytea, err := json.Marshal(user)
	if err != nil {
		return nil, err
	}
	return bytea, nil
}
