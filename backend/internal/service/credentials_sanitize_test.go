package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeStoredCredentials_StripsEphemeralSSOSecrets(t *testing.T) {
	creds := map[string]any{
		"access_token":      "at",
		"refresh_token":     "rt",
		"password":          "secret",
		"sso_token":         "sso",
		"sso":               "cookie-sso",
		"sso-rw":            "rw",
		"clearTextPassword": "plain",
		"cookie":            "jar",
		"base_url":          "https://api.x.ai",
	}
	out := SanitizeStoredCredentials(PlatformGrok, creds)
	require.Equal(t, "at", out["access_token"])
	require.Equal(t, "rt", out["refresh_token"])
	require.Equal(t, "https://api.x.ai", out["base_url"])
	require.NotContains(t, out, "password")
	require.NotContains(t, out, "sso_token")
	require.NotContains(t, out, "sso")
	require.NotContains(t, out, "sso-rw")
	require.NotContains(t, out, "clearTextPassword")
	require.NotContains(t, out, "cookie")
}

// Adobe 的长期凭据就是浏览器 cookie（没有 refresh_token），删掉它整个渠道就无法认证：
// 账号会以「无 cookie 无 token」落库，后台刷新器永远没有可换的东西。
// 这曾经真的发生过——4 个 Adobe 账号入库后 credentials 里只剩 model_mapping。
func TestSanitizeStoredCredentials_KeepsAdobeCookie(t *testing.T) {
	creds := map[string]any{
		"cookie":       "aux_sid=abc; ims_sid=def",
		"access_token": "at",
		"password":     "x",
		"sso_token":    "y",
	}
	out := SanitizeStoredCredentials(PlatformAdobe, creds)
	require.Equal(t, "aux_sid=abc; ims_sid=def", out["cookie"],
		"Adobe 的 cookie 必须落库，否则 AdobeTokenRefresher.CanRefresh 永远为 false")
	require.Equal(t, "at", out["access_token"])
	// 其它临时密钥照删不误。
	require.NotContains(t, out, "password")
	require.NotContains(t, out, "sso_token")
}

func TestSanitizeStoredCredentials_StripsCookieForOtherPlatforms(t *testing.T) {
	// Bulk paths may pass empty platform; cookie must never persist next to tokens.
	for _, platform := range []string{PlatformOpenAI, PlatformGrok, ""} {
		creds := map[string]any{
			"cookie":   "session",
			"password": "x",
			"api_key":  "k",
		}
		out := SanitizeStoredCredentials(platform, creds)
		require.Equal(t, "k", out["api_key"], platform)
		require.NotContains(t, out, "password", platform)
		require.NotContains(t, out, "cookie", platform)
	}
}

func TestSanitizeStoredCredentials_NilSafe(t *testing.T) {
	require.Nil(t, SanitizeStoredCredentials(PlatformGrok, nil))
}
