package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// GetOpenAIProvisionDemand aggregates recorded OAuth-backed OpenAI traffic
// for the coordinator's recent demand window. Keeping this query at the
// repository boundary avoids loading individual usage rows into memory.
func (r *usageLogRepository) GetOpenAIProvisionDemand(ctx context.Context, start, end time.Time) (service.OpenAIProvisionDemand, error) {
	const query = `
		WITH usage_demand AS (
			SELECT
				COUNT(DISTINCT ul.user_id) AS active_users,
				COUNT(*) AS requests,
				COALESCE(SUM(ul.input_tokens::bigint + ul.output_tokens::bigint + ul.cache_creation_tokens::bigint + ul.cache_read_tokens::bigint), 0) AS tokens
			FROM usage_logs AS ul
			JOIN accounts AS a ON a.id = ul.account_id
			WHERE a.platform = $1
			  AND a.type = $2
			  AND ul.created_at >= $3
			  AND ul.created_at < $4
			  AND (ul.actual_cost > 0 OR ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens > 0)
		)
		SELECT
			usage_demand.active_users,
			usage_demand.requests,
			usage_demand.tokens
		FROM usage_demand`

	var demand service.OpenAIProvisionDemand
	if err := scanSingleRow(ctx, r.sql, query, []any{
		service.PlatformOpenAI,
		service.AccountTypeOAuth,
		start,
		end,
	}, &demand.ActiveUsers, &demand.Requests, &demand.Tokens); err != nil {
		return service.OpenAIProvisionDemand{}, err
	}
	return demand, nil
}
