//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// --- mock: Internal500CounterCache ---

type mockInternal500Cache struct {
	incrementCount int64
	incrementErr   error
	resetErr       error

	incrementCalls []int64 // 记录 IncrementInternal500Count 被调用时的 accountID
	resetCalls     []int64 // 记录 ResetInternal500Count 被调用时的 accountID
}

func (m *mockInternal500Cache) IncrementInternal500Count(_ context.Context, accountID int64) (int64, error) {
	m.incrementCalls = append(m.incrementCalls, accountID)
	return m.incrementCount, m.incrementErr
}

func (m *mockInternal500Cache) ResetInternal500Count(_ context.Context, accountID int64) error {
	m.resetCalls = append(m.resetCalls, accountID)
	return m.resetErr
}

// --- mock: 专用于 internal500 惩罚测试的 AccountRepository ---

type internal500AccountRepoStub struct {
	AccountRepository // 嵌入接口，未实现的方法会 panic（不应被调用）

	tempUnschedCalls []tempUnschedCall
	setErrorCalls    []setErrorCall
}

type tempUnschedCall struct {
	accountID int64
	until     time.Time
	reason    string
}

type setErrorCall struct {
	accountID int64
	reason    string
}

func (r *internal500AccountRepoStub) SetTempUnschedulable(_ context.Context, id int64, until time.Time, reason string) error {
	r.tempUnschedCalls = append(r.tempUnschedCalls, tempUnschedCall{accountID: id, until: until, reason: reason})
	return nil
}

func (r *internal500AccountRepoStub) SetError(_ context.Context, id int64, errorMsg string) error {
	r.setErrorCalls = append(r.setErrorCalls, setErrorCall{accountID: id, reason: errorMsg})
	return nil
}

// =============================================================================
// TestIsAntigravityInternalServerError
// =============================================================================

func TestIsAntigravityInternalServerError(t *testing.T) {
	t.Run("匹配完整的 INTERNAL 500 body", func(t *testing.T) {
		body := []byte(`{"error":{"code":500,"message":"Internal error encountered.","status":"INTERNAL"}}`)
		require.True(t, isAntigravityInternalServerError(500, body))
	})

	t.Run("statusCode 不是 500", func(t *testing.T) {
		body := []byte(`{"error":{"code":500,"message":"Internal error encountered.","status":"INTERNAL"}}`)
		require.False(t, isAntigravityInternalServerError(429, body))
		require.False(t, isAntigravityInternalServerError(503, body))
		require.False(t, isAntigravityInternalServerError(200, body))
	})

	t.Run("body 中 message 不匹配", func(t *testing.T) {
		body := []byte(`{"error":{"code":500,"message":"Some other error","status":"INTERNAL"}}`)
		require.False(t, isAntigravityInternalServerError(500, body))
	})

	t.Run("body 中 status 不匹配", func(t *testing.T) {
		body := []byte(`{"error":{"code":500,"message":"Internal error encountered.","status":"UNAVAILABLE"}}`)
		require.False(t, isAntigravityInternalServerError(500, body))
	})

	t.Run("body 中 code 不匹配", func(t *testing.T) {
		body := []byte(`{"error":{"code":503,"message":"Internal error encountered.","status":"INTERNAL"}}`)
		require.False(t, isAntigravityInternalServerError(500, body))
	})

	t.Run("空 body", func(t *testing.T) {
		require.False(t, isAntigravityInternalServerError(500, []byte{}))
		require.False(t, isAntigravityInternalServerError(500, nil))
	})

	t.Run("其他 500 错误格式（纯文本）", func(t *testing.T) {
		body := []byte(`Internal Server Error`)
		require.False(t, isAntigravityInternalServerError(500, body))
	})

	t.Run("其他 500 错误格式（不同 JSON 结构）", func(t *testing.T) {
		body := []byte(`{"message":"Internal Server Error","statusCode":500}`)
		require.False(t, isAntigravityInternalServerError(500, body))
	})
}

// =============================================================================
// TestApplyInternal500Penalty
// =============================================================================

func TestApplyInternal500Penalty(t *testing.T) {
	t.Run("count=1 → SetTempUnschedulable 10 分钟", func(t *testing.T) {
		repo := &internal500AccountRepoStub{}
		svc := &AntigravityGatewayService{accountRepo: repo}
		account := &Account{ID: 1, Name: "acc-1"}

		before := time.Now()
		svc.applyInternal500Penalty(context.Background(), "[test]", account, 1)
		after := time.Now()

		require.Len(t, repo.tempUnschedCalls, 1)
		require.Empty(t, repo.setErrorCalls)

		call := repo.tempUnschedCalls[0]
		require.Equal(t, int64(1), call.accountID)
		require.Contains(t, call.reason, "INTERNAL 500")
		// until 应在 [before+10m, after+10m] 范围内
		require.True(t, call.until.After(before.Add(internal500PenaltyTier1Duration).Add(-time.Second)))
		require.True(t, call.until.Before(after.Add(internal500PenaltyTier1Duration).Add(time.Second)))
	})

	t.Run("count=2 → SetTempUnschedulable 10 小时", func(t *testing.T) {
		repo := &internal500AccountRepoStub{}
		svc := &AntigravityGatewayService{accountRepo: repo}
		account := &Account{ID: 2, Name: "acc-2"}

		before := time.Now()
		svc.applyInternal500Penalty(context.Background(), "[test]", account, 2)
		after := time.Now()

		require.Len(t, repo.tempUnschedCalls, 1)
		require.Empty(t, repo.setErrorCalls)

		call := repo.tempUnschedCalls[0]
		require.Equal(t, int64(2), call.accountID)
		require.Contains(t, call.reason, "INTERNAL 500")
		require.True(t, call.until.After(before.Add(internal500PenaltyTier2Duration).Add(-time.Second)))
		require.True(t, call.until.Before(after.Add(internal500PenaltyTier2Duration).Add(time.Second)))
	})

	t.Run("count=3 → SetError 永久禁用", func(t *testing.T) {
		repo := &internal500AccountRepoStub{}
		svc := &AntigravityGatewayService{accountRepo: repo}
		account := &Account{ID: 3, Name: "acc-3"}

		svc.applyInternal500Penalty(context.Background(), "[test]", account, 3)

		require.Empty(t, repo.tempUnschedCalls)
		require.Len(t, repo.setErrorCalls, 1)

		call := repo.setErrorCalls[0]
		require.Equal(t, int64(3), call.accountID)
		require.Contains(t, call.reason, "INTERNAL 500 consecutive failures: 3")
	})

	t.Run("count=5 → SetError 永久禁用（>=3 都走永久禁用）", func(t *testing.T) {
		repo := &internal500AccountRepoStub{}
		svc := &AntigravityGatewayService{accountRepo: repo}
		account := &Account{ID: 5, Name: "acc-5"}

		svc.applyInternal500Penalty(context.Background(), "[test]", account, 5)

		require.Empty(t, repo.tempUnschedCalls)
		require.Len(t, repo.setErrorCalls, 1)

		call := repo.setErrorCalls[0]
		require.Equal(t, int64(5), call.accountID)
		require.Contains(t, call.reason, "INTERNAL 500 consecutive failures: 5")
	})

	t.Run("count=0 → 不调用任何方法", func(t *testing.T) {
		repo := &internal500AccountRepoStub{}
		svc := &AntigravityGatewayService{accountRepo: repo}
		account := &Account{ID: 10, Name: "acc-10"}

		svc.applyInternal500Penalty(context.Background(), "[test]", account, 0)

		require.Empty(t, repo.tempUnschedCalls)
		require.Empty(t, repo.setErrorCalls)
	})
}

// =============================================================================
// TestHandleInternal500RetryExhausted
// =============================================================================

func TestHandleInternal500RetryExhausted(t *testing.T) {
	t.Run("internal500Cache 为 nil → 不 panic，不调用任何方法", func(t *testing.T) {
		repo := &internal500AccountRepoStub{}
		svc := &AntigravityGatewayService{
			accountRepo:      repo,
			internal500Cache: nil,
		}
		account := &Account{ID: 1, Name: "acc-1"}

		// 不应 panic
		require.NotPanics(t, func() {
			svc.handleInternal500RetryExhausted(context.Background(), "[test]", account)
		})
		require.Empty(t, repo.tempUnschedCalls)
		require.Empty(t, repo.setErrorCalls)
	})

	t.Run("IncrementInternal500Count 返回 error → 不调用惩罚方法", func(t *testing.T) {
		repo := &internal500AccountRepoStub{}
		cache := &mockInternal500Cache{
			incrementErr: errors.New("redis connection error"),
		}
		svc := &AntigravityGatewayService{
			accountRepo:      repo,
			internal500Cache: cache,
		}
		account := &Account{ID: 2, Name: "acc-2"}

		svc.handleInternal500RetryExhausted(context.Background(), "[test]", account)

		require.Len(t, cache.incrementCalls, 1)
		require.Equal(t, int64(2), cache.incrementCalls[0])
		require.Empty(t, repo.tempUnschedCalls)
		require.Empty(t, repo.setErrorCalls)
	})

	t.Run("IncrementInternal500Count 返回 count=1 → 触发 tier1 惩罚", func(t *testing.T) {
		repo := &internal500AccountRepoStub{}
		cache := &mockInternal500Cache{
			incrementCount: 1,
		}
		svc := &AntigravityGatewayService{
			accountRepo:      repo,
			internal500Cache: cache,
		}
		account := &Account{ID: 3, Name: "acc-3"}

		svc.handleInternal500RetryExhausted(context.Background(), "[test]", account)

		require.Len(t, cache.incrementCalls, 1)
		require.Equal(t, int64(3), cache.incrementCalls[0])
		// tier1: SetTempUnschedulable
		require.Len(t, repo.tempUnschedCalls, 1)
		require.Equal(t, int64(3), repo.tempUnschedCalls[0].accountID)
		require.Empty(t, repo.setErrorCalls)
	})

	t.Run("IncrementInternal500Count 返回 count=3 → 触发 tier3 永久禁用", func(t *testing.T) {
		repo := &internal500AccountRepoStub{}
		cache := &mockInternal500Cache{
			incrementCount: 3,
		}
		svc := &AntigravityGatewayService{
			accountRepo:      repo,
			internal500Cache: cache,
		}
		account := &Account{ID: 4, Name: "acc-4"}

		svc.handleInternal500RetryExhausted(context.Background(), "[test]", account)

		require.Len(t, cache.incrementCalls, 1)
		require.Empty(t, repo.tempUnschedCalls)
		require.Len(t, repo.setErrorCalls, 1)
		require.Equal(t, int64(4), repo.setErrorCalls[0].accountID)
	})
}

// =============================================================================
// TestResetInternal500Counter
// =============================================================================

func TestResetInternal500Counter(t *testing.T) {
	t.Run("internal500Cache 为 nil → 不 panic", func(t *testing.T) {
		svc := &AntigravityGatewayService{
			internal500Cache: nil,
		}

		require.NotPanics(t, func() {
			svc.resetInternal500Counter(context.Background(), "[test]", 1)
		})
	})

	t.Run("ResetInternal500Count 返回 error → 不 panic（仅日志）", func(t *testing.T) {
		cache := &mockInternal500Cache{
			resetErr: errors.New("redis timeout"),
		}
		svc := &AntigravityGatewayService{
			internal500Cache: cache,
		}

		require.NotPanics(t, func() {
			svc.resetInternal500Counter(context.Background(), "[test]", 42)
		})
		require.Len(t, cache.resetCalls, 1)
		require.Equal(t, int64(42), cache.resetCalls[0])
	})

	t.Run("正常调用 → 调用 ResetInternal500Count", func(t *testing.T) {
		cache := &mockInternal500Cache{}
		svc := &AntigravityGatewayService{
			internal500Cache: cache,
		}

		svc.resetInternal500Counter(context.Background(), "[test]", 99)

		require.Len(t, cache.resetCalls, 1)
		require.Equal(t, int64(99), cache.resetCalls[0])
	})
}
