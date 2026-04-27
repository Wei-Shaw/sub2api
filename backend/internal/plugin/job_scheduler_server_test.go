//go:build unit

package plugin

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// fakeStream is a minimal in-memory bidirectional stream for unit-testing
// JobSchedulerServer.Subscribe without bringing up a real gRPC server.
type fakeStream struct {
	ctx       context.Context
	cancel    context.CancelFunc
	incoming  chan *pluginsdk.JobMessage // host receives from plugin
	outgoing  chan *pluginsdk.JobTrigger // plugin receives from host
	sendErr   error
	closeOnce sync.Once
}

func newFakeStream() *fakeStream {
	ctx, cancel := context.WithCancel(context.Background())
	// Caller identity comes from metadata; the resolver in our test
	// inspects this directly so we wire it via metadata.NewIncomingContext.
	md := metadata.Pairs(callerMetadataKey, "test-plugin")
	ctx = metadata.NewIncomingContext(ctx, md)
	return &fakeStream{
		ctx:      ctx,
		cancel:   cancel,
		incoming: make(chan *pluginsdk.JobMessage, 8),
		outgoing: make(chan *pluginsdk.JobTrigger, 8),
	}
}

func (s *fakeStream) Send(t *pluginsdk.JobTrigger) error {
	if s.sendErr != nil {
		return s.sendErr
	}
	select {
	case s.outgoing <- t:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func (s *fakeStream) Recv() (*pluginsdk.JobMessage, error) {
	select {
	case m, ok := <-s.incoming:
		if !ok {
			return nil, io.EOF
		}
		return m, nil
	case <-s.ctx.Done():
		return nil, io.EOF
	}
}

func (s *fakeStream) Context() context.Context     { return s.ctx }
func (s *fakeStream) SendHeader(metadata.MD) error { return nil }
func (s *fakeStream) SetHeader(metadata.MD) error  { return nil }
func (s *fakeStream) SetTrailer(metadata.MD)       {}
func (s *fakeStream) SendMsg(any) error            { return nil }
func (s *fakeStream) RecvMsg(any) error            { return nil }

func (s *fakeStream) close() {
	s.closeOnce.Do(func() {
		close(s.incoming)
		s.cancel()
	})
}

// recordingHistory captures RecordRun calls for assertions.
type recordingHistory struct {
	mu   sync.Mutex
	runs []JobRunRecord
}

func (r *recordingHistory) RecordRun(_ context.Context, run JobRunRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs = append(r.runs, run)
}

func (r *recordingHistory) snapshot() []JobRunRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]JobRunRecord, len(r.runs))
	copy(out, r.runs)
	return out
}

// alwaysLeader is a leader-lock provider that always grants leadership.
type alwaysLeader struct{}

func (alwaysLeader) TryAcquire(context.Context, string) (func(), bool) {
	return func() {}, true
}

// neverLeader rejects every TryAcquire so we can verify leader_only specs
// stay quiescent on non-leader nodes.
type neverLeader struct{}

func (neverLeader) TryAcquire(context.Context, string) (func(), bool) {
	return nil, false
}

// TestSubscribe_FiresIntervalAndAcks verifies the happy path: a plugin
// registers an interval spec, the host fires a trigger, the plugin acks, the
// history records success.
func TestSubscribe_FiresIntervalAndAcks(t *testing.T) {
	t.Parallel()
	hist := &recordingHistory{}
	srv := NewJobSchedulerServer(
		func(ctx context.Context) string { return "test-plugin" },
		alwaysLeader{},
		hist,
		nil,
	)
	defer srv.Stop()

	stream := newFakeStream()
	defer stream.close()

	// Run Subscribe in a goroutine — it blocks on the bidi stream.
	subDone := make(chan error, 1)
	go func() { subDone <- srv.Subscribe(stream) }()

	// Send Register frame.
	stream.incoming <- &pluginsdk.JobMessage{
		Msg: &pluginsdk.JobMessage_Register{Register: &pluginsdk.JobRegistration{
			Specs: []*pluginsdk.JobSpec{{
				Name:          "tick",
				Kind:          "interval",
				IntervalNanos: int64(20 * time.Millisecond),
				Concurrency:   1,
			}},
		}},
	}

	// Wait for the first trigger.
	var trig *pluginsdk.JobTrigger
	select {
	case trig = <-stream.outgoing:
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive trigger within 2s")
	}
	if trig.GetJobName() != "tick" {
		t.Fatalf("trigger name = %q", trig.GetJobName())
	}
	if trig.GetTriggerId() == "" {
		t.Fatal("trigger missing id")
	}

	// Send the matching ack.
	stream.incoming <- &pluginsdk.JobMessage{
		Msg: &pluginsdk.JobMessage_Ack{Ack: &pluginsdk.JobAck{
			TriggerId:     trig.GetTriggerId(),
			Success:       true,
			DurationNanos: int64(15 * time.Millisecond),
		}},
	}

	// Poll for the history record. The recordHistory call happens on the
	// recv goroutine so it may race with our assertion — the loop bounds it.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runs := hist.snapshot()
		if len(runs) >= 1 {
			if !runs[0].Success {
				t.Fatalf("expected success ack, got %+v", runs[0])
			}
			if runs[0].JobName != "tick" {
				t.Fatalf("unexpected job name: %q", runs[0].JobName)
			}
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	stream.close()
	select {
	case <-subDone:
	case <-time.After(2 * time.Second):
		t.Fatal("subscribe did not return after stream close")
	}
}

// TestSubscribe_LeaderOnlySkipsWhenNotLeader verifies leader_only=true specs
// are silenced on a non-leader node.
func TestSubscribe_LeaderOnlySkipsWhenNotLeader(t *testing.T) {
	t.Parallel()
	hist := &recordingHistory{}
	srv := NewJobSchedulerServer(
		func(ctx context.Context) string { return "test-plugin" },
		neverLeader{},
		hist,
		nil,
	)
	defer srv.Stop()

	stream := newFakeStream()
	defer stream.close()

	go func() { _ = srv.Subscribe(stream) }()

	stream.incoming <- &pluginsdk.JobMessage{
		Msg: &pluginsdk.JobMessage_Register{Register: &pluginsdk.JobRegistration{
			Specs: []*pluginsdk.JobSpec{{
				Name:          "leader-job",
				Kind:          "interval",
				IntervalNanos: int64(10 * time.Millisecond),
				LeaderOnly:    true,
			}},
		}},
	}

	// Wait long enough to observe several would-be ticks.
	select {
	case trig := <-stream.outgoing:
		t.Fatalf("non-leader should not receive triggers, got %+v", trig)
	case <-time.After(150 * time.Millisecond):
	}
}

// TestSubscribe_RejectsNonRegisterFirstFrame guards the protocol contract
// that the first message must be Register.
func TestSubscribe_RejectsNonRegisterFirstFrame(t *testing.T) {
	t.Parallel()
	srv := NewJobSchedulerServer(
		func(ctx context.Context) string { return "test-plugin" },
		alwaysLeader{},
		nil,
		nil,
	)
	defer srv.Stop()

	stream := newFakeStream()
	defer stream.close()

	subDone := make(chan error, 1)
	go func() { subDone <- srv.Subscribe(stream) }()

	stream.incoming <- &pluginsdk.JobMessage{
		Msg: &pluginsdk.JobMessage_Ack{Ack: &pluginsdk.JobAck{TriggerId: "x"}},
	}

	select {
	case err := <-subDone:
		if err == nil {
			t.Fatal("expected protocol error, got nil")
		}
	case <-time.After(time.Second):
		t.Fatal("subscribe did not return error in time")
	}
}

// TestApplySpecs_ValidationErrors is a table-driven check that the validation
// pass rejects bad specs without side effects.
func TestApplySpecs_ValidationErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		spec    *pluginsdk.JobSpec
		wantErr string
	}{
		{
			name:    "missing name",
			spec:    &pluginsdk.JobSpec{Kind: "interval", IntervalNanos: 1000},
			wantErr: "spec name is required",
		},
		{
			name:    "interval zero",
			spec:    &pluginsdk.JobSpec{Name: "x", Kind: "interval", IntervalNanos: 0},
			wantErr: "interval must be > 0",
		},
		{
			name:    "fixed_delay zero",
			spec:    &pluginsdk.JobSpec{Name: "x", Kind: "fixed_delay", FixedDelayNanos: 0},
			wantErr: "fixed_delay must be > 0",
		},
		{
			name:    "cron malformed",
			spec:    &pluginsdk.JobSpec{Name: "x", Kind: "cron", CronSpec: "bogus"},
			wantErr: "cron parse",
		},
		{
			name:    "unknown kind",
			spec:    &pluginsdk.JobSpec{Name: "x", Kind: "weekly"},
			wantErr: "unknown kind",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ps := newPluginScheduler("p", alwaysLeader{}, nil, nil, context.Background())
			err := ps.applySpecs([]*pluginsdk.JobSpec{tc.spec})
			if err == nil || !contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && (indexOf(haystack, needle) >= 0))
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// Sanity check: Subscribe with a nil resolver always returns Unauthenticated.
func TestSubscribe_NoResolver(t *testing.T) {
	t.Parallel()
	srv := NewJobSchedulerServer(nil, alwaysLeader{}, nil, nil)
	defer srv.Stop()

	stream := newFakeStream()
	defer stream.close()

	err := srv.Subscribe(stream)
	if err == nil {
		t.Fatal("expected error from nil resolver")
	}
}

// TestStop_NilSafe verifies that calling Stop on a nil scheduler is a no-op
// rather than a panic — important because PluginManager.ShutdownAll fires
// regardless of whether startSDKServer ran successfully.
func TestStop_NilSafe(t *testing.T) {
	var srv *JobSchedulerServer
	srv.Stop()
}
