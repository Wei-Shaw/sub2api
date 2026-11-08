//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// TestSettingService_ModelPlazaProEnabledPublicAndInjected pins the opt-in
// contract of model_plaza_pro_enabled: absent/false → disabled, strict "true" →
// enabled, and the SSR injection payload must mirror the same value (the drift
// test in handler/dto only checks the JSON tag set, not the plumbing).
func TestSettingService_ModelPlazaProEnabledPublicAndInjected(t *testing.T) {
	cases := []struct {
		name  string
		value map[string]string
		want  bool
	}{
		{name: "missing key defaults to false", value: map[string]string{}, want: false},
		{name: "explicit false stays disabled", value: map[string]string{SettingKeyModelPlazaProEnabled: "false"}, want: false},
		{name: "non boolean garbage stays disabled", value: map[string]string{SettingKeyModelPlazaProEnabled: "yes"}, want: false},
		{name: "explicit true enables", value: map[string]string{SettingKeyModelPlazaProEnabled: "true"}, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewSettingService(&settingPublicRepoStub{values: tc.value}, &config.Config{})

			settings, err := svc.GetPublicSettings(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.want, settings.ModelPlazaProEnabled)

			raw, err := svc.GetPublicSettingsForInjection(context.Background())
			require.NoError(t, err)
			payload, ok := raw.(*PublicSettingsInjectionPayload)
			require.True(t, ok)
			require.Equal(t, tc.want, payload.ModelPlazaProEnabled,
				"injection payload must mirror %s", SettingKeyModelPlazaProEnabled)
		})
	}
}

// TestSettingService_ModelPlazaProEnabledDefaultsFalseOnInit guards the
// viper-style default table: a fresh install must persist "false" (opt-in), the
// same way model_plaza_enabled does.
func TestSettingService_ModelPlazaProEnabledDefaultsFalseOnInit(t *testing.T) {
	repo := &forwardedIPMigrationRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	require.NoError(t, svc.InitializeDefaultSettings(context.Background()))
	require.Equal(t, "false", repo.values[SettingKeyModelPlazaProEnabled])
}

// TestSettingService_ModelPlazaProEnabledUpdateAndAdminView round-trips the key
// through UpdateSettings (admin PUT) and GetAllSettings (admin GET).
func TestSettingService_ModelPlazaProEnabledUpdateAndAdminView(t *testing.T) {
	updateRepo := &settingUpdateRepoStub{}
	svc := NewSettingService(updateRepo, &config.Config{})

	require.NoError(t, svc.UpdateSettings(context.Background(), &SystemSettings{ModelPlazaProEnabled: true}))
	require.Equal(t, "true", updateRepo.updates[SettingKeyModelPlazaProEnabled])

	adminRepo := &settingGetAllRepoStub{values: map[string]string{SettingKeyModelPlazaProEnabled: "true"}}
	adminSvc := NewSettingService(adminRepo, &config.Config{})
	settings, err := adminSvc.GetAllSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.ModelPlazaProEnabled)

	// Missing key → false in the admin view too.
	emptySvc := NewSettingService(&settingGetAllRepoStub{values: map[string]string{}}, &config.Config{})
	settings, err = emptySvc.GetAllSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.ModelPlazaProEnabled)
}
