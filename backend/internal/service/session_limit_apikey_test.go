package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// sessionLimitCheckStub 只实现 checkAndRegisterSession 所需的 RegisterSession，
// 其余接口方法由内嵌的 SessionLimitCache 接口占位（与 gateway_hotpath_optimization_test.go 同模式）。
type sessionLimitCheckStub struct {
	SessionLimitCache

	mu     sync.Mutex
	active map[int64]map[string]bool
	err    error
}

func (s *sessionLimitCheckStub) RegisterSession(_ context.Context, accountID int64, sessionUUID string, maxSessions int, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return false, s.err
	}
	if s.active == nil {
		s.active = make(map[int64]map[string]bool)
	}
	if s.active[accountID] == nil {
		s.active[accountID] = make(map[string]bool)
	}
	if s.active[accountID][sessionUUID] {
		return true, nil
	}
	if len(s.active[accountID]) >= maxSessions {
		return false, nil
	}
	s.active[accountID][sessionUUID] = true
	return true, nil
}

func TestCheckAndRegisterSessionForAPIKeyAccounts(t *testing.T) {
	ctx := context.Background()

	t.Run("api key account with max_sessions enforces the cap", func(t *testing.T) {
		svc := &GatewayService{sessionLimitCache: &sessionLimitCheckStub{}}
		account := &Account{
			ID:       101,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra:    map[string]any{"max_sessions": 2},
		}
		require.True(t, svc.checkAndRegisterSession(ctx, account, "session-a"))
		require.True(t, svc.checkAndRegisterSession(ctx, account, "session-a")) // 已有会话保持允许
		require.True(t, svc.checkAndRegisterSession(ctx, account, "session-b"))
		require.False(t, svc.checkAndRegisterSession(ctx, account, "session-c")) // 第 3 个新会话被拒绝
	})

	t.Run("api key account without max_sessions stays unlimited", func(t *testing.T) {
		svc := &GatewayService{sessionLimitCache: &sessionLimitCheckStub{}}
		account := &Account{ID: 102, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
		for i := 0; i < 10; i++ {
			require.True(t, svc.checkAndRegisterSession(ctx, account, "session-"+string(rune('a'+i))))
		}
	})

	t.Run("anthropic oauth accounts keep the existing limit", func(t *testing.T) {
		svc := &GatewayService{sessionLimitCache: &sessionLimitCheckStub{}}
		account := &Account{
			ID:       103,
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"max_sessions": 1},
		}
		require.True(t, svc.checkAndRegisterSession(ctx, account, "session-a"))
		require.False(t, svc.checkAndRegisterSession(ctx, account, "session-b"))
	})

	t.Run("other account types are unaffected", func(t *testing.T) {
		svc := &GatewayService{sessionLimitCache: &sessionLimitCheckStub{}}
		account := &Account{
			ID:       104,
			Platform: PlatformOpenAI,
			Type:     AccountTypeUpstream,
			Extra:    map[string]any{"max_sessions": 1},
		}
		require.True(t, svc.checkAndRegisterSession(ctx, account, "session-a"))
		require.True(t, svc.checkAndRegisterSession(ctx, account, "session-b"))
	})

	t.Run("cache error fails open for api key accounts", func(t *testing.T) {
		svc := &GatewayService{sessionLimitCache: &sessionLimitCheckStub{err: context.Canceled}}
		account := &Account{
			ID:       105,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra:    map[string]any{"max_sessions": 1},
		}
		require.True(t, svc.checkAndRegisterSession(ctx, account, "session-a"))
	})
}
