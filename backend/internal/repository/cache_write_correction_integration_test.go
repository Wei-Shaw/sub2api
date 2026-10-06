//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestUsageBillingRepositoryApply_CacheWriteCorrectionAtomic(t *testing.T) {
	for _, preserveStats := range []bool{false, true} {
		name := "rewrite_account_stats"
		if preserveStats {
			name = "preserve_account_stats_when_unspecified"
		}
		t.Run(name, func(t *testing.T) {
			f := newCacheWriteCorrectionFixture(t)
			f.createOriginalLog(t)
			if preserveStats {
				f.adjustment.CacheWriteCorrection.CorrectedAccountStatsCost = nil
			}

			result, err := f.billing.Apply(context.Background(), f.adjustment)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.True(t, result.Applied)
			require.True(t, result.APIKeyQuotaExhausted)
			require.NotNil(t, result.NewBalance)
			require.InDelta(t, 96.8, *result.NewBalance, 1e-8)
			require.NotNil(t, result.QuotaState)
			require.InDelta(t, 0.8, result.QuotaState.TotalUsed, 1e-8)

			f.requireCorrectedState(t)
		})
	}
}

func TestUsageBillingRepositoryApply_CacheWriteCorrectionDeduplicatesReplay(t *testing.T) {
	f := newCacheWriteCorrectionFixture(t)
	f.createOriginalLog(t)
	ctx := context.Background()

	result, err := f.billing.Apply(ctx, f.adjustment)
	require.NoError(t, err)
	require.True(t, result.Applied)
	beforeReplay := f.billingState(t)

	result, err = f.billing.Apply(ctx, f.adjustment)
	require.NoError(t, err)
	require.False(t, result.Applied)

	// Replaying the original bill after reconciliation must not charge it again.
	result, err = f.billing.Apply(ctx, f.originalBill)
	require.NoError(t, err)
	require.False(t, result.Applied)

	// The correction payload participates in deduplication even when the
	// monetary delta and adjustment request ID are unchanged.
	conflicting := *f.adjustment
	correction := *f.adjustment.CacheWriteCorrection
	correction.CorrectedInputTokens--
	correction.CorrectedCacheCreationTokens++
	conflicting.CacheWriteCorrection = &correction
	conflicting.RequestFingerprint = ""
	result, err = f.billing.Apply(ctx, &conflicting)
	require.ErrorIs(t, err, service.ErrUsageBillingRequestConflict)
	require.Nil(t, result)

	require.Equal(t, beforeReplay, f.billingState(t))
	f.requireCorrectedState(t)
}

func TestUsageBillingRepositoryApply_CacheWriteCorrectionMissingLogRollsBack(t *testing.T) {
	f := newCacheWriteCorrectionFixture(t)
	before := f.billingState(t)

	result, err := f.billing.Apply(context.Background(), f.adjustment)
	require.ErrorContains(t, err, "cache write correction target usage log is missing")
	require.Nil(t, result)
	require.Equal(t, before, f.billingState(t), "all monetary effects and quota status must roll back")
	f.requireDedupCount(t, f.adjustment.RequestID, 0)
	f.requireDedupCount(t, f.originalBill.RequestID, 1)

	var count int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM usage_logs WHERE api_key_id = $1", f.original.APIKeyID).Scan(&count))
	require.Zero(t, count)

	// Restoring the original log permits the exact same adjustment to retry:
	// the failed transaction must not leave a dedup claim behind.
	f.createOriginalLog(t)
	result, err = f.billing.Apply(context.Background(), f.adjustment)
	require.NoError(t, err)
	require.True(t, result.Applied)
	f.requireCorrectedState(t)
}

func TestUsageBillingRepositoryApply_CacheWriteCorrectionConflictingLogRollsBack(t *testing.T) {
	f := newCacheWriteCorrectionFixture(t)
	// A row that matches neither the original nor corrected token buckets
	// must not be overwritten, even though its request ID and API key match.
	f.original.InputTokens--
	f.original.CacheCreationTokens++
	f.createOriginalLog(t)
	before := f.billingState(t)
	original, err := f.logs.GetByID(context.Background(), f.original.ID)
	require.NoError(t, err)

	result, err := f.billing.Apply(context.Background(), f.adjustment)
	require.ErrorContains(t, err, "cache write usage reconciliation conflict")
	require.Nil(t, result)
	require.Equal(t, before, f.billingState(t), "balance, quota counters, windows and status must roll back")
	f.requireDedupCount(t, f.adjustment.RequestID, 0)
	f.requireDedupCount(t, f.originalBill.RequestID, 1)

	after, err := f.logs.GetByID(context.Background(), f.original.ID)
	require.NoError(t, err)
	require.Equal(t, original, after, "conflicting usage row must remain unchanged")
}

type cacheWriteCorrectionFixture struct {
	billing      service.UsageBillingRepository
	logs         *usageLogRepository
	original     *service.UsageLog
	originalBill *service.UsageBillingCommand
	adjustment   *service.UsageBillingCommand
}

func newCacheWriteCorrectionFixture(t *testing.T) *cacheWriteCorrectionFixture {
	t.Helper()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{
		Email:   "cache-write-correction-" + uuid.NewString() + "@example.com",
		Balance: 100,
	})
	key := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-cache-write-correction-" + uuid.NewString(),
		Quota:  3.1,
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name:     "cache-write-correction-" + uuid.NewString(),
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeAPIKey,
		Extra:    map[string]any{"quota_limit": 100.0},
	})
	statsCost, accountMultiplier := 0.75, 0.5
	original := &service.UsageLog{
		UserID:                user.ID,
		APIKeyID:              key.ID,
		AccountID:             account.ID,
		RequestID:             uuid.NewString(),
		Model:                 "gpt-5.4",
		InputTokens:           1000,
		OutputTokens:          100,
		CacheReadTokens:       200,
		InputCost:             1,
		OutputCost:            0.4,
		CacheReadCost:         0.1,
		TotalCost:             1.5,
		ActualCost:            3,
		RateMultiplier:        2,
		AccountRateMultiplier: &accountMultiplier,
		AccountStatsCost:      &statsCost,
		BillingType:           service.BillingTypeBalance,
		CreatedAt:             time.Now().UTC(),
	}
	originalBill := &service.UsageBillingCommand{
		RequestID:           original.RequestID,
		APIKeyID:            key.ID,
		UserID:              user.ID,
		AccountID:           account.ID,
		AccountType:         account.Type,
		Model:               original.Model,
		InputTokens:         original.InputTokens,
		OutputTokens:        original.OutputTokens,
		CacheReadTokens:     original.CacheReadTokens,
		BalanceCost:         3,
		APIKeyQuotaCost:     3,
		APIKeyRateLimitCost: 3,
		AccountQuotaCost:    0.75,
	}
	correctedStatsCost := 0.8
	f := &cacheWriteCorrectionFixture{
		billing:      NewUsageBillingRepository(client, integrationDB),
		logs:         newUsageLogRepositoryWithSQL(client, integrationDB),
		original:     original,
		originalBill: originalBill,
		adjustment: &service.UsageBillingCommand{
			RequestID:           "cwr:" + uuid.NewString(),
			APIKeyID:            key.ID,
			UserID:              user.ID,
			AccountID:           account.ID,
			AccountType:         account.Type,
			Model:               original.Model,
			CacheCreationTokens: 400,
			BalanceCost:         0.2,
			APIKeyQuotaCost:     0.2,
			APIKeyRateLimitCost: 0.2,
			AccountQuotaCost:    0.05,
			CacheWriteCorrection: &service.OpenAICacheWriteUsageCorrection{
				RequestID:                    original.RequestID,
				APIKeyID:                     key.ID,
				OriginalInputTokens:          1000,
				OriginalCacheCreationTokens:  0,
				CorrectedInputTokens:         600,
				CorrectedCacheCreationTokens: 400,
				CorrectedInputCost:           0.6,
				CorrectedCacheCreationCost:   0.5,
				CorrectedTotalCost:           1.6,
				CorrectedActualCost:          3.2,
				CorrectedAccountStatsCost:    &correctedStatsCost,
			},
		},
	}
	result, err := f.billing.Apply(context.Background(), originalBill)
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.False(t, result.APIKeyQuotaExhausted)
	return f
}

func (f *cacheWriteCorrectionFixture) createOriginalLog(t *testing.T) {
	t.Helper()
	created, err := f.logs.Create(context.Background(), f.original)
	require.NoError(t, err)
	require.True(t, created)
	require.NotZero(t, f.original.ID)
}

type cacheWriteCorrectionBillingState struct {
	Balance, QuotaUsed, Usage5h, Usage1d, Usage7d, AccountQuotaUsed float64
	Status                                                          string
	Window5h, Window1d, Window7d                                    sql.NullTime
}

func (f *cacheWriteCorrectionFixture) billingState(t *testing.T) cacheWriteCorrectionBillingState {
	t.Helper()
	var state cacheWriteCorrectionBillingState
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), `
		SELECT u.balance, k.quota_used, k.usage_5h, k.usage_1d, k.usage_7d,
			COALESCE((a.extra->>'quota_used')::numeric, 0), k.status,
			k.window_5h_start, k.window_1d_start, k.window_7d_start
		FROM users u, api_keys k, accounts a
		WHERE u.id = $1 AND k.id = $2 AND a.id = $3
	`, f.original.UserID, f.original.APIKeyID, f.original.AccountID).Scan(
		&state.Balance, &state.QuotaUsed, &state.Usage5h, &state.Usage1d, &state.Usage7d,
		&state.AccountQuotaUsed, &state.Status, &state.Window5h, &state.Window1d, &state.Window7d,
	))
	return state
}

func (f *cacheWriteCorrectionFixture) requireDedupCount(t *testing.T, requestID string, expected int) {
	t.Helper()
	var count int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = $1 AND api_key_id = $2",
		requestID, f.original.APIKeyID).Scan(&count))
	require.Equal(t, expected, count)
}

func (f *cacheWriteCorrectionFixture) requireCorrectedState(t *testing.T) {
	t.Helper()
	state := f.billingState(t)
	require.InDelta(t, 96.8, state.Balance, 1e-8)
	require.InDelta(t, 3.2, state.QuotaUsed, 1e-8)
	require.InDelta(t, 3.2, state.Usage5h, 1e-8)
	require.InDelta(t, 3.2, state.Usage1d, 1e-8)
	require.InDelta(t, 3.2, state.Usage7d, 1e-8)
	require.InDelta(t, 0.8, state.AccountQuotaUsed, 1e-8)
	require.Equal(t, service.StatusAPIKeyQuotaExhausted, state.Status)
	f.requireDedupCount(t, f.originalBill.RequestID, 1)
	f.requireDedupCount(t, f.adjustment.RequestID, 1)

	row, err := f.logs.GetByID(context.Background(), f.original.ID)
	require.NoError(t, err)
	require.Equal(t, f.original.RequestID, row.RequestID)
	require.Equal(t, f.original.APIKeyID, row.APIKeyID)
	require.Equal(t, 600, row.InputTokens)
	require.Equal(t, 400, row.CacheCreationTokens)
	require.Equal(t, f.original.InputTokens+f.original.CacheCreationTokens, row.InputTokens+row.CacheCreationTokens)
	require.Equal(t, f.original.OutputTokens, row.OutputTokens)
	require.Equal(t, f.original.CacheReadTokens, row.CacheReadTokens)
	require.InDelta(t, 0.6, row.InputCost, 1e-8)
	require.InDelta(t, 0.5, row.CacheCreationCost, 1e-8)
	require.InDelta(t, f.original.OutputCost, row.OutputCost, 1e-8)
	require.InDelta(t, f.original.CacheReadCost, row.CacheReadCost, 1e-8)
	require.InDelta(t, 1.6, row.TotalCost, 1e-8)
	require.InDelta(t, 3.2, row.ActualCost, 1e-8)
	require.InDelta(t, row.InputCost+row.OutputCost+row.CacheCreationCost+row.CacheReadCost, row.TotalCost, 1e-8)
	require.InDelta(t, row.TotalCost*row.RateMultiplier, row.ActualCost, 1e-8)
	wantStats := f.original.AccountStatsCost
	if correctionStats := f.adjustment.CacheWriteCorrection.CorrectedAccountStatsCost; correctionStats != nil {
		wantStats = correctionStats
	}
	require.NotNil(t, row.AccountStatsCost)
	require.InDelta(t, *wantStats, *row.AccountStatsCost, 1e-8)

	var count int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM usage_logs WHERE api_key_id = $1", f.original.APIKeyID).Scan(&count))
	require.Equal(t, 1, count, "reconciliation must rewrite the original row without adding usage")
}
