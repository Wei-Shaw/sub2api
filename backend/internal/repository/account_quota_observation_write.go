package repository

import (
	"encoding/json"
	"time"
)

// quotaObservationWrite adds the observation to the same statement as the
// account update. Reading accounts.extra later loses a terminal sample when a
// new window arrives before the history worker's next poll. Only explicitly
// allowed quota fields from this update are retained, never credentials or a
// merge with the previous window's fields.
func quotaObservationWrite(query string, args []any, updates map[string]any) (string, []any) {
	stamp, ok := updates["codex_usage_updated_at"].(string)
	if reset, resetOK := updates["codex_history_reset_at"].(string); resetOK {
		stamp, ok = reset, true
	}
	if !ok {
		return query, args
	}
	observedAt, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return query, args
	}
	payload := make(map[string]any)
	for _, key := range []string{
		"codex_usage_updated_at", "codex_history_reset_at", "codex_history_reset_windows",
		"codex_5h_used_percent", "codex_5h_reset_at", "codex_5h_window_minutes", "codex_5h_reset_after_seconds",
		"codex_7d_used_percent", "codex_7d_reset_at", "codex_7d_window_minutes", "codex_7d_reset_after_seconds",
	} {
		if value, exists := updates[key]; exists {
			payload[key] = value
		}
	}
	if len(payload) <= 1 {
		return query, args
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return query, args
	}
	// $1/$2 belong to the existing UPDATE; keep the UPDATE as the last
	// statement so RowsAffected retains its existing account-not-found meaning.
	query = `WITH quota_observation AS (
		INSERT INTO account_quota_observations (account_id, observed_at, payload)
		SELECT id, $3, $4::jsonb FROM accounts
		WHERE id = $2 AND deleted_at IS NULL
		  AND platform = 'openai' AND type = 'oauth' AND parent_account_id IS NULL
		  AND LOWER(TRIM(COALESCE(quota_dimension, ''))) IN ('', 'global')
		  AND LOWER(TRIM(COALESCE(credentials->>'auth_mode', ''))) NOT IN ('agent_identity', 'personal_access_token', 'personalaccesstoken')
		  AND LOWER(TRIM(COALESCE(credentials->>'openai_auth_mode', ''))) NOT IN ('agent_identity', 'personal_access_token', 'personalaccesstoken')
	) ` + query
	return query, append(args, observedAt.UTC(), string(data))
}
