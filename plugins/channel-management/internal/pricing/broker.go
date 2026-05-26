// Package pricing — broker.go
//
// Broker is the in-process pub/sub fanout that channel-management uses to
// turn DB CRUD into PricingOverrideEvent stream traffic for the host's
// PricingExtension Watch loop.
//
// Lifecycle:
//
//   - One Broker is constructed at plugin startup and shared by the
//     PricingExtension Server (publisher target on the read side) and the
//     ChannelService (publisher on CRUD commit).
//   - Subscribers register from WatchPricingOverrides handlers; one
//     subscriber per active gRPC stream. Unsubscribe is invoked on stream
//     close (ctx done or send error).
//   - Publish is non-blocking: when a subscriber's buffered channel is
//     full, the broker drops the subscriber and closes its channel so the
//     Watch loop returns to the host, which re-syncs via
//     ListPricingOverrides on reconnect. This keeps slow consumers from
//     stalling CRUD writers.
//
// The broker carries no business logic — it only owns the subscriber set
// and the goroutine-safe fanout. Event construction (UPSERT vs DELETE,
// version assignment, payload encoding) lives in the publisher (see
// ChannelService) so the broker stays reusable for future event types.

package pricing

import (
	"log/slog"
	"sync"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// brokerSubBuffer is the buffered chan size handed to each subscriber.
// Sized so a typical CRUD burst (one channel update fanning out to N
// (group, platform, model) tuples) lands without dropping when the host
// stream consumes promptly. If the host stalls (network blip, GC pause,
// host shutdown), the buffer absorbs the burst long enough for the
// Watch goroutine to either resume or be reaped by ctx cancellation.
const brokerSubBuffer = 64

// Broker fans events out to a dynamic set of in-process subscribers.
//
// All public methods are safe to call from multiple goroutines. The zero
// value is not usable; construct via NewBroker.
type Broker struct {
	mu     sync.Mutex
	subs   map[*subscriber]struct{}
	closed bool
}

// subscriber is one active Watch stream. ch is buffered; closeOnce
// guards against double-close from concurrent unsubscribe + drop paths.
type subscriber struct {
	ch        chan *pb.PricingOverrideEvent
	closeOnce sync.Once
}

// NewBroker returns an empty Broker ready to accept Subscribe / Publish
// calls. The returned value owns its subscriber map; do not copy.
func NewBroker() *Broker {
	return &Broker{
		subs: make(map[*subscriber]struct{}),
	}
}

// Subscribe registers a new subscriber and returns its receive-only
// channel plus an unsubscribe func.
//
//   - The channel is buffered (brokerSubBuffer). Receivers must drain
//     promptly; the broker drops and closes the channel when a Publish
//     would block.
//   - The channel is closed exactly once: by unsubscribe (clean exit) or
//     by the broker (drop on full buffer / Close). Receivers must use the
//     two-value form `evt, ok := <-ch` to detect closure.
//   - Calling unsubscribe twice is safe (second call is a no-op).
//
// Subscribe on a closed broker returns an already-closed channel and a
// no-op unsubscribe so callers do not need to handle that race specially.
func (b *Broker) Subscribe() (<-chan *pb.PricingOverrideEvent, func()) {
	sub := &subscriber{
		ch: make(chan *pb.PricingOverrideEvent, brokerSubBuffer),
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		sub.closeOnce.Do(func() { close(sub.ch) })
		return sub.ch, func() {}
	}
	b.subs[sub] = struct{}{}
	b.mu.Unlock()
	return sub.ch, func() { b.unsubscribe(sub) }
}

// unsubscribe removes sub from the registry and closes its channel.
// Idempotent — repeated calls are safe.
func (b *Broker) unsubscribe(sub *subscriber) {
	b.mu.Lock()
	delete(b.subs, sub)
	b.mu.Unlock()
	sub.closeOnce.Do(func() { close(sub.ch) })
}

// Publish fans evt out to every subscriber. Subscribers whose buffered
// channel is full are dropped and their channels closed — this keeps the
// publisher non-blocking even when a stream consumer is slow or dead.
//
// Publish is a no-op when evt is nil or the broker is closed.
func (b *Broker) Publish(evt *pb.PricingOverrideEvent) {
	if evt == nil {
		return
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	// Snapshot subscribers to avoid sending under the lock.
	snapshot := make([]*subscriber, 0, len(b.subs))
	for s := range b.subs {
		snapshot = append(snapshot, s)
	}
	b.mu.Unlock()

	var dropped []*subscriber
	for _, s := range snapshot {
		select {
		case s.ch <- evt:
		default:
			dropped = append(dropped, s)
		}
	}
	if len(dropped) == 0 {
		return
	}

	// Reap slow subscribers. The host's Watch loop will reconnect and
	// re-sync via ListPricingOverrides when it sees the stream end, so
	// dropping here is the safe fallback for sub-second freshness.
	b.mu.Lock()
	for _, s := range dropped {
		delete(b.subs, s)
	}
	b.mu.Unlock()
	for _, s := range dropped {
		s.closeOnce.Do(func() { close(s.ch) })
	}
	slog.Warn("pricing broker: dropped slow subscribers",
		"count", len(dropped))
}

// Close marks the broker as closed and tears down every subscriber.
// Subsequent Subscribe / Publish calls become no-ops. Used during plugin
// shutdown to unblock Watch handlers waiting on the channel.
func (b *Broker) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	subs := b.subs
	b.subs = nil
	b.mu.Unlock()
	for s := range subs {
		s.closeOnce.Do(func() { close(s.ch) })
	}
}

// SubscriberCount returns the current subscriber count. Test-only helper.
func (b *Broker) SubscriberCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}
