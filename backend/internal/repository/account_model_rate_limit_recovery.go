package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// RecoverModelRateLimitIfUnchanged removes only the cooldown generation observed
// before an upstream usage query. The row version protects concurrent re-arms,
// including ones with an identical JSON payload. Persist the fresh usage and
// outbox event atomically with the removal: publishing an old 100% snapshot
// could otherwise immediately re-arm a scheduling threshold cooldown.
func (r *accountRepository) RecoverModelRateLimitIfUnchanged(ctx context.Context, id int64, scope string, expected json.RawMessage, expectedUpdatedAt time.Time, usageUpdates map[string]any) (bool, error) {
	if scoped := r.accountTransactionScope(ctx); scoped != r {
		return scoped.RecoverModelRateLimitIfUnchanged(ctx, id, scope, expected, expectedUpdatedAt, usageUpdates)
	}
	if scope == "" || len(expected) == 0 || expectedUpdatedAt.IsZero() || len(usageUpdates) == 0 {
		return false, nil
	}
	updates, err := json.Marshal(usageUpdates)
	if err != nil {
		return false, err
	}
	result, err := r.sql.ExecContext(ctx, `
		WITH updated AS (
			UPDATE accounts AS a
			SET extra = (a.extra || $5::jsonb) #- ARRAY['model_rate_limits', $3]::text[], updated_at = NOW()
			WHERE a.id = $1 AND a.deleted_at IS NULL AND a.updated_at = $2
				AND (a.extra->'model_rate_limits'->$3::text) = $4::jsonb
			RETURNING a.id
		)
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		SELECT $6, updated.id, NULL, NULL FROM updated
	`, id, expectedUpdatedAt, scope, expected, string(updates), service.SchedulerOutboxEventAccountChanged)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected > 0 {
		r.syncSchedulerAccountSnapshotDetached(ctx, id)
	}
	return affected > 0, nil
}
