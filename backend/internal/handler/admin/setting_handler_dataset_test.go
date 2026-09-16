package admin

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type adminDatasetSettingsEncryptor struct{}

func (adminDatasetSettingsEncryptor) Encrypt(plaintext string) (string, error) {
	return "encrypted:" + plaintext, nil
}

func (adminDatasetSettingsEncryptor) Decrypt(ciphertext string) (string, error) {
	return strings.TrimPrefix(ciphertext, "encrypted:"), nil
}

func TestUpdateSettingsDatasetCredentialsAreEncryptedAndNotReturned(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, nil)
	h.settingService.SetSecretEncryptor(adminDatasetSettingsEncryptor{})

	credentials := `{"installed":{"client_id":"client-id","client_secret":"client-secret","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}}`
	rec := doUpdateSettings(t, h, map[string]any{
		"dataset_google_drive_credentials": credentials,
	}, nil)

	require.Equal(t, 200, rec.Code)
	require.Equal(t, "encrypted:"+credentials, repo.lastUpdates[service.SettingKeyDatasetGoogleDriveCredentials])
	require.NotContains(t, rec.Body.String(), "client-secret")
	require.NotContains(t, rec.Body.String(), "dataset_google_drive_credentials\":")
}

func TestUpdateSettingsDatasetCredentialsRemainConfiguredWhenOmitted(t *testing.T) {
	storedCredentials := "encrypted:{\"installed\":{\"client_id\":\"client-id\"}}"
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeyDatasetGoogleDriveCredentials: storedCredentials,
	})

	rec := doUpdateSettings(t, h, map[string]any{"dataset_enabled": true}, nil)

	require.Equal(t, 200, rec.Code)
	require.Equal(t, storedCredentials, repo.values[service.SettingKeyDatasetGoogleDriveCredentials])
	require.Contains(t, rec.Body.String(), `"dataset_google_drive_credentials_configured":true`)
	require.NotContains(t, rec.Body.String(), "client-id")
}
