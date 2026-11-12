package service

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type transientExpirySnapshotCache struct {
	SchedulerCache
	accounts []*Account
	hit      bool
}

func (c *transientExpirySnapshotCache) GetSnapshot(context.Context, SchedulerBucket) ([]*Account, bool, error) {
	return c.accounts, c.hit, nil
}

func (c *transientExpirySnapshotCache) CaptureBucketWriteToken(_ context.Context, bucket SchedulerBucket) (SchedulerBucketWriteToken, error) {
	return SchedulerBucketWriteToken{Bucket: bucket, Epoch: 1}, nil
}

func (c *transientExpirySnapshotCache) SetSnapshot(_ context.Context, _ SchedulerBucket, _ SchedulerBucketWriteToken, accounts []Account) error {
	c.accounts = nil
	for _, account := range accounts {
		copy := account
		c.accounts = append(c.accounts, &copy)
	}
	c.hit = true
	return nil
}

type transientExpiryAccountRepo struct {
	AccountRepository
	accounts []Account
	calls    int
}

func (r *transientExpiryAccountRepo) ListModelAvailabilityCandidates(context.Context, *int64, []string, bool) ([]Account, error) {
	r.calls++
	var accounts []Account
	for _, account := range r.accounts {
		if account.IsActive() && account.Schedulable {
			accounts = append(accounts, account)
		}
	}
	return accounts, nil
}

// Match the current repository contract: the source query excludes accounts
// while any transient cooldown is in the future.
func (r *transientExpiryAccountRepo) ListSchedulableByPlatform(context.Context, string) ([]Account, error) {
	r.calls++
	var accounts []Account
	for _, account := range r.accounts {
		if account.IsSchedulable() {
			accounts = append(accounts, account)
		}
	}
	return accounts, nil
}

func TestSchedulerSnapshotRecoversTransientCooldownWithoutIntervalRebuild(t *testing.T) {
	for _, field := range []string{"rate_limit_reset_at", "overload_until", "temp_unschedulable_until"} {
		t.Run(field, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				until := time.Now().Add(30 * time.Second)
				past := time.Now().Add(-time.Second)
				ready := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true}
				cooling := ready
				cooling.ID = 2
				switch field {
				case "rate_limit_reset_at":
					cooling.RateLimitResetAt = &until
				case "overload_until":
					cooling.OverloadUntil = &until
				case "temp_unschedulable_until":
					cooling.TempUnschedulableUntil = &until
				}
				disabled := ready
				disabled.ID, disabled.Status = 3, StatusDisabled
				manual := ready
				manual.ID, manual.Schedulable = 4, false
				expired := ready
				expired.ID, expired.AutoPauseOnExpired, expired.ExpiresAt = 5, true, &past
				expiring := ready
				expiring.ID, expiring.AutoPauseOnExpired, expiring.ExpiresAt = 6, true, &until
				repo := &transientExpiryAccountRepo{accounts: []Account{ready, cooling, disabled, manual, expired, expiring}}
				cache := &transientExpirySnapshotCache{}
				cfg := &config.Config{RunMode: config.RunModeSimple}
				cfg.Gateway.Scheduling.DbFallbackEnabled = true
				svc := NewSchedulerSnapshotService(cache, nil, repo, nil, cfg)
				eligible := func(accounts []Account) []int64 {
					var ids []int64
					for _, account := range accounts {
						if account.IsSchedulable() {
							ids = append(ids, account.ID)
						}
					}
					return ids
				}

				accounts, _, err := svc.ListSchedulableAccounts(context.Background(), nil, PlatformOpenAI, false)
				require.NoError(t, err)
				require.Equal(t, []int64{1, 6}, eligible(accounts))
				require.Len(t, accounts, 2, "the DB fallback returns only currently eligible accounts")
				require.True(t, cache.hit)
				require.Len(t, cache.accounts, 4, "the stored pool retains cooling and expired configured members")
				require.Equal(t, 1, repo.calls)

				time.Sleep(31 * time.Second)
				require.True(t, cooling.IsSchedulable(), "the account's own cooldown has expired")
				accounts, _, err = svc.ListSchedulableAccounts(context.Background(), nil, PlatformOpenAI, false)
				require.NoError(t, err)
				t.Logf("after expiry: eligible=%v cached_members=%d source_calls=%d", eligible(accounts), len(cache.accounts), repo.calls)
				assert.ElementsMatch(t, []int64{1, 2}, eligible(accounts), "an expired cooldown must not wait for the interval rebuild")
				require.Len(t, accounts, 2, "the cache hit excludes disabled, manually paused and expired accounts")
				require.Equal(t, 1, repo.calls, "expiry must not require a database read")

				// Positive control: a fresh source read immediately restores the account.
				cache.hit = false
				accounts, _, err = svc.ListSchedulableAccounts(context.Background(), nil, PlatformOpenAI, false)
				require.NoError(t, err)
				require.ElementsMatch(t, []int64{1, 2}, eligible(accounts))
			})
		})
	}
}
