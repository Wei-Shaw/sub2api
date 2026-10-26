package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func wsCapabilityGroup(allowlist service.GroupCapabilityAllowlist) *service.Group {
	return &service.Group{ID: 4201, Platform: service.PlatformOpenAI, CapabilityAllowlist: allowlist}
}

// 首帧携带被禁用的 Ultrafast：连接以 1008 关闭，不发上游。
func TestOpenAIResponsesWebSocket_FirstFrameCapabilityNotAllowedCloses(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeDedicated} {
		t.Run(mode, func(t *testing.T) {
			runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
				firstPayload:            `{"type":"response.create","model":"gpt-5.4","service_tier":"ultrafast","stream":false}`,
				group:                   wsCapabilityGroup(service.GroupCapabilityAllowlist{Enabled: true, ServiceTiers: []string{"priority"}}),
				ingressMode:             mode,
				firstFrameCloseExpected: true,
				closeReason:             `Capability "ultrafast" is not allowed for this group`,
			})
		})
	}
}

// 后续帧携带被禁用的 Daybreak：连接以 1008 关闭并给出同样的消息（只准入不重选）。
func TestOpenAIResponsesWebSocket_SubsequentTurnCapabilityNotAllowedCloses(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeDedicated} {
		t.Run(mode, func(t *testing.T) {
			runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
				firstPayload:            `{"type":"response.create","model":"gpt-5.4","stream":false}`,
				secondPayload:           `{"type":"response.create","model":"gpt-5.4","access_programs":{"cyber":"daybreak_blue"},"stream":false}`,
				group:                   wsCapabilityGroup(service.GroupCapabilityAllowlist{Enabled: true, ServiceTiers: []string{"priority"}}),
				ingressMode:             mode,
				secondTurnCloseExpected: true,
				closeReason:             `Capability "daybreak_blue" is not allowed for this group`,
			})
		})
	}
}

// 首帧允许的能力（API Key 账号不受过滤）：正常建连，需求字段原样到达上游。
func TestOpenAIResponsesWebSocket_AllowedCapabilityProceedsAndKeepsFields(t *testing.T) {
	got := runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload: `{"type":"response.create","model":"gpt-5.4","service_tier":"priority","access_programs":{"cyber":"daybreak_blue"},"stream":false}`,
		group:        wsCapabilityGroup(service.GroupCapabilityAllowlist{Enabled: true, CyberPrograms: []string{"daybreak_blue"}, ServiceTiers: []string{"priority"}}),
	})
	require.Len(t, got.clientEvents, 1)
	require.Equal(t, "daybreak_blue", gjson.GetBytes(got.upstreamFirstPayload, "access_programs.cyber").String())
	require.Equal(t, "priority", gjson.GetBytes(got.upstreamFirstPayload, "service_tier").String())
}

// 首帧需求 Blue 而分组内唯一账号是无记录的 OAuth 账号：先收到 capability_not_available
// error 事件，再以策略违规关闭。
func TestOpenAIResponsesWebSocket_FirstFrameCapabilityUnavailableWritesErrorThenCloses(t *testing.T) {
	runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload: `{"type":"response.create","model":"gpt-5.4","access_programs":{"cyber":"daybreak_blue"},"stream":false}`,
		group:        wsCapabilityGroup(service.GroupCapabilityAllowlist{}),
		accountType:  service.AccountTypeOAuth,
		accountExtra: map[string]any{
			"openai_oauth_responses_websockets_v2_enabled": true,
			"openai_oauth_responses_websockets_v2_mode":    service.OpenAIWSIngressModePassthrough,
		},
		firstFrameCloseExpected:  true,
		firstFrameErrorEventCode: "capability_not_available",
		closeReason:              `No account in this group supports capability "daybreak_blue"`,
	})
}
