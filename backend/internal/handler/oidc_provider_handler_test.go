package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// timeFromNow 返回 time.Now() 之后 sec 秒的 UTC 时间，仅供测试构造过期时间。
func timeFromNow(sec int) time.Time {
	return time.Now().UTC().Add(time.Duration(sec) * time.Second)
}

// ─── 测试基础设施 ────────────────────────────────────────────────────────────

// oidcMemSettingRepo 是 service.SettingRepository 的内存实现 (仅供 handler 测试)。
type oidcMemSettingRepo struct {
	mu     sync.Mutex
	values map[string]string
}

func newOidcMemSettingRepo() *oidcMemSettingRepo {
	return &oidcMemSettingRepo{values: map[string]string{}}
}

func (r *oidcMemSettingRepo) Get(_ context.Context, key string) (*service.Setting, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.values[key]
	if !ok {
		return nil, service.ErrSettingNotFound
	}
	return &service.Setting{Key: key, Value: v}, nil
}

func (r *oidcMemSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return v, nil
}

func (r *oidcMemSettingRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[key] = value
	return nil
}

func (r *oidcMemSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		if v, ok := r.values[k]; ok {
			out[k] = v
		}
	}
	return out, nil
}

func (r *oidcMemSettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, v := range settings {
		r.values[k] = v
	}
	return nil
}

func (r *oidcMemSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(r.values))
	for k, v := range r.values {
		out[k] = v
	}
	return out, nil
}

func (r *oidcMemSettingRepo) Delete(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.values, key)
	return nil
}

// oidcHandlerTestEnv 聚合 handler 测试所需依赖。
type oidcHandlerTestEnv struct {
	client   *dbent.Client
	repo     *oidcMemSettingRepo
	provider *service.OidcProviderService
	clients  *service.OidcClientService
	sso      *service.SsoSessionService
	router   *gin.Engine
}

func newOidcHandlerTestEnv(t *testing.T, enabled bool) *oidcHandlerTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	name := strings.ReplaceAll(t.Name(), "/", "_")
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", name)
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	repo := newOidcMemSettingRepo()
	if enabled {
		require.NoError(t, repo.Set(context.Background(), service.SettingKeyOidcProviderEnabled, "true"))
	}
	require.NoError(t, repo.Set(context.Background(), service.SettingKeyOidcProviderIssuerURL, "https://op.example.com"))

	signing := service.NewOidcSigningService(client, repo)
	require.NoError(t, signing.EnsureActiveKey(context.Background()))
	clientSvc := service.NewOidcClientService(client)
	consentSvc := service.NewOidcConsentService(client)
	provider := service.NewOidcProviderService(client, repo, signing, clientSvc, consentSvc)
	sso := service.NewSsoSessionService(client, repo)

	h := NewOidcProviderHandler(provider, sso, nil)

	r := gin.New()
	r.GET("/.well-known/openid-configuration", h.Discovery)
	r.GET("/.well-known/jwks.json", h.JWKS)
	r.GET("/oidc/authorize", h.Authorize)
	r.POST("/oidc/token", h.Token)
	r.GET("/oidc/userinfo", h.UserInfo)

	return &oidcHandlerTestEnv{
		client:   client,
		repo:     repo,
		provider: provider,
		clients:  clientSvc,
		sso:      sso,
		router:   r,
	}
}

func pkceS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// createRP 创建一个测试 RP，返回 view 与明文 secret。
func (e *oidcHandlerTestEnv) createRP(t *testing.T, scopes []string) (*service.OidcClientView, string) {
	t.Helper()
	view, secret, err := e.clients.Create(context.Background(), service.CreateOidcClientRequest{
		ClientName:      "RP",
		RedirectURIs:    []string{"https://rp.example.com/cb"},
		AllowedScopes:   scopes,
		ConsentRequired: false,
		Enabled:         true,
	})
	require.NoError(t, err)
	return view, secret
}

func (e *oidcHandlerTestEnv) createUser(t *testing.T) int64 {
	t.Helper()
	u, err := e.client.User.Create().
		SetEmail("bob@example.com").
		SetUsername("bob").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		SetBalance(5).
		SetConcurrency(1).
		Save(context.Background())
	require.NoError(t, err)
	return u.ID
}

// ─── Discovery / JWKS ────────────────────────────────────────────────────────

func TestOidcHandler_Discovery_404WhenDisabled(t *testing.T) {
	e := newOidcHandlerTestEnv(t, false)
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/.well-known/openid-configuration", nil))
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestOidcHandler_Discovery_OKWhenEnabled(t *testing.T) {
	e := newOidcHandlerTestEnv(t, true)
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/.well-known/openid-configuration", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &doc))
	require.Equal(t, "https://op.example.com", doc["issuer"])
}

func TestOidcHandler_JWKS_OKWhenEnabled(t *testing.T) {
	e := newOidcHandlerTestEnv(t, true)
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Keys []map[string]any `json:"keys"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.GreaterOrEqual(t, len(body.Keys), 1)
	for _, k := range body.Keys {
		_, hasPrivate := k["d"]
		require.False(t, hasPrivate, "JWKS 不得泄露私钥")
	}
}

// ─── Token Endpoint ──────────────────────────────────────────────────────────

func (e *oidcHandlerTestEnv) issueCode(t *testing.T, rp *service.OidcClientView, userID int64, scopes []string, verifier string) string {
	t.Helper()
	code, err := e.provider.IssueCode(context.Background(), service.OidcIssueCodeInput{
		Client:      rp,
		UserID:      userID,
		Scopes:      scopes,
		RedirectURI: "https://rp.example.com/cb",
		Challenge:   pkceS256(verifier),
		Method:      "S256",
	})
	require.NoError(t, err)
	return code
}

func (e *oidcHandlerTestEnv) postToken(form url.Values) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oidc/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	e.router.ServeHTTP(w, req)
	return w
}

func TestOidcHandler_Token_404WhenDisabled(t *testing.T) {
	e := newOidcHandlerTestEnv(t, false)
	w := e.postToken(url.Values{"grant_type": {"authorization_code"}})
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestOidcHandler_Token_AuthorizationCodeSucceeds(t *testing.T) {
	e := newOidcHandlerTestEnv(t, true)
	rp, secret := e.createRP(t, []string{"openid", "profile", "offline_access"})
	userID := e.createUser(t)
	verifier := "verifier-0123456789-0123456789-0123456789"
	code := e.issueCode(t, rp, userID, []string{"openid", "profile", "offline_access"}, verifier)

	w := e.postToken(url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {"https://rp.example.com/cb"},
		"code_verifier": {verifier},
		"client_id":     {rp.ClientID},
		"client_secret": {secret},
	})
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))

	var resp service.OidcTokenResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.AccessToken)
	require.NotEmpty(t, resp.RefreshToken)
	require.NotEmpty(t, resp.IDToken)
}

func TestOidcHandler_Token_RefreshGrant(t *testing.T) {
	e := newOidcHandlerTestEnv(t, true)
	rp, secret := e.createRP(t, []string{"openid", "offline_access"})
	userID := e.createUser(t)
	verifier := "verifier-0123456789-0123456789-0123456789"
	code := e.issueCode(t, rp, userID, []string{"openid", "offline_access"}, verifier)

	w := e.postToken(url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {"https://rp.example.com/cb"},
		"code_verifier": {verifier},
		"client_id":     {rp.ClientID},
		"client_secret": {secret},
	})
	require.Equal(t, http.StatusOK, w.Code)
	var first service.OidcTokenResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &first))

	w2 := e.postToken(url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {first.RefreshToken},
		"client_id":     {rp.ClientID},
		"client_secret": {secret},
	})
	require.Equal(t, http.StatusOK, w2.Code)
	var second service.OidcTokenResponse
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &second))
	require.NotEmpty(t, second.AccessToken)
	require.NotEqual(t, first.RefreshToken, second.RefreshToken)
}

func TestOidcHandler_Token_InvalidClient(t *testing.T) {
	e := newOidcHandlerTestEnv(t, true)
	rp, _ := e.createRP(t, []string{"openid"})

	w := e.postToken(url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"x"},
		"client_id":     {rp.ClientID},
		"client_secret": {"wrong-secret"},
	})
	require.Equal(t, http.StatusUnauthorized, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "invalid_client", body["error"])
}

func TestOidcHandler_Token_UnsupportedGrant(t *testing.T) {
	e := newOidcHandlerTestEnv(t, true)
	rp, secret := e.createRP(t, []string{"openid"})
	w := e.postToken(url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {rp.ClientID},
		"client_secret": {secret},
	})
	require.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "unsupported_grant_type", body["error"])
}

// ─── UserInfo Endpoint ───────────────────────────────────────────────────────

func TestOidcHandler_UserInfo_BearerReturnsClaims(t *testing.T) {
	e := newOidcHandlerTestEnv(t, true)
	rp, secret := e.createRP(t, []string{"openid", "profile", "email"})
	userID := e.createUser(t)
	verifier := "verifier-0123456789-0123456789-0123456789"
	code := e.issueCode(t, rp, userID, []string{"openid", "profile", "email"}, verifier)

	w := e.postToken(url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {"https://rp.example.com/cb"},
		"code_verifier": {verifier},
		"client_id":     {rp.ClientID},
		"client_secret": {secret},
	})
	require.Equal(t, http.StatusOK, w.Code)
	var tok service.OidcTokenResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &tok))

	uw := httptest.NewRecorder()
	ureq := httptest.NewRequest(http.MethodGet, "/oidc/userinfo", nil)
	ureq.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	e.router.ServeHTTP(uw, ureq)
	require.Equal(t, http.StatusOK, uw.Code)

	var claims map[string]any
	require.NoError(t, json.Unmarshal(uw.Body.Bytes(), &claims))
	require.Equal(t, "bob", claims["name"])
	require.Equal(t, "bob@example.com", claims["email"])
}

func TestOidcHandler_UserInfo_MissingTokenChallenges(t *testing.T) {
	e := newOidcHandlerTestEnv(t, true)
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/oidc/userinfo", nil))
	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Contains(t, w.Header().Get("WWW-Authenticate"), "Bearer")
}

func TestOidcHandler_UserInfo_InvalidToken(t *testing.T) {
	e := newOidcHandlerTestEnv(t, true)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/oidc/userinfo", nil)
	req.Header.Set("Authorization", "Bearer not-real")
	e.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Contains(t, w.Header().Get("WWW-Authenticate"), `error="invalid_token"`)
}

// ─── Authorize Endpoint (未登录跳转) ────────────────────────────────────────

func TestOidcHandler_Authorize_RedirectsToLoginWhenNoSession(t *testing.T) {
	e := newOidcHandlerTestEnv(t, true)
	rp, _ := e.createRP(t, []string{"openid"})

	q := url.Values{
		"client_id":      {rp.ClientID},
		"redirect_uri":   {"https://rp.example.com/cb"},
		"response_type":  {"code"},
		"scope":          {"openid"},
		"code_challenge": {pkceS256("verifier-0123456789-0123456789")},
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/oidc/authorize?"+q.Encode(), nil))
	require.Equal(t, http.StatusFound, w.Code)
	require.Contains(t, w.Header().Get("Location"), "/login?redirect=")
}

func TestOidcHandler_Authorize_RejectsBadRedirectURI(t *testing.T) {
	e := newOidcHandlerTestEnv(t, true)
	rp, _ := e.createRP(t, []string{"openid"})
	q := url.Values{
		"client_id":      {rp.ClientID},
		"redirect_uri":   {"https://evil.example.com/cb"},
		"response_type":  {"code"},
		"scope":          {"openid"},
		"code_challenge": {pkceS256("v")},
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/oidc/authorize?"+q.Encode(), nil))
	// 不可信 redirect_uri → 直接 JSON 错误 (不回跳)
	require.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "invalid_request", body["error"])
}

// ─── Resource: ListAPIKeys (鉴权分支) ───────────────────────────────────────
//
// 完整成功路径需要装配真实 APIKeyService（含 repo 图），由集成测试覆盖；
// 此处仅覆盖 OIDC 鉴权与 scope 控制分支。

// newOidcHandlerTestEnvWithResource 构造一个带 ListAPIKeys 路由的测试 env
// （apiKeyService 注入为 nil，仅用于触发鉴权前的 401 / 403 分支）。
func newOidcHandlerTestEnvWithResource(t *testing.T, enabled bool) *oidcHandlerTestEnv {
	t.Helper()
	e := newOidcHandlerTestEnv(t, enabled)
	h := NewOidcProviderHandler(e.provider, e.sso, nil)
	e.router.GET("/oidc/resource/api-keys", h.ListAPIKeys)
	return e
}

func TestOidcHandler_ListAPIKeys_MissingTokenReturns401(t *testing.T) {
	e := newOidcHandlerTestEnvWithResource(t, true)
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/oidc/resource/api-keys", nil))
	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Contains(t, w.Header().Get("WWW-Authenticate"), "Bearer")
}

func TestOidcHandler_ListAPIKeys_InvalidTokenReturns401(t *testing.T) {
	e := newOidcHandlerTestEnvWithResource(t, true)
	req := httptest.NewRequest(http.MethodGet, "/oidc/resource/api-keys", nil)
	req.Header.Set("Authorization", "Bearer not-real")
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Contains(t, w.Header().Get("WWW-Authenticate"), `error="invalid_token"`)
}

func TestOidcHandler_ListAPIKeys_DisabledProviderReturns404(t *testing.T) {
	e := newOidcHandlerTestEnvWithResource(t, false)
	req := httptest.NewRequest(http.MethodGet, "/oidc/resource/api-keys", nil)
	req.Header.Set("Authorization", "Bearer whatever")
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestOidcHandler_ListAPIKeys_InsufficientScopeReturns403(t *testing.T) {
	e := newOidcHandlerTestEnvWithResource(t, true)

	// 直接注入一条仅含 "openid profile" scope 的 access_token 行，
	// 触发 resolveOidcBearer 中的 insufficient_scope 分支。
	ctx := context.Background()
	tokenVal := "test-access-token-no-apikey-scope"
	_, err := e.client.OidcAccessToken.Create().
		SetToken(tokenVal).
		SetClientID("rp-test").
		SetUserID(1).
		SetScopes([]string{"openid", "profile"}).
		SetRefreshFamilyID("").
		SetExpiresAt(timeFromNow(3600)).
		Save(ctx)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/oidc/resource/api-keys", nil)
	req.Header.Set("Authorization", "Bearer "+tokenVal)
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Header().Get("WWW-Authenticate"), `error="insufficient_scope"`)
	require.Contains(t, w.Header().Get("WWW-Authenticate"), `scope="sub2api:apikey"`)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "insufficient_scope", body["error"])
}

func TestOidcHandler_ListAPIKeys_ScopeOKButServiceMissingReturns500(t *testing.T) {
	e := newOidcHandlerTestEnvWithResource(t, true)

	// 注入一条带 sub2api:apikey scope 的 access_token；apiKeys==nil 触发 500 兜底分支。
	ctx := context.Background()
	tokenVal := "test-access-token-with-apikey-scope"
	_, err := e.client.OidcAccessToken.Create().
		SetToken(tokenVal).
		SetClientID("rp-test").
		SetUserID(1).
		SetScopes([]string{"openid", "sub2api:apikey"}).
		SetRefreshFamilyID("").
		SetExpiresAt(timeFromNow(3600)).
		Save(ctx)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/oidc/resource/api-keys", nil)
	req.Header.Set("Authorization", "Bearer "+tokenVal)
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "server_error", body["error"])
}
