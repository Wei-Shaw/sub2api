// Package traffic measures HTTP entity bodies, excluding HTTP headers, framing,
// TLS, and WebSocket messages. A nil count is unknown, not an empty body.
package traffic

import (
	"context"
	"sync"
	"time"
)

// Measurement describes one direction before and after content coding. Counts
// are observed entity bytes, not an estimate of the entire intended payload.
// An upstream response retains bytes already read even if it ends before HTTP
// EOF. Missing/unprovable dimensions stay unknown; a partial compressed body
// cannot supply a paired original size. Compressed describes content coding,
// not whether it saved bytes.
type Measurement struct {
	OriginalBytes *int64 `json:"original_bytes"`
	ActualBytes   *int64 `json:"actual_bytes"`
	Compressed    bool   `json:"compressed"`
}

// Ratio returns percent saved. Expansion is negative; empty/unknown originals
// have no meaningful ratio.
func (m *Measurement) Ratio() *float64 {
	if m == nil || m.OriginalBytes == nil || m.ActualBytes == nil || *m.OriginalBytes <= 0 {
		return nil
	}
	ratio := (1 - float64(*m.ActualBytes)/float64(*m.OriginalBytes)) * 100
	return &ratio
}

type Link struct {
	Request  *Measurement `json:"request"`
	Response *Measurement `json:"response"`
}

type Usage struct {
	Upstream   *Link `json:"upstream"`
	Downstream *Link `json:"downstream"`
}

type Direction uint8

const (
	UpstreamRequest Direction = iota
	UpstreamResponse
	DownstreamRequest
	DownstreamResponse
)

// Counter tracks a single body/attempt, not just the successful attempt. A
// dimension remains unknown until its observation is reliable. Outgoing bytes
// require write confirmation; received response bytes are reliable as consumed.
// Explicit invalidation is sticky.
type Counter struct {
	mu                               sync.Mutex
	original, actual                 int64
	originalComplete, actualComplete bool
	originalFailed, actualFailed     bool
	compressed                       bool
}

func (b *Counter) AddOriginal(n int) {
	if b != nil {
		b.mu.Lock()
		b.original += int64(n)
		b.mu.Unlock()
	}
}
func (b *Counter) AddActual(n int) {
	if b != nil {
		b.mu.Lock()
		b.actual += int64(n)
		b.mu.Unlock()
	}
}
func (b *Counter) CompleteOriginal() {
	if b != nil {
		b.mu.Lock()
		b.originalComplete = true
		b.mu.Unlock()
	}
}
func (b *Counter) CompleteActual() {
	if b != nil {
		b.mu.Lock()
		b.actualComplete = true
		b.mu.Unlock()
	}
}
func (b *Counter) FailOriginal() {
	if b != nil {
		b.mu.Lock()
		b.originalFailed = true
		b.mu.Unlock()
	}
}
func (b *Counter) FailActual() {
	if b != nil {
		b.mu.Lock()
		b.actualFailed = true
		b.mu.Unlock()
	}
}
func (b *Counter) SetCompressed() {
	if b != nil {
		b.mu.Lock()
		b.compressed = true
		b.mu.Unlock()
	}
}

// SetOriginal records bytes counted by the request decoder before any JSON or
// protocol transformation. It replaces, rather than adds, reread body counts.
func (b *Counter) SetOriginal(n int64) {
	if b != nil {
		b.mu.Lock()
		b.original = n
		b.originalComplete = true
		b.mu.Unlock()
	}
}

func (b *Counter) snapshot() *Measurement {
	b.mu.Lock()
	defer b.mu.Unlock()
	m := &Measurement{Compressed: b.compressed}
	if b.originalComplete && !b.originalFailed {
		n := b.original
		m.OriginalBytes = &n
	}
	if b.actualComplete && !b.actualFailed {
		n := b.actual
		m.ActualBytes = &n
	}
	return m
}

// Collector is request-scoped and safe to share with detached billing tasks.
// Finalize freezes all measurements before invoking callbacks outside the lock.
type Collector struct {
	mu               sync.Mutex
	bodies           [4][]*Counter
	finalized        bool
	excluded         bool
	upstreamExcluded bool
	finalizedAt      time.Time
	final            *Usage
	callbacks        []func(*Usage)
}

func NewCollector() *Collector { return &Collector{} }

func (c *Collector) Begin(direction Direction, compressed bool) *Counter {
	if c == nil || direction > DownstreamResponse {
		return nil
	}
	b := &Counter{compressed: compressed}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.finalized {
		c.bodies[direction] = append(c.bodies[direction], b)
	}
	return b
}

func (c *Collector) first(direction Direction) *Counter {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.bodies[direction]) == 0 {
		return nil
	}
	return c.bodies[direction][0]
}

// Exclude is used for hijacked/WebSocket sessions, whose messages are not HTTP
// entity bodies and must not be mixed into HTTP usage.
func (c *Collector) Exclude() {
	if c != nil {
		c.mu.Lock()
		c.excluded = true
		c.mu.Unlock()
		// A successful hijack ends HTTP measurement immediately. Drain existing
		// callbacks and let subsequent WS turns receive nil without waiting for
		// the potentially long-lived session to close.
		c.Finalize()
	}
}

// ExcludeUpstream marks requests that attempt a non-HTTP upstream (WebSocket).
// It is sticky across subsequent HTTP fallbacks, but preserves HTTP downstream.
func (c *Collector) ExcludeUpstream() {
	if c != nil {
		c.mu.Lock()
		c.upstreamExcluded = true
		c.mu.Unlock()
	}
}

// Adopt moves every measurement other collected into c. It exists for probes
// that were measured apart from the request only because they might be
// discarded: once such an attempt turns out to be the delivered request, its
// counters belong to the request. Bodies already wrapped by the transport keep
// updating the same counters after the move, so adoption must happen before the
// destination is finalized. The source is left empty, so adopting twice cannot
// double count.
//
// A finalized collector is never extended, and a finalized source is refused:
// its measurements were already published through OnFinalize and must not be
// published twice. Lock order is always source then destination.
func (c *Collector) Adopt(other *Collector) bool {
	if c == nil || other == nil || c == other {
		return false
	}
	other.mu.Lock()
	defer other.mu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	// An excluded source measured a non-HTTP session, not an entity body.
	if c.finalized || other.finalized || other.excluded || other.upstreamExcluded {
		return false
	}
	adopted := false
	for direction := range other.bodies {
		if len(other.bodies[direction]) == 0 {
			continue
		}
		c.bodies[direction] = append(c.bodies[direction], other.bodies[direction]...)
		other.bodies[direction] = nil
		adopted = true
	}
	return adopted
}

func (c *Collector) snapshotLocked() *Usage {
	if c.excluded {
		return nil
	}
	var directions [4]*Measurement
	for direction, bodies := range c.bodies {
		if c.upstreamExcluded && (direction == int(UpstreamRequest) || direction == int(UpstreamResponse)) {
			continue
		}
		if len(bodies) == 0 {
			continue
		}
		original, actual := int64(0), int64(0)
		m := &Measurement{OriginalBytes: &original, ActualBytes: &actual}
		for _, body := range bodies {
			part := body.snapshot()
			m.Compressed = m.Compressed || part.Compressed
			if part.OriginalBytes == nil {
				m.OriginalBytes = nil
			} else {
				original += *part.OriginalBytes
			}
			if part.ActualBytes == nil {
				m.ActualBytes = nil
			} else {
				actual += *part.ActualBytes
			}
		}
		directions[direction] = m
	}
	link := func(request, response *Measurement) *Link {
		if request == nil && response == nil {
			return nil
		}
		return &Link{Request: request, Response: response}
	}
	return &Usage{
		Upstream:   link(directions[UpstreamRequest], directions[UpstreamResponse]),
		Downstream: link(directions[DownstreamRequest], directions[DownstreamResponse]),
	}
}

func (c *Collector) Snapshot() *Usage {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.finalized {
		return Clone(c.final)
	}
	return c.snapshotLocked()
}

func (c *Collector) Finalize() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.finalized {
		c.mu.Unlock()
		return
	}
	c.final = c.snapshotLocked()
	c.finalizedAt = time.Now().UTC()
	c.finalized = true
	callbacks := c.callbacks
	c.callbacks = nil
	snapshot := c.final
	c.mu.Unlock()
	for _, callback := range callbacks {
		callback(Clone(snapshot))
	}
}

// Finalized reports whether callbacks can run immediately. A missing collector
// is already complete with unknown usage. This is only a scheduling hint;
// callers must still use OnFinalize to obtain the immutable snapshot.
func (c *Collector) Finalized() bool {
	if c == nil {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.finalized
}

// FinalizedAt is fixed at snapshot freeze, not at callback execution. It is
// zero before finalization (or for a missing collector).
func (c *Collector) FinalizedAt() time.Time {
	if c == nil {
		return time.Time{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.finalizedAt
}

func (c *Collector) OnFinalize(callback func(*Usage)) {
	if callback == nil {
		return
	}
	if c == nil {
		callback(nil)
		return
	}
	c.mu.Lock()
	if !c.finalized {
		c.callbacks = append(c.callbacks, callback)
		c.mu.Unlock()
		return
	}
	snapshot := Clone(c.final)
	c.mu.Unlock()
	callback(snapshot)
}

func Clone(u *Usage) *Usage {
	if u == nil {
		return nil
	}
	cloneMeasurement := func(m *Measurement) *Measurement {
		if m == nil {
			return nil
		}
		copy := *m
		if m.OriginalBytes != nil {
			n := *m.OriginalBytes
			copy.OriginalBytes = &n
		}
		if m.ActualBytes != nil {
			n := *m.ActualBytes
			copy.ActualBytes = &n
		}
		return &copy
	}
	cloneLink := func(link *Link) *Link {
		if link == nil {
			return nil
		}
		return &Link{Request: cloneMeasurement(link.Request), Response: cloneMeasurement(link.Response)}
	}
	return &Usage{Upstream: cloneLink(u.Upstream), Downstream: cloneLink(u.Downstream)}
}

type contextKey struct{}

func WithCollector(ctx context.Context, collector *Collector) context.Context {
	return context.WithValue(ctx, contextKey{}, collector)
}
func FromContext(ctx context.Context) *Collector {
	if ctx == nil {
		return nil
	}
	collector, _ := ctx.Value(contextKey{}).(*Collector)
	return collector
}
func Snapshot(ctx context.Context) *Usage                      { return FromContext(ctx).Snapshot() }
func AfterFinalize(ctx context.Context, callback func(*Usage)) { FromContext(ctx).OnFinalize(callback) }
func RequestCounter(ctx context.Context) *Counter              { return FromContext(ctx).first(DownstreamRequest) }
func ResponseCounter(ctx context.Context) *Counter             { return FromContext(ctx).first(DownstreamResponse) }
