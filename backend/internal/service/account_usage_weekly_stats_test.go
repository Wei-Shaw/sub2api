package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

type weeklyStatsCall struct {
	accountID int64
	start     time.Time
}
type weeklyStatsRepo struct {
	calls []weeklyStatsCall
	stats *usagestats.AccountStats
	err   error
}

func (r *weeklyStatsRepo) GetAccountWindowStats(_ context.Context, id int64, start time.Time) (*usagestats.AccountStats, error) {
	r.calls = append(r.calls, weeklyStatsCall{id, start})
	return r.stats, r.err
}

func TestLoadClaudeWeeklyStatsBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 8, 7, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name  string
		reset *time.Time
		start time.Time
	}{
		{"missing uses rolling week", nil, now.Add(-claudeWeeklyWindow)},
		{"zero uses rolling week", weeklyTime(time.Time{}), now.Add(-claudeWeeklyWindow)},
		{"expired uses rolling week", weeklyTime(now.Add(-time.Second)), now.Add(-claudeWeeklyWindow)},
		{"equal now uses rolling week", &now, now.Add(-claudeWeeklyWindow)},
		{"active uses reset boundary", weeklyTime(now.Add(2 * time.Hour)), now.Add(2*time.Hour - claudeWeeklyWindow)},
		{"exactly one week ahead", weeklyTime(now.Add(claudeWeeklyWindow)), now},
		{"does not add a Claude-only future limit", weeklyTime(now.Add(claudeWeeklyWindow + time.Hour)), now.Add(time.Hour)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			want := &usagestats.AccountStats{Requests: 2, Tokens: 300, Cost: 12, StandardCost: 24, UserCost: 48}
			repo := &weeklyStatsRepo{stats: want}
			got, err := loadClaudeWeeklyStats(context.Background(), repo, &sync.Map{}, 42, tt.reset, 0.5, now)
			if err != nil {
				t.Fatal(err)
			}
			if got != want || len(repo.calls) != 1 {
				t.Fatalf("stats/query mismatch: %v, %v", got, repo.calls)
			}
			if call := repo.calls[0]; call.accountID != 42 || !call.start.Equal(tt.start) {
				t.Fatalf("wrong account/window: %+v, want %v", call, tt.start)
			}
		})
	}
}

func TestLoadClaudeWeeklyStatsCacheIsolation(t *testing.T) {
	now := time.Date(2026, 10, 8, 7, 0, 0, 0, time.UTC)
	reset := now.Add(2 * time.Hour)
	repo := &weeklyStatsRepo{stats: &usagestats.AccountStats{Cost: 12}}
	cache := &sync.Map{}
	load := func(id int64, at, boundary time.Time, utilization float64, wantCalls int) {
		t.Helper()
		if _, err := loadClaudeWeeklyStats(context.Background(), repo, cache, id, &boundary, utilization, at); err != nil {
			t.Fatal(err)
		}
		if len(repo.calls) != wantCalls {
			t.Fatalf("want %d queries, got %d", wantCalls, len(repo.calls))
		}
	}
	load(42, now, reset, 40, 1)
	load(42, now.Add(10*time.Second), reset, 40, 1)
	load(43, now.Add(10*time.Second), reset, 40, 2)                // account isolation
	load(42, now.Add(11*time.Second), reset, 41, 3)                // fresher percentage
	load(42, now.Add(12*time.Second), reset.Add(time.Hour), 41, 4) // early/manual reset
	load(42, now.Add(72*time.Second), reset.Add(time.Hour), 41, 5) // TTL boundary
	load(42, now.Add(71*time.Second), reset.Add(time.Hour), 41, 6) // backwards clock
	entries := 0
	cache.Range(func(_, _ any) bool { entries++; return true })
	if entries != 2 {
		t.Fatalf("cache must keep one entry per account, got %d", entries)
	}
	// Once reset expires, switch to the same rolling-week fallback as GPT.
	load(42, reset.Add(time.Hour), reset.Add(time.Hour), 41, 7)
}

func TestLoadClaudeWeeklyStatsFailureAndNoCache(t *testing.T) {
	now := time.Now()
	reset := now.Add(time.Hour)
	ctx := context.Background()
	if got, err := loadClaudeWeeklyStats(ctx, nil, nil, 42, &reset, 40, now); got != nil || err != nil {
		t.Fatal("nil repository must be safe")
	}
	cache := &sync.Map{}
	repo := &weeklyStatsRepo{err: errors.New("database unavailable")}
	if got, err := loadClaudeWeeklyStats(ctx, repo, cache, 42, &reset, 40, now); got != nil || err == nil {
		t.Fatal("query failure must not become zero-cost data")
	}
	repo.err = nil
	if got, err := loadClaudeWeeklyStats(ctx, repo, cache, 42, &reset, 40, now); got != nil || err != nil {
		t.Fatal("nil statistics must stay unavailable")
	}
	repo.stats = &usagestats.AccountStats{Cost: 12}
	if got, err := loadClaudeWeeklyStats(ctx, repo, nil, 42, &reset, 40, now); got != repo.stats || err != nil {
		t.Fatal("nil cache must still query")
	}
	if len(repo.calls) != 3 {
		t.Fatal("errors/nil stats must not be cached")
	}
}

func weeklyTime(t time.Time) *time.Time { return &t }
