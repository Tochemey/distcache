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
	"errors"
	"fmt"
	"time"

	"github.com/groupcache/groupcache-go/v3"
	"github.com/groupcache/groupcache-go/v3/transport"

	"github.com/tochemey/distcache/internal/syncmap"
)

const (
	tagValue     byte = 0x00
	tagTombstone byte = 0x01
)

type keySpaceWrapper struct {
	keyspace   KeySpace
	config     KeySpaceConfig
	dataSource DataSource
}

func (s *keySpaceWrapper) negativeCachingEnabled() bool {
	return s.config.NegativeTTL > 0
}

func (s *keySpaceWrapper) wrap(value []byte) []byte {
	if !s.negativeCachingEnabled() {
		return value
	}
	out := make([]byte, len(value)+1)
	out[0] = tagValue
	copy(out[1:], value)
	return out
}

func (s *keySpaceWrapper) unwrap(value []byte) ([]byte, error) {
	if !s.negativeCachingEnabled() {
		return value, nil
	}
	if len(value) == 0 {
		return nil, fmt.Errorf("invalid cache entry: empty payload for keyspace %q", s.keyspace.Name())
	}
	switch value[0] {
	case tagValue:
		return value[1:], nil
	case tagTombstone:
		return nil, ErrNotFound
	default:
		return nil, fmt.Errorf("invalid cache entry tag %#x for keyspace %q", value[0], s.keyspace.Name())
	}
}

func newKeySpaceWrapper(config *Config, keyspace KeySpace) (*keySpaceWrapper, error) {
	if keyspace == nil {
		return nil, fmt.Errorf("keyspace is nil")
	}

	if keyspace.Name() == "" {
		return nil, fmt.Errorf("keyspace name is required")
	}

	if keyspace.DataSource() == nil {
		return nil, fmt.Errorf("keyspace data source is required")
	}

	if keyspace.MaxBytes() <= 0 {
		return nil, fmt.Errorf("keyspace max bytes is required")
	}

	var ksConfig KeySpaceConfig
	if config.keySpaceConfigs != nil {
		if override, ok := config.keySpaceConfigs[keyspace.Name()]; ok {
			ksConfig = override
		}
	}

	if err := newKeySpaceConfigValidator(keyspace.Name(), ksConfig).Validate(); err != nil {
		return nil, err
	}

	effective := ksConfig
	if effective.MaxBytes <= 0 {
		effective.MaxBytes = keyspace.MaxBytes()
	}

	if len(effective.WarmKeys) > 0 {
		keys := make([]string, len(effective.WarmKeys))
		copy(keys, effective.WarmKeys)
		effective.WarmKeys = keys
	}

	dataSourceConfig := mergeDataSourceConfig(config.dataSourcePolicy, effective.RateLimit, effective.CircuitBreaker)
	if dataSourceConfig != nil {
		if err := dataSourceConfig.Validate(); err != nil {
			return nil, err
		}
	}

	source := keyspace.DataSource()
	if dataSourceConfig != nil {
		protected := dataSourceConfig.Apply(source)
		if protected == nil {
			return nil, fmt.Errorf("keyspace data source policy returned nil")
		}
		source = protected
	}

	return &keySpaceWrapper{
		keyspace:   keyspace,
		config:     effective,
		dataSource: source,
	}, nil
}

func (x *engine) readTimeout(spec *keySpaceWrapper) time.Duration {
	if spec != nil && spec.config.ReadTimeout > 0 {
		return spec.config.ReadTimeout
	}
	return x.config.ReadTimeout()
}

func (x *engine) writeTimeout(spec *keySpaceWrapper) time.Duration {
	if spec != nil && spec.config.WriteTimeout > 0 {
		return spec.config.WriteTimeout
	}
	return x.config.WriteTimeout()
}

func (x *engine) createGroup(spec *keySpaceWrapper) (transport.Group, error) {
	group, err := x.daemon.NewGroup(spec.keyspace.Name(), spec.config.MaxBytes, groupcache.GetterFunc(
		func(ctx context.Context, id string, dest transport.Sink) error {
			start := time.Now()
			bytea, err := spec.dataSource.Fetch(ctx, id)
			x.instrumentation.recordFetch(ctx, spec.keyspace.Name(), start)
			if err != nil {
				if errors.Is(err, ErrNotFound) && spec.negativeCachingEnabled() {
					return dest.SetBytes([]byte{tagTombstone}, time.Now().Add(spec.config.NegativeTTL))
				}
				return err
			}
			expiredAt := spec.keyspace.ExpiresAt(ctx, id)
			if expiredAt.IsZero() && spec.config.DefaultTTL > 0 {
				expiredAt = time.Now().Add(spec.config.DefaultTTL)
			}
			return dest.SetBytes(spec.wrap(bytea), expiredAt)
		}))

	if err != nil {
		return nil, fmt.Errorf("failed to create group: %w", err)
	}

	return group, nil
}

func (x *engine) ensureKeySpaces() error {
	if x.keySpaces != nil && x.keySpaces.Len() > 0 {
		return nil
	}

	if x.keySpaces == nil {
		x.keySpaces = syncmap.New[string, *keySpaceWrapper]()
	}

	for _, keyspace := range x.config.KeySpaces() {
		spec, err := newKeySpaceWrapper(x.config, keyspace)
		if err != nil {
			return err
		}
		if _, exists := x.keySpaces.Get(spec.keyspace.Name()); exists {
			return fmt.Errorf("duplicate keyspace: %s", spec.keyspace.Name())
		}
		x.keySpaces.Set(spec.keyspace.Name(), spec)
	}

	return nil
}

func (x *engine) replaceKeySpaceInConfig(keyspace KeySpace) {
	for idx, existing := range x.config.keySpaces {
		if existing.Name() == keyspace.Name() {
			x.config.keySpaces[idx] = keyspace
			return
		}
	}
	x.config.keySpaces = append(x.config.keySpaces, keyspace)
}

func (x *engine) removeKeySpaceFromConfig(name string) {
	for idx, existing := range x.config.keySpaces {
		if existing.Name() == name {
			x.config.keySpaces = append(x.config.keySpaces[:idx], x.config.keySpaces[idx+1:]...)
			return
		}
	}
}
