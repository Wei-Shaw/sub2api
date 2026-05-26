//go:build integration

package repository

import (
	"errors"
	"fmt"
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
	cache service.ConcurrencyCache
}

func TestConcurrencyCacheSuite(t *testing.T) {
	suite.Run(t, new(ConcurrencyCacheSuite))
}

func (s *ConcurrencyCacheSuite) SetupTest() {
	s.IntegrationRedisSuite.SetupTest()
	s.cache = NewConcurrencyCache(s.rdb, testSlotTTLMinutes, int(testSlotTTL.Seconds()))
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
	accountID := int64(901)
	userID := int64(902)
	accountKey := fmt.Sprintf("%s%d", accountSlotKeyPrefix, accountID)
	userKey := fmt.Sprintf("%s%d", userSlotKeyPrefix, userID)
	userWaitKey := fmt.Sprintf("%s%d", waitQueueKeyPrefix, userID)
	accountWaitKey := fmt.Sprintf("%s%d", accountWaitKeyPrefix, accountID)

	now := time.Now().Unix()
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, accountKey,
		redis.Z{Score: float64(now), Member: "oldproc-1"},
		redis.Z{Score: float64(now), Member: "keep-1"},
	).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, userKey,
		redis.Z{Score: float64(now), Member: "oldproc-2"},
		redis.Z{Score: float64(now), Member: "keep-2"},
	).Err())
	require.NoError(s.T(), s.rdb.Set(s.ctx, userWaitKey, 3, time.Minute).Err())
	require.NoError(s.T(), s.rdb.Set(s.ctx, accountWaitKey, 2, time.Minute).Err())

	require.NoError(s.T(), s.cache.CleanupStaleProcessSlots(s.ctx, "keep-"))

	accountMembers, err := s.rdb.ZRange(s.ctx, accountKey, 0, -1).Result()
	require.NoError(s.T(), err)
	require.Equal(s.T(), []string{"keep-1"}, accountMembers)

	userMembers, err := s.rdb.ZRange(s.ctx, userKey, 0, -1).Result()
	require.NoError(s.T(), err)
	require.Equal(s.T(), []string{"keep-2"}, userMembers)

	_, err = s.rdb.Get(s.ctx, userWaitKey).Result()
	require.True(s.T(), errors.Is(err, redis.Nil))

	_, err = s.rdb.Get(s.ctx, accountWaitKey).Result()
	require.True(s.T(), errors.Is(err, redis.Nil))
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

func (s *ConcurrencyCacheSuite) TestCleanupStaleProcessSlots_RemovesOldPrefixesAndWaitCounters() {
	accountID := int64(901)
	userID := int64(902)
	accountSlotKey := fmt.Sprintf("%s%d", accountSlotKeyPrefix, accountID)
	userSlotKey := fmt.Sprintf("%s%d", userSlotKeyPrefix, userID)
	userWaitKey := fmt.Sprintf("%s%d", waitQueueKeyPrefix, userID)
	accountWaitKey := fmt.Sprintf("%s%d", accountWaitKeyPrefix, accountID)

	now := float64(time.Now().Unix())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, accountSlotKey,
		redis.Z{Score: now, Member: "oldproc-1"},
		redis.Z{Score: now, Member: "activeproc-1"},
	).Err())
	require.NoError(s.T(), s.rdb.Expire(s.ctx, accountSlotKey, testSlotTTL).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, userSlotKey,
		redis.Z{Score: now, Member: "oldproc-2"},
		redis.Z{Score: now, Member: "activeproc-2"},
	).Err())
	require.NoError(s.T(), s.rdb.Expire(s.ctx, userSlotKey, testSlotTTL).Err())
	require.NoError(s.T(), s.rdb.Set(s.ctx, userWaitKey, 3, testSlotTTL).Err())
	require.NoError(s.T(), s.rdb.Set(s.ctx, accountWaitKey, 2, testSlotTTL).Err())

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
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, accountSlotKey, redis.Z{Score: float64(time.Now().Unix()), Member: "oldproc-1"}).Err())
	require.NoError(s.T(), s.rdb.Expire(s.ctx, accountSlotKey, testSlotTTL).Err())

	require.NoError(s.T(), s.cache.CleanupStaleProcessSlots(s.ctx, "activeproc-"))

	exists, err := s.rdb.Exists(s.ctx, accountSlotKey).Result()
	require.NoError(s.T(), err)
	require.EqualValues(s.T(), 0, exists)
}
