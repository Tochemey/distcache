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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEventTypeString(t *testing.T) {
	require.Equal(t, "peer_joined", EventPeerJoined.String())
	require.Equal(t, "peer_left", EventPeerLeft.String())
	require.Equal(t, "peer_updated", EventPeerUpdated.String())
	require.Equal(t, "unknown", EventType(99).String())
}

func TestEventBusFanOut(t *testing.T) {
	bus := newEventBus()
	subA := bus.subscribe()
	subB := bus.subscribe()

	bus.publish(Event{Type: EventPeerJoined, At: time.Now()})

	require.Equal(t, EventPeerJoined, (<-subA).Type)
	require.Equal(t, EventPeerJoined, (<-subB).Type)
}

func TestEventBusDropsOnFullSubscriber(t *testing.T) {
	bus := newEventBus()
	sub := bus.subscribe()

	for range eventBufferSize + 10 {
		bus.publish(Event{Type: EventPeerJoined})
	}

	// Buffer should be exactly eventBufferSize; extras are dropped, not blocked.
	require.Len(t, sub, eventBufferSize)
}

func TestEventBusCloseStopsDelivery(t *testing.T) {
	bus := newEventBus()
	sub := bus.subscribe()
	bus.close()

	_, ok := <-sub
	require.False(t, ok, "channel should be closed")

	// Subsequent subscriptions return an already-closed channel.
	late := bus.subscribe()
	_, ok = <-late
	require.False(t, ok)

	// Publish after close is a no-op (must not panic).
	bus.publish(Event{Type: EventPeerJoined})
}

func TestEnginePublishEventNilSafe(t *testing.T) {
	eng := &engine{}
	eng.publishEvent(EventPeerJoined, &Peer{})
}
