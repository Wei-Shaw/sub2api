//go:build unit

package routes

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type bridgeRouteKeyRepo struct {
	service.APIKeyRepository
	key *service.APIKey
}

func (r bridgeRouteKeyRepo) GetByKeyForAuth(_ context.Context, key string) (*service.APIKey, error) {
	if key != "fixture-token" {
		return nil, service.ErrAPIKeyNotFound
	}
	return r.key, nil
}
func (bridgeRouteKeyRepo) UpdateLastUsed(context.Context, int64, time.Time) error { return nil }

type bridgeRouteUpstream struct {
	service.HTTPUpstream
	url   string
	body  string
	reply string
	calls int
}

func (u *bridgeRouteUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls++
	u.url = req.URL.String()
	b, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	u.body = string(b)
	reply := u.reply
	if reply == "" {
		reply = `{"model":"gpt-6-luna","answers":[{"type":"predicate","probability":0.9}],"usage":{"input_tokens":4,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":0},"output_tokens":0,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":4}}`
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(reply))}, nil
}

type bridgeRouteGroupRepo struct{ service.GroupRepository }

func (bridgeRouteGroupRepo) GetByIDLite(_ context.Context, id int64) (*service.Group, error) {
	return &service.Group{ID: id, Platform: service.PlatformTypeSafe, Status: service.StatusActive, Hydrated: true}, nil
}

func TestDecisionBridgeRegisteredAuthenticatedDecisionsRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{RunMode: config.RunModeSimple, Gateway: config.GatewayConfig{MaxBodySize: 1 << 20, TextMaxBodySize: 1 << 20}}
	groupID := int64(3133)
	group := &service.Group{ID: groupID, Platform: service.PlatformTypeSafe, Status: service.StatusActive, Hydrated: true}
	user := &service.User{ID: 100, Role: service.RoleUser, Status: service.StatusActive, Balance: 10}
	key := &service.APIKey{ID: 99, UserID: user.ID, Key: "fixture-token", Status: service.StatusActive, GroupID: &groupID, Group: group, User: user}
	keys := service.NewAPIKeyService(bridgeRouteKeyRepo{key: key}, nil, nil, nil, nil, nil, cfg)
	accounts := compatibleImagesAccounts{accounts: []service.Account{{ID: 8, Name: "fixture", Platform: service.PlatformTypeSafe, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Credentials: map[string]any{"api_key": "fixture-upstream", "base_url": "https://api.typesafe.ai"}}}}
	upstream := &bridgeRouteUpstream{reply: `{"model":"jev-1.13.0","answers":{"q0":{"type":"noul","noul":0.8}},"usage":{"input_tokens":5,"output_tokens":0}}`}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	gateway := service.NewGatewayService(accounts, bridgeRouteGroupRepo{}, nil, nil, nil, nil, nil, nil, cfg, nil, service.NewConcurrencyService(nil), nil, nil, billing, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	typesafe := handler.NewGatewayHandler(gateway, nil, nil, nil, nil, service.NewConcurrencyService(nil), billing, nil, keys, nil, nil, nil, nil, cfg, nil)
	router := gin.New()
	RegisterGatewayRoutes(router, &handler.Handlers{Gateway: typesafe, OpenAIGateway: &handler.OpenAIGatewayHandler{}, AsyncImage: handler.NewAsyncImageHandler(nil, nil)}, middleware.NewAPIKeyAuthMiddleware(keys, nil, cfg), keys, nil, nil, nil, nil, cfg)
	req := httptest.NewRequest(http.MethodPost, "/v1/decisions", strings.NewReader(`{"model":"gpt-6-luna","input":"evidence","questions":[{"type":"predicate","instructions":"valid?"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer fixture-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, 1, upstream.calls)
	require.Contains(t, upstream.url, "/v1/systemone")
	require.Equal(t, "evidence", gjson.Get(upstream.body, "state").String())
	require.Equal(t, "jev-latest", gjson.Get(upstream.body, "model").String())
	require.Equal(t, "gpt-6-luna", gjson.GetBytes(rec.Body.Bytes(), "model").String())
	require.Equal(t, 0.8, gjson.GetBytes(rec.Body.Bytes(), "answers.0.probability").Float())
}

// This uses the registered production route, real API-key middleware, scheduler,
// forwarding service and both converters. Only the upstream transport is controlled.
func TestDecisionBridgeRegisteredAuthenticatedSuccessRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{RunMode: config.RunModeSimple, Gateway: config.GatewayConfig{MaxBodySize: 1 << 20, TextMaxBodySize: 1 << 20}}
	groupID := int64(3132)
	group := &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive, Hydrated: true}
	user := &service.User{ID: 100, Role: service.RoleUser, Status: service.StatusActive, Balance: 10}
	key := &service.APIKey{ID: 99, UserID: user.ID, Key: "fixture-token", Status: service.StatusActive, GroupID: &groupID, Group: group, User: user}
	keys := service.NewAPIKeyService(bridgeRouteKeyRepo{key: key}, nil, nil, nil, nil, nil, cfg)
	accounts := compatibleImagesAccounts{accounts: []service.Account{{ID: 1, Name: "fixture", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Credentials: map[string]any{"api_key": "fixture-upstream", "openai_decisions_protocol": "openai"}}}}
	upstream := &bridgeRouteUpstream{}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	gateway := service.NewOpenAIGatewayService(accounts, nil, nil, nil, nil, nil, nil, cfg, nil, nil, service.NewBillingService(cfg, nil), nil, billing, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
	openai := handler.NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(nil), billing, keys, nil, nil, nil, nil, cfg)
	router := gin.New()
	RegisterGatewayRoutes(router, &handler.Handlers{Gateway: &handler.GatewayHandler{}, OpenAIGateway: openai, AsyncImage: handler.NewAsyncImageHandler(nil, nil)}, middleware.NewAPIKeyAuthMiddleware(keys, nil, cfg), keys, nil, nil, nil, nil, cfg)
	upstream.reply = `{"model":"gpt-6-luna","answers":[{"type":"score","score":0.2,"confidence":0.8,"probabilities":[{"value":0.0,"label":"low","probability":0.8},{"value":1.0,"label":"high","probability":0.2}]}],"usage":{"input_tokens":4,"output_tokens":0,"total_tokens":4}}`
	body := `{"model":"jev-latest","state":"hello","questions":{"q":{"type":"score","instructions":"How bad?","criteria":["low","high"]}}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/systemone", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer fixture-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, 1, upstream.calls)
	require.Contains(t, upstream.url, "/v1/decisions")
	require.Equal(t, "hello", gjson.Get(upstream.body, "input").String())
	require.Equal(t, "gpt-6-luna", gjson.Get(upstream.body, "model").String())
	require.Equal(t, "jev-latest", gjson.GetBytes(rec.Body.Bytes(), "model").String())
	require.Equal(t, 0.2, gjson.GetBytes(rec.Body.Bytes(), "answers.q.score").Float())
	require.Equal(t, 0.8, gjson.GetBytes(rec.Body.Bytes(), "answers.q.probabilities.0").Float())
	require.Equal(t, 0.2, gjson.GetBytes(rec.Body.Bytes(), "answers.q.probabilities.1").Float())
}
