//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestParseSettingsFeishuRegistrationBypassFailsClosed(t *testing.T) {
	cases := []struct {
		name     string
		settings map[string]string
		want     bool
	}{
		{
			name: "safe combination",
			settings: map[string]string{
				SettingKeyFeishuConnectEnabled:            "true",
				SettingKeyFeishuConnectBypassRegistration: "true",
				SettingKeyFeishuConnectRestrictTenant:     "true",
				SettingKeyFeishuConnectAllowedTenantKeys:  "tenant-one",
			},
			want: true,
		},
		{
			name: "restriction disabled",
			settings: map[string]string{
				SettingKeyFeishuConnectEnabled:            "true",
				SettingKeyFeishuConnectBypassRegistration: "true",
				SettingKeyFeishuConnectRestrictTenant:     "false",
				SettingKeyFeishuConnectAllowedTenantKeys:  "tenant-one",
			},
		},
		{
			name: "allowlist empty",
			settings: map[string]string{
				SettingKeyFeishuConnectEnabled:            "true",
				SettingKeyFeishuConnectBypassRegistration: "true",
				SettingKeyFeishuConnectRestrictTenant:     "true",
				SettingKeyFeishuConnectAllowedTenantKeys:  " ",
			},
		},
	}

	svc := NewSettingService(&settingRepoStub{values: map[string]string{}}, &config.Config{})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.parseSettings(tc.settings)
			require.Equal(t, tc.want, got.FeishuConnectBypassRegistration)
		})
	}
}

func TestParseSettingsFeishuRegistrationBypassConfigFallback(t *testing.T) {
	svc := NewSettingService(&settingRepoStub{values: map[string]string{}}, &config.Config{
		Feishu: config.FeishuConnectConfig{
			Enabled:            true,
			RestrictTenant:     true,
			AllowedTenantKeys:  "tenant-one",
			BypassRegistration: true,
		},
	})

	require.True(t, svc.parseSettings(map[string]string{}).FeishuConnectBypassRegistration)
	require.False(t, svc.parseSettings(map[string]string{
		SettingKeyFeishuConnectRestrictTenant: "false",
	}).FeishuConnectBypassRegistration)
}
