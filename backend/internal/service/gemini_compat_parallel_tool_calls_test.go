package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type geminiStreamedToolCall struct {
	id   string
	name string
	args string
}

// 按 OpenAI 客户端的方式（按 tool_calls[].index 累积）还原流式 tool_calls。
func collectGeminiChatToolCalls(t *testing.T, raw string) []geminiStreamedToolCall {
	t.Helper()
	var calls []geminiStreamedToolCall
	for _, line := range strings.Split(raw, "\n") {
		payload, ok := strings.CutPrefix(line, "data: ")
		if !ok || payload == "[DONE]" {
			continue
		}
		for _, tc := range gjson.Get(payload, "choices.0.delta.tool_calls").Array() {
			idx := int(tc.Get("index").Int())
			for len(calls) <= idx {
				calls = append(calls, geminiStreamedToolCall{})
			}
			if id := tc.Get("id").String(); id != "" {
				calls[idx].id = id
			}
			if name := tc.Get("function.name").String(); name != "" {
				calls[idx].name = name
			}
			calls[idx].args += tc.Get("function.arguments").String()
		}
	}
	return calls
}

// 按 Anthropic content block index 还原流式 tool_use。
func collectGeminiAnthropicToolUses(t *testing.T, raw string) []geminiStreamedToolCall {
	t.Helper()
	byIndex := map[int]int{}
	var calls []geminiStreamedToolCall
	for _, chunk := range strings.Split(raw, "\n\n") {
		var dataLine string
		for _, line := range strings.Split(chunk, "\n") {
			if strings.HasPrefix(line, "data:") {
				dataLine = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}
		}
		if dataLine == "" {
			continue
		}
		ev := gjson.Parse(dataLine)
		idx := int(ev.Get("index").Int())
		switch ev.Get("type").String() {
		case "content_block_start":
			if ev.Get("content_block.type").String() == "tool_use" {
				byIndex[idx] = len(calls)
				calls = append(calls, geminiStreamedToolCall{
					id:   ev.Get("content_block.id").String(),
					name: ev.Get("content_block.name").String(),
				})
			}
		case "content_block_delta":
			if ev.Get("delta.type").String() == "input_json_delta" {
				pos, ok := byIndex[idx]
				require.Truef(t, ok, "input_json_delta for unknown block %d", idx)
				calls[pos].args += ev.Get("delta.partial_json").String()
			}
		}
	}
	return calls
}

func requireDistinctGeminiToolCalls(t *testing.T, calls []geminiStreamedToolCall, name string, wantArgs ...string) {
	t.Helper()
	require.Len(t, calls, len(wantArgs))
	seenIDs := map[string]bool{}
	for i, call := range calls {
		require.Equal(t, name, call.name)
		require.NotEmpty(t, call.id)
		require.Falsef(t, seenIDs[call.id], "duplicate tool call id %q", call.id)
		seenIDs[call.id] = true
		require.Truef(t, json.Valid([]byte(call.args)), "call %d arguments not valid JSON: %s", i, call.args)
		require.JSONEq(t, wantArgs[i], call.args)
	}
}

const geminiParallelSameNameInOneChunk = `data: {"candidates":[{"content":{"role":"model","parts":[` +
	`{"functionCall":{"name":"get_weather","args":{"city":"Beijing"}}},` +
	`{"functionCall":{"name":"get_weather","args":{"city":"Shanghai"}}}` +
	`]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}` + "\n\n"

const geminiParallelSameNameAcrossChunks = `data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"get_weather","args":{"city":"Beijing"}}}]}}]}` + "\n\n" +
	`data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"get_weather","args":{"city":"Shanghai"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}` + "\n\n"

const geminiParallelNoArgCalls = `data: {"candidates":[{"content":{"role":"model","parts":[` +
	`{"functionCall":{"name":"get_time","args":{}}},` +
	`{"functionCall":{"name":"get_time","args":{}}}` +
	`]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}` + "\n\n"

func runGeminiChatStream(t *testing.T, upstreamBody string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}}
	svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
	body := []byte(`{"model":"gemini-3.8-flash","stream":true,"messages":[{"role":"user","content":"weather in Beijing and Shanghai"}],` +
		`"tools":[{"type":"function","function":{"name":"get_weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}}]}`)
	c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)
	_, err := svc.ForwardAsChatCompletions(context.Background(), c, geminiCompatThinkingTestAccount(), body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	return recorder.Body.String()
}

func runGeminiMessagesStream(t *testing.T, upstreamBody string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	svc := &GeminiMessagesCompatService{}
	_, err := svc.handleStreamingResponse(c, resp, time.Now(), "claude-sonnet-4-5")
	require.NoError(t, err)
	return rec.Body.String()
}

func TestGeminiChatStream_ParallelSameNameToolCallsStaySeparate(t *testing.T) {
	for name, upstreamBody := range map[string]string{
		"one_chunk":     geminiParallelSameNameInOneChunk,
		"across_chunks": geminiParallelSameNameAcrossChunks,
	} {
		t.Run(name, func(t *testing.T) {
			out := runGeminiChatStream(t, upstreamBody)
			calls := collectGeminiChatToolCalls(t, out)
			requireDistinctGeminiToolCalls(t, calls, "get_weather", `{"city":"Beijing"}`, `{"city":"Shanghai"}`)
			require.Contains(t, out, `"finish_reason":"tool_calls"`)
		})
	}
}

func TestGeminiChatStream_ParallelNoArgCallsStaySeparate(t *testing.T) {
	out := runGeminiChatStream(t, geminiParallelNoArgCalls)
	calls := collectGeminiChatToolCalls(t, out)
	requireDistinctGeminiToolCalls(t, calls, "get_time", `{}`, `{}`)
}

func TestGeminiMessagesStream_ParallelSameNameToolUsesStaySeparate(t *testing.T) {
	for name, upstreamBody := range map[string]string{
		"one_chunk":     geminiParallelSameNameInOneChunk,
		"across_chunks": geminiParallelSameNameAcrossChunks,
		"no_arg":        geminiParallelNoArgCalls,
	} {
		t.Run(name, func(t *testing.T) {
			out := runGeminiMessagesStream(t, upstreamBody)
			calls := collectGeminiAnthropicToolUses(t, out)
			if name == "no_arg" {
				requireDistinctGeminiToolCalls(t, calls, "get_time", `{}`, `{}`)
			} else {
				requireDistinctGeminiToolCalls(t, calls, "get_weather", `{"city":"Beijing"}`, `{"city":"Shanghai"}`)
			}

			// 块必须严格串行：前一个 tool_use 关闭后才开下一个。
			open := -1
			for _, ev := range parseAnthropicContentBlockEvents(t, out) {
				switch ev.event {
				case "content_block_start":
					require.Equalf(t, -1, open, "block %d opened while block %d still open", ev.index, open)
					open = ev.index
				case "content_block_stop":
					require.Equal(t, open, ev.index)
					open = -1
				}
			}
			require.Equal(t, -1, open)
			require.Contains(t, out, `"stop_reason":"tool_use"`)
		})
	}
}

// 负向护栏：确实分片下发的参数（累积参数尚不是合法 JSON）仍并入同一个调用。
func TestGeminiMessagesStream_FragmentedArgsStillMerge(t *testing.T) {
	upstreamBody := `data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"get_weather","args":"{\"city\":"}}]}}]}` + "\n\n" +
		`data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"get_weather","args":"\"Beijing\"}"}}]},"finishReason":"STOP"}]}` + "\n\n"
	out := runGeminiMessagesStream(t, upstreamBody)
	calls := collectGeminiAnthropicToolUses(t, out)
	requireDistinctGeminiToolCalls(t, calls, "get_weather", `{"city":"Beijing"}`)
}

func TestGeminiFunctionCallContinuesOpenTool(t *testing.T) {
	cases := []struct {
		openName, seen, name string
		want                 bool
	}{
		{"get_weather", `{"city":"Beijing"}`, "get_weather", false}, // 完整参数后同名 = 新调用
		{"get_weather", `{}`, "get_weather", false},
		{"get_weather", `{"city":`, "get_weather", true}, // 分片中 = 续传
		{"get_weather", ``, "get_weather", true},
		{"get_weather", `{"city":`, "get_time", false}, // 换名一律新调用
	}
	for _, tc := range cases {
		require.Equalf(t, tc.want, geminiFunctionCallContinuesOpenTool(tc.openName, tc.seen, tc.name), "%+v", tc)
	}
}
