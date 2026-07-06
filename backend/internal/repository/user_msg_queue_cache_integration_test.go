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
REDACTED

func TestUserMsgQueueCacheSuite(t *testing.T) {
	suite.Run(t, new(UserMsgQueueCacheSuite))
REDACTED

func (s *UserMsgQueueCacheSuite) SetupTest() {
	s.IntegrationRedisSuite.SetupTest()
	s.cache = NewUserMsgQueueCache(s.rdb).(*userMsgQueueCache)
REDACTED

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
REDACTED

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
REDACTED, time.Second, 10*time.Millisecond)

	cleaned, err := s.cache.ReconcileExpiredLockCandidates(s.ctx, 1000)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 0, cleaned)

	_, err = s.rdb.ZScore(s.ctx, umqLockIndexKey, "702").Result()
	require.ErrorIs(s.T(), err, redis.Nil)
REDACTED

func (s *UserMsgQueueCacheSuite) TestReconcileExpiredLockCandidatesRefreshesLiveLockIndex() {
	accountID := int64(703)
	nowMs, err := s.cache.GetCurrentTimeMs(s.ctx)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.rdb.Set(s.ctx, umqLockKey(accountID), "req-703", time.Minute).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, umqLockIndexKey, redis.Z{
		Score:  float64(nowMs - 1),
		Member: "703",
REDACTED).Err())

	cleaned, err := s.cache.ReconcileExpiredLockCandidates(s.ctx, 1000)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 0, cleaned)

	score, err := s.rdb.ZScore(s.ctx, umqLockIndexKey, "703").Result()
	require.NoError(s.T(), err)
	require.Greater(s.T(), int64(score), nowMs)
	exists, err := s.rdb.Exists(s.ctx, umqLockKey(accountID)).Result()
	require.NoError(s.T(), err)
	require.EqualValues(s.T(), 1, exists)
REDACTED

func (s *UserMsgQueueCacheSuite) TestReconcileExpiredLockCandidatesDeletesNoTTLLock() {
	accountID := int64(704)
	nowMs, err := s.cache.GetCurrentTimeMs(s.ctx)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.rdb.Set(s.ctx, umqLockKey(accountID), "req-704", 0).Err())
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, umqLockIndexKey, redis.Z{
		Score:  float64(nowMs),
		Member: "704",
REDACTED).Err())

	cleaned, err := s.cache.ReconcileExpiredLockCandidates(s.ctx, 1000)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, cleaned)

	exists, err := s.rdb.Exists(s.ctx, umqLockKey(accountID)).Result()
	require.NoError(s.T(), err)
	require.EqualValues(s.T(), 0, exists)
	_, err = s.rdb.ZScore(s.ctx, umqLockIndexKey, "704").Result()
	require.ErrorIs(s.T(), err, redis.Nil)
REDACTED

func (s *UserMsgQueueCacheSuite) TestReconcileExpiredLockCandidatesRemovesInvalidMember() {
	nowMs, err := s.cache.GetCurrentTimeMs(s.ctx)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.rdb.ZAdd(s.ctx, umqLockIndexKey, redis.Z{
		Score:  float64(nowMs),
		Member: "not-an-account-id",
REDACTED).Err())

	cleaned, err := s.cache.ReconcileExpiredLockCandidates(s.ctx, 1000)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 0, cleaned)

	_, err = s.rdb.ZScore(s.ctx, umqLockIndexKey, "not-an-account-id").Result()
	require.True(s.T(), errors.Is(err, redis.Nil))
REDACTED
