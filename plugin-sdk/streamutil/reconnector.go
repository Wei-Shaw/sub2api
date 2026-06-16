// Package streamutil contains plumbing shared by the SDK's long-lived
// streaming clients (jobs, settings, events, log_remote). Each of those
// clients used to carry its own near-identical "open stream → drain →
// failure → backoff → ctx-cancel" loop with subtly different multipliers
// (×2 vs ×3) and slightly different jitter / sleep helpers. Loop is the
// single replacement so the four sites converge on one schedule and one
// ctx-cancel semantics.
//
// streamutil intentionally has zero dependencies outside the standard
// library. It now lives at the plugin-sdk root (out of internal/) so the
// host's pricing / channel-mgmt extension clients can reuse the same
// reconnect schedule; without this, a host-side watch loop would silently
// drift away from the SDK's "reset backoff after a successful attempt"
// invariant. Keep new helpers small and well-typed — this is plumbing,
// not a place to grow business logic.
package streamutil

import (
	"context"
	"log/slog"
	"math/rand"
	"time"
)

// Config tunes the Loop reconnect schedule.
//
// The zero value is NOT usable — at minimum Initial / Max / Multiplier
// must be set. Loop falls back to a slog.Default() logger when Logger
// is nil so call sites can pass a nil logger during tests.
type Config struct {
	// Name is a short identifier used in log lines, e.g. "events.subscribe"
	// or "settings.watch". Plugins do not see this; it is for SDK debug
	// logging only.
	Name string

	// Initial is the first delay applied after a failed attempt and the
	// value backoff is reset to whenever attempt returns nil.
	Initial time.Duration

	// Max is the upper bound on a single backoff sleep. Loop never sleeps
	// longer than Max regardless of Multiplier.
	Max time.Duration

	// Multiplier scales the backoff after each consecutive failure.
	// Typical values are 2.0 or 3.0; anything <=1 keeps the delay flat.
	Multiplier float64

	// JitterRatio is the symmetric jitter applied to each sleep, expressed
	// as a fraction of the current backoff. 0.2 means ±20%. Values <=0
	// disable jitter; values >=1 are clamped to 1 (which would put the
	// minimum at 0, so we floor at 1ms internally to avoid spinning).
	JitterRatio float64

	// Logger receives "stream attempt failed" warnings. nil is allowed
	// and falls back to slog.Default().
	Logger *slog.Logger
}

// Loop runs attempt repeatedly with reconnect backoff:
//
//   - attempt is invoked immediately (no initial sleep) and is expected
//     to block: open the stream, drain it, return an error when the
//     stream breaks. A nil return means the call ended cleanly (e.g. the
//     remote closed without error or ctx was cancelled mid-drain) and
//     resets the backoff to Initial before the next call.
//   - When attempt returns a non-nil error, Loop sleeps for the current
//     backoff (with jitter) and then re-invokes attempt. The backoff
//     grows by Multiplier up to Max.
//   - Loop returns when ctx is cancelled, propagating ctx.Err() so
//     callers can `defer close(loopDone)` without an extra check.
//
// attempt errors are intentionally NOT returned to the caller: callers
// of Loop want to retry forever and only exit via ctx. If a stream
// client needs to surface "operator must fix" errors (PermissionDenied,
// Unimplemented, ...) call LoopWithRetryClass with a classifier instead.
//
// 抽象 (4)：Loop 是 LoopWithRetryClass(..., nil, ...) 的薄包装；同一份
// backoff schedule 既覆盖 "始终重试" 的 jobs / settings / log_remote，也
// 覆盖 "致命错误立即退出" 的 events / 未来扩展。
func Loop(ctx context.Context, cfg Config, attempt func(context.Context) error) error {
	return LoopWithRetryClass(ctx, cfg, nil, attempt)
}

// nextBackoff multiplies cur by mult, clamped to max. Pulled out so
// the unit test can verify the schedule independently of the loop.
func nextBackoff(cur time.Duration, mult float64, max time.Duration) time.Duration {
	if cur <= 0 {
		return max
	}
	next := time.Duration(float64(cur) * mult)
	if next <= cur {
		// Multiplier <=1 keeps delay flat. We deliberately do not bump
		// by an arbitrary amount; flat backoff is a legitimate config.
		next = cur
	}
	if next > max {
		return max
	}
	return next
}

// jitter applies ±ratio*d randomness, floored at 1ms so we never
// busy-spin on a misconfigured Initial=0.
func jitter(d time.Duration, ratio float64) time.Duration {
	if d <= 0 {
		return time.Millisecond
	}
	if ratio <= 0 {
		return d
	}
	if ratio > 1 {
		ratio = 1
	}
	span := float64(d) * ratio
	delta := (rand.Float64()*2 - 1) * span
	out := time.Duration(float64(d) + delta)
	if out <= 0 {
		return time.Millisecond
	}
	return out
}

// sleep waits d or until ctx is done. Returns true if the wait
// completed normally, false if ctx cancelled.
func sleep(ctx context.Context, d time.Duration) bool {
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
