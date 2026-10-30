// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"encoding/json"
	"log/slog"
	"sync"
)

// Event types (API.md; do not add values without updating it).
const (
	EventFileUpdated    = "file.updated"
	EventJobUpdated     = "job.updated"
	EventNodeUpdated    = "node.updated"
	EventLibraryUpdated = "library.updated"
	// The in-app update (task 17): progress carries the stage name,
	// succeeded and failed carry the outcome.
	EventUpdateProgress  = "update.progress"
	EventUpdateSucceeded = "update.succeeded"
	EventUpdateFailed    = "update.failed"
)

// Event is one SSE message: the type plus pre-marshaled JSON data.
type Event struct {
	Type string
	Data []byte
}

// EventBus fans server-side state changes out to SSE subscribers. Publish
// never blocks: a slow subscriber's buffer is dropped from instead of
// stalling scans or queue operations — events are best-effort UI state.
type EventBus struct {
	mu   sync.Mutex
	next int
	subs map[int]*subscriber
	log  *slog.Logger
}

type subscriber struct {
	ch chan Event
}

// NewEventBus builds an event bus. log may be nil.
func NewEventBus(log *slog.Logger) *EventBus {
	if log == nil {
		log = slog.Default()
	}
	return &EventBus{subs: map[int]*subscriber{}, log: log}
}

// eventBuffer is how many events a subscriber may lag behind before drops.
const eventBuffer = 64

// Subscribe registers a new subscriber and returns its id plus the event
// channel. Call Unsubscribe with the id when done.
func (b *EventBus) Subscribe() (int, <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.next
	b.next++
	ch := make(chan Event, eventBuffer)
	b.subs[id] = &subscriber{ch: ch}
	return id, ch
}

// Unsubscribe removes a subscriber and closes its channel.
func (b *EventBus) Unsubscribe(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if sub, ok := b.subs[id]; ok {
		delete(b.subs, id)
		close(sub.ch)
	}
}

// Publish marshals payload and delivers the event to every subscriber.
// Events are dropped for subscribers whose buffer is full.
func (b *EventBus) Publish(eventType string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		b.log.Error("event marshal", "type", eventType, "err", err)
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, sub := range b.subs {
		select {
		case sub.ch <- Event{Type: eventType, Data: data}:
		default:
			// Slow client; drop rather than block the publisher.
		}
	}
}
