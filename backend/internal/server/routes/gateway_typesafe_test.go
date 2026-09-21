package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// newTypeSafeGatewayRoutesTestRouter 与 newGatewayRoutesTestRouter 同构，额外桩上
// auth subject，使请求能穿过完整中间件链后真正进入处理器主体（依赖检查处 503）。
func newTypeSafeGatewayRoutesTestRouter(platform string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	RegisterGatewayRoutes(
		router,
		&handler.Handlers{
			Gateway:       &handler.GatewayHandler{},
			OpenAIGateway: &handler.OpenAIGatewayHandler{},
			AsyncImage:    handler.NewAsyncImageHandler(nil, nil),
		},
		servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
			groupID := int64(1)
			c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
				GroupID: &groupID,
				Group:   &service.Group{Platform: platform},
			})
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 4242, Concurrency: 1})
			c.Next()
		}),
		nil,
		nil,
		nil,
		nil,
		nil,
		&config.Config{Gateway: config.GatewayConfig{MaxBodySize: 1024 * 1024, TextMaxBodySize: 1024 * 1024}},
	)

	return router
}

// TestGatewayRoutesTypeSafeGroupCanReachSystemone 路由级钉住：
// typesafe 分组的 API Key 必须能穿过 /v1 组既有中间件（尤其 requireGroupAnthropic，
// 见 gateway.go 的 gateway.Use(...) 链）到达 /v1/systemone 处理器。
// 测试路由的 handler 依赖为空，进入处理器后必然停在依赖检查 → 503。
func TestGatewayRoutesTypeSafeGroupCanReachSystemone(t *testing.T) {
	router := newTypeSafeGatewayRoutesTestRouter(service.PlatformTypeSafe)

	req := httptest.NewRequest(http.MethodPost, "/v1/systemone", strings.NewReader(`{
		"model":"jev-judge-v1",
		"state":"the customer asked for a refund",
		"questions":[{"id":"q1","text":"is the customer angry?"}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.NotEqual(t, http.StatusNotFound, w.Code, "typesafe 分组必须能到达 /v1/systemone")
	require.NotContains(t, w.Body.String(), "System One API is not supported for this platform")
	// 503「依赖缺失」而不是 401/403/404：证明请求已穿过 apiKeyAuth、
	// groupModelAllowlist、compositeTarget 与 requireGroupAnthropic 进入处理器主体。
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Contains(t, w.Body.String(), "Service temporarily unavailable")
}

func TestGatewayRoutesSystemoneIsRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformTypeSafe)

	registered := make(map[string]bool)
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	require.True(t, registered["POST /v1/systemone"], "POST /v1/systemone should be registered")
}

// TestGatewayRoutesSystemoneRejectedForOtherPlatforms 平台门：只有 typesafe 分组可用。
func TestGatewayRoutesSystemoneRejectedForOtherPlatforms(t *testing.T) {
	for _, platform := range []string{
		service.PlatformOpenAI,
		service.PlatformAnthropic,
		service.PlatformGrok,
		service.PlatformKimi,
		service.PlatformZhipu,
		service.PlatformDeepseek,
		service.PlatformMiniMax,
		service.PlatformOpenCodeGo,
		service.PlatformGemini,
	} {
		t.Run(platform, func(t *testing.T) {
			router := newGatewayRoutesTestRouter(platform)
			req := httptest.NewRequest(http.MethodPost, "/v1/systemone", strings.NewReader(`{"model":"jev-judge-v1","state":"s","questions":[]}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusNotFound, w.Code, "platform=%s", platform)
			require.Contains(t, w.Body.String(), "System One API is not supported for this platform")
		})
	}

	// composite 分组单独断言，且同时接受两条既有的拒绝路径 —— 合并 integration 后
	// 它的拒绝发生点会前移，所以这里不能在两条路径里只钉一条：
	//
	//   - 本分支（以及未合入 composite 分支的 main 基线）：/v1/systemone 的平台门回
	//     404 + "System One API is not supported for this platform"；
	//   - 合入 integration 后：integration 的 composite 入口准入
	//     （compositeTargetPlatformMiddleware）在平台门之前就以
	//     400 + "cannot be resolved to any platform in this composite group"
	//     拒掉 —— composite 分组解析不出 jev-judge-v1 归属的具体平台，
	//     请求根本到不了 /v1/systemone。这是 integration 的 composite 入口准入
	//     引入的差异，不是 systemone 平台门本身的语义。
	//
	// 两条路径都是既有拒绝，都不等于「composite 分组拿到了 SystemOne」；除这两种
	// 之外的任何响应（尤其 2xx/成功）一律算失败。
	t.Run(service.PlatformComposite, func(t *testing.T) {
		router := newGatewayRoutesTestRouter(service.PlatformComposite)
		req := httptest.NewRequest(http.MethodPost, "/v1/systemone", strings.NewReader(`{"model":"jev-judge-v1","state":"s","questions":[]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		body := w.Body.String()
		switch w.Code {
		case http.StatusNotFound:
			require.Contains(t, body, "System One API is not supported for this platform",
				"404 必须来自 /v1/systemone 的平台门，body=%s", body)
		case http.StatusBadRequest:
			require.Contains(t, body, "cannot be resolved to any platform in this composite group",
				"400 必须来自 composite 入口准入，body=%s", body)
		default:
			t.Fatalf("composite 分组必须以已知理由被拒（404 平台门或 400 composite 准入），实际 code=%d body=%s", w.Code, body)
		}
	})
}

// TestGatewayRoutesTypeSafeGroupCannotReachConversationalEndpoints 安全回归：
// typesafe 分组没有对话端点（/messages、/chat/completions、/responses、
// /messages/count_tokens），不拦就会落到通用网关/上游并拿到语义不清的错误，这里改为
// 干净的显式 404。曾经担心的「key 被当 Anthropic key 发到 api.anthropic.com」已不成立——
// (*Account).GetBaseURL() 现在对 typesafe 早退返回空串，「凭据外泄」这一层已在 base URL
// 层关闭。测试路由的 GatewayHandler 是零值，一旦进入通用网关就会触发 nil 依赖，因此
// 「拿到平台门的 404 文案」即证明在入口就被拒了。
func TestGatewayRoutesTypeSafeGroupCannotReachConversationalEndpoints(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformTypeSafe)

	for _, path := range []string{
		"/v1/messages",
		"/v1/messages/count_tokens",
		"/v1/chat/completions",
		"/v1/responses",
		// 与上面同一批处理器的别名路由也必须被拒。
		"/messages/count_tokens",
		"/chat/completions",
		"/responses",
		"/backend-api/codex/responses",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code, "path=%s 必须在入口被拒", path)
		require.Contains(t, w.Body.String(), "is not supported for TypeSafe groups", "path=%s", path)
		require.Contains(t, w.Body.String(), "/v1/systemone", "path=%s 必须提示唯一可用端点", path)
	}
}

// TestGatewayRoutesOtherPlatformsKeepConversationalEndpoints 反向保护：
// 对话端点门只针对 typesafe，其他平台行为不变（例如 openai/kimi/zhipu 分组
// 仍进入 OpenAI 网关，而不是被新门拦掉）。
func TestGatewayRoutesOtherPlatformsKeepConversationalEndpoints(t *testing.T) {
	for _, platform := range []string{service.PlatformOpenAI, service.PlatformKimi, service.PlatformZhipu} {
		router := newGatewayRoutesTestRouter(platform)
		req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		require.NotEqual(t, http.StatusNotFound, w.Code, "platform=%s", platform)
		require.NotContains(t, w.Body.String(), "TypeSafe groups", "platform=%s", platform)
	}
}

// TestGatewayRoutesTypeSafeGroupCannotReachResponsesWebSocketIngress 安全回归：
// GET /responses（Responses WebSocket ingress）是 typesafe 分组的最后一个未门禁
// 对话入口。带上真实的 Upgrade: websocket 头请求，仍必须在入口拿到与 POST
// /responses 完全一致的 404 文案 —— 零值 OpenAIGatewayHandler 一旦被进入就不可能
// 返回该 404，因此「拿到 404」同时证明处理器未被进入（无上游调度、无上游调用）。
func TestGatewayRoutesTypeSafeGroupCannotReachResponsesWebSocketIngress(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformTypeSafe)

	for _, path := range []string{"/v1/responses", "/responses", "/backend-api/codex/responses"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
		req.Header.Set("Sec-WebSocket-Version", "13")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code, "path=%s 必须在入口被拒", path)
		require.Contains(t, w.Body.String(), "Responses API is not supported for TypeSafe groups", "path=%s", path)
		require.Contains(t, w.Body.String(), "/v1/systemone", "path=%s 必须提示唯一可用端点", path)
	}
}

// TestGatewayRoutesOtherPlatformsKeepResponsesWebSocketIngress 反向保护：
// 新门只针对 typesafe，其他平台的 GET /responses 仍进入 ResponsesWebSocket 处理器
// （无 Upgrade 头时表现为 426，而不是平台门的 404）。
func TestGatewayRoutesOtherPlatformsKeepResponsesWebSocketIngress(t *testing.T) {
	for _, platform := range []string{service.PlatformOpenAI, service.PlatformKimi, service.PlatformGrok} {
		for _, path := range []string{"/v1/responses", "/responses", "/backend-api/codex/responses"} {
			router := newGatewayRoutesTestRouter(platform)
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusUpgradeRequired, w.Code, "platform=%s path=%s", platform, path)
			require.NotContains(t, w.Body.String(), "TypeSafe groups", "platform=%s path=%s", platform, path)
		}
	}
}
