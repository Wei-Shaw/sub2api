package securityaudit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

type WorkerRuntime struct {
	active           atomic.Int64
	processed        atomic.Int64
	failed           atomic.Int64
	heartbeatNS      atomic.Int64
	lastProcessedNS  atomic.Int64
	lastErrorMu      sync.RWMutex
	lastErrorCode    string
	lastErrorMessage string
REDACTED

type Runner struct {
	config  ConfigStore
	repo    JobRepository
	payload PayloadStore
	scanner PromptScanner
	metrics Metrics
	clock   Clock
	runtime WorkerRuntime

	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
REDACTED

func NewRunner(config ConfigStore, repo JobRepository, payload PayloadStore, scanner PromptScanner, metrics Metrics) *Runner {
	return &Runner{config: config, repo: repo, payload: payload, scanner: scanner, metrics: metrics, clock: realClock{REDACTEDREDACTED
REDACTED

func (r *Runner) Start(ctx context.Context) error {
	if r == nil || r.config == nil || r.repo == nil || r.payload == nil || r.scanner == nil {
		return errors.New("prompt audit worker dependencies unavailable")
REDACTED
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return nil
REDACTED
	runCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.mu.Unlock()
	if err := r.payload.Ping(runCtx); err != nil {
		r.setLastError("payload_store_unavailable", err.Error())
REDACTED
	for workerID := 0; workerID < MaxWorkerCount; workerID++ {
		r.wg.Add(1)
		go r.worker(runCtx, workerID)
REDACTED
	r.wg.Add(1)
	go r.reclaimer(runCtx)
	return nil
REDACTED

func (r *Runner) Shutdown(ctx context.Context) error {
	if r == nil {
		return nil
REDACTED
	r.mu.Lock()
	cancel := r.cancel
	r.cancel = nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
REDACTED
	done := make(chan struct{REDACTED)
	go func() { r.wg.Wait(); close(done) REDACTED()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		LogWarn(EventProcessFailed, map[string]any{"status": "shutdown_timeout", "error_code": "worker_shutdown_timeout"REDACTED)
		return ctx.Err()
REDACTED
REDACTED

func (r *Runner) worker(ctx context.Context, workerID int) {
	defer r.wg.Done()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runtime.heartbeatNS.Store(r.clock.Now().UnixNano())
			cfg, ok := r.config.Active()
			if !ok || !cfg.RiskControlEnabled || !cfg.Enabled || workerID >= cfg.WorkerCount {
				continue
		REDACTED
			for {
				job, claimed, err := r.repo.ClaimNextJob(ctx, r.clock.Now())
				if err != nil {
					r.setLastError("claim_job_failed", err.Error())
					break
			REDACTED
				if !claimed {
					break
			REDACTED
				r.runtime.active.Add(1)
				r.processSafely(ctx, workerID, cfg, job)
				r.runtime.active.Add(-1)
		REDACTED
	REDACTED
REDACTED
REDACTED

func (r *Runner) processSafely(ctx context.Context, workerID int, cfg ActiveConfig, job *Job) {
	defer func() {
		if recovered := recover(); recovered != nil {
			r.runtime.failed.Add(1)
			// Panic values may contain scanner response fragments or prompt data.
			// Keep only a stable generic message in runtime state and logs.
			r.setLastError("worker_panic", "worker panic recovered")
			_ = r.repo.Fail(ctx, job.ID, job.ClaimVersion, "worker_panic", "worker panic recovered")
			LogError(EventProcessFailed, mergeLogFields(jobLogFields(job), map[string]any{"worker_id": workerID, "status": "failed", "error_code": "worker_panic"REDACTED))
	REDACTED
REDACTED()
	if err := r.processJob(ctx, workerID, cfg, job); err != nil {
		r.runtime.failed.Add(1)
REDACTED else {
		r.runtime.processed.Add(1)
		r.runtime.lastProcessedNS.Store(r.clock.Now().UnixNano())
REDACTED
REDACTED

func (r *Runner) processJob(ctx context.Context, workerID int, cfg ActiveConfig, job *Job) error {
	baseFields := jobLogFields(job)
	LogInfo(EventAuditStarted, mergeLogFields(baseFields, map[string]any{"worker_id": workerID, "attempts": job.Attempts, "status": "processing"REDACTED))
	scanText, err := r.payload.Get(ctx, job.ID)
	if err != nil {
		return r.finishFailure(ctx, job, &GuardError{Code: "payload_missing", Retryable: false, Cause: errREDACTED)
REDACTED
	// The job row only carries redacted metadata; the full prompt for the audit
	// event is reconstructed here from the transient scan payload.
	job.Snapshot.FullPrompt = FullPromptFromScanText(scanText)
	endpoints := cfg.EnabledEndpoints()
	if len(endpoints) == 0 {
		return r.finishFailure(ctx, job, &GuardError{Code: "no_enabled_endpoint", Retryable: trueREDACTED)
REDACTED
	chunks := SplitRunes(scanText, minimumInputLimit(endpoints))
	results := make([]*NormalizedResult, 0, len(chunks))
	started := r.clock.Now()
	for index, chunk := range chunks {
		if err := r.repo.RefreshLease(ctx, job.ID, job.ClaimVersion, r.clock.Now()); err != nil {
			return err
	REDACTED
		chunkStarted := r.clock.Now()
		LogInfo(EventChunkStarted, mergeLogFields(baseFields, map[string]any{"worker_id": workerID, "chunk_index": index + 1, "chunk_total": len(chunks), "chunk_chars": len([]rune(chunk)), "input_chars": job.Snapshot.PromptLength, "input_limit": minimumInputLimit(endpoints), "status": "started"REDACTED))
		result, scanErr := scanWithFailover(ctx, r.scanner, cfg.Scanners, endpoints, chunk, r.metrics)
		if scanErr != nil {
			LogWarn(EventChunkFailed, mergeLogFields(baseFields, map[string]any{
				"worker_id": workerID, "chunk_index": index + 1, "chunk_total": len(chunks),
				"chunk_chars": len([]rune(chunk)), "input_chars": job.Snapshot.PromptLength,
				"input_limit": minimumInputLimit(endpoints), "latency_ms": r.clock.Now().Sub(chunkStarted).Milliseconds(),
				"error_code": guardErrorCode(scanErr), "status": "failed",
		REDACTED))
			r.observeAsyncFailure(scanErr, r.clock.Now().Sub(started))
			return r.finishFailure(ctx, job, scanErr)
	REDACTED
		results = append(results, result)
		LogInfo(EventChunkCompleted, mergeLogFields(baseFields, map[string]any{"worker_id": workerID, "chunk_index": index + 1, "chunk_total": len(chunks), "guard_endpoint_id": result.GuardEndpointID, "action": result.Action, "latency_ms": r.clock.Now().Sub(chunkStarted).Milliseconds(), "status": "completed"REDACTED))
		if result.Action == ActionBlock {
			break
	REDACTED
REDACTED
	aggregated, err := AggregateResults(results, r.clock.Now().Sub(started))
	if err != nil {
		if r.metrics != nil {
			r.metrics.Observe(DecisionInvalid, r.clock.Now().Sub(started))
	REDACTED
		return r.finishFailure(ctx, job, &GuardError{Code: ErrorCodeInvalidResponse, Cause: errREDACTED)
REDACTED
	aggregated.ChunkTotal = len(chunks)
	if r.metrics != nil {
		r.metrics.Observe(decisionKindForResult(aggregated), r.clock.Now().Sub(started))
REDACTED
	LogInfo(EventChunksAggregated, mergeLogFields(baseFields, map[string]any{
		"worker_id": workerID, "decision": aggregated.Decision, "risk_level": aggregated.RiskLevel,
		"action": aggregated.Action, "chunk_total": aggregated.ChunkTotal,
		"latency_ms": aggregated.LatencyMS, "guard_endpoint_id": aggregated.GuardEndpointID, "status": "completed",
REDACTED))
	event, err := r.repo.Complete(ctx, job, aggregated, cfg.StorePassEvents)
	if err != nil {
		return err
REDACTED
	if deleteErr := r.payload.Delete(ctx, job.ID); deleteErr != nil {
		LogWarn(EventProcessFailed, mergeLogFields(baseFields, map[string]any{"worker_id": workerID, "status": "payload_delete_deferred", "error_code": "payload_delete_failed"REDACTED))
REDACTED
	LogInfo(EventProcessed, mergeLogFields(baseFields, map[string]any{"worker_id": workerID, "event_id": eventID(event), "decision": aggregated.Decision, "risk_level": aggregated.RiskLevel, "action": aggregated.Action, "guard_endpoint_id": aggregated.GuardEndpointID, "latency_ms": aggregated.LatencyMS, "status": "done"REDACTED))
	if event != nil && aggregated.Decision != EventPass {
		LogWarn(EventFindingRecorded, mergeLogFields(baseFields, map[string]any{"worker_id": workerID, "event_id": event.ID, "decision": aggregated.Decision, "risk_level": aggregated.RiskLevel, "action": aggregated.Action, "guard_endpoint_id": aggregated.GuardEndpointID, "status": "recorded"REDACTED))
REDACTED
	return nil
REDACTED

func (r *Runner) observeAsyncFailure(err error, latency time.Duration) {
	if r == nil || r.metrics == nil {
		return
REDACTED
	kind := DecisionUnavailable
	if guardErrorCode(err) == ErrorCodeInvalidResponse {
		kind = DecisionInvalid
REDACTED
	r.metrics.Observe(kind, latency)
	var guardErr *GuardError
	if errors.As(err, &guardErr) && guardErr.Timeout {
		r.metrics.IncTimeout()
REDACTED
REDACTED

func decisionKindForResult(result *NormalizedResult) DecisionKind {
	if result == nil {
		return DecisionInvalid
REDACTED
	switch result.Action {
	case ActionBlock:
		return DecisionBlock
	case ActionWarn:
		return DecisionFlag
	default:
		return DecisionAllow
REDACTED
REDACTED

func (r *Runner) finishFailure(ctx context.Context, job *Job, err error) error {
	baseFields := jobLogFields(job)
	code := guardErrorCode(err)
	retryable := false
	var guardErr *GuardError
	if errors.As(err, &guardErr) {
		retryable = guardErr.Retryable
REDACTED
	if retryable && job.Attempts < job.MaxAttempts {
		next := r.clock.Now().Add(retryBackoff(job.Attempts))
		if updateErr := r.repo.Retry(ctx, job.ID, job.ClaimVersion, next, code, "prompt guard temporarily unavailable"); updateErr != nil {
			return updateErr
	REDACTED
		LogWarn(EventProcessFailed, mergeLogFields(baseFields, map[string]any{"attempts": job.Attempts, "max_attempts": job.MaxAttempts, "status": "retry", "error_code": code, "retryable": trueREDACTED))
REDACTED else {
		if updateErr := r.repo.Fail(ctx, job.ID, job.ClaimVersion, code, "prompt guard processing failed"); updateErr != nil {
			return updateErr
	REDACTED
		_ = r.payload.Delete(ctx, job.ID)
		LogError(EventProcessFailed, mergeLogFields(baseFields, map[string]any{"attempts": job.Attempts, "max_attempts": job.MaxAttempts, "status": "failed", "error_code": code, "retryable": falseREDACTED))
REDACTED
	r.setLastError(code, err.Error())
	return err
REDACTED

func (r *Runner) reclaimer(ctx context.Context) {
	defer r.wg.Done()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := r.clock.Now()
			count, err := r.repo.ReclaimStale(ctx, now.Add(-2*time.Minute), now.Add(-90*time.Second), 100)
			if err != nil {
				r.setLastError("reclaim_failed", err.Error())
				continue
		REDACTED
			if count > 0 {
				LogWarn(EventProcessingReclaimed, map[string]any{"reclaimed_total": count, "status": "reclaimed"REDACTED)
		REDACTED
	REDACTED
REDACTED
REDACTED

func (r *Runner) Snapshot() (active, processed, failed int64, heartbeat, lastProcessed *time.Time, code, message string) {
	if r == nil {
		return
REDACTED
	active, processed, failed = r.runtime.active.Load(), r.runtime.processed.Load(), r.runtime.failed.Load()
	if ns := r.runtime.heartbeatNS.Load(); ns > 0 {
		value := time.Unix(0, ns).UTC()
		heartbeat = &value
REDACTED
	if ns := r.runtime.lastProcessedNS.Load(); ns > 0 {
		value := time.Unix(0, ns).UTC()
		lastProcessed = &value
REDACTED
	r.runtime.lastErrorMu.RLock()
	code, message = r.runtime.lastErrorCode, r.runtime.lastErrorMessage
	r.runtime.lastErrorMu.RUnlock()
	return
REDACTED

func (r *Runner) setLastError(code, _ string) {
	code, message := sanitizeStoredError(code)
	r.runtime.lastErrorMu.Lock()
	r.runtime.lastErrorCode = code
	r.runtime.lastErrorMessage = message
	r.runtime.lastErrorMu.Unlock()
REDACTED

func scanWithFailover(ctx context.Context, scanner PromptScanner, scanners []string, endpoints []ActiveEndpoint, chunk string, metrics Metrics) (*NormalizedResult, error) {
	var lastErr error
	for index, endpoint := range endpoints {
		result, err := scanner.Scan(ctx, endpoint, chunk, scanners)
		if err == nil && result != nil {
			return result, nil
	REDACTED
		if err == nil {
			err = &GuardError{Code: ErrorCodeInvalidResponse, Retryable: falseREDACTED
	REDACTED
		lastErr = err
		var guardErr *GuardError
		if !errors.As(err, &guardErr) || !guardErr.Retryable {
			return nil, err
	REDACTED
		if index < len(endpoints)-1 && metrics != nil {
			metrics.IncFailover()
	REDACTED
REDACTED
	if lastErr == nil {
		lastErr = &GuardError{Code: ErrorCodeUnavailableREDACTED
REDACTED
	return nil, lastErr
REDACTED

func retryBackoff(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 5 * time.Second
	case 2:
		return 30 * time.Second
	default:
		return 2 * time.Minute
REDACTED
REDACTED

func eventID(event *Event) int64 {
	if event == nil {
		return 0
REDACTED
	return event.ID
REDACTED
