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

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type alphaSearchAccountRepo struct {
	AccountRepository
	account *Account
}

func (r *alphaSearchAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	if r.account == nil || r.account.ID != id {
		return nil, fmt.Errorf("account %d not found", id)
	}
	return r.account, nil
}

func TestForwardAlphaSearchOAuthPreservesWire(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"id":"search-session",
		"model":"gpt-5.6-sol",
		"reasoning":{"effort":"max","context":"all_turns"},
		"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"latest news"}]}],
		"commands":{"search_query":[{"q":"OpenAI news","recency":1}]},
		"settings":{"allowed_callers":["direct"],"external_web_access":true},
		"max_output_tokens":2000,
		"future_field":{"keep":true}
	}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/alpha/search?feature=standalone", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", codexCLIUserAgent)
	c.Request.Header.Set("Originator", "codex_cli_rs")
	c.Request.Header.Set("Version", "0.144.1")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"encrypted_output":"ciphertext","output":"search result"}`)),
	}}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID:          42,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-account",
		},
	}

	result, err := service.ForwardAlphaSearch(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.WebSearchCalls)
	require.Equal(t, "gpt-5.6-sol", result.Model)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"encrypted_output":"ciphertext","output":"search result"}`, recorder.Body.String())
	require.Equal(t, chatgptCodexAlphaSearchURL+"?feature=standalone", upstream.lastReq.URL.String())
	require.Equal(t, "chatgpt.com", upstream.lastReq.Host)
	require.Equal(t, "Bearer oauth-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "chatgpt-account", upstream.lastReq.Header.Get("chatgpt-account-id"))
	require.Equal(t, "application/json", upstream.lastReq.Header.Get("Accept"))
	require.Equal(t, "0.144.1", upstream.lastReq.Header.Get("Version"))
	require.Empty(t, upstream.lastReq.Header.Get("OpenAI-Beta"))
	require.JSONEq(t, string(body), string(upstream.lastBody))
}
func TestForwardAlphaSearchRejectsAgentIdentityWithoutUpstreamCall(t *testing.T) {
	gin.SetMode(gin.TestMode)
	parent := &Account{
		ID:       90,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"auth_mode": OpenAIAuthModeAgentIdentity,
		},
	}
	parentID := parent.ID
	tests := []struct {
		name    string
		account *Account
		repo    AccountRepository
	}{
		{name: "direct", account: parent},
		{
			name: "shadow",
			account: &Account{
				ID:              91,
				Platform:        PlatformOpenAI,
				Type:            AccountTypeOAuth,
				ParentAccountID: &parentID,
			},
			repo: &alphaSearchAccountRepo{account: parent},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.4"}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/alpha/search", bytes.NewReader(body))
			upstream := &httpUpstreamRecorder{}
			service := &OpenAIGatewayService{
				accountRepo:  tt.repo,
				cfg:          &config.Config{},
				httpUpstream: upstream,
			}

			result, err := service.ForwardAlphaSearch(context.Background(), c, tt.account, body)

			require.Error(t, err)
			require.Nil(t, result)
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Equal(t, "invalid_request_error", gjson.Get(recorder.Body.String(), "error.type").String())
			require.Contains(t, gjson.Get(recorder.Body.String(), "error.message").String(), "only supports Responses API")
			require.Nil(t, upstream.lastReq)
		})
	}
}

func TestForwardAlphaSearchAPIKeyMapsModelAndPassesThroughError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"id":"search-session","model":"gpt-5.6-sol","commands":{"search_query":[{"q":"news"}]}}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/alpha/search", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := `{"error":{"type":"invalid_request_error","message":"bad search"}}`
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID:       7,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://compat.example/v4",
			"model_mapping": map[string]any{
				"gpt-5.6-sol": "upstream-5.6",
			},
		},
	}

	result, err := service.ForwardAlphaSearch(context.Background(), c, account, body)

	require.NoError(t, err)
	// 上游错误透传不是一次成功的搜索：不返回 result、不产生按次计费。
	require.Nil(t, result)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.JSONEq(t, upstreamBody, recorder.Body.String())
	require.Equal(t, "https://compat.example/v4/alpha/search", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-test", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "upstream-5.6", gjson.GetBytes(upstream.lastBody, "model").String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "commands.search_query").IsArray())
}

func TestForwardAlphaSearchReturnsFailoverBeforeWriting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"id":"search-session","model":"gpt-5.6-sol","commands":{}}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/alpha/search", bytes.NewReader(body))

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"rate limited"}}`)),
	}}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID:       8,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "sk-test",
		},
	}

	result, err := service.ForwardAlphaSearch(context.Background(), c, account, body)

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.Equal(t, openAIPlatformAlphaSearchURL, upstream.lastReq.URL.String())
	require.False(t, c.Writer.Written())
	require.Empty(t, recorder.Body.String())
}
