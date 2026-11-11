//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type accountStatsWindowsPlanNode struct {
	NodeType   string                        `json:"Node Type"`
	IndexName  string                        `json:"Index Name"`
	IndexCond  string                        `json:"Index Cond"`
	Filter     string                        `json:"Filter"`
	ActualRows float64                       `json:"Actual Rows"`
	Loops      float64                       `json:"Actual Loops"`
	Plans      []accountStatsWindowsPlanNode `json:"Plans"`
}

func TestGetAccountStatsByWindowsIntegrationPlanAndTiming(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newUsageLogRepositoryWithSQL(nil, tx)

	// Copy the migrated table and every existing index into this transaction's
	// temporary test schema. The fixture preserves column layout and index choices
	// without FK/trigger work or statistics changes in the shared harness table.
	_, err := tx.ExecContext(ctx, `
		CREATE TEMP TABLE usage_logs
		(LIKE public.usage_logs INCLUDING ALL)
		ON COMMIT DROP
	`)
	require.NoError(t, err)
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	const accounts, hours = 200, 31 * 24
	inserted, err := tx.ExecContext(ctx, `
		INSERT INTO usage_logs (
			id, user_id, api_key_id, account_id, model, requested_model,
			input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
			total_cost, actual_cost, account_stats_cost, account_rate_multiplier, created_at
		)
		SELECT
			(account_id - 1) * $3 + hour_offset + 1,
			1, 1, account_id,
			'model-' || (hour_offset % 4), 'model-' || (hour_offset % 4),
			100, 50, 25, 75,
			0.10, 0.07,
			CASE WHEN hour_offset % 3 = 0 THEN NULL ELSE 0.08 END,
			CASE WHEN hour_offset % 5 = 0 THEN NULL ELSE 0.5 END,
			$1::timestamptz + hour_offset * INTERVAL '1 hour'
		FROM generate_series(1, $2::integer) AS a(account_id)
		CROSS JOIN generate_series(0, $3::integer - 1) AS h(hour_offset)
	`, start, accounts, hours)
	require.NoError(t, err)
	rowCount, err := inserted.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(accounts*hours), rowCount)
	_, err = tx.ExecContext(ctx, "ANALYZE pg_temp.usage_logs")
	require.NoError(t, err)

	var postgresVersion string
	require.NoError(t, tx.QueryRowContext(ctx, "SHOW server_version").Scan(&postgresVersion))
	t.Logf("PostgreSQL %s; isolated temporary table cloned from migrated schema; %d rows, %d accounts, %d hourly samples/account; planner settings unchanged",
		postgresVersion, rowCount, accounts, hours)

	windows := make([]usagestats.AccountStatsWindow, 100)
	for i := range windows {
		// Mix subscription windows of 1 through 31 days and different reset hours.
		days := 1 + i%31
		windowEnd := start.Add(time.Duration(hours-i%24) * time.Hour)
		windows[i] = usagestats.AccountStatsWindow{
			AccountID: int64(i + 1),
			StartAt:   windowEnd.Add(-time.Duration(days) * 24 * time.Hour),
			EndAt:     windowEnd,
		}
	}

	for _, count := range []int{1, 10, 32, 100} {
		t.Run(fmt.Sprintf("%d_accounts", count), func(t *testing.T) {
			batch := windows[:count]
			query, args := buildAccountStatsByWindowsQuery(batch)
			var rawPlan []byte
			require.NoError(t, tx.QueryRowContext(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+query, args...).Scan(&rawPlan))
			var plans []struct {
				Plan          accountStatsWindowsPlanNode `json:"Plan"`
				PlanningTime  float64                     `json:"Planning Time"`
				ExecutionTime float64                     `json:"Execution Time"`
			}
			require.NoError(t, json.Unmarshal(rawPlan, &plans))
			require.Len(t, plans, 1)
			var indexedAccount, indexedAccountWindow bool
			var inspect func(accountStatsWindowsPlanNode)
			inspect = func(node accountStatsWindowsPlanNode) {
				if node.IndexName != "" {
					t.Logf("natural plan: %s index=%s condition=%s filter=%s rows/loop=%.0f loops=%.0f",
						node.NodeType, node.IndexName, node.IndexCond, node.Filter, node.ActualRows, node.Loops)
					if strings.Contains(node.IndexCond, "account_id") {
						indexedAccount = true
						if strings.Contains(node.IndexCond, "created_at >=") &&
							strings.Contains(node.IndexCond, "created_at <") {
							indexedAccountWindow = true
						}
					}
				}
				for _, child := range node.Plans {
					inspect(child)
				}
			}
			inspect(plans[0].Plan)
			require.True(t, indexedAccount, "natural plan should use an account index: %s", rawPlan)
			if count > 1 {
				require.True(t, indexedAccountWindow, "batch plan should constrain the account and both time bounds in an index: %s", rawPlan)
			}
			// With one constant account, PostgreSQL may prefer its smaller
			// account-only index and apply the timestamp bounds as a filter.
			require.Equal(t, float64(count), plans[0].Plan.ActualRows)
			t.Logf("EXPLAIN ANALYZE: planning=%.3fms execution=%.3fms", plans[0].PlanningTime, plans[0].ExecutionTime)

			runBatch := func() map[int64]*usagestats.AccountStats {
				result, err := repo.GetAccountStatsByWindows(ctx, batch)
				require.NoError(t, err)
				require.Len(t, result, count)
				return result
			}
			runIndividual := func() map[int64]*usagestats.AccountStats {
				result := make(map[int64]*usagestats.AccountStats, count)
				for _, window := range batch {
					one, err := repo.GetAccountStatsByWindows(ctx, []usagestats.AccountStatsWindow{window})
					require.NoError(t, err)
					result[window.AccountID] = one[window.AccountID]
				}
				return result
			}
			runLegacy := func() map[int64]*usagestats.AccountStats {
				result := make(map[int64]*usagestats.AccountStats, count)
				for _, window := range batch {
					models, err := repo.GetModelStatsWithFilters(ctx, window.StartAt, window.EndAt, 0, 0, window.AccountID, 0, nil, nil, nil)
					require.NoError(t, err)
					stats := &usagestats.AccountStats{}
					for _, model := range models {
						stats.Requests += model.Requests
						stats.Tokens += model.TotalTokens
						stats.Cost += model.ActualCost
						stats.StandardCost += model.Cost
					}
					result[window.AccountID] = stats
				}
				return result
			}

			// Warm every path and verify the same totals before timing. Legacy
			// model statistics do not expose user_cost with an account-only filter.
			expected := runBatch()
			require.Equal(t, expected, runIndividual())
			for id, legacy := range runLegacy() {
				require.Equal(t, expected[id].Requests, legacy.Requests)
				require.Equal(t, expected[id].Tokens, legacy.Tokens)
				require.InDelta(t, expected[id].Cost, legacy.Cost, 1e-9)
				require.InDelta(t, expected[id].StandardCost, legacy.StandardCost, 1e-9)
			}

			const rounds = 5
			timings := [3][]time.Duration{}
			runners := []func() map[int64]*usagestats.AccountStats{runBatch, runIndividual, runLegacy}
			for round := 0; round < rounds; round++ {
				// Rotate order to avoid consistently favoring the same warm path.
				for offset := range runners {
					index := (round + offset) % len(runners)
					began := time.Now()
					runners[index]()
					timings[index] = append(timings[index], time.Since(began))
				}
			}
			medians := [3]time.Duration{}
			for i, samples := range timings {
				sort.Slice(samples, func(a, b int) bool { return samples[a] < samples[b] })
				medians[i] = samples[len(samples)/2]
			}
			t.Logf("warm medians over %d rounds, %d accounts: batch(1 query)=%s individual totals(%d queries)=%s legacy models(%d queries)=%s; local Docker timings, no latency injection or production extrapolation",
				rounds, count, medians[0], count, medians[1], count, medians[2])
		})
	}
}
