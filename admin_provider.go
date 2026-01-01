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

import "github.com/tochemey/distcache/admin"

func newAdminProvider(eng *engine) admin.SnapshotProvider {
	return &adminProvider{engine: eng}
}

type adminProvider struct {
	engine *engine
}

func (x *adminProvider) SnapshotPeers() (any, error) {
	if x.engine == nil {
		return nil, nil
	}

	return x.engine.snapshotPeers()
}

func (x *adminProvider) SnapshotKeySpaces() any {
	if x.engine == nil {
		return nil
	}

	return x.engine.snapshotKeySpaces()
}

func (x *engine) snapshotPeers() ([]*Peer, error) {
	peers, err := x.peers()
	if err != nil {
		return nil, err
	}

	result := make([]*Peer, 0, len(peers)+1)
	result = append(result, x.hostNode)
	result = append(result, peers...)
	return result, nil
}

func (x *engine) snapshotKeySpaces() []admin.KeySpaceSnapshot {
	keyspaces := make([]admin.KeySpaceSnapshot, 0, x.keySpaces.Len())
	x.keySpaces.Range(func(_ string, spec *keySpaceWrapper) {
		if spec == nil {
			return
		}
		var mainBytes int64
		var hotBytes int64
		var stats *admin.KeySpaceStatsSnapshot
		if group, ok := x.groups.Get(spec.keyspace.Name()); ok && group != nil {
			mainBytes, hotBytes = group.UsedBytes()
			stats = admin.SnapshotKeySpaceStats(group)
		}
		warmKeys := append([]string(nil), spec.config.WarmKeys...)
		keyspaces = append(keyspaces, admin.KeySpaceSnapshot{
			Name:           spec.keyspace.Name(),
			MaxBytes:       spec.config.MaxBytes,
			DefaultTTL:     spec.config.DefaultTTL,
			ReadTimeout:    x.readTimeout(spec),
			WriteTimeout:   x.writeTimeout(spec),
			WarmKeys:       warmKeys,
			MainCacheBytes: mainBytes,
			HotCacheBytes:  hotBytes,
			Stats:          stats,
		})
	})
	return keyspaces
}
