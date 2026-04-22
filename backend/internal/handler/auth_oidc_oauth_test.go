package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
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
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestOIDCSyntheticEmailStableAndDistinct(t *testing.T) {
	k1 := oidcIdentityKey("https://issuer.example.com", "subject-a")
	k2 := oidcIdentityKey("https://issuer.example.com", "subject-b")

	e1 := oidcSyntheticEmailFromIdentityKey(k1)
	e1Again := oidcSyntheticEmailFromIdentityKey(k1)
	e2 := oidcSyntheticEmailFromIdentityKey(k2)

	require.Equal(t, e1, e1Again)
	require.NotEqual(t, e1, e2)
	require.Contains(t, e1, "@oidc-connect.invalid")
}

func TestBuildOIDCAuthorizeURLIncludesNonceAndPKCE(t *testing.T) {
	cfg := config.OIDCConnectConfig{
		AuthorizeURL: "https://issuer.example.com/auth",
		ClientID:     "cid",
		Scopes:       "openid email profile",
	}

	u, err := buildOIDCAuthorizeURL(cfg, "state123", "nonce123", "challenge123", "https://app.example.com/callback")
	require.NoError(t, err)
	require.Contains(t, u, "nonce=nonce123")
	require.Contains(t, u, "code_challenge=challenge123")
	require.Contains(t, u, "code_challenge_method=S256")
	require.Contains(t, u, "scope=openid+email+profile")
}

func TestOIDCParseAndValidateIDToken(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	kid := "kid-1"
	jwks := oidcJWKSet{Keys: []oidcJWK{buildRSAJWK(kid, &priv.PublicKey)}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(jwks))
	}))
	defer srv.Close()

	now := time.Now()
	claims := oidcIDTokenClaims{
		Nonce: "nonce-ok",
		Azp:   "client-1",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "https://issuer.example.com",
			Subject:   "subject-1",
			Audience:  jwt.ClaimStrings{"client-1", "another-aud"},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(priv)
	require.NoError(t, err)

	cfg := config.OIDCConnectConfig{
		ClientID:           "client-1",
		IssuerURL:          "https://issuer.example.com",
		JWKSURL:            srv.URL,
		AllowedSigningAlgs: "RS256",
		ClockSkewSeconds:   120,
	}

	parsed, err := oidcParseAndValidateIDToken(context.Background(), cfg, signed, "nonce-ok")
	require.NoError(t, err)
	require.Equal(t, "subject-1", parsed.Subject)
	require.Equal(t, "https://issuer.example.com", parsed.Issuer)

	_, err = oidcParseAndValidateIDToken(context.Background(), cfg, signed, "bad-nonce")
	require.Error(t, err)
}

func TestOIDCParseUserInfoIncludesSuggestedProfile(t *testing.T) {
	cfg := config.OIDCConnectConfig{}

	claims := oidcParseUserInfo(`{
		"sub":"subject-1",
		"preferred_username":"alice",
		"name":"Alice Example",
		"picture":"https://cdn.example/avatar.png",
		"email":"alice@example.com",
		"email_verified":true
	}`, cfg)

	require.Equal(t, "subject-1", claims.Subject)
	require.Equal(t, "alice", claims.Username)
	require.Equal(t, "Alice Example", claims.DisplayName)
	require.Equal(t, "https://cdn.example/avatar.png", claims.AvatarURL)
	require.NotNil(t, claims.EmailVerified)
	require.True(t, *claims.EmailVerified)
}

func buildRSAJWK(kid string, pub *rsa.PublicKey) oidcJWK {
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())
	return oidcJWK{
		Kty: "RSA",
		Kid: kid,
		Use: "sig",
		Alg: "RS256",
		N:   n,
		E:   e,
	}
}

func TestOIDCOAuthBindStartRedirectsAndSetsBindCookies(t *testing.T) {
	handler := newOIDCOAuthTestHandler(t, false, config.OIDCConnectConfig{
		Enabled:              true,
		ClientID:             "oidc-client",
		ClientSecret:         "oidc-secret",
		IssuerURL:            "https://issuer.example.com",
		AuthorizeURL:         "https://issuer.example.com/oauth/authorize",
		TokenURL:             "https://issuer.example.com/oauth/token",
		UserInfoURL:          "https://issuer.example.com/oauth/userinfo",
		JWKSURL:              "https://issuer.example.com/oauth/jwks",
		Scopes:               "openid profile email",
		RedirectURL:          "https://api.example.com/api/v1/auth/oauth/oidc/callback",
		FrontendRedirectURL:  "/auth/oidc/callback",
		TokenAuthMethod:      "client_secret_post",
		UsePKCE:              true,
		ValidateIDToken:      true,
		AllowedSigningAlgs:   "RS256",
		ClockSkewSeconds:     120,
		RequireEmailVerified: false,
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/bind/start?intent=bind_current_user&redirect=/settings/connections", nil)
	c.Request = req
	c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 84})

	handler.OIDCOAuthStart(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	location := recorder.Header().Get("Location")
	require.Contains(t, location, "issuer.example.com/oauth/authorize")
	require.Contains(t, location, "client_id=oidc-client")
	require.Contains(t, location, "nonce=")

	cookies := recorder.Result().Cookies()
	require.NotNil(t, findCookie(cookies, oidcOAuthStateCookieName))
	require.NotNil(t, findCookie(cookies, oidcOAuthRedirectCookie))
	require.NotNil(t, findCookie(cookies, oidcOAuthVerifierCookie))
	require.NotNil(t, findCookie(cookies, oidcOAuthNonceCookie))
	require.NotNil(t, findCookie(cookies, oauthPendingBrowserCookieName))

	intentCookie := findCookie(cookies, oidcOAuthIntentCookieName)
	require.NotNil(t, intentCookie)
	require.Equal(t, oauthIntentBindCurrentUser, decodeCookieValueForTest(t, intentCookie.Value))

	bindCookie := findCookie(cookies, oidcOAuthBindUserCookieName)
	require.NotNil(t, bindCookie)
	userID, err := parseOAuthBindUserCookieValue(decodeCookieValueForTest(t, bindCookie.Value), "test-secret")
	require.NoError(t, err)
	require.Equal(t, int64(84), userID)
}

func TestOIDCOAuthStartOmitsPKCEAndNonceWhenDisabled(t *testing.T) {
	handler := newOIDCOAuthTestHandler(t, false, config.OIDCConnectConfig{
		Enabled:              true,
		ClientID:             "oidc-client",
		ClientSecret:         "oidc-secret",
		IssuerURL:            "https://issuer.example.com",
		AuthorizeURL:         "https://issuer.example.com/oauth/authorize",
		TokenURL:             "https://issuer.example.com/oauth/token",
		UserInfoURL:          "https://issuer.example.com/oauth/userinfo",
		Scopes:               "openid profile email",
		RedirectURL:          "https://api.example.com/api/v1/auth/oauth/oidc/callback",
		FrontendRedirectURL:  "/auth/oidc/callback",
		TokenAuthMethod:      "client_secret_post",
		UsePKCE:              false,
		ValidateIDToken:      false,
		RequireEmailVerified: false,
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/start?redirect=/dashboard", nil)

	handler.OIDCOAuthStart(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	location := recorder.Header().Get("Location")
	require.NotContains(t, location, "code_challenge=")
	require.NotContains(t, location, "nonce=")
	require.Nil(t, findCookie(recorder.Result().Cookies(), oidcOAuthVerifierCookie))
	require.Nil(t, findCookie(recorder.Result().Cookies(), oidcOAuthNonceCookie))
}

func TestOIDCOAuthCallbackAllowsOptionalPKCEAndIDTokenValidation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			require.NoError(t, r.ParseForm())
			require.Empty(t, r.PostForm.Get("code_verifier"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"oidc-access","token_type":"Bearer","expires_in":3600}`))
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sub":"oidc-subject-compat","preferred_username":"oidc_user","name":"OIDC Display","email":"oidc@example.com"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	handler, client := newOIDCOAuthHandlerAndClient(t, false, config.OIDCConnectConfig{
		Enabled:              true,
		ClientID:             "oidc-client",
		ClientSecret:         "oidc-secret",
		IssuerURL:            "https://issuer.example.com",
		AuthorizeURL:         upstream.URL + "/authorize",
		TokenURL:             upstream.URL + "/token",
		UserInfoURL:          upstream.URL + "/userinfo",
		Scopes:               "openid profile email",
		RedirectURL:          "https://api.example.com/api/v1/auth/oauth/oidc/callback",
		FrontendRedirectURL:  "/auth/oidc/callback",
		TokenAuthMethod:      "client_secret_post",
		UsePKCE:              false,
		ValidateIDToken:      false,
		RequireEmailVerified: false,
	})
	t.Cleanup(func() { _ = client.Close() })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/callback?code=oidc-code&state=state-123", nil)
	req.AddCookie(encodedCookie(oidcOAuthStateCookieName, "state-123"))
	req.AddCookie(encodedCookie(oidcOAuthRedirectCookie, "/dashboard"))
	req.AddCookie(encodedCookie(oidcOAuthIntentCookieName, oauthIntentLogin))
	req.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "browser-123"))
	c.Request = req

	handler.OIDCOAuthCallback(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "/auth/oidc/callback", recorder.Header().Get("Location"))
	require.NotNil(t, findCookie(recorder.Result().Cookies(), oauthPendingSessionCookieName))
}

func TestOIDCOAuthCallbackCreatesLoginPendingSessionForExistingIdentityUser(t *testing.T) {
	cfg, cleanup := newOIDCTestProvider(t, oidcProviderFixture{
		Subject:           "oidc-subject-login",
		PreferredUsername: "oidc_login",
		DisplayName:       "OIDC Login Display",
		AvatarURL:         "https://cdn.example/oidc-login.png",
		Email:             "oidc-login@example.com",
		EmailVerified:     true,
	})
	defer cleanup()

	handler, client := newOIDCOAuthHandlerAndClient(t, false, cfg)
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	existingUser, err := client.User.Create().
		SetEmail(oidcSyntheticEmailFromIdentityKey(oidcIdentityKey(cfg.IssuerURL, "oidc-subject-login"))).
		SetUsername("legacy-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.AuthIdentity.Create().
		SetUserID(existingUser.ID).
		SetProviderType("oidc").
		SetProviderKey(cfg.IssuerURL).
		SetProviderSubject("oidc-subject-login").
		SetMetadata(map[string]any{"username": "legacy-user"}).
		Save(ctx)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/callback?code=oidc-code&state=state-123", nil)
	req.AddCookie(encodedCookie(oidcOAuthStateCookieName, "state-123"))
	req.AddCookie(encodedCookie(oidcOAuthRedirectCookie, "/dashboard"))
	req.AddCookie(encodedCookie(oidcOAuthVerifierCookie, "verifier-123"))
	req.AddCookie(encodedCookie(oidcOAuthNonceCookie, "nonce-oidc-subject-login"))
	req.AddCookie(encodedCookie(oidcOAuthIntentCookieName, oauthIntentLogin))
	req.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "browser-123"))
	c.Request = req

	handler.OIDCOAuthCallback(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "/auth/oidc/callback", recorder.Header().Get("Location"))

	sessionCookie := findCookie(recorder.Result().Cookies(), oauthPendingSessionCookieName)
	require.NotNil(t, sessionCookie)

	session, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.SessionTokenEQ(decodeCookieValueForTest(t, sessionCookie.Value))).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, oauthIntentLogin, session.Intent)
	require.NotNil(t, session.TargetUserID)
	require.Equal(t, existingUser.ID, *session.TargetUserID)
	require.Equal(t, cfg.IssuerURL, session.ProviderKey)
	require.Equal(t, "OIDC Login Display", session.UpstreamIdentityClaims["suggested_display_name"])

	completion, ok := session.LocalFlowState[oauthCompletionResponseKey].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "/dashboard", completion["redirect"])
	_, hasAccessToken := completion["access_token"]
	require.False(t, hasAccessToken)
	_, hasRefreshToken := completion["refresh_token"]
	require.False(t, hasRefreshToken)
	require.Nil(t, completion["error"])
}

func TestOIDCOAuthCallbackRejectsDisabledExistingIdentityUser(t *testing.T) {
	cfg, cleanup := newOIDCTestProvider(t, oidcProviderFixture{
		Subject:           "oidc-disabled-subject",
		PreferredUsername: "oidc_disabled",
		DisplayName:       "OIDC Disabled",
	})
	defer cleanup()

	handler, client := newOIDCOAuthHandlerAndClient(t, false, cfg)
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	existingUser, err := client.User.Create().
		SetEmail(oidcSyntheticEmailFromIdentityKey(oidcIdentityKey(cfg.IssuerURL, "oidc-disabled-subject"))).
		SetUsername("disabled-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusDisabled).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.AuthIdentity.Create().
		SetUserID(existingUser.ID).
		SetProviderType("oidc").
		SetProviderKey(cfg.IssuerURL).
		SetProviderSubject("oidc-disabled-subject").
		Save(ctx)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/callback?code=oidc-code&state=state-disabled", nil)
	req.AddCookie(encodedCookie(oidcOAuthStateCookieName, "state-disabled"))
	req.AddCookie(encodedCookie(oidcOAuthRedirectCookie, "/dashboard"))
	req.AddCookie(encodedCookie(oidcOAuthVerifierCookie, "verifier-disabled"))
	req.AddCookie(encodedCookie(oidcOAuthNonceCookie, "nonce-oidc-disabled-subject"))
	req.AddCookie(encodedCookie(oidcOAuthIntentCookieName, oauthIntentLogin))
	req.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "browser-disabled"))
	c.Request = req

	handler.OIDCOAuthCallback(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Nil(t, findCookie(recorder.Result().Cookies(), oauthPendingSessionCookieName))
	assertOAuthRedirectError(t, recorder.Header().Get("Location"), "session_error", "USER_NOT_ACTIVE")

	count, err := client.PendingAuthSession.Query().Count(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestOIDCOAuthCallbackCreatesBindPendingSessionForCompatEmailUser(t *testing.T) {
	cfg, cleanup := newOIDCTestProvider(t, oidcProviderFixture{
		Subject:           "oidc-subject-compat",
		PreferredUsername: "oidc_compat",
		DisplayName:       "OIDC Compat Display",
		AvatarURL:         "https://cdn.example/oidc-compat.png",
		Email:             "legacy@example.com",
		EmailVerified:     true,
	})
	defer cleanup()

	handler, client := newOIDCOAuthHandlerAndClient(t, false, cfg)
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	existingUser, err := client.User.Create().
		SetEmail("legacy@example.com").
		SetUsername("legacy-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/callback?code=oidc-code&state=state-compat", nil)
	req.AddCookie(encodedCookie(oidcOAuthStateCookieName, "state-compat"))
	req.AddCookie(encodedCookie(oidcOAuthRedirectCookie, "/dashboard"))
	req.AddCookie(encodedCookie(oidcOAuthVerifierCookie, "verifier-compat"))
	req.AddCookie(encodedCookie(oidcOAuthNonceCookie, "nonce-oidc-subject-compat"))
	req.AddCookie(encodedCookie(oidcOAuthIntentCookieName, oauthIntentLogin))
	req.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "browser-compat"))
	c.Request = req

	handler.OIDCOAuthCallback(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "/auth/oidc/callback", recorder.Header().Get("Location"))

	sessionCookie := findCookie(recorder.Result().Cookies(), oauthPendingSessionCookieName)
	require.NotNil(t, sessionCookie)

	session, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.SessionTokenEQ(decodeCookieValueForTest(t, sessionCookie.Value))).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, oauthIntentLogin, session.Intent)
	require.NotNil(t, session.TargetUserID)
	require.Equal(t, existingUser.ID, *session.TargetUserID)
	require.Equal(t, existingUser.Email, session.ResolvedEmail)
	require.Equal(t, "legacy@example.com", session.UpstreamIdentityClaims["compat_email"])

	completion, ok := session.LocalFlowState[oauthCompletionResponseKey].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "/dashboard", completion["redirect"])
	require.Equal(t, oauthPendingChoiceStep, completion["step"])
	require.Equal(t, existingUser.Email, completion["email"])
	require.Equal(t, existingUser.Email, completion["existing_account_email"])
	require.Equal(t, true, completion["existing_account_bindable"])
	require.Equal(t, "compat_email_match", completion["choice_reason"])
	_, hasAccessToken := completion["access_token"]
	require.False(t, hasAccessToken)
}

func TestOIDCOAuthCallbackAllowsCompatEmailBindWhenUpstreamEmailIsUnverified(t *testing.T) {
	cfg, cleanup := newOIDCTestProvider(t, oidcProviderFixture{
		Subject:           "oidc-subject-unverified-compat",
		PreferredUsername: "oidc_unverified",
		DisplayName:       "OIDC Unverified Compat Display",
		AvatarURL:         "https://cdn.example/oidc-unverified.png",
		Email:             "owner@example.com",
		EmailVerified:     false,
	})
	defer cleanup()
	cfg.RequireEmailVerified = true

	handler, client := newOIDCOAuthHandlerAndClient(t, false, cfg)
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	_, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/callback?code=oidc-code&state=state-unverified-compat", nil)
	req.AddCookie(encodedCookie(oidcOAuthStateCookieName, "state-unverified-compat"))
	req.AddCookie(encodedCookie(oidcOAuthRedirectCookie, "/settings/connections"))
	req.AddCookie(encodedCookie(oidcOAuthVerifierCookie, "verifier-unverified-compat"))
	req.AddCookie(encodedCookie(oidcOAuthNonceCookie, "nonce-oidc-subject-unverified-compat"))
	req.AddCookie(encodedCookie(oidcOAuthIntentCookieName, oauthIntentLogin))
	req.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "browser-unverified-compat"))
	c.Request = req

	handler.OIDCOAuthCallback(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "/auth/oidc/callback#error=email_not_verified&error_message=email+is+not+verified", recorder.Header().Get("Location"))
	require.Nil(t, findCookie(recorder.Result().Cookies(), oauthPendingSessionCookieName))

	count, err := client.PendingAuthSession.Query().Count(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestOIDCOAuthCallbackCreatesChoicePendingSessionWhenSignupRequiresInvite(t *testing.T) {
	cfg, cleanup := newOIDCTestProvider(t, oidcProviderFixture{
		Subject:           "oidc-subject-invite",
		PreferredUsername: "oidc_invite",
		DisplayName:       "OIDC Invite Display",
		AvatarURL:         "https://cdn.example/oidc-invite.png",
		Email:             "oidc-invite@example.com",
		EmailVerified:     true,
	})
	defer cleanup()

	handler, client := newOIDCOAuthHandlerAndClient(t, true, cfg)
	t.Cleanup(func() { _ = client.Close() })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/callback?code=oidc-code&state=state-456", nil)
	req.AddCookie(encodedCookie(oidcOAuthStateCookieName, "state-456"))
	req.AddCookie(encodedCookie(oidcOAuthRedirectCookie, "/dashboard"))
	req.AddCookie(encodedCookie(oidcOAuthVerifierCookie, "verifier-456"))
	req.AddCookie(encodedCookie(oidcOAuthNonceCookie, "nonce-oidc-subject-invite"))
	req.AddCookie(encodedCookie(oidcOAuthIntentCookieName, oauthIntentLogin))
	req.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "browser-456"))
	c.Request = req

	handler.OIDCOAuthCallback(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "/auth/oidc/callback", recorder.Header().Get("Location"))

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
	require.Equal(t, oauthPendingChoiceStep, completion["step"])
	require.Equal(t, "/dashboard", completion["redirect"])
	require.Equal(t, "third_party_signup", completion["choice_reason"])
}

func TestOIDCOAuthCallbackCreatesBindPendingSessionForCurrentUser(t *testing.T) {
	cfg, cleanup := newOIDCTestProvider(t, oidcProviderFixture{
		Subject:           "oidc-subject-bind",
		PreferredUsername: "oidc_bind",
		DisplayName:       "OIDC Bind Display",
		AvatarURL:         "https://cdn.example/oidc-bind.png",
		Email:             "oidc-bind@example.com",
		EmailVerified:     true,
	})
	defer cleanup()

	handler, client := newOIDCOAuthHandlerAndClient(t, false, cfg)
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/callback?code=oidc-code&state=state-bind", nil)
	req.AddCookie(encodedCookie(oidcOAuthStateCookieName, "state-bind"))
	req.AddCookie(encodedCookie(oidcOAuthRedirectCookie, "/settings/connections"))
	req.AddCookie(encodedCookie(oidcOAuthVerifierCookie, "verifier-bind"))
	req.AddCookie(encodedCookie(oidcOAuthNonceCookie, "nonce-oidc-subject-bind"))
	req.AddCookie(encodedCookie(oidcOAuthIntentCookieName, oauthIntentBindCurrentUser))
	req.AddCookie(encodedCookie(oidcOAuthBindUserCookieName, buildEncodedOAuthBindUserCookie(t, currentUser.ID, "test-secret")))
	req.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "browser-bind"))
	c.Request = req

	handler.OIDCOAuthCallback(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "/auth/oidc/callback", recorder.Header().Get("Location"))

	sessionCookie := findCookie(recorder.Result().Cookies(), oauthPendingSessionCookieName)
	require.NotNil(t, sessionCookie)

	session, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.SessionTokenEQ(decodeCookieValueForTest(t, sessionCookie.Value))).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, oauthIntentBindCurrentUser, session.Intent)
	require.NotNil(t, session.TargetUserID)
	require.Equal(t, currentUser.ID, *session.TargetUserID)
	require.Equal(t, cfg.IssuerURL, session.ProviderKey)
	require.Equal(t, "OIDC Bind Display", session.UpstreamIdentityClaims["suggested_display_name"])

	completion, ok := session.LocalFlowState[oauthCompletionResponseKey].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "/settings/connections", completion["redirect"])
	require.Empty(t, completion["access_token"])

	userCount, err := client.User.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, userCount)
}

func TestCompleteOIDCOAuthRegistrationAppliesPendingAdoptionDecision(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("oidc-complete-session").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example.com").
		SetProviderSubject("oidc-subject-1").
		SetResolvedEmail("93a310f4c1944c5bbd2e246df1f76485@oidc-connect.invalid").
		SetBrowserSessionKey("oidc-browser").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "oidc_user",
			"issuer":                 "https://issuer.example.com",
			"suggested_display_name": "OIDC Display",
			"suggested_avatar_url":   "https://cdn.example/oidc.png",
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/complete-registration", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("oidc-browser")})
	c.Request = req

	handler.CompleteOIDCOAuthRegistration(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	responseData := decodeJSONBody(t, recorder)
	require.NotEmpty(t, responseData["access_token"])

	userEntity, err := client.User.Query().
		Where(dbuser.EmailEQ(session.ResolvedEmail)).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, "OIDC Display", userEntity.Username)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("oidc"),
			authidentity.ProviderKeyEQ("https://issuer.example.com"),
			authidentity.ProviderSubjectEQ("oidc-subject-1"),
		).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, userEntity.ID, identity.UserID)
	require.Equal(t, "OIDC Display", identity.Metadata["display_name"])
	require.Equal(t, "https://cdn.example/oidc.png", identity.Metadata["avatar_url"])

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

func TestCompleteOIDCOAuthRegistrationRejectsAdoptExistingUserSession(t *testing.T) {
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
		SetSessionToken("oidc-complete-invalid-session").
		SetIntent("adopt_existing_user_by_email").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example.com").
		SetProviderSubject("oidc-invalid-subject-1").
		SetTargetUserID(existingUser.ID).
		SetResolvedEmail(existingUser.Email).
		SetBrowserSessionKey("oidc-invalid-browser").
		SetUpstreamIdentityClaims(map[string]any{
			"username": "oidc_user",
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/complete-registration", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("oidc-invalid-browser")})
	c.Request = req

	handler.CompleteOIDCOAuthRegistration(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, storedSession.ConsumedAt)
}

func TestCompleteOIDCOAuthRegistrationReturnsPendingSessionWhenChoiceStillRequired(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("oidc-complete-choice-session").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example.com").
		SetProviderSubject("oidc-choice-subject-1").
		SetResolvedEmail("oidc-choice-subject-1@oidc-connect.invalid").
		SetBrowserSessionKey("oidc-choice-browser").
		SetUpstreamIdentityClaims(map[string]any{
			"username": "oidc_user",
			"issuer":   "https://issuer.example.com",
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/complete-registration", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("oidc-choice-browser")})
	c.Request = req

	handler.CompleteOIDCOAuthRegistration(c)

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

func TestCompleteOIDCOAuthRegistrationBindsIdentityWithoutAdoptionFlags(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("oidc-complete-no-adoption-session").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example.com").
		SetProviderSubject("oidc-subject-no-adoption").
		SetResolvedEmail("8c9f12b2a2e14b1db9efc08b27e0ef5c@oidc-connect.invalid").
		SetBrowserSessionKey("oidc-browser-no-adoption").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "oidc_user",
			"issuer":                 "https://issuer.example.com",
			"suggested_display_name": "OIDC Legacy",
			"suggested_avatar_url":   "https://cdn.example/oidc-legacy.png",
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"invitation_code":"invite-1"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/complete-registration", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("oidc-browser-no-adoption")})
	c.Request = req

	handler.CompleteOIDCOAuthRegistration(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	responseData := decodeJSONBody(t, recorder)
	require.NotEmpty(t, responseData["access_token"])
	require.NotEmpty(t, responseData["refresh_token"])

	userEntity, err := client.User.Query().
		Where(dbuser.EmailEQ(session.ResolvedEmail)).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, "oidc_user", userEntity.Username)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("oidc"),
			authidentity.ProviderKeyEQ("https://issuer.example.com"),
			authidentity.ProviderSubjectEQ("oidc-subject-no-adoption"),
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

func TestCompleteOIDCOAuthRegistrationRejectsIdentityOwnershipConflictBeforeUserCreation(t *testing.T) {
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
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example.com").
		SetProviderSubject("oidc-conflict-subject").
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("oidc-complete-conflict-session").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example.com").
		SetProviderSubject("oidc-conflict-subject").
		SetResolvedEmail("f6f5f1f16f9248ccb11e0d633963b290@oidc-connect.invalid").
		SetBrowserSessionKey("oidc-conflict-browser").
		SetUpstreamIdentityClaims(map[string]any{
			"username": "oidc_user",
			"issuer":   "https://issuer.example.com",
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"invitation_code":"invite-1"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/complete-registration", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("oidc-conflict-browser")})
	c.Request = req

	handler.CompleteOIDCOAuthRegistration(c)

	require.Equal(t, http.StatusConflict, recorder.Code)
	payload := decodeJSONBody(t, recorder)
	require.Equal(t, "AUTH_IDENTITY_OWNERSHIP_CONFLICT", payload["reason"])

	userCount, err := client.User.Query().
		Where(dbuser.EmailEQ("f6f5f1f16f9248ccb11e0d633963b290@oidc-connect.invalid")).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, userCount)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, storedSession.ConsumedAt)
}

type oidcProviderFixture struct {
	Subject           string
	PreferredUsername string
	DisplayName       string
	AvatarURL         string
	Email             string
	EmailVerified     bool
}

func newOIDCOAuthTestHandler(t *testing.T, invitationEnabled bool, oauthCfg config.OIDCConnectConfig) *AuthHandler {
	t.Helper()
	handler, _ := newOIDCOAuthHandlerAndClient(t, invitationEnabled, oauthCfg)
	return handler
}

func newOIDCOAuthHandlerAndClient(t *testing.T, invitationEnabled bool, oauthCfg config.OIDCConnectConfig) (*AuthHandler, *dbent.Client) {
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
		OIDC: oauthCfg,
	}
	return handler, client
}

func newOIDCTestProvider(t *testing.T, fixture oidcProviderFixture) (config.OIDCConnectConfig, func()) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	kid := "test-kid"
	jwks := oidcJWKSet{Keys: []oidcJWK{buildRSAJWK(kid, &privateKey.PublicKey)}}
	tokenResponse := oidcTokenResponse{
		AccessToken: "oidc-access-token",
		TokenType:   "Bearer",
		ExpiresIn:   3600,
	}

	userInfoPayload := map[string]any{
		"sub":                fixture.Subject,
		"preferred_username": fixture.PreferredUsername,
		"name":               fixture.DisplayName,
		"picture":            fixture.AvatarURL,
		"email":              fixture.Email,
		"email_verified":     fixture.EmailVerified,
	}

	var issuer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			require.NoError(t, json.NewEncoder(w).Encode(tokenResponse))
		case "/userinfo":
			require.NoError(t, json.NewEncoder(w).Encode(userInfoPayload))
		case "/jwks":
			require.NoError(t, json.NewEncoder(w).Encode(jwks))
		default:
			http.NotFound(w, r)
		}
	}))

	issuer = server.URL
	now := time.Now()
	claims := oidcIDTokenClaims{
		Email:             fixture.Email,
		EmailVerified:     boolPtr(fixture.EmailVerified),
		PreferredUsername: fixture.PreferredUsername,
		Name:              fixture.DisplayName,
		Nonce:             "nonce-" + fixture.Subject,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   fixture.Subject,
			Audience:  jwt.ClaimStrings{"oidc-client"},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	tokenResponse.IDToken, err = token.SignedString(privateKey)
	require.NoError(t, err)

	cfg := config.OIDCConnectConfig{
		Enabled:              true,
		ProviderName:         "Test OIDC",
		ClientID:             "oidc-client",
		ClientSecret:         "oidc-secret",
		IssuerURL:            issuer,
		AuthorizeURL:         issuer + "/authorize",
		TokenURL:             issuer + "/token",
		UserInfoURL:          issuer + "/userinfo",
		JWKSURL:              issuer + "/jwks",
		Scopes:               "openid profile email",
		RedirectURL:          "https://api.example.com/api/v1/auth/oauth/oidc/callback",
		FrontendRedirectURL:  "/auth/oidc/callback",
		TokenAuthMethod:      "client_secret_post",
		UsePKCE:              true,
		ValidateIDToken:      true,
		AllowedSigningAlgs:   "RS256",
		ClockSkewSeconds:     120,
		RequireEmailVerified: false,
	}
	return cfg, server.Close
}
