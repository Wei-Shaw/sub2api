package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/model"
)

// newUATestContext 构造一个只带指定 User-Agent 头的 *gin.Context,用于驱动按入站 UA 的路由匹配。
func newUATestContext(ua string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("User-Agent", ua)
	c.Request = req
	return c
}

// routingTestProfileService 提供若干带名字的指纹模板,断言时按 Name 区分选中的是哪一个。
func routingTestProfileService() *TLSFingerprintProfileService {
	return &TLSFingerprintProfileService{localCache: map[int64]*model.TLSFingerprintProfile{
		100: {ID: 100, Name: "claude-fp"},
		200: {ID: 200, Name: "python-fp"},
		300: {ID: 300, Name: "codex-fp"},
		400: {ID: 400, Name: "vscode-fp"},
		500: {ID: 500, Name: "account-fixed"},
	}}
}

// TestGatewayService_AnthropicRoutingByUA 验证 Anthropic 账号按入站 UA 选指纹:
// 命中规则用规则 profile;未命中回落账号固定 profile。级联:router 规则 > 账号固定 profile。
func TestGatewayService_AnthropicRoutingByUA(t *testing.T) {
	router := &model.TLSFingerprintRouter{
		ID:      1,
		Name:    "anthropic-clients",
		Enabled: true,
		Rules: []model.TLSFingerprintRouterRule{
			{Name: "claude", Enabled: true, MatchType: model.TLSRouterMatchContains, Pattern: "claude", TLSFingerprintProfileID: 100},
			{Name: "python", Enabled: true, MatchType: model.TLSRouterMatchContains, Pattern: "python", TLSFingerprintProfileID: 200},
		},
	}
	gw := &GatewayService{
		tlsFPProfileService: routingTestProfileService(),
		tlsFPRouterService:  newTLSFingerprintRouterTestService(router),
	}
	account := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"enable_tls_fingerprint":     true,
			"tls_fingerprint_router_id":  int64(1),
			"tls_fingerprint_profile_id": int64(500), // 账号固定 profile(未命中规则时回落)
		},
	}

	// 命中 "claude" 规则 → 规则 profile(优先于账号固定 profile)。
	require.Equal(t, "claude-fp",
		gw.resolveTLSProfileForRequest(newUATestContext("claude-cli/2.1.161 (external, cli)"), account).Name)
	// 命中 "python" 规则。
	require.Equal(t, "python-fp",
		gw.resolveTLSProfileForRequest(newUATestContext("python-requests/2.31"), account).Name)
	// 未命中任何规则 → 回落账号固定 profile。
	require.Equal(t, "account-fixed",
		gw.resolveTLSProfileForRequest(newUATestContext("curl/8.0"), account).Name)
}

// TestOpenAIGatewayService_RoutingByUA 验证 OpenAI 账号按入站 UA 选指纹 + 规则改写出站 UA/originator。
func TestOpenAIGatewayService_RoutingByUA(t *testing.T) {
	router := &model.TLSFingerprintRouter{
		ID:      1,
		Name:    "openai-clients",
		Enabled: true,
		Rules: []model.TLSFingerprintRouterRule{
			{
				Name: "codex-cli", Enabled: true,
				MatchType: model.TLSRouterMatchContains, Pattern: "codex_cli_rs",
				TLSFingerprintProfileID: 300,
				UpstreamUserAgent:       "codex_cli_rs/0.125.0",
				UpstreamOriginator:      "codex_cli_rs",
			},
			{
				Name: "codex-vscode", Enabled: true,
				MatchType: model.TLSRouterMatchContains, Pattern: "codex_vscode",
				TLSFingerprintProfileID: 400,
			},
		},
	}
	svc := &OpenAIGatewayService{
		tlsFPProfileService: routingTestProfileService(),
		tlsFPRouterService:  newTLSFingerprintRouterTestService(router),
	}
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"enable_tls_fingerprint":     true,
			"tls_fingerprint_router_id":  int64(1),
			"tls_fingerprint_profile_id": int64(500),
		},
	}

	// codex CLI 入站 → codex-fp,且规则改写出站 UA/originator 使其与指纹自洽。
	cliMatch := svc.matchTLSFingerprintRouter(
		newUATestContext("codex_cli_rs/0.125.0 (Ubuntu 22.4.0; x86_64) xterm-256color"), account)
	require.True(t, cliMatch.Matched)
	require.Equal(t, "codex_cli_rs/0.125.0", cliMatch.UpstreamUserAgent)
	require.Equal(t, "codex_cli_rs", cliMatch.UpstreamOriginator)
	require.Equal(t, "codex-fp", svc.resolveOpenAITLSProfile(account, cliMatch).Name)

	// codex VS Code 扩展入站 → vscode-fp。
	vscodeMatch := svc.matchTLSFingerprintRouter(newUATestContext("codex_vscode/1.2.3"), account)
	require.True(t, vscodeMatch.Matched)
	require.Equal(t, "vscode-fp", svc.resolveOpenAITLSProfile(account, vscodeMatch).Name)

	// 未命中(浏览器型 UA)→ 不匹配路由,resolveOpenAITLSProfile 回落账号固定 profile。
	noMatch := svc.matchTLSFingerprintRouter(newUATestContext("Mozilla/5.0"), account)
	require.False(t, noMatch.Matched)
	require.Equal(t, "account-fixed", svc.resolveOpenAITLSProfile(account, noMatch).Name)
}
