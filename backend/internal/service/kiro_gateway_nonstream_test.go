//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// --- I3：kiroAccumulateAnthropicResponse 单元测试 ---
//
// 直接手工构造事件切片喂给累积器，不需要起假上游/走完整的 event-stream
// 二进制编码——累积器是纯函数（输入 []apicompat.AnthropicStreamEvent，输出
// *apicompat.AnthropicResponse, error），这是它被设计成这样的直接目的。

// TestKiroAccumulateAnthropicResponse_TextToolUseAndStopReason 覆盖 I3 要求的
// 三个最小场景：一个文本块、一个由多个 input_json_delta 分片拼出来的
// tool_use 块、以及最终的 stop_reason。
func TestKiroAccumulateAnthropicResponse_TextToolUseAndStopReason(t *testing.T) {
	events := []apicompat.AnthropicStreamEvent{
		{
			Type: "message_start",
			Message: &apicompat.AnthropicResponse{
				ID:      "msg_1",
				Type:    "message",
				Role:    "assistant",
				Content: []apicompat.AnthropicContentBlock{},
				Model:   "claude-sonnet-4.6",
			},
		},
		{Type: "content_block_start", Index: intPtr(0), ContentBlock: &apicompat.AnthropicContentBlock{Type: "text"}},
		{Type: "content_block_delta", Index: intPtr(0), Delta: &apicompat.AnthropicDelta{Type: "text_delta", Text: "Hello, "}},
		{Type: "content_block_delta", Index: intPtr(0), Delta: &apicompat.AnthropicDelta{Type: "text_delta", Text: "world"}},
		{Type: "content_block_stop", Index: intPtr(0)},

		{Type: "content_block_start", Index: intPtr(1), ContentBlock: &apicompat.AnthropicContentBlock{
			Type: "tool_use", ID: "tu_1", Name: "Read", Input: json.RawMessage("{}"),
		}},
		{Type: "content_block_delta", Index: intPtr(1), Delta: &apicompat.AnthropicDelta{Type: "input_json_delta", PartialJSON: `{"pa`}},
		{Type: "content_block_delta", Index: intPtr(1), Delta: &apicompat.AnthropicDelta{Type: "input_json_delta", PartialJSON: `th":1}`}},
		{Type: "content_block_stop", Index: intPtr(1)},

		{Type: "message_delta", Delta: &apicompat.AnthropicDelta{StopReason: "tool_use"}},
		{Type: "message_stop"},
	}

	resp, err := kiroAccumulateAnthropicResponse(events)
	require.NoError(t, err)
	require.NotNil(t, resp)

	require.Equal(t, "msg_1", resp.ID)
	require.Equal(t, "message", resp.Type)
	require.Equal(t, "assistant", resp.Role)
	require.Equal(t, "claude-sonnet-4.6", resp.Model)

	require.Len(t, resp.Content, 2)

	require.Equal(t, "text", resp.Content[0].Type)
	require.Equal(t, "Hello, world", resp.Content[0].Text)

	require.Equal(t, "tool_use", resp.Content[1].Type)
	require.Equal(t, "tu_1", resp.Content[1].ID)
	require.Equal(t, "Read", resp.Content[1].Name)
	require.JSONEq(t, `{"path":1}`, string(resp.Content[1].Input))

	require.NotNil(t, resp.StopReason)
	require.Equal(t, "tool_use", *resp.StopReason)
}

// TestKiroAccumulateAnthropicResponse_ToolUseWithoutAnyInputDeltaDefaultsToEmptyObject
// 覆盖 I3 明确要求确认的边界情况：一个 tool_use 块从未收到过任何
// input_json_delta 分片就直接 content_block_stop（等价于 stream.go 里
// handleToolUse 对 tu.Input == "" 时的处理——从不追加分片，content_block_start
// 本身已经把 Input 设成了 "{}"）。累积器不能把它误判成"有分片但为空"从而
// 覆写出一个不同的值，也不能因为找不到 pendingToolInput 条目就 panic。
func TestKiroAccumulateAnthropicResponse_ToolUseWithoutAnyInputDeltaDefaultsToEmptyObject(t *testing.T) {
	events := []apicompat.AnthropicStreamEvent{
		{Type: "message_start", Message: &apicompat.AnthropicResponse{ID: "msg_2", Type: "message", Role: "assistant"}},
		{Type: "content_block_start", Index: intPtr(0), ContentBlock: &apicompat.AnthropicContentBlock{
			Type: "tool_use", ID: "tu_1", Name: "ListFiles", Input: json.RawMessage("{}"),
		}},
		{Type: "content_block_stop", Index: intPtr(0)},
		{Type: "message_delta", Delta: &apicompat.AnthropicDelta{StopReason: "tool_use"}},
		{Type: "message_stop"},
	}

	resp, err := kiroAccumulateAnthropicResponse(events)
	require.NoError(t, err)
	require.Len(t, resp.Content, 1)
	require.JSONEq(t, `{}`, string(resp.Content[0].Input))
}

// TestKiroAccumulateAnthropicResponse_ThinkingAndSignatureDeltas 覆盖假思考
// 场景下 thinking_delta/signature_delta 的累加。
func TestKiroAccumulateAnthropicResponse_ThinkingAndSignatureDeltas(t *testing.T) {
	events := []apicompat.AnthropicStreamEvent{
		{Type: "message_start", Message: &apicompat.AnthropicResponse{ID: "msg_3", Type: "message", Role: "assistant"}},
		{Type: "content_block_start", Index: intPtr(0), ContentBlock: &apicompat.AnthropicContentBlock{Type: "thinking"}},
		{Type: "content_block_delta", Index: intPtr(0), Delta: &apicompat.AnthropicDelta{Type: "thinking_delta", Thinking: "let me "}},
		{Type: "content_block_delta", Index: intPtr(0), Delta: &apicompat.AnthropicDelta{Type: "thinking_delta", Thinking: "think"}},
		{Type: "content_block_delta", Index: intPtr(0), Delta: &apicompat.AnthropicDelta{Type: "signature_delta", Signature: "sig-"}},
		{Type: "content_block_delta", Index: intPtr(0), Delta: &apicompat.AnthropicDelta{Type: "signature_delta", Signature: "abc"}},
		{Type: "content_block_stop", Index: intPtr(0)},
		{Type: "message_delta", Delta: &apicompat.AnthropicDelta{StopReason: "end_turn"}},
		{Type: "message_stop"},
	}

	resp, err := kiroAccumulateAnthropicResponse(events)
	require.NoError(t, err)
	require.Len(t, resp.Content, 1)
	require.Equal(t, "let me think", resp.Content[0].Thinking)
	require.Equal(t, "sig-abc", resp.Content[0].Signature)
}

// TestKiroAccumulateAnthropicResponse_StopSequenceIsCopied 覆盖 message_delta
// 里 StopSequence 的透传（不像 StopReason 是字符串，这里已经是指针，直接
// 赋值即可，但仍需确认累积器真的做了这一步而不是遗漏）。
func TestKiroAccumulateAnthropicResponse_StopSequenceIsCopied(t *testing.T) {
	stop := "STOP"
	events := []apicompat.AnthropicStreamEvent{
		{Type: "message_start", Message: &apicompat.AnthropicResponse{ID: "msg_4", Type: "message", Role: "assistant"}},
		{Type: "message_delta", Delta: &apicompat.AnthropicDelta{StopReason: "stop_sequence", StopSequence: &stop}},
		{Type: "message_stop"},
	}

	resp, err := kiroAccumulateAnthropicResponse(events)
	require.NoError(t, err)
	require.NotNil(t, resp.StopSequence)
	require.Equal(t, "STOP", *resp.StopSequence)
}

// TestKiroAccumulateAnthropicResponse_EmptyEventsIsDefensive 覆盖空输入——
// 不应该 panic，返回一个零值响应即可（实际生产路径中 Finalize 保证
// message_start 恒为第一个事件，这里只是防御性覆盖）。
func TestKiroAccumulateAnthropicResponse_EmptyEventsIsDefensive(t *testing.T) {
	resp, err := kiroAccumulateAnthropicResponse(nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Empty(t, resp.Content)
}

// TestKiroAccumulateAnthropicResponse_NoContentBlocksSerializesAsEmptyArray
// 是复查（scoped re-review）发现的 Important 级回归：message_start 之后没有
// 任何 content_block_* 事件时（上游空回复/静默截断——stream.go 的 Finalize
// 注释明确关心的场景），累积器必须把 Content 重置成空切片而不是 nil。
// AnthropicResponse.Content 的 json tag 没有 omitempty，nil 切片会序列化成
// "content":null，违反 Anthropic 线协议（客户端/SDK 期待 content 恒为数组，
// 即便是空数组）；流式路径的 message_start 发的就是 "content":[]
// （stream.go:217），非流式路径的行为必须与之一致。
func TestKiroAccumulateAnthropicResponse_NoContentBlocksSerializesAsEmptyArray(t *testing.T) {
	events := []apicompat.AnthropicStreamEvent{
		{Type: "message_start", Message: &apicompat.AnthropicResponse{ID: "msg_5", Type: "message", Role: "assistant"}},
		{Type: "message_delta", Delta: &apicompat.AnthropicDelta{StopReason: "end_turn"}},
		{Type: "message_stop"},
	}

	resp, err := kiroAccumulateAnthropicResponse(events)
	require.NoError(t, err)
	require.NotNil(t, resp.Content, "Content 必须是非 nil 的空切片，不能是 nil——否则序列化成 \"content\":null")
	require.Empty(t, resp.Content)

	body, err := json.Marshal(resp)
	require.NoError(t, err)
	require.Contains(t, string(body), `"content":[]`,
		"回归核心断言：线上协议必须是 \"content\":[]，不能是 \"content\":null")
	require.NotContains(t, string(body), `"content":null`)
}

// TestKiroAccumulateAnthropicResponse_TruncatedToolInputFallsBackToEmptyObject
// 是复查发现的 Minor 级回归：上游在 input_json_delta 中途断流时
// （feedTranslatorChunk 的 sawContent-优雅截断路径，流式侧本就能正常截断
// 返回），分片拼接结果是半截、无效的 JSON——之前的代码只在分片为空字符串时
// 才兜底成 "{}"，非空但无效的分片会原样塞进 json.RawMessage，导致
// nonStreamToClient 后续对整个响应做 json.Marshal 时失败，已经收到的正文/
// usage 全部作废。累积器必须像"从未收到任何分片"一样，对无效 JSON 也兜底成
// "{}"，保持与流式路径一致的"尽力截断"语义。
func TestKiroAccumulateAnthropicResponse_TruncatedToolInputFallsBackToEmptyObject(t *testing.T) {
	events := []apicompat.AnthropicStreamEvent{
		{Type: "message_start", Message: &apicompat.AnthropicResponse{ID: "msg_6", Type: "message", Role: "assistant"}},
		{Type: "content_block_start", Index: intPtr(0), ContentBlock: &apicompat.AnthropicContentBlock{
			Type: "tool_use", ID: "tu_1", Name: "Read", Input: json.RawMessage("{}"),
		}},
		// 断流：只收到半截 JSON，没有后续分片能补完它。
		{Type: "content_block_delta", Index: intPtr(0), Delta: &apicompat.AnthropicDelta{Type: "input_json_delta", PartialJSON: `{"pa`}},
		{Type: "content_block_stop", Index: intPtr(0)},
		{Type: "message_stop"},
	}

	resp, err := kiroAccumulateAnthropicResponse(events)
	require.NoError(t, err, "半截 JSON 必须被兜底吸收，不能让累积器本身报错")
	require.Len(t, resp.Content, 1)
	require.JSONEq(t, `{}`, string(resp.Content[0].Input))

	// 核心断言：兜底之后，最终响应必须能被完整 marshal——这正是原 bug
	// 丢失全部已收内容/用量的地方。
	_, err = json.Marshal(resp)
	require.NoError(t, err, "无效的 tool input 分片不应该拖垮整个响应的 marshal")
}

// --- I3：ForwardUpstream 端到端非流式集成测试 ---

// TestKiroForwardUpstreamNonStreamingReturnsSingleJSONResponse 是 I3 的核心
// 回归：inbound.Stream == false 时，ForwardUpstream 不应该像流式请求一样吐
// text/event-stream，而应该把上游流式响应折叠成一次性的 application/json。
func TestKiroForwardUpstreamNonStreamingReturnsSingleJSONResponse(t *testing.T) {
	frames := kiroTestConcatFrames(
		kiroTestEventFrame("assistantResponseEvent", `{"content":"Hello"}`),
		kiroTestEventFrame("assistantResponseEvent", `{"content":", world"}`),
		kiroTestEventFrame("metadataEvent", `{"stopReason":"end_turn"}`),
		kiroTestEventFrame("meteringEvent", `{"unit":"credit","usage":1,"cacheReadInputTokens":12,"cacheCreationInputTokens":3}`),
	)

	srv, calls := kiroTestFakeUpstream(t, func(int) (int, []byte) {
		return http.StatusOK, frames
	})

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(400)

	body := []byte(`{"model":"claude-sonnet-4-5-20250929","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"stream":false}`)

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))

	result, err := svc.ForwardUpstream(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Stream)
	require.EqualValues(t, 1, atomic.LoadInt32(calls))

	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	require.NotEqual(t, "text/event-stream", rec.Header().Get("Content-Type"))
	require.NotContains(t, rec.Body.String(), "event: ", "非流式响应不应该是 SSE 帧")

	var resp apicompat.AnthropicResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Content, 1)
	require.Equal(t, "text", resp.Content[0].Type)
	require.Equal(t, "Hello, world", resp.Content[0].Text)
	require.NotNil(t, resp.StopReason)
	require.Equal(t, "end_turn", *resp.StopReason)
	require.Equal(t, 12, resp.Usage.CacheReadInputTokens)
	require.Equal(t, 3, resp.Usage.CacheCreationInputTokens)

	require.Equal(t, 12, result.Usage.CacheReadInputTokens, "ForwardResult.Usage 必须和写给客户端 JSON 里的 usage 一致")
}
