package service

import (
	"testing"
	"time"
)

func TestQuotaAnchorMarker(t *testing.T) {
	tests := []struct {
		name    string
		extra   map[string]any
		marked  bool
		enabled bool
	}{
		{name: "missing", extra: nil},
		{name: "true", extra: map[string]any{userPlatformQuotaAnchorKey: true}, marked: true, enabled: true},
		{name: "false", extra: map[string]any{userPlatformQuotaAnchorKey: false}, marked: true},
		{name: "string true", extra: map[string]any{userPlatformQuotaAnchorKey: "true"}, marked: true, enabled: true},
		{name: "invalid string", extra: map[string]any{userPlatformQuotaAnchorKey: "yes"}, marked: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			marked, enabled := quotaAnchorMarker(tt.extra)
			if marked != tt.marked || enabled != tt.enabled {
				t.Fatalf("quotaAnchorMarker() = (%v, %v), want (%v, %v)", marked, enabled, tt.marked, tt.enabled)
			}
		})
	}
}

func TestSevenDaySnapshotTransition(t *testing.T) {
	resetAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	previous, current, gotResetAt, ok := sevenDaySnapshotTransition(
		map[string]any{"codex_7d_used_percent": 37.5},
		map[string]any{
			"codex_7d_used_percent": 0,
			"codex_7d_reset_at":     resetAt.Format(time.RFC3339),
		},
	)
	if !ok {
		t.Fatal("transition should be recognized")
	}
	if previous != 37.5 || current != 0 {
		t.Fatalf("usage transition = (%v, %v), want (37.5, 0)", previous, current)
	}
	if gotResetAt == nil || !gotResetAt.Equal(resetAt) {
		t.Fatalf("reset time = %v, want %v", gotResetAt, resetAt)
	}
}

func TestSevenDaySnapshotTransitionRequiresBothValues(t *testing.T) {
	_, _, _, ok := sevenDaySnapshotTransition(
		map[string]any{"codex_7d_used_percent": 10},
		map[string]any{},
	)
	if ok {
		t.Fatal("transition should not be recognized without the current value")
	}
}
