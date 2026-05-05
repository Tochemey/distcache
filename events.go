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
	"sync"
	"time"
)

// EventType identifies the kind of cluster event delivered to subscribers.
type EventType int

const (
	// EventPeerJoined is published when a peer joins the cluster.
	EventPeerJoined EventType = iota
	// EventPeerLeft is published when a peer leaves the cluster.
	EventPeerLeft
	// EventPeerUpdated is published when a peer's metadata changes.
	EventPeerUpdated
)

// String returns a stable lower-case identifier for the event type.
func (t EventType) String() string {
	switch t {
	case EventPeerJoined:
		return "peer_joined"
	case EventPeerLeft:
		return "peer_left"
	case EventPeerUpdated:
		return "peer_updated"
	default:
		return "unknown"
	}
}

// Event carries a single cluster-membership notification.
//
// Peer is the peer the event is about. At is the time the event was published
// by the engine (not when the underlying memberlist event was generated).
type Event struct {
	Type EventType
	Peer *Peer
	At   time.Time
}

// eventBufferSize is the per-subscriber channel buffer. Subscribers slower than
// this rate will lose events (publish is non-blocking, drop-on-full).
const eventBufferSize = 64

type eventBus struct {
	mu     sync.RWMutex
	subs   []chan Event
	closed bool
}

func newEventBus() *eventBus {
	return &eventBus{}
}

func (b *eventBus) subscribe() <-chan Event {
	ch := make(chan Event, eventBufferSize)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		close(ch)
		return ch
	}
	b.subs = append(b.subs, ch)
	return ch
}

func (b *eventBus) publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return
	}
	for _, ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

func (b *eventBus) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for _, ch := range b.subs {
		close(ch)
	}
	b.subs = nil
}
