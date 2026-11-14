//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const bufferedParityMessagesBody = `{"model":"gpt-5.5","max_tokens":4096,"stream":%t,"thinking":{"type":"enabled","budget_tokens":2048},"tools":[{"name":"record_version","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"USER_PROMPT_PLACEHOLDER"}]}`

const bufferedParityChatBody = `{"model":"gpt-5.5","stream":%t,"reasoning_effort":"medium","tools":[{"type":"function","function":{"name":"record_version","parameters":{"type":"object"}}}],"messages":[{"role":"user","content":"USER_PROMPT_PLACEHOLDER"}]}`

func bufferedParityReasoning(index int, id, text, encrypted string) []string {
	summary := `[]`
	if text != "" {
		summary = fmt.Sprintf(`[{"type":"summary_text","text":%q}]`, text)
	}
	events := []string{
		fmt.Sprintf(`{"type":"response.output_item.added","output_index":%d,"item":{"type":"reasoning","id":%q,"summary":[]}}`, index, id),
	}
	if text != "" {
		events = append(events,
			fmt.Sprintf(`{"type":"response.reasoning_summary_part.added","output_index":%d,"summary_index":0,"part":{"type":"summary_text","text":""}}`, index),
			fmt.Sprintf(`{"type":"response.reasoning_summary_text.delta","output_index":%d,"summary_index":0,"delta":%q}`, index, text),
			fmt.Sprintf(`{"type":"response.reasoning_summary_text.done","output_index":%d,"summary_index":0,"text":%q}`, index, text),
		)
	}
	return append(events, fmt.Sprintf(`{"type":"response.output_item.done","output_index":%d,"item":{"type":"reasoning","id":%q,"summary":%s,"encrypted_content":%q}}`, index, id, summary, encrypted))
}

func bufferedParityWebSearch(index int, id, query string) []string {
	return []string{
		fmt.Sprintf(`{"type":"response.output_item.added","output_index":%d,"item":{"type":"web_search_call","id":%q,"status":"in_progress"}}`, index, id),
		fmt.Sprintf(`{"type":"response.web_search_call.in_progress","output_index":%d,"item_id":%q}`, index, id),
		fmt.Sprintf(`{"type":"response.web_search_call.searching","output_index":%d,"item_id":%q}`, index, id),
		fmt.Sprintf(`{"type":"response.web_search_call.completed","output_index":%d,"item_id":%q}`, index, id),
		fmt.Sprintf(`{"type":"response.output_item.done","output_index":%d,"item":{"type":"web_search_call","id":%q,"status":"completed","action":{"type":"search","query":%q}}}`, index, id, query),
	}
}

func bufferedParityFunctionCall(index int) []string {
	return []string{
		fmt.Sprintf(`{"type":"response.output_item.added","output_index":%d,"item":{"type":"function_call","id":"fc_1","call_id":"call_c","name":"record_version","arguments":""}}`, index),
		fmt.Sprintf(`{"type":"response.function_call_arguments.delta","output_index":%d,"item_id":"fc_1","delta":"{\"version\""}`, index),
		fmt.Sprintf(`{"type":"response.function_call_arguments.delta","output_index":%d,"item_id":"fc_1","delta":":\"1.25\"}"}`, index),
		fmt.Sprintf(`{"type":"response.function_call_arguments.done","output_index":%d,"item_id":"fc_1","arguments":"{\"version\":\"1.25\"}"}`, index),
		fmt.Sprintf(`{"type":"response.output_item.done","output_index":%d,"item":{"type":"function_call","id":"fc_1","call_id":"call_c","name":"record_version","arguments":"{\"version\":\"1.25\"}","status":"completed"}}`, index),
	}
}

func bufferedParityMessage(index int, text string) []string {
	content := `[{"type":"output_text","text":""}]`
	events := []string{
		fmt.Sprintf(`{"type":"response.output_item.added","output_index":%d,"item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","phase":"final_answer","content":[]}}`, index),
		fmt.Sprintf(`{"type":"response.content_part.added","output_index":%d,"content_index":0,"part":{"type":"output_text","text":""}}`, index),
	}
	if text != "" {
		content = fmt.Sprintf(`[{"type":"output_text","text":%q}]`, text)
		events = append(events,
			fmt.Sprintf(`{"type":"response.output_text.delta","output_index":%d,"content_index":0,"delta":%q}`, index, text),
			fmt.Sprintf(`{"type":"response.output_text.done","output_index":%d,"content_index":0,"text":%q}`, index, text),
		)
	}
	return append(events,
		fmt.Sprintf(`{"type":"response.content_part.done","output_index":%d,"content_index":0,"part":{"type":"output_text","text":%q}}`, index, text),
		fmt.Sprintf(`{"type":"response.output_item.done","output_index":%d,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","phase":"final_answer","content":%s}}`, index, content),
	)
}

func bufferedParityStream(terminalOutput string, groups ...[]string) string {
	events := []string{bufferedHostedItemsCreated}
	for _, group := range groups {
		events = append(events, group...)
	}
	events = append(events, fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_1","status":"completed","output":%s,"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}`, terminalOutput))
	return bufferedItemsSSE(events...)
}

// bufferedParityBlocks reduces an Anthropic content array to one line per block.
func bufferedParityBlocks(blocks []gjson.Result) []string {
	var lines []string
	for _, block := range blocks {
		switch block.Get("type").String() {
		case "thinking":
			lines = append(lines, fmt.Sprintf("thinking %q signature=%q", block.Get("thinking").String(), block.Get("signature").String()))
		case "text":
			lines = append(lines, fmt.Sprintf("text %q", block.Get("text").String()))
		case "tool_use":
			lines = append(lines, fmt.Sprintf("tool_use %s %s %s", block.Get("id").String(), block.Get("name").String(), block.Get("input").Raw))
		case "server_tool_use":
			lines = append(lines, fmt.Sprintf("server_tool_use %s %s", block.Get("id").String(), block.Get("input").Raw))
		case "web_search_tool_result":
			lines = append(lines, fmt.Sprintf("web_search_tool_result %s", block.Get("tool_use_id").String()))
		default:
			lines = append(lines, "other "+block.Get("type").String())
		}
	}
	return lines
}

// bufferedParityStreamBlocks folds an Anthropic SSE body back into content blocks.
func bufferedParityStreamBlocks(t *testing.T, sse string) []string {
	t.Helper()
	type streamed struct {
		kind, id, name, toolUseID, thinking, text, signature, input string
	}
	byIndex := map[int64]*streamed{}
	for _, line := range strings.Split(sse, "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok || !gjson.Valid(data) {
			continue
		}
		index := gjson.Get(data, "index").Int()
		switch gjson.Get(data, "type").String() {
		case "content_block_start":
			block := gjson.Get(data, "content_block")
			byIndex[index] = &streamed{
				kind: block.Get("type").String(), id: block.Get("id").String(), name: block.Get("name").String(),
				toolUseID: block.Get("tool_use_id").String(), input: block.Get("input").Raw,
			}
		case "content_block_delta":
			block := byIndex[index]
			require.NotNil(t, block, data)
			delta := gjson.Get(data, "delta")
			switch delta.Get("type").String() {
			case "thinking_delta":
				block.thinking += delta.Get("thinking").String()
			case "text_delta":
				block.text += delta.Get("text").String()
			case "signature_delta":
				block.signature = delta.Get("signature").String()
			case "input_json_delta":
				if block.input == "{}" {
					block.input = ""
				}
				block.input += delta.Get("partial_json").String()
			}
		}
	}
	indexes := make([]int64, 0, len(byIndex))
	for index := range byIndex {
		indexes = append(indexes, index)
	}
	sort.Slice(indexes, func(i, j int) bool { return indexes[i] < indexes[j] })
	var lines []string
	for _, index := range indexes {
		block := byIndex[index]
		switch block.kind {
		case "thinking":
			lines = append(lines, fmt.Sprintf("thinking %q signature=%q", block.thinking, block.signature))
		case "text":
			lines = append(lines, fmt.Sprintf("text %q", block.text))
		case "tool_use":
			lines = append(lines, fmt.Sprintf("tool_use %s %s %s", block.id, block.name, block.input))
		case "server_tool_use":
			lines = append(lines, fmt.Sprintf("server_tool_use %s %s", block.id, block.input))
		case "web_search_tool_result":
			lines = append(lines, fmt.Sprintf("web_search_tool_result %s", block.toolUseID))
		default:
			lines = append(lines, "other "+block.kind)
		}
	}
	return lines
}

// bufferedParityChat reduces a Chat Completions answer to the fields a client reads.
func bufferedParityChatSummary(reasoning, content string, calls []string, finish string) string {
	return fmt.Sprintf("reasoning=%q content=%q tool_calls=%v finish_reason=%s", reasoning, content, calls, finish)
}

func bufferedParityChatNonStream(body string) string {
	message := gjson.Get(body, "choices.0.message")
	var calls []string
	for _, call := range message.Get("tool_calls").Array() {
		calls = append(calls, fmt.Sprintf("%s %s %s", call.Get("id").String(), call.Get("function.name").String(), call.Get("function.arguments").String()))
	}
	return bufferedParityChatSummary(message.Get("reasoning_content").String(), message.Get("content").String(), calls, gjson.Get(body, "choices.0.finish_reason").String())
}

func bufferedParityChatStream(sse string) string {
	var reasoning, content, finish string
	type call struct{ id, name, args string }
	calls := map[int64]*call{}
	for _, line := range strings.Split(sse, "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok || !gjson.Valid(data) {
			continue
		}
		choice := gjson.Get(data, "choices.0")
		reasoning += choice.Get("delta.reasoning_content").String()
		content += choice.Get("delta.content").String()
		if reason := choice.Get("finish_reason"); reason.Exists() && reason.Type != gjson.Null {
			finish = reason.String()
		}
		for _, tc := range choice.Get("delta.tool_calls").Array() {
			index := tc.Get("index").Int()
			if calls[index] == nil {
				calls[index] = &call{}
			}
			if id := tc.Get("id").String(); id != "" {
				calls[index].id = id
			}
			if name := tc.Get("function.name").String(); name != "" {
				calls[index].name = name
			}
			calls[index].args += tc.Get("function.arguments").String()
		}
	}
	indexes := make([]int64, 0, len(calls))
	for index := range calls {
		indexes = append(indexes, index)
	}
	sort.Slice(indexes, func(i, j int) bool { return indexes[i] < indexes[j] })
	var lines []string
	for _, index := range indexes {
		lines = append(lines, fmt.Sprintf("%s %s %s", calls[index].id, calls[index].name, calls[index].args))
	}
	return bufferedParityChatSummary(reasoning, content, lines, finish)
}

func forwardBufferedParity(t *testing.T, path, requestBody, upstreamSSE string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(requestBody)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	svc := &OpenAIGatewayService{
		httpUpstream: &httpUpstreamRecorder{resp: bufferedHostedItemsSSEResponse(upstreamSSE)},
		cfg:          rawChatCompletionsTestConfig(),
	}
	account := bufferedHostedItemsOAuthAccount()
	var err error
	if path == "/v1/chat/completions" {
		_, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	} else {
		_, err = svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
	}
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return rec.Body.String()
}

// A buffered /v1/messages or /v1/chat/completions answer must carry what the
// streaming path delivers for the same upstream event stream. The shapes are the
// ones real Codex streams produced; texts are placeholders.
func TestBufferedCompat_MatchesStreamingPathOnRealStreamShapes(t *testing.T) {
	reasoningThenMessage := func(terminal string) string {
		return bufferedParityStream(terminal,
			bufferedParityReasoning(0, "rs_1", "REASON_TEXT_PLACEHOLDER", "ENC_PLACEHOLDER_1"),
			bufferedParityMessage(1, "ANSWER_PLACEHOLDER"))
	}
	signatureOnlyThenEmptyMessage := func(terminal string) string {
		return bufferedParityStream(terminal,
			bufferedParityReasoning(0, "rs_1", "", "ENC_PLACEHOLDER_1"),
			bufferedParityMessage(1, ""))
	}
	const filledTerminal = `[{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"REASON_TEXT_PLACEHOLDER"}],"encrypted_content":"ENC_PLACEHOLDER_1"},{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ANSWER_PLACEHOLDER"}]}]`
	const signatureOnlyTerminal = `[{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"ENC_PLACEHOLDER_1"},{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":""}]}]`
	webSearchTurn := bufferedParityStream(`[]`,
		bufferedParityReasoning(0, "rs_1", "REASON_ONE_PLACEHOLDER", "ENC_PLACEHOLDER_1"),
		bufferedParityWebSearch(1, "ws_1", "QUERY_ONE_PLACEHOLDER"),
		bufferedParityReasoning(2, "rs_2", "REASON_TWO_PLACEHOLDER", "ENC_PLACEHOLDER_2"),
		bufferedParityWebSearch(3, "ws_2", "QUERY_TWO_PLACEHOLDER"),
		bufferedParityReasoning(4, "rs_3", "REASON_THREE_PLACEHOLDER", "ENC_PLACEHOLDER_3"),
		bufferedParityFunctionCall(5))

	cases := []struct {
		name         string
		sse          string
		wantMessages []string
		wantChat     string
	}{
		{
			name: "reasoning and message, filled terminal",
			sse:  reasoningThenMessage(filledTerminal),
			wantMessages: []string{
				`thinking "REASON_TEXT_PLACEHOLDER" signature="ENC_PLACEHOLDER_1"`,
				`text "ANSWER_PLACEHOLDER"`,
			},
			wantChat: bufferedParityChatSummary("REASON_TEXT_PLACEHOLDER", "ANSWER_PLACEHOLDER", nil, "stop"),
		},
		{
			name: "reasoning and message, empty terminal",
			sse:  reasoningThenMessage(`[]`),
			wantMessages: []string{
				`thinking "REASON_TEXT_PLACEHOLDER" signature="ENC_PLACEHOLDER_1"`,
				`text "ANSWER_PLACEHOLDER"`,
			},
			wantChat: bufferedParityChatSummary("REASON_TEXT_PLACEHOLDER", "ANSWER_PLACEHOLDER", nil, "stop"),
		},
		{
			name:         "signature only reasoning and empty message, filled terminal",
			sse:          signatureOnlyThenEmptyMessage(signatureOnlyTerminal),
			wantMessages: []string{`thinking "" signature="ENC_PLACEHOLDER_1"`},
			wantChat:     bufferedParityChatSummary("", "", nil, "stop"),
		},
		{
			name:         "signature only reasoning and empty message, empty terminal",
			sse:          signatureOnlyThenEmptyMessage(`[]`),
			wantMessages: []string{`thinking "" signature="ENC_PLACEHOLDER_1"`},
			wantChat:     bufferedParityChatSummary("", "", nil, "stop"),
		},
		{
			name: "web search turn with a client tool, empty terminal",
			sse:  webSearchTurn,
			wantMessages: []string{
				`thinking "REASON_ONE_PLACEHOLDER" signature="ENC_PLACEHOLDER_1"`,
				`server_tool_use srvtoolu_ws_1 {"query":"QUERY_ONE_PLACEHOLDER"}`,
				`web_search_tool_result srvtoolu_ws_1`,
				`thinking "REASON_TWO_PLACEHOLDER" signature="ENC_PLACEHOLDER_2"`,
				`server_tool_use srvtoolu_ws_2 {"query":"QUERY_TWO_PLACEHOLDER"}`,
				`web_search_tool_result srvtoolu_ws_2`,
				`thinking "REASON_THREE_PLACEHOLDER" signature="ENC_PLACEHOLDER_3"`,
				`tool_use call_c record_version {"version":"1.25"}`,
			},
			wantChat: bufferedParityChatSummary("REASON_ONE_PLACEHOLDERREASON_TWO_PLACEHOLDERREASON_THREE_PLACEHOLDER", "", []string{`call_c record_version {"version":"1.25"}`}, "tool_calls"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name+" messages", func(t *testing.T) {
			buffered := forwardBufferedParity(t, "/v1/messages", fmt.Sprintf(bufferedParityMessagesBody, false), tc.sse)
			streamed := forwardBufferedParity(t, "/v1/messages", fmt.Sprintf(bufferedParityMessagesBody, true), tc.sse)
			require.Equal(t, tc.wantMessages, bufferedParityBlocks(gjson.Get(buffered, "content").Array()), buffered)
			streamedBlocks := bufferedParityStreamBlocks(t, streamed)
			require.Equal(t, tc.wantMessages, streamedBlocks, streamed)
		})
		t.Run(tc.name+" chat", func(t *testing.T) {
			buffered := forwardBufferedParity(t, "/v1/chat/completions", fmt.Sprintf(bufferedParityChatBody, false), tc.sse)
			streamed := forwardBufferedParity(t, "/v1/chat/completions", fmt.Sprintf(bufferedParityChatBody, true), tc.sse)
			require.Equal(t, tc.wantChat, bufferedParityChatNonStream(buffered), buffered)
			require.Equal(t, tc.wantChat, bufferedParityChatStream(streamed), streamed)
		})
	}
}
