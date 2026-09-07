//go:build unit

package service

import (
	"regexp"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/stretchr/testify/require"
)

func kiroAccount(creds map[string]any) *Account {
	return &Account{ID: 42, Platform: PlatformKiro, Credentials: creds}
}

func TestKiroAuthMethodDefaultsToSocial(t *testing.T) {
	require.Equal(t, kiro.AuthSocial, kiroAccount(nil).KiroAuthMethod())
	require.Equal(t, kiro.AuthSocial, kiroAccount(map[string]any{}).KiroAuthMethod())
	require.Equal(t, kiro.AuthIdC, kiroAccount(map[string]any{"auth_method": "idc"}).KiroAuthMethod())
	require.Equal(t, kiro.AuthBuilderID, kiroAccount(map[string]any{"auth_method": "builder_id"}).KiroAuthMethod())
}

func TestIsKiroAPIKeyAccount(t *testing.T) {
	require.True(t, kiroAccount(map[string]any{"auth_method": "api_key"}).IsKiroAPIKeyAccount())
	require.False(t, kiroAccount(map[string]any{"auth_method": "social"}).IsKiroAPIKeyAccount())
}

func TestKiroRegionDefaults(t *testing.T) {
	require.Equal(t, "us-east-1", kiroAccount(nil).KiroRegion())
	require.Equal(t, "eu-central-1", kiroAccount(map[string]any{"region": "eu-central-1"}).KiroRegion())
	// 空白值也退回默认。
	require.Equal(t, "us-east-1", kiroAccount(map[string]any{"region": "   "}).KiroRegion())
}

// TestKiroBearerTokenPicksAPIKeyForAPIKeyAccounts 覆盖两类账号的取值差异。
func TestKiroBearerTokenPicksAPIKeyForAPIKeyAccounts(t *testing.T) {
	apiKeyAcc := kiroAccount(map[string]any{
		"auth_method":  "api_key",
		"api_key":      "kiro_ak_123",
		"access_token": "should_not_be_used",
	})
	require.Equal(t, "kiro_ak_123", apiKeyAcc.KiroBearerToken())

	oauthAcc := kiroAccount(map[string]any{
		"auth_method":  "social",
		"access_token": "at_456",
	})
	require.Equal(t, "at_456", oauthAcc.KiroBearerToken())
}

func TestKiroClientCredentials(t *testing.T) {
	id, secret := kiroAccount(map[string]any{
		"client_id": "cid", "client_secret": "csec",
	}).KiroClientCredentials()
	require.Equal(t, "cid", id)
	require.Equal(t, "csec", secret)

	id, secret = kiroAccount(nil).KiroClientCredentials()
	require.Empty(t, id)
	require.Empty(t, secret)
}

func TestKiroFakeThinkingDefaultsOn(t *testing.T) {
	require.True(t, kiroAccount(nil).KiroFakeThinking(), "假思考默认开启——客户端的推理强度诉求默认要在 Kiro 端生效")
	require.True(t, kiroAccount(map[string]any{}).KiroFakeThinking())
	require.True(t, kiroAccount(map[string]any{"fake_thinking": true}).KiroFakeThinking())
	require.False(t, kiroAccount(map[string]any{"fake_thinking": false}).KiroFakeThinking(), "显式设为 false 必须能关闭")
	// JSONB 往返后布尔可能变成字符串。
	require.True(t, kiroAccount(map[string]any{"fake_thinking": "true"}).KiroFakeThinking())
	require.False(t, kiroAccount(map[string]any{"fake_thinking": "false"}).KiroFakeThinking())
}

// TestEnsureKiroMachineIDGeneratesOnceAndPersists 覆盖 §5.5 第 2 点：
// machine_id 参与设备指纹，必须一次生成、永久稳定。
func TestEnsureKiroMachineIDGeneratesOnceAndPersists(t *testing.T) {
	creds := map[string]any{}

	id, created := EnsureKiroMachineID(creds)
	require.True(t, created, "首次必须报告已新建，调用方据此落库")
	require.NotEmpty(t, id)
	require.Equal(t, id, creds["machine_id"], "必须写回 creds")

	again, created := EnsureKiroMachineID(creds)
	require.False(t, created, "已存在时不得重新生成")
	require.Equal(t, id, again, "同一账号的 machine_id 必须稳定")
}

func TestGenerateKiroMachineIDShape(t *testing.T) {
	id, err := GenerateKiroMachineID()
	require.NoError(t, err)
	require.Regexp(t, regexp.MustCompile(`^[0-9a-f]{64}$`), id,
		"形态需与 Kiro IDE 的机器指纹一致（64 位十六进制）")

	other, err := GenerateKiroMachineID()
	require.NoError(t, err)
	require.NotEqual(t, id, other)
}

func TestKiroTokenCacheKey(t *testing.T) {
	require.Equal(t, "kiro:account:42", KiroTokenCacheKey(kiroAccount(nil)))
	require.Equal(t, "kiro:account:0", KiroTokenCacheKey(nil))
}

// TestNormalizeKiroAccountTypeMatchesAntigravityConvention 是"把 Kiro 鉴权
// 方式改成跟 Antigravity 一样的 OAuth 和 API key"这个需求的核心断言：
// social/builder_id/idc 都是真 OAuth，只有 api_key 不是——不能像此前
// CreateAccountModal.vue 那样不管选哪种都恒填 'apikey'。
func TestNormalizeKiroAccountTypeMatchesAntigravityConvention(t *testing.T) {
	tests := []struct {
		name       string
		authMethod string
		want       string
	}{
		{"social is oauth", "social", AccountTypeOAuth},
		{"builder_id is oauth", "builder_id", AccountTypeOAuth},
		{"idc is oauth", "idc", AccountTypeOAuth},
		{"api_key is apikey", "api_key", AccountTypeAPIKey},
		{"missing auth_method defaults to social, so oauth", "", AccountTypeOAuth},
		{"unrecognized auth_method falls back to social, so oauth", "whatever", AccountTypeOAuth},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds := map[string]any{}
			if tt.authMethod != "" {
				creds["auth_method"] = tt.authMethod
			}
			require.Equal(t, tt.want, normalizeKiroAccountType(PlatformKiro, creds))
		})
	}
}
