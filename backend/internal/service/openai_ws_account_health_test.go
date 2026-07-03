package service

import (
	"testing"
	"time"
)

func TestOpenAIWSAccountHealthTrackerPenaltyExpires(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tracker := newOpenAIWSAccountHealthTracker()
	tracker.now = func() time.Time { return now }

	if got := tracker.PriorityPenalty(68); got != 0 {
		t.Fatalf("initial penalty = %d, want 0", got)
	}

	if got := tracker.RecordPreflightFailure(68); got != 1 {
		t.Fatalf("failure count = %d, want 1", got)
	}
	if got := tracker.PriorityPenalty(68); got != openAIWSAccountHealthPenaltyStep {
		t.Fatalf("penalty after first failure = %d, want %d", got, openAIWSAccountHealthPenaltyStep)
	}

	for i := 0; i < openAIWSAccountHealthMaxFailures+2; i++ {
		tracker.RecordPreflightFailure(68)
	}
	if got := tracker.PriorityPenalty(68); got != openAIWSAccountHealthMaxFailures*openAIWSAccountHealthPenaltyStep {
		t.Fatalf("capped penalty = %d, want %d", got, openAIWSAccountHealthMaxFailures*openAIWSAccountHealthPenaltyStep)
	}

	now = now.Add(openAIWSAccountHealthFailureWindow + time.Second)
	if got := tracker.PriorityPenalty(68); got != 0 {
		t.Fatalf("expired penalty = %d, want 0", got)
	}
}
