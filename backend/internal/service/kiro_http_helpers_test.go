//go:build unit

package service

import (
	"context"
	"net/http"
	"strings"
	"testing"

	kiropkg "github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBuildKiroAccountKeyIgnoresAccessToken(t *testing.T) {
	accountA := &Account{
		ID: 99,
		Credentials: map[string]any{
			"access_token": "token-a",
		},
	}
	accountB := &Account{
		ID: 99,
		Credentials: map[string]any{
			"access_token": "token-b",
		},
	}

	require.Equal(t, buildKiroAccountKey(accountA), buildKiroAccountKey(accountB))
}

func TestBuildKiroMachineIDPrefersExplicitCredential(t *testing.T) {
	account := &Account{
		ID:       101,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"machineId":     "2582956e-cc88-4669-b546-07adbffcb894",
			"refresh_token": "refresh-token",
		},
	}

	require.Equal(t, "2582956ecc884669b54607adbffcb8942582956ecc884669b54607adbffcb894", buildKiroMachineID(account))
}

func TestBuildKiroMachineIDDerivesFromRefreshToken(t *testing.T) {
	account := &Account{
		ID:       102,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"refresh_token": "refresh-token",
		},
	}

	require.Equal(t, kiropkg.BuildMachineID("refresh-token", "", "account:102"), buildKiroMachineID(account))
}

func TestBuildKiroMachineIDDerivesFromAPIKeyAccount(t *testing.T) {
	account := &Account{
		ID:       103,
		Platform: PlatformKiro,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"kiroApiKey": "kiro-api-key",
		},
	}

	require.Equal(t, kiropkg.BuildMachineID("", "kiro-api-key", "account:103"), buildKiroMachineID(account))
}

func TestNewKiroJSONRequestAddsConditionalHeaders(t *testing.T) {
	account := &Account{
		Credentials: map[string]any{
			"auth_method": "external_idp",
			"provider":    "Internal",
			"profile_arn": "arn:aws:codewhisperer:us-east-1:123456789012:profile/HEADER",
		},
	}

	req, err := newKiroJSONRequest(
		context.Background(),
		"https://q.us-east-1.amazonaws.com/generateAssistantResponse",
		[]byte(`{"ok":true}`),
		"access-token",
		"account-key",
		buildKiroMachineID(account),
		"",
		account,
	)
	require.NoError(t, err)
	require.Equal(t, "EXTERNAL_IDP", req.Header.Get("TokenType"))
	require.Equal(t, "true", req.Header.Get("redirect-for-internal"))
	require.Equal(t, "arn:aws:codewhisperer:us-east-1:123456789012:profile/HEADER", req.Header.Get("x-amzn-kiro-profile-arn"))
	require.Equal(t, "vibe", req.Header.Get("x-amzn-kiro-agent-mode"))
	require.Equal(t, "true", req.Header.Get("x-amzn-codewhisperer-optout"))
	require.Contains(t, req.Header.Get("User-Agent"), "aws-sdk-js/1.0.34")
	require.Contains(t, req.Header.Get("User-Agent"), "md/nodejs#22.22.0")
	require.Contains(t, req.Header.Get("User-Agent"), buildKiroMachineID(account))
	require.Contains(t, req.Header.Get("X-Amz-User-Agent"), buildKiroMachineID(account))
	require.True(t, strings.Contains(req.Header.Get("User-Agent"), "api/codewhispererstreaming#1.0.34"))
	require.Empty(t, req.Header.Get("Anthropic-Beta"))
}

func TestApplyKiroConditionalHeadersAPIKeyTokenType(t *testing.T) {
	apiKeyAccount := &Account{
		Platform: PlatformKiro,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"kiroApiKey": "ksk_test_key",
		},
	}
	req, err := newKiroJSONRequest(
		context.Background(),
		"https://q.us-east-1.amazonaws.com/generateAssistantResponse",
		[]byte(`{"ok":true}`),
		"ksk_test_key",
		"account-key",
		buildKiroMachineID(apiKeyAccount),
		"",
		apiKeyAccount,
	)
	require.NoError(t, err)
	// API Key 账号必须带小写 tokentype: API_KEY(对齐 kiro.rs;直接读原始 map key,
	// 因为头以小写 key 存储,Header.Get 会按规范化的 "Tokentype" 查找而读不到)
	require.Equal(t, []string{"API_KEY"}, req.Header["TokenType"])
	require.Equal(t, "Bearer ksk_test_key", req.Header.Get("Authorization"))
	// 非 external_idp,不应带规范化的 EXTERNAL_IDP 头
	require.Empty(t, req.Header["Tokentype"])

	// OAuth 账号不应带 API_KEY tokentype
	oauthAccount := &Account{
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"refresh_token": "refresh-token",
		},
	}
	oauthReq, err := newKiroJSONRequest(
		context.Background(),
		"https://q.us-east-1.amazonaws.com/generateAssistantResponse",
		[]byte(`{"ok":true}`),
		"access-token",
		"account-key",
		buildKiroMachineID(oauthAccount),
		"",
		oauthAccount,
	)
	require.NoError(t, err)
	require.Empty(t, oauthReq.Header["TokenType"])
}

func TestIsKiroDirectModeAccount(t *testing.T) {
	require.True(t, isKiroDirectModeAccount(&Account{Platform: PlatformKiro, Type: AccountTypeOAuth}))
	require.True(t, isKiroDirectModeAccount(&Account{Platform: PlatformKiro, Type: AccountTypeAPIKey}))
	// 非 Kiro 平台的 API Key 账号不算直连模式
	require.False(t, isKiroDirectModeAccount(&Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey}))
	// 其他账号类型不算
	require.False(t, isKiroDirectModeAccount(&Account{Platform: PlatformKiro, Type: AccountTypeServiceAccount}))
	require.False(t, isKiroDirectModeAccount(nil))
}

func TestBuildKiroEndpointsAPIKeyDirectAWS(t *testing.T) {
	// 默认 region → us-east-1
	defaultAccount := &Account{
		Platform:    PlatformKiro,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"kiroApiKey": "ksk_x"},
	}
	endpoints := buildKiroEndpoints(defaultAccount, KiroEndpointModeQ)
	require.Len(t, endpoints, 1)
	require.Equal(t, "https://q.us-east-1.amazonaws.com/generateAssistantResponse", endpoints[0].URL)

	// 凭据 api_region 覆盖 → us-west-2
	regionAccount := &Account{
		Platform:    PlatformKiro,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"kiroApiKey": "ksk_x", "api_region": "us-west-2"},
	}
	regionEndpoints := buildKiroEndpoints(regionAccount, KiroEndpointModeQ)
	require.Equal(t, "https://q.us-west-2.amazonaws.com/generateAssistantResponse", regionEndpoints[0].URL)
}

func TestIsKiroInvalidModelIDBodyRecognizesKnownForms(t *testing.T) {
	tests := []string{
		`{"message":"Invalid model ID. Please select a different model to continue.","reason":"INVALID_MODEL_ID"}`,
		`{"message":"Invalid model. Please select a different model to continue."}`,
		`API Error: 400 {"error":{"message":"Invalid model. Please select a different model to continue.","type":"upstream_error"}}`,
	}

	for _, body := range tests {
		classification := classifyKiroHTTPError(http.StatusBadRequest, body)
		require.Equal(t, kiroErrorBadRequestInvalidModel, classification.Category, body)
	}
}

func TestBuildKiroPayloadForAccountPropagatesThinkingHeaders(t *testing.T) {
	account := &Account{
		ID:       7,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"profile_arn": "arn:aws:codewhisperer:us-east-1:123456789012:profile/test",
		},
	}
	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"messages":[{"role":"user","content":"hello"}]
	}`)
	headers := http.Header{}
	headers.Set("Anthropic-Beta", "interleaved-thinking-2025-05-14")

	buildResult, err := (&GatewayService{}).buildKiroPayloadForAccount(
		context.Background(),
		account,
		nil,
		body,
		"claude-sonnet-4.6",
		"kiro-access-token",
		"claude-sonnet-4-6",
		headers,
	)
	require.NoError(t, err)
	payload := buildResult.Payload
	require.NotContains(t, string(payload), "CHUNKED WRITE PROTOCOL")
	require.Contains(t, string(payload), "\\u003cthinking_mode\\u003eenabled\\u003c/thinking_mode\\u003e")
}

func TestBuildKiroPayloadForAccountPreservesThinkingAliasAfterMapping(t *testing.T) {
	account := &Account{
		ID:       8,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
	}
	body := []byte(`{
		"model":"claude-opus-4.6",
		"messages":[{"role":"user","content":"hello"}]
	}`)

	buildResult, err := (&GatewayService{}).buildKiroPayloadForAccount(
		context.Background(),
		account,
		nil,
		body,
		"claude-opus-4.6",
		"kiro-access-token",
		"claude-opus-4-6-thinking",
		nil,
	)
	require.NoError(t, err)
	payload := buildResult.Payload

	require.Equal(t, "claude-opus-4.6", gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.modelId").String())
	systemContent := gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String()
	require.Contains(t, systemContent, "<thinking_mode>adaptive</thinking_mode>")
	require.Contains(t, systemContent, "<thinking_effort>high</thinking_effort>")
}

func TestBuildKiroPayloadForAccountMapsOpus47ToDottedModelID(t *testing.T) {
	account := &Account{
		ID:       10,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"claude-opus-4-7": "claude-opus-4.7",
			},
		},
	}
	body := []byte(`{
		"model":"claude-opus-4.7",
		"messages":[{"role":"user","content":"hello"}]
	}`)

	mappedModel := account.GetMappedModel("claude-opus-4-7")
	modelID := kiropkg.MapModel(mappedModel)
	require.Equal(t, "claude-opus-4.7", modelID)

	buildResult, err := (&GatewayService{}).buildKiroPayloadForAccount(
		context.Background(),
		account,
		nil,
		body,
		modelID,
		"kiro-access-token",
		"claude-opus-4-7",
		nil,
	)
	require.NoError(t, err)
	payload := buildResult.Payload

	require.Equal(t, "claude-opus-4.7", gjson.GetBytes(payload, "conversationState.currentMessage.userInputMessage.modelId").String())
}

func TestBuildKiroPayloadForAccountDoesNotEnableThinkingForNonThinkingAlias(t *testing.T) {
	account := &Account{
		ID:       9,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
	}
	body := []byte(`{
		"model":"claude-opus-4.6",
		"messages":[{"role":"user","content":"hello"}]
	}`)

	buildResult, err := (&GatewayService{}).buildKiroPayloadForAccount(
		context.Background(),
		account,
		nil,
		body,
		"claude-opus-4.6",
		"kiro-access-token",
		"claude-opus-4-6",
		nil,
	)
	require.NoError(t, err)
	payload := buildResult.Payload

	systemContent := gjson.GetBytes(payload, "conversationState.history.0.userInputMessage.content").String()
	require.NotContains(t, systemContent, "<thinking_mode>")
}

func TestKiroAPIRegionPrefersAPIRegionOverProfileARN(t *testing.T) {
	account := &Account{
		Credentials: map[string]any{
			"api_region":  "eu-west-1",
			"profile_arn": "arn:aws:codewhisperer:us-west-2:123456789012:profile/test",
			"region":      "ap-northeast-1",
		},
	}

	require.Equal(t, "eu-west-1", kiroAPIRegion(account))
}

func TestKiroAPIRegionIgnoresProfileARNRegionFallback(t *testing.T) {
	account := &Account{
		Credentials: map[string]any{
			"profile_arn": "arn:aws:codewhisperer:us-west-2:123456789012:profile/test",
		},
	}

	require.Equal(t, kiroDefaultRegion, kiroAPIRegion(account))
}

func TestKiroAPIRegionIgnoresOIDCRegionFallback(t *testing.T) {
	account := &Account{
		Credentials: map[string]any{
			"region": "ap-northeast-2",
		},
	}

	require.Equal(t, kiroDefaultRegion, kiroAPIRegion(account))
}

func TestBuildKiroEndpointsUsesOnlyAmazonQEndpoint(t *testing.T) {
	account := &Account{
		Credentials: map[string]any{
			"api_region":         "us-west-2",
			"preferred_endpoint": "cw",
		},
	}

	endpoints := buildKiroEndpoints(account, KiroEndpointModeQ)
	require.Len(t, endpoints, 1)
	require.Equal(t, "AmazonQ", endpoints[0].Name)
	require.Equal(t, "q.us-west-2.amazonaws.com/generateAssistantResponse", endpoints[0].URL[8:])
	require.Empty(t, endpoints[0].AmzTarget)
}

func TestBuildKiroEndpointsIgnoresPreferredEndpoint(t *testing.T) {
	for _, preferred := range []string{"codewhisperer", "cw", "unknown"} {
		account := &Account{
			Credentials: map[string]any{
				"api_region":         "us-west-2",
				"preferred_endpoint": preferred,
			},
		}

		endpoints := buildKiroEndpoints(account, KiroEndpointModeQ)
		require.Len(t, endpoints, 1)
		require.Equal(t, "AmazonQ", endpoints[0].Name)
		require.Equal(t, "q.us-west-2.amazonaws.com/generateAssistantResponse", endpoints[0].URL[8:])
	}
}

// TestBuildKiroEndpointsKRS 验证 mode=krs 时走 Kiro Runtime Service 固定 endpoint。
func TestBuildKiroEndpointsKRS(t *testing.T) {
	account := &Account{Credentials: map[string]any{"api_region": "us-west-2"}}
	endpoints := buildKiroEndpoints(account, KiroEndpointModeKRS)
	require.Len(t, endpoints, 1)
	require.Equal(t, "KiroRuntime", endpoints[0].Name)
	require.Equal(t, kiroKRSEndpointURL, endpoints[0].URL)
}

// TestBuildKiroEndpointsAuto 验证 mode=auto 时返回 Q + KRS 双端点（Q 优先）。
func TestBuildKiroEndpointsAuto(t *testing.T) {
	account := &Account{Credentials: map[string]any{"api_region": "us-west-2"}}
	endpoints := buildKiroEndpoints(account, KiroEndpointModeAuto)
	require.Len(t, endpoints, 2)
	// 第一个端点为 Q
	require.Equal(t, "AmazonQ", endpoints[0].Name)
	require.Equal(t, "https://q.us-west-2.amazonaws.com/generateAssistantResponse", endpoints[0].URL)
	// 第二个端点为 KRS
	require.Equal(t, "KiroRuntime", endpoints[1].Name)
	require.Equal(t, kiroKRSEndpointURL, endpoints[1].URL)
}

// TestEffectiveKiroEndpointMode 验证 group 取值与兜底。
func TestEffectiveKiroEndpointMode(t *testing.T) {
	require.Equal(t, KiroEndpointModeQ, (*Group)(nil).EffectiveKiroEndpointMode())
	require.Equal(t, KiroEndpointModeQ, (&Group{Platform: PlatformAnthropic, KiroEndpointMode: "krs"}).EffectiveKiroEndpointMode())
	require.Equal(t, KiroEndpointModeKRS, (&Group{Platform: PlatformKiro, KiroEndpointMode: "krs"}).EffectiveKiroEndpointMode())
	require.Equal(t, KiroEndpointModeAuto, (&Group{Platform: PlatformKiro, KiroEndpointMode: "auto"}).EffectiveKiroEndpointMode())
	require.Equal(t, KiroEndpointModeQ, (&Group{Platform: PlatformKiro, KiroEndpointMode: "bogus"}).EffectiveKiroEndpointMode())
}

// TestNormalizeKiroEndpointFieldsAuto 验证 auto 模式在 normalize 后被保留。
func TestNormalizeKiroEndpointFieldsAuto(t *testing.T) {
	g := &Group{Platform: PlatformKiro, KiroEndpointMode: KiroEndpointModeAuto}
	normalizeKiroEndpointFields(g)
	require.Equal(t, KiroEndpointModeAuto, g.KiroEndpointMode)

	// 非 kiro 平台清空
	g2 := &Group{Platform: PlatformAnthropic, KiroEndpointMode: KiroEndpointModeAuto}
	normalizeKiroEndpointFields(g2)
	require.Equal(t, "", g2.KiroEndpointMode)
}
