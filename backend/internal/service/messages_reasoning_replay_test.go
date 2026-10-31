//go:build unit

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

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func reasoningReplayHTTPResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func reasoningReplayMessagesBody(stream bool, signature string) []byte {
	return []byte(fmt.Sprintf(`{"model":"claude-opus-5-5","max_tokens":32,"stream":%v,"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{}}}],"messages":[{"role":"user","content":"hello"},{"role":"assistant","content":[{"type":"thinking","signature":%q},{"type":"thinking","signature":"CAQS-native"},{"type":"text","text":"visible"},{"type":"tool_use","id":"toolu_1","name":"lookup","input":{"n":9007199254740993}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"result"}]}]}`, stream, signature))
}

func reasoningReplayContext(body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("session_id", "client-session")
	c.Request.Header.Set("conversation_id", "client-conversation")
	c.Request.Header.Set("x-codex-turn-state", "stable-turn-state")
	return c, rec
}

func TestForwardAsAnthropicReasoningReplayHTTPRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		for _, oauth := range []bool{false, true} {
			for _, status := range []int{400, 502} {
				t.Run(fmt.Sprintf("stream=%v/oauth=%v/status=%d", stream, oauth, status), func(t *testing.T) {
					body := reasoningReplayMessagesBody(stream, "gAAAA-stale")
					c, rec := reasoningReplayContext(body)
					upstream := &httpUpstreamRecorder{responses: []*http.Response{
						reasoningReplayHTTPResponse(status, `{"error":{"code":"invalid_encrypted_content","message":"could not verify ciphertext"}}`),
						openAICompatSSECompletedResponse("resp_recovered", "gpt-6.1-sol"),
					}}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
					account := rawChatCompletionsTestAccount()
					if oauth {
						account.Type = AccountTypeOAuth
						account.Credentials = map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"}
					}
					result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "stable-cache-key", "gpt-6.1-sol")
					require.NoError(t, err)
					require.NotNil(t, result)
					require.Equal(t, 200, rec.Code)
					require.Len(t, upstream.bodies, 2)
					require.Contains(t, string(upstream.bodies[0]), "gAAAA-stale")
					for _, item := range gjson.GetBytes(upstream.bodies[0], "input").Array() {
						if item.Get("type").String() == "reasoning" {
							require.Equal(t, "[]", item.Get("summary").Raw, "API key and OAuth replay both require summary:[]")
						} else {
							require.False(t, item.Get("summary").Exists())
						}
					}
					require.NotContains(t, string(upstream.bodies[0]), "CAQS-native")
					require.NotContains(t, string(upstream.bodies[1]), "gAAAA-stale")
					expected, changed, err := SanitizeOpenAICrossModeFailoverReasoning(upstream.bodies[0])
					require.NoError(t, err)
					require.True(t, changed)
					require.JSONEq(t, string(expected), string(upstream.bodies[1]), "only rejected reasoning should change")
					require.Contains(t, string(upstream.bodies[1]), "9007199254740993")
					require.Contains(t, string(upstream.bodies[1]), "toolu_1")
					require.Contains(t, string(upstream.bodies[1]), "result")
					for _, header := range []string{"session_id", "conversation_id", "User-Agent", "originator", "OpenAI-Beta", "Chatgpt-Account-Id", "x-codex-turn-state", "Authorization"} {
						require.Equal(t, upstream.requests[0].Header.Values(header), upstream.requests[1].Header.Values(header), header)
					}
					require.NotEmpty(t, upstream.requests[0].Header.Get("session_id"))
					if oauth {
						require.NotEmpty(t, upstream.requests[0].Header.Get("originator"))
						require.NotEmpty(t, upstream.requests[0].Header.Get("OpenAI-Beta"))
					}
				})
			}
		}
	}
}

func TestForwardAsAnthropicReasoningReplayNoBroadRetry(t *testing.T) {
	for _, tc := range []struct {
		name           string
		status         int
		code, msg, sig string
		twice          bool
	}{
		{"ordinary400", 400, "invalid_request_error", "bad request", "gAAAA-stale", false},
		{"generic502", 502, "upstream_error", "invalid_encrypted_content", "gAAAA-stale", false},
		{"wrongStatus", 503, "invalid_encrypted_content", "bad", "gAAAA-stale", false},
		{"nativeAlreadyFiltered", 502, "invalid_encrypted_content", "bad", "CAQS-native", false},
		{"unknownAlreadyFiltered", 400, "invalid_encrypted_content", "bad", "opaque", false},
		{"secondFailureTerminal", 502, "invalid_encrypted_content", "bad", "gAAAA-stale", true},
		{"unrelatedAlias", 400, "thinking_signature_invalid", "Invalid signature", "gAAAA-stale", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := reasoningReplayMessagesBody(false, tc.sig)
			c, _ := reasoningReplayContext(body)
			errorBody, _ := json.Marshal(map[string]any{"error": map[string]string{"code": tc.code, "message": tc.msg}})
			upstream := &httpUpstreamRecorder{resp: reasoningReplayHTTPResponse(tc.status, string(errorBody))}
			if tc.twice {
				upstream.responses = []*http.Response{reasoningReplayHTTPResponse(tc.status, string(errorBody)), reasoningReplayHTTPResponse(tc.status, string(errorBody))}
			}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			_, err := svc.ForwardAsAnthropic(context.Background(), c, rawChatCompletionsTestAccount(), body, "stable-cache-key", "gpt-6.1-sol")
			require.Error(t, err)
			want := 1
			if tc.twice {
				want = 2
			}
			require.Len(t, upstream.bodies, want)
		})
	}
}

func TestForwardAsAnthropicReasoningReplaySSEBeforeOutput(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, kind := range []string{"error", "response.failed"} {
			t.Run(fmt.Sprintf("stream=%v/%s", stream, kind), func(t *testing.T) {
				body := reasoningReplayMessagesBody(stream, "gAAAA-stale")
				c, rec := reasoningReplayContext(body)
				payload := `{"type":"error","error":{"code":"invalid_encrypted_content","message":"bad ciphertext"}}`
				if kind == "response.failed" {
					payload = `{"type":"response.failed","response":{"status":"failed","error":{"code":"invalid_encrypted_content","message":"bad ciphertext"}}}`
				}
				upstream := &httpUpstreamRecorder{responses: []*http.Response{
					reasoningReplayHTTPResponse(200, "data: "+payload+"\n\n"), openAICompatSSECompletedResponse("resp_sse_recovered", "gpt-6.1-sol"),
				}}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				_, err := svc.ForwardAsAnthropic(context.Background(), c, rawChatCompletionsTestAccount(), body, "stable-cache-key", "gpt-6.1-sol")
				require.NoError(t, err)
				require.Len(t, upstream.bodies, 2)
				require.NotContains(t, rec.Body.String(), "bad ciphertext")
				require.NotContains(t, string(upstream.bodies[1]), "gAAAA-stale")
			})
		}
	}
}

func TestForwardAsAnthropicReasoningReplayNeverRetriesAfterStreamOutput(t *testing.T) {
	body := reasoningReplayMessagesBody(true, "gAAAA-stale")
	c, rec := reasoningReplayContext(body)
	upstream := &httpUpstreamRecorder{resp: reasoningReplayHTTPResponse(200, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_started\",\"status\":\"in_progress\"}}\n\ndata: {\"type\":\"error\",\"error\":{\"code\":\"invalid_encrypted_content\",\"message\":\"bad ciphertext\"}}\n\n")}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	_, err := svc.ForwardAsAnthropic(context.Background(), c, rawChatCompletionsTestAccount(), body, "stable-cache-key", "gpt-6.1-sol")
	require.Error(t, err)
	require.Len(t, upstream.bodies, 1)
	require.Contains(t, rec.Body.String(), "event: error")
}

func TestMessagesReasoningReplayCompactionUntouched(t *testing.T) {
	for _, reasoning := range []bool{false, true} {
		input := `{"type":"compaction","id":"cmp_1","encrypted_content":"gAAAA-compaction"}`
		if reasoning {
			input += `,{"type":"reasoning","encrypted_content":"gAAAA-stale"}`
		}
		body := []byte(`{"model":"gpt-6.1-sol","tools":[{"type":"function","name":"lookup"}],"input":[` + input + `,{"type":"function_call_output","call_id":"toolu_1","output":"result"}]}`)
		cleaned, changed, err := SanitizeOpenAICrossModeFailoverReasoning(body)
		require.NoError(t, err)
		require.Equal(t, reasoning, changed)
		require.Equal(t, "gAAAA-compaction", gjson.GetBytes(cleaned, "input.0.encrypted_content").String())
		require.Equal(t, "cmp_1", gjson.GetBytes(cleaned, "input.0.id").String())
		require.Contains(t, string(cleaned), "toolu_1")
		require.False(t, messagesRequestHasEncryptedReasoning(cleaned))
	}
}

func TestMessagesReasoningReplayPolicyUsesActualAccount(t *testing.T) {
	for _, tc := range []struct {
		platform, sig string
		want          bool
	}{
		{PlatformOpenAI, "gAAAA-valid", true}, {PlatformOpenAI, "CAQS-native", false}, {PlatformOpenAI, "enc-rs-1", false},
		{PlatformGrok, "enc-rs-1", true}, {PlatformGrok, "gAAAA-valid", false}, {PlatformGrok, "CAQS-native", false},
		{"custom", "gAAAA-valid", false}, {"custom", "opaque", false},
	} {
		req := &apicompat.AnthropicRequest{Model: "claude-opus-5-5", Messages: []apicompat.AnthropicMessage{{Role: "assistant", Content: json.RawMessage(fmt.Sprintf(`[{"type":"thinking","signature":%q},{"type":"text","text":"hello"}]`, tc.sig))}}}
		out, err := anthropicToResponses(nil, req, &Account{Platform: tc.platform})
		require.NoError(t, err)
		require.Equal(t, tc.want, messagesRequestHasEncryptedReasoning(mustMarshalMessagesReplay(t, out)))
	}
}
func mustMarshalMessagesReplay(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func TestDropClearlyForeignAnthropicThinking(t *testing.T) {
	body := []byte(`{"model":"claude-opus-5-5","messages":[{"role":"assistant","content":[{"type":"thinking","signature":"gAAAA-openai","thinking":"foreign"},{"type":"text","text":"visible","cache_control":{"type":"ephemeral"}},{"type":"thinking","signature":"CAQS-native","thinking":"native"},{"type":"tool_use","id":"toolu_1","name":"lookup","input":{"n":9007199254740993}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"result"}]},{"role":"assistant","content":[{"type":"thinking","signature":"gAAAA-only"}]}]}`)
	cleaned, changed := dropClearlyForeignAnthropicThinking(body)
	require.True(t, changed)
	require.NotContains(t, string(cleaned), "gAAAA")
	require.Len(t, gjson.GetBytes(cleaned, "messages").Array(), 2)
	require.Equal(t, "visible", gjson.GetBytes(cleaned, "messages.0.content.0.text").String())
	require.Equal(t, "CAQS-native", gjson.GetBytes(cleaned, "messages.0.content.1.signature").String())
	require.Contains(t, string(cleaned), "9007199254740993")
	require.Contains(t, string(cleaned), "cache_control")
	twice, changed := dropClearlyForeignAnthropicThinking(cleaned)
	require.False(t, changed)
	require.Equal(t, string(cleaned), string(twice))
}

func TestMessagesReasoningReplayClassifier(t *testing.T) {
	strict := `{"error":{"code":"thinking_signature_invalid","message":"The encrypted content abc could not be verified. Reason: Encrypted content could not be decrypted or parsed."}}`
	require.True(t, isMessagesReasoningReplayError(400, []byte(strict)))
	require.True(t, isMessagesReasoningReplayError(502, []byte(strict)))
	require.False(t, isMessagesReasoningReplayError(503, []byte(strict)))
	require.False(t, isMessagesReasoningReplayError(400, []byte(`{"error":{"message":"invalid_encrypted_content"}}`)))
}

func TestForwardAsAnthropicReasoningReplayMarkerSurvivesContinuationFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// An already-cleaned request can recursively fall back from previous_response_id.
	// Neither recursive entry may restore rejected ciphertext or acquire a new retry.
	body := reasoningReplayMessagesBody(false, "gAAAA-stale")
	c, _ := reasoningReplayContext(body)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		reasoningReplayHTTPResponse(400, `{"error":{"message":"previous_response_id is not available for this user"}}`),
		reasoningReplayHTTPResponse(502, `{"error":{"code":"invalid_encrypted_content","message":"still invalid"}}`),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := rawGPT56ResponsesAPIKeyAccount("claude-opus-5-5", "gpt-6-astra")
	svc.bindOpenAICompatSessionResponseID(context.Background(), c, account, "stable-cache-key", "resp_previous")
	ctx := markMessagesReasoningReplayRetried(context.Background())
	_, err := svc.ForwardAsAnthropic(ctx, c, account, body, "stable-cache-key", "gpt-6-astra")
	require.Error(t, err)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "resp_previous", gjson.GetBytes(upstream.bodies[0], "previous_response_id").String())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "previous_response_id").Exists())
	for _, sent := range upstream.bodies {
		require.NotContains(t, string(sent), "gAAAA-stale")
	}
}

func TestForwardAsAnthropicReasoningReplayPreservesCachedTurnState(t *testing.T) {
	body := reasoningReplayMessagesBody(false, "gAAAA-stale")
	c, _ := reasoningReplayContext(body)
	c.Request.Header.Del("x-codex-turn-state")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{reasoningReplayHTTPResponse(502, `{"error":{"code":"invalid_encrypted_content"}}`), openAICompatSSECompletedResponse("resp_ok", "gpt-6.1-sol")}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"}}
	svc.bindOpenAICompatSessionTurnState(context.Background(), c, account, "stable-cache-key", "cached-turn-state")
	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "stable-cache-key", "gpt-6.1-sol")
	require.NoError(t, err)
	require.Len(t, upstream.requests, 2)
	for _, req := range upstream.requests {
		require.Equal(t, "cached-turn-state", req.Header.Get("x-codex-turn-state"))
	}
}
