//go:build unit

package kimi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidateKimiURLsAllowOfficialHosts(t *testing.T) {
	deviceURL, err := ValidateOAuthEndpointURL(DefaultDeviceAuthorizationURL)
	require.NoError(t, err)
	require.Equal(t, DefaultDeviceAuthorizationURL, deviceURL)

	tokenURL, err := ValidateOAuthEndpointURL(DefaultTokenURL)
	require.NoError(t, err)
	require.Equal(t, DefaultTokenURL, tokenURL)

	baseURL, err := ValidateBaseURL(DefaultBaseURL)
	require.NoError(t, err)
	require.Equal(t, DefaultBaseURL, baseURL)

	// 空路径自动补齐 /coding/v1
	baseURLNoPath, err := ValidateBaseURL("https://api.kimi.com")
	require.NoError(t, err)
	require.Equal(t, DefaultBaseURL, baseURLNoPath)

	chatURL, err := BuildChatCompletionsURL(DefaultBaseURL + "/")
	require.NoError(t, err)
	require.Equal(t, DefaultBaseURL+"/chat/completions", chatURL)

	modelsURL, err := BuildModelsURL(DefaultBaseURL)
	require.NoError(t, err)
	require.Equal(t, DefaultBaseURL+"/models", modelsURL)
}

func TestValidateKimiURLsRejectArbitraryHostsByDefault(t *testing.T) {
	_, err := ValidateOAuthEndpointURL("https://auth.example.test/api/oauth/token")
	require.Error(t, err)

	_, err = ValidateBaseURL("https://kimi.test/coding/v1")
	require.Error(t, err)

	_, err = ValidateBaseURL("http://127.0.0.1:8080/coding/v1")
	require.Error(t, err)

	// 路径必须恰好是 /coding/v1
	_, err = ValidateBaseURL("https://api.kimi.com/custom")
	require.Error(t, err)
}

func TestValidateKimiURLsAllowUnsafeDevOverride(t *testing.T) {
	t.Setenv(EnvAllowUnsafeURLOverrides, "true")

	tokenURL, err := ValidateOAuthEndpointURL("http://127.0.0.1:8080/api/oauth/token")
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:8080/api/oauth/token", tokenURL)

	baseURL, err := ValidateBaseURL("http://127.0.0.1:8080/coding/v1/")
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:8080/coding/v1", baseURL)
}

func TestEffectiveOAuthHostEnvPriority(t *testing.T) {
	t.Setenv(EnvOAuthHost, "https://auth-a.kimi.com/")
	t.Setenv(EnvCodeOAuthHost, "https://auth-b.kimi.com/")
	require.Equal(t, "https://auth-b.kimi.com", EffectiveOAuthHost())
	require.Equal(t, "https://auth-b.kimi.com/api/oauth/token", EffectiveTokenURL())

	t.Setenv(EnvCodeOAuthHost, "")
	require.Equal(t, "https://auth-a.kimi.com", EffectiveOAuthHost())
	require.Equal(t, "https://auth-a.kimi.com/api/oauth/device_authorization", EffectiveDeviceAuthorizationURL())

	// 显式 URL 覆盖优先于 host 推导
	t.Setenv(EnvTokenURL, "https://auth-c.kimi.com/custom/token")
	require.Equal(t, "https://auth-c.kimi.com/custom/token", EffectiveTokenURL())
}

func TestParseOAuthErrorClassifiesRFC8628Errors(t *testing.T) {
	body, _ := json.Marshal(map[string]string{
		"error":             "authorization_pending",
		"error_description": "Authorization is pending",
	})
	oauthErr := ParseOAuthError(http.StatusBadRequest, body)
	require.NotNil(t, oauthErr)
	require.True(t, oauthErr.IsPending())
	require.False(t, oauthErr.IsSlowDown())
	require.False(t, oauthErr.IsUnauthorized())
	require.Contains(t, oauthErr.Error(), "authorization_pending")

	slowDown := ParseOAuthError(http.StatusBadRequest, []byte(`{"error":"slow_down"}`))
	require.True(t, slowDown.IsSlowDown())

	expired := ParseOAuthError(http.StatusBadRequest, []byte(`{"error":"expired_token"}`))
	require.True(t, expired.IsExpiredToken())

	denied := ParseOAuthError(http.StatusBadRequest, []byte(`{"error":"access_denied"}`))
	require.True(t, denied.IsAccessDenied())

	invalidGrant := ParseOAuthError(http.StatusBadRequest, []byte(`{"error":"invalid_grant","error_description":"The provided authorization grant is invalid"}`))
	require.True(t, invalidGrant.IsInvalidGrant())
	require.True(t, invalidGrant.IsUnauthorized())

	unauthorized := ParseOAuthError(http.StatusUnauthorized, []byte(`{"error":"invalid_client"}`))
	require.True(t, unauthorized.IsUnauthorized())

	// 非 JSON / 无 error 字段：仍返回带状态码的占位错误
	generic := ParseOAuthError(http.StatusInternalServerError, []byte(`upstream boom`))
	require.NotNil(t, generic)
	require.Equal(t, http.StatusInternalServerError, generic.StatusCode)
	require.Empty(t, generic.Code)
	require.False(t, generic.IsPending())
}

func TestFingerprintHeadersContainAllRequiredHeaders(t *testing.T) {
	t.Setenv(EnvDeviceName, "")
	t.Setenv(EnvDeviceModel, "")
	t.Setenv(EnvOSVersion, "")

	headers := FingerprintHeaders("0123456789abcdef0123456789abcdef")
	// 7 个指纹头：User-Agent + X-Msh-Platform/Version/Device-Name/Device-Model/Os-Version/Device-Id
	// （第 8 个头 Authorization 由调用方单独设置）
	require.Len(t, headers, 7)
	require.Equal(t, "KimiCLI/"+CLIVersion, headers["User-Agent"])
	require.Equal(t, "KimiCLI/1.3", headers["User-Agent"])
	require.True(t, strings.HasPrefix(headers["User-Agent"], "KimiCLI/"))
	require.Equal(t, "kimi_cli", headers["X-Msh-Platform"])
	require.Equal(t, CLIVersion, headers["X-Msh-Version"])
	require.Equal(t, "1.3", headers["X-Msh-Version"])
	require.NotEmpty(t, headers["X-Msh-Device-Name"])
	require.NotEmpty(t, headers["X-Msh-Device-Model"])
	require.NotEmpty(t, headers["X-Msh-Os-Version"])
	require.Equal(t, "0123456789abcdef0123456789abcdef", headers["X-Msh-Device-Id"])

	for key, value := range headers {
		for _, r := range value {
			require.True(t, r >= 32 && r <= 126, "header %s contains non-ascii char", key)
		}
	}
}

func TestFingerprintHeadersEnvOverridesAndASCIIStrip(t *testing.T) {
	t.Setenv(EnvDeviceName, "my-host-测试")
	t.Setenv(EnvDeviceModel, "macOS 15.1.1 arm64")
	t.Setenv(EnvOSVersion, "Darwin Kernel Version 24.1.0")

	headers := FingerprintHeaders("dev-id")
	require.Equal(t, "my-host-", headers["X-Msh-Device-Name"])
	require.Equal(t, "macOS 15.1.1 arm64", headers["X-Msh-Device-Model"])
	require.Equal(t, "Darwin Kernel Version 24.1.0", headers["X-Msh-Os-Version"])
}

func TestSetFingerprintHeadersWritesHTTPHeader(t *testing.T) {
	header := http.Header{}
	SetFingerprintHeaders(header, "abc123")
	require.Equal(t, UserAgent, header.Get("User-Agent"))
	require.Equal(t, "abc123", header.Get("X-Msh-Device-Id"))
	require.Equal(t, MshPlatform, header.Get("X-Msh-Platform"))
}

func TestGenerateDeviceIDFormat(t *testing.T) {
	id, err := GenerateDeviceID()
	require.NoError(t, err)
	require.Len(t, id, 32)
	require.NotContains(t, id, "-")
	for _, r := range id {
		require.True(t, (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f'))
	}

	other, err := GenerateDeviceID()
	require.NoError(t, err)
	require.NotEqual(t, id, other)
}

func TestSessionStoreExpiryAndCompletedToken(t *testing.T) {
	store := NewSessionStore()
	defer store.Stop()

	session := &DeviceSession{
		DeviceCode:      "device-code",
		UserCode:        "ABCD-EFGH",
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(time.Minute),
		IntervalSeconds: DefaultPollIntervalSeconds,
	}
	store.Set("sid", session)

	got, ok := store.Get("sid")
	require.True(t, ok)
	require.Equal(t, "device-code", got.DeviceCode)

	// 过期会话不可取
	store.Set("expired", &DeviceSession{
		DeviceCode: "old",
		CreatedAt:  time.Now().Add(-time.Hour),
		ExpiresAt:  time.Now().Add(-time.Minute),
	})
	_, ok = store.Get("expired")
	require.False(t, ok)

	// Completed token 随会话保存
	got.Token = &TokenResponse{AccessToken: "access"}
	got.Completed = true
	store.Set("sid", got)
	got2, ok := store.Get("sid")
	require.True(t, ok)
	require.True(t, got2.Completed)
	require.Equal(t, "access", got2.Token.AccessToken)

	store.Delete("sid")
	_, ok = store.Get("sid")
	require.False(t, ok)
}

func TestSessionTTLFromExpiresIn(t *testing.T) {
	require.Equal(t, DefaultSessionTTL, SessionTTLFromExpiresIn(0))
	require.Equal(t, DefaultSessionTTL, SessionTTLFromExpiresIn(-5))
	require.Equal(t, 1800*time.Second, SessionTTLFromExpiresIn(1800))
}

func TestDefaultModelMappingIncludesKimiAliases(t *testing.T) {
	t.Parallel()

	mapping := DefaultModelMapping()
	require.Equal(t, "kimi-for-coding", mapping["kimi"])
	require.Equal(t, "kimi-for-coding", mapping["kimi-latest"])
	require.Equal(t, "kimi-for-coding", mapping["kimi-code"])
	require.Equal(t, "kimi-for-coding", mapping["kimi-for-coding"])
	require.Equal(t, "kimi-for-coding-highspeed", mapping["kimi-for-coding-highspeed"])
	require.Equal(t, "k3", mapping["k3"])
	require.Equal(t, "k2p7", mapping["k2p7"])
	require.Equal(t, "kimi-k2-thinking", mapping["kimi-k2-thinking"])

	ids := DefaultModelIDs()
	require.Contains(t, ids, "kimi-for-coding")
	require.Len(t, DefaultModels(), len(ids))
}
