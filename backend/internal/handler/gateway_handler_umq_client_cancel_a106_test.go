//go:build unit

package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// a106CountingUpstream 统计上游调用次数；本测试中任何调用都意味着请求被转发。
type a106CountingUpstream struct {
	calls atomic.Int64
}

func (u *a106CountingUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	u.calls.Add(1)
	return nil, errors.New("a106: upstream unavailable")
}

func (u *a106CountingUpstream) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	u.calls.Add(1)
	return nil, errors.New("a106: upstream unavailable")
}

// a106HeldLockUMQCache：串行锁始终被其他请求持有。
type a106HeldLockUMQCache struct{}

func (a106HeldLockUMQCache) AcquireLock(context.Context, int64, string, int) (bool, error) {
	return false, nil
}
func (a106HeldLockUMQCache) ReleaseLock(context.Context, int64, string) (bool, error) {
	return true, nil
}
func (a106HeldLockUMQCache) GetLastCompletedMs(context.Context, int64) (int64, error) { return 0, nil }
func (a106HeldLockUMQCache) GetCurrentTimeMs(context.Context) (int64, error)          { return 0, nil }
func (a106HeldLockUMQCache) ReconcileExpiredLockCandidates(context.Context, int) (int, error) {
	return 0, nil
}

// a106RunMessagesUMQ 以流式真实用户消息调用 Messages；cancelAfter>0 时在排队期间模拟客户端断开。
func a106RunMessagesUMQ(t *testing.T, mode string, waitTimeoutMs int, cancelAfter time.Duration) (int64, int) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	groupID := int64(10601)
	accountID := int64(10602)
	group := &service.Group{ID: groupID, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive}
	account := &service.Account{
		ID:            accountID,
		Name:          "oauth-a106",
		Platform:      service.PlatformAnthropic,
		Type:          service.AccountTypeOAuth,
		Credentials:   map[string]any{"access_token": "tok_a106"},
		Extra:         map[string]any{"user_msg_queue_mode": mode},
		Concurrency:   1,
		Priority:      1,
		Status:        service.StatusActive,
		Schedulable:   true,
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.UserMessageQueue.WaitTimeoutMs = waitTimeoutMs
	cfg.Gateway.UserMessageQueue.MinDelayMs = 5000
	cfg.Gateway.UserMessageQueue.MaxDelayMs = 5000

	upstream := &a106CountingUpstream{}
	schedulerSnapshot := service.NewSchedulerSnapshotService(&fakeSchedulerCache{accounts: []*service.Account{account}}, nil, nil, nil, nil)
	gwSvc := service.NewGatewayService(
		nil, &fakeGroupRepo{group: group}, nil, nil, nil, nil, nil, nil, nil,
		schedulerSnapshot, nil, nil, nil, nil, nil, upstream, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheSvc.Stop)

	umqSvc := service.NewUserMessageQueueService(a106HeldLockUMQCache{}, nil, &cfg.Gateway.UserMessageQueue)
	h := &GatewayHandler{
		gatewayService:      gwSvc,
		billingCacheService: billingCacheSvc,
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(&fakeConcurrencyCache{}), SSEPingFormatClaude, 0),
		userMsgQueueHelper:  NewUserMsgQueueHelper(umqSvc, SSEPingFormatClaude, time.Hour),
		maxAccountSwitches:  1,
		cfg:                 cfg,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":256,"stream":true,"messages":[{"role":"user","content":"hello a106"}]}`)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ctxkey.Group, group))
	defer cancel()
	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	apiKey := &service.APIKey{
		ID: 10603, UserID: 10604, GroupID: &groupID, Group: group, Status: service.StatusActive,
		User: &service.User{ID: 10604, Concurrency: 10, Balance: 100},
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 10})

	if cancelAfter > 0 {
		time.AfterFunc(cancelAfter, cancel)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.Messages(c)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Messages did not return")
	}
	return upstream.calls.Load(), c.Writer.Status()
}

// 排队（serialize 等锁 / throttle 延迟）期间客户端断开：不得继续转发上游。
func TestA106MessagesUMQClientCancelDoesNotForward(t *testing.T) {
	for _, mode := range []string{config.UMQModeSerialize, config.UMQModeThrottle} {
		t.Run(mode, func(t *testing.T) {
			calls, status := a106RunMessagesUMQ(t, mode, 5000, 100*time.Millisecond)
			require.Zero(t, calls, "客户端已断开的排队请求不应转发上游")
			require.Equal(t, statusClientClosedRequest, status)
		})
	}
}

// 客户端仍在线时的真实等待超时保持 fail-open：照常转发。
func TestA106MessagesUMQWaitTimeoutStillFailsOpen(t *testing.T) {
	calls, _ := a106RunMessagesUMQ(t, config.UMQModeSerialize, 100, 0)
	require.NotZero(t, calls, "等待超时且客户端在线时应 fail-open 继续转发")
}
