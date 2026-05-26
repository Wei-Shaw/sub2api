package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/lib/pq"
)

// GetUsageTrendForAccounts returns aggregate trend data for a fixed set of accounts.
func (r *usageLogRepository) GetUsageTrendForAccounts(ctx context.Context, accountIDs []int64, startTime, endTime time.Time, granularity string) (results []TrendDataPoint, err error) {
	normalizedAccountIDs := normalizePositiveInt64IDs(accountIDs)
	if len(normalizedAccountIDs) == 0 {
		return []TrendDataPoint{}, nil
	}

	dateFormat := safeDateFormat(granularity)
	query := fmt.Sprintf(`
		SELECT
			TO_CHAR(created_at, '%s') as date,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as cache_read_tokens,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) as actual_cost
		FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2 AND account_id = ANY($3)
		GROUP BY date
		ORDER BY date ASC
	`, dateFormat)

	rows, err := r.sql.QueryContext(ctx, query, startTime, endTime, pq.Array(normalizedAccountIDs))
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()

	results, err = scanTrendRows(rows)
	if err != nil {
		return nil, err
	}
	return results, nil
}

// GetModelStatsForAccounts returns aggregate model statistics for a fixed set of accounts.
func (r *usageLogRepository) GetModelStatsForAccounts(ctx context.Context, accountIDs []int64, startTime, endTime time.Time, source string) (results []ModelStat, err error) {
	normalizedAccountIDs := normalizePositiveInt64IDs(accountIDs)
	if len(normalizedAccountIDs) == 0 {
		return []ModelStat{}, nil
	}

	modelExpr := resolveModelDimensionExpression(usagestats.NormalizeModelSource(source))
	query := fmt.Sprintf(`
		SELECT
			%s as model,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as cache_read_tokens,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) as actual_cost,
			COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) as account_cost
		FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2 AND account_id = ANY($3)
		GROUP BY %s
		ORDER BY total_tokens DESC
	`, modelExpr, modelExpr)

	rows, err := r.sql.QueryContext(ctx, query, startTime, endTime, pq.Array(normalizedAccountIDs))
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()

	results, err = scanModelStatsRows(rows)
	if err != nil {
		return nil, err
	}
	return results, nil
}
