//go:build unit

package handler

import (
	"bytes"
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// a108ConcurrencyCache：账号槽始终被占满（选号只能拿到 WaitPlan），等待队列是否已满可配置。
type a108ConcurrencyCache struct {
	fakeConcurrencyCache
	waitAllowed bool
}

func (f *a108ConcurrencyCache) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	return false, nil
}

func (f *a108ConcurrencyCache) IncrementAccountWaitCount(context.Context, int64, int) (bool, error) {
	return f.waitAllowed, nil
}

// a108SessionLimitCache 记录会话注册/释放调用。
type a108SessionLimitCache struct {
	service.SessionLimitCache

	mu           sync.Mutex
	registered   map[int64][]string
	unregistered map[int64][]string
}

func (s *a108SessionLimitCache) RegisterSession(_ context.Context, accountID int64, sessionUUID string, _ int, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registered[accountID] = append(s.registered[accountID], sessionUUID)
	return true, nil
}

func (s *a108SessionLimitCache) UnregisterSession(_ context.Context, accountID int64, sessionUUID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unregistered[accountID] = append(s.unregistered[accountID], sessionUUID)
	return nil
}

func (s *a108SessionLimitCache) snapshot(accountID int64) (registered, unregistered []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.registered[accountID]...), append([]string(nil), s.unregistered[accountID]...)
}

func a108RunMessagesWithWaitPlan(t *testing.T, waitAllowed bool) (int, []string, []string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	groupID := int64(10801)
	accountID := int64(10802)
	group := &service.Group{ID: groupID, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive}
	account := &service.Account{
		ID:            accountID,
		Name:          "oauth-a108",
		Platform:      service.PlatformAnthropic,
		Type:          service.AccountTypeOAuth,
		Credentials:   map[string]any{"access_token": "tok_a108"},
		Extra:         map[string]any{"max_sessions": 3},
		Concurrency:   1,
		Priority:      1,
		Status:        service.StatusActive,
		Schedulable:   true,
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	cfg.Gateway.Scheduling.FallbackWaitTimeout = 50 * time.Millisecond
	cfg.Gateway.Scheduling.FallbackMaxWaiting = 10

	concurrencySvc := service.NewConcurrencyService(&a108ConcurrencyCache{waitAllowed: waitAllowed})
	sessionCache := &a108SessionLimitCache{registered: map[int64][]string{}, unregistered: map[int64][]string{}}
	schedulerSnapshot := service.NewSchedulerSnapshotService(&fakeSchedulerCache{accounts: []*service.Account{account}}, nil, nil, nil, nil)
	gwSvc := service.NewGatewayService(
		nil, &fakeGroupRepo{group: group}, nil, nil, nil, nil, nil, nil, cfg,
		schedulerSnapshot, concurrencySvc, nil, nil, nil, nil, nil, nil, nil,
		sessionCache, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheSvc.Stop)

	h := &GatewayHandler{
		gatewayService:      gwSvc,
		billingCacheService: billingCacheSvc,
		concurrencyHelper:   NewConcurrencyHelper(concurrencySvc, SSEPingFormatClaude, 0),
		maxAccountSwitches:  1,
		cfg:                 cfg,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":256,"messages":[{"role":"user","content":"hello a108"}]}`)
	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
	c.Request = req

	apiKey := &service.APIKey{
		ID: 10803, UserID: 10804, GroupID: &groupID, Group: group, Status: service.StatusActive,
		User: &service.User{ID: 10804, Concurrency: 10, Balance: 100},
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 10})

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

	registered, unregistered := sessionCache.snapshot(accountID)
	return rec.Code, registered, unregistered
}

// 等待队列已满：选号阶段已注册会话，429 返回前必须释放。
func TestA108MessagesWaitQueueFullReleasesSession(t *testing.T) {
	code, registered, unregistered := a108RunMessagesWithWaitPlan(t, false)

	require.Equal(t, 429, code)
	require.NotEmpty(t, registered, "选号阶段应已注册会话")
	require.Equal(t, registered[:1], unregistered, "等待队列已满返回时必须释放会话注册")
}

// 账号槽等待超时：选号阶段已注册会话，失败返回前必须释放。
func TestA108MessagesAccountSlotWaitTimeoutReleasesSession(t *testing.T) {
	code, registered, unregistered := a108RunMessagesWithWaitPlan(t, true)

	require.NotEqual(t, 200, code)
	require.NotEmpty(t, registered, "选号阶段应已注册会话")
	require.Equal(t, registered[:1], unregistered, "账号槽等待失败返回时必须释放会话注册")
}
