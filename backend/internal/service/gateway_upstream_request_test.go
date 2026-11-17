//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Scenario: OAuth / setup-token 凭据只能走签发方协议，API Key 与其他凭据类型不受限
func TestAnthropicMessagesCredentialAllowed(t *testing.T) {
	cases := []struct {
		platform string
		typ      string
		allowed  bool
	}{
		{PlatformAnthropic, AccountTypeOAuth, true},
		{PlatformAnthropic, AccountTypeSetupToken, true},
		{PlatformAnthropic, AccountTypeServiceAccount, true},
		{PlatformAnthropic, AccountTypeBedrock, true},
		{PlatformTypeSafe, AccountTypeOAuth, true},
		{PlatformOpenAI, AccountTypeAPIKey, true},
		{PlatformDeepseek, AccountTypeAPIKey, true},
		{PlatformOpenAI, AccountTypeOAuth, false},
		{PlatformOpenAI, AccountTypeSetupToken, false},
		{PlatformGrok, AccountTypeOAuth, false},
		{PlatformGemini, AccountTypeOAuth, false},
		{PlatformAntigravity, AccountTypeOAuth, false},
	}
	for _, tc := range cases {
		allowed := anthropicMessagesCredentialAllowed(&Account{ID: 1, Platform: tc.platform, Type: tc.typ})
		require.Equal(t, tc.allowed, allowed, "%s/%s", tc.platform, tc.typ)
	}
}

// Scenario: 回归 —— OpenAI OAuth 账号误入 Anthropic 管线时，在构建请求前即被拒绝
// （ChatGPT token 不会发往 api.anthropic.com），且只让 handler 换号、不处罚账号
func TestGatewayBuildUpstreamRequestRejectsForeignOAuthWithoutPenalty(t *testing.T) {
	account := &Account{ID: 25, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	body := []byte(`{"model":"gpt-6-astra","messages":[]}`)

	req, _, err := (&GatewayService{}).buildUpstreamRequest(context.Background(), nil, account, body, "chatgpt-token", "oauth", "gpt-6-astra", true, false)
	require.Nil(t, req)

	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.True(t, failoverErr.ShouldRetryNextAccount())
	require.False(t, failoverErr.RetryableOnSameAccount, "same-account retry would end in temp-unschedulable")
	require.False(t, failoverErr.IsCredentialFailure())
	require.Equal(t, anthropicMessagesForeignOAuthReason, failoverErr.Reason)
}

// Scenario: count_tokens 对不匹配凭据按「平台不支持」返回 404，不取 token、不出站
func TestForwardCountTokensRejectsForeignOAuthAsUnsupported(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)

	account := &Account{ID: 25, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	parsed := &ParsedRequest{Model: "gpt-6-astra", Body: NewRequestBodyRef([]byte(`{"model":"gpt-6-astra","messages":[]}`))}

	err := (&GatewayService{}).ForwardCountTokens(context.Background(), c, account, parsed)

	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, w.Code)
}
