package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// GetAccountStatsByWindows returns each account's totals for its exact [start, end)
// interval. The lateral aggregate produces a zero-valued row even without usage.
func (r *usageLogRepository) GetAccountStatsByWindows(ctx context.Context, windows []usagestats.AccountStatsWindow) (result map[int64]*usagestats.AccountStats, err error) {
	if err := usagestats.ValidateAccountStatsWindows(windows); err != nil {
		return nil, err
	}

	query, args := buildAccountStatsByWindowsQuery(windows)
	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			result = nil
			err = closeErr
		}
	}()

	result = make(map[int64]*usagestats.AccountStats, len(windows))
	for rows.Next() {
		var accountID int64
		var stats usagestats.AccountStats
		if err := rows.Scan(&accountID, &stats.Requests, &stats.Tokens, &stats.Cost, &stats.StandardCost, &stats.UserCost); err != nil {
			return nil, err
		}
		result[accountID] = &stats
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// buildAccountStatsByWindowsQuery expects validated windows. Explicit types keep
// VALUES parameters unambiguous, including a batch containing only one account.
func buildAccountStatsByWindowsQuery(windows []usagestats.AccountStatsWindow) (string, []any) {
	values := make([]string, len(windows))
	args := make([]any, 0, 3*len(windows))
	for i, window := range windows {
		offset := len(args)
		values[i] = fmt.Sprintf("($%d::bigint, $%d::timestamptz, $%d::timestamptz)", offset+1, offset+2, offset+3)
		args = append(args, window.AccountID, window.StartAt, window.EndAt)
	}

	query := `
		SELECT w.account_id, totals.requests, totals.tokens,
			totals.cost, totals.standard_cost, totals.user_cost
		FROM (VALUES ` + strings.Join(values, ", ") + `) AS w(account_id, start_at, end_at)
		CROSS JOIN LATERAL (
			SELECT
				COUNT(*) AS requests,
				COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens), 0) AS tokens,
				COALESCE(SUM(COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)), 0) AS cost,
				COALESCE(SUM(ul.total_cost), 0) AS standard_cost,
				COALESCE(SUM(ul.actual_cost), 0) AS user_cost
			FROM usage_logs AS ul
			WHERE ul.account_id = w.account_id
				AND ul.created_at >= w.start_at
				AND ul.created_at < w.end_at
		) AS totals
	`
	return query, args
}
