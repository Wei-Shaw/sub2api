//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// TestNormalizeCaptchaProvider_AcceptsTencent 覆盖 §3.1：tencent_captcha 正确识别且大小写/前后空白容错。
func TestNormalizeCaptchaProvider_AcceptsTencent(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"tencent_captcha", CaptchaProviderTencent},
		{"  TENCENT_CAPTCHA  ", CaptchaProviderTencent},
		{"hcaptcha", CaptchaProviderHcaptcha},
		{"turnstile", CaptchaProviderTurnstile},
		{"unknown_provider", CaptchaProviderTurnstile}, // 未知 provider 仍然降级 Turnstile（保持既有契约）
		{"", CaptchaProviderTurnstile},
	}
	for _, tc := range cases {
		require.Equal(t, tc.expected, normalizeCaptchaProvider(tc.input), "input=%q", tc.input)
	}
}

// TestMaskCaptchaConfig_TencentRemovesAllSecretFields 覆盖 §3.2：天御场景下 3 个敏感字段必须全部剥除，
// 仅 enabled / captcha_app_id 等公开字段保留。
func TestMaskCaptchaConfig_TencentRemovesAllSecretFields(t *testing.T) {
	in := map[string]string{
		"enabled":        "true",
		"captcha_app_id": "200000000",
		"app_secret_key": "secret-AAAA",
		"secret_id":      "AKID-BBBB",
		"secret_key":     "iam-CCCC",
	}
	out := maskCaptchaConfig(CaptchaProviderTencent, in)
	require.Equal(t, "true", out["enabled"])
	require.Equal(t, "200000000", out["captcha_app_id"])
	_, hasASK := out["app_secret_key"]
	_, hasSID := out["secret_id"]
	_, hasSK := out["secret_key"]
	require.False(t, hasASK, "app_secret_key must be masked")
	require.False(t, hasSID, "secret_id must be masked")
	require.False(t, hasSK, "secret_key must be masked")
	// 输入未被原地修改。
	require.Equal(t, "secret-AAAA", in["app_secret_key"])
}

// TestMaskCaptchaConfig_TurnstileBackwardCompat 覆盖 §3.2：Turnstile / hCaptcha 历史字段集仍按旧契约脱敏。
func TestMaskCaptchaConfig_TurnstileBackwardCompat(t *testing.T) {
	for _, provider := range []string{CaptchaProviderTurnstile, CaptchaProviderHcaptcha} {
		in := map[string]string{
			"enabled":    "true",
			"site_key":   "0xABCD",
			"secret_key": "shh",
		}
		out := maskCaptchaConfig(provider, in)
		require.Equal(t, "0xABCD", out["site_key"], provider)
		_, hasSK := out["secret_key"]
		require.False(t, hasSK, provider)
	}
}

// TestMaskCaptchaConfig_UnknownProviderConservativeStrip 覆盖 §3.2 防御性降级：未知 provider 时
// 任何已知敏感字段都必须被剥除，避免泄漏。
func TestMaskCaptchaConfig_UnknownProviderConservativeStrip(t *testing.T) {
	in := map[string]string{
		"enabled":        "true",
		"site_key":       "x",
		"secret_key":     "shh",
		"app_secret_key": "shh2",
		"secret_id":      "shh3",
	}
	out := maskCaptchaConfig("brand_new_provider", in)
	for _, k := range []string{"secret_key", "app_secret_key", "secret_id"} {
		_, has := out[k]
		require.False(t, has, "field %s must be masked under unknown provider", k)
	}
	require.Equal(t, "x", out["site_key"])
}

// TestCaptchaRuntimeFromSettings_TencentSiteKeyAndSecretKey 覆盖 §3.4：天御 provider 下
// CaptchaRuntime.SiteKey 必须是 captcha_app_id，SecretKey 是 app_secret_key（用于 Configured: bool 判定）。
func TestCaptchaRuntimeFromSettings_TencentSiteKeyAndSecretKey(t *testing.T) {
	settings := map[string]string{
		SettingKeyCaptchaProvider: CaptchaProviderTencent,
		SettingKeyCaptchaConfig:   `{"enabled":"true","captcha_app_id":"200000123","app_secret_key":"secret-AAA","secret_id":"AKID","secret_key":"iam"}`,
	}
	rt := captchaRuntimeFromSettings(settings)
	require.Equal(t, CaptchaProviderTencent, rt.Provider)
	require.True(t, rt.Enabled)
	require.Equal(t, "200000123", rt.SiteKey, "tencent site_key must come from captcha_app_id")
	require.Equal(t, "secret-AAA", rt.SecretKey, "tencent primary secret must come from app_secret_key")
	// 完整 config 仍然在 Config 中保留，便于业务层取 secret_id / secret_key。
	require.Equal(t, "AKID", rt.Config["secret_id"])
	require.Equal(t, "iam", rt.Config["secret_key"])
}

// TestCaptchaRuntimeFromSettings_TurnstileBackwardCompat 覆盖 §3.4：非天御 provider 行为不变。
func TestCaptchaRuntimeFromSettings_TurnstileBackwardCompat(t *testing.T) {
	settings := map[string]string{
		SettingKeyCaptchaProvider: CaptchaProviderTurnstile,
		SettingKeyCaptchaConfig:   `{"enabled":"true","site_key":"0xPUB","secret_key":"shh"}`,
	}
	rt := captchaRuntimeFromSettings(settings)
	require.Equal(t, CaptchaProviderTurnstile, rt.Provider)
	require.Equal(t, "0xPUB", rt.SiteKey)
	require.Equal(t, "shh", rt.SecretKey)
}

// TestSettingService_GetPublicSettings_TencentSiteKeyExposedAsAppId 覆盖 §3.4 + §3.5：
// 天御 provider 的 PublicSettings.CaptchaSiteKey 必须是 captcha_app_id（而非 site_key）。
func TestSettingService_GetPublicSettings_TencentSiteKeyExposedAsAppId(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyCaptchaProvider: CaptchaProviderTencent,
			SettingKeyCaptchaConfig:   `{"enabled":"true","captcha_app_id":"200000999","app_secret_key":"AAA","secret_id":"BBB","secret_key":"CCC"}`,
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, CaptchaProviderTencent, settings.CaptchaProvider)
	require.True(t, settings.CaptchaEnabled)
	require.Equal(t, "200000999", settings.CaptchaSiteKey, "PublicSettings must expose captcha_app_id under tencent_captcha")
}
