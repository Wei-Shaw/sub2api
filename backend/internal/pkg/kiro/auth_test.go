package kiro

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseAuthMethod(t *testing.T) {
	t.Parallel()

	require.Equal(t, AuthSocial, ParseAuthMethod("social"))
	require.Equal(t, AuthBuilderID, ParseAuthMethod("builder_id"))
	require.Equal(t, AuthIdC, ParseAuthMethod("idc"))
	require.Equal(t, AuthAPIKey, ParseAuthMethod("api_key"))
	require.Equal(t, AuthSocial, ParseAuthMethod("  SOCIAL  "))
	// 未知值退回 social —— 历史账号多数是 social 导入的。
	require.Equal(t, AuthSocial, ParseAuthMethod("whatever"))
}

func TestBaseURLs(t *testing.T) {
	t.Parallel()

	require.Equal(t, "https://oidc.us-east-1.amazonaws.com", OIDCBase("us-east-1"))
	require.Equal(t, "https://prod.us-east-1.auth.desktop.kiro.dev", SocialBase("us-east-1"))
	// 空 region 用默认。
	require.Contains(t, OIDCBase(""), defaultRegion)
	require.Contains(t, SocialBase(""), defaultRegion)
}

// TestRefreshSocialReturnsProfileArn 覆盖 §5.5 第 1 点：
// profileArn 必须从刷新响应回传，漏掉会导致账号跑一段时间后 403。
func TestRefreshSocialReturnsProfileArn(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/refreshToken", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "rt_old", body["refreshToken"])
		require.NotContains(t, body, "clientId", "social 刷新不带 clientId")

		_, _ = w.Write([]byte(`{
			"accessToken":"at_new","refreshToken":"rt_new",
			"expiresIn":3600,"profileArn":"arn:aws:codewhisperer:::profile/XYZ"
		}`))
	}))
	defer srv.Close()

	ts, err := RefreshSocial(context.Background(), srv.Client(), srv.URL, "rt_old")
	require.NoError(t, err)
	require.Equal(t, "at_new", ts.AccessToken)
	require.Equal(t, "rt_new", ts.RefreshToken)
	require.Equal(t, "arn:aws:codewhisperer:::profile/XYZ", ts.ProfileArn)
	require.WithinDuration(t, time.Now().Add(time.Hour), ts.ExpiresAt, 30*time.Second)
}

func TestRefreshSocialKeepsOldRefreshTokenWhenUpstreamOmitsIt(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"accessToken":"at","expiresIn":600}`))
	}))
	defer srv.Close()

	ts, err := RefreshSocial(context.Background(), srv.Client(), srv.URL, "rt_old")
	require.NoError(t, err)
	require.Equal(t, "rt_old", ts.RefreshToken, "上游不回 refreshToken 时必须沿用旧值")
}

func TestRefreshOIDCSendsClientCredentials(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/token", r.URL.Path)

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "cid", body["clientId"])
		require.Equal(t, "csecret", body["clientSecret"])
		require.Equal(t, "rt", body["refreshToken"])
		require.Equal(t, "refresh_token", body["grantType"])

		_, _ = w.Write([]byte(`{"accessToken":"at","refreshToken":"rt2","expiresIn":1800}`))
	}))
	defer srv.Close()

	ts, err := RefreshOIDC(context.Background(), srv.Client(), srv.URL, "cid", "csecret", "rt")
	require.NoError(t, err)
	require.Equal(t, "at", ts.AccessToken)
	require.Equal(t, "rt2", ts.RefreshToken)
}

func TestRefreshOIDCRequiresClientCredentials(t *testing.T) {
	t.Parallel()

	_, err := RefreshOIDC(context.Background(), http.DefaultClient, "http://unused", "", "", "rt")
	require.Error(t, err)
	require.Contains(t, err.Error(), "clientId")
}

func TestRefreshPropagatesUpstreamError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()

	_, err := RefreshSocial(context.Background(), srv.Client(), srv.URL, "rt")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid_grant")
}

func TestRegisterOIDCClientIdCFlow(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/client/register", r.URL.Path)

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "https://d-90667b4f8e.awsapps.com/start", body["issuerUrl"])
		require.Equal(t, "public", body["clientType"])

		grants, ok := body["grantTypes"].([]any)
		require.True(t, ok)
		require.Contains(t, grants, "authorization_code")
		require.Contains(t, grants, "refresh_token")
		require.NotContains(t, grants, "urn:ietf:params:oauth:grant-type:device_code")

		redirects, ok := body["redirectUris"].([]any)
		require.True(t, ok)
		require.Equal(t, []any{"https://gw.example.com/admin/kiro/oauth/callback"}, redirects)

		_, _ = w.Write([]byte(`{"clientId":"cid","clientSecret":"csec"}`))
	}))
	defer srv.Close()

	reg, err := RegisterOIDCClient(context.Background(), srv.Client(), srv.URL,
		"https://d-90667b4f8e.awsapps.com/start",
		"https://gw.example.com/admin/kiro/oauth/callback", false)
	require.NoError(t, err)
	require.Equal(t, "cid", reg.ClientID)
	require.Equal(t, "csec", reg.ClientSecret)
}

func TestRegisterOIDCClientDeviceFlowRequestsDeviceGrant(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))

		grants, ok := body["grantTypes"].([]any)
		require.True(t, ok)
		require.Contains(t, grants, "urn:ietf:params:oauth:grant-type:device_code")
		require.Contains(t, grants, "refresh_token")

		_, _ = w.Write([]byte(`{"clientId":"cid","clientSecret":"csec"}`))
	}))
	defer srv.Close()

	_, err := RegisterOIDCClient(context.Background(), srv.Client(), srv.URL, BuilderIDStartURL, "", true)
	require.NoError(t, err)
}

func TestNewPKCEProducesValidChallenge(t *testing.T) {
	t.Parallel()

	p, err := NewPKCE()
	require.NoError(t, err)
	require.NotEmpty(t, p.Verifier)
	require.NotEmpty(t, p.Challenge)
	require.NotEqual(t, p.Verifier, p.Challenge)
	// base64url 无填充。
	require.NotContains(t, p.Challenge, "=")
	require.NotContains(t, p.Challenge, "+")
	require.NotContains(t, p.Challenge, "/")

	other, err := NewPKCE()
	require.NoError(t, err)
	require.NotEqual(t, p.Verifier, other.Verifier, "每次必须不同")
}

func TestBuildAuthorizeURLCarriesPKCEAndScopes(t *testing.T) {
	t.Parallel()

	raw := BuildAuthorizeURL("https://oidc.us-east-1.amazonaws.com", "cid",
		"https://gw.example.com/cb", "state-1", "chal-1")

	u, err := url.Parse(raw)
	require.NoError(t, err)
	require.Equal(t, "/authorize", u.Path)

	q := u.Query()
	require.Equal(t, "code", q.Get("response_type"))
	require.Equal(t, "cid", q.Get("client_id"))
	require.Equal(t, "https://gw.example.com/cb", q.Get("redirect_uri"))
	require.Equal(t, "state-1", q.Get("state"))
	require.Equal(t, "chal-1", q.Get("code_challenge"))
	require.Equal(t, "S256", q.Get("code_challenge_method"))
	require.Contains(t, q.Get("scopes"), "codewhisperer:conversations")
}

func TestExchangeAuthorizationCode(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/token", r.URL.Path)

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "authorization_code", body["grantType"])
		require.Equal(t, "the-code", body["code"])
		require.Equal(t, "the-verifier", body["codeVerifier"])

		_, _ = w.Write([]byte(`{"accessToken":"at","refreshToken":"rt","expiresIn":3600,"profileArn":"arn:x"}`))
	}))
	defer srv.Close()

	ts, err := ExchangeAuthorizationCode(context.Background(), srv.Client(), srv.URL,
		"cid", "csec", "the-code", "the-verifier", "https://gw.example.com/cb")
	require.NoError(t, err)
	require.Equal(t, "at", ts.AccessToken)
	require.Equal(t, "arn:x", ts.ProfileArn)
}

func TestStartDeviceAuthorization(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/device_authorization", r.URL.Path)

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "cid", body["clientId"])
		require.Equal(t, BuilderIDStartURL, body["startUrl"])

		_, _ = w.Write([]byte(`{
			"deviceCode":"dc","userCode":"ABCD-EFGH",
			"verificationUri":"https://view.awsapps.com/start/#/device",
			"verificationUriComplete":"https://view.awsapps.com/start/#/device?user_code=ABCD-EFGH",
			"expiresIn":600,"interval":5
		}`))
	}))
	defer srv.Close()

	da, err := StartDeviceAuthorization(context.Background(), srv.Client(), srv.URL, "cid", "csec", BuilderIDStartURL)
	require.NoError(t, err)
	require.Equal(t, "dc", da.DeviceCode)
	require.Equal(t, "ABCD-EFGH", da.UserCode)
	require.Contains(t, da.VerificationURIComplete, "ABCD-EFGH")
	require.Equal(t, 5, da.Interval)
}

// TestPollDeviceTokenPendingAndSlowDown 覆盖设备码轮询的三种非终态。
func TestPollDeviceTokenPendingAndSlowDown(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		errCode string
		want    error
	}{
		{"pending", "authorization_pending", ErrAuthorizationPending},
		{"slow_down", "slow_down", ErrSlowDown},
		{"expired", "expired_token", ErrDeviceCodeExpired},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"` + tc.errCode + `"}`))
			}))
			defer srv.Close()

			_, err := PollDeviceToken(context.Background(), srv.Client(), srv.URL, "cid", "csec", "dc")
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestPollDeviceTokenSuccess(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "urn:ietf:params:oauth:grant-type:device_code", body["grantType"])
		require.Equal(t, "dc", body["deviceCode"])

		_, _ = w.Write([]byte(`{"accessToken":"at","refreshToken":"rt","expiresIn":3600}`))
	}))
	defer srv.Close()

	ts, err := PollDeviceToken(context.Background(), srv.Client(), srv.URL, "cid", "csec", "dc")
	require.NoError(t, err)
	require.Equal(t, "at", ts.AccessToken)
}

func TestDefaultScopesCoverCodeWhisperer(t *testing.T) {
	t.Parallel()

	joined := strings.Join(DefaultScopes, ",")
	for _, want := range []string{
		"codewhisperer:completions", "codewhisperer:analysis",
		"codewhisperer:conversations", "codewhisperer:transformations",
		"codewhisperer:taskassist",
	} {
		require.Contains(t, joined, want)
	}
}
