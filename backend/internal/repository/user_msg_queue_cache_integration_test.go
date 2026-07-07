//go:build integration

package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type UserMsgQueueCacheSuite struct {
	IntegrationRedisSuite
	cache *userMsgQueueCache
}

func TestUserMsgQueueCacheSuite(t *testing.T) {
	suite.Run(t, new(UserMsgQueueCacheSuite))
}

func (s *UserMsgQueueCacheSuite) SetupTest() {
	s.IntegrationRedisSuite.SetupTest()
	s.cache = NewUserMsgQueueCache(s.rdb).(*userMsgQueueCache)
}

func (s *UserMsgQueueCacheSuite) TestAcquireLockWritesIndexAndReleaseRemovesIt() {
	accountID := int64(701)
	nowMs, err := s.cache.GetCurrentTimeMs(s.ctx)
	require.NoError(s.T(), err)

	acquired, err := s.cache.AcquireLock(s.ctx, accountID, "req-701", 10_000)
	require.NoError(s.T(), err)
	require.True(s.T(), acquired)

	score, err := s.rdb.ZScore(s.ctx, umqLockIndexKey, "701").Result()
	require.NoError(s.T(), err)
	require.Greater(s.T(), int64(score), nowMs)

	released, err := s.cache.ReleaseLock(s.ctx, accountID, "req-701")
	require.NoError(s.T(), err)
	require.True(s.T(), released)

	_, err = s.rdb.ZScore(s.ctx, umqLockIndexKey, "701").Result()
	require.ErrorIs(s.T(), err, redis.Nil)
}

func (s *UserMsgQueueCacheSuite) TestReconcileExpiredLockCandidatesRemovesNaturallyExpiredLockIndex() {
	accountID := int64(702)
	acquired, err := s.cache.AcquireLock(s.ctx, accountID, "req-702", 20)
	require.NoError(s.T(), err)
	require.True(s.T(), acquired)

	score, err := s.rdb.ZScore(s.ctx, umqLockIndexKey, "702").Result()
	require.NoError(s.T(), err)
	require.Eventually(s.T(), func() bool {
		nowMs, err := s.cache.GetCurrentTimeMs(s.ctx)
		return err == nil && nowMs >= int64(score)
	}, time.Second, 10*time.Millisecond)

	cleaned, err := s.cache.ReconcileExpiredLockCandidates(s.ctx, 1000)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 0, cleaned)

	_, err = s.rdb.ZScore(s.ctx, umqLockIndexKey, "702").Result()
	require.ErrorIs(s.T(), err, redis.Nil)
}

func (s *UserMsgQueueCacheSuite) TestReconcileExpiredLockCandidatesRefreshesLiveLockIndex() {
	accountID := int64(703)
	nowMs, err := s.cache.GetCurrentTimeMs(s.ctx)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.rdb.Set(s.ctx, umqLockKey(accountID), "req-703", time.Minute).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, umqLockIndexKey, redis.Z{
		Score:  float64(nowMs - 1),
		Member: "703",
	}).Err())

	cleaned, err := s.cache.ReconcileExpiredLockCandidates(s.ctx, 1000)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 0, cleaned)

	score, err := s.rdb.ZScore(s.ctx, umqLockIndexKey, "703").Result()
	require.NoError(s.T(), err)
	require.Greater(s.T(), int64(score), nowMs)
	exists, err := s.rdb.Exists(s.ctx, umqLockKey(accountID)).Result()
	require.NoError(s.T(), err)
	require.EqualValues(s.T(), 1, exists)
}

func (s *UserMsgQueueCacheSuite) TestReconcileExpiredLockCandidatesDeletesNoTTLLock() {
	accountID := int64(704)
	nowMs, err := s.cache.GetCurrentTimeMs(s.ctx)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.rdb.Set(s.ctx, umqLockKey(accountID), "req-704", 0).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, umqLockIndexKey, redis.Z{
		Score:  float64(nowMs),
		Member: "704",
	}).Err())

	cleaned, err := s.cache.ReconcileExpiredLockCandidates(s.ctx, 1000)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, cleaned)

	exists, err := s.rdb.Exists(s.ctx, umqLockKey(accountID)).Result()
	require.NoError(s.T(), err)
	require.EqualValues(s.T(), 0, exists)
	_, err = s.rdb.ZScore(s.ctx, umqLockIndexKey, "704").Result()
	require.ErrorIs(s.T(), err, redis.Nil)
}

func (s *UserMsgQueueCacheSuite) TestReconcileExpiredLockCandidatesRemovesInvalidMember() {
	nowMs, err := s.cache.GetCurrentTimeMs(s.ctx)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, umqLockIndexKey, redis.Z{
		Score:  float64(nowMs),
		Member: "not-an-account-id",
	}).Err())

	cleaned, err := s.cache.ReconcileExpiredLockCandidates(s.ctx, 1000)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 0, cleaned)

	_, err = s.rdb.ZScore(s.ctx, umqLockIndexKey, "not-an-account-id").Result()
	require.True(s.T(), errors.Is(err, redis.Nil))
}

func (s *UserMsgQueueCacheSuite) TestAcquireLockBusyPathReindexesUnindexedLiveLock() {
	// 模拟索引丢失的存量锁（升级窗口/索引写失败/释放竞态误删）：
	// 锁存在且有 TTL，但索引里没有对应 member。
	accountID := int64(705)
	nowMs, err := s.cache.GetCurrentTimeMs(s.ctx)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.rdb.Set(s.ctx, umqLockKey(accountID), "holder-705", time.Minute).Err())

	// 另一个请求争锁失败，应顺手把观测到的持有者锁回填进索引。
	acquired, err := s.cache.AcquireLock(s.ctx, accountID, "contender-705", 10_000)
	require.NoError(s.T(), err)
	require.False(s.T(), acquired)

	score, err := s.rdb.ZScore(s.ctx, umqLockIndexKey, "705").Result()
	require.NoError(s.T(), err, "busy acquire should re-index the observed live lock")
	require.Greater(s.T(), int64(score), nowMs)
	// 锁本身不应被争锁方改动。
	val, err := s.rdb.Get(s.ctx, umqLockKey(accountID)).Result()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "holder-705", val)
}

func (s *UserMsgQueueCacheSuite) TestAcquireLockBusyPathMakesNoTTLLockReconcilable() {
	// PTTL == -1 的异常锁若不在索引中，永远不会被 reconcile 发现；
	// 争锁失败路径必须以“已到期候选”的 score 回填它，形成自愈闭环。
	accountID := int64(706)
	require.NoError(s.T(), s.rdb.Set(s.ctx, umqLockKey(accountID), "holder-706", 0).Err())

	acquired, err := s.cache.AcquireLock(s.ctx, accountID, "contender-706", 10_000)
	require.NoError(s.T(), err)
	require.False(s.T(), acquired)

	nowMs, err := s.cache.GetCurrentTimeMs(s.ctx)
	require.NoError(s.T(), err)
	score, err := s.rdb.ZScore(s.ctx, umqLockIndexKey, "706").Result()
	require.NoError(s.T(), err, "busy acquire should index the anomalous lock")
	require.LessOrEqual(s.T(), int64(score), nowMs, "anomalous lock should be an immediately-expired candidate")

	cleaned, err := s.cache.ReconcileExpiredLockCandidates(s.ctx, 1000)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, cleaned, "reconcile should delete the no-TTL lock")

	exists, err := s.rdb.Exists(s.ctx, umqLockKey(accountID)).Result()
	require.NoError(s.T(), err)
	require.EqualValues(s.T(), 0, exists, "queue is unblocked after reconcile")
	_, err = s.rdb.ZScore(s.ctx, umqLockIndexKey, "706").Result()
	require.ErrorIs(s.T(), err, redis.Nil)
}
