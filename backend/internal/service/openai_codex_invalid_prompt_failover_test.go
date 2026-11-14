package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

const openAICodexInvalidPromptBody = `{"error":{"code":"invalid_prompt","type":"invalid_request_error","message":"Invalid prompt: your prompt was flagged as potentially violating our usage policy. Please try again with a different prompt: https://platform.openai.com/docs/guides/reasoning#advice-on-prompting"}}`

func openAICodexInvalidPromptTestAccount() *Account {
	return &Account{ID: 42, Name: "codex-account", Platform: PlatformOpenAI, Type: AccountTypeOAuth}
}

func TestOpenAICodexInvalidPromptFailoverScope(t *testing.T) {
	messageOnlyBody := `{"error":{"type":"invalid_request_error","message":"Invalid prompt: your prompt was flagged as potentially violating our usage policy. Please try again with a different prompt."}}`
	tests := []struct {
		name       string
		account    *Account
		statusCode int
		body       string
		want       bool
	}{
		{
			name:       "codex oauth structured 400",
			account:    openAICodexInvalidPromptTestAccount(),
			statusCode: http.StatusBadRequest,
			body:       openAICodexInvalidPromptBody,
			want:       true,
		},
		{
			name:       "codex oauth structured 404",
			account:    openAICodexInvalidPromptTestAccount(),
			statusCode: http.StatusNotFound,
			body:       openAICodexInvalidPromptBody,
			want:       true,
		},
		{
			name:       "codex oauth message-only payload",
			account:    openAICodexInvalidPromptTestAccount(),
			statusCode: http.StatusBadRequest,
			body:       messageOnlyBody,
			want:       true,
		},
		{
			name:       "api key policy rejection stays terminal",
			account:    &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			statusCode: http.StatusBadRequest,
			body:       openAICodexInvalidPromptBody,
			want:       false,
		},
		{
			name:       "non-openai oauth stays terminal",
			account:    &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth},
			statusCode: http.StatusBadRequest,
			body:       openAICodexInvalidPromptBody,
			want:       false,
		},
		{
			name:       "missing account stays terminal",
			account:    nil,
			statusCode: http.StatusBadRequest,
			body:       openAICodexInvalidPromptBody,
			want:       false,
		},
		{
			name:       "invalid_prompt code without policy text",
			account:    openAICodexInvalidPromptTestAccount(),
			statusCode: http.StatusBadRequest,
			body:       `{"error":{"code":"invalid_prompt","message":"Invalid value for temperature"}}`,
			want:       false,
		},
		{
			name:       "generic policy rejection without invalid prompt marker",
			account:    openAICodexInvalidPromptTestAccount(),
			statusCode: http.StatusBadRequest,
			body:       `{"error":{"code":"invalid_request_error","message":"Your prompt was flagged as potentially violating our usage policy."}}`,
			want:       false,
		},
		{
			name:       "echoed phrase is ignored",
			account:    openAICodexInvalidPromptTestAccount(),
			statusCode: http.StatusBadRequest,
			body:       `{"error":{"code":"invalid_request_error","message":"Invalid request"},"echo":"prompt was flagged as potentially violating our usage policy"}`,
			want:       false,
		},
		{
			name:       "non policy status is not a failover signal",
			account:    openAICodexInvalidPromptTestAccount(),
			statusCode: http.StatusUnprocessableEntity,
			body:       openAICodexInvalidPromptBody,
			want:       false,
		},
	}

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, svc.shouldFailoverOpenAIUpstreamResponse(tt.account, tt.statusCode, "", []byte(tt.body)))
		})
	}
}

func TestShouldFailoverOpenAIUpstreamResponse_CodexInvalidPrompt(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := openAICodexInvalidPromptTestAccount()

	require.True(t, svc.shouldFailoverOpenAIUpstreamResponse(account, http.StatusBadRequest, "", []byte(openAICodexInvalidPromptBody)))
	require.True(t, svc.shouldFailoverOpenAIUpstreamResponse(account, http.StatusNotFound, "", []byte(openAICodexInvalidPromptBody)))
	require.False(t, svc.shouldFailoverOpenAIUpstreamResponse(
		account,
		http.StatusBadRequest,
		"",
		[]byte(`{"error":{"code":"invalid_request_error","message":"Invalid value for temperature"}}`),
	))
	require.False(t, svc.shouldFailoverOpenAIUpstreamResponse(
		account,
		http.StatusNotFound,
		"",
		[]byte(`{"error":{"message":"Unknown request URL"}}`),
	))
}

func TestFailoverOpenAIUpstreamHTTPError_CodexInvalidPrompt(t *testing.T) {
	c, _ := newOpenAIUpstreamErrorTestContext(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := openAICodexInvalidPromptTestAccount()
	body := []byte(openAICodexInvalidPromptBody)
	resp := newOpenAIUpstreamErrorResponse(http.StatusBadRequest, openAICodexInvalidPromptBody)

	failoverErr := svc.failoverOpenAIUpstreamHTTPError(
		context.Background(), c, account, resp, body, "Invalid prompt", "gpt-6-sol",
	)

	require.NotNil(t, failoverErr)
	require.Equal(t, http.StatusBadRequest, failoverErr.StatusCode)
	require.Equal(t, body, failoverErr.ResponseBody)
	require.True(t, failoverErr.ShouldRetryNextAccount())
	require.False(t, failoverErr.RetryableOnSameAccount, "the same account is expected to reproduce the false positive")
	require.False(t, IsResponseCommitted(c))
}

func TestOpenAICodexInvalidPromptDoesNotTouchAccountHealth(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := openAICodexInvalidPromptTestAccount()

	require.False(t, svc.handleOpenAIAccountUpstreamError(
		context.Background(), account, http.StatusBadRequest, nil, []byte(openAICodexInvalidPromptBody),
	))
	require.False(t, svc.handleOpenAIAccountUpstreamError(
		context.Background(), account, http.StatusNotFound, nil, []byte(openAICodexInvalidPromptBody),
	))
}

func TestOpenAICodexGenericPolicyErrorRemainsTerminal(t *testing.T) {
	c, rec := newOpenAIUpstreamErrorTestContext(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	body := `{"error":{"code":"invalid_request_error","message":"Your prompt was flagged as potentially violating our usage policy."}}`

	_, err := svc.handleErrorResponse(
		context.Background(),
		newOpenAIUpstreamErrorResponse(http.StatusBadRequest, body),
		c, openAICodexInvalidPromptTestAccount(), nil,
	)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "generic usage-policy rejections must not rotate accounts")
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
