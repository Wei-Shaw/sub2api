//go:build unit

package repository

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

func accountStatsWindowMockRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"account_id", "requests", "tokens", "cost", "standard_cost", "user_cost"})
}

func TestGetAccountStatsByWindowsSingleQueryExactParameters(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := newUsageLogRepositoryWithSQL(nil, db)
	start := time.Date(2026, 9, 28, 13, 45, 7, 123456000, time.FixedZone("UTC+8", 8*60*60))
	windows := []usagestats.AccountStatsWindow{
		{AccountID: 19, StartAt: start, EndAt: start.Add(12 * time.Hour)},
		{AccountID: 3, StartAt: start.Add(-2 * time.Hour), EndAt: start.Add(30 * time.Minute)},
	}
	expectedQuery := `
		SELECT w.account_id, totals.requests, totals.tokens,
			totals.cost, totals.standard_cost, totals.user_cost
		FROM (VALUES ($1::bigint, $2::timestamptz, $3::timestamptz), ($4::bigint, $5::timestamptz, $6::timestamptz)) AS w(account_id, start_at, end_at)
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
	mock.ExpectQuery(regexp.QuoteMeta(expectedQuery)).
		WithArgs(int64(19), windows[0].StartAt, windows[0].EndAt, int64(3), windows[1].StartAt, windows[1].EndAt).
		WillReturnRows(accountStatsWindowMockRows().
			AddRow(int64(3), int64(0), int64(0), 0.0, 0.0, 0.0).
			AddRow(int64(19), int64(4), int64(170), 6.25, 10.0, 12.5)).
		RowsWillBeClosed()

	result, err := repo.GetAccountStatsByWindows(context.Background(), windows)
	require.NoError(t, err)
	require.Equal(t, map[int64]*usagestats.AccountStats{
		19: {Requests: 4, Tokens: 170, Cost: 6.25, StandardCost: 10, UserCost: 12.5},
		3:  {},
	}, result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAccountStatsByWindowsAcceptsLimits(t *testing.T) {
	for _, count := range []int{1, 100} {
		t.Run(fmt.Sprintf("%d_accounts", count), func(t *testing.T) {
			db, mock := newSQLMock(t)
			repo := newUsageLogRepositoryWithSQL(nil, db)
			start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
			windows := make([]usagestats.AccountStatsWindow, count)
			args := make([]driver.Value, 0, count*3)
			rows := accountStatsWindowMockRows()
			for i := range windows {
				windows[i] = usagestats.AccountStatsWindow{
					AccountID: int64(i + 1),
					StartAt:   start.Add(time.Duration(i) * time.Minute),
					EndAt:     start.Add(time.Duration(i)*time.Minute + 31*24*time.Hour),
				}
				args = append(args, windows[i].AccountID, windows[i].StartAt, windows[i].EndAt)
				rows.AddRow(windows[i].AccountID, int64(0), int64(0), 0.0, 0.0, 0.0)
			}
			query, _ := buildAccountStatsByWindowsQuery(windows)
			mock.ExpectQuery(regexp.QuoteMeta(query)).
				WithArgs(args...).
				WillReturnRows(rows).
				RowsWillBeClosed()

			result, err := repo.GetAccountStatsByWindows(context.Background(), windows)
			require.NoError(t, err)
			require.Len(t, result, count)
			for _, window := range windows {
				require.Equal(t, &usagestats.AccountStats{}, result[window.AccountID])
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGetAccountStatsByWindowsRejectsInvalidInputBeforeQuery(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	valid := usagestats.AccountStatsWindow{AccountID: 1, StartAt: start, EndAt: start.Add(time.Hour)}
	tooMany := make([]usagestats.AccountStatsWindow, 101)
	for i := range tooMany {
		tooMany[i] = valid
		tooMany[i].AccountID = int64(i + 1)
	}
	tests := []struct {
		name    string
		windows []usagestats.AccountStatsWindow
	}{
		{name: "nil"},
		{name: "empty", windows: []usagestats.AccountStatsWindow{}},
		{name: "too_many", windows: tooMany},
		{name: "zero_account", windows: []usagestats.AccountStatsWindow{{AccountID: 0, StartAt: start, EndAt: valid.EndAt}}},
		{name: "negative_account", windows: []usagestats.AccountStatsWindow{{AccountID: -1, StartAt: start, EndAt: valid.EndAt}}},
		{name: "duplicate_account", windows: []usagestats.AccountStatsWindow{valid, {AccountID: 1, StartAt: start.Add(time.Hour), EndAt: start.Add(2 * time.Hour)}}},
		{name: "zero_start", windows: []usagestats.AccountStatsWindow{{AccountID: 1, EndAt: valid.EndAt}}},
		{name: "zero_end", windows: []usagestats.AccountStatsWindow{{AccountID: 1, StartAt: start}}},
		{name: "equal_bounds", windows: []usagestats.AccountStatsWindow{{AccountID: 1, StartAt: start, EndAt: start}}},
		{name: "reversed_bounds", windows: []usagestats.AccountStatsWindow{{AccountID: 1, StartAt: valid.EndAt, EndAt: start}}},
		{name: "over_31_days", windows: []usagestats.AccountStatsWindow{{AccountID: 1, StartAt: start, EndAt: start.Add(31*24*time.Hour + time.Nanosecond)}}},
		{name: "invalid_second_window", windows: []usagestats.AccountStatsWindow{valid, {AccountID: 2, StartAt: start}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, mock := newSQLMock(t)
			repo := newUsageLogRepositoryWithSQL(nil, db)
			validationErr := usagestats.ValidateAccountStatsWindows(test.windows)
			require.Error(t, validationErr)

			result, err := repo.GetAccountStatsByWindows(context.Background(), test.windows)
			require.EqualError(t, err, validationErr.Error())
			require.Nil(t, result)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGetAccountStatsByWindowsErrorsDiscardResults(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	windows := []usagestats.AccountStatsWindow{
		{AccountID: 1, StartAt: start, EndAt: start.Add(time.Hour)},
		{AccountID: 2, StartAt: start, EndAt: start.Add(2 * time.Hour)},
	}
	queryErr := errors.New("query failed")
	iterationErr := errors.New("iteration failed")
	closeErr := errors.New("close failed")
	for _, test := range []struct {
		name       string
		queryErr   error
		rows       *sqlmock.Rows
		wantErr    error
		wantErrMsg string
	}{
		{name: "query", queryErr: queryErr, wantErr: queryErr},
		{name: "context_canceled", queryErr: context.Canceled, wantErr: context.Canceled},
		{
			name: "scan_after_partial_result",
			rows: accountStatsWindowMockRows().
				AddRow(int64(1), int64(1), int64(10), 1.0, 2.0, 3.0).
				AddRow(int64(2), int64(1), "invalid-tokens", 1.0, 2.0, 3.0).
				CloseError(closeErr),
			wantErrMsg: "invalid-tokens",
		},
		{
			name: "iteration_after_partial_result",
			rows: accountStatsWindowMockRows().
				AddRow(int64(1), int64(1), int64(10), 1.0, 2.0, 3.0).
				AddRow(int64(2), int64(1), int64(20), 4.0, 5.0, 6.0).
				RowError(1, iterationErr).
				CloseError(closeErr),
			wantErr: iterationErr,
		},
		{
			name: "close_after_complete_result",
			rows: accountStatsWindowMockRows().
				AddRow(int64(1), int64(1), int64(10), 1.0, 2.0, 3.0).
				AddRow(int64(2), int64(1), int64(20), 4.0, 5.0, 6.0).
				CloseError(closeErr),
			wantErr: closeErr,
		},
		{name: "close_without_rows", rows: accountStatsWindowMockRows().CloseError(closeErr), wantErr: closeErr},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, mock := newSQLMock(t)
			repo := newUsageLogRepositoryWithSQL(nil, db)
			expected := mock.ExpectQuery("CROSS JOIN LATERAL").
				WithArgs(int64(1), windows[0].StartAt, windows[0].EndAt, int64(2), windows[1].StartAt, windows[1].EndAt)
			if test.queryErr != nil {
				expected.WillReturnError(test.queryErr)
			} else {
				expected.WillReturnRows(test.rows).RowsWillBeClosed()
			}

			result, err := repo.GetAccountStatsByWindows(context.Background(), windows)
			require.Nil(t, result)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
			} else {
				require.ErrorContains(t, err, test.wantErrMsg)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
