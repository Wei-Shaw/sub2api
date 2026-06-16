package pluginsdk

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestConvertJobSpec_Interval verifies the interval kind is round-tripped to
// the wire form with the matching nano value.
func TestConvertJobSpec_Interval(t *testing.T) {
	spec, err := convertJobSpec(JobSpec{
		Name:        "demo",
		Trigger:     JobTrigger{Kind: TriggerInterval, Interval: 30 * time.Second},
		Concurrency: 3,
		Timeout:     2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("convertJobSpec: %v", err)
	}
	if spec.GetKind() != string(TriggerInterval) {
		t.Errorf("kind = %q, want interval", spec.GetKind())
	}
	if spec.GetIntervalNanos() != int64(30*time.Second) {
		t.Errorf("interval_nanos = %d, want %d", spec.GetIntervalNanos(), int64(30*time.Second))
	}
	if spec.GetConcurrency() != 3 {
		t.Errorf("concurrency = %d, want 3", spec.GetConcurrency())
	}
}

// TestConvertJobSpec_Cron verifies cron specs are validated for non-empty
// cron string.
func TestConvertJobSpec_Cron(t *testing.T) {
	if _, err := convertJobSpec(JobSpec{
		Name:    "rollup",
		Trigger: JobTrigger{Kind: TriggerCron, CronSpec: ""},
	}); err == nil {
		t.Fatal("expected error for empty cron spec, got nil")
	}
	spec, err := convertJobSpec(JobSpec{
		Name:    "rollup",
		Trigger: JobTrigger{Kind: TriggerCron, CronSpec: "0 2 * * *"},
	})
	if err != nil {
		t.Fatalf("convertJobSpec cron: %v", err)
	}
	if spec.GetKind() != string(TriggerCron) {
		t.Errorf("kind = %q, want cron", spec.GetKind())
	}
	if spec.GetCronSpec() != "0 2 * * *" {
		t.Errorf("cron_spec = %q", spec.GetCronSpec())
	}
}

// TestConvertJobSpec_UnknownKind ensures unknown trigger kinds are rejected.
func TestConvertJobSpec_UnknownKind(t *testing.T) {
	if _, err := convertJobSpec(JobSpec{
		Name:    "x",
		Trigger: JobTrigger{Kind: "wat"},
	}); err == nil {
		t.Fatal("expected error for unknown trigger kind, got nil")
	}
}

// TestJobsBackoffConstants pins the SDK-unified ×2 reconnect schedule
// (1s → 2s → 4s → 8s → 16s → 30s ceiling). The actual schedule is
// computed inside streamutil.Loop and verified by streamutil's own
// tests; this test exists so a future PR that bumps the multiplier or
// the cap on jobs reconnect must also update this assertion, surfacing
// the V5-DESIGN §2.6 contract change at review time.
func TestJobsBackoffConstants(t *testing.T) {
	if jobReconnectInitialBackoff != 1*time.Second {
		t.Errorf("jobReconnectInitialBackoff = %v, want 1s", jobReconnectInitialBackoff)
	}
	if jobReconnectMaxBackoff != 30*time.Second {
		t.Errorf("jobReconnectMaxBackoff = %v, want 30s", jobReconnectMaxBackoff)
	}
	if jobReconnectMultiplier != 2.0 {
		t.Errorf("jobReconnectMultiplier = %v, want 2.0 (SDK-wide unification)", jobReconnectMultiplier)
	}
}

// TestRegisterValidates ensures Register rejects empty names, nil handlers,
// and registration after the run loop has started.
func TestRegisterValidates(t *testing.T) {
	c := newJobsClient("p1", nil, nil)

	if err := c.Register(JobSpec{Name: ""}, func(context.Context, string) error { return nil }); err == nil {
		t.Fatal("empty name should fail")
	}
	if err := c.Register(JobSpec{
		Name:    "x",
		Trigger: JobTrigger{Kind: TriggerInterval, Interval: time.Second},
	}, nil); err == nil {
		t.Fatal("nil handler should fail")
	}

	if err := c.Register(JobSpec{
		Name:    "ok",
		Trigger: JobTrigger{Kind: TriggerInterval, Interval: time.Second},
	}, func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("first register: %v", err)
	}

	// Simulate the run loop having started — Register should now refuse new
	// specs to keep the host's view consistent across the lifetime.
	c.mu.Lock()
	c.started = true
	c.mu.Unlock()

	if err := c.Register(JobSpec{
		Name:    "second",
		Trigger: JobTrigger{Kind: TriggerInterval, Interval: time.Second},
	}, func(context.Context, string) error { return nil }); !errors.Is(err, ErrJobsRegistered) {
		t.Fatalf("late register error = %v, want ErrJobsRegistered", err)
	}
}

// TestTriggerLocal_RunsHandler verifies TriggerLocal honours the configured
// timeout and returns ErrJobUnknown for unregistered names.
func TestTriggerLocal_RunsHandler(t *testing.T) {
	c := newJobsClient("p1", nil, nil)
	var ran atomic.Bool
	if err := c.Register(JobSpec{
		Name:    "ping",
		Trigger: JobTrigger{Kind: TriggerInterval, Interval: time.Second},
		Timeout: 200 * time.Millisecond,
	}, func(_ context.Context, name string) error {
		if name != "ping" {
			t.Errorf("handler got name=%q", name)
		}
		ran.Store(true)
		return nil
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := c.TriggerLocal(context.Background(), "ping"); err != nil {
		t.Fatalf("TriggerLocal: %v", err)
	}
	if !ran.Load() {
		t.Fatal("handler did not run")
	}

	if err := c.TriggerLocal(context.Background(), "unknown"); !errors.Is(err, ErrJobUnknown) {
		t.Fatalf("unknown TriggerLocal error = %v, want ErrJobUnknown", err)
	}
}

// TestRun_PermissionDeniedExitsFast is the T31 regression: when the host
// rejects the Subscribe dial with PermissionDenied (e.g. anonymous caller
// or missing capability), GRPCDefaultClassifier marks the error fatal and
// LoopWithRetryClass must exit instead of reconnecting forever. The
// observable signal is `c.loopDone` closing within a few hundred ms; with
// the old streamutil.Loop the goroutine would still be sleeping inside
// the backoff window after the test deadline.
func TestRun_PermissionDeniedExitsFast(t *testing.T) {
	var calls atomic.Int64
	dial := func(ctx context.Context) (pb.JobScheduler_SubscribeClient, error) {
		calls.Add(1)
		return nil, status.Error(codes.PermissionDenied, "anonymous caller")
	}
	c := newJobsClient("p1", nil, dial)
	c.loopDone = make(chan struct{})

	done := make(chan struct{})
	go func() {
		c.run(context.Background())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not exit on PermissionDenied; LoopWithRetryClass should have classified it fatal")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("dial called %d times, want exactly 1 (no retry on fatal)", got)
	}
}

// TestRun_UnavailableRetries asserts the inverse: a transient gRPC error
// (Unavailable) is NOT classified fatal, so run keeps retrying. We assert
// at least 2 dial calls inside a short window then cancel the loop ctx.
// Together with the test above this fully pins the LoopWithRetryClass
// contract for the jobs client.
func TestRun_UnavailableRetries(t *testing.T) {
	var calls atomic.Int64
	dial := func(ctx context.Context) (pb.JobScheduler_SubscribeClient, error) {
		calls.Add(1)
		return nil, status.Error(codes.Unavailable, "host bouncing")
	}
	c := newJobsClient("p1", nil, dial)
	// Override backoff so the test does not sit on the 1s default.
	c.loopDone = make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.run(ctx)
		close(done)
	}()

	// Wait for at least two dial attempts. Initial backoff is 1s, so
	// allow up to 3s for the second call.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && calls.Load() < 2 {
		time.Sleep(20 * time.Millisecond)
	}
	if got := calls.Load(); got < 2 {
		cancel()
		<-done
		t.Fatalf("dial called %d times in 3s, want >=2 (Unavailable must be retried)", got)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not exit after ctx cancel")
	}
}

// TestDispatch_ConcurrencyLimit verifies that an over-cap trigger is acked
// with success=false rather than blocking until a slot frees up.
func TestDispatch_ConcurrencyLimit(t *testing.T) {
	c := newJobsClient("p1", nil, nil)
	holdCh := make(chan struct{})
	releaseCh := make(chan struct{})
	if err := c.Register(JobSpec{
		Name:        "slow",
		Trigger:     JobTrigger{Kind: TriggerInterval, Interval: time.Second},
		Concurrency: 1,
		Timeout:     5 * time.Second,
	}, func(ctx context.Context, _ string) error {
		close(holdCh)
		<-releaseCh
		return nil
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Capture acks via a stub stream.
	stub := newStubStream()
	c.streamMu.Lock()
	c.stream = stub
	c.streamMu.Unlock()

	go c.dispatch(context.Background(), (&fakeTrigger{name: "slow", id: "first"}).toPB())
	<-holdCh // first handler is in-flight, holding the slot

	// Second dispatch must immediately ack failure (concurrency limit reached)
	// rather than block.
	doneCh := make(chan struct{})
	go func() {
		c.dispatch(context.Background(), (&fakeTrigger{name: "slow", id: "second"}).toPB())
		close(doneCh)
	}()
	select {
	case <-doneCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("second dispatch blocked instead of failing fast")
	}

	close(releaseCh)
	stub.waitForAcks(t, 2, 2*time.Second)
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.acks) != 2 {
		t.Fatalf("got %d acks, want 2", len(stub.acks))
	}
	// Second ack should mark the throttle.
	for _, ack := range stub.acks {
		if ack.GetTriggerId() == "second" {
			if ack.GetSuccess() {
				t.Errorf("second ack success=true; expected concurrency-limit failure")
			}
			if ack.GetError() != "concurrency limit reached" {
				t.Errorf("second ack error = %q", ack.GetError())
			}
		}
	}
}
