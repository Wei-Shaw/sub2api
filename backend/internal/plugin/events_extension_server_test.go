// Package plugin — events_extension_server_test.go
//
// Phase B unit tests for EventPublisher and EventsExtensionServer.
//
// We exercise the in-process gRPC stream by using bufconn so the test can
// drive a real EventsExtensionClient against a real EventsExtensionServer
// without binding a TCP port.
package plugin

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// fakeAllowList is a minimal EventsAllowList for tests.
type fakeAllowList struct{ events map[string][]string }

func (f *fakeAllowList) AllowedEvents(name string) []string {
	out := append([]string(nil), f.events[name]...)
	return out
}

// fakeCaps is a CapabilityChecker stub.
type fakeCaps struct {
	caps map[string]map[string]struct{}
}

func (f *fakeCaps) HasCapability(name, capability string) bool {
	if f == nil || f.caps == nil {
		return false
	}
	set, ok := f.caps[name]
	if !ok {
		return false
	}
	_, has := set[capability]
	return has
}

// staticResolver returns the plugin name from metadata, falling back to
// the configured default.
func staticResolver(name string) func(context.Context) string {
	return func(ctx context.Context) string {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return name
		}
		vals := md.Get(callerMetadataKey)
		if len(vals) > 0 {
			return vals[0]
		}
		return name
	}
}

// testRig wires a bufconn-backed gRPC server with the events service.
type testRig struct {
	publisher *EventPublisher
	client    pb.EventsExtensionClient
	cleanup   func()
}

func startTestServer(t *testing.T, allowList EventsAllowList, caps CapabilityChecker) *testRig {
	t.Helper()
	ln := bufconn.Listen(1 << 20)
	publisher := NewEventPublisher(nil)
	srv := grpc.NewServer()
	NewEventsExtensionServer(publisher, allowList, caps, staticResolver("")).Register(srv)

	go func() { _ = srv.Serve(ln) }()

	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) { return ln.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	cleanup := func() {
		_ = conn.Close()
		srv.Stop()
	}
	return &testRig{
		publisher: publisher,
		client:    pb.NewEventsExtensionClient(conn),
		cleanup:   cleanup,
	}
}

func withPlugin(ctx context.Context, name string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, callerMetadataKey, name)
}

func TestSubscribeAndReceive(t *testing.T) {
	allow := &fakeAllowList{events: map[string][]string{
		"my-plugin": {EventTypePaymentOrderCreated},
	}}
	rig := startTestServer(t, allow, nil)
	defer rig.cleanup()

	ctx, cancel := context.WithTimeout(withPlugin(context.Background(), "my-plugin"), 3*time.Second)
	defer cancel()

	stream, err := rig.client.Subscribe(ctx, &pb.EventSubscribeRequest{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	waitForSubscribers(t, rig.publisher, 1)

	rig.publisher.PublishPaymentOrderCreated(&pb.PaymentOrderCreated{
		OrderId:    42,
		OutTradeNo: "out-42",
		UserId:     7,
	})

	evt, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	if evt.GetEventType() != EventTypePaymentOrderCreated {
		t.Fatalf("unexpected event type: %s", evt.GetEventType())
	}
	if evt.GetPaymentOrderCreated().GetOrderId() != 42 {
		t.Fatalf("unexpected order id: %d", evt.GetPaymentOrderCreated().GetOrderId())
	}
	if evt.GetDroppedSinceLastSend() != 0 {
		t.Fatalf("expected zero drops, got %d", evt.GetDroppedSinceLastSend())
	}
}

func TestUndeclaredEventTypeRejected(t *testing.T) {
	allow := &fakeAllowList{events: map[string][]string{
		"my-plugin": {EventTypePaymentOrderCreated},
	}}
	rig := startTestServer(t, allow, nil)
	defer rig.cleanup()

	ctx := withPlugin(context.Background(), "my-plugin")
	stream, err := rig.client.Subscribe(ctx, &pb.EventSubscribeRequest{
		EventTypes: []string{EventTypeAuthUserRegistered},
	})
	if err != nil {
		t.Fatalf("subscribe call: %v", err)
	}
	_, err = stream.Recv()
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got: %v", err)
	}
}

func TestGatewayCapabilityRequired(t *testing.T) {
	allow := &fakeAllowList{events: map[string][]string{
		"my-plugin": {EventTypeGatewayModelInvoked},
	}}
	caps := &fakeCaps{caps: map[string]map[string]struct{}{
		"my-plugin": {},
	}}
	rig := startTestServer(t, allow, caps)
	defer rig.cleanup()

	ctx := withPlugin(context.Background(), "my-plugin")
	stream, err := rig.client.Subscribe(ctx, &pb.EventSubscribeRequest{
		EventTypes: []string{EventTypeGatewayModelInvoked},
	})
	if err != nil {
		t.Fatalf("subscribe call: %v", err)
	}
	_, err = stream.Recv()
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got: %v", err)
	}
}

func TestGatewayCapabilityGranted(t *testing.T) {
	allow := &fakeAllowList{events: map[string][]string{
		"my-plugin": {EventTypeGatewayModelInvoked},
	}}
	caps := &fakeCaps{caps: map[string]map[string]struct{}{
		"my-plugin": {CapabilityEventsGateway: {}},
	}}
	rig := startTestServer(t, allow, caps)
	defer rig.cleanup()

	ctx, cancel := context.WithTimeout(withPlugin(context.Background(), "my-plugin"), 3*time.Second)
	defer cancel()
	stream, err := rig.client.Subscribe(ctx, &pb.EventSubscribeRequest{
		EventTypes: []string{EventTypeGatewayModelInvoked},
	})
	if err != nil {
		t.Fatalf("subscribe call: %v", err)
	}
	waitForSubscribers(t, rig.publisher, 1)

	rig.publisher.PublishGatewayModelInvoked(&pb.GatewayModelInvoked{
		RequestId: "req-1",
		AccountId: 99,
		Model:     "claude-3",
	})
	evt, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	if evt.GetGatewayModelInvoked().GetAccountId() != 99 {
		t.Fatalf("unexpected account: %d", evt.GetGatewayModelInvoked().GetAccountId())
	}
}

func TestBufferOverflowDropsOldest(t *testing.T) {
	allow := &fakeAllowList{events: map[string][]string{
		"my-plugin": {EventTypePaymentOrderCreated},
	}}
	rig := startTestServer(t, allow, nil)
	defer rig.cleanup()

	ctx, cancel := context.WithTimeout(withPlugin(context.Background(), "my-plugin"), 5*time.Second)
	defer cancel()
	stream, err := rig.client.Subscribe(ctx, &pb.EventSubscribeRequest{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	waitForSubscribers(t, rig.publisher, 1)

	// Burst publish at 100x buffer size in a tight loop. Consumer receives
	// concurrently, but with this many events the buffer is virtually
	// guaranteed to overflow at least once. We then scan up to `total`
	// events for the first non-zero DroppedSinceLastSend marker.
	const total = eventBufferSize * 100
	for i := 0; i < total; i++ {
		rig.publisher.PublishPaymentOrderCreated(&pb.PaymentOrderCreated{OrderId: int64(i)})
	}

	sawDrop := false
	for i := 0; i < total; i++ {
		evt, err := stream.Recv()
		if err != nil {
			t.Fatalf("recv at %d: %v", i, err)
		}
		if evt.GetDroppedSinceLastSend() > 0 {
			sawDrop = true
			break
		}
	}
	if !sawDrop {
		t.Fatalf("expected a non-zero DroppedSinceLastSend after buffer overflow")
	}
}

func TestReSubscribeReplacesPriorStream(t *testing.T) {
	allow := &fakeAllowList{events: map[string][]string{
		"my-plugin": {EventTypePaymentOrderCreated},
	}}
	rig := startTestServer(t, allow, nil)
	defer rig.cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream1, err := rig.client.Subscribe(withPlugin(ctx, "my-plugin"), &pb.EventSubscribeRequest{})
	if err != nil {
		t.Fatalf("first subscribe: %v", err)
	}
	waitForSubscribers(t, rig.publisher, 1)

	stream2, err := rig.client.Subscribe(withPlugin(ctx, "my-plugin"), &pb.EventSubscribeRequest{})
	if err != nil {
		t.Fatalf("second subscribe: %v", err)
	}

	doneFirst := make(chan struct{})
	go func() {
		defer close(doneFirst)
		_, _ = stream1.Recv()
	}()
	select {
	case <-doneFirst:
	case <-time.After(2 * time.Second):
		t.Fatalf("first stream did not close after re-Subscribe")
	}

	waitForSubscribers(t, rig.publisher, 1)
	rig.publisher.PublishPaymentOrderCreated(&pb.PaymentOrderCreated{OrderId: 7})
	evt, err := stream2.Recv()
	if err != nil {
		t.Fatalf("second stream recv: %v", err)
	}
	if evt.GetPaymentOrderCreated().GetOrderId() != 7 {
		t.Fatalf("unexpected order id: %d", evt.GetPaymentOrderCreated().GetOrderId())
	}
}

func TestNoManifestDeclarationDeniesSubscribe(t *testing.T) {
	allow := &fakeAllowList{events: map[string][]string{}}
	rig := startTestServer(t, allow, nil)
	defer rig.cleanup()

	ctx := withPlugin(context.Background(), "my-plugin")
	stream, err := rig.client.Subscribe(ctx, &pb.EventSubscribeRequest{})
	if err != nil {
		t.Fatalf("subscribe call: %v", err)
	}
	_, err = stream.Recv()
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got: %v", err)
	}
}

func TestAnonymousCallerDenied(t *testing.T) {
	allow := &fakeAllowList{}
	rig := startTestServer(t, allow, nil)
	defer rig.cleanup()

	stream, err := rig.client.Subscribe(context.Background(), &pb.EventSubscribeRequest{})
	if err != nil {
		t.Fatalf("subscribe call: %v", err)
	}
	_, err = stream.Recv()
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got: %v", err)
	}
}

func TestPublishWithoutSubscribersIsNoop(t *testing.T) {
	pub := NewEventPublisher(nil)
	pub.PublishPaymentOrderCreated(&pb.PaymentOrderCreated{OrderId: 1})
	pub.PublishPaymentOrderFulfilled(&pb.PaymentOrderFulfilled{OrderId: 1})
	pub.PublishGatewayModelInvoked(&pb.GatewayModelInvoked{RequestId: "x"})
	pub.PublishAuthUserRegistered(&pb.AuthUserRegistered{UserId: 1})
	pub.PublishAccountRateLimitTriggered(&pb.AccountRateLimitTriggered{AccountId: 1})
}

// TestProbeSubscriptionAccepted asserts ProbeSubscription returns OK when
// the manifest declares the requested types and capabilities are satisfied.
// The response active_subscriptions is informational only — we just sanity-
// check it stays a valid int64.
func TestProbeSubscriptionAccepted(t *testing.T) {
	allow := &fakeAllowList{events: map[string][]string{
		"my-plugin": {EventTypePaymentOrderCreated},
	}}
	rig := startTestServer(t, allow, nil)
	defer rig.cleanup()

	ctx, cancel := context.WithTimeout(withPlugin(context.Background(), "my-plugin"), 2*time.Second)
	defer cancel()

	resp, err := rig.client.ProbeSubscription(ctx, &pb.EventSubscribeRequest{
		EventTypes: []string{EventTypePaymentOrderCreated},
	})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if resp == nil {
		t.Fatalf("expected non-nil response")
	}
	if resp.GetActiveSubscriptions() < 0 {
		t.Fatalf("active_subscriptions should be >= 0, got %d", resp.GetActiveSubscriptions())
	}
	// A successful Probe must NOT register a subscriber; if it did, the
	// publisher would carry stale state across plugin restarts.
	rig.publisher.mu.RLock()
	subs := len(rig.publisher.subscribers)
	rig.publisher.mu.RUnlock()
	if subs != 0 {
		t.Fatalf("Probe should not register subscribers, got %d", subs)
	}
}

// TestProbeSubscriptionRejectsUndeclared mirrors TestUndeclaredEventTypeRejected
// but on the synchronous Probe path. The error must be PermissionDenied so
// the SDK surfaces it to the caller without retrying.
func TestProbeSubscriptionRejectsUndeclared(t *testing.T) {
	allow := &fakeAllowList{events: map[string][]string{
		"my-plugin": {EventTypePaymentOrderCreated},
	}}
	rig := startTestServer(t, allow, nil)
	defer rig.cleanup()

	ctx := withPlugin(context.Background(), "my-plugin")
	_, err := rig.client.ProbeSubscription(ctx, &pb.EventSubscribeRequest{
		EventTypes: []string{EventTypeAuthUserRegistered},
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got: %v", err)
	}
}

// TestProbeSubscriptionRejectsAnonymous mirrors TestAnonymousCallerDenied but
// on the unary Probe path. Defence-in-depth: if the caller-identity
// interceptor is missing the handler still rejects.
func TestProbeSubscriptionRejectsAnonymous(t *testing.T) {
	rig := startTestServer(t, &fakeAllowList{}, nil)
	defer rig.cleanup()

	_, err := rig.client.ProbeSubscription(context.Background(), &pb.EventSubscribeRequest{})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got: %v", err)
	}
}

// TestProbeSubscriptionMissingManifestDeclaration covers the third gate:
// caller is identified but the manifest does not declare any events. The
// host must reject before applying capability checks.
func TestProbeSubscriptionMissingManifestDeclaration(t *testing.T) {
	allow := &fakeAllowList{events: map[string][]string{}}
	rig := startTestServer(t, allow, nil)
	defer rig.cleanup()

	ctx := withPlugin(context.Background(), "my-plugin")
	_, err := rig.client.ProbeSubscription(ctx, &pb.EventSubscribeRequest{})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got: %v", err)
	}
}

// TestProbeSubscriptionEnforcesCapability covers the high-frequency gate.
// gateway.model.invoked requires events.subscribe.gateway; without it the
// Probe must fail synchronously, matching the server-streaming Subscribe
// contract.
func TestProbeSubscriptionEnforcesCapability(t *testing.T) {
	allow := &fakeAllowList{events: map[string][]string{
		"my-plugin": {EventTypeGatewayModelInvoked},
	}}
	caps := &fakeCaps{caps: map[string]map[string]struct{}{
		"my-plugin": {},
	}}
	rig := startTestServer(t, allow, caps)
	defer rig.cleanup()

	ctx := withPlugin(context.Background(), "my-plugin")
	_, err := rig.client.ProbeSubscription(ctx, &pb.EventSubscribeRequest{
		EventTypes: []string{EventTypeGatewayModelInvoked},
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got: %v", err)
	}
}

// waitForSubscribers polls until the publisher reports the expected
// subscriber count, bounded by a deadline.
func waitForSubscribers(t *testing.T, pub *EventPublisher, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		pub.mu.RLock()
		got := len(pub.subscribers)
		pub.mu.RUnlock()
		if got == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("subscriber count did not reach %d", want)
}

// stallStream is a minimal grpc.ServerStreamingServer[pb.HostEvent] stub
// whose Send hangs until the test cancels its context. We use it to drive
// the 2s send timeout path in pump without a full bufconn rig.
type stallStream struct {
	grpc.ServerStream
	ctx context.Context

	// sendHits 用 atomic.Int64 自身就线程安全, 不需要额外 mu;
	// 历史曾留 sync.Mutex 但从未读/写 → 删除以消 unused 警告。
	sendHits atomic.Int64
}

func (s *stallStream) Context() context.Context { return s.ctx }

func (s *stallStream) Send(*pb.HostEvent) error {
	s.sendHits.Add(1)
	<-s.ctx.Done()
	return s.ctx.Err()
}

func TestSendTimeoutClosesStream(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping >2s timeout test in -short mode")
	}
	publisher := NewEventPublisher(nil)
	srv := NewEventsExtensionServer(publisher, &fakeAllowList{}, nil, staticResolver(""))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	timer := time.NewTimer(eventSendTimeout)
	if !timer.Stop() {
		<-timer.C
	}
	sub := &eventSubscriber{
		pluginName: "test",
		buffer:     make(chan *pb.HostEvent, eventBufferSize),
		closed:     make(chan struct{}),
		sendTimer:  timer,
		sendDone:   make(chan error, 1),
	}
	publisher.register(sub)
	defer publisher.unregister(sub)

	pumpDone := make(chan error, 1)
	stream := &stallStream{ctx: ctx}
	go func() {
		pumpDone <- srv.pump(ctx, stream, sub)
	}()

	sub.buffer <- &pb.HostEvent{EventType: EventTypePaymentOrderCreated}

	select {
	case err := <-pumpDone:
		if err == nil {
			t.Fatalf("expected timeout error, got nil")
		}
	case <-time.After(eventSendTimeout + 2*time.Second):
		t.Fatalf("pump did not exit after send timeout")
	}
}
