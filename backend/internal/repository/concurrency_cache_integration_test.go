//go:build integration

package repository

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/ports"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ConcurrencyCacheSuite struct {
	IntegrationRedisSuite
	cache ports.ConcurrencyCache
}

func (s *ConcurrencyCacheSuite) SetupTest() {
	s.IntegrationRedisSuite.SetupTest()
	s.cache = NewConcurrencyCache(s.rdb)
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
	slotKey := fmt.Sprintf("%s%d:%s", accountSlotKeyPrefix, accountID, reqID)

	ok, err := s.cache.AcquireAccountSlot(s.ctx, accountID, 5, reqID)
	require.NoError(s.T(), err, "AcquireAccountSlot")
	require.True(s.T(), ok)

	ttl, err := s.rdb.TTL(s.ctx, slotKey).Result()
	require.NoError(s.T(), err, "TTL")
	s.AssertTTLWithin(ttl, 1*time.Second, slotTTL)
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
	slotKey := fmt.Sprintf("%s%d:%s", userSlotKeyPrefix, userID, reqID)

	ok, err := s.cache.AcquireUserSlot(s.ctx, userID, 5, reqID)
	require.NoError(s.T(), err, "AcquireUserSlot")
	require.True(s.T(), ok)

	ttl, err := s.rdb.TTL(s.ctx, slotKey).Result()
	require.NoError(s.T(), err, "TTL")
	s.AssertTTLWithin(ttl, 1*time.Second, slotTTL)
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
	s.AssertTTLWithin(ttl, 1*time.Second, slotTTL)

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

func TestConcurrencyCacheSuite(t *testing.T) {
	suite.Run(t, new(ConcurrencyCacheSuite))
}
