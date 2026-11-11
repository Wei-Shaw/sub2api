//go:build unit

package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func compositeHTTPTestDecision() service.CompositeRouteDecision {
	return service.CompositeRouteDecision{Matched: true, Source: service.CompositeRouteSourceExplicit, PublicModel: "public-ds-flash", TargetPlatform: service.PlatformOpenCodeGo, UpstreamModel: "deepseek-v4.1-flash", Endpoint: service.CompositeRouteEndpointAny, FallbackTargets: []service.CompositeFallbackTarget{{TargetPlatform: service.PlatformDeepseek, UpstreamModel: "deepseek-flash"}}}
}

func TestCompositeHTTPFallbackSafety(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		body                    string
		fail                    *service.UpstreamFailoverError
		selection               error
		write, cancel, disabled bool
		want                    bool
	}{
		{name: "empty pool", selection: service.ErrNoAvailableAccounts, want: true},
		{name: "candidate model exclusions", selection: errors.Join(service.ErrNoAvailableAccounts, errors.New("model_not_supported=1, rate_limited=1")), want: true},
		{name: "candidate capability exclusions", selection: errors.Join(service.ErrNoAvailableAccounts, errors.New("capability_mismatch=1")), want: true},
		{name: "channel pricing restriction", selection: errors.Join(service.ErrNoAvailableAccounts, errors.New("channel pricing restriction"))},
		{name: "channel upstream restriction", selection: errors.Join(service.ErrNoAvailableAccounts, errors.New("channel_upstream_restricted=1"))},
		{name: "mapping restriction", selection: errors.Join(service.ErrNoAvailableAccounts, errors.New("composite_fallback_model_restricted=1"))},
		{name: "429", fail: &service.UpstreamFailoverError{StatusCode: 429}, want: true},
		{name: "network", fail: &service.UpstreamFailoverError{StatusCode: 502}, want: true},
		{name: "timeout", fail: &service.UpstreamFailoverError{StatusCode: 408}, want: true},
		{name: "invalid request", fail: &service.UpstreamFailoverError{StatusCode: 400}},
		{name: "policy", fail: &service.UpstreamFailoverError{StatusCode: 403}},
		{name: "structured policy 502", fail: &service.UpstreamFailoverError{StatusCode: 502, ResponseBody: []byte(`{"error":{"code":"content_policy_violation"}}`)}},
		{name: "request scope", fail: &service.UpstreamFailoverError{StatusCode: 502, Scope: service.GatewayFailureScopeRequest}},
		{name: "permission", selection: errors.New("permission denied")},
		{name: "canceled", cancel: true},
		{name: "semantic output", write: true},
		{name: "bound response", body: `{"previous_response_id":"resp_bound"}`},
		{name: "bound cache", body: `{"cached_content":"cached/1"}`},
		{name: "media", disabled: true},
		{name: "stop", fail: &service.UpstreamFailoverError{StatusCode: 500, NextAccountAction: service.NextAccountStop}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil).WithContext(service.WithCompositeRouteDecision(ctx, compositeHTTPTestDecision()))
			f := newCompositeHTTPFallback(c, []byte(tc.body), tc.disabled)
			if tc.write {
				_, _ = c.Writer.WriteString("data: semantic\n\n")
			}
			if tc.cancel {
				cancel()
			}
			require.Equal(t, tc.want, f.advance(c, tc.selection, tc.fail))
			if tc.want {
				platform, _ := service.ResolvedTargetPlatformFromContext(c.Request.Context())
				require.Equal(t, service.PlatformDeepseek, platform)
				require.False(t, f.advance(c, service.ErrNoAvailableAccounts, nil), "backup is attempted at most once")
			}
		})
	}
}

type compositeFallbackHTTPUpstream struct {
	service.HTTPUpstream
	mu          sync.Mutex
	ids         []int64
	models      []string
	status      int
	transport   bool
	readError   bool
	streamBreak bool
}

type compositeFailingReader struct{}

func (compositeFailingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func (u *compositeFallbackHTTPUpstream) Do(req *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	u.mu.Lock()
	u.ids = append(u.ids, id)
	u.models = append(u.models, gjson.GetBytes(body, "model").String())
	u.mu.Unlock()
	if id == 801 {
		if u.transport {
			return nil, errors.New("connection reset by peer")
		}
		if u.readError {
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(compositeFailingReader{})}, nil
		}
		if u.streamBreak {
			data := "data: {\"id\":\"chatcmpl_primary\",\"object\":\"chat.completion.chunk\",\"model\":\"deepseek-v4.1-flash\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"primary semantic output\"}}]}\n\n"
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(io.MultiReader(bytes.NewBufferString(data), compositeFailingReader{}))}, nil
		}
		return &http.Response{StatusCode: u.status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewBufferString(`{"error":{"message":"upstream unavailable"}}`))}, nil
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewBufferString(`{"id":"chatcmpl_backup","object":"chat.completion","model":"deepseek-flash","choices":[{"index":0,"message":{"role":"assistant","content":"backup ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))}, nil
}

func TestCompositeHTTPFallbackActualHandlers(t *testing.T) {
	for _, endpoint := range []string{"chat/completions", "responses", "messages"} {
		for _, mode := range []string{"empty", "429", "network", "read error", "stream break", "400"} {
			t.Run(endpoint+"/"+mode, func(t *testing.T) {
				h, repo, _, _, cleanup := newGrokCredentialFailoverHandler(t, "unused")
				defer cleanup()
				upstream := &compositeFallbackHTTPUpstream{status: 429, transport: mode == "network", readError: mode == "read error", streamBreak: mode == "stream break"}
				if mode == "400" {
					upstream.status = 400
				}
				logs := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 8)}
				h.gatewayService = service.NewOpenAIGatewayService(
					repo, logs, nil, nil, nil, nil, nil, h.cfg, nil, nil,
					service.NewBillingService(h.cfg, nil), nil, h.billingCacheService, upstream,
					&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
				)
				repo.accounts = []service.Account{
					{ID: 801, Platform: service.PlatformOpenCodeGo, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: mode != "empty", Concurrency: 1, Credentials: map[string]any{"api_key": "test", "base_url": "https://example.com/v1", "model_mapping": map[string]any{"deepseek-v4.1-flash": "deepseek-v4.1-flash"}}},
					{ID: 802, Platform: service.PlatformDeepseek, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"api_key": "test", "base_url": "https://example.com/v1", "model_mapping": map[string]any{"deepseek-flash": "deepseek-flash"}}},
				}
				groupID := int64(901)
				key := &service.APIKey{ID: 902, GroupID: &groupID, User: &service.User{ID: 903, Status: service.StatusActive}, Group: &service.Group{ID: groupID, Platform: service.PlatformComposite, Status: service.StatusActive}}
				router := gin.New()
				var quotaPlatform string
				router.Use(func(c *gin.Context) {
					c.Set(string(middleware.ContextKeyAPIKey), key)
					c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: key.User.ID, Concurrency: 1})
					c.Request = c.Request.WithContext(service.WithCompositeRouteDecision(c.Request.Context(), compositeHTTPTestDecision()))
					c.Next()
					quotaPlatform = service.QuotaPlatform(c.Request.Context(), key)
				})
				router.POST("/v1/chat/completions", h.ChatCompletions)
				router.POST("/v1/responses", h.Responses)
				router.POST("/v1/messages", h.Messages)
				payload := `{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":"hello"}],"max_tokens":10,"stream":false}`
				if endpoint == "responses" {
					payload = `{"model":"deepseek-v4.1-flash","input":"hello","stream":false}`
				}
				if mode == "stream break" {
					payload = string(bytes.ReplaceAll([]byte(payload), []byte(`"stream":false`), []byte(`"stream":true`)))
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/"+endpoint, bytes.NewBufferString(payload)))
				upstream.mu.Lock()
				ids, models := append([]int64(nil), upstream.ids...), append([]string(nil), upstream.models...)
				upstream.mu.Unlock()
				if mode == "stream break" {
					require.Contains(t, rec.Body.String(), "primary semantic output")
					require.NotContains(t, ids, int64(802), "semantic output must prevent cross-provider replay")
					return
				}
				if mode == "400" {
					require.NotEqual(t, http.StatusOK, rec.Code, rec.Body.String())
					require.NotContains(t, ids, int64(802))
					return
				}
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.Contains(t, rec.Body.String(), "backup ok")
				require.Equal(t, "public-ds-flash", gjson.Get(rec.Body.String(), "model").String())
				require.Contains(t, ids, int64(802))
				require.Equal(t, "deepseek-flash", models[len(models)-1])
				require.Equal(t, service.PlatformDeepseek, quotaPlatform)
				require.LessOrEqual(t, len(ids), 5)
				require.Len(t, logs.created, 1, "failed primary attempts must not create usage rows")
				usage := <-logs.created
				require.Equal(t, int64(802), usage.AccountID)
				require.Equal(t, "public-ds-flash", usage.RequestedModel)
				require.NotNil(t, usage.UpstreamModel)
				require.Equal(t, "deepseek-flash", *usage.UpstreamModel)
			})
		}
	}
}
