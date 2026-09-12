//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestQuotaObservationSurvivesExtraOverwrite(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	account := mustCreateAccount(t, client, &service.Account{Name: "quota-journal-" + uuid.NewString(), Platform: "openai", Type: "oauth"})
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(context.Background(), "DELETE FROM accounts WHERE id=$1", account.ID)
		require.NoError(t, err)
	})
	repo := newAccountRepositoryWithSQL(client, integrationDB, nil)
	at := time.Now().UTC()
	for i, used := range []float64{100, 0} {
		require.NoError(t, repo.UpdateExtra(ctx, account.ID, map[string]any{
			"codex_usage_updated_at":  at.Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano),
			"codex_5h_used_percent":   used,
			"codex_5h_reset_at":       at.Add(time.Duration(i) * 5 * time.Hour).Format(time.RFC3339Nano),
			"codex_5h_window_minutes": 300,
		}))
	}
	rows, err := integrationDB.QueryContext(ctx, "SELECT payload FROM account_quota_observations WHERE account_id=$1 ORDER BY id", account.ID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var percentages []float64
	for rows.Next() {
		var raw []byte
		require.NoError(t, rows.Scan(&raw))
		var payload map[string]any
		require.NoError(t, json.Unmarshal(raw, &payload))
		percentages = append(percentages, payload["codex_5h_used_percent"].(float64))
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []float64{100, 0}, percentages)
	updated, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, 0.0, updated.Extra["codex_5h_used_percent"])
}
