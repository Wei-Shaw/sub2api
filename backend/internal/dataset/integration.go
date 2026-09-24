package dataset

import (
	"context"
	"log"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// CollectorSingleton holds the global dataset collector instance.
var CollectorSingleton *Collector

// WorkerSingleton holds the global dataset worker instance.
var WorkerSingleton *Worker

// InitializeDatasetCollection creates and starts the dataset collector and worker
// if dataset collection is enabled in the config.
// Returns cleanup function that should be called during graceful shutdown.
func InitializeDatasetCollection(ctx context.Context, cfg *config.Config) func() {
	if cfg == nil || !cfg.Dataset.Enabled {
		log.Println("[Dataset] Collection disabled in config")
		return func() {}
	}

	log.Printf("[Dataset] Initializing collection: credentials=%s folder=%s batch_size=%d buffer_max=%d",
		cfg.Dataset.GoogleDriveCredentials,
		cfg.Dataset.GoogleDriveFolderID,
		cfg.Dataset.BatchSize,
		cfg.Dataset.BufferMaxItems,
	)

	// Create Drive client
	driveClient, err := NewDriveClient(
		ctx,
		cfg.Dataset.GoogleDriveCredentials,
		cfg.Dataset.GoogleDriveFolderID,
	)
	if err != nil {
		log.Printf("[Dataset] WARN: Failed to create Drive client: %v (collection disabled)", err)
		return func() {}
	}

	// Create bounded collector
	CollectorSingleton = NewCollector(cfg.Dataset.BufferMaxItems)

	// Create and start background worker
	WorkerSingleton = NewWorker(
		CollectorSingleton,
		driveClient,
		cfg.Dataset.BatchSize,
		cfg.Dataset.BatchMaxMB,
		cfg.Dataset.BatchIntervalSec,
	)
	WorkerSingleton.Start()

	log.Println("[Dataset] Collection infrastructure started successfully")

	// Return cleanup function
	return func() {
		log.Println("[Dataset] Shutting down collection infrastructure")
		if WorkerSingleton != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			WorkerSingleton.Stop(shutdownCtx)
		}
		log.Println("[Dataset] Shutdown complete")
	}
}
