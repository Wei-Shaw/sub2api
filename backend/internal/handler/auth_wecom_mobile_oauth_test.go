package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBuildWeComMobileAuthorizeURLUsesPrivateInfoScope(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://app.example.com/login", nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	cfg := weComOAuthConfig{
		corpID:      "wwcorp",
		agentID:     "1000002",
		redirectURI: "https://app.example.com/api/v1/auth/oauth/wecom/callback",
	}

	rawURL, err := buildWeComMobileAuthorizeURL(c, cfg, "state-1")
	if err != nil {
		t.Fatalf("buildWeComMobileAuthorizeURL returned error: %v", err)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse authorize url: %v", err)
	}

	if got := parsed.Scheme + "://" + parsed.Host + parsed.Path; got != weComOAuthWebviewAuthorizeURL {
		t.Fatalf("unexpected authorize endpoint: %s", got)
	}
	if parsed.Fragment != "wechat_redirect" {
		t.Fatalf("expected wechat_redirect fragment, got %q", parsed.Fragment)
	}
	q := parsed.Query()
	if q.Get("appid") != "wwcorp" || q.Get("agentid") != "1000002" {
		t.Fatalf("unexpected app identifiers: %s", parsed.RawQuery)
	}
	if q.Get("scope") != "snsapi_privateinfo" {
		t.Fatalf("expected snsapi_privateinfo scope, got %q", q.Get("scope"))
	}
	if q.Get("redirect_uri") != "https://app.example.com/api/v1/auth/oauth/wecom/mobile/callback" {
		t.Fatalf("unexpected redirect_uri: %q", q.Get("redirect_uri"))
	}
}

func TestCreateWeComMobilePendingSessionStoresPromoCode(t *testing.T) {
	handler, _ := newOAuthPendingFlowTestHandler(t, false)

	session, err := handler.createWeComMobilePendingSession(context.Background(), weComMobilePendingSessionInput{
		state:             "state-1",
		browserSessionKey: "browser-session-1",
		intent:            oauthIntentLogin,
		redirectTo:        "/dashboard",
		authorizeURL:      "https://open.weixin.qq.com/connect/oauth2/authorize",
		promoCode:         " PROMO2026 ",
	})
	if err != nil {
		t.Fatalf("createWeComMobilePendingSession returned error: %v", err)
	}

	if got := pendingOAuthPromoCode(session); got != "PROMO2026" {
		t.Fatalf("expected promo code to be stored, got %q", got)
	}
}
