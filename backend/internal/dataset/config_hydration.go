package dataset

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// ApplyPersistedConfig overlays DB-backed dataset settings onto the bootstrap
// configuration before the collector is initialized. If loading fails, the
// caller can keep using the unchanged bootstrap configuration.
//
// The OAuth token path is deployment configuration rather than an admin
// setting, so an empty persisted value preserves the bootstrap value.
func ApplyPersistedConfig(ctx context.Context, cfg *config.Config, reloader ConfigReloader) error {
	if cfg == nil || reloader == nil {
		return nil
	}

	persisted, err := reloader.LoadDatasetConfig(ctx)
	if err != nil {
		return fmt.Errorf("load persisted dataset config: %w", err)
	}
	if persisted == nil {
		return nil
	}

	preserveDeploymentConfig(&cfg.Dataset, persisted)
	cfg.Dataset = *persisted
	return nil
}

func preserveDeploymentConfig(base, persisted *config.DatasetConfig) {
	if base == nil || persisted == nil {
		return
	}
	if strings.TrimSpace(persisted.GoogleDriveTokenPath) == "" {
		persisted.GoogleDriveTokenPath = strings.TrimSpace(base.GoogleDriveTokenPath)
	}
}
