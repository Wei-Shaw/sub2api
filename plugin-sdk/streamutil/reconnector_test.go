package streamutil

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// TestNextBackoffSchedule verifies the multiplier + cap behaviour
// independently of the loop so callers can reason about the schedule
// from a docstring.
func TestNextBackoffSchedule(t *testing.T) {
	cases := []struct {
		name string
		cur  time.Duration
		mult float64
		max  time.Duration
		want time.Duration
	}{
		{"double", 1 * time.Second, 2, 30 * time.Second, 2 * time.Second},
		{"triple", 1 * time.Second, 3, 30 * time.Second, 3 * time.Second},
		{"capped", 20 * time.Second, 2, 30 * time.Second, 30 * time.Second},
		{"already-at-cap", 30 * time.Second, 2, 30 * time.Second, 30 * time.Second},
		{"flat-multiplier", 5 * time.Second, 1, 30 * time.Second, 5 * time.Second},
		{"sub-flat-multiplier", 5 * time.Second, 0.5, 30 * time.Second, 5 * time.Second},
		{"zero-cur-jumps-to-max", 0, 2, 30 * time.Second, 30 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextBackoff(tc.cur, tc.mult, tc.max); got != tc.want {
				t.Fatalf("nextBackoff(%v,%v,%v)=%v want %v", tc.cur, tc.mult, tc.max, got, tc.want)
			}
		})
	}
}

// TestLoopExitsOnCtxCancel ensures ctx cancellation breaks the loop and
// the ctx.Err() is returned, regardless of which phase (sleep / attempt)
// the loop is in.
func TestLoopExitsOnCtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := atomic.Int32{}

	done := make(chan error, 1)
	go func() {
		done <- Loop(ctx, Config{
			Name:        "test.exit",
			Initial:     10 * time.Millisecond,
			Max:         100 * time.Millisecond,
			Multiplier:  2,
			JitterRatio: 0,
		}, func(ctx context.Context) error {
			calls.Add(1)
			// Block in attempt so the cancel hits us mid-call. Honour ctx.
			<-ctx.Done()
			return ctx.Err()
		})
	}()

	// Give the loop a chance to enter attempt.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Loop returned %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Loop did not return after ctx cancel")
	}
	if got := calls.Load(); got < 1 {
		t.Fatalf("attempt called %d times, want >=1", got)
	}
}

// TestLoopBackoffGrowsThenResetsOnSuccess simulates a fail / fail /
// succeed / fail sequence and asserts the second failure-after-success
// uses Initial again, not the previously grown delay.
func TestLoopBackoffGrowsThenResetsOnSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Sequence: error, error, nil, error, then block until cancel.
	type call struct {
		retErr  error
		blocked bool
	}
	plan := []call{
		{retErr: errors.New("fail-1")},
		{retErr: errors.New("fail-2")},
		{retErr: nil}, // resets backoff
		{retErr: errors.New("fail-after-success")},
		{blocked: true},
	}
	idx := atomic.Int32{}
	delays := make(chan time.Duration, 8)
	var lastReturn time.Time

	attempt := func(ctx context.Context) error {
		i := int(idx.Load())
		if i >= len(plan) {
			<-ctx.Done()
			return ctx.Err()
		}
		// Record the gap since the previous return — that's the sleep
		// the Loop performed before this call.
		if !lastReturn.IsZero() {
			delays <- time.Since(lastReturn)
		}
		idx.Add(1)
		c := plan[i]
		if c.blocked {
			<-ctx.Done()
			return ctx.Err()
		}
		lastReturn = time.Now()
		return c.retErr
	}

	go func() {
		_ = Loop(ctx, Config{
			Name:        "test.reset",
			Initial:     20 * time.Millisecond,
			Max:         500 * time.Millisecond,
			Multiplier:  2,
			JitterRatio: 0,
		}, attempt)
	}()

	// Collect the 3 inter-call gaps we expect: after fail-1 (~20ms),
	// after fail-2 (~40ms), after success (gap before fail-after-success
	// is essentially 0 because attempt returns nil and Loop immediately
	// re-invokes), and after fail-after-success (~20ms again — proves
	// reset on success).
	collected := make([]time.Duration, 0, 4)
	deadline := time.After(2 * time.Second)
	for len(collected) < 4 {
		select {
		case d := <-delays:
			collected = append(collected, d)
		case <-deadline:
			t.Fatalf("timeout collecting delays; got %d so far: %v", len(collected), collected)
		}
	}

	// Tolerance: 8ms either side covers timer scheduling jitter on CI.
	approx := func(got, want time.Duration) bool {
		return got >= want-8*time.Millisecond && got <= want+30*time.Millisecond
	}

	if !approx(collected[0], 20*time.Millisecond) {
		t.Errorf("first backoff (fail-1) = %v, want ~20ms", collected[0])
	}
	if !approx(collected[1], 40*time.Millisecond) {
		t.Errorf("second backoff (fail-2) = %v, want ~40ms", collected[1])
	}
	// collected[2] is the gap after a nil return; should be sub-millisecond.
	if collected[2] > 5*time.Millisecond {
		t.Errorf("post-success gap = %v, want <5ms (Loop should not sleep on nil)", collected[2])
	}
	// collected[3] is the gap after fail-after-success — reset means Initial again.
	if !approx(collected[3], 20*time.Millisecond) {
		t.Errorf("post-success-then-fail backoff = %v, want ~20ms (reset to Initial)", collected[3])
	}
}

// TestLoopHonoursMax confirms the cap is reached and not exceeded under
// a multiplier that would normally blow past it.
func TestLoopHonoursMax(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	delays := make(chan time.Duration, 16)
	var lastReturn time.Time
	calls := atomic.Int32{}

	go func() {
		_ = Loop(ctx, Config{
			Name:        "test.max",
			Initial:     5 * time.Millisecond,
			Max:         20 * time.Millisecond,
			Multiplier:  3,
			JitterRatio: 0,
		}, func(ctx context.Context) error {
			n := calls.Add(1)
			if !lastReturn.IsZero() {
				select {
				case delays <- time.Since(lastReturn):
				default:
				}
			}
			lastReturn = time.Now()
			if n > 6 {
				<-ctx.Done()
				return ctx.Err()
			}
			return errors.New("always fails")
		})
	}()

	// Collect 5 delays — first 3 should ramp 5→15→20→20→20 (capped).
	got := make([]time.Duration, 0, 5)
	deadline := time.After(2 * time.Second)
	for len(got) < 5 {
		select {
		case d := <-delays:
			got = append(got, d)
		case <-deadline:
			t.Fatalf("timeout; got %d delays: %v", len(got), got)
		}
	}
	for i, d := range got {
		if d > 20*time.Millisecond+30*time.Millisecond {
			t.Errorf("delay[%d] = %v exceeds Max(20ms) + scheduling slack", i, d)
		}
	}
}
