package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// sessionLimitReleaseCacheStub 记录 UnregisterSession 调用，用于验证释放逻辑
type sessionLimitReleaseCacheStub struct {
	SessionLimitCache

	unregistered map[int64][]string
	err          error
}

func newSessionLimitReleaseCacheStub() *sessionLimitReleaseCacheStub {
	return &sessionLimitReleaseCacheStub{
		unregistered: make(map[int64][]string),
	}
}

func (s *sessionLimitReleaseCacheStub) UnregisterSession(_ context.Context, accountID int64, sessionUUID string) error {
	if s.err != nil {
		return s.err
	}
	s.unregistered[accountID] = append(s.unregistered[accountID], sessionUUID)
	return nil
}

func newSessionLimitTestAccount() *Account {
	return &Account{
		ID:       42,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"max_sessions": 1},
	}
}

// TestReleaseAccountSession_ReleasesRegisteredSlot 验证：
// 对启用会话限制的 Anthropic OAuth 账号，ReleaseAccountSession 必须立即移除
// 该账号上注册的会话（不等待空闲超时）。
func TestReleaseAccountSession_ReleasesRegisteredSlot(t *testing.T) {
	cache := newSessionLimitReleaseCacheStub()
	svc := &GatewayService{sessionLimitCache: cache}
	acc := newSessionLimitTestAccount()

	svc.ReleaseAccountSession(context.Background(), acc, "session-hash-1")

	require.Equal(t, []string{"session-hash-1"}, cache.unregistered[42],
		"应立即移除注册的会话槽")
}

// TestReleaseAccountSession_NoOpForInapplicableAccounts 验证：
// - 非 Anthropic OAuth/SetupToken 账号
// - 未启用 max_sessions 的账号
// - 空 sessionID
// 以上场景均为 no-op，不得触发 UnregisterSession。
func TestReleaseAccountSession_NoOpForInapplicableAccounts(t *testing.T) {
	apiKeyAcc := &Account{
		ID:       43,
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{"max_sessions": 1},
	}
	noLimitAcc := &Account{
		ID:       44,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
	}
	enabledAcc := newSessionLimitTestAccount()

	cases := []struct {
		name      string
		account   *Account
		sessionID string
	}{
		{"api_key_account", apiKeyAcc, "session-hash"},
		{"max_sessions_disabled", noLimitAcc, "session-hash"},
		{"empty_session_id", enabledAcc, ""},
		{"nil_account", nil, "session-hash"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cache := newSessionLimitReleaseCacheStub()
			svc := &GatewayService{sessionLimitCache: cache}
			svc.ReleaseAccountSession(context.Background(), tc.account, tc.sessionID)
			require.Empty(t, cache.unregistered, "不适用账号不应触发释放")
		})
	}
}

// TestReleaseAccountSession_NilCacheAndErrorTolerance 验证：
// sessionLimitCache 不可用时 no-op；UnregisterSession 返回错误时不 panic（仅记录日志）。
func TestReleaseAccountSession_NilCacheAndErrorTolerance(t *testing.T) {
	// nil cache：no-op
	svc := &GatewayService{}
	svc.ReleaseAccountSession(context.Background(), newSessionLimitTestAccount(), "session-hash")

	// 底层错误：不 panic
	cache := &sessionLimitReleaseCacheStub{err: errors.New("redis down")}
	svc = &GatewayService{sessionLimitCache: cache}
	svc.ReleaseAccountSession(context.Background(), newSessionLimitTestAccount(), "session-hash")
}

// TestReleaseAccountSession_Idempotent 验证释放操作幂等，可安全重复调用
// （failover 链上按次释放 + defer 兜底可能对同一账号重复释放）。
func TestReleaseAccountSession_Idempotent(t *testing.T) {
	cache := newSessionLimitReleaseCacheStub()
	svc := &GatewayService{sessionLimitCache: cache}
	acc := newSessionLimitTestAccount()

	svc.ReleaseAccountSession(context.Background(), acc, "session-hash")
	svc.ReleaseAccountSession(context.Background(), acc, "session-hash")

	// 两次调用都透传到缓存层（Redis ZREM 本身幂等，重复移除无副作用）
	require.Len(t, cache.unregistered[42], 2)
}
