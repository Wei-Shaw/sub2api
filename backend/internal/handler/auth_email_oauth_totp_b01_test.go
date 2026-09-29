//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func b01NewTotpEmailOAuthHandler(t *testing.T) (*AuthHandler, *dbent.Client) {
	t.Helper()
	return newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		settingValues: map[string]string{
			service.SettingKeyTotpEnabled: "true",
		},
		totpCache:     &oauthPendingFlowTotpCacheStub{},
		totpEncryptor: oauthPendingFlowTotpEncryptorStub{},
	})
}

func b01CreateTotpUser(t *testing.T, client *dbent.Client, email string) *dbent.User {
	t.Helper()
	user, err := client.User.Create().
		SetEmail(email).
		SetUsername("b01-totp-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		SetTotpEnabled(true).
		SetTotpSecretEncrypted("JBSWY3DPEHPK3PXP").
		SetTotpEnabledAt(time.Now().UTC().Add(-time.Hour)).
		Save(context.Background())
	require.NoError(t, err)
	return user
}

func b01RunGoogleCallback(handler *AuthHandler, subject, email string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/callback", nil)
	handler.emailOAuthCallbackWithProfile(c, "google", config.EmailOAuthProviderConfig{
		Enabled:             true,
		ClientID:            "google-client",
		ClientSecret:        "google-secret",
		RedirectURL:         "https://app.example/api/v1/auth/oauth/google/callback",
		FrontendRedirectURL: "/auth/oauth/callback",
	}, "/auth/oauth/callback", "/dashboard", &emailOAuthProfile{
		Subject:       subject,
		Email:         email,
		EmailVerified: true,
		Username:      "b01-totp-user",
	})
	return recorder
}

// 未绑定的 Google 身份按邮箱匹配到已启用 TOTP 的账号时，不得直接签发令牌或绑定身份。
func TestB01EmailOAuthCallbackExistingTotpUserIsNotAutoLoggedIn(t *testing.T) {
	handler, client := b01NewTotpEmailOAuthHandler(t)
	ctx := context.Background()
	b01CreateTotpUser(t, client, "b01-victim@example.com")

	recorder := b01RunGoogleCallback(handler, "b01-google-attacker", "b01-victim@example.com")

	require.Equal(t, http.StatusFound, recorder.Code)
	location := recorder.Header().Get("Location")
	require.NotContains(t, location, "access_token=")
	require.NotContains(t, location, "refresh_token=")
	require.Contains(t, location, "error=")

	identityCount, err := client.AuthIdentity.Query().Where(
		authidentity.ProviderTypeEQ("google"),
		authidentity.ProviderSubjectEQ("b01-google-attacker"),
	).Count(ctx)
	require.NoError(t, err)
	require.Zero(t, identityCount)
}

// 已绑定的身份继续按原逻辑登录（与其他 OAuth 提供方保持一致）。
func TestB01EmailOAuthCallbackBoundIdentityWithTotpStillLogsIn(t *testing.T) {
	handler, client := b01NewTotpEmailOAuthHandler(t)
	ctx := context.Background()
	user := b01CreateTotpUser(t, client, "b01-bound@example.com")

	_, err := client.AuthIdentity.Create().
		SetUserID(user.ID).
		SetProviderType("google").
		SetProviderKey("google").
		SetProviderSubject("b01-google-bound").
		SetMetadata(map[string]any{"email": "b01-bound@example.com"}).
		Save(ctx)
	require.NoError(t, err)

	recorder := b01RunGoogleCallback(handler, "b01-google-bound", "b01-bound@example.com")

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Contains(t, recorder.Header().Get("Location"), "access_token=")
}
