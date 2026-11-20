package routes

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
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
		require.Contains(t, w.Body.String(), "more than once with different values", "POST %s", tc.path)
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
	require.Contains(t, bothAllowed.Body.String(), "more than once with different values")
}

// 取值相同的重复 model 不算歧义，不会被这层拒绝。
func TestGatewayRoutesRejectConflictingFieldsAllowsIdenticalDuplicates(t *testing.T) {
	router := newGatewayRoutesTestRouterWithGroup(allowlistGroup(service.PlatformOpenAI, false))

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4","input":"hi","model":"gpt-5.4"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.NotContains(t, w.Body.String(), "more than once with different values",
		"identical duplicates must pass the conflict check, got %d: %s", w.Code, w.Body.String())
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
