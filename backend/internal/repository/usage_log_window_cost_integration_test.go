//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestAccountWindowCostStateBackfillsThenTracksIncrementally(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageLogRepository(client, integrationDB).(*usageLogRepository)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("window-cost-%s@example.com", uuid.NewString()),
		PasswordHash: "hash",
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-window-cost-" + uuid.NewString(),
		Name:   "window-cost",
	})
	windowStart := time.Now().UTC().Add(-time.Hour).Truncate(time.Minute)
	windowEnd := windowStart.Add(5 * time.Hour)
	account := mustCreateAccount(t, client, &service.Account{
		Name:               "window-cost-" + uuid.NewString(),
		Platform:           service.PlatformAnthropic,
		Type:               service.AccountTypeOAuth,
		Extra:              map[string]any{"window_cost_limit": 100.0},
		SessionWindowStart: &windowStart,
		SessionWindowEnd:   &windowEnd,
	})

	insert := func(requestID string, createdAt time.Time, cost float64) int64 {
		t.Helper()
		log := &service.UsageLog{
			UserID:     user.ID,
			APIKeyID:   apiKey.ID,
			AccountID:  account.ID,
			RequestID:  requestID,
			Model:      "claude-test",
			TotalCost:  cost,
			ActualCost: cost,
			CreatedAt:  createdAt,
		}
		inserted, err := repo.Create(ctx, log)
		require.NoError(t, err)
		require.True(t, inserted)
		return log.ID
	}

	firstID := insert(uuid.NewString(), windowStart.Add(10*time.Minute), 1.25)
	_, err := integrationDB.ExecContext(ctx, "DELETE FROM account_window_cost_state WHERE account_id = $1", account.ID)
	require.NoError(t, err)

	costs, err := repo.GetAccountWindowCostsBatch(ctx, []int64{account.ID}, windowStart)
	require.NoError(t, err)
	require.InDelta(t, 1.25, costs[account.ID], 0.0000001)

	secondCreatedAt := windowStart.Add(40 * time.Minute)
	secondID := insert(uuid.NewString(), secondCreatedAt, 2.5)
	costs, err = repo.GetAccountWindowCostsBatch(ctx, []int64{account.ID}, windowStart)
	require.NoError(t, err)
	require.InDelta(t, 3.75, costs[account.ID], 0.0000001)

	_, err = integrationDB.ExecContext(ctx, `
		UPDATE accounts
		SET extra = jsonb_set(extra, '{window_cost_limit}', '200'::jsonb)
		WHERE id = $1
	`, account.ID)
	require.NoError(t, err)
	var initialized bool
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT initialized
		FROM account_window_cost_state
		WHERE account_id = $1
	`, account.ID).Scan(&initialized))
	require.False(t, initialized)
	costs, err = repo.GetAccountWindowCostsBatch(ctx, []int64{account.ID}, windowStart)
	require.NoError(t, err)
	require.InDelta(t, 3.75, costs[account.ID], 0.0000001)

	correctedStart := windowStart.Add(30 * time.Minute)
	costs, err = repo.GetAccountWindowCostsBatch(ctx, []int64{account.ID}, correctedStart)
	require.NoError(t, err)
	require.InDelta(t, 2.5, costs[account.ID], 0.0000001)

	require.NoError(t, repo.Delete(ctx, secondID))
	costs, err = repo.GetAccountWindowCostsBatch(ctx, []int64{account.ID}, correctedStart)
	require.NoError(t, err)
	require.Zero(t, costs[account.ID])

	require.NoError(t, repo.Delete(ctx, firstID))
}
