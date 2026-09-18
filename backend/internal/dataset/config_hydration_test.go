//go:build unit

package dataset

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type datasetConfigReloaderStub struct {
	cfg *config.DatasetConfig
	err error
}

func (s datasetConfigReloaderStub) LoadDatasetConfig(context.Context) (*config.DatasetConfig, error) {
	return s.cfg, s.err
}

func TestApplyPersistedConfigUsesDatabaseValues(t *testing.T) {
	cfg := &config.Config{Dataset: config.DatasetConfig{
		Enabled:                false,
		GoogleDriveCredentials: "/run/secrets/google-drive.json",
		GoogleDriveTokenPath:   "/run/secrets/google-drive.token",
		GoogleDriveFolderID:    "env-folder",
		BatchSize:              50,
		BatchMaxMB:             5,
		BatchIntervalSec:       60,
		BufferMaxItems:         100,
	}}
	persisted := &config.DatasetConfig{
		Enabled:                true,
		GoogleDriveCredentials: `{"installed":{"client_id":"client-id"}}`,
		GoogleDriveFolderID:    "db-folder",
		BatchSize:              100,
		BatchMaxMB:             10,
		BatchIntervalSec:       300,
		BufferMaxItems:         1000,
	}

	err := ApplyPersistedConfig(context.Background(), cfg, datasetConfigReloaderStub{cfg: persisted})

	require.NoError(t, err)
	require.Equal(t, *persisted, cfg.Dataset)
	require.Equal(t, "/run/secrets/google-drive.token", cfg.Dataset.GoogleDriveTokenPath)
}

func TestApplyPersistedConfigKeepsBootstrapValuesWhenDatabaseLoadFails(t *testing.T) {
	original := config.DatasetConfig{
		Enabled:                true,
		GoogleDriveCredentials: "/run/secrets/google-drive.json",
		GoogleDriveTokenPath:   "/run/secrets/google-drive.token",
		GoogleDriveFolderID:    "env-folder",
		BatchSize:              50,
		BatchMaxMB:             5,
		BatchIntervalSec:       60,
		BufferMaxItems:         100,
	}
	cfg := &config.Config{Dataset: original}

	err := ApplyPersistedConfig(context.Background(), cfg, datasetConfigReloaderStub{err: errors.New("database unavailable")})

	require.Error(t, err)
	require.Equal(t, original, cfg.Dataset)
}

func TestPreserveDeploymentConfigKeepsExplicitPersistedTokenPath(t *testing.T) {
	base := &config.DatasetConfig{GoogleDriveTokenPath: "/run/secrets/bootstrap.token"}
	persisted := &config.DatasetConfig{GoogleDriveTokenPath: "/run/secrets/db-specific.token"}

	preserveDeploymentConfig(base, persisted)

	require.Equal(t, "/run/secrets/db-specific.token", persisted.GoogleDriveTokenPath)
}
