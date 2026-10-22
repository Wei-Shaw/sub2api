package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCompleteDingTalkOAuthRegistrationWithoutInvitationStillRequiresSession(t *testing.T) {
	for _, body := range []string{`{}`, `{"invitation_code":""}`, `{"adopt_display_name":true,"adopt_avatar":false}`} {
		t.Run(body, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/dingtalk/complete-registration", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			(&AuthHandler{}).CompleteDingTalkOAuthRegistration(c)

			require.Equal(t, http.StatusNotFound, recorder.Code)
			var response map[string]any
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			require.Equal(t, "PENDING_AUTH_SESSION_NOT_FOUND", response["reason"])
			require.NotContains(t, response, "access_token")
		})
	}
}

func TestCompleteDingTalkOAuthRegistrationNoEmailInvitationPolicy(t *testing.T) {
	for _, invitationEnabled := range []bool{false, true} {
		name := "invitation_disabled"
		if invitationEnabled {
			name = "invitation_enabled"
		}
		t.Run(name, func(t *testing.T) {
			handler, client := newOAuthPendingFlowTestHandler(t, invitationEnabled)
			ctx := context.Background()
			email := "dingtalk-test-union@dingtalk-connect.invalid"
			session, err := client.PendingAuthSession.Create().
				SetSessionToken("dingtalk-test-session").
				SetIntent("login").SetProviderType("dingtalk").SetProviderKey("dingtalk").
				SetProviderSubject("test-union").SetResolvedEmail(email).
				SetBrowserSessionKey("dingtalk-test-browser").
				SetRedirectTo("/dashboard").
				SetUpstreamIdentityClaims(map[string]any{"username": "DingTalk Test User", "suggested_display_name": "DingTalk Test User"}).
				SetLocalFlowState(map[string]any{oauthCompletionResponseKey: map[string]any{"redirect": "/dashboard", "synthetic_email": email}}).
				SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).Save(ctx)
			require.NoError(t, err)

			request := func(path, body, browserKey string) (*gin.Context, *httptest.ResponseRecorder) {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Request.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
				c.Request.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue(browserKey)})
				return c, recorder
			}

			// Exercise the real exchange then real registration handler, with an
			// isolated in-memory database instead of impersonating a live employee.
			exchange, exchangeRecorder := request("/api/v1/auth/oauth/pending/exchange", `{}`, session.BrowserSessionKey)
			handler.ExchangePendingOAuthCompletion(exchange)
			require.Equal(t, http.StatusOK, exchangeRecorder.Code)
			exchangeData := decodeJSONResponseData(t, exchangeRecorder)
			require.Equal(t, email, exchangeData["synthetic_email"])
			require.NotContains(t, exchangeData, "access_token")

			completePath := "/api/v1/auth/oauth/dingtalk/complete-registration"
			body := `{"adopt_display_name":true,"adopt_avatar":false}`
			wrongBrowser, wrongRecorder := request(completePath, body, "other-browser")
			handler.CompleteDingTalkOAuthRegistration(wrongBrowser)
			require.Equal(t, http.StatusUnauthorized, wrongRecorder.Code)
			userCount, err := client.User.Query().Count(ctx)
			require.NoError(t, err)
			require.Zero(t, userCount)

			completion, completionRecorder := request(completePath, body, session.BrowserSessionKey)
			handler.CompleteDingTalkOAuthRegistration(completion)
			if invitationEnabled {
				require.Equal(t, http.StatusForbidden, completionRecorder.Code)
				var rejected map[string]any
				require.NoError(t, json.Unmarshal(completionRecorder.Body.Bytes(), &rejected))
				require.Equal(t, "OAUTH_INVITATION_REQUIRED", rejected["reason"])
				userCount, err = client.User.Query().Count(ctx)
				require.NoError(t, err)
				require.Zero(t, userCount)
				stored, err := client.PendingAuthSession.Get(ctx, session.ID)
				require.NoError(t, err)
				require.Nil(t, stored.ConsumedAt)

				invalid, invalidRecorder := request(completePath, `{"invitation_code":"invalid-invite","adopt_display_name":true,"adopt_avatar":false}`, session.BrowserSessionKey)
				handler.CompleteDingTalkOAuthRegistration(invalid)
				require.Equal(t, http.StatusBadRequest, invalidRecorder.Code)
				userCount, err = client.User.Query().Count(ctx)
				require.NoError(t, err)
				require.Zero(t, userCount)

				_, err = client.RedeemCode.Create().SetCode("test-invite").SetType(service.RedeemTypeInvitation).
					SetStatus(service.StatusUnused).SetValue(0).Save(ctx)
				require.NoError(t, err)
				completion, completionRecorder = request(completePath, `{"invitation_code":"test-invite","adopt_display_name":true,"adopt_avatar":false}`, session.BrowserSessionKey)
				handler.CompleteDingTalkOAuthRegistration(completion)
			}
			require.Equal(t, http.StatusOK, completionRecorder.Code)
			var tokens map[string]any
			require.NoError(t, json.Unmarshal(completionRecorder.Body.Bytes(), &tokens))
			require.NotEmpty(t, tokens["refresh_token"])
			require.Equal(t, "Bearer", tokens["token_type"])
			accessToken, ok := tokens["access_token"].(string)
			require.True(t, ok)
			claims, err := handler.authService.ValidateToken(accessToken)
			require.NoError(t, err)
			identity, err := client.AuthIdentity.Query().Where(authidentity.ProviderSubjectEQ("test-union")).Only(ctx)
			require.NoError(t, err)
			require.Equal(t, identity.UserID, claims.UserID)
			user, err := client.User.Get(ctx, identity.UserID)
			require.NoError(t, err)
			require.Equal(t, email, user.Email)
			require.Equal(t, "DingTalk Test User", user.Username)
			stored, err := client.PendingAuthSession.Get(ctx, session.ID)
			require.NoError(t, err)
			require.NotNil(t, stored.ConsumedAt)
			replay, replayRecorder := request(completePath, body, session.BrowserSessionKey)
			handler.CompleteDingTalkOAuthRegistration(replay)
			require.NotEqual(t, http.StatusOK, replayRecorder.Code)
			userCount, err = client.User.Query().Count(ctx)
			require.NoError(t, err)
			require.Equal(t, 1, userCount)
		})
	}
}

// A nonempty invitation must not let another provider use DingTalk signup policy.
func TestCompleteDingTalkOAuthRegistrationRejectsWrongProvider(t *testing.T) {
	for _, spec := range []struct{ provider, key string }{
		{"oidc", "dingtalk"}, {"dingtalk", "oidc"}, {"wechat", "wechat"},
	} {
		t.Run(spec.provider+"/"+spec.key, func(t *testing.T) {
			h, client := newOAuthPendingFlowTestHandler(t, false)
			ctx := context.Background()
			session, err := client.PendingAuthSession.Create().
				SetSessionToken("provider-test").SetIntent("login").
				SetProviderType(spec.provider).SetProviderKey(spec.key).SetProviderSubject("other-provider").
				SetResolvedEmail("other-provider@example.test").SetBrowserSessionKey("provider-browser").
				SetUpstreamIdentityClaims(map[string]any{"username": "Test User"}).
				SetLocalFlowState(map[string]any{oauthCompletionResponseKey: map[string]any{"synthetic_email": "other-provider@example.test"}}).
				SetExpiresAt(time.Now().UTC().Add(time.Minute)).Save(ctx)
			require.NoError(t, err)
			r := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(r)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/dingtalk/complete-registration", strings.NewReader(`{"invitation_code":"nonempty"}`))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Request.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
			c.Request.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue(session.BrowserSessionKey)})
			h.CompleteDingTalkOAuthRegistration(c)
			require.Equal(t, http.StatusBadRequest, r.Code)
			require.NotContains(t, r.Body.String(), "access_token")
			users, err := client.User.Query().Count(ctx)
			require.NoError(t, err)
			require.Zero(t, users)
			identities, err := client.AuthIdentity.Query().Count(ctx)
			require.NoError(t, err)
			require.Zero(t, identities)
			stored, err := client.PendingAuthSession.Get(ctx, session.ID)
			require.NoError(t, err)
			require.Nil(t, stored.ConsumedAt)
		})
	}
}
