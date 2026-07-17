package securityaudit

import (
	"context"
	"errors"
	"sync"
	"time"
)

type GuardEvaluator struct {
	scanner PromptScanner
	repo    JobRepository
	metrics Metrics
	clock   Clock

	global       chan struct{REDACTED
	perNodeLimit int
	nodeMu       sync.Mutex
	nodes        map[string]chan struct{REDACTED
REDACTED

func NewGuardEvaluator(scanner PromptScanner, repo JobRepository, metrics Metrics) *GuardEvaluator {
	return newGuardEvaluator(scanner, repo, metrics, 64, 16)
REDACTED

func newGuardEvaluator(scanner PromptScanner, repo JobRepository, metrics Metrics, globalLimit, perNodeLimit int) *GuardEvaluator {
	if globalLimit < 1 {
		globalLimit = 64
REDACTED
	if perNodeLimit < 1 {
		perNodeLimit = 16
REDACTED
	return &GuardEvaluator{scanner: scanner, repo: repo, metrics: metrics, clock: realClock{REDACTED,
		global: make(chan struct{REDACTED, globalLimit), perNodeLimit: perNodeLimit, nodes: map[string]chan struct{REDACTED{REDACTEDREDACTED
REDACTED

func (g *GuardEvaluator) Evaluate(ctx context.Context, cfg ActiveConfig, snapshot PromptSnapshot) (*PromptDecision, error) {
	if g == nil || g.scanner == nil {
		if g != nil && g.metrics != nil {
			g.metrics.Observe(DecisionUnavailable, 0)
	REDACTED
		logGuardFailure(snapshot, cfg, DecisionUnavailable, ErrorCodeUnavailable, "", 0)
		return nil, &GuardError{Code: ErrorCodeUnavailableREDACTED
REDACTED
	start := g.clock.Now()
	baseFields := snapshotLogFields(snapshot)
	baseFields["config_version"] = cfg.ConfigVersion
	endpoints := cfg.EnabledEndpoints()
	if len(endpoints) == 0 {
		if g.metrics != nil {
			g.metrics.Observe(DecisionUnavailable, g.clock.Now().Sub(start))
	REDACTED
		logGuardFailure(snapshot, cfg, DecisionUnavailable, ErrorCodeUnavailable, "", g.clock.Now().Sub(start))
		return nil, &GuardError{Code: ErrorCodeUnavailableREDACTED
REDACTED
	select {
	case g.global <- struct{REDACTED{REDACTED:
		defer func() { <-g.global REDACTED()
	default:
		if g.metrics != nil {
			g.metrics.IncBulkheadFull()
			g.metrics.Observe(DecisionUnavailable, g.clock.Now().Sub(start))
	REDACTED
		logGuardFailure(snapshot, cfg, DecisionUnavailable, ErrorCodeUnavailable, "", g.clock.Now().Sub(start))
		return nil, &GuardError{Code: ErrorCodeUnavailableREDACTED
REDACTED
	timeout := time.Duration(endpoints[0].TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = DefaultTimeoutMS * time.Millisecond
REDACTED
	evalCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	inputLimit := minimumInputLimit(endpoints)
	chunks := SplitRunes(snapshot.ScanText, inputLimit)
	if len(chunks) == 0 {
		if g.metrics != nil {
			g.metrics.Observe(DecisionAllow, g.clock.Now().Sub(start))
	REDACTED
		return &PromptDecision{Kind: DecisionAllow, AllowNextStage: trueREDACTED, nil
REDACTED
	LogInfo(EventEvaluationStarted, mergeLogFields(baseFields, map[string]any{"chunk_total": len(chunks), "status": "started"REDACTED))
	results := make([]*NormalizedResult, 0, len(chunks))
	for index, chunk := range chunks {
		chunkStarted := g.clock.Now()
		LogInfo(EventChunkStarted, mergeLogFields(baseFields, map[string]any{
			"chunk_index": index + 1, "chunk_total": len(chunks),
			"chunk_chars": len([]rune(chunk)), "input_chars": snapshot.PromptLength, "input_limit": inputLimit,
			"status": "started",
	REDACTED))
		result, err := g.scanChunk(evalCtx, cfg, endpoints, chunk)
		if err != nil {
			code := guardErrorCode(err)
			LogWarn(EventChunkFailed, mergeLogFields(baseFields, map[string]any{
				"chunk_index": index + 1, "chunk_total": len(chunks),
				"chunk_chars": len([]rune(chunk)), "input_chars": snapshot.PromptLength, "input_limit": inputLimit,
				"latency_ms": g.clock.Now().Sub(chunkStarted).Milliseconds(), "error_code": code, "status": "failed",
		REDACTED))
			kind := DecisionUnavailable
			if code == ErrorCodeInvalidResponse {
				kind = DecisionInvalid
		REDACTED
			if g.metrics != nil {
				g.metrics.Observe(kind, g.clock.Now().Sub(start))
				var guardErr *GuardError
				if errors.As(err, &guardErr) && guardErr.Timeout {
					g.metrics.IncTimeout()
			REDACTED
		REDACTED
			logGuardFailure(snapshot, cfg, kind, code, "", g.clock.Now().Sub(start))
			return nil, err
	REDACTED
		result.ChunkTotal = len(chunks)
		results = append(results, result)
		LogInfo(EventChunkCompleted, mergeLogFields(baseFields, map[string]any{
			"chunk_index": index + 1, "chunk_total": len(chunks),
			"chunk_chars": len([]rune(chunk)), "input_chars": snapshot.PromptLength, "input_limit": inputLimit,
			"guard_endpoint_id": result.GuardEndpointID, "action": result.Action,
			"latency_ms": g.clock.Now().Sub(chunkStarted).Milliseconds(), "status": "completed",
	REDACTED))
		if result.Action == ActionBlock {
			break
	REDACTED
REDACTED
	aggregated, err := AggregateResults(results, g.clock.Now().Sub(start))
	if err != nil {
		if g.metrics != nil {
			g.metrics.Observe(DecisionInvalid, g.clock.Now().Sub(start))
	REDACTED
		logGuardFailure(snapshot, cfg, DecisionInvalid, ErrorCodeInvalidResponse, "", g.clock.Now().Sub(start))
		return nil, &GuardError{Code: ErrorCodeInvalidResponse, Cause: errREDACTED
REDACTED
	aggregated.ChunkTotal = len(chunks)
	kind := DecisionAllow
	if aggregated.Action == ActionWarn {
		kind = DecisionFlag
REDACTED
	if aggregated.Action == ActionBlock {
		kind = DecisionBlock
REDACTED
	decision := &PromptDecision{Kind: kind, Result: aggregated, AllowNextStage: kind == DecisionAllow || kind == DecisionFlagREDACTED
	if kind == DecisionBlock {
		decision.ErrorCode = ErrorCodeBlocked
REDACTED
	if g.metrics != nil {
		g.metrics.Observe(kind, g.clock.Now().Sub(start))
REDACTED
	LogInfo(EventChunksAggregated, mergeLogFields(baseFields, map[string]any{
		"decision":   kind,
		"risk_level": aggregated.RiskLevel, "action": aggregated.Action, "chunk_total": aggregated.ChunkTotal,
		"latency_ms": aggregated.LatencyMS, "guard_endpoint_id": aggregated.GuardEndpointID, "stage": snapshot.Stage,
		"status": "completed",
REDACTED))
	if g.repo != nil {
		if _, recordErr := g.repo.RecordBlocking(ctx, snapshot.Redacted(), cfg.ConfigVersion, aggregated, cfg.StorePassEvents); recordErr != nil {
			if g.metrics != nil {
				g.metrics.IncRecordFailed()
		REDACTED
			LogWarn(EventResultRecordFailed, mergeLogFields(baseFields, map[string]any{
				"decision": kind, "error_code": "result_record_failed", "stage": snapshot.Stage,
				"status": "failed",
		REDACTED))
	REDACTED
REDACTED
	if kind == DecisionBlock {
		LogWarn(EventGuardBlocked, mergeLogFields(baseFields, map[string]any{
			"guard_endpoint_id": aggregated.GuardEndpointID,
			"decision":          kind, "risk_level": aggregated.RiskLevel, "action": aggregated.Action, "chunk_total": aggregated.ChunkTotal,
			"latency_ms": aggregated.LatencyMS, "status": "blocked", "error_code": ErrorCodeBlocked,
			"stage": snapshot.Stage, "upstream_dispatched": false, "billing_preconsumed": false,
	REDACTED))
REDACTED else {
		LogInfo(EventGuardAllowed, mergeLogFields(baseFields, map[string]any{
			"decision": kind, "risk_level": aggregated.RiskLevel, "action": aggregated.Action,
			"guard_endpoint_id": aggregated.GuardEndpointID, "chunk_total": aggregated.ChunkTotal,
			"latency_ms": aggregated.LatencyMS, "stage": snapshot.Stage, "status": "allowed",
	REDACTED))
REDACTED
	return decision, nil
REDACTED

func logGuardFailure(snapshot PromptSnapshot, cfg ActiveConfig, kind DecisionKind, code, guardEndpointID string, latency time.Duration) {
	fields := snapshotLogFields(snapshot)
	fields["config_version"] = cfg.ConfigVersion
	LogWarn(EventGuardFailed, mergeLogFields(fields, map[string]any{
		"decision": kind, "guard_endpoint_id": guardEndpointID, "latency_ms": latency.Milliseconds(),
		"status": "failed", "error_code": code, "upstream_dispatched": false, "billing_preconsumed": false,
REDACTED))
REDACTED

func (g *GuardEvaluator) scanChunk(ctx context.Context, cfg ActiveConfig, endpoints []ActiveEndpoint, chunk string) (*NormalizedResult, error) {
	var lastErr error
	for index, endpoint := range endpoints {
		semaphore := g.nodeSemaphore(endpoint.ID)
		select {
		case semaphore <- struct{REDACTED{REDACTED:
		case <-ctx.Done():
			return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: true, Timeout: errors.Is(ctx.Err(), context.DeadlineExceeded), Cause: ctx.Err()REDACTED
		default:
			if g.metrics != nil {
				g.metrics.IncBulkheadFull()
		REDACTED
			lastErr = &GuardError{Code: ErrorCodeUnavailable, Retryable: trueREDACTED
			if index < len(endpoints)-1 && g.metrics != nil {
				g.metrics.IncFailover()
		REDACTED
			continue
	REDACTED
		result, err := callPromptScanner(ctx, g.scanner, endpoint, chunk, cfg.Scanners)
		<-semaphore
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
		if index < len(endpoints)-1 && g.metrics != nil {
			g.metrics.IncFailover()
	REDACTED
REDACTED
	if lastErr == nil {
		lastErr = &GuardError{Code: ErrorCodeUnavailableREDACTED
REDACTED
	return nil, lastErr
REDACTED

func callPromptScanner(ctx context.Context, scanner PromptScanner, endpoint ActiveEndpoint, chunk string, scanners []string) (result *NormalizedResult, err error) {
	defer func() {
		if recover() != nil {
			result = nil
			err = &GuardError{Code: ErrorCodeUnavailable, Retryable: falseREDACTED
	REDACTED
REDACTED()
	return scanner.Scan(ctx, endpoint, chunk, scanners)
REDACTED

func (g *GuardEvaluator) nodeSemaphore(id string) chan struct{REDACTED {
	g.nodeMu.Lock()
	defer g.nodeMu.Unlock()
	semaphore := g.nodes[id]
	if semaphore == nil {
		semaphore = make(chan struct{REDACTED, g.perNodeLimit)
		g.nodes[id] = semaphore
REDACTED
	return semaphore
REDACTED

func minimumInputLimit(endpoints []ActiveEndpoint) int {
	limit := DefaultInputLimit
	for index, endpoint := range endpoints {
		value := endpoint.InputLimit
		if value <= 0 {
			value = DefaultInputLimit
	REDACTED
		if index == 0 || value < limit {
			limit = value
	REDACTED
REDACTED
	return limit
REDACTED

func guardErrorCode(err error) string {
	var guardErr *GuardError
	if errors.As(err, &guardErr) && guardErr.Code != "" {
		return guardErr.Code
REDACTED
	return ErrorCodeUnavailable
REDACTED

func pointerLogID(value *int64) int64 {
	if value == nil {
		return 0
REDACTED
	return *value
REDACTED
