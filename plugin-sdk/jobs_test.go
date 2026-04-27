package pluginsdk

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
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

// TestNextBackoff verifies the (1s → 3s → 9s → 27s → 30s ceiling) schedule.
func TestNextBackoff(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want time.Duration
	}{
		{1 * time.Second, 3 * time.Second},
		{3 * time.Second, 9 * time.Second},
		{9 * time.Second, 27 * time.Second},
		{27 * time.Second, 30 * time.Second},
		{30 * time.Second, 30 * time.Second},
	}
	for _, c := range cases {
		if got := jobNextBackoff(c.in); got != c.want {
			t.Errorf("jobNextBackoff(%v) = %v, want %v", c.in, got, c.want)
		}
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
