//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSettingService_ParseSettingsMasksTencentCaptchaCredentials(t *testing.T) {
	svc := NewSettingService(&settingGetAllRepoStub{values: map[string]string{
		SettingKeyTencentCaptchaEnabled:        "true",
		SettingKeyTencentCaptchaAppID:          "123456789",
		SettingKeyTencentCaptchaAppSecretKey:   "app-secret",
		SettingKeyTencentCaptchaCloudSecretID:  "cloud-secret-id",
		SettingKeyTencentCaptchaCloudSecretKey: "cloud-secret-key",
REDACTEDREDACTED, &config.Config{REDACTED)

	settings, err := svc.GetAllSettings(context.Background())

REDACTED
	require.True(t, settings.TencentCaptchaEnabled)
	require.Equal(t, "123456789", settings.TencentCaptchaAppID)
	require.True(t, settings.TencentCaptchaAppSecretKeyConfigured)
	require.True(t, settings.TencentCaptchaCloudSecretIDConfigured)
	require.True(t, settings.TencentCaptchaCloudSecretKeyConfigured)
	require.Equal(t, "app-secret", settings.TencentCaptchaAppSecretKey)
	require.Equal(t, "cloud-secret-id", settings.TencentCaptchaCloudSecretID)
	require.Equal(t, "cloud-secret-key", settings.TencentCaptchaCloudSecretKey)
REDACTED

func TestSettingService_GetPublicSettingsExposesOnlyTencentCaptchaAppID(t *testing.T) {
	svc := NewSettingService(&settingPublicRepoStub{values: map[string]string{
		SettingKeyTencentCaptchaEnabled:        "true",
		SettingKeyTencentCaptchaAppID:          "123456789",
		SettingKeyTencentCaptchaAppSecretKey:   "app-secret",
		SettingKeyTencentCaptchaCloudSecretID:  "cloud-secret-id",
		SettingKeyTencentCaptchaCloudSecretKey: "cloud-secret-key",
REDACTEDREDACTED, &config.Config{REDACTED)

	settings, err := svc.GetPublicSettings(context.Background())
REDACTED
	require.True(t, settings.TencentCaptchaEnabled)
	require.Equal(t, "123456789", settings.TencentCaptchaAppID)

	raw, err := json.Marshal(settings)
REDACTED
	require.NotContains(t, string(raw), "app-secret")
	require.NotContains(t, string(raw), "cloud-secret-id")
	require.NotContains(t, string(raw), "cloud-secret-key")
REDACTED

func TestSettingService_GetTencentCaptchaConfig(t *testing.T) {
	repo := &settingPublicRepoStub{values: map[string]string{
		SettingKeyTencentCaptchaEnabled:        "true",
		SettingKeyTencentCaptchaAppID:          "123456789",
		SettingKeyTencentCaptchaAppSecretKey:   "app-secret",
		SettingKeyTencentCaptchaCloudSecretID:  "cloud-secret-id",
		SettingKeyTencentCaptchaCloudSecretKey: "cloud-secret-key",
REDACTEDREDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)

	got := svc.GetTencentCaptchaConfig(context.Background())

	require.Equal(t, TencentCaptchaConfig{
		Enabled:        true,
		AppID:          "123456789",
		AppSecretKey:   "app-secret",
		CloudSecretID:  "cloud-secret-id",
		CloudSecretKey: "cloud-secret-key",
REDACTED, got)
REDACTED
