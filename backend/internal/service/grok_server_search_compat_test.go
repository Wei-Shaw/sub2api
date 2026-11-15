//go:build unit

package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// #8823: xAI executes xs_/as_ custom calls; clients must not execute them again.
func TestGrokServerSearchCompat(t *testing.T) {
	for _, endpoint := range []string{"/v1/messages", "/v1/chat/completions"} {
		for _, stream := range []bool{false, true} {
			for _, terminalOutput := range []bool{false, true} {
				for _, prefix := range []string{"xs_", "as_"} {
					name := fmt.Sprintf("%s/stream=%t/output=%t/%s", endpoint, stream, terminalOutput, prefix)
					t.Run(name, func(t *testing.T) {
						sse := grokServerSearchSSE("custom_tool_call", prefix+"search", terminalOutput)
						if terminalOutput {
							sse += "\n" // Also exercise a terminated SSE frame instead of EOF flushing.
						}
						result, body := forwardGrokServerSearchFixture(t, endpoint, stream, true, sse)
						assertGrokSearchCompatOutput(t, endpoint, stream, body, false)
						wantSearchCount := 0
						if prefix == "xs_" {
							wantSearchCount = 1
						}
						// Attachment searches do not incur the existing X/web search surcharge.
						require.Equal(t, wantSearchCount, result.SearchCount)
						require.Equal(t, 5, result.Usage.InputTokens)
						require.Equal(t, 2, result.Usage.OutputTokens)
					})
				}
			}
		}
	}
}

func TestGrokServerSearchCompatRetainsTerminalSearchCount(t *testing.T) {
	for _, endpoint := range []string{"/v1/messages", "/v1/chat/completions"} {
		for _, terminal := range []string{"completed", "done", "incomplete", "cancelled", "canceled"} {
			t.Run(endpoint+"/"+terminal, func(t *testing.T) {
				sse := grokServerSearchSSE("web_search_call", "search_native", true)
				sse = strings.ReplaceAll(sse, `"type":"response.completed"`, `"type":"response.`+terminal+`"`)
				if terminal != "done" {
					sse = strings.ReplaceAll(sse, `"status":"completed","output":`, `"status":"`+terminal+`","output":`)
				}
				var lines []string
				for _, line := range strings.Split(sse, "\n") {
					if !strings.Contains(line, `"type":"response.output_item.done"`) {
						lines = append(lines, line)
					}
				}
				result, _ := forwardGrokServerSearchFixture(t, endpoint, false, true, strings.Join(lines, "\n"))
				require.Equal(t, 1, result.SearchCount, "keep searches reported only in a terminal output")
			})
		}
	}
}

func TestGrokServerSearchCount(t *testing.T) {
	for _, item := range []struct {
		itemType, callID, name string
		want                   int
	}{
		{"custom_tool_call", "xs_search", "x_keyword_search", 1},
		{"custom_tool_call", "as_search", "x_search", 0},
		{"function_call", "xs_client", "lookup", 0},
		{"custom_tool_call", "call_client", "lookup", 0},
		{"custom_tool_call", "XS_client", "lookup", 0},
	} {
		t.Run(item.itemType+"/"+item.callID, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"output":[{"type":%q,"call_id":%q,"name":%q}]}`, item.itemType, item.callID, item.name))
			require.Equal(t, item.want, countGrokNativeSearchCallsFromJSONBytes(body))
		})
	}
}

func TestGrokServerSearchCompatPreservesClientTools(t *testing.T) {
	for _, endpoint := range []string{"/v1/messages", "/v1/chat/completions"} {
		for _, stream := range []bool{false, true} {
			for _, call := range []struct{ itemType, callID string }{
				{"function_call", "xs_client"},
				{"function_call", "as_client"},
				{"custom_tool_call", "call_client"},
			} {
				name := fmt.Sprintf("%s/stream=%t/%s/%s", endpoint, stream, call.itemType, call.callID)
				t.Run(name, func(t *testing.T) {
					sse := grokServerSearchSSE(call.itemType, call.callID, false)
					result, body := forwardGrokServerSearchFixture(t, endpoint, stream, true, sse)
					assertGrokSearchCompatOutput(t, endpoint, stream, body, true)
					require.Zero(t, result.SearchCount)
				})
			}
			for _, prefix := range []string{"xs_", "as_"} {
				t.Run(fmt.Sprintf("%s/stream=%t/non-Grok/%s", endpoint, stream, prefix), func(t *testing.T) {
					sse := grokServerSearchSSE("custom_tool_call", prefix+"client", false)
					_, body := forwardGrokServerSearchFixture(t, endpoint, stream, false, sse)
					assertGrokSearchCompatOutput(t, endpoint, stream, body, true)
				})
			}
		}
	}
}

func TestGrokServerSearchCompatInterleavedClientCall(t *testing.T) {
	for _, endpoint := range []string{"/v1/messages", "/v1/chat/completions"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", endpoint, stream), func(t *testing.T) {
				var serverEvents strings.Builder
				for _, line := range strings.Split(grokServerSearchSSE("custom_tool_call", "xs_search", false), "\n") {
					payload, ok := strings.CutPrefix(line, "data: ")
					if ok && (gjson.Get(payload, "item.type").String() == "custom_tool_call" ||
						strings.HasPrefix(gjson.Get(payload, "type").String(), "response.custom_tool_call_input.")) {
						serverEvents.WriteString(strings.ReplaceAll(line, `"output_index":0`, `"output_index":1`) + "\n\n")
					}
				}
				sse := grokServerSearchSSE("function_call", "call_client", false)
				sse = strings.ReplaceAll(sse, `"output_index":1`, `"output_index":2`)
				anchor := `data: {"type":"response.function_call_arguments.delta"`
				require.Contains(t, sse, anchor)
				sse = strings.Replace(sse, anchor, serverEvents.String()+anchor, 1)
				result, body := forwardGrokServerSearchFixture(t, endpoint, stream, true, sse)
				assertGrokSearchCompatOutput(t, endpoint, stream, body, true)
				require.Equal(t, 1, result.SearchCount)
			})
		}
	}
}

func forwardGrokServerSearchFixture(t *testing.T, endpoint string, stream, grok bool, sse string) (*OpenAIForwardResult, string) {
	t.Helper()
	c, rec := grokAuditContext(endpoint)
	account := healthyGrokOAuthGatewayTestAccount(88230, "access-token")
	if !grok {
		account = &Account{
			ID: 88231, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "test-key", "base_url": "https://example.com/v1"},
		}
	}
	upstream := &httpUpstreamRecorder{resp: grokAuditHTTPResponse("text/event-stream", sse)}
	svc := grokAuditOAuthService(upstream, account)
	model := "grok-4.6"
	if !grok {
		model = "gpt-5.4"
	}
	body := []byte(fmt.Sprintf(`{"model":%q,"max_tokens":256,"stream":%t,"messages":[{"role":"user","content":"hi"}]}`, model, stream))
	var result *OpenAIForwardResult
	var err error
	if endpoint == "/v1/messages" {
		result, err = svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
	} else {
		result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	}
	require.NoError(t, err, rec.Body.String())
	require.NotNil(t, result)
	require.Equal(t, 200, rec.Code)
	require.True(t, strings.HasSuffix(upstream.lastReq.URL.Path, "/responses"), upstream.lastReq.URL.String())
	return result, rec.Body.String()
}

func assertGrokSearchCompatOutput(t *testing.T, endpoint string, stream bool, body string, clientTool bool) {
	t.Helper()
	stopReason, toolReason := "stop", "tool_calls"
	if endpoint == "/v1/messages" {
		stopReason, toolReason = "end_turn", "tool_use"
	}
	if clientTool {
		stopReason = toolReason
	}
	if stream {
		var text, arguments, stop string
		toolCount := 0
		for _, line := range strings.Split(body, "\n") {
			payload, ok := strings.CutPrefix(line, "data: ")
			if !ok || payload == "[DONE]" {
				continue
			}
			if endpoint == "/v1/messages" {
				if gjson.Get(payload, "content_block.type").String() == "tool_use" {
					toolCount++
				}
				text += gjson.Get(payload, "delta.text").String()
				arguments += gjson.Get(payload, "delta.partial_json").String()
				if value := gjson.Get(payload, "delta.stop_reason").String(); value != "" {
					stop = value
				}
			} else {
				text += gjson.Get(payload, "choices.0.delta.content").String()
				for _, call := range gjson.Get(payload, "choices.0.delta.tool_calls").Array() {
					if call.Get("id").String() != "" {
						toolCount++
					}
					arguments += call.Get("function.arguments").String()
				}
				if value := gjson.Get(payload, "choices.0.finish_reason").String(); value != "" {
					stop = value
				}
			}
		}
		require.Equal(t, "answer", text, body)
		require.Equal(t, stopReason, stop, body)
		if clientTool {
			require.Equal(t, 1, toolCount, body)
			require.JSONEq(t, `{"query":"topic"}`, arguments)
		} else {
			require.Zero(t, toolCount, body)
			require.Empty(t, arguments, body)
		}
		return
	}
	if endpoint == "/v1/messages" {
		require.Equal(t, stopReason, gjson.Get(body, "stop_reason").String(), body)
		require.Equal(t, "answer", gjson.Get(body, `content.#(type=="text").text`).String(), body)
		require.Equal(t, clientTool, gjson.Get(body, `content.#(type=="tool_use")`).Exists(), body)
	} else {
		require.Equal(t, stopReason, gjson.Get(body, "choices.0.finish_reason").String(), body)
		require.Equal(t, "answer", gjson.Get(body, "choices.0.message.content").String(), body)
		require.Equal(t, clientTool, gjson.Get(body, "choices.0.message.tool_calls.0").Exists(), body)
	}
}

func grokServerSearchSSE(itemType, callID string, terminalOutput bool) string {
	item := fmt.Sprintf(`{"type":%q,"id":"item_search","call_id":%q,"name":"lookup","status":"completed","input":"{\"query\":\"topic\"}","arguments":"{\"query\":\"topic\"}"}`, itemType, callID)
	deltaType := "response.custom_tool_call_input.delta"
	doneType, doneField := "response.custom_tool_call_input.done", "input"
	if itemType == "function_call" {
		deltaType = "response.function_call_arguments.delta"
		doneType, doneField = "response.function_call_arguments.done", "arguments"
	}
	output := "[]"
	if terminalOutput {
		output = `[` + item + `,{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}]`
	}
	return strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_search","model":"grok-4.6","status":"in_progress","output":[]}}`, "",
		`data: {"type":"response.output_item.added","output_index":0,"item":` + item + `}`, "",
		fmt.Sprintf(`data: {"type":%q,"output_index":0,"delta":"{\"query\":\"topic\"}"}`, deltaType), "",
		fmt.Sprintf(`data: {"type":%q,"output_index":0,%q:"{\"query\":\"topic\"}"}`, doneType, doneField), "",
		`data: {"type":"response.output_item.done","output_index":0,"item":` + item + `}`, "",
		// A duplicate completion must not add another surcharge.
		`data: {"type":"response.output_item.done","output_index":0,"item":` + item + `}`, "",
		`data: {"type":"response.output_item.added","output_index":1,"item":{"type":"message","role":"assistant","content":[]}}`, "",
		`data: {"type":"response.output_text.delta","output_index":1,"content_index":0,"delta":"answer"}`, "",
		`data: {"type":"response.completed","response":{"id":"resp_search","model":"grok-4.6","status":"completed","output":` + output + `,"usage":{"input_tokens":5,"output_tokens":2}}}`, "",
	}, "\n")
}
