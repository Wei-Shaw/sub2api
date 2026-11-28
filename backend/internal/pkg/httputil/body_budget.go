package httputil

import (
	"context"
	"errors"
	"math"
	"sync"
)

// ErrRequestBodyMemoryBudget means this instance cannot buffer another request
// body yet. It is transient overload, not a malformed or oversized request.
var ErrRequestBodyMemoryBudget = errors.New("request body memory budget exhausted")

// ErrRequestBodyMemoryTooLarge means processing the request needs more than this
// instance's whole budget, so it could not be admitted even when idle. Retrying
// cannot succeed; it is reported as 413 rather than transient overload.
var ErrRequestBodyMemoryTooLarge = errors.New("request body needs more processing memory than this instance allows")

// BodyMemoryBudget shares ingress allocations and explicitly reserved processing
// workspace across requests. It counts chunks, contiguous copies, decoded output
// and decoder windows; processing admission adds conservative expansion headroom.
// It is not a hard bound on the application's total heap or RSS.
type BodyMemoryBudget struct {
	mu    sync.Mutex
	limit int64
	used  int64
}

func NewBodyMemoryBudget(limit int64) *BodyMemoryBudget {
	return &BodyMemoryBudget{limit: limit}
}

// Used reports the bytes currently reserved across all requests.
func (b *BodyMemoryBudget) Used() int64 {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used
}

type bodyMemoryReservation struct {
	mu              sync.Mutex
	budget          *BodyMemoryBudget
	bytes           int64
	processingBytes int64
	// processingSettled is what processingBytes drops to once the transformed
	// body has been handed to the upstream: one body-sized outgoing copy.
	processingSettled int64
	closed            bool
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

// ReserveJSONProcessingMemory admits the synchronous JSON processing peak
// before decoding, content-derived session hashing, audit or rewriting.
// Ingress already accounts for the original body. Reserve six additional body
// lengths for decoded strings, decoder/encoder growth and simultaneous
// representations, plus structural overhead for object/array-heavy JSON; the
// live peak of an OAuth Responses rewrite is about five extra body lengths for
// both image-heavy and transcript-heavy bodies. This is admission headroom, not
// a hard RSS bound; the auto budget leaves the other three quarters for the
// rest of the process and GC. Repeated calls in the same reservation only raise
// its high-water mark, so retries do not accumulate charges. The charge lives
// until that scope ends or SettleJSONProcessingMemory lowers it.
//
// A request whose peak cannot fit even in an idle budget gets
// ErrRequestBodyMemoryTooLarge instead of ErrRequestBodyMemoryBudget.
func ReserveJSONProcessingMemory(ctx context.Context, body []byte) error {
	r, _ := ctx.Value(bodyMemoryReservationKey{}).(*bodyMemoryReservation)
	if r == nil || len(body) == 0 {
		return nil
	}
	const copies, perStructure = int64(6), int64(128)
	if int64(len(body)) > math.MaxInt64/copies {
		return ErrRequestBodyMemoryTooLarge
	}
	n := int64(len(body)) * copies
	quoted := false
	for i := 0; i < len(body); i++ {
		b := body[i]
		if quoted {
			switch b {
			case '\\':
				i++
			case '"':
				quoted = false
			}
			continue
		}
		if b == '"' {
			quoted = true
			continue
		}
		switch b {
		case '{', '[', ':', ',':
			if n > math.MaxInt64-perStructure {
				return ErrRequestBodyMemoryTooLarge
			}
			n += perStructure
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrRequestBodyMemoryBudget
	}
	settled := min(int64(len(body)), n)
	if n <= r.processingBytes {
		r.processingSettled = max(r.processingSettled, settled)
		return nil
	}
	delta := n - r.processingBytes
	r.budget.mu.Lock()
	defer r.budget.mu.Unlock()
	if held := max(r.bytes-r.processingBytes, 0); n > r.budget.limit-held {
		return ErrRequestBodyMemoryTooLarge
	}
	if delta > r.budget.limit-r.budget.used {
		return ErrRequestBodyMemoryBudget
	}
	r.budget.used += delta
	r.bytes += delta
	r.processingBytes = n
	r.processingSettled = max(r.processingSettled, settled)
	return nil
}

// SettleJSONProcessingMemory lowers the processing reservation in ctx to one
// body length once the rewritten body has been handed to the upstream. After
// that point a streaming request keeps only the ingress body and the outgoing
// copy alive, so holding the transform peak for the whole stream would starve
// concurrent requests. Later ReserveJSONProcessingMemory calls raise it again.
// Without a processing reservation in ctx it is a no-op.
func SettleJSONProcessingMemory(ctx context.Context) {
	if ctx == nil {
		return
	}
	r, _ := ctx.Value(bodyMemoryReservationKey{}).(*bodyMemoryReservation)
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.processingBytes <= r.processingSettled {
		return
	}
	delta := min(r.processingBytes-r.processingSettled, r.bytes)
	r.budget.mu.Lock()
	r.budget.used -= delta
	r.budget.mu.Unlock()
	r.bytes -= delta
	r.processingBytes = r.processingSettled
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
