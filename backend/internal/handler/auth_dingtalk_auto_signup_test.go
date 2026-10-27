package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func autoSignupTestConfig() config.DingTalkConnectConfig {
	return config.DingTalkConnectConfig{
		Enabled: true, AutoRegister: true, AppType: "internal", DingTalkAppKind: "internal_app",
		CorpRestrictionPolicy: "internal_only", RequireEmail: true, SyncDisplayName: true,
		ClientID: "test-client", ClientSecret: "test-secret",
		AuthorizeURL: "https://login.example/authorize", TokenURL: "https://login.example/token",
		UserInfoURL: "https://login.example/user", RedirectURL: "https://app.example/callback",
		FrontendRedirectURL: "/auth/dingtalk/callback",
	}
}

func TestCanAutoRegisterDingTalk(t *testing.T) {
	for _, scenario := range []string{"enabled", "disabled", "external", "personal_email", "missing_staff", "registration_closed", "force_email", "invitation"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := autoSignupTestConfig()
			staff := &DingTalkStaffInfo{UserID: "employee-1", Email: "employee@example.com", OrgEmail: "employee@example.com"}
			blocked, force, invitation := false, false, false
			switch scenario {
			case "disabled":
				cfg.AutoRegister = false
			case "external":
				cfg.CorpRestrictionPolicy = "none"
			case "personal_email":
				staff.OrgEmail = ""
			case "missing_staff":
				staff.UserID = ""
			case "registration_closed":
				blocked = true
			case "force_email":
				force = true
			case "invitation":
				invitation = true
			}
			require.Equal(t, scenario == "enabled", canAutoRegisterDingTalk(cfg, staff, blocked, force, invitation))
		})
	}
}

func newAutoSignupSession(t *testing.T, client *dbent.Client) *dbent.PendingAuthSession {
	t.Helper()
	session, err := client.PendingAuthSession.Create().
		SetSessionToken("auto-session").SetBrowserSessionKey("auto-browser").
		SetIntent("login").SetProviderType("dingtalk").SetProviderKey("dingtalk").SetProviderSubject("union-employee").
		SetResolvedEmail("employee@example.com").SetRedirectTo("/dashboard").
		SetUpstreamIdentityClaims(map[string]any{
			"enterprise_verified_email": "employee@example.com", "corp_user_id": "employee-1", "username": "企业员工",
		}).
		SetLocalFlowState(map[string]any{oauthCompletionResponseKey: map[string]any{"step": dingTalkAutoSignupStep}}).
		SetExpiresAt(time.Now().Add(10 * time.Minute)).Save(context.Background())
	require.NoError(t, err)
	return session
}

func exchangeAutoSignup(h *AuthHandler, session *dbent.PendingAuthSession, browser string) *httptest.ResponseRecorder {
	r := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(r)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange",
		strings.NewReader(`{"email":"admin@example.com","password":"attacker-chosen","adopt_display_name":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	c.Request.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue(browser)})
	h.ExchangePendingOAuthCompletion(c)
	return r
}

func TestDingTalkAutoSignupCreatesUserAndBindsIdentity(t *testing.T) {
	cfg := autoSignupTestConfig()
	h, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{dingTalk: &cfg, emailVerifyEnabled: true})
	session := newAutoSignupSession(t, client)
	r := exchangeAutoSignup(h, session, "auto-browser")
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	require.Contains(t, r.Body.String(), "access_token")
	user, err := client.User.Query().Only(context.Background())
	require.NoError(t, err)
	require.Equal(t, "employee@example.com", user.Email)
	require.Equal(t, service.RoleUser, user.Role)
	require.Equal(t, "dingtalk", user.SignupSource)
	require.Equal(t, "企业员工", user.Username)
	require.False(t, h.authService.CheckPassword("attacker-chosen", user.PasswordHash))
	identity, err := client.AuthIdentity.Query().All(context.Background())
	require.NoError(t, err)
	found := false
	for _, i := range identity {
		if i.ProviderType == "dingtalk" {
			require.Equal(t, "union-employee", i.ProviderSubject)
			require.Equal(t, user.ID, i.UserID)
			found = true
		}
	}
	require.True(t, found)
	stored, err := client.PendingAuthSession.Get(context.Background(), session.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.ConsumedAt)
	r = exchangeAutoSignup(h, session, "auto-browser")
	require.Equal(t, http.StatusUnauthorized, r.Code)
	require.NotContains(t, r.Body.String(), "access_token")
	count, err := client.User.Query().Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestDingTalkAutoSignupNeverAdoptsExistingEmail(t *testing.T) {
	cfg := autoSignupTestConfig()
	h, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{dingTalk: &cfg})
	owner, err := client.User.Create().SetEmail("employee@example.com").SetPasswordHash("existing-hash").SetRole("admin").Save(context.Background())
	require.NoError(t, err)
	session := newAutoSignupSession(t, client)
	r := exchangeAutoSignup(h, session, "auto-browser")
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	require.NotContains(t, r.Body.String(), "access_token")
	require.Contains(t, r.Body.String(), "choose_account_action_required")
	count, err := client.AuthIdentity.Query().Count(context.Background())
	require.NoError(t, err)
	require.Zero(t, count)
	unchanged, err := client.User.Get(context.Background(), owner.ID)
	require.NoError(t, err)
	require.Equal(t, "existing-hash", unchanged.PasswordHash)
	require.Equal(t, "admin", unchanged.Role)
}

func TestDingTalkAutoSignupRejectsInvalidContext(t *testing.T) {
	for _, scenario := range []string{"wrong_browser", "disabled", "wrong_provider", "email_mismatch", "expired", "invitation"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := autoSignupTestConfig()
			if scenario == "disabled" {
				cfg.AutoRegister = false
			}
			h, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{dingTalk: &cfg, invitationEnabled: scenario == "invitation"})
			session := newAutoSignupSession(t, client)
			browser := "auto-browser"
			var err error
			switch scenario {
			case "wrong_browser":
				browser = "other-browser"
			case "wrong_provider":
				session, err = client.PendingAuthSession.UpdateOne(session).SetProviderType("oidc").Save(context.Background())
			case "email_mismatch":
				session, err = client.PendingAuthSession.UpdateOne(session).SetResolvedEmail("admin@example.com").Save(context.Background())
			case "expired":
				session, err = client.PendingAuthSession.UpdateOne(session).SetExpiresAt(time.Now().Add(-time.Minute)).Save(context.Background())
			}
			require.NoError(t, err)
			r := exchangeAutoSignup(h, session, browser)
			require.GreaterOrEqual(t, r.Code, 400, r.Body.String())
			require.NotContains(t, r.Body.String(), "access_token")
			count, err := client.User.Query().Count(context.Background())
			require.NoError(t, err)
			require.Zero(t, count)
		})
	}
}

func TestDingTalkAutoSignupRollsBackOnIdentityConflict(t *testing.T) {
	cfg := autoSignupTestConfig()
	h, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{dingTalk: &cfg})
	owner, err := client.User.Create().SetEmail("original@example.com").SetPasswordHash("existing-hash").Save(context.Background())
	require.NoError(t, err)
	_, err = client.AuthIdentity.Create().SetUserID(owner.ID).SetProviderType("dingtalk").SetProviderKey("dingtalk").SetProviderSubject("union-employee").Save(context.Background())
	require.NoError(t, err)
	session := newAutoSignupSession(t, client)
	r := exchangeAutoSignup(h, session, "auto-browser")
	require.GreaterOrEqual(t, r.Code, 400, r.Body.String())
	require.NotContains(t, r.Body.String(), "access_token")
	users, err := client.User.Query().All(context.Background())
	require.NoError(t, err)
	require.Len(t, users, 1)
	require.Equal(t, owner.ID, users[0].ID)
}

func TestDingTalkAutoSignupCallbackAndRepeatLogin(t *testing.T) {
	for _, scenario := range []string{"enterprise_email", "personal_email_only", "not_an_employee"} {
		t.Run(scenario, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1.0/oauth2/userAccessToken", "/v1.0/oauth2/accessToken":
					_, _ = w.Write([]byte(`{"accessToken":"test-token","expireIn":7200}`))
				case "/v1.0/contact/users/me":
					_, _ = w.Write([]byte(`{"unionId":"union-employee","nick":"Employee"}`))
				case "/topapi/user/getbyunionid":
					if scenario == "not_an_employee" {
						_, _ = w.Write([]byte(`{"errcode":60011,"errmsg":"not in directory"}`))
					} else {
						_, _ = w.Write([]byte(`{"errcode":0,"result":{"userid":"employee-1"}}`))
					}
				case "/topapi/v2/user/get":
					result := map[string]any{"userid": "employee-1", "name": "Employee", "email": "employee@example.com"}
					if scenario == "enterprise_email" {
						result["org_email"] = "employee@example.com"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "result": result})
				default:
					http.NotFound(w, r)
				}
			}))
			defer upstream.Close()
			cfg := autoSignupTestConfig()
			cfg.TokenURL = upstream.URL + "/v1.0/oauth2/userAccessToken"
			cfg.UserInfoURL = upstream.URL + "/v1.0/contact/users/me"
			h, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{dingTalk: &cfg, emailVerifyEnabled: true})
			for attempt := 0; attempt < 2; attempt++ {
				r := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(r)
				c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/dingtalk/callback?code=test-code&state=test-state", nil)
				c.Request.AddCookie(&http.Cookie{Name: dingTalkOAuthStateCookieName, Value: encodeCookieValue("test-state")})
				c.Request.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("auto-browser")})
				h.DingTalkOAuthCallback(c)
				require.Equal(t, http.StatusFound, r.Code, r.Body.String())
				require.NotContains(t, r.Header().Get("Location"), "access_token")
				if scenario == "not_an_employee" {
					count, err := client.PendingAuthSession.Query().Count(context.Background())
					require.NoError(t, err)
					require.Zero(t, count)
					break
				}
				var token string
				for _, cookie := range r.Result().Cookies() {
					if cookie.Name == oauthPendingSessionCookieName {
						token = cookie.Value
					}
				}
				require.NotEmpty(t, token)
				sessions, err := client.PendingAuthSession.Query().All(context.Background())
				require.NoError(t, err)
				var session *dbent.PendingAuthSession
				for _, candidate := range sessions {
					if encodeCookieValue(candidate.SessionToken) == token {
						session = candidate
					}
				}
				require.NotNil(t, session)
				r = exchangeAutoSignup(h, session, "auto-browser")
				require.Equal(t, http.StatusOK, r.Code, r.Body.String())
				if scenario == "personal_email_only" {
					require.NotContains(t, r.Body.String(), "access_token")
					require.Contains(t, r.Body.String(), "choose_account_action_required")
					break
				}
				require.Contains(t, r.Body.String(), "access_token")
				count, err := client.User.Query().Count(context.Background())
				require.NoError(t, err)
				require.Equal(t, 1, count, "Repeat login must reuse the bound user")
			}
		})
	}
}
