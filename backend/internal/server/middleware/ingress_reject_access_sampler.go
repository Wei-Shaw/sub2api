package middleware

import (
	"sync"
	"time"
)

const (
	ingressRejectAccessLogLimit       = 20
	ingressRejectAccessLogWindow      = time.Second
	ingressRejectDroppedSummaryPeriod = 30 * time.Second
)

type ingressRejectAccessSampler struct {
	mu            sync.Mutex
	limit         int
	window        time.Duration
	summaryPeriod time.Duration
	windowStart   time.Time
	emitted       int
	dropped       uint64
	lastSummary   time.Time
}

func newIngressRejectAccessSampler(limit int, window, summaryPeriod time.Duration) *ingressRejectAccessSampler {
	return &ingressRejectAccessSampler{limit: limit, window: window, summaryPeriod: summaryPeriod}
}

// allow applies one process-wide fixed-window budget. It stores no attacker
// dimensions, so memory remains constant even for rotating keys and addresses.
func (s *ingressRejectAccessSampler) allow(now time.Time) (allowed bool, droppedSummary uint64) {
	if s == nil || s.limit <= 0 || s.window <= 0 {
		return false, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.windowStart.IsZero() || now.Sub(s.windowStart) >= s.window || now.Before(s.windowStart) {
		s.windowStart = now
		s.emitted = 0
	}
	if s.emitted < s.limit {
		s.emitted++
		return true, 0
	}
	s.dropped++
	if s.summaryPeriod > 0 && (s.lastSummary.IsZero() || now.Sub(s.lastSummary) >= s.summaryPeriod) {
		droppedSummary = s.dropped
		s.dropped = 0
		s.lastSummary = now
	}
	return false, droppedSummary
}

var globalIngressRejectAccessSampler = newIngressRejectAccessSampler(
	ingressRejectAccessLogLimit,
	ingressRejectAccessLogWindow,
	ingressRejectDroppedSummaryPeriod,
)
