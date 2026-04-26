package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/pendingauthsession"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestLogoutClearsOAuthStateCookiesAndConsumesPendingSession(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("logout-pending-session-token").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("logout-subject-123").
		SetBrowserSessionKey("logout-browser-session-key").
		SetResolvedEmail("logout@example.com").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("logout-browser-session-key")})
	req.AddCookie(&http.Cookie{Name: oauthBindAccessTokenCookieName, Value: "bind-access-token"})
	req.AddCookie(&http.Cookie{Name: linuxDoOAuthStateCookieName, Value: encodeCookieValue("linuxdo-state")})
	req.AddCookie(&http.Cookie{Name: oidcOAuthStateCookieName, Value: encodeCookieValue("oidc-state")})
	req.AddCookie(&http.Cookie{Name: wechatOAuthStateCookieName, Value: encodeCookieValue("wechat-state")})
	req.AddCookie(&http.Cookie{Name: wechatPaymentOAuthStateName, Value: encodeCookieValue("wechat-payment-state")})
	ginCtx.Request = req

	handler.Logout(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	cookies := recorder.Result().Cookies()
	for _, name := range []string{
		oauthPendingSessionCookieName,
		oauthPendingBrowserCookieName,
		oauthBindAccessTokenCookieName,
		linuxDoOAuthStateCookieName,
		oidcOAuthStateCookieName,
		wechatOAuthStateCookieName,
		wechatPaymentOAuthStateName,
	} {
		cookie := findCookie(cookies, name)
		require.NotNil(t, cookie, name)
		require.Equal(t, -1, cookie.MaxAge, name)
		require.True(t, cookie.HttpOnly, name)
	}

	storedSession, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, storedSession.ConsumedAt)
}
