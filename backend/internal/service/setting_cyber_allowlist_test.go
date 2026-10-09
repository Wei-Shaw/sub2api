//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type cyberAllowlistSettingsRepo struct {
	authSourceDefaultsRepoStub
}

func (r *cyberAllowlistSettingsRepo) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func TestUpdateSettingsCyberAllowlistRoundTripAndImmediateRefresh(t *testing.T) {
	ctx := context.Background()
	repo := &cyberAllowlistSettingsRepo{authSourceDefaultsRepoStub{values: map[string]string{}}}
	svc := NewSettingService(repo, &config.Config{})
	resetGatewayForwardingSettingsCacheForTest(t)
	defer svc.refreshCachedSettings(&SystemSettings{})
	require.False(t, svc.IsCyberPolicyUserAllowlisted(ctx, 12))
	settings := &SystemSettings{CyberPolicyUserAllowlist: "12, 34"}
	require.NoError(t, svc.UpdateSettings(ctx, settings))
	require.True(t, svc.IsCyberPolicyUserAllowlisted(ctx, 12))
	require.Equal(t, "12, 34", svc.parseSettings(repo.values).CyberPolicyUserAllowlist)
	settings.CyberPolicyUserAllowlist = "12, invalid"
	require.Error(t, svc.UpdateSettings(ctx, settings))
	require.Equal(t, "12, 34", repo.values[SettingKeyCyberPolicyUserAllowlist])
	settings.CyberPolicyUserAllowlist = ""
	require.NoError(t, svc.UpdateSettings(ctx, settings))
	require.False(t, svc.IsCyberPolicyUserAllowlisted(ctx, 12))
}
