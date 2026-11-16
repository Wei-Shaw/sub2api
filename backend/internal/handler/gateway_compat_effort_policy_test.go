//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type compatEffortPolicyUpstream struct {
	service.HTTPUpstream
	bodies [][]byte
}

func (u *compatEffortPolicyUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	u.bodies = append(u.bodies, body)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(compatStreamUsagePartialStream + compatStreamUsageCompletion)),
	}, nil
}

func TestGatewayCompatibilityHandlersReasoningEffortPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const model = "claude-sonnet-4.5"
	for _, endpoint := range []string{"chat/completions", "responses"} {
		for _, platform := range []string{service.PlatformAnthropic, service.PlatformComposite} {
			for _, stream := range []bool{false, true} {
				for _, tc := range []struct {
					name      string
					effort    string
					ceiling   string
					overLimit string
					mappings  []service.ReasoningEffortMapping
					want      string
					denied    bool
				}{
					{name: "ceiling", effort: "high", ceiling: "medium", want: "medium"},
					{name: "mapping", effort: "high", mappings: []service.ReasoningEffortMapping{{From: "high", To: "low"}}, want: "low"},
					{name: "mapping then ceiling", effort: "low", ceiling: "medium", mappings: []service.ReasoningEffortMapping{{From: "low", To: "high"}}, want: "medium"},
					{name: "model mapping", effort: "high", mappings: []service.ReasoningEffortMapping{{From: "high", To: "low", MatchType: "exact", Model: model}}, want: "low"},
					{name: "other model mapping", effort: "high", mappings: []service.ReasoningEffortMapping{{From: "high", To: "low", MatchType: "exact", Model: "other-model"}}, want: "high"},
					{name: "deny ceiling", effort: "high", ceiling: "medium", overLimit: service.ReasoningEffortOverLimitDeny, denied: true},
					{name: "deny mapping", effort: "high", mappings: []service.ReasoningEffortMapping{{From: "high", To: "deny"}}, denied: true},
					{name: "within ceiling", effort: "low", ceiling: "medium", want: "low"},
					{name: "no policy", effort: "high", want: "high"},
					{name: "omitted", ceiling: "medium"},
				} {
					t.Run(fmt.Sprintf("%s/%s/stream=%t/%s", endpoint, platform, stream, tc.name), func(t *testing.T) {
						cfg := &config.Config{}
						groupID := int64(9400)
						group := &service.Group{
							ID: groupID, Hydrated: true, Platform: platform, Status: service.StatusActive, RateMultiplier: 1,
							MaxReasoningEffort: tc.ceiling, MaxReasoningEffortOverLimit: tc.overLimit, ReasoningEffortMappings: tc.mappings,
						}
						account := &service.Account{
							ID: 9401, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
							Status: service.StatusActive, Schedulable: true, Concurrency: 1,
							Credentials:   map[string]any{"api_key": "test-key"},
							AccountGroups: []service.AccountGroup{{AccountID: 9401, GroupID: groupID}},
						}
						cache := &countingGatewaySchedulerCache{fakeSchedulerCache: &fakeSchedulerCache{accounts: []*service.Account{account}}}
						snapshot := service.NewSchedulerSnapshotService(cache, nil, nil, nil, nil)
						upstream := &compatEffortPolicyUpstream{}
						usageRepo := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 1)}
						billingRepo := &compatStreamUsageBillingRepo{}
						billingCache := service.NewBillingCacheService(newHandlerInflightCache(100), nil, nil, nil, nil, nil, cfg, nil)
						t.Cleanup(billingCache.Stop)
						gateway := service.NewGatewayService(
							nil, &fakeGroupRepo{group: group}, usageRepo, billingRepo, nil, nil, nil, nil, cfg,
							snapshot, nil, service.NewBillingService(cfg, nil), nil, nil, nil, upstream,
							&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
						)
						h := &GatewayHandler{
							gatewayService: gateway, billingCacheService: billingCache, cfg: cfg, maxAccountSwitches: 1,
							concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(&fakeConcurrencyCache{}), SSEPingFormatClaude, 0),
						}

						request := map[string]any{"model": model, "stream": stream}
						if endpoint == "chat/completions" {
							request["messages"] = []map[string]string{{"role": "user", "content": "hello"}}
							if tc.effort != "" {
								request["reasoning_effort"] = tc.effort
							}
						} else {
							request["input"] = "hello"
							if tc.effort != "" {
								request["reasoning"] = map[string]string{"effort": tc.effort}
							}
						}
						body, err := json.Marshal(request)
						require.NoError(t, err)
						recorder := httptest.NewRecorder()
						c, _ := gin.CreateTestContext(recorder)
						ctx := context.WithValue(context.Background(), ctxkey.Group, group)
						c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+endpoint, bytes.NewReader(body)).WithContext(ctx)
						c.Request.Header.Set("Content-Type", "application/json")
						apiKey := &service.APIKey{
							ID: 9402, UserID: 9403, GroupID: &groupID, Group: group,
							User: &service.User{ID: 9403, Balance: 100},
						}
						c.Set(string(middleware.ContextKeyAPIKey), apiKey)
						c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 9403})
						if endpoint == "chat/completions" {
							h.ChatCompletions(c)
						} else {
							h.Responses(c)
						}

						if tc.denied {
							require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
							errorField := "error.type"
							if endpoint == "responses" {
								errorField = "error.code"
							}
							require.Equal(t, "permission_error", gjson.Get(recorder.Body.String(), errorField).String())
							require.Empty(t, upstream.bodies)
							require.Zero(t, cache.snapshotCalls.Load(), "local policy denial precedes account selection")
							require.Empty(t, usageRepo.created)
							require.Empty(t, billingRepo.commands)
							return
						}
						// Check the final Anthropic request, beyond handler parsing and both protocol conversions.
						require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
						require.Len(t, upstream.bodies, 1)
						require.Equal(t, tc.want, gjson.GetBytes(upstream.bodies[0], "output_config.effort").String())
						require.Equal(t, model, gjson.GetBytes(upstream.bodies[0], "model").String())
						require.Contains(t, recorder.Body.String(), "partial")
						require.Len(t, usageRepo.created, 1)
						usage := <-usageRepo.created
						if tc.effort == "" {
							require.Nil(t, usage.RequestedReasoningEffort)
						} else {
							require.Equal(t, &tc.effort, usage.RequestedReasoningEffort)
							require.Equal(t, &tc.want, usage.ReasoningEffort)
						}
					})
				}
			}
		}
	}
}

type antigravityCompatPolicyUpstream struct {
	service.HTTPUpstream
	bodies [][]byte
}

func (u *antigravityCompatPolicyUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	u.bodies = append(u.bodies, body)
	sse := `data: {"response":{"responseId":"resp_ag","candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":3}}}` + "\n\n"
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}, nil
}

func TestGatewayCompatibilityHandlersAntigravityEffortPolicySchedulingAndAttribution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetAt := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)

	for _, endpoint := range []string{"chat/completions", "responses"} {
		t.Run(endpoint+"/policy_rewrite_high_to_low_uses_post_policy_cooldown_and_preserves_requested_effort", func(t *testing.T) {
			cfg := &config.Config{}
			groupID := int64(9410)
			group := &service.Group{
				ID: groupID, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive, RateMultiplier: 1,
				ReasoningEffortMappings: []service.ReasoningEffortMapping{{From: "high", To: "low"}},
			}
			// -high 处于冷却，但策略改写后实际应命中健康的 -low 变体。
			agAccount := &service.Account{
				ID: 9411, Name: "ag-mixed", Platform: service.PlatformAntigravity, Type: service.AccountTypeOAuth,
				Status: service.StatusActive, Schedulable: true, Concurrency: 1,
				Credentials: map[string]any{"access_token": "tok", "project_id": "proj"},
				Extra: map[string]any{
					"mixed_scheduling": true,
					"model_rate_limits": map[string]any{
						"claude-opus-5-5-high": map[string]any{"rate_limit_reset_at": resetAt},
					},
				},
				AccountGroups: []service.AccountGroup{{AccountID: 9411, GroupID: groupID}},
			}
			cache := &countingGatewaySchedulerCache{fakeSchedulerCache: &fakeSchedulerCache{accounts: []*service.Account{agAccount}}}
			snapshot := service.NewSchedulerSnapshotService(cache, nil, nil, nil, nil)
			upstream := &antigravityCompatPolicyUpstream{}
			usageRepo := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 1)}
			billingRepo := &compatStreamUsageBillingRepo{}
			billingCache := service.NewBillingCacheService(newHandlerInflightCache(100), nil, nil, nil, nil, nil, cfg, nil)
			t.Cleanup(billingCache.Stop)
			gateway := service.NewGatewayService(
				nil, &fakeGroupRepo{group: group}, usageRepo, billingRepo, nil, nil, nil, nil, cfg,
				snapshot, nil, service.NewBillingService(cfg, nil), nil, nil, nil, upstream,
				&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			)
			agGateway := service.NewAntigravityGatewayService(
				nil, nil, snapshot, service.NewAntigravityTokenProvider(nil, nil, nil), nil, upstream, service.NewSettingService(&handlerMCPSettings{}, cfg), nil,
			)
			h := &GatewayHandler{
				gatewayService: gateway, antigravityGatewayService: agGateway,
				billingCacheService: billingCache, cfg: cfg, maxAccountSwitches: 1,
				concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(&fakeConcurrencyCache{}), SSEPingFormatClaude, 0),
			}

			reqMap := map[string]any{"model": "claude-opus-5-5", "stream": false}
			if endpoint == "chat/completions" {
				reqMap["messages"] = []map[string]string{{"role": "user", "content": "hello"}}
				reqMap["reasoning_effort"] = "high"
			} else {
				reqMap["input"] = "hello"
				reqMap["reasoning"] = map[string]string{"effort": "high"}
			}
			body, err := json.Marshal(reqMap)
			require.NoError(t, err)

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			ctx := context.WithValue(context.Background(), ctxkey.Group, group)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+endpoint, bytes.NewReader(body)).WithContext(ctx)
			c.Request.Header.Set("Content-Type", "application/json")
			apiKey := &service.APIKey{
				ID: 9412, UserID: 9413, GroupID: &groupID, Group: group,
				User: &service.User{ID: 9413, Balance: 100},
			}
			c.Set(string(middleware.ContextKeyAPIKey), apiKey)
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 9413})
			if endpoint == "chat/completions" {
				h.ChatCompletions(c)
			} else {
				h.Responses(c)
			}

			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			require.Len(t, upstream.bodies, 1)
			require.Equal(t, "claude-opus-5-5-low", gjson.GetBytes(upstream.bodies[0], "model").String())
			require.Len(t, usageRepo.created, 1)
			usage := <-usageRepo.created
			wantRequested, wantEffective := "high", "low"
			require.Equal(t, &wantRequested, usage.RequestedReasoningEffort)
			require.Equal(t, &wantEffective, usage.ReasoningEffort)
		})

		t.Run(endpoint+"/policy_rewrite_high_to_low_skips_cooled_low_variant", func(t *testing.T) {
			cfg := &config.Config{}
			groupID := int64(9420)
			group := &service.Group{
				ID: groupID, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive, RateMultiplier: 1,
				ReasoningEffortMappings: []service.ReasoningEffortMapping{{From: "high", To: "low"}},
			}
			// -low 处于冷却、-high 健康时，high->low 改写后的请求应在调度阶段被拦截而非误放行。
			agAccount := &service.Account{
				ID: 9421, Name: "ag-low-cooled", Platform: service.PlatformAntigravity, Type: service.AccountTypeOAuth,
				Status: service.StatusActive, Schedulable: true, Concurrency: 1,
				Credentials: map[string]any{"access_token": "tok", "project_id": "proj"},
				Extra: map[string]any{
					"mixed_scheduling": true,
					"model_rate_limits": map[string]any{
						"claude-opus-5-5-low": map[string]any{"rate_limit_reset_at": resetAt},
					},
				},
				AccountGroups: []service.AccountGroup{{AccountID: 9421, GroupID: groupID}},
			}
			cache := &countingGatewaySchedulerCache{fakeSchedulerCache: &fakeSchedulerCache{accounts: []*service.Account{agAccount}}}
			snapshot := service.NewSchedulerSnapshotService(cache, nil, nil, nil, nil)
			upstream := &antigravityCompatPolicyUpstream{}
			billingCache := service.NewBillingCacheService(newHandlerInflightCache(100), nil, nil, nil, nil, nil, cfg, nil)
			t.Cleanup(billingCache.Stop)
			gateway := service.NewGatewayService(
				nil, &fakeGroupRepo{group: group}, nil, nil, nil, nil, nil, nil, cfg,
				snapshot, nil, service.NewBillingService(cfg, nil), nil, nil, nil, upstream,
				&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			)
			h := &GatewayHandler{
				gatewayService: gateway, billingCacheService: billingCache, cfg: cfg, maxAccountSwitches: 1,
				concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(&fakeConcurrencyCache{}), SSEPingFormatClaude, 0),
			}

			reqMap := map[string]any{"model": "claude-opus-5-5", "stream": false}
			if endpoint == "chat/completions" {
				reqMap["messages"] = []map[string]string{{"role": "user", "content": "hello"}}
				reqMap["reasoning_effort"] = "high"
			} else {
				reqMap["input"] = "hello"
				reqMap["reasoning"] = map[string]string{"effort": "high"}
			}
			body, err := json.Marshal(reqMap)
			require.NoError(t, err)

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			ctx := context.WithValue(context.Background(), ctxkey.Group, group)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+endpoint, bytes.NewReader(body)).WithContext(ctx)
			c.Request.Header.Set("Content-Type", "application/json")
			apiKey := &service.APIKey{
				ID: 9422, UserID: 9423, GroupID: &groupID, Group: group,
				User: &service.User{ID: 9423, Balance: 100},
			}
			c.Set(string(middleware.ContextKeyAPIKey), apiKey)
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 9423})
			if endpoint == "chat/completions" {
				h.ChatCompletions(c)
			} else {
				h.Responses(c)
			}

			require.Equal(t, http.StatusTooManyRequests, recorder.Code, recorder.Body.String())
			require.Empty(t, upstream.bodies)
		})
	}
}
