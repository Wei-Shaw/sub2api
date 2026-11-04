package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 0.166.0 serializes base instructions as input developer messages, not as
// top-level instructions. Keep replay/optional fields opaque to the gateway.
const codex166ProtocolFixture = `{
 "model":"gpt-5.3-codex","store":false,"stream":true,
 "input":[
  {"type":"message","id":"msg_base","role":"developer","content":[{"type":"input_text","text":"client-owned base instructions"}]},
  {"type":"message","role":"assistant","phase":"partial_answer","encrypted_content":"message-cipher","content":[{"type":"output_text","text":"working","annotations":[],"logprobs":[]}]},
  {"type":"function_call","call_id":"fc_lookup","name":"lookup","arguments":"{}","encrypted_content":"call-cipher","encrypted_function_args":"args-cipher"},
  {"type":"function_call_output","call_id":"fc_lookup","output":"done"},
  {"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}
 ],
 "tools":[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{},"additionalProperties":false}}],
 "tool_choice":"auto","parallel_tool_calls":true,
 "reasoning":{"effort":"high","summary":"auto"},
 "include":["reasoning.encrypted_content","output.encrypted_content"],
 "stream_options":{"reasoning_summary_delivery":"sequential_cutoff"},
 "text":{"format":{"type":"json_schema","name":"answer","strict":true,"schema":{"type":"object","properties":{},"additionalProperties":false}}},
 "access_programs":{"cyber":"standard"},
 "client_metadata":{"root_turn_id":"fixture-root","future_key":"preserved"},
 "future_optional_field":{"number":9007199254740993}
}`

func TestCodex166DeveloperInstructionsTransform(t *testing.T) {
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(codex166ProtocolFixture), &body))
	result := applyCodexOAuthTransform(body, true, false)
	require.NoError(t, result.Error)
	require.NotContains(t, body, "instructions")
	require.Equal(t, "client-owned base instructions", extractPromptLikeInstructionsFromInput(body))
}

func TestCodex166RequestFieldsForward(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		t.Run(fmt.Sprintf("passthrough=%t", passthrough), func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			body := []byte(codex166ProtocolFixture)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			c.Request.Header.Set("User-Agent", codexCLIUserAgent)
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_fixture\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\ndata: [DONE]\n\n")),
			}}
			svc := &OpenAIGatewayService{httpUpstream: upstream}
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "fixture", "chatgpt_account_id": "fixture-account"}, Extra: map[string]any{"openai_passthrough": passthrough}}
			_, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotEmpty(t, upstream.lastBody)
			require.Equal(t, "9007199254740993", gjson.GetBytes(upstream.lastBody, "future_optional_field.number").Raw)
			require.False(t, gjson.GetBytes(upstream.lastBody, "instructions").Exists(), string(upstream.lastBody))
			for _, path := range []string{"include", "stream_options", "text", "access_programs", "client_metadata.future_key", "client_metadata.root_turn_id", "future_optional_field", "input.#(role==developer).content", "input.#(phase==partial_answer).content", "input.#(phase==partial_answer).encrypted_content", "input.#(type==function_call).encrypted_content", "input.#(type==function_call).encrypted_function_args"} {
				require.JSONEq(t, gjson.GetBytes(body, path).Raw, gjson.GetBytes(upstream.lastBody, path).Raw, path)
			}
		})
	}
}

func TestCodex166StreamOptionsAcrossTransports(t *testing.T) {
	for _, ws := range []bool{false, true} {
		for _, raw := range []string{
			`{"stream_options":{"reasoning_summary_delivery":"sequential_cutoff"}}`,
			`{"stream_options":{"reasoning_summary_delivery":"sequential_cutoff","include_usage":true}}`,
			`{"stream_options":{"include_usage":true}}`,
		} {
			var updated []byte
			var err error
			if ws {
				updated, _, err = normalizeOpenAIResponsesWebSocketCompatibilityBody([]byte(raw), &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, false)
			} else {
				updated, _, err = normalizeOpenAIPassthroughOAuthBody([]byte(raw), false)
			}
			require.NoError(t, err)
			want := gjson.Get(raw, "stream_options.reasoning_summary_delivery").Exists()
			require.Equal(t, want, gjson.GetBytes(updated, "stream_options.reasoning_summary_delivery").Exists())
			require.False(t, gjson.GetBytes(updated, "stream_options.include_usage").Exists())
		}
	}
}

func TestCodex166LegacyCompactDropsStreamingOptions(t *testing.T) {
	raw := []byte(codex166ProtocolFixture)
	updated, _, err := normalizeOpenAIPassthroughOAuthBody(raw, true)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(updated, "stream_options").Exists())
	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	result := applyCodexOAuthTransform(body, true, true)
	require.NoError(t, result.Error)
	require.NotContains(t, body, "stream_options")
}
