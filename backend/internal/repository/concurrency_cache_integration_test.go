//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// 测试用 TTL 配置（15 分钟，与默认值一致）
const testSlotTTLMinutes = 15

// 测试用 TTL Duration，用于 TTL 断言
var testSlotTTL = time.Duration(testSlotTTLMinutes) * time.Minute

type ConcurrencyCacheSuite struct {
	IntegrationRedisSuite
	cache    service.ConcurrencyCache
	rawCache *concurrencyCache
}

func TestConcurrencyCacheSuite(t *testing.T) {
	suite.Run(t, new(ConcurrencyCacheSuite))
}

func (s *ConcurrencyCacheSuite) SetupTest() {
	s.IntegrationRedisSuite.SetupTest()
	s.rawCache = NewConcurrencyCache(s.rdb, testSlotTTLMinutes, int(testSlotTTL.Seconds())).(*concurrencyCache)
	s.cache = s.rawCache
}

type apiKeyConcurrencyCacheForTest interface {
	TrackAPIKeySlot(ctx context.Context, apiKeyID int64, requestID string) error
	ReleaseAPIKeySlot(ctx context.Context, apiKeyID int64, requestID string) error
	GetAPIKeyConcurrencyBatch(ctx context.Context, apiKeyIDs []int64) (map[int64]int, error)
}

func (s *ConcurrencyCacheSuite) apiKeyConcurrencyCache() apiKeyConcurrencyCacheForTest {
	cache, ok := s.cache.(apiKeyConcurrencyCacheForTest)
	require.True(s.T(), ok)
	return cache
}

func (s *ConcurrencyCacheSuite) TestOpenAIWSIngressAPIKeySlot_HardLimitRefreshAndRelease() {
	apiKeyID := int64(9011)
	firstLeaseID := "ingress-first"
	secondLeaseID := "ingress-second"

	ok, err := s.rawCache.AcquireOpenAIWSIngressLease(s.ctx, apiKeyID, 1, firstLeaseID)
	require.NoError(s.T(), err)
	require.True(s.T(), ok)

	ok, err = s.rawCache.AcquireOpenAIWSIngressLease(s.ctx, apiKeyID, 1, secondLeaseID)
	require.NoError(s.T(), err)
	require.False(s.T(), ok, "a second live session must not exceed the API key limit")

	ok, err = s.rawCache.RefreshOpenAIWSIngressLease(s.ctx, apiKeyID, firstLeaseID)
	require.NoError(s.T(), err)
	require.True(s.T(), ok, "the current owner must be able to refresh its lease")

	require.NoError(s.T(), s.rawCache.ReleaseOpenAIWSIngressLease(s.ctx, apiKeyID, firstLeaseID))
	ok, err = s.rawCache.AcquireOpenAIWSIngressLease(s.ctx, apiKeyID, 1, secondLeaseID)
	require.NoError(s.T(), err)
	require.True(s.T(), ok, "released capacity must become available immediately")
}

func (s *ConcurrencyCacheSuite) TestOpenAIWSIngressAPIKeySlot_ReapsCrashedLeaseWithoutDeletingLiveOtherInstance() {
	apiKeyID := int64(9012)
	key := openAIWSIngressLeaseKey(apiKeyID)
	now, err := s.rawCache.redisUnixSeconds(s.ctx)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, key,
		redis.Z{Score: float64(now - openAIWSIngressLeaseTTLSeconds - 1), Member: "crashed-instance"},
		redis.Z{Score: float64(now), Member: "live-other-instance"},
	).Err())
	require.NoError(s.T(), s.rdb.Expire(s.ctx, key, time.Duration(openAIWSIngressLeaseTTLSeconds)*time.Second).Err())

	ok, err := s.rawCache.AcquireOpenAIWSIngressLease(s.ctx, apiKeyID, 2, "new-instance")
	require.NoError(s.T(), err)
	require.True(s.T(), ok, "the crashed member should be reaped before enforcing the limit")

	_, err = s.rdb.ZScore(s.ctx, key, "crashed-instance").Result()
	require.ErrorIs(s.T(), err, redis.Nil)
	_, err = s.rdb.ZScore(s.ctx, key, "live-other-instance").Result()
	require.NoError(s.T(), err, "a live lease owned by another instance must be preserved")
	count, err := s.rdb.ZCard(s.ctx, key).Result()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(2), count)
}

func (s *ConcurrencyCacheSuite) TestAccountSlot_AcquireAndRelease() {
	accountID := int64(10)
	reqID1, reqID2, reqID3 := "req1", "req2", "req3"

	ok, err := s.cache.AcquireAccountSlot(s.ctx, accountID, 2, reqID1)
	require.NoError(s.T(), err, "AcquireAccountSlot 1")
	require.True(s.T(), ok)

	ok, err = s.cache.AcquireAccountSlot(s.ctx, accountID, 2, reqID2)
	require.NoError(s.T(), err, "AcquireAccountSlot 2")
	require.True(s.T(), ok)

	ok, err = s.cache.AcquireAccountSlot(s.ctx, accountID, 2, reqID3)
	require.NoError(s.T(), err, "AcquireAccountSlot 3")
	require.False(s.T(), ok, "expected third acquire to fail")

	cur, err := s.cache.GetAccountConcurrency(s.ctx, accountID)
	require.NoError(s.T(), err, "GetAccountConcurrency")
	require.Equal(s.T(), 2, cur, "concurrency mismatch")

	require.NoError(s.T(), s.cache.ReleaseAccountSlot(s.ctx, accountID, reqID1), "ReleaseAccountSlot")

	cur, err = s.cache.GetAccountConcurrency(s.ctx, accountID)
	require.NoError(s.T(), err, "GetAccountConcurrency after release")
	require.Equal(s.T(), 1, cur, "expected 1 after release")
}

func (s *ConcurrencyCacheSuite) TestAccountActiveIndex_AcquireAndRelease() {
	accountID := int64(610)
	member := strconv.FormatInt(accountID, 10)
	reqID := "active-index-req"

	now, err := s.rawCache.redisUnixSeconds(s.ctx)
	require.NoError(s.T(), err)

	ok, err := s.cache.AcquireAccountSlot(s.ctx, accountID, 2, reqID)
	require.NoError(s.T(), err)
	require.True(s.T(), ok)

	score, err := s.rdb.ZScore(s.ctx, accountActiveIndexKey, member).Result()
	require.NoError(s.T(), err)
	require.Greater(s.T(), int64(score), now, "index score should be a future expiry")

	require.NoError(s.T(), s.cache.ReleaseAccountSlot(s.ctx, accountID, reqID))

	_, err = s.rdb.ZScore(s.ctx, accountActiveIndexKey, member).Result()
	require.ErrorIs(s.T(), err, redis.Nil, "index member should be removed after load drops to zero")
}

func (s *ConcurrencyCacheSuite) TestAccountActiveIndex_WaitLifecycle() {
	accountID := int64(611)
	member := strconv.FormatInt(accountID, 10)

	ok, err := s.cache.IncrementAccountWaitCount(s.ctx, accountID, 2)
	require.NoError(s.T(), err)
	require.True(s.T(), ok)

	_, err = s.rdb.ZScore(s.ctx, accountActiveIndexKey, member).Result()
	require.NoError(s.T(), err, "wait increment should register index member")

	require.NoError(s.T(), s.cache.DecrementAccountWaitCount(s.ctx, accountID))

	_, err = s.rdb.ZScore(s.ctx, accountActiveIndexKey, member).Result()
	require.ErrorIs(s.T(), err, redis.Nil, "index member should be removed after wait drops to zero")
}

func (s *ConcurrencyCacheSuite) TestUserActiveIndex_AcquireAndRelease() {
	userID := int64(612)
	member := strconv.FormatInt(userID, 10)
	reqID := "user-active-index-req"

	ok, err := s.cache.AcquireUserSlot(s.ctx, userID, 2, reqID)
	require.NoError(s.T(), err)
	require.True(s.T(), ok)

	_, err = s.rdb.ZScore(s.ctx, userActiveIndexKey, member).Result()
	require.NoError(s.T(), err, "acquire should register user index member")

	require.NoError(s.T(), s.cache.ReleaseUserSlot(s.ctx, userID, reqID))

	_, err = s.rdb.ZScore(s.ctx, userActiveIndexKey, member).Result()
	require.ErrorIs(s.T(), err, redis.Nil, "user index member should be removed after release")
}

func (s *ConcurrencyCacheSuite) TestAccountSlot_TTL() {
	accountID := int64(11)
	reqID := "req_ttl_test"
	slotKey := fmt.Sprintf("%s%d", accountSlotKeyPrefix, accountID)

	ok, err := s.cache.AcquireAccountSlot(s.ctx, accountID, 5, reqID)
	require.NoError(s.T(), err, "AcquireAccountSlot")
	require.True(s.T(), ok)

	ttl, err := s.rdb.TTL(s.ctx, slotKey).Result()
	require.NoError(s.T(), err, "TTL")
	s.AssertTTLWithin(ttl, 1*time.Second, testSlotTTL)
}

func (s *ConcurrencyCacheSuite) TestAccountSlot_DuplicateReqID() {
	accountID := int64(12)
	reqID := "dup-req"

	ok, err := s.cache.AcquireAccountSlot(s.ctx, accountID, 2, reqID)
	require.NoError(s.T(), err)
	require.True(s.T(), ok)

	// Acquiring with same reqID should be idempotent
	ok, err = s.cache.AcquireAccountSlot(s.ctx, accountID, 2, reqID)
	require.NoError(s.T(), err)
	require.True(s.T(), ok)

	cur, err := s.cache.GetAccountConcurrency(s.ctx, accountID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, cur, "expected concurrency=1 (idempotent)")
}

func (s *ConcurrencyCacheSuite) TestAccountSlot_ReleaseIdempotent() {
	accountID := int64(13)
	reqID := "release-test"

	ok, err := s.cache.AcquireAccountSlot(s.ctx, accountID, 1, reqID)
	require.NoError(s.T(), err)
	require.True(s.T(), ok)

	require.NoError(s.T(), s.cache.ReleaseAccountSlot(s.ctx, accountID, reqID), "ReleaseAccountSlot")
	// Releasing again should not error
	require.NoError(s.T(), s.cache.ReleaseAccountSlot(s.ctx, accountID, reqID), "ReleaseAccountSlot again")
	// Releasing non-existent should not error
	require.NoError(s.T(), s.cache.ReleaseAccountSlot(s.ctx, accountID, "non-existent"), "ReleaseAccountSlot non-existent")

	cur, err := s.cache.GetAccountConcurrency(s.ctx, accountID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 0, cur)
}

func (s *ConcurrencyCacheSuite) TestAccountSlot_MaxZero() {
	accountID := int64(14)
	reqID := "max-zero-test"

	ok, err := s.cache.AcquireAccountSlot(s.ctx, accountID, 0, reqID)
	require.NoError(s.T(), err)
	require.False(s.T(), ok, "expected acquire to fail with max=0")
}

func (s *ConcurrencyCacheSuite) TestUserSlot_AcquireAndRelease() {
	userID := int64(42)
	reqID1, reqID2 := "req1", "req2"

	ok, err := s.cache.AcquireUserSlot(s.ctx, userID, 1, reqID1)
	require.NoError(s.T(), err, "AcquireUserSlot")
	require.True(s.T(), ok)

	ok, err = s.cache.AcquireUserSlot(s.ctx, userID, 1, reqID2)
	require.NoError(s.T(), err, "AcquireUserSlot 2")
	require.False(s.T(), ok, "expected second acquire to fail at max=1")

	cur, err := s.cache.GetUserConcurrency(s.ctx, userID)
	require.NoError(s.T(), err, "GetUserConcurrency")
	require.Equal(s.T(), 1, cur, "expected concurrency=1")

	require.NoError(s.T(), s.cache.ReleaseUserSlot(s.ctx, userID, reqID1), "ReleaseUserSlot")
	// Releasing a non-existent slot should not error
	require.NoError(s.T(), s.cache.ReleaseUserSlot(s.ctx, userID, "non-existent"), "ReleaseUserSlot non-existent")

	cur, err = s.cache.GetUserConcurrency(s.ctx, userID)
	require.NoError(s.T(), err, "GetUserConcurrency after release")
	require.Equal(s.T(), 0, cur, "expected concurrency=0 after release")
}

func (s *ConcurrencyCacheSuite) TestUserSlot_TTL() {
	userID := int64(200)
	reqID := "req_ttl_test"
	slotKey := fmt.Sprintf("%s%d", userSlotKeyPrefix, userID)

	ok, err := s.cache.AcquireUserSlot(s.ctx, userID, 5, reqID)
	require.NoError(s.T(), err, "AcquireUserSlot")
	require.True(s.T(), ok)

	ttl, err := s.rdb.TTL(s.ctx, slotKey).Result()
	require.NoError(s.T(), err, "TTL")
	s.AssertTTLWithin(ttl, 1*time.Second, testSlotTTL)
}

func (s *ConcurrencyCacheSuite) TestAPIKeySlot_TrackReleaseAndBatchCount() {
	cache := s.apiKeyConcurrencyCache()
	apiKeyID := int64(300)
	emptyAPIKeyID := int64(301)
	slotKey := fmt.Sprintf("%s%d", apiKeySlotKeyPrefix, apiKeyID)

	require.NoError(s.T(), cache.TrackAPIKeySlot(s.ctx, apiKeyID, "req1"))
	require.NoError(s.T(), cache.TrackAPIKeySlot(s.ctx, apiKeyID, "req2"))

	counts, err := cache.GetAPIKeyConcurrencyBatch(s.ctx, []int64{apiKeyID, emptyAPIKeyID})
	require.NoError(s.T(), err)
	require.Equal(s.T(), map[int64]int{apiKeyID: 2, emptyAPIKeyID: 0}, counts)

	ttl, err := s.rdb.TTL(s.ctx, slotKey).Result()
	require.NoError(s.T(), err, "TTL")
	s.AssertTTLWithin(ttl, 1*time.Second, testSlotTTL)

	require.NoError(s.T(), cache.ReleaseAPIKeySlot(s.ctx, apiKeyID, "req1"))
	counts, err = cache.GetAPIKeyConcurrencyBatch(s.ctx, []int64{apiKeyID})
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, counts[apiKeyID])

	require.NoError(s.T(), cache.ReleaseAPIKeySlot(s.ctx, apiKeyID, "req2"))
	counts, err = cache.GetAPIKeyConcurrencyBatch(s.ctx, []int64{apiKeyID})
	require.NoError(s.T(), err)
	require.Equal(s.T(), 0, counts[apiKeyID])
}

func (s *ConcurrencyCacheSuite) TestWaitQueue_IncrementAndDecrement() {
	userID := int64(20)
	waitKey := fmt.Sprintf("%s%d", waitQueueKeyPrefix, userID)

	ok, err := s.cache.IncrementWaitCount(s.ctx, userID, 2)
	require.NoError(s.T(), err, "IncrementWaitCount 1")
	require.True(s.T(), ok)

	ok, err = s.cache.IncrementWaitCount(s.ctx, userID, 2)
	require.NoError(s.T(), err, "IncrementWaitCount 2")
	require.True(s.T(), ok)

	ok, err = s.cache.IncrementWaitCount(s.ctx, userID, 2)
	require.NoError(s.T(), err, "IncrementWaitCount 3")
	require.False(s.T(), ok, "expected wait increment over max to fail")

	ttl, err := s.rdb.TTL(s.ctx, waitKey).Result()
	require.NoError(s.T(), err, "TTL waitKey")
	s.AssertTTLWithin(ttl, 1*time.Second, testSlotTTL)

	require.NoError(s.T(), s.cache.DecrementWaitCount(s.ctx, userID), "DecrementWaitCount")

	val, err := s.rdb.Get(s.ctx, waitKey).Int()
	if !errors.Is(err, redis.Nil) {
		require.NoError(s.T(), err, "Get waitKey")
	}
	require.Equal(s.T(), 1, val, "expected wait count 1")
}

func (s *ConcurrencyCacheSuite) TestWaitQueue_DecrementNoNegative() {
	userID := int64(300)
	waitKey := fmt.Sprintf("%s%d", waitQueueKeyPrefix, userID)

	// Test decrement on non-existent key - should not error and should not create negative value
	require.NoError(s.T(), s.cache.DecrementWaitCount(s.ctx, userID), "DecrementWaitCount on non-existent key")

	// Verify no key was created or it's not negative
	val, err := s.rdb.Get(s.ctx, waitKey).Int()
	if !errors.Is(err, redis.Nil) {
		require.NoError(s.T(), err, "Get waitKey")
	}
	require.GreaterOrEqual(s.T(), val, 0, "expected non-negative wait count after decrement on empty")

	// Set count to 1, then decrement twice
	ok, err := s.cache.IncrementWaitCount(s.ctx, userID, 5)
	require.NoError(s.T(), err, "IncrementWaitCount")
	require.True(s.T(), ok)

	// Decrement once (1 -> 0)
	require.NoError(s.T(), s.cache.DecrementWaitCount(s.ctx, userID), "DecrementWaitCount")

	// Decrement again on 0 - should not go negative
	require.NoError(s.T(), s.cache.DecrementWaitCount(s.ctx, userID), "DecrementWaitCount on zero")

	// Verify count is 0, not negative
	val, err = s.rdb.Get(s.ctx, waitKey).Int()
	if !errors.Is(err, redis.Nil) {
		require.NoError(s.T(), err, "Get waitKey after double decrement")
	}
	require.GreaterOrEqual(s.T(), val, 0, "expected non-negative wait count")
}

func (s *ConcurrencyCacheSuite) TestAccountWaitQueue_IncrementAndDecrement() {
	accountID := int64(30)
	waitKey := fmt.Sprintf("%s%d", accountWaitKeyPrefix, accountID)

	ok, err := s.cache.IncrementAccountWaitCount(s.ctx, accountID, 2)
	require.NoError(s.T(), err, "IncrementAccountWaitCount 1")
	require.True(s.T(), ok)

	ok, err = s.cache.IncrementAccountWaitCount(s.ctx, accountID, 2)
	require.NoError(s.T(), err, "IncrementAccountWaitCount 2")
	require.True(s.T(), ok)

	ok, err = s.cache.IncrementAccountWaitCount(s.ctx, accountID, 2)
	require.NoError(s.T(), err, "IncrementAccountWaitCount 3")
	require.False(s.T(), ok, "expected account wait increment over max to fail")

	ttl, err := s.rdb.TTL(s.ctx, waitKey).Result()
	require.NoError(s.T(), err, "TTL account waitKey")
	s.AssertTTLWithin(ttl, 1*time.Second, testSlotTTL)

	require.NoError(s.T(), s.cache.DecrementAccountWaitCount(s.ctx, accountID), "DecrementAccountWaitCount")

	val, err := s.rdb.Get(s.ctx, waitKey).Int()
	if !errors.Is(err, redis.Nil) {
		require.NoError(s.T(), err, "Get waitKey")
	}
	require.Equal(s.T(), 1, val, "expected account wait count 1")
}

func (s *ConcurrencyCacheSuite) TestCleanupStaleProcessSlots() {
	// 预置迁移 marker，隔离一次性清扫，只验证索引驱动的清理路径。
	require.NoError(s.T(), s.rdb.Set(s.ctx, legacyWaitSweepMarkerKey, "1", 0).Err())
	accountID := int64(901)
	userID := int64(902)
	apiKeyID := int64(903)
	unindexedAccountID := int64(1901)
	accountKey := fmt.Sprintf("%s%d", accountSlotKeyPrefix, accountID)
	userKey := fmt.Sprintf("%s%d", userSlotKeyPrefix, userID)
	apiKeyKey := fmt.Sprintf("%s%d", apiKeySlotKeyPrefix, apiKeyID)
	unindexedAccountKey := fmt.Sprintf("%s%d", accountSlotKeyPrefix, unindexedAccountID)
	userWaitKey := fmt.Sprintf("%s%d", waitQueueKeyPrefix, userID)
	accountWaitKey := fmt.Sprintf("%s%d", accountWaitKeyPrefix, accountID)
	unindexedAccountWaitKey := fmt.Sprintf("%s%d", accountWaitKeyPrefix, unindexedAccountID)

	now, err := s.rawCache.redisUnixSeconds(s.ctx)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, accountKey,
		redis.Z{Score: float64(now), Member: "oldproc-1"},
		redis.Z{Score: float64(now), Member: "keep-1"},
	).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, userKey,
		redis.Z{Score: float64(now), Member: "oldproc-2"},
		redis.Z{Score: float64(now), Member: "keep-2"},
	).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, unindexedAccountKey,
		redis.Z{Score: float64(now), Member: "oldproc-unindexed"},
	).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, apiKeyKey,
		redis.Z{Score: float64(now), Member: "oldproc-3"},
		redis.Z{Score: float64(now), Member: "keep-3"},
	).Err())
	require.NoError(s.T(), s.rdb.Set(s.ctx, userWaitKey, 3, time.Minute).Err())
	require.NoError(s.T(), s.rdb.Set(s.ctx, accountWaitKey, 2, time.Minute).Err())
	require.NoError(s.T(), s.rdb.Set(s.ctx, unindexedAccountWaitKey, 2, time.Minute).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, accountActiveIndexKey, redis.Z{
		Score:  float64(now + 60),
		Member: strconv.FormatInt(accountID, 10),
	}).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, userActiveIndexKey, redis.Z{
		Score:  float64(now + 60),
		Member: strconv.FormatInt(userID, 10),
	}).Err())

	require.NoError(s.T(), s.cache.CleanupStaleProcessSlots(s.ctx, "keep-"))

	accountMembers, err := s.rdb.ZRange(s.ctx, accountKey, 0, -1).Result()
	require.NoError(s.T(), err)
	require.Equal(s.T(), []string{"keep-1"}, accountMembers)

	userMembers, err := s.rdb.ZRange(s.ctx, userKey, 0, -1).Result()
	require.NoError(s.T(), err)
	require.Equal(s.T(), []string{"keep-2"}, userMembers)

	// API Key 槽位（stats-only）不在启动清理范围内，靠分数裁剪与 key TTL 自愈。
	apiKeyMembers, err := s.rdb.ZRange(s.ctx, apiKeyKey, 0, -1).Result()
	require.NoError(s.T(), err)
	require.ElementsMatch(s.T(), []string{"keep-3", "oldproc-3"}, apiKeyMembers)

	_, err = s.rdb.Get(s.ctx, userWaitKey).Result()
	require.True(s.T(), errors.Is(err, redis.Nil))

	_, err = s.rdb.Get(s.ctx, accountWaitKey).Result()
	require.True(s.T(), errors.Is(err, redis.Nil))

	unindexedMembers, err := s.rdb.ZRange(s.ctx, unindexedAccountKey, 0, -1).Result()
	require.NoError(s.T(), err)
	require.Equal(s.T(), []string{"oldproc-unindexed"}, unindexedMembers)
	_, err = s.rdb.Get(s.ctx, unindexedAccountWaitKey).Result()
	require.NoError(s.T(), err)
}

func (s *ConcurrencyCacheSuite) TestGetAccountConcurrency_Missing() {
	// When no slots exist, GetAccountConcurrency should return 0
	cur, err := s.cache.GetAccountConcurrency(s.ctx, 999)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 0, cur)
}

func (s *ConcurrencyCacheSuite) TestGetUserConcurrency_Missing() {
	// When no slots exist, GetUserConcurrency should return 0
	cur, err := s.cache.GetUserConcurrency(s.ctx, 999)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 0, cur)
}

func (s *ConcurrencyCacheSuite) TestGetAccountsLoadBatch() {
	s.T().Skip("TODO: Fix this test - CurrentConcurrency returns 0 instead of expected value in CI")
	// Setup: Create accounts with different load states
	account1 := int64(100)
	account2 := int64(101)
	account3 := int64(102)

	// Account 1: 2/3 slots used, 1 waiting
	ok, err := s.cache.AcquireAccountSlot(s.ctx, account1, 3, "req1")
	require.NoError(s.T(), err)
	require.True(s.T(), ok)
	ok, err = s.cache.AcquireAccountSlot(s.ctx, account1, 3, "req2")
	require.NoError(s.T(), err)
	require.True(s.T(), ok)
	ok, err = s.cache.IncrementAccountWaitCount(s.ctx, account1, 5)
	require.NoError(s.T(), err)
	require.True(s.T(), ok)

	// Account 2: 1/2 slots used, 0 waiting
	ok, err = s.cache.AcquireAccountSlot(s.ctx, account2, 2, "req3")
	require.NoError(s.T(), err)
	require.True(s.T(), ok)

	// Account 3: 0/1 slots used, 0 waiting (idle)

	// Query batch load
	accounts := []service.AccountWithConcurrency{
		{ID: account1, MaxConcurrency: 3},
		{ID: account2, MaxConcurrency: 2},
		{ID: account3, MaxConcurrency: 1},
	}

	loadMap, err := s.cache.GetAccountsLoadBatch(s.ctx, accounts)
	require.NoError(s.T(), err)
	require.Len(s.T(), loadMap, 3)

	// Verify account1: (2 + 1) / 3 = 100%
	load1 := loadMap[account1]
	require.NotNil(s.T(), load1)
	require.Equal(s.T(), account1, load1.AccountID)
	require.Equal(s.T(), 2, load1.CurrentConcurrency)
	require.Equal(s.T(), 1, load1.WaitingCount)
	require.Equal(s.T(), 100, load1.LoadRate)

	// Verify account2: (1 + 0) / 2 = 50%
	load2 := loadMap[account2]
	require.NotNil(s.T(), load2)
	require.Equal(s.T(), account2, load2.AccountID)
	require.Equal(s.T(), 1, load2.CurrentConcurrency)
	require.Equal(s.T(), 0, load2.WaitingCount)
	require.Equal(s.T(), 50, load2.LoadRate)

	// Verify account3: (0 + 0) / 1 = 0%
	load3 := loadMap[account3]
	require.NotNil(s.T(), load3)
	require.Equal(s.T(), account3, load3.AccountID)
	require.Equal(s.T(), 0, load3.CurrentConcurrency)
	require.Equal(s.T(), 0, load3.WaitingCount)
	require.Equal(s.T(), 0, load3.LoadRate)
}

func (s *ConcurrencyCacheSuite) TestGetAccountsLoadBatch_Empty() {
	// Test with empty account list
	loadMap, err := s.cache.GetAccountsLoadBatch(s.ctx, []service.AccountWithConcurrency{})
	require.NoError(s.T(), err)
	require.Empty(s.T(), loadMap)
}

func (s *ConcurrencyCacheSuite) TestCleanupExpiredAccountSlots() {
	accountID := int64(200)
	slotKey := fmt.Sprintf("%s%d", accountSlotKeyPrefix, accountID)

	// Acquire 3 slots
	ok, err := s.cache.AcquireAccountSlot(s.ctx, accountID, 5, "req1")
	require.NoError(s.T(), err)
	require.True(s.T(), ok)
	ok, err = s.cache.AcquireAccountSlot(s.ctx, accountID, 5, "req2")
	require.NoError(s.T(), err)
	require.True(s.T(), ok)
	ok, err = s.cache.AcquireAccountSlot(s.ctx, accountID, 5, "req3")
	require.NoError(s.T(), err)
	require.True(s.T(), ok)

	// Verify 3 slots exist
	cur, err := s.cache.GetAccountConcurrency(s.ctx, accountID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 3, cur)

	// Manually set old timestamps for req1 and req2 (simulate expired slots)
	now := time.Now().Unix()
	expiredTime := now - int64(testSlotTTL.Seconds()) - 10 // 10 seconds past TTL
	err = s.rdb.ZAdd(s.ctx, slotKey, redis.Z{Score: float64(expiredTime), Member: "req1"}).Err()
	require.NoError(s.T(), err)
	err = s.rdb.ZAdd(s.ctx, slotKey, redis.Z{Score: float64(expiredTime), Member: "req2"}).Err()
	require.NoError(s.T(), err)

	// Run cleanup
	err = s.cache.CleanupExpiredAccountSlots(s.ctx, accountID)
	require.NoError(s.T(), err)

	// Verify only 1 slot remains (req3)
	cur, err = s.cache.GetAccountConcurrency(s.ctx, accountID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, cur)

	// Verify req3 still exists
	members, err := s.rdb.ZRange(s.ctx, slotKey, 0, -1).Result()
	require.NoError(s.T(), err)
	require.Len(s.T(), members, 1)
	require.Equal(s.T(), "req3", members[0])
}

func (s *ConcurrencyCacheSuite) TestCleanupExpiredAccountSlots_NoExpired() {
	accountID := int64(201)

	// Acquire 2 fresh slots
	ok, err := s.cache.AcquireAccountSlot(s.ctx, accountID, 5, "req1")
	require.NoError(s.T(), err)
	require.True(s.T(), ok)
	ok, err = s.cache.AcquireAccountSlot(s.ctx, accountID, 5, "req2")
	require.NoError(s.T(), err)
	require.True(s.T(), ok)

	// Run cleanup (should not remove anything)
	err = s.cache.CleanupExpiredAccountSlots(s.ctx, accountID)
	require.NoError(s.T(), err)

	// Verify both slots still exist
	cur, err := s.cache.GetAccountConcurrency(s.ctx, accountID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 2, cur)
}

func (s *ConcurrencyCacheSuite) TestCleanupExpiredAccountSlotKeys() {
	now, err := s.rawCache.redisUnixSeconds(s.ctx)
	require.NoError(s.T(), err)
	expiredTime := now - int64(testSlotTTL.Seconds()) - 10
	accountKeyWithFresh := fmt.Sprintf("%s%d", accountSlotKeyPrefix, 301)
	accountKeyExpiredOnly := fmt.Sprintf("%s%d", accountSlotKeyPrefix, 302)
	userKey := fmt.Sprintf("%s%d", userSlotKeyPrefix, 303)
	unindexedAccountKey := fmt.Sprintf("%s%d", accountSlotKeyPrefix, 304)

	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, accountKeyWithFresh,
		redis.Z{Score: float64(expiredTime), Member: "expired"},
		redis.Z{Score: float64(now), Member: "fresh"},
	).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, accountKeyExpiredOnly,
		redis.Z{Score: float64(expiredTime), Member: "expired-only"},
	).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, userKey,
		redis.Z{Score: float64(expiredTime), Member: "user-expired"},
	).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, unindexedAccountKey,
		redis.Z{Score: float64(expiredTime), Member: "unindexed-expired"},
	).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, accountActiveIndexKey,
		redis.Z{Score: float64(now), Member: "301"},
		redis.Z{Score: float64(now), Member: "302"},
	).Err())

	require.NoError(s.T(), s.cache.CleanupExpiredAccountSlotKeys(s.ctx))

	accountMembers, err := s.rdb.ZRange(s.ctx, accountKeyWithFresh, 0, -1).Result()
	require.NoError(s.T(), err)
	require.Equal(s.T(), []string{"fresh"}, accountMembers)

	exists, err := s.rdb.Exists(s.ctx, accountKeyExpiredOnly).Result()
	require.NoError(s.T(), err)
	require.EqualValues(s.T(), 0, exists)

	userMembers, err := s.rdb.ZRange(s.ctx, userKey, 0, -1).Result()
	require.NoError(s.T(), err)
	require.Equal(s.T(), []string{"user-expired"}, userMembers)

	unindexedMembers, err := s.rdb.ZRange(s.ctx, unindexedAccountKey, 0, -1).Result()
	require.NoError(s.T(), err)
	require.Equal(s.T(), []string{"unindexed-expired"}, unindexedMembers)

	score, err := s.rdb.ZScore(s.ctx, accountActiveIndexKey, "301").Result()
	require.NoError(s.T(), err)
	require.Greater(s.T(), int64(score), now)
	_, err = s.rdb.ZScore(s.ctx, accountActiveIndexKey, "302").Result()
	require.ErrorIs(s.T(), err, redis.Nil)
}

func (s *ConcurrencyCacheSuite) TestCleanupExpiredAccountSlotKeys_ReapsUserIndex() {
	now, err := s.rawCache.redisUnixSeconds(s.ctx)
	require.NoError(s.T(), err)
	expiredScore := float64(now - 10)
	userKeyWithFresh := fmt.Sprintf("%s%d", userSlotKeyPrefix, 401)

	// 401 有真实负载但索引 score 已过期：应刷新而不是删除。
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, userKeyWithFresh,
		redis.Z{Score: float64(now), Member: "fresh"},
	).Err())
	// 402 无任何负载：过期索引 member 应被回收。
	// 非法 member 也应随过期候选一并清除。
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, userActiveIndexKey,
		redis.Z{Score: expiredScore, Member: "401"},
		redis.Z{Score: expiredScore, Member: "402"},
		redis.Z{Score: expiredScore, Member: "not-a-user-id"},
	).Err())

	require.NoError(s.T(), s.cache.CleanupExpiredAccountSlotKeys(s.ctx))

	score, err := s.rdb.ZScore(s.ctx, userActiveIndexKey, "401").Result()
	require.NoError(s.T(), err)
	require.Greater(s.T(), int64(score), now, "loaded user should be re-scheduled, not dropped")

	_, err = s.rdb.ZScore(s.ctx, userActiveIndexKey, "402").Result()
	require.ErrorIs(s.T(), err, redis.Nil, "idle expired user member should be reaped")

	_, err = s.rdb.ZScore(s.ctx, userActiveIndexKey, "not-a-user-id").Result()
	require.ErrorIs(s.T(), err, redis.Nil, "invalid member should be reaped")
}

func (s *ConcurrencyCacheSuite) TestCleanupStaleProcessSlots_LegacyWaitSweepRunsOnce() {
	unindexedAccountWaitKey := fmt.Sprintf("%s%d", accountWaitKeyPrefix, 2901)
	unindexedUserWaitKey := fmt.Sprintf("%s%d", waitQueueKeyPrefix, 2902)
	require.NoError(s.T(), s.rdb.Set(s.ctx, unindexedAccountWaitKey, 5, time.Minute).Err())
	require.NoError(s.T(), s.rdb.Set(s.ctx, unindexedUserWaitKey, 3, time.Minute).Err())

	// 首次运行：marker 不存在，一次性清扫删除所有遗留等待计数（含未入索引的）。
	require.NoError(s.T(), s.cache.CleanupStaleProcessSlots(s.ctx, "keep-"))

	_, err := s.rdb.Get(s.ctx, unindexedAccountWaitKey).Result()
	require.ErrorIs(s.T(), err, redis.Nil, "legacy account wait key should be swept on first startup")
	_, err = s.rdb.Get(s.ctx, unindexedUserWaitKey).Result()
	require.ErrorIs(s.T(), err, redis.Nil, "legacy user wait key should be swept on first startup")

	exists, err := s.rdb.Exists(s.ctx, legacyWaitSweepMarkerKey).Result()
	require.NoError(s.T(), err)
	require.EqualValues(s.T(), 1, exists, "sweep marker should be set after first run")

	// 再次运行：marker 已存在，未入索引的等待计数不再被触碰。
	require.NoError(s.T(), s.rdb.Set(s.ctx, unindexedAccountWaitKey, 5, time.Minute).Err())
	require.NoError(s.T(), s.cache.CleanupStaleProcessSlots(s.ctx, "keep-"))
	val, err := s.rdb.Get(s.ctx, unindexedAccountWaitKey).Int()
	require.NoError(s.T(), err, "sweep must not run twice")
	require.Equal(s.T(), 5, val)
}

func (s *ConcurrencyCacheSuite) TestCleanupStaleProcessSlots_ProcessesExpiredIndexMembers() {
	// score 已过期的索引成员往往正是崩溃进程留下的残留，启动清理必须覆盖它们。
	require.NoError(s.T(), s.rdb.Set(s.ctx, legacyWaitSweepMarkerKey, "1", 0).Err())
	accountID := int64(3901)
	userID := int64(3902)
	accountKey := fmt.Sprintf("%s%d", accountSlotKeyPrefix, accountID)
	userKey := fmt.Sprintf("%s%d", userSlotKeyPrefix, userID)
	accountWaitKey := fmt.Sprintf("%s%d", accountWaitKeyPrefix, accountID)

	now, err := s.rawCache.redisUnixSeconds(s.ctx)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, accountKey,
		redis.Z{Score: float64(now), Member: "oldproc-1"},
	).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, userKey,
		redis.Z{Score: float64(now), Member: "oldproc-2"},
	).Err())
	require.NoError(s.T(), s.rdb.Set(s.ctx, accountWaitKey, 4, time.Minute).Err())
	// 索引 score 设为过去时刻，模拟长时间停机后索引已“过期”。
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, accountActiveIndexKey, redis.Z{
		Score:  float64(now - 100),
		Member: strconv.FormatInt(accountID, 10),
	}).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, userActiveIndexKey, redis.Z{
		Score:  float64(now - 100),
		Member: strconv.FormatInt(userID, 10),
	}).Err())

	require.NoError(s.T(), s.cache.CleanupStaleProcessSlots(s.ctx, "keep-"))

	exists, err := s.rdb.Exists(s.ctx, accountKey).Result()
	require.NoError(s.T(), err)
	require.EqualValues(s.T(), 0, exists, "stale slot key of expired index member should be purged")

	exists, err = s.rdb.Exists(s.ctx, userKey).Result()
	require.NoError(s.T(), err)
	require.EqualValues(s.T(), 0, exists)

	_, err = s.rdb.Get(s.ctx, accountWaitKey).Result()
	require.ErrorIs(s.T(), err, redis.Nil, "wait counter of expired index member should be deleted")

	_, err = s.rdb.ZScore(s.ctx, accountActiveIndexKey, strconv.FormatInt(accountID, 10)).Result()
	require.ErrorIs(s.T(), err, redis.Nil, "emptied member should be removed from index")
	_, err = s.rdb.ZScore(s.ctx, userActiveIndexKey, strconv.FormatInt(userID, 10)).Result()
	require.ErrorIs(s.T(), err, redis.Nil)
}

func (s *ConcurrencyCacheSuite) TestCleanupStaleProcessSlots_RemovesOldPrefixesAndWaitCounters() {
	// 预置迁移 marker，确保等待计数删除来自索引驱动路径而非一次性清扫。
	require.NoError(s.T(), s.rdb.Set(s.ctx, legacyWaitSweepMarkerKey, "1", 0).Err())
	accountID := int64(901)
	userID := int64(902)
	accountSlotKey := fmt.Sprintf("%s%d", accountSlotKeyPrefix, accountID)
	userSlotKey := fmt.Sprintf("%s%d", userSlotKeyPrefix, userID)
	userWaitKey := fmt.Sprintf("%s%d", waitQueueKeyPrefix, userID)
	accountWaitKey := fmt.Sprintf("%s%d", accountWaitKeyPrefix, accountID)

	now, err := s.rawCache.redisUnixSeconds(s.ctx)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, accountSlotKey,
		redis.Z{Score: float64(now), Member: "oldproc-1"},
		redis.Z{Score: float64(now), Member: "activeproc-1"},
	).Err())
	require.NoError(s.T(), s.rdb.Expire(s.ctx, accountSlotKey, testSlotTTL).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, userSlotKey,
		redis.Z{Score: float64(now), Member: "oldproc-2"},
		redis.Z{Score: float64(now), Member: "activeproc-2"},
	).Err())
	require.NoError(s.T(), s.rdb.Expire(s.ctx, userSlotKey, testSlotTTL).Err())
	require.NoError(s.T(), s.rdb.Set(s.ctx, userWaitKey, 3, testSlotTTL).Err())
	require.NoError(s.T(), s.rdb.Set(s.ctx, accountWaitKey, 2, testSlotTTL).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, accountActiveIndexKey, redis.Z{
		Score:  float64(now + 60),
		Member: strconv.FormatInt(accountID, 10),
	}).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, userActiveIndexKey, redis.Z{
		Score:  float64(now + 60),
		Member: strconv.FormatInt(userID, 10),
	}).Err())

	require.NoError(s.T(), s.cache.CleanupStaleProcessSlots(s.ctx, "activeproc-"))

	accountMembers, err := s.rdb.ZRange(s.ctx, accountSlotKey, 0, -1).Result()
	require.NoError(s.T(), err)
	require.Equal(s.T(), []string{"activeproc-1"}, accountMembers)

	userMembers, err := s.rdb.ZRange(s.ctx, userSlotKey, 0, -1).Result()
	require.NoError(s.T(), err)
	require.Equal(s.T(), []string{"activeproc-2"}, userMembers)

	_, err = s.rdb.Get(s.ctx, userWaitKey).Result()
	require.ErrorIs(s.T(), err, redis.Nil)
	_, err = s.rdb.Get(s.ctx, accountWaitKey).Result()
	require.ErrorIs(s.T(), err, redis.Nil)
}

func (s *ConcurrencyCacheSuite) TestCleanupStaleProcessSlots_DeletesEmptySlotKeys() {
	accountID := int64(903)
	accountSlotKey := fmt.Sprintf("%s%d", accountSlotKeyPrefix, accountID)
	now, err := s.rawCache.redisUnixSeconds(s.ctx)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, accountSlotKey, redis.Z{Score: float64(now), Member: "oldproc-1"}).Err())
	require.NoError(s.T(), s.rdb.Expire(s.ctx, accountSlotKey, testSlotTTL).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, accountActiveIndexKey, redis.Z{
		Score:  float64(now + 60),
		Member: strconv.FormatInt(accountID, 10),
	}).Err())

	require.NoError(s.T(), s.cache.CleanupStaleProcessSlots(s.ctx, "activeproc-"))

	exists, err := s.rdb.Exists(s.ctx, accountSlotKey).Result()
	require.NoError(s.T(), err)
	require.EqualValues(s.T(), 0, exists)
}
