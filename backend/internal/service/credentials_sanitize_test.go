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

func TestSanitizeStoredCredentials_AlwaysStripsCookie(t *testing.T) {
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
