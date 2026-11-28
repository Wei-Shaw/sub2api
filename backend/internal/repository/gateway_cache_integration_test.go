//go:build integration

package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type GatewayCacheSuite struct {
	IntegrationRedisSuite
	cache service.GatewayCache
}

func (s *GatewayCacheSuite) SetupTest() {
	s.IntegrationRedisSuite.SetupTest()
	s.cache = NewGatewayCache(s.rdb)
}

func (s *GatewayCacheSuite) TestGetSessionAccountID_Missing() {
	_, err := s.cache.GetSessionAccountID(s.ctx, 1, "nonexistent")
	require.True(s.T(), errors.Is(err, service.ErrStickySessionNotFound), "expected ErrStickySessionNotFound for missing session")
}

func (s *GatewayCacheSuite) TestSetAndGetSessionAccountID() {
	sessionID := "s1"
	accountID := int64(99)
	groupID := int64(1)
	sessionTTL := 1 * time.Minute

	require.NoError(s.T(), s.cache.SetSessionAccountID(s.ctx, groupID, sessionID, accountID, sessionTTL), "SetSessionAccountID")

	sid, err := s.cache.GetSessionAccountID(s.ctx, groupID, sessionID)
	require.NoError(s.T(), err, "GetSessionAccountID")
	require.Equal(s.T(), accountID, sid, "session id mismatch")
}

func (s *GatewayCacheSuite) TestSessionAccountID_TTL() {
	sessionID := "s2"
	accountID := int64(100)
	groupID := int64(1)
	sessionTTL := 1 * time.Minute

	require.NoError(s.T(), s.cache.SetSessionAccountID(s.ctx, groupID, sessionID, accountID, sessionTTL), "SetSessionAccountID")

	sessionKey := buildSessionKey(groupID, sessionID)
	ttl, err := s.rdb.TTL(s.ctx, sessionKey).Result()
	require.NoError(s.T(), err, "TTL sessionKey after Set")
	s.AssertTTLWithin(ttl, 1*time.Second, sessionTTL)
}

func (s *GatewayCacheSuite) TestRefreshSessionTTL() {
	sessionID := "s3"
	accountID := int64(101)
	groupID := int64(1)
	initialTTL := 1 * time.Minute
	refreshTTL := 3 * time.Minute

	require.NoError(s.T(), s.cache.SetSessionAccountID(s.ctx, groupID, sessionID, accountID, initialTTL), "SetSessionAccountID")

	require.NoError(s.T(), s.cache.RefreshSessionTTL(s.ctx, groupID, sessionID, refreshTTL), "RefreshSessionTTL")

	sessionKey := buildSessionKey(groupID, sessionID)
	ttl, err := s.rdb.TTL(s.ctx, sessionKey).Result()
	require.NoError(s.T(), err, "TTL after Refresh")
	s.AssertTTLWithin(ttl, 1*time.Second, refreshTTL)
}

func (s *GatewayCacheSuite) TestRefreshSessionTTL_MissingKey() {
	// RefreshSessionTTL on a missing key should not error (no-op)
	err := s.cache.RefreshSessionTTL(s.ctx, 1, "missing-session", 1*time.Minute)
	require.NoError(s.T(), err, "RefreshSessionTTL on missing key should not error")
}

func (s *GatewayCacheSuite) TestDeleteSessionAccountID() {
	sessionID := "openai:s4"
	accountID := int64(102)
	groupID := int64(1)
	sessionTTL := 1 * time.Minute

	require.NoError(s.T(), s.cache.SetSessionAccountID(s.ctx, groupID, sessionID, accountID, sessionTTL), "SetSessionAccountID")
	require.NoError(s.T(), s.cache.DeleteSessionAccountID(s.ctx, groupID, sessionID), "DeleteSessionAccountID")

	_, err := s.cache.GetSessionAccountID(s.ctx, groupID, sessionID)
	require.True(s.T(), errors.Is(err, service.ErrStickySessionNotFound), "expected ErrStickySessionNotFound after delete")
}

func (s *GatewayCacheSuite) TestCompareAndBindSessionAccountID_BindsWhenHoldingReplaceable() {
	sessionID := "cas-replaceable"
	groupID := int64(1)
	replaceable := int64(9499)
	accountID := int64(9500)

	require.NoError(s.T(), s.cache.SetSessionAccountID(s.ctx, groupID, sessionID, replaceable, time.Minute), "SetSessionAccountID")
	bound, err := s.cache.(service.GatewayStickySessionCompareCache).CompareAndBindSessionAccountID(s.ctx, groupID, sessionID, accountID, []int64{9400, replaceable}, time.Minute)

	require.NoError(s.T(), err)
	require.True(s.T(), bound, "a key holding a replaceable account must be rewritten")
	sid, err := s.cache.GetSessionAccountID(s.ctx, groupID, sessionID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), accountID, sid)
}

func (s *GatewayCacheSuite) TestCompareAndBindSessionAccountID_RefreshesBoundAccount() {
	sessionID := "cas-refresh"
	groupID := int64(1)
	accountID := int64(9501)

	require.NoError(s.T(), s.cache.SetSessionAccountID(s.ctx, groupID, sessionID, accountID, time.Minute), "SetSessionAccountID")
	bound, err := s.cache.(service.GatewayStickySessionCompareCache).CompareAndBindSessionAccountID(s.ctx, groupID, sessionID, accountID, nil, 5*time.Minute)

	require.NoError(s.T(), err)
	require.True(s.T(), bound)
	sid, err := s.cache.GetSessionAccountID(s.ctx, groupID, sessionID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), accountID, sid)
	ttl, err := s.rdb.TTL(s.ctx, buildSessionKey(groupID, sessionID)).Result()
	require.NoError(s.T(), err)
	s.AssertTTLWithin(ttl, 1*time.Second, 5*time.Minute)
}

func (s *GatewayCacheSuite) TestCompareAndBindSessionAccountID_LeavesThirdAccount() {
	sessionID := "cas-third"
	groupID := int64(1)
	third := int64(9499)

	require.NoError(s.T(), s.cache.SetSessionAccountID(s.ctx, groupID, sessionID, third, time.Minute), "SetSessionAccountID")
	bound, err := s.cache.(service.GatewayStickySessionCompareCache).CompareAndBindSessionAccountID(s.ctx, groupID, sessionID, 9401, []int64{9400, 9402}, time.Minute)

	require.NoError(s.T(), err)
	require.False(s.T(), bound, "a key holding a third account must be left alone")
	sid, err := s.cache.GetSessionAccountID(s.ctx, groupID, sessionID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), third, sid)
}

func (s *GatewayCacheSuite) TestCompareAndBindSessionAccountID_LeavesThirdAccountWithoutReplaceables() {
	sessionID := "cas-third-nil"
	groupID := int64(1)
	third := int64(9498)

	require.NoError(s.T(), s.cache.SetSessionAccountID(s.ctx, groupID, sessionID, third, time.Minute), "SetSessionAccountID")
	bound, err := s.cache.(service.GatewayStickySessionCompareCache).CompareAndBindSessionAccountID(s.ctx, groupID, sessionID, 9401, nil, time.Minute)

	require.NoError(s.T(), err)
	require.False(s.T(), bound, "a key holding any other account must be left alone")
	sid, err := s.cache.GetSessionAccountID(s.ctx, groupID, sessionID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), third, sid)
}

func (s *GatewayCacheSuite) TestCompareAndBindSessionAccountID_BindsMissingKey() {
	sessionID := "cas-missing"
	groupID := int64(1)
	accountID := int64(9402)

	bound, err := s.cache.(service.GatewayStickySessionCompareCache).CompareAndBindSessionAccountID(s.ctx, groupID, sessionID, accountID, []int64{9401}, time.Minute)

	require.NoError(s.T(), err)
	require.True(s.T(), bound, "a missing key must be written even when replaceables were passed")
	sid, err := s.cache.GetSessionAccountID(s.ctx, groupID, sessionID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), accountID, sid)
}

func (s *GatewayCacheSuite) TestGetSessionAccountID_CorruptedValue() {
	sessionID := "corrupted"
	groupID := int64(1)
	sessionKey := buildSessionKey(groupID, sessionID)

	// Set a non-integer value
	require.NoError(s.T(), s.rdb.Set(s.ctx, sessionKey, "not-a-number", 1*time.Minute).Err(), "Set invalid value")

	_, err := s.cache.GetSessionAccountID(s.ctx, groupID, sessionID)
	require.Error(s.T(), err, "expected error for corrupted value")
	require.False(s.T(), errors.Is(err, service.ErrStickySessionNotFound), "expected parsing error, not a miss")
}

func TestGatewayCacheSuite(t *testing.T) {
	suite.Run(t, new(GatewayCacheSuite))
}
