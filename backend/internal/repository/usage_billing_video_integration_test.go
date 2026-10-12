//go:build integration

package repository

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// A real SQL insert failure must undo every prior billing write, including the
// dedup claim. Concurrent retries then commit exactly one bill and one full row.
func TestUsageBillingVideo_AtomicSQLPersistence(t *testing.T) {
	for _, subscriptionBilling := range []bool{false, true} {
		name := "balance"
		if subscriptionBilling {
			name = "subscription"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			client := testEntClient(t)
			repo := NewUsageBillingRepository(client, integrationDB).(service.AtomicVideoUsageBillingRepository)
			user := mustCreateUser(t, client, &service.User{
				Email: "atomic-video-" + uuid.NewString() + "@example.com", PasswordHash: "hash", Balance: 100,
			})
			group := mustCreateGroup(t, client, &service.Group{
				Name: "atomic-video-" + uuid.NewString(), Platform: service.PlatformGrok,
				SubscriptionType: service.SubscriptionTypeSubscription,
			})
			key := mustCreateApiKey(t, client, &service.APIKey{
				UserID: user.ID, GroupID: &group.ID, Key: "sk-atomic-video-" + uuid.NewString(), Name: "atomic-video", Quota: 1,
			})
			account := mustCreateAccount(t, client, &service.Account{
				Name: "atomic-video-" + uuid.NewString(), Type: service.AccountTypeAPIKey,
				Extra: map[string]any{"quota_limit": 100.0},
			})
			subscription := mustCreateSubscription(t, client, &service.UserSubscription{UserID: user.ID, GroupID: group.ID})
			cmd := &service.UsageBillingCommand{
				RequestID: "grok-video:" + uuid.NewString(), APIKeyID: key.ID, UserID: user.ID,
				AccountID: account.ID, AccountType: account.Type, Model: "grok-imagine-video-1.5",
				BalanceCost: 1.25, APIKeyQuotaCost: 1.25, APIKeyRateLimitCost: 1.25, AccountQuotaCost: 1.25,
			}
			mode, resolution, duration := "video", "720p", 5
			log := &service.UsageLog{
				RequestID: cmd.RequestID, APIKeyID: key.ID, UserID: user.ID, AccountID: account.ID,
				Model: cmd.Model, GroupID: &group.ID, TotalCost: 1.25, ActualCost: 1.25,
				RateMultiplier: 1, BillingMode: &mode, VideoCount: 1, VideoResolution: &resolution, VideoDurationSeconds: &duration,
			}
			if subscriptionBilling {
				cmd.BalanceCost = 0
				cmd.SubscriptionCost = 1.25
				cmd.SubscriptionID = &subscription.ID
				cmd.BillingType = service.BillingTypeSubscription
				log.SubscriptionID = &subscription.ID
				log.BillingType = service.BillingTypeSubscription
			}

			assertState := func(billed bool) {
				t.Helper()
				count, cost, balance := 0, 0.0, 100.0
				if billed {
					count, cost = 1, 1.25
					if !subscriptionBilling {
						balance -= cost
					}
				}
				var actualBalance, quota, usage5h, usage1d, usage7d, accountQuota float64
				var status string
				require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&actualBalance))
				require.Equal(t, balance, actualBalance)
				require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT quota_used, usage_5h, usage_1d, usage_7d, status FROM api_keys WHERE id = $1", key.ID).Scan(&quota, &usage5h, &usage1d, &usage7d, &status))
				require.Equal(t, cost, quota)
				require.Equal(t, cost, usage5h)
				require.Equal(t, cost, usage1d)
				require.Equal(t, cost, usage7d)
				if billed {
					require.Equal(t, service.StatusAPIKeyQuotaExhausted, status)
				} else {
					require.Equal(t, service.StatusAPIKeyActive, status)
				}
				require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COALESCE((extra->>'quota_used')::numeric, 0) FROM accounts WHERE id = $1", account.ID).Scan(&accountQuota))
				require.Equal(t, cost, accountQuota)
				var daily, weekly, monthly float64
				require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT daily_usage_usd, weekly_usage_usd, monthly_usage_usd FROM user_subscriptions WHERE id = $1", subscription.ID).Scan(&daily, &weekly, &monthly))
				subCost := 0.0
				if subscriptionBilling {
					subCost = cost
				}
				require.Equal(t, subCost, daily)
				require.Equal(t, subCost, weekly)
				require.Equal(t, subCost, monthly)
				for _, table := range []string{"usage_logs", "usage_billing_dedup"} {
					var actualCount int
					require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE request_id = $1 AND api_key_id = $2", cmd.RequestID, key.ID).Scan(&actualCount))
					require.Equal(t, count, actualCount, table)
				}
			}

			// Only the usage row has this invalid VARCHAR(10) value, so failure
			// occurs after balance/subscription, key and account quota updates.
			invalidResolution := strings.Repeat("x", 11)
			log.VideoResolution = &invalidResolution
			result, err := repo.ApplyVideo(ctx, cmd, log)
			require.Error(t, err)
			var pgErr *pq.Error
			require.ErrorAs(t, err, &pgErr)
			require.Equal(t, pq.ErrorCode("22001"), pgErr.Code)
			require.Nil(t, result)
			require.Zero(t, log.ID)
			assertState(false)

			log.VideoResolution = &resolution
			missingGroupID := int64(-1)
			log.GroupID = &missingGroupID
			result, err = repo.ApplyVideo(ctx, cmd, log)
			require.Error(t, err)
			require.ErrorAs(t, err, &pgErr)
			require.Equal(t, pq.ErrorCode("23503"), pgErr.Code, "usage log foreign key failure must propagate")
			require.Nil(t, result)
			require.Zero(t, log.ID)
			assertState(false)

			log.GroupID = &group.ID
			const retries = 8
			results := make([]*service.UsageBillingApplyResult, retries)
			errs := make([]error, retries)
			var wg sync.WaitGroup
			for i := range results {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					localCmd, localLog := *cmd, *log
					results[i], errs[i] = repo.ApplyVideo(ctx, &localCmd, &localLog)
				}(i)
			}
			wg.Wait()
			applied := 0
			for i := range results {
				require.NoError(t, errs[i])
				require.NotNil(t, results[i])
				if results[i].Applied {
					applied++
				}
			}
			require.Equal(t, 1, applied)
			assertState(true)
			var persistedResolution, persistedMode string
			var videoCount, seconds int
			var actualCost float64
			require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT video_count, video_resolution, video_duration_seconds, billing_mode, actual_cost FROM usage_logs WHERE request_id = $1 AND api_key_id = $2", cmd.RequestID, key.ID).Scan(&videoCount, &persistedResolution, &seconds, &persistedMode, &actualCost))
			require.Equal(t, 1, videoCount)
			require.Equal(t, resolution, persistedResolution)
			require.Equal(t, duration, seconds)
			require.Equal(t, mode, persistedMode)
			require.Equal(t, 1.25, actualCost)
		})
	}
}
