//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryGetOpenAITokenStats_WeightsRateByDuration(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{})
	group := mustCreateGroup(t, client, &service.Group{Name: "ops-token-stats"})
	account := mustCreateAccount(t, client, &service.Account{Name: "ops-token-stats"})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID})
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(ctx, "DELETE FROM usage_logs WHERE group_id = $1", group.ID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, "DELETE FROM api_keys WHERE id = $1", key.ID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, "DELETE FROM accounts WHERE id = $1", account.ID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, "DELETE FROM groups WHERE id = $1", group.ID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)
		require.NoError(t, err)
	})

	now := time.Now().UTC()
	_, err := integrationDB.ExecContext(ctx, `
INSERT INTO usage_logs (user_id, api_key_id, account_id, group_id, created_at, model, output_tokens, duration_ms)
VALUES
  ($1, $2, $3, $4, $5, 'gpt-mixed-durations', 100, 1000),
  ($1, $2, $3, $4, $5, 'gpt-mixed-durations', 100, 4000),
  ($1, $2, $3, $4, $5, 'gpt-mixed-durations', 0, 9000),
  ($1, $2, $3, $4, $5, 'gpt-mixed-durations', 10, 0),
  ($1, $2, $3, $4, $5, 'gpt-mixed-durations', 7, NULL),
  ($1, $2, $3, $4, $5, 'gpt-no-valid-samples', 0, 1000)
`, user.ID, key.ID, account.ID, group.ID, now)
	require.NoError(t, err)

	repo := NewOpsRepository(integrationDB)
	stats, err := repo.GetOpenAITokenStats(ctx, &service.OpsOpenAITokenStatsFilter{
		StartTime: now.Add(-time.Minute),
		EndTime:   now.Add(time.Minute),
		GroupID:   &group.ID,
		TopN:      10,
	})
	require.NoError(t, err)
	require.Len(t, stats.Items, 2)

	mixed := stats.Items[0]
	require.Equal(t, "gpt-mixed-durations", mixed.Model)
	require.NotNil(t, mixed.AvgTokensPerSec)
	require.Equal(t, 40.0, *mixed.AvgTokensPerSec)
	require.Equal(t, int64(5), mixed.RequestCount)
	require.Equal(t, int64(217), mixed.TotalOutputTokens)

	require.Equal(t, "gpt-no-valid-samples", stats.Items[1].Model)
	require.Nil(t, stats.Items[1].AvgTokensPerSec)
}
