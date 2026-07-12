package handler

// oidc_integration_test.go 驱动一条完整的 OIDC Authorization Code + PKCE + Refresh
// Token 流程，并用 JWKS 端点对 ID Token 做 RS256 验签（手写 stdlib 验签，因为
// go-oidc 未 vendored）。覆盖 task 12.1。
//
// 流程：
//  1. 启用 Provider、创建 RP (consent_required=false) 与用户
//  2. 签发 SSO cookie，带 cookie 命中 GET /oidc/authorize → 302 回跳 redirect_uri?code=
//  3. POST /oidc/token (authorization_code + PKCE) → 拿 access/refresh/id token
//  4. 拉 /.well-known/jwks.json，手工重建 RSA 公钥，jwt.Parse 验 RS256 + 校验 iss/aud/sub
//  5. POST /oidc/token (refresh_token) → refresh 轮转、再次签发可验证的 id token
//  6. GET /oidc/userinfo (Bearer) → 校验 sub/name/email

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

// issueSsoCookie 通过 SsoSessionService 签发一个会话并返回可附加到请求的 cookie。
func (e *oidcHandlerTestEnv) issueSsoCookie(t *testing.T, userID int64) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	dummy := httptest.NewRequest(http.MethodGet, "/oidc/authorize", nil)
	_, err := e.sso.Issue(context.Background(), rec, dummy, userID)
	require.NoError(t, err)
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == service.SsoCookieName {
			return ck
		}
	}
	t.Fatalf("SSO cookie %q not set", service.SsoCookieName)
	return nil
}

// jwksToKeyfunc 把 JWKS 响应转成 jwt.Keyfunc（按 header.kid 选 RSA 公钥）。
func jwksToKeyfunc(t *testing.T, jwksBody []byte) jwt.Keyfunc {
	t.Helper()
	var doc struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	require.NoError(t, json.Unmarshal(jwksBody, &doc))
	require.NotEmpty(t, doc.Keys)

	keys := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		require.Equal(t, "RSA", k.Kty)
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		require.NoError(t, err)
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		require.NoError(t, err)
		keys[k.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: int(new(big.Int).SetBytes(eBytes).Int64()),
		}
	}

	return func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		pub, ok := keys[kid]
		require.Truef(t, ok, "kid %q not found in JWKS", kid)
		return pub, nil
	}
}

func TestOidcIntegration_AuthCodePKCERefreshFlow(t *testing.T) {
	e := newOidcHandlerTestEnv(t, true)
	scopes := []string{"openid", "profile", "email", "offline_access"}
	rp, secret := e.createRP(t, scopes)
	userID := e.createUser(t)
	cookie := e.issueSsoCookie(t, userID)

	verifier := "integration-verifier-0123456789-0123456789"
	const state = "xyz-state"
	const nonce = "n-0S6_WzA2Mj"

	// ── 1. Authorize（带 SSO cookie）→ 302 回跳 redirect_uri?code= ──────────────
	q := url.Values{
		"client_id":             {rp.ClientID},
		"redirect_uri":          {"https://rp.example.com/cb"},
		"response_type":         {"code"},
		"scope":                 {strings.Join(scopes, " ")},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {pkceS256(verifier)},
		"code_challenge_method": {"S256"},
	}
	aw := httptest.NewRecorder()
	areq := httptest.NewRequest(http.MethodGet, "/oidc/authorize?"+q.Encode(), nil)
	areq.AddCookie(cookie)
	e.router.ServeHTTP(aw, areq)

	require.Equal(t, http.StatusFound, aw.Code)
	loc, err := url.Parse(aw.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, "https://rp.example.com/cb", loc.Scheme+"://"+loc.Host+loc.Path)
	require.Equal(t, state, loc.Query().Get("state"))
	code := loc.Query().Get("code")
	require.NotEmpty(t, code)

	// ── 2. Token：authorization_code + PKCE ────────────────────────────────────
	tw := e.postToken(url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {"https://rp.example.com/cb"},
		"code_verifier": {verifier},
		"client_id":     {rp.ClientID},
		"client_secret": {secret},
	})
	require.Equal(t, http.StatusOK, tw.Code)
	var tok service.OidcTokenResponse
	require.NoError(t, json.Unmarshal(tw.Body.Bytes(), &tok))
	require.NotEmpty(t, tok.AccessToken)
	require.NotEmpty(t, tok.RefreshToken)
	require.NotEmpty(t, tok.IDToken)

	// ── 3. 拉 JWKS 并对 ID Token 做 RS256 验签 ────────────────────────────────
	jw := httptest.NewRecorder()
	e.router.ServeHTTP(jw, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))
	require.Equal(t, http.StatusOK, jw.Code)
	keyfunc := jwksToKeyfunc(t, jw.Body.Bytes())

	parsed, err := jwt.Parse(tok.IDToken, keyfunc, jwt.WithValidMethods([]string{"RS256"}))
	require.NoError(t, err)
	require.True(t, parsed.Valid)
	claims, ok := parsed.Claims.(jwt.MapClaims)
	require.True(t, ok)
	require.Equal(t, "https://op.example.com", claims["iss"])
	require.Equal(t, rp.ClientID, claims["aud"])
	require.Equal(t, strconv.FormatInt(userID, 10), claims["sub"])
	require.Equal(t, nonce, claims["nonce"])

	// ── 4. Refresh：轮转出新 refresh + 可验证的新 id token ────────────────────
	rw := e.postToken(url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tok.RefreshToken},
		"client_id":     {rp.ClientID},
		"client_secret": {secret},
	})
	require.Equal(t, http.StatusOK, rw.Code)
	var refreshed service.OidcTokenResponse
	require.NoError(t, json.Unmarshal(rw.Body.Bytes(), &refreshed))
	require.NotEmpty(t, refreshed.AccessToken)
	require.NotEqual(t, tok.RefreshToken, refreshed.RefreshToken, "refresh token 必须轮转")
	require.NotEmpty(t, refreshed.IDToken)

	refreshedToken, err := jwt.Parse(refreshed.IDToken, keyfunc, jwt.WithValidMethods([]string{"RS256"}))
	require.NoError(t, err)
	require.True(t, refreshedToken.Valid)

	// ── 5. UserInfo：用新 access token 取 claims ──────────────────────────────
	uw := httptest.NewRecorder()
	ureq := httptest.NewRequest(http.MethodGet, "/oidc/userinfo", nil)
	ureq.Header.Set("Authorization", "Bearer "+refreshed.AccessToken)
	e.router.ServeHTTP(uw, ureq)
	require.Equal(t, http.StatusOK, uw.Code)
	var info map[string]any
	require.NoError(t, json.Unmarshal(uw.Body.Bytes(), &info))
	require.Equal(t, strconv.FormatInt(userID, 10), info["sub"])
	require.Equal(t, "bob", info["name"])
	require.Equal(t, "bob@example.com", info["email"])

	// ── 6. 旧 refresh token 复用应触发整 family 吊销 ──────────────────────────
	reuse := e.postToken(url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tok.RefreshToken},
		"client_id":     {rp.ClientID},
		"client_secret": {secret},
	})
	require.Equal(t, http.StatusBadRequest, reuse.Code)
	var reuseErr map[string]any
	require.NoError(t, json.Unmarshal(reuse.Body.Bytes(), &reuseErr))
	require.Equal(t, "invalid_grant", reuseErr["error"])
}
