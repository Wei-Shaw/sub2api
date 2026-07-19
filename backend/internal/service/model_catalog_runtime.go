package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	modelCatalogRefreshRetryAttempts = 3
	modelCatalogRefreshRetryBase     = time.Second
)

// ModelCatalogProjectionRuntime runs the additive projection when shadow mode
// is enabled. Explicit DB read modes bootstrap the durable active snapshot even
// when legacy importing remains disabled.
type ModelCatalogProjectionRuntime struct {
	projection      *ModelCatalogProjectionService
	reader          *AtomicModelCatalogReader
	mode            string
	readModes       ModelCatalogReadModes
	refreshInterval time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// ModelCatalogReadModes keeps the four rollout controls independent. Admission
// observation can bootstrap an already-published snapshot without enabling the
// legacy importer.
type ModelCatalogReadModes struct {
	ImportMode      string
	ListReadMode    string
	PricingReadMode string
	AdmissionMode   string
}

func NewModelCatalogProjectionRuntime(projection *ModelCatalogProjectionService, reader *AtomicModelCatalogReader, mode string, refreshInterval time.Duration) *ModelCatalogProjectionRuntime {
	defaultImportMode := "off"
	if normalizeCatalogMode(mode) == "shadow" {
		defaultImportMode = "shadow"
	}
	return NewModelCatalogProjectionRuntimeWithModes(projection, reader, mode, refreshInterval, ModelCatalogReadModes{
		ImportMode:      defaultImportMode,
		ListReadMode:    "legacy",
		PricingReadMode: "legacy",
		AdmissionMode:   "off",
	})
}

func NewModelCatalogProjectionRuntimeWithModes(projection *ModelCatalogProjectionService, reader *AtomicModelCatalogReader, mode string, refreshInterval time.Duration, modes ModelCatalogReadModes) *ModelCatalogProjectionRuntime {
	return &ModelCatalogProjectionRuntime{
		projection:      projection,
		reader:          reader,
		mode:            normalizeCatalogMode(mode),
		readModes:       normalizeCatalogReadModes(modes),
		refreshInterval: refreshInterval,
	}
}

func (r *ModelCatalogProjectionRuntime) Reader() ModelCatalogReader {
	if r == nil {
		return nil
	}
	return r.reader
}

func (r *ModelCatalogProjectionRuntime) Mode() string {
	if r == nil {
		return "legacy"
	}
	return r.mode
}

func (r *ModelCatalogProjectionRuntime) ReadModes() ModelCatalogReadModes {
	if r == nil {
		return ModelCatalogReadModes{ImportMode: "off", ListReadMode: "legacy", PricingReadMode: "legacy", AdmissionMode: "off"}
	}
	return r.readModes
}

// SyncFromSource performs an explicit admin-triggered legacy pricing import.
// It is a control-plane operation; request-path services still use the
// immutable reader snapshot after publication.
func (r *ModelCatalogProjectionRuntime) SyncFromSource(ctx context.Context) error {
	if r == nil || r.projection == nil {
		return ErrCatalogUnavailable
	}
	return r.projection.Refresh(ctx)
}

// ReloadPublished reloads the durable active publication into this process.
// The publication/outbox transaction remains the cross-instance authority.
func (r *ModelCatalogProjectionRuntime) ReloadPublished(ctx context.Context) error {
	if r == nil || r.projection == nil {
		return ErrCatalogUnavailable
	}
	return r.projection.Bootstrap(ctx)
}

func (r *ModelCatalogProjectionRuntime) needsPublishedSnapshot() bool {
	if r == nil {
		return false
	}
	return r.readModes.ListReadMode == "db" ||
		r.readModes.PricingReadMode == "db" ||
		r.readModes.AdmissionMode != "off"
}

func (r *ModelCatalogProjectionRuntime) Start() {
	if r == nil || r.projection == nil || r.mode != "shadow" {
		return
	}
	if r.readModes.ImportMode == "off" {
		if !r.needsPublishedSnapshot() {
			return
		}
		// DB-backed request reads and admission observe/enforce consume the
		// durable active projection but must not stage or publish legacy pricing
		// while import_mode is off.
		if err := r.projection.Bootstrap(context.Background()); err != nil {
			logger.LegacyPrintf("service.model_catalog", "active snapshot bootstrap failed: %v", err)
		}
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.done = make(chan struct{})
	interval := r.refreshInterval
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	go func() {
		defer close(r.done)
		if err := r.refreshWithRetry(ctx); err != nil && ctx.Err() == nil {
			logger.LegacyPrintf("service.model_catalog", "shadow refresh failed: %v", err)
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := r.refreshWithRetry(ctx); err != nil && ctx.Err() == nil {
					logger.LegacyPrintf("service.model_catalog", "shadow refresh failed: %v", err)
				}
			}
		}
	}()
}

func (r *ModelCatalogProjectionRuntime) refreshWithRetry(ctx context.Context) error {
	var lastErr error
	for attempt := 0; attempt < modelCatalogRefreshRetryAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := r.projection.Refresh(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt == modelCatalogRefreshRetryAttempts-1 {
			break
		}
		timer := time.NewTimer(modelCatalogRefreshRetryBase << attempt)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func (r *ModelCatalogProjectionRuntime) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	cancel := r.cancel
	done := r.done
	r.cancel = nil
	r.done = nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func normalizeCatalogMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "shadow":
		return "shadow"
	default:
		return "legacy"
	}
}

func normalizeModelCatalogReadMode(value, fallback string, allowed ...string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return fallback
}

func normalizeCatalogReadModes(modes ModelCatalogReadModes) ModelCatalogReadModes {
	return ModelCatalogReadModes{
		ImportMode:      normalizeModelCatalogReadMode(modes.ImportMode, "off", "off", "shadow", "publish"),
		ListReadMode:    normalizeModelCatalogReadMode(modes.ListReadMode, "legacy", "legacy", "shadow", "db"),
		PricingReadMode: normalizeModelCatalogReadMode(modes.PricingReadMode, "legacy", "legacy", "shadow", "db"),
		AdmissionMode:   normalizeModelCatalogReadMode(modes.AdmissionMode, "off", "off", "observe", "enforce"),
	}
}
