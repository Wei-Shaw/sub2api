//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// C-04：批量生图的核销只在 users.balance / frozen_balance 之间搬钱，
// 从不累加 api_keys.quota_used、5h/1d/7d 窗口和 user×platform 配额，
// 其它计费端点都经 applyUsageBilling 完成这些累加。

type c04APIKeyReader struct {
	key   *APIKey
	err   error
	calls int
}

func (r *c04APIKeyReader) GetByID(_ context.Context, id int64) (*APIKey, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	if r.key == nil || r.key.ID != id {
		return nil, ErrAPIKeyNotFound
	}
	return r.key, nil
}

type c04BillingCache struct {
	BillingCache
	mu                  sync.Mutex
	entry               *UserPlatformQuotaCacheEntry
	platformIncrs       []incrCall
	rateLimitInvalidate []int64
}

func (c *c04BillingCache) GetUserPlatformQuotaCache(_ context.Context, _ int64, _ string) (*UserPlatformQuotaCacheEntry, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.entry, c.entry != nil, nil
}

func (c *c04BillingCache) IncrUserPlatformQuotaUsageCache(_ context.Context, userID int64, platform string, cost float64, ttl time.Duration, markDirty bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.platformIncrs = append(c.platformIncrs, incrCall{userID: userID, platform: platform, cost: cost, ttl: ttl, markDirty: markDirty})
	return nil
}

func (c *c04BillingCache) InvalidateAPIKeyRateLimit(_ context.Context, keyID int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rateLimitInvalidate = append(c.rateLimitInvalidate, keyID)
	return nil
}

// c04CapturingBillingRepo 在 fakeBatchImageBillingRepo 之上模拟真实仓储的 Captured 语义。
type c04CapturingBillingRepo struct {
	fakeBatchImageBillingRepo
	quotaExhausted bool
}

func (r *c04CapturingBillingRepo) CaptureBatchImageBalance(ctx context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	result, err := r.fakeBatchImageBillingRepo.CaptureBatchImageBalance(ctx, cmd)
	if err != nil || result == nil || !result.Applied {
		return result, err
	}
	result.Captured = cmd.ActualAmount > 0 || cmd.HoldAmount > 0
	result.APIKeyQuotaExhausted = r.quotaExhausted && cmd.APIKeyQuotaCost > 0
	return result, nil
}

func newC04SettlementService(job *BatchImageJob, key *APIKey) (*BatchImageSettlementService, *c04CapturingBillingRepo, *c04BillingCache, *fakeBatchImageAuthCacheInvalidator) {
	repo := newFakeBatchImageRepository()
	repo.jobs[job.BatchID] = job
	billing := &c04CapturingBillingRepo{}
	daily := 5.0
	cache := &c04BillingCache{entry: &UserPlatformQuotaCacheEntry{DailyLimitUSD: &daily}}
	cfg := &config.Config{}
	cfg.Billing.UserPlatformQuotaCacheTTLSeconds = 60
	// flusher 开启时只写 Redis（带脏标记），避免测试依赖异步 DB goroutine。
	cfg.Database.UserPlatformQuotaFlusherEnabled = true
	authCache := &fakeBatchImageAuthCacheInvalidator{}
	svc := &BatchImageSettlementService{
		Repo:                  repo,
		BillingRepo:           billing,
		Pricing:               &fakeBatchImagePricingResolver{unitPrice: 0.25},
		AuthCache:             authCache,
		Config:                cfg,
		APIKeyRepo:            &c04APIKeyReader{key: key},
		BillingCache:          &BillingCacheService{cache: cache, cfg: cfg},
		UserPlatformQuotaRepo: &fakeQuotaRepo{},
	}
	return svc, billing, cache, authCache
}

func c04LimitedAPIKey() *APIKey {
	groupID := int64(9)
	return &APIKey{
		ID:          321,
		UserID:      123,
		Key:         "sk-c04",
		Status:      StatusAPIKeyActive,
		Quota:       10,
		QuotaUsed:   1,
		RateLimit5h: 5,
		RateLimit1d: 20,
		GroupID:     &groupID,
		Group:       &Group{ID: groupID, Platform: PlatformGemini},
	}
}

func TestBatchImageSettlementAppliesAPIKeyQuotaAndRateLimitCosts(t *testing.T) {
	job := testSettlingBatchImageJob("imgbatch_c04_limits")
	svc, billing, cache, _ := newC04SettlementService(job, c04LimitedAPIKey())

	result, err := svc.Settle(context.Background(), job.BatchID)
	require.NoError(t, err)
	require.Equal(t, 0.5, result.ActualCost)

	require.Len(t, billing.captures, 1)
	capture := billing.captures[0]
	// 扣多少钱不变：核销仍是 2 张 × 0.25。
	require.Equal(t, 0.5, capture.ActualAmount)
	require.Equal(t, 1.25, capture.HoldAmount)
	// 同一事务里累加 API Key 总额度与 5h/1d/7d 窗口。
	require.Equal(t, 0.5, capture.APIKeyQuotaCost)
	require.Equal(t, 0.5, capture.APIKeyRateLimitCost)

	// user×platform 配额按核销金额累加（Redis 同步写，flusher 负责落库）。
	require.Equal(t, []incrCall{{userID: job.UserID, platform: PlatformGemini, cost: 0.5, ttl: 60 * time.Second, markDirty: true}}, cache.platformIncrs)
	// 窗口用量缓存失效，下次 preflight 从数据库重新加载已累加的值。
	require.Equal(t, []int64{321}, cache.rateLimitInvalidate)
}

func TestBatchImageSettlementSkipsLimitCostsForUnlimitedAPIKey(t *testing.T) {
	job := testSettlingBatchImageJob("imgbatch_c04_unlimited")
	key := c04LimitedAPIKey()
	key.Quota = 0
	key.RateLimit5h = 0
	key.RateLimit1d = 0
	svc, billing, cache, _ := newC04SettlementService(job, key)

	_, err := svc.Settle(context.Background(), job.BatchID)
	require.NoError(t, err)
	require.Len(t, billing.captures, 1)
	require.Zero(t, billing.captures[0].APIKeyQuotaCost)
	require.Zero(t, billing.captures[0].APIKeyRateLimitCost)
	require.Empty(t, cache.rateLimitInvalidate)
	// user×platform 配额与 Key 配置无关，仍然累加。
	require.Len(t, cache.platformIncrs, 1)
}

func TestBatchImageSettlementDeletedAPIKeyStillCaptures(t *testing.T) {
	job := testSettlingBatchImageJob("imgbatch_c04_deleted_key")
	svc, billing, _, _ := newC04SettlementService(job, nil)

	_, err := svc.Settle(context.Background(), job.BatchID)
	require.NoError(t, err)
	require.Len(t, billing.captures, 1)
	require.Equal(t, 0.5, billing.captures[0].ActualAmount)
	require.Zero(t, billing.captures[0].APIKeyQuotaCost)
	require.Zero(t, billing.captures[0].APIKeyRateLimitCost)
}

func TestBatchImageSettlementAPIKeyLookupFailureIsRetried(t *testing.T) {
	job := testSettlingBatchImageJob("imgbatch_c04_lookup_failure")
	svc, billing, _, _ := newC04SettlementService(job, c04LimitedAPIKey())
	svc.APIKeyRepo = &c04APIKeyReader{err: errors.New("db down")}

	_, err := svc.Settle(context.Background(), job.BatchID)
	require.ErrorIs(t, err, ErrBatchImageSettlementBillingFailed)
	require.Empty(t, billing.captures)
	require.Equal(t, BatchImageJobStatusSettling, svc.Repo.(*fakeBatchImageRepository).jobs[job.BatchID].Status)
}

func TestBatchImageSettlementReplayDoesNotDoubleCountPlatformQuota(t *testing.T) {
	job := testSettlingBatchImageJob("imgbatch_c04_replay")
	svc, billing, cache, _ := newC04SettlementService(job, c04LimitedAPIKey())
	billing.alreadyApplied = map[string]bool{BatchImageCaptureRequestID(job.BatchID): true}

	_, err := svc.Settle(context.Background(), job.BatchID)
	require.NoError(t, err)
	require.Len(t, billing.captures, 1)
	require.Empty(t, cache.platformIncrs)
	require.Empty(t, cache.rateLimitInvalidate)
}

func TestBatchImageSettlementQuotaExhaustedInvalidatesKeyAuthCache(t *testing.T) {
	job := testSettlingBatchImageJob("imgbatch_c04_exhausted")
	svc, billing, _, authCache := newC04SettlementService(job, c04LimitedAPIKey())
	billing.quotaExhausted = true

	_, err := svc.Settle(context.Background(), job.BatchID)
	require.NoError(t, err)
	require.Equal(t, []string{"sk-c04"}, authCache.keys)
	require.Contains(t, authCache.userIDs, job.UserID)
}

func TestBatchImageSubmitRejectsEstimateAboveRemainingAPIKeyQuota(t *testing.T) {
	ctx := context.Background()
	svc, repo, queue, gemini, _ := newTestBatchImagePublicService(true)
	owner := testBatchImageOwner()
	// 2 张 × 0.125（含批量折扣）= 0.25，剩余额度只有 0.2。
	owner.APIKeyQuota = 1
	owner.APIKeyQuotaUsed = 0.8

	_, err := svc.Submit(ctx, owner, validBatchImageSubmitRequest(), "")
	require.ErrorIs(t, err, ErrBatchImageAPIKeyQuotaInsufficient)
	require.Empty(t, repo.jobs)
	require.Empty(t, gemini.submits)
	require.Empty(t, queue.enqueued)
	require.Empty(t, svc.BillingRepo.(*fakeBatchImageBillingRepo).reserves)
}

func TestBatchImageSubmitAllowsEstimateWithinRemainingAPIKeyQuota(t *testing.T) {
	ctx := context.Background()
	svc, repo, _, _, _ := newTestBatchImagePublicService(true)
	owner := testBatchImageOwner()
	owner.APIKeyQuota = 1
	owner.APIKeyQuotaUsed = 0.75

	got, err := svc.Submit(ctx, owner, validBatchImageSubmitRequest(), "")
	require.NoError(t, err)
	require.Len(t, repo.jobs, 1)
	require.Equal(t, 0.25, got.EstimatedCost)
}
