package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

// TestCodexTLSProfileCipherSuites 覆盖 contracts/tls-profile-values.md 记录的密码套件表：
// 顺序和内容必须逐项等于三次真实抓包 + 官方 openai/codex 源码交叉验证过的值。
func TestCodexTLSProfileCipherSuites(t *testing.T) {
	want := []uint16{
		0x1302, 0x1301, 0x1303,
		0xc02c, 0xc02b, 0xcca9,
		0xc030, 0xc02f, 0xcca8,
		0x00ff,
	}
	require.Equal(t, want, codexTLSProfile.CipherSuites)
}

// TestCodexTLSProfileGroups 覆盖 supported_groups 与 key_share 的分组集合：两者必须一致
// （真实客户端里 key_share 携带的分组就是 supported_groups 声明的分组，见 data-model.md）。
func TestCodexTLSProfileGroups(t *testing.T) {
	want := []uint16{0x11ec, 0x001d, 0x0017, 0x0018}
	require.Equal(t, want, codexTLSProfile.Curves)
	require.Equal(t, want, codexTLSProfile.KeyShareGroups)
}

// TestCodexTLSProfilePointFormatsAndALPN 覆盖 ec_point_formats 固定值，以及真实 Codex CLI
// （reqwest 编译时未启用 http2 feature）从不发送 ALPN 这一行为。
func TestCodexTLSProfilePointFormatsAndALPN(t *testing.T) {
	require.Equal(t, []uint16{0}, codexTLSProfile.PointFormats)
	require.Empty(t, codexTLSProfile.ALPNProtocols)
	require.NotContains(t, codexTLSProfile.Extensions, uint16(16), "不应声明 ALPN 扩展类型 ID")
}

// TestCodexTLSProfileExtensionSet 覆盖扩展类型集合：必须恰好是 contract 文档的 10 项，
// 不多不少；顺序由 US2 的随机打乱负责，这里只比较集合。
func TestCodexTLSProfileExtensionSet(t *testing.T) {
	want := map[uint16]bool{
		0: true, 5: true, 10: true, 11: true, 13: true,
		23: true, 35: true, 43: true, 45: true, 51: true,
	}
	require.Len(t, codexTLSProfile.Extensions, len(want))
	for _, id := range codexTLSProfile.Extensions {
		require.True(t, want[id], "扩展类型 %d 不在预期集合内", id)
	}
	for _, forbidden := range []uint16{18, 65281, 65037} {
		require.NotContains(t, codexTLSProfile.Extensions, forbidden)
	}
}

// TestCodexTLSProfileRandomizesExtensionOrder 覆盖 US2 的验收标准：codexTLSProfile 必须
// 开启扩展顺序随机化开关。
func TestCodexTLSProfileRandomizesExtensionOrder(t *testing.T) {
	require.True(t, codexTLSProfile.RandomizeExtensionOrder)
}

// TestResolveOpenAICodexTLSProfile 覆盖 research.md §6 的三层解析规则：
// 账号已显式配置的 TLS 指纹优先；否则 OpenAI Codex OAuth 账号自动套用 codexTLSProfile；
// 其它情况不启用 TLS 指纹。
func TestResolveOpenAICodexTLSProfile(t *testing.T) {
	codexOAuthAccount := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKeyAccount := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	t.Run("账号已显式配置时使用账号自己的 Profile", func(t *testing.T) {
		explicit := &tlsfingerprint.Profile{Name: "admin-picked"}
		got := resolveOpenAICodexTLSProfile(explicit, codexOAuthAccount)
		require.Same(t, explicit, got)
	})

	t.Run("Codex OAuth 账号未显式配置时自动套用 codexTLSProfile", func(t *testing.T) {
		got := resolveOpenAICodexTLSProfile(nil, codexOAuthAccount)
		require.Same(t, codexTLSProfile, got)
	})

	t.Run("非 Codex OAuth 账号未显式配置时不启用 TLS 指纹", func(t *testing.T) {
		got := resolveOpenAICodexTLSProfile(nil, apiKeyAccount)
		require.Nil(t, got)
	})
}
