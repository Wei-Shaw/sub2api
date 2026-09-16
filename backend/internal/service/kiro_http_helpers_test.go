//go:build unit

package service

import (
	"context"
	"net/http"
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
	require.Equal(t, "arn:aws:codewhisperer:us-east-1:123456789012:profile/HEADER", req.Header.Get("x-amzn-kiro-profile-arn"))
	require.Equal(t, "vibe", req.Header.Get("x-amzn-kiro-agent-mode"))
	require.Equal(t, "true", req.Header.Get("x-amzn-codewhisperer-optout"))
	// streamingSDKVersions 是多元素池，seed 决定命中哪一条：直接与 helper 生成的
	// 参考 UA 全等比较，天然覆盖 aws-sdk-js/ 与 api/codewhispererstreaming# 段。
	expectedUA := kiropkg.BuildRuntimeUserAgent("account-key", buildKiroMachineID(account))
	expectedAmzUA := kiropkg.BuildRuntimeAmzUserAgent("account-key", buildKiroMachineID(account))
	require.Equal(t, expectedUA, req.Header.Get("User-Agent"))
	require.Equal(t, expectedAmzUA, req.Header.Get("X-Amz-User-Agent"))
	require.Contains(t, req.Header.Get("User-Agent"), "md/nodejs#22.22.0")
	require.Contains(t, req.Header.Get("User-Agent"), buildKiroMachineID(account))
	require.Contains(t, req.Header.Get("X-Amz-User-Agent"), buildKiroMachineID(account))
	require.Contains(t, req.Header.Get("User-Agent"), "api/codewhispererstreaming#")
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
	// API Key 账号必须带 TokenType: API_KEY。
	require.Equal(t, []string{"API_KEY"}, req.Header["TokenType"])
	require.Equal(t, "Bearer ksk_test_key", req.Header.Get("Authorization"))

	// OAuth 账号不应带 API_KEY TokenType
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
		Type: AccountTypeAPIKey,
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

// 推理区域只认凭据 api_region。IdC/SSO 授权写入的 region 是 Identity Center
// 区域(用于 oidc.{region}.amazonaws.com 换取令牌),与推理端点无关,任何账号
// 类型下都不得回退到它,否则会把 SSO 区域误当推理区域。
func TestKiroAPIRegionIgnoresSSORegion(t *testing.T) {
	cases := []struct {
		name        string
		accountType string
		credentials map[string]any
	}{
		{
			name:        "oauth",
			accountType: AccountTypeOAuth,
			credentials: map[string]any{"region": "ap-northeast-2"},
		},
		{
			name:        "oauth idc",
			accountType: AccountTypeOAuth,
			credentials: map[string]any{"auth_method": "idc", "region": "eu-central-1"},
		},
		{
			name:        "apikey",
			accountType: AccountTypeAPIKey,
			credentials: map[string]any{"region": "eu-central-1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{Type: tc.accountType, Credentials: tc.credentials}
			require.Equal(t, kiroDefaultRegion, kiroAPIRegion(account))
		})
	}
}

func TestBuildKiroEndpointsUsesOnlyAmazonQEndpoint(t *testing.T) {
	account := &Account{
		Type: AccountTypeAPIKey,
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

// 端点构造同样只认 api_region:仅有 SSO region 时落到默认区域,不得据其拼出端点。
func TestBuildKiroEndpointsIgnoresSSORegion(t *testing.T) {
	account := &Account{
		Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"region": "eu-central-1",
		},
	}

	endpoints := buildKiroEndpoints(account, KiroEndpointModeQ)
	require.Len(t, endpoints, 1)
	require.Equal(t, "AmazonQ", endpoints[0].Name)
	require.Equal(t, "https://q."+kiroDefaultRegion+".amazonaws.com/generateAssistantResponse", endpoints[0].URL)
}

func TestBuildKiroEndpointsIgnoresPreferredEndpoint(t *testing.T) {
	for _, preferred := range []string{"codewhisperer", "cw", "unknown"} {
		account := &Account{
			Type: AccountTypeAPIKey,
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

// TestKiroAccountLacksEnterpriseProfile 验证 BuilderId/Social 被判定为无企业 profile（跳过
// ListAvailableProfiles），而 Enterprise/ExternalIdp 及未知 provider 一律照旧查询。
func TestKiroAccountLacksEnterpriseProfile(t *testing.T) {
	cases := []struct {
		name        string
		credentials map[string]any
		want        bool
	}{
		{
			name:        "provider BuilderId 跳过",
			credentials: map[string]any{"auth_method": "idc", "provider": "BuilderId"},
			want:        true,
		},
		{
			name:        "provider 大小写不敏感",
			credentials: map[string]any{"auth_method": "idc", "provider": "builderid"},
			want:        true,
		},
		{
			name:        "social 登录跳过",
			credentials: map[string]any{"auth_method": "social", "provider": "Github"},
			want:        true,
		},
		{
			name:        "builder id start_url 跳过",
			credentials: map[string]any{"auth_method": "idc", "start_url": kiropkg.BuilderIDStartURL},
			want:        true,
		},
		{
			name:        "builder id start_url 末尾斜杠跳过",
			credentials: map[string]any{"auth_method": "idc", "start_url": kiropkg.BuilderIDStartURL + "/"},
			want:        true,
		},
		{
			name:        "provider 与 start_url 均缺失视为 BuilderId",
			credentials: map[string]any{"api_region": "us-west-2"},
			want:        true,
		},
		{
			name:        "provider Enterprise 照旧查询",
			credentials: map[string]any{"auth_method": "idc", "provider": "Enterprise"},
			want:        false,
		},
		{
			name:        "provider Enterprise 大小写不敏感",
			credentials: map[string]any{"auth_method": "idc", "provider": "enterprise"},
			want:        false,
		},
		{
			name:        "遗留 provider AWS + 企业 start_url 照旧查询",
			credentials: map[string]any{"auth_method": "idc", "provider": "AWS", "start_url": "https://d-example.awsapps.com/start"},
			want:        false,
		},
		{
			name:        "遗留 provider AWS 无 start_url 跳过",
			credentials: map[string]any{"auth_method": "idc", "provider": "AWS"},
			want:        true,
		},
		{
			name:        "遗留 provider AWS + builder id start_url 跳过",
			credentials: map[string]any{"auth_method": "idc", "provider": "AWS", "start_url": kiropkg.BuilderIDStartURL},
			want:        true,
		},
		{
			name:        "企业 start_url 无 provider 照旧查询",
			credentials: map[string]any{"auth_method": "idc", "start_url": "https://d-example.awsapps.com/start"},
			want:        false,
		},
		{
			name:        "external_idp 照旧查询",
			credentials: map[string]any{"auth_method": "external_idp", "provider": "ExternalIdp"},
			want:        false,
		},
		{
			name:        "auth_method external_idp 无 provider 照旧查询",
			credentials: map[string]any{"auth_method": "external_idp"},
			want:        false,
		},
		{
			name:        "provider BuilderId 优先于企业 start_url 跳过",
			credentials: map[string]any{"auth_method": "idc", "provider": "BuilderId", "start_url": "https://d-example.awsapps.com/start"},
			want:        true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{Platform: PlatformKiro, Type: AccountTypeOAuth, Credentials: tc.credentials}
			require.Equal(t, tc.want, kiroAccountLacksEnterpriseProfile(account))
		})
	}

	require.False(t, kiroAccountLacksEnterpriseProfile(nil))
}
