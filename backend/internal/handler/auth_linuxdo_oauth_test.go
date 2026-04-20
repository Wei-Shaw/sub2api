package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
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
REDACTED

func TestBuildBearerAuthorization(t *testing.T) {
	auth, err := buildBearerAuthorization("", "token123")
REDACTED
	require.Equal(t, "Bearer token123", auth)

	auth, err = buildBearerAuthorization("bearer", "token123")
REDACTED
	require.Equal(t, "Bearer token123", auth)

	_, err = buildBearerAuthorization("MAC", "token123")
REDACTED

	_, err = buildBearerAuthorization("Bearer", "token 123")
REDACTED
REDACTED

func TestLinuxDoParseUserInfoParsesIDAndUsername(t *testing.T) {
	cfg := config.LinuxDoConnectConfig{
		UserInfoURL: "https://connect.linux.do/api/user",
REDACTED

	email, username, subject, displayName, avatarURL, err := linuxDoParseUserInfo(`{"id":123,"username":"alice","name":"Alice","avatar_url":"https://cdn.example/avatar.png"REDACTED`, cfg)
REDACTED
	require.Equal(t, "123", subject)
	require.Equal(t, "alice", username)
	require.Equal(t, "linuxdo-123@linuxdo-connect.invalid", email)
	require.Equal(t, "Alice", displayName)
	require.Equal(t, "https://cdn.example/avatar.png", avatarURL)
REDACTED

func TestLinuxDoParseUserInfoDefaultsUsername(t *testing.T) {
	cfg := config.LinuxDoConnectConfig{
		UserInfoURL: "https://connect.linux.do/api/user",
REDACTED

	email, username, subject, displayName, avatarURL, err := linuxDoParseUserInfo(`{"id":"123"REDACTED`, cfg)
REDACTED
	require.Equal(t, "123", subject)
	require.Equal(t, "linuxdo_123", username)
	require.Equal(t, "linuxdo-123@linuxdo-connect.invalid", email)
	require.Equal(t, "linuxdo_123", displayName)
	require.Equal(t, "", avatarURL)
REDACTED

func TestLinuxDoParseUserInfoRejectsUnsafeSubject(t *testing.T) {
	cfg := config.LinuxDoConnectConfig{
		UserInfoURL: "https://connect.linux.do/api/user",
REDACTED

	_, _, _, _, _, err := linuxDoParseUserInfo(`{"id":"123@456"REDACTED`, cfg)
REDACTED

	tooLong := strings.Repeat("a", linuxDoOAuthMaxSubjectLen+1)
	_, _, _, _, _, err = linuxDoParseUserInfo(`{"id":"`+tooLong+`"REDACTED`, cfg)
REDACTED
REDACTED

func TestParseOAuthProviderErrorJSON(t *testing.T) {
	code, desc := parseOAuthProviderError(`{"error":"invalid_client","error_description":"bad secret"REDACTED`)
	require.Equal(t, "invalid_client", code)
	require.Equal(t, "bad secret", desc)
REDACTED

func TestParseOAuthProviderErrorForm(t *testing.T) {
	code, desc := parseOAuthProviderError("error=invalid_request&error_description=Missing+code_verifier")
	require.Equal(t, "invalid_request", code)
	require.Equal(t, "Missing code_verifier", desc)
REDACTED

func TestParseLinuxDoTokenResponseJSON(t *testing.T) {
	token, ok := parseLinuxDoTokenResponse(`{"access_token":"t1","token_type":"Bearer","expires_in":3600,"scope":"user"REDACTED`)
	require.True(t, ok)
	require.Equal(t, "t1", token.AccessToken)
	require.Equal(t, "Bearer", token.TokenType)
	require.Equal(t, int64(3600), token.ExpiresIn)
	require.Equal(t, "user", token.Scope)
REDACTED

func TestParseLinuxDoTokenResponseForm(t *testing.T) {
	token, ok := parseLinuxDoTokenResponse("access_token=t2&token_type=bearer&expires_in=60")
	require.True(t, ok)
	require.Equal(t, "t2", token.AccessToken)
	require.Equal(t, "bearer", token.TokenType)
	require.Equal(t, int64(60), token.ExpiresIn)
REDACTED

func TestSingleLineStripsWhitespace(t *testing.T) {
	require.Equal(t, "hello world", singleLine("hello\r\nworld"))
	require.Equal(t, "", singleLine("\n\t\r"))
REDACTED

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
REDACTED)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/linuxdo/bind/start?intent=bind_current_user&redirect=/settings/connections", nil)
	c.Request = req
	c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 42REDACTED)

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
REDACTED
	require.Equal(t, int64(42), userID)
REDACTED

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
REDACTED)
	defer client.Close()

	user, err := client.User.Create().
		SetEmail("bind-cookie@example.com").
		SetUsername("bind-cookie-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(context.Background())
REDACTED

	token, err := handler.authService.GenerateToken(&service.User{
		ID:           user.ID,
		Email:        user.Email,
		Username:     user.Username,
		PasswordHash: user.PasswordHash,
		Role:         user.Role,
		Status:       user.Status,
REDACTED)
REDACTED

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/linuxdo/start?intent=bind_current_user&redirect=/settings/connections", nil)
	req.AddCookie(&http.Cookie{Name: oauthBindAccessTokenCookieName, Value: token, Path: oauthBindAccessTokenCookiePathREDACTED)
	c.Request = req

	handler.LinuxDoOAuthStart(c)

	require.Equal(t, http.StatusFound, recorder.Code)

	bindCookie := findCookie(recorder.Result().Cookies(), linuxDoOAuthBindUserCookieName)
	require.NotNil(t, bindCookie)
	userID, err := parseOAuthBindUserCookieValue(decodeCookieValueForTest(t, bindCookie.Value), "test-secret")
REDACTED
	require.Equal(t, user.ID, userID)

	accessTokenCookie := findCookie(recorder.Result().Cookies(), oauthBindAccessTokenCookieName)
	require.NotNil(t, accessTokenCookie)
	require.Equal(t, -1, accessTokenCookie.MaxAge)
REDACTED

func TestLinuxDoOAuthCallbackCreatesLoginPendingSessionForExistingUser(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"linuxdo-access","token_type":"Bearer","expires_in":3600REDACTED`))
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"321","username":"linuxdo_user","name":"LinuxDo Display","avatar_url":"https://cdn.example/linuxdo.png"REDACTED`))
		default:
			http.NotFound(w, r)
	REDACTED
REDACTED))
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
REDACTED)
	defer client.Close()

	ctx := context.Background()
	existingUser, err := client.User.Create().
		SetEmail(linuxDoSyntheticEmail("321")).
		SetUsername("legacy-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

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
REDACTED
	require.Equal(t, oauthIntentLogin, session.Intent)
	require.NotNil(t, session.TargetUserID)
	require.Equal(t, existingUser.ID, *session.TargetUserID)
	require.Equal(t, linuxDoSyntheticEmail("321"), session.ResolvedEmail)
	require.Equal(t, "LinuxDo Display", session.UpstreamIdentityClaims["suggested_display_name"])

	completion := session.LocalFlowState[oauthCompletionResponseKey].(map[string]any)
	require.Equal(t, "/dashboard", completion["redirect"])
	require.NotEmpty(t, completion["access_token"])
	require.Nil(t, completion["error"])
REDACTED

func TestLinuxDoOAuthCallbackCreatesInvitationPendingSessionWhenSignupRequiresInvite(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"linuxdo-access","token_type":"Bearer","expires_in":3600REDACTED`))
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"654","username":"linuxdo_invite","name":"Need Invite","avatar_url":"https://cdn.example/invite.png"REDACTED`))
		default:
			http.NotFound(w, r)
	REDACTED
REDACTED))
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
REDACTED)
	defer client.Close()

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
REDACTED
	require.Equal(t, oauthIntentLogin, session.Intent)
	require.Nil(t, session.TargetUserID)

	completion := session.LocalFlowState[oauthCompletionResponseKey].(map[string]any)
	require.Equal(t, "invitation_required", completion["error"])
	require.Equal(t, "/dashboard", completion["redirect"])
REDACTED

func TestLinuxDoOAuthCallbackCreatesBindPendingSessionForCurrentUser(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"linuxdo-access","token_type":"Bearer","expires_in":3600REDACTED`))
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"999","username":"bind_user","name":"Bind Display","avatar_url":"https://cdn.example/bind.png"REDACTED`))
		default:
			http.NotFound(w, r)
	REDACTED
REDACTED))
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
REDACTED)
	defer client.Close()

	ctx := context.Background()
	currentUser, err := client.User.Create().
		SetEmail("current@example.com").
		SetUsername("current-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

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
REDACTED
	require.Equal(t, oauthIntentBindCurrentUser, session.Intent)
	require.NotNil(t, session.TargetUserID)
	require.Equal(t, currentUser.ID, *session.TargetUserID)
	require.Equal(t, linuxDoSyntheticEmail("999"), session.ResolvedEmail)

	completion := session.LocalFlowState[oauthCompletionResponseKey].(map[string]any)
	require.Equal(t, "/settings/connections", completion["redirect"])
	require.Empty(t, completion["access_token"])
	require.Equal(t, "Bind Display", session.UpstreamIdentityClaims["suggested_display_name"])

	userCount, err := client.User.Query().Count(ctx)
REDACTED
	require.Equal(t, 1, userCount)
REDACTED

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
	REDACTED).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
REDACTED

	_, err = service.NewAuthPendingIdentityService(client).UpsertAdoptionDecision(ctx, service.PendingIdentityAdoptionDecisionInput{
		PendingAuthSessionID: session.ID,
		AdoptAvatar:          true,
REDACTED)
REDACTED

	body := bytes.NewBufferString(`{"invitation_code":"invite-1","adopt_display_name":trueREDACTED`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/linuxdo/complete-registration", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)REDACTED)
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("linuxdo-browser")REDACTED)
	c.Request = req

	handler.CompleteLinuxDoOAuthRegistration(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	responseData := decodeJSONBody(t, recorder)
	require.NotEmpty(t, responseData["access_token"])

	userEntity, err := client.User.Query().
		Where(dbuser.EmailEQ(session.ResolvedEmail)).
		Only(ctx)
REDACTED
	require.Equal(t, "LinuxDo Display", userEntity.Username)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("linuxdo"),
			authidentity.ProviderKeyEQ("linuxdo"),
			authidentity.ProviderSubjectEQ("linuxdo-subject-1"),
		).
		Only(ctx)
REDACTED
	require.Equal(t, userEntity.ID, identity.UserID)
	require.Equal(t, "LinuxDo Display", identity.Metadata["display_name"])
	require.Equal(t, "https://cdn.example/linuxdo.png", identity.Metadata["avatar_url"])

	decision, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(session.ID)).
		Only(ctx)
REDACTED
	require.NotNil(t, decision.IdentityID)
	require.Equal(t, identity.ID, *decision.IdentityID)
	require.True(t, decision.AdoptDisplayName)
	require.True(t, decision.AdoptAvatar)

	consumed, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
REDACTED
	require.NotNil(t, consumed.ConsumedAt)
REDACTED

func newLinuxDoOAuthTestHandler(t *testing.T, invitationEnabled bool, oauthCfg config.LinuxDoConnectConfig) *AuthHandler {
REDACTED
	handler, _ := newLinuxDoOAuthHandlerAndClient(t, invitationEnabled, oauthCfg)
	return handler
REDACTED

func newLinuxDoOAuthHandlerAndClient(t *testing.T, invitationEnabled bool, oauthCfg config.LinuxDoConnectConfig) (*AuthHandler, *dbent.Client) {
REDACTED
	handler, client := newOAuthPendingFlowTestHandler(t, invitationEnabled)
	handler.settingSvc = nil
	handler.cfg = &config.Config{
		JWT: config.JWTConfig{
			Secret:                   "test-secret",
			ExpireHour:               1,
			AccessTokenExpireMinutes: 60,
			RefreshTokenExpireDays:   7,
	REDACTED,
		LinuxDo: oauthCfg,
REDACTED
	return handler, client
REDACTED
