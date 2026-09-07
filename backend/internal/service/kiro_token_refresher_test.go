//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestKiroRefresherCanRefresh(t *testing.T) {
	r := NewKiroTokenRefresher(nil)

	oauth := &Account{Platform: PlatformKiro, Credentials: map[string]any{
		"auth_method": "social", "refresh_token": "rt",
	}}
	require.True(t, r.CanRefresh(oauth))

	// API Key 账号不刷新 —— 否则后台循环会持续报错。
	apiKey := &Account{Platform: PlatformKiro, Credentials: map[string]any{
		"auth_method": "api_key", "api_key": "k",
	}}
	require.False(t, r.CanRefresh(apiKey))

	// 无 refresh token。
	require.False(t, r.CanRefresh(&Account{Platform: PlatformKiro, Credentials: map[string]any{
		"auth_method": "social",
	}}))

	// 别的平台。
	require.False(t, r.CanRefresh(&Account{Platform: PlatformAnthropic, Credentials: map[string]any{
		"refresh_token": "rt",
	}}))

	require.False(t, r.CanRefresh(nil))
}

func TestKiroRefresherNeedsRefresh(t *testing.T) {
	r := NewKiroTokenRefresher(nil)
	window := time.Hour

	// 没有 access token → 需要刷新。
	require.True(t, r.NeedsRefresh(&Account{ID: 1, Credentials: map[string]any{
		"refresh_token": "rt",
	}}, window))

	// 没有 expires_at → 需要刷新。
	require.True(t, r.NeedsRefresh(&Account{ID: 1, Credentials: map[string]any{
		"refresh_token": "rt", "access_token": "at",
	}}, window))

	// 远未过期 → 不需要。
	require.False(t, r.NeedsRefresh(&Account{ID: 1, Credentials: map[string]any{
		"refresh_token": "rt", "access_token": "at",
		"expires_at": time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
	}}, window))

	// 即将过期 → 需要。
	require.True(t, r.NeedsRefresh(&Account{ID: 1, Credentials: map[string]any{
		"refresh_token": "rt", "access_token": "at",
		"expires_at": time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339),
	}}, window))

	// 无 refresh token 时不参与刷新判定。
	require.False(t, r.NeedsRefresh(&Account{ID: 1, Credentials: map[string]any{
		"access_token": "at",
	}}, window))
}

// TestKiroRefresherPreservesExistingCredentials 覆盖关键回归：
// machine_id / fake_thinking / issuer_url 都不在刷新响应里，丢了等于换设备。
func TestKiroRefresherPreservesExistingCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"accessToken":"at_new","refreshToken":"rt_new",
			"expiresIn":3600,"profileArn":"arn:new"}`))
	}))
	defer srv.Close()

	oauthSvc := newTestKiroOAuthService(t, srv)
	r := NewKiroTokenRefresher(oauthSvc)

	account := &Account{ID: 7, Platform: PlatformKiro, Credentials: map[string]any{
		"auth_method":   "social",
		"refresh_token": "rt_old",
		"access_token":  "at_old",
		"machine_id":    "fixed-machine-id",
		"fake_thinking": true,
		"region":        "us-east-1",
	}}

	got, err := r.Refresh(context.Background(), account)
	require.NoError(t, err)

	require.Equal(t, "at_new", got["access_token"])
	require.Equal(t, "rt_new", got["refresh_token"])
	require.Equal(t, "arn:new", got["profile_arn"], "profile_arn 必须回写")

	require.Equal(t, "fixed-machine-id", got["machine_id"], "设备指纹不得因刷新而改变")
	require.Equal(t, true, got["fake_thinking"], "账号级开关不得丢失")
	require.Equal(t, "us-east-1", got["region"])
}

// TestKiroRefresherBackfillsMissingProfileArnForIdCAccount 覆盖新增的
// profileArn 自动发现回填：idc 账号的 OIDC 刷新响应本身不带 profileArn
// （真实行为，见 tokenResponse.ProfileArn 只在 social 场景填充），但
// ListAvailableProfiles 探测能找到一个 profile 时，Refresh 的返回值必须
// 把它补上，而不是让 profile_arn 一直空着等管理员手填。
func TestKiroRefresherBackfillsMissingProfileArnForIdCAccount(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		// OIDC 刷新响应里没有 profileArn 字段——与真实 IdC 行为一致。
		_, _ = w.Write([]byte(`{"accessToken":"at_new","refreshToken":"rt_new","expiresIn":3600}`))
	})
	mux.HandleFunc("/ListAvailableProfiles", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer at_new", r.Header.Get("Authorization"), "发现请求必须用刷新后的新 access token，不能用旧的")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"profiles": []map[string]string{{"arn": kiroTestValidProfileArn, "profileName": "default"}},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	oauthSvc := newTestKiroOAuthService(t, srv)
	oauthSvc.listProfilesHost = func(string) string { return srv.URL }
	r := NewKiroTokenRefresher(oauthSvc)

	account := &Account{ID: 8, Platform: PlatformKiro, Credentials: map[string]any{
		"auth_method":   "idc",
		"refresh_token": "rt_old",
		"access_token":  "at_old",
		"client_id":     "client-1",
		"client_secret": "secret-1",
		"issuer_url":    "https://d-example.awsapps.com/start",
		"region":        "us-east-1",
		"machine_id":    "fixed-machine-id",
	}}

	got, err := r.Refresh(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "at_new", got["access_token"])
	require.Equal(t, kiroTestValidProfileArn, got["profile_arn"], "刷新后仍然缺 profile_arn 时必须尝试自动发现并回填")
}

// TestKiroRefresherKeepsExistingProfileArnWithoutRediscovering 覆盖已经有
// profile_arn 时不应该再去发现——不需要浪费一次额外的 HTTP 调用，也不能
// 让发现失败/换了一个不同的 profile 意外覆盖已经生效的值。
func TestKiroRefresherKeepsExistingProfileArnWithoutRediscovering(t *testing.T) {
	discoveryCalled := false
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"accessToken":"at_new","refreshToken":"rt_new","expiresIn":3600}`))
	})
	mux.HandleFunc("/ListAvailableProfiles", func(w http.ResponseWriter, r *http.Request) {
		discoveryCalled = true
		_ = json.NewEncoder(w).Encode(map[string]any{"profiles": []map[string]string{}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	oauthSvc := newTestKiroOAuthService(t, srv)
	oauthSvc.listProfilesHost = func(string) string { return srv.URL }
	r := NewKiroTokenRefresher(oauthSvc)

	account := &Account{ID: 8, Platform: PlatformKiro, Credentials: map[string]any{
		"auth_method":   "idc",
		"refresh_token": "rt_old",
		"access_token":  "at_old",
		"client_id":     "client-1",
		"client_secret": "secret-1",
		"issuer_url":    "https://d-example.awsapps.com/start",
		"region":        "us-east-1",
		"profile_arn":   kiroTestValidProfileArn,
	}}

	got, err := r.Refresh(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, kiroTestValidProfileArn, got["profile_arn"])
	require.False(t, discoveryCalled, "已经有 profile_arn 时不应该再发起发现请求")
}

func TestKiroRefresherCacheKey(t *testing.T) {
	r := NewKiroTokenRefresher(nil)
	require.Equal(t, "kiro:account:9", r.CacheKey(&Account{ID: 9}))
}

// TestKiroRefresherImplementsExecutorInterface 保证注册表能接受它。
func TestKiroRefresherImplementsExecutorInterface(t *testing.T) {
	var _ TokenRefresher = (*KiroTokenRefresher)(nil)
	var _ OAuthRefreshExecutor = (*KiroTokenRefresher)(nil)
}
