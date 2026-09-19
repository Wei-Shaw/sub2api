package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const deepSeekResponsesToolsFragment = `"tools":[{"type":"function","name":"shell","description":"run",` +
	`"parameters":{"type":"object","properties":{"cmd":{"type":"string"}},"required":["cmd"]}}]`

const deepSeekRemoteCompactTestSummary = "Implemented the gateway bridge; next run the focused regression tests."

func openaiMappedDeepSeekResponsesAccount() *Account {
	return &Account{
		ID:       18,
		Name:     "openai-deepseek-responses",
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesSupported: true,
		},
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": DefaultDeepseekBaseURL,
			"model_mapping": map[string]any{
				"gpt-5.6-sol": "deepseek-flash",
			},
		},
		Status:      StatusActive,
		Schedulable: true,
	}
}

func nativeDeepSeekResponsesAccount() *Account {
	return &Account{
		ID:          11,
		Name:        "deepseek-apikey",
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-test",
			"api_protocol": APIProtocolResponses,
		},
		Status:      StatusActive,
		Schedulable: true,
	}
}

func officialOpenAIResponsesAccount() *Account {
	return &Account{
		ID:       17,
		Name:     "openai-official",
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesSupported: true,
		},
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com",
		},
		Status:      StatusActive,
		Schedulable: true,
	}
}

func deepSeekResponsesTestContext(t *testing.T, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, recorder
}

func deepSeekResponsesJSONUpstream() *httpUpstreamRecorder {
	return &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_1","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`,
		)),
	}}
}

func deepSeekMappedResponsesTestConfig() *config.Config {
	cfg := deepSeekChatFallbackTestConfig()
	cfg.JWT = config.JWTConfig{Secret: "deepseek-compact-test-jwt-secret-32-bytes"}
	return cfg
}

func deepSeekCompactTestContext(userID int64) context.Context {
	return context.WithValue(context.Background(), ctxkey.UserID, userID)
}

func assertAssistantMessagesGuarded(t *testing.T, body []byte) {
	t.Helper()
	items := gjson.GetBytes(body, "input").Array()
	require.NotEmpty(t, items)
	for i, item := range items {
		if strings.TrimSpace(item.Get("type").String()) != "reasoning" {
			continue
		}
		require.NotEmptyf(t, item.Get("content.0.text").String(), "input.%d 的 reasoning 缺明文", i)
	}
	for i, item := range items {
		if responsesInputItemTypeFromJSON(item) != "message" || item.Get("role").String() != "assistant" {
			continue
		}
		require.Greaterf(t, i, 0, "assistant 消息不应位于 input 首位")
		prev := items[i-1]
		require.Equalf(t, "reasoning", prev.Get("type").String(), "input.%d 前应是 reasoning item", i)
		require.NotEmptyf(t, prev.Get("content.0.text").String(), "input.%d 的 reasoning 明文不得为空", i-1)
	}
}

func TestEnsureDeepSeekResponsesReasoningPlaceholders(t *testing.T) {
	missing := []byte(`{"model":"deepseek-flash",` + deepSeekResponsesToolsFragment + `,"input":[` +
		`{"type":"message","role":"user","content":"go"},` +
		`{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":""}]},` +
		`{"type":"function_call","call_id":"c1","name":"shell","arguments":"{}"},` +
		`{"type":"function_call_output","call_id":"c1","output":"ok"}]}`)
	got, changed := ensureDeepSeekResponsesReasoningPlaceholders(missing, true)
	require.True(t, changed)
	require.Equal(t, "reasoning", gjson.GetBytes(got, "input.1.type").String())
	require.Equal(t, responsesReasoningPlaceholderText, gjson.GetBytes(got, "input.1.content.0.text").String())
	assertAssistantMessagesGuarded(t, got)

	noTools := []byte(`{"model":"deepseek-flash","input":[` +
		`{"type":"message","role":"user","content":"go"},` +
		`{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":""}]}]}`)
	unchanged, changed := ensureDeepSeekResponsesReasoningPlaceholders(noTools, false)
	require.False(t, changed)
	require.Equal(t, string(noTools), string(unchanged))
}

func TestForwardResponses_MappedDeepSeekInjectsReasoningPlaceholders(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","stream":false,` + deepSeekResponsesToolsFragment + `,"input":[` +
		`{"type":"message","role":"user","content":"go"},` +
		`{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":""}]},` +
		`{"type":"function_call","call_id":"c1","name":"shell","arguments":"{}"},` +
		`{"type":"function_call_output","call_id":"c1","output":"ok"}]}`)

	upstream := deepSeekResponsesJSONUpstream()
	svc := &OpenAIGatewayService{cfg: deepSeekMappedResponsesTestConfig(), httpUpstream: upstream}
	c, _ := deepSeekResponsesTestContext(t, body)

	_, err := svc.Forward(context.Background(), c, openaiMappedDeepSeekResponsesAccount(), body)
	require.NoError(t, err)
	require.Contains(t, upstream.lastReq.URL.String(), "/responses")
	assertAssistantMessagesGuarded(t, upstream.lastBody)
	require.Equal(t, "rs_ph_msg_1", gjson.GetBytes(upstream.lastBody, "input.1.id").String())
	require.Equal(t, responsesReasoningPlaceholderText, gjson.GetBytes(upstream.lastBody, "input.1.content.0.text").String())
}

func TestForwardResponses_MappedDeepSeekKeepsReasoningText(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","stream":false,` + deepSeekResponsesToolsFragment + `,"input":[` +
		`{"type":"message","role":"user","content":"go"},` +
		`{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"portable"}],"content":[{"type":"reasoning_text","text":"visible reasoning"}]},` +
		`{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":""}]},` +
		`{"type":"function_call","call_id":"c1","name":"shell","arguments":"{}"},` +
		`{"type":"function_call_output","call_id":"c1","output":"ok"}]}`)

	upstream := deepSeekResponsesJSONUpstream()
	svc := &OpenAIGatewayService{cfg: deepSeekMappedResponsesTestConfig(), httpUpstream: upstream}
	c, _ := deepSeekResponsesTestContext(t, body)

	_, err := svc.Forward(context.Background(), c, openaiMappedDeepSeekResponsesAccount(), body)
	require.NoError(t, err)
	require.Equal(t, "visible reasoning", gjson.GetBytes(upstream.lastBody, "input.1.content.0.text").String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "input.1.content").Exists())
	assertAssistantMessagesGuarded(t, upstream.lastBody)
}

func TestForwardResponses_OfficialOpenAIStillStripsReasoningContent(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","stream":false,` + deepSeekResponsesToolsFragment + `,"input":[` +
		`{"type":"message","role":"user","content":"go"},` +
		`{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"portable"}],"content":[{"type":"reasoning_text","text":"visible reasoning"}]},` +
		`{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}`)

	upstream := deepSeekResponsesJSONUpstream()
	svc := &OpenAIGatewayService{cfg: deepSeekMappedResponsesTestConfig(), httpUpstream: upstream}
	c, _ := deepSeekResponsesTestContext(t, body)

	_, err := svc.Forward(context.Background(), c, officialOpenAIResponsesAccount(), body)
	require.NoError(t, err)
	require.Contains(t, upstream.lastReq.URL.String(), "/responses")
	require.Equal(t, "reasoning", gjson.GetBytes(upstream.lastBody, "input.1.type").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "input.1.content").Exists())
	require.Equal(t, "portable", gjson.GetBytes(upstream.lastBody, "input.1.summary.0.text").String())
}

func TestForwardResponses_NativeDeepSeekInjectsReasoningPlaceholders(t *testing.T) {
	body := []byte(`{"model":"deepseek-flash","stream":false,` + deepSeekResponsesToolsFragment + `,"input":[` +
		`{"type":"message","role":"user","content":"go"},` +
		`{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":""}]},` +
		`{"type":"function_call","call_id":"c1","name":"shell","arguments":"{}"},` +
		`{"type":"function_call_output","call_id":"c1","output":"ok"}]}`)

	upstream := deepSeekResponsesJSONUpstream()
	svc := &OpenAIGatewayService{cfg: deepSeekMappedResponsesTestConfig(), httpUpstream: upstream}
	c, _ := deepSeekResponsesTestContext(t, body)

	_, err := svc.Forward(context.Background(), c, nativeDeepSeekResponsesAccount(), body)
	require.NoError(t, err)
	require.Equal(t, "https://api.deepseek.com/responses", upstream.lastReq.URL.String())
	assertAssistantMessagesGuarded(t, upstream.lastBody)
}

func TestForwardOpenAIMappedDeepSeekRemoteCompactionSynthesizesOneItem(t *testing.T) {
	body := mustMarshalDeepSeekCompactTestJSON(t, map[string]any{
		"model": "gpt-5.6-sol", "stream": true, "store": true,
		"instructions": "You are Codex. Preserve the current engineering task.",
		"reasoning":    map[string]any{"effort": "max", "summary": "detailed"},
		"tools":        []any{map[string]any{"type": "function", "name": "shell", "description": "Run a command", "parameters": map[string]any{"type": "object"}}},
		"tool_choice":  "auto",
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": strings.Repeat("Implement the compact bridge with full contract coverage. ", 80)}}},
			map[string]any{"type": "function_call", "call_id": "call_1", "name": "shell", "arguments": `{"cmd":"go test ./..."}`},
			map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "one focused test is failing"},
			map[string]any{"type": "compaction_trigger"},
		},
	})
	c, recorder := deepSeekResponsesTestContext(t, body)
	c.Request.Header.Set("Accept", "text/event-stream")
	c.Request.Header.Set("X-Codex-Beta-Features", "remote_compaction_v2")
	MarkOpenAINativeCompactionV2(c)

	upstream := &httpUpstreamRecorder{resp: deepSeekRemoteCompactResponsesResponse(deepSeekRemoteCompactTestSummary)}
	svc := &OpenAIGatewayService{cfg: deepSeekMappedResponsesTestConfig(), httpUpstream: upstream}

	result, err := svc.Forward(deepSeekCompactTestContext(42), c, openaiMappedDeepSeekResponsesAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, upstream.lastReq.URL.String(), "/responses")
	require.Equal(t, "deepseek-flash", gjson.GetBytes(upstream.lastBody, "model").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "tools").Exists())
	items := gjson.GetBytes(upstream.lastBody, "input").Array()
	require.NotEmpty(t, items)
	require.NotEqual(t, "compaction_trigger", items[len(items)-1].Get("type").String())
	require.Equal(t, deepSeekCompactInstruction, items[len(items)-1].Get("content.0.text").String())

	events := parseCompactBridgeSSE(t, recorder.Body.String())
	require.Len(t, events, 2)
	require.Equal(t, "response.output_item.done", events[0][0])
	require.Equal(t, "response.completed", events[1][0])
	outputs := gjson.Get(events[1][1], "response.output").Array()
	require.Len(t, outputs, 1)
	require.Equal(t, "compaction", outputs[0].Get("type").String())
	require.Equal(t, "compaction", gjson.Get(events[0][1], "item.type").String())
	require.NotEmpty(t, outputs[0].Get("encrypted_content").String())
}

func TestForwardOfficialOpenAIRemoteCompactionDoesNotSynthesize(t *testing.T) {
	body := mustMarshalDeepSeekCompactTestJSON(t, map[string]any{
		"model":  "gpt-5.6-sol",
		"stream": true,
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": "hello"},
			map[string]any{"type": "compaction_trigger"},
		},
	})
	c, _ := deepSeekResponsesTestContext(t, body)
	MarkOpenAINativeCompactionV2(c)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(deepSeekRemoteCompactSSEWire(
			`{"type":"response.output_item.done","item":{"type":"compaction","encrypted_content":"openai-native"}}`,
			`{"type":"response.completed","response":{"id":"resp_oa","status":"completed","output":[{"type":"compaction","encrypted_content":"openai-native"}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`,
		))),
	}}
	svc := &OpenAIGatewayService{cfg: deepSeekMappedResponsesTestConfig(), httpUpstream: upstream}

	_, err := svc.Forward(deepSeekCompactTestContext(42), c, officialOpenAIResponsesAccount(), body)
	require.NoError(t, err)
	require.Contains(t, upstream.lastReq.URL.String(), "api.openai.com")
	require.Equal(t, "compaction_trigger", gjson.GetBytes(upstream.lastBody, "input.1.type").String())
}

func TestForwardMappedDeepSeekRestoresOwnedCompactionAsUserCheckpoint(t *testing.T) {
	compactBody := mustMarshalDeepSeekCompactTestJSON(t, map[string]any{
		"model": "gpt-5.6-sol", "stream": true,
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": strings.Repeat("history ", 40)}}},
			map[string]any{"type": "compaction_trigger"},
		},
	})
	c, recorder := deepSeekResponsesTestContext(t, compactBody)
	MarkOpenAINativeCompactionV2(c)
	upstream := &httpUpstreamRecorder{resp: deepSeekRemoteCompactResponsesResponse(deepSeekRemoteCompactTestSummary)}
	svc := &OpenAIGatewayService{cfg: deepSeekMappedResponsesTestConfig(), httpUpstream: upstream}
	_, err := svc.Forward(deepSeekCompactTestContext(42), c, openaiMappedDeepSeekResponsesAccount(), compactBody)
	require.NoError(t, err)
	events := parseCompactBridgeSSE(t, recorder.Body.String())
	envelope := gjson.Get(events[0][1], "item.encrypted_content").String()
	require.NotEmpty(t, envelope)

	nextBody := mustMarshalDeepSeekCompactTestJSON(t, map[string]any{
		"model": "gpt-5.6-sol", "stream": false,
		"input": []any{
			map[string]any{"type": "compaction", "encrypted_content": envelope},
			map[string]any{"type": "message", "role": "user", "content": "continue"},
		},
	})
	nextUpstream := deepSeekResponsesJSONUpstream()
	svc.httpUpstream = nextUpstream
	nextCtx, _ := deepSeekResponsesTestContext(t, nextBody)
	_, err = svc.Forward(deepSeekCompactTestContext(42), nextCtx, openaiMappedDeepSeekResponsesAccount(), nextBody)
	require.NoError(t, err)
	require.Equal(t, "message", gjson.GetBytes(nextUpstream.lastBody, "input.0.type").String())
	require.Equal(t, "user", gjson.GetBytes(nextUpstream.lastBody, "input.0.role").String())
	require.Contains(t, gjson.GetBytes(nextUpstream.lastBody, "input.0.content.0.text").String(), deepSeekRemoteCompactTestSummary)
	require.False(t, gjson.GetBytes(nextUpstream.lastBody, `input.#(type=="compaction")`).Exists())
}

func mustMarshalDeepSeekCompactTestJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return encoded
}

func deepSeekRemoteCompactResponsesResponse(summary string) *http.Response {
	completed := deepSeekRemoteCompactCompletedPayload(summary)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Body: io.NopCloser(strings.NewReader(deepSeekRemoteCompactSSEWire(
			`{"type":"response.output_text.delta","delta":"visible"}`,
			`{"type":"response.output_item.done","output_index":0,"item":{"id":"msg_intermediate","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"this intermediate item is not the checkpoint"}]}}`,
			completed,
		))),
	}
}

func deepSeekRemoteCompactCompletedPayload(summary string) string {
	output := []any{
		map[string]any{"id": "rs_1", "type": "reasoning", "status": "completed", "summary": []any{map[string]any{"type": "summary_text", "text": "private chain of thought"}}},
		map[string]any{"id": "msg_final", "type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": summary}}},
	}
	payload, _ := json.Marshal(map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id": "resp_ds_compact", "object": "response", "model": "deepseek-flash", "status": "completed", "output": output,
			"usage": map[string]any{
				"input_tokens": 31, "output_tokens": 7, "total_tokens": 38,
				"input_tokens_details":  map[string]any{"cached_tokens": 5},
				"output_tokens_details": map[string]any{"reasoning_tokens": 3},
			},
		},
	})
	return string(payload)
}

func deepSeekRemoteCompactSSEWire(payloads ...string) string {
	var wire strings.Builder
	for _, payload := range payloads {
		if payload != "[DONE]" {
			if eventType := strings.TrimSpace(gjson.Get(payload, "type").String()); eventType != "" {
				_, _ = wire.WriteString("event: " + eventType + "\n")
			}
		}
		_, _ = wire.WriteString("data: " + payload + "\n\n")
	}
	return wire.String()
}
