package repository

import (
	"context"
	"fmt"

	"github.com/lib/pq"
)

func (r *channelRepository) ListAccountMappingModelsByGroupIDs(ctx context.Context, groupIDs []int64) (map[int64][]string, error) {
	result := make(map[int64][]string)
	if len(groupIDs) == 0 {
		return result, nil
	}

	rows, err := r.db.QueryContext(ctx, `
		WITH mapped AS (
			SELECT ag.group_id, jsonb_object_keys(a.credentials->'model_mapping') AS model
			FROM account_groups ag
			JOIN accounts a ON a.id = ag.account_id
			WHERE ag.group_id = ANY($1)
			  AND a.deleted_at IS NULL
			  AND a.status = 'active'
			  AND a.schedulable = true
			  AND jsonb_typeof(a.credentials->'model_mapping') = 'object'
		)
		SELECT group_id, array_agg(DISTINCT model ORDER BY model)
		FROM mapped
		WHERE model <> ''
		GROUP BY group_id
	`, pq.Array(groupIDs))
	if err != nil {
		return nil, fmt.Errorf("query account mapping models: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var groupID int64
		var models pq.StringArray
		if err := rows.Scan(&groupID, &models); err != nil {
			return nil, fmt.Errorf("scan account mapping models: %w", err)
		}
		result[groupID] = []string(models)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate account mapping models: %w", err)
	}
	return result, nil
}
