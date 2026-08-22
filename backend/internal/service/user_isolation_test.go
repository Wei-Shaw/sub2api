package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func userIsolationTestContext(userID int64) context.Context {
	return context.WithValue(context.Background(), ctxkey.UserID, userID)
}

func userIsolationTestConfig() *config.Config {
	return &config.Config{JWT: config.JWTConfig{Secret: "01234567890123456789012345678901"}}
}

func TestDeriveManagedUserIsolationIDIsStableAndAccountScoped(t *testing.T) {
	account := &Account{ID: 11, Platform: PlatformOpenAI}
	first := deriveManagedUserIsolationID("secret", account, 42)

	require.Equal(t, first, deriveManagedUserIsolationID("secret", account, 42))
	require.NotEqual(t, first, deriveManagedUserIsolationID("secret", account, 43))
	require.NotEqual(t, first, deriveManagedUserIsolationID("secret", &Account{ID: 12, Platform: PlatformOpenAI}, 42))
	require.Len(t, first, 46)
	require.Regexp(t, `^u1_[A-Za-z0-9_-]+$`, first)
}

func TestApplyManagedUserIsolationUsesFinalProtocolField(t *testing.T) {
	tests := []struct {
		name     string
		account  *Account
		endpoint userIsolationEndpoint
		path     string
	}{
		{
			name:     "anthropic messages",
			account:  &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			endpoint: userIsolationEndpointAnthropicMessages,
			path:     "metadata.user_id",
		},
		{
			name:     "anthropic oauth messages",
			account:  &Account{ID: 8, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			endpoint: userIsolationEndpointAnthropicMessages,
			path:     "metadata.user_id",
		},
		{
			name:     "openai responses",
			account:  &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			endpoint: userIsolationEndpointResponses,
			path:     "safety_identifier",
		},
		{
			name:     "openai chat completions",
			account:  &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			endpoint: userIsolationEndpointChatCompletions,
			path:     "safety_identifier",
		},
		{
			name:     "openai oauth responses",
			account:  &Account{ID: 9, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			endpoint: userIsolationEndpointResponses,
			path:     "safety_identifier",
		},
		{
			name:     "grok chat completions",
			account:  &Account{ID: 10, Platform: PlatformGrok, Type: AccountTypeAPIKey, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			endpoint: userIsolationEndpointChatCompletions,
			path:     "user",
		},
		{
			name:     "grok oauth responses",
			account:  &Account{ID: 11, Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			endpoint: userIsolationEndpointResponses,
			path:     "user",
		},
		{
			name:     "kimi payg chat completions",
			account:  &Account{ID: 12, Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{"account_mode": AccountModePayG}, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			endpoint: userIsolationEndpointChatCompletions,
			path:     "safety_identifier",
		},
		{
			name:     "kimi coding anthropic",
			account:  &Account{ID: 13, Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{"account_mode": AccountModeCoding, "api_protocol": APIProtocolAnthropic}, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			endpoint: userIsolationEndpointAnthropicMessages,
			path:     "metadata.user_id",
		},
		{
			name:     "zhipu chat completions",
			account:  &Account{ID: 4, Platform: PlatformZhipu, Type: AccountTypeAPIKey, Credentials: map[string]any{"account_mode": AccountModePayG}, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			endpoint: userIsolationEndpointChatCompletions,
			path:     "user_id",
		},
		{
			name:     "zhipu coding anthropic",
			account:  &Account{ID: 14, Platform: PlatformZhipu, Type: AccountTypeAPIKey, Credentials: map[string]any{"account_mode": AccountModeCoding, "api_protocol": APIProtocolAnthropic}, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			endpoint: userIsolationEndpointAnthropicMessages,
			path:     "metadata.user_id",
		},
		{
			name:     "deepseek chat completions",
			account:  &Account{ID: 5, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			endpoint: userIsolationEndpointChatCompletions,
			path:     "user_id",
		},
		{
			name:     "deepseek anthropic",
			account:  &Account{ID: 6, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			endpoint: userIsolationEndpointAnthropicMessages,
			path:     "metadata.user_id",
		},
		{
			name:     "deepseek responses",
			account:  &Account{ID: 7, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			endpoint: userIsolationEndpointResponses,
			path:     "user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := applyManagedUserIsolation(userIsolationTestContext(42), userIsolationTestConfig(), tt.account, tt.endpoint, []byte(`{"model":"test","user":"spoofed","user_id":"spoofed","safety_identifier":"spoofed","metadata":{"user_id":"spoofed"}}`))
			require.NoError(t, err)
			want := deriveManagedUserIsolationID(userIsolationTestConfig().JWT.Secret, tt.account, 42)
			require.Equal(t, want, gjson.GetBytes(body, tt.path).String())
		})
	}
}

func TestApplyManagedUserIsolationDisabledIsNoop(t *testing.T) {
	body := []byte(`{"user_id":"client-value"}`)
	updated, err := applyManagedUserIsolation(context.Background(), nil, &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey}, userIsolationEndpointChatCompletions, body)
	require.NoError(t, err)
	require.Equal(t, body, updated)
}

func TestApplyManagedUserIsolationRejectsUnsupportedAndMissingIdentity(t *testing.T) {
	unsupported := &Account{ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey, Extra: map[string]any{UserIsolationEnabledExtraKey: true}}
	_, err := applyManagedUserIsolation(userIsolationTestContext(42), userIsolationTestConfig(), unsupported, userIsolationEndpointChatCompletions, []byte(`{}`))
	require.ErrorContains(t, err, "unsupported")

	supported := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{UserIsolationEnabledExtraKey: true}}
	_, err = applyManagedUserIsolation(context.Background(), userIsolationTestConfig(), supported, userIsolationEndpointResponses, []byte(`{}`))
	require.ErrorContains(t, err, "authenticated user ID is unavailable")
}

func TestApplyManagedUserIsolationRejectsAmbiguousIdentityKeys(t *testing.T) {
	tests := []struct {
		name     string
		account  *Account
		endpoint userIsolationEndpoint
		body     string
		wantErr  string
	}{
		{
			name:     "duplicate chat user id",
			account:  &Account{ID: 1, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			endpoint: userIsolationEndpointChatCompletions,
			body:     `{"user_id":"first","user_id":"last"}`,
			wantErr:  "duplicate user isolation key",
		},
		{
			name:     "duplicate responses user",
			account:  &Account{ID: 2, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			endpoint: userIsolationEndpointResponses,
			body:     `{"user":"first","user":"last"}`,
			wantErr:  "duplicate user isolation key",
		},
		{
			name:     "duplicate metadata",
			account:  &Account{ID: 3, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			endpoint: userIsolationEndpointAnthropicMessages,
			body:     `{"metadata":{"user_id":"first"},"metadata":{"user_id":"last"}}`,
			wantErr:  "duplicate user isolation key",
		},
		{
			name:     "duplicate metadata user id",
			account:  &Account{ID: 4, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			endpoint: userIsolationEndpointAnthropicMessages,
			body:     `{"metadata":{"user_id":"first","user_id":"last"}}`,
			wantErr:  "duplicate user isolation key",
		},
		{
			name:     "duplicate safety identifier",
			account:  &Account{ID: 5, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			endpoint: userIsolationEndpointResponses,
			body:     `{"safety_identifier":"first","safety_identifier":"last"}`,
			wantErr:  "duplicate user isolation key",
		},
		{
			name:     "non canonical safety identifier",
			account:  &Account{ID: 6, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			endpoint: userIsolationEndpointResponses,
			body:     `{"Safety_Identifier":"client-value"}`,
			wantErr:  "non-canonical user isolation key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := applyManagedUserIsolation(userIsolationTestContext(42), userIsolationTestConfig(), tt.account, tt.endpoint, []byte(tt.body))
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestOpenAICompactBuildersSkipUserIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{
		ID:       17,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{UserIsolationEnabledExtraKey: true},
	}
	svc := &OpenAIGatewayService{cfg: userIsolationTestConfig()}
	body := []byte(`{"model":"gpt-5.6-sol","input":"compact me"}`)

	tests := []struct {
		name  string
		build func(context.Context, *gin.Context) (*http.Request, error)
	}{
		{
			name: "ordinary",
			build: func(ctx context.Context, c *gin.Context) (*http.Request, error) {
				return svc.buildUpstreamRequest(ctx, c, account, body, "token", false, "", false)
			},
		},
		{
			name: "passthrough",
			build: func(ctx context.Context, c *gin.Context) (*http.Request, error) {
				return svc.buildUpstreamRequestOpenAIPassthrough(ctx, c, account, body, "token")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest("POST", "/v1/responses/compact", nil)

			req, err := tt.build(userIsolationTestContext(42), c)
			require.NoError(t, err)
			wireBody, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.False(t, gjson.GetBytes(wireBody, "safety_identifier").Exists())
		})
	}
}

func TestValidateUserIsolationAccount(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		wantErr bool
	}{
		{name: "disabled unsupported account", account: &Account{Platform: PlatformKimi, Type: AccountTypeAPIKey}},
		{name: "openai api key", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{UserIsolationEnabledExtraKey: true}}},
		{name: "openai oauth", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{UserIsolationEnabledExtraKey: true}}},
		{name: "openai setup token", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeSetupToken, Extra: map[string]any{UserIsolationEnabledExtraKey: true}}},
		{name: "anthropic oauth", account: &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth, Extra: map[string]any{UserIsolationEnabledExtraKey: true}}},
		{name: "anthropic setup token", account: &Account{Platform: PlatformAnthropic, Type: AccountTypeSetupToken, Extra: map[string]any{UserIsolationEnabledExtraKey: true}}},
		{name: "grok api key", account: &Account{Platform: PlatformGrok, Type: AccountTypeAPIKey, Extra: map[string]any{UserIsolationEnabledExtraKey: true}}},
		{name: "grok oauth", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{UserIsolationEnabledExtraKey: true}}},
		{name: "kimi payg chat", account: &Account{Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{"account_mode": AccountModePayG, "api_protocol": APIProtocolChatCompletions}, Extra: map[string]any{UserIsolationEnabledExtraKey: true}}},
		{name: "kimi coding adaptive", account: &Account{Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{"account_mode": AccountModeCoding, "api_protocol": APIProtocolAdaptive}, Extra: map[string]any{UserIsolationEnabledExtraKey: true}}},
		{name: "deepseek adaptive", account: &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_protocol": APIProtocolAdaptive}, Extra: map[string]any{UserIsolationEnabledExtraKey: true}}},
		{name: "zhipu coding", account: &Account{Platform: PlatformZhipu, Type: AccountTypeAPIKey, Credentials: map[string]any{"account_mode": AccountModeCoding}, Extra: map[string]any{UserIsolationEnabledExtraKey: true}}},
		{name: "zhipu anthropic", account: &Account{Platform: PlatformZhipu, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_protocol": APIProtocolAnthropic}, Extra: map[string]any{UserIsolationEnabledExtraKey: true}}},
		{name: "gemini api key", account: &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Extra: map[string]any{UserIsolationEnabledExtraKey: true}}, wantErr: true},
		{name: "antigravity oauth", account: &Account{Platform: PlatformAntigravity, Type: AccountTypeOAuth, Extra: map[string]any{UserIsolationEnabledExtraKey: true}}, wantErr: true},
		{name: "anthropic bedrock", account: &Account{Platform: PlatformAnthropic, Type: AccountTypeBedrock, Extra: map[string]any{UserIsolationEnabledExtraKey: true}}, wantErr: true},
		{name: "anthropic service account", account: &Account{Platform: PlatformAnthropic, Type: AccountTypeServiceAccount, Extra: map[string]any{UserIsolationEnabledExtraKey: true}}, wantErr: true},
		{name: "deepseek coding", account: &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Credentials: map[string]any{"account_mode": AccountModeCoding}, Extra: map[string]any{UserIsolationEnabledExtraKey: true}}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUserIsolationAccount(tt.account)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
func TestValidateUserIsolationAccountUpdateAllowsExperimentalProtocol(t *testing.T) {
	account := &Account{
		ID:       1,
		Platform: PlatformZhipu,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"account_mode": AccountModePayG,
			"api_protocol": APIProtocolChatCompletions,
		},
		Extra: map[string]any{UserIsolationEnabledExtraKey: true},
	}

	err := validateUserIsolationAccountUpdate(account, map[string]any{"api_protocol": APIProtocolAnthropic}, nil)
	require.NoError(t, err)
	require.Equal(t, APIProtocolChatCompletions, account.GetAPIProtocol())
}

func TestBuildDeepSeekResponsesRequestAppliesUserIsolationAfterNormalization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)

	account := &Account{
		ID:       19,
		Platform: PlatformDeepseek,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_protocol": APIProtocolResponses,
		},
		Extra: map[string]any{UserIsolationEnabledExtraKey: true},
	}
	svc := &OpenAIGatewayService{cfg: userIsolationTestConfig()}
	ctx := userIsolationTestContext(42)
	req, err := svc.buildUpstreamRequest(
		ctx,
		c,
		account,
		[]byte(`{"model":"deepseek-chat","user":"spoofed","store":true,"previous_response_id":"resp_1"}`),
		"token",
		false,
		"",
		false,
	)
	require.NoError(t, err)
	wireBody, err := io.ReadAll(req.Body)
	require.NoError(t, err)

	require.Equal(t, deriveManagedUserIsolationID(userIsolationTestConfig().JWT.Secret, account, 42), gjson.GetBytes(wireBody, "user").String())
	require.False(t, gjson.GetBytes(wireBody, "store").Bool())
	require.False(t, gjson.GetBytes(wireBody, "previous_response_id").Exists())
}

func TestBuildSubscriptionResponsesRequestsApplyUserIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name     string
		account  *Account
		buildReq func(context.Context, *gin.Context, *Account) (*http.Request, error)
		path     string
	}{
		{
			name:    "openai oauth",
			account: &Account{ID: 20, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			buildReq: func(ctx context.Context, c *gin.Context, account *Account) (*http.Request, error) {
				svc := &OpenAIGatewayService{cfg: userIsolationTestConfig()}
				return svc.buildUpstreamRequest(ctx, c, account, []byte(`{"model":"gpt-5.6-sol","safety_identifier":"spoofed"}`), "token", false, "", false)
			},
			path: "safety_identifier",
		},
		{
			name:    "grok oauth",
			account: &Account{ID: 21, Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{UserIsolationEnabledExtraKey: true}},
			buildReq: func(ctx context.Context, c *gin.Context, account *Account) (*http.Request, error) {
				return buildGrokResponsesRequest(ctx, c, account, []byte(`{"model":"grok-4.3","user":"spoofed"}`), "token", "", userIsolationTestConfig())
			},
			path: "user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

			req, err := tt.buildReq(userIsolationTestContext(42), c, tt.account)
			require.NoError(t, err)
			wireBody, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Equal(t, deriveManagedUserIsolationID(userIsolationTestConfig().JWT.Secret, tt.account, 42), gjson.GetBytes(wireBody, tt.path).String())
		})
	}
}

func TestBuildGrokCompactRequestSkipsUserIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
	account := &Account{ID: 22, Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{UserIsolationEnabledExtraKey: true}}

	req, err := buildGrokResponsesRequest(
		userIsolationTestContext(42),
		c,
		account,
		[]byte(`{"model":"grok-4.3","input":"compact me"}`),
		"token",
		"",
		userIsolationTestConfig(),
	)
	require.NoError(t, err)
	wireBody, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(wireBody, "user").Exists())
}

func TestBuildAnthropicRequestAppliesUserIsolationAtWireBoundary(t *testing.T) {
	for _, accountType := range []string{AccountTypeAPIKey, AccountTypeOAuth, AccountTypeSetupToken} {
		t.Run(accountType, func(t *testing.T) {
			account := &Account{
				ID:       23,
				Platform: PlatformAnthropic,
				Type:     accountType,
				Extra:    map[string]any{UserIsolationEnabledExtraKey: true},
			}
			tokenType := "apikey"
			if accountType != AccountTypeAPIKey {
				tokenType = "oauth"
			}
			svc := &GatewayService{cfg: userIsolationTestConfig()}
			req, wireBody, err := svc.buildUpstreamRequest(
				userIsolationTestContext(42),
				nil,
				account,
				[]byte(`{"model":"claude-sonnet-4-6","metadata":{"user_id":"spoofed"}}`),
				"token",
				tokenType,
				"claude-sonnet-4-6",
				false,
				false,
			)
			require.NoError(t, err)
			require.NotNil(t, req)
			require.Equal(t, deriveManagedUserIsolationID(userIsolationTestConfig().JWT.Secret, account, 42), gjson.GetBytes(wireBody, "metadata.user_id").String())
		})
	}
}
