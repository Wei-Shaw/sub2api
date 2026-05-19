package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

type stubOAuthPauseUsageReader struct {
	standardCost float64
	calls        int
}

func (s *stubOAuthPauseUsageReader) GetAccountWindowStats(ctx context.Context, accountID int64, startTime time.Time) (*usagestats.AccountStats, error) {
	s.calls++
	return &usagestats.AccountStats{StandardCost: s.standardCost}, nil
}

func TestEvaluateOAuthPreemptivePauseOpenAIPercent(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	reset5h := now.Add(90 * time.Minute)
	reset7d := now.Add(24 * time.Hour)
	account := &Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"oauth_5h_pause_percent": 95.0,
			"oauth_7d_pause_percent": 90.0,
			"codex_5h_used_percent":  95.0,
			"codex_5h_reset_at":      reset5h.Format(time.RFC3339Nano),
			"codex_7d_used_percent":  89.0,
			"codex_7d_reset_at":      reset7d.Format(time.RFC3339Nano),
		},
	}

	got, ok := evaluateOAuthPreemptivePause(context.Background(), account, nil, now, nil)
	if !ok {
		t.Fatal("expected pause to trigger")
	}
	if !got.Equal(reset5h) {
		t.Fatalf("expected 5h reset %v, got %v", reset5h, got)
	}
}

func TestEvaluateOAuthPreemptivePauseAnthropicPercent(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	sessionStart := now.Add(-2 * time.Hour)
	sessionEnd := now.Add(3 * time.Hour)
	reset7d := now.Add(7 * 24 * time.Hour)
	account := &Account{
		ID:                 2,
		Platform:           PlatformAnthropic,
		Type:               AccountTypeOAuth,
		SessionWindowStart: &sessionStart,
		SessionWindowEnd:   &sessionEnd,
		Extra: map[string]any{
			"oauth_5h_pause_percent":          80.0,
			"oauth_7d_pause_percent":          75.0,
			"session_window_utilization":      0.79,
			"passive_usage_7d_utilization":    0.75,
			"passive_usage_7d_reset":          float64(reset7d.Unix()),
			"oauth_5h_pause_amount_usd":       0.0,
			"oauth_7d_pause_amount_usd":       0.0,
			"effective_oauth_5h_pause_amount": 0.0,
		},
	}

	got, ok := evaluateOAuthPreemptivePause(context.Background(), account, nil, now, nil)
	if !ok {
		t.Fatal("expected pause to trigger")
	}
	if !got.Equal(time.Unix(reset7d.Unix(), 0)) {
		t.Fatalf("expected 7d reset %v, got %v", reset7d, got)
	}
}

func TestEvaluateOAuthPreemptivePauseAmountUsesStandardCost(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	reset5h := now.Add(5 * time.Hour)
	usage := &stubOAuthPauseUsageReader{standardCost: 12.5}
	account := &Account{
		ID:       3,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"oauth_5h_pause_amount_usd": 12.0,
			"codex_5h_used_percent":     20.0,
			"codex_5h_reset_at":         reset5h.Format(time.RFC3339Nano),
		},
	}

	got, ok := evaluateOAuthPreemptivePause(context.Background(), account, usage, now, nil)
	if !ok {
		t.Fatal("expected amount threshold to trigger")
	}
	if !got.Equal(reset5h) {
		t.Fatalf("expected reset %v, got %v", reset5h, got)
	}
	if usage.calls != 1 {
		t.Fatalf("expected one usage query, got %d", usage.calls)
	}
}

func TestEvaluateOAuthPreemptivePauseReturnsLaterResetWhenMultipleWindowsTrigger(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	reset5h := now.Add(4 * time.Hour)
	reset7d := now.Add(48 * time.Hour)
	account := &Account{
		ID:       4,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"oauth_5h_pause_percent": 90.0,
			"oauth_7d_pause_percent": 90.0,
			"codex_5h_used_percent":  91.0,
			"codex_5h_reset_at":      reset5h.Format(time.RFC3339Nano),
			"codex_7d_used_percent":  92.0,
			"codex_7d_reset_at":      reset7d.Format(time.RFC3339Nano),
		},
	}

	got, ok := evaluateOAuthPreemptivePause(context.Background(), account, nil, now, nil)
	if !ok {
		t.Fatal("expected pause to trigger")
	}
	if !got.Equal(reset7d) {
		t.Fatalf("expected later reset %v, got %v", reset7d, got)
	}
}

func TestEvaluateOAuthPreemptivePauseAccountConfigOverridesEffectiveGroupConfig(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	reset5h := now.Add(5 * time.Hour)
	account := &Account{
		ID:       5,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"oauth_5h_pause_percent":           95.0,
			"effective_oauth_5h_pause_percent": 80.0,
			"codex_5h_used_percent":            90.0,
			"codex_5h_reset_at":                reset5h.Format(time.RFC3339Nano),
		},
	}

	if got, ok := evaluateOAuthPreemptivePause(context.Background(), account, nil, now, nil); ok {
		t.Fatalf("expected account-level threshold to override group fallback, got reset %v", got)
	}
}
