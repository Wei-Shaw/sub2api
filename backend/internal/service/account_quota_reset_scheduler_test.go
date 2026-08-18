package service

import (
	"testing"
	"time"
)

func TestResolveAccountQuotaResetClaudePrefers7d(t *testing.T) {
	now := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	fiveH := now.Add(time.Hour)
	sevenD := now.Add(4 * 24 * time.Hour)
	account := &Account{Platform: PlatformAnthropic, SessionWindowEnd: &fiveH, Extra: map[string]any{
		"passive_usage_7d_reset":       float64(sevenD.Unix()),
		"passive_usage_7d_utilization": 0.8,
		"passive_usage_sampled_at":     now.Format(time.RFC3339),
	}}
	snapshot := ResolveAccountQuotaReset(account, PlatformAnthropic, "claude-sonnet-4-6", "auto", now)
	if snapshot.Window != "7d" || snapshot.ResetAt == nil || !snapshot.ResetAt.Equal(sevenD) {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestResolveAccountQuotaResetFableUsesModelSpecificWindow(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	fable := now.Add(6 * time.Hour)
	normal := now.Add(2 * time.Hour)
	account := &Account{Platform: PlatformAnthropic, SessionWindowEnd: &normal, Extra: map[string]any{
		"passive_usage_7d_oi_reset": float64(fable.Unix()),
		"passive_usage_sampled_at":  now.Format(time.RFC3339),
	}}
	snapshot := ResolveAccountQuotaReset(account, PlatformAnthropic, "claude-fable-5", "auto", now)
	if snapshot.Window != "7d_oi" || snapshot.ResetAt == nil {
		t.Fatalf("expected fable 7d_oi snapshot, got %+v", snapshot)
	}
}

func TestResolveAccountQuotaResetStaleAndExpiredAreUnknown(t *testing.T) {
	now := time.Now().UTC()
	account := &Account{Platform: PlatformOpenAI, Extra: map[string]any{
		"codex_7d_reset_at":      now.Add(time.Hour).Format(time.RFC3339),
		"codex_usage_updated_at": now.Add(-time.Hour).Format(time.RFC3339),
	}}
	stale := ResolveAccountQuotaReset(account, PlatformOpenAI, "", "7d", now)
	if stale.ResetAt != nil || !stale.Stale {
		t.Fatalf("expected stale reset to be ignored, got %+v", stale)
	}
	account.Extra["codex_usage_updated_at"] = now.Format(time.RFC3339)
	account.Extra["codex_7d_reset_at"] = now.Add(-time.Minute).Format(time.RFC3339)
	expired := ResolveAccountQuotaReset(account, PlatformOpenAI, "", "7d", now)
	if expired.ResetAt != nil {
		t.Fatalf("expected expired reset to be ignored, got %+v", expired)
	}
}

func TestResolveAccountQuotaResetKeepsUtilizationForTieBreaking(t *testing.T) {
	now := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	reset := now.Add(24 * time.Hour)
	account := &Account{Platform: PlatformAnthropic, Extra: map[string]any{
		"passive_usage_7d_reset":       reset.Unix(),
		"passive_usage_7d_utilization": 0.25,
		"passive_usage_sampled_at":     now,
	}}
	snapshot := ResolveAccountQuotaReset(account, PlatformAnthropic, "claude-sonnet-4-6", "7d", now)
	if snapshot.Utilization == nil || *snapshot.Utilization != 0.25 {
		t.Fatalf("expected utilization to be preserved, got %+v", snapshot)
	}
}

func TestResolveAccountQuotaResetAutoDoesNotUse5hFallback(t *testing.T) {
	now := time.Now().UTC()
	fiveH := now.Add(time.Hour)
	account := &Account{Platform: PlatformOpenAI, SessionWindowEnd: &fiveH}
	snapshot := ResolveAccountQuotaReset(account, PlatformOpenAI, "", "auto", now)
	if snapshot.ResetAt != nil || snapshot.Window != "7d" {
		t.Fatalf("auto mode should ignore 5h fallback, got %+v", snapshot)
	}
}

func TestCompareQuotaResetSnapshotsDaysThenRemainingUsageThenHours(t *testing.T) {
	now := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	first := now.Add(26 * time.Hour)
	second := now.Add(25 * time.Hour)
	firstUtil, secondUtil := 0.10, 0.40
	a := AccountQuotaResetSnapshot{ResetAt: &first, Utilization: &firstUtil}
	b := AccountQuotaResetSnapshot{ResetAt: &second, Utilization: &secondUtil}
	if got := compareQuotaResetSnapshots(a, b, now); got >= 0 {
		t.Fatalf("same-day account with more remaining quota should win, got comparison %d", got)
	}
	third := now.Add(48 * time.Hour)
	c := AccountQuotaResetSnapshot{ResetAt: &third, Utilization: &firstUtil}
	if got := compareQuotaResetSnapshots(a, c, now); got >= 0 {
		t.Fatalf("earlier reset day should win regardless of utilization, got comparison %d", got)
	}
}
