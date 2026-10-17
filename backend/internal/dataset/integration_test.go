//go:build unit

package dataset

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestConfigChangedIncludesGoogleDriveTokenStatePath(t *testing.T) {
	old := &config.DatasetConfig{
		GoogleDriveCredentials:    "credentials.json",
		GoogleDriveTokenPath:      "seed.token",
		GoogleDriveTokenStatePath: "state-a.token",
		GoogleDriveFolderID:       "folder",
		BatchSize:                 10,
		BatchMaxMB:                5,
		BatchIntervalSec:          60,
	}
	updated := *old
	updated.GoogleDriveTokenStatePath = "state-b.token"

	require.True(t, configChanged(old, &updated))
}
