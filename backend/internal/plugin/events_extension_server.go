// Package plugin — events_extension_server.go
//
// Phase B — host EventsExtension gRPC server + in-process EventPublisher.
//
// The host calls EventPublisher.Publish<TypedHelper>() from business code
// (payment, gateway, auth, ratelimit) to fan an event out to every plugin
// subscriber matching the requested type. Delivery semantics are documented
// in plugin-sdk/proto/sdk.proto (EventsExtension service block):
//
//   - at-most-once, no replay, no persistence.
//   - per-subscriber buffer: 256 events; full buffer drops the oldest entry
//     and bumps dropped_since_last_send on the next successful send.
//   - send timeout: 2s. Timeout or send error closes the stream; the SDK
//     reconnects with backoff.
//   - high-frequency events (currently only gateway.model.invoked) require
//     the events.gateway capability — Subscribe rejects with PERMISSION_DENIED
//     otherwise.
//   - manifest.SubscribedEvents acts as an allow-list: a plugin cannot
//     subscribe to a type it did not declare up front. Empty event_types in
//     the request expands to "all declared types".
//
// Publish is non-blocking: the per-subscriber send happens on a goroutine
// pulling from the buffer, and Publish itself only does a select-default
// drop-oldest + buffered enqueue. Business main paths must never wait on
// plugin delivery.
package plugin

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// Event type strings — keep in sync with plugin-sdk/proto/sdk.proto and the
// SDK-side constants. They are deliberately literal here so the plugin
// package does not pick up a reverse dependency on pluginsdk constants
// beyond what we already import.
const (
	EventTypePaymentOrderCreated       = "payment.order.created"
	EventTypePaymentOrderFulfilled     = "payment.order.fulfilled"
	EventTypeGatewayModelInvoked       = "gateway.model.invoked"
	EventTypeAuthUserRegistered        = "auth.user.registered"
	EventTypeAccountRateLimitTriggered = "account.rate_limit.triggered"
)

// CapabilityEventsGateway is the manifest capability required to subscribe
// to high-frequency gateway events. Mirrors pluginsdk.Capability* style
// without forcing an extra import.
const CapabilityEventsGateway = "events.gateway"

// highFrequencyEventCapability maps an event type to the capability a
// plugin must hold in order to subscribe to it. Events absent from this
// table are unrestricted.
var highFrequencyEventCapability = map[string]string{
	EventTypeGatewayModelInvoked: CapabilityEventsGateway,
}

// eventBufferSize is the per-subscriber buffer documented in the proto
// contract. Full buffer drops the oldest entry. Sized to absorb a short
// burst (e.g. a sweep over many accounts) without applying back-pressure
// to host code.
const eventBufferSize = 256

// eventSendTimeout is the deadline applied to a single stream.Send call.
// Exceeding it closes the stream and lets the SDK reconnect.
const eventSendTimeout = 2 * time.Second

// eventSubscriber is the per-plugin state held by the publisher. Only the
// publisher's pump goroutine writes to lastSendTime / dropped after the
// subscriber is registered; readers (Publish, tests) use atomics so we
// avoid taking the publisher mutex on every fan-out.
type eventSubscriber struct {
	pluginName   string
	eventTypes   map[string]struct{} // empty = all declared (allow-list applied at registration)
	capabilities map[string]struct{}
	buffer       chan *pb.HostEvent
	dropped      atomic.Uint64
	closeOnce    sync.Once
	closed       chan struct{}
}

// close signals the pump goroutine to stop. Safe to call multiple times.
func (s *eventSubscriber) close() {
	s.closeOnce.Do(func() { close(s.closed) })
}

// EventPublisher is the host-side fan-out hub. It is safe for concurrent
// use; the public Publish* helpers acquire only a read lock so the hot
// path scales linearly with the number of subscribers.
type EventPublisher struct {
	mu sync.RWMutex
	// subscribers keyed by plugin name. Re-Subscribe by the same plugin
	// closes the previous entry — see Subscribe.
	subscribers map[string]*eventSubscriber

	logger *slog.Logger
}

// NewEventPublisher constructs an empty publisher. Callers wire it into
// the SDK gRPC server via NewEventsExtensionServer and inject the same
// publisher into business services so they can publish.
func NewEventPublisher(logger *slog.Logger) *EventPublisher {
	if logger == nil {
		logger = slog.Default()
	}
	return &EventPublisher{
		subscribers: make(map[string]*eventSubscriber),
		logger:      logger.With("component", "plugin_events"),
	}
}

// register attaches a subscriber. If a subscriber with the same plugin
// name already exists it is closed (the old gRPC stream sees its buffer
// channel close on the next read and exits).
func (p *EventPublisher) register(sub *eventSubscriber) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if old, ok := p.subscribers[sub.pluginName]; ok {
		old.close()
	}
	p.subscribers[sub.pluginName] = sub
}

// unregister removes a subscriber. Compares by pointer because a
// re-Subscribe between the pump goroutine starting and ending would
// otherwise have unregister evict the new subscriber instead of the old.
func (p *EventPublisher) unregister(sub *eventSubscriber) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if cur, ok := p.subscribers[sub.pluginName]; ok && cur == sub {
		delete(p.subscribers, sub.pluginName)
	}
}

// publish dispatches the event to every interested subscriber. Non-blocking:
// when a subscriber's buffer is full we drop the oldest entry and increment
// its dropped counter; the next successful send carries the count.
func (p *EventPublisher) publish(eventType string, build func() *pb.HostEvent) {
	if p == nil {
		return
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.subscribers) == 0 {
		return
	}
	var event *pb.HostEvent // lazy-built so a no-subscriber call costs ~nothing
	for _, sub := range p.subscribers {
		if !subscriberWants(sub, eventType) {
			continue
		}
		if event == nil {
			event = build()
		}
		// Non-blocking enqueue with drop-oldest.
		select {
		case sub.buffer <- event:
		default:
			// Buffer full — drop the oldest entry to make room.
			select {
			case <-sub.buffer:
				sub.dropped.Add(1)
				p.logger.Warn("event buffer full, dropped oldest",
					"plugin", sub.pluginName,
					"event_type", eventType,
					"dropped_total", sub.dropped.Load())
			default:
				// Race: someone else just drained an entry; retry once.
			}
			select {
			case sub.buffer <- event:
			default:
				// Still full and we already accounted for the drop above.
				sub.dropped.Add(1)
			}
		}
	}
}

func subscriberWants(sub *eventSubscriber, eventType string) bool {
	if len(sub.eventTypes) == 0 {
		return true
	}
	_, ok := sub.eventTypes[eventType]
	return ok
}

// PublishPaymentOrderCreated is the typed helper for payment.order.created.
func (p *EventPublisher) PublishPaymentOrderCreated(payload *pb.PaymentOrderCreated) {
	p.publish(EventTypePaymentOrderCreated, func() *pb.HostEvent {
		return &pb.HostEvent{
			EventType:      EventTypePaymentOrderCreated,
			TimestampNanos: time.Now().UnixNano(),
			Payload: &pb.HostEvent_PaymentOrderCreated{
				PaymentOrderCreated: payload,
			},
		}
	})
}

// PublishPaymentOrderFulfilled — payment.order.fulfilled.
func (p *EventPublisher) PublishPaymentOrderFulfilled(payload *pb.PaymentOrderFulfilled) {
	p.publish(EventTypePaymentOrderFulfilled, func() *pb.HostEvent {
		return &pb.HostEvent{
			EventType:      EventTypePaymentOrderFulfilled,
			TimestampNanos: time.Now().UnixNano(),
			Payload: &pb.HostEvent_PaymentOrderFulfilled{
				PaymentOrderFulfilled: payload,
			},
		}
	})
}

// PublishGatewayModelInvoked — gateway.model.invoked. High-frequency: the
// SDK side requires the events.gateway capability.
func (p *EventPublisher) PublishGatewayModelInvoked(payload *pb.GatewayModelInvoked) {
	p.publish(EventTypeGatewayModelInvoked, func() *pb.HostEvent {
		return &pb.HostEvent{
			EventType:      EventTypeGatewayModelInvoked,
			TimestampNanos: time.Now().UnixNano(),
			Payload: &pb.HostEvent_GatewayModelInvoked{
				GatewayModelInvoked: payload,
			},
		}
	})
}

// PublishAuthUserRegistered — auth.user.registered.
func (p *EventPublisher) PublishAuthUserRegistered(payload *pb.AuthUserRegistered) {
	p.publish(EventTypeAuthUserRegistered, func() *pb.HostEvent {
		return &pb.HostEvent{
			EventType:      EventTypeAuthUserRegistered,
			TimestampNanos: time.Now().UnixNano(),
			Payload: &pb.HostEvent_AuthUserRegistered{
				AuthUserRegistered: payload,
			},
		}
	})
}

// PublishAccountRateLimitTriggered — account.rate_limit.triggered.
func (p *EventPublisher) PublishAccountRateLimitTriggered(payload *pb.AccountRateLimitTriggered) {
	p.publish(EventTypeAccountRateLimitTriggered, func() *pb.HostEvent {
		return &pb.HostEvent{
			EventType:      EventTypeAccountRateLimitTriggered,
			TimestampNanos: time.Now().UnixNano(),
			Payload: &pb.HostEvent_AccountRateLimitTriggered{
				AccountRateLimitTriggered: payload,
			},
		}
	})
}

// EventsAllowList is the lookup the EventsExtensionServer uses to validate
// requested event_types against a plugin's manifest declaration.
type EventsAllowList interface {
	// AllowedEvents returns the set of event types the named plugin
	// declared in manifest.SubscribedEvents (verbatim, no expansion).
	AllowedEvents(pluginName string) []string
}

// EventsAllowListRegistry stores per-plugin SubscribedEvents declarations.
// Populated by the manager once a plugin's manifest has been processed.
type EventsAllowListRegistry struct {
	mu      sync.RWMutex
	entries map[string][]string
}

// NewEventsAllowListRegistry constructs an empty registry.
func NewEventsAllowListRegistry() *EventsAllowListRegistry {
	return &EventsAllowListRegistry{entries: make(map[string][]string)}
}

// Set replaces the declared events for the plugin. Empty slice means
// "no events declared", which Subscribe treats as deny-all.
func (r *EventsAllowListRegistry) Set(pluginName string, events []string) {
	cp := append([]string(nil), events...)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[pluginName] = cp
}

// Forget drops the entry. Called by the manager when stopping a plugin.
func (r *EventsAllowListRegistry) Forget(pluginName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, pluginName)
}

// AllowedEvents implements EventsAllowList.
func (r *EventsAllowListRegistry) AllowedEvents(pluginName string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cp := r.entries[pluginName]
	out := make([]string, len(cp))
	copy(out, cp)
	return out
}

// CapabilityChecker abstracts the manifest capability lookup. SDKServer
// already implements this via pluginCapabilityRegistry; tests pass a stub.
type CapabilityChecker interface {
	HasCapability(pluginName, capability string) bool
}

// EventsExtensionServer implements pb.EventsExtensionServer.
type EventsExtensionServer struct {
	pb.UnimplementedEventsExtensionServer

	publisher    *EventPublisher
	allowList    EventsAllowList
	capabilities CapabilityChecker
	resolver     func(ctx context.Context) string
	logger       *slog.Logger
}

// NewEventsExtensionServer constructs the gRPC service. resolver MUST be
// non-nil; passing nil substitutes a sentinel that always returns the
// empty string so PERMISSION_DENIED is reported instead of a nil panic.
func NewEventsExtensionServer(
	publisher *EventPublisher,
	allowList EventsAllowList,
	capabilities CapabilityChecker,
	resolver func(ctx context.Context) string,
) *EventsExtensionServer {
	if resolver == nil {
		resolver = func(context.Context) string { return "" }
	}
	return &EventsExtensionServer{
		publisher:    publisher,
		allowList:    allowList,
		capabilities: capabilities,
		resolver:     resolver,
		logger:       slog.Default().With("component", "plugin_events_grpc"),
	}
}

// Register attaches the server to the supplied gRPC server.
func (s *EventsExtensionServer) Register(grpcServer *grpc.Server) {
	pb.RegisterEventsExtensionServer(grpcServer, s)
}

// Subscribe implements pb.EventsExtensionServer.Subscribe. Lifecycle:
//  1. Resolve caller; reject anonymous.
//  2. Validate every requested event_type against the plugin's manifest
//     allow-list. Empty event_types expands to the full declared set.
//  3. Apply capability gating (events.gateway for gateway.model.invoked).
//  4. Register the subscriber with the publisher (replacing any prior
//     stream from the same plugin).
//  5. Pump events from the buffer to the stream until the context is
//     cancelled, the stream errors, or a single Send exceeds the 2s deadline.
func (s *EventsExtensionServer) Subscribe(
	req *pb.EventSubscribeRequest, stream grpc.ServerStreamingServer[pb.HostEvent],
) error {
	ctx := stream.Context()
	pluginName := s.resolver(ctx)
	if pluginName == "" {
		return status.Error(codes.PermissionDenied, "events: caller identity missing")
	}

	declared := s.declaredEvents(pluginName)
	if len(declared) == 0 {
		return status.Error(codes.PermissionDenied,
			"events: plugin did not declare any SubscribedEvents in its manifest")
	}

	requested, err := s.resolveRequestedTypes(req.GetEventTypes(), declared)
	if err != nil {
		return err
	}

	// Capability gate. We check after manifest validation so the error
	// message does not leak unrelated info.
	if err := s.checkCapabilities(pluginName, requested); err != nil {
		return err
	}

	sub := &eventSubscriber{
		pluginName:   pluginName,
		eventTypes:   requested,
		capabilities: s.collectCaps(pluginName),
		buffer:       make(chan *pb.HostEvent, eventBufferSize),
		closed:       make(chan struct{}),
	}
	s.publisher.register(sub)
	defer s.publisher.unregister(sub)
	defer sub.close()

	return s.pump(ctx, stream, sub)
}

// declaredEvents returns the SubscribedEvents declared in the plugin's
// manifest. Returned slice is a copy.
func (s *EventsExtensionServer) declaredEvents(pluginName string) []string {
	if s.allowList == nil {
		return nil
	}
	return s.allowList.AllowedEvents(pluginName)
}

// resolveRequestedTypes computes the final event-type filter. Empty
// req.event_types expands to all declared types. Any type outside the
// declared set rejects with PERMISSION_DENIED so plugins cannot widen
// their interest at runtime.
func (s *EventsExtensionServer) resolveRequestedTypes(
	requested, declared []string,
) (map[string]struct{}, error) {
	declaredSet := make(map[string]struct{}, len(declared))
	for _, e := range declared {
		declaredSet[e] = struct{}{}
	}
	if len(requested) == 0 {
		return declaredSet, nil
	}
	out := make(map[string]struct{}, len(requested))
	for _, e := range requested {
		if _, ok := declaredSet[e]; !ok {
			return nil, status.Errorf(codes.PermissionDenied,
				"events: type %q not declared in manifest.SubscribedEvents", e)
		}
		out[e] = struct{}{}
	}
	return out, nil
}

// checkCapabilities verifies every requested high-frequency event has the
// corresponding capability granted.
func (s *EventsExtensionServer) checkCapabilities(
	pluginName string, requested map[string]struct{},
) error {
	if s.capabilities == nil {
		return nil
	}
	for evt := range requested {
		capName, needs := highFrequencyEventCapability[evt]
		if !needs {
			continue
		}
		if !s.capabilities.HasCapability(pluginName, capName) {
			return status.Errorf(codes.PermissionDenied,
				"events: type %q requires capability %q", evt, capName)
		}
	}
	return nil
}

// collectCaps snapshots the plugin's capability set into a map for the
// subscriber record. Currently unused at delivery time but kept for
// future per-event runtime checks.
func (s *EventsExtensionServer) collectCaps(pluginName string) map[string]struct{} {
	out := make(map[string]struct{})
	if s.capabilities == nil {
		return out
	}
	for _, capName := range highFrequencyEventCapability {
		if s.capabilities.HasCapability(pluginName, capName) {
			out[capName] = struct{}{}
		}
	}
	return out
}

// pump moves events from the buffer to the stream. It enforces the 2s
// per-Send deadline using a watchdog timer; on timeout we close the stream
// so the SDK reconnects rather than allowing a stuck plugin to back up
// the host's buffer indefinitely.
func (s *EventsExtensionServer) pump(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.HostEvent],
	sub *eventSubscriber,
) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sub.closed:
			// Re-Subscribe by the same plugin closed our slot.
			return nil
		case event, ok := <-sub.buffer:
			if !ok {
				return nil
			}
			// Stamp dropped count atomically — Publish may bump it after
			// we read but the SDK contract is "since last successful
			// send", which is the value at this moment.
			drop := sub.dropped.Swap(0)
			event.DroppedSinceLastSend = drop
			if err := s.sendWithTimeout(stream, event); err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					s.logger.Warn("event send timeout, closing stream",
						"plugin", sub.pluginName, "event_type", event.EventType)
				}
				return err
			}
		}
	}
}

// sendWithTimeout enforces the 2s deadline by running stream.Send in a
// goroutine and racing it against a timer. Returns context.DeadlineExceeded
// on timeout and the underlying send error otherwise.
//
// gRPC server-streaming Send does not accept a context, so this is the
// only way to bound a slow client without touching unsafe internals. The
// goroutine leaks at most until the underlying Send returns or the
// connection is torn down.
func (s *EventsExtensionServer) sendWithTimeout(
	stream grpc.ServerStreamingServer[pb.HostEvent],
	event *pb.HostEvent,
) error {
	done := make(chan error, 1)
	go func() { done <- stream.Send(event) }()
	select {
	case err := <-done:
		return err
	case <-time.After(eventSendTimeout):
		return context.DeadlineExceeded
	}
}
