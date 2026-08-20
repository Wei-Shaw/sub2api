package handler

// CN 分组 /v1/messages 调度闸门回归（修复:正常途径创建的 CN 分组曾恒 403）：
// sanitizeGroupMessagesDispatchFields 对非 openai 平台强制 AllowMessagesDispatch
// =false，故 CN 分组必须与 grok 一样在闸门处豁免，否则原生 Anthropic 直通
//（Claude Code 主用例）永远不可达。composite 分组同理：sanitize 对 composite
// 恒置 false，解析到 grok/CN 目标时必须按目标平台豁免，解析到 openai 目标
// 仍受开关控制。

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAllowOpenAICompatibleMessagesDispatch_CNProvidersExempt(t *testing.T) {
	require.True(t, allowOpenAICompatibleMessagesDispatch(nil, nil), "无 key 保持放行")

	for _, platform := range []string{service.PlatformKimi, service.PlatformZhipu, service.PlatformDeepseek, service.PlatformGrok} {
		apiKey := &service.APIKey{Group: &service.Group{Platform: platform, AllowMessagesDispatch: false}}
		require.True(t, allowOpenAICompatibleMessagesDispatch(nil, apiKey),
			"%s 分组必须豁免 allow_messages_dispatch 闸门", platform)
	}

	// 非回归：openai 分组仍受开关控制。
	openaiOff := &service.APIKey{Group: &service.Group{Platform: service.PlatformOpenAI, AllowMessagesDispatch: false}}
	require.False(t, allowOpenAICompatibleMessagesDispatch(nil, openaiOff))
	openaiOn := &service.APIKey{Group: &service.Group{Platform: service.PlatformOpenAI, AllowMessagesDispatch: true}}
	require.True(t, allowOpenAICompatibleMessagesDispatch(nil, openaiOn))
}

func TestAllowOpenAICompatibleMessagesDispatch_CompositeResolvedTargets(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newCompositeCtx := func(model string) (*gin.Context, *service.APIKey) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
		apiKey := &service.APIKey{Group: &service.Group{Platform: service.PlatformComposite, AllowMessagesDispatch: false}}
		ensureCompositeTargetPlatform(c, apiKey, model)
		return c, apiKey
	}

	// 解析到 grok/CN 目标：与对应独立分组同语义豁免。
	for _, model := range []string{"grok-4.3", "kimi-k2-thinking", "glm-5.2", "deepseek-v3.2"} {
		c, apiKey := newCompositeCtx(model)
		require.True(t, allowOpenAICompatibleMessagesDispatch(c, apiKey), "model=%s", model)
	}

	// 解析到 openai 目标：仍受开关控制（composite 被 sanitize 恒置 false ⇒ 拒绝）。
	c, apiKey := newCompositeCtx("gpt-5.5")
	require.False(t, allowOpenAICompatibleMessagesDispatch(c, apiKey))

	// 未解析出目标平台：保持拒绝，不放宽。
	cNone, _ := gin.CreateTestContext(httptest.NewRecorder())
	cNone.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	require.False(t, allowOpenAICompatibleMessagesDispatch(cNone,
		&service.APIKey{Group: &service.Group{Platform: service.PlatformComposite, AllowMessagesDispatch: false}}))
}

// composite 解析到 grok/CN 目标时，Group 级调度映射（gpt-5.x 默认值为 openai
// 专属）不得注入，模型改写完全交给账号级 model_mapping。
func TestResolveOpenAIMessagesDispatchMappedModel_CompositeCNTargetsSkipGroupMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, model := range []string{"kimi-k2-thinking", "glm-5.2", "deepseek-v3.2", "grok-4.3"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
		apiKey := &service.APIKey{Group: &service.Group{Platform: service.PlatformComposite}}
		ensureCompositeTargetPlatform(c, apiKey, model)

		require.Empty(t, resolveOpenAIMessagesDispatchMappedModel(c, apiKey, "claude-sonnet-4-5-20250929"), "model=%s", model)
	}
}
