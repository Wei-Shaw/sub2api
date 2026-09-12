package service

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"sync"
	"time"
)

const (
	codexObservationQueueSize   = 64
	codexObservationMaxWriters  = 256
	codexObservationMaxStates   = 16384
	codexObservationEnqueueWait = 100 * time.Millisecond
)

var errCodexObservationQueueFull = errors.New("Codex observation queue is full")

type codexObservationState struct {
	at     time.Time
	limits NormalizedCodexLimits
}
type codexObservationWrite struct {
	updates  map[string]any
	critical bool
	persist  func(context.Context, map[string]any) error
}
type codexObservationQueue struct{ pending []codexObservationWrite }

// Classification and admission share a lock so concurrent responses cannot
// classify in one order and start their database writes in the reverse order.
// Only accounts with pending work have a writer; both queues and writers are bounded.
type codexObservationGate struct {
	mu       sync.Mutex
	accounts map[int64]codexObservationState
	queues   map[int64]*codexObservationQueue
	changed  chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	closed   bool
}

func (g *codexObservationGate) classifyLocked(id int64, snapshot *OpenAICodexUsageSnapshot, now time.Time) (codexObservationState, bool, bool) {
	normalized := snapshot.Normalize()
	if normalized == nil {
		return codexObservationState{}, false, false
	}
	state := codexObservationState{at: codexSnapshotBaseTime(snapshot, now), limits: *normalized}
	previous, exists := g.accounts[id]
	if exists && !state.at.After(previous.at) {
		return state, false, false
	}
	critical := !exists || codexWindowBoundaryChanged(previous.at, state.at,
		previous.limits.Used5hPercent, normalized.Used5hPercent,
		previous.limits.Reset5hSeconds, normalized.Reset5hSeconds,
		previous.limits.Window5hMinutes, normalized.Window5hMinutes) ||
		codexWindowBoundaryChanged(previous.at, state.at,
			previous.limits.Used7dPercent, normalized.Used7dPercent,
			previous.limits.Reset7dSeconds, normalized.Reset7dSeconds,
			previous.limits.Window7dMinutes, normalized.Window7dMinutes)
	return state, true, critical
}

func (g *codexObservationGate) rememberLocked(id int64, state codexObservationState) {
	if g.accounts == nil {
		g.accounts = make(map[int64]codexObservationState)
	}
	// Evict one inactive entry instead of scanning every account on every response.
	// An evicted account's next observation is critical and therefore re-establishes its state.
	if _, exists := g.accounts[id]; !exists && len(g.accounts) >= codexObservationMaxStates {
		for candidate := range g.accounts {
			if g.queues[candidate] == nil {
				delete(g.accounts, candidate)
				break
			}
		}
	}
	g.accounts[id] = state
}

func (g *codexObservationGate) classify(id int64, snapshot *OpenAICodexUsageSnapshot, now time.Time) (bool, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	state, fresh, critical := g.classifyLocked(id, snapshot, now)
	if fresh {
		g.rememberLocked(id, state)
	}
	return fresh, critical
}

// Normal traffic never waits. Critical observations wait at most 100ms for
// bounded capacity; a rejected observation does not advance classification.
// The warning includes the quota-only payload so an operator can recover the
// observation if overload exhausts even this bounded wait.
func (g *codexObservationGate) enqueue(ctx context.Context, id int64, snapshot *OpenAICodexUsageSnapshot, now time.Time, updates map[string]any, allow func() bool, persist func(context.Context, map[string]any) error) error {
	var deadline <-chan time.Time
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for {
		g.mu.Lock()
		if g.closed {
			g.mu.Unlock()
			return context.Canceled
		}
		state, fresh, critical := g.classifyLocked(id, snapshot, now)
		if !fresh {
			g.mu.Unlock()
			if timer != nil {
				slog.Error("openai_codex_observation_superseded_while_waiting", "account_id", id, "quota_observation", updates)
				return errCodexObservationQueueFull
			}
			return nil
		}
		if g.changed == nil {
			g.changed = make(chan struct{})
		}
		queue := g.queues[id]
		if (queue != nil && len(queue.pending) < codexObservationQueueSize) || (queue == nil && len(g.queues) < codexObservationMaxWriters) {
			// Skipped observations never advance accepted state. Otherwise an
			// unqueued 100% sample could prevent retrying a failed terminal write.
			if !critical && !allow() {
				g.mu.Unlock()
				return nil
			}
			start := queue == nil
			if start {
				if g.queues == nil {
					g.queues = make(map[int64]*codexObservationQueue)
				}
				if g.ctx == nil {
					g.ctx, g.cancel = context.WithCancel(context.Background())
				}
				queue = &codexObservationQueue{}
				g.queues[id] = queue
			}
			// Advance the existing throttle for admitted critical observations as well.
			if critical {
				allow()
			}
			g.rememberLocked(id, state)
			queue.pending = append(queue.pending, codexObservationWrite{updates: updates, critical: critical, persist: persist})
			g.mu.Unlock()
			if start {
				go g.drain(id, queue)
			}
			return nil
		}
		changed := g.changed
		g.mu.Unlock()
		if !critical {
			slog.Warn("openai_codex_observation_overload", "account_id", id, "critical", false, "observed_at", snapshot.UpdatedAt)
			return errCodexObservationQueueFull
		}
		if timer == nil {
			timer = time.NewTimer(codexObservationEnqueueWait)
			deadline = timer.C
		}
		select {
		case <-changed:
		case <-ctx.Done():
			slog.Error("openai_codex_observation_not_queued", "account_id", id, "error", ctx.Err(), "quota_observation", updates)
			return ctx.Err()
		case <-deadline:
			slog.Error("openai_codex_observation_not_queued", "account_id", id, "error", errCodexObservationQueueFull, "quota_observation", updates)
			return errCodexObservationQueueFull
		}
	}
}

func (g *codexObservationGate) signalLocked() {
	if g.changed != nil {
		close(g.changed)
	}
	g.changed = make(chan struct{})
}

func (g *codexObservationGate) drain(id int64, queue *codexObservationQueue) {
	for {
		g.mu.Lock()
		if g.closed || len(queue.pending) == 0 {
			delete(g.queues, id)
			g.signalLocked()
			g.mu.Unlock()
			return
		}
		write := queue.pending[0]
		queue.pending[0] = codexObservationWrite{}
		queue.pending = queue.pending[1:]
		baseCtx := g.ctx
		g.signalLocked()
		g.mu.Unlock()

		attempts := 1
		if write.critical {
			attempts = 3
		}
		var err error
		for attempt := 0; attempt < attempts; attempt++ {
			if attempt > 0 {
				timer := time.NewTimer(time.Duration(attempt) * 50 * time.Millisecond)
				select {
				case <-timer.C:
				case <-baseCtx.Done():
					timer.Stop()
				}
			}
			if baseCtx.Err() != nil {
				err = baseCtx.Err()
				break
			}
			writeCtx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
			err = write.persist(writeCtx, write.updates)
			cancel()
			if err == nil {
				break
			}
		}
		if err != nil {
			slog.Error("openai_codex_observation_persist_failed", "account_id", id, "critical", write.critical, "error", err, "quota_observation", write.updates)
			// Forget failed state so another observation at the same percentage can
			// retry as critical, unless newer accepted observations already supersede it.
			g.mu.Lock()
			if state, ok := g.accounts[id]; ok && state.at.UTC().Format(time.RFC3339Nano) == write.updates["codex_usage_updated_at"] {
				delete(g.accounts, id)
			}
			g.mu.Unlock()
		}
	}
}

// close cancels in-flight I/O and backoff without waiting for blocked writers.
// Repository methods must honor their context, as they do on the request path.
func (g *codexObservationGate) close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return
	}
	g.closed = true
	if g.cancel != nil {
		g.cancel()
	}
	for id, queue := range g.queues {
		if len(queue.pending) > 0 {
			slog.Warn("openai_codex_observations_shutdown", "account_id", id, "pending", len(queue.pending))
			queue.pending = nil
		}
	}
	g.signalLocked()
}

func codexWindowBoundaryChanged(previousAt, at time.Time, previousUsed, used *float64, previousReset, reset, previousMinutes, minutes *int) bool {
	if used != nil && !math.IsNaN(*used) && !math.IsInf(*used, 0) {
		if previousUsed == nil || (*used >= 100 && *previousUsed < 100) || *used < *previousUsed {
			return true
		}
	}
	if minutes != nil && (previousMinutes == nil || *minutes != *previousMinutes) {
		return true
	}
	if reset != nil {
		if previousReset == nil {
			return true
		}
		delta := at.Add(time.Duration(*reset) * time.Second).Sub(previousAt.Add(time.Duration(*previousReset) * time.Second))
		return delta > 2*time.Second || delta < -2*time.Second
	}
	return false
}
