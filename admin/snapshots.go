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
	"time"

	"github.com/groupcache/groupcache-go/v3"
	"github.com/groupcache/groupcache-go/v3/transport"
)

// KeySpaceSnapshot is the admin-facing JSON payload describing a keyspace at a
// single point in time.
//
// It combines configuration (capacity, TTL, read/write timeouts, warm keys)
// with runtime cache sizing and optional stats derived from the underlying
// group implementation. Time-based fields are serialized using time.Duration's
// string form (for example, "500ms"). Stats is nil when the group does not
// expose counters, so callers should treat it as optional.
type KeySpaceSnapshot struct {
	Name           string                 `json:"name"`
	MaxBytes       int64                  `json:"max_bytes"`
	DefaultTTL     time.Duration          `json:"default_ttl"`
	ReadTimeout    time.Duration          `json:"read_timeout"`
	WriteTimeout   time.Duration          `json:"write_timeout"`
	WarmKeys       []string               `json:"warm_keys,omitempty"`
	MainCacheBytes int64                  `json:"main_cache_bytes"`
	HotCacheBytes  int64                  `json:"hot_cache_bytes"`
	Stats          *KeySpaceStatsSnapshot `json:"stats,omitempty"`
}

// KeySpaceStatsSnapshot captures keyspace-level counters and gauges reported by
// the underlying groupcache group.
//
// Values are sampled at request time. Counters represent process-lifetime
// totals and are not reset between calls. GetFromPeersLatencyLower is the
// maximum observed peer fetch latency in milliseconds, matching groupcache's
// instrumentation. RemoveKeysRequests and RemovedKeys track explicit
// invalidation activity via RemoveKeys.
type KeySpaceStatsSnapshot struct {
	Gets                     int64 `json:"gets"`
	CacheHits                int64 `json:"cache_hits"`
	GetFromPeersLatencyLower int64 `json:"get_from_peers_latency_lower"`
	PeerLoads                int64 `json:"peer_loads"`
	PeerErrors               int64 `json:"peer_errors"`
	Loads                    int64 `json:"loads"`
	LoadsDeduped             int64 `json:"loads_deduped"`
	LocalLoads               int64 `json:"local_loads"`
	LocalLoadErrs            int64 `json:"local_load_errs"`
	RemoveKeysRequests       int64 `json:"remove_keys_requests"`
	RemovedKeys              int64 `json:"removed_keys"`
}

// SnapshotKeySpaceStats captures groupcache statistics when supported.
func SnapshotKeySpaceStats(group transport.Group) *KeySpaceStatsSnapshot {
	gcGroup, ok := group.(groupcache.Group)
	if !ok {
		return nil
	}
	stats := gcGroup.GroupStats()
	return &KeySpaceStatsSnapshot{
		Gets:                     stats.Gets.Get(),
		CacheHits:                stats.CacheHits.Get(),
		GetFromPeersLatencyLower: stats.GetFromPeersLatencyLower.Get(),
		PeerLoads:                stats.PeerLoads.Get(),
		PeerErrors:               stats.PeerErrors.Get(),
		Loads:                    stats.Loads.Get(),
		LoadsDeduped:             stats.LoadsDeduped.Get(),
		LocalLoads:               stats.LocalLoads.Get(),
		LocalLoadErrs:            stats.LocalLoadErrs.Get(),
		RemoveKeysRequests:       stats.RemoveKeysRequests.Get(),
		RemovedKeys:              stats.RemovedKeys.Get(),
	}
}
