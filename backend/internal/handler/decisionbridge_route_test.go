//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type decisionBridgeGroupRepo struct{ service.GroupRepository }

func (decisionBridgeGroupRepo) GetByIDLite(_ context.Context, id int64) (*service.Group, error) {
	return &service.Group{ID: id, Platform: service.PlatformTypeSafe, Status: service.StatusActive, Hydrated: true}, nil
}

type decisionBridgeKeyRepo struct {
	service.APIKeyRepository
	key *service.APIKey
}

func (r decisionBridgeKeyRepo) GetByKeyForAuth(_ context.Context, key string) (*service.APIKey, error) {
	if key != "fixture-token" {
		return nil, service.ErrAPIKeyNotFound
	}
	return r.key, nil
}
func (r decisionBridgeKeyRepo) UpdateLastUsed(_ context.Context, _ int64, _ time.Time) error {
	return nil
}
func bridgeAuth(cfg *config.Config, platform string, id int64) gin.HandlerFunc {
	group := &service.Group{ID: id, Platform: platform, Status: service.StatusActive, Hydrated: true}
	user := &service.User{ID: 100, Role: service.RoleUser, Status: service.StatusActive, Balance: 10}
	key := &service.APIKey{ID: 99, UserID: user.ID, Key: "fixture-token", Status: service.StatusActive, GroupID: &id, Group: group, User: user}
	apiKeyService := service.NewAPIKeyService(decisionBridgeKeyRepo{key: key}, nil, nil, nil, nil, nil, cfg)
	return gin.HandlerFunc(middleware.NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg))
}

func TestDecisionBridgeAuthenticatedRouteOneCase(t *testing.T) {
	upstreamReply := astra200()
	upstreamReply.Body = ioBody(`{"model":"gpt-6-luna","answers":[{"type":"predicate","probability":0.9}],"usage":{"input_tokens":4,"output_tokens":0}}`)
	upstream := newAstraProCapturedUpstream(upstreamReply)
	h := newOpenAIResponsesFailoverTestHandler(t, upstream, decisionsAccount(1, "openai"))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(bridgeAuth(&config.Config{RunMode: config.RunModeSimple}, service.PlatformOpenAI, 3132))
	router.POST("/v1/systemone", h.SystemOneViaDecisions)
	req := httptest.NewRequest(http.MethodPost, "/v1/systemone", strings.NewReader(`{"model":"jev-latest","state":"hello","questions":{"q":{"type":"noul","instructions":"Is this valid?"}}}`))
	req.Header.Set("Authorization", "Bearer fixture-token")
	req.Header.Set("Content-Type", "application/json")
	denied := httptest.NewRecorder()
	unauthorized := req.Clone(req.Context())
	unauthorized.Header = http.Header{}
	router.ServeHTTP(denied, unauthorized)
	require.Equal(t, 401, denied.Code)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	urls, ids, bodies := upstream.snapshot()
	require.Equal(t, 200, rec.Code, rec.Body.String())
	require.Equal(t, []int64{1}, ids)
	require.Len(t, urls, 1)
	require.Contains(t, urls[0], "/v1/decisions")
	require.Equal(t, "gpt-6-luna", gjson.GetBytes(bodies[0], "model").String())
	require.Equal(t, "hello", gjson.GetBytes(bodies[0], "input").String())
	require.Equal(t, "jev-latest", gjson.GetBytes(rec.Body.Bytes(), "model").String())
	require.Equal(t, 0.9, gjson.GetBytes(rec.Body.Bytes(), "answers.q.noul").Float())
}

func TestDecisionBridgeUsageSnapshotPreservesClientAndUpstream(t *testing.T) {
	c, _ := newAstraProFailoverContext(t, `{"model":"jev-latest","state":"hello","questions":{"q":{"type":"noul","instructions":"x"}}}`)
	c.Request.URL.Path = "/v1/systemone"
	key, ok := middleware.GetAPIKeyFromContext(c)
	require.True(t, ok)
	result := &service.OpenAIForwardResult{Model: "jev-latest", UpstreamModel: "gpt-6-luna", BillingModel: "gpt-6-luna", UpstreamEndpoint: "/v1/decisions"}
	mapping := service.ChannelMappingResult{BillingModelSource: service.BillingModelSourceUpstream}
	input := decisionsUsageSnapshot(c, key, &service.Account{Platform: service.PlatformOpenAI}, nil, []byte(`{"model":"jev-latest"}`), result, time.Now(), mapping)
	require.Equal(t, "/v1/systemone", input.InboundEndpoint)
	require.Equal(t, "/v1/decisions", input.UpstreamEndpoint)
	require.Equal(t, "jev-latest", input.OriginalModel)
	require.Equal(t, service.BillingModelSourceUpstream, input.BillingModelSource)
	require.NotEmpty(t, input.RequestPayloadHash)
}

func TestDecisionBridgeUpstreamErrorKeepsSystemOneEnvelope(t *testing.T) {
	reply := astra403()
	reply.StatusCode = http.StatusBadRequest
	upstream := newAstraProCapturedUpstream(reply)
	h := newOpenAIResponsesFailoverTestHandler(t, upstream, decisionsAccount(1, "openai"))
	c, rec := newAstraProFailoverContext(t, `{"model":"jev-latest","state":"hello","questions":{"q":{"type":"noul","instructions":"x"}}}`)
	c.Request.URL.Path = "/v1/systemone"
	h.SystemOneViaDecisions(c)
	_, ids, _ := upstream.snapshot()
	require.Equal(t, []int64{1}, ids)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "error", gjson.GetBytes(rec.Body.Bytes(), "type").String())
	require.Equal(t, "invalid_request_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
}

func TestDecisionBridgeRejectsUnrepresentableUpstreamResponse(t *testing.T) {
	reply := astra200()
	reply.Body = ioBody(`{"model":"gpt-6-luna","answers":[{"type":"refusal"}],"usage":{"input_tokens":4,"output_tokens":0}}`)
	upstream := newAstraProCapturedUpstream(reply)
	h := newOpenAIResponsesFailoverTestHandler(t, upstream, decisionsAccount(1, "openai"))
	c, rec := newAstraProFailoverContext(t, `{"model":"jev-latest","state":"hello","questions":{"q":{"type":"noul","instructions":"x"}}}`)
	c.Request.URL.Path = "/v1/systemone"
	h.SystemOneViaDecisions(c)
	_, ids, _ := upstream.snapshot()
	require.Equal(t, []int64{1}, ids)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.NotContains(t, rec.Body.String(), `"type":"refusal"`)
}

func TestDecisionBridgeSystemOneIngressFailover(t *testing.T) {
	failed := astra403()
	failed.StatusCode = http.StatusServiceUnavailable
	winning := astra200()
	winning.Body = ioBody(`{"model":"gpt-6-luna","answers":[{"type":"predicate","probability":0.7}],"usage":{"input_tokens":7,"output_tokens":0}}`)
	upstream := newAstraProCapturedUpstream(failed, winning)
	h := newOpenAIResponsesFailoverTestHandler(t, upstream, decisionsAccount(1, "openai"), decisionsAccount(2, "openai"))
	c, rec := newAstraProFailoverContext(t, `{"model":"jev-latest","state":"hello","questions":{"q":{"type":"noul","instructions":"Is this valid?"}}}`)
	c.Request.URL.Path = "/v1/systemone"
	h.SystemOneViaDecisions(c)
	urls, ids, bodies := upstream.snapshot()
	require.Equal(t, 200, rec.Code, rec.Body.String())
	require.Equal(t, []int64{1, 2}, ids)
	for i := range urls {
		require.Contains(t, urls[i], "/v1/decisions")
		require.Equal(t, "hello", gjson.GetBytes(bodies[i], "input").String())
	}
	require.Equal(t, 0.7, gjson.GetBytes(rec.Body.Bytes(), "answers.q.noul").Float())
}

func TestDecisionBridgeDecisionsIngressToSystemOneRoute(t *testing.T) {
	first := astra403()
	first.StatusCode = http.StatusServiceUnavailable
	upstreamReply := astra200()
	upstreamReply.Body = ioBody(`{"model":"jev-1.13.0","answers":{"q0":{"type":"noul","noul":0.8}},"usage":{"input_tokens":5,"output_tokens":0}}`)
	upstream := newAstraProCapturedUpstream(first, upstreamReply)
	cfg := &config.Config{RunMode: config.RunModeSimple, Gateway: config.GatewayConfig{MaxBodySize: 1 << 20}}
	account := service.Account{ID: 8, Name: "typesafe-fixture", Platform: service.PlatformTypeSafe, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Credentials: map[string]any{"api_key": "fixture-upstream", "base_url": "https://api.typesafe.ai"}}
	other := account
	other.ID = 9
	other.Priority = 1
	accounts := openAIImagesFailoverAccountRepo{accounts: []service.Account{account, other}}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	svc := service.NewGatewayService(accounts, decisionBridgeGroupRepo{}, nil, nil, nil, nil, nil, nil, cfg, nil, service.NewConcurrencyService(nil), nil, nil, billing, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewGatewayHandler(svc, nil, nil, nil, nil, service.NewConcurrencyService(nil), billing, nil, nil, nil, nil, nil, nil, cfg, nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(bridgeAuth(cfg, service.PlatformTypeSafe, 3133))
	router.POST("/v1/decisions", h.DecisionsViaSystemOne)
	req := httptest.NewRequest(http.MethodPost, "/v1/decisions", strings.NewReader(decisionsBody))
	req.Header.Set("Authorization", "Bearer fixture-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	urls, ids, bodies := upstream.snapshot()
	require.Equal(t, 200, rec.Code, rec.Body.String())
	require.Equal(t, []int64{8, 9}, ids)
	require.Len(t, urls, 2)
	for i := range urls {
		require.Contains(t, urls[i], "/v1/systemone")
		require.Equal(t, "jev-latest", gjson.GetBytes(bodies[i], "model").String())
		require.Equal(t, "evidence", gjson.GetBytes(bodies[i], "state").String())
	}
	require.Equal(t, "gpt-6-luna", gjson.GetBytes(rec.Body.Bytes(), "model").String())
	require.Equal(t, 0.8, gjson.GetBytes(rec.Body.Bytes(), "answers.0.probability").Float())
}
