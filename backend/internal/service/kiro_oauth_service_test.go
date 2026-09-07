//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/stretchr/testify/require"
)

// kiroSessionStoreLocalOnlyLen 通过只读反射读取 kiro.SessionStore 未导出的
// localOnly map 长度，用来证明 ExchangeCode 自己调用了 Delete（见下面
// TestKiroExchangeCodeConsumedSessionLeavesNoLocalOnlyResidue 的收窄说明）。
// 只调用 Len()，不调用 Interface()，所以不需要 unsafe.Pointer/reflect.NewAt
// 去绕过未导出字段的只读限制——普通的 reflect.Value.Len() 本来就能读。
func kiroSessionStoreLocalOnlyLen(t *testing.T, store *kiro.SessionStore) int {
	t.Helper()
	return reflect.ValueOf(store).Elem().FieldByName("localOnly").Len()
}

// newTestKiroOAuthService 返回一个把两个 base URL 都指向 srv 的服务实例。
func newTestKiroOAuthService(t *testing.T, srv *httptest.Server) *KiroOAuthService {
	t.Helper()
	svc := NewKiroOAuthService(nil)
	t.Cleanup(svc.Stop)
	svc.oidcBase = func(string) string { return srv.URL }
	svc.socialBase = func(string) string { return srv.URL }
	return svc
}

// TestKiroGenerateAuthURLRegistersClientAndStoresSession 同时覆盖真实账号
// 联调发现的回归：AWS SSO-OIDC 的 client/register 对 clientType=public 强制
// 要求 redirect_uri 是裸 loopback 地址（服务端自建回调页会被拒，见
// kiroIdCRedirectURI 的文档），必须固定用这个值注册/授权，不能再接受调用方
// 传入的地址。
func TestKiroGenerateAuthURLRegistersClientAndStoresSession(t *testing.T) {
	var registeredRedirectURIs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/client/register", r.URL.Path)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		if raw, ok := body["redirectUris"].([]any); ok {
			for _, v := range raw {
				registeredRedirectURIs = append(registeredRedirectURIs, v.(string))
			}
		}
		_, _ = w.Write([]byte(`{"clientId":"cid","clientSecret":"csec"}`))
	}))
	defer srv.Close()

	svc := newTestKiroOAuthService(t, srv)

	res, err := svc.GenerateAuthURL(context.Background(), &KiroAuthURLInput{
		IssuerURL: "https://d-90667b4f8e.awsapps.com/start",
		Region:    "us-east-1",
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.SessionID)
	require.Positive(t, res.ExpiresIn)

	require.Equal(t, []string{kiroIdCRedirectURI}, registeredRedirectURIs,
		"client/register 必须固定用裸 loopback 地址注册，不能是服务端自建回调页——"+
			"真实 AWS 账号会拒绝后者（invalid_redirect_uri）")

	u, err := url.Parse(res.AuthorizeURL)
	require.NoError(t, err)
	q := u.Query()
	require.Equal(t, "cid", q.Get("client_id"))
	require.Equal(t, "S256", q.Get("code_challenge_method"))
	require.Equal(t, kiroIdCRedirectURI, q.Get("redirect_uri"))
	require.NotEmpty(t, q.Get("state"))

	// 会话必须落库，且带上 PKCE verifier 与客户端凭据。
	sess, ok := svc.sessionStore.Get(context.Background(), res.SessionID)
	require.True(t, ok)
	require.Equal(t, kiro.AuthIdC, sess.Method)
	require.Equal(t, "cid", sess.ClientID)
	require.Equal(t, "csec", sess.ClientSecret)
	require.NotEmpty(t, sess.Verifier)
	require.Equal(t, kiroIdCRedirectURI, sess.RedirectURI)
	require.Equal(t, q.Get("state"), sess.State)
}

func TestKiroGenerateAuthURLRequiresIssuer(t *testing.T) {
	svc := NewKiroOAuthService(nil)
	defer svc.Stop()

	_, err := svc.GenerateAuthURL(context.Background(), &KiroAuthURLInput{})
	require.Error(t, err)
}

// --- CompleteIdCLogin：管理员手动粘贴回调 URL 完成 IdC 授权码兑换 ---

func TestKiroCompleteIdCLoginParsesCallbackURLAndExchanges(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/token", r.URL.Path)
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "the-code", body["code"])
		_, _ = w.Write([]byte(`{"accessToken":"at","refreshToken":"rt","expiresIn":3600,"profileArn":"arn:x"}`))
	}))
	defer srv.Close()

	svc := newTestKiroOAuthService(t, srv)
	ctx := context.Background()
	svc.sessionStore.Set(ctx, "sid", &kiro.OAuthSession{
		Method: kiro.AuthIdC, State: "st", ClientID: "cid", ClientSecret: "csec",
		Verifier: "ver", RedirectURI: kiroIdCRedirectURI, Region: "us-east-1",
		ExpiresAt: time.Now().Add(kiro.SessionTTL),
	})

	// 真实场景：这个 URL 是浏览器地址栏里"无法连接"页面的完整地址，
	// 管理员手动复制粘贴过来的。
	pasted := kiroIdCRedirectURI + "?code=the-code&state=st"

	ts, sess, err := svc.CompleteIdCLogin(ctx, "sid", pasted, nil)
	require.NoError(t, err)
	require.Equal(t, "at", ts.AccessToken)
	require.Equal(t, "arn:x", ts.ProfileArn)
	require.Equal(t, kiro.AuthIdC, sess.Method)
}

func TestKiroCompleteIdCLoginSurfacesAuthorizeError(t *testing.T) {
	svc := NewKiroOAuthService(nil)
	defer svc.Stop()
	ctx := context.Background()
	svc.sessionStore.Set(ctx, "sid", &kiro.OAuthSession{
		Method: kiro.AuthIdC, State: "st", ExpiresAt: time.Now().Add(kiro.SessionTTL),
	})

	pasted := kiroIdCRedirectURI + "?error=access_denied&error_description=User+declined"
	_, _, err := svc.CompleteIdCLogin(ctx, "sid", pasted, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "access_denied")
}

func TestKiroCompleteIdCLoginRejectsUnparsableURL(t *testing.T) {
	svc := NewKiroOAuthService(nil)
	defer svc.Stop()

	_, _, err := svc.CompleteIdCLogin(context.Background(), "sid", "://not a url", nil)
	require.Error(t, err)
}

func TestKiroCompleteIdCLoginRequiresSessionID(t *testing.T) {
	svc := NewKiroOAuthService(nil)
	defer svc.Stop()

	_, _, err := svc.CompleteIdCLogin(context.Background(), "", kiroIdCRedirectURI+"?code=c&state=s", nil)
	require.Error(t, err)
}

// TestKiroExchangeCodeRejectsStateMismatch 是 CSRF 防护回归。
func TestKiroExchangeCodeRejectsStateMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("state 不匹配时不应发起 token 交换")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	svc := newTestKiroOAuthService(t, srv)
	ctx := context.Background()
	svc.sessionStore.Set(ctx, "sid", &kiro.OAuthSession{
		Method: kiro.AuthIdC, State: "correct", ExpiresAt: time.Now().Add(kiro.SessionTTL),
	})

	_, _, err := svc.ExchangeCode(ctx, &KiroExchangeCodeInput{
		SessionID: "sid", Code: "c", State: "forged",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "state")
}

// TestKiroExchangeCodeIsSingleUse 是重放防护回归：
// 回调 URL 会留在浏览器历史里，第二次兑换必须失败。
func TestKiroExchangeCodeIsSingleUse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/token", r.URL.Path)
		_, _ = w.Write([]byte(`{"accessToken":"at","refreshToken":"rt","expiresIn":3600,"profileArn":"arn:x"}`))
	}))
	defer srv.Close()

	svc := newTestKiroOAuthService(t, srv)
	ctx := context.Background()
	svc.sessionStore.Set(ctx, "sid", &kiro.OAuthSession{
		Method: kiro.AuthIdC, State: "st", ClientID: "cid", ClientSecret: "csec",
		Verifier: "ver", RedirectURI: "https://gw/cb", Region: "us-east-1",
		ExpiresAt: time.Now().Add(kiro.SessionTTL),
	})

	ts, sess, err := svc.ExchangeCode(ctx, &KiroExchangeCodeInput{SessionID: "sid", Code: "c", State: "st"})
	require.NoError(t, err)
	require.Equal(t, "at", ts.AccessToken)
	require.Equal(t, "arn:x", ts.ProfileArn)
	require.Equal(t, kiro.AuthIdC, sess.Method)

	_, _, err = svc.ExchangeCode(ctx, &KiroExchangeCodeInput{SessionID: "sid", Code: "c", State: "st"})
	require.Error(t, err, "同一授权码不得兑换两次")
}

func TestKiroExchangeCodeUnknownSession(t *testing.T) {
	svc := NewKiroOAuthService(nil)
	defer svc.Stop()

	_, _, err := svc.ExchangeCode(context.Background(), &KiroExchangeCodeInput{
		SessionID: "nope", Code: "c", State: "s",
	})
	require.Error(t, err)
}

func TestKiroStartAndPollDeviceAuth(t *testing.T) {
	var tokenCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/client/register":
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Contains(t, body["grantTypes"].([]any),
				"urn:ietf:params:oauth:grant-type:device_code")
			_, _ = w.Write([]byte(`{"clientId":"cid","clientSecret":"csec"}`))
		case "/device_authorization":
			_, _ = w.Write([]byte(`{"deviceCode":"dc","userCode":"ABCD-EFGH",
				"verificationUri":"https://view.awsapps.com/start/#/device",
				"verificationUriComplete":"https://view.awsapps.com/start/#/device?user_code=ABCD-EFGH",
				"expiresIn":600,"interval":5}`))
		case "/token":
			tokenCalls++
			if tokenCalls == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
				return
			}
			_, _ = w.Write([]byte(`{"accessToken":"at","refreshToken":"rt","expiresIn":3600}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	svc := newTestKiroOAuthService(t, srv)
	ctx := context.Background()

	res, err := svc.StartDeviceAuth(ctx, nil, "us-east-1")
	require.NoError(t, err)
	require.Equal(t, "ABCD-EFGH", res.UserCode)
	require.Contains(t, res.VerificationURIComplete, "ABCD-EFGH")
	require.Equal(t, 5, res.Interval)

	// 首次轮询：尚未授权。
	_, _, err = svc.PollDeviceAuth(ctx, res.SessionID, nil)
	require.ErrorIs(t, err, kiro.ErrAuthorizationPending)

	// 会话必须保留，供继续轮询。
	_, ok := svc.sessionStore.Get(ctx, res.SessionID)
	require.True(t, ok, "pending 不得销毁会话")

	// 第二次：成功。
	ts, sess, err := svc.PollDeviceAuth(ctx, res.SessionID, nil)
	require.NoError(t, err)
	require.Equal(t, "at", ts.AccessToken)
	require.Equal(t, kiro.AuthBuilderID, sess.Method)
}

func TestKiroRefreshAccountTokenDispatchesByAuthMethod(t *testing.T) {
	var socialHits, oidcHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/refreshToken":
			socialHits++
		case "/token":
			oidcHits++
		}
		_, _ = w.Write([]byte(`{"accessToken":"at","refreshToken":"rt2","expiresIn":3600,"profileArn":"arn:y"}`))
	}))
	defer srv.Close()

	svc := newTestKiroOAuthService(t, srv)
	ctx := context.Background()

	social := &Account{ID: 1, Platform: PlatformKiro, Credentials: map[string]any{
		"auth_method": "social", "refresh_token": "rt",
	}}
	ts, err := svc.RefreshAccountToken(ctx, social)
	require.NoError(t, err)
	require.Equal(t, "arn:y", ts.ProfileArn)
	require.Equal(t, 1, socialHits)

	idc := &Account{ID: 2, Platform: PlatformKiro, Credentials: map[string]any{
		"auth_method": "idc", "refresh_token": "rt",
		"client_id": "cid", "client_secret": "csec",
	}}
	_, err = svc.RefreshAccountToken(ctx, idc)
	require.NoError(t, err)
	require.Equal(t, 1, oidcHits)
}

func TestKiroRefreshAccountTokenRejectsAPIKeyAccounts(t *testing.T) {
	svc := NewKiroOAuthService(nil)
	defer svc.Stop()

	apiKeyAcc := &Account{ID: 3, Platform: PlatformKiro, Credentials: map[string]any{
		"auth_method": "api_key", "api_key": "k",
	}}
	_, err := svc.RefreshAccountToken(context.Background(), apiKeyAcc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "api_key")
}

// TestKiroBuildAccountCredentialsWritesProfileArn 覆盖 §5.5 第 1 点。
func TestKiroBuildAccountCredentialsWritesProfileArn(t *testing.T) {
	svc := NewKiroOAuthService(nil)
	defer svc.Stop()

	creds := svc.BuildAccountCredentials(KiroCredentialInput{
		TokenSet: &kiro.TokenSet{
			AccessToken: "at", RefreshToken: "rt",
			ProfileArn: "arn:aws:codewhisperer:::profile/ABC",
			ExpiresAt:  time.Now().Add(time.Hour),
		},
		Method:       kiro.AuthIdC,
		Region:       "us-east-1",
		IssuerURL:    "https://d-90667b4f8e.awsapps.com/start",
		ClientID:     "cid",
		ClientSecret: "csec",
	})

	require.Equal(t, "at", creds["access_token"])
	require.Equal(t, "rt", creds["refresh_token"])
	require.Equal(t, "arn:aws:codewhisperer:::profile/ABC", creds["profile_arn"])
	require.Equal(t, "idc", creds["auth_method"])
	require.Equal(t, "us-east-1", creds["region"])
	require.Equal(t, "cid", creds["client_id"])
	require.Equal(t, "csec", creds["client_secret"])
	require.NotEmpty(t, creds["expires_at"])
	require.NotEmpty(t, creds["machine_id"], "首次建号即固化设备指纹")
}

func TestKiroBuildAccountCredentialsOmitsEmptyClientCreds(t *testing.T) {
	svc := NewKiroOAuthService(nil)
	defer svc.Stop()

	creds := svc.BuildAccountCredentials(KiroCredentialInput{
		TokenSet: &kiro.TokenSet{AccessToken: "at", RefreshToken: "rt"},
		Method:   kiro.AuthSocial,
		Region:   "us-east-1",
	})

	require.NotContains(t, creds, "client_id", "social 不产生客户端凭据")
	require.NotContains(t, creds, "client_secret")
	require.Equal(t, "social", creds["auth_method"])
}

// TestKiroExchangeCodeConsumedSessionLeavesNoLocalOnlyResidue 是
// localOnly 泄漏回归（非 brief 要求，本任务分派额外要求）。
//
// SessionStore 自己的契约——「TryConsume 不清 localOnly 标记，只有
// Delete 才清」——现在由 pkg/kiro 包内的
// TestSessionStoreTryConsumeLeavesLocalOnlyUntilDelete 白盒覆盖（同包、
// 直接访问未导出字段，零反射）。这个测试的职责收窄为只证明一件事：
// ExchangeCode 自己确实调用了 Delete（localOnly 计数从 1 变成 0），
// 不重新证明 SessionStore 那条通用契约——那已经是 pkg/kiro 的活了。
//
// 仅凭 sessionStore.Get() 观察不到 Delete 是否被调用：TryConsume 单独
// 调用后 Get() 就已经返回 false 了（因为 memory 表项被删了），无法
// 区分「TryConsume 自己顺带够用了」和「Delete 真的被调用了」。所以
// 仍然需要 kiroSessionStoreLocalOnlyLen 这个跨包只读反射 helper。
func TestKiroExchangeCodeConsumedSessionLeavesNoLocalOnlyResidue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"accessToken":"at","refreshToken":"rt","expiresIn":3600,"profileArn":"arn:x"}`))
	}))
	defer srv.Close()

	svc := newTestKiroOAuthService(t, srv)
	ctx := context.Background()
	svc.sessionStore.Set(ctx, "sid", &kiro.OAuthSession{
		Method: kiro.AuthIdC, State: "st", ClientID: "cid", ClientSecret: "csec",
		Verifier: "ver", RedirectURI: "https://gw/cb", Region: "us-east-1",
		ExpiresAt: time.Now().Add(kiro.SessionTTL),
	})
	// 服务默认的内存会话存储没有配置 Redis，所以这个会话必然落 localOnly 路径
	// （与生产环境 Redis 降级时的路径完全一致）。
	require.Equal(t, 1, kiroSessionStoreLocalOnlyLen(t, svc.sessionStore),
		"Set 之后应当有且仅有一条 localOnly 标记")

	_, _, err := svc.ExchangeCode(ctx, &KiroExchangeCodeInput{SessionID: "sid", Code: "c", State: "st"})
	require.NoError(t, err)

	_, ok := svc.sessionStore.Get(ctx, "sid")
	require.False(t, ok, "会话应当在成功兑换后彻底消失")
	require.Zero(t, kiroSessionStoreLocalOnlyLen(t, svc.sessionStore),
		"ExchangeCode 必须调用 Delete 清理 localOnly 标记，否则每完成一次 IdC 授权就永久泄漏一条记录")
}
