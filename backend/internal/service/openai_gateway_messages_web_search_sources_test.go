//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const messagesWebSearchBody = `{"model":"gpt-5.5","max_tokens":64,"stream":true,
 "tools":[{"type":"web_search_20250305","name":"web_search"}],
 "messages":[{"role":"user","content":"search the web"}]}`

const messagesWithoutWebSearchBody = `{"model":"gpt-5.5","max_tokens":64,"stream":true,
 "messages":[{"role":"user","content":"hello"}]}`

const webSearchCompletedSSE = "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_ws\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":5,\"output_tokens\":2,\"total_tokens\":7}}}\n\n"

func codexOAuthWebSearchTestAccount() *Account {
	return &Account{
		ID: 1, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"},
	}
}

func upstreamInclude(body []byte) []string {
	var include []string
	for _, v := range gjson.GetBytes(body, "include").Array() {
		include = append(include, v.String())
	}
	return include
}

func forwardMessagesWebSearch(t *testing.T, account *Account, requestBody, upstreamSSE string) (*httpUpstreamRecorder, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(requestBody)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamSSE)),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          rawChatCompletionsTestConfig(),
	}
	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.5")
	require.NoError(t, err)
	return upstream, rec
}

func forwardGrokMessages(t *testing.T, requestBody string) *httpUpstreamRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := []byte(requestBody)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	account := healthyGrokOAuthGatewayTestAccount(71, "access-token")
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{accountsByID: map[int64]*Account{71: account}}}
	upstream := &httpUpstreamRecorder{resp: grokMessagesSSECompletedResponse("resp_grok_ws", 0)}
	svc := &OpenAIGatewayService{httpUpstream: upstream, grokTokenProvider: NewGrokTokenProvider(repo, nil), accountRepo: repo}

	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
	require.NoError(t, err)
	return upstream
}

func TestForwardAsAnthropic_WebSearchRequestsSources(t *testing.T) {
	for name, account := range map[string]*Account{
		"openai apikey": rawChatCompletionsTestAccount(),
		"codex oauth":   codexOAuthWebSearchTestAccount(),
	} {
		t.Run(name, func(t *testing.T) {
			upstream, _ := forwardMessagesWebSearch(t, account, messagesWebSearchBody, webSearchCompletedSSE)
			require.Equal(t, []string{"reasoning.encrypted_content", "web_search_call.action.sources"}, upstreamInclude(upstream.lastBody))
		})
	}
}

func TestForwardAsAnthropic_WebSearchSourcesNotRequestedWithoutTool(t *testing.T) {
	for name, account := range map[string]*Account{
		"openai apikey": rawChatCompletionsTestAccount(),
		"codex oauth":   codexOAuthWebSearchTestAccount(),
	} {
		t.Run(name, func(t *testing.T) {
			upstream, _ := forwardMessagesWebSearch(t, account, messagesWithoutWebSearchBody, webSearchCompletedSSE)
			require.NotContains(t, upstreamInclude(upstream.lastBody), "web_search_call.action.sources")
		})
	}
	t.Run("grok with web_search tool", func(t *testing.T) {
		upstream := forwardGrokMessages(t, messagesWebSearchBody)
		require.NotEmpty(t, gjson.GetBytes(upstream.lastBody, "tools").Array(), "the web_search tool reaches xAI")
		require.NotContains(t, upstreamInclude(upstream.lastBody), "web_search_call.action.sources")
	})
}

// A source entry of an unexpected shape must not cost the terminal event that
// carries it: that would turn a finished answer into a missing-terminal error.
func TestWebSearchMalformedSourcesKeepTerminalEvent(t *testing.T) {
	sse := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_ws\",\"model\":\"gpt-5.5\"}}\n\n" +
		"data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"found it\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_ws\",\"status\":\"completed\",\"output\":[" +
		"{\"type\":\"message\",\"id\":\"msg_1\",\"role\":\"assistant\",\"status\":\"completed\",\"content\":[{\"type\":\"output_text\",\"text\":\"found it\"}]}," +
		"{\"type\":\"web_search_call\",\"id\":\"ws_1\",\"status\":\"completed\",\"action\":{\"type\":\"search\",\"query\":\"q\",\"sources\":[\"https://a\"]}}]," +
		"\"usage\":{\"input_tokens\":5,\"output_tokens\":2,\"total_tokens\":7}}}\n\n"
	_, rec := forwardMessagesWebSearch(t, rawChatCompletionsTestAccount(), messagesWebSearchBody, sse)

	out := strings.TrimSpace(rec.Body.String())
	require.True(t, strings.HasSuffix(out, `{"type":"message_stop"}`), "body: %s", out)
}
