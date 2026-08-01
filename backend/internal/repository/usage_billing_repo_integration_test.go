//go:build integration

package repository

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestUsageBillingRepositoryApply_DeduplicatesBalanceBilling(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-" + uuid.NewString(),
		Name:   "billing",
		Quota:  1,
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "usage-billing-account-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
	})

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{
		RequestID:           requestID,
		APIKeyID:            apiKey.ID,
		UserID:              user.ID,
		AccountID:           account.ID,
		AccountType:         service.AccountTypeAPIKey,
		BalanceCost:         1.25,
		APIKeyQuotaCost:     1.25,
		APIKeyRateLimitCost: 1.25,
	}

	result1, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.NotNil(t, result1)
	require.True(t, result1.Applied)
	require.True(t, result1.APIKeyQuotaExhausted)

	result2, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.NotNil(t, result2)
	require.False(t, result2.Applied)

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, 98.75, balance, 0.000001)

	var quotaUsed float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT quota_used FROM api_keys WHERE id = $1", apiKey.ID).Scan(&quotaUsed))
	require.InDelta(t, 1.25, quotaUsed, 0.000001)

	var usage5h float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT usage_5h FROM api_keys WHERE id = $1", apiKey.ID).Scan(&usage5h))
	require.InDelta(t, 1.25, usage5h, 0.000001)

	var status string
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status FROM api_keys WHERE id = $1", apiKey.ID).Scan(&status))
	require.Equal(t, service.StatusAPIKeyQuotaExhausted, status)

	var dedupCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = $1 AND api_key_id = $2", requestID, apiKey.ID).Scan(&dedupCount))
	require.Equal(t, 1, dedupCount)
}

func TestUsageBillingRepositoryApply_DeduplicatesSubscriptionBilling(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-sub-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name:             "usage-billing-group-" + uuid.NewString(),
		Platform:         service.PlatformAnthropic,
		SubscriptionType: service.SubscriptionTypeSubscription,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID:  user.ID,
		GroupID: &group.ID,
		Key:     "sk-usage-billing-sub-" + uuid.NewString(),
		Name:    "billing-sub",
	})
	subscription := mustCreateSubscription(t, client, &service.UserSubscription{
		UserID:  user.ID,
		GroupID: group.ID,
	})

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{
		RequestID:        requestID,
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		AccountID:        0,
		SubscriptionID:   &subscription.ID,
		SubscriptionCost: 2.5,
	}

	result1, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, result1.Applied)

	result2, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.False(t, result2.Applied)

	var dailyUsage float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT daily_usage_usd FROM user_subscriptions WHERE id = $1", subscription.ID).Scan(&dailyUsage))
	require.InDelta(t, 2.5, dailyUsage, 0.000001)
}

func TestUsageBillingRepositoryApply_RequestFingerprintConflict(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-conflict-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-conflict-" + uuid.NewString(),
		Name:   "billing-conflict",
	})

	requestID := uuid.NewString()
	_, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:   requestID,
		APIKeyID:    apiKey.ID,
		UserID:      user.ID,
		BalanceCost: 1.25,
	})
	require.NoError(t, err)

	_, err = repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:   requestID,
		APIKeyID:    apiKey.ID,
		UserID:      user.ID,
		BalanceCost: 2.50,
	})
	require.ErrorIs(t, err, service.ErrUsageBillingRequestConflict)
}

func TestUsageBillingRepositoryApply_UpdatesAccountQuota(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-account-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-account-" + uuid.NewString(),
		Name:   "billing-account",
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "usage-billing-account-quota-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
		Extra: map[string]any{
			"quota_limit": 100.0,
		},
	})

	_, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:        uuid.NewString(),
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		AccountID:        account.ID,
		AccountType:      service.AccountTypeAPIKey,
		AccountQuotaCost: 3.5,
	})
	require.NoError(t, err)

	var quotaUsed float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COALESCE((extra->>'quota_used')::numeric, 0) FROM accounts WHERE id = $1", account.ID).Scan(&quotaUsed))
	require.InDelta(t, 3.5, quotaUsed, 0.000001)
}

func TestUsageBillingRepositoryApply_EnqueuesSchedulerOutboxOnQuotaCrossing(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	newFixture := func(t *testing.T, extra map[string]any) (int64, int64) {
		t.Helper()
		user := mustCreateUser(t, client, &service.User{
			Email:        fmt.Sprintf("usage-billing-outbox-user-%d-%s@example.com", time.Now().UnixNano(), uuid.NewString()),
			PasswordHash: "hash",
		})
		apiKey := mustCreateApiKey(t, client, &service.APIKey{
			UserID: user.ID,
			Key:    "sk-usage-billing-outbox-" + uuid.NewString(),
			Name:   "billing-outbox",
		})
		account := mustCreateAccount(t, client, &service.Account{
			Name:  "usage-billing-outbox-" + uuid.NewString(),
			Type:  service.AccountTypeAPIKey,
			Extra: extra,
		})
		return apiKey.ID, account.ID
	}

	outboxCountFor := func(t *testing.T, accountID int64) int {
		t.Helper()
		var count int
		require.NoError(t, integrationDB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM scheduler_outbox WHERE event_type = $1 AND account_id = $2",
			service.SchedulerOutboxEventAccountChanged, accountID,
		).Scan(&count))
		return count
	}

	t.Run("daily_first_crossing_enqueues", func(t *testing.T) {
		apiKeyID, accountID := newFixture(t, map[string]any{
			"quota_daily_limit": 10.0,
		})
		// 第一次低于日限额：不应入队 outbox
		_, err := repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			AccountID:        accountID,
			AccountType:      service.AccountTypeAPIKey,
			AccountQuotaCost: 4,
		})
		require.NoError(t, err)
		require.Equal(t, 0, outboxCountFor(t, accountID), "below limit should not enqueue")

		// 第二次跨越日限额：应入队一次 outbox
		_, err = repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			AccountID:        accountID,
			AccountType:      service.AccountTypeAPIKey,
			AccountQuotaCost: 8,
		})
		require.NoError(t, err)
		require.Equal(t, 1, outboxCountFor(t, accountID), "crossing daily limit should enqueue once")

		// 再次递增（已超）：不应重复入队
		_, err = repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			AccountID:        accountID,
			AccountType:      service.AccountTypeAPIKey,
			AccountQuotaCost: 2,
		})
		require.NoError(t, err)
		require.Equal(t, 1, outboxCountFor(t, accountID), "subsequent increments beyond limit should not re-enqueue")
	})

	t.Run("weekly_first_crossing_enqueues", func(t *testing.T) {
		apiKeyID, accountID := newFixture(t, map[string]any{
			"quota_weekly_limit": 10.0,
		})
		_, err := repo.Apply(ctx, &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKeyID,
			AccountID:        accountID,
			AccountType:      service.AccountTypeAPIKey,
			AccountQuotaCost: 15, // 单次即跨越
		})
		require.NoError(t, err)
		require.Equal(t, 1, outboxCountFor(t, accountID), "single-shot crossing weekly limit should enqueue once")
	})
}

func TestDashboardAggregationRepositoryCleanupUsageBillingDedup_BatchDeletesOldRows(t *testing.T) {
	ctx := context.Background()
	repo := newDashboardAggregationRepositoryWithSQL(integrationDB)

	oldRequestID := "dedup-old-" + uuid.NewString()
	newRequestID := "dedup-new-" + uuid.NewString()
	oldCreatedAt := time.Now().UTC().AddDate(0, 0, -400)
	newCreatedAt := time.Now().UTC().Add(-time.Hour)

	_, err := integrationDB.ExecContext(ctx, `
		INSERT INTO usage_billing_dedup (request_id, api_key_id, request_fingerprint, created_at)
		VALUES ($1, 1, $2, $3), ($4, 1, $5, $6)
	`,
		oldRequestID, strings.Repeat("a", 64), oldCreatedAt,
		newRequestID, strings.Repeat("b", 64), newCreatedAt,
	)
	require.NoError(t, err)

	require.NoError(t, repo.CleanupUsageBillingDedup(ctx, time.Now().UTC().AddDate(0, 0, -365)))

	var oldCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = $1", oldRequestID).Scan(&oldCount))
	require.Equal(t, 0, oldCount)

	var newCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = $1", newRequestID).Scan(&newCount))
	require.Equal(t, 1, newCount)

	var archivedCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup_archive WHERE request_id = $1", oldRequestID).Scan(&archivedCount))
	require.Equal(t, 1, archivedCount)
}

func TestUsageBillingRepositoryApply_DeduplicatesAgainstArchivedKey(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)
	aggRepo := newDashboardAggregationRepositoryWithSQL(integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-billing-archive-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-usage-billing-archive-" + uuid.NewString(),
		Name:   "billing-archive",
	})

	requestID := uuid.NewString()
	cmd := &service.UsageBillingCommand{
		RequestID:   requestID,
		APIKeyID:    apiKey.ID,
		UserID:      user.ID,
		BalanceCost: 1.25,
	}

	result1, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, result1.Applied)

	_, err = integrationDB.ExecContext(ctx, `
		UPDATE usage_billing_dedup
		SET created_at = $1
		WHERE request_id = $2 AND api_key_id = $3
	`, time.Now().UTC().AddDate(0, 0, -400), requestID, apiKey.ID)
	require.NoError(t, err)
	require.NoError(t, aggRepo.CleanupUsageBillingDedup(ctx, time.Now().UTC().AddDate(0, 0, -365)))

	result2, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.False(t, result2.Applied)

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, 98.75, balance, 0.000001)
}

func TestUsageBillingSettlement_DurableRetryUsesPersistedSnapshotAndDeductsOnce(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	usageRepo := NewUsageLogRepository(client, integrationDB)
	repo := NewUsageBillingRepository(client, integrationDB)
	repo.SetUsageLogRepository(usageRepo)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-settlement-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-integration-" + uuid.NewString(),
		Name:   "settlement",
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "usage-settlement-account-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
	})

	requestID := uuid.NewString()
	catalogEpoch := int64(7)
	catalogRevisionID := int64(700)
	pricingSource := "catalog-shadow"
	pinnedActualCost := 0.006
	usageLog := &service.UsageLog{
		UserID:            user.ID,
		APIKeyID:          apiKey.ID,
		AccountID:         account.ID,
		RequestID:         requestID,
		Model:             "catalog-model",
		RequestedModel:    "catalog-model",
		InputTokens:       1000,
		OutputTokens:      500,
		TotalCost:         pinnedActualCost,
		ActualCost:        pinnedActualCost,
		CatalogEpoch:      &catalogEpoch,
		CatalogRevisionID: &catalogRevisionID,
		PricingSource:     &pricingSource,
		PricingSnapshot: map[string]any{
			"_settlement_cost": map[string]any{
				"total_cost":  pinnedActualCost,
				"actual_cost": pinnedActualCost,
			},
		},
		RequestType: service.RequestTypeSync,
		CreatedAt:   time.Now().UTC(),
	}
	cmd := &service.UsageBillingCommand{
		RequestID:           requestID,
		APIKeyID:            apiKey.ID,
		UserID:              user.ID,
		AccountID:           account.ID,
		AccountType:         service.AccountTypeAPIKey,
		Model:               usageLog.Model,
		BalanceCost:         pinnedActualCost,
		CatalogEpoch:        catalogEpoch,
		CatalogRevisionID:   catalogRevisionID,
		PricingSource:       pricingSource,
		PricingSnapshotHash: "pinned-snapshot-hash",
	}
	cmd.Normalize()

	// RED/GREEN boundary: the usage row and retry command are durable before
	// any balance mutation is attempted.
	require.NoError(t, repo.EnqueueSettlement(ctx, cmd, usageLog))

	var persistedSnapshot []byte
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT pricing_snapshot FROM usage_logs WHERE request_id = $1 AND api_key_id = $2",
		requestID, apiKey.ID,
	).Scan(&persistedSnapshot))
	require.Contains(t, string(persistedSnapshot), "_settlement_cost")
	require.Contains(t, string(persistedSnapshot), "0.006")

	jobs, err := repo.ClaimDueSettlements(ctx, 1, time.Minute)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.InDelta(t, pinnedActualCost, jobs[0].Command.BalanceCost, 0.000000001)

	// Simulate a transient worker failure. A later catalog publication changes
	// the current cost, but cannot rewrite the persisted usage/outbox command.
	require.NoError(t, repo.MarkSettlementRetry(ctx, jobs[0].ID, "transient billing failure", time.Second))
	_, err = integrationDB.ExecContext(ctx,
		"UPDATE usage_billing_settlements SET next_attempt_at = NOW() WHERE id = $1", jobs[0].ID)
	require.NoError(t, err)
	newlyPublishedCost := 456.0

	worker := service.NewUsageBillingSettlementWorker(repo)
	require.NoError(t, worker.RunOnce(ctx))
	worker.Stop()

	var persistedCommand []byte
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT command FROM usage_billing_settlements WHERE id = $1", jobs[0].ID).Scan(&persistedCommand))
	var retriedCommand service.UsageBillingCommand
	require.NoError(t, json.Unmarshal(persistedCommand, &retriedCommand))
	require.NotEqual(t, newlyPublishedCost, retriedCommand.BalanceCost)
	require.InDelta(t, pinnedActualCost, retriedCommand.BalanceCost, 0.000000001)

	// A redelivery after the worker acknowledgement must not deduct again.
	redelivery, err := repo.Apply(ctx, &retriedCommand)
	require.NoError(t, err)
	require.False(t, redelivery.Applied)

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, 100-pinnedActualCost, balance, 0.000000001)

	var usageCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM usage_logs WHERE request_id = $1 AND api_key_id = $2",
		requestID, apiKey.ID).Scan(&usageCount))
	require.Equal(t, 1, usageCount)

	var status string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT status FROM usage_billing_settlements WHERE request_id = $1 AND api_key_id = $2",
		requestID, apiKey.ID).Scan(&status))
	require.Equal(t, "applied", status)
}

func TestUsageBillingSettlementWorker_RetriesApplyFailureAfterRestart(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	usageRepo := NewUsageLogRepository(client, integrationDB)
	repo := NewUsageBillingRepository(client, integrationDB)
	repo.SetUsageLogRepository(usageRepo)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-settlement-retry-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "«redacted:sk-…»" + uuid.NewString(),
		Name:   "settlement-retry",
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "usage-settlement-retry-account-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
	})

	requestID := uuid.NewString()
	pinnedCost := 0.006
	usageLog := &service.UsageLog{
		UserID:         user.ID,
		APIKeyID:       apiKey.ID,
		AccountID:      account.ID,
		RequestID:      requestID,
		Model:          "retry-model",
		RequestedModel: "retry-model",
		InputTokens:    10,
		OutputTokens:   4,
		TotalCost:      pinnedCost,
		ActualCost:     pinnedCost,
		PricingSnapshot: map[string]any{
			"_settlement_cost": map[string]any{
				"total_cost":  pinnedCost,
				"actual_cost": pinnedCost,
			},
		},
		RequestType: service.RequestTypeSync,
		CreatedAt:   time.Now().UTC(),
	}
	cmd := &service.UsageBillingCommand{
		RequestID:   requestID,
		APIKeyID:    apiKey.ID,
		UserID:      999999999,
		AccountID:   account.ID,
		AccountType: service.AccountTypeAPIKey,
		Model:       usageLog.Model,
		BalanceCost: pinnedCost,
	}
	cmd.Normalize()

	require.NoError(t, repo.EnqueueSettlement(ctx, cmd, usageLog))

	worker := service.NewUsageBillingSettlementWorker(repo)
	require.NoError(t, worker.RunOnce(ctx))
	worker.Stop()

	var status string
	var attempts int
	var lastError string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT status, attempts, COALESCE(last_error, '')
		FROM usage_billing_settlements
		WHERE request_id = $1 AND api_key_id = $2
	`, requestID, apiKey.ID).Scan(&status, &attempts, &lastError))
	require.Equal(t, "retry", status)
	require.GreaterOrEqual(t, attempts, 1)
	require.Contains(t, lastError, "USER_NOT_FOUND")

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, 100, balance, 0.000000001)

	// Simulate process restart recovery: restore the command and make the
	// leased retry immediately due before constructing a new worker instance.
	_, err := integrationDB.ExecContext(ctx, `
		UPDATE usage_billing_settlements
		SET command = jsonb_set(command, '{UserID}', to_jsonb($1::bigint)),
			status = 'retry', locked_until = NULL, next_attempt_at = NOW(),
			last_error = NULL, updated_at = NOW()
		WHERE request_id = $2 AND api_key_id = $3
	`, user.ID, requestID, apiKey.ID)
	require.NoError(t, err)

	restartedWorker := service.NewUsageBillingSettlementWorker(repo)
	require.NoError(t, restartedWorker.RunOnce(ctx))
	restartedWorker.Stop()

	var appliedStatus string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT status FROM usage_billing_settlements WHERE request_id = $1 AND api_key_id = $2",
		requestID, apiKey.ID).Scan(&appliedStatus))
	require.Equal(t, "applied", appliedStatus)

	cmd.UserID = user.ID
	redelivery, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.False(t, redelivery.Applied)

	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, 100-pinnedCost, balance, 0.000000001)

	var usageCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM usage_logs
		WHERE request_id = $1 AND api_key_id = $2
	`, requestID, apiKey.ID).Scan(&usageCount))
	require.Equal(t, 1, usageCount)
}

const (
	settlementClaimHelperEnv    = "SUB2API_SETTLEMENT_CLAIM_HELPER"
	settlementClaimHelperMarker = "SETTLEMENT_CLAIM_HELD"
	settlementClaimHelperLease  = 2 * time.Second
)

func TestUsageBillingSettlementWorker_RedeliversKilledClaimAfterLeaseExpiry(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	usageRepo := NewUsageLogRepository(client, integrationDB)
	repo := NewUsageBillingRepository(client, integrationDB)
	repo.SetUsageLogRepository(usageRepo)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("usage-settlement-lease-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-settlement-lease-" + uuid.NewString(),
		Name:   "settlement-lease",
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "usage-settlement-lease-account-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
	})

	requestID := uuid.NewString()
	pinnedCost := 0.007
	usageLog := &service.UsageLog{
		UserID:         user.ID,
		APIKeyID:       apiKey.ID,
		AccountID:      account.ID,
		RequestID:      requestID,
		Model:          "lease-recovery-model",
		RequestedModel: "lease-recovery-model",
		InputTokens:    11,
		OutputTokens:   5,
		TotalCost:      pinnedCost,
		ActualCost:     pinnedCost,
		PricingSnapshot: map[string]any{
			"_settlement_cost": map[string]any{
				"total_cost":  pinnedCost,
				"actual_cost": pinnedCost,
			},
		},
		RequestType: service.RequestTypeSync,
		CreatedAt:   time.Now().UTC(),
	}
	cmd := &service.UsageBillingCommand{
		RequestID:   requestID,
		APIKeyID:    apiKey.ID,
		UserID:      user.ID,
		AccountID:   account.ID,
		AccountType: service.AccountTypeAPIKey,
		Model:       usageLog.Model,
		BalanceCost: pinnedCost,
	}
	cmd.Normalize()

	require.NoError(t, repo.EnqueueSettlement(ctx, cmd, usageLog))

	helper := exec.Command(os.Args[0], "-test.run=^TestUsageBillingSettlementClaimHelperProcess$", "-test.v")
	helper.Env = append(os.Environ(), settlementClaimHelperEnv+"=1")
	stdout, err := helper.StdoutPipe()
	require.NoError(t, err)
	var helperStderr strings.Builder
	helper.Stderr = &helperStderr
	require.NoError(t, helper.Start())
	t.Cleanup(func() {
		if helper.Process != nil && helper.ProcessState == nil {
			_ = helper.Process.Kill()
			_ = helper.Wait()
		}
	})

	claimReady := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) == settlementClaimHelperMarker {
				claimReady <- nil
				return
			}
		}
		if err := scanner.Err(); err != nil {
			claimReady <- err
			return
		}
		claimReady <- fmt.Errorf("claim helper exited before marker")
	}()

	select {
	case err := <-claimReady:
		if err != nil {
			_ = helper.Process.Kill()
			_ = helper.Wait()
			t.Fatalf("claim helper failed: %v; stderr=%s", err, helperStderr.String())
		}
	case <-time.After(10 * time.Second):
		_ = helper.Process.Kill()
		_ = helper.Wait()
		t.Fatalf("timed out waiting for claim helper; stderr=%s", helperStderr.String())
	}

	var processingStatus string
	var lockedUntil time.Time
	var leaseHeld bool
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT status, locked_until, locked_until > NOW()
		FROM usage_billing_settlements
		WHERE request_id = $1 AND api_key_id = $2
	`, requestID, apiKey.ID).Scan(&processingStatus, &lockedUntil, &leaseHeld))
	require.Equal(t, "processing", processingStatus)
	require.False(t, lockedUntil.IsZero())
	require.True(t, leaseHeld)

	require.NoError(t, helper.Process.Kill())
	require.Error(t, helper.Wait(), "hard-killed helper must not exit successfully")

	// The helper process is hard-killed immediately after the durable claim,
	// without applying, acknowledging, or marking a retry.
	// A replacement worker must not steal the row until PostgreSQL expires the
	// lease, then it must safely redeliver the persisted command.
	restartedWorker := service.NewUsageBillingSettlementWorker(repo)
	t.Cleanup(restartedWorker.Stop)

	// The replacement must not steal the command while the killed process's
	// lease is still valid.
	require.NoError(t, restartedWorker.RunOnce(ctx))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT status
		FROM usage_billing_settlements
		WHERE request_id = $1 AND api_key_id = $2
	`, requestID, apiKey.ID).Scan(&processingStatus))
	require.Equal(t, "processing", processingStatus)
	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, 100, balance, 0.000000001)

	var appliedStatus string
	require.Eventually(t, func() bool {
		if err := restartedWorker.RunOnce(ctx); err != nil {
			return false
		}
		if err := integrationDB.QueryRowContext(ctx, `
			SELECT status
			FROM usage_billing_settlements
			WHERE request_id = $1 AND api_key_id = $2
		`, requestID, apiKey.ID).Scan(&appliedStatus); err != nil {
			return false
		}
		return appliedStatus == "applied"
	}, 5*time.Second, 25*time.Millisecond)

	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, 100-pinnedCost, balance, 0.000000001)

	redelivery, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.False(t, redelivery.Applied)
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, 100-pinnedCost, balance, 0.000000001)

	var usageCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM usage_logs
		WHERE request_id = $1 AND api_key_id = $2
	`, requestID, apiKey.ID).Scan(&usageCount))
	require.Equal(t, 1, usageCount)
}

func TestUsageBillingSettlementClaimHelperProcess(t *testing.T) {
	if os.Getenv(settlementClaimHelperEnv) != "1" {
		t.Skip("helper process only")
	}

	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)
	claimed, err := repo.ClaimDueSettlements(context.Background(), 1, settlementClaimHelperLease)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	_, err = fmt.Fprintln(os.Stdout, settlementClaimHelperMarker)
	require.NoError(t, err)
	require.NoError(t, os.Stdout.Sync())
	select {}
}
