package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func wsAllowlistGroup(enabled bool, models ...string) *service.Group {
	return &service.Group{
		ID:       4201,
		Platform: service.PlatformOpenAI,
		ModelAllowlist: service.GroupModelAllowlist{
			Enabled: enabled,
			Models:  models,
		},
	}
}

// 首帧拒绝：模型不在白名单时连接被 1008 关闭（passthrough relay 模式）。
func TestOpenAIResponsesWebSocket_FirstFrameModelNotAllowedCloses_Passthrough(t *testing.T) {
	runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:            `{"type":"response.create","model":"gpt-4.1","stream":false}`,
		group:                   wsAllowlistGroup(true, "gpt-5.4"),
		ingressMode:             service.OpenAIWSIngressModePassthrough,
		firstFrameCloseExpected: true,
	})
}

// 首帧拒绝：原生 ingress 模式。
func TestOpenAIResponsesWebSocket_FirstFrameModelNotAllowedCloses_NativeIngress(t *testing.T) {
	runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:            `{"type":"response.create","model":"gpt-4.1","stream":false}`,
		group:                   wsAllowlistGroup(true, "gpt-5.4"),
		ingressMode:             service.OpenAIWSIngressModeDedicated,
		firstFrameCloseExpected: true,
	})
}

// 首帧命中白名单：连接正常建立并收到 response.completed（passthrough）。
func TestOpenAIResponsesWebSocket_FirstFrameAllowlistedModelProceeds(t *testing.T) {
	got := runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload: `{"type":"response.create","model":"gpt-5.4","stream":false}`,
		group:        wsAllowlistGroup(true, "gpt-5.4"),
	})
	if len(got.clientEvents) != 1 {
		t.Fatalf("expected one completed event, got %d", len(got.clientEvents))
	}
}

// 后续 turn 切换到白名单外的模型：连接被 1008 关闭（passthrough relay 模式）。
func TestOpenAIResponsesWebSocket_SubsequentTurnModelNotAllowedCloses_Passthrough(t *testing.T) {
	runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:            `{"type":"response.create","model":"gpt-5.4","stream":false}`,
		secondPayload:           `{"type":"response.create","model":"gpt-4.1","stream":false}`,
		group:                   wsAllowlistGroup(true, "gpt-5.4"),
		ingressMode:             service.OpenAIWSIngressModePassthrough,
		secondTurnCloseExpected: true,
	})
}

// 后续 turn 切换到白名单外的模型：连接被 1008 关闭（原生 ingress 模式）。
func TestOpenAIResponsesWebSocket_SubsequentTurnModelNotAllowedCloses_NativeIngress(t *testing.T) {
	runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:            `{"type":"response.create","model":"gpt-5.4","stream":false}`,
		secondPayload:           `{"type":"response.create","model":"gpt-4.1","stream":false}`,
		group:                   wsAllowlistGroup(true, "gpt-5.4"),
		ingressMode:             service.OpenAIWSIngressModeDedicated,
		secondTurnCloseExpected: true,
	})
}

// 后续 turn 省略 model：沿用会话初始模型，白名单校验通过。
func TestOpenAIResponsesWebSocket_SubsequentTurnOmittedModelUsesSessionModel(t *testing.T) {
	got := runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:  `{"type":"response.create","model":"gpt-5.4","stream":false}`,
		secondPayload: `{"type":"response.create","stream":false}`,
		group:         wsAllowlistGroup(true, "gpt-5.4"),
	})
	if len(got.clientEvents) != 2 {
		t.Fatalf("expected two completed events, got %d", len(got.clientEvents))
	}
	if len(got.upstreamPayloads) != 2 {
		t.Fatalf("expected two upstream frames, got %d", len(got.upstreamPayloads))
	}
}

// 白名单关闭：不受任何影响。
func TestOpenAIResponsesWebSocket_DisabledAllowlistDoesNotInterfere(t *testing.T) {
	got := runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:  `{"type":"response.create","model":"gpt-4.1","stream":false}`,
		secondPayload: `{"type":"response.create","model":"gpt-5.4","stream":false}`,
		group:         wsAllowlistGroup(false, "gpt-5.4"),
	})
	if len(got.clientEvents) != 2 {
		t.Fatalf("expected two completed events, got %d", len(got.clientEvents))
	}
}

// 首帧含重复 model 键（首个在白名单、末个不在）：passthrough 会原样转发两个字段，
// 上游按末值绑定时得到禁用模型，必须在入口拒绝。
func TestOpenAIResponsesWebSocket_FirstFrameDuplicateModelKeysRejected(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeDedicated} {
		t.Run(mode, func(t *testing.T) {
			runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
				firstPayload:            `{"type":"response.create","model":"gpt-5.4","model":"gpt-4.1","stream":false}`,
				group:                   wsAllowlistGroup(true, "gpt-5.4"),
				ingressMode:             mode,
				firstFrameCloseExpected: true,
			})
		})
	}
}

// 首帧混用大小写变体键（{"model":"allowed","Model":"blocked"}）：绑定系上游
// 大小写不敏感且末值生效会绑定 blocked，必须在入口拒绝。仅含变体键的帧则会在
// 「model is required」处更早被关闭。
func TestOpenAIResponsesWebSocket_FirstFrameCaseVariantModelKeyRejected(t *testing.T) {
	runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:            `{"type":"response.create","model":"gpt-5.4","Model":"gpt-4.1","stream":false}`,
		group:                   wsAllowlistGroup(true, "gpt-5.4"),
		ingressMode:             service.OpenAIWSIngressModePassthrough,
		firstFrameCloseExpected: true,
	})
}

// 后续 turn 含重复 model 键（首个在白名单、末个不在）：拒绝并关闭连接。
func TestOpenAIResponsesWebSocket_SubsequentTurnDuplicateModelKeysRejected(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeDedicated} {
		t.Run(mode, func(t *testing.T) {
			runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
				firstPayload:            `{"type":"response.create","model":"gpt-5.4","stream":false}`,
				secondPayload:           `{"type":"response.create","model":"gpt-5.4","model":"gpt-4.1","stream":false}`,
				group:                   wsAllowlistGroup(true, "gpt-5.4"),
				ingressMode:             mode,
				secondTurnCloseExpected: true,
			})
		})
	}
}

// 重复但同值的 model 键不误伤，连接正常完成两个 turn。
func TestOpenAIResponsesWebSocket_DuplicateIdenticalModelKeysAllowed(t *testing.T) {
	got := runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:  `{"type":"response.create","model":"gpt-5.4","model":"gpt-5.4","stream":false}`,
		secondPayload: `{"type":"response.create","model":"gpt-5.4","stream":false}`,
		group:         wsAllowlistGroup(true, "gpt-5.4"),
	})
	if len(got.clientEvents) != 2 {
		t.Fatalf("expected two completed events, got %d", len(got.clientEvents))
	}
}

// session.update 轮换绕过：首帧用白名单内模型建立会话，session.update 把会话
// 模型改为白名单外的模型，随后 response.create 携带嵌套 session.model（白名单
// 内）让帧内候选非空。实际生效模型（轮换后的会话模型）必须始终参与校验。
func TestOpenAIResponsesWebSocket_SessionUpdateRotationBypassRejected(t *testing.T) {
	runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:            `{"type":"response.create","model":"gpt-5.4","stream":false}`,
		midPayload:              `{"type":"session.update","session":{"model":"gpt-4.1"}}`,
		secondPayload:           `{"type":"response.create","session":{"model":"gpt-5.4"},"stream":false}`,
		group:                   wsAllowlistGroup(true, "gpt-5.4"),
		ingressMode:             service.OpenAIWSIngressModePassthrough,
		secondTurnCloseExpected: true,
	})
}

// 轮换后的会话模型本身在白名单内时，省略 model 的后续 turn 正常放行。
func TestOpenAIResponsesWebSocket_SessionUpdateToAllowedModelStillWorks(t *testing.T) {
	got := runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:  `{"type":"response.create","model":"gpt-5.4","stream":false}`,
		midPayload:    `{"type":"session.update","session":{"model":"gpt-5.4"}}`,
		secondPayload: `{"type":"response.create","stream":false}`,
		group:         wsAllowlistGroup(true, "gpt-5.4"),
	})
	if len(got.clientEvents) < 2 {
		t.Fatalf("expected at least two completed events, got %d", len(got.clientEvents))
	}
}
