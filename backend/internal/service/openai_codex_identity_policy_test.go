package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// 账号自身开关语义（默认关闭/非OAuth等）已由 TestAccount_IsForceCodexIdentityEnabled 覆盖，
// 本测试只针对 resolveOpenAIForceCodexIdentityEnabled 自身的母账号解析分支：普通/影子/坏父。
func TestResolveOpenAIForceCodexIdentityEnabled(t *testing.T) {
	ctx := context.Background()
	pid := int64(100)

	t.Run("普通账号-读取自身开关", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"force_codex_identity": true},
		}
		repo := newStubCredRepo(nil)
		enabled, err := resolveOpenAIForceCodexIdentityEnabled(ctx, repo, account)
		require.NoError(t, err)
		require.True(t, enabled)
	})

	t.Run("spark影子继承母账号开关（影子自身无extra）", func(t *testing.T) {
		parent := &Account{
			ID:       100,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"force_codex_identity": true},
		}
		shadow := &Account{ID: 200, ParentAccountID: &pid, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: nil}
		repo := newStubCredRepo(parent)
		enabled, err := resolveOpenAIForceCodexIdentityEnabled(ctx, repo, shadow)
		require.NoError(t, err)
		require.True(t, enabled)
	})

	t.Run("母账号损坏时返回错误", func(t *testing.T) {
		shadow := &Account{ID: 200, ParentAccountID: &pid, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
		badRepo := newStubCredRepo(&Account{ID: 100, Platform: PlatformOpenAI, Type: AccountTypeAPIKey})
		_, err := resolveOpenAIForceCodexIdentityEnabled(ctx, badRepo, shadow)
		require.Error(t, err)
	})
}

func TestApplyForcedCodexCLIUserAgent(t *testing.T) {
	header := http.Header{}
	header.Set("user-agent", "curl/8.0")
	header.Set("originator", "opencode")
	header.Set("version", "0.125.0")

	applyForcedCodexCLIUserAgent(header)

	require.Equal(t, codexCLIUserAgent, header.Get("user-agent"))
	require.Equal(t, "opencode", header.Get("originator"))

	enforceCodexIdentityHeaders(header)
	require.Equal(t, codexCLIUserAgent, header.Get("user-agent"))
	require.Equal(t, "codex_cli_rs", header.Get("originator"))
	require.Equal(t, codexCLIVersion, header.Get("version"))
	require.NotPanics(t, func() { applyForcedCodexCLIUserAgent(nil) })
}
