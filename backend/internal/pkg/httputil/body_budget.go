package httputil

import (
	"context"
	"errors"
	"sync"
)

// ErrRequestBodyMemoryBudget means this instance cannot buffer another request
// body yet. It is transient overload, not a malformed or oversized request.
var ErrRequestBodyMemoryBudget = errors.New("request body memory budget exhausted")

// BodyMemoryBudget bounds buffered ingress allocations across requests. It
// counts chunks, their contiguous copy, decoded output and the decoder window;
// it is not a bound on the application's total heap or protocol conversion.
type BodyMemoryBudget struct {
	mu    sync.Mutex
	limit int64
	used  int64
}

func NewBodyMemoryBudget(limit int64) *BodyMemoryBudget {
	return &BodyMemoryBudget{limit: limit}
}

type bodyMemoryReservation struct {
	mu     sync.Mutex
	budget *BodyMemoryBudget
	bytes  int64
	closed bool
}

type bodyMemoryReservationKey struct{}

// WithRequestBodyMemoryBudget retains final body reservations until the HTTP
// handler returns, including while a buffered request waits for a user slot.
// Read/decode/normalize helpers release temporary allocations when no longer
// owned. No goroutines or wait queue are created on exhaustion.
func WithRequestBodyMemoryBudget(ctx context.Context, budget *BodyMemoryBudget) (context.Context, func()) {
	if budget == nil || budget.limit <= 0 || ctx.Value(bodyMemoryReservationKey{}) != nil {
		return ctx, func() {}
	}
	r := &bodyMemoryReservation{budget: budget}
	return context.WithValue(ctx, bodyMemoryReservationKey{}, r), r.release
}

// WithRequestBodyMemoryScope opens a nested reservation on the request's budget
// for representations that die before the handler returns, such as one upstream
// attempt's rewritten body. Release returns only the scope's bytes and is
// idempotent. Without a request budget in ctx it is a no-op; a scope opened after
// the request released its budget rejects every reservation.
func WithRequestBodyMemoryScope(ctx context.Context) (context.Context, func()) {
	parent, _ := ctx.Value(bodyMemoryReservationKey{}).(*bodyMemoryReservation)
	if parent == nil {
		return ctx, func() {}
	}
	parent.mu.Lock()
	closed := parent.closed
	parent.mu.Unlock()
	r := &bodyMemoryReservation{budget: parent.budget, closed: closed}
	return context.WithValue(ctx, bodyMemoryReservationKey{}, r), r.release
}

// DetachRequestBodyMemory opens a reservation on the request's budget for work
// that keeps request-derived buffers after the handler returns, such as an
// accepted asynchronous task. It takes over up to n bytes the request already
// holds for the retained body and reserves only the remainder, so the body is
// never charged twice; reservations made through the returned ctx accumulate on
// it. The handler's release does not affect it, so the caller owns release and
// must call it when the work ends. Without a request budget in ctx it is a no-op.
func DetachRequestBodyMemory(ctx context.Context, n int64) (context.Context, func(), error) {
	parent, _ := ctx.Value(bodyMemoryReservationKey{}).(*bodyMemoryReservation)
	if parent == nil {
		return ctx, func() {}, nil
	}
	detached, release := WithRequestBodyMemoryScope(ctx)
	r, _ := detached.Value(bodyMemoryReservationKey{}).(*bodyMemoryReservation)
	parent.mu.Lock()
	moved := int64(0)
	if !parent.closed && !r.closed {
		moved = min(max(n, 0), parent.bytes)
		parent.bytes -= moved
	}
	parent.mu.Unlock()
	r.mu.Lock()
	r.bytes += moved
	r.mu.Unlock()
	if err := reserveRequestBodyMemory(detached, n-moved); err != nil {
		// Hand the body back: the handler still owns it until it returns.
		r.mu.Lock()
		r.bytes -= moved
		r.mu.Unlock()
		parent.mu.Lock()
		if parent.closed {
			parent.budget.mu.Lock()
			parent.budget.used -= moved
			parent.budget.mu.Unlock()
		} else {
			parent.bytes += moved
		}
		parent.mu.Unlock()
		release()
		return ctx, func() {}, err
	}
	return detached, release, nil
}

func (r *bodyMemoryReservation) release() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	r.budget.mu.Lock()
	r.budget.used -= r.bytes
	r.budget.mu.Unlock()
	r.bytes = 0
}

// ReserveRequestBodyMemory reserves n bytes before allocating another buffered
// representation of the request body. The reservation shares the ingress budget
// and is retained until the release function of the handler, or of the innermost
// WithRequestBodyMemoryScope in ctx, runs. Without a budget in
// ctx it is a no-op. Callers must propagate exhaustion instead of allocating
// through an unbudgeted fallback. Unlike ingress reads, it ignores ctx
// cancellation: callers that keep working after a client disconnect own that
// policy.
func ReserveRequestBodyMemory(ctx context.Context, n int64) error {
	return reserveBodyMemory(ctx, n)
}

func reserveRequestBodyMemory(ctx context.Context, n int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return reserveBodyMemory(ctx, n)
}

func reserveBodyMemory(ctx context.Context, n int64) error {
	r, _ := ctx.Value(bodyMemoryReservationKey{}).(*bodyMemoryReservation)
	if r == nil || n <= 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrRequestBodyMemoryBudget
	}
	r.budget.mu.Lock()
	defer r.budget.mu.Unlock()
	if n > r.budget.limit-r.budget.used {
		return ErrRequestBodyMemoryBudget
	}
	r.budget.used += n
	r.bytes += n
	return nil
}

// ReleaseRequestBodyMemory returns up to n bytes previously reserved through ctx,
// for a representation that was never allocated or came out smaller than its
// reservation. It never refunds another request's or scope's bytes.
func ReleaseRequestBodyMemory(ctx context.Context, n int64) {
	releaseRequestBodyMemory(ctx, n)
}

// releaseRequestBodyMemory refunds storage that a helper no longer owns. Each
// allocation must be released once, or left for the handler's final cleanup.
// Cleanup may race with a cancelled read, so a closed reservation is a no-op.
func releaseRequestBodyMemory(ctx context.Context, n int64) {
	r, _ := ctx.Value(bodyMemoryReservationKey{}).(*bodyMemoryReservation)
	if r == nil || n <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	n = min(n, r.bytes)
	r.budget.mu.Lock()
	r.budget.used -= n
	r.budget.mu.Unlock()
	r.bytes -= n
}
