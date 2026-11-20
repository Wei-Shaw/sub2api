package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const wsModelConflictCloseReason = "more than once"

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

// 取值相同的重复键同样拒绝：网关的 sjson 改写（模型映射、推理强度降级）只改
// 首个，改写后两者就会分叉。首帧与后续 turn 都要关闭连接。
func TestOpenAIResponsesWebSocket_IdenticalDuplicateKeysRejectedWithoutAllowlist(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeDedicated} {
		t.Run(mode+"/first_frame", func(t *testing.T) {
			runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
				firstPayload:            `{"type":"response.create","model":"gpt-5.4","model":"gpt-5.4","stream":false}`,
				group:                   wsAllowlistGroup(false),
				ingressMode:             mode,
				firstFrameCloseExpected: true,
				closeReason:             `declares field "model" more than once`,
			})
		})
		t.Run(mode+"/subsequent_turn", func(t *testing.T) {
			runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
				firstPayload:            `{"type":"response.create","model":"gpt-5.4","stream":false}`,
				secondPayload:           `{"type":"response.create","model":"gpt-5.4","reasoning":{"effort":"xhigh","effort":"xhigh"},"stream":false}`,
				group:                   wsAllowlistGroup(false),
				ingressMode:             mode,
				secondTurnCloseExpected: true,
				closeReason:             `declares field "reasoning.effort" more than once`,
			})
		})
	}
}

// 非 model 字段同样参与计费与策略：首帧 service_tier、后续 turn 嵌套的
// reasoning.effort 取值不一致都必须关闭连接。
func TestOpenAIResponsesWebSocket_ConflictingBillingFieldsRejected(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeDedicated} {
		t.Run(mode+"/first_frame_service_tier", func(t *testing.T) {
			runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
				firstPayload:            `{"type":"response.create","model":"gpt-5.4","service_tier":"flex","stream":false,"service_tier":"priority"}`,
				group:                   wsAllowlistGroup(false),
				ingressMode:             mode,
				firstFrameCloseExpected: true,
				closeReason:             `declares field "service_tier" more than once`,
			})
		})
		t.Run(mode+"/subsequent_turn_reasoning_effort", func(t *testing.T) {
			runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
				firstPayload:            `{"type":"response.create","model":"gpt-5.4","stream":false}`,
				secondPayload:           `{"type":"response.create","model":"gpt-5.4","reasoning":{"effort":"low","effort":"xhigh"},"stream":false}`,
				group:                   wsAllowlistGroup(false),
				ingressMode:             mode,
				secondTurnCloseExpected: true,
				closeReason:             `declares field "reasoning.effort" more than once`,
			})
		})
	}
}
