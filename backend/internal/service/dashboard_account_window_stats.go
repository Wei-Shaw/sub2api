package service

import (
	"context"
	"fmt"
	"net/http"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

type accountStatsWindowsReader interface {
	GetAccountStatsByWindows(context.Context, []usagestats.AccountStatsWindow) (map[int64]*usagestats.AccountStats, error)
}

func (s *DashboardService) GetAccountStatsByWindows(ctx context.Context, windows []usagestats.AccountStatsWindow) (map[int64]*usagestats.AccountStats, error) {
	if err := usagestats.ValidateAccountStatsWindows(windows); err != nil {
		return nil, infraerrors.New(http.StatusBadRequest, "INVALID_ACCOUNT_STATS_WINDOWS", err.Error())
	}
	repo, ok := s.usageRepo.(accountStatsWindowsReader)
	if !ok {
		// Do not silently reintroduce per-account database calls.
		return nil, fmt.Errorf("usage repository does not support batch account windows")
	}
	stats, err := repo.GetAccountStatsByWindows(ctx, windows)
	if err != nil {
		return nil, fmt.Errorf("get batch account window statistics: %w", err)
	}
	return stats, nil
}
