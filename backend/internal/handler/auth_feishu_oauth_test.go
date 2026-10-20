//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type feishuTestTransport func(*http.Request) (*http.Response, error)

func (f feishuTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func feishuTestConfig() config.FeishuConnectConfig {
	return config.FeishuConnectConfig{Enabled: true, ClientID: "cli_test", ClientSecret: "private-client-secret", RedirectURL: "https://portal.example/api/v1/auth/oauth/feishu/callback", AllowedTenantKeys: []string{"tenant-a"}}
}

func TestFeishuJSONExchangeAndTenantAdmission(t *testing.T) {
	cases := []struct {
		name, token, profile string
		allowed              bool
	}{
		{"valid", `{"code":0,"access_token":"private-user-token"}`, `{"code":0,"data":{"tenant_key":"tenant-a","union_id":"stable-union","open_id":"stable-open","name":"Employee"}}`, true},
		{"wrong tenant", `{"code":0,"access_token":"private-user-token"}`, `{"code":0,"data":{"tenant_key":"tenant-b","union_id":"stable-union"}}`, false},
		{"missing tenant", `{"code":0,"access_token":"private-user-token"}`, `{"code":0,"data":{"open_id":"stable-open"}}`, false},
		{"missing subject", `{"code":0,"access_token":"private-user-token"}`, `{"code":0,"data":{"tenant_key":"tenant-a","email":"existing@example.test"}}`, false},
		{"token error", `{"code":100,"msg":"private-client-secret"}`, `{}`, false},
		{"missing status", `{"access_token":"private-user-token"}`, `{}`, false},
		{"userinfo error", `{"code":0,"access_token":"private-user-token"}`, `{"code":100,"msg":"private-user-token"}`, false},
		{"invalid json", `{"code":0,"access_token":"private-user-token"}`, `not-json`, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: feishuTestTransport(func(r *http.Request) (*http.Response, error) {
				body := test.profile
				switch r.URL.String() {
				case feishuTokenURL:
					require.Equal(t, http.MethodPost, r.Method)
					require.Equal(t, "application/json", r.Header.Get("Content-Type"))
					var payload map[string]string
					require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
					require.Equal(t, "private-client-secret", payload["client_secret"])
					require.Equal(t, "authorization-code", payload["code"])
					require.Equal(t, feishuTestConfig().RedirectURL, payload["redirect_uri"])
					body = test.token
				case feishuUserInfoURL:
					require.Equal(t, http.MethodGet, r.Method)
					require.Equal(t, "Bearer private-user-token", r.Header.Get("Authorization"))
				default:
					t.Fatalf("unexpected endpoint %s", r.URL.String())
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}
			profile, err := fetchFeishuProfile(context.Background(), client, feishuTestConfig(), "authorization-code")
			if test.allowed {
				require.NoError(t, err)
				require.Equal(t, "tenant-a", profile.TenantKey)
				return
			}
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private-client-secret")
			require.NotContains(t, err.Error(), "private-user-token")
		})
	}
}

func TestFeishuTokenRedirectCannotExfiltrateCredentials(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: feishuTestTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: 307, Header: http.Header{"Location": []string{"https://untrusted.example/token"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})}
	_, err := fetchFeishuProfile(context.Background(), client, feishuTestConfig(), "authorization-code")
	require.Error(t, err)
	require.Equal(t, 1, requests)
}

func TestFeishuCallbackRejectsInvalidStateBeforeProviderRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AuthHandler{cfg: &config.Config{Feishu: feishuTestConfig()}, feishuHTTPClient: &http.Client{Transport: feishuTestTransport(func(_ *http.Request) (*http.Response, error) {
		t.Fatal("provider must not be called for invalid state")
		return nil, nil
	})}}
	for _, query := range []string{"code=authorization-code&state=forged", "code=authorization-code", "state=forged"} {
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Request = httptest.NewRequest(http.MethodGet, "https://portal.example/api/v1/auth/oauth/feishu/callback?"+query, nil)
		c.Request.AddCookie(&http.Cookie{Name: "feishu_oauth_state", Value: encodeCookieValue("expected-state")})
		h.FeishuOAuthCallback(c)
		require.Equal(t, http.StatusFound, response.Code)
		redirect, err := url.Parse(response.Header().Get("Location"))
		require.NoError(t, err)
		require.Equal(t, feishuFrontendCallback, redirect.Path)
		require.Contains(t, redirect.Fragment, "invalid_state")
		for _, cookie := range response.Result().Cookies() {
			if strings.HasPrefix(cookie.Name, "feishu_oauth_") {
				require.Equal(t, -1, cookie.MaxAge)
			}
		}
	}
}

func TestFeishuStableCandidatesPreserveVerifiedFallbackAndTenantBoundary(t *testing.T) {
	old := feishuSubjectCandidates(feishuProfile{TenantKey: "tenant-a", OpenID: "stable-open"})
	updated := feishuSubjectCandidates(feishuProfile{TenantKey: "tenant-a", UnionID: "new-union", OpenID: "stable-open"})
	otherTenant := feishuSubjectCandidates(feishuProfile{TenantKey: "tenant-b", OpenID: "stable-open"})
	require.Contains(t, updated, old[0])
	require.NotEqual(t, old[0], otherTenant[0])
}

func TestFeishuCallbackUsesMappedIdentityWithoutRecreatingOrCreditingUser(t *testing.T) {
	h, client := newOAuthPendingFlowTestHandler(t, false)
	h.cfg = &config.Config{Feishu: feishuTestConfig()}
	ctx := context.Background()
	employee, err := client.User.Create().SetEmail("employee@example.test").SetPasswordHash("hash").SetUsername("Employee").SetRole("user").SetStatus("active").SetBalance(1837.125).Save(ctx)
	require.NoError(t, err)
	subject := feishuSubjectCandidates(feishuProfile{TenantKey: "tenant-a", OpenID: "verified-open"})[0]
	_, err = client.AuthIdentity.Create().SetUserID(employee.ID).SetProviderType("feishu").SetProviderKey("cli_test").SetProviderSubject(subject).Save(ctx)
	require.NoError(t, err)
	h.feishuHTTPClient = feishuProfileTestClient(`{"code":0,"data":{"tenant_key":"tenant-a","union_id":"new-union","open_id":"verified-open","name":"Updated name"}}`)
	recorder := runFeishuCallbackForTest(h)
	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, feishuFrontendCallback, recorder.Header().Get("Location"))
	session, err := client.PendingAuthSession.Query().Only(ctx)
	require.NoError(t, err)
	require.Equal(t, employee.ID, *session.TargetUserID)
	require.Equal(t, subject, session.ProviderSubject)
	require.Equal(t, "feishu", session.ProviderType)
	stored, err := client.User.Get(ctx, employee.ID)
	require.NoError(t, err)
	require.Equal(t, 1837.125, stored.Balance)
	require.Equal(t, "Employee", stored.Username)
	count, err := client.User.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestFeishuCallbackDoesNotClaimAnAccountByProviderEmail(t *testing.T) {
	h, client := newOAuthPendingFlowTestHandler(t, false)
	h.cfg = &config.Config{Feishu: feishuTestConfig()}
	ctx := context.Background()
	_, err := client.User.Create().SetEmail("existing@example.test").SetPasswordHash("hash").SetRole("user").SetStatus("active").Save(ctx)
	require.NoError(t, err)
	h.feishuHTTPClient = feishuProfileTestClient(`{"code":0,"data":{"tenant_key":"tenant-a","open_id":"new-open","email":"existing@example.test"}}`)
	recorder := runFeishuCallbackForTest(h)
	require.Equal(t, feishuFrontendCallback, recorder.Header().Get("Location"))
	session, err := client.PendingAuthSession.Query().Only(ctx)
	require.NoError(t, err)
	require.Nil(t, session.TargetUserID)
	require.NotEqual(t, "existing@example.test", session.ResolvedEmail)
	count, err := client.User.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestFeishuCallbackRejectsConflictingVerifiedIdentityOwners(t *testing.T) {
	h, client := newOAuthPendingFlowTestHandler(t, false)
	h.cfg = &config.Config{Feishu: feishuTestConfig()}
	ctx := context.Background()
	subjects := feishuSubjectCandidates(feishuProfile{TenantKey: "tenant-a", UnionID: "union", OpenID: "open"})
	for index, subject := range subjects {
		employee, err := client.User.Create().SetEmail(fmt.Sprintf("employee-%d@example.test", index)).SetPasswordHash("hash").SetRole("user").SetStatus("active").Save(ctx)
		require.NoError(t, err)
		_, err = client.AuthIdentity.Create().SetUserID(employee.ID).SetProviderType("feishu").SetProviderKey("cli_test").SetProviderSubject(subject).Save(ctx)
		require.NoError(t, err)
	}
	h.feishuHTTPClient = feishuProfileTestClient(`{"code":0,"data":{"tenant_key":"tenant-a","union_id":"union","open_id":"open"}}`)
	recorder := runFeishuCallbackForTest(h)
	require.Contains(t, recorder.Header().Get("Location"), "identity_conflict")
	count, err := client.PendingAuthSession.Query().Count(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
}

func feishuProfileTestClient(profile string) *http.Client {
	return &http.Client{Transport: feishuTestTransport(func(r *http.Request) (*http.Response, error) {
		body := profile
		if r.URL.String() == feishuTokenURL {
			body = `{"code":0,"access_token":"provider-test-token"}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
}

func runFeishuCallbackForTest(h *AuthHandler) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "https://portal.example/api/v1/auth/oauth/feishu/callback?code=verified-code&state=expected-state", nil)
	c.Request.AddCookie(&http.Cookie{Name: "feishu_oauth_state", Value: encodeCookieValue("expected-state")})
	c.Request.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("browser-session-key")})
	h.FeishuOAuthCallback(c)
	return recorder
}
