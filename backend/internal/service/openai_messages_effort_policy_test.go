//go:build unit

package service

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMessagesEffortPolicyHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	noneToLow := []ReasoningEffortMapping{{From: "none", To: "low"}}
	for _, chat := range []bool{false, true} {
		for _, auth := range []string{AccountTypeAPIKey, AccountTypeOAuth} {
			for _, stream := range []bool{false, true} {
				for _, test := range []struct {
					name       string
					preference string
					mappings   []ReasoningEffortMapping
					ceiling    string
					model      string
					terminal   bool
					want       string
					status     int
				}{
					{name: "omitted unchanged", want: "medium"},
					{name: "omitted unrelated mapping unchanged", mappings: []ReasoningEffortMapping{{From: "medium", To: "low"}}, want: "medium"},
					{name: "omitted ceiling unchanged", ceiling: "low", want: "medium"},
					{name: "omitted none mapping and ceiling unchanged", mappings: noneToLow, ceiling: "low", want: "medium"},
					{name: "omitted none mapping leaves default", mappings: noneToLow, want: "medium"},
					{name: "disabled omitted mapped", preference: `,"thinking":{"type":"disabled"}`, mappings: noneToLow, want: "low"},
					{name: "disabled low mapped", preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"low"}`, mappings: noneToLow, want: "low"},
					{name: "disabled high mapped", preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"high"}`, mappings: noneToLow, want: "low"},
					{name: "disabled max mapped", preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"max"}`, mappings: noneToLow, want: "low"},
					{name: "disabled no mapping rejected", preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"high"}`, status: 400},
					{name: "disabled omitted no mapping rejected", preference: `,"thinking":{"type":"disabled"}`, status: 400},
					{name: "disabled unrelated mapping rejected", preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"high"}`, mappings: []ReasoningEffortMapping{{From: "high", To: "low"}}, status: 400},
					{name: "disabled wrong model scope rejected", preference: `,"thinking":{"type":"disabled"}`, mappings: []ReasoningEffortMapping{{From: "none", To: "low", Model: "gpt-6-sol", MatchType: "exact"}}, status: 400},
					{name: "raw none cannot be mapped away", preference: `,"output_config":{"effort":"none"}`, mappings: noneToLow, status: 400},
					{name: "raw none disabled cannot be hidden", preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"none"}`, mappings: noneToLow, status: 400},
					{name: "raw minimal disabled cannot be hidden", preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"minimal"}`, mappings: append(append([]ReasoningEffortMapping{}, noneToLow...), ReasoningEffortMapping{From: "minimal", To: "low"}), status: 400},
					{name: "raw minimal cannot be mapped away", preference: `,"output_config":{"effort":"minimal"}`, mappings: []ReasoningEffortMapping{{From: "minimal", To: "low"}}, status: 400},
					{name: "raw reasoning none cannot be hidden", preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"high"},"reasoning":{"effort":"none"}`, mappings: noneToLow, status: 400},
					{name: "raw chat minimal cannot be hidden", preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"high"},"reasoning_effort":"minimal"`, mappings: noneToLow, status: 400},
					{name: "raw alias cannot be hidden", model: "gpt-6.1-sol-minimal", preference: `,"thinking":{"type":"disabled"}`, mappings: noneToLow, status: 400},
					{name: "one hop mapping", preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"high"}`, mappings: []ReasoningEffortMapping{{From: "none", To: "low"}, {From: "low", To: "high"}}, want: "low"},
					{name: "mapping final unsupported rejected", preference: `,"thinking":{"type":"disabled"}`, mappings: []ReasoningEffortMapping{{From: "none", To: "minimal"}}, status: 400},
					{name: "mapping deny preserved", preference: `,"thinking":{"type":"disabled"}`, mappings: []ReasoningEffortMapping{{From: "none", To: "deny"}}, status: 403},
					{name: "adaptive explicit unchanged", preference: `,"thinking":{"type":"adaptive"},"output_config":{"effort":"high"}`, mappings: noneToLow, want: "high"},
					{name: "adaptive explicit mapped", preference: `,"thinking":{"type":"adaptive"},"output_config":{"effort":"high"}`, mappings: []ReasoningEffortMapping{{From: "high", To: "low"}, {From: "low", To: "medium"}}, want: "low"},
					{name: "explicit ceiling preserved", preference: `,"output_config":{"effort":"high"}`, ceiling: "medium", want: "medium"},
					{name: "terminal override not mapped twice", terminal: true, preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"high"}`, mappings: []ReasoningEffortMapping{{From: "none", To: "low", Model: "gpt-6.1-sol", MatchType: "exact"}}, want: "low"},
					{name: "terminal override raw rejected", terminal: true, preference: `,"thinking":{"type":"disabled"},"output_config":{"effort":"none"}`, mappings: noneToLow, status: 400},
				} {
					t.Run(fmt.Sprintf("chat=%t/%s/stream=%t/%s", chat, auth, stream, test.name), func(t *testing.T) {
						model := test.model
						if model == "" {
							model = "gpt-6.1-sol"
						}
						account := rawGPT56ResponsesAPIKeyAccount(model, "gpt-6.1-sol")
						if auth == AccountTypeOAuth {
							account = rawGPT56ResponsesOAuthAccount(model, "gpt-6.1-sol")
						}
						if chat {
							account.Extra = map[string]any{openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions)}
						}
						defaultMapped := ""
						if test.terminal {
							model = "public-alias"
							defaultMapped = "gpt-6.1-sol"
							account.Credentials["model_mapping"] = map[string]any{"gpt-6.1-sol": "gpt-6-sol"}
						}
						usesChat := chat && auth == AccountTypeAPIKey
						response := openAICompatSSECompletedResponse("resp_effort", "gpt-6.1-sol")
						if usesChat {
							payload := `{"id":"chat_effort","model":"gpt-6.1-sol","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`
							contentType := "application/json"
							if stream {
								payload = "data: " + `{"id":"chat_effort","model":"gpt-6.1-sol","choices":[{"index":0,"delta":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}` + "\n\ndata: [DONE]\n\n"
								contentType = "text/event-stream"
							}
							response = &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(payload))}
						}
						upstream := &httpUpstreamRecorder{resp: response}
						svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
						body := fmt.Sprintf(`{"model":%q,"max_tokens":1024,"messages":[{"role":"user","content":"Hello"}],"stream":%t%s}`, model, stream, test.preference)
						var forwardErr error
						var result *OpenAIForwardResult
						router := gin.New()
						router.POST("/v1/messages", func(c *gin.Context) {
							ctx := WithOpenAIReasoningEffortPolicyForMessages(c.Request.Context(), []byte(body), test.ceiling, test.mappings, "")
							result, forwardErr = svc.ForwardAsAnthropic(ctx, c, account, []byte(body), "", defaultMapped)
						})
						recorder := httptest.NewRecorder()
						router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body)))
						if test.status != 0 {
							require.Error(t, forwardErr)
							require.Equal(t, test.status, recorder.Code, recorder.Body.String())
							require.Empty(t, upstream.requests)
							return
						}
						preferredEffortPath := "reasoning.effort"
						endpoint := "/responses"
						if usesChat {
							preferredEffortPath = "reasoning_effort"
							endpoint = "/chat/completions"
						}
						require.NoError(t, forwardErr)
						require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
						require.Len(t, upstream.requests, 1)
						require.True(t, strings.HasSuffix(upstream.lastReq.URL.Path, endpoint), upstream.lastReq.URL.String())
						require.Equal(t, "gpt-6.1-sol", gjson.GetBytes(upstream.lastBody, "model").String())
						require.Equal(t, test.want, gjson.GetBytes(upstream.lastBody, preferredEffortPath).String())
						require.NotNil(t, result)
						require.NotNil(t, result.ReasoningEffort)
						require.Equal(t, test.want, *result.ReasoningEffort)
						require.Equal(t, stream, result.Stream)
						require.Equal(t, stream || !usesChat, gjson.GetBytes(upstream.lastBody, "stream").Bool())
						if stream {
							require.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
							require.Contains(t, recorder.Body.String(), "message_stop")
						} else {
							require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
							require.Equal(t, "ok", gjson.GetBytes(recorder.Body.Bytes(), "content.0.text").String())
						}
					})
				}
			}
		}
	}
}

func TestMessagesRawEffortValidation(t *testing.T) {
	for _, test := range []struct {
		body string
		deny bool
	}{
		{body: `{"output_config":{"effort":"high"},"thinking":{"type":"disabled"}}`},
		{body: `{"output_config":{"effort":"none"},"thinking":{"type":"disabled"}}`, deny: true},
		{body: `{"reasoning_effort":"none"}`, deny: true},
		{body: `{"reasoning":{"effort":"minimal"}}`, deny: true},
		{body: `{"model":"gpt-6.1-sol-none"}`, deny: true},
	} {
		require.Equal(t, test.deny, validateGPT61SolRawEffort([]byte(test.body), "gpt-6.1-sol") != nil, test.body)
		require.NoError(t, validateGPT61SolRawEffort([]byte(test.body), "gpt-6-sol"))
	}
}
