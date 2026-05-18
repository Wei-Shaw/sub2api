package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/ent/identityadoptiondecision"
	"github.com/Wei-Shaw/sub2api/ent/pendingauthsession"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/config"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSanitizeFrontendRedirectPath(t *testing.T) {
	require.Equal(t, "/dashboard", sanitizeFrontendRedirectPath("/dashboard"))
	require.Equal(t, "/dashboard", sanitizeFrontendRedirectPath(" /dashboard "))
	require.Equal(t, "", sanitizeFrontendRedirectPath("dashboard"))
	require.Equal(t, "", sanitizeFrontendRedirectPath("//evil.com"))
	require.Equal(t, "", sanitizeFrontendRedirectPath("https://evil.com"))
	require.Equal(t, "", sanitizeFrontendRedirectPath("/\nfoo"))

	long := "/" + strings.Repeat("a", linuxDoOAuthMaxRedirectLen)
	require.Equal(t, "", sanitizeFrontendRedirectPath(long))
}

func TestBuildBearerAuthorization(t *testing.T) {
	auth, err := buildBearerAuthorization("", "token123")
	require.NoError(t, err)
	require.Equal(t, "Bearer token123", auth)

	auth, err = buildBearerAuthorization("bearer", "token123")
	require.NoError(t, err)
	require.Equal(t, "Bearer token123", auth)

	_, err = buildBearerAuthorization("MAC", "token123")
	require.Error(t, err)

	_, err = buildBearerAuthorization("Bearer", "token 123")
	require.Error(t, err)
}

func TestLinuxDoParseUserInfoParsesIDAndUsername(t *testing.T) {
	cfg := config.LinuxDoConnectConfig{
		UserInfoURL: "https://connect.linux.do/api/user",
	}

	email, username, subject, displayName, avatarURL, err := linuxDoParseUserInfo(`{"id":123,"username":"alice","name":"Alice","avatar_url":"https://cdn.example/avatar.png"}`, cfg)
	require.NoError(t, err)
	require.Equal(t, "123", subject)
	require.Equal(t, "alice", username)
	require.Equal(t, "linuxdo-123@linuxdo-connect.invalid", email)
	require.Equal(t, "Alice", displayName)
	require.Equal(t, "https://cdn.example/avatar.png", avatarURL)
}

func TestLinuxDoParseUserInfoDefaultsUsername(t *testing.T) {
	cfg := config.LinuxDoConnectConfig{
		UserInfoURL: "https://connect.linux.do/api/user",
	}

	email, username, subject, displayName, avatarURL, err := linuxDoParseUserInfo(`{"id":"123"}`, cfg)
	require.NoError(t, err)
	require.Equal(t, "123", subject)
	require.Equal(t, "linuxdo_123", username)
	require.Equal(t, "linuxdo-123@linuxdo-connect.invalid", email)
	require.Equal(t, "linuxdo_123", displayName)
	require.Equal(t, "", avatarURL)
}

func TestLinuxDoParseUserInfoRejectsUnsafeSubject(t *testing.T) {
	cfg := config.LinuxDoConnectConfig{
		UserInfoURL: "https://connect.linux.do/api/user",
	}

	_, _, _, _, _, err := linuxDoParseUserInfo(`{"id":"123@456"}`, cfg)
	require.Error(t, err)

	tooLong := strings.Repeat("a", linuxDoOAuthMaxSubjectLen+1)
	_, _, _, _, _, err = linuxDoParseUserInfo(`{"id":"`+tooLong+`"}`, cfg)
	require.Error(t, err)
}

func TestParseOAuthProviderErrorJSON(t *testing.T) {
	code, desc := parseOAuthProviderError(`{"error":"invalid_client","error_description":"bad secret"}`)
	require.Equal(t, "invalid_client", code)
	require.Equal(t, "bad secret", desc)
}

func TestParseOAuthProviderErrorForm(t *testing.T) {
	code, desc := parseOAuthProviderError("error=invalid_request&error_description=Missing+code_verifier")
	require.Equal(t, "invalid_request", code)
	require.Equal(t, "Missing code_verifier", desc)
}

func TestParseLinuxDoTokenResponseJSON(t *testing.T) {
	token, ok := parseLinuxDoTokenResponse(`{"access_token":"t1","token_type":"Bearer","expires_in":3600,"scope":"user"}`)
	require.True(t, ok)
	require.Equal(t, "t1", token.AccessToken)
	require.Equal(t, "Bearer", token.TokenType)
	require.Equal(t, int64(3600), token.ExpiresIn)
	require.Equal(t, "user", token.Scope)
}

func TestParseLinuxDoTokenResponseForm(t *testing.T) {
	token, ok := parseLinuxDoTokenResponse("access_token=t2&token_type=bearer&expires_in=60")
	require.True(t, ok)
	require.Equal(t, "t2", token.AccessToken)
	require.Equal(t, "bearer", token.TokenType)
	require.Equal(t, int64(60), token.ExpiresIn)
}

func TestSingleLineStripsWhitespace(t *testing.T) {
	require.Equal(t, "hello world", singleLine("hello\r\nworld"))
	require.Equal(t, "", singleLine("\n\t\r"))
}

func TestLinuxDoOAuthBindStartRedirectsAndSetsBindCookies(t *testing.T) {
	handler := newLinuxDoOAuthTestHandler(t, false, config.LinuxDoConnectConfig{
		Enabled:             true,
		ClientID:            "linuxdo-client",
		ClientSecret:        "linuxdo-secret",
		AuthorizeURL:        "https://connect.linux.do/oauth/authorize",
		TokenURL:            "https://connect.linux.do/oauth/token",
		UserInfoURL:         "https://connect.linux.do/api/user",
		Scopes:              "read",
		RedirectURL:         "https://api.example.com/api/v1/auth/oauth/linuxdo/callback",
		FrontendRedirectURL: "/auth/linuxdo/callback",
		TokenAuthMethod:     "client_secret_post",
		UsePKCE:             true,
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/linuxdo/bind/start?intent=bind_current_user&redirect=/settings/connections", nil)
	c.Request = req
	c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 42})

	handler.LinuxDoOAuthStart(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	location := recorder.Header().Get("Location")
	require.Contains(t, location, "connect.linux.do/oauth/authorize")
	require.Contains(t, location, "client_id=linuxdo-client")
	require.Contains(t, location, "code_challenge=")

	cookies := recorder.Result().Cookies()
	require.NotNil(t, findCookie(cookies, linuxDoOAuthStateCookieName))
	require.NotNil(t, findCookie(cookies, linuxDoOAuthRedirectCookie))
	require.NotNil(t, findCookie(cookies, linuxDoOAuthVerifierCookie))
	require.NotNil(t, findCookie(cookies, oauthPendingBrowserCookieName))

	intentCookie := findCookie(cookies, linuxDoOAuthIntentCookieName)
	require.NotNil(t, intentCookie)
	require.Equal(t, oauthIntentBindCurrentUser, decodeCookieValueForTest(t, intentCookie.Value))

	bindCookie := findCookie(cookies, linuxDoOAuthBindUserCookieName)
	require.NotNil(t, bindCookie)
	userID, err := parseOAuthBindUserCookieValue(decodeCookieValueForTest(t, bindCookie.Value), "test-secret")
	require.NoError(t, err)
	require.Equal(t, int64(42), userID)
}

func TestLinuxDoOAuthStartOmitsPKCEWhenDisabled(t *testing.T) {
	handler := newLinuxDoOAuthTestHandler(t, false, config.LinuxDoConnectConfig{
		Enabled:             true,
		ClientID:            "linuxdo-client",
		ClientSecret:        "linuxdo-secret",
		AuthorizeURL:        "https://connect.linux.do/oauth/authorize",
		TokenURL:            "https://connect.linux.do/oauth/token",
		UserInfoURL:         "https://connect.linux.do/api/user",
		Scopes:              "read",
		RedirectURL:         "https://api.example.com/api/v1/auth/oauth/linuxdo/callback",
		FrontendRedirectURL: "/auth/linuxdo/callback",
		TokenAuthMethod:     "client_secret_post",
		UsePKCE:             false,
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/linuxdo/start?redirect=/dashboard", nil)

	handler.LinuxDoOAuthStart(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.NotContains(t, recorder.Header().Get("Location"), "code_challenge=")
	require.Nil(t, findCookie(recorder.Result().Cookies(), linuxDoOAuthVerifierCookie))
}

func TestLinuxDoOAuthCallbackAllowsMissingVerifierWhenPKCEDisabled(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			require.NoError(t, r.ParseForm())
			require.Empty(t, r.PostForm.Get("code_verifier"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"linuxdo-access","token_type":"Bearer","expires_in":3600}`))
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"compat-subject","username":"linuxdo_user","name":"LinuxDo Display"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	handler, client := newLinuxDoOAuthHandlerAndClient(t, false, config.LinuxDoConnectConfig{
		Enabled:             true,
		ClientID:            "linuxdo-client",
		ClientSecret:        "linuxdo-secret",
		AuthorizeURL:        upstream.URL + "/authorize",
		TokenURL:            upstream.URL + "/token",
		UserInfoURL:         upstream.URL + "/userinfo",
		Scopes:              "read",
		RedirectURL:         "https://api.example.com/api/v1/auth/oauth/linuxdo/callback",
		FrontendRedirectURL: "/auth/linuxdo/callback",
		TokenAuthMethod:     "client_secret_post",
		UsePKCE:             false,
	})
	t.Cleanup(func() { _ = client.Close() })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/linuxdo/callback?code=linuxdo-code&state=state-123", nil)
	req.AddCookie(encodedCookie(linuxDoOAuthStateCookieName, "state-123"))
	req.AddCookie(encodedCookie(linuxDoOAuthRedirectCookie, "/dashboard"))
	req.AddCookie(encodedCookie(linuxDoOAuthIntentCookieName, oauthIntentLogin))
	req.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "browser-123"))
	c.Request = req

	handler.LinuxDoOAuthCallback(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "/auth/linuxdo/callback", recorder.Header().Get("Location"))
	require.NotNil(t, findCookie(recorder.Result().Cookies(), oauthPendingSessionCookieName))
}

func TestLinuxDoOAuthBindStartAcceptsAccessTokenCookie(t *testing.T) {
	handler, client := newLinuxDoOAuthHandlerAndClient(t, false, config.LinuxDoConnectConfig{
		Enabled:             true,
		ClientID:            "linuxdo-client",
		ClientSecret:        "linuxdo-secret",
		AuthorizeURL:        "https://connect.linux.do/oauth/authorize",
		TokenURL:            "https://connect.linux.do/oauth/token",
		UserInfoURL:         "https://connect.linux.do/api/user",
		Scopes:              "read",
		RedirectURL:         "https://api.example.com/api/v1/auth/oauth/linuxdo/callback",
		FrontendRedirectURL: "/auth/linuxdo/callback",
		TokenAuthMethod:     "client_secret_post",
		UsePKCE:             true,
	})
	t.Cleanup(func() { _ = client.Close() })

	user, err := client.User.Create().
		SetEmail("bind-cookie@example.com").
		SetUsername("bind-cookie-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(context.Background())
	require.NoError(t, err)

	token, err := handler.authService.GenerateToken(&service.User{
		ID:           user.ID,
		Email:        user.Email,
		Username:     user.Username,
		PasswordHash: user.PasswordHash,
		Role:         user.Role,
		Status:       user.Status,
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/linuxdo/start?intent=bind_current_user&redirect=/settings/connections", nil)
	req.AddCookie(&http.Cookie{Name: oauthBindAccessTokenCookieName, Value: token, Path: oauthBindAccessTokenCookiePath})
	c.Request = req

	handler.LinuxDoOAuthStart(c)

	require.Equal(t, http.StatusFound, recorder.Code)

	bindCookie := findCookie(recorder.Result().Cookies(), linuxDoOAuthBindUserCookieName)
	require.NotNil(t, bindCookie)
	userID, err := parseOAuthBindUserCookieValue(decodeCookieValueForTest(t, bindCookie.Value), "test-secret")
	require.NoError(t, err)
	require.Equal(t, user.ID, userID)

	accessTokenCookie := findCookie(recorder.Result().Cookies(), oauthBindAccessTokenCookieName)
	require.NotNil(t, accessTokenCookie)
	require.Equal(t, -1, accessTokenCookie.MaxAge)
}

func TestPrepareOAuthBindAccessTokenCookieSetsHttpOnlyCookie(t *testing.T) {
	handler, client := newLinuxDoOAuthHandlerAndClient(t, false, config.LinuxDoConnectConfig{})
	t.Cleanup(func() { _ = client.Close() })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/bind-token", nil)
	req.Header.Set("Authorization", "Bearer access-token-value")
	c.Request = req

	handler.PrepareOAuthBindAccessTokenCookie(c)

	require.Equal(t, http.StatusNoContent, recorder.Code)
	accessTokenCookie := findCookie(recorder.Result().Cookies(), oauthBindAccessTokenCookieName)
	require.NotNil(t, accessTokenCookie)
	require.Equal(t, oauthBindAccessTokenCookiePath, accessTokenCookie.Path)
	require.Equal(t, linuxDoOAuthCookieMaxAgeSec, accessTokenCookie.MaxAge)
	require.True(t, accessTokenCookie.HttpOnly)
	require.Equal(t, url.QueryEscape("access-token-value"), accessTokenCookie.Value)
}

func TestLinuxDoOAuthCallbackCreatesLoginPendingSessionForExistingIdentityUser(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"linuxdo-access","token_type":"Bearer","expires_in":3600}`))
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"321","username":"linuxdo_user","name":"LinuxDo Display","avatar_url":"https://cdn.example/linuxdo.png"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	handler, client := newLinuxDoOAuthHandlerAndClient(t, false, config.LinuxDoConnectConfig{
		Enabled:             true,
		ClientID:            "linuxdo-client",
		ClientSecret:        "linuxdo-secret",
		AuthorizeURL:        upstream.URL + "/authorize",
		TokenURL:            upstream.URL + "/token",
		UserInfoURL:         upstream.URL + "/userinfo",
		Scopes:              "read",
		RedirectURL:         "https://api.example.com/api/v1/auth/oauth/linuxdo/callback",
		FrontendRedirectURL: "/auth/linuxdo/callback",
		TokenAuthMethod:     "client_secret_post",
		UsePKCE:             true,
	})
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	existingUser, err := client.User.Create().
		SetEmail(linuxDoSyntheticEmail("321")).
		SetUsername("legacy-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.AuthIdentity.Create().
		SetUserID(existingUser.ID).
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("321").
		SetMetadata(map[string]any{"username": "legacy-user"}).
		Save(ctx)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/linuxdo/callback?code=code-123&state=state-123", nil)
	req.AddCookie(encodedCookie(linuxDoOAuthStateCookieName, "state-123"))
	req.AddCookie(encodedCookie(linuxDoOAuthRedirectCookie, "/dashboard"))
	req.AddCookie(encodedCookie(linuxDoOAuthVerifierCookie, "verifier-123"))
	req.AddCookie(encodedCookie(linuxDoOAuthIntentCookieName, oauthIntentLogin))
	req.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "browser-123"))
	c.Request = req

	handler.LinuxDoOAuthCallback(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "/auth/linuxdo/callback", recorder.Header().Get("Location"))

	sessionCookie := findCookie(recorder.Result().Cookies(), oauthPendingSessionCookieName)
	require.NotNil(t, sessionCookie)

	session, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.SessionTokenEQ(decodeCookieValueForTest(t, sessionCookie.Value))).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, oauthIntentLogin, session.Intent)
	require.NotNil(t, session.TargetUserID)
	require.Equal(t, existingUser.ID, *session.TargetUserID)
	require.Equal(t, linuxDoSyntheticEmail("321"), session.ResolvedEmail)
	require.Equal(t, "LinuxDo Display", session.UpstreamIdentityClaims["suggested_display_name"])

	completion, ok := session.LocalFlowState[oauthCompletionResponseKey].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "/dashboard", completion["redirect"])
	_, hasAccessToken := completion["access_token"]
	require.False(t, hasAccessToken)
	_, hasRefreshToken := completion["refresh_token"]
	require.False(t, hasRefreshToken)
	require.Nil(t, completion["error"])
}

func TestLinuxDoOAuthCallbackRejectsDisabledExistingIdentityUser(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"linuxdo-access","token_type":"Bearer","expires_in":3600}`))
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"654","username":"linuxdo_disabled","name":"LinuxDo Disabled"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	handler, client := newLinuxDoOAuthHandlerAndClient(t, false, config.LinuxDoConnectConfig{
		Enabled:             true,
		ClientID:            "linuxdo-client",
		ClientSecret:        "linuxdo-secret",
		AuthorizeURL:        upstream.URL + "/authorize",
		TokenURL:            upstream.URL + "/token",
		UserInfoURL:         upstream.URL + "/userinfo",
		Scopes:              "read",
		RedirectURL:         "https://api.example.com/api/v1/auth/oauth/linuxdo/callback",
		FrontendRedirectURL: "/auth/linuxdo/callback",
		TokenAuthMethod:     "client_secret_post",
		UsePKCE:             true,
	})
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	existingUser, err := client.User.Create().
		SetEmail(linuxDoSyntheticEmail("654")).
		SetUsername("disabled-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusDisabled).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.AuthIdentity.Create().
		SetUserID(existingUser.ID).
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("654").
		Save(ctx)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/linuxdo/callback?code=code-disabled&state=state-disabled", nil)
	req.AddCookie(encodedCookie(linuxDoOAuthStateCookieName, "state-disabled"))
	req.AddCookie(encodedCookie(linuxDoOAuthRedirectCookie, "/dashboard"))
	req.AddCookie(encodedCookie(linuxDoOAuthVerifierCookie, "verifier-disabled"))
	req.AddCookie(encodedCookie(linuxDoOAuthIntentCookieName, oauthIntentLogin))
	req.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "browser-disabled"))
	c.Request = req

	handler.LinuxDoOAuthCallback(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Nil(t, findCookie(recorder.Result().Cookies(), oauthPendingSessionCookieName))
	assertOAuthRedirectError(t, recorder.Header().Get("Location"), "session_error", "USER_NOT_ACTIVE")

	count, err := client.PendingAuthSession.Query().Count(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestLinuxDoOAuthCallbackCreatesBindPendingSessionForCompatEmailUser(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"linuxdo-access","token_type":"Bearer","expires_in":3600}`))
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"321","email":"legacy@example.com","username":"linuxdo_user","name":"LinuxDo Display","avatar_url":"https://cdn.example/linuxdo.png"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	handler, client := newLinuxDoOAuthHandlerAndClient(t, false, config.LinuxDoConnectConfig{
		Enabled:             true,
		ClientID:            "linuxdo-client",
		ClientSecret:        "linuxdo-secret",
		AuthorizeURL:        upstream.URL + "/authorize",
		TokenURL:            upstream.URL + "/token",
		UserInfoURL:         upstream.URL + "/userinfo",
		Scopes:              "read",
		RedirectURL:         "https://api.example.com/api/v1/auth/oauth/linuxdo/callback",
		FrontendRedirectURL: "/auth/linuxdo/callback",
		TokenAuthMethod:     "client_secret_post",
		UsePKCE:             true,
	})
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	existingUser, err := client.User.Create().
		SetEmail(" Legacy@Example.com ").
		SetUsername("legacy-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/linuxdo/callback?code=code-compat&state=state-compat", nil)
	req.AddCookie(encodedCookie(linuxDoOAuthStateCookieName, "state-compat"))
	req.AddCookie(encodedCookie(linuxDoOAuthRedirectCookie, "/dashboard"))
	req.AddCookie(encodedCookie(linuxDoOAuthVerifierCookie, "verifier-compat"))
	req.AddCookie(encodedCookie(linuxDoOAuthIntentCookieName, oauthIntentLogin))
	req.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "browser-compat"))
	c.Request = req

	handler.LinuxDoOAuthCallback(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "/auth/linuxdo/callback", recorder.Header().Get("Location"))

	sessionCookie := findCookie(recorder.Result().Cookies(), oauthPendingSessionCookieName)
	require.NotNil(t, sessionCookie)

	session, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.SessionTokenEQ(decodeCookieValueForTest(t, sessionCookie.Value))).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, oauthIntentLogin, session.Intent)
	require.NotNil(t, session.TargetUserID)
	require.Equal(t, existingUser.ID, *session.TargetUserID)
	require.Equal(t, strings.TrimSpace(existingUser.Email), session.ResolvedEmail)
	require.Equal(t, "legacy@example.com", session.UpstreamIdentityClaims["compat_email"])

	completion, ok := session.LocalFlowState[oauthCompletionResponseKey].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "/dashboard", completion["redirect"])
	require.Equal(t, oauthPendingChoiceStep, completion["step"])
	require.Equal(t, strings.TrimSpace(existingUser.Email), completion["email"])
	require.Equal(t, strings.TrimSpace(existingUser.Email), completion["existing_account_email"])
	require.Equal(t, true, completion["existing_account_bindable"])
	require.Equal(t, "compat_email_match", completion["choice_reason"])
	_, hasAccessToken := completion["access_token"]
	require.False(t, hasAccessToken)
}

func TestLinuxDoOAuthCallbackCreatesInvitationPendingSessionWhenEmailVerificationIsDisabled(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"linuxdo-access","token_type":"Bearer","expires_in":3600}`))
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"654","username":"linuxdo_invite","name":"Need Invite","avatar_url":"https://cdn.example/invite.png"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	handler, client := newLinuxDoOAuthHandlerAndClient(t, true, config.LinuxDoConnectConfig{
		Enabled:             true,
		ClientID:            "linuxdo-client",
		ClientSecret:        "linuxdo-secret",
		AuthorizeURL:        upstream.URL + "/authorize",
		TokenURL:            upstream.URL + "/token",
		UserInfoURL:         upstream.URL + "/userinfo",
		Scopes:              "read",
		RedirectURL:         "https://api.example.com/api/v1/auth/oauth/linuxdo/callback",
		FrontendRedirectURL: "/auth/linuxdo/callback",
		TokenAuthMethod:     "client_secret_post",
		UsePKCE:             true,
	})
	t.Cleanup(func() { _ = client.Close() })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/linuxdo/callback?code=code-456&state=state-456", nil)
	req.AddCookie(encodedCookie(linuxDoOAuthStateCookieName, "state-456"))
	req.AddCookie(encodedCookie(linuxDoOAuthRedirectCookie, "/dashboard"))
	req.AddCookie(encodedCookie(linuxDoOAuthVerifierCookie, "verifier-456"))
	req.AddCookie(encodedCookie(linuxDoOAuthIntentCookieName, oauthIntentLogin))
	req.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "browser-456"))
	c.Request = req

	handler.LinuxDoOAuthCallback(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "/auth/linuxdo/callback", recorder.Header().Get("Location"))

	sessionCookie := findCookie(recorder.Result().Cookies(), oauthPendingSessionCookieName)
	require.NotNil(t, sessionCookie)

	ctx := context.Background()
	session, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.SessionTokenEQ(decodeCookieValueForTest(t, sessionCookie.Value))).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, oauthIntentLogin, session.Intent)
	require.Nil(t, session.TargetUserID)

	completion, ok := session.LocalFlowState[oauthCompletionResponseKey].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "/dashboard", completion["redirect"])
	require.Equal(t, "invitation_required", completion["error"])
	require.Equal(t, "invitation_required", completion["choice_reason"])
}

func TestLinuxDoOAuthCallbackCreatesAutoRegisterPendingSessionWhenEmailVerificationIsDisabled(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"linuxdo-access","token_type":"Bearer","expires_in":3600}`))
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"777","username":"linuxdo_autoreg","name":"Auto Register","avatar_url":"https://cdn.example/autoreg.png"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	handler, client := newLinuxDoOAuthHandlerAndClient(t, false, config.LinuxDoConnectConfig{
		Enabled:             true,
		ClientID:            "linuxdo-client",
		ClientSecret:        "linuxdo-secret",
		AuthorizeURL:        upstream.URL + "/authorize",
		TokenURL:            upstream.URL + "/token",
		UserInfoURL:         upstream.URL + "/userinfo",
		Scopes:              "read",
		RedirectURL:         "https://api.example.com/api/v1/auth/oauth/linuxdo/callback",
		FrontendRedirectURL: "/auth/linuxdo/callback",
		TokenAuthMethod:     "client_secret_post",
		UsePKCE:             true,
	})
	t.Cleanup(func() { _ = client.Close() })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/linuxdo/callback?code=code-777&state=state-777", nil)
	req.AddCookie(encodedCookie(linuxDoOAuthStateCookieName, "state-777"))
	req.AddCookie(encodedCookie(linuxDoOAuthRedirectCookie, "/dashboard"))
	req.AddCookie(encodedCookie(linuxDoOAuthVerifierCookie, "verifier-777"))
	req.AddCookie(encodedCookie(linuxDoOAuthIntentCookieName, oauthIntentLogin))
	req.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "browser-777"))
	c.Request = req

	handler.LinuxDoOAuthCallback(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "/auth/linuxdo/callback", recorder.Header().Get("Location"))

	sessionCookie := findCookie(recorder.Result().Cookies(), oauthPendingSessionCookieName)
	require.NotNil(t, sessionCookie)

	ctx := context.Background()
	session, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.SessionTokenEQ(decodeCookieValueForTest(t, sessionCookie.Value))).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, oauthIntentLogin, session.Intent)
	require.Nil(t, session.TargetUserID)
	require.Equal(t, linuxDoSyntheticEmail("777"), session.ResolvedEmail)

	completion, ok := session.LocalFlowState[oauthCompletionResponseKey].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "/dashboard", completion["redirect"])
	require.Equal(t, true, completion["auto_register"])
	_, hasStep := completion["step"]
	require.False(t, hasStep)
}

func TestLinuxDoOAuthCallbackCreatesBindPendingSessionForCurrentUser(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"linuxdo-access","token_type":"Bearer","expires_in":3600}`))
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"999","username":"bind_user","name":"Bind Display","avatar_url":"https://cdn.example/bind.png"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	handler, client := newLinuxDoOAuthHandlerAndClient(t, false, config.LinuxDoConnectConfig{
		Enabled:             true,
		ClientID:            "linuxdo-client",
		ClientSecret:        "linuxdo-secret",
		AuthorizeURL:        upstream.URL + "/authorize",
		TokenURL:            upstream.URL + "/token",
		UserInfoURL:         upstream.URL + "/userinfo",
		Scopes:              "read",
		RedirectURL:         "https://api.example.com/api/v1/auth/oauth/linuxdo/callback",
		FrontendRedirectURL: "/auth/linuxdo/callback",
		TokenAuthMethod:     "client_secret_post",
		UsePKCE:             true,
	})
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	currentUser, err := client.User.Create().
		SetEmail("current@example.com").
		SetUsername("current-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/linuxdo/callback?code=code-bind&state=state-bind", nil)
	req.AddCookie(encodedCookie(linuxDoOAuthStateCookieName, "state-bind"))
	req.AddCookie(encodedCookie(linuxDoOAuthRedirectCookie, "/settings/connections"))
	req.AddCookie(encodedCookie(linuxDoOAuthVerifierCookie, "verifier-bind"))
	req.AddCookie(encodedCookie(linuxDoOAuthIntentCookieName, oauthIntentBindCurrentUser))
	req.AddCookie(encodedCookie(linuxDoOAuthBindUserCookieName, buildEncodedOAuthBindUserCookie(t, currentUser.ID, "test-secret")))
	req.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "browser-bind"))
	c.Request = req

	handler.LinuxDoOAuthCallback(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "/auth/linuxdo/callback", recorder.Header().Get("Location"))

	sessionCookie := findCookie(recorder.Result().Cookies(), oauthPendingSessionCookieName)
	require.NotNil(t, sessionCookie)

	session, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.SessionTokenEQ(decodeCookieValueForTest(t, sessionCookie.Value))).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, oauthIntentBindCurrentUser, session.Intent)
	require.NotNil(t, session.TargetUserID)
	require.Equal(t, currentUser.ID, *session.TargetUserID)
	require.Equal(t, linuxDoSyntheticEmail("999"), session.ResolvedEmail)

	completion, ok := session.LocalFlowState[oauthCompletionResponseKey].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "/settings/connections", completion["redirect"])
	require.Empty(t, completion["access_token"])
	require.Equal(t, "Bind Display", session.UpstreamIdentityClaims["suggested_display_name"])

	userCount, err := client.User.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, userCount)
}

func TestCompleteLinuxDoOAuthRegistrationAppliesPendingAdoptionDecision(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("linuxdo-complete-session").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("linuxdo-subject-1").
		SetResolvedEmail("linuxdo-subject-1@linuxdo-connect.invalid").
		SetBrowserSessionKey("linuxdo-browser").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "linuxdo_user",
			"suggested_display_name": "LinuxDo Display",
			"suggested_avatar_url":   "https://cdn.example/linuxdo.png",
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	_, err = service.NewAuthPendingIdentityService(client).UpsertAdoptionDecision(ctx, service.PendingIdentityAdoptionDecisionInput{
		PendingAuthSessionID: session.ID,
		AdoptAvatar:          true,
	})
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"invitation_code":"invite-1","adopt_display_name":true}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/linuxdo/complete-registration", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("linuxdo-browser")})
	c.Request = req

	handler.CompleteLinuxDoOAuthRegistration(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	responseData := decodeJSONBody(t, recorder)
	require.NotEmpty(t, responseData["access_token"])

	userEntity, err := client.User.Query().
		Where(dbuser.EmailEQ(session.ResolvedEmail)).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, "LinuxDo Display", userEntity.Username)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("linuxdo"),
			authidentity.ProviderKeyEQ("linuxdo"),
			authidentity.ProviderSubjectEQ("linuxdo-subject-1"),
		).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, userEntity.ID, identity.UserID)
	require.Equal(t, "LinuxDo Display", identity.Metadata["display_name"])
	require.Equal(t, "https://cdn.example/linuxdo.png", identity.Metadata["avatar_url"])

	decision, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, decision.IdentityID)
	require.Equal(t, identity.ID, *decision.IdentityID)
	require.True(t, decision.AdoptDisplayName)
	require.True(t, decision.AdoptAvatar)

	consumed, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, consumed.ConsumedAt)
}

func TestCompleteLinuxDoOAuthRegistrationRejectsAdoptExistingUserSession(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	existingUser, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("linuxdo-complete-invalid-session").
		SetIntent("adopt_existing_user_by_email").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("linuxdo-invalid-subject-1").
		SetTargetUserID(existingUser.ID).
		SetResolvedEmail(existingUser.Email).
		SetBrowserSessionKey("linuxdo-invalid-browser").
		SetUpstreamIdentityClaims(map[string]any{
			"username": "linuxdo_user",
		}).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"step": "bind_login_required",
			},
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"invitation_code":"invite-1"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/linuxdo/complete-registration", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("linuxdo-invalid-browser")})
	c.Request = req

	handler.CompleteLinuxDoOAuthRegistration(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, storedSession.ConsumedAt)
}

func TestCompleteLinuxDoOAuthRegistrationReturnsPendingSessionWhenChoiceStillRequired(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("linuxdo-complete-choice-session").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("linuxdo-choice-subject-1").
		SetResolvedEmail("linuxdo-choice-subject-1@linuxdo-connect.invalid").
		SetBrowserSessionKey("linuxdo-choice-browser").
		SetUpstreamIdentityClaims(map[string]any{
			"username": "linuxdo_user",
		}).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"step":                  oauthPendingChoiceStep,
				"redirect":              "/dashboard",
				"email":                 "fresh@example.com",
				"resolved_email":        "fresh@example.com",
				"force_email_on_signup": true,
			},
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"invitation_code":"invite-1"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/linuxdo/complete-registration", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("linuxdo-choice-browser")})
	c.Request = req

	handler.CompleteLinuxDoOAuthRegistration(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	responseData := decodeJSONBody(t, recorder)
	require.Equal(t, "pending_session", responseData["auth_result"])
	require.Equal(t, oauthPendingChoiceStep, responseData["step"])
	require.Equal(t, true, responseData["force_email_on_signup"])
	require.Empty(t, responseData["access_token"])

	userCount, err := client.User.Query().Count(ctx)
	require.NoError(t, err)
	require.Zero(t, userCount)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, storedSession.ConsumedAt)
}

func TestCompleteLinuxDoOAuthRegistrationBindsIdentityWithoutAdoptionFlags(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("linuxdo-complete-no-adoption-session").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("linuxdo-subject-no-adoption").
		SetResolvedEmail("linuxdo-subject-no-adoption@linuxdo-connect.invalid").
		SetBrowserSessionKey("linuxdo-browser-no-adoption").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "linuxdo_user",
			"suggested_display_name": "LinuxDo Legacy",
			"suggested_avatar_url":   "https://cdn.example/linuxdo-legacy.png",
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"invitation_code":"invite-1"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/linuxdo/complete-registration", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("linuxdo-browser-no-adoption")})
	c.Request = req

	handler.CompleteLinuxDoOAuthRegistration(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	responseData := decodeJSONBody(t, recorder)
	require.NotEmpty(t, responseData["access_token"])
	require.NotEmpty(t, responseData["refresh_token"])

	userEntity, err := client.User.Query().
		Where(dbuser.EmailEQ(session.ResolvedEmail)).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, "linuxdo_user", userEntity.Username)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("linuxdo"),
			authidentity.ProviderKeyEQ("linuxdo"),
			authidentity.ProviderSubjectEQ("linuxdo-subject-no-adoption"),
		).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, userEntity.ID, identity.UserID)

	decision, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, decision.IdentityID)
	require.Equal(t, identity.ID, *decision.IdentityID)
	require.False(t, decision.AdoptDisplayName)
	require.False(t, decision.AdoptAvatar)
}

func TestCompleteLinuxDoOAuthRegistrationRejectsIdentityOwnershipConflictBeforeUserCreation(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	existingOwner, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.AuthIdentity.Create().
		SetUserID(existingOwner.ID).
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("linuxdo-conflict-subject").
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("linuxdo-complete-conflict-session").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("linuxdo-conflict-subject").
		SetResolvedEmail("linuxdo-conflict-subject@linuxdo-connect.invalid").
		SetBrowserSessionKey("linuxdo-conflict-browser").
		SetUpstreamIdentityClaims(map[string]any{
			"username": "linuxdo_user",
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"invitation_code":"invite-1"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/linuxdo/complete-registration", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("linuxdo-conflict-browser")})
	c.Request = req

	handler.CompleteLinuxDoOAuthRegistration(c)

	require.Equal(t, http.StatusConflict, recorder.Code)
	payload := decodeJSONBody(t, recorder)
	require.Equal(t, "AUTH_IDENTITY_OWNERSHIP_CONFLICT", payload["reason"])

	userCount, err := client.User.Query().
		Where(dbuser.EmailEQ("linuxdo-conflict-subject@linuxdo-connect.invalid")).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, userCount)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, storedSession.ConsumedAt)
}

func newLinuxDoOAuthTestHandler(t *testing.T, invitationEnabled bool, oauthCfg config.LinuxDoConnectConfig) *AuthHandler {
	t.Helper()
	handler, _ := newLinuxDoOAuthHandlerAndClient(t, invitationEnabled, oauthCfg)
	return handler
}

func newLinuxDoOAuthHandlerAndClient(t *testing.T, invitationEnabled bool, oauthCfg config.LinuxDoConnectConfig) (*AuthHandler, *dbent.Client) {
	t.Helper()
	handler, client := newOAuthPendingFlowTestHandler(t, invitationEnabled)
	handler.settingSvc = nil
	handler.cfg = &config.Config{
		JWT: config.JWTConfig{
			Secret:                   "test-secret",
			ExpireHour:               1,
			AccessTokenExpireMinutes: 60,
			RefreshTokenExpireDays:   7,
		},
		LinuxDo: oauthCfg,
	}
	return handler, client
}
