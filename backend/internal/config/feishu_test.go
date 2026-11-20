//go:build unit

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFeishuEnterpriseConfigurationFailsClosed(t *testing.T) {
	valid := FeishuConnectConfig{Enabled: true, ClientID: "cli_test", ClientSecret: "secret", RedirectURL: "https://portal.example/api/v1/auth/oauth/feishu/callback", AllowedTenantKeys: []string{"tenant-a"}}
	require.NoError(t, ValidateFeishuConfig(valid))
	require.NoError(t, ValidateFeishuConfig(FeishuConnectConfig{}))
	for _, mutate := range []func(*FeishuConnectConfig){
		func(c *FeishuConnectConfig) { c.ClientID = "" },
		func(c *FeishuConnectConfig) { c.ClientSecret = "" },
		func(c *FeishuConnectConfig) { c.AllowedTenantKeys = nil },
		func(c *FeishuConnectConfig) { c.AllowedTenantKeys = []string{" "} },
		func(c *FeishuConnectConfig) { c.RedirectURL = "http://public.example/callback" },
		func(c *FeishuConnectConfig) { c.RedirectURL = "https://user:password@portal.example/callback" },
		func(c *FeishuConnectConfig) { c.RedirectURL = "https://portal.example/callback#token" },
	} {
		candidate := valid
		mutate(&candidate)
		require.Error(t, ValidateFeishuConfig(candidate))
	}
}
