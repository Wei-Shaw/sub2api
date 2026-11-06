//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type antigravityLocalStatsRepo struct {
	UsageLogRepository
	starts []time.Time
	ids    []int64
	err    error
}

func (r *antigravityLocalStatsRepo) GetAccountWindowStats(_ context.Context, id int64, start time.Time) (*usagestats.AccountStats, error) {
	r.starts = append(r.starts, start)
	r.ids = append(r.ids, id)
	return &usagestats.AccountStats{}, r.err
}

func TestAntigravityLocalUsage_UsesRollingWindows(t *testing.T) {
	repo := &antigravityLocalStatsRepo{}
	svc := &AccountUsageService{usageLogRepo: repo}
	info := &UsageInfo{AntigravityQuotaGroups: []AntigravityQuotaGroup{{Name: "Gemini 4 Argon"}}}
	before := time.Now()
	svc.attachAntigravityWindowStats(context.Background(), &Account{ID: 42}, info)
	after := time.Now()
	require.Equal(t, []int64{42, 42}, repo.ids)
	for i, duration := range []time.Duration{5 * time.Hour, 7 * 24 * time.Hour} {
		require.False(t, repo.starts[i].Before(before.Add(-duration)))
		require.False(t, repo.starts[i].After(after.Add(-duration)))
	}
	for _, window := range []*UsageProgress{info.FiveHour, info.SevenDay} {
		require.NotNil(t, window.WindowStats, "confirmed zero local activity is meaningful")
		require.Zero(t, window.WindowStats.Requests)
		require.Nil(t, window.ResetsAt, "local rolling statistics do not assert upstream reset times")
	}
	require.Len(t, info.AntigravityQuotaGroups, 1)

	repo.err = errors.New("database unavailable")
	failed := &UsageInfo{}
	svc.attachAntigravityWindowStats(context.Background(), &Account{ID: 42}, failed)
	require.Nil(t, failed.FiveHour, "failed reads must not become fabricated zero activity")
	require.Nil(t, failed.SevenDay)
}
