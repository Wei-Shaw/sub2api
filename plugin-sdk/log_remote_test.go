package pluginsdk

import (
	"context"
	"errors"
	"log/slog"
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

// fakeLogProxyServer captures every record pushed by the SDK so tests can
// assert structured field round-tripping.
type fakeLogProxyServer struct {
	pb.UnimplementedLogProxyServer
	mu       sync.Mutex
	records  []*pb.LogRecord
	sendErr  error
	gotEOF   chan struct{}
	onRecord func(*pb.LogRecord)
}

func newFakeLogProxyServer() *fakeLogProxyServer {
	return &fakeLogProxyServer{gotEOF: make(chan struct{}, 1)}
}

func (s *fakeLogProxyServer) PushLogs(stream pb.LogProxy_PushLogsServer) error {
	for {
		rec, err := stream.Recv()
		if err != nil {
			select {
			case s.gotEOF <- struct{}{}:
			default:
			}
			if errors.Is(err, context.Canceled) {
				return nil
			}
			// EOF is the normal end-of-stream from a client CloseSend.
			return stream.SendAndClose(&pb.LogPushSummary{Received: uint64(len(s.records))})
		}
		s.mu.Lock()
		s.records = append(s.records, rec)
		if s.onRecord != nil {
			s.onRecord(rec)
		}
		s.mu.Unlock()
		if s.sendErr != nil {
			return s.sendErr
		}
	}
}

func (s *fakeLogProxyServer) snapshot() []*pb.LogRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*pb.LogRecord, len(s.records))
	copy(out, s.records)
	return out
}

// startBufconnServer wires a fake LogProxy onto an in-memory bufconn so tests
// avoid a real TCP socket.
func startBufconnServer(t *testing.T, fake *fakeLogProxyServer) (pb.LogProxyClient, func()) {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	pb.RegisterLogProxyServer(srv, fake)
	go func() {
		_ = srv.Serve(lis)
	}()
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		srv.Stop()
		_ = lis.Close()
		t.Fatalf("dial bufconn: %v", err)
	}
	cleanup := func() {
		_ = conn.Close()
		srv.Stop()
		_ = lis.Close()
	}
	return pb.NewLogProxyClient(conn), cleanup
}

func TestRemoteSlogHandler_RoundTripAttrKinds(t *testing.T) {
	fake := newFakeLogProxyServer()
	client, cleanup := startBufconnServer(t, fake)
	defer cleanup()

	h := newRemoteSlogHandler(slog.LevelDebug)
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		defer close(done)
		runRemoteSlogSender(ctx, client, h, nil)
	}()

	logger := slog.New(h)
	now := time.Date(2026, 4, 27, 10, 30, 0, 0, time.UTC)
	logger.LogAttrs(ctx, slog.LevelInfo, "structured-test",
		slog.String("trace_id", "abc"),
		slog.Int64("count", 42),
		slog.Float64("ratio", 0.75),
		slog.Bool("ok", true),
		slog.Time("happened_at", now),
		slog.Duration("elapsed", 250*time.Millisecond),
		slog.Group("user", slog.Int64("id", 7), slog.String("role", "admin")),
	)

	// Allow async send.
	waitFor(t, func() bool {
		return len(fake.snapshot()) > 0
	}, time.Second)

	h.Close()
	<-done
	cancel()

	records := fake.snapshot()
	if len(records) == 0 {
		t.Fatalf("no record captured")
	}
	rec := records[0]
	if rec.Msg != "structured-test" || rec.Level != int32(slog.LevelInfo) {
		t.Fatalf("level/msg wrong: %+v", rec)
	}
	got := map[string]*pb.LogAttr{}
	for _, a := range rec.Attrs {
		got[a.Key] = a
	}
	mustString(t, got, "trace_id", "abc")
	mustInt(t, got, "count", 42)
	mustFloat(t, got, "ratio", 0.75)
	mustBool(t, got, "ok", true)
	if g := got["happened_at"]; g == nil || g.GetTimeUnixNano() != uint64(now.UnixNano()) {
		t.Fatalf("time wrong: %+v", got["happened_at"])
	}
	if g := got["elapsed"]; g == nil || g.GetDurationNanos() != int64(250*time.Millisecond) {
		t.Fatalf("duration wrong: %+v", got["elapsed"])
	}
	mustInt(t, got, "user.id", 7)
	mustString(t, got, "user.role", "admin")
}

func TestRemoteSlogHandler_DropOnFullReportsCounter(t *testing.T) {
	fake := newFakeLogProxyServer()
	// onRecord blocks the first record so the channel fills up.
	release := make(chan struct{})
	var first sync.Once
	fake.onRecord = func(_ *pb.LogRecord) {
		first.Do(func() { <-release })
	}
	client, cleanup := startBufconnServer(t, fake)
	defer cleanup()

	h := newRemoteSlogHandler(slog.LevelInfo)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		runRemoteSlogSender(ctx, client, h, nil)
	}()

	logger := slog.New(h)
	// Fire enough to overflow the 256-cap channel.
	const n = LogChannelCapacity + 50
	for i := 0; i < n; i++ {
		logger.Info("burst", "i", i)
	}
	if h.droppedCount.Load() == 0 {
		// We may need to wait briefly for the consumer to pick records up.
		waitFor(t, func() bool { return h.droppedCount.Load() > 0 }, 500*time.Millisecond)
	}
	dropped := h.droppedCount.Load()
	if dropped == 0 {
		t.Fatalf("expected some drops, got 0")
	}

	// Release the consumer; the next successful Send should attach
	// dropped_since_last_send and reset the counter.
	close(release)
	waitFor(t, func() bool { return h.droppedCount.Load() == 0 }, time.Second)

	records := fake.snapshot()
	if len(records) == 0 {
		t.Fatalf("expected at least one record")
	}
	hasDropReport := false
	for _, r := range records {
		if r.DroppedSinceLastSend > 0 {
			hasDropReport = true
			break
		}
	}
	if !hasDropReport {
		t.Fatalf("dropped_since_last_send was never reported")
	}

	h.Close()
	<-done
}

func TestRemoteSlogHandler_WithAttrsAndGroup(t *testing.T) {
	fake := newFakeLogProxyServer()
	client, cleanup := startBufconnServer(t, fake)
	defer cleanup()

	h := newRemoteSlogHandler(slog.LevelInfo)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		runRemoteSlogSender(ctx, client, h, nil)
	}()

	base := slog.New(h).With("plugin", "test").WithGroup("req")
	base.Info("scoped", "id", "x")

	waitFor(t, func() bool { return len(fake.snapshot()) > 0 }, time.Second)
	rec := fake.snapshot()[0]
	got := map[string]string{}
	for _, a := range rec.Attrs {
		if a.GetStringValue() != "" {
			got[a.Key] = a.GetStringValue()
		}
	}
	if got["req.plugin"] != "test" {
		t.Fatalf("With attr should be flattened under group prefix, got %+v", got)
	}
	if got["req.id"] != "x" {
		t.Fatalf("group prefix not applied to record attr, got %+v", got)
	}

	h.Close()
	<-done
}

// fakeUnauthLogProxyServer always rejects PushLogs with Unauthenticated,
// matching the host-side behaviour for callers whose identity has been
// invalidated. The subscribeCalls counter lets the T31 regression assert
// that the SDK does not retry on a fatal classification.
type fakeUnauthLogProxyServer struct {
	pb.UnimplementedLogProxyServer
	calls atomic.Int64
}

func (s *fakeUnauthLogProxyServer) PushLogs(stream pb.LogProxy_PushLogsServer) error {
	s.calls.Add(1)
	return status.Error(codes.Unauthenticated, "principal expired")
}

// TestRunRemoteSlogSender_UnauthenticatedExitsFast pins the T31 contract:
// when the host returns Unauthenticated from PushLogs (e.g. credentials
// invalidated mid-process), GRPCDefaultClassifier marks the error fatal
// and the sender goroutine exits instead of looping forever. We also
// assert PushLogs is invoked exactly once — any retry would indicate the
// classifier was bypassed.
func TestRunRemoteSlogSender_UnauthenticatedExitsFast(t *testing.T) {
	fake := &fakeUnauthLogProxyServer{}
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	pb.RegisterLogProxyServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	defer conn.Close()
	client := pb.NewLogProxyClient(conn)

	h := newRemoteSlogHandler(slog.LevelInfo)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		runRemoteSlogSender(ctx, client, h, nil)
	}()

	// Push records until the sender exits. The first record opens
	// PushLogs (server returns Unauthenticated immediately); subsequent
	// records keep the consumer goroutine attempting Send so EOF
	// surfaces and CloseAndRecv can read the terminal status.
	logger := slog.New(h)
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				logger.Info("ping", "i", i)
				i++
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		close(stop)
		t.Fatalf("runRemoteSlogSender did not exit on Unauthenticated (server calls=%d, dropped=%d); LoopWithRetryClass should classify it fatal",
			fake.calls.Load(), h.droppedCount.Load())
	}
	close(stop)
	if got := fake.calls.Load(); got != 1 {
		t.Fatalf("PushLogs called %d times, want exactly 1 (no retry on fatal)", got)
	}
}

func TestRemoteSlogHandler_EnabledHonoursLevel(t *testing.T) {
	h := newRemoteSlogHandler(slog.LevelWarn)
	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatalf("Info should be disabled at Warn threshold")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Fatalf("Error should be enabled at Warn threshold")
	}
}

func waitFor(t *testing.T, cond func() bool, max time.Duration) {
	t.Helper()
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("waitFor: condition never satisfied within %s", max)
}

func mustString(t *testing.T, m map[string]*pb.LogAttr, key, want string) {
	t.Helper()
	a := m[key]
	if a == nil {
		t.Fatalf("missing attr %q", key)
	}
	if got := a.GetStringValue(); got != want {
		t.Fatalf("attr %q = %q, want %q", key, got, want)
	}
}

func mustInt(t *testing.T, m map[string]*pb.LogAttr, key string, want int64) {
	t.Helper()
	a := m[key]
	if a == nil {
		t.Fatalf("missing attr %q", key)
	}
	if got := a.GetIntValue(); got != want {
		t.Fatalf("attr %q = %d, want %d", key, got, want)
	}
}

func mustFloat(t *testing.T, m map[string]*pb.LogAttr, key string, want float64) {
	t.Helper()
	a := m[key]
	if a == nil {
		t.Fatalf("missing attr %q", key)
	}
	if got := a.GetFloatValue(); got != want {
		t.Fatalf("attr %q = %v, want %v", key, got, want)
	}
}

func mustBool(t *testing.T, m map[string]*pb.LogAttr, key string, want bool) {
	t.Helper()
	a := m[key]
	if a == nil {
		t.Fatalf("missing attr %q", key)
	}
	if got := a.GetBoolValue(); got != want {
		t.Fatalf("attr %q = %v, want %v", key, got, want)
	}
}
