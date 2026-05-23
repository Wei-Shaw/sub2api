package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

type accountUsageWindowStatsRepo struct {
	UsageLogRepository
	statsByStart map[int64]*usagestats.AccountStats
	calls        []time.Time
}

func (r *accountUsageWindowStatsRepo) GetAccountWindowStats(_ context.Context, _ int64, startTime time.Time) (*usagestats.AccountStats, error) {
	r.calls = append(r.calls, startTime)
	if stats, ok := r.statsByStart[startTime.UnixNano()]; ok {
		return stats, nil
	}
	return &usagestats.AccountStats{}, nil
}

type accountUsageCodexProbeRepo struct {
	stubOpenAIAccountRepo
	updateExtraCh chan map[string]any
	rateLimitCh   chan time.Time
}

func (r *accountUsageCodexProbeRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	if r.updateExtraCh != nil {
		copied := make(map[string]any, len(updates))
		for k, v := range updates {
			copied[k] = v
		}
		r.updateExtraCh <- copied
	}
	return nil
}

func (r *accountUsageCodexProbeRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	if r.rateLimitCh != nil {
		r.rateLimitCh <- resetAt
	}
	return nil
}

func TestShouldRefreshOpenAICodexSnapshot(t *testing.T) {
	t.Parallel()

	rateLimitedUntil := time.Now().Add(5 * time.Minute)
	now := time.Now()
	usage := &UsageInfo{
		FiveHour: &UsageProgress{Utilization: 0},
		SevenDay: &UsageProgress{Utilization: 0},
	}

	if !shouldRefreshOpenAICodexSnapshot(&Account{RateLimitResetAt: &rateLimitedUntil}, usage, now) {
		t.Fatal("expected rate-limited account to force codex snapshot refresh")
	}

	if shouldRefreshOpenAICodexSnapshot(&Account{}, usage, now) {
		t.Fatal("expected complete non-rate-limited usage to skip codex snapshot refresh")
	}

	if !shouldRefreshOpenAICodexSnapshot(&Account{}, &UsageInfo{FiveHour: nil, SevenDay: &UsageProgress{}}, now) {
		t.Fatal("expected missing 5h snapshot to require refresh")
	}

	staleAt := now.Add(-(openAIProbeCacheTTL + time.Minute)).Format(time.RFC3339)
	if !shouldRefreshOpenAICodexSnapshot(&Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_enabled": true,
			"codex_usage_updated_at":                       staleAt,
		},
	}, usage, now) {
		t.Fatal("expected stale ws snapshot to trigger refresh")
	}
}

func TestAccountUsageService_AddWindowStatsAttachesAnthropicSevenDay(t *testing.T) {
	now := time.Now().UTC()
	fiveHourStart := now.Add(-2 * time.Hour).Truncate(time.Second)
	fiveHourEnd := now.Add(3 * time.Hour).Truncate(time.Second)
	sevenDayReset := now.Add(48 * time.Hour).Truncate(time.Second)
	sevenDayStart := sevenDayReset.Add(-7 * 24 * time.Hour)

	repo := &accountUsageWindowStatsRepo{statsByStart: map[int64]*usagestats.AccountStats{
		fiveHourStart.UnixNano(): {
			Requests:     5,
			Tokens:       50,
			StandardCost: 0.5,
		},
		sevenDayStart.UnixNano(): {
			Requests:     70,
			Tokens:       700,
			StandardCost: 7,
		},
	}}
	svc := &AccountUsageService{usageLogRepo: repo, cache: NewUsageCache()}
	usage := &UsageInfo{
		FiveHour: &UsageProgress{Utilization: 10},
		SevenDay: &UsageProgress{Utilization: 20, ResetsAt: &sevenDayReset},
	}
	account := &Account{ID: 42, SessionWindowStart: &fiveHourStart, SessionWindowEnd: &fiveHourEnd}

	svc.addWindowStats(context.Background(), account, usage)

	if usage.FiveHour.WindowStats == nil || usage.FiveHour.WindowStats.Requests != 5 {
		t.Fatalf("FiveHour.WindowStats = %#v, want requests=5", usage.FiveHour.WindowStats)
	}
	if usage.SevenDay.WindowStats == nil || usage.SevenDay.WindowStats.Requests != 70 {
		t.Fatalf("SevenDay.WindowStats = %#v, want requests=70", usage.SevenDay.WindowStats)
	}
	if len(repo.calls) != 2 {
		t.Fatalf("GetAccountWindowStats calls = %d, want 2", len(repo.calls))
	}

	svc.addWindowStats(context.Background(), account, usage)
	if len(repo.calls) != 2 {
		t.Fatalf("GetAccountWindowStats calls after cache hit = %d, want 2", len(repo.calls))
	}
}

func TestExtractOpenAICodexProbeUpdatesAccepts429WithCodexHeaders(t *testing.T) {
	t.Parallel()

	headers := make(http.Header)
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "604800")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "100")
	headers.Set("x-codex-secondary-reset-after-seconds", "18000")
	headers.Set("x-codex-secondary-window-minutes", "300")

	updates, err := extractOpenAICodexProbeUpdates(&http.Response{StatusCode: http.StatusTooManyRequests, Header: headers})
	if err != nil {
		t.Fatalf("extractOpenAICodexProbeUpdates() error = %v", err)
	}
	if len(updates) == 0 {
		t.Fatal("expected codex probe updates from 429 headers")
	}
	if got := updates["codex_5h_used_percent"]; got != 100.0 {
		t.Fatalf("codex_5h_used_percent = %v, want 100", got)
	}
	if got := updates["codex_7d_used_percent"]; got != 100.0 {
		t.Fatalf("codex_7d_used_percent = %v, want 100", got)
	}
}

func TestAccountUsageService_PersistOpenAICodexProbeSnapshotOnlyUpdatesExtra(t *testing.T) {
	t.Parallel()

	repo := &accountUsageCodexProbeRepo{
		updateExtraCh: make(chan map[string]any, 1),
		rateLimitCh:   make(chan time.Time, 1),
	}
	svc := &AccountUsageService{accountRepo: repo}
	svc.persistOpenAICodexProbeSnapshot(321, map[string]any{
		"codex_7d_used_percent": 100.0,
		"codex_7d_reset_at":     time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339),
	})

	select {
	case updates := <-repo.updateExtraCh:
		if got := updates["codex_7d_used_percent"]; got != 100.0 {
			t.Fatalf("codex_7d_used_percent = %v, want 100", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("等待 codex 探测快照写入 extra 超时")
	}

	select {
	case got := <-repo.rateLimitCh:
		t.Fatalf("不应将探测快照写入运行时限流状态: %v", got)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestAccountUsageService_GetOpenAIUsage_DoesNotPromoteCodexExtraToRateLimit(t *testing.T) {
	t.Parallel()

	resetAt := time.Now().Add(6 * 24 * time.Hour).UTC().Truncate(time.Second)
	repo := &accountUsageCodexProbeRepo{
		rateLimitCh: make(chan time.Time, 1),
	}
	svc := &AccountUsageService{accountRepo: repo}
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_5h_used_percent": 1.0,
			"codex_5h_reset_at":     time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339),
			"codex_7d_used_percent": 100.0,
			"codex_7d_reset_at":     resetAt.Format(time.RFC3339),
		},
	}

	usage, err := svc.getOpenAIUsage(context.Background(), account, false)
	if err != nil {
		t.Fatalf("getOpenAIUsage() error = %v", err)
	}
	if usage.SevenDay == nil || usage.SevenDay.Utilization != 100.0 {
		t.Fatalf("预期 7 天用量仍然可见，实际为 %#v", usage.SevenDay)
	}
	if account.RateLimitResetAt != nil {
		t.Fatalf("不应让已耗尽的 codex extra 改写运行时限流状态: %v", account.RateLimitResetAt)
	}
	select {
	case got := <-repo.rateLimitCh:
		t.Fatalf("不应将已耗尽的 codex extra 持久化为运行时限流状态: %v", got)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestBuildCodexUsageProgressFromExtra_ZerosExpiredWindow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC)

	t.Run("expired 5h window zeroes utilization", func(t *testing.T) {
		extra := map[string]any{
			"codex_5h_used_percent": 42.0,
			"codex_5h_reset_at":     "2026-03-16T10:00:00Z", // 2h ago
		}
		progress := buildCodexUsageProgressFromExtra(extra, "5h", now)
		if progress == nil {
			t.Fatal("expected non-nil progress")
		}
		if progress.Utilization != 0 {
			t.Fatalf("expected Utilization=0 for expired window, got %v", progress.Utilization)
		}
		if progress.RemainingSeconds != 0 {
			t.Fatalf("expected RemainingSeconds=0, got %v", progress.RemainingSeconds)
		}
	})

	t.Run("active 5h window keeps utilization", func(t *testing.T) {
		resetAt := now.Add(2 * time.Hour).Format(time.RFC3339)
		extra := map[string]any{
			"codex_5h_used_percent": 42.0,
			"codex_5h_reset_at":     resetAt,
		}
		progress := buildCodexUsageProgressFromExtra(extra, "5h", now)
		if progress == nil {
			t.Fatal("expected non-nil progress")
		}
		if progress.Utilization != 42.0 {
			t.Fatalf("expected Utilization=42, got %v", progress.Utilization)
		}
	})

	t.Run("expired 7d window zeroes utilization", func(t *testing.T) {
		extra := map[string]any{
			"codex_7d_used_percent": 88.0,
			"codex_7d_reset_at":     "2026-03-15T00:00:00Z", // yesterday
		}
		progress := buildCodexUsageProgressFromExtra(extra, "7d", now)
		if progress == nil {
			t.Fatal("expected non-nil progress")
		}
		if progress.Utilization != 0 {
			t.Fatalf("expected Utilization=0 for expired 7d window, got %v", progress.Utilization)
		}
	})
}

func TestCodexWindowStart(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC)

	t.Run("uses reset minus 5h window length", func(t *testing.T) {
		extra := map[string]any{
			"codex_5h_reset_at":       "2026-03-16T15:30:00Z",
			"codex_5h_window_minutes": 300,
		}
		got := codexWindowStart(extra, "5h", now)
		want := time.Date(2026, 3, 16, 10, 30, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Fatalf("codexWindowStart() = %v, want %v", got, want)
		}
	})

	t.Run("uses reset minus 7d window length", func(t *testing.T) {
		extra := map[string]any{
			"codex_7d_reset_at":       "2026-03-20T00:00:00Z",
			"codex_7d_window_minutes": 10080,
		}
		got := codexWindowStart(extra, "7d", now)
		want := time.Date(2026, 3, 13, 0, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Fatalf("codexWindowStart() = %v, want %v", got, want)
		}
	})

	t.Run("falls back to rolling window when metadata is missing", func(t *testing.T) {
		got := codexWindowStart(map[string]any{}, "5h", now)
		want := now.Add(-5 * time.Hour)
		if !got.Equal(want) {
			t.Fatalf("codexWindowStart() = %v, want %v", got, want)
		}
	})

	t.Run("falls back to rolling window when reset is expired", func(t *testing.T) {
		extra := map[string]any{
			"codex_5h_reset_at":       "2026-03-16T10:00:00Z",
			"codex_5h_window_minutes": 300,
		}
		got := codexWindowStart(extra, "5h", now)
		want := now.Add(-5 * time.Hour)
		if !got.Equal(want) {
			t.Fatalf("codexWindowStart() = %v, want %v", got, want)
		}
	})
}
