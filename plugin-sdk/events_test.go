// Package pluginsdk — events_test.go
//
// T25 added these tests when Subscribe was rewritten around the new
// ProbeSubscription unary RPC (see sdk.proto changelog). The cases focus on
// the SDK guarantees the refactor must preserve:
//
//   - Probe failures surface synchronously to the caller and DO NOT spin up
//     the background loop goroutine (resource cleanup invariant).
//   - Probe successes hand control to streamutil.Loop, which keeps reconnecting
//     on transient stream failures until ctx is cancelled.
//   - Subscribe input validation (nil ctx / nil handler / empty types) still
//     short-circuits before the RPC layer.
//
// The tests run an in-process bufconn gRPC server with a stub
// EventsExtensionServer so we can vary Probe / Subscribe behaviour without
// dragging the host's real publisher into plugin-sdk.
package pluginsdk

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// fakeEventsServer is an in-process EventsExtensionServer the tests drive.
// probeErr / streamErr / events let each test pick the behaviour it cares
// about; counters expose how many times each RPC was invoked so the tests
// can assert "Subscribe never opened a stream" after a Probe rejection.
type fakeEventsServer struct {
	pb.UnimplementedEventsExtensionServer

	probeErr error
	// streamErr is returned from Subscribe BEFORE any event is sent. Used to
	// simulate a transient stream-open failure that the SDK should retry.
	streamErr error
	// events lists events to push to the subscriber after Subscribe accepts.
	// Tests can leave it empty when only the lifecycle matters.
	events []*pb.HostEvent

	probeCalls     atomic.Int64
	subscribeCalls atomic.Int64

	// streamReady is signalled when Subscribe has pushed all configured
	// events; tests wait on it before asserting receive.
	streamReady chan struct{}
	once        sync.Once
}

func (s *fakeEventsServer) ProbeSubscription(
	ctx context.Context, req *pb.EventSubscribeRequest,
) (*pb.ProbeSubscriptionResponse, error) {
	s.probeCalls.Add(1)
	if s.probeErr != nil {
		return nil, s.probeErr
	}
	return &pb.ProbeSubscriptionResponse{ActiveSubscriptions: 0}, nil
}

func (s *fakeEventsServer) Subscribe(
	req *pb.EventSubscribeRequest, stream grpc.ServerStreamingServer[pb.HostEvent],
) error {
	s.subscribeCalls.Add(1)
	if s.streamErr != nil {
		return s.streamErr
	}
	for _, evt := range s.events {
		if err := stream.Send(evt); err != nil {
			return err
		}
	}
	s.once.Do(func() {
		if s.streamReady != nil {
			close(s.streamReady)
		}
	})
	// Block until ctx is cancelled so the SDK's drainStream can observe a
	// "ctx done" exit (mirrors a graceful shutdown).
	<-stream.Context().Done()
	return stream.Context().Err()
}

// startFakeEventsServer spins up the in-process gRPC server and returns a
// connected EventsExtensionClient.
func startFakeEventsServer(t *testing.T, fake *fakeEventsServer) (pb.EventsExtensionClient, func()) {
	t.Helper()
	ln := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	pb.RegisterEventsExtensionServer(srv, fake)
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
	return pb.NewEventsExtensionClient(conn), cleanup
}

// TestSubscribeProbeFailureSurfacesSynchronously asserts that a Probe error
// is returned from Subscribe directly and that no streamutil.Loop goroutine
// is spawned to retry the streaming RPC. The "no streaming call" check is
// the load-bearing invariant: the whole point of moving validation into
// Probe is that mis-configured plugins do not chew CPU on a doomed reconnect
// loop.
//
// We assert "no streaming call" rather than counting goroutines because
// the gRPC client maintains its own background workers whose count can
// drift between runs; subscribeCalls is the only signal that directly
// reflects whether the SDK started the long-lived loop.
func TestSubscribeProbeFailureSurfacesSynchronously(t *testing.T) {
	fake := &fakeEventsServer{
		probeErr: status.Error(codes.PermissionDenied, "missing capability"),
	}
	cli, cleanup := startFakeEventsServer(t, fake)
	defer cleanup()

	c := newEventsClient(cli, "test-plugin", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := c.Subscribe(ctx, []string{EventTypePaymentOrderCreated},
		func(context.Context, *HostEvent) {})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got: %v", err)
	}
	if fake.probeCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 probe call, got %d", fake.probeCalls.Load())
	}
	// Subscribe should have NEVER opened the streaming RPC because Probe
	// already rejected the request. Allow a brief pause so any erroneous
	// goroutine has time to wake up and call into the fake.
	time.Sleep(50 * time.Millisecond)
	if fake.subscribeCalls.Load() != 0 {
		t.Fatalf("Subscribe stream should not be opened after Probe failure; calls=%d",
			fake.subscribeCalls.Load())
	}
}

// TestSubscribeProbeRejectsInvalidArgument mirrors the PermissionDenied case
// for InvalidArgument. Same invariant: synchronous return, no stream opened.
func TestSubscribeProbeRejectsInvalidArgument(t *testing.T) {
	fake := &fakeEventsServer{
		probeErr: status.Error(codes.InvalidArgument, "bad type"),
	}
	cli, cleanup := startFakeEventsServer(t, fake)
	defer cleanup()

	c := newEventsClient(cli, "test-plugin", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := c.Subscribe(ctx, []string{EventTypePaymentOrderCreated},
		func(context.Context, *HostEvent) {})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got: %v", err)
	}
	if fake.subscribeCalls.Load() != 0 {
		t.Fatalf("Subscribe stream should not be opened after Probe failure")
	}
}

// TestSubscribeProbeSuccessOpensStream asserts the happy path: Probe accepts,
// Subscribe runs, events flow to the handler. The point of this case is to
// catch regressions where the refactor accidentally drops the streaming
// goroutine after a successful Probe.
func TestSubscribeProbeSuccessOpensStream(t *testing.T) {
	streamReady := make(chan struct{})
	fake := &fakeEventsServer{
		events: []*pb.HostEvent{
			{
				EventType: EventTypePaymentOrderCreated,
				Payload: &pb.HostEvent_PaymentOrderCreated{
					PaymentOrderCreated: &pb.PaymentOrderCreated{OrderId: 99},
				},
			},
		},
		streamReady: streamReady,
	}
	cli, cleanup := startFakeEventsServer(t, fake)
	defer cleanup()

	c := newEventsClient(cli, "test-plugin", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	delivered := make(chan *HostEvent, 1)
	err := c.Subscribe(ctx, []string{EventTypePaymentOrderCreated},
		func(_ context.Context, evt *HostEvent) {
			select {
			case delivered <- evt:
			default:
			}
		})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	select {
	case evt := <-delivered:
		if evt.GetPaymentOrderCreated().GetOrderId() != 99 {
			t.Fatalf("unexpected order id: %d", evt.GetPaymentOrderCreated().GetOrderId())
		}
	case <-ctx.Done():
		t.Fatalf("did not receive event before deadline")
	}
	if fake.probeCalls.Load() == 0 {
		t.Fatalf("expected Probe to be called at least once")
	}
	if fake.subscribeCalls.Load() == 0 {
		t.Fatalf("expected Subscribe stream to be opened after Probe success")
	}
}

// TestSubscribeInputValidation verifies the early-return checks that exist
// independently of the RPC layer. These remain because they catch developer
// errors before the network call.
func TestSubscribeInputValidation(t *testing.T) {
	fake := &fakeEventsServer{}
	cli, cleanup := startFakeEventsServer(t, fake)
	defer cleanup()

	c := newEventsClient(cli, "test-plugin", nil)
	ctx := context.Background()
	handler := func(context.Context, *HostEvent) {}

	if err := c.Subscribe(ctx, []string{EventTypePaymentOrderCreated}, nil); err == nil {
		t.Fatalf("expected error for nil handler")
	}
	if err := c.Subscribe(ctx, nil, handler); err == nil {
		t.Fatalf("expected error for empty event types")
	}
	//nolint:staticcheck // intentionally passing nil to assert the guard
	if err := c.Subscribe(nil, []string{EventTypePaymentOrderCreated}, handler); err == nil {
		t.Fatalf("expected error for nil ctx")
	}
	// None of those should ever reach the server.
	if fake.probeCalls.Load() != 0 || fake.subscribeCalls.Load() != 0 {
		t.Fatalf("validation guards should short-circuit before any RPC")
	}
}

// TestSubscribeReturnsErrorWhenServerLacksProbe asserts that a host that
// returns Unimplemented for Probe is treated like any other non-retryable
// error: the SDK reports it to the caller. This is the "old plugin against
// a new SDK" cliff edge — the SDK requires Probe support and intentionally
// does NOT silently fall back to the streaming RPC.
//
// 兼容性说明：plugin / host 必须配套部署。SDK 不再尝试在 Probe 缺失时回退
// 到 first-Recv 模式，因为那条退路正是本任务要消除的抽象泄漏。
func TestSubscribeReturnsErrorWhenServerLacksProbe(t *testing.T) {
	fake := &fakeEventsServer{
		probeErr: status.Error(codes.Unimplemented, "ProbeSubscription not implemented"),
	}
	cli, cleanup := startFakeEventsServer(t, fake)
	defer cleanup()

	c := newEventsClient(cli, "test-plugin", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := c.Subscribe(ctx, []string{EventTypePaymentOrderCreated},
		func(context.Context, *HostEvent) {})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("expected Unimplemented, got: %v", err)
	}
	if fake.subscribeCalls.Load() != 0 {
		t.Fatalf("stream must not be opened when Probe is missing")
	}
}

// TestNilEventsClientReportsMissingHost is a sanity check on the fallback
// implementation used by hosts that do not register EventsExtension.
func TestNilEventsClientReportsMissingHost(t *testing.T) {
	c := nilEventsClient{}
	err := c.Subscribe(context.Background(), []string{EventTypePaymentOrderCreated},
		func(context.Context, *HostEvent) {})
	if err == nil || !errorsContains(err, "EventsExtension not available") {
		t.Fatalf("expected nilEventsClient to surface 'not available' error, got: %v", err)
	}
}

// errorsContains reports whether err's message contains substr. Local helper
// because errors.Is doesn't apply to the bare errors.New value here.
func errorsContains(err error, substr string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for i := 0; i+len(substr) <= len(msg); i++ {
		if msg[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// _ keeps errors imported even if some test bodies are removed/rearranged
// during future maintenance — the helper above already uses it indirectly
// via .Error() but we want a static reference.
var _ = errors.New
