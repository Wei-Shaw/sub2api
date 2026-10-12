package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const replayReasoning = `{"type":"reasoning","id":"rs_saved","content":[{"type":"reasoning_text","text":"original full thinking"}],"summary":[{"type":"summary_text","text":"short summary"}]}`
const replayCall = `{"type":"function_call","id":"fc_saved","call_id":"call_saved","name":"lookup","arguments":"{}","status":"completed"}`
const replayResponse = `{"object":"response","id":"resp_saved","model":"deepseek-v4.1-flash","status":"completed","output":[` + replayReasoning + `,` + replayCall + `],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}`

func replayContext(body []byte, chat bool, keyID int64) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	path := "/v1/responses"
	if chat {
		path = "/v1/chat/completions"
	}
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: keyID})
	return c, rec
}

func TestDeepSeekReplayCacheRestoresHistoryAndIsolatesKeys(t *testing.T) {
	cache := &reasoningRecordingCache{}
	svc := &OpenAIGatewayService{cache: cache}
	account := rawChatCompletionsTestAccount()
	c, _ := replayContext(nil, false, 10)
	model := "deepseek-v4.1-flash"
	svc.newDeepSeekReasoningCache(c, account, model, model).observe([]byte(replayResponse))
	cache.getResp = cache.snapshotSets()
	for _, history := range []string{
		`{"type":"reasoning","id":"rs_saved","summary":[],"encrypted_content":"opaque"},` + replayCall,
		`{"type":"reasoning","id":"rs_saved","summary":[{"type":"summary_text","text":"client summary"}]},` + replayCall,
		replayCall,
	} {
		body := []byte(`{"model":"` + model + `","opaque":9007199254740993,"input":[` + history + `,{"type":"function_call_output","call_id":"call_saved","output":"done"}]}`)
		out := svc.restoreDeepSeekReasoning(c, account, body, model, model)
		require.Contains(t, string(out), "original full thinking")
		require.Contains(t, string(out), "9007199254740993")
		require.Equal(t, "function_call_output", gjson.GetBytes(out, "input.2.type").String())
		other, _ := replayContext(nil, false, 11)
		require.Equal(t, body, svc.restoreDeepSeekReasoning(other, account, body, model, model))
		require.Equal(t, body, svc.restoreDeepSeekReasoning(c, account, body, "deepseek-v4-pro", "deepseek-v4-pro"))
		require.Equal(t, body, svc.restoreDeepSeekReasoning(c, account, body, "gpt-5", "gpt-5"))
	}
	body := []byte(`{"input":[` + replayReasoning + `,` + replayCall + `]}`)
	require.Equal(t, body, svc.restoreDeepSeekReasoning(c, account, body, model, model), "explicit reasoning wins byte-for-byte")
	changedCall := []byte(`{"input":[` + strings.ReplaceAll(replayCall, "lookup", "different_tool") + `]}`)
	require.Equal(t, changedCall, svc.restoreDeepSeekReasoning(c, account, changedCall, model, model))
	cache.getResp = nil // Expiry/restart: no fabrication, no error, no rewrite.
	missing := []byte(`{"input":[` + replayCall + `]}`)
	require.Equal(t, missing, svc.restoreDeepSeekReasoning(c, account, missing, model, model))
}

// Exercise actual forwarding, response parsing, and a second tool-result turn.
// The client intentionally drops the reasoning item / reasoning_content.
func TestDeepSeekReplayCacheForwardRoundTrip(t *testing.T) {
	for _, mode := range []string{"responses", "chat", "passthrough", "responses_sse", "passthrough_sse", "messages"} {
		for _, stream := range []bool{false, true} {
			chat := mode == "chat"
			messages := mode == "messages"
			name := mode + "_json"
			if stream {
				name = mode + "_stream"
			}
			t.Run(name, func(t *testing.T) {
				model := "deepseek-v4.1-flash"
				streamFlag := "false"
				if stream {
					streamFlag = "true"
				}
				body := []byte(`{"model":"` + model + `","stream":` + streamFlag + `,"input":"hello"}`)
				if chat {
					body = []byte(`{"model":"` + model + `","stream":` + streamFlag + `,"messages":[{"role":"user","content":"hello"}]}`)
				}
				if messages {
					body = []byte(`{"model":"` + model + `","max_tokens":100,"stream":` + streamFlag + `,"messages":[{"role":"user","content":"hello"}]}`)
				}
				upstreamBody := replayResponse
				contentType := "application/json"
				if stream || chat || messages || strings.HasSuffix(mode, "_sse") {
					contentType = "text/event-stream"
					upstreamBody = "data: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":" + replayReasoning + "}\n\n" +
						"data: {\"type\":\"response.output_item.added\",\"output_index\":1,\"item\":" + replayCall + "}\n\n" +
						"data: {\"type\":\"response.function_call_arguments.delta\",\"output_index\":1,\"item_id\":\"fc_saved\",\"delta\":\"{}\"}\n\n" +
						"data: {\"type\":\"response.output_item.done\",\"output_index\":1,\"item\":" + replayCall + "}\n\n" +
						"data: {\"type\":\"response.completed\",\"response\":" + replayResponse + "}\n\n"
				}
				upstream := &httpUpstreamRecorder{}
				cache := &reasoningRecordingCache{}
				svc := &OpenAIGatewayService{cache: cache, cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream, toolCorrector: NewCodexToolCorrector()}
				account := rawChatCompletionsTestAccount()
				account.Extra = map[string]any{"openai_responses_supported": true}
				if strings.HasPrefix(mode, "passthrough") {
					account.Extra["openai_passthrough"] = true
				}
				forward := func(request []byte) *httptest.ResponseRecorder {
					upstream.resp = &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(upstreamBody))}
					c, rec := replayContext(request, chat, 10)
					var result *OpenAIForwardResult
					var err error
					if chat {
						result, err = svc.forwardAsChatCompletions(context.Background(), c, account, request, "", "", false)
					} else if messages {
						c.Request.URL.Path = "/v1/messages"
						result, err = svc.ForwardAsAnthropic(context.Background(), c, account, request, "", "")
					} else {
						result, err = svc.Forward(context.Background(), c, account, request)
					}
					require.NoError(t, err)
					require.NotNil(t, result)
					require.Equal(t, 5, result.Usage.OutputTokens)
					return rec
				}
				rec := forward(body)
				require.Contains(t, rec.Body.String(), "call_saved")
				cache.getResp = cache.snapshotSets()
				require.NotEmpty(t, cache.getResp)
				next := []byte(`{"model":"` + model + `","stream":` + streamFlag + `,"input":[` + replayCall + `,{"type":"function_call_output","call_id":"call_saved","output":"done"}]}`)
				if chat {
					next = []byte(`{"model":"` + model + `","stream":` + streamFlag + `,"messages":[{"role":"assistant","tool_calls":[{"type":"function","id":"call_saved","function":{"name":"lookup","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_saved","content":"done"}]}`)
				}
				if messages {
					next = []byte(`{"model":"` + model + `","max_tokens":100,"stream":` + streamFlag + `,"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call_saved","name":"lookup","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_saved","content":"done"}]}]}`)
				}
				forward(next)
				require.Contains(t, string(upstream.lastBody), "original full thinking")
				require.Equal(t, "reasoning", gjson.GetBytes(upstream.lastBody, "input.0.type").String())
			})
		}
	}
}

func TestDeepSeekReplayCacheBoundsAndNoIdentity(t *testing.T) {
	cache := &reasoningRecordingCache{}
	svc := &OpenAIGatewayService{cache: cache}
	account := rawChatCompletionsTestAccount()
	c, _ := replayContext(nil, false, 0)
	require.Nil(t, svc.newDeepSeekReasoningCache(c, account, "deepseek-v4-flash", "deepseek-v4-flash"))
	c.Set("api_key", &APIKey{ID: 10})
	r := svc.newDeepSeekReasoningCache(c, account, "deepseek-v4-flash", "deepseek-v4-flash")
	r.access("item", "too_big", strings.Repeat("x", deepSeekReplayMaxBytes+1), true)
	require.Empty(t, cache.snapshotSets())
	r.remaining = 0
	require.Empty(t, r.access("item", "timeout", "", false))
	r.remaining = time.Second
	r.operations = deepSeekReplayMaxItems
	r.access("item", "over_limit", "text", true)
	require.Empty(t, cache.snapshotSets())
}

func TestDeepSeekReplayCacheMultipleToolTurns(t *testing.T) {
	cache := &reasoningRecordingCache{}
	svc := &OpenAIGatewayService{cache: cache}
	account := rawChatCompletionsTestAccount()
	c, _ := replayContext(nil, false, 10)
	model := "deepseek-v4.1-flash"
	second := strings.NewReplacer("call_saved", "call_second", "rs_saved", "rs_second", "original full thinking", "second thinking").Replace(replayResponse)
	svc.newDeepSeekReasoningCache(c, account, model, model).observe([]byte(replayResponse))
	svc.newDeepSeekReasoningCache(c, account, model, model).observe([]byte(second))
	cache.getResp = cache.snapshotSets()
	secondCall := strings.ReplaceAll(replayCall, "call_saved", "call_second")
	body := []byte(`{"input":[` + replayCall + `,{"type":"function_call_output","call_id":"call_saved","output":"one"},` + secondCall + `,{"type":"function_call_output","call_id":"call_second","output":"two"}]}`)
	out := svc.restoreDeepSeekReasoning(c, account, body, model, model)
	require.Equal(t, "original full thinking", gjson.GetBytes(out, "input.0.content.0.text").String())
	require.Equal(t, "second thinking", gjson.GetBytes(out, "input.3.content.0.text").String())
}

func TestDeepSeekReplayCacheCompletedOutputOrdering(t *testing.T) {
	for _, terminal := range []string{`{"type":"response.completed","response":{"status":"completed","output":[]}}`, `{"type":"response.done","response":{"status":"completed","output":[]}}`} {
		cache := &reasoningRecordingCache{}
		svc := &OpenAIGatewayService{cache: cache}
		c, _ := replayContext(nil, false, 10)
		r := svc.newDeepSeekReasoningCache(c, rawChatCompletionsTestAccount(), "deepseek-v4-flash", "deepseek-v4-flash")
		// Tool completion can arrive first. It must still bind to the preceding
		// reasoning in output order, never to whichever event arrived last.
		r.observe([]byte(`{"type":"response.output_item.done","output_index":1,"item":` + replayCall + `}`))
		r.observe([]byte(`{"type":"response.output_item.done","output_index":0,"item":` + replayReasoning + `}`))
		require.Empty(t, cache.snapshotSets(), "partial stream must not populate cache")
		r.observe([]byte(`{"type":"response.failed","response":{"status":"failed"}}`))
		require.Empty(t, cache.snapshotSets())
		r.observe([]byte(terminal))
		require.Equal(t, "original full thinking", cache.snapshotSets()[r.key("call", deepSeekReplayCallIdentity(gjson.Parse(replayCall)))])
	}
}

func TestDeepSeekReplayCacheDeltaOnly(t *testing.T) {
	cache := &reasoningRecordingCache{}
	svc := &OpenAIGatewayService{cache: cache}
	c, _ := replayContext(nil, false, 10)
	r := svc.newDeepSeekReasoningCache(c, rawChatCompletionsTestAccount(), "deepseek-v4-flash", "deepseek-v4-flash")
	for _, event := range []string{
		`{"type":"response.reasoning_text.delta","output_index":0,"delta":"full thought"}`,
		`{"type":"response.reasoning_summary_text.delta","output_index":0,"delta":"summary must not be appended"}`,
		`{"type":"response.output_item.added","output_index":1,"item":` + replayCall + `}`,
		`{"type":"response.function_call_arguments.delta","output_index":1,"delta":"{}"}`,
		`{"type":"response.completed","response":{"status":"completed","output":[]}}`,
	} {
		r.observe([]byte(event))
	}
	require.Equal(t, "full thought", cache.snapshotSets()[r.key("call", deepSeekReplayCallIdentity(gjson.Parse(replayCall)))])
}

type unavailableReplayCache struct{ *reasoningRecordingCache }

func (c unavailableReplayCache) GetReasoningContent(ctx context.Context, key string) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func TestDeepSeekReplayCacheUnavailableDoesNotBlockRequest(t *testing.T) {
	svc := &OpenAIGatewayService{cache: unavailableReplayCache{&reasoningRecordingCache{}}}
	c, _ := replayContext(nil, false, 10)
	body := []byte(`{"input":[` + replayCall + `,` + replayCall + `]}`)
	start := time.Now()
	out := svc.restoreDeepSeekReasoning(c, rawChatCompletionsTestAccount(), body, "deepseek-v4-flash", "deepseek-v4-flash")
	require.Equal(t, body, out)
	require.Less(t, time.Since(start), time.Second, "cache deadline must be shared across calls")
}

func TestDeepSeekReplayCacheBeforeClientToolAdaptation(t *testing.T) {
	for _, call := range []string{
		`{"type":"function_call","call_id":"call_saved","namespace":"workspace","name":"lookup","arguments":"{}"}`,
		`{"type":"custom_tool_call","call_id":"call_saved","name":"apply_patch","input":"patch text"}`,
	} {
		cache := &reasoningRecordingCache{}
		upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(replayResponse))}}
		svc := &OpenAIGatewayService{cache: cache, cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream, toolCorrector: NewCodexToolCorrector()}
		account := rawChatCompletionsTestAccount()
		account.Extra = map[string]any{"openai_responses_supported": true}
		model := "deepseek-v4.1-flash"
		c, _ := replayContext(nil, false, 10)
		svc.newDeepSeekReasoningCache(c, account, model, model).observe([]byte(strings.Replace(replayResponse, replayCall, call, 1)))
		cache.getResp = cache.snapshotSets()
		body := []byte(`{"model":"` + model + `","input":[` + call + `]}`)
		c, _ = replayContext(body, false, 10)
		_, err := svc.Forward(context.Background(), c, account, body)
		require.NoError(t, err)
		require.Equal(t, "original full thinking", gjson.GetBytes(upstream.lastBody, "input.0.content.0.text").String())
	}
}
