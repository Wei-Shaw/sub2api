package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/oidcrefreshtoken"

	"github.com/stretchr/testify/require"
)

// ─── 测试基础设施 ────────────────────────────────────────────────────────────

// providerTestEnv 聚合一套可直接驱动 OIDC 核心协议流程的测试依赖。
type providerTestEnv struct {
	client  *dbent.Client
	repo    *signingMemSettingRepo
	signing *OidcSigningService
	clients *OidcClientService
	svc     *OidcProviderService
}

// newProviderTestEnv 组装 provider 服务：内存 ent + 内存 setting repo (issuer 已配)
// + 已 EnsureActiveKey 的签名服务 + client/consent 服务。
func newProviderTestEnv(t *testing.T) *providerTestEnv {
	t.Helper()
	client := newOidcSigningTestClient(t)

	repo := newSigningMemSettingRepo()
	require.NoError(t, repo.Set(context.Background(), SettingKeyOidcProviderEnabled, "true"))
	require.NoError(t, repo.Set(context.Background(), SettingKeyOidcProviderIssuerURL, "https://op.example.com"))

	signing := NewOidcSigningService(client, repo)
	signing.rsaBits = 1024
	require.NoError(t, signing.EnsureActiveKey(context.Background()))

	clientSvc := newOidcClientServiceForTest(t, client)
	consentSvc := NewOidcConsentService(client)

	svc := NewOidcProviderService(client, repo, signing, clientSvc, consentSvc)

	return &providerTestEnv{
		client:  client,
		repo:    repo,
		signing: signing,
		clients: clientSvc,
		svc:     svc,
	}
}

// newRP 创建一个测试 RP 客户端并返回其 view。
func (e *providerTestEnv) newRP(t *testing.T, scopes []string, consentRequired bool) *OidcClientView {
	t.Helper()
	view, _, err := e.clients.Create(context.Background(), CreateOidcClientRequest{
		ClientName:      "Acme RP",
		RedirectURIs:    []string{"https://acme.example.com/cb"},
		AllowedScopes:   scopes,
		ConsentRequired: consentRequired,
		Enabled:         true,
	})
	require.NoError(t, err)
	return view
}

// newUser 创建一个测试用户。
func (e *providerTestEnv) newUser(t *testing.T) *dbent.User {
	t.Helper()
	u, err := e.client.User.Create().
		SetEmail("alice@example.com").
		SetUsername("alice").
		SetPasswordHash("hash").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		SetBalance(12.5).
		SetTotalRecharged(100).
		SetConcurrency(1).
		Save(context.Background())
	require.NoError(t, err)
	return u
}

// pkceChallenge 计算 base64url(sha256(verifier))。
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// ─── ValidateAuthorize (Authorize Endpoint 场景) ────────────────────────────

func TestOidcProvider_ValidateAuthorize_HappyPath(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid", "profile", "offline_access"}, true)

	validated, canRedirect, oerr := e.svc.ValidateAuthorize(context.Background(), OidcAuthorizeRequest{
		ClientID:      rp.ClientID,
		RedirectURI:   "https://acme.example.com/cb",
		ResponseType:  "code",
		Scope:         "openid profile",
		CodeChallenge: pkceChallenge("verifier-123"),
	})
	require.Nil(t, oerr)
	require.True(t, canRedirect)
	require.NotNil(t, validated)
	require.Equal(t, []string{"openid", "profile"}, validated.Scopes)
}

func TestOidcProvider_ValidateAuthorize_UnknownClient(t *testing.T) {
	e := newProviderTestEnv(t)
	_, canRedirect, oerr := e.svc.ValidateAuthorize(context.Background(), OidcAuthorizeRequest{
		ClientID:      "rp_does_not_exist",
		RedirectURI:   "https://acme.example.com/cb",
		ResponseType:  "code",
		Scope:         "openid",
		CodeChallenge: pkceChallenge("v"),
	})
	require.NotNil(t, oerr)
	require.Equal(t, "invalid_client", oerr.Code)
	// 不可信 client → 不能回跳
	require.False(t, canRedirect)
}

func TestOidcProvider_ValidateAuthorize_RedirectURIMismatch(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid"}, true)
	_, canRedirect, oerr := e.svc.ValidateAuthorize(context.Background(), OidcAuthorizeRequest{
		ClientID:      rp.ClientID,
		RedirectURI:   "https://evil.example.com/cb",
		ResponseType:  "code",
		Scope:         "openid",
		CodeChallenge: pkceChallenge("v"),
	})
	require.NotNil(t, oerr)
	require.Equal(t, "invalid_request", oerr.Code)
	require.False(t, canRedirect, "mismatched redirect_uri must not be redirected to")
}

func TestOidcProvider_ValidateAuthorize_RequiresPKCE(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid"}, true)
	_, canRedirect, oerr := e.svc.ValidateAuthorize(context.Background(), OidcAuthorizeRequest{
		ClientID:     rp.ClientID,
		RedirectURI:  "https://acme.example.com/cb",
		ResponseType: "code",
		Scope:        "openid",
		// 无 code_challenge
	})
	require.NotNil(t, oerr)
	require.Equal(t, "invalid_request", oerr.Code)
	require.True(t, canRedirect, "redirect_uri 合法时错误应可回跳")
}

func TestOidcProvider_ValidateAuthorize_RejectsNonS256(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid"}, true)
	_, _, oerr := e.svc.ValidateAuthorize(context.Background(), OidcAuthorizeRequest{
		ClientID:            rp.ClientID,
		RedirectURI:         "https://acme.example.com/cb",
		ResponseType:        "code",
		Scope:               "openid",
		CodeChallenge:       pkceChallenge("v"),
		CodeChallengeMethod: "plain",
	})
	require.NotNil(t, oerr)
	require.Equal(t, "invalid_request", oerr.Code)
}

func TestOidcProvider_ValidateAuthorize_ScopeMustIncludeOpenID(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid", "profile"}, true)
	_, _, oerr := e.svc.ValidateAuthorize(context.Background(), OidcAuthorizeRequest{
		ClientID:      rp.ClientID,
		RedirectURI:   "https://acme.example.com/cb",
		ResponseType:  "code",
		Scope:         "profile",
		CodeChallenge: pkceChallenge("v"),
	})
	require.NotNil(t, oerr)
	require.Equal(t, "invalid_scope", oerr.Code)
}

func TestOidcProvider_ValidateAuthorize_ScopeNotAllowed(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid"}, true)
	_, _, oerr := e.svc.ValidateAuthorize(context.Background(), OidcAuthorizeRequest{
		ClientID:      rp.ClientID,
		RedirectURI:   "https://acme.example.com/cb",
		ResponseType:  "code",
		Scope:         "openid sub2api:apikey",
		CodeChallenge: pkceChallenge("v"),
	})
	require.NotNil(t, oerr)
	require.Equal(t, "invalid_scope", oerr.Code)
}

func TestOidcProvider_ValidateAuthorize_UnsupportedResponseType(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid"}, true)
	_, _, oerr := e.svc.ValidateAuthorize(context.Background(), OidcAuthorizeRequest{
		ClientID:      rp.ClientID,
		RedirectURI:   "https://acme.example.com/cb",
		ResponseType:  "token",
		Scope:         "openid",
		CodeChallenge: pkceChallenge("v"),
	})
	require.NotNil(t, oerr)
	require.Equal(t, "unsupported_response_type", oerr.Code)
}

func TestOidcProvider_ValidateAuthorize_DisabledClient(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid"}, true)
	disabled := false
	_, err := e.clients.Update(context.Background(), rp.ID, UpdateOidcClientPatch{Enabled: &disabled})
	require.NoError(t, err)

	_, _, oerr := e.svc.ValidateAuthorize(context.Background(), OidcAuthorizeRequest{
		ClientID:      rp.ClientID,
		RedirectURI:   "https://acme.example.com/cb",
		ResponseType:  "code",
		Scope:         "openid",
		CodeChallenge: pkceChallenge("v"),
	})
	require.NotNil(t, oerr)
	require.Equal(t, "invalid_client", oerr.Code)
}

// ─── ExchangeCode (Token Endpoint: Authorization Code Grant) ────────────────

// issueValidCode 走完整 IssueCode 路径返回 (code, verifier)。
func (e *providerTestEnv) issueValidCode(t *testing.T, rp *OidcClientView, userID int64, scopes []string, nonce string) (string, string) {
	t.Helper()
	verifier := "verifier-abcdefghijklmnopqrstuvwxyz0123456789"
	code, err := e.svc.IssueCode(context.Background(), OidcIssueCodeInput{
		Client:      rp,
		UserID:      userID,
		Scopes:      scopes,
		RedirectURI: "https://acme.example.com/cb",
		Nonce:       nonce,
		Challenge:   pkceChallenge(verifier),
		Method:      "S256",
	})
	require.NoError(t, err)
	return code, verifier
}

func TestOidcProvider_ExchangeCode_Succeeds(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid", "profile", "offline_access"}, true)
	user := e.newUser(t)

	code, verifier := e.issueValidCode(t, rp, user.ID, []string{"openid", "profile", "offline_access"}, "n-1")

	resp, oerr := e.svc.ExchangeCode(context.Background(), OidcExchangeCodeInput{
		Client:       rp,
		Code:         code,
		RedirectURI:  "https://acme.example.com/cb",
		CodeVerifier: verifier,
	})
	require.Nil(t, oerr)
	require.NotEmpty(t, resp.AccessToken)
	require.Equal(t, "Bearer", resp.TokenType)
	require.NotEmpty(t, resp.RefreshToken, "offline_access 必须签发 refresh token")
	require.NotEmpty(t, resp.IDToken, "openid 必须签发 id token")
}

func TestOidcProvider_ExchangeCode_NoRefreshWithoutOfflineAccess(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid", "profile"}, true)
	user := e.newUser(t)

	code, verifier := e.issueValidCode(t, rp, user.ID, []string{"openid", "profile"}, "")
	resp, oerr := e.svc.ExchangeCode(context.Background(), OidcExchangeCodeInput{
		Client:       rp,
		Code:         code,
		RedirectURI:  "https://acme.example.com/cb",
		CodeVerifier: verifier,
	})
	require.Nil(t, oerr)
	require.Empty(t, resp.RefreshToken, "无 offline_access 不应签发 refresh token")
	require.NotEmpty(t, resp.IDToken)
}

func TestOidcProvider_ExchangeCode_SingleUse(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid", "offline_access"}, true)
	user := e.newUser(t)

	code, verifier := e.issueValidCode(t, rp, user.ID, []string{"openid", "offline_access"}, "")
	in := OidcExchangeCodeInput{Client: rp, Code: code, RedirectURI: "https://acme.example.com/cb", CodeVerifier: verifier}

	_, oerr := e.svc.ExchangeCode(context.Background(), in)
	require.Nil(t, oerr)

	// 第二次兑换同一 code → invalid_grant
	_, oerr2 := e.svc.ExchangeCode(context.Background(), in)
	require.NotNil(t, oerr2)
	require.Equal(t, "invalid_grant", oerr2.Code)

	// 复用 code 触发派生 token family 吊销
	revoked, err := e.client.OidcRefreshToken.Query().
		Where(oidcrefreshtoken.UserIDEQ(user.ID), oidcrefreshtoken.RevokedAtNotNil()).
		Count(context.Background())
	require.NoError(t, err)
	require.GreaterOrEqual(t, revoked, 1)
}

func TestOidcProvider_ExchangeCode_RedirectURIMismatch(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid"}, true)
	user := e.newUser(t)
	code, verifier := e.issueValidCode(t, rp, user.ID, []string{"openid"}, "")

	_, oerr := e.svc.ExchangeCode(context.Background(), OidcExchangeCodeInput{
		Client:       rp,
		Code:         code,
		RedirectURI:  "https://acme.example.com/other",
		CodeVerifier: verifier,
	})
	require.NotNil(t, oerr)
	require.Equal(t, "invalid_grant", oerr.Code)
}

func TestOidcProvider_ExchangeCode_PKCEMismatch(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid"}, true)
	user := e.newUser(t)
	code, _ := e.issueValidCode(t, rp, user.ID, []string{"openid"}, "")

	_, oerr := e.svc.ExchangeCode(context.Background(), OidcExchangeCodeInput{
		Client:       rp,
		Code:         code,
		RedirectURI:  "https://acme.example.com/cb",
		CodeVerifier: "wrong-verifier",
	})
	require.NotNil(t, oerr)
	require.Equal(t, "invalid_grant", oerr.Code)
}

func TestOidcProvider_ExchangeCode_UnknownCode(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid"}, true)
	_, oerr := e.svc.ExchangeCode(context.Background(), OidcExchangeCodeInput{
		Client:       rp,
		Code:         "nope",
		RedirectURI:  "https://acme.example.com/cb",
		CodeVerifier: "v",
	})
	require.NotNil(t, oerr)
	require.Equal(t, "invalid_grant", oerr.Code)
}

func TestOidcProvider_ExchangeCode_WrongClient(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid"}, true)
	user := e.newUser(t)
	code, verifier := e.issueValidCode(t, rp, user.ID, []string{"openid"}, "")

	other, _, err := e.clients.Create(context.Background(), CreateOidcClientRequest{
		ClientName:    "Other RP",
		RedirectURIs:  []string{"https://other.example.com/cb"},
		AllowedScopes: []string{"openid"},
		Enabled:       true,
	})
	require.NoError(t, err)

	_, oerr := e.svc.ExchangeCode(context.Background(), OidcExchangeCodeInput{
		Client:       other,
		Code:         code,
		RedirectURI:  "https://acme.example.com/cb",
		CodeVerifier: verifier,
	})
	require.NotNil(t, oerr)
	require.Equal(t, "invalid_grant", oerr.Code)
}

// ─── RefreshToken (Token Endpoint: Refresh Token Grant) ─────────────────────

// exchangeForRefresh 走完整 authorize→exchange 返回首个 refresh token。
func (e *providerTestEnv) exchangeForRefresh(t *testing.T, rp *OidcClientView, userID int64, scopes []string) string {
	t.Helper()
	code, verifier := e.issueValidCode(t, rp, userID, scopes, "")
	resp, oerr := e.svc.ExchangeCode(context.Background(), OidcExchangeCodeInput{
		Client:       rp,
		Code:         code,
		RedirectURI:  "https://acme.example.com/cb",
		CodeVerifier: verifier,
	})
	require.Nil(t, oerr)
	require.NotEmpty(t, resp.RefreshToken)
	return resp.RefreshToken
}

func TestOidcProvider_RefreshToken_RotationSucceeds(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid", "offline_access"}, true)
	user := e.newUser(t)
	refresh := e.exchangeForRefresh(t, rp, user.ID, []string{"openid", "offline_access"})

	resp, oerr := e.svc.RefreshToken(context.Background(), OidcRefreshInput{Client: rp, RefreshToken: refresh})
	require.Nil(t, oerr)
	require.NotEmpty(t, resp.RefreshToken)
	require.NotEqual(t, refresh, resp.RefreshToken, "轮转后应签发新 refresh token")
	require.NotEmpty(t, resp.AccessToken)

	// 旧 refresh token 应被标记吊销
	old, err := e.client.OidcRefreshToken.Query().
		Where(oidcrefreshtoken.TokenEQ(refresh)).
		Only(context.Background())
	require.NoError(t, err)
	require.NotNil(t, old.RevokedAt)
}

func TestOidcProvider_RefreshToken_ReuseTriggersFamilyRevocation(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid", "offline_access"}, true)
	user := e.newUser(t)
	refresh := e.exchangeForRefresh(t, rp, user.ID, []string{"openid", "offline_access"})

	// 第一次轮转得到新 token
	resp, oerr := e.svc.RefreshToken(context.Background(), OidcRefreshInput{Client: rp, RefreshToken: refresh})
	require.Nil(t, oerr)
	newToken := resp.RefreshToken

	// 再次使用已被吊销的旧 token → reuse 检测
	_, oerr2 := e.svc.RefreshToken(context.Background(), OidcRefreshInput{Client: rp, RefreshToken: refresh})
	require.NotNil(t, oerr2)
	require.Equal(t, "invalid_grant", oerr2.Code)

	// reuse 应吊销整个 family，连带刚签发的 newToken 也失效
	row, err := e.client.OidcRefreshToken.Query().
		Where(oidcrefreshtoken.TokenEQ(newToken)).
		Only(context.Background())
	require.NoError(t, err)
	require.NotNil(t, row.RevokedAt, "family 内所有 refresh token 均应被吊销")
}

func TestOidcProvider_RefreshToken_ScopeDowngrade(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid", "profile", "offline_access"}, true)
	user := e.newUser(t)
	refresh := e.exchangeForRefresh(t, rp, user.ID, []string{"openid", "profile", "offline_access"})

	resp, oerr := e.svc.RefreshToken(context.Background(), OidcRefreshInput{
		Client:       rp,
		RefreshToken: refresh,
		Scope:        "openid",
	})
	require.Nil(t, oerr)
	require.Equal(t, "openid", resp.Scope)
}

func TestOidcProvider_RefreshToken_ScopeUpgradeRejected(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid", "offline_access"}, true)
	user := e.newUser(t)
	refresh := e.exchangeForRefresh(t, rp, user.ID, []string{"openid", "offline_access"})

	_, oerr := e.svc.RefreshToken(context.Background(), OidcRefreshInput{
		Client:       rp,
		RefreshToken: refresh,
		Scope:        "openid profile", // 超出原始授权
	})
	require.NotNil(t, oerr)
	require.Equal(t, "invalid_scope", oerr.Code)
}

func TestOidcProvider_RefreshToken_UnknownToken(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid", "offline_access"}, true)
	_, oerr := e.svc.RefreshToken(context.Background(), OidcRefreshInput{Client: rp, RefreshToken: "nope"})
	require.NotNil(t, oerr)
	require.Equal(t, "invalid_grant", oerr.Code)
}

// ─── BuildUserInfo (UserInfo Endpoint) ──────────────────────────────────────

func TestOidcProvider_BuildUserInfo_ScopedClaims(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid", "profile", "email"}, true)
	user := e.newUser(t)

	code, verifier := e.issueValidCode(t, rp, user.ID, []string{"openid", "profile", "email"}, "")
	resp, oerr := e.svc.ExchangeCode(context.Background(), OidcExchangeCodeInput{
		Client: rp, Code: code, RedirectURI: "https://acme.example.com/cb", CodeVerifier: verifier,
	})
	require.Nil(t, oerr)

	claims, uerr := e.svc.BuildUserInfo(context.Background(), resp.AccessToken)
	require.Nil(t, uerr)
	require.Equal(t, "alice", claims["name"])
	require.Equal(t, "alice@example.com", claims["email"])
	require.Equal(t, true, claims["email_verified"])
	// 未授权的私有 scope 不应出现
	_, hasBalance := claims["sub2api_balance"]
	require.False(t, hasBalance)
}

func TestOidcProvider_BuildUserInfo_ApikeyScopeCountOnly(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid", "sub2api:apikey"}, true)
	user := e.newUser(t)

	code, verifier := e.issueValidCode(t, rp, user.ID, []string{"openid", "sub2api:apikey"}, "")
	resp, oerr := e.svc.ExchangeCode(context.Background(), OidcExchangeCodeInput{
		Client: rp, Code: code, RedirectURI: "https://acme.example.com/cb", CodeVerifier: verifier,
	})
	require.Nil(t, oerr)

	claims, uerr := e.svc.BuildUserInfo(context.Background(), resp.AccessToken)
	require.Nil(t, uerr)
	require.Contains(t, claims, "sub2api_apikey_count")
	require.Contains(t, claims, "sub2api_apikeys")

	// 验证 apikeys 字段是一个数组（实际类型为 []map[string]any）
	apikeys, ok := claims["sub2api_apikeys"].([]map[string]any)
	require.True(t, ok)
	require.Equal(t, 0, len(apikeys)) // 新用户没有 API Key
	require.Equal(t, 0, claims["sub2api_apikey_count"])
}

func TestOidcProvider_BuildUserInfo_ApikeyScopeReturnsKeyValue(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid", "sub2api:apikey"}, true)
	user := e.newUser(t)

	// 为用户创建一个 API Key
	_, err := e.client.APIKey.Create().
		SetUserID(user.ID).
		SetKey("sk-test-plaintext-key-123").
		SetName("test-key").
		SetStatus("active").
		Save(context.Background())
	require.NoError(t, err)

	code, verifier := e.issueValidCode(t, rp, user.ID, []string{"openid", "sub2api:apikey"}, "")
	resp, oerr := e.svc.ExchangeCode(context.Background(), OidcExchangeCodeInput{
		Client: rp, Code: code, RedirectURI: "https://acme.example.com/cb", CodeVerifier: verifier,
	})
	require.Nil(t, oerr)

	claims, uerr := e.svc.BuildUserInfo(context.Background(), resp.AccessToken)
	require.Nil(t, uerr)
	require.Equal(t, 1, claims["sub2api_apikey_count"])

	apikeys, ok := claims["sub2api_apikeys"].([]map[string]any)
	require.True(t, ok)
	require.Equal(t, 1, len(apikeys))
	require.Equal(t, "sk-test-plaintext-key-123", apikeys[0]["key"])
	require.Equal(t, "test-key", apikeys[0]["name"])
	require.Equal(t, "active", apikeys[0]["status"])
}

func TestOidcProvider_BuildUserInfo_MissingToken(t *testing.T) {
	e := newProviderTestEnv(t)
	_, uerr := e.svc.BuildUserInfo(context.Background(), "")
	require.NotNil(t, uerr)
	require.Equal(t, "invalid_token", uerr.Code)
	require.Equal(t, 401, uerr.Status)
}

func TestOidcProvider_BuildUserInfo_UnknownToken(t *testing.T) {
	e := newProviderTestEnv(t)
	_, uerr := e.svc.BuildUserInfo(context.Background(), "not-a-real-token")
	require.NotNil(t, uerr)
	require.Equal(t, "invalid_token", uerr.Code)
}

// ─── ResolveAccessToken (Resource Endpoint 鉴权) ────────────────────────────

func TestOidcProvider_ResolveAccessToken_OK(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid", "sub2api:apikey"}, true)
	user := e.newUser(t)

	code, verifier := e.issueValidCode(t, rp, user.ID, []string{"openid", "sub2api:apikey"}, "")
	resp, oerr := e.svc.ExchangeCode(context.Background(), OidcExchangeCodeInput{
		Client: rp, Code: code, RedirectURI: "https://acme.example.com/cb", CodeVerifier: verifier,
	})
	require.Nil(t, oerr)

	resolved, rerr := e.svc.ResolveAccessToken(context.Background(), resp.AccessToken)
	require.Nil(t, rerr)
	require.Equal(t, user.ID, resolved.UserID)
	require.Equal(t, rp.ClientID, resolved.ClientID)
	require.True(t, resolved.HasScope("sub2api:apikey"))
	require.True(t, resolved.HasScope("openid"))
	require.False(t, resolved.HasScope("sub2api:balance"))
}

func TestOidcProvider_ResolveAccessToken_MissingToken(t *testing.T) {
	e := newProviderTestEnv(t)
	_, rerr := e.svc.ResolveAccessToken(context.Background(), "")
	require.NotNil(t, rerr)
	require.Equal(t, "invalid_token", rerr.Code)
	require.Equal(t, 401, rerr.Status)
}

func TestOidcProvider_ResolveAccessToken_UnknownToken(t *testing.T) {
	e := newProviderTestEnv(t)
	_, rerr := e.svc.ResolveAccessToken(context.Background(), "not-a-real-token")
	require.NotNil(t, rerr)
	require.Equal(t, "invalid_token", rerr.Code)
	require.Equal(t, 401, rerr.Status)
}

// ─── 同意令牌 round-trip ─────────────────────────────────────────────────────

func TestOidcProvider_ConsentToken_RoundTrip(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid", "profile"}, true)

	tok, err := e.svc.IssueConsentToken(context.Background(), OidcConsentTokenInput{
		UserID:      77,
		ClientID:    rp.ClientID,
		Scopes:      []string{"openid", "profile"},
		RedirectURI: "https://acme.example.com/cb",
		State:       "st",
		Nonce:       "no",
		Challenge:   pkceChallenge("v"),
		Method:      "S256",
	})
	require.NoError(t, err)

	claims, oerr := e.svc.DecodeConsentToken(tok)
	require.Nil(t, oerr)
	require.Equal(t, int64(77), claims.UserID)
	require.Equal(t, rp.ClientID, claims.ClientID)
	require.Equal(t, []string{"openid", "profile"}, claims.Scopes)
}

func TestOidcProvider_ConsentToken_RejectsTampered(t *testing.T) {
	e := newProviderTestEnv(t)
	_, oerr := e.svc.DecodeConsentToken("garbage.token.value")
	require.NotNil(t, oerr)
	require.Equal(t, "invalid_request", oerr.Code)
}

// ─── ConsentNeeded (superset bypass) ────────────────────────────────────────

func TestOidcProvider_ConsentNeeded_SupersetBypass(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid", "profile"}, true)
	user := e.newUser(t)

	// 首次：需要同意
	need, err := e.svc.ConsentNeeded(context.Background(), rp, user.ID, []string{"openid", "profile"})
	require.NoError(t, err)
	require.True(t, need)

	// 记录授权后：覆盖请求 scope → 不再需要
	require.NoError(t, e.svc.RecordConsent(context.Background(), user.ID, rp.ClientID, []string{"openid", "profile"}))
	need2, err := e.svc.ConsentNeeded(context.Background(), rp, user.ID, []string{"openid"})
	require.NoError(t, err)
	require.False(t, need2)
}

func TestOidcProvider_ConsentNeeded_NotRequiredWhenFlagOff(t *testing.T) {
	e := newProviderTestEnv(t)
	rp := e.newRP(t, []string{"openid"}, false) // consent_required=false
	need, err := e.svc.ConsentNeeded(context.Background(), rp, 1, []string{"openid"})
	require.NoError(t, err)
	require.False(t, need)
}
