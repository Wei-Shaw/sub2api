//go:build unit

package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

type datasetSettingsEncryptor struct{}

func (datasetSettingsEncryptor) Encrypt(plaintext string) (string, error) {
	return "encrypted:" + plaintext, nil
}

func (datasetSettingsEncryptor) Decrypt(ciphertext string) (string, error) {
	return strings.TrimPrefix(ciphertext, "encrypted:"), nil
}

func TestSettingServiceDatasetSettingsUseConfigFallbackAndMaskCredentials(t *testing.T) {
	originalMappingOptions := xai.RuntimeModelMappingOptions()
	t.Cleanup(func() { xai.SetRuntimeModelMappingOptions(originalMappingOptions) })

	cfg := &config.Config{Dataset: config.DatasetConfig{
		Enabled:                true,
		GoogleDriveCredentials: "/run/secrets/google-drive.json",
		GoogleDriveFolderID:    "config-folder",
		BatchSize:              100,
		BatchMaxMB:             10,
		BatchIntervalSec:       300,
		BufferMaxItems:         1000,
	}}
	svc := NewSettingService(&settingGetAllRepoStub{values: map[string]string{}}, cfg)

	settings := svc.parseSettings(map[string]string{})

	require.True(t, settings.DatasetEnabled)
	require.Equal(t, "config-folder", settings.DatasetGoogleDriveFolderID)
	require.True(t, settings.DatasetGoogleDriveCredentialsConfigured)
	require.Empty(t, settings.DatasetGoogleDriveCredentials)
	require.Equal(t, 100, settings.DatasetBatchSize)
}

func TestSettingServiceDatasetCredentialsAreEncryptedOnWrite(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})
	svc.SetSecretEncryptor(datasetSettingsEncryptor{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		DatasetGoogleDriveCredentials: `{"installed":{"client_id":"client-id","client_secret":"secret","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}}`,
	})

	require.NoError(t, err)
	require.Equal(t, "encrypted:{\"installed\":{\"client_id\":\"client-id\",\"client_secret\":\"secret\",\"auth_uri\":\"https://accounts.google.com/o/oauth2/auth\",\"token_uri\":\"https://oauth2.googleapis.com/token\"}}", repo.updates[SettingKeyDatasetGoogleDriveCredentials])
}

func TestSettingServiceDatasetCredentialsRequireEncryption(t *testing.T) {
	svc := NewSettingService(&settingUpdateRepoStub{}, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		DatasetGoogleDriveCredentials: `{"installed":{"client_id":"client-id","client_secret":"secret","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}}`,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "dataset credentials encryption is unavailable")
}

func TestSettingServiceEmptyDatasetCredentialsDoNotOverwriteStoredValue(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})
	svc.SetSecretEncryptor(datasetSettingsEncryptor{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		DatasetGoogleDriveCredentialsSet: true,
	})

	require.NoError(t, err)
	_, exists := repo.updates[SettingKeyDatasetGoogleDriveCredentials]
	require.False(t, exists)
}

func TestValidateDatasetGoogleDriveCredentials(t *testing.T) {
	valid := `{"installed":{"client_id":"client-id","client_secret":"secret","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}}`

	require.NoError(t, ValidateDatasetGoogleDriveCredentials(valid))
	require.NoError(t, ValidateDatasetGoogleDriveCredentials("  "))
	require.Error(t, ValidateDatasetGoogleDriveCredentials("not-json"))
	require.Error(t, ValidateDatasetGoogleDriveCredentials(`{"installed":{}}`))
	require.Error(t, ValidateDatasetGoogleDriveCredentials(strings.Repeat("x", DatasetGoogleDriveCredentialsMaxBytes+1)))
}

func TestValidateDatasetSettingsValues(t *testing.T) {
	tests := []struct {
		name   string
		values [4]int
		valid  bool
	}{
		{name: "defaults", values: [4]int{100, 10, 300, 1000}, valid: true},
		{name: "zero batch size", values: [4]int{0, 10, 300, 1000}},
		{name: "oversized batch size", values: [4]int{1001, 10, 300, 1000}},
		{name: "zero max mb", values: [4]int{100, 0, 300, 1000}},
		{name: "zero interval", values: [4]int{100, 10, 0, 1000}},
		{name: "zero buffer", values: [4]int{100, 10, 300, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDatasetSettingsValues(tt.values[0], tt.values[1], tt.values[2], tt.values[3])
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
