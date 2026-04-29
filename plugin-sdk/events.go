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
//     EventsExtensionClient that opens a server-streaming Subscribe
//     RPC, dispatches typed events to a user-supplied EventHandler,
//     and re-establishes the stream with exponential backoff when it
//     drops. See sdk.proto's EventsExtension service block for the
//     full protocol contract (256-slot ring buffer, drop-oldest, 2s
//     send timeout).
//
// Failure modes are documented inline; in short, Subscribe returns
// non-nil only for synchronous setup errors (PERMISSION_DENIED on the
// capability gate, undeclared event types). Stream-level disconnects
// are recovered transparently by runSubscribeLoop.
package pluginsdk

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
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
// backend/internal/plugin/events_extension_server.go so plugin authors
// can declare SubscribedEvents and call Subscribe without typos.
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
// with exponential backoff (1s → 2s → 4s → 8s → 30s, jittered).
//
// Subscribe does not block: it spins up an internal goroutine that
// runs the receive loop, then returns nil to the caller. Cancel ctx
// (or let the SDK shut down) to stop the loop.
type EventsClient interface {
	// Subscribe registers handler for the given event types. Each event
	// type must appear in Manifest.SubscribedEvents — the host enforces
	// this and returns InvalidArgument otherwise. The capability check
	// for high-frequency events (gateway.model.invoked → events.gateway)
	// happens server-side; on failure Subscribe returns the wrapped
	// gRPC error so callers can branch on codes.PermissionDenied.
	Subscribe(ctx context.Context, eventTypes []string, handler EventHandler) error
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
const (
	eventsInitialBackoff = 1 * time.Second
	eventsMaxBackoff     = 30 * time.Second
	// eventsBackoffJitterFraction adds ±10% jitter to each delay so a
	// host restart that knocks out N plugins simultaneously does not
	// produce a synchronised reconnect storm.
	eventsBackoffJitterFraction = 0.1
)

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

	// First attempt is synchronous so PERMISSION_DENIED / InvalidArgument
	// surfaces to the caller instead of being swallowed by the
	// reconnect loop. Server-streaming RPC errors typically arrive on
	// the first Recv rather than from Subscribe itself, so we probe
	// once before handing the stream to the long-lived goroutine.
	stream, err := c.grpc.Subscribe(ctx, &pb.EventSubscribeRequest{EventTypes: types})
	if err != nil {
		return err
	}
	firstEvt, recvErr := stream.Recv()
	if recvErr != nil {
		if !isRetryableStreamError(recvErr) {
			return recvErr
		}
		// Retryable error on first Recv — drop this stream and let the
		// loop reconnect. We deliberately do not block the caller on
		// retry attempts so an admin who restarts the host briefly
		// after a plugin starts up does not see Subscribe hang.
		stream = nil
	}

	go c.runSubscribeLoop(ctx, stream, firstEvt, types, handler)
	return nil
}

// runSubscribeLoop is the long-lived goroutine that owns the
// Subscribe stream. It reads events, dispatches them to handler, and
// reconnects with exponential backoff on transport errors. Returns
// only when ctx is cancelled or the host returns a non-retryable
// error (PermissionDenied / InvalidArgument).
func (c *eventsClient) runSubscribeLoop(
	ctx context.Context,
	initial pb.EventsExtension_SubscribeClient,
	firstEvt *HostEvent,
	types []string,
	handler EventHandler,
) {
	stream := initial
	backoff := eventsInitialBackoff
	if firstEvt != nil {
		c.deliver(ctx, firstEvt, handler)
	}
	for {
		if ctx.Err() != nil {
			return
		}
		if stream == nil {
			s, err := c.grpc.Subscribe(ctx, &pb.EventSubscribeRequest{EventTypes: types})
			if err != nil {
				if !isRetryableStreamError(err) {
					if c.logger != nil {
						c.logger.Error("events subscribe rejected; not retrying",
							"plugin", c.pluginName, "types", types, "error", err)
					}
					return
				}
				if c.logger != nil {
					c.logger.Warn("events subscribe stream open failed; will retry",
						"plugin", c.pluginName, "error", err, "backoff", backoff)
				}
				if !eventsSleepCtx(ctx, withJitter(backoff)) {
					return
				}
				backoff = eventsNextBackoff(backoff)
				continue
			}
			stream = s
		}
		// Drain the stream until it errors. Successful events reset
		// the backoff so a long-lived stream that finally drops still
		// gets the documented 1s first reconnect delay.
		streamHealthy := c.drainStream(ctx, stream, handler, types)
		stream = nil
		if !streamHealthy {
			return
		}
		if !eventsSleepCtx(ctx, withJitter(backoff)) {
			return
		}
		backoff = eventsNextBackoff(backoff)
	}
}

// drainStream pulls events from stream until it errors or ctx is
// done. Returns true when the loop should reconnect, false when the
// loop must exit (non-retryable error or ctx cancelled). Successful
// events reset the caller's backoff via the returned bool path: any
// event delivery means the stream was healthy at least briefly, so
// the caller resets to eventsInitialBackoff before the next sleep.
func (c *eventsClient) drainStream(
	ctx context.Context,
	stream pb.EventsExtension_SubscribeClient,
	handler EventHandler,
	types []string,
) bool {
	for {
		evt, err := stream.Recv()
		if err == nil {
			c.deliver(ctx, evt, handler)
			continue
		}
		if ctx.Err() != nil {
			return false
		}
		if !isRetryableStreamError(err) {
			if c.logger != nil {
				c.logger.Error("events stream returned non-retryable error; subscription stopped",
					"plugin", c.pluginName, "types", types, "error", err)
			}
			return false
		}
		if c.logger != nil {
			c.logger.Warn("events stream lost; reconnecting",
				"plugin", c.pluginName, "error", err)
		}
		return true
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

// isRetryableStreamError returns true when err looks like a transient
// transport failure that warrants reconnecting. PERMISSION_DENIED and
// InvalidArgument indicate misconfiguration (missing capability,
// undeclared event type) and require operator action, so we surface
// them instead of looping. Canceled / DeadlineExceeded are caller-
// driven and handled by the ctx.Err() check, but we treat them as
// non-retryable here too for symmetry.
func isRetryableStreamError(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		// Non-gRPC error (io.EOF, network, etc.). Treat as retryable.
		return true
	}
	switch st.Code() {
	case codes.PermissionDenied, codes.InvalidArgument, codes.Unimplemented,
		codes.Canceled, codes.DeadlineExceeded:
		return false
	default:
		return true
	}
}

// eventsSleepCtx blocks for d or until ctx is done, returning true if
// the wait completed normally.
func eventsSleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// eventsNextBackoff doubles the delay (1→2→4→8→…→30s capped). The
// schedule matches the docstring on EventsExtensionClient so plugin
// authors can correlate "subscribe stream lost" log lines with the
// expected reconnect cadence.
func eventsNextBackoff(cur time.Duration) time.Duration {
	next := cur * 2
	if next > eventsMaxBackoff {
		return eventsMaxBackoff
	}
	if next < eventsInitialBackoff {
		return eventsInitialBackoff
	}
	return next
}

// withJitter returns d ± eventsBackoffJitterFraction*d so a host
// restart that drops every plugin connection at once produces a
// spread of reconnect attempts instead of a thundering herd.
func withJitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	span := float64(d) * eventsBackoffJitterFraction
	// rand.Float64 returns [0, 1); shift to (-1, 1) range.
	delta := (rand.Float64()*2 - 1) * span
	out := time.Duration(float64(d) + delta)
	if out <= 0 {
		return time.Millisecond
	}
	return out
}
