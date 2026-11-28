package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type accountWindowsRepoStub struct {
	UsageLogRepository
	calls   int
	windows []usagestats.AccountStatsWindow
	stats   map[int64]*usagestats.AccountStats
	err     error
	ctx     context.Context
}

func (r *accountWindowsRepoStub) GetAccountStatsByWindows(ctx context.Context, windows []usagestats.AccountStatsWindow) (map[int64]*usagestats.AccountStats, error) {
	r.calls++
	r.windows = windows
	r.ctx = ctx
	return r.stats, r.err
}

func TestDashboardServiceGetAccountStatsByWindows(t *testing.T) {
	start := time.Date(2026, 9, 1, 12, 1, 2, 345000000, time.UTC)
	windows := []usagestats.AccountStatsWindow{{AccountID: 18, StartAt: start, EndAt: start.Add(7 * 24 * time.Hour)}}
	repo := &accountWindowsRepoStub{stats: map[int64]*usagestats.AccountStats{18: {Requests: 2, Cost: 3}}}
	svc := NewDashboardService(repo, nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got, err := svc.GetAccountStatsByWindows(ctx, windows)
	require.NoError(t, err)
	require.Equal(t, repo.stats, got)
	require.Equal(t, windows, repo.windows)
	require.Same(t, ctx, repo.ctx)
	require.Equal(t, 1, repo.calls)

	_, err = svc.GetAccountStatsByWindows(ctx, nil)
	code, _ := infraerrors.ToHTTP(err)
	require.Equal(t, http.StatusBadRequest, code)
	require.Equal(t, 1, repo.calls, "invalid input must not reach the repository")

	dbErr := errors.New("database query failed")
	repo.err = dbErr
	got, err = svc.GetAccountStatsByWindows(ctx, windows)
	require.ErrorIs(t, err, dbErr)
	require.Nil(t, got, "partial data must not be returned as valid statistics")

	unsupported := NewDashboardService(nil, nil, nil, nil)
	_, err = unsupported.GetAccountStatsByWindows(ctx, windows)
	require.ErrorContains(t, err, "does not support batch")
}
