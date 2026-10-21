package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// The upstream remains reachable after departure: OAuth succeeds, while only
// the corporate directory stops recognizing the employee.
func newDingTalkLoginTestHandler(t *testing.T, policy, email string, directoryError *atomic.Value) (*AuthHandler, *dbent.Client) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1.0/oauth2/userAccessToken", "/v1.0/oauth2/accessToken":
			_, _ = w.Write([]byte(`{"accessToken":"token","expireIn":7200}`))
		case "/v1.0/contact/users/me":
			_, _ = w.Write([]byte(`{"unionId":"employee-union","nick":"昵称"}`))
		case "/topapi/user/getbyunionid":
			if directoryError.Load().(string) != "" {
				_, _ = w.Write([]byte(directoryError.Load().(string)))
				return
			}
			_, _ = w.Write([]byte(`{"errcode":0,"result":{"userid":"staff-1"}}`))
		case "/topapi/v2/user/get":
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "result": map[string]any{"userid": "staff-1", "name": "张三", "email": email, "dept_id_list": []int{1}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
	h, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		invitationEnabled: true, emailVerifyEnabled: true,
		refreshTokenCache: &dingTalkRefreshCache{tokens: make(map[string]*service.RefreshTokenData)},
		settingValues:     map[string]string{service.SettingKeyRegistrationEnabled: "false", service.SettingKeyForceEmailOnThirdPartySignup: "true"},
	})
	cfg := &config.Config{DingTalk: config.DingTalkConnectConfig{
		Enabled: true, RequireEmail: true, ClientID: "app", ClientSecret: "secret", DingTalkAppKind: "internal_app", AppType: "internal",
		CorpRestrictionPolicy: policy, AuthorizeURL: upstream.URL + "/authorize", TokenURL: upstream.URL + "/v1.0/oauth2/userAccessToken",
		UserInfoURL: upstream.URL + "/v1.0/contact/users/me", RedirectURL: "https://app.example/api/v1/auth/oauth/dingtalk/callback",
		FrontendRedirectURL: dingTalkOAuthDefaultFrontendCB,
	}}
	settings := service.NewSettingService(&oauthPendingFlowSettingRepoStub{values: map[string]string{service.SettingKeyForceEmailOnThirdPartySignup: "true"}}, cfg)
	h = NewAuthHandler(cfg, h.authService, h.userService, settings, nil, nil, nil, nil)
	return h, client
}

func callDingTalkCallback(t *testing.T, h *AuthHandler, apps ...string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	state := "state"
	if len(apps) > 0 {
		state += "." + apps[0]
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/dingtalk/callback?code=code&state="+state, nil)
	c.Request.AddCookie(encodedCookie(dingTalkOAuthStateCookieName, state))
	c.Request.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "browser"))
	c.Request.AddCookie(encodedCookie(dingTalkOAuthRedirectCookie, "/dashboard"))
	h.DingTalkOAuthCallback(c)
	require.Equal(t, http.StatusFound, w.Code, w.Body.String())
	return w
}

func exchangeDingTalkCallback(h *AuthHandler, callback *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", strings.NewReader(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.AddCookie(encodedCookie(oauthPendingBrowserCookieName, "browser"))
	c.Request.AddCookie(findCookie(callback.Result().Cookies(), oauthPendingSessionCookieName))
	h.ExchangePendingOAuthCompletion(c)
	return w
}

func TestDingTalkLoginAutoProvisionsWithoutEmailOrRegistration(t *testing.T) {
	for _, email := range []string{"", "existing@example.com"} {
		t.Run(email, func(t *testing.T) {
			directoryError := atomic.Value{}
			directoryError.Store("")
			h, client := newDingTalkLoginTestHandler(t, "none", email, &directoryError)
			ctx := context.Background()
			// Matching enterprise email must not take over an existing local account.
			existing, err := client.User.Create().SetEmail("existing@example.com").SetPasswordHash("hash").SetUsername("original").Save(ctx)
			require.NoError(t, err)
			callback := callDingTalkCallback(t, h)
			require.Equal(t, dingTalkOAuthDefaultFrontendCB, callback.Header().Get("Location"))
			identity, err := client.AuthIdentity.Query().Where(authidentity.ProviderTypeEQ("dingtalk")).Only(ctx)
			require.NoError(t, err)
			require.NotEqual(t, existing.ID, identity.UserID)
			user, err := client.User.Get(ctx, identity.UserID)
			require.NoError(t, err)
			require.Equal(t, "张三", user.Username)
			require.Equal(t, "dingtalk", user.SignupSource)
			require.True(t, strings.HasSuffix(user.Email, service.DingTalkConnectSyntheticEmailDomain))
			exchange := exchangeDingTalkCallback(h, callback)
			require.Equal(t, http.StatusOK, exchange.Code, exchange.Body.String())
			require.Contains(t, exchange.Body.String(), `"access_token"`)
			var completion struct {
				Data struct {
					RefreshToken string `json:"refresh_token"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(exchange.Body.Bytes(), &completion))
			require.NotEmpty(t, completion.Data.RefreshToken)

			require.NotContains(t, exchange.Body.String(), "email_completion")
			require.NotContains(t, exchange.Body.String(), "bind_login_required")
			replay := exchangeDingTalkCallback(h, callback)
			require.NotEqual(t, http.StatusOK, replay.Code)
			callback = callDingTalkCallback(t, h)
			require.Equal(t, dingTalkOAuthDefaultFrontendCB, callback.Header().Get("Location"))
			count, err := client.User.Query().Count(ctx)
			require.NoError(t, err)
			require.Equal(t, 2, count, "repeat login reuses the same user")

			// Departure between callback and token exchange must also fail closed.
			directoryError.Store(`{"errcode":60121,"errmsg":"not in directory"}`)
			refreshed, refreshErr := h.authService.RefreshTokenPair(ctx, completion.Data.RefreshToken)
			require.Error(t, refreshErr)
			require.Nil(t, refreshed, "departed employees must not renew a prior session")
			denied := exchangeDingTalkCallback(h, callback)
			require.NotEqual(t, http.StatusOK, denied.Code, denied.Body.String())
			require.NotContains(t, denied.Body.String(), `"access_token"`)
			localUser, err := h.userService.GetByID(ctx, user.ID)
			require.NoError(t, err)
			pair, err := h.authService.GenerateTokenPair(ctx, localUser, "existing-refresh-family")
			require.Error(t, err)
			require.Nil(t, pair)
			token, err := h.authService.GenerateToken(ctx, localUser)
			require.Error(t, err)
			require.Empty(t, token, "access-token fallback must not bypass membership")
		})
	}
}

func TestDingTalkLoginRejectsMissingMembershipForAllPolicies(t *testing.T) {
	for _, policy := range []string{"none", "internal_only", ""} {
		for _, upstreamError := range []string{`{"errcode":60121,"errmsg":"departed"}`, `{"errcode":88,"errmsg":"rate limited"}`} {
			t.Run(policy+upstreamError, func(t *testing.T) {
				directoryError := atomic.Value{}
				directoryError.Store(upstreamError)
				h, client := newDingTalkLoginTestHandler(t, policy, "", &directoryError)
				callback := callDingTalkCallback(t, h)
				require.Contains(t, callback.Header().Get("Location"), "error=")
				require.Nil(t, findCookie(callback.Result().Cookies(), oauthPendingSessionCookieName))
				count, err := client.User.Query().Count(context.Background())
				require.NoError(t, err)
				require.Zero(t, count)
			})
		}
	}
}

func TestDingTalkLoginDoesNotReplaceDisabledAccount(t *testing.T) {
	directoryError := atomic.Value{}
	directoryError.Store("")
	h, client := newDingTalkLoginTestHandler(t, "none", "", &directoryError)
	require.Equal(t, dingTalkOAuthDefaultFrontendCB, callDingTalkCallback(t, h).Header().Get("Location"))
	user, err := client.User.Query().Only(context.Background())
	require.NoError(t, err)
	require.NoError(t, client.User.UpdateOneID(user.ID).SetStatus("disabled").Exec(context.Background()))
	callback := callDingTalkCallback(t, h)
	require.Contains(t, callback.Header().Get("Location"), "error=account_unavailable")
	count, err := client.User.Query().Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

// Keep actual refresh tokens so departure can be tested against the rotation API.
type dingTalkRefreshCache struct {
	oauthPendingFlowRefreshTokenCacheStub
	tokens map[string]*service.RefreshTokenData
}

func (s *dingTalkRefreshCache) StoreRefreshToken(_ context.Context, key string, data *service.RefreshTokenData, _ time.Duration) error {
	s.tokens[key] = data
	return nil
}
func (s *dingTalkRefreshCache) GetRefreshToken(_ context.Context, key string) (*service.RefreshTokenData, error) {
	if data, ok := s.tokens[key]; ok {
		return data, nil
	}
	return nil, service.ErrRefreshTokenNotFound
}
func (s *dingTalkRefreshCache) DeleteRefreshToken(_ context.Context, key string) error {
	delete(s.tokens, key)
	return nil
}

func (s *dingTalkRefreshCache) DeleteUserRefreshTokens(_ context.Context, userID int64) error {
	for key, token := range s.tokens {
		if token.UserID == userID {
			delete(s.tokens, key)
		}
	}
	return nil
}

func TestDingTalkLoginSeparatesApplicationIdentities(t *testing.T) {
	directoryError := atomic.Value{}
	directoryError.Store("")
	h, client := newDingTalkLoginTestHandler(t, "internal_only", "", &directoryError)
	for _, id := range []string{"one", "two"} {
		h.cfg.DingTalk.Apps = append(h.cfg.DingTalk.Apps, config.DingTalkAppConfig{ID: id, Name: id, Enabled: true, ClientID: id, ClientSecret: "secret", RedirectURL: h.cfg.DingTalk.RedirectURL})
	}
	for _, id := range []string{"one", "two"} {
		callback := callDingTalkCallback(t, h, id)
		require.Equal(t, dingTalkOAuthDefaultFrontendCB, callback.Header().Get("Location"))
		exchange := exchangeDingTalkCallback(h, callback)
		require.Equal(t, http.StatusOK, exchange.Code, exchange.Body.String())
	}
	users, err := client.User.Query().All(context.Background())
	require.NoError(t, err)
	require.Len(t, users, 2)
	require.NotEqual(t, users[0].Email, users[1].Email)
}

func TestDingTalkSyntheticIdentityPreservesSubjectCase(t *testing.T) {
	require.NotEqual(t, buildDingTalkAppSyntheticEmail("", "ABC"), buildDingTalkAppSyntheticEmail("", "abc"))
	require.Equal(t, buildDingTalkAppSyntheticEmail("", "ABC"), buildDingTalkAppSyntheticEmail("default", "ABC"))
}
