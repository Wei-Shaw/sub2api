package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestBuildWeComAuthorizeURLWebview(t *testing.T) {
	cfg := weComOAuthConfig{
		corpID:      "wwcorp",
		agentID:     "1000002",
		scope:       "snsapi_base",
		redirectURI: "https://example.com/api/v1/auth/oauth/wecom/callback",
	}

	rawURL, err := buildWeComAuthorizeURL(cfg, "webview", "state123")
	if err != nil {
		t.Fatalf("buildWeComAuthorizeURL returned error: %v", err)
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
	if q.Get("appid") != cfg.corpID || q.Get("agentid") != cfg.agentID {
		t.Fatalf("unexpected app identifiers in query: %s", parsed.RawQuery)
	}
	if q.Get("redirect_uri") != cfg.redirectURI {
		t.Fatalf("redirect_uri not preserved: %q", q.Get("redirect_uri"))
	}
	if q.Get("response_type") != "code" || q.Get("scope") != cfg.scope || q.Get("state") != "state123" {
		t.Fatalf("unexpected OAuth query: %s", parsed.RawQuery)
	}
}

func TestBuildWeComAuthorizeURLWeb(t *testing.T) {
	cfg := weComOAuthConfig{
		corpID:      "wwcorp",
		agentID:     "1000002",
		redirectURI: "https://example.com/api/v1/auth/oauth/wecom/callback",
	}

	rawURL, err := buildWeComAuthorizeURL(cfg, "web", "state123")
	if err != nil {
		t.Fatalf("buildWeComAuthorizeURL returned error: %v", err)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse authorize url: %v", err)
	}

	if got := parsed.Scheme + "://" + parsed.Host + parsed.Path; got != weComOAuthWebAuthorizeURL {
		t.Fatalf("unexpected authorize endpoint: %s", got)
	}
	q := parsed.Query()
	if q.Get("login_type") != "CorpApp" {
		t.Fatalf("expected CorpApp login_type, got %q", q.Get("login_type"))
	}
	if q.Get("appid") != cfg.corpID || q.Get("agentid") != cfg.agentID || q.Get("redirect_uri") != cfg.redirectURI || q.Get("state") != "state123" {
		t.Fatalf("unexpected Web login query: %s", parsed.RawQuery)
	}
}

func TestFetchWeComOAuthIdentityUsesCachedTokenAndParsesUserID(t *testing.T) {
	tokenRequests := 0
	userInfoRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/gettoken":
			tokenRequests++
			if r.URL.Query().Get("corpid") != "wwcorp" || r.URL.Query().Get("corpsecret") != "secret" {
				http.Error(w, "unexpected token query", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(weComAccessTokenResponse{AccessToken: "token-1", ExpiresIn: 7200})
		case "/cgi-bin/auth/getuserinfo":
			userInfoRequests++
			if r.URL.Query().Get("access_token") != "token-1" || r.URL.Query().Get("code") != "code-1" {
				http.Error(w, "unexpected userinfo query", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"UserId":"zhangsan","errcode":0}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	origTokenURL := weComOAuthGetTokenURL
	origUserInfoURL := weComOAuthGetUserInfoURL
	origCache := weComTokenCache
	weComOAuthGetTokenURL = server.URL + "/cgi-bin/gettoken"
	weComOAuthGetUserInfoURL = server.URL + "/cgi-bin/auth/getuserinfo"
	weComTokenCache = map[string]cachedWeComToken{}
	defer func() {
		weComOAuthGetTokenURL = origTokenURL
		weComOAuthGetUserInfoURL = origUserInfoURL
		weComTokenCache = origCache
	}()

	cfg := weComOAuthConfig{corpID: "wwcorp", secret: "secret"}
	first, err := fetchWeComOAuthIdentity(context.Background(), cfg, "code-1")
	if err != nil {
		t.Fatalf("fetchWeComOAuthIdentity returned error: %v", err)
	}
	second, err := fetchWeComOAuthIdentity(context.Background(), cfg, "code-1")
	if err != nil {
		t.Fatalf("fetchWeComOAuthIdentity returned error on cached token: %v", err)
	}

	if first.UserID != "zhangsan" || second.UserID != "zhangsan" {
		t.Fatalf("unexpected user ids: %q %q", first.UserID, second.UserID)
	}
	if tokenRequests != 1 {
		t.Fatalf("expected one token request due cache, got %d", tokenRequests)
	}
	if userInfoRequests != 2 {
		t.Fatalf("expected two userinfo requests, got %d", userInfoRequests)
	}
}

func TestEnrichWeComProfileClaimsFetchesDirectoryUserWhenTicketMissing(t *testing.T) {
	tokenRequests := 0
	userRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/gettoken":
			tokenRequests++
			_ = json.NewEncoder(w).Encode(weComAccessTokenResponse{AccessToken: "token-1", ExpiresIn: 7200})
		case "/cgi-bin/user/get":
			userRequests++
			if r.URL.Query().Get("access_token") != "token-1" || r.URL.Query().Get("userid") != "zhangsan" {
				t.Errorf("unexpected user get query: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(weComUserDetailResponse{
				UserID:  "zhangsan",
				Name:    "张三",
				Email:   "zhangsan@example.com",
				BizMail: "zhangsan@corp.example",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	origTokenURL := weComOAuthGetTokenURL
	origUserURL := weComOAuthGetUserURL
	origCache := weComTokenCache
	weComOAuthGetTokenURL = server.URL + "/cgi-bin/gettoken"
	weComOAuthGetUserURL = server.URL + "/cgi-bin/user/get"
	weComTokenCache = map[string]cachedWeComToken{}
	defer func() {
		weComOAuthGetTokenURL = origTokenURL
		weComOAuthGetUserURL = origUserURL
		weComTokenCache = origCache
	}()

	claims := map[string]any{"username": "zhangsan"}
	username := enrichWeComProfileClaims(
		context.Background(),
		weComOAuthConfig{corpID: "wwcorp", secret: "secret", scope: "snsapi_privateinfo"},
		weComUserInfoResponse{UserID: "zhangsan"},
		"zhangsan",
		claims,
	)

	if username != "张三" {
		t.Fatalf("expected directory username, got %q", username)
	}
	if claims["wecom_email"] != "zhangsan@example.com" || claims["wecom_biz_mail"] != "zhangsan@corp.example" {
		t.Fatalf("expected directory email claims, got %#v", claims)
	}
	if tokenRequests != 1 || userRequests != 1 {
		t.Fatalf("expected one token and one user request, got token=%d user=%d", tokenRequests, userRequests)
	}
}

func TestWeComSyntheticEmailUsesReservedDomain(t *testing.T) {
	email := weComSyntheticEmail("wwcorp/zhangsan")
	if !strings.HasSuffix(email, "@wecom-connect.invalid") {
		t.Fatalf("expected reserved WeCom synthetic email domain, got %q", email)
	}
	if len(strings.TrimSuffix(email, "@wecom-connect.invalid")) != 64 {
		t.Fatalf("expected sha256 hex local part, got %q", email)
	}
}

func TestEnrichWeComProfileClaimsFetchesPrivateInfoDetail(t *testing.T) {
	tokenRequests := 0
	detailRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/gettoken":
			tokenRequests++
			_ = json.NewEncoder(w).Encode(weComAccessTokenResponse{AccessToken: "token-1", ExpiresIn: 7200})
		case "/cgi-bin/auth/getuserdetail":
			detailRequests++
			if r.Method != http.MethodPost {
				t.Errorf("expected POST detail request, got %s", r.Method)
			}
			if r.URL.Query().Get("access_token") != "token-1" {
				t.Errorf("unexpected access token: %q", r.URL.Query().Get("access_token"))
			}
			body, _ := io.ReadAll(r.Body)
			var payload map[string]string
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Errorf("decode detail payload: %v", err)
			}
			if payload["user_ticket"] != "ticket-1" {
				t.Errorf("unexpected user_ticket payload: %q", payload["user_ticket"])
			}
			_ = json.NewEncoder(w).Encode(weComUserDetailResponse{
				UserID:  "zhangsan",
				Name:    "张三",
				Avatar:  "https://cdn.example/avatar.png",
				Email:   "zhangsan@example.com",
				BizMail: "zhangsan@corp.example",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	origTokenURL := weComOAuthGetTokenURL
	origDetailURL := weComOAuthGetUserDetailURL
	origCache := weComTokenCache
	weComOAuthGetTokenURL = server.URL + "/cgi-bin/gettoken"
	weComOAuthGetUserDetailURL = server.URL + "/cgi-bin/auth/getuserdetail"
	weComTokenCache = map[string]cachedWeComToken{}
	defer func() {
		weComOAuthGetTokenURL = origTokenURL
		weComOAuthGetUserDetailURL = origDetailURL
		weComTokenCache = origCache
	}()

	claims := map[string]any{"username": "zhangsan"}
	username := enrichWeComProfileClaims(
		context.Background(),
		weComOAuthConfig{corpID: "wwcorp", secret: "secret", scope: "snsapi_privateinfo"},
		weComUserInfoResponse{UserID: "zhangsan", UserTicket: "ticket-1"},
		"zhangsan",
		claims,
	)

	if username != "张三" {
		t.Fatalf("expected enriched username, got %q", username)
	}
	if claims["username"] != "张三" || claims["suggested_display_name"] != "张三" {
		t.Fatalf("expected display name claims, got %#v", claims)
	}
	if claims["suggested_avatar_url"] != "https://cdn.example/avatar.png" {
		t.Fatalf("expected avatar suggestion, got %#v", claims["suggested_avatar_url"])
	}
	if claims["wecom_email"] != "zhangsan@example.com" || claims["wecom_biz_mail"] != "zhangsan@corp.example" {
		t.Fatalf("expected WeCom email claims, got %#v", claims)
	}
	if tokenRequests != 1 || detailRequests != 1 {
		t.Fatalf("expected one token and one detail request, got token=%d detail=%d", tokenRequests, detailRequests)
	}
}

func TestApplyWeComResolvedAvatarStoresUserAvatar(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{})
	ctx := context.Background()

	userEntity, err := client.User.Create().
		SetEmail("wecom-avatar@example.com").
		SetUsername("wecom-avatar").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetBalance(0).
		SetConcurrency(1).
		SetStatus(service.StatusActive).
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	resolved := weComResolvedOAuthIdentity{
		upstreamClaims: map[string]any{
			"suggested_avatar_url": "https://wework.qpic.cn/wwpic/avatar/0",
		},
	}
	if err := handler.applyWeComResolvedAvatar(ctx, userEntity.ID, resolved); err != nil {
		t.Fatalf("apply wecom avatar: %v", err)
	}

	record := loadUserAvatarRecord(t, client, userEntity.ID)
	if record == nil {
		t.Fatal("expected stored user avatar")
	}
	if record.StorageProvider != "remote_url" {
		t.Fatalf("expected remote_url avatar, got %q", record.StorageProvider)
	}
	if record.URL != "https://wework.qpic.cn/wwpic/avatar/0" {
		t.Fatalf("unexpected avatar url: %q", record.URL)
	}
}

func TestEnrichWeComProfileClaimsSupplementsNameFromDirectory(t *testing.T) {
	tokenRequests := 0
	detailRequests := 0
	userRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/gettoken":
			tokenRequests++
			_ = json.NewEncoder(w).Encode(weComAccessTokenResponse{AccessToken: "token-1", ExpiresIn: 7200})
		case "/cgi-bin/auth/getuserdetail":
			detailRequests++
			_ = json.NewEncoder(w).Encode(weComUserDetailResponse{
				UserID:  "zhangsan",
				Email:   "zhangsan@example.com",
				BizMail: "zhangsan@corp.example",
			})
		case "/cgi-bin/user/get":
			userRequests++
			if r.URL.Query().Get("userid") != "zhangsan" {
				t.Errorf("unexpected directory userid: %q", r.URL.Query().Get("userid"))
			}
			_ = json.NewEncoder(w).Encode(weComUserDetailResponse{
				UserID: "zhangsan",
				Name:   "张三",
				Avatar: "https://cdn.example/directory-avatar.png",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	origTokenURL := weComOAuthGetTokenURL
	origDetailURL := weComOAuthGetUserDetailURL
	origUserURL := weComOAuthGetUserURL
	origCache := weComTokenCache
	weComOAuthGetTokenURL = server.URL + "/cgi-bin/gettoken"
	weComOAuthGetUserDetailURL = server.URL + "/cgi-bin/auth/getuserdetail"
	weComOAuthGetUserURL = server.URL + "/cgi-bin/user/get"
	weComTokenCache = map[string]cachedWeComToken{}
	defer func() {
		weComOAuthGetTokenURL = origTokenURL
		weComOAuthGetUserDetailURL = origDetailURL
		weComOAuthGetUserURL = origUserURL
		weComTokenCache = origCache
	}()

	claims := map[string]any{"username": "zhangsan"}
	username := enrichWeComProfileClaims(
		context.Background(),
		weComOAuthConfig{corpID: "wwcorp", secret: "secret", scope: "snsapi_privateinfo"},
		weComUserInfoResponse{UserID: "zhangsan", UserTicket: "ticket-1"},
		"zhangsan",
		claims,
	)

	if username != "张三" || claims["suggested_display_name"] != "张三" {
		t.Fatalf("expected directory display name, got username=%q claims=%#v", username, claims)
	}
	if claims["wecom_email"] != "zhangsan@example.com" || claims["wecom_biz_mail"] != "zhangsan@corp.example" {
		t.Fatalf("expected private email claims to be preserved, got %#v", claims)
	}
	if claims["suggested_avatar_url"] != "https://cdn.example/directory-avatar.png" {
		t.Fatalf("expected directory avatar, got %#v", claims["suggested_avatar_url"])
	}
	if tokenRequests != 1 || detailRequests != 1 || userRequests != 1 {
		t.Fatalf("unexpected request counts token=%d detail=%d user=%d", tokenRequests, detailRequests, userRequests)
	}
}

func TestMaybeFetchWeComUserDetailSkipsWithoutPrivateInfoScopeOrTicket(t *testing.T) {
	detail, err := maybeFetchWeComUserDetail(
		context.Background(),
		weComOAuthConfig{scope: "snsapi_base"},
		weComUserInfoResponse{UserTicket: "ticket-1"},
	)
	if err != nil || detail != nil {
		t.Fatalf("expected snsapi_base to skip detail fetch, got detail=%#v err=%v", detail, err)
	}

	detail, err = maybeFetchWeComUserDetail(
		context.Background(),
		weComOAuthConfig{scope: "snsapi_privateinfo"},
		weComUserInfoResponse{},
	)
	if err != nil || detail != nil {
		t.Fatalf("expected missing ticket to skip detail fetch, got detail=%#v err=%v", detail, err)
	}
}

func TestEnrichWeComProfileClaimsKeepsLoginWhenDetailFetchFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/gettoken":
			_ = json.NewEncoder(w).Encode(weComAccessTokenResponse{AccessToken: "token-1", ExpiresIn: 7200})
		case "/cgi-bin/auth/getuserdetail":
			_ = json.NewEncoder(w).Encode(weComUserDetailResponse{ErrCode: 40003, ErrMsg: "invalid user_ticket"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	origTokenURL := weComOAuthGetTokenURL
	origDetailURL := weComOAuthGetUserDetailURL
	origCache := weComTokenCache
	weComOAuthGetTokenURL = server.URL + "/cgi-bin/gettoken"
	weComOAuthGetUserDetailURL = server.URL + "/cgi-bin/auth/getuserdetail"
	weComTokenCache = map[string]cachedWeComToken{}
	defer func() {
		weComOAuthGetTokenURL = origTokenURL
		weComOAuthGetUserDetailURL = origDetailURL
		weComTokenCache = origCache
	}()

	claims := map[string]any{"username": "zhangsan"}
	username := enrichWeComProfileClaims(
		context.Background(),
		weComOAuthConfig{corpID: "wwcorp", secret: "secret", scope: "snsapi_privateinfo"},
		weComUserInfoResponse{UserID: "zhangsan", UserTicket: "ticket-1"},
		"zhangsan",
		claims,
	)

	if username != "zhangsan" {
		t.Fatalf("expected fallback username after detail failure, got %q", username)
	}
	if _, exists := claims["suggested_display_name"]; exists {
		t.Fatalf("did not expect suggested display name on detail failure: %#v", claims)
	}
	if claims["wecom_profile_detail_error"] == "" {
		t.Fatalf("expected detail failure to be recorded in claims")
	}
}

func TestNormalizeWeComOAuthMode(t *testing.T) {
	if got := normalizeWeComOAuthMode("", "Mozilla wxwork"); got != "webview" {
		t.Fatalf("expected wxwork user agent to select webview, got %q", got)
	}
	if got := normalizeWeComOAuthMode("", "Mozilla"); got != "web" {
		t.Fatalf("expected normal browser to select web, got %q", got)
	}
	if got := normalizeWeComOAuthMode("webview", "Mozilla"); got != "webview" {
		t.Fatalf("explicit webview not honored: %q", got)
	}
}

func TestFetchWeComAccessTokenRefreshesNearExpiredCache(t *testing.T) {
	origCache := weComTokenCache
	weComTokenMu.Lock()
	weComTokenCache = map[string]cachedWeComToken{
		"wwcorp\x00secret": {accessToken: "expired-token", expiresAt: time.Now().Add(-time.Second)},
	}
	weComTokenMu.Unlock()

	tokenRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenRequests++
		_ = json.NewEncoder(w).Encode(weComAccessTokenResponse{AccessToken: "fresh-token", ExpiresIn: 7200})
	}))
	defer server.Close()
	origTokenURL := weComOAuthGetTokenURL
	weComOAuthGetTokenURL = server.URL
	defer func() {
		weComOAuthGetTokenURL = origTokenURL
		weComTokenCache = origCache
	}()

	token, err := fetchWeComAccessToken(context.Background(), weComOAuthConfig{corpID: "wwcorp", secret: "secret"})
	if err != nil {
		t.Fatalf("fetchWeComAccessToken returned error: %v", err)
	}
	if token != "fresh-token" || tokenRequests != 1 {
		t.Fatalf("expected refreshed token once, got token=%q requests=%d", token, tokenRequests)
	}
}
