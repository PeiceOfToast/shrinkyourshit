// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventBusPublishSubscribe(t *testing.T) {
	bus := NewEventBus(nil)
	id, ch := bus.Subscribe()
	defer bus.Unsubscribe(id)

	bus.Publish(EventJobUpdated, map[string]string{"id": "j1"})

	select {
	case ev := <-ch:
		assert.Equal(t, EventJobUpdated, ev.Type)
		assert.Contains(t, string(ev.Data), `"id":"j1"`)
	case <-time.After(time.Second):
		t.Fatal("event not delivered")
	}
}

func TestEventBusUnsubscribeClosesChannel(t *testing.T) {
	bus := NewEventBus(nil)
	id, ch := bus.Subscribe()
	bus.Publish(EventFileUpdated, map[string]string{"id": "f1"})
	<-ch // drain
	bus.Unsubscribe(id)

	_, ok := <-ch
	assert.False(t, ok, "channel closed after unsubscribe")

	// Publishing after unsubscribe must not panic.
	assert.NotPanics(t, func() {
		bus.Publish(EventFileUpdated, map[string]string{"id": "f2"})
	})
}

func TestEventBusDropsForSlowSubscriber(t *testing.T) {
	bus := NewEventBus(nil)
	id, ch := bus.Subscribe()
	defer bus.Unsubscribe(id)

	// Flood well past the buffer: the publisher must not block.
	for i := range eventBuffer * 4 {
		bus.Publish(EventJobUpdated, map[string]int{"n": i})
	}

	// The subscriber sees some events and the publisher never stalled.
	received := 0
	for {
		select {
		case <-ch:
			received++
			continue
		default:
		}
		break
	}
	assert.Greater(t, received, 0)
	assert.LessOrEqual(t, received, eventBuffer)
}

func TestEventBusMultipleSubscribers(t *testing.T) {
	bus := NewEventBus(nil)
	id1, ch1 := bus.Subscribe()
	id2, ch2 := bus.Subscribe()
	defer bus.Unsubscribe(id1)
	defer bus.Unsubscribe(id2)

	bus.Publish(EventLibraryUpdated, map[string]string{"id": "l1"})

	for _, ch := range []<-chan Event{ch1, ch2} {
		select {
		case ev := <-ch:
			require.Equal(t, EventLibraryUpdated, ev.Type)
		case <-time.After(time.Second):
			t.Fatal("subscriber missed the event")
		}
	}
}
