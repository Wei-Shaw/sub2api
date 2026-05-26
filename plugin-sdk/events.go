// Package pluginsdk — events extension client.
//
// This file implements the plugin-side helpers for the Phase A
// EventsExtension capability. The plugin author interacts with two
// surfaces:
//
//  1. Manifest.SubscribedEvents — declares which HostEvent types this
//     plugin wants to receive. The host validates Subscribe requests
//     against this list and rejects undeclared types.
//
//  2. PluginContext.Events() — a thin wrapper around
//     EventsExtensionClient that dispatches typed events to a user-
//     supplied EventHandler and re-establishes the underlying stream
//     with exponential backoff (via streamutil.Loop) when it drops. See
//     sdk.proto's EventsExtension service block for the full protocol
//     contract (256-slot ring buffer, drop-oldest, 2s send timeout).
//
// Subscribe() is split into two phases (T25 — see sdk.proto changelog):
//   - synchronous ProbeSubscription RPC: validates capability + argument
//     legality. Errors here surface to the caller immediately.
//   - asynchronous streamutil.Loop running Subscribe (the streaming RPC).
//     Each iteration opens a fresh stream and drains events until the
//     stream errors; transient transport failures trigger a backoff and
//     retry, non-retryable errors stop the loop.
//
// Removing the historical "first-Recv probe" (peeling off the first
// stream message just to surface synchronous errors) eliminated an
// abstraction leak — Subscribe used to smuggle a pre-consumed event into
// the background loop, which made retry-correctness fragile because that
// event could survive a stream reset.
package pluginsdk

import (
	"context"
	"errors"
	"log/slog"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/streamutil"
)

// HostEvent re-exports pb.HostEvent so plugin authors don't have to
// import plugin-sdk/proto/pluginsdk directly. The accessors generated
// on the proto type (GetPaymentOrderCreated, GetGatewayModelInvoked,
// etc.) are the recommended way to extract the typed payload.
type HostEvent = pb.HostEvent

// Typed event payload aliases. One per oneof variant declared in
// sdk.proto's HostEvent message. Plugin authors switch on event type
// (HostEvent.GetEventType()) and call the matching getter; the typed
// payload is nil when the variant does not match.
type (
	PaymentOrderCreated       = pb.PaymentOrderCreated
	PaymentOrderFulfilled     = pb.PaymentOrderFulfilled
	GatewayModelInvoked       = pb.GatewayModelInvoked
	AuthUserRegistered        = pb.AuthUserRegistered
	AccountRateLimitTriggered = pb.AccountRateLimitTriggered
)

// EventType string constants. These mirror the host-side constants in
// backend/internal/plugin/events_publisher.go so plugin authors can
// declare SubscribedEvents and call Subscribe without typos.
const (
	EventTypePaymentOrderCreated       = "payment.order.created"
	EventTypePaymentOrderFulfilled     = "payment.order.fulfilled"
	EventTypeGatewayModelInvoked       = "gateway.model.invoked"
	EventTypeAuthUserRegistered        = "auth.user.registered"
	EventTypeAccountRateLimitTriggered = "account.rate_limit.triggered"
)

// EventHandler is invoked once per delivered event. It runs on the
// SDK's subscribe goroutine — there is one goroutine per Subscribe
// call, so handlers run serially per subscription.
//
// The host's 256-slot ring buffer + drop-oldest semantics protect
// throughput, but a handler that blocks for more than ~2s will cause
// the host to time out the send and close the stream (the SDK then
// reconnects with backoff). If you need to do non-trivial work, fork
// a goroutine inside the handler.
//
// The *HostEvent pointer must NOT be retained past the call; the SDK
// does not reuse the buffer today, but the contract is "forget on
// return" so we keep the option open.
type EventHandler func(ctx context.Context, event *HostEvent)

// EventsClient lets a plugin subscribe to typed host business events.
//
// Subscribe registers a callback for one or more event types. Subscribe
// returns a non-nil error only for synchronous failures
// (PERMISSION_DENIED on capability gate, undeclared event types,
// nil/empty inputs). Stream-level disconnects are handled internally
// with exponential backoff (1s → 2s → 4s → 8s → 30s, jittered) via
// streamutil.Loop.
//
// Subscribe does not block: it spins up an internal goroutine that
// runs the receive loop, then returns nil to the caller. Cancel ctx
// (or let the SDK shut down) to stop the loop.
type EventsClient interface {
	// Subscribe registers handler for the given event types. Each event
	// type must appear in Manifest.SubscribedEvents — the host enforces
	// this and returns InvalidArgument otherwise. The capability check
	// for high-frequency events (gateway.model.invoked → events.subscribe.gateway)
	// happens server-side; on failure Subscribe returns the wrapped
	// gRPC error so callers can branch on codes.PermissionDenied.
	Subscribe(ctx context.Context, eventTypes []string, handler EventHandler) error

	// Publish emits a HostEvent the host re-broadcasts to other subscribers.
	// The plugin must declare the matching capability (e.g.
	// CapabilityEventsPublishPayment for payment.* events) — the host
	// enforces this and returns codes.PermissionDenied otherwise.
	//
	// Errors:
	//   - codes.PermissionDenied: capability not granted
	//   - codes.InvalidArgument: event_type not allowlisted for this caller
	//   - codes.Unavailable: host not ready
	// Plugin authors should log+swallow Publish errors rather than
	// surfacing them to end users — events are best-effort signalling,
	// not a mandatory side-effect.
	Publish(ctx context.Context, event *HostEvent) error
}

// nilEventsClient is returned when the plugin process was started by
// a host that does not register the EventsExtension service (older
// hosts, or test rigs that wire only a subset of the SDK). Every call
// returns a clear error so debugging is straightforward; we do NOT
// silently no-op because that would mask real misconfiguration.
type nilEventsClient struct{}

func (nilEventsClient) Subscribe(ctx context.Context, types []string, h EventHandler) error {
	return errors.New("pluginsdk: EventsExtension not available on this host")
}

func (nilEventsClient) Publish(ctx context.Context, event *HostEvent) error {
	return errors.New("pluginsdk: EventsExtension not available on this host")
}

// eventsClient is the concrete implementation backing EventsClient.
// It holds a long-lived gRPC client; one client may serve any number
// of concurrent Subscribe calls (each spawns its own goroutine).
type eventsClient struct {
	grpc       pb.EventsExtensionClient
	pluginName string
	logger     *slog.Logger
}

// newEventsClient wires up an EventsClient on top of an existing gRPC
// connection. pluginName is captured for log attribution; the actual
// identity the host trusts is the metadata interceptor configured in
// runner.go.
func newEventsClient(grpc pb.EventsExtensionClient, pluginName string, logger *slog.Logger) *eventsClient {
	return &eventsClient{grpc: grpc, pluginName: pluginName, logger: logger}
}

// Backoff schedule for stream reconnects. Documented in sdk.proto's
// EventsExtension comment block (1s → 2s → 4s → 8s → 30s) so plugin
// authors can correlate host-side timeouts with reconnect attempts.
// The schedule is materialised through streamutil.Loop; constants stay
// here so plugin authors reading events.go see the contract without
// chasing into streamutil.
const (
	eventsInitialBackoff = 1 * time.Second
	eventsMaxBackoff     = 30 * time.Second
	// eventsBackoffMultiplier matches the SDK-wide ×2 schedule.
	eventsBackoffMultiplier = 2.0
	// eventsBackoffJitterFraction adds ±20% jitter to each delay so a
	// host restart that knocks out N plugins simultaneously does not
	// produce a synchronised reconnect storm. (Bumped from ±10% during
	// the streamutil.Loop unification — the SDK now uses ±20% across
	// all four reconnect sites.)
	eventsBackoffJitterFraction = 0.2
)

// Subscribe runs the two-phase setup documented at the package level:
// synchronous Probe → background streamutil.Loop. Probe failures are
// returned to the caller (capability misconfig requires operator action
// rather than a retry storm); transport failures during the streaming
// phase are absorbed by streamutil.Loop and cease only when ctx is
// cancelled or the host returns a non-retryable status.
//
// 抽象 (4)：Probe 抽象出 "同步前置校验" 语义；Loop 抽象出 "流式重连"
// 语义。Subscribe 不再做 "首条 Recv 探活"，调用者只看到 "发起订阅就好"，
// 看不到底层 attempt / pre-consumed event / 续传细节。
func (c *eventsClient) Subscribe(ctx context.Context, eventTypes []string, handler EventHandler) error {
	if handler == nil {
		return errors.New("pluginsdk: events Subscribe: handler must not be nil")
	}
	if len(eventTypes) == 0 {
		return errors.New("pluginsdk: events Subscribe: eventTypes must not be empty")
	}
	if ctx == nil {
		return errors.New("pluginsdk: events Subscribe: ctx must not be nil")
	}
	// Defensive copy so the caller can mutate the slice after Subscribe
	// returns without affecting the loop.
	types := append([]string(nil), eventTypes...)
	req := &pb.EventSubscribeRequest{EventTypes: types}

	// Phase 1: synchronous capability + argument validation. Probe never
	// registers a subscriber, so a Probe success does not leak any
	// resources if the caller decides not to start the loop afterwards.
	if _, err := c.grpc.ProbeSubscription(ctx, req); err != nil {
		return err
	}

	// Phase 2: background loop owning the streaming RPC. We only spin up
	// the goroutine after Probe accepts, so a permission-denied caller
	// never spawns one.
	go c.runEventsStreamLoop(ctx, req, handler)
	return nil
}

// Publish emits a single HostEvent through the host's EventsExtension.
// The event must already carry a populated EventType + matching oneof
// payload (use the typed payload aliases declared above and the proto
// builder pattern). publishRPCTimeout caps the synchronous round trip so
// a slow host cannot block the plugin's foreground work.
func (c *eventsClient) Publish(ctx context.Context, event *HostEvent) error {
	if event == nil {
		return errors.New("pluginsdk: events Publish: event must not be nil")
	}
	if ctx == nil {
		return errors.New("pluginsdk: events Publish: ctx must not be nil")
	}
	rpcCtx, cancel := context.WithTimeout(ctx, eventsPublishTimeout)
	defer cancel()
	_, err := c.grpc.Publish(rpcCtx, &pb.PublishEventRequest{Event: event})
	return err
}

// eventsPublishTimeout caps the Publish RPC. Events are best-effort and
// the host fan-out is buffered, so 2s is plenty; longer means the host
// is degraded and the plugin should not stall foreground work.
const eventsPublishTimeout = 2 * time.Second

// runEventsStreamLoop opens the Subscribe stream repeatedly, draining each
// stream until it errors or ctx is cancelled. streamutil.LoopWithRetryClass
// owns both the reconnect schedule (initial backoff + multiplier + jitter)
// AND the retry-vs-fatal decision, so attempt is now a pure
// "open + drain" function with no policy embedded.
//
// 抽象 (4)：T25 把 fatal-vs-retryable 的判断写在 attempt 里，再用 cancel()
// 强迫 Loop 退出；T30 把那段逻辑提升到 streamutil.LoopWithRetryClass +
// GRPCDefaultClassifier，attempt 不再持有 cancel 句柄、不再返回 nil 来
// "假装成功"。
//
// 复用 (1)：与 jobs / settings / log_remote 共用同一个 streamutil.Loop;
// 同一份 backoff schedule 不会因为各自实现而漂移。GRPCDefaultClassifier
// 也是共用的——所有 SDK 流客户端按同一标准判定 PermissionDenied /
// Unimplemented / InvalidArgument / Unauthenticated 是 fatal。
func (c *eventsClient) runEventsStreamLoop(
	ctx context.Context,
	req *pb.EventSubscribeRequest,
	handler EventHandler,
) {
	attempt := func(actx context.Context) error {
		stream, err := c.grpc.Subscribe(actx, req)
		if err != nil {
			// Both fatal (PermissionDenied / Unimplemented / ...) and
			// transient errors come back through this path. Return them
			// as-is so the classifier in LoopWithRetryClass decides; we
			// no longer need to call cancel() to stop the loop.
			return err
		}
		// drainStream returns nil on clean ctx-cancel exit, the
		// underlying Recv error otherwise. The classifier decides
		// whether that error is fatal.
		return c.drainStream(actx, stream, handler, req.GetEventTypes())
	}

	err := streamutil.LoopWithRetryClass(ctx, streamutil.Config{
		Name:        "events.subscribe",
		Initial:     eventsInitialBackoff,
		Max:         eventsMaxBackoff,
		Multiplier:  eventsBackoffMultiplier,
		JitterRatio: eventsBackoffJitterFraction,
		Logger:      c.logger,
	}, streamutil.GRPCDefaultClassifier, attempt)
	if err != nil && !errors.Is(err, context.Canceled) &&
		!errors.Is(err, context.DeadlineExceeded) && c.logger != nil {
		// Fatal errors are already logged by LoopWithRetryClass at
		// error level; we add the plugin / event-type context here so
		// operators can correlate the failure with a specific
		// subscription.
		c.logger.Error("events subscription stopped; not retrying",
			"plugin", c.pluginName,
			"types", req.GetEventTypes(),
			"error", err)
	}
}

// drainStream pulls events from stream until it errors or ctx is
// done. Returns nil when ctx was cancelled mid-drain (clean shutdown),
// or the Recv error so LoopWithRetryClass can classify it. Successful
// event delivery does not reset backoff here — that happens implicitly
// when attempt returns and Loop re-invokes it; only a "stream lost"
// error after a healthy run propagates.
func (c *eventsClient) drainStream(
	ctx context.Context,
	stream pb.EventsExtension_SubscribeClient,
	handler EventHandler,
	types []string,
) error {
	for {
		evt, err := stream.Recv()
		if err == nil {
			c.deliver(ctx, evt, handler)
			continue
		}
		if ctx.Err() != nil {
			return nil
		}
		// Surface the error to the loop. The classifier marks
		// PermissionDenied / Unimplemented / InvalidArgument /
		// Unauthenticated as fatal; everything else (Unavailable,
		// DeadlineExceeded, io.EOF, transport blips) is retried.
		if c.logger != nil {
			c.logger.Warn("events stream lost; loop will classify and possibly reconnect",
				"plugin", c.pluginName, "types", types, "error", err)
		}
		return err
	}
}

// deliver wraps the handler call so we can centralise drop-counter
// logging. The host increments dropped_since_last_send on the next
// event after a buffer overflow; surfacing it here lets plugin
// authors correlate dropped events with their handler latency without
// having to wire their own counter.
func (c *eventsClient) deliver(ctx context.Context, evt *HostEvent, handler EventHandler) {
	if evt == nil {
		return
	}
	if dropped := evt.GetDroppedSinceLastSend(); dropped > 0 && c.logger != nil {
		c.logger.Warn("host dropped events before this delivery; consider faster handlers or fewer subscriptions",
			"plugin", c.pluginName,
			"event_type", evt.GetEventType(),
			"dropped", dropped,
		)
	}
	handler(ctx, evt)
}
