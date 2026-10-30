package routes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// This repository exercises real OAuth services and HTTP handlers without a
// database. SQL locking/rollback behavior is covered by repository unit tests.
type mcpOAuthRouteRepo struct {
	mu          sync.Mutex
	grants      map[string]*service.MCPOAuthGrant
	codes       map[string]*service.MCPOAuthCode
	credentials map[string]*service.MCPOAuthCredential
}

func (r *mcpOAuthRouteRepo) ClientIDKey(context.Context) ([]byte, error) {
	return []byte("route-test-mcp-client-id-key-000"), nil
}

func (r *mcpOAuthRouteRepo) CreateAuthorization(_ context.Context, grant *service.MCPOAuthGrant, code *service.MCPOAuthCode, limit int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, item := range r.grants {
		if item.UserID == grant.UserID && item.RevokedAt == nil && item.ExpiresAt.After(grant.CreatedAt) {
			count++
		}
	}
	if count >= limit {
		return service.ErrMCPOAuthGrantLimit
	}
	r.grants[grant.ID], r.codes[code.Hash] = grant, code
	return nil
}

func (r *mcpOAuthRouteRepo) GetCode(_ context.Context, hash string) (*service.MCPOAuthCode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item := r.codes[hash]; item != nil {
		copy := *item
		return &copy, nil
	}
	return nil, service.ErrMCPOAuthNotFound
}

func (r *mcpOAuthRouteRepo) GetCredential(_ context.Context, hash string) (*service.MCPOAuthCredential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item := r.credentials[hash]; item != nil {
		copy := *item
		return &copy, nil
	}
	return nil, service.ErrMCPOAuthNotFound
}

func (r *mcpOAuthRouteRepo) GetGrant(_ context.Context, id string) (*service.MCPOAuthGrant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item := r.grants[id]; item != nil {
		copy := *item
		return &copy, nil
	}
	return nil, service.ErrMCPOAuthNotFound
}

func (r *mcpOAuthRouteRepo) consume(hash, id string, now time.Time, access, refresh *service.MCPOAuthCredential, rotating bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	grant := r.grants[id]
	if grant == nil || grant.RevokedAt != nil || !now.Before(grant.ExpiresAt) {
		return service.ErrMCPOAuthNotFound
	}
	var expiresAt time.Time
	var usedAt **time.Time
	if rotating {
		item := r.credentials[hash]
		if item == nil || item.GrantID != id || item.Kind != "refresh" {
			return service.ErrMCPOAuthNotFound
		}
		expiresAt, usedAt = item.ExpiresAt, &item.UsedAt
	} else {
		item := r.codes[hash]
		if item == nil || item.GrantID != id {
			return service.ErrMCPOAuthNotFound
		}
		expiresAt, usedAt = item.ExpiresAt, &item.ConsumedAt
	}
	if *usedAt != nil {
		grant.RevokedAt = &now
		return service.ErrMCPOAuthNotFound
	}
	if !now.Before(expiresAt) {
		return service.ErrMCPOAuthNotFound
	}
	*usedAt = &now
	r.credentials[access.Hash] = access
	if refresh != nil {
		r.credentials[refresh.Hash] = refresh
	}
	return nil
}

func (r *mcpOAuthRouteRepo) ExchangeCode(_ context.Context, hash, id string, now time.Time, access, refresh *service.MCPOAuthCredential) error {
	return r.consume(hash, id, now, access, refresh, false)
}

func (r *mcpOAuthRouteRepo) RotateRefresh(_ context.Context, hash, id string, now time.Time, access, refresh *service.MCPOAuthCredential) error {
	return r.consume(hash, id, now, access, refresh, true)
}

func (r *mcpOAuthRouteRepo) TouchGrant(_ context.Context, id string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item := r.grants[id]
	if item == nil || item.RevokedAt != nil || !now.Before(item.ExpiresAt) {
		return service.ErrMCPOAuthNotFound
	}
	item.LastUsedAt = &now
	return nil
}

func (r *mcpOAuthRouteRepo) ListGrants(_ context.Context, userID int64) ([]*service.MCPOAuthGrant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := []*service.MCPOAuthGrant{}
	for _, item := range r.grants {
		if item.UserID == userID {
			copy := *item
			items = append(items, &copy)
		}
	}
	return items, nil
}

func (r *mcpOAuthRouteRepo) RevokeGrant(_ context.Context, userID int64, id string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item := r.grants[id]
	if item == nil || item.UserID != userID {
		return service.ErrMCPOAuthNotFound
	}
	if item.RevokedAt == nil {
		item.RevokedAt = &now
	}
	return nil
}

func (r *mcpOAuthRouteRepo) RevokeUserGrants(_ context.Context, userID int64, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range r.grants {
		if item.UserID == userID && item.RevokedAt == nil {
			item.RevokedAt = &now
		}
	}
	return nil
}

type mcpOAuthRouteUsers struct {
	service.UserRepository
	user *service.User
}

func (r *mcpOAuthRouteUsers) GetByID(_ context.Context, id int64) (*service.User, error) {
	if r.user == nil || r.user.ID != id {
		return nil, service.ErrUserNotFound
	}
	copy := *r.user
	return &copy, nil
}

func (r *mcpOAuthRouteUsers) GetUserAvatar(context.Context, int64) (*service.UserAvatar, error) {
	return nil, nil
}

func newMCPRouteOAuthService(role string) (*service.MCPOAuthService, *mcpOAuthRouteRepo, *mcpOAuthRouteUsers) {
	repo := &mcpOAuthRouteRepo{
		grants: map[string]*service.MCPOAuthGrant{}, codes: map[string]*service.MCPOAuthCode{},
		credentials: map[string]*service.MCPOAuthCredential{},
	}
	users := &mcpOAuthRouteUsers{user: &service.User{ID: 7, Role: role, Status: service.StatusActive, PasswordHash: "test-password", Email: "mcp@example.com"}}
	return service.NewMCPOAuthService(repo, users), repo, users
}

const mcpOAuthRouteVerifier = "a-valid-43-character-pkce-verifier-for-testing-only"

func mcpOAuthRouteAuthorization(clientID, resource string) service.MCPOAuthAuthorizeInput {
	sum := sha256.Sum256([]byte(mcpOAuthRouteVerifier))
	return service.MCPOAuthAuthorizeInput{
		ResponseType: "code", ClientID: clientID, RedirectURI: "http://127.0.0.1:37777/callback",
		Scope: "mcp", State: "opaque-state-with&symbols", CodeChallenge: base64.RawURLEncoding.EncodeToString(sum[:]),
		CodeChallengeMethod: "S256", Resource: resource,
	}
}

func issueMCPRouteOAuthToken(t *testing.T, oauth *service.MCPOAuthService, resource, scope string, readOnly bool) string {
	t.Helper()
	client, err := oauth.Register(context.Background(), service.MCPOAuthRegisterInput{RedirectURIs: []string{"http://127.0.0.1:37777/callback"}})
	require.NoError(t, err)
	authorization := mcpOAuthRouteAuthorization(client.ClientID, resource)
	redirect, err := oauth.Decide(context.Background(), 7, service.MCPOAuthConsentInput{Authorization: authorization, Approved: true, Scope: scope, ReadOnly: &readOnly}, resource)
	require.NoError(t, err)
	u, err := url.Parse(redirect)
	require.NoError(t, err)
	tokens, err := oauth.Token(context.Background(), service.MCPOAuthTokenInput{GrantType: "authorization_code", ClientID: client.ClientID, Code: u.Query().Get("code"), RedirectURI: authorization.RedirectURI, CodeVerifier: mcpOAuthRouteVerifier, Resource: resource}, resource)
	require.NoError(t, err)
	return tokens.AccessToken
}

type mcpOAuthRouteStepUpCache struct{ service.TotpCache }

func (mcpOAuthRouteStepUpCache) HasStepUpGrant(_ context.Context, _ int64, session string) (bool, error) {
	return session == "browser-session", nil
}

type mcpOAuthHTTPFixture struct {
	server   *httptest.Server
	router   *gin.Engine
	resource string
	cfg      *config.Config
	settings *service.SettingService
	handlers *handler.Handlers
	oauth    *service.MCPOAuthService
	repo     *mcpOAuthRouteRepo
	users    *mcpOAuthRouteUsers
}

func newMCPOAuthHTTPFixture(t *testing.T, role string) *mcpOAuthHTTPFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	server := httptest.NewUnstartedServer(router)
	base := "http://" + server.Listener.Addr().String()
	cfg := &config.Config{MCP: config.MCPConfig{PublicURL: base + "/api/v1/mcp"}}
	cfg.Server.FrontendURL = base
	cfg.JWT.Secret = "test-jwt-secret-at-least-thirty-two-bytes"
	settings := service.NewSettingService(&mcpRouteSettingsRepo{values: map[string]string{service.SettingKeyStepUpEnabled: "false"}}, cfg)
	oauth, repo, users := newMCPRouteOAuthService(role)
	// Administrator write grants always pass the real step-up gate.
	users.user.TotpEnabled = true
	totp := service.NewTotpService(users, nil, mcpOAuthRouteStepUpCache{}, settings, nil, nil)
	h := mcpRouteHandlers()
	h.MCPOAuth = handler.NewMCPOAuthHandler(oauth, totp, service.NewUserService(users, nil, nil, nil), settings, cfg)
	h.APIKey = handler.NewAPIKeyHandler(service.NewAPIKeyService(&mcpRouteAPIKeyRepo{}, nil, nil, nil, nil, nil, nil))
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	browserAuth := middleware.JWTAuthMiddleware(func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer browser-session" {
			middleware.AbortWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Browser login required")
			return
		}
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		c.Set(string(middleware.ContextKeyUserRole), role)
		c.Set(middleware.ContextKeySessionID, "browser-session")
		c.Next()
	})
	v1 := router.Group("/api/v1")
	RegisterUserRoutes(v1, h, browserAuth, middleware.AuditLogMiddleware(mcpRouteNext), settings, nil)
	RegisterMCPOAuthRoutes(router, v1, h, rdb)
	RegisterMCPRoutes(v1, h, middleware.AuditLogMiddleware(mcpRouteNext), middleware.StepUpAuthMiddleware(mcpRouteNext), settings, nil, nil, rdb)
	server.Start()
	t.Cleanup(server.Close)
	return &mcpOAuthHTTPFixture{server: server, router: router, resource: cfg.MCP.PublicURL, cfg: cfg, settings: settings, handlers: h, oauth: oauth, repo: repo, users: users}
}

type mcpOAuthHTTPResult struct {
	status int
	header http.Header
	body   []byte
}

func (f *mcpOAuthHTTPFixture) request(t *testing.T, method, path, auth, contentType string, body []byte, spoofHost bool) mcpOAuthHTTPResult {
	t.Helper()
	req, err := http.NewRequest(method, f.server.URL+path, bytes.NewReader(body))
	require.NoError(t, err)
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	if spoofHost {
		req.Host = "attacker.example"
		req.Header.Set("X-Forwarded-Host", "attacker.example")
		req.Header.Set("X-Forwarded-Proto", "https")
	}
	res, err := f.server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return mcpOAuthHTTPResult{status: res.StatusCode, header: res.Header.Clone(), body: raw}
}

func (f *mcpOAuthHTTPFixture) json(t *testing.T, method, path, auth string, body any) mcpOAuthHTTPResult {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	return f.request(t, method, path, auth, "application/json", raw, false)
}

func (f *mcpOAuthHTTPFixture) form(t *testing.T, path string, body url.Values) mcpOAuthHTTPResult {
	t.Helper()
	return f.request(t, http.MethodPost, path, "", "application/x-www-form-urlencoded", []byte(body.Encode()), false)
}

func (f *mcpOAuthHTTPFixture) rpc(t *testing.T, access, method string, params any) mcpOAuthHTTPResult {
	t.Helper()
	return f.json(t, http.MethodPost, "/api/v1/mcp", access, map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
}

func (f *mcpOAuthHTTPFixture) authorize(t *testing.T, scope string, readOnly bool) (service.MCPOAuthClient, service.MCPOAuthTokenResponse) {
	t.Helper()
	registration := f.json(t, http.MethodPost, "/api/v1/mcp/oauth/register", "", map[string]any{
		"client_name": "Integration client", "redirect_uris": []string{"http://127.0.0.1:37777/callback"},
		"token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"},
	})
	require.Equal(t, http.StatusCreated, registration.status, string(registration.body))
	var client service.MCPOAuthClient
	require.NoError(t, json.Unmarshal(registration.body, &client))
	authorization := mcpOAuthRouteAuthorization(client.ClientID, f.resource)
	query := url.Values{"response_type": {authorization.ResponseType}, "client_id": {authorization.ClientID}, "redirect_uri": {authorization.RedirectURI}, "scope": {authorization.Scope}, "state": {authorization.State}, "code_challenge": {authorization.CodeChallenge}, "code_challenge_method": {authorization.CodeChallengeMethod}, "resource": {authorization.Resource}}
	before, err := f.oauth.ListGrants(context.Background(), 7)
	require.NoError(t, err)
	preview := f.request(t, http.MethodGet, "/api/v1/user/mcp/authorize?"+query.Encode(), "browser-session", "", nil, false)
	require.Equal(t, http.StatusOK, preview.status, string(preview.body))
	require.Contains(t, string(preview.body), "Integration client")
	afterPreview, err := f.oauth.ListGrants(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, afterPreview, len(before), "preview must not create an authorization")
	decided := f.json(t, http.MethodPost, "/api/v1/user/mcp/authorize", "browser-session", service.MCPOAuthConsentInput{Authorization: authorization, Approved: true, Scope: scope, ReadOnly: &readOnly})
	require.Equal(t, http.StatusOK, decided.status, string(decided.body))
	var consent struct {
		Data struct {
			RedirectURL string `json:"redirect_url"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(decided.body, &consent))
	redirect, err := url.Parse(consent.Data.RedirectURL)
	require.NoError(t, err)
	require.Equal(t, authorization.State, redirect.Query().Get("state"))
	require.Equal(t, "127.0.0.1:37777", redirect.Host)
	after, err := f.oauth.ListGrants(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, after, len(before)+1)
	token := f.form(t, "/api/v1/mcp/oauth/token", url.Values{"grant_type": {"authorization_code"}, "client_id": {client.ClientID}, "code": {redirect.Query().Get("code")}, "redirect_uri": {authorization.RedirectURI}, "code_verifier": {mcpOAuthRouteVerifier}, "resource": {f.resource}})
	require.Equal(t, http.StatusOK, token.status, string(token.body))
	require.Equal(t, "no-store", token.header.Get("Cache-Control"))
	var tokens service.MCPOAuthTokenResponse
	require.NoError(t, json.Unmarshal(token.body, &tokens))
	require.NotEmpty(t, tokens.AccessToken)
	require.NotEmpty(t, tokens.RefreshToken)
	return client, tokens
}

func TestMCPOAuthHTTPFlowDiscoveryConsentRefreshAndRevocation(t *testing.T) {
	f := newMCPOAuthHTTPFixture(t, service.RoleUser)
	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/api/v1/mcp"} {
		metadata := f.request(t, http.MethodGet, path, "", "", nil, true)
		require.Equal(t, http.StatusOK, metadata.status, string(metadata.body))
		var value struct {
			Resource             string   `json:"resource"`
			AuthorizationServers []string `json:"authorization_servers"`
		}
		require.NoError(t, json.Unmarshal(metadata.body, &value))
		require.Equal(t, f.resource, value.Resource)
		require.Equal(t, []string{f.server.URL}, value.AuthorizationServers)
		require.NotContains(t, string(metadata.body), "attacker.example")
	}
	authorizationMetadata := f.request(t, http.MethodGet, "/.well-known/oauth-authorization-server", "", "", nil, true)
	require.Equal(t, http.StatusOK, authorizationMetadata.status, string(authorizationMetadata.body))
	var endpoints map[string]any
	require.NoError(t, json.Unmarshal(authorizationMetadata.body, &endpoints))
	require.Equal(t, f.server.URL, endpoints["issuer"])
	require.Equal(t, f.server.URL+"/oauth/mcp/authorize", endpoints["authorization_endpoint"])
	require.Equal(t, f.server.URL+"/api/v1/mcp/oauth/token", endpoints["token_endpoint"])
	unauthorized := f.request(t, http.MethodPost, "/api/v1/mcp", "", "application/json", []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`), true)
	require.Equal(t, http.StatusUnauthorized, unauthorized.status)
	require.Contains(t, unauthorized.header.Get("WWW-Authenticate"), f.server.URL+"/.well-known/oauth-protected-resource/api/v1/mcp")
	require.NotContains(t, unauthorized.header.Get("WWW-Authenticate"), "attacker.example")
	client, tokens := f.authorize(t, service.MCPScopeUser, true)
	initialized := f.rpc(t, tokens.AccessToken, "initialize", map[string]any{
		"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "oauth-integration", "version": "1.0"},
	})
	require.Equal(t, http.StatusOK, initialized.status, string(initialized.body))
	require.Contains(t, string(initialized.body), "protocolVersion")
	tools := f.rpc(t, tokens.AccessToken, "tools/list", map[string]any{})
	require.Equal(t, http.StatusOK, tools.status, string(tools.body))
	require.Contains(t, string(tools.body), "user_get_api_key")
	require.NotContains(t, string(tools.body), "user_create_api_key")
	require.NotContains(t, string(tools.body), "admin_get_dashboard")
	call := f.rpc(t, tokens.AccessToken, "tools/call", map[string]any{"name": "user_get_api_key", "arguments": map[string]any{"id": 9}})
	require.Equal(t, http.StatusOK, call.status, string(call.body))
	result := decodeMCPRouteToolResult(t, call.body)
	require.Equal(t, http.StatusOK, result.Result.Payload.HTTPStatus)
	require.Contains(t, string(call.body), "owner-key")
	require.NotContains(t, string(call.body), "must-not-leak")
	refresh := f.form(t, "/api/v1/mcp/oauth/token", url.Values{"grant_type": {"refresh_token"}, "client_id": {client.ClientID}, "refresh_token": {tokens.RefreshToken}})
	require.Equal(t, http.StatusOK, refresh.status, string(refresh.body))
	var rotated service.MCPOAuthTokenResponse
	require.NoError(t, json.Unmarshal(refresh.body, &rotated))
	require.NotEqual(t, tokens.AccessToken, rotated.AccessToken)
	require.Equal(t, http.StatusOK, f.rpc(t, rotated.AccessToken, "tools/list", map[string]any{}).status)
	revoked := f.form(t, "/api/v1/mcp/oauth/revoke", url.Values{"client_id": {client.ClientID}, "token": {rotated.RefreshToken}})
	require.Equal(t, http.StatusOK, revoked.status, string(revoked.body))
	require.Equal(t, http.StatusUnauthorized, f.rpc(t, rotated.AccessToken, "tools/list", map[string]any{}).status)
	require.Equal(t, http.StatusUnauthorized, f.rpc(t, tokens.AccessToken, "tools/list", map[string]any{}).status)
}

func TestMCPOAuthHTTPPermissionAndCredentialBoundaries(t *testing.T) {
	f := newMCPOAuthHTTPFixture(t, service.RoleAdmin)
	_, user := f.authorize(t, service.MCPScopeUser, true)
	userTools := f.rpc(t, user.AccessToken, "tools/list", map[string]any{})
	require.NotContains(t, string(userTools.body), "admin_get_dashboard")
	_, admin := f.authorize(t, service.MCPScopeAdmin, false)
	adminTools := f.rpc(t, admin.AccessToken, "tools/list", map[string]any{})
	require.Equal(t, http.StatusOK, adminTools.status, string(adminTools.body))
	require.Contains(t, string(adminTools.body), "admin_get_dashboard")
	require.Contains(t, string(adminTools.body), "user_create_api_key")
	for _, tool := range []string{"admin_get_dashboard", "user_delete_api_key"} {
		blocked := f.rpc(t, user.AccessToken, "tools/call", map[string]any{"name": tool, "arguments": map[string]any{"id": 9}})
		var result mcpRouteToolResult
		require.NoError(t, json.Unmarshal(blocked.body, &result))
		require.NotEmpty(t, result.Error, string(blocked.body))
	}
	for _, credential := range []string{"browser-session", "sk-inference-key", "mcp_legacy_personal_token", admin.RefreshToken} {
		require.Equal(t, http.StatusUnauthorized, f.rpc(t, credential, "tools/list", map[string]any{}).status, credential)
	}
	// Use the real production JWT middleware to prove MCP access credentials
	// cannot become browser REST credentials, even for an administrator owner.
	rest := gin.New()
	auth := service.NewAuthService(nil, nil, nil, nil, f.cfg, f.settings, nil, nil, nil, nil, nil, nil, nil)
	jwtAuth := middleware.NewJWTAuthMiddleware(auth, nil, f.settings, nil)
	RegisterUserRoutes(rest.Group("/api/v1"), f.handlers, jwtAuth, middleware.AuditLogMiddleware(mcpRouteNext), f.settings, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/mcp/grants", nil)
	req.Header.Set("Authorization", "Bearer "+admin.AccessToken)
	w := httptest.NewRecorder()
	rest.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "INVALID_TOKEN")
}

func TestMCPOAuthHTTPConsentRequiresBrowserAndExplicitApproval(t *testing.T) {
	f := newMCPOAuthHTTPFixture(t, service.RoleUser)
	client, err := f.oauth.Register(context.Background(), service.MCPOAuthRegisterInput{RedirectURIs: []string{"http://127.0.0.1:37777/callback"}})
	require.NoError(t, err)
	input := service.MCPOAuthConsentInput{Authorization: mcpOAuthRouteAuthorization(client.ClientID, f.resource), Approved: true, Scope: service.MCPScopeAdmin}
	unauthenticated := f.json(t, http.MethodPost, "/api/v1/user/mcp/authorize", "", input)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.status)
	escalated := f.json(t, http.MethodPost, "/api/v1/user/mcp/authorize", "browser-session", input)
	require.Equal(t, http.StatusForbidden, escalated.status, string(escalated.body))
	input.Approved = false
	denied := f.json(t, http.MethodPost, "/api/v1/user/mcp/authorize", "browser-session", input)
	require.Equal(t, http.StatusOK, denied.status, string(denied.body))
	require.Contains(t, string(denied.body), "access_denied")
	grants, err := f.oauth.ListGrants(context.Background(), 7)
	require.NoError(t, err)
	require.Empty(t, grants)
	legacy := f.request(t, http.MethodPost, "/api/v1/user/mcp-tokens", "browser-session", "application/json", []byte(`{"name":"legacy"}`), false)
	require.Equal(t, http.StatusNotFound, legacy.status)
}

func TestMCPOAuthHTTPRejectsAmbiguousTokenForms(t *testing.T) {
	f := newMCPOAuthHTTPFixture(t, service.RoleUser)
	for _, form := range []string{
		"grant_type=authorization_code&client_id=first&client_id=second",
		"grant_type=authorization_code&resource=a&resource=b",
		"grant_type=authorization_code&client_secret=not-supported",
	} {
		result := f.request(t, http.MethodPost, "/api/v1/mcp/oauth/token", "", "application/x-www-form-urlencoded", []byte(form), false)
		require.Equal(t, http.StatusBadRequest, result.status, string(result.body))
		require.Equal(t, "no-store", result.header.Get("Cache-Control"))
		require.True(t, strings.Contains(string(result.body), "invalid_request") || strings.Contains(string(result.body), "invalid_client"))
	}
}
