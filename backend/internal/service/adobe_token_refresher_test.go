//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
	"github.com/stretchr/testify/require"
)

func adobeTestToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(claims)
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." +
		base64.RawURLEncoding.EncodeToString(raw) + ".sig"
}

func newAdobeTestRefresher(t *testing.T, handler func(*adobe.Request, int) (*adobe.Response, error)) *AdobeTokenRefresher {
	t.Helper()
	transport := &adobeFakeTransport{handler: handler}
	client := adobe.NewClient(adobe.ClientConfig{Transport: transport, DownloadTransport: transport})
	r := NewAdobeTokenRefresher()
	r.clients.newClient = func(string) *adobe.Client { return client }
	return r
}

func TestAdobeTokenRefresherCanRefresh(t *testing.T) {
	withCookie := &Account{
		Platform: PlatformAdobe, Type: AccountTypeOAuth,
		Credentials: map[string]any{"cookie": "aux_sid=1"},
	}
	r := NewAdobeTokenRefresher()
	require.True(t, r.CanRefresh(withCookie))

	// 没有 cookie 就换不了 token——Adobe 没有 refresh_token 这条退路。
	require.False(t, r.CanRefresh(&Account{
		Platform: PlatformAdobe, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "tok"},
	}))
	require.False(t, r.CanRefresh(&Account{Platform: PlatformKiro, Type: AccountTypeOAuth,
		Credentials: map[string]any{"cookie": "c"}}))
	require.False(t, r.CanRefresh(&Account{Platform: PlatformAdobe, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"cookie": "c"}}))
	require.False(t, r.CanRefresh(nil))
}

func TestAdobeTokenRefresherNeedsRefresh(t *testing.T) {
	r := NewAdobeTokenRefresher()
	account := func(token string) *Account {
		return &Account{
			Platform: PlatformAdobe, Type: AccountTypeOAuth,
			Credentials: map[string]any{"cookie": "aux_sid=1", "access_token": token},
		}
	}

	// token 缺失必须刷新，否则新建的账号永远不会被填上 token。
	require.True(t, r.NeedsRefresh(account(""), time.Minute))
	require.True(t, r.NeedsRefresh(
		account(adobeTestToken(t, map[string]any{"exp": time.Now().Add(-time.Hour).Unix()})), time.Minute))
	require.False(t, r.NeedsRefresh(
		account(adobeTestToken(t, map[string]any{"exp": time.Now().Add(24 * time.Hour).Unix()})), time.Minute))

	// 传入窗口比 Adobe 的默认窗口还小时应取默认值：IMS token 寿命短，
	// 窗口过小会让刷新赶不上过期。
	soon := account(adobeTestToken(t, map[string]any{"exp": time.Now().Add(10 * time.Minute).Unix()}))
	require.True(t, r.NeedsRefresh(soon, time.Second))
}

// Refresh 必须保留 credentials 里的既有字段：cookie 和 model_mapping 都在里面，
// 直接返回新 map 会把账号的配置抹掉。
func TestAdobeTokenRefresherPreservesExistingCredentials(t *testing.T) {
	newToken := adobeTestToken(t, map[string]any{
		"user_id": "u1", "exp": time.Now().Add(24 * time.Hour).Unix(),
	})
	r := newAdobeTestRefresher(t, func(*adobe.Request, int) (*adobe.Response, error) {
		body, _ := json.Marshal(map[string]any{"access_token": newToken, "expires_in": 86400})
		return &adobe.Response{StatusCode: 200, Headers: map[string]string{}, Body: body}, nil
	})

	account := &Account{
		ID: 3, Platform: PlatformAdobe, Type: AccountTypeOAuth,
		Credentials: map[string]any{
			"cookie":        "aux_sid=abc",
			"access_token":  "stale",
			"model_mapping": map[string]any{"gpt-image-2": "firefly-gpt-image-2"},
		},
	}
	updated, err := r.Refresh(context.Background(), account)
	require.NoError(t, err)

	require.Equal(t, newToken, updated["access_token"])
	require.Equal(t, "aux_sid=abc", updated["cookie"])
	require.NotNil(t, updated["model_mapping"])
	require.NotEmpty(t, updated["expires_at"])
}

// cookie 失效只能由用户重新导出，必须给出明确信号而不是当成可重试的临时故障。
func TestAdobeTokenRefresherSurfacesDeadCookie(t *testing.T) {
	r := newAdobeTestRefresher(t, func(*adobe.Request, int) (*adobe.Response, error) {
		return &adobe.Response{StatusCode: 401, Headers: map[string]string{}, Body: []byte("{}")}, nil
	})
	_, err := r.Refresh(context.Background(), &Account{
		ID: 4, Platform: PlatformAdobe, Type: AccountTypeOAuth,
		Credentials: map[string]any{"cookie": "dead"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "re-export")

	var authErr *adobe.AuthError
	require.True(t, errors.As(err, &authErr), "应保留 AuthError 以便上层区分")
}

func TestAdobeTokenRefresherRejectsNonAdobeAccount(t *testing.T) {
	r := NewAdobeTokenRefresher()
	_, err := r.Refresh(context.Background(), &Account{Platform: PlatformKiro, Type: AccountTypeOAuth})
	require.ErrorContains(t, err, "not a refreshable adobe account")
}

func TestAdobeTokenRefresherCacheKey(t *testing.T) {
	r := NewAdobeTokenRefresher()
	require.Equal(t, "adobe:token:42", r.CacheKey(&Account{ID: 42}))
	require.Empty(t, r.CacheKey(nil))
}
