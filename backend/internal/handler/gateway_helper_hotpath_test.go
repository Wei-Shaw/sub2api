package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type helperConcurrencyCacheStub struct {
	mu sync.Mutex

	accountSeq []bool
	userSeq    []bool

	accountAcquireCalls int
	userAcquireCalls    int
	accountReleaseCalls int
	userReleaseCalls    int
}

func (s *helperConcurrencyCacheStub) AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accountAcquireCalls++
	if len(s.accountSeq) == 0 {
		return false, nil
	}
	v := s.accountSeq[0]
	s.accountSeq = s.accountSeq[1:]
	return v, nil
}

func (s *helperConcurrencyCacheStub) ReleaseAccountSlot(ctx context.Context, accountID int64, requestID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accountReleaseCalls++
	return nil
}

func (s *helperConcurrencyCacheStub) GetAccountConcurrency(ctx context.Context, accountID int64) (int, error) {
	return 0, nil
}

func (s *helperConcurrencyCacheStub) GetAccountConcurrencyBatch(ctx context.Context, accountIDs []int64) (map[int64]int, error) {
	out := make(map[int64]int, len(accountIDs))
	for _, accountID := range accountIDs {
		out[accountID] = 0
	}
	return out, nil
}

func (s *helperConcurrencyCacheStub) IncrementAccountWaitCount(ctx context.Context, accountID int64, maxWait int) (bool, error) {
	return true, nil
}

func (s *helperConcurrencyCacheStub) DecrementAccountWaitCount(ctx context.Context, accountID int64) error {
	return nil
}

func (s *helperConcurrencyCacheStub) GetAccountWaitingCount(ctx context.Context, accountID int64) (int, error) {
	return 0, nil
}

func (s *helperConcurrencyCacheStub) AcquireUserSlot(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userAcquireCalls++
	if len(s.userSeq) == 0 {
		return false, nil
	}
	v := s.userSeq[0]
	s.userSeq = s.userSeq[1:]
	return v, nil
}

func (s *helperConcurrencyCacheStub) ReleaseUserSlot(ctx context.Context, userID int64, requestID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userReleaseCalls++
	return nil
}

func (s *helperConcurrencyCacheStub) GetUserConcurrency(ctx context.Context, userID int64) (int, error) {
	return 0, nil
}

func (s *helperConcurrencyCacheStub) IncrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error) {
	return true, nil
}

func (s *helperConcurrencyCacheStub) DecrementWaitCount(ctx context.Context, userID int64) error {
	return nil
}

func (s *helperConcurrencyCacheStub) GetAccountsLoadBatch(ctx context.Context, accounts []service.AccountWithConcurrency) (map[int64]*service.AccountLoadInfo, error) {
	out := make(map[int64]*service.AccountLoadInfo, len(accounts))
	for _, acc := range accounts {
		out[acc.ID] = &service.AccountLoadInfo{AccountID: acc.ID}
	}
	return out, nil
}

func (s *helperConcurrencyCacheStub) GetUsersLoadBatch(ctx context.Context, users []service.UserWithConcurrency) (map[int64]*service.UserLoadInfo, error) {
	out := make(map[int64]*service.UserLoadInfo, len(users))
	for _, user := range users {
		out[user.ID] = &service.UserLoadInfo{UserID: user.ID}
	}
	return out, nil
}

func (s *helperConcurrencyCacheStub) CleanupExpiredAccountSlots(ctx context.Context, accountID int64) error {
	return nil
}

func (s *helperConcurrencyCacheStub) CleanupStaleProcessSlots(ctx context.Context, activeRequestPrefix string) error {
	return nil
}

func newHelperTestContext(method, path string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, path, nil)
	return c, rec
}

func validClaudeCodeBodyJSON() []byte {
	return []byte(`{
		"model":"claude-3-5-sonnet-20241022",
		"system":[{"text":"You are Claude Code, Anthropic's official CLI for Claude."}],
		"metadata":{"user_id":"user_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa_account__session_aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}
	}`)
}

func TestSetClaudeCodeClientContext_FastPathAndStrictPath(t *testing.T) {
	t.Run("non_cli_user_agent_sets_false", func(t *testing.T) {
		c, _ := newHelperTestContext(http.MethodPost, "/v1/messages")
		c.Request.Header.Set("User-Agent", "curl/8.6.0")

		SetClaudeCodeClientContext(c, validClaudeCodeBodyJSON(), nil)
		require.False(t, service.IsClaudeCodeClient(c.Request.Context()))
	})

	t.Run("cli_non_messages_path_sets_true", func(t *testing.T) {
		c, _ := newHelperTestContext(http.MethodGet, "/v1/models")
		c.Request.Header.Set("User-Agent", "claude-cli/1.0.1")

		SetClaudeCodeClientContext(c, nil, nil)
		require.True(t, service.IsClaudeCodeClient(c.Request.Context()))
	})

	t.Run("cli_messages_path_valid_body_sets_true", func(t *testing.T) {
		c, _ := newHelperTestContext(http.MethodPost, "/v1/messages")
		c.Request.Header.Set("User-Agent", "claude-cli/1.0.1")
		c.Request.Header.Set("X-App", "claude-code")
		c.Request.Header.Set("anthropic-beta", "message-batches-2024-09-24")
		c.Request.Header.Set("anthropic-version", "2023-06-01")

		SetClaudeCodeClientContext(c, validClaudeCodeBodyJSON(), nil)
		require.True(t, service.IsClaudeCodeClient(c.Request.Context()))
	})

	t.Run("cli_messages_path_invalid_body_sets_false", func(t *testing.T) {
		c, _ := newHelperTestContext(http.MethodPost, "/v1/messages")
		c.Request.Header.Set("User-Agent", "claude-cli/1.0.1")
		// 缺少严格校验所需 header + body 字段
		SetClaudeCodeClientContext(c, []byte(`{"model":"x"}`), nil)
		require.False(t, service.IsClaudeCodeClient(c.Request.Context()))
	})
}

func TestSetClaudeCodeClientContext_ReuseParsedRequestAndContextCache(t *testing.T) {
	t.Run("reuse parsed request without body unmarshal", func(t *testing.T) {
		c, _ := newHelperTestContext(http.MethodPost, "/v1/messages")
		c.Request.Header.Set("User-Agent", "claude-cli/1.0.1")
		c.Request.Header.Set("X-App", "claude-code")
		c.Request.Header.Set("anthropic-beta", "message-batches-2024-09-24")
		c.Request.Header.Set("anthropic-version", "2023-06-01")

		parsedReq := &service.ParsedRequest{
			Model: "claude-3-5-sonnet-20241022",
			System: []any{
				map[string]any{"text": "You are Claude Code, Anthropic's official CLI for Claude."},
			},
			MetadataUserID: "user_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa_account__session_aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		}

		// body 非法 JSON，如果函数复用 parsedReq 成功则仍应判定为 Claude Code。
		SetClaudeCodeClientContext(c, []byte(`{invalid`), parsedReq)
		require.True(t, service.IsClaudeCodeClient(c.Request.Context()))
	})

	t.Run("reuse context cache without body unmarshal", func(t *testing.T) {
		c, _ := newHelperTestContext(http.MethodPost, "/v1/messages")
		c.Request.Header.Set("User-Agent", "claude-cli/1.0.1")
		c.Request.Header.Set("X-App", "claude-code")
		c.Request.Header.Set("anthropic-beta", "message-batches-2024-09-24")
		c.Request.Header.Set("anthropic-version", "2023-06-01")
		c.Set(service.OpenAIParsedRequestBodyKey, map[string]any{
			"model": "claude-3-5-sonnet-20241022",
			"system": []any{
				map[string]any{"text": "You are Claude Code, Anthropic's official CLI for Claude."},
			},
			"metadata": map[string]any{"user_id": "user_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa_account__session_aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"},
		})

		SetClaudeCodeClientContext(c, []byte(`{invalid`), nil)
		require.True(t, service.IsClaudeCodeClient(c.Request.Context()))
	})
}

func TestWaitForSlotWithPingTimeout_AccountAndUserAcquire(t *testing.T) {
	cache := &helperConcurrencyCacheStub{
		accountSeq: []bool{false, true},
		userSeq:    []bool{false, true},
	}
	concurrency := service.NewConcurrencyService(cache)
	helper := NewConcurrencyHelper(concurrency, SSEPingFormatNone, 5*time.Millisecond)

	t.Run("account_slot_acquired_after_retry", func(t *testing.T) {
		c, _ := newHelperTestContext(http.MethodPost, "/v1/messages")
		streamStarted := false
		release, err := helper.waitForSlotWithPingTimeout(c, "account", 101, 2, time.Second, false, &streamStarted, true)
		require.NoError(t, err)
		require.NotNil(t, release)
		require.False(t, streamStarted)
		release()
		require.GreaterOrEqual(t, cache.accountAcquireCalls, 2)
		require.GreaterOrEqual(t, cache.accountReleaseCalls, 1)
	})

	t.Run("user_slot_acquired_after_retry", func(t *testing.T) {
		c, _ := newHelperTestContext(http.MethodPost, "/v1/messages")
		streamStarted := false
		release, err := helper.waitForSlotWithPingTimeout(c, "user", 202, 3, time.Second, false, &streamStarted, true)
		require.NoError(t, err)
		require.NotNil(t, release)
		release()
		require.GreaterOrEqual(t, cache.userAcquireCalls, 2)
		require.GreaterOrEqual(t, cache.userReleaseCalls, 1)
	})
}

func TestWaitForSlotWithPingTimeout_TimeoutAndStreamPing(t *testing.T) {
	cache := &helperConcurrencyCacheStub{
		accountSeq: []bool{false, false, false},
	}
	concurrency := service.NewConcurrencyService(cache)

	t.Run("timeout_returns_concurrency_error", func(t *testing.T) {
		helper := NewConcurrencyHelper(concurrency, SSEPingFormatNone, 5*time.Millisecond)
		c, _ := newHelperTestContext(http.MethodPost, "/v1/messages")
		streamStarted := false
		release, err := helper.waitForSlotWithPingTimeout(c, "account", 101, 2, 130*time.Millisecond, false, &streamStarted, true)
		require.Nil(t, release)
		var cErr *ConcurrencyError
		require.ErrorAs(t, err, &cErr)
		require.True(t, cErr.IsTimeout)
	})

	t.Run("stream_mode_sends_ping_before_timeout", func(t *testing.T) {
		helper := NewConcurrencyHelper(concurrency, SSEPingFormatComment, 10*time.Millisecond)
		c, rec := newHelperTestContext(http.MethodPost, "/v1/messages")
		streamStarted := false
		release, err := helper.waitForSlotWithPingTimeout(c, "account", 101, 2, 70*time.Millisecond, true, &streamStarted, true)
		require.Nil(t, release)
		var cErr *ConcurrencyError
		require.ErrorAs(t, err, &cErr)
		require.True(t, cErr.IsTimeout)
		require.True(t, streamStarted)
		require.Contains(t, rec.Body.String(), ":\n\n")
	})
}

func TestWaitForSlotWithPingTimeout_AcquireError(t *testing.T) {
	errCache := &helperConcurrencyCacheStubWithError{
		err: errors.New("redis unavailable"),
	}
	concurrency := service.NewConcurrencyService(errCache)
	helper := NewConcurrencyHelper(concurrency, SSEPingFormatNone, 5*time.Millisecond)
	c, _ := newHelperTestContext(http.MethodPost, "/v1/messages")
	streamStarted := false
	release, err := helper.waitForSlotWithPingTimeout(c, "account", 1, 1, 200*time.Millisecond, false, &streamStarted, true)
	require.Nil(t, release)
	require.Error(t, err)
	require.Contains(t, err.Error(), "redis unavailable")
}

func TestAcquireAccountSlotWithWaitTimeout_ImmediateAttemptBeforeBackoff(t *testing.T) {
	cache := &helperConcurrencyCacheStub{
		accountSeq: []bool{false},
	}
	concurrency := service.NewConcurrencyService(cache)
	helper := NewConcurrencyHelper(concurrency, SSEPingFormatNone, 5*time.Millisecond)
	c, _ := newHelperTestContext(http.MethodPost, "/v1/messages")
	streamStarted := false

	release, err := helper.AcquireAccountSlotWithWaitTimeout(c, 301, 1, 30*time.Millisecond, false, &streamStarted)
	require.Nil(t, release)
	var cErr *ConcurrencyError
	require.ErrorAs(t, err, &cErr)
	require.True(t, cErr.IsTimeout)
	require.GreaterOrEqual(t, cache.accountAcquireCalls, 1)
}

type helperConcurrencyCacheStubWithError struct {
	helperConcurrencyCacheStub
	err error
}

func (s *helperConcurrencyCacheStubWithError) AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
	return false, s.err
}
