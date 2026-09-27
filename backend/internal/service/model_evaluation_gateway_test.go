//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestModelEvaluationRealGatewayProtocols(t *testing.T) {
	for _, protocol := range []string{"responses", "chat", "oauth"} {
		t.Run(protocol, func(t *testing.T) {
			s, _, _ := newEvaluationTestService()
			account := rawChatCompletionsTestAccount()
			account.Status, account.Schedulable = StatusActive, true
			response := evaluationResponse
			contentType := "application/json"
			if protocol == "oauth" {
				account.Type = AccountTypeOAuth
				account.Credentials = map[string]any{"access_token": "sk-test", "chatgpt_account_id": "evaluation-account"}
				response = "data: {\"type\":\"response.completed\",\"response\":" + evaluationResponse + "}\n\n"
				contentType = "text/event-stream"
			}
			if protocol == "chat" {
				account.Extra = map[string]any{openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions)}
				response = `{"id":"chat1","model":"gpt-5.4","choices":[{"message":{"role":"assistant","content":"FINAL_ANSWER: 21"},"finish_reason":"stop"}],"usage":{"prompt_tokens":120,"completion_tokens":44,"completion_tokens_details":{"reasoning_tokens":30}}}`
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(response)),
			}}
			s.gateway = &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			s.accounts = evaluationAccountsStub{account: account}
			result, err := s.Run(context.Background(), ModelEvaluationRequest{TargetType: "account", TargetID: account.ID, Model: "gpt-5.4", Effort: "high"})
			require.NoError(t, err)
			require.Equal(t, "correct", result.Status, result.Error)
			require.NotNil(t, result.InputTokens)
			require.EqualValues(t, 120, *result.InputTokens)
			require.NotNil(t, result.ReasoningTokens)
			require.EqualValues(t, 30, *result.ReasoningTokens)
			require.Equal(t, "Bearer sk-test", upstream.lastReq.Header.Get("Authorization"))
			require.Contains(t, string(upstream.lastBody), "不能识别口味")
			require.NotContains(t, result.Text, "sk-test")
			_, hasDeadline := upstream.lastReq.Context().Deadline()
			require.True(t, hasDeadline)
			if protocol == "chat" {
				require.Equal(t, "/v1/chat/completions", upstream.lastReq.URL.Path)
				require.Equal(t, "high", gjson.GetBytes(upstream.lastBody, "reasoning_effort").String())
			} else if protocol == "responses" {
				require.Equal(t, "/v1/responses", upstream.lastReq.URL.Path)
				require.Equal(t, "high", gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
			} else {
				require.Equal(t, "chatgpt.com", upstream.lastReq.URL.Host)
				require.Equal(t, "/backend-api/codex/responses", upstream.lastReq.URL.Path)
				require.False(t, gjson.GetBytes(upstream.lastBody, "store").Bool())
			}
		})
	}
}
