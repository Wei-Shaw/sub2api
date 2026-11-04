//go:build unit

package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type geminiImageGateEndpoint struct {
	name          string
	path          string
	body          string
	params        gin.Params
	forcePlatform string
	call          func(*GatewayHandler, *gin.Context)
}

func geminiImageGateEndpoints(model string) []geminiImageGateEndpoint {
	return []geminiImageGateEndpoint{
		{
			name: "messages", path: "/v1/messages",
			body: `{"model":"` + model + `","max_tokens":16,"messages":[{"role":"user","content":"draw a cat"}]}`,
			call: (*GatewayHandler).Messages,
		},
		{
			name: "chat completions", path: "/v1/chat/completions",
			body: `{"model":"` + model + `","messages":[{"role":"user","content":"draw a cat"}]}`,
			call: (*GatewayHandler).ChatCompletions,
		},
		{
			name: "responses", path: "/v1/responses",
			body: `{"model":"` + model + `","input":"draw a cat"}`,
			call: (*GatewayHandler).Responses,
		},
		{
			name: "gemini native", path: "/v1beta/models/" + model + ":generateContent",
			body:   `{"contents":[{"role":"user","parts":[{"text":"draw a cat"}]}]}`,
			params: gin.Params{{Key: "modelAction", Value: "/" + model + ":generateContent"}},
			call:   (*GatewayHandler).GeminiV1BetaModels,
		},
	}
}

func runGeminiImageGateEndpoint(t *testing.T, ep geminiImageGateEndpoint, group *service.Group, accountPlatform string) (*httptest.ResponseRecorder, *gin.Context, *groupScopedSchedulerCache) {
	t.Helper()
	const accountID = int64(9411)
	schedulerCache := &groupScopedSchedulerCache{fakeSchedulerCache: &fakeSchedulerCache{accounts: []*service.Account{{
		ID: accountID, Platform: accountPlatform, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, Concurrency: 1,
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: group.ID}},
	}}}}
	gatewayService := service.NewGatewayService(
		nil, &groupMapRepo{fakeGroupRepo: &fakeGroupRepo{}, groups: map[int64]*service.Group{group.ID: group}},
		nil, nil, nil, nil, nil, nil, nil,
		service.NewSchedulerSnapshotService(schedulerCache, nil, nil, nil, nil),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheService.Stop)
	h := &GatewayHandler{
		gatewayService:      gatewayService,
		billingCacheService: billingCacheService,
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(&fakeConcurrencyCache{}), SSEPingFormatClaude, 0),
		maxAccountSwitches:  1,
		cfg:                 cfg,
	}

	groupID := group.ID
	apiKey := &service.APIKey{
		ID: 9420, UserID: 9430, GroupID: &groupID, Group: group, Status: service.StatusActive,
		User: &service.User{ID: 9430, Concurrency: 10, Balance: 100},
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx := context.WithValue(context.Background(), ctxkey.Group, group)
	req := httptest.NewRequest(http.MethodPost, ep.path, bytes.NewBufferString(ep.body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Params = ep.params
	if ep.forcePlatform != "" {
		c.Set(string(middleware.ContextKeyForcePlatform), ep.forcePlatform)
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 10})

	ep.call(h, c)
	return recorder, c, schedulerCache
}

func TestGatewayHandlers_GeminiImageGenerationDisabledGroupRejectsBeforeSelection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, platform := range []string{service.PlatformGemini, service.PlatformAntigravity} {
		for _, ep := range geminiImageGateEndpoints("gemini-3-pro-image") {
			if platform == service.PlatformAntigravity && ep.params != nil {
				ep.path = "/antigravity" + ep.path
				ep.forcePlatform = service.PlatformAntigravity
			}
			t.Run(platform+"/"+ep.name, func(t *testing.T) {
				group := &service.Group{
					ID: 9400, Hydrated: true, Platform: platform, Status: service.StatusActive,
					AllowImageGeneration: false,
				}
				recorder, c, schedulerCache := runGeminiImageGateEndpoint(t, ep, group, platform)

				require.Equal(t, http.StatusBadRequest, recorder.Code)
				require.Contains(t, recorder.Body.String(), service.ImageGenerationPermissionMessage())
				_, reachedSelection := c.Get(opsAccountIDKey)
				require.False(t, reachedSelection)
				require.Empty(t, schedulerCache.queriedGroupIDs())
			})
		}
	}
}

func TestGatewayHandlers_GeminiImageGenerationGateSkipsGroupsWithoutTheSwitch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Anthropic 分组在后台不展示生图开关，混合调度 Antigravity 账号时不受门禁约束。
	for _, ep := range geminiImageGateEndpoints("gemini-3-pro-image")[:3] {
		t.Run(ep.name, func(t *testing.T) {
			group := &service.Group{
				ID: 9401, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive,
				AllowImageGeneration: false,
			}
			recorder, _, schedulerCache := runGeminiImageGateEndpoint(t, ep, group, service.PlatformAnthropic)

			require.NotContains(t, recorder.Body.String(), service.ImageGenerationPermissionMessage())
			require.NotEmpty(t, schedulerCache.queriedGroupIDs())
		})
	}
}

func TestGatewayHandlers_GeminiImageGenerationGateAllowsTextModelsAndEnabledGroups(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name  string
		model string
		allow bool
	}{
		{name: "text model in disabled group", model: "gemini-3.7-flash", allow: false},
		{name: "image model in enabled group", model: "gemini-3-pro-image", allow: true},
	}
	for _, tc := range cases {
		for _, ep := range geminiImageGateEndpoints(tc.model)[:3] {
			t.Run(tc.name+"/"+ep.name, func(t *testing.T) {
				group := &service.Group{
					ID: 9402, Hydrated: true, Platform: service.PlatformGemini, Status: service.StatusActive,
					AllowImageGeneration: tc.allow,
				}
				recorder, _, schedulerCache := runGeminiImageGateEndpoint(t, ep, group, service.PlatformGemini)

				require.NotContains(t, recorder.Body.String(), service.ImageGenerationPermissionMessage())
				require.NotEmpty(t, schedulerCache.queriedGroupIDs())
			})
		}
	}
}
