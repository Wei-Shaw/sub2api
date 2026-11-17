//go:build unit

package handler

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const handlerMCPResource = "https://api.example.com/api/v1/mcp"

type handlerMCPRepo struct {
	service.MCPOAuthRepository
	clientID      string
	reads, writes int
	grant         *service.MCPOAuthGrant
}

// Resolving a client ID needs its signing key, so key loads count as reads.
func (r *handlerMCPRepo) ClientIDKey(context.Context) ([]byte, error) {
	r.reads++
	return []byte("handler-test-mcp-client-id-key-0"), nil
}
func (r *handlerMCPRepo) CreateAuthorization(_ context.Context, grant *service.MCPOAuthGrant, _ *service.MCPOAuthCode, _ int) error {
	r.writes++
	r.grant = grant
	return nil
}

type handlerMCPUsers struct {
	service.UserRepository
	user *service.User
}

func (r *handlerMCPUsers) GetByID(context.Context, int64) (*service.User, error) { return r.user, nil }
func (r *handlerMCPUsers) GetUserAvatar(context.Context, int64) (*service.UserAvatar, error) {
	return nil, nil
}

type handlerMCPSettings struct {
	service.SettingRepository
	values map[string]string
}

func (r *handlerMCPSettings) GetValue(_ context.Context, key string) (string, error) {
	return r.values[key], nil
}

type handlerMCPCache struct {
	service.TotpCache
	granted bool
	err     error
	calls   int
	userID  int64
	session string
}

func (r *handlerMCPCache) HasStepUpGrant(_ context.Context, id int64, session string) (bool, error) {
	r.calls++
	r.userID, r.session = id, session
	return r.granted, r.err
}
func newHandlerMCPFixture(t *testing.T) (*MCPOAuthHandler, *handlerMCPRepo, *handlerMCPUsers, *handlerMCPCache) {
	t.Helper()
	cfg := &config.Config{MCP: config.MCPConfig{PublicURL: handlerMCPResource}}
	cfg.Server.FrontendURL = "https://platform.example.com"
	repo := &handlerMCPRepo{}
	users := &handlerMCPUsers{user: &service.User{ID: 7, Role: service.RoleAdmin, Status: service.StatusActive, TotpEnabled: true}}
	settings := service.NewSettingService(&handlerMCPSettings{values: map[string]string{service.SettingKeyStepUpEnabled: "true"}}, cfg)
	cache := &handlerMCPCache{}
	totp := service.NewTotpService(users, nil, cache, settings, nil, nil)
	oauth := service.NewMCPOAuthService(repo, users)
	client, err := oauth.Register(context.Background(), service.MCPOAuthRegisterInput{ClientName: "Example assistant", RedirectURIs: []string{"https://client.example.com/callback?workspace=one"}})
	require.NoError(t, err)
	repo.clientID, repo.reads = client.ClientID, 0
	return NewMCPOAuthHandler(oauth, totp, service.NewUserService(users, nil, nil, nil), settings, cfg), repo, users, cache
}
func handlerMCPContext(t *testing.T, method, target, body, contentType string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	if contentType != "" {
		c.Request.Header.Set("Content-Type", contentType)
	}
	return c, rec
}
func handlerMCPSubject(c *gin.Context, role string) {
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
	c.Set(string(middleware.ContextKeyUserRole), role)
	c.Set(middleware.ContextKeySessionID, "browser-session-7")
}
func handlerMCPAuthorization(clientID string) service.MCPOAuthAuthorizeInput {
	digest := sha256.Sum256([]byte(strings.Repeat("v", 43)))
	return service.MCPOAuthAuthorizeInput{ResponseType: "code", ClientID: clientID, RedirectURI: "https://client.example.com/callback?workspace=one", Scope: "mcp", State: "a+b&c=d", CodeChallenge: base64.RawURLEncoding.EncodeToString(digest[:]), CodeChallengeMethod: "S256", Resource: handlerMCPResource}
}
func handlerMCPBody(t *testing.T, input any) string {
	t.Helper()
	raw, err := json.Marshal(input)
	require.NoError(t, err)
	return string(raw)
}
func handlerMCPJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body
}

func TestMCPOAuthHandlerResourceURLTrustedConfiguration(t *testing.T) {
	for _, tt := range []struct{ name, public, frontend, stored, want string }{
		{"explicit API origin", handlerMCPResource, "https://platform.example.com", "", handlerMCPResource},
		{"normalize explicit", "  https://API.EXAMPLE.COM/api/v1/mcp  ", "", "", handlerMCPResource},
		{"explicit resource wins", handlerMCPResource, "", "https://stored.example.com", handlerMCPResource},
		{"loopback development", "http://127.0.0.1:8080/api/v1/mcp", "", "", "http://127.0.0.1:8080/api/v1/mcp"},
		{"missing configuration", "", "", "", ""},
		// A configured frontend must not implicitly enable MCP after an upgrade.
		{"configured frontend stays disabled", "", "https://platform.example.com", "", ""},
		{"stored frontend stays disabled", "", "", "https://stored.example.com", ""},
		{"insecure public URL", "http://api.example.com/api/v1/mcp", "", "", ""},
		{"wrong resource path", "https://api.example.com/mcp", "", "", ""},
		{"sub-path deployment", "https://example.com/sub2api/api/v1/mcp", "", "", ""},
		{"resource userinfo", "https://user:password@api.example.com/api/v1/mcp", "", "", ""},
		{"resource query", handlerMCPResource + "?q=1", "", "", ""},
		{"resource fragment", handlerMCPResource + "#x", "", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{MCP: config.MCPConfig{PublicURL: tt.public}}
			cfg.Server.FrontendURL = tt.frontend
			var settings *service.SettingService
			if tt.stored != "" {
				settings = service.NewSettingService(&handlerMCPSettings{values: map[string]string{service.SettingKeyFrontendURL: tt.stored}}, cfg)
			}
			h := NewMCPOAuthHandler(nil, nil, nil, settings, cfg)
			got, err := h.ResourceURL(context.Background())
			if tt.want == "" {
				require.Error(t, err)
				require.Empty(t, got)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestMCPOAuthHandlerMetadataRawAndHostIndependent(t *testing.T) {
	h, _, _, _ := newHandlerMCPFixture(t)
	for _, auth := range []bool{false, true} {
		c, rec := handlerMCPContext(t, http.MethodGet, "https://attacker.example/.well-known/oauth-authorization-server", "", "")
		c.Request.Header.Set("X-Forwarded-Host", "forwarded-attacker.example")
		c.Request.Header.Set("X-Forwarded-Proto", "http")
		if auth {
			h.AuthorizationMetadata(c)
		} else {
			h.ResourceMetadata(c)
		}
		require.Equal(t, http.StatusOK, rec.Code)
		body := handlerMCPJSON(t, rec)
		require.NotContains(t, body, "data")
		require.NotContains(t, body, "code")
		require.NotContains(t, rec.Body.String(), "attacker")
		require.Equal(t, []any{"mcp"}, body["scopes_supported"])
		if auth {
			require.Equal(t, "https://api.example.com", body["issuer"])
			require.Equal(t, "https://platform.example.com/oauth/mcp/authorize", body["authorization_endpoint"])
			require.Equal(t, "https://api.example.com/api/v1/mcp/oauth/token", body["token_endpoint"])
			require.Equal(t, "https://api.example.com/api/v1/mcp/oauth/register", body["registration_endpoint"])
			require.Equal(t, "https://api.example.com/api/v1/mcp/oauth/revoke", body["revocation_endpoint"])
			require.Equal(t, []any{"S256"}, body["code_challenge_methods_supported"])
			require.Equal(t, []any{"none"}, body["token_endpoint_auth_methods_supported"])
		} else {
			require.Equal(t, handlerMCPResource, body["resource"])
			require.Equal(t, []any{"https://api.example.com"}, body["authorization_servers"])
			require.Equal(t, []any{"header"}, body["bearer_methods_supported"])
		}
	}
	h = NewMCPOAuthHandler(nil, nil, nil, nil, &config.Config{})
	for _, serve := range []gin.HandlerFunc{h.ResourceMetadata, h.AuthorizationMetadata} {
		c, rec := handlerMCPContext(t, http.MethodGet, "https://attacker.example/metadata", "", "")
		serve(c)
		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
		require.Equal(t, "server_error", handlerMCPJSON(t, rec)["error"])
		require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	}
}

func TestMCPOAuthHandlerConnectionConfigUsesCanonicalResource(t *testing.T) {
	h, repo, _, _ := newHandlerMCPFixture(t)
	c, rec := handlerMCPContext(t, http.MethodGet, "https://attacker.example/user/mcp/config", "", "")
	c.Request.Header.Set("X-Forwarded-Host", "forwarded-attacker.example")
	handlerMCPSubject(c, service.RoleUser)
	h.ConnectionConfig(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	require.Equal(t, map[string]any{"resource": handlerMCPResource}, handlerMCPJSON(t, rec)["data"])
	require.NotContains(t, rec.Body.String(), "attacker")
	require.Zero(t, repo.reads)
	require.Zero(t, repo.writes)

	h = NewMCPOAuthHandler(nil, nil, nil, nil, &config.Config{})
	c, rec = handlerMCPContext(t, http.MethodGet, "/user/mcp/config", "", "")
	handlerMCPSubject(c, service.RoleUser)
	h.ConnectionConfig(c)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	require.Contains(t, handlerMCPJSON(t, rec), "code")
}

func TestMCPOAuthHandlerAuthorizationMetadataValidatesFrontendURL(t *testing.T) {
	for _, tt := range []struct{ frontend, want string }{
		{"https://platform.example.com/console/", "https://platform.example.com/console/oauth/mcp/authorize"},
		{"http://127.0.0.1:3000/", "http://127.0.0.1:3000/oauth/mcp/authorize"},
		{"", "https://api.example.com/oauth/mcp/authorize"},
		{"http://platform.example.com", ""}, {"https://user:password@platform.example.com", ""},
		{"https://platform.example.com?next=attacker", ""}, {"https://platform.example.com#fragment", ""},
		{"https://platform.example.com?", ""}, {"https://platform.example.com#", ""},
	} {
		t.Run(tt.frontend, func(t *testing.T) {
			cfg := &config.Config{MCP: config.MCPConfig{PublicURL: handlerMCPResource}}
			cfg.Server.FrontendURL = tt.frontend
			h := NewMCPOAuthHandler(nil, nil, nil, nil, cfg)
			c, rec := handlerMCPContext(t, http.MethodGet, "https://attacker.example/metadata", "", "")
			h.AuthorizationMetadata(c)
			if tt.want == "" {
				require.Equal(t, http.StatusServiceUnavailable, rec.Code)
				require.Equal(t, "server_error", handlerMCPJSON(t, rec)["error"])
			} else {
				require.Equal(t, http.StatusOK, rec.Code)
				require.Equal(t, tt.want, handlerMCPJSON(t, rec)["authorization_endpoint"])
			}
		})
	}
}

func TestMCPOAuthHandlerJSONValidation(t *testing.T) {
	for _, tt := range []struct {
		name, contentType, body string
		browser, valid          bool
	}{
		{"valid browser", "application/json; charset=utf-8", `{"approved":false,"authorization":{"client_id":"client-1"}}`, true, true},
		{"unknown browser field", "application/json", `{"approved":false,"unexpected":true}`, true, false},
		{"unknown nested field", "application/json", `{"authorization":{"unexpected":true}}`, true, false},
		{"wrong type", "application/json", `{"approved":"true"}`, true, false},
		{"two documents", "application/json", `{} {}`, true, false},
		{"trailing bytes", "application/json", `{} extra`, true, false},
		{"truncated", "application/json", `{"approved":`, true, false},
		{"wrong content type", "text/plain", `{}`, true, false},
		{"missing content type", "", `{}`, true, false},
		{"oversized browser", "application/json", `{"authorization":{"state":"` + strings.Repeat("x", 17<<10) + `"}}`, true, false},
		{"registration extension metadata", "application/json", `{"client_name":"example","client_uri":"https://client.example.com"}`, false, true},
		{"oversized registration", "application/json", `{"client_name":"` + strings.Repeat("x", 17<<10) + `"}`, false, false},
		{"registration above 4 KiB", "application/json", `{"client_name":"x","client_uri":"` + strings.Repeat("x", 4<<10) + `"}`, false, false},
		{"browser body above registration limit", "application/json", `{"authorization":{"state":"` + strings.Repeat("x", 8<<10) + `"}}`, true, true},
		{"malformed registration", "application/json", `{`, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, rec := handlerMCPContext(t, http.MethodPost, "/oauth", tt.body, tt.contentType)
			var input any = &service.MCPOAuthRegisterInput{}
			if tt.browser {
				input = &service.MCPOAuthConsentInput{}
			}
			require.Equal(t, tt.valid, mcpOAuthReadJSON(c, input, tt.browser))
			if !tt.valid {
				require.Equal(t, http.StatusBadRequest, rec.Code)
				body := handlerMCPJSON(t, rec)
				if tt.browser {
					require.Contains(t, body, "code")
				} else {
					require.Equal(t, "invalid_request", body["error"])
					require.NotContains(t, body, "data")
				}
			}
		})
	}
}

func TestMCPOAuthHandlerFormValidation(t *testing.T) {
	for _, tt := range []struct{ name, contentType, body string }{
		{"JSON", "application/json", `{}`}, {"missing content type", "", "client_id=client-1"},
		{"repeated client", "application/x-www-form-urlencoded", "client_id=one&client_id=two"},
		{"repeated code", "application/x-www-form-urlencoded", "code=one&code=two"},
		{"invalid escape", "application/x-www-form-urlencoded", "client_id=%zz"},
		{"oversized", "application/x-www-form-urlencoded", "code=" + strings.Repeat("x", 17<<10)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, rec := handlerMCPContext(t, http.MethodPost, "/oauth/token", tt.body, tt.contentType)
			_, ok := mcpOAuthReadForm(c)
			require.False(t, ok)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, "invalid_request", handlerMCPJSON(t, rec)["error"])
			require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
		})
	}
	c, _ := handlerMCPContext(t, http.MethodPost, "/oauth/token?client_id=attacker&code=query-code", "client_id=client-1&code=body-code", "application/x-www-form-urlencoded; charset=utf-8")
	values, ok := mcpOAuthReadForm(c)
	require.True(t, ok)
	require.Equal(t, url.Values{"client_id": {"client-1"}, "code": {"body-code"}}, values)
}

func TestMCPOAuthHandlerTokenEndpointsRejectConfidentialAuthentication(t *testing.T) {
	for _, endpoint := range []string{"token", "revoke"} {
		for _, auth := range []string{"authorization", "client_secret"} {
			t.Run(endpoint+"/"+auth, func(t *testing.T) {
				h, repo, _, _ := newHandlerMCPFixture(t)
				values := url.Values{"client_id": {"client-1"}}
				if auth == "client_secret" {
					values.Set("client_secret", "not-supported")
				}
				c, rec := handlerMCPContext(t, http.MethodPost, "/oauth/"+endpoint, values.Encode(), "application/x-www-form-urlencoded")
				if auth == "authorization" {
					c.Request.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
				}
				if endpoint == "token" {
					h.Token(c)
				} else {
					h.RevokeToken(c)
				}
				require.Equal(t, http.StatusBadRequest, rec.Code)
				require.Equal(t, "invalid_client", handlerMCPJSON(t, rec)["error"])
				require.Zero(t, repo.reads)
				require.Zero(t, repo.writes)
			})
		}
	}
}

func TestMCPOAuthHandlerPreviewFrontendContract(t *testing.T) {
	h, repo, _, _ := newHandlerMCPFixture(t)
	input := handlerMCPAuthorization(repo.clientID)
	query := url.Values{"response_type": {input.ResponseType}, "client_id": {input.ClientID}, "redirect_uri": {input.RedirectURI}, "scope": {input.Scope}, "state": {input.State}, "code_challenge": {input.CodeChallenge}, "code_challenge_method": {input.CodeChallengeMethod}, "resource": {input.Resource}, "timezone": {"Asia/Shanghai"}}
	c, rec := handlerMCPContext(t, http.MethodGet, "/user/mcp/authorize?"+query.Encode(), "", "")
	handlerMCPSubject(c, service.RoleUser)
	h.Preview(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	body := handlerMCPJSON(t, rec)
	require.Equal(t, float64(0), body["code"])
	require.Equal(t, map[string]any{"client_id": repo.clientID, "client_name": "Example assistant", "redirect_uri": input.RedirectURI, "resource": handlerMCPResource, "scope": "mcp"}, body["data"])
	require.Zero(t, repo.writes)
	query.Add("redirect_uri", "https://attacker.example/callback")
	c, rec = handlerMCPContext(t, http.MethodGet, "/user/mcp/authorize?"+query.Encode(), "", "")
	handlerMCPSubject(c, service.RoleUser)
	h.Preview(c)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, 1, repo.reads, "reject duplicate parameters before service access")
}

func TestMCPOAuthHandlerAdminDecisionGatesBeforePersistence(t *testing.T) {
	for _, tt := range []struct {
		name, role, authMethod, marker string
		totp, granted                  bool
		cacheErr                       error
		status                         int
	}{
		{name: "non-admin", role: service.RoleUser, totp: true, granted: true, status: 403, marker: "Only administrators"},
		{name: "missing role", totp: true, granted: true, status: 403, marker: "Only administrators"},
		{name: "TOTP disabled", role: service.RoleAdmin, status: 403, marker: "STEP_UP_TOTP_NOT_ENABLED"},
		{name: "no recent grant", role: service.RoleAdmin, totp: true, status: 403, marker: "STEP_UP_REQUIRED"},
		{name: "step-up unavailable", role: service.RoleAdmin, totp: true, cacheErr: errors.New("cache unavailable"), status: 503, marker: "STEP_UP_UNAVAILABLE"},
		{name: "admin API key", role: service.RoleAdmin, authMethod: service.AuditAuthMethodAdminAPIKey, totp: true, granted: true, status: 403, marker: "STEP_UP_ADMIN_API_KEY_FORBIDDEN"},
		{name: "MCP credential", role: service.RoleAdmin, authMethod: service.AuditAuthMethodMCPOAuth, totp: true, granted: true, status: 403, marker: "STEP_UP_MCP_OAUTH_FORBIDDEN"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h, repo, users, cache := newHandlerMCPFixture(t)
			users.user.TotpEnabled = tt.totp
			cache.granted, cache.err = tt.granted, tt.cacheErr
			input := service.MCPOAuthConsentInput{Authorization: handlerMCPAuthorization(repo.clientID), Approved: true, Scope: service.MCPScopeAdmin}
			c, rec := handlerMCPContext(t, http.MethodPost, "/user/mcp/authorize", handlerMCPBody(t, input), "application/json")
			handlerMCPSubject(c, tt.role)
			c.Set("auth_method", tt.authMethod)
			h.Decide(c)
			require.Equal(t, tt.status, rec.Code)
			require.Contains(t, rec.Body.String(), tt.marker)
			require.Zero(t, repo.reads)
			require.Zero(t, repo.writes)
			require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
		})
	}
}

func TestMCPOAuthHandlerAdminWriteAlwaysRequiresStepUp(t *testing.T) {
	for _, tt := range []struct {
		name           string
		readOnly       *bool
		totp, granted  bool
		status, writes int
		marker         string
	}{
		{name: "write without TOTP", readOnly: boolPtr(false), status: 403, marker: "STEP_UP_TOTP_NOT_ENABLED"},
		{name: "write without recent verification", readOnly: boolPtr(false), totp: true, status: 403, marker: "STEP_UP_REQUIRED"},
		{name: "verified write", readOnly: boolPtr(false), totp: true, granted: true, status: 200, writes: 1},
		{name: "read-only follows the switch", readOnly: boolPtr(true), status: 200, writes: 1},
		{name: "default read-only follows the switch", status: 200, writes: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h, repo, users, cache := newHandlerMCPFixture(t)
			// Global step-up is disabled; write grants still need verification.
			h.settings = service.NewSettingService(&handlerMCPSettings{values: map[string]string{service.SettingKeyStepUpEnabled: "false"}}, h.cfg)
			users.user.TotpEnabled = tt.totp
			cache.granted = tt.granted
			input := service.MCPOAuthConsentInput{Authorization: handlerMCPAuthorization(repo.clientID), Approved: true, Scope: service.MCPScopeAdmin, ReadOnly: tt.readOnly}
			c, rec := handlerMCPContext(t, http.MethodPost, "/user/mcp/authorize", handlerMCPBody(t, input), "application/json")
			handlerMCPSubject(c, service.RoleAdmin)
			h.Decide(c)
			require.Equal(t, tt.status, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), tt.marker)
			require.Equal(t, tt.writes, repo.writes)
		})
	}
}

func TestMCPOAuthHandlerRegistrationPersistsNothing(t *testing.T) {
	h, repo, _, _ := newHandlerMCPFixture(t)
	c, rec := handlerMCPContext(t, http.MethodPost, "/api/v1/mcp/oauth/register", `{"client_name":"Example assistant","redirect_uris":["https://client.example.com/cb"]}`, "application/json")
	h.Register(c)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	body := handlerMCPJSON(t, rec)
	require.True(t, strings.HasPrefix(body["client_id"].(string), "mcp_client_"))
	require.NotEqual(t, repo.clientID, body["client_id"])
	require.Equal(t, []any{"https://client.example.com/cb"}, body["redirect_uris"])
	require.Zero(t, repo.writes)
}

func TestMCPOAuthHandlerAdminApprovalUsesBrowserSession(t *testing.T) {
	h, repo, _, cache := newHandlerMCPFixture(t)
	cache.granted = true
	readOnly := false
	input := service.MCPOAuthConsentInput{Authorization: handlerMCPAuthorization(repo.clientID), Approved: true, Scope: service.MCPScopeAdmin, ReadOnly: &readOnly}
	c, rec := handlerMCPContext(t, http.MethodPost, "/user/mcp/authorize", handlerMCPBody(t, input), "application/json")
	handlerMCPSubject(c, service.RoleAdmin)
	h.Decide(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(7), cache.userID)
	require.Equal(t, "browser-session-7", cache.session)
	require.Equal(t, 1, repo.writes)
	require.Equal(t, service.MCPScopeAdmin, repo.grant.Scope)
	require.False(t, repo.grant.ReadOnly)
	result := handlerMCPJSON(t, rec)["data"].(map[string]any)
	destination, err := url.Parse(result["redirect_url"].(string))
	require.NoError(t, err)
	require.Equal(t, "client.example.com", destination.Host)
	require.Equal(t, "one", destination.Query().Get("workspace"))
	require.Equal(t, input.Authorization.State, destination.Query().Get("state"))
	require.NotEmpty(t, destination.Query().Get("code"))
	require.Empty(t, rec.Header().Get("Location"), "return a validated URL without an automatic HTTP redirect")
}

func TestMCPOAuthHandlerDenialSkipsStepUpAndPersistence(t *testing.T) {
	h, repo, _, cache := newHandlerMCPFixture(t)
	cache.err = errors.New("must not be called")
	input := service.MCPOAuthConsentInput{Authorization: handlerMCPAuthorization(repo.clientID), Approved: false, Scope: service.MCPScopeAdmin}
	c, rec := handlerMCPContext(t, http.MethodPost, "/user/mcp/authorize", handlerMCPBody(t, input), "application/json")
	handlerMCPSubject(c, service.RoleUser)
	h.Decide(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Zero(t, cache.calls)
	require.Zero(t, repo.writes)
	result := handlerMCPJSON(t, rec)["data"].(map[string]any)
	destination, err := url.Parse(result["redirect_url"].(string))
	require.NoError(t, err)
	require.Equal(t, "client.example.com", destination.Host)
	require.Equal(t, "access_denied", destination.Query().Get("error"))
	require.Equal(t, input.Authorization.State, destination.Query().Get("state"))
	require.Empty(t, destination.Query().Get("code"))
}

func TestMCPOAuthHandlerBrowserEndpointsRequireSession(t *testing.T) {
	h, repo, _, _ := newHandlerMCPFixture(t)
	for _, serve := range []gin.HandlerFunc{h.ConnectionConfig, h.Preview, h.Decide, h.ListGrants, h.RevokeGrant} {
		c, rec := handlerMCPContext(t, http.MethodPost, "/user/mcp", `{}`, "application/json")
		serve(c)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	}
	require.Zero(t, repo.reads)
	require.Zero(t, repo.writes)
}
