//go:build integration

package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type GatewayCacheSuite struct {
	IntegrationRedisSuite
	cache service.GatewayCache
REDACTED

func (s *GatewayCacheSuite) SetupTest() {
	s.IntegrationRedisSuite.SetupTest()
	s.cache = NewGatewayCache(s.rdb)
REDACTED

func (s *GatewayCacheSuite) TestGetSessionAccountID_Missing() {
	_, err := s.cache.GetSessionAccountID(s.ctx, 1, "nonexistent")
	require.True(s.T(), errors.Is(err, redis.Nil), "expected redis.Nil for missing session")
REDACTED

func (s *GatewayCacheSuite) TestSetAndGetSessionAccountID() {
	sessionID := "s1"
	accountID := int64(99)
	groupID := int64(1)
	sessionTTL := 1 * time.Minute

	require.NoError(s.T(), s.cache.SetSessionAccountID(s.ctx, groupID, sessionID, accountID, sessionTTL), "SetSessionAccountID")

	sid, err := s.cache.GetSessionAccountID(s.ctx, groupID, sessionID)
	require.NoError(s.T(), err, "GetSessionAccountID")
	require.Equal(s.T(), accountID, sid, "session id mismatch")
REDACTED

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
REDACTED

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
REDACTED

func (s *GatewayCacheSuite) TestRefreshSessionTTL_MissingKey() {
	// RefreshSessionTTL on a missing key should not error (no-op)
	err := s.cache.RefreshSessionTTL(s.ctx, 1, "missing-session", 1*time.Minute)
	require.NoError(s.T(), err, "RefreshSessionTTL on missing key should not error")
REDACTED

func (s *GatewayCacheSuite) TestDeleteSessionAccountID() {
	sessionID := "openai:s4"
	accountID := int64(102)
	groupID := int64(1)
	sessionTTL := 1 * time.Minute

	require.NoError(s.T(), s.cache.SetSessionAccountID(s.ctx, groupID, sessionID, accountID, sessionTTL), "SetSessionAccountID")
	require.NoError(s.T(), s.cache.DeleteSessionAccountID(s.ctx, groupID, sessionID), "DeleteSessionAccountID")

	_, err := s.cache.GetSessionAccountID(s.ctx, groupID, sessionID)
	require.True(s.T(), errors.Is(err, redis.Nil), "expected redis.Nil after delete")
REDACTED

func (s *GatewayCacheSuite) TestGetSessionAccountID_CorruptedValue() {
	sessionID := "corrupted"
	groupID := int64(1)
	sessionKey := buildSessionKey(groupID, sessionID)

	// Set a non-integer value
	require.NoError(s.T(), s.rdb.Set(s.ctx, sessionKey, "not-a-number", 1*time.Minute).Err(), "Set invalid value")

	_, err := s.cache.GetSessionAccountID(s.ctx, groupID, sessionID)
	require.Error(s.T(), err, "expected error for corrupted value")
	require.False(s.T(), errors.Is(err, redis.Nil), "expected parsing error, not redis.Nil")
REDACTED

// ============ Gemini Trie 会话测试 ============

func (s *GatewayCacheSuite) TestGeminiSessionTrie_SaveAndFind() {
	groupID := int64(1)
	prefixHash := "testprefix"
	digestChain := "u:hash1-m:hash2-u:hash3"
	uuid := "test-uuid-123"
	accountID := int64(42)

	// 保存会话
	err := s.cache.SaveGeminiSession(s.ctx, groupID, prefixHash, digestChain, uuid, accountID)
	require.NoError(s.T(), err, "SaveGeminiSession")

	// 精确匹配查找
	foundUUID, foundAccountID, found := s.cache.FindGeminiSession(s.ctx, groupID, prefixHash, digestChain)
	require.True(s.T(), found, "should find exact match")
	require.Equal(s.T(), uuid, foundUUID)
	require.Equal(s.T(), accountID, foundAccountID)
REDACTED

func (s *GatewayCacheSuite) TestGeminiSessionTrie_PrefixMatch() {
	groupID := int64(1)
	prefixHash := "prefixmatch"
	shortChain := "u:a-m:b"
	longChain := "u:a-m:b-u:c-m:d"
	uuid := "uuid-prefix"
	accountID := int64(100)

	// 保存短链
	err := s.cache.SaveGeminiSession(s.ctx, groupID, prefixHash, shortChain, uuid, accountID)
	require.NoError(s.T(), err)

	// 用长链查找，应该匹配到短链（前缀匹配）
	foundUUID, foundAccountID, found := s.cache.FindGeminiSession(s.ctx, groupID, prefixHash, longChain)
	require.True(s.T(), found, "should find prefix match")
	require.Equal(s.T(), uuid, foundUUID)
	require.Equal(s.T(), accountID, foundAccountID)
REDACTED

func (s *GatewayCacheSuite) TestGeminiSessionTrie_LongestPrefixMatch() {
	groupID := int64(1)
	prefixHash := "longestmatch"

	// 保存多个不同长度的链
	err := s.cache.SaveGeminiSession(s.ctx, groupID, prefixHash, "u:a", "uuid-short", 1)
	require.NoError(s.T(), err)
	err = s.cache.SaveGeminiSession(s.ctx, groupID, prefixHash, "u:a-m:b", "uuid-medium", 2)
	require.NoError(s.T(), err)
	err = s.cache.SaveGeminiSession(s.ctx, groupID, prefixHash, "u:a-m:b-u:c", "uuid-long", 3)
	require.NoError(s.T(), err)

	// 查找更长的链，应该匹配到最长的前缀
	foundUUID, foundAccountID, found := s.cache.FindGeminiSession(s.ctx, groupID, prefixHash, "u:a-m:b-u:c-m:d-u:e")
	require.True(s.T(), found, "should find longest prefix match")
	require.Equal(s.T(), "uuid-long", foundUUID)
	require.Equal(s.T(), int64(3), foundAccountID)

	// 查找中等长度的链
	foundUUID, foundAccountID, found = s.cache.FindGeminiSession(s.ctx, groupID, prefixHash, "u:a-m:b-u:x")
	require.True(s.T(), found)
	require.Equal(s.T(), "uuid-medium", foundUUID)
	require.Equal(s.T(), int64(2), foundAccountID)
REDACTED

func (s *GatewayCacheSuite) TestGeminiSessionTrie_NoMatch() {
	groupID := int64(1)
	prefixHash := "nomatch"
	digestChain := "u:a-m:b"

	// 保存一个会话
	err := s.cache.SaveGeminiSession(s.ctx, groupID, prefixHash, digestChain, "uuid", 1)
	require.NoError(s.T(), err)

	// 用不同的链查找，应该找不到
	_, _, found := s.cache.FindGeminiSession(s.ctx, groupID, prefixHash, "u:x-m:y")
	require.False(s.T(), found, "should not find non-matching chain")
REDACTED

func (s *GatewayCacheSuite) TestGeminiSessionTrie_DifferentPrefixHash() {
	groupID := int64(1)
	digestChain := "u:a-m:b"

	// 保存到 prefixHash1
	err := s.cache.SaveGeminiSession(s.ctx, groupID, "prefix1", digestChain, "uuid1", 1)
	require.NoError(s.T(), err)

	// 用 prefixHash2 查找，应该找不到（不同用户/客户端隔离）
	_, _, found := s.cache.FindGeminiSession(s.ctx, groupID, "prefix2", digestChain)
	require.False(s.T(), found, "different prefixHash should be isolated")
REDACTED

func (s *GatewayCacheSuite) TestGeminiSessionTrie_DifferentGroupID() {
	prefixHash := "sameprefix"
	digestChain := "u:a-m:b"

	// 保存到 groupID 1
	err := s.cache.SaveGeminiSession(s.ctx, 1, prefixHash, digestChain, "uuid1", 1)
	require.NoError(s.T(), err)

	// 用 groupID 2 查找，应该找不到（分组隔离）
	_, _, found := s.cache.FindGeminiSession(s.ctx, 2, prefixHash, digestChain)
	require.False(s.T(), found, "different groupID should be isolated")
REDACTED

func (s *GatewayCacheSuite) TestGeminiSessionTrie_EmptyDigestChain() {
	groupID := int64(1)
	prefixHash := "emptytest"

	// 空链不应该保存
	err := s.cache.SaveGeminiSession(s.ctx, groupID, prefixHash, "", "uuid", 1)
	require.NoError(s.T(), err, "empty chain should not error")

	// 空链查找应该返回 false
	_, _, found := s.cache.FindGeminiSession(s.ctx, groupID, prefixHash, "")
	require.False(s.T(), found, "empty chain should not match")
REDACTED

func (s *GatewayCacheSuite) TestGeminiSessionTrie_MultipleSessions() {
	groupID := int64(1)
	prefixHash := "multisession"

	// 保存多个不同会话（模拟 1000 个并发会话的场景）
	sessions := []struct {
		chain     string
		uuid      string
		accountID int64
REDACTED{
		{"u:session1", "uuid-1", 1REDACTED,
		{"u:session2-m:reply2", "uuid-2", 2REDACTED,
		{"u:session3-m:reply3-u:msg3", "uuid-3", 3REDACTED,
REDACTED

	for _, sess := range sessions {
		err := s.cache.SaveGeminiSession(s.ctx, groupID, prefixHash, sess.chain, sess.uuid, sess.accountID)
		require.NoError(s.T(), err)
REDACTED

	// 验证每个会话都能正确查找
	for _, sess := range sessions {
		foundUUID, foundAccountID, found := s.cache.FindGeminiSession(s.ctx, groupID, prefixHash, sess.chain)
		require.True(s.T(), found, "should find session: %s", sess.chain)
		require.Equal(s.T(), sess.uuid, foundUUID)
		require.Equal(s.T(), sess.accountID, foundAccountID)
REDACTED

	// 验证继续对话的场景
	foundUUID, foundAccountID, found := s.cache.FindGeminiSession(s.ctx, groupID, prefixHash, "u:session2-m:reply2-u:newmsg")
	require.True(s.T(), found)
	require.Equal(s.T(), "uuid-2", foundUUID)
	require.Equal(s.T(), int64(2), foundAccountID)
REDACTED

func TestGatewayCacheSuite(t *testing.T) {
	suite.Run(t, new(GatewayCacheSuite))
REDACTED
