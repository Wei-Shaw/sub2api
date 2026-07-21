package service

import (
	"sync/atomic"
	"time"
)

// SchedulerOptimizationMetricsSnapshot is a process-local diagnostic view of
// the lossless scheduler optimizations. It is intentionally cumulative; a
// caller can take two snapshots and use their difference as a test window.
type SchedulerOptimizationMetricsSnapshot struct {
	Snapshot    SchedulerSnapshotMetricsSnapshot    `json:"snapshot"`
	Load        SchedulerLoadMetricsSnapshot        `json:"load"`
	Database    SchedulerDBBatchMetricsSnapshot     `json:"database"`
	Incremental SchedulerIncrementalMetricsSnapshot `json:"incremental"`
	Rebuild     SchedulerRebuildMetricsSnapshot     `json:"rebuild"`
	Drift       SchedulerDriftMetricsSnapshot       `json:"drift"`
}

type SchedulerSnapshotMetricsSnapshot struct {
	ReadTotal                 uint64  `json:"read_total"`
	LocalHitTotal             uint64  `json:"local_hit_total"`
	LocalMissTotal            uint64  `json:"local_miss_total"`
	LocalInvalidationTotal    uint64  `json:"local_invalidation_total"`
	RedisHitTotal             uint64  `json:"redis_hit_total"`
	EmptyMissTotal            uint64  `json:"empty_miss_total"`
	ErrorTotal                uint64  `json:"error_total"`
	AccountTotal              uint64  `json:"account_total"`
	DurationTotalMs           float64 `json:"duration_total_ms"`
	DBFallbackTotal           uint64  `json:"db_fallback_total"`
	DBFallbackErrorTotal      uint64  `json:"db_fallback_error_total"`
	DBFallbackLimitedTotal    uint64  `json:"db_fallback_limited_total"`
	DBFallbackDisabledTotal   uint64  `json:"db_fallback_disabled_total"`
	DBFallbackAccountTotal    uint64  `json:"db_fallback_account_total"`
	DBFallbackDurationTotalMs float64 `json:"db_fallback_duration_total_ms"`
}

type SchedulerLoadMetricsSnapshot struct {
	RequestTotal           uint64  `json:"request_total"`
	CacheHitTotal          uint64  `json:"cache_hit_total"`
	CacheMissTotal         uint64  `json:"cache_miss_total"`
	CacheBypassTotal       uint64  `json:"cache_bypass_total"`
	BackendFetchTotal      uint64  `json:"backend_fetch_total"`
	BackendFetchErrorTotal uint64  `json:"backend_fetch_error_total"`
	BackendAccountTotal    uint64  `json:"backend_account_total"`
	BackendDurationTotalMs float64 `json:"backend_duration_total_ms"`
	ScriptTotal            uint64  `json:"script_total"`
	PipelineTotal          uint64  `json:"pipeline_total"`
	PipelineFallbackTotal  uint64  `json:"pipeline_fallback_total"`
	RedisErrorTotal        uint64  `json:"redis_error_total"`
	RedisAccountTotal      uint64  `json:"redis_account_total"`
	RedisBatchTotal        uint64  `json:"redis_batch_total"`
	RedisDurationTotalMs   float64 `json:"redis_duration_total_ms"`
}

type SchedulerDBBatchMetricsSnapshot struct {
	BatchTotal           uint64  `json:"batch_total"`
	QueryTotal           uint64  `json:"query_total"`
	CandidateIDTotal     uint64  `json:"candidate_id_total"`
	ParentIDTotal        uint64  `json:"parent_id_total"`
	ReturnedAccountTotal uint64  `json:"returned_account_total"`
	MissingAccountTotal  uint64  `json:"missing_account_total"`
	ErrorTotal           uint64  `json:"error_total"`
	DurationTotalMs      float64 `json:"duration_total_ms"`
}

type SchedulerIncrementalMetricsSnapshot struct {
	EventAttemptTotal    uint64  `json:"event_attempt_total"`
	EventCommittedTotal  uint64  `json:"event_committed_total"`
	EventReplayTotal     uint64  `json:"event_replay_total"`
	EventFailureTotal    uint64  `json:"event_failure_total"`
	BucketAttemptTotal   uint64  `json:"bucket_attempt_total"`
	BucketRemovedTotal   uint64  `json:"bucket_removed_total"`
	BucketNoopTotal      uint64  `json:"bucket_noop_total"`
	BucketErrorTotal     uint64  `json:"bucket_error_total"`
	FallbackRebuildTotal uint64  `json:"fallback_rebuild_total"`
	DurationTotalMs      float64 `json:"duration_total_ms"`
}

type SchedulerRebuildMetricsSnapshot struct {
	BucketAttemptTotal    uint64  `json:"bucket_attempt_total"`
	BucketSuccessTotal    uint64  `json:"bucket_success_total"`
	BucketErrorTotal      uint64  `json:"bucket_error_total"`
	BucketFencedTotal     uint64  `json:"bucket_fenced_total"`
	BucketBusyTotal       uint64  `json:"bucket_busy_total"`
	AccountTotal          uint64  `json:"account_total"`
	DurationTotalMs       float64 `json:"duration_total_ms"`
	FullRebuildTotal      uint64  `json:"full_rebuild_total"`
	FullRebuildErrorTotal uint64  `json:"full_rebuild_error_total"`
	FullRebuildDurationMs float64 `json:"full_rebuild_duration_total_ms"`
}

type SchedulerDriftMetricsSnapshot struct {
	LagWarningTotal    uint64 `json:"lag_warning_total"`
	CheckErrorTotal    uint64 `json:"check_error_total"`
	RepairTotal        uint64 `json:"repair_total"`
	RepairErrorTotal   uint64 `json:"repair_error_total"`
	CurrentLagSeconds  int64  `json:"current_lag_seconds"`
	CurrentBacklogRows int64  `json:"current_backlog_rows"`
}

type schedulerOptimizationMetrics struct {
	snapshotReadTotal                atomic.Uint64
	snapshotLocalHitTotal            atomic.Uint64
	snapshotLocalMissTotal           atomic.Uint64
	snapshotLocalInvalidationTotal   atomic.Uint64
	snapshotRedisHitTotal            atomic.Uint64
	snapshotEmptyMissTotal           atomic.Uint64
	snapshotErrorTotal               atomic.Uint64
	snapshotAccountTotal             atomic.Uint64
	snapshotDurationMicros           atomic.Uint64
	snapshotDBFallbackTotal          atomic.Uint64
	snapshotDBFallbackErrorTotal     atomic.Uint64
	snapshotDBFallbackLimitedTotal   atomic.Uint64
	snapshotDBFallbackDisabledTotal  atomic.Uint64
	snapshotDBFallbackAccountTotal   atomic.Uint64
	snapshotDBFallbackDurationMicros atomic.Uint64

	loadRequestTotal           atomic.Uint64
	loadCacheHitTotal          atomic.Uint64
	loadCacheMissTotal         atomic.Uint64
	loadCacheBypassTotal       atomic.Uint64
	loadBackendFetchTotal      atomic.Uint64
	loadBackendFetchErrorTotal atomic.Uint64
	loadBackendAccountTotal    atomic.Uint64
	loadBackendDurationMicros  atomic.Uint64
	loadScriptTotal            atomic.Uint64
	loadPipelineTotal          atomic.Uint64
	loadPipelineFallbackTotal  atomic.Uint64
	loadRedisErrorTotal        atomic.Uint64
	loadRedisAccountTotal      atomic.Uint64
	loadRedisBatchTotal        atomic.Uint64
	loadRedisDurationMicros    atomic.Uint64

	dbBatchTotal           atomic.Uint64
	dbQueryTotal           atomic.Uint64
	dbCandidateIDTotal     atomic.Uint64
	dbParentIDTotal        atomic.Uint64
	dbReturnedAccountTotal atomic.Uint64
	dbMissingAccountTotal  atomic.Uint64
	dbErrorTotal           atomic.Uint64
	dbDurationMicros       atomic.Uint64

	incrementalEventAttemptTotal    atomic.Uint64
	incrementalEventCommittedTotal  atomic.Uint64
	incrementalEventReplayTotal     atomic.Uint64
	incrementalEventFailureTotal    atomic.Uint64
	incrementalBucketAttemptTotal   atomic.Uint64
	incrementalBucketRemovedTotal   atomic.Uint64
	incrementalBucketNoopTotal      atomic.Uint64
	incrementalBucketErrorTotal     atomic.Uint64
	incrementalFallbackRebuildTotal atomic.Uint64
	incrementalDurationMicros       atomic.Uint64

	rebuildBucketAttemptTotal atomic.Uint64
	rebuildBucketSuccessTotal atomic.Uint64
	rebuildBucketErrorTotal   atomic.Uint64
	rebuildBucketFencedTotal  atomic.Uint64
	rebuildBucketBusyTotal    atomic.Uint64
	rebuildAccountTotal       atomic.Uint64
	rebuildDurationMicros     atomic.Uint64
	fullRebuildTotal          atomic.Uint64
	fullRebuildErrorTotal     atomic.Uint64
	fullRebuildDurationMicros atomic.Uint64

	driftLagWarningTotal    atomic.Uint64
	driftCheckErrorTotal    atomic.Uint64
	driftRepairTotal        atomic.Uint64
	driftRepairErrorTotal   atomic.Uint64
	driftCurrentLagSeconds  atomic.Int64
	driftCurrentBacklogRows atomic.Int64
}

var defaultSchedulerOptimizationMetrics schedulerOptimizationMetrics

// GetSchedulerOptimizationMetricsSnapshot returns cumulative process-local
// scheduler optimization counters for diagnostics and the admin dashboard.
func GetSchedulerOptimizationMetricsSnapshot() SchedulerOptimizationMetricsSnapshot {
	m := &defaultSchedulerOptimizationMetrics
	return SchedulerOptimizationMetricsSnapshot{
		Snapshot: SchedulerSnapshotMetricsSnapshot{
			ReadTotal: m.snapshotReadTotal.Load(), LocalHitTotal: m.snapshotLocalHitTotal.Load(),
			LocalMissTotal: m.snapshotLocalMissTotal.Load(), LocalInvalidationTotal: m.snapshotLocalInvalidationTotal.Load(),
			RedisHitTotal: m.snapshotRedisHitTotal.Load(), EmptyMissTotal: m.snapshotEmptyMissTotal.Load(),
			ErrorTotal: m.snapshotErrorTotal.Load(), AccountTotal: m.snapshotAccountTotal.Load(),
			DurationTotalMs: microsToMillis(m.snapshotDurationMicros.Load()),
			DBFallbackTotal: m.snapshotDBFallbackTotal.Load(), DBFallbackErrorTotal: m.snapshotDBFallbackErrorTotal.Load(),
			DBFallbackLimitedTotal: m.snapshotDBFallbackLimitedTotal.Load(), DBFallbackDisabledTotal: m.snapshotDBFallbackDisabledTotal.Load(),
			DBFallbackAccountTotal: m.snapshotDBFallbackAccountTotal.Load(), DBFallbackDurationTotalMs: microsToMillis(m.snapshotDBFallbackDurationMicros.Load()),
		},
		Load: SchedulerLoadMetricsSnapshot{
			RequestTotal: m.loadRequestTotal.Load(), CacheHitTotal: m.loadCacheHitTotal.Load(), CacheMissTotal: m.loadCacheMissTotal.Load(),
			CacheBypassTotal: m.loadCacheBypassTotal.Load(), BackendFetchTotal: m.loadBackendFetchTotal.Load(),
			BackendFetchErrorTotal: m.loadBackendFetchErrorTotal.Load(), BackendAccountTotal: m.loadBackendAccountTotal.Load(),
			BackendDurationTotalMs: microsToMillis(m.loadBackendDurationMicros.Load()), ScriptTotal: m.loadScriptTotal.Load(),
			PipelineTotal: m.loadPipelineTotal.Load(), PipelineFallbackTotal: m.loadPipelineFallbackTotal.Load(),
			RedisErrorTotal: m.loadRedisErrorTotal.Load(), RedisAccountTotal: m.loadRedisAccountTotal.Load(),
			RedisBatchTotal: m.loadRedisBatchTotal.Load(), RedisDurationTotalMs: microsToMillis(m.loadRedisDurationMicros.Load()),
		},
		Database: SchedulerDBBatchMetricsSnapshot{
			BatchTotal: m.dbBatchTotal.Load(), QueryTotal: m.dbQueryTotal.Load(), CandidateIDTotal: m.dbCandidateIDTotal.Load(),
			ParentIDTotal: m.dbParentIDTotal.Load(), ReturnedAccountTotal: m.dbReturnedAccountTotal.Load(), MissingAccountTotal: m.dbMissingAccountTotal.Load(),
			ErrorTotal: m.dbErrorTotal.Load(), DurationTotalMs: microsToMillis(m.dbDurationMicros.Load()),
		},
		Incremental: SchedulerIncrementalMetricsSnapshot{
			EventAttemptTotal: m.incrementalEventAttemptTotal.Load(), EventCommittedTotal: m.incrementalEventCommittedTotal.Load(),
			EventReplayTotal: m.incrementalEventReplayTotal.Load(), EventFailureTotal: m.incrementalEventFailureTotal.Load(),
			BucketAttemptTotal: m.incrementalBucketAttemptTotal.Load(), BucketRemovedTotal: m.incrementalBucketRemovedTotal.Load(),
			BucketNoopTotal: m.incrementalBucketNoopTotal.Load(), BucketErrorTotal: m.incrementalBucketErrorTotal.Load(),
			FallbackRebuildTotal: m.incrementalFallbackRebuildTotal.Load(), DurationTotalMs: microsToMillis(m.incrementalDurationMicros.Load()),
		},
		Rebuild: SchedulerRebuildMetricsSnapshot{
			BucketAttemptTotal: m.rebuildBucketAttemptTotal.Load(), BucketSuccessTotal: m.rebuildBucketSuccessTotal.Load(),
			BucketErrorTotal: m.rebuildBucketErrorTotal.Load(), BucketFencedTotal: m.rebuildBucketFencedTotal.Load(),
			BucketBusyTotal: m.rebuildBucketBusyTotal.Load(), AccountTotal: m.rebuildAccountTotal.Load(),
			DurationTotalMs: microsToMillis(m.rebuildDurationMicros.Load()), FullRebuildTotal: m.fullRebuildTotal.Load(),
			FullRebuildErrorTotal: m.fullRebuildErrorTotal.Load(), FullRebuildDurationMs: microsToMillis(m.fullRebuildDurationMicros.Load()),
		},
		Drift: SchedulerDriftMetricsSnapshot{
			LagWarningTotal: m.driftLagWarningTotal.Load(), CheckErrorTotal: m.driftCheckErrorTotal.Load(),
			RepairTotal: m.driftRepairTotal.Load(), RepairErrorTotal: m.driftRepairErrorTotal.Load(),
			CurrentLagSeconds: m.driftCurrentLagSeconds.Load(), CurrentBacklogRows: m.driftCurrentBacklogRows.Load(),
		},
	}
}

func RecordSchedulerSnapshotRead(path string, accountCount int, hit bool, duration time.Duration, err error) {
	m := &defaultSchedulerOptimizationMetrics
	m.snapshotReadTotal.Add(1)
	m.snapshotAccountTotal.Add(uint64(maxMetricCount(accountCount)))
	m.snapshotDurationMicros.Add(durationMicros(duration))
	if err != nil {
		m.snapshotErrorTotal.Add(1)
	}
	switch path {
	case "local_hit":
		m.snapshotLocalHitTotal.Add(1)
	case "local_miss", "local_bypass":
		m.snapshotLocalMissTotal.Add(1)
	}
	if path != "local_hit" && hit {
		m.snapshotRedisHitTotal.Add(1)
	}
	if !hit && err == nil {
		m.snapshotEmptyMissTotal.Add(1)
	}
}

func RecordSchedulerSnapshotLocalInvalidation() {
	defaultSchedulerOptimizationMetrics.snapshotLocalInvalidationTotal.Add(1)
}

func RecordSchedulerSnapshotDBFallback(accountCount int, duration time.Duration, err error, limited, disabled bool) {
	m := &defaultSchedulerOptimizationMetrics
	m.snapshotDBFallbackTotal.Add(1)
	m.snapshotDBFallbackAccountTotal.Add(uint64(maxMetricCount(accountCount)))
	m.snapshotDBFallbackDurationMicros.Add(durationMicros(duration))
	if err != nil {
		m.snapshotDBFallbackErrorTotal.Add(1)
	}
	if limited {
		m.snapshotDBFallbackLimitedTotal.Add(1)
	}
	if disabled {
		m.snapshotDBFallbackDisabledTotal.Add(1)
	}
}

func RecordSchedulerLoadCache(hit, bypass bool) {
	m := &defaultSchedulerOptimizationMetrics
	m.loadRequestTotal.Add(1)
	if hit {
		m.loadCacheHitTotal.Add(1)
	} else {
		m.loadCacheMissTotal.Add(1)
	}
	if bypass {
		m.loadCacheBypassTotal.Add(1)
	}
}

func RecordSchedulerLoadBackend(accountCount int, duration time.Duration, err error) {
	m := &defaultSchedulerOptimizationMetrics
	m.loadBackendFetchTotal.Add(1)
	m.loadBackendAccountTotal.Add(uint64(maxMetricCount(accountCount)))
	m.loadBackendDurationMicros.Add(durationMicros(duration))
	if err != nil {
		m.loadBackendFetchErrorTotal.Add(1)
	}
}

// RecordSchedulerLoadRedis records the concrete Redis implementation path.
// path is one of script, pipeline, or pipeline_fallback.
func RecordSchedulerLoadRedis(path string, accountCount, batchCount int, duration time.Duration, err error) {
	m := &defaultSchedulerOptimizationMetrics
	switch path {
	case "script":
		m.loadScriptTotal.Add(1)
	case "pipeline_fallback":
		m.loadPipelineFallbackTotal.Add(1)
		m.loadPipelineTotal.Add(1)
	default:
		m.loadPipelineTotal.Add(1)
	}
	m.loadRedisAccountTotal.Add(uint64(maxMetricCount(accountCount)))
	m.loadRedisBatchTotal.Add(uint64(maxMetricCount(batchCount)))
	m.loadRedisDurationMicros.Add(durationMicros(duration))
	if err != nil {
		m.loadRedisErrorTotal.Add(1)
	}
}

func RecordSchedulerDBBatch(candidateCount, parentCount, queryCount, returnedCount, missingCount int, duration time.Duration, err error) {
	m := &defaultSchedulerOptimizationMetrics
	m.dbBatchTotal.Add(1)
	m.dbQueryTotal.Add(uint64(maxMetricCount(queryCount)))
	m.dbCandidateIDTotal.Add(uint64(maxMetricCount(candidateCount)))
	m.dbParentIDTotal.Add(uint64(maxMetricCount(parentCount)))
	m.dbReturnedAccountTotal.Add(uint64(maxMetricCount(returnedCount)))
	m.dbMissingAccountTotal.Add(uint64(maxMetricCount(missingCount)))
	m.dbDurationMicros.Add(durationMicros(duration))
	if err != nil {
		m.dbErrorTotal.Add(1)
	}
}

func RecordSchedulerOutboxEventAttempt(replay bool) {
	m := &defaultSchedulerOptimizationMetrics
	m.incrementalEventAttemptTotal.Add(1)
	if replay {
		m.incrementalEventReplayTotal.Add(1)
	}
}

func RecordSchedulerOutboxEventsCommitted(count int) {
	defaultSchedulerOptimizationMetrics.incrementalEventCommittedTotal.Add(uint64(maxMetricCount(count)))
}

func RecordSchedulerOutboxEventFailure() {
	defaultSchedulerOptimizationMetrics.incrementalEventFailureTotal.Add(1)
}

func RecordSchedulerIncrementalBucket(removed bool, duration time.Duration, err error) {
	m := &defaultSchedulerOptimizationMetrics
	m.incrementalBucketAttemptTotal.Add(1)
	m.incrementalDurationMicros.Add(durationMicros(duration))
	if err != nil {
		m.incrementalBucketErrorTotal.Add(1)
	} else if removed {
		m.incrementalBucketRemovedTotal.Add(1)
	} else {
		m.incrementalBucketNoopTotal.Add(1)
	}
}

func RecordSchedulerIncrementalFallback() {
	defaultSchedulerOptimizationMetrics.incrementalFallbackRebuildTotal.Add(1)
}

// outcome is success, error, fenced, or busy.
func RecordSchedulerBucketRebuild(outcome string, accountCount int, duration time.Duration) {
	m := &defaultSchedulerOptimizationMetrics
	m.rebuildBucketAttemptTotal.Add(1)
	m.rebuildAccountTotal.Add(uint64(maxMetricCount(accountCount)))
	m.rebuildDurationMicros.Add(durationMicros(duration))
	switch outcome {
	case "success":
		m.rebuildBucketSuccessTotal.Add(1)
	case "fenced":
		m.rebuildBucketFencedTotal.Add(1)
	case "busy":
		m.rebuildBucketBusyTotal.Add(1)
	default:
		m.rebuildBucketErrorTotal.Add(1)
	}
}

func RecordSchedulerFullRebuild(duration time.Duration, err error) {
	m := &defaultSchedulerOptimizationMetrics
	m.fullRebuildTotal.Add(1)
	m.fullRebuildDurationMicros.Add(durationMicros(duration))
	if err != nil {
		m.fullRebuildErrorTotal.Add(1)
	}
}

func RecordSchedulerOutboxState(lagSeconds, backlogRows int64, warning, checkError bool) {
	m := &defaultSchedulerOptimizationMetrics
	if lagSeconds < 0 {
		lagSeconds = 0
	}
	if backlogRows < 0 {
		backlogRows = 0
	}
	m.driftCurrentLagSeconds.Store(lagSeconds)
	m.driftCurrentBacklogRows.Store(backlogRows)
	if warning {
		m.driftLagWarningTotal.Add(1)
	}
	if checkError {
		m.driftCheckErrorTotal.Add(1)
	}
}

func RecordSchedulerOutboxRepair(err error) {
	m := &defaultSchedulerOptimizationMetrics
	m.driftRepairTotal.Add(1)
	if err != nil {
		m.driftRepairErrorTotal.Add(1)
	}
}

func durationMicros(duration time.Duration) uint64 {
	if duration <= 0 {
		return 0
	}
	return uint64(duration.Microseconds())
}

func microsToMillis(micros uint64) float64 {
	return float64(micros) / 1000
}

func maxMetricCount(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
