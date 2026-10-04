package host

import (
	"encoding/json"
	"sync"
)

// Event is one typed bus event. Publish marshals Data exactly once and hands
// the same bytes to the sink and to every subscriber.
type Event struct {
	Type    string // host event type; serve maps it to an opencode SSE name
	Session string // owning session id; "" for server-wide
	Data    any
}

// Bus is the typed event bus. Exported despite having no in-slice caller: the repo's
// `unused` linter rejects unexported dead code and the item names the bus. Unexporting
// this breaks gate 3.
type Bus struct {
	sink        func(Event, []byte)
	mu          sync.Mutex
	subscribers map[uint64]*Subscription
	nextSubID   uint64
}

func newBus(sink func(Event, []byte)) *Bus {
	return &Bus{
		sink:        sink,
		subscribers: make(map[uint64]*Subscription),
	}
}

// Publish calls json.Marshal(ev) once and passes the resulting []byte to the
// sink and to every subscriber, so fan-out is one pointer copy each, never a
// per-subscriber marshal.
func (b *Bus) Publish(ev Event) {
	payload, err := json.Marshal(ev)
	if err != nil {
		return
	}
	if b.sink != nil {
		b.sink(ev, payload)
	}

	b.mu.Lock()
	subs := make([]*Subscription, 0, len(b.subscribers))
	for _, sub := range b.subscribers {
		subs = append(subs, sub)
	}
	b.mu.Unlock()

	for _, sub := range subs {
		select {
		case sub.ch <- payload:
		case <-sub.doneCh:
		}
	}
}

// Subscribe creates a new subscription with the given buffer size.
// buffer is a parameter, not a constant, because the capacity derives from a
// maximum event size which is a property of serve-4's SSE mapping and does not
// exist yet.
func (b *Bus) Subscribe(buffer int) *Subscription {
	b.mu.Lock()
	id := b.nextSubID
	b.nextSubID++
	sub := &Subscription{
		bus:    b,
		id:     id,
		ch:     make(chan []byte, buffer),
		doneCh: make(chan struct{}),
	}
	b.subscribers[id] = sub
	b.mu.Unlock()
	return sub
}

// Subscription is one subscriber's handle. doneCh is the field, Done the accessor:
// distinct names, so the select in Publish cannot be confused with the method.
type Subscription struct {
	bus    *Bus
	id     uint64
	ch     chan []byte
	doneCh chan struct{}
}

// Payloads returns a read-only channel of serialized event payloads.
func (s *Subscription) Payloads() <-chan []byte {
	return s.ch
}

// Done returns a channel that is closed when the subscription is closed.
func (s *Subscription) Done() <-chan struct{} {
	return s.doneCh
}

// Close removes the subscription from the bus and closes doneCh.
// It does NOT close the payload channel. Publish snapshots the subscriber
// list under the mutex, then sends outside it. Closing the payload channel
// would be wrong twice: a Publish already past the mutex would send on a
// closed channel, and holding the mutex across the send to avoid that
// deadlocks against a consumer that calls Close when the channel is full.
func (s *Subscription) Close() {
	s.bus.mu.Lock()
	delete(s.bus.subscribers, s.id)
	s.bus.mu.Unlock()
	close(s.doneCh)
}
