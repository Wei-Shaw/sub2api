// Package plugin — events_extension_server.go
//
// Phase B — host EventsExtension gRPC server. The in-process publisher,
// per-subscriber state, typed Publish helpers, and shared event-type /
// capability constants live in events_publisher.go (split during T25 so
// this file stays under the 600-line wrapper budget while keeping the same
// package and unexported types).
//
// Wire contract — documented in plugin-sdk/proto/sdk.proto's
// EventsExtension service block:
//
//   - at-most-once, no replay, no persistence.
//   - per-subscriber buffer: 256 events; full buffer drops the oldest entry
//     and bumps dropped_since_last_send on the next successful send.
//   - send timeout: 2s. Timeout or send error closes the stream; the SDK
//     reconnects with backoff via streamutil.Loop.
//   - high-frequency events (currently only gateway.model.invoked) require
//     the events.subscribe.gateway capability — Subscribe / Probe reject with
//     PERMISSION_DENIED otherwise.
//   - manifest.SubscribedEvents acts as an allow-list: a plugin cannot
//     subscribe to a type it did not declare up front. Empty event_types in
//     the request expands to "all declared types".
//
// T25 added ProbeSubscription as a synchronous precondition check so the
// SDK no longer needs to peel off the first stream.Recv just to surface
// permission / argument errors. Subscribe still re-runs the gates so older
// SDKs (without Probe) keep working unchanged.
package plugin

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pluginsdkroot "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

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
//
// Caller identity 现在由 RequirePluginIdentityStream 拦截器统一校验并塞入
// stream.Context(), handler 通过 CallerFromContext 取出。本结构不再持有
// resolver 字段; 旧测试若直接调 server method, CallerFromContext 会从
// metadata 兜底解析。
type EventsExtensionServer struct {
	pb.UnimplementedEventsExtensionServer

	publisher    *EventPublisher
	allowList    EventsAllowList
	capabilities CapabilityChecker
	logger       *slog.Logger
}

// NewEventsExtensionServer constructs the gRPC service. resolver 参数保留
// 是为了让 wire_gen.go 生成的调用点继续可编译; 传入的函数已被拦截器接管,
// 此处只是丢弃。新接入方传 nil 即可。
func NewEventsExtensionServer(
	publisher *EventPublisher,
	allowList EventsAllowList,
	capabilities CapabilityChecker,
	_ func(ctx context.Context) string,
) *EventsExtensionServer {
	return &EventsExtensionServer{
		publisher:    publisher,
		allowList:    allowList,
		capabilities: capabilities,
		logger:       slog.Default().With("component", "plugin_events_grpc"),
	}
}

// Register attaches the server to the supplied gRPC server.
func (s *EventsExtensionServer) Register(grpcServer *grpc.Server) {
	pb.RegisterEventsExtensionServer(grpcServer, s)
}

// Publish implements pb.EventsExtensionServer.Publish. Plugins use this
// to emit their own HostEvents (e.g. payment.* from a payment plugin)
// over the same fan-out machinery the host's typed helpers use.
//
// Authorization: the caller must hold the publish capability matching
// the event-type prefix (currently only events.publish.payment for
// payment.*). Anonymous callers are rejected up-front. Capability gates
// keep one plugin from injecting fake events into another's stream.
//
// Stamping: missing event_id is replaced with a fresh UUID v4 so
// downstream consumers can dedupe; missing timestamp_nanos is set to
// the host wall clock at publish time.
func (s *EventsExtensionServer) Publish(
	ctx context.Context, req *pb.PublishEventRequest,
) (*pb.PublishEventResponse, error) {
	pluginName, ok := CallerFromContext(ctx)
	if !ok || pluginName == "" {
		return nil, status.Error(codes.PermissionDenied,
			"events: caller identity missing")
	}
	event := req.GetEvent()
	if event == nil {
		return nil, status.Error(codes.InvalidArgument, "events: event payload is required")
	}
	eventType := strings.TrimSpace(event.GetEventType())
	if eventType == "" {
		return nil, status.Error(codes.InvalidArgument, "events: event_type is required")
	}
	if err := s.checkPublishCapability(pluginName, eventType); err != nil {
		return nil, err
	}
	if event.GetEventId() == "" {
		event.EventId = uuid.NewString()
	}
	if event.GetTimestampNanos() == 0 {
		event.TimestampNanos = time.Now().UnixNano()
	}
	delivered := s.publisher.PublishGeneric(event)
	return &pb.PublishEventResponse{DeliveredTo: delivered}, nil
}

// publishCapabilityForType maps the leading dotted-prefix of an event
// type to the manifest capability required to publish it. Unrecognised
// prefixes reject with PermissionDenied — adding a new namespace
// requires both a registry entry and a row here.
func publishCapabilityForType(eventType string) (string, bool) {
	prefix := eventType
	if i := strings.IndexByte(eventType, '.'); i > 0 {
		prefix = eventType[:i]
	}
	switch prefix {
	case "payment":
		return pluginsdkroot.CapabilityEventsPublishPayment, true
	default:
		return "", false
	}
}

// checkPublishCapability gates Publish against the event-type's
// required capability.
func (s *EventsExtensionServer) checkPublishCapability(pluginName, eventType string) error {
	capName, known := publishCapabilityForType(eventType)
	if !known {
		return status.Errorf(codes.PermissionDenied,
			"events: publishing %q is not supported", eventType)
	}
	if s.capabilities == nil {
		return status.Error(codes.PermissionDenied,
			"events: capability checker not configured")
	}
	if !s.capabilities.HasCapability(pluginName, capName) {
		return status.Errorf(codes.PermissionDenied,
			"events: publishing %q requires capability %q", eventType, capName)
	}
	return nil
}

// ProbeSubscription implements pb.EventsExtensionServer.ProbeSubscription.
//
// Probe is a pure precondition check the SDK runs synchronously before
// opening the long-lived Subscribe stream. It runs the same gates as
// Subscribe but does NOT register a subscriber, so:
//   - PermissionDenied / InvalidArgument surface immediately to the caller
//     (no goroutine spin-up, no streamutil.Loop swallowing the error).
//   - Successful Probe is purely informational; the caller still has to
//     open Subscribe to start receiving events. The reverse channel — Probe
//     OK then Subscribe denied — is theoretically possible if the manifest
//     mutates between the two calls, but the streaming RPC re-runs every
//     check so the worst case is that streamutil.Loop sees the denial and
//     stops retrying (matches isRetryableStreamError on the SDK side).
//
// 同步校验权限与参数，不真正建流；为 SDK 删除 "first-Recv 探活" 抽象泄漏
// 而新增的 unary RPC（T25）。
func (s *EventsExtensionServer) ProbeSubscription(
	ctx context.Context, req *pb.EventSubscribeRequest,
) (*pb.ProbeSubscriptionResponse, error) {
	if _, err := s.authorise(ctx, req); err != nil {
		return nil, err
	}
	return &pb.ProbeSubscriptionResponse{
		ActiveSubscriptions: int64(s.publisher.subscriberCount()),
	}, nil
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
//
// authorise() centralises steps 1-3 so ProbeSubscription can run the exact
// same gates without duplicating the validation logic.
func (s *EventsExtensionServer) Subscribe(
	req *pb.EventSubscribeRequest, stream grpc.ServerStreamingServer[pb.HostEvent],
) error {
	ctx := stream.Context()
	authResult, err := s.authorise(ctx, req)
	if err != nil {
		return err
	}
	pluginName := authResult.pluginName
	requested := authResult.requested

	// sendTimer 创建后立即 stop 并清空 chan，等待第一次 Reset 时重新启动。
	timer := time.NewTimer(eventSendTimeout)
	if !timer.Stop() {
		<-timer.C
	}
	sub := &eventSubscriber{
		pluginName:   pluginName,
		eventTypes:   requested,
		capabilities: s.collectCaps(pluginName),
		buffer:       make(chan *pb.HostEvent, eventBufferSize),
		closed:       make(chan struct{}),
		sendTimer:    timer,
		sendDone:     make(chan error, 1),
	}
	s.publisher.register(sub)
	defer s.publisher.unregister(sub)
	defer sub.close()

	return s.pump(ctx, stream, sub)
}

// authoriseResult bundles the outputs of authorise so Subscribe can reuse
// them without re-running the resolver / allow-list / capability gates.
//
// authorise 的复用产物：把 caller 名 + 已解析的事件类型集合一起回传给
// Subscribe，避免 Probe / Subscribe 两条路径写两份相同的校验代码。
type authoriseResult struct {
	pluginName string
	requested  map[string]struct{}
}

// authorise runs the caller-identity / manifest / capability gates that
// both Subscribe and ProbeSubscription share. Returning an error short-
// circuits the caller with the appropriate gRPC status; on success the
// caller proceeds to its own follow-on (register subscriber for Subscribe,
// build the response for Probe).
//
// 复用 (1)：避免 Subscribe 和 ProbeSubscription 各自写一遍三段式校验，
// 任何一段调整都只改这一个函数。
func (s *EventsExtensionServer) authorise(
	ctx context.Context, req *pb.EventSubscribeRequest,
) (authoriseResult, error) {
	pluginName, ok := CallerFromContext(ctx)
	if !ok || pluginName == "" {
		// Defence-in-depth: RequirePluginIdentityStream / Unary 拦截器已经在
		// 生产链路上把匿名调用挡掉。能进到这里的只有未装拦截器的场景
		// (老测试或直接调用), 也必须拒绝以避免任何身份未知的订阅者。
		return authoriseResult{}, status.Error(codes.PermissionDenied,
			"events: caller identity missing")
	}

	declared := s.declaredEvents(pluginName)
	if len(declared) == 0 {
		return authoriseResult{}, status.Error(codes.PermissionDenied,
			"events: plugin did not declare any SubscribedEvents in its manifest")
	}

	requested, err := s.resolveRequestedTypes(req.GetEventTypes(), declared)
	if err != nil {
		return authoriseResult{}, err
	}

	// Capability gate. We check after manifest validation so the error
	// message does not leak unrelated info.
	if err := s.checkCapabilities(pluginName, requested); err != nil {
		return authoriseResult{}, err
	}
	return authoriseResult{pluginName: pluginName, requested: requested}, nil
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
			if err := s.sendWithTimeout(ctx, stream, sub, event); err != nil {
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
// goroutine and racing it against the subscriber's reusable timer.
// Returns context.DeadlineExceeded on timeout and the underlying send
// error otherwise.
//
// gRPC server-streaming Send does not accept a context, so这是在不动底层
// 内部状态的前提下能给慢客户端设上限的唯一办法。返回超时后 pump 立即退出
// 并触发 defer 链：register 注销 + sub.close + stream cancel；底层 Send
// 在 stream context 被 cancel 后会随之返回，发送 goroutine 就不会无界
// 滞留——最长寿命 = 单次 Send 自行解除阻塞或者 stream 被 gRPC 销毁。
//
// 复用 sub.sendTimer / sub.sendDone：每条事件不再分配新 chan / timer。
func (s *EventsExtensionServer) sendWithTimeout(
	ctx context.Context,
	stream grpc.ServerStreamingServer[pb.HostEvent],
	sub *eventSubscriber,
	event *pb.HostEvent,
) error {
	// drain 上一轮可能残留的结果(成功路径下 chan 已被 select 拿走，超时路径
	// 下另一 goroutine 会在自己的 Send 返回时再尝试写入；这里在 reuse 前先
	// 清掉以保证当前一次 send 的语义干净)。
	select {
	case <-sub.sendDone:
	default:
	}
	go func() {
		// chan 容量为 1；若上一次超时遗留的 goroutine 已经把结果写入，这里
		// 用 non-blocking send 避免再阻塞。
		select {
		case sub.sendDone <- stream.Send(event):
		default:
		}
	}()
	sub.sendTimer.Reset(eventSendTimeout)
	select {
	case err := <-sub.sendDone:
		// 成功/失败都要把 timer 关掉，下一次 Reset 才能从干净状态开始。
		if !sub.sendTimer.Stop() {
			select {
			case <-sub.sendTimer.C:
			default:
			}
		}
		return err
	case <-sub.sendTimer.C:
		return context.DeadlineExceeded
	case <-ctx.Done():
		// stream 已被取消(例如 plugin 断连)；停掉 timer 避免后续 Reset 误读。
		if !sub.sendTimer.Stop() {
			select {
			case <-sub.sendTimer.C:
			default:
			}
		}
		return ctx.Err()
	}
}
