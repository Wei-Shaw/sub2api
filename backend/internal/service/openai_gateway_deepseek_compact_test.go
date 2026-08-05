package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPrepareDeepSeekResponsesRequest_BuildsLocalCompactTurn(t *testing.T) {
	c, _ := newDeepSeekCompactTestContext()
	account := newDeepSeekCompactTestAccount(false)
	body := []byte(`{
		"model":"deepseek-v4-flash",
		"stream":true,
		"store":true,
		"previous_response_id":"resp_old",
		"context_management":{"type":"compact"},
		"text":{"format":{"type":"json_object"}},
		"tools":[{"type":"function","name":"shell"}],
		"tool_choice":"auto",
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]},
			{"type":"compaction_trigger"}
		]
	}`)

	prepared, bridged, err := prepareDeepSeekResponsesRequest(c, account, body)
	require.NoError(t, err)
	require.True(t, bridged)
	require.True(t, isDeepSeekLocalCompactBridge(c))
	require.False(t, openAICompactClientWantsStream(c))
	require.False(t, gjson.GetBytes(prepared, "stream").Bool())
	require.False(t, gjson.GetBytes(prepared, "store").Bool())
	require.False(t, gjson.GetBytes(prepared, "previous_response_id").Exists())
	require.False(t, gjson.GetBytes(prepared, "context_management").Exists())
	require.False(t, gjson.GetBytes(prepared, "text").Exists())
	require.False(t, gjson.GetBytes(prepared, "tools").Exists())
	require.False(t, gjson.GetBytes(prepared, "tool_choice").Exists())
	require.Len(t, gjson.GetBytes(prepared, "input").Array(), 2)
	require.Equal(t, "hello", gjson.GetBytes(prepared, "input.0.content.0.text").String())
	require.Contains(t, gjson.GetBytes(prepared, "input.1.content.0.text").String(), "CONTEXT CHECKPOINT COMPACTION")
	for _, item := range gjson.GetBytes(prepared, "input").Array() {
		require.NotEqual(t, "compaction_trigger", item.Get("type").String())
	}
}

func TestPrepareDeepSeekResponsesRequest_UsesMappedUpstreamModel(t *testing.T) {
	c, _ := newDeepSeekCompactTestContext()
	account := newDeepSeekCompactTestAccount(false)
	account.Credentials["model_mapping"] = map[string]any{
		"gpt-5.6-sol": "deepseek-v4-flash",
	}
	body := []byte(`{"model":"gpt-5.6-sol","stream":true,"input":[{"type":"compaction_trigger"}]}`)

	prepared, bridged, err := prepareDeepSeekResponsesRequest(c, account, body)
	require.NoError(t, err)
	require.True(t, bridged)
	require.False(t, gjson.GetBytes(prepared, "stream").Bool())
	require.Equal(t, "message", gjson.GetBytes(prepared, "input.0.type").String())
}

func TestPrepareDeepSeekResponsesRequest_LeavesNonDeepSeekCompactUntouched(t *testing.T) {
	c, _ := newDeepSeekCompactTestContext()
	account := newDeepSeekCompactTestAccount(false)
	body := []byte(`{"model":"gpt-5.6-sol","stream":true,"input":[{"type":"compaction_trigger"}]}`)

	prepared, bridged, err := prepareDeepSeekResponsesRequest(c, account, body)
	require.NoError(t, err)
	require.False(t, bridged)
	require.Equal(t, body, prepared)
	require.False(t, isDeepSeekLocalCompactBridge(c))
	require.False(t, openAICompactClientWantsStream(c))
}

func TestPrepareDeepSeekResponsesRequest_RestoresSyntheticCompaction(t *testing.T) {
	c, _ := newDeepSeekCompactTestContext()
	account := newDeepSeekCompactTestAccount(false)
	envelope := encodeDeepSeekCompactEnvelope("finished setup; next run the focused tests")
	body := []byte(`{
		"model":"deepseek-v4-flash",
		"stream":true,
		"input":[
			{"id":"cmp_1","type":"compaction","encrypted_content":"` + envelope + `"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"}]}
		]
	}`)

	prepared, bridged, err := prepareDeepSeekResponsesRequest(c, account, body)
	require.NoError(t, err)
	require.False(t, bridged)
	require.False(t, isDeepSeekLocalCompactBridge(c))
	require.True(t, gjson.GetBytes(prepared, "stream").Bool())
	require.Len(t, gjson.GetBytes(prepared, "input").Array(), 2)
	require.Equal(t, "message", gjson.GetBytes(prepared, "input.0.type").String())
	require.Contains(t, gjson.GetBytes(prepared, "input.0.content.0.text").String(), deepSeekCompactSummaryContextPrefix)
	require.Contains(t, gjson.GetBytes(prepared, "input.0.content.0.text").String(), "next run the focused tests")
	require.Equal(t, "continue", gjson.GetBytes(prepared, "input.1.content.0.text").String())
}

func TestPrepareDeepSeekResponsesRequest_RestoresSyntheticCompactionBeforeProviderSelection(t *testing.T) {
	c, _ := newDeepSeekCompactTestContext()
	account := newDeepSeekCompactTestAccount(false)
	account.Credentials["base_url"] = "https://api.openai.com"
	envelope := encodeDeepSeekCompactEnvelope("restore this summary before switching accounts")
	body := []byte(`{
		"model":"gpt-5.6-sol",
		"stream":true,
		"input":[
			{"id":"cmp_1","type":"compaction","encrypted_content":"` + envelope + `"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"}]}
		]
	}`)

	prepared, bridged, err := prepareDeepSeekResponsesRequest(c, account, body)
	require.NoError(t, err)
	require.False(t, bridged)
	require.False(t, isDeepSeekLocalCompactBridge(c))
	require.Equal(t, "message", gjson.GetBytes(prepared, "input.0.type").String())
	require.Contains(t, gjson.GetBytes(prepared, "input.0.content.0.text").String(), "restore this summary")
	require.False(t, gjson.GetBytes(prepared, "input.#(type==\"compaction\")").Exists())
}

func TestPrepareDeepSeekResponsesRequest_ConvertsCodexToolHistoryForCompaction(t *testing.T) {
	c, _ := newDeepSeekCompactTestContext()
	account := newDeepSeekCompactTestAccount(false)
	body := []byte(`{
		"model":"deepseek-v4-flash",
		"stream":true,
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"fix the bug"}]},
			{"type":"custom_tool_call","call_id":"call_shell","name":"shell","input":"go test ./..."},
			{"type":"custom_tool_call_output","call_id":"call_shell","output":"all tests passed"},
			{"type":"compaction_trigger"}
		]
	}`)

	prepared, bridged, err := prepareDeepSeekResponsesRequest(c, account, body)
	require.NoError(t, err)
	require.True(t, bridged)
	require.Len(t, gjson.GetBytes(prepared, "input").Array(), 4)
	require.Equal(t, "message", gjson.GetBytes(prepared, "input.1.type").String())
	require.Contains(t, gjson.GetBytes(prepared, "input.1.content.0.text").String(), `"name":"shell"`)
	require.Equal(t, "message", gjson.GetBytes(prepared, "input.2.type").String())
	require.Contains(t, gjson.GetBytes(prepared, "input.2.content.0.text").String(), "all tests passed")
	require.False(t, gjson.GetBytes(prepared, "input.#(type==\"custom_tool_call\")").Exists())
	require.False(t, gjson.GetBytes(prepared, "input.#(type==\"custom_tool_call_output\")").Exists())
}

func TestPrepareDeepSeekResponsesRequest_ConvertsShellHistoryForCompaction(t *testing.T) {
	c, _ := newDeepSeekCompactTestContext()
	account := newDeepSeekCompactTestAccount(false)
	body := []byte(`{
		"model":"deepseek-v4-flash",
		"stream":true,
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"fix the build"}]},
			{"type":"local_shell_call","call_id":"call_local","action":{"type":"exec","command":["go","test","./..."]}},
			{"type":"local_shell_call_output","id":"call_local","output":"compile failed"},
			{"type":"shell_call","call_id":"call_shell","action":{"commands":["go test ./internal/service"]}},
			{"type":"shell_call_output","call_id":"call_shell","output":[{"stdout":"all tests passed","stderr":"","outcome":{"type":"exit","exit_code":0}}]},
			{"type":"compaction_trigger"}
		]
	}`)

	prepared, bridged, err := prepareDeepSeekResponsesRequest(c, account, body)
	require.NoError(t, err)
	require.True(t, bridged)
	require.Len(t, gjson.GetBytes(prepared, "input").Array(), 6)
	for index := 1; index <= 4; index++ {
		require.Equal(t, "message", gjson.GetBytes(prepared, fmt.Sprintf("input.%d.type", index)).String())
	}
	require.Contains(t, gjson.GetBytes(prepared, "input.1.content.0.text").String(), "local_shell_call")
	require.Contains(t, gjson.GetBytes(prepared, "input.2.content.0.text").String(), "compile failed")
	require.Contains(t, gjson.GetBytes(prepared, "input.3.content.0.text").String(), "shell_call")
	require.Contains(t, gjson.GetBytes(prepared, "input.4.content.0.text").String(), "all tests passed")
	for _, itemType := range []string{"local_shell_call", "local_shell_call_output", "shell_call", "shell_call_output"} {
		require.False(t, gjson.GetBytes(prepared, `input.#(type=="`+itemType+`")`).Exists())
	}
}

func TestConvertDeepSeekResponseToOpenAICompact_PreservesUsageAndRoundTripsSummary(t *testing.T) {
	body := []byte(`{
		"id":"resp_deepseek_1",
		"object":"response",
		"status":"completed",
		"model":"deepseek-v4-flash",
		"output":[
			{"id":"rs_1","type":"reasoning","content":[{"type":"reasoning_text","text":"private work"}]},
			{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"summary text"}]}
		],
		"usage":{"input_tokens":10,"output_tokens":4,"total_tokens":14}
	}`)

	converted, err := convertDeepSeekResponseToOpenAICompact(body)
	require.NoError(t, err)
	require.Equal(t, "resp_deepseek_1", gjson.GetBytes(converted, "id").String())
	require.Len(t, gjson.GetBytes(converted, "output").Array(), 1)
	require.Equal(t, "compaction", gjson.GetBytes(converted, "output.0.type").String())
	require.Equal(t, "summary text", gjson.GetBytes(converted, "output.0.summary.0.text").String())
	require.Equal(t, int64(14), gjson.GetBytes(converted, "usage.total_tokens").Int())

	summary, ok := decodeDeepSeekCompactEnvelope(gjson.GetBytes(converted, "output.0.encrypted_content").String())
	require.True(t, ok)
	require.Equal(t, "summary text", summary)
}

func TestOpenAIGatewayService_Forward_DeepSeekRemoteCompactV2Bridge(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, upstreamSSE := range []bool{false, true} {
			name := "native_json"
			switch {
			case passthrough && upstreamSSE:
				name = "passthrough_sse"
			case passthrough:
				name = "passthrough_json"
			case upstreamSSE:
				name = "native_sse"
			}
			t.Run(name, func(t *testing.T) {
				c, rec := newDeepSeekCompactTestContext()
				MarkOpenAICompactClientStream(c)
				upstream := &httpUpstreamRecorder{resp: deepSeekCompactUpstreamResponse(upstreamSSE)}
				cfg := &config.Config{}
				cfg.Security.URLAllowlist.Enabled = false
				svc := &OpenAIGatewayService{
					cfg:           cfg,
					httpUpstream:  upstream,
					toolCorrector: NewCodexToolCorrector(),
				}
				account := newDeepSeekCompactTestAccount(passthrough)
				body := []byte(`{
					"model":"deepseek-v4-flash",
					"stream":true,
					"instructions":"You are Codex.",
					"tools":[{"type":"function","name":"shell","parameters":{"type":"object"}}],
					"input":[
						{"type":"message","role":"user","content":[{"type":"input_text","text":"implement the bridge"}]},
						{"type":"compaction_trigger"}
					]
				}`)

				result, err := svc.Forward(context.Background(), c, account, body)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.True(t, result.Stream)
				require.NotNil(t, upstream.lastReq)
				require.Equal(t, "application/json", upstream.lastReq.Header.Get("Accept"))
				require.False(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
				require.False(t, gjson.GetBytes(upstream.lastBody, "tools").Exists())
				require.Equal(t, "deepseek-v4-flash", gjson.GetBytes(upstream.lastBody, "model").String())
				require.Contains(t, gjson.GetBytes(upstream.lastBody, "input.1.content.0.text").String(), "CONTEXT CHECKPOINT COMPACTION")

				require.Equal(t, http.StatusOK, rec.Code)
				require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
				events := parseCompactBridgeSSE(t, rec.Body.String())
				require.Len(t, events, 2)
				require.Equal(t, "response.output_item.done", events[0][0])
				require.Equal(t, "compaction", gjson.Get(events[0][1], "item.type").String())
				summary, ok := decodeDeepSeekCompactEnvelope(gjson.Get(events[0][1], "item.encrypted_content").String())
				require.True(t, ok)
				require.Equal(t, "bridge summary", summary)
				require.Equal(t, "response.completed", events[1][0])
				require.Equal(t, int64(14), gjson.Get(events[1][1], "response.usage.total_tokens").Int())
			})
		}
	}
}

func TestOpenAIGatewayService_Forward_DeepSeekRemoteCompactV2ChatFallback(t *testing.T) {
	c, rec := newDeepSeekCompactTestContext()
	MarkOpenAICompactClientStream(c)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_deepseek_chat_bridge"}},
		Body: io.NopCloser(strings.NewReader(`{
			"id":"chatcmpl_deepseek_bridge",
			"object":"chat.completion",
			"model":"deepseek-v4-flash",
			"choices":[{"index":0,"message":{"role":"assistant","content":"chat fallback summary"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}
		}`)),
	}}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{
		cfg:           cfg,
		httpUpstream:  upstream,
		toolCorrector: NewCodexToolCorrector(),
	}
	account := newDeepSeekCompactTestAccount(false)
	account.Extra["openai_responses_supported"] = false
	body := []byte(`{
		"model":"deepseek-v4-flash",
		"stream":true,
		"instructions":"You are Codex.",
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"implement the bridge"}]},
			{"type":"compaction_trigger"}
		]
	}`)

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Equal(t, "/v1/chat/completions", upstream.lastReq.URL.Path)
	require.False(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	messages := gjson.GetBytes(upstream.lastBody, "messages").Array()
	require.NotEmpty(t, messages)
	require.Contains(t, messages[len(messages)-1].Get("content").String(), "CONTEXT CHECKPOINT COMPACTION")

	events := parseCompactBridgeSSE(t, rec.Body.String())
	require.Len(t, events, 2)
	require.Equal(t, "compaction", gjson.Get(events[0][1], "item.type").String())
	summary, ok := decodeDeepSeekCompactEnvelope(gjson.Get(events[0][1], "item.encrypted_content").String())
	require.True(t, ok)
	require.Equal(t, "chat fallback summary", summary)
	require.Equal(t, int64(14), gjson.Get(events[1][1], "response.usage.total_tokens").Int())
}

func TestOpenAIGatewayService_Forward_DeepSeekCompactPathUsesResponsesUpstream(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "passthrough"}[passthrough], func(t *testing.T) {
			c, rec := newDeepSeekCompactTestContext()
			c.Request.URL.Path = "/v1/responses/compact"
			upstream := &httpUpstreamRecorder{resp: deepSeekCompactUpstreamResponse(false)}
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.Enabled = false
			svc := &OpenAIGatewayService{
				cfg:           cfg,
				httpUpstream:  upstream,
				toolCorrector: NewCodexToolCorrector(),
			}
			account := newDeepSeekCompactTestAccount(passthrough)
			body := []byte(`{
				"model":"deepseek-v4-flash",
				"stream":false,
				"input":[
					{"type":"message","role":"user","content":[{"type":"input_text","text":"summarize this"}]},
					{"type":"compaction_trigger"}
				]
			}`)

			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.False(t, result.Stream)
			require.Equal(t, "/v1/responses", upstream.lastReq.URL.Path)
			require.Equal(t, "application/json", upstream.lastReq.Header.Get("Accept"))
			require.False(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
			require.False(t, openAICompactClientWantsStream(c))
			require.NotContains(t, rec.Header().Get("Content-Type"), "text/event-stream")
			require.Equal(t, "compaction", gjson.Get(rec.Body.String(), "output.0.type").String())
		})
	}
}

func newDeepSeekCompactTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Accept", "text/event-stream")
	c.Request.Header.Set("x-codex-beta-features", "remote_compaction_v2")
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
	return c, rec
}

func newDeepSeekCompactTestAccount(passthrough bool) *Account {
	return &Account{
		ID:          1,
		Name:        "deepseek-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.deepseek.com",
		},
		Extra: map[string]any{
			"openai_responses_supported": true,
			"openai_passthrough":         passthrough,
		},
		Status:      StatusActive,
		Schedulable: true,
	}
}

func deepSeekCompactUpstreamResponse(stream bool) *http.Response {
	finalResponse := `{"id":"resp_deepseek_bridge","object":"response","status":"completed","model":"deepseek-v4-flash","output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"bridge summary"}]}],"usage":{"input_tokens":10,"output_tokens":4,"total_tokens":14}}`
	if !stream {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_deepseek_bridge"}},
			Body:       io.NopCloser(strings.NewReader(finalResponse)),
		}
	}
	sse := "event: response.output_item.done\n" +
		`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"bridge summary"}]}}` + "\n\n" +
		"event: response.completed\n" +
		`data: {"type":"response.completed","response":` + finalResponse + "}\n\n"
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_deepseek_bridge_sse"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}
}
