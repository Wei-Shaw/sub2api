package dataset

import (
	"context"
	"log"
	"time"
)

// Worker periodically drains the collector and uploads batches to Google Drive.
type Worker struct {
	collector        *Collector
	driveClient      DriveClientInterface
	batchSize        int
	batchMaxMB       int
	batchIntervalSec int
	stopCh           chan struct{}
	doneCh           chan struct{}
}

// NewWorker creates a new background worker.
func NewWorker(
	collector *Collector,
	driveClient DriveClientInterface,
	batchSize int,
	batchMaxMB int,
	batchIntervalSec int,
) *Worker {
	return &Worker{
		collector:        collector,
		driveClient:      driveClient,
		batchSize:        batchSize,
		batchMaxMB:       batchMaxMB,
		batchIntervalSec: batchIntervalSec,
		stopCh:           make(chan struct{}),
		doneCh:           make(chan struct{}),
	}
}

// Start begins the background batch upload loop.
func (w *Worker) Start() {
	go w.run()
}

// Stop signals the worker to stop and waits for graceful shutdown.
func (w *Worker) Stop(ctx context.Context) {
	close(w.stopCh)
	select {
	case <-w.doneCh:
		log.Println("[Dataset] Worker stopped gracefully")
	case <-ctx.Done():
		log.Println("[Dataset] Worker stop timeout")
	}
}

func (w *Worker) run() {
	defer close(w.doneCh)

	ticker := time.NewTicker(time.Duration(w.batchIntervalSec) * time.Second)
	defer ticker.Stop()

	log.Printf("[Dataset] Worker started: batch_size=%d batch_max_mb=%d interval=%ds",
		w.batchSize, w.batchMaxMB, w.batchIntervalSec)

	for {
		select {
		case <-w.stopCh:
			// Final drain before exit
			w.flushBatch(context.Background())
			return
		case <-ticker.C:
			w.flushBatch(context.Background())
		}
	}
}

func (w *Worker) flushBatch(ctx context.Context) {
	// Peek without removing (allows retry on failure)
	entries := w.collector.Peek(w.batchSize)
	if len(entries) == 0 {
		return
	}

	// Check size limit
	totalSize := 0
	for _, e := range entries {
		totalSize += e.SizeBytes()
	}
	maxBytes := w.batchMaxMB * 1024 * 1024
	if totalSize > maxBytes {
		log.Printf("[Dataset] Batch too large: %d bytes > %d bytes, splitting", totalSize, maxBytes)
		// Simple split: upload first half only
		mid := len(entries) / 2
		entries = entries[:mid]
	}

	// Upload with bounded retry
	var fileID string
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		uploadCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		fileID, err = w.driveClient.UploadBatch(uploadCtx, entries)
		cancel()

		if err == nil {
			break
		}

		if attempt < 3 {
			backoff := time.Duration(attempt*2) * time.Second
			log.Printf("[Dataset] Upload attempt %d/3 failed: %v (retry in %v)", attempt, err, backoff)
			time.Sleep(backoff)
		}
	}

	if err != nil {
		log.Printf("[Dataset] Failed to upload after 3 attempts: %v (entries retained for next tick)", err)
		return
	}

	// Clear only after confirmed success
	w.collector.Clear(len(entries))

	total, dropped, buffered := w.collector.Stats()
	log.Printf("[Dataset] Batch uploaded: file_id=%s entries=%d total=%d dropped=%d buffered=%d",
		fileID, len(entries), total, dropped, buffered)
}
