package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const wsModelConflictCloseReason = "declares model more than once with different values"

// 没有开启白名单的分组：首帧里重复 model 键取值不一致（计费按第一个、上游可能
// 按最后一个绑定）必须以 1008 关闭，帧不到达上游。
func TestOpenAIResponsesWebSocket_FirstFrameConflictingModelKeysRejectedWithoutAllowlist(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeDedicated} {
		t.Run(mode, func(t *testing.T) {
			runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
				firstPayload:            `{"type":"response.create","model":"gpt-5.4-nano","model":"gpt-5.4","stream":false}`,
				group:                   wsAllowlistGroup(false),
				ingressMode:             mode,
				firstFrameCloseExpected: true,
				closeReason:             wsModelConflictCloseReason,
			})
		})
	}
}

// 大小写变体键同样会被 encoding/json 绑定到同一个字段。
func TestOpenAIResponsesWebSocket_FirstFrameConflictingCaseVariantModelKeyRejected(t *testing.T) {
	runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:            `{"type":"response.create","model":"gpt-5.4-nano","Model":"gpt-5.4","stream":false}`,
		group:                   wsAllowlistGroup(false),
		ingressMode:             service.OpenAIWSIngressModePassthrough,
		firstFrameCloseExpected: true,
		closeReason:             wsModelConflictCloseReason,
	})
}

// 白名单开启且两个模型都在名单内：白名单放行，模型歧义检查仍要拒绝。
func TestOpenAIResponsesWebSocket_FirstFrameConflictingAllowlistedModelKeysRejected(t *testing.T) {
	runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:            `{"type":"response.create","model":"gpt-5.4-nano","model":"gpt-5.4","stream":false}`,
		group:                   wsAllowlistGroup(true, "gpt-5.4-nano", "gpt-5.4"),
		ingressMode:             service.OpenAIWSIngressModePassthrough,
		firstFrameCloseExpected: true,
		closeReason:             wsModelConflictCloseReason,
	})
}

// 后续 turn 的帧同样检查。
func TestOpenAIResponsesWebSocket_SubsequentTurnConflictingModelKeysRejectedWithoutAllowlist(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeDedicated} {
		t.Run(mode, func(t *testing.T) {
			runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
				firstPayload:            `{"type":"response.create","model":"gpt-5.4","stream":false}`,
				secondPayload:           `{"type":"response.create","model":"gpt-5.4-nano","model":"gpt-5.4","stream":false}`,
				group:                   wsAllowlistGroup(false),
				ingressMode:             mode,
				secondTurnCloseExpected: true,
				closeReason:             wsModelConflictCloseReason,
			})
		})
	}
}

// 取值相同的重复 model 键不受影响，两个 turn 正常完成。
func TestOpenAIResponsesWebSocket_IdenticalDuplicateModelKeysAllowedWithoutAllowlist(t *testing.T) {
	got := runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
		firstPayload:  `{"type":"response.create","model":"gpt-5.4","model":"gpt-5.4","stream":false}`,
		secondPayload: `{"type":"response.create","model":"gpt-5.4","model":"gpt-5.4","stream":false}`,
		group:         wsAllowlistGroup(false),
	})
	if len(got.clientEvents) != 2 {
		t.Fatalf("expected two completed events, got %d", len(got.clientEvents))
	}
}
