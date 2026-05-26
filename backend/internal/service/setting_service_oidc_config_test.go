//go:build unit

package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type settingOIDCRepoStub struct {
	values map[string]string
}

func (s *settingOIDCRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *settingOIDCRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *settingOIDCRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *settingOIDCRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *settingOIDCRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *settingOIDCRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *settingOIDCRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestGetOIDCConnectOAuthConfig_ResolvesEndpointsFromIssuerDiscovery(t *testing.T) {
	var discoveryHits int
	var baseURL string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/issuer/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		discoveryHits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{
			"authorization_endpoint":"%s/issuer/protocol/openid-connect/auth",
			"token_endpoint":"%s/issuer/protocol/openid-connect/token",
			"userinfo_endpoint":"%s/issuer/protocol/openid-connect/userinfo",
			"jwks_uri":"%s/issuer/protocol/openid-connect/certs"
		}`, baseURL, baseURL, baseURL, baseURL)))
	}))
	defer srv.Close()
	baseURL = srv.URL

	cfg := &config.Config{
		OIDC: config.OIDCConnectConfig{
			Enabled:             true,
			ProviderName:        "OIDC",
			ClientID:            "oidc-client",
			ClientSecret:        "oidc-secret",
			IssuerURL:           srv.URL + "/issuer",
			RedirectURL:         "https://example.com/api/v1/auth/oauth/oidc/callback",
			FrontendRedirectURL: "/auth/oidc/callback",
			Scopes:              "openid email profile",
			TokenAuthMethod:     "client_secret_post",
			ValidateIDToken:     true,
			AllowedSigningAlgs:  "RS256",
			ClockSkewSeconds:    120,
		},
	}

	repo := &settingOIDCRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, cfg)

	got, err := svc.GetOIDCConnectOAuthConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, discoveryHits)
	require.Equal(t, srv.URL+"/issuer/.well-known/openid-configuration", got.DiscoveryURL)
	require.Equal(t, srv.URL+"/issuer/protocol/openid-connect/auth", got.AuthorizeURL)
	require.Equal(t, srv.URL+"/issuer/protocol/openid-connect/token", got.TokenURL)
	require.Equal(t, srv.URL+"/issuer/protocol/openid-connect/userinfo", got.UserInfoURL)
	require.Equal(t, srv.URL+"/issuer/protocol/openid-connect/certs", got.JWKSURL)
}
