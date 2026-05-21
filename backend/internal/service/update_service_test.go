package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateService_CheckUpdate_UsesManualScriptChannel(t *testing.T) {
	svc := NewUpdateService(nil, nil, "1.2.3", "release")

	info, err := svc.CheckUpdate(context.Background(), false)
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, "1.2.3", info.CurrentVersion)
	require.Equal(t, "1.2.3", info.LatestVersion)
	require.False(t, info.HasUpdate)
	require.Equal(t, UpdateChannelManualScript, info.UpdateChannel)
	require.Equal(t, manualUpgradeCommand, info.UpgradeCommand)
	require.Contains(t, info.UpgradeHint, "deploy")
}

func TestUpdateService_PerformUpdate_DisabledForManualScriptChannel(t *testing.T) {
	svc := NewUpdateService(nil, nil, "1.2.3", "release")

	err := svc.PerformUpdate(context.Background())
	require.Error(t, err)
	require.ErrorIs(t, err, ErrManualUpgradeOnly)
	require.Contains(t, err.Error(), manualUpgradeCommand)
}

func TestUpdateService_Rollback_DisabledForManualScriptChannel(t *testing.T) {
	svc := NewUpdateService(nil, nil, "1.2.3", "release")

	err := svc.Rollback()
	require.Error(t, err)
	require.ErrorIs(t, err, ErrManualRollbackOnly)
	require.Contains(t, err.Error(), "upgrade_main.sh --restore latest")
}
