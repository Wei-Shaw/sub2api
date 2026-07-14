//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func newDurableBillingQueueIntegrationRepo() *queuedUsageBillingRepository {
	return &queuedUsageBillingRepository{
		direct:         &usageBillingRepository{db: integrationDB},
		db:             integrationDB,
		rdb:            integrationRedis,
		consumerCount:  2,
		readBatchSize:  128,
		pollInterval:   10 * time.Millisecond,
		commandTimeout: 15 * time.Second,
		maxRetryDelay:  time.Second,
		wakeCh:         make(chan struct{}, 2),
	}
}

func resetDurableBillingQueueTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(), `
		TRUNCATE usage_billing_jobs, usage_billing_dead_letters, usage_billing_dedup RESTART IDENTITY
	`)
	require.NoError(t, err)
	require.NoError(t, integrationRedis.FlushDB(context.Background()).Err())
}

func TestDurableUsageBillingQueueSurvivesRedisLoss(t *testing.T) {
	resetDurableBillingQueueTables(t)
	ctx := context.Background()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("durable-billing-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-durable-" + uuid.NewString(),
		Name:   "durable",
		Quota:  100,
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "durable-account-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
	})
	cmd := service.UsageBillingCommand{
		RequestID:             uuid.NewString(),
		APIKeyID:              apiKey.ID,
		UserID:                user.ID,
		AccountID:             account.ID,
		AccountType:           service.AccountTypeAPIKey,
		BalanceCost:           1.25,
		APIKeyQuotaCost:       1.25,
		QuotaPlatform:         service.PlatformOpenAI,
		UserPlatformQuotaCost: 1.25,
	}
	cmd.Normalize()
	payload, err := json.Marshal(&cmd)
	require.NoError(t, err)
	input, err := json.Marshal([]usageBillingBatchInput{{
		RequestID:          cmd.RequestID,
		APIKeyID:           cmd.APIKeyID,
		RequestFingerprint: cmd.RequestFingerprint,
		Payload:            payload,
	}})
	require.NoError(t, err)

	repo := newDurableBillingQueueIntegrationRepo()
	statuses, err := repo.insertEnqueueBatch(ctx, input)
	require.NoError(t, err)
	require.Equal(t, usageBillingEnqueueInserted, statuses[usageBillingRequestKey(cmd.RequestID, cmd.APIKeyID)].status)
	repo.reconcilePendingOverlay(&cmd)

	// Simulate total Redis data loss before PostgreSQL settlement.
	require.NoError(t, integrationRedis.FlushDB(ctx).Err())
	repo.recoverPendingOverlays()
	require.InDelta(t, 1.25, mustRedisFloat(t, integrationRedis, usageBillingPendingBalanceKey(user.ID)), 1e-9)
	processed, err := repo.processJobBatch(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, processed)

	var balance, quotaUsed float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT quota_used FROM api_keys WHERE id = $1", apiKey.ID).Scan(&quotaUsed))
	require.InDelta(t, 98.75, balance, 1e-9)
	require.InDelta(t, 1.25, quotaUsed, 1e-9)
	var platformUsage float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT daily_usage_usd FROM user_platform_quotas
		WHERE user_id = $1 AND platform = $2 AND deleted_at IS NULL
	`, user.ID, service.PlatformOpenAI).Scan(&platformUsage))
	require.InDelta(t, 1.25, platformUsage, 1e-9)

	var jobs, dedup int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_jobs").Scan(&jobs))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = $1 AND api_key_id = $2", cmd.RequestID, cmd.APIKeyID).Scan(&dedup))
	require.Zero(t, jobs)
	require.Equal(t, 1, dedup)

	statuses, err = repo.insertEnqueueBatch(ctx, input)
	require.NoError(t, err)
	require.Equal(t, usageBillingEnqueueApplied, statuses[usageBillingRequestKey(cmd.RequestID, cmd.APIKeyID)].status)
}

func TestDurableUsageBillingQueueConcurrentConsumersApplyExactlyOnce(t *testing.T) {
	resetDurableBillingQueueTables(t)
	ctx := context.Background()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("durable-billing-load-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      10000,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-durable-load-" + uuid.NewString(),
		Name:   "durable-load",
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "durable-load-account-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
	})

	const jobCount = 1000
	inputs := make([]usageBillingBatchInput, 0, jobCount)
	for i := 0; i < jobCount; i++ {
		cmd := service.UsageBillingCommand{
			RequestID:   fmt.Sprintf("durable-load-%s-%d", uuid.NewString(), i),
			APIKeyID:    apiKey.ID,
			UserID:      user.ID,
			AccountID:   account.ID,
			AccountType: service.AccountTypeAPIKey,
			BalanceCost: 0.01,
		}
		cmd.Normalize()
		payload, err := json.Marshal(&cmd)
		require.NoError(t, err)
		inputs = append(inputs, usageBillingBatchInput{
			RequestID:          cmd.RequestID,
			APIKeyID:           cmd.APIKeyID,
			RequestFingerprint: cmd.RequestFingerprint,
			Payload:            payload,
		})
	}
	batchJSON, err := json.Marshal(inputs)
	require.NoError(t, err)
	repoA := newDurableBillingQueueIntegrationRepo()
	statuses, err := repoA.insertEnqueueBatch(ctx, batchJSON)
	require.NoError(t, err)
	require.Len(t, statuses, jobCount)
	repoB := newDurableBillingQueueIntegrationRepo()

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for _, repo := range []*queuedUsageBillingRepository{repoA, repoB} {
		wg.Add(1)
		go func(repo *queuedUsageBillingRepository) {
			defer wg.Done()
			for {
				processed, processErr := repo.processJobBatch(ctx)
				if processErr != nil {
					errCh <- processErr
					return
				}
				if processed == 0 {
					return
				}
			}
		}(repo)
	}
	wg.Wait()
	close(errCh)
	for processErr := range errCh {
		require.NoError(t, processErr)
	}

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, 9990, balance, 1e-7)
	var jobs, dedup int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_jobs").Scan(&jobs))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE api_key_id = $1", apiKey.ID).Scan(&dedup))
	require.Zero(t, jobs)
	require.Equal(t, jobCount, dedup)
}

func BenchmarkDurableUsageBillingQueueEnqueue(b *testing.B) {
	_, err := integrationDB.ExecContext(context.Background(), `
		TRUNCATE usage_billing_jobs, usage_billing_dead_letters, usage_billing_dedup RESTART IDENTITY
	`)
	require.NoError(b, err)
	repo := &queuedUsageBillingRepository{
		direct:         &usageBillingRepository{db: integrationDB},
		db:             integrationDB,
		consumerCount:  4,
		readBatchSize:  128,
		pollInterval:   50 * time.Millisecond,
		commandTimeout: 15 * time.Second,
		maxRetryDelay:  30 * time.Second,
		enqueueCh:      make(chan usageBillingEnqueueRequest, usageBillingEnqueueQueueSize),
		wakeCh:         make(chan struct{}, 4),
	}
	repo.start()
	defer repo.Stop()

	var sequence atomic.Uint64
	started := time.Now()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n := sequence.Add(1)
			cmd := &service.UsageBillingCommand{
				RequestID: fmt.Sprintf("billing-benchmark-%d", n),
				APIKeyID:  1,
				UserID:    1,
			}
			if _, applyErr := repo.Apply(context.Background(), cmd); applyErr != nil {
				b.Error(applyErr)
				return
			}
		}
	})
	b.StopTimer()
	elapsed := time.Since(started)
	b.ReportMetric(float64(b.N)/elapsed.Minutes(), "rpm")
}
