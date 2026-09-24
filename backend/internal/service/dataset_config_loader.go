package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// LoadDatasetConfig loads dataset config from DB settings (for runtime reload).
func (s *SettingService) LoadDatasetConfig(ctx context.Context) (*config.DatasetConfig, error) {
	settings, err := s.settingRepo.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	enabled := datasetBoolSetting(s, settings, SettingKeyDatasetEnabled)
	if !enabled {
		return &config.DatasetConfig{Enabled: false}, nil
	}

	credentialsEncrypted := datasetStringSetting(s, settings, SettingKeyDatasetGoogleDriveCredentials,
		func(c *config.Config) string { return c.Dataset.GoogleDriveCredentials })
	credentials := s.decryptDatasetCredentials(credentialsEncrypted)

	folderID := datasetStringSetting(s, settings, SettingKeyDatasetGoogleDriveFolderID,
		func(c *config.Config) string { return c.Dataset.GoogleDriveFolderID })

	batchSize := datasetIntSetting(s, settings, SettingKeyDatasetBatchSize,
		func(c *config.Config) int { return c.Dataset.BatchSize })

	batchMaxMB := datasetIntSetting(s, settings, SettingKeyDatasetBatchMaxMB,
		func(c *config.Config) int { return c.Dataset.BatchMaxMB })

	batchIntervalSec := datasetIntSetting(s, settings, SettingKeyDatasetBatchIntervalSec,
		func(c *config.Config) int { return c.Dataset.BatchIntervalSec })

	bufferMaxItems := datasetIntSetting(s, settings, SettingKeyDatasetBufferMaxItems,
		func(c *config.Config) int { return c.Dataset.BufferMaxItems })

	return &config.DatasetConfig{
		Enabled:                enabled,
		GoogleDriveCredentials: credentials,
		GoogleDriveFolderID:    folderID,
		BatchSize:              batchSize,
		BatchMaxMB:             batchMaxMB,
		BatchIntervalSec:       batchIntervalSec,
		BufferMaxItems:         bufferMaxItems,
	}, nil
}
