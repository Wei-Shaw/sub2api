package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

// codexUpstreamCallRecorder 记录 Do/DoWithTLS 各自被调用的次数与最后一次收到的 profile，
// 用于断言 doOpenAIUpstream 走的是哪条出站路径。
type codexUpstreamCallRecorder struct {
	doCalls        int
	doWithTLSCalls int
	lastProfile    *tlsfingerprint.Profile
}

func (u *codexUpstreamCallRecorder) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.doCalls++
	return httptest.NewRecorder().Result(), nil
}

func (u *codexUpstreamCallRecorder) DoWithTLS(_ *http.Request, _ string, _ int64, _ int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	u.doWithTLSCalls++
	u.lastProfile = profile
	return httptest.NewRecorder().Result(), nil
}

// TestDoOpenAIUpstreamUsesResolvedCodexTLSProfile 覆盖 research.md §5：doOpenAIUpstream
// 在没有插件接管时必须调用 DoWithTLS（而不是 Do），且传入的 profile 由
// resolveOpenAICodexTLSProfile 决定——Codex OAuth 账号未显式配置时是 codexTLSProfile，
// API Key 账号是 nil（等价于既有 Do 行为，不引入回归）。
func TestDoOpenAIUpstreamUsesResolvedCodexTLSProfile(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)

	t.Run("Codex OAuth 账号自动套用 codexTLSProfile", func(t *testing.T) {
		upstream := &codexUpstreamCallRecorder{}
		svc := &OpenAIGatewayService{httpUpstream: upstream}
		account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1}

		_, err := svc.doOpenAIUpstream(req, "", account)
		require.NoError(t, err)
		require.Equal(t, 0, upstream.doCalls, "不应再直接调用 Do")
		require.Equal(t, 1, upstream.doWithTLSCalls)
		require.Same(t, codexTLSProfile, upstream.lastProfile)
	})

	t.Run("API Key 账号不启用 TLS 指纹", func(t *testing.T) {
		upstream := &codexUpstreamCallRecorder{}
		svc := &OpenAIGatewayService{httpUpstream: upstream}
		account := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1}

		_, err := svc.doOpenAIUpstream(req, "", account)
		require.NoError(t, err)
		require.Equal(t, 1, upstream.doWithTLSCalls, "统一走 DoWithTLS 分发点")
		require.Nil(t, upstream.lastProfile, "profile 为 nil 时 DoWithTLS 退化为普通 Do 行为")
	})
}
