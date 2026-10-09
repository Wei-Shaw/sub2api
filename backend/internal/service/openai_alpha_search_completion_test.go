package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAlphaSearchFallbackRequiresSuccessfulCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	delta := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"unfinished answer\"}\n\n"
	cases := map[string]string{
		"empty":             "",
		"truncated":         delta,
		"done_only":         delta + "data: [DONE]\n\n",
		"failed":            delta + "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\"}}\n\n",
		"incomplete":        delta + "data: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\"}}\n\n",
		"error":             delta + "data: {\"type\":\"error\",\"error\":{\"message\":\"unavailable\"}}\n\n",
		"invalid_completed": delta + "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"failed\"}}\n\n",
		"missing_response":  delta + "data: {\"type\":\"response.completed\"}\n\n",
	}
	for name, wire := range cases {
		t.Run(name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.5","commands":{"search_query":[{"q":"example"}]}}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/alpha/search", bytes.NewReader(body))
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(wire)),
			}}
			service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := &Account{ID: 43, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
				Credentials: map[string]any{"access_token": "at-test-token", "auth_mode": OpenAIAuthModePersonalAccessToken, "chatgpt_account_id": "fixture-account"}}
			result, err := service.ForwardAlphaSearch(context.Background(), c, account, body)
			require.Error(t, err)
			require.Nil(t, result, "a failed search must not produce a billable WebSearchCalls result")
			require.False(t, c.Writer.Written(), "do not write partial content as a successful search")
		})
	}
}

func TestAlphaSearchFallbackCompletionPreservesOutputAndCitations(t *testing.T) {
	for _, wire := range []string{
		alphaSearchResponsesSSE("search result"),
		"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"search result\",\"annotations\":[{\"type\":\"url_citation\",\"url\":\"https://example.com/news\",\"title\":\"Example News\"}]}]}]}}\n\n",
	} {
		body, err := openAIAlphaSearchResponseFromResponsesSSE([]byte(wire))
		require.NoError(t, err)
		require.JSONEq(t, `{"output":"search result","results":[{"type":"text_result","ref_id":"turn0search0","url":"https://example.com/news","title":"Example News"}]}`, string(body))
	}
}
