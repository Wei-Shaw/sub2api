package service

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

const claudeWeeklyWindow = 7 * 24 * time.Hour

type claudeWeeklyStatsReader interface {
	GetAccountWindowStats(context.Context, int64, time.Time) (*usagestats.AccountStats, error)
}

type claudeWeeklyStatsEntry struct {
	resetAt     time.Time
	utilization float64
	timestamp   time.Time
	stats       *usagestats.AccountStats
}

// loadClaudeWeeklyStats uses the same window-start rule as OpenAI's
// codexWindowStatsStart: reset minus seven days while the reset is in the
// future; otherwise the last seven days. The local account-cost equivalent
// and upstream percentage are intentionally the same estimate inputs as GPT.
func loadClaudeWeeklyStats(ctx context.Context, repo claudeWeeklyStatsReader, cache *sync.Map, accountID int64, resetAt *time.Time, utilization float64, now time.Time) (*usagestats.AccountStats, error) {
	if repo == nil {
		return nil, nil
	}
	start := now.Add(-claudeWeeklyWindow)
	resetKey := time.Time{}
	if resetAt != nil && now.Before(*resetAt) {
		start = resetAt.Add(-claudeWeeklyWindow)
		resetKey = *resetAt
	}

	if cache != nil {
		if cached, ok := cache.Load(accountID); ok {
			if entry, ok := cached.(*claudeWeeklyStatsEntry); ok && entry.resetAt.Equal(resetKey) && entry.utilization == utilization && now.Sub(entry.timestamp) >= 0 && now.Sub(entry.timestamp) < time.Minute {
				return entry.stats, nil
			}
		}
	}

	stats, err := repo.GetAccountWindowStats(ctx, accountID, start)
	if err != nil || stats == nil {
		return nil, err
	}
	if cache != nil {
		// One entry per account; a reset (including an early reset) replaces the
		// previous window rather than keeping an unbounded history of cache keys.
		cache.Store(accountID, &claudeWeeklyStatsEntry{resetAt: resetKey, utilization: utilization, timestamp: now, stats: stats})
	}
	return stats, nil
}
