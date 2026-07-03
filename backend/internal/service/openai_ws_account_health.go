package service

import (
	"sync"
	"time"
)

const (
	openAIWSAccountHealthFailureWindow = 2 * time.Minute
	openAIWSAccountHealthPenaltyStep   = 10
	openAIWSAccountHealthMaxFailures   = 5
)

type openAIWSAccountHealthTracker struct {
	mu     sync.Mutex
	now    func() time.Time
	events map[int64][]time.Time
}

func newOpenAIWSAccountHealthTracker() *openAIWSAccountHealthTracker {
	return &openAIWSAccountHealthTracker{
		now:    time.Now,
		events: make(map[int64][]time.Time),
	}
}

func (t *openAIWSAccountHealthTracker) RecordPreflightFailure(accountID int64) int {
	if t == nil || accountID <= 0 {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.currentTime()
	recent := t.recentFailuresLocked(accountID, now)
	recent = append(recent, now)
	t.events[accountID] = recent
	return len(recent)
}

func (t *openAIWSAccountHealthTracker) PriorityPenalty(accountID int64) int {
	if t == nil || accountID <= 0 {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	failures := len(t.recentFailuresLocked(accountID, t.currentTime()))
	if failures == 0 {
		delete(t.events, accountID)
		return 0
	}
	if failures > openAIWSAccountHealthMaxFailures {
		failures = openAIWSAccountHealthMaxFailures
	}
	return failures * openAIWSAccountHealthPenaltyStep
}

func (t *openAIWSAccountHealthTracker) currentTime() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

func (t *openAIWSAccountHealthTracker) recentFailuresLocked(accountID int64, now time.Time) []time.Time {
	failures := t.events[accountID]
	if len(failures) == 0 {
		return nil
	}
	cutoff := now.Add(-openAIWSAccountHealthFailureWindow)
	keepFrom := 0
	for keepFrom < len(failures) && failures[keepFrom].Before(cutoff) {
		keepFrom++
	}
	if keepFrom > 0 {
		failures = append([]time.Time(nil), failures[keepFrom:]...)
		t.events[accountID] = failures
	}
	return failures
}

func openAIWSInt64Ptr(v int64) *int64 {
	return &v
}
