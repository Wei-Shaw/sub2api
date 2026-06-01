package streamutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestGRPCDefaultClassifierFatalCodes locks down the four codes treated
// as fatal so a future "let's also retry PermissionDenied" change has
// to delete this test deliberately.
func TestGRPCDefaultClassifierFatalCodes(t *testing.T) {
	fatal := []codes.Code{
		codes.Unimplemented,
		codes.PermissionDenied,
		codes.InvalidArgument,
		codes.Unauthenticated,
	}
	for _, c := range fatal {
		err := status.Error(c, "boom")
		if got := GRPCDefaultClassifier(err); got != RetryDecisionFatal {
			t.Errorf("code %s classified as %v, want RetryDecisionFatal", c, got)
		}
	}
}

// TestGRPCDefaultClassifierRetryableCodes spot-checks the non-fatal
// codes the SDK leans on most often.
func TestGRPCDefaultClassifierRetryableCodes(t *testing.T) {
	retryable := []codes.Code{
		codes.Unavailable,
		codes.DeadlineExceeded,
		codes.Internal,
		codes.ResourceExhausted,
		codes.Aborted,
		codes.Canceled, // ctx-driven, but explicitly NOT fatal here so a
		// server-side cancel doesn't kill the loop forever.
	}
	for _, c := range retryable {
		err := status.Error(c, "boom")
		if got := GRPCDefaultClassifier(err); got != RetryDecisionRetry {
			t.Errorf("code %s classified as %v, want RetryDecisionRetry", c, got)
		}
	}
}

// TestGRPCDefaultClassifierWrappedStatus ensures fmt.Errorf("...: %w", st)
// still surfaces fatal codes. status.FromError only unwraps directly-
// wrapped status errors, so the classifier walks the chain via errors.As.
func TestGRPCDefaultClassifierWrappedStatus(t *testing.T) {
	wrapped := fmt.Errorf("subscribe failed: %w", status.Error(codes.PermissionDenied, "no cap"))
	if got := GRPCDefaultClassifier(wrapped); got != RetryDecisionFatal {
		t.Fatalf("wrapped PermissionDenied classified as %v, want fatal", got)
	}
}

// TestGRPCDefaultClassifierNonGRPC keeps non-gRPC errors retryable so a
// transient io.EOF / net.OpError reconnects rather than killing the loop.
func TestGRPCDefaultClassifierNonGRPC(t *testing.T) {
	cases := []error{
		io.EOF,
		errors.New("plain"),
		&net.OpError{Op: "read", Err: errors.New("conn reset")},
	}
	for _, e := range cases {
		if got := GRPCDefaultClassifier(e); got != RetryDecisionRetry {
			t.Errorf("error %v classified as %v, want retry", e, got)
		}
	}
}

// TestGRPCDefaultClassifierNil makes the nil branch explicit so a future
// caller that forgets to short-circuit before the classifier still gets
// a sane decision (we treat nil as retry — i.e. "don't escalate").
func TestGRPCDefaultClassifierNil(t *testing.T) {
	if got := GRPCDefaultClassifier(nil); got != RetryDecisionRetry {
		t.Fatalf("nil err classified as %v, want retry", got)
	}
}

// TestLoopWithRetryClassNilClassifierMatchesLoop covers the strict-
// superset claim from retry_class.go: with classifier == nil,
// LoopWithRetryClass should retry forever and exit only on ctx cancel.
// We rely on TestLoopBackoffGrowsThenResetsOnSuccess in reconnector_test
// to lock down Loop's schedule; here we only verify the "doesn't exit
// on errors" part.
func TestLoopWithRetryClassNilClassifierMatchesLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := atomic.Int32{}
	done := make(chan error, 1)
	go func() {
		done <- LoopWithRetryClass(ctx, Config{
			Name:        "test.nil-classifier",
			Initial:     5 * time.Millisecond,
			Max:         20 * time.Millisecond,
			Multiplier:  2,
			JitterRatio: 0,
		}, nil, func(ctx context.Context) error {
			n := calls.Add(1)
			if n > 3 {
				<-ctx.Done()
				return ctx.Err()
			}
			return status.Error(codes.PermissionDenied, "would be fatal under default classifier")
		})
	}()

	// PermissionDenied with nil classifier must retry, not exit.
	time.Sleep(80 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled (nil classifier should retry forever)", err)
		}
	case <-time.After(time.Second):
		t.Fatal("LoopWithRetryClass did not return after cancel")
	}
	if got := calls.Load(); got < 2 {
		t.Errorf("attempt called %d times; expected retries before cancel", got)
	}
}

// TestLoopWithRetryClassFatalExits is the load-bearing test: a fatal
// classification must terminate the loop on the first failing attempt
// and surface a wrapped error the caller can errors.Is against.
func TestLoopWithRetryClassFatalExits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := atomic.Int32{}
	fatalErr := status.Error(codes.PermissionDenied, "missing cap")
	err := LoopWithRetryClass(ctx, Config{
		Name:        "test.fatal",
		Initial:     5 * time.Millisecond,
		Max:         20 * time.Millisecond,
		Multiplier:  2,
		JitterRatio: 0,
	}, GRPCDefaultClassifier, func(ctx context.Context) error {
		calls.Add(1)
		return fatalErr
	})
	if err == nil {
		t.Fatal("LoopWithRetryClass returned nil for a fatal classifier; want wrapped err")
	}
	if !errors.Is(err, fatalErr) {
		t.Fatalf("wrapped err = %v; want it to wrap %v", err, fatalErr)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("attempt called %d times; want exactly 1 (fatal must not retry)", got)
	}
	// Wrapper format check — keeps log greps stable.
	want := "stream test.fatal fatal:"
	if msg := err.Error(); len(msg) < len(want) || msg[:len(want)] != want {
		t.Errorf("err.Error()=%q; expected to start with %q", msg, want)
	}
}

// TestLoopWithRetryClassRetryableErrorReconnects asserts retryable
// errors are NOT promoted to fatal: a string of Unavailable failures
// should keep the loop alive so a server bounce can recover.
func TestLoopWithRetryClassRetryableErrorReconnects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := atomic.Int32{}
	done := make(chan error, 1)
	go func() {
		done <- LoopWithRetryClass(ctx, Config{
			Name:        "test.retryable",
			Initial:     5 * time.Millisecond,
			Max:         20 * time.Millisecond,
			Multiplier:  2,
			JitterRatio: 0,
		}, GRPCDefaultClassifier, func(ctx context.Context) error {
			calls.Add(1)
			return status.Error(codes.Unavailable, "host bouncing")
		})
	}()
	time.Sleep(60 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v; want context.Canceled (loop should keep retrying retryable errors)", err)
		}
	case <-time.After(time.Second):
		t.Fatal("LoopWithRetryClass did not return after cancel")
	}
	if got := calls.Load(); got < 2 {
		t.Errorf("attempt called %d times; expected multiple retries within 60ms", got)
	}
}

// TestLoopWithRetryClassNilOnSuccessDoesNotExit replicates the Loop
// semantics for nil returns: success resets backoff and the loop keeps
// going until ctx is cancelled. Classifier must NOT be consulted for
// nil errors (we don't want a stale classifier closure to kill a
// healthy stream).
func TestLoopWithRetryClassNilOnSuccessDoesNotExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	classifierCalls := atomic.Int32{}
	classifier := func(err error) RetryDecision {
		classifierCalls.Add(1)
		return RetryDecisionFatal // would kill loop if invoked
	}

	calls := atomic.Int32{}
	done := make(chan error, 1)
	go func() {
		done <- LoopWithRetryClass(ctx, Config{
			Name:        "test.nil-success",
			Initial:     5 * time.Millisecond,
			Max:         20 * time.Millisecond,
			Multiplier:  2,
			JitterRatio: 0,
		}, classifier, func(ctx context.Context) error {
			n := calls.Add(1)
			if n > 5 {
				<-ctx.Done()
				return ctx.Err()
			}
			return nil
		})
	}()
	time.Sleep(40 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v; want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("LoopWithRetryClass did not return after cancel")
	}
	if classifierCalls.Load() != 0 {
		t.Fatalf("classifier called %d times for nil errors; want 0", classifierCalls.Load())
	}
	if calls.Load() < 2 {
		t.Errorf("attempt called %d times; expected several iterations", calls.Load())
	}
}

// TestLoopWithRetryClassCtxCancelDuringSleep reproduces the historical
// Loop ctx-during-sleep exit so the new helper preserves the contract.
func TestLoopWithRetryClassCtxCancelDuringSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- LoopWithRetryClass(ctx, Config{
			Name:        "test.cancel-sleep",
			Initial:     200 * time.Millisecond,
			Max:         time.Second,
			Multiplier:  2,
			JitterRatio: 0,
		}, GRPCDefaultClassifier, func(ctx context.Context) error {
			return status.Error(codes.Unavailable, "blip")
		})
	}()

	// Let attempt return once so the loop is now sleeping; cancel
	// during sleep and ensure the loop exits promptly with ctx.Err().
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v; want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("LoopWithRetryClass did not return promptly after cancel during sleep")
	}
}
