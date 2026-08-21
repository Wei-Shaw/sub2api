package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// TestGetCodexSessionSwitchPurificationEnabled covers the runtime getter:
// nil service → false, missing key → default off (fail-closed), "true" → true.
func TestGetCodexSessionSwitchPurificationEnabled(t *testing.T) {
	var nilSvc *SettingService
	require.False(t, nilSvc.GetCodexSessionSwitchPurificationEnabled(context.Background()))

	svc := &SettingService{settingRepo: &fakeSettingRepo{vals: map[string]string{}}}
	require.False(t, svc.GetCodexSessionSwitchPurificationEnabled(context.Background()), "missing key → default off")

	svc = &SettingService{settingRepo: &fakeSettingRepo{vals: map[string]string{
		SettingKeyCodexSessionSwitchPurificationEnabled: "true",
	}}}
	require.True(t, svc.GetCodexSessionSwitchPurificationEnabled(context.Background()))

	svc = &SettingService{settingRepo: &fakeSettingRepo{vals: map[string]string{
		SettingKeyCodexSessionSwitchPurificationEnabled: "false",
	}}}
	require.False(t, svc.GetCodexSessionSwitchPurificationEnabled(context.Background()))
}

// TestParseSettings_CodexSessionSwitchPurificationDefault verifies parseSettings
// yields false by default and mirrors the stored value when set.
func TestParseSettings_CodexSessionSwitchPurificationDefault(t *testing.T) {
	s := &SettingService{cfg: &config.Config{}}
	require.False(t, s.parseSettings(map[string]string{}).CodexSessionSwitchPurificationEnabled)
	require.True(t, s.parseSettings(map[string]string{
		SettingKeyCodexSessionSwitchPurificationEnabled: "true",
	}).CodexSessionSwitchPurificationEnabled)
}

// TestBuildSystemSettingsUpdates_CodexSessionSwitchPurification verifies the
// update path persists the toggle value.
func TestBuildSystemSettingsUpdates_CodexSessionSwitchPurification(t *testing.T) {
	s := &SettingService{cfg: &config.Config{}}
	updates, err := s.buildSystemSettingsUpdates(context.Background(), &SystemSettings{CodexSessionSwitchPurificationEnabled: true})
	require.NoError(t, err)
	require.Equal(t, "true", updates[SettingKeyCodexSessionSwitchPurificationEnabled])
}

// TestRefreshCachedSettings_CodexSessionSwitchPurification verifies a settings
// write immediately refreshes the runtime cache (no 60s staleness after save).
func TestRefreshCachedSettings_CodexSessionSwitchPurification(t *testing.T) {
	s := &SettingService{}
	s.refreshCachedSettings(&SystemSettings{CodexSessionSwitchPurificationEnabled: true})
	require.True(t, s.GetCodexSessionSwitchPurificationEnabled(context.Background()))
}
