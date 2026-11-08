//go:build unit

package handler

import (
	"bytes"
	"context"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// bucketRecordingSchedulerCache 记录读取过的快照桶平台。
type bucketRecordingSchedulerCache struct {
	*fakeSchedulerCache
	mu        sync.Mutex
	platforms []string
}

func (c *bucketRecordingSchedulerCache) GetSnapshot(ctx context.Context, bucket service.SchedulerBucket) ([]*service.Account, bool, error) {
	c.mu.Lock()
	c.platforms = append(c.platforms, bucket.Platform)
	c.mu.Unlock()
	return c.fakeSchedulerCache.GetSnapshot(ctx, bucket)
}

func (c *bucketRecordingSchedulerCache) readPlatforms() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.platforms...)
}

// 非 antigravity 账号的请求不读取单 antigravity 账号分组判定，选号之外不查询 antigravity 快照桶。
func TestGeminiGroupRequestsDoNotReadAntigravityBucket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name  string
		route string
		path  string
		body  string
		call  func(h *GatewayHandler) func(*gin.Context)
	}{
		{
			name:  "gemini native",
			route: "/v1beta/models/*modelAction",
			path:  "/v1beta/models/gemini-2.5-flash:generateContent",
			body:  `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`,
			call:  func(h *GatewayHandler) func(*gin.Context) { return h.GeminiV1BetaModels },
		},
		{
			name:  "messages on gemini group",
			route: "/v1/messages",
			path:  "/v1/messages",
			body:  `{"model":"gemini-2.5-flash","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`,
			call:  func(h *GatewayHandler) func(*gin.Context) { return h.Messages },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newGeminiClientCancelFixture(t)

			f.serve(t, tt.route, tt.path, tt.body, tt.call(f.handler))

			require.Equal(t, int32(1), f.upstream.calls.Load(), "请求应完成选号并到达上游")
			platforms := f.schedulerCache.readPlatforms()
			require.NotEmpty(t, platforms)
			require.NotContains(t, platforms, service.PlatformAntigravity)
		})
	}
}

func TestMessagesAnthropicGroupDoesNotReadAntigravityBucket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(9400)
	accountID := int64(9401)
	group := &service.Group{ID: groupID, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive}
	account := &service.Account{
		ID:            accountID,
		Platform:      service.PlatformAnthropic,
		Type:          service.AccountTypeAPIKey,
		Credentials:   map[string]any{"api_key": "test-key", "intercept_warmup_requests": true},
		Status:        service.StatusActive,
		Schedulable:   true,
		Concurrency:   1,
		Priority:      1,
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}
	schedulerCache := &bucketRecordingSchedulerCache{fakeSchedulerCache: &fakeSchedulerCache{accounts: []*service.Account{account}}}
	h, cleanup := newTestGatewayHandlerWithSchedulerCache(t, group, schedulerCache)
	t.Cleanup(cleanup)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := `{"model":"claude-sonnet-4-5","max_tokens":256,"messages":[{"role":"user","content":[{"type":"text","text":"Warmup"}]}]}`
	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
	apiKey := &service.APIKey{
		ID: 9402, UserID: 9403, GroupID: &groupID, Group: group, Status: service.StatusActive,
		User: &service.User{ID: 9403, Concurrency: 10, Balance: 100},
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 10})

	h.Messages(c)

	require.Equal(t, 200, rec.Code)
	selected, ok := c.Get(opsAccountIDKey)
	require.True(t, ok)
	require.Equal(t, accountID, selected)
	platforms := schedulerCache.readPlatforms()
	require.NotEmpty(t, platforms)
	require.NotContains(t, platforms, service.PlatformAntigravity)
}
