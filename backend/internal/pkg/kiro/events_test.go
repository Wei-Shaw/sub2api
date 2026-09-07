package kiro

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func frameOf(t *testing.T, eventType string, payload string) *Frame {
	t.Helper()
	return &Frame{
		Headers: map[string]HeaderValue{
			":message-type": {Type: hdrString, Str: "event"},
			":event-type":   {Type: hdrString, Str: eventType},
		},
		Payload: []byte(payload),
	}
}

func TestParseEventAssistantResponse(t *testing.T) {
	t.Parallel()

	ev, err := ParseEvent(frameOf(t, "assistantResponseEvent", `{"content":"hello","modelId":"claude-sonnet-4.6"}`))
	require.NoError(t, err)
	require.Equal(t, EventAssistantResponse, ev.Kind)
	require.NotNil(t, ev.Assistant)
	require.Equal(t, "hello", ev.Assistant.Content)
	require.Equal(t, "claude-sonnet-4.6", ev.Assistant.ModelID)
}

func TestParseEventToolUsePartialAndStop(t *testing.T) {
	t.Parallel()

	ev, err := ParseEvent(frameOf(t, "toolUseEvent", `{"name":"Read","toolUseId":"tu_1","input":"{\"pa","stop":false}`))
	require.NoError(t, err)
	require.Equal(t, EventToolUse, ev.Kind)
	require.Equal(t, "Read", ev.ToolUse.Name)
	require.Equal(t, "tu_1", ev.ToolUse.ToolUseID)
	require.Equal(t, `{"pa`, ev.ToolUse.Input)
	require.False(t, ev.ToolUse.Stop)

	ev, err = ParseEvent(frameOf(t, "toolUseEvent", `{"name":"Read","toolUseId":"tu_1","input":"th\":1}","stop":true}`))
	require.NoError(t, err)
	require.True(t, ev.ToolUse.Stop)
}

// TestParseEventMetadataStopReason 是回归测试：metadataEvent 只有 Kiro-Go 处理了，
// kiro2cc-proxy 的事件枚举里没有它。漏掉会导致 stop_reason 永远是 end_turn。
func TestParseEventMetadataStopReason(t *testing.T) {
	t.Parallel()

	ev, err := ParseEvent(frameOf(t, "metadataEvent", `{"stopReason":"max_tokens"}`))
	require.NoError(t, err)
	require.Equal(t, EventMetadata, ev.Kind)
	require.Equal(t, "max_tokens", ev.Metadata.StopReason)

	// 蛇形键名也要认。
	ev, err = ParseEvent(frameOf(t, "metadataEvent", `{"stop_reason":"stop_sequence"}`))
	require.NoError(t, err)
	require.Equal(t, "stop_sequence", ev.Metadata.StopReason)
}

func TestParseEventMetering(t *testing.T) {
	t.Parallel()

	ev, err := ParseEvent(frameOf(t, "meteringEvent",
		`{"unit":"credit","usage":1.5,"cacheReadInputTokens":120,"cacheCreationInputTokens":30}`))
	require.NoError(t, err)
	require.Equal(t, EventMetering, ev.Kind)
	require.InDelta(t, 1.5, ev.Metering.Usage, 1e-9)
	require.Equal(t, 120, ev.Metering.CacheReadInputTokens)
	require.Equal(t, 30, ev.Metering.CacheCreationInputTokens)
}

func TestParseEventContextUsageAndCodeReference(t *testing.T) {
	t.Parallel()

	ev, err := ParseEvent(frameOf(t, "contextUsageEvent", `{"contextUsagePercentage":42.5}`))
	require.NoError(t, err)
	require.Equal(t, EventContextUsage, ev.Kind)
	require.InDelta(t, 42.5, ev.ContextUsage.Percentage, 1e-9)

	ev, err = ParseEvent(frameOf(t, "codeReferenceEvent", `{"references":[]}`))
	require.NoError(t, err)
	require.Equal(t, EventCodeReference, ev.Kind)
}

func TestParseEventException(t *testing.T) {
	t.Parallel()

	f := &Frame{
		Headers: map[string]HeaderValue{
			":message-type":   {Type: hdrString, Str: "exception"},
			":exception-type": {Type: hdrString, Str: "ThrottlingException"},
		},
		Payload: []byte(`{"message":"Too many requests"}`),
	}

	ev, err := ParseEvent(f)
	require.NoError(t, err)
	require.Equal(t, EventException, ev.Kind)
	require.Equal(t, "ThrottlingException", ev.Exception.Type)
	require.Equal(t, "Too many requests", ev.Exception.Message)
}

// TestParseEventExceptionTypeNotOverwrittenByPayload 保护 Exception.Type 的 json:"-" 标签：
// Type 来自 :exception-type 帧头，绝不能被 payload 里的同名字段覆写。
func TestParseEventExceptionTypeNotOverwrittenByPayload(t *testing.T) {
	t.Parallel()

	f := &Frame{
		Headers: map[string]HeaderValue{
			":message-type":   {Type: hdrString, Str: "exception"},
			":exception-type": {Type: hdrString, Str: "ThrottlingException"},
		},
		Payload: []byte(`{"type":"AttackerControlledType","message":"slow down"}`),
	}

	ev, err := ParseEvent(f)
	require.NoError(t, err)
	require.Equal(t, "ThrottlingException", ev.Exception.Type,
		"Type 必须来自帧头，不得被 payload 覆写")
	require.Equal(t, "slow down", ev.Exception.Message)
}

func TestParseEventUnknownIsNotAnError(t *testing.T) {
	t.Parallel()

	ev, err := ParseEvent(frameOf(t, "someFutureEvent", `{"whatever":1}`))
	require.NoError(t, err)
	require.Equal(t, EventUnknown, ev.Kind)
}

func TestParseEventMalformedPayload(t *testing.T) {
	t.Parallel()

	_, err := ParseEvent(frameOf(t, "assistantResponseEvent", `{not json`))
	require.Error(t, err)
}
