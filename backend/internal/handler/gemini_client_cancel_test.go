//go:build unit

package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// clientCancelUpstream 模拟上游尚未响应时客户端断开：Do 期间取消请求 context，
// 并按 net/http 的形态返回包裹 context.Canceled 的 *url.Error。
type clientCancelUpstream struct {
	service.HTTPUpstream
	cancel context.CancelFunc
	calls  atomic.Int32
}

func (u *clientCancelUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls.Add(1)
	u.cancel()
	return nil, &url.Error{Op: "Post", URL: req.URL.String(), Err: context.Canceled}
}

type geminiClientCancelFixture struct {
	handler  *GatewayHandler
	group    *service.Group
	apiKey   *service.APIKey
	upstream *clientCancelUpstream
	ctx      context.Context
}

func newGeminiClientCancelFixture(t *testing.T) *geminiClientCancelFixture {
	t.Helper()
	groupID := int64(9200)
	accountID := int64(9201)
	group := &service.Group{ID: groupID, Hydrated: true, Platform: service.PlatformGemini, Status: service.StatusActive}
	account := &service.Account{
		ID:            accountID,
		Platform:      service.PlatformGemini,
		Type:          service.AccountTypeAPIKey,
		Credentials:   map[string]any{"api_key": "test-key"},
		Status:        service.StatusActive,
		Schedulable:   true,
		Concurrency:   1,
		Priority:      1,
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}
	h, cleanup := newTestGatewayHandler(t, group, []*service.Account{account})
	t.Cleanup(cleanup)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	upstream := &clientCancelUpstream{cancel: cancel}
	h.geminiCompatService = service.NewGeminiMessagesCompatService(nil, nil, nil, nil, nil, nil, upstream, nil, &config.Config{})

	apiKey := &service.APIKey{
		ID: 9202, UserID: 9203, GroupID: &groupID, Group: group, Status: service.StatusActive,
		User: &service.User{ID: 9203, Concurrency: 10, Balance: 100},
	}
	return &geminiClientCancelFixture{handler: h, group: group, apiKey: apiKey, upstream: upstream, ctx: ctx}
}

// serve 经 ops 错误日志中间件执行请求，返回响应与 ops 队列中的条目数。
func (f *geminiClientCancelFixture) serve(t *testing.T, route, path, body string, call func(*gin.Context)) (*httptest.ResponseRecorder, int64) {
	t.Helper()
	setupOpsErrorLogTestQueue(t, 4)
	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.POST(route, func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), f.apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: f.apiKey.UserID, Concurrency: 10})
		call(c)
	})

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req = req.WithContext(context.WithValue(f.ctx, ctxkey.Group, f.group))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec, OpsErrorLogQueueLength()
}

// 原生入口：上游响应前客户端断开，响应未提交，标记 499；纯客户端取消不落 ops 错误日志。
func TestGeminiV1BetaModels_ClientCancelBeforeUpstreamResponseMarks499(t *testing.T) {
	gin.SetMode(gin.TestMode)
	f := newGeminiClientCancelFixture(t)

	rec, opsQueued := f.serve(t,
		"/v1beta/models/*modelAction",
		"/v1beta/models/gemini-2.5-flash:streamGenerateContent?alt=sse",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`,
		f.handler.GeminiV1BetaModels,
	)

	require.Equal(t, int32(1), f.upstream.calls.Load())
	require.Equal(t, statusClientClosedRequest, rec.Code)
	require.Zero(t, rec.Body.Len())
	require.Zero(t, opsQueued)
}

// Chat Completions 兼容入口（Gemini 分组）：同一场景标记 499，不再补写 502。
func TestGatewayChatCompletions_GeminiClientCancelBeforeUpstreamResponseMarks499(t *testing.T) {
	gin.SetMode(gin.TestMode)
	f := newGeminiClientCancelFixture(t)

	rec, opsQueued := f.serve(t,
		"/v1/chat/completions",
		"/v1/chat/completions",
		`{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"hi"}],"stream":true}`,
		f.handler.ChatCompletions,
	)

	require.Equal(t, int32(1), f.upstream.calls.Load())
	require.Equal(t, statusClientClosedRequest, rec.Code)
	require.Zero(t, rec.Body.Len())
	require.Zero(t, opsQueued)
}
