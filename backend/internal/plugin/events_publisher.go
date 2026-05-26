// Package plugin — events_publisher.go
//
// In-process EventPublisher and the per-subscriber state it owns. Split from
// events_extension_server.go (T25) so the gRPC handler stays under the 600-
// line wrapper budget while still sharing the same package and unexported
// types. Wire contract — buffer / drop-oldest / send-timeout — is documented
// at the top of events_extension_server.go and on the proto service block.
package plugin

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

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
//
// P12·B-1 renamed the canonical name to "events.subscribe.gateway"; the
// legacy "events.gateway" alias is normalised to this value by the
// manager's approveCapabilities, so the registry only ever stores the
// new name.
const CapabilityEventsGateway = "events.subscribe.gateway"

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
//
// sendTimer 与 sendDone 仅由单一 pump goroutine 使用，复用避免每条事件分配
// 新的 chan / timer。dropped / closed 由其它 goroutine 触达，所以仍保留原
// 子 / chan 类型。
type eventSubscriber struct {
	pluginName   string
	eventTypes   map[string]struct{} // empty = all declared (allow-list applied at registration)
	capabilities map[string]struct{}
	buffer       chan *pb.HostEvent
	dropped      atomic.Uint64
	closeOnce    sync.Once
	closed       chan struct{}

	// sendTimer 在 pump 内复用，替代 time.After 的反复分配。
	sendTimer *time.Timer
	// sendDone 缓冲为 1，复用同一个 chan 接收每次 stream.Send 的结果。
	sendDone chan error
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
	return &EventPublisher{
		subscribers: make(map[string]*eventSubscriber),
		logger:      loggerOrDefault(logger).With("component", "plugin_events"),
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

// subscriberCount returns the number of currently registered subscribers.
// Used by ProbeSubscription to surface a debug counter; ops dashboards may
// also probe via the publisher directly. Safe for concurrent use.
//
// 仅作为观测指标暴露给 Probe 响应；nil receiver 返回 0 让 host 在尚未注入
// publisher 时也能优雅地报告 “0 个订阅者”。
func (p *EventPublisher) subscriberCount() int {
	if p == nil {
		return 0
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.subscribers)
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

// PublishGeneric dispatches a fully-formed *pb.HostEvent to every
// interested subscriber and reports how many subscribers accepted it
// synchronously (i.e. successfully enqueued without dropping). The
// counter is observability-only — callers MUST NOT branch on it.
//
// Used by EventsExtension.Publish so plugins can emit their own events
// (e.g. payment.order.created from a payment plugin) over the same
// fan-out machinery the host's typed helpers use.
func (p *EventPublisher) PublishGeneric(event *pb.HostEvent) int64 {
	if p == nil || event == nil {
		return 0
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	var delivered int64
	for _, sub := range p.subscribers {
		if !subscriberWants(sub, event.GetEventType()) {
			continue
		}
		select {
		case sub.buffer <- event:
			delivered++
		default:
			// Buffer full — drop the oldest entry to make room.
			select {
			case <-sub.buffer:
				sub.dropped.Add(1)
				p.logger.Warn("event buffer full, dropped oldest",
					"plugin", sub.pluginName,
					"event_type", event.GetEventType(),
					"dropped_total", sub.dropped.Load())
			default:
			}
			select {
			case sub.buffer <- event:
				delivered++
			default:
				sub.dropped.Add(1)
			}
		}
	}
	return delivered
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
