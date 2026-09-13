package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/keyprotection"
	"github.com/stretchr/testify/require"
)

func TestKeyProtectionSettingsUnsetDisabled(t *testing.T) {
	svc := newPanelRateLimitTestService(&panelRateLimitSettingRepo{})
	cfg, err := svc.GetKeyProtectionConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, keyprotection.DefaultConfig(), cfg)
	require.False(t, cfg.Enabled)
}

func TestKeyProtectionSettingsFailureDoesNotDisableProtection(t *testing.T) {
	for _, value := range []string{"", "null", "[]", "{broken", `{"enabled":true,"rules":["unknown"]}`, `{"enabled":true,"user_ids":[-1]}`} {
		t.Run(value, func(t *testing.T) {
			svc := newPanelRateLimitTestService(&panelRateLimitSettingRepo{values: map[string]string{SettingKeyKeyProtection: value}})
			_, err := svc.GetKeyProtectionConfig(context.Background())
			require.Error(t, err)
		})
	}
	repo := &panelRateLimitSettingRepo{}
	svc := newPanelRateLimitTestService(repo)
	cfg := keyprotection.DefaultConfig()
	cfg.Enabled = true
	require.NoError(t, svc.SetKeyProtectionConfig(context.Background(), cfg))
	repo.getValueErr = errors.New("sensitive storage details")
	_, err := svc.GetKeyProtectionConfig(context.Background())
	require.Error(t, err)
	require.NotContains(t, err.Error(), "sensitive storage details")
	var unavailable *SettingService
	_, err = unavailable.GetKeyProtectionConfig(context.Background())
	require.Error(t, err)
}

func TestKeyProtectionSettingsRoundTripAndIsolation(t *testing.T) {
	repo := &panelRateLimitSettingRepo{}
	svc := newPanelRateLimitTestService(repo)
	cfg := keyprotection.DefaultConfig()
	cfg.Enabled = true
	cfg.UserIDs = []int64{4, 5}
	cfg.GroupIDs = []int64{7}
	cfg.CustomRules = []keyprotection.Rule{{Name: "fictional", Pattern: `fictional_[a-z]{20}`}}
	require.NoError(t, svc.SetKeyProtectionConfig(context.Background(), cfg))
	read, err := svc.GetKeyProtectionConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, cfg, read)
	read.UserIDs[0] = 999
	read.CustomRules[0].Pattern = "changed"
	read, err = svc.GetKeyProtectionConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, cfg, read, "request policy snapshots must not share mutable slices")
	// A second instance observes a saved policy without a stale disabled cache.
	other := newPanelRateLimitTestService(repo)
	read, err = other.GetKeyProtectionConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, cfg, read)
	invalid := cfg
	invalid.Rules = []string{"typo"}
	require.Error(t, svc.SetKeyProtectionConfig(context.Background(), invalid))
	read, err = svc.GetKeyProtectionConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, cfg, read, "invalid updates must preserve the existing policy")
}

type keyProtectionFailingWriteRepo struct{ panelRateLimitSettingRepo }

func (*keyProtectionFailingWriteRepo) Set(context.Context, string, string) error {
	return errors.New("sensitive storage details")
}

func TestKeyProtectionSettingsWriteFailure(t *testing.T) {
	svc := newPanelRateLimitTestService(&keyProtectionFailingWriteRepo{})
	err := svc.SetKeyProtectionConfig(context.Background(), keyprotection.DefaultConfig())
	require.Error(t, err)
	require.NotContains(t, err.Error(), "sensitive storage details")
}
