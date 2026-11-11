package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newCompositePoolTestContext(t *testing.T, path string, platforms ...string) (*gin.Context, *service.APIKey) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", path, nil)
	if len(platforms) > 0 {
		c.Request = c.Request.WithContext(service.WithCompositeCandidatePlatforms(c.Request.Context(), platforms))
	}
	apiKey := &service.APIKey{Group: &service.Group{Platform: service.PlatformComposite}}
	return c, apiKey
}

// 账号池请求不得在 handler 侧再做单平台 detector 解析：池保持未折叠，
// ResolvedTargetPlatform 不被写入，候选集合原样保留。
func TestEnsureCompositeTargetPlatformKeepsPoolUnfolded(t *testing.T) {
	c, apiKey := newCompositePoolTestContext(t, "/v1/chat/completions",
		service.PlatformDeepseek, service.PlatformZhipu)

	ensureCompositeTargetPlatform(c, apiKey, "deepseek-chat")

	_, resolved := service.ResolvedTargetPlatformFromContext(c.Request.Context())
	require.False(t, resolved, "pool request must not gain a single resolved platform from the detector")
	candidates := service.CompositeCandidatePlatformsFromContext(c.Request.Context())
	require.Equal(t, []string{service.PlatformDeepseek, service.PlatformZhipu}, candidates)
}

// 兼容族文本端点准入：全部候选属于 7 类兼容族的池放行，任一候选越界拒绝；
// 池不被折叠成单一平台后单独判断。
func TestOpenAICompatibleTextTargetAllowedPoolSemantics(t *testing.T) {
	family := []string{
		service.PlatformOpenAI, service.PlatformGrok,
		service.PlatformKimi, service.PlatformZhipu, service.PlatformDeepseek,
		service.PlatformMiniMax, service.PlatformOpenCodeGo,
	}

	// 全族整体（7 候选）与任意真子集（此处抽 3 个代表）都放行。
	for _, subset := range [][]string{family, {service.PlatformDeepseek, service.PlatformOpenCodeGo}, {service.PlatformGrok}, {service.PlatformMiniMax, service.PlatformZhipu, service.PlatformKimi}} {
		c, apiKey := newCompositePoolTestContext(t, "/v1/messages", subset...)
		require.Truef(t, openAICompatibleTextTargetAllowed(c, apiKey, "deepseek-flash"), "subset=%v", subset)
	}

	for _, subset := range [][]string{
		{service.PlatformAnthropic, service.PlatformDeepseek},
		{service.PlatformOpenAI, service.PlatformGemini},
		{service.PlatformAntigravity},
	} {
		c, apiKey := newCompositePoolTestContext(t, "/v1/messages", subset...)
		require.Falsef(t, openAICompatibleTextTargetAllowed(c, apiKey, "deepseek-flash"), "subset=%v", subset)
	}
}

// openai-only 端点（live/alpha search/embeddings 既有准入）：池收窄为合法
// single（仅 {openai}）才放行，更宽的池清晰拒绝。
func TestCompositeTargetPlatformAllowedPoolNarrowsOpenAIOnlyEndpoints(t *testing.T) {
	c, apiKey := newCompositePoolTestContext(t, "/v1/live", service.PlatformOpenAI)
	require.True(t, compositeTargetPlatformAllowed(c, apiKey, "gpt-realtime", service.PlatformOpenAI))

	c, apiKey = newCompositePoolTestContext(t, "/v1/live", service.PlatformOpenAI, service.PlatformGrok)
	require.False(t, compositeTargetPlatformAllowed(c, apiKey, "gpt-realtime", service.PlatformOpenAI))
	c, apiKey = newCompositePoolTestContext(t, "/alpha/search", service.PlatformGrok)
	require.False(t, compositeTargetPlatformAllowed(c, apiKey, "gpt-5", service.PlatformOpenAI))
}

// 池是已解析的完整决策：compositeTargetPlatformResolved 不得因缺少单一
// ResolvedTargetPlatform 把池请求判为未解析。
func TestCompositeTargetPlatformResolvedPoolCountsAsResolved(t *testing.T) {
	c, apiKey := newCompositePoolTestContext(t, "/v1/messages",
		service.PlatformDeepseek, service.PlatformOpenCodeGo)
	require.True(t, compositeTargetPlatformResolved(c, apiKey, "deepseek-flash"))
	_, resolved := service.ResolvedTargetPlatformFromContext(c.Request.Context())
	require.False(t, resolved)
}

// messages dispatch 豁免与单目标同语义：候选全部为 grok/CN/OpenCode Go 时
// 豁免分组开关；混有 openai 候选时仍受开关控制。
func TestAllowOpenAICompatibleMessagesDispatchPoolExemptions(t *testing.T) {
	newKey := func(allow bool) *service.APIKey {
		return &service.APIKey{Group: &service.Group{
			Platform:              service.PlatformComposite,
			AllowMessagesDispatch: allow,
		}}
	}

	// 全 CN/grok 池：与对应独立分组同语义豁免。
	c, _ := newCompositePoolTestContext(t, "/v1/messages", service.PlatformDeepseek, service.PlatformGrok, service.PlatformKimi)
	require.True(t, allowOpenAICompatibleMessagesDispatch(c, newKey(false)))

	// 混有 openai 候选：受分组开关控制。
	c, _ = newCompositePoolTestContext(t, "/v1/messages", service.PlatformOpenAI, service.PlatformDeepseek)
	require.False(t, allowOpenAICompatibleMessagesDispatch(c, newKey(false)))
	require.True(t, allowOpenAICompatibleMessagesDispatch(c, newKey(true)))

	// 纯 openai 池：受分组开关控制。
	c, _ = newCompositePoolTestContext(t, "/v1/messages", service.PlatformOpenAI)
	require.False(t, allowOpenAICompatibleMessagesDispatch(c, newKey(false)))

	// 非 composite 分组不读池上下文。
	c, _ = newCompositePoolTestContext(t, "/v1/messages", service.PlatformDeepseek)
	anthropicKey := &service.APIKey{Group: &service.Group{Platform: service.PlatformAnthropic}}
	require.False(t, allowOpenAICompatibleMessagesDispatch(c, anthropicKey))
}

// effectiveAPIKeyPlatform 在池请求上保持组平台回退语义（不猜单平台）。
func TestEffectiveAPIKeyPlatformWithPoolKeepsGroupFallback(t *testing.T) {
	c, apiKey := newCompositePoolTestContext(t, "/v1/messages", service.PlatformDeepseek, service.PlatformKimi)
	require.Equal(t, service.PlatformComposite, effectiveAPIKeyPlatform(c, apiKey))
}

// 池请求的组 effort 策略在账号选定后补齐：选中 openai 账号 → 与
// single-openai 同语义应用组上限；选中 grok/CN 账号 → 维持不套用语义；
// 单目标请求不重复应用。
func TestApplyCompositePoolReasoningEffortPolicyForSelectedAccount(t *testing.T) {
	overCeiling := []byte(`{"model":"gpt-5.6","reasoning":{"effort":"xhigh"}}`)

	// 池 + openai 账号：超限 effort 被压到组上限。
	c, apiKey := newCompositePoolTestContext(t, "/v1/responses", service.PlatformOpenAI, service.PlatformDeepseek)
	apiKey.Group.MaxReasoningEffort = "medium"
	capped, changed, err := applyCompositePoolReasoningEffortPolicyForSelectedAccount(
		c, apiKey, &service.Account{Platform: service.PlatformOpenAI}, overCeiling)
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, string(capped), `"effort":"medium"`)

	// 池 + grok 账号：维持 grok 独立分组不套用语义。
	c, apiKey = newCompositePoolTestContext(t, "/v1/responses", service.PlatformOpenAI, service.PlatformDeepseek)
	_, changed, err = applyCompositePoolReasoningEffortPolicyForSelectedAccount(
		c, apiKey, &service.Account{Platform: service.PlatformGrok}, overCeiling)
	require.NoError(t, err)
	require.False(t, changed)

	// 池 + CN 账号：同上。
	c, apiKey = newCompositePoolTestContext(t, "/v1/responses", service.PlatformDeepseek)
	_, changed, err = applyCompositePoolReasoningEffortPolicyForSelectedAccount(
		c, apiKey, &service.Account{Platform: service.PlatformDeepseek}, overCeiling)
	require.NoError(t, err)
	require.False(t, changed)

	// 单目标请求已在准入期应用，选后不重复。
	c, apiKey = newCompositePoolTestContext(t, "/v1/responses")
	c.Request = c.Request.WithContext(service.WithResolvedTargetPlatform(c.Request.Context(), service.PlatformOpenAI))
	_, changed, err = applyCompositePoolReasoningEffortPolicyForSelectedAccount(
		c, apiKey, &service.Account{Platform: service.PlatformOpenAI}, overCeiling)
	require.NoError(t, err)
	require.False(t, changed)

	// 非 composite 分组不适用。
	plainKey := &service.APIKey{Group: &service.Group{Platform: service.PlatformOpenAI, MaxReasoningEffort: "medium"}}
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	_, changed, err = applyCompositePoolReasoningEffortPolicyForSelectedAccount(
		c2, plainKey, &service.Account{Platform: service.PlatformOpenAI}, overCeiling)
	require.NoError(t, err)
	require.False(t, changed)
}

// 池请求在 messages 桥准入期绑定组策略 ctx：服务侧 ForwardAsAnthropic 按选中
// 账号是否 openai 决定是否应用；显式 effort 缺省时不绑定（保桥默认值语义）。
func TestBindOpenAIReasoningEffortPolicyForMessagesRequestPool(t *testing.T) {
	explicit := []byte(`{"model":"deepseek-flash","output_config":{"effort":"xhigh"}}`)
	defaulted := []byte(`{"model":"deepseek-flash"}`)

	// 池 + 显式 effort：绑定组策略。
	c, apiKey := newCompositePoolTestContext(t, "/v1/messages", service.PlatformDeepseek, service.PlatformOpenCodeGo)
	apiKey.Group.MaxReasoningEffort = "medium"
	bindOpenAIReasoningEffortPolicyForMessagesRequest(c, apiKey, explicit)
	capped, changed, err := service.ApplyOpenAIReasoningEffortPolicyFromContext(c.Request.Context(), explicit)
	require.NoError(t, err)
	require.True(t, changed, "pool messages bind must carry the group policy for the bridge to apply")
	require.Contains(t, string(capped), `"effort":"medium"`, "xhigh must be clamped to the group ceiling")

	// 池 + 无显式 effort：不绑定（桥默认 effort 不受上限改写）。
	c, apiKey = newCompositePoolTestContext(t, "/v1/messages", service.PlatformDeepseek, service.PlatformOpenCodeGo)
	bindOpenAIReasoningEffortPolicyForMessagesRequest(c, apiKey, defaulted)
	_, changed, err = service.ApplyOpenAIReasoningEffortPolicyFromContext(c.Request.Context(), defaulted)
	require.NoError(t, err)
	require.False(t, changed)
}

// pool 判定与 service 一致：已有 resolved endpoint pin（Gemini fallback）时按
// single 语义——候选数据保留但不作为活跃池；allowed/dispatch 以 pin 优先。
func TestCompositePoolDetectionDefersToResolvedPin(t *testing.T) {
	c, apiKey := newCompositePoolTestContext(t, "/v1beta/models/gemini-2.5-pro:generateContent",
		service.PlatformGemini, service.PlatformDeepseek)
	c.Request = c.Request.WithContext(service.WithResolvedTargetPlatform(c.Request.Context(), service.PlatformGemini))

	// pin 存在时不按活跃池判定：gemini pin 被允许集完整包含。
	require.True(t, compositeTargetPlatformAllowed(c, apiKey, "gemini-2.5-pro", service.PlatformGemini))
	require.True(t, compositeTargetPlatformResolved(c, apiKey, "gemini-2.5-pro"))
	_, isPool := compositeAccountPoolCandidates(c)
	require.False(t, isPool, "resolved pin must deactivate the pool for admission semantics")

	// pin 存在时按 pin 作为生效平台（single 语义）。
	require.Equal(t, service.PlatformGemini, effectiveAPIKeyPlatform(c, apiKey))
}
