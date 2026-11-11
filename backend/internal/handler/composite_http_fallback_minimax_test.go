//go:build unit

package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type minimaxCompositeUpstream struct {
	service.HTTPUpstream
	ids       []int64
	models    []string
	readError bool
}

func (u *minimaxCompositeUpstream) Do(req *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	u.ids = append(u.ids, id)
	u.models = append(u.models, gjson.GetBytes(body, "model").String())
	if id == 801 {
		if u.readError {
			contentType := "application/json"
			if gjson.GetBytes(body, "stream").Bool() {
				contentType = "text/event-stream"
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(compositeFailingReader{})}, nil
		}
		return &http.Response{StatusCode: 429, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"overloaded"}}`))}, nil
	}
	payload := `{"id":"msg_backup","type":"message","role":"assistant","model":"minimax-m2.7","content":[{"type":"text","text":"backup ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
	contentType := "application/json"
	if gjson.GetBytes(body, "stream").Bool() {
		contentType = "text/event-stream"
		payload = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_backup\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"minimax-m2.7\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n" +
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"backup ok\"}}\n\n" +
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\n" +
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(payload))}, nil
}

func TestCompositeHTTPFallbackMiniMaxNativeHandlers(t *testing.T) {
	for _, tc := range []struct {
		name, endpoint    string
		stream, readError bool
	}{
		{name: "chat", endpoint: "chat/completions"},
		{name: "responses", endpoint: "responses"},
		{name: "messages", endpoint: "messages"},
		{name: "messages stream", endpoint: "messages", stream: true},
		{name: "messages read failure", endpoint: "messages", readError: true},
		{name: "messages stream read failure", endpoint: "messages", stream: true, readError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, repo, _, _, cleanup := newGrokCredentialFailoverHandler(t, "unused")
			defer cleanup()
			upstream := &minimaxCompositeUpstream{readError: tc.readError}
			logs := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 8)}
			h.gatewayService = service.NewOpenAIGatewayService(repo, logs, nil, nil, nil, nil, nil, h.cfg, nil, nil, service.NewBillingService(h.cfg, nil), nil, h.billingCacheService, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
			repo.accounts = []service.Account{
				{ID: 801, Platform: service.PlatformMiniMax, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"api_key": "test", "base_url": "https://example.com", "api_protocol": service.APIProtocolAnthropic, "model_mapping": map[string]any{"MiniMax-M2.7": "MiniMax-M2.7"}}},
				{ID: 802, Platform: service.PlatformOpenCodeGo, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"api_key": "test", "base_url": "https://example.com/v1", "model_mapping": map[string]any{"minimax-m2.7": "minimax-m2.7"}}},
			}
			groupID := int64(901)
			key := &service.APIKey{ID: 902, GroupID: &groupID, User: &service.User{ID: 903, Status: service.StatusActive}, Group: &service.Group{ID: groupID, Platform: service.PlatformComposite, Status: service.StatusActive}}
			decision := service.CompositeRouteDecision{Matched: true, Source: service.CompositeRouteSourceExplicit, PublicModel: "public-minimax", TargetPlatform: service.PlatformMiniMax, UpstreamModel: "MiniMax-M2.7", Endpoint: service.CompositeRouteEndpointAny, FallbackTargets: []service.CompositeFallbackTarget{{TargetPlatform: service.PlatformOpenCodeGo, UpstreamModel: "minimax-m2.7"}}}
			router := gin.New()
			var quota string
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyAPIKey), key)
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: key.User.ID, Concurrency: 1})
				c.Request = c.Request.WithContext(service.WithCompositeRouteDecision(c.Request.Context(), decision))
				c.Next()
				quota = service.QuotaPlatform(c.Request.Context(), key)
			})
			router.POST("/v1/chat/completions", h.ChatCompletions)
			router.POST("/v1/responses", h.Responses)
			router.POST("/v1/messages", h.Messages)
			payload := `{"model":"MiniMax-M2.7","messages":[{"role":"user","content":"hello"}],"max_tokens":10,"stream":false}`
			if tc.endpoint == "responses" {
				payload = `{"model":"MiniMax-M2.7","input":"hello","stream":false}`
			}
			if tc.stream {
				payload = strings.Replace(payload, `"stream":false`, `"stream":true`, 1)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/"+tc.endpoint, bytes.NewBufferString(payload)))
			require.Equal(t, 200, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), "backup ok")
			if tc.stream {
				require.Contains(t, rec.Body.String(), `"model":"public-minimax"`)
			} else {
				require.Equal(t, "public-minimax", gjson.Get(rec.Body.String(), "model").String())
			}
			require.Equal(t, []int64{801, 802}, upstream.ids)
			require.Equal(t, []string{"MiniMax-M2.7", "minimax-m2.7"}, upstream.models)
			require.Equal(t, service.PlatformOpenCodeGo, quota)
			require.Len(t, logs.created, 1)
			usage := <-logs.created
			require.Equal(t, int64(802), usage.AccountID)
			require.Equal(t, "public-minimax", usage.RequestedModel)
		})
	}
}

type compositeInflightEstimator struct{}

func (*compositeInflightEstimator) EstimateInflightReservation(ctx context.Context, _ *service.APIKey, req service.InflightEstimateRequest) (float64, bool) {
	platform, _ := service.ResolvedTargetPlatformFromContext(ctx)
	if platform == service.PlatformDeepseek && req.Model == "deepseek-flash" {
		return 3, true
	}
	return 1, true
}

func TestCompositeFallbackInflightUsesMaximumTargetPrice(t *testing.T) {
	ctx := service.WithCompositeRouteDecision(context.Background(), compositeHTTPTestDecision())
	amount, priced := estimateCompositeInflightReservation(ctx, &compositeInflightEstimator{}, nil, service.InflightEstimateRequest{Kind: service.InflightEstimateToken, Model: "deepseek-v4.1-flash"})
	require.True(t, priced)
	require.Equal(t, float64(3), amount)
	base := usageRecordContext(ctx, context.Background())
	decision, ok := service.CompositeFallbackDecisionFromContext(base)
	require.True(t, ok)
	require.Equal(t, service.PlatformOpenCodeGo, decision.TargetPlatform)
}
