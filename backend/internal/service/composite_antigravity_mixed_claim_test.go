package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// antigravity 账号的平台默认映射（Claude / Gemini 模型）只在开启混合调度时构成
// composite 声明；未开启时只认管理员显式配置的映射，与非 composite 混合调度开关同语义。
func TestCompositeClaimAntigravityDefaultMappingRequiresMixedScheduling(t *testing.T) {
	const model = "claude-sonnet-4-5"
	mixedOff := &Account{ID: 1, Platform: PlatformAntigravity, Type: AccountTypeOAuth}
	require.Equal(t, CompositeClaimNone, CompositeAccountClaimStrength(mixedOff, model))
	require.False(t, CompositeAccountClaimsModel(mixedOff, model))

	mixedOn := &Account{ID: 2, Platform: PlatformAntigravity, Type: AccountTypeOAuth, Extra: map[string]any{"mixed_scheduling": true}}
	require.Equal(t, CompositeClaimExplicit, CompositeAccountClaimStrength(mixedOn, model))

	explicitMixedOff := &Account{ID: 3, Platform: PlatformAntigravity, Type: AccountTypeOAuth,
		Credentials: map[string]any{"model_mapping": map[string]any{"team-alias": "claude-opus-4-8"}}}
	require.Equal(t, CompositeClaimExplicit, CompositeAccountClaimStrength(explicitMixedOff, "team-alias"))
	require.Equal(t, CompositeClaimNone, CompositeAccountClaimStrength(explicitMixedOff, model), "默认映射条目不因存在自定义映射而生效")
}
