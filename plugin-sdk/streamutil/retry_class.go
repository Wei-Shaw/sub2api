// retry_class.go — extends Loop with a pluggable error classifier so
// stream clients (events / settings / jobs / log_remote / host
// extensions) can mark certain attempt errors as "fatal — stop
// reconnecting and surface to the caller" without each site re-
// implementing the open / drain / classify state machine.
//
// Why this exists
// ----------------
// streamutil.Loop (T6) absorbs every attempt error and reconnects
// forever, exiting only when ctx is cancelled. That is correct for
// transport blips (Unavailable / DeadlineExceeded / EOF) but produces
// an unbounded reconnect storm when the failure is a configuration
// error the operator must fix — e.g. a host that returns
// PermissionDenied because the plugin manifest did not declare the
// required capability, or Unimplemented because plugin and host
// versions are mismatched.
//
// Before this file, events.go (T25) worked around the limitation by
// inspecting the error in attempt, calling cancel() on a private
// context, and returning nil from attempt to coax Loop into exiting
// via ctx.Err(). That smuggled the retry policy into the attempt
// closure and made the control flow non-obvious.
//
// LoopWithRetryClass moves the policy back where it belongs (the
// reconnector) and lets attempt stay a pure "open + drain" function.
// 抽象 (4)：把 events 一处的特殊逻辑提升为可复用抽象，jobs / settings /
// log_remote 现在能用同一个 GRPCDefaultClassifier 拒绝重试 fatal 错误，
// 不必各自实现 cancel-hack。
package streamutil

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RetryDecision describes how Loop should handle an attempt error.
//
// RetryDecisionRetry is the default (zero value) so a classifier that
// only knows about a few fatal codes can return RetryDecisionRetry as
// the catch-all without explicit reasoning about transport-level
// errors.
type RetryDecision int

const (
	// RetryDecisionRetry — wait the configured backoff and re-invoke
	// attempt. Equivalent to the historical Loop behaviour.
	RetryDecisionRetry RetryDecision = iota
	// RetryDecisionFatal — stop the loop immediately and return the
	// (wrapped) error to the caller. The caller is expected to log /
	// surface the failure rather than restart the loop.
	RetryDecisionFatal
)

// RetryClassifier inspects an attempt error and decides whether the
// loop should retry or exit. Called only for non-nil errors; a nil
// error always resets backoff and continues the loop (handled inside
// LoopWithRetryClass before classifier is consulted).
//
// classifier == nil is permitted: LoopWithRetryClass then behaves
// exactly like Loop (every error is retryable). This keeps the new
// helper a strict superset of Loop and lets us rewrite Loop as a thin
// wrapper without behaviour changes for existing callers.
type RetryClassifier func(err error) RetryDecision

// LoopWithRetryClass runs attempt repeatedly with reconnect backoff
// and consults classifier for every non-nil attempt error.
//
// Behaviour summary:
//   - attempt returns nil → reset backoff to Initial, immediately
//     re-invoke attempt (same as Loop).
//   - attempt returns err and classifier == nil → retry with backoff
//     (same as Loop).
//   - attempt returns err and classifier(err) == RetryDecisionRetry →
//     log + sleep backoff + re-invoke (same as Loop).
//   - attempt returns err and classifier(err) == RetryDecisionFatal →
//     log at error level, return fmt.Errorf("stream %s fatal: %w",
//     cfg.Name, err). The caller can errors.Is / errors.As against
//     the wrapped err.
//   - ctx cancelled at any point → return ctx.Err() (same as Loop).
//
// Loop in this package is now a thin wrapper around this function with
// classifier == nil; existing callers do not need to change.
func LoopWithRetryClass(
	ctx context.Context,
	cfg Config,
	classifier RetryClassifier,
	attempt func(context.Context) error,
) error {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	backoff := cfg.Initial
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := attempt(ctx)
		if err == nil {
			backoff = cfg.Initial
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if classifier != nil && classifier(err) == RetryDecisionFatal {
			logger.Error("stream attempt fatal; not retrying",
				"name", cfg.Name, "err", err)
			return fmt.Errorf("stream %s fatal: %w", cfg.Name, err)
		}
		delay := jitter(backoff, cfg.JitterRatio)
		logger.Warn("stream attempt failed; will reconnect",
			"name", cfg.Name, "backoff", delay, "err", err)
		if !sleep(ctx, delay) {
			return ctx.Err()
		}
		backoff = nextBackoff(backoff, cfg.Multiplier, cfg.Max)
	}
}

// GRPCDefaultClassifier marks the standard "operator must fix" gRPC
// status codes as fatal:
//
//   - codes.Unimplemented — server does not expose this RPC. Retrying
//     will not change that; either the host is too old or the plugin
//     is too new. Surface immediately so the operator can update one
//     side.
//   - codes.PermissionDenied — caller lacks the capability. Plugin
//     manifest needs updating; reconnect storm helps no one.
//   - codes.InvalidArgument — request contents are malformed (e.g.
//     undeclared event type, unknown setting key). Code change needed.
//   - codes.Unauthenticated — credentials invalid / expired in a way
//     the SDK cannot recover from. The host emits this for principal-
//     mismatch checks that will not pass on retry.
//
// Every other status (including codes.Unavailable, DeadlineExceeded,
// Internal, ResourceExhausted, Aborted) is classified as retryable.
// Non-gRPC errors (io.EOF, net.OpError, etc.) are also retryable —
// they signal a transient transport blip rather than a misconfiguration.
//
// 复用 (1)：events / settings / jobs / log_remote / host extension
// clients 都可以共用这一份判断；没人需要再 copy 同一份 switch 表。
func GRPCDefaultClassifier(err error) RetryDecision {
	if err == nil {
		return RetryDecisionRetry
	}
	st, ok := status.FromError(err)
	if !ok {
		// errors.As path for wrapped status errors. status.FromError
		// only unwraps directly-wrapped statuses; manually walking the
		// chain catches `fmt.Errorf("...: %w", statusErr)` callers.
		var se interface{ GRPCStatus() *status.Status }
		if errors.As(err, &se) {
			st = se.GRPCStatus()
			ok = st != nil
		}
	}
	if !ok {
		return RetryDecisionRetry
	}
	switch st.Code() {
	case codes.Unimplemented,
		codes.PermissionDenied,
		codes.InvalidArgument,
		codes.Unauthenticated:
		return RetryDecisionFatal
	default:
		return RetryDecisionRetry
	}
}
