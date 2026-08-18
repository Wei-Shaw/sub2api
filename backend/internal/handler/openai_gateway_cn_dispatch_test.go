package handler

// CN 分组 /v1/messages 调度闸门回归（修复:正常途径创建的 CN 分组曾恒 403）：
// sanitizeGroupMessagesDispatchFields 对非 openai 平台强制 AllowMessagesDispatch
// =false，故 CN 分组必须与 grok 一样在闸门处豁免，否则原生 Anthropic 直通
//（Claude Code 主用例）永远不可达。

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAllowOpenAICompatibleMessagesDispatch_CNProvidersExempt(t *testing.T) {
	require.True(t, allowOpenAICompatibleMessagesDispatch(nil), "无 key 保持放行")

	for _, platform := range []string{service.PlatformKimi, service.PlatformZhipu, service.PlatformDeepseek, service.PlatformGrok} {
		apiKey := &service.APIKey{Group: &service.Group{Platform: platform, AllowMessagesDispatch: false}}
		require.True(t, allowOpenAICompatibleMessagesDispatch(apiKey),
			"%s 分组必须豁免 allow_messages_dispatch 闸门", platform)
	}

	// 非回归：openai 分组仍受开关控制。
	openaiOff := &service.APIKey{Group: &service.Group{Platform: service.PlatformOpenAI, AllowMessagesDispatch: false}}
	require.False(t, allowOpenAICompatibleMessagesDispatch(openaiOff))
	openaiOn := &service.APIKey{Group: &service.Group{Platform: service.PlatformOpenAI, AllowMessagesDispatch: true}}
	require.True(t, allowOpenAICompatibleMessagesDispatch(openaiOn))
}
