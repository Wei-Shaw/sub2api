//go:build unit

package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/identityadoptiondecision"
	"github.com/Wei-Shaw/sub2api/ent/pendingauthsession"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestWeChatOAuthStartRedirectsAndSetsPendingCookies(t *testing.T) {
	t.Setenv("WECHAT_OAUTH_OPEN_APP_ID", "wx-open-app")
	t.Setenv("WECHAT_OAUTH_OPEN_APP_SECRET", "wx-open-secret")

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/wechat/start?mode=open&redirect=/billing", nil)
	c.Request.Host = "api.example.com"

	handler := &AuthHandler{REDACTED
	handler.WeChatOAuthStart(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	location := recorder.Header().Get("Location")
	require.NotEmpty(t, location)
	require.Contains(t, location, "open.weixin.qq.com")
	require.Contains(t, location, "appid=wx-open-app")
	require.Contains(t, location, "scope=snsapi_login")

	cookies := recorder.Result().Cookies()
	require.NotEmpty(t, findCookie(cookies, wechatOAuthStateCookieName))
	require.NotEmpty(t, findCookie(cookies, wechatOAuthRedirectCookieName))
	require.NotEmpty(t, findCookie(cookies, wechatOAuthModeCookieName))
	require.NotEmpty(t, findCookie(cookies, oauthPendingBrowserCookieName))
REDACTED

func TestWeChatOAuthCallbackCreatesPendingSessionForUnifiedFlow(t *testing.T) {
	t.Setenv("WECHAT_OAUTH_OPEN_APP_ID", "wx-open-app")
	t.Setenv("WECHAT_OAUTH_OPEN_APP_SECRET", "wx-open-secret")
	t.Setenv("WECHAT_OAUTH_FRONTEND_REDIRECT_URL", "/auth/wechat/callback")

	originalAccessTokenURL := wechatOAuthAccessTokenURL
	originalUserInfoURL := wechatOAuthUserInfoURL
	t.Cleanup(func() {
		wechatOAuthAccessTokenURL = originalAccessTokenURL
		wechatOAuthUserInfoURL = originalUserInfoURL
REDACTED)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/sns/oauth2/access_token"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"wechat-access","openid":"openid-123","unionid":"union-456","scope":"snsapi_login"REDACTED`))
		case strings.Contains(r.URL.Path, "/sns/userinfo"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"openid":"openid-123","unionid":"union-456","nickname":"WeChat Nick","headimgurl":"https://cdn.example/avatar.png"REDACTED`))
		default:
			http.NotFound(w, r)
	REDACTED
REDACTED))
	defer upstream.Close()
	wechatOAuthAccessTokenURL = upstream.URL + "/sns/oauth2/access_token"
	wechatOAuthUserInfoURL = upstream.URL + "/sns/userinfo"

	handler, client := newWeChatOAuthTestHandler(t, false)
	defer client.Close()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/wechat/callback?code=wechat-code&state=state-123", nil)
	req.Host = "api.example.com"
	req.AddCookie(encodedCookie(wechatOAuthStateCookieName, "state-123"))
	req.AddCookie(encodedCookie(wechatOAuthRedirectCookieName, "/dashboard"))
	req.AddCookie(encodedCookie(wechatOAuthModeCookieName, "open"))
	req.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "browser-123"))
	c.Request = req

	handler.WeChatOAuthCallback(c)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "/auth/wechat/callback", recorder.Header().Get("Location"))

	sessionCookie := findCookie(recorder.Result().Cookies(), oauthPendingSessionCookieName)
	require.NotNil(t, sessionCookie)

	ctx := context.Background()
	session, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.SessionTokenEQ(decodeCookieValueForTest(t, sessionCookie.Value))).
		Only(ctx)
REDACTED
	require.Equal(t, "wechat", session.ProviderType)
	require.Equal(t, "wechat-main", session.ProviderKey)
	require.Equal(t, "union-456", session.ProviderSubject)
	require.Equal(t, "wechat-union-456@wechat-connect.invalid", session.ResolvedEmail)
	require.Equal(t, "WeChat Nick", session.UpstreamIdentityClaims["suggested_display_name"])
	require.Equal(t, "https://cdn.example/avatar.png", session.UpstreamIdentityClaims["suggested_avatar_url"])
	require.Equal(t, "union-456", session.UpstreamIdentityClaims["unionid"])
	require.Equal(t, "openid-123", session.UpstreamIdentityClaims["openid"])
REDACTED

func TestCompleteWeChatOAuthRegistrationAfterInvitationPendingSession(t *testing.T) {
	t.Setenv("WECHAT_OAUTH_OPEN_APP_ID", "wx-open-app")
	t.Setenv("WECHAT_OAUTH_OPEN_APP_SECRET", "wx-open-secret")
	t.Setenv("WECHAT_OAUTH_FRONTEND_REDIRECT_URL", "/auth/wechat/callback")

	originalAccessTokenURL := wechatOAuthAccessTokenURL
	originalUserInfoURL := wechatOAuthUserInfoURL
	t.Cleanup(func() {
		wechatOAuthAccessTokenURL = originalAccessTokenURL
		wechatOAuthUserInfoURL = originalUserInfoURL
REDACTED)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/sns/oauth2/access_token"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"wechat-access","openid":"openid-123","unionid":"union-456","scope":"snsapi_login"REDACTED`))
		case strings.Contains(r.URL.Path, "/sns/userinfo"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"openid":"openid-123","unionid":"union-456","nickname":"WeChat Display","headimgurl":"https://cdn.example/wechat.png"REDACTED`))
		default:
			http.NotFound(w, r)
	REDACTED
REDACTED))
	defer upstream.Close()
	wechatOAuthAccessTokenURL = upstream.URL + "/sns/oauth2/access_token"
	wechatOAuthUserInfoURL = upstream.URL + "/sns/userinfo"

	handler, client := newWeChatOAuthTestHandler(t, true)
	defer client.Close()

	ctx := context.Background()
	redeemRepo := repository.NewRedeemCodeRepository(client)
	require.NoError(t, redeemRepo.Create(ctx, &service.RedeemCode{
		Code:   "invite-1",
		Type:   service.RedeemTypeInvitation,
		Status: service.StatusUnused,
REDACTED))

	callbackRecorder := httptest.NewRecorder()
	callbackCtx, _ := gin.CreateTestContext(callbackRecorder)
	callbackReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/wechat/callback?code=wechat-code&state=state-123", nil)
	callbackReq.Host = "api.example.com"
	callbackReq.AddCookie(encodedCookie(wechatOAuthStateCookieName, "state-123"))
	callbackReq.AddCookie(encodedCookie(wechatOAuthRedirectCookieName, "/dashboard"))
	callbackReq.AddCookie(encodedCookie(wechatOAuthModeCookieName, "open"))
	callbackReq.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "browser-123"))
	callbackCtx.Request = callbackReq

	handler.WeChatOAuthCallback(callbackCtx)

	require.Equal(t, http.StatusFound, callbackRecorder.Code)
	require.Equal(t, "/auth/wechat/callback", callbackRecorder.Header().Get("Location"))

	sessionCookie := findCookie(callbackRecorder.Result().Cookies(), oauthPendingSessionCookieName)
	require.NotNil(t, sessionCookie)
	sessionToken := decodeCookieValueForTest(t, sessionCookie.Value)

	pendingSession, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.SessionTokenEQ(sessionToken)).
		Only(ctx)
REDACTED
	require.Equal(t, "invitation_required", pendingSession.LocalFlowState[oauthCompletionResponseKey].(map[string]any)["error"])

	body := bytes.NewBufferString(`{"invitation_code":"invite-1","adopt_display_name":true,"adopt_avatar":trueREDACTED`)
	completeRecorder := httptest.NewRecorder()
	completeCtx, _ := gin.CreateTestContext(completeRecorder)
	completeReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/wechat/complete-registration", body)
	completeReq.Header.Set("Content-Type", "application/json")
	completeReq.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(sessionToken)REDACTED)
	completeReq.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("browser-123")REDACTED)
	completeCtx.Request = completeReq

	handler.CompleteWeChatOAuthRegistration(completeCtx)

	require.Equal(t, http.StatusOK, completeRecorder.Code)
	responseData := decodeJSONBody(t, completeRecorder)
	require.NotEmpty(t, responseData["access_token"])

	userEntity, err := client.User.Query().
		Where(dbuser.EmailEQ("wechat-union-456@wechat-connect.invalid")).
		Only(ctx)
REDACTED
	require.Equal(t, "WeChat Display", userEntity.Username)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("wechat"),
			authidentity.ProviderKeyEQ("wechat-main"),
			authidentity.ProviderSubjectEQ("union-456"),
		).
		Only(ctx)
REDACTED
	require.Equal(t, userEntity.ID, identity.UserID)
	require.Equal(t, "WeChat Display", identity.Metadata["display_name"])
	require.Equal(t, "https://cdn.example/wechat.png", identity.Metadata["avatar_url"])

	decision, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(pendingSession.ID)).
		Only(ctx)
REDACTED
	require.NotNil(t, decision.IdentityID)
	require.Equal(t, identity.ID, *decision.IdentityID)
	require.True(t, decision.AdoptDisplayName)
	require.True(t, decision.AdoptAvatar)

	consumed, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(pendingSession.ID)).
		Only(ctx)
REDACTED
	require.NotNil(t, consumed.ConsumedAt)
REDACTED

func newWeChatOAuthTestHandler(t *testing.T, invitationEnabled bool) (*AuthHandler, *dbent.Client) {
REDACTED

	db, err := sql.Open("sqlite", "file:auth_wechat_oauth?mode=memory&cache=shared")
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)

	_, err = db.Exec("PRAGMA foreign_keys = ON")
REDACTED

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))

	userRepo := &oauthPendingFlowUserRepo{client: clientREDACTED
	redeemRepo := repository.NewRedeemCodeRepository(client)
	settingSvc := service.NewSettingService(&wechatOAuthSettingRepoStub{
		values: map[string]string{
			service.SettingKeyRegistrationEnabled:   "true",
			service.SettingKeyInvitationCodeEnabled: boolSettingValue(invitationEnabled),
	REDACTED,
REDACTED, &config.Config{
		JWT: config.JWTConfig{
			Secret:                   "test-secret",
			ExpireHour:               1,
			AccessTokenExpireMinutes: 60,
			RefreshTokenExpireDays:   7,
	REDACTED,
		Default: config.DefaultConfig{
			UserBalance:     0,
			UserConcurrency: 1,
	REDACTED,
REDACTED)

	authSvc := service.NewAuthService(
		client,
		userRepo,
		redeemRepo,
		&wechatOAuthRefreshTokenCacheStub{REDACTED,
		&config.Config{
			JWT: config.JWTConfig{
				Secret:                   "test-secret",
				ExpireHour:               1,
				AccessTokenExpireMinutes: 60,
				RefreshTokenExpireDays:   7,
		REDACTED,
			Default: config.DefaultConfig{
				UserBalance:     0,
				UserConcurrency: 1,
		REDACTED,
	REDACTED,
		settingSvc,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	return &AuthHandler{
		authService: authSvc,
		settingSvc:  settingSvc,
REDACTED, client
REDACTED

func encodedCookie(name, value string) *http.Cookie {
	return &http.Cookie{
		Name:  name,
		Value: encodeCookieValue(value),
		Path:  "/",
REDACTED
REDACTED

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
	REDACTED
REDACTED
	return nil
REDACTED

func decodeCookieValueForTest(t *testing.T, value string) string {
REDACTED
	raw, err := base64.RawURLEncoding.DecodeString(value)
REDACTED
	return string(raw)
REDACTED

type wechatOAuthSettingRepoStub struct {
	values map[string]string
REDACTED

func (s *wechatOAuthSettingRepoStub) Get(context.Context, string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
REDACTED

func (s *wechatOAuthSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
REDACTED
	return value, nil
REDACTED

func (s *wechatOAuthSettingRepoStub) Set(context.Context, string, string) error {
	return nil
REDACTED

func (s *wechatOAuthSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			result[key] = value
	REDACTED
REDACTED
	return result, nil
REDACTED

func (s *wechatOAuthSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	return nil
REDACTED

func (s *wechatOAuthSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	result := make(map[string]string, len(s.values))
	for key, value := range s.values {
		result[key] = value
REDACTED
	return result, nil
REDACTED

func (s *wechatOAuthSettingRepoStub) Delete(context.Context, string) error {
	return nil
REDACTED

type wechatOAuthRefreshTokenCacheStub struct{REDACTED

func (s *wechatOAuthRefreshTokenCacheStub) StoreRefreshToken(context.Context, string, *service.RefreshTokenData, time.Duration) error {
	return nil
REDACTED

func (s *wechatOAuthRefreshTokenCacheStub) GetRefreshToken(context.Context, string) (*service.RefreshTokenData, error) {
	return nil, service.ErrRefreshTokenNotFound
REDACTED

func (s *wechatOAuthRefreshTokenCacheStub) DeleteRefreshToken(context.Context, string) error {
	return nil
REDACTED

func (s *wechatOAuthRefreshTokenCacheStub) DeleteUserRefreshTokens(context.Context, int64) error {
	return nil
REDACTED

func (s *wechatOAuthRefreshTokenCacheStub) DeleteTokenFamily(context.Context, string) error {
	return nil
REDACTED

func (s *wechatOAuthRefreshTokenCacheStub) AddToUserTokenSet(context.Context, int64, string, time.Duration) error {
	return nil
REDACTED

func (s *wechatOAuthRefreshTokenCacheStub) AddToFamilyTokenSet(context.Context, string, string, time.Duration) error {
	return nil
REDACTED

func (s *wechatOAuthRefreshTokenCacheStub) GetUserTokenHashes(context.Context, int64) ([]string, error) {
	return nil, nil
REDACTED

func (s *wechatOAuthRefreshTokenCacheStub) GetFamilyTokenHashes(context.Context, string) ([]string, error) {
	return nil, nil
REDACTED

func (s *wechatOAuthRefreshTokenCacheStub) IsTokenInFamily(context.Context, string, string) (bool, error) {
	return false, nil
REDACTED
