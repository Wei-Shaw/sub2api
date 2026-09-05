//go:build integration

package repository

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

type SessionLimitCacheSuite struct {
	IntegrationRedisSuite
	cache service.SessionLimitCache
}

func (s *SessionLimitCacheSuite) SetupTest() {
	s.IntegrationRedisSuite.SetupTest()
	s.cache = NewSessionLimitCache(s.rdb, 5)
}

// TestUnregisterSession_FreesSlotForNewSession 复现并验证修复场景：
// max_sessions=1 的账号上，会话 A 注册后若请求转发失败，会话槽会被 A 占住
// 整个空闲超时窗口（默认 5 分钟），导致窗口内新会话 B 全部被拒。
// UnregisterSession 必须立即释放槽位，让 B 无需等待空闲超时即可注册。
func (s *SessionLimitCacheSuite) TestUnregisterSession_FreesSlotForNewSession() {
	const accountID = int64(101)
	maxSessions := 1
	idleTimeout := 5 * time.Minute

	// 会话 A 注册成功（请求开始转发）
	allowed, err := s.cache.RegisterSession(s.ctx, accountID, "session-A", maxSessions, idleTimeout)
	s.RequireNoError(err, "RegisterSession A")
	s.True(allowed, "首个会话应被允许")

	// 转发失败后的新会话 B 在空闲超时窗口内被拒（修复前的行为）
	allowed, err = s.cache.RegisterSession(s.ctx, accountID, "session-B", maxSessions, idleTimeout)
	s.RequireNoError(err, "RegisterSession B before release")
	s.False(allowed, "槽位被 A 占用时新会话应被拒绝")

	// 请求最终失败：立即释放 A 的会话注册（不等待空闲超时）
	s.RequireNoError(s.cache.UnregisterSession(s.ctx, accountID, "session-A"), "UnregisterSession A")

	// A 释放后 B 立即可注册，无需等待空闲超时
	allowed, err = s.cache.RegisterSession(s.ctx, accountID, "session-B", maxSessions, idleTimeout)
	s.RequireNoError(err, "RegisterSession B after release")
	s.True(allowed, "释放后新会话应立即可注册")

	count, err := s.cache.GetActiveSessionCount(s.ctx, accountID)
	s.RequireNoError(err, "GetActiveSessionCount")
	s.Equal(1, count, "释放后活跃会话数应为 1（仅 B）")
}

// TestUnregisterSession_IdempotentAndMissing 验证释放操作幂等：
// 重复释放与释放不存在的会话均不应报错。
func (s *SessionLimitCacheSuite) TestUnregisterSession_IdempotentAndMissing() {
	const accountID = int64(102)

	s.RequireNoError(s.cache.UnregisterSession(s.ctx, accountID, "missing"), "释放不存在的会话")
	s.RequireNoError(s.cache.UnregisterSession(s.ctx, accountID, "missing"), "重复释放")

	s.RequireNoError(s.cache.UnregisterSession(s.ctx, accountID, ""), "空 sessionUUID 应为 no-op")
}

// TestUnregisterSession_OnlyTargetsSpecifiedSession 验证释放只影响目标会话，
// 不影响同账号上的其他活跃会话。
func (s *SessionLimitCacheSuite) TestUnregisterSession_OnlyTargetsSpecifiedSession() {
	const accountID = int64(103)
	maxSessions := 2
	idleTimeout := 5 * time.Minute

	for _, sid := range []string{"session-A", "session-B"} {
		allowed, err := s.cache.RegisterSession(s.ctx, accountID, sid, maxSessions, idleTimeout)
		s.RequireNoError(err, "RegisterSession "+sid)
		s.True(allowed)
	}

	s.RequireNoError(s.cache.UnregisterSession(s.ctx, accountID, "session-A"), "UnregisterSession A")

	active, err := s.cache.IsSessionActive(s.ctx, accountID, "session-A")
	s.RequireNoError(err, "IsSessionActive A")
	s.False(active, "A 应已释放")

	active, err = s.cache.IsSessionActive(s.ctx, accountID, "session-B")
	s.RequireNoError(err, "IsSessionActive B")
	s.True(active, "B 不应受影响")
}

func TestSessionLimitCacheSuite(t *testing.T) {
	suite.Run(t, new(SessionLimitCacheSuite))
}
