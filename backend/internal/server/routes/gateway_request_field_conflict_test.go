package routes

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestGatewayRoutesRejectConflictingFieldsMountedOnEveryGatewayRoute 沿用
// gateway_model_allowlist_test.go 的源码级断言：每条网关链都必须紧跟在白名单
// 准入之后挂载 rejectConflictingFields，这样它也在合成路由改写之前。
func TestGatewayRoutesRejectConflictingFieldsMountedOnEveryGatewayRoute(t *testing.T) {
	routeSource, err := os.ReadFile("gateway.go")
	require.NoError(t, err)
	source := string(routeSource)

	// 每一处白名单挂载点后面都紧跟着字段歧义拒绝，两者数量一致。
	allowlistUses := regexp.MustCompile(`\bgroupModelAllowlist\b`).FindAllStringIndex(source, -1)
	followedByConflict := regexp.MustCompile(`\bgroupModelAllowlist(\)\s+\w+\.Use\(|, )rejectConflictingFields\b`).FindAllStringIndex(source, -1)
	// allowlistUses 里有一处是变量定义本身。
	require.Len(t, followedByConflict, len(allowlistUses)-1,
		"every chain that mounts groupModelAllowlist must mount rejectConflictingFields right after it")
	require.GreaterOrEqual(t, len(followedByConflict), 6, "gateway, gemini, root aliases, codexDirect and both antigravity chains")

	// 带合成路由改写的链：歧义拒绝必须在改写之前。
	for _, chain := range []string{
		`gateway.Use(rejectConflictingFields)` + "\n\t" + `gateway.Use(compositeTarget)`,
		`gemini.Use(rejectConflictingFields)` + "\n\t" + `gemini.Use(compositeGeminiTarget)`,
		`groupModelAllowlist, rejectConflictingFields, compositeTarget, requireGroupAnthropic, handler)`,
		`groupModelAllowlist, rejectConflictingFields, compositeTarget, requireGroupAnthropic)`,
	} {
		require.Contains(t, source, chain)
	}
}

// TestGatewayRoutesRejectConflictingFieldsCoversGatewayRoutes 用没有开启白名单
// 的分组请求各个入口：重复声明且取值不一致的 model 必须在进入 handler 之前
// 得到 400。分组不需要任何额外配置，这正是计费绕过的前提条件。
func TestGatewayRoutesRejectConflictingFieldsCoversGatewayRoutes(t *testing.T) {
	router := newGatewayRoutesTestRouterWithGroup(allowlistGroup(service.PlatformOpenAI, false))

	const duplicate = `"model":"gpt-5.4-nano","input":"hi","model":"gpt-5.4"`
	paths := []struct {
		path string
		body string
	}{
		{"/responses", `{` + duplicate + `}`},
		{"/responses/compact", `{` + duplicate + `}`},
		{"/chat/completions", `{` + duplicate + `}`},
		{"/embeddings", `{` + duplicate + `}`},
		{"/images/generations", `{` + duplicate + `}`},
		{"/videos/generations", `{` + duplicate + `}`},
		{"/messages/count_tokens", `{` + duplicate + `}`},
		{"/tts", `{` + duplicate + `}`},
		{"/alpha/search", `{` + duplicate + `}`},
		{"/v1/responses", `{` + duplicate + `}`},
		{"/v1/messages", `{` + duplicate + `}`},
		{"/v1/messages/count_tokens", `{` + duplicate + `}`},
		{"/v1/chat/completions", `{` + duplicate + `}`},
		{"/v1/embeddings", `{` + duplicate + `}`},
		{"/v1/images/generations", `{` + duplicate + `}`},
		{"/v1/videos/generations", `{` + duplicate + `}`},
		{"/v1/systemone", `{` + duplicate + `}`},
		{"/v1/contents/generations/tasks", `{` + duplicate + `}`},
		{"/api/v3/contents/generations/tasks", `{` + duplicate + `}`},
		{"/v1/live", `{"sdp":"v=0","session":{"model":"gpt-live-a","model":"gpt-live-b"}}`},
		{"/backend-api/codex/responses", `{` + duplicate + `}`},
		{"/backend-api/codex/realtime/calls", `{"sdp":"v=0","session":{"model":"gpt-live-a","model":"gpt-live-b"}}`},
		{"/antigravity/v1/messages", `{` + duplicate + `}`},
	}

	for _, tc := range paths {
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code, "POST %s should be rejected, got body: %s", tc.path, w.Body.String())
		require.Contains(t, w.Body.String(), "more than once", "POST %s", tc.path)
	}
}

// 白名单开启时，不在名单内的模型仍由白名单按原有格式拒绝（404），不会被这层
// 改写成 400；两个候选都在名单内但取值不同时，由这层拒绝。
func TestGatewayRoutesRejectConflictingFieldsKeepsAllowlistResponse(t *testing.T) {
	router := newGatewayRoutesTestRouterWithGroup(allowlistGroup(service.PlatformOpenAI, true, "gpt-5.4", "gpt-5.4-nano"))

	send := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	blocked := send(`{"model":"gpt-5.4","input":"hi","model":"gpt-4.1"}`)
	require.Equal(t, http.StatusNotFound, blocked.Code, blocked.Body.String())
	require.Contains(t, blocked.Body.String(), "not available for this group")

	bothAllowed := send(`{"model":"gpt-5.4-nano","input":"hi","model":"gpt-5.4"}`)
	require.Equal(t, http.StatusBadRequest, bothAllowed.Code, bothAllowed.Body.String())
	require.Contains(t, bothAllowed.Body.String(), "more than once")
}

// 取值相同的重复字段同样拒绝：网关的 sjson 改写只改首个，改写后两者就会分叉。
func TestGatewayRoutesRejectConflictingFieldsRejectsIdenticalDuplicates(t *testing.T) {
	router := newGatewayRoutesTestRouterWithGroup(allowlistGroup(service.PlatformOpenAI, false))

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4","input":"hi","reasoning":{"effort":"xhigh","effort":"xhigh"}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "reasoning.effort")
}

// 回归：Content-Type 标成 multipart 的 JSON 请求体，既不能绕过歧义检查，也不能
// 绕过分组白名单（此前白名单按请求头只做 multipart 解析，拿不到模型就放行）。
func TestGatewayRoutesMultipartLabelDoesNotBypassChecks(t *testing.T) {
	send := func(group *service.Group, contentType, body string) *httptest.ResponseRecorder {
		router := newGatewayRoutesTestRouterWithGroup(group)
		req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
		req.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	for _, contentType := range []string{"multipart/form-data", "multipart/form-data; boundary=zzz"} {
		w := send(allowlistGroup(service.PlatformAnthropic, false), contentType, `{"model":"claude-3-5-haiku-20241022","messages":[],"model":"claude-opus-4-8"}`)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "more than once")

		w = send(allowlistGroup(service.PlatformAnthropic, true, "claude-3-5-haiku-20241022"), contentType, `{"model":"claude-opus-4-8","messages":[]}`)
		require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "not available for this group")
	}
}

// 回归：带 UTF-8 BOM 的请求体（handler 宽容读取会剥掉 BOM）不能绕过分组白名单。
func TestGatewayRoutesBOMDoesNotBypassAllowlist(t *testing.T) {
	router := newGatewayRoutesTestRouterWithGroup(allowlistGroup(service.PlatformAnthropic, true, "claude-3-5-haiku-20241022"))
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("\xef\xbb\xbf"+`{"model":"claude-opus-4-8","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "not available for this group")
}

// 回归：纯文本入口的上限必须在分组中间件读体之前生效，否则中间件会先按通用
// 上限把请求体整体读入并解析。超出文本上限的请求体直接 413，不会走到歧义检查。
func TestGatewayRoutesTextBodyLimitAppliesBeforeGroupMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterGatewayRoutes(
		router,
		&handler.Handlers{Gateway: &handler.GatewayHandler{}, OpenAIGateway: &handler.OpenAIGatewayHandler{}, AsyncImage: handler.NewAsyncImageHandler(nil, nil)},
		servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
			groupID := int64(1)
			c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{GroupID: &groupID, Group: allowlistGroup(service.PlatformOpenAI, false)})
			c.Next()
		}),
		nil, nil, nil, nil, nil,
		&config.Config{Gateway: config.GatewayConfig{MaxBodySize: 1 << 20, TextMaxBodySize: 256}},
	)
	body := `{"model":"a","input":"` + strings.Repeat("x", 1024) + `","model":"b"}`
	for _, path := range []string{"/v1/embeddings", "/v1/alpha/search", "/v1/systemone", "/backend-api/codex/alpha/search", "/embeddings", "/alpha/search"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusRequestEntityTooLarge, w.Code, "POST %s: %s", path, w.Body.String())
		require.Contains(t, w.Body.String(), "limit is 256B", "POST %s", path)
	}
}

// 嵌套字段取值不一致（reasoning.effort 参与计费倍率与分组推理强度上限、
// thinking.type 决定是否清洗 thinking 块）同样在进入 handler 之前拒绝。
func TestGatewayRoutesRejectConflictingNestedFields(t *testing.T) {
	cases := []struct {
		platform string
		path     string
		body     string
		field    string
	}{
		{service.PlatformOpenAI, "/v1/responses", `{"model":"gpt-5.4","reasoning":{"effort":"low","effort":"xhigh"},"input":"hi"}`, `reasoning.effort`},
		{service.PlatformOpenAI, "/backend-api/codex/responses", `{"model":"gpt-5.4","input":[{"role":"user","content":"a","role":"system"}]}`, `input[0].role`},
		{service.PlatformAnthropic, "/v1/messages", `{"model":"claude-sonnet-4-5","thinking":{"type":"disabled","type":"enabled","budget_tokens":1024},"messages":[]}`, `thinking.type`},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			router := newGatewayRoutesTestRouterWithGroup(allowlistGroup(tc.platform, false))
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), "invalid_request_error")
			require.Contains(t, w.Body.String(), tc.field)
		})
	}
}
