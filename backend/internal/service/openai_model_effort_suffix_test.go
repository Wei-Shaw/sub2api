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
	"github.com/tidwall/gjson"
)

func TestOpenAIModelEffortSuffixChatCompletionsModifiesOnlyUpstreamBody(t *testing.T) {
	body := []byte(`{"model":"my-alias-low","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	before := append([]byte(nil), body...)
	upstream := newModelEffortSuffixUpstream(`{"id":"chatcmpl_1","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	svc, account, c := newModelEffortSuffixForwardTest(t, upstream, "/v1/chat/completions", false)

	_, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")

	require.NoError(t, err)
	require.Equal(t, before, body, "handler/routing body must stay untouched")
	require.Equal(t, "my-alias", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "low", gjson.GetBytes(upstream.lastBody, "reasoning_effort").String())
}

func TestOpenAIModelEffortSuffixChatCompletionsResponsesUpstreamBody(t *testing.T) {
	body := []byte(`{"model":"my-alias-medium","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	before := append([]byte(nil), body...)
	upstream := newModelEffortSuffixUpstream(strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","model":"my-alias","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))
	upstream.resp.Header.Set("Content-Type", "text/event-stream")
	svc, account, c := newModelEffortSuffixForwardTest(t, upstream, "/v1/chat/completions", true)

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, before, body, "handler/routing body must stay untouched")
	require.Equal(t, "my-alias", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "medium", gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
	require.Equal(t, "my-alias-medium", result.Model)
	require.Equal(t, "my-alias-medium", result.BillingModel)
}

func TestOpenAIModelEffortSuffixResponsesModifiesOnlyUpstreamBody(t *testing.T) {
	body := []byte(`{"model":"my-alias-high","input":"hello","stream":false}`)
	before := append([]byte(nil), body...)
	upstream := newModelEffortSuffixUpstream(`{"id":"resp_1","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
	svc, account, c := newModelEffortSuffixForwardTest(t, upstream, "/v1/responses", true)

	_, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.Equal(t, before, body, "handler/routing body must stay untouched")
	require.Equal(t, "my-alias", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "high", gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
}

func TestOpenAIModelEffortSuffixExplicitEffortWins(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		body         []byte
		responsesAPI bool
		effortPath   string
	}{
		{
			name:       "Chat Completions",
			path:       "/v1/chat/completions",
			body:       []byte(`{"model":"my-alias-high","reasoning_effort":"low","messages":[],"stream":false}`),
			effortPath: "reasoning_effort",
		},
		{
			name:         "Responses",
			path:         "/v1/responses",
			body:         []byte(`{"model":"my-alias-high","reasoning":{"effort":"medium"},"input":"hello","stream":false}`),
			responsesAPI: true,
			effortPath:   "reasoning.effort",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := `{"id":"chatcmpl_1","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
			if tt.responsesAPI {
				response = `{"id":"resp_1","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
			}
			upstream := newModelEffortSuffixUpstream(response)
			svc, account, c := newModelEffortSuffixForwardTest(t, upstream, tt.path, tt.responsesAPI)

			var err error
			if tt.responsesAPI {
				_, err = svc.Forward(context.Background(), c, account, tt.body)
			} else {
				_, err = svc.forwardAsRawChatCompletions(context.Background(), c, account, tt.body, "")
			}

			require.NoError(t, err)
			require.Equal(t, "my-alias", gjson.GetBytes(upstream.lastBody, "model").String())
			require.Equal(t, map[bool]string{false: "low", true: "medium"}[tt.responsesAPI], gjson.GetBytes(upstream.lastBody, tt.effortPath).String())
		})
	}
}

func TestOpenAIModelEffortSuffixPreservesCatalogAndAccountAliases(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		mapping map[string]any
	}{
		{name: "Codex catalog alias", model: "gpt-5.1-codex-max"},
		{name: "Gemini catalog model", model: "gemini-3.6-flash-high"},
		{name: "account exact alias", model: "private-model-low", mapping: map[string]any{"private-model-low": "vendor-model-low"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"` + tt.model + `","messages":[],"stream":false}`)
			upstream := newModelEffortSuffixUpstream(`{"id":"chatcmpl_1","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
			svc, account, c := newModelEffortSuffixForwardTest(t, upstream, "/v1/chat/completions", false)
			if tt.mapping != nil {
				account.Credentials["model_mapping"] = tt.mapping
			}

			_, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")

			require.NoError(t, err)
			wantModel := tt.model
			if tt.mapping != nil {
				mappedModel, ok := tt.mapping[tt.model].(string)
				require.True(t, ok)
				wantModel = mappedModel
			}
			require.Equal(t, wantModel, gjson.GetBytes(upstream.lastBody, "model").String())
			require.False(t, gjson.GetBytes(upstream.lastBody, "reasoning_effort").Exists())
		})
	}
}

func newModelEffortSuffixUpstream(response string) *httpUpstreamRecorder {
	return &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(response)),
	}}
}

func newModelEffortSuffixForwardTest(t *testing.T, upstream *httpUpstreamRecorder, path string, responsesAPI bool) (*OpenAIGatewayService, *Account, *gin.Context) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	account := &Account{
		ID: 1, Name: "test", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://example.com"},
		Extra:       map[string]any{"use_responses_api": responsesAPI},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(nil))
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
	return svc, account, c
}
