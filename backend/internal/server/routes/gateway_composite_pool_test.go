package routes

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// compositePoolOwnershipStub 让 resolver 对任意模型返回多平台账号池决策。
type compositePoolOwnershipStub struct {
	platforms []string
}

func (s compositePoolOwnershipStub) ResolveModelOwnership(_ context.Context, _ int64, _ string) (service.CompositeModelOwnership, error) {
	return service.CompositeModelOwnership{CandidatePlatforms: s.platforms}, nil
}

func newCompositePoolTestRouter(t *testing.T, terminal gin.HandlerFunc, ownership compositePoolOwnershipStub, groupPlatform string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.HandlerFunc(servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
		groupID := int64(1)
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: groupPlatform},
		})
		c.Next()
	})))
	resolver := service.NewCompositeRouteResolver(compositeRouteRepoStub{})
	resolver.SetModelOwnershipResolver(ownership.ResolveModelOwnership)
	router.Use(compositeTargetPlatformMiddleware(resolver))
	router.POST("/v1/chat/completions", terminal)
	return router
}

// 账号池决策必须把完整候选集合写入 ctx（去重升序），且不写单一
// ResolvedTargetPlatform、不改写 body 模型、来源标记 account_pool。
func TestCompositeTargetPlatformMiddlewareWritesPoolDecision(t *testing.T) {
	seen := false
	router := newCompositePoolTestRouter(t, func(c *gin.Context) {
		seen = true
		candidates := service.CompositeCandidatePlatformsFromContext(c.Request.Context())
		require.Equal(t, []string{service.PlatformDeepseek, service.PlatformZhipu}, candidates)

		_, resolved := service.ResolvedTargetPlatformFromContext(c.Request.Context())
		require.False(t, resolved, "pool decision must not pin a single target platform")

		source, ok := service.CompositeRouteSourceFromContext(c.Request.Context())
		require.True(t, ok)
		require.Equal(t, service.CompositeRouteSourceAccountPool, source)

		publicModel, ok := service.RequestedPublicModelFromContext(c.Request.Context())
		require.True(t, ok)
		require.Equal(t, "cn-alias", publicModel)

		_, upstream := service.ResolvedUpstreamModelFromContext(c.Request.Context())
		require.False(t, upstream, "pool decision must not resolve an upstream model")

		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"model":"cn-alias","messages":[]}`, string(body),
			"pool request must keep the public model in the body")
		c.Status(http.StatusNoContent)
	}, compositePoolOwnershipStub{platforms: []string{service.PlatformZhipu, service.PlatformDeepseek}}, service.PlatformComposite)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"cn-alias","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.True(t, seen, "pool request must reach the terminal handler")
	require.Equal(t, http.StatusNoContent, w.Code)
}

// composite 请求的模型无法解析到任何平台时必须显式 400，不能再静默放行到
// 通用网关按通用协议报错。
func TestCompositeTargetPlatformMiddlewareRejectsUnresolvedModel(t *testing.T) {
	router := newCompositePoolTestRouter(t, func(c *gin.Context) {
		t.Fatal("unresolvable composite model must not reach any handler")
	}, compositePoolOwnershipStub{}, service.PlatformComposite)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"totally-unknown-alias"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "totally-unknown-alias")
}

// 非 composite 分组保持原行为：无模型解析拦截。
func TestCompositeTargetPlatformMiddlewareKeepsNonCompositeBehavior(t *testing.T) {
	router := newCompositePoolTestRouter(t, func(c *gin.Context) {
		_, resolved := service.ResolvedTargetPlatformFromContext(c.Request.Context())
		require.False(t, resolved)
		candidates := service.CompositeCandidatePlatformsFromContext(c.Request.Context())
		require.Empty(t, candidates)
		c.Status(http.StatusNoContent)
	}, compositePoolOwnershipStub{platforms: []string{service.PlatformDeepseek}}, service.PlatformAnthropic)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"weird-model"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
}

// 全部 7 类兼容族平台的任意非空子集池都必须进入 OpenAI 网关 handler；
// 池大小不参与族判断。
func TestDispatchOpenAICompatibleGatewayRoutesAllFamilyPoolSubsets(t *testing.T) {
	family := []string{
		service.PlatformOpenAI, service.PlatformGrok,
		service.PlatformKimi, service.PlatformZhipu, service.PlatformDeepseek,
		service.PlatformMiniMax, service.PlatformOpenCodeGo,
	}
	subsets := allNonEmptySubsets(family)
	require.Len(t, subsets, 127)
	for _, subset := range subsets {
		var openAI, generic int
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(
			service.WithCompositeCandidatePlatforms(context.Background(), subset))
		dispatchOpenAICompatibleGateway(c,
			func(c *gin.Context) { openAI++ },
			func(c *gin.Context) { generic++ })
		require.Equal(t, 1, openAI, "subset=%v must route to the OpenAI gateway handler", subset)
		require.Zero(t, generic, "subset=%v must not fall through to the generic handler", subset)
	}
}

// 单一 DeepSeek 目标与全兼容族池走同一个 OpenAI handler，不产生第二次选号
// 或不同 sticky 语义。
func TestDispatchOpenAICompatibleGatewaySingleDeepSeekMatchesPoolFamily(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{Platform: service.PlatformComposite},
	})
	c.Request = c.Request.WithContext(service.WithResolvedTargetPlatform(c.Request.Context(), service.PlatformDeepseek))

	var openAI, generic int
	dispatchOpenAICompatibleGateway(c,
		func(c *gin.Context) { openAI++ },
		func(c *gin.Context) { generic++ })
	require.Equal(t, 1, openAI)
	require.Zero(t, generic)
}

// 跨原生协议族池（兼容族混 anthropic/gemini/antigravity，或全部为原生族）
// 必须显式 400 并点名候选平台，不得落入通用 handler。
func TestDispatchOpenAICompatibleGatewayRejectsCrossFamilyPool(t *testing.T) {
	for _, subset := range [][]string{
		{service.PlatformAnthropic, service.PlatformDeepseek},
		{service.PlatformOpenAI, service.PlatformGrok, service.PlatformGemini},
		{service.PlatformAnthropic},
		{service.PlatformAntigravity, service.PlatformKimi},
		{service.PlatformAnthropic, service.PlatformGemini, service.PlatformAntigravity},
	} {
		var openAI, generic int
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(
			service.WithCompositeCandidatePlatforms(context.Background(), subset))
		dispatchOpenAICompatibleGateway(c,
			func(c *gin.Context) { openAI++ },
			func(c *gin.Context) { generic++ })
		require.Zero(t, openAI, "subset=%v must not enter the OpenAI handler", subset)
		require.Zero(t, generic, "subset=%v must not fall through to the generic handler", subset)
		require.True(t, c.IsAborted(), "subset=%v must abort", subset)
		require.Equal(t, http.StatusBadRequest, recorder.Code, "subset=%v", subset)
		for _, platform := range subset {
			require.Contains(t, recorder.Body.String(), platform, "subset=%v must name every candidate", subset)
		}
	}
}

// 非 composite 分组与解析到原生族单目标的 composite 分组保持既有落通用
// handler 的行为（显式 route + 普通分组不回归）。
func TestDispatchOpenAICompatibleGatewayKeepsGenericTargets(t *testing.T) {
	// anthropic 普通分组。
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{Platform: service.PlatformAnthropic},
	})
	var openAI, generic int
	dispatchOpenAICompatibleGateway(c,
		func(c *gin.Context) { openAI++ },
		func(c *gin.Context) { generic++ })
	require.Zero(t, openAI)
	require.Equal(t, 1, generic)

	// composite 解析到 claude 单目标。
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c2.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{Platform: service.PlatformComposite},
	})
	c2.Request = c2.Request.WithContext(service.WithResolvedTargetPlatform(c2.Request.Context(), service.PlatformAnthropic))
	openAI, generic = 0, 0
	dispatchOpenAICompatibleGateway(c2,
		func(c *gin.Context) { openAI++ },
		func(c *gin.Context) { generic++ })
	require.Zero(t, openAI)
	require.Equal(t, 1, generic)
}

// count_tokens 沿用既有端点语义：兼容族池进 OpenAI CountTokens，跨族池
// 明确拒绝，grok 单目标保留 GrokCountTokens。
func TestDispatchOpenAICompatibleCountTokensPoolAndSingle(t *testing.T) {
	var compatible, grok, generic int
	handlers := []gin.HandlerFunc{
		func(c *gin.Context) { compatible++ },
		func(c *gin.Context) { grok++ },
		func(c *gin.Context) { generic++ },
	}

	newContext := func(seed func(c *gin.Context)) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)
		seed(c)
		return c
	}

	// 兼容族池（含 grok 候选）进 OpenAI CountTokens。
	dispatchOpenAICompatibleCountTokens(newContext(func(c *gin.Context) {
		c.Request = c.Request.WithContext(service.WithCompositeCandidatePlatforms(c.Request.Context(),
			[]string{service.PlatformDeepseek, service.PlatformGrok}))
	}), handlers[0], handlers[1], handlers[2])
	require.Equal(t, 1, compatible)
	require.Zero(t, grok)
	require.Zero(t, generic)

	// 跨族池拒绝，任何 handler 都不执行。
	c := newContext(func(c *gin.Context) {
		c.Request = c.Request.WithContext(service.WithCompositeCandidatePlatforms(c.Request.Context(),
			[]string{service.PlatformAnthropic, service.PlatformDeepseek}))
	})
	compatible, grok, generic = 0, 0, 0
	dispatchOpenAICompatibleCountTokens(c, handlers[0], handlers[1], handlers[2])
	require.Zero(t, compatible)
	require.Zero(t, grok)
	require.Zero(t, generic)
	require.True(t, c.IsAborted())

	// grok 单目标保留本地估算路径。
	compatible, grok, generic = 0, 0, 0
	dispatchOpenAICompatibleCountTokens(newContext(func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
			Group: &service.Group{Platform: service.PlatformGrok},
		})
	}), handlers[0], handlers[1], handlers[2])
	require.Equal(t, 1, grok)
	require.Zero(t, compatible)
	require.Zero(t, generic)

	// anthropic 普通分组保持通用 CountTokens。
	compatible, grok, generic = 0, 0, 0
	dispatchOpenAICompatibleCountTokens(newContext(func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
			Group: &service.Group{Platform: service.PlatformAnthropic},
		})
	}), handlers[0], handlers[1], handlers[2])
	require.Equal(t, 1, generic)
	require.Zero(t, compatible)
	require.Zero(t, grok)
}

// allNonEmptySubsets 返回 platforms 的全部非空子集（2^n - 1 个）。
func allNonEmptySubsets(platforms []string) [][]string {
	total := 1 << len(platforms)
	subsets := make([][]string, 0, total-1)
	for mask := 1; mask < total; mask++ {
		subset := make([]string, 0, len(platforms))
		for i, platform := range platforms {
			if mask&(1<<i) != 0 {
				subset = append(subset, platform)
			}
		}
		subsets = append(subsets, subset)
	}
	return subsets
}

// pool 判定与 service 一致：ctx 同时携带池候选与 resolved pin（Gemini
// fallback）时按 single 语义落通用 handler，不按跨族池 400、不进 OpenAI 族。
func TestDispatchOpenAICompatibleGatewayDefersToResolvedPinOverPool(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{Platform: service.PlatformComposite},
	})
	ctx := service.WithCompositeCandidatePlatforms(c.Request.Context(),
		[]string{service.PlatformDeepseek, service.PlatformGemini})
	ctx = service.WithResolvedTargetPlatform(ctx, service.PlatformGemini)
	c.Request = c.Request.WithContext(ctx)

	var openAI, generic int
	dispatchOpenAICompatibleGateway(c,
		func(c *gin.Context) { openAI++ },
		func(c *gin.Context) { generic++ })
	require.Zero(t, openAI)
	require.Equal(t, 1, generic, "resolved pin must take single semantics over the pool candidates")
	require.False(t, c.IsAborted())
}

// 入口准入 400 信封按入站协议选择：/v1/messages 系列用 Anthropic 信封。
func TestCompositeTargetPlatformMiddlewareUsesAnthropicEnvelopeOnMessages(t *testing.T) {
	router := newCompositePoolTestRouter(t, func(c *gin.Context) {
		t.Fatal("unresolvable composite model must not reach any handler")
	}, compositePoolOwnershipStub{}, service.PlatformComposite)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"totally-unknown-alias","max_tokens":1}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var decoded struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &decoded))
	require.Equal(t, "error", decoded.Type, "/v1/messages rejections must use the Anthropic error envelope")
	require.Equal(t, "invalid_request_error", decoded.Error.Type)
	require.Contains(t, decoded.Error.Message, "totally-unknown-alias")
}

// 跨族池在 /v1/messages 上同样返回 Anthropic 信封；CC 端点保持 OpenAI 信封。
func TestCompositeCrossFamilyPoolEnvelopeFollowsInboundProtocol(t *testing.T) {
	crossFamily := []string{service.PlatformAnthropic, service.PlatformDeepseek}

	// /v1/messages：Anthropic 信封。
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(
		service.WithCompositeCandidatePlatforms(context.Background(), crossFamily))
	dispatchOpenAICompatibleGateway(c, func(c *gin.Context) {}, func(c *gin.Context) {})
	require.True(t, c.IsAborted())
	var decoded struct {
		Type string `json:"type"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &decoded))
	require.Equal(t, "error", decoded.Type, "messages-family rejections must use the Anthropic envelope")

	// /v1/chat/completions：OpenAI 信封。
	recorder2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(recorder2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(
		service.WithCompositeCandidatePlatforms(context.Background(), crossFamily))
	dispatchOpenAICompatibleGateway(c2, func(c *gin.Context) {}, func(c *gin.Context) {})
	require.True(t, c2.IsAborted())
	var decoded2 struct {
		Type string `json:"type"`
	}
	require.NoError(t, json.Unmarshal(recorder2.Body.Bytes(), &decoded2))
	require.Empty(t, decoded2.Type, "chat completions rejections must keep the OpenAI envelope")
}
