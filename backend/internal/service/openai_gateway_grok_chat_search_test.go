//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGrokChatNativeSearchEligibility(t *testing.T) {
	for _, tools := range []string{
		`[{"type":"web_search"}]`,
		`[{"type":"x_search"}]`,
		`[{"type": "web_search"}]`,
		`[{"type":"web_search"},{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]`,
	} {
		body := []byte(`{"model":"grok-4.6","messages":[{"role":"user","content":"news"}],"tool_choice":"required","tools":` + tools + `}`)
		ok, reason := grokChatResponsesBridgeEligibility(body)
		require.True(t, ok, reason)
	}
	// Only bare hosted declarations are added to this bridge. Keep the existing
	// boundary for filters, malformed declarations, and other hosted tool types.
	for _, tools := range []string{
		`[{"type":"web_search","allowed_domains":["example.com"]}]`,
		`[{"type":"x_search","allowed_x_handles":["example"]}]`,
		`[{"type":"web_search","function":{"name":"lookup","parameters":{"type":"object"}}}]`,
		`[{"type":"code_execution"}]`,
	} {
		ok, _ := grokChatResponsesBridgeEligibility([]byte(`{"model":"grok-4.6","messages":[{"role":"user","content":"news"}],"tools":` + tools + `}`))
		require.False(t, ok)
	}
}

func TestForwardGrokChatNativeSearchKeepsRawBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, apikey := range []bool{false, true} {
		t.Run(fmt.Sprintf("apiKey=%t", apikey), func(t *testing.T) {
			body := []byte(`{"model":"grok-4.3","messages":[{"role":"user","content":"news"}],"tools":[{"type":"web_search"}]}`)
			account := grokChatBridgeTestAccount(8316)
			if apikey {
				body = bytes.ReplaceAll(body, []byte("grok-4.3"), []byte("grok-4.6"))
				account.Type = AccountTypeAPIKey
				account.Credentials["api_key"] = "test-key"
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, grokChatRawEndpoint, bytes.NewReader(body))
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":"chat1","object":"chat.completion","model":"grok-4.3","choices":[{"index":0,"message":{"role":"assistant","content":"raw"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`)),
			}}
			repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{accountsByID: map[int64]*Account{account.ID: account}}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream, accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, nil)}
			result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, xai.DefaultCLIBaseURL+"/chat/completions", upstream.lastReq.URL.String())
			require.Equal(t, "web_search", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
			require.Equal(t, "raw", gjson.Get(recorder.Body.String(), "choices.0.message.content").String())
		})
	}
}

func TestForwardGrokChatNativeSearch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		for _, cached := range []bool{false, true} {
			for _, choice := range []string{"", "auto", "required", "none"} {
				t.Run(fmt.Sprintf("stream=%t/cache=%t/choice=%s", stream, cached, choice), func(t *testing.T) {
					toolFields := `"tools":[{"type":"web_search"}]`
					if choice != "" {
						toolFields = fmt.Sprintf(`"tool_choice":%q,"tools":[{"type":"web_search"},{"type":"x_search"},{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]`, choice)
					}
					body := []byte(fmt.Sprintf(`{"model":"grok","messages":[{"role":"user","content":"news"}],"stream":%t,%s}`, stream, toolFields))
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, grokChatRawEndpoint, bytes.NewReader(body))
					if cached {
						c.Set("api_key", &APIKey{ID: 8315})
					}
					account := grokChatBridgeTestAccount(8315)
					repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
						accountsByID: map[int64]*Account{account.ID: account},
					}}
					searchOutput := `{"type":"web_search_call","id":"ws1","status":"completed"},{"type":"x_search_call","id":"xs1","status":"completed"},`
					searchEvent := `data: {"type":"response.output_item.done","item":{"type":"web_search_call","id":"ws1","status":"completed"}}` + "\n\n"
					wantSearchCount := 2
					if choice == "" {
						searchOutput = `{"type":"web_search_call","id":"ws1","status":"completed"},`
						wantSearchCount = 1
					}
					if choice == "none" {
						searchOutput, searchEvent, wantSearchCount = "", "", 0
					}
					response := searchEvent + `data: {"type":"response.output_text.delta","delta":"news answer"}` + "\n\n" +
						`data: {"type":"response.completed","response":{"id":"resp_search","object":"response","model":"grok-4.6","status":"completed","output":[` + searchOutput +
						`{"type":"message","id":"msg1","role":"assistant","content":[{"type":"output_text","text":"news answer"}]}],"usage":{"input_tokens":20,"output_tokens":5,"total_tokens":25}}}` + "\n\n"
					upstream := &httpUpstreamRecorder{resp: &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": {"text/event-stream"}},
						Body:       io.NopCloser(strings.NewReader(response)),
					}}
					svc := &OpenAIGatewayService{httpUpstream: upstream, accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, nil)}
					result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
					require.NoError(t, err)
					require.NotNil(t, result)
					require.Equal(t, xai.DefaultCLIBaseURL+"/responses", upstream.lastReq.URL.String())
					require.Equal(t, grokChatResponsesEndpoint, result.UpstreamEndpoint)
					require.Equal(t, "grok-4.6", result.UpstreamModel)
					require.Equal(t, "grok", result.Model)
					require.Equal(t, wantSearchCount, result.SearchCount)
					require.Equal(t, 20, result.Usage.InputTokens)
					require.Equal(t, 5, result.Usage.OutputTokens)
					require.Equal(t, choice, gjson.GetBytes(upstream.lastBody, "tool_choice").String())
					require.Equal(t, "web_search", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
					if choice != "" {
						require.Equal(t, "x_search", gjson.GetBytes(upstream.lastBody, "tools.1.type").String())
						require.Equal(t, "lookup", gjson.GetBytes(upstream.lastBody, "tools.2.name").String())
					}
					require.Equal(t, cached, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").Exists())
					require.Equal(t, http.StatusOK, recorder.Code)
					if stream {
						require.Contains(t, recorder.Body.String(), `"content":"news answer"`)
						require.Contains(t, recorder.Body.String(), "data: [DONE]")
					} else {
						require.Equal(t, "news answer", gjson.Get(recorder.Body.String(), "choices.0.message.content").String())
					}
				})
			}
		}
	}
}
