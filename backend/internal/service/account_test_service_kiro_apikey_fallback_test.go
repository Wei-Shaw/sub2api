//go:build unit

package service

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// Kiro API Key 账号现已直连 AWS(q.{region}.amazonaws.com),与 OAuth 账号同路径,
// 不再走 Anthropic 兼容反代(base_url + /v1/messages)。
func TestAccountTestService_KiroAPIKeyDirectAWSEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	account := &Account{
		ID:          19,
		Name:        "kiro-apikey-test",
		Platform:    PlatformKiro,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "kiro-api-key",
			"model_mapping": map[string]any{
				"claude-sonnet-4-6": "claude-sonnet-4-6",
			},
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			newJSONResponse(http.StatusUnauthorized, `{"message":"invalid token"}`),
		},
	}
	svc := &AccountTestService{
		accountRepo:         repo,
		httpUpstream:        upstream,
		cfg:                 &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		tlsFPProfileService: &TLSFingerprintProfileService{},
	}

	err := svc.TestAccountConnection(ctx, account.ID, "claude-sonnet-4-6", "", AccountTestModeDefault)
	require.Error(t, err)
	require.Len(t, upstream.requests, 1)

	req := upstream.requests[0]
	// 直连 AWS Q endpoint,默认 us-east-1
	require.Equal(t, "q.us-east-1.amazonaws.com", req.URL.Host)
	require.Equal(t, "/generateAssistantResponse", req.URL.Path)
	// API Key 走 Bearer + 小写 tokentype: API_KEY(对齐 kiro.rs)
	require.Equal(t, "Bearer kiro-api-key", req.Header.Get("Authorization"))
	require.Equal(t, []string{"API_KEY"}, req.Header["tokentype"])
	require.Empty(t, req.Header.Get("x-api-key"))
}

// 缺少 base_url 不再报错:Kiro API Key 账号直连 AWS,base_url 与其无关。
func TestAccountTestService_KiroAPIKeyWithoutBaseURLDirectAWS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	account := &Account{
		ID:          20,
		Name:        "kiro-apikey-no-base-url",
		Platform:    PlatformKiro,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "kiro-api-key",
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			newJSONResponse(http.StatusUnauthorized, `{"message":"invalid token"}`),
		},
	}
	svc := &AccountTestService{
		accountRepo:         repo,
		httpUpstream:        upstream,
		cfg:                 &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		tlsFPProfileService: &TLSFingerprintProfileService{},
	}

	err := svc.TestAccountConnection(ctx, account.ID, "claude-sonnet-4-6", "", AccountTestModeDefault)
	// 不再因缺少 base_url 报错;直连 AWS 后因 mock 返回 401 而报上游错误
	require.Error(t, err)
	require.NotContains(t, err.Error(), "Base URL")
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "q.us-east-1.amazonaws.com", upstream.requests[0].URL.Host)
}

// Kiro API Key + base_url(非空)= 外部 Anthropic 兼容中转账号:
// 不直连 AWS,而是转发到 {base_url}/v1/messages,走 x-api-key(通用反代路径)。
func TestAccountTestService_KiroAPIKeyRelayUsesBaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	account := &Account{
		ID:          21,
		Name:        "kiro-apikey-relay",
		Platform:    PlatformKiro,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"base_url": "https://relay-upstream.example.com",
			"api_key":  "relay-api-key",
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			newJSONResponse(http.StatusUnauthorized, `{"type":"error","error":{"type":"authentication_error","message":"invalid api key"}}`),
		},
	}
	svc := &AccountTestService{
		accountRepo:         repo,
		httpUpstream:        upstream,
		cfg:                 &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		tlsFPProfileService: &TLSFingerprintProfileService{},
	}

	err := svc.TestAccountConnection(ctx, account.ID, "claude-sonnet-4-6", "", AccountTestModeDefault)
	require.Error(t, err)
	require.Len(t, upstream.requests, 1)

	req := upstream.requests[0]
	// 转发到外部上游,而非 AWS
	require.Equal(t, "relay-upstream.example.com", req.URL.Host)
	require.Equal(t, "/v1/messages", req.URL.Path)
	// 中转走 x-api-key,不带 AWS 的 Bearer/tokentype
	require.Equal(t, "relay-api-key", req.Header.Get("x-api-key"))
	require.Empty(t, req.Header.Get("Authorization"))
	require.Empty(t, req.Header["tokentype"])
}

func TestIsKiroDirectModeAccount_RelayVsDirect(t *testing.T) {
	// 直连 AWS:Kiro + APIKey + 空 base_url
	require.True(t, isKiroDirectModeAccount(&Account{
		Platform: PlatformKiro, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "ksk_x"},
	}))
	// 外部中转:Kiro + APIKey + 非空 base_url → 不算直连
	require.False(t, isKiroDirectModeAccount(&Account{
		Platform: PlatformKiro, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "x", "base_url": "https://relay.example.com"},
	}))
	// OAuth 始终直连
	require.True(t, isKiroDirectModeAccount(&Account{Platform: PlatformKiro, Type: AccountTypeOAuth}))
	// 非 Kiro 平台不算
	require.False(t, isKiroDirectModeAccount(&Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey}))
}
