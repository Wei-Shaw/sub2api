package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateDingTalkConfig_Disabled_Skip(t *testing.T) {
	require.NoError(t, ValidateDingTalkConfig(DingTalkConnectConfig{Enabled: false}))
}

func TestValidateDingTalkConfig_V4_DingTalkAppKind(t *testing.T) {
	err := ValidateDingTalkConfig(DingTalkConnectConfig{
		Enabled:               true,
		DingTalkAppKind:       "third_party_enterprise_app",
		CorpRestrictionPolicy: "none",
	})
	require.ErrorIs(t, err, ErrDingTalkV4InvalidAppKind)
}

func TestValidateDingTalkConfig_V1_InternalOnlyRequiresInternalAppType(t *testing.T) {
	err := ValidateDingTalkConfig(DingTalkConnectConfig{
		Enabled:               true,
		DingTalkAppKind:       "internal_app",
		AppType:               "public",
		CorpRestrictionPolicy: "internal_only",
		InternalCorpID:        "dingABC",
	})
	require.ErrorIs(t, err, ErrDingTalkV1AppTypeMismatch)
}

// TestValidateDingTalkConfig_V3_InternalOnlyAllowsEmptyCorpID 验证方案 A：
// internal_only 策略下，InternalCorpID="" 应通过校验（企业隔离由钉钉 AppType=internal 保证）。
func TestValidateDingTalkConfig_V3_InternalOnlyAllowsEmptyCorpID(t *testing.T) {
	err := ValidateDingTalkConfig(DingTalkConnectConfig{
		Enabled:               true,
		DingTalkAppKind:       "internal_app",
		AppType:               "internal",
		CorpRestrictionPolicy: "internal_only",
		InternalCorpID:        "",
	})
	require.NoError(t, err)
}

func TestValidateDingTalkConfig_HappyPath_None(t *testing.T) {
	require.NoError(t, ValidateDingTalkConfig(DingTalkConnectConfig{
		Enabled:               true,
		DingTalkAppKind:       "internal_app",
		AppType:               "public",
		CorpRestrictionPolicy: "none",
	}))
}
