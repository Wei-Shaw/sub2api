package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	// CandidateBackgroundRuntimeReadinessContract is the JSON sidecar schema
	// written after the complete application graph has registered its workers.
	CandidateBackgroundRuntimeReadinessContract = "workers-ready-v1"
	// CandidateBackgroundRuntimeReadyMarkerPath is container state, not shared
	// application data. Deployment tooling mounts this exact directory.
	CandidateBackgroundRuntimeReadyMarkerPath = "/run/luchikey-candidate-state/workers-ready-v1.json"
	// CandidateBackgroundRuntimeInventoryOCILabel pins the same hash in the
	// image manifest so deploy tooling can compare image, readiness, and log.
	CandidateBackgroundRuntimeInventoryOCILabel = "luchikey.candidate_worker_inventory_sha256"
	// CandidateBackgroundRuntimeInventorySHA256 hashes the newline-terminated,
	// lexically sorted inventory below.
	CandidateBackgroundRuntimeInventorySHA256 = "26753d83d2e9393f01fa5da35ce69c2213c91104fd9b05d9cb99c689401b7ea4"
	CandidateBackgroundRuntimeInventoryCount  = 49
)

var candidateBackgroundRuntimeExpectedWorkers = []string{
	"account_expiry",
	"api_key_auth_cache_invalidation_subscriber",
	"audit_log",
	"auth_cache_invalidation_outbox",
	"backup",
	"batch_image_cleanup",
	"batch_image_worker",
	"billing_cache_write_workers",
	"channel_monitor_runner",
	"channel_monitor_v2_aggregator",
	"cn_provider_balance_check",
	"concurrency_slot_cleanup",
	"concurrency_startup_cleanup",
	"content_moderation",
	"dashboard_aggregation",
	"deferred_service",
	"email_queue",
	"idempotency_cleanup",
	"ollama_cloud_usage",
	"openai_codex_version_sync",
	"openai_quota_auto_reset",
	"openai_quota_settings_warmup",
	"ops_aggregation",
	"ops_alert_evaluator",
	"ops_cleanup",
	"ops_ingress_reject_aggregator",
	"ops_metrics_collector",
	"ops_runtime_settings_refresh",
	"ops_scheduled_report",
	"ops_system_log_sink",
	"payment_order_expiry",
	"plugin_manager",
	"pricing_remote_sync",
	"prompt_audit",
	"proxy_expiry",
	"scheduled_test_runner",
	"scheduler_snapshot",
	"setting_startup_migrations",
	"subscription_expiry",
	"timing_wheel",
	"token_refresh",
	"training_capture_recovery",
	"training_simple_spool_startup_cleanup",
	"training_trace_store",
	"upstream_billing_probe",
	"usage_cleanup",
	"usage_record_worker_pool",
	"user_message_queue_cleanup",
	"user_platform_quota_usage_flusher",
}

var candidateBackgroundRuntimeReadyMarkerPath = CandidateBackgroundRuntimeReadyMarkerPath

type CandidateBackgroundRuntimeReadiness struct {
	Contract        string   `json:"contract"`
	InventorySHA256 string   `json:"inventory_sha256"`
	ExpectedCount   int      `json:"expected_count"`
	ReadyCount      int      `json:"ready_count"`
	ExpectedWorkers []string `json:"expected_workers"`
	ReadyWorkers    []string `json:"ready_workers"`
}

type CandidateBackgroundRuntimePromotionReceipt struct {
	CandidateBackgroundRuntimeReadiness
	StartedCount int `json:"started_count"`
}

// candidateBackgroundRuntime records ordinary service start functions while an
// isolated candidate verifies the shared production data plane. Promotion is
// explicit and occurs only after the traffic switch and old-instance drain.
var candidateBackgroundRuntime = struct {
	sync.Mutex
	promoted   bool
	workers    []candidateBackgroundWorker
	registered map[string]struct{}
	started    map[string]struct{}
	sealed     bool
	dirty      bool
	readiness  CandidateBackgroundRuntimeReadiness
}{
	registered: make(map[string]struct{}),
	started:    make(map[string]struct{}),
}

type candidateBackgroundWorker struct {
	name  string
	start func()
}

// StartBackgroundRuntime starts immediately in normal operation. Candidate
// mode defers the start until PromoteCandidateBackgroundRuntimes. Duplicate
// names are coalesced so every shared consumer starts at most once.
func StartBackgroundRuntime(name string, start func()) {
	if start == nil {
		return
	}
	if !config.IsCandidateRuntimeConfigured() {
		start()
		return
	}

	candidateBackgroundRuntime.Lock()
	if _, exists := candidateBackgroundRuntime.registered[name]; !exists {
		candidateBackgroundRuntime.registered[name] = struct{}{}
		candidateBackgroundRuntime.workers = append(candidateBackgroundRuntime.workers, candidateBackgroundWorker{name: name, start: start})
		if candidateBackgroundRuntime.sealed {
			candidateBackgroundRuntime.dirty = true
		}
	}
	if candidateBackgroundRuntime.promoted || !config.IsCandidateRuntime() {
		shouldStart := markCandidateBackgroundRuntimeStartedLocked(name)
		candidateBackgroundRuntime.Unlock()
		if shouldStart {
			start()
		}
		return
	}
	candidateBackgroundRuntime.Unlock()
}

// SealCandidateBackgroundRuntimeInventory is called after the application
// graph is fully constructed and before signal handlers are installed. It
// compares the exact registered set with the image-pinned inventory and writes
// a durable machine-readable readiness sidecar without starting any worker.
func SealCandidateBackgroundRuntimeInventory() (CandidateBackgroundRuntimeReadiness, error) {
	if !config.IsCandidateRuntimeConfigured() {
		return CandidateBackgroundRuntimeReadiness{}, errors.New("candidate background runtime inventory can only be sealed in candidate mode")
	}

	candidateBackgroundRuntime.Lock()
	defer candidateBackgroundRuntime.Unlock()
	if err := removeCandidateBackgroundRuntimeReadiness(candidateBackgroundRuntimeReadyMarkerPath); err != nil {
		return CandidateBackgroundRuntimeReadiness{}, err
	}
	ready, missing, extra := candidateBackgroundRuntimeInventorySetsLocked()
	if len(missing) > 0 || len(extra) > 0 {
		return CandidateBackgroundRuntimeReadiness{}, fmt.Errorf(
			"candidate background runtime inventory mismatch: missing=%v extra=%v",
			missing,
			extra,
		)
	}
	if err := validateCandidateBackgroundRuntimeInventoryHash(); err != nil {
		return CandidateBackgroundRuntimeReadiness{}, err
	}
	readiness := CandidateBackgroundRuntimeReadiness{
		Contract:        CandidateBackgroundRuntimeReadinessContract,
		InventorySHA256: CandidateBackgroundRuntimeInventorySHA256,
		ExpectedCount:   len(candidateBackgroundRuntimeExpectedWorkers),
		ReadyCount:      len(ready),
		ExpectedWorkers: append([]string(nil), candidateBackgroundRuntimeExpectedWorkers...),
		ReadyWorkers:    append([]string(nil), ready...),
	}
	if err := writeCandidateBackgroundRuntimeReadiness(candidateBackgroundRuntimeReadyMarkerPath, readiness); err != nil {
		return CandidateBackgroundRuntimeReadiness{}, err
	}
	candidateBackgroundRuntime.sealed = true
	candidateBackgroundRuntime.dirty = false
	candidateBackgroundRuntime.readiness = readiness
	return cloneCandidateBackgroundRuntimeReadiness(readiness), nil
}

func removeCandidateBackgroundRuntimeReadiness(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("candidate background runtime readiness path must be absolute: %s", path)
	}
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("invalidate stale candidate background runtime readiness: %w", err)
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open candidate background runtime state directory after invalidation: %w", err)
	}
	defer func() { _ = dir.Close() }()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync candidate background runtime state directory after invalidation: %w", err)
	}
	return nil
}

func candidateBackgroundRuntimeInventorySetsLocked() (ready, missing, extra []string) {
	ready = make([]string, 0, len(candidateBackgroundRuntime.registered))
	for name := range candidateBackgroundRuntime.registered {
		ready = append(ready, name)
	}
	sort.Strings(ready)
	expected := make(map[string]struct{}, len(candidateBackgroundRuntimeExpectedWorkers))
	for _, name := range candidateBackgroundRuntimeExpectedWorkers {
		expected[name] = struct{}{}
		if _, exists := candidateBackgroundRuntime.registered[name]; !exists {
			missing = append(missing, name)
		}
	}
	for _, name := range ready {
		if _, exists := expected[name]; !exists {
			extra = append(extra, name)
		}
	}
	return ready, missing, extra
}

func validateCandidateBackgroundRuntimeInventoryHash() error {
	if len(candidateBackgroundRuntimeExpectedWorkers) != CandidateBackgroundRuntimeInventoryCount {
		return fmt.Errorf(
			"candidate background runtime inventory count mismatch: compiled=%d expected=%d",
			len(candidateBackgroundRuntimeExpectedWorkers),
			CandidateBackgroundRuntimeInventoryCount,
		)
	}
	if !sort.StringsAreSorted(candidateBackgroundRuntimeExpectedWorkers) {
		return errors.New("candidate background runtime inventory must be lexically sorted")
	}
	for i := 1; i < len(candidateBackgroundRuntimeExpectedWorkers); i++ {
		if candidateBackgroundRuntimeExpectedWorkers[i-1] == candidateBackgroundRuntimeExpectedWorkers[i] {
			return fmt.Errorf("candidate background runtime inventory contains duplicate %q", candidateBackgroundRuntimeExpectedWorkers[i])
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(candidateBackgroundRuntimeExpectedWorkers, "\n") + "\n"))
	actual := hex.EncodeToString(sum[:])
	if actual != CandidateBackgroundRuntimeInventorySHA256 {
		return fmt.Errorf("candidate background runtime inventory hash mismatch: compiled=%s expected=%s", actual, CandidateBackgroundRuntimeInventorySHA256)
	}
	return nil
}

func writeCandidateBackgroundRuntimeReadiness(path string, readiness CandidateBackgroundRuntimeReadiness) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("candidate background runtime readiness path must be absolute: %s", path)
	}
	contents, err := json.Marshal(readiness)
	if err != nil {
		return fmt.Errorf("marshal candidate background runtime readiness: %w", err)
	}
	contents = append(contents, '\n')
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create candidate background runtime state directory: %w", err)
	}
	temp, err := os.CreateTemp(parent, ".workers-ready-v1-*")
	if err != nil {
		return fmt.Errorf("create candidate background runtime readiness temp file: %w", err)
	}
	tempPath := temp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("chmod candidate background runtime readiness temp file: %w", err)
	}
	if _, err := temp.Write(contents); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write candidate background runtime readiness temp file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync candidate background runtime readiness temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close candidate background runtime readiness temp file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("publish candidate background runtime readiness: %w", err)
	}
	removeTemp = false
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(parent)
	if err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("open candidate background runtime state directory: %w", err)
	}
	defer func() { _ = dir.Close() }()
	if err := dir.Sync(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("sync candidate background runtime state directory: %w", err)
	}
	return nil
}

func cloneCandidateBackgroundRuntimeReadiness(readiness CandidateBackgroundRuntimeReadiness) CandidateBackgroundRuntimeReadiness {
	readiness.ExpectedWorkers = append([]string(nil), readiness.ExpectedWorkers...)
	readiness.ReadyWorkers = append([]string(nil), readiness.ReadyWorkers...)
	return readiness
}

func markCandidateBackgroundRuntimeStartedLocked(name string) bool {
	if _, exists := candidateBackgroundRuntime.started[name]; exists {
		return false
	}
	candidateBackgroundRuntime.started[name] = struct{}{}
	return true
}

// PromoteCandidateBackgroundRuntimes durably records promotion before it
// starts every deferred runtime exactly once.
func PromoteCandidateBackgroundRuntimes() (int, error) {
	return promoteCandidateBackgroundRuntimes(config.MarkCandidateRuntimePromoted)
}

// PromoteCandidateBackgroundRuntimesWithReadiness is the production promotion
// entrypoint. It requires the sealed exact inventory and returns the same hash
// and complete sets persisted in workers-ready-v1.json for structured logging.
func PromoteCandidateBackgroundRuntimesWithReadiness() (CandidateBackgroundRuntimePromotionReceipt, error) {
	candidateBackgroundRuntime.Lock()
	if !candidateBackgroundRuntime.sealed || candidateBackgroundRuntime.dirty {
		candidateBackgroundRuntime.Unlock()
		return CandidateBackgroundRuntimePromotionReceipt{}, errors.New("candidate background runtime inventory is not sealed and ready")
	}
	candidateBackgroundRuntime.Unlock()

	started, err := PromoteCandidateBackgroundRuntimes()
	if err != nil {
		return CandidateBackgroundRuntimePromotionReceipt{}, err
	}
	candidateBackgroundRuntime.Lock()
	defer candidateBackgroundRuntime.Unlock()
	return CandidateBackgroundRuntimePromotionReceipt{
		CandidateBackgroundRuntimeReadiness: cloneCandidateBackgroundRuntimeReadiness(candidateBackgroundRuntime.readiness),
		StartedCount:                        started,
	}, nil
}

func promoteCandidateBackgroundRuntimes(markPromoted func() error) (int, error) {
	if !config.IsCandidateRuntimeConfigured() {
		return 0, nil
	}

	candidateBackgroundRuntime.Lock()
	if candidateBackgroundRuntime.promoted {
		candidateBackgroundRuntime.Unlock()
		return 0, nil
	}
	if candidateBackgroundRuntime.sealed {
		_, missing, extra := candidateBackgroundRuntimeInventorySetsLocked()
		if candidateBackgroundRuntime.dirty || len(missing) > 0 || len(extra) > 0 {
			candidateBackgroundRuntime.Unlock()
			return 0, fmt.Errorf("candidate background runtime inventory changed after seal: missing=%v extra=%v", missing, extra)
		}
	}
	// Registration remains fenced while the marker is published, so no worker
	// can observe a partially committed promotion.
	if markPromoted == nil {
		candidateBackgroundRuntime.Unlock()
		return 0, errors.New("candidate promotion marker writer is unavailable")
	}
	if err := markPromoted(); err != nil {
		candidateBackgroundRuntime.Unlock()
		return 0, fmt.Errorf("persist candidate promotion: %w", err)
	}
	workers := candidateBackgroundRuntime.workers
	candidateBackgroundRuntime.workers = nil
	candidateBackgroundRuntime.registered = make(map[string]struct{})
	candidateBackgroundRuntime.promoted = true
	starts := make([]func(), 0, len(workers))
	for _, worker := range workers {
		if markCandidateBackgroundRuntimeStartedLocked(worker.name) {
			starts = append(starts, worker.start)
		}
	}
	candidateBackgroundRuntime.Unlock()

	for _, start := range starts {
		start()
	}
	return len(starts), nil
}

func resetCandidateBackgroundRuntimeForTest(t interface{ Cleanup(func()) }) {
	candidateBackgroundRuntime.Lock()
	previousPromoted := candidateBackgroundRuntime.promoted
	previousWorkers := candidateBackgroundRuntime.workers
	previousRegistered := candidateBackgroundRuntime.registered
	previousStarted := candidateBackgroundRuntime.started
	previousSealed := candidateBackgroundRuntime.sealed
	previousDirty := candidateBackgroundRuntime.dirty
	previousReadiness := candidateBackgroundRuntime.readiness
	candidateBackgroundRuntime.promoted = false
	candidateBackgroundRuntime.workers = nil
	candidateBackgroundRuntime.registered = make(map[string]struct{})
	candidateBackgroundRuntime.started = make(map[string]struct{})
	candidateBackgroundRuntime.sealed = false
	candidateBackgroundRuntime.dirty = false
	candidateBackgroundRuntime.readiness = CandidateBackgroundRuntimeReadiness{}
	candidateBackgroundRuntime.Unlock()
	t.Cleanup(func() {
		candidateBackgroundRuntime.Lock()
		candidateBackgroundRuntime.promoted = previousPromoted
		candidateBackgroundRuntime.workers = previousWorkers
		candidateBackgroundRuntime.registered = previousRegistered
		candidateBackgroundRuntime.started = previousStarted
		candidateBackgroundRuntime.sealed = previousSealed
		candidateBackgroundRuntime.dirty = previousDirty
		candidateBackgroundRuntime.readiness = previousReadiness
		candidateBackgroundRuntime.Unlock()
	})
}
