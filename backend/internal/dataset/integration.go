package dataset

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// CollectorSingleton holds the global dataset collector instance.
var CollectorSingleton *Collector

// WorkerSingleton holds the global dataset worker instance.
var WorkerSingleton *Worker

var (
	workerMu      sync.RWMutex
	currentConfig *config.DatasetConfig
	stopWatchCh   chan struct{}
)

// ConfigReloader defines interface for loading dataset config from DB.
type ConfigReloader interface {
	LoadDatasetConfig(ctx context.Context) (*config.DatasetConfig, error)
}

var configReloader ConfigReloader

// SetConfigReloader sets the DB-backed config loader (called from main after Wire init).
func SetConfigReloader(reloader ConfigReloader) {
	configReloader = reloader
}

// InitializeDatasetCollection creates and starts the dataset collector and worker
// if dataset collection is enabled in the config.
// Returns cleanup function that should be called during graceful shutdown.
func InitializeDatasetCollection(ctx context.Context, cfg *config.Config) func() {
	if cfg == nil || !cfg.Dataset.Enabled {
		log.Println("[Dataset] Collection disabled in config")
		return func() {}
	}

	log.Printf("[Dataset] Initializing collection: credential_source=%s credential_configured=%t folder_configured=%t batch_size=%d buffer_max=%d",
		credentialSourceKind(cfg.Dataset.GoogleDriveCredentials),
		strings.TrimSpace(cfg.Dataset.GoogleDriveCredentials) != "",
		strings.TrimSpace(cfg.Dataset.GoogleDriveFolderID) != "",
		cfg.Dataset.BatchSize,
		cfg.Dataset.BufferMaxItems,
	)

	currentConfig = &cfg.Dataset

	// Start initial worker
	if err := startWorker(ctx, cfg); err != nil {
		log.Printf("[Dataset] WARN: Failed to start initial worker: %v (collection disabled)", err)
		return func() {}
	}

	// Start config watcher (reloads from DB every 30s)
	stopWatchCh = make(chan struct{})
	go watchConfigChanges(ctx, cfg)

	log.Println("[Dataset] Collection infrastructure started successfully")

	// Return cleanup function
	return func() {
		log.Println("[Dataset] Shutting down collection infrastructure")
		if stopWatchCh != nil {
			close(stopWatchCh)
		}
		stopWorker(context.Background())
		log.Println("[Dataset] Shutdown complete")
	}
}

func startWorker(ctx context.Context, cfg *config.Config) error {
	workerMu.Lock()
	defer workerMu.Unlock()

	// Stop existing worker
	if WorkerSingleton != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		WorkerSingleton.Stop(shutdownCtx)
		WorkerSingleton = nil
	}

	// Create Drive client with new credentials
	driveClient, err := NewDriveClientWithTokenPath(ctx, cfg.Dataset.GoogleDriveCredentials, cfg.Dataset.GoogleDriveTokenPath, cfg.Dataset.GoogleDriveFolderID)
	if err != nil {
		return err
	}

	// Reuse collector (preserves buffered entries across restart)
	if CollectorSingleton == nil {
		CollectorSingleton = NewCollector(cfg.Dataset.BufferMaxItems)
	}

	// Create new worker
	WorkerSingleton = NewWorker(CollectorSingleton, driveClient,
		cfg.Dataset.BatchSize, cfg.Dataset.BatchMaxMB, cfg.Dataset.BatchIntervalSec)
	WorkerSingleton.Start()

	log.Println("[Dataset] Worker (re)started with new config")
	return nil
}

func stopWorker(ctx context.Context) {
	workerMu.Lock()
	defer workerMu.Unlock()

	if WorkerSingleton != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		WorkerSingleton.Stop(shutdownCtx)
		WorkerSingleton = nil
	}
}

func watchConfigChanges(ctx context.Context, cfg *config.Config) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if configReloader == nil {
				continue
			}

			newCfg, err := configReloader.LoadDatasetConfig(ctx)
			if err != nil {
				log.Printf("[Dataset] Config reload failed: %v", err)
				continue
			}

			preserveDeploymentConfig(&cfg.Dataset, newCfg)
			if newCfg == nil || !newCfg.Enabled {
				// Dataset disabled via Admin Settings
				workerMu.RLock()
				workerRunning := WorkerSingleton != nil
				workerMu.RUnlock()

				if workerRunning {
					log.Println("[Dataset] Config disabled, stopping worker")
					stopWorker(context.Background())
					currentConfig = nil
				}
				continue
			}

			if currentConfig == nil || configChanged(currentConfig, newCfg) {
				log.Println("[Dataset] Config changed, restarting worker")
				cfg.Dataset = *newCfg
				currentConfig = newCfg
				if err := startWorker(context.Background(), cfg); err != nil {
					log.Printf("[Dataset] Failed to restart worker: %v", err)
				}
			}

		case <-stopWatchCh:
			return
		}
	}
}

func configChanged(old, new *config.DatasetConfig) bool {
	return old.GoogleDriveCredentials != new.GoogleDriveCredentials ||
		old.GoogleDriveTokenPath != new.GoogleDriveTokenPath ||
		old.GoogleDriveFolderID != new.GoogleDriveFolderID ||
		old.BatchSize != new.BatchSize ||
		old.BatchMaxMB != new.BatchMaxMB ||
		old.BatchIntervalSec != new.BatchIntervalSec
}
