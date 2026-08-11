package handler

import (
	"net/url"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCheckFeishuTenantAllowed(t *testing.T) {
	tests := []struct {
		name      string
		cfg       config.FeishuConnectConfig
		tenantKey string
		allowed   bool
	}{
		{
			name:      "restriction disabled allows any tenant",
			cfg:       config.FeishuConnectConfig{RestrictTenant: false},
			tenantKey: "unlisted-tenant",
			allowed:   true,
		},
		{
			name:    "empty allowlist fails closed",
			cfg:     config.FeishuConnectConfig{RestrictTenant: true},
			allowed: false,
		},
		{
			name:      "configured tenant is allowed",
			cfg:       config.FeishuConnectConfig{RestrictTenant: true, AllowedTenantKeys: "tenant-one, tenant-two\ntenant-three"},
			tenantKey: " tenant-two ",
			allowed:   true,
		},
		{
			name:      "unlisted tenant is rejected",
			cfg:       config.FeishuConnectConfig{RestrictTenant: true, AllowedTenantKeys: "tenant-one"},
			tenantKey: "tenant-two",
			allowed:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.allowed, checkFeishuTenantAllowed(tt.cfg, tt.tenantKey))
		})
	}
}

func TestBuildFeishuAuthorizeURLIncludesOAuthParametersAndState(t *testing.T) {
	cfg := config.FeishuConnectConfig{
		AppID:        "test-app-id",
		AuthorizeURL: "https://accounts.feishu.cn/open-apis/authen/v1/authorize?source=test",
		RedirectURL:  "https://gateway.example.com/api/v1/auth/oauth/feishu/callback",
		Scopes:       "contact:user.email offline_access",
	}

	authorizeURL, err := buildFeishuAuthorizeURL(cfg, "state-for-csrf")
	require.NoError(t, err)

	parsed, err := url.Parse(authorizeURL)
	require.NoError(t, err)
	require.Equal(t, "https", parsed.Scheme)
	require.Equal(t, "accounts.feishu.cn", parsed.Host)
	require.Equal(t, "/open-apis/authen/v1/authorize", parsed.Path)
	query := parsed.Query()
	require.Equal(t, "test", query.Get("source"))
	require.Equal(t, "test-app-id", query.Get("client_id"))
	require.Equal(t, cfg.RedirectURL, query.Get("redirect_uri"))
	require.Equal(t, "code", query.Get("response_type"))
	require.Equal(t, cfg.Scopes, query.Get("scope"))
	require.Equal(t, "state-for-csrf", query.Get("state"))
}
