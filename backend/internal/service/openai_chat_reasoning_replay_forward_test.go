//go:build unit

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
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func replayForwardTestAccount() *Account {
	return &Account{
		ID:          9,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}
}

func replayForwardTestContext(t *testing.T, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, rec
}

// A Codex Responses stream: reasoning (with encrypted_content) followed by a
// function call, then the terminal event.
func replayCodexToolCallStream() string {
	return strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_turn1","model":"gpt-5.6","status":"in_progress"}}`,
		``,
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[]}}`,
		``,
		`data: {"type":"response.reasoning_summary_text.delta","output_index":0,"delta":"look at the tree"}`,
		``,
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"look at the tree"}],"encrypted_content":"ENC_TURN1"}}`,
		``,
		`data: {"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_ls_1","name":"ls","arguments":""}}`,
		``,
		`data: {"type":"response.function_call_arguments.delta","output_index":1,"delta":"{\"path\":\".\"}"}`,
		``,
		`data: {"type":"response.output_item.done","output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_ls_1","name":"ls","arguments":"{\"path\":\".\"}","status":"completed"}}`,
		``,
		`data: {"type":"response.completed","response":{"id":"resp_turn1","model":"gpt-5.6","status":"completed","output":[],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}`,
		``,
	}, "\n")
}

func replayStreamResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_replay"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func replayInputItems(t *testing.T, upstreamBody []byte) []map[string]any {
	t.Helper()
	var items []map[string]any
	require.NoError(t, json.Unmarshal([]byte(gjson.GetBytes(upstreamBody, "input").Raw), &items), string(upstreamBody))
	return items
}

// Turn 1: the streamed tool call is delivered to the client unchanged and the
// reasoning ciphertext is cached under the tool call id.
// Turn 2: the client replays the tool call history; the upstream body now
// carries the reasoning item right before the function_call and the
// plaintext <thinking> echo is gone.
func TestForwardAsChatCompletions_ReasoningReplayAcrossToolTurns(t *testing.T) {
	cache := &reasoningRecordingCache{}
	upstream := &httpUpstreamRecorder{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, cache: cache}
	account := replayForwardTestAccount()

	// --- turn 1 ---
	turn1 := []byte(`{"model":"gpt-5.6","stream":true,"reasoning_effort":"high","tools":[{"type":"function","function":{"name":"ls","parameters":{"type":"object"}}}],"messages":[{"role":"user","content":"list files"}]}`)
	c1, rec1 := replayForwardTestContext(t, turn1)
	upstream.responses = []*http.Response{replayStreamResponse(replayCodexToolCallStream())}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c1, account, turn1, "", "gpt-5.6")
	require.NoError(t, err)
	require.NotNil(t, result)

	clientStream := rec1.Body.String()
	assert.Contains(t, clientStream, `"reasoning_content":"look at the tree"`)
	assert.Contains(t, clientStream, `"id":"call_ls_1"`)
	assert.NotContains(t, clientStream, "ENC_TURN1", "ciphertext must never leak to the chat client")

	stored := cache.snapshotSets()
	raw, ok := stored[chatReasoningReplayKeyPrefix+"call_ls_1"]
	require.True(t, ok, "turn 1 must cache reasoning under the tool call id, got %v", stored)
	var record chatReasoningReplayRecord
	require.NoError(t, json.Unmarshal([]byte(raw), &record))
	assert.Equal(t, account.ID, record.AccountID)
	assert.Equal(t, "resp_turn1", record.ResponseID)
	assert.Equal(t, []string{"ENC_TURN1"}, record.Items)

	// --- turn 2 ---
	cache.getResp = stored
	turn2 := []byte(`{"model":"gpt-5.6","stream":true,"reasoning_effort":"high","tools":[{"type":"function","function":{"name":"ls","parameters":{"type":"object"}}}],"messages":[
		{"role":"user","content":"list files"},
		{"role":"assistant","content":null,"reasoning_content":"look at the tree","tool_calls":[{"id":"call_ls_1","type":"function","function":{"name":"ls","arguments":"{\"path\":\".\"}"}}]},
		{"role":"tool","tool_call_id":"call_ls_1","content":"a.txt\nb.txt"}
	]}`)
	c2, _ := replayForwardTestContext(t, turn2)
	upstream.responses = []*http.Response{replayStreamResponse(strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_turn2","model":"gpt-5.6","status":"in_progress"}}`,
		``,
		`data: {"type":"response.output_text.delta","output_index":0,"delta":"two files"}`,
		``,
		`data: {"type":"response.completed","response":{"id":"resp_turn2","model":"gpt-5.6","status":"completed","output":[],"usage":{"input_tokens":20,"output_tokens":2,"total_tokens":22}}}`,
		``,
	}, "\n"))}

	result, err = svc.ForwardAsChatCompletions(context.Background(), c2, account, turn2, "", "gpt-5.6")
	require.NoError(t, err)
	require.NotNil(t, result)

	items := replayInputItems(t, upstream.lastBody)
	types := make([]string, 0, len(items))
	for _, item := range items {
		typ, _ := item["type"].(string)
		if typ == "" {
			typ, _ = item["role"].(string)
		}
		types = append(types, typ)
	}
	require.Equal(t, []string{"user", "reasoning", "function_call", "function_call_output"}, types, string(upstream.lastBody))
	assert.Equal(t, "ENC_TURN1", items[1]["encrypted_content"])
	assert.Equal(t, []any{}, items[1]["summary"])
	_, hasID := items[1]["id"]
	assert.False(t, hasID, "codex transform must not attach an rs_ id to the replayed item")
	// The codex transform normalizes call ids for the upstream (call_ -> fc_);
	// the replayed reasoning item still sits directly before that call.
	assert.Contains(t, []any{"call_ls_1", "fc_ls_1"}, items[2]["call_id"])
	assert.NotContains(t, gjson.GetBytes(upstream.lastBody, "input").String(), "thinking")
	assert.Contains(t, gjson.GetBytes(upstream.lastBody, "include").Raw, "reasoning.encrypted_content")
}

// A miss keeps the previous request shape: the plaintext reasoning_content is
// echoed as <thinking> and no reasoning item is inserted.
func TestForwardAsChatCompletions_ReasoningReplayMissKeepsLegacyBody(t *testing.T) {
	cache := &reasoningRecordingCache{}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"stop before response parsing"}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, cache: cache}

	body := []byte(`{"model":"gpt-5.6","stream":true,"messages":[
		{"role":"user","content":"list files"},
		{"role":"assistant","content":null,"reasoning_content":"look at the tree","tool_calls":[{"id":"call_unknown","type":"function","function":{"name":"ls","arguments":"{}"}}]},
		{"role":"tool","tool_call_id":"call_unknown","content":"a.txt"}
	]}`)
	c, _ := replayForwardTestContext(t, body)
	_, err := svc.ForwardAsChatCompletions(context.Background(), c, replayForwardTestAccount(), body, "", "gpt-5.6")
	require.Error(t, err)
	require.Len(t, upstream.bodies, 1)

	items := replayInputItems(t, upstream.lastBody)
	for _, item := range items {
		assert.NotEqual(t, "reasoning", item["type"])
	}
	assert.Equal(t, "<thinking>look at the tree</thinking>", gjson.GetBytes(upstream.lastBody, "input.1.content.0.text").String())
}

// When the upstream rejects the injected ciphertext (typically after the
// sticky account changed), the request is retried once without replay and
// the client still gets the answer.
func TestForwardAsChatCompletions_ReasoningReplayRetriesWithoutReplayOnRejectedCipher(t *testing.T) {
	record, _ := json.Marshal(chatReasoningReplayRecord{AccountID: 9, Items: []string{"ENC_STALE"}, CreatedAt: 1})
	cache := &reasoningRecordingCache{getResp: map[string]string{chatReasoningReplayKeyPrefix + "call_ls_1": string(record)}}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_reject"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","code":"invalid_encrypted_content","message":"The encrypted content could not be verified"}}`)),
		},
		replayStreamResponse(strings.Join([]string{
			`data: {"type":"response.created","response":{"id":"resp_retry","model":"gpt-5.6","status":"in_progress"}}`,
			``,
			`data: {"type":"response.output_text.delta","output_index":0,"delta":"ok"}`,
			``,
			`data: {"type":"response.completed","response":{"id":"resp_retry","model":"gpt-5.6","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`,
			``,
		}, "\n")),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, cache: cache}

	body := []byte(`{"model":"gpt-5.6","stream":true,"messages":[
		{"role":"user","content":"list files"},
		{"role":"assistant","content":null,"tool_calls":[{"id":"call_ls_1","type":"function","function":{"name":"ls","arguments":"{}"}}]},
		{"role":"tool","tool_call_id":"call_ls_1","content":"a.txt"}
	]}`)
	c, rec := replayForwardTestContext(t, body)
	result, err := svc.ForwardAsChatCompletions(context.Background(), c, replayForwardTestAccount(), body, "", "gpt-5.6")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2, "exactly one retry")

	assert.Contains(t, string(upstream.bodies[0]), "ENC_STALE", "first attempt injects the cached reasoning")
	assert.NotContains(t, string(upstream.bodies[1]), "ENC_STALE", "retry must not inject again")
	for _, item := range replayInputItems(t, upstream.bodies[1]) {
		assert.NotEqual(t, "reasoning", item["type"])
	}
	assert.Contains(t, rec.Body.String(), `"content":"ok"`)
}
