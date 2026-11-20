package securityaudit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/ristretto"
)

// maxConcurrentChunks bounds how many chunks of one request are audited at the
// same time. The per-node bulkheads are shared by all requests, so one long
// prompt must not take every slot of a node.
const maxConcurrentChunks = 8

const (
	verdictCacheEntries = 10000
	verdictCacheTTL     = 24 * time.Hour
)

type GuardEvaluator struct {
	scanner  PromptScanner
	repo     JobRepository
	metrics  Metrics
	clock    Clock
	verdicts *verdictCache

	global       chan struct{}
	perNodeLimit int
	nodeMu       sync.Mutex
	nodes        map[string]chan struct{}
}

// verdictCache keeps the verdicts of recently audited texts. A verdict depends
// only on the audited text and the config version. With latest-turn-only
// blocking, every request of one agent turn carries the same audited text:
// tool calls and their outputs add none, so the text changes only when the
// user writes again. A reused verdict costs no guard call, so it carries no
// AuditUsage and bills no fee.
type verdictCache struct{ cache *ristretto.Cache }

func newVerdictCache() *verdictCache {
	// Each entry costs 1, so the internal per-item size must not count, or the
	// cache would hold only MaxCost/57 entries.
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: verdictCacheEntries * 10, MaxCost: verdictCacheEntries, BufferItems: 64,
		IgnoreInternalCost: true,
	})
	if err != nil {
		panic(err)
	}
	return &verdictCache{cache: cache}
}

func verdictCacheKey(configVersion int64, scanText string) string {
	digest := sha256.Sum256([]byte(scanText))
	return strconv.FormatInt(configVersion, 10) + ":" + hex.EncodeToString(digest[:])
}

// get returns a copy, so callers may change the result they are handed.
func (c *verdictCache) get(key string) (*NormalizedResult, bool) {
	value, ok := c.cache.Get(key)
	if !ok {
		return nil, false
	}
	result, isResult := value.(*NormalizedResult)
	if !isResult {
		panic(fmt.Sprintf("verdict cache holds %T", value))
	}
	return cloneNormalizedResult(result), true
}

func (c *verdictCache) put(key string, result *NormalizedResult) {
	stored := cloneNormalizedResult(result)
	stored.AuditUsage = nil
	stored.LatencyMS = 0
	c.cache.SetWithTTL(key, stored, 1, verdictCacheTTL)
	c.cache.Wait()
}

func cloneNormalizedResult(result *NormalizedResult) *NormalizedResult {
	clone := *result
	clone.Categories = slices.Clone(result.Categories)
	clone.MatchedScanners = slices.Clone(result.MatchedScanners)
	clone.UnknownCategories = slices.Clone(result.UnknownCategories)
	clone.ScannerScores = maps.Clone(result.ScannerScores)
	clone.ScannerEvidence = maps.Clone(result.ScannerEvidence)
	if result.AuditUsage != nil {
		usage := *result.AuditUsage
		clone.AuditUsage = &usage
	}
	return &clone
}

func NewGuardEvaluator(scanner PromptScanner, repo JobRepository, metrics Metrics) *GuardEvaluator {
	return newGuardEvaluator(scanner, repo, metrics, 64, 16)
}

func newGuardEvaluator(scanner PromptScanner, repo JobRepository, metrics Metrics, globalLimit, perNodeLimit int) *GuardEvaluator {
	if globalLimit < 1 {
		globalLimit = 64
	}
	if perNodeLimit < 1 {
		perNodeLimit = 16
	}
	return &GuardEvaluator{scanner: scanner, repo: repo, metrics: metrics, clock: realClock{}, verdicts: newVerdictCache(),
		global: make(chan struct{}, globalLimit), perNodeLimit: perNodeLimit, nodes: map[string]chan struct{}{}}
}

func (g *GuardEvaluator) Evaluate(ctx context.Context, cfg ActiveConfig, snapshot PromptSnapshot) (*PromptDecision, error) {
	if g == nil || g.scanner == nil {
		if g != nil && g.metrics != nil {
			g.metrics.Observe(DecisionUnavailable, 0)
		}
		logGuardFailure(snapshot, cfg, DecisionUnavailable, ErrorCodeUnavailable, "", 0)
		return nil, &GuardError{Code: ErrorCodeUnavailable}
	}
	start := g.clock.Now()
	baseFields := snapshotLogFields(snapshot)
	baseFields["config_version"] = cfg.ConfigVersion
	endpoints := cfg.EnabledEndpoints()
	if len(endpoints) == 0 {
		if g.metrics != nil {
			g.metrics.Observe(DecisionUnavailable, g.clock.Now().Sub(start))
		}
		logGuardFailure(snapshot, cfg, DecisionUnavailable, ErrorCodeUnavailable, "", g.clock.Now().Sub(start))
		return nil, &GuardError{Code: ErrorCodeUnavailable}
	}
	verdictKey := verdictCacheKey(cfg.ConfigVersion, snapshot.ScanText)
	aggregated, reused := g.verdicts.get(verdictKey)
	if reused {
		aggregated.LatencyMS = int(g.clock.Now().Sub(start).Milliseconds())
	} else {
		var err error
		aggregated, err = g.scan(ctx, cfg, endpoints, snapshot, baseFields, start)
		if err != nil {
			return nil, err
		}
		if aggregated == nil {
			return &PromptDecision{Kind: DecisionAllow, AllowNextStage: true}, nil
		}
		g.verdicts.put(verdictKey, aggregated)
	}
	kind := DecisionAllow
	if aggregated.Action == ActionWarn {
		kind = DecisionFlag
	}
	if aggregated.Action == ActionBlock {
		kind = DecisionBlock
	}
	decision := &PromptDecision{Kind: kind, Result: aggregated, AllowNextStage: kind == DecisionAllow || kind == DecisionFlag}
	if kind == DecisionBlock {
		decision.ErrorCode = ErrorCodeBlocked
	}
	if g.metrics != nil {
		g.metrics.Observe(kind, g.clock.Now().Sub(start))
	}
	LogInfo(EventChunksAggregated, mergeLogFields(baseFields, map[string]any{
		"decision":   kind,
		"risk_level": aggregated.RiskLevel, "action": aggregated.Action, "chunk_total": aggregated.ChunkTotal,
		"latency_ms": aggregated.LatencyMS, "guard_endpoint_id": aggregated.GuardEndpointID, "stage": snapshot.Stage,
		"status": "completed", "verdict_reused": reused,
	}))
	if g.repo != nil {
		if _, recordErr := g.repo.RecordBlocking(ctx, snapshot.Redacted(), cfg.ConfigVersion, aggregated, cfg.StorePassEvents); recordErr != nil {
			if g.metrics != nil {
				g.metrics.IncRecordFailed()
			}
			LogWarn(EventResultRecordFailed, mergeLogFields(baseFields, map[string]any{
				"decision": kind, "error_code": "result_record_failed", "stage": snapshot.Stage,
				"status": "failed",
			}))
		}
	}
	if kind == DecisionBlock {
		LogWarn(EventGuardBlocked, mergeLogFields(baseFields, map[string]any{
			"guard_endpoint_id": aggregated.GuardEndpointID,
			"decision":          kind, "risk_level": aggregated.RiskLevel, "action": aggregated.Action, "chunk_total": aggregated.ChunkTotal,
			"latency_ms": aggregated.LatencyMS, "status": "blocked", "error_code": ErrorCodeBlocked,
			"stage": snapshot.Stage, "upstream_dispatched": false, "billing_preconsumed": false, "verdict_reused": reused,
		}))
	} else {
		LogInfo(EventGuardAllowed, mergeLogFields(baseFields, map[string]any{
			"decision": kind, "risk_level": aggregated.RiskLevel, "action": aggregated.Action,
			"guard_endpoint_id": aggregated.GuardEndpointID, "chunk_total": aggregated.ChunkTotal,
			"latency_ms": aggregated.LatencyMS, "stage": snapshot.Stage, "status": "allowed", "verdict_reused": reused,
		}))
	}
	return decision, nil
}

// scan audits the snapshot's text on the endpoints and returns the verdict, or
// nil when there is no text to audit. It logs and counts its own failures.
func (g *GuardEvaluator) scan(ctx context.Context, cfg ActiveConfig, endpoints []ActiveEndpoint, snapshot PromptSnapshot, baseFields map[string]any, start time.Time) (*NormalizedResult, error) {
	select {
	case g.global <- struct{}{}:
		defer func() { <-g.global }()
	default:
		if g.metrics != nil {
			g.metrics.IncBulkheadFull()
			g.metrics.Observe(DecisionUnavailable, g.clock.Now().Sub(start))
		}
		logGuardFailure(snapshot, cfg, DecisionUnavailable, ErrorCodeUnavailable, "", g.clock.Now().Sub(start))
		return nil, &GuardError{Code: ErrorCodeUnavailable}
	}
	timeout := time.Duration(endpoints[0].TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = DefaultTimeoutMS * time.Millisecond
	}
	evalCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	inputLimit := minimumInputLimit(endpoints)
	chunks := scanChunksFor(snapshot.ScanText, endpoints)
	if len(chunks) == 0 {
		if g.metrics != nil {
			g.metrics.Observe(DecisionAllow, g.clock.Now().Sub(start))
		}
		return nil, nil
	}
	LogInfo(EventEvaluationStarted, mergeLogFields(baseFields, map[string]any{"chunk_total": len(chunks), "status": "started"}))
	chunkFields := func(index int, chunk string) map[string]any {
		return mergeLogFields(baseFields, map[string]any{
			"chunk_index": index + 1, "chunk_total": len(chunks),
			"chunk_chars": len([]rune(chunk)), "input_chars": snapshot.PromptLength, "input_limit": inputLimit,
		})
	}
	results, err := g.scanChunks(evalCtx, cfg, endpoints, chunks, chunkFields)
	if err != nil {
		code := guardErrorCode(err)
		kind := DecisionUnavailable
		if code == ErrorCodeInvalidResponse {
			kind = DecisionInvalid
		}
		if g.metrics != nil {
			g.metrics.Observe(kind, g.clock.Now().Sub(start))
			var guardErr *GuardError
			if errors.As(err, &guardErr) && guardErr.Timeout {
				g.metrics.IncTimeout()
			}
		}
		logGuardFailure(snapshot, cfg, kind, code, "", g.clock.Now().Sub(start))
		return nil, err
	}
	aggregated, err := AggregateResults(results, g.clock.Now().Sub(start))
	if err != nil {
		if g.metrics != nil {
			g.metrics.Observe(DecisionInvalid, g.clock.Now().Sub(start))
		}
		logGuardFailure(snapshot, cfg, DecisionInvalid, ErrorCodeInvalidResponse, "", g.clock.Now().Sub(start))
		return nil, &GuardError{Code: ErrorCodeInvalidResponse, Cause: err}
	}
	aggregated.ChunkTotal = len(chunks)
	return aggregated, nil
}

// scanChunks audits the chunks concurrently, at most maxConcurrentChunks at a
// time, all under ctx, so a long prompt costs about one round trip per
// maxConcurrentChunks chunks instead of one per chunk. A Block result cancels
// the chunks still running and wins over failures: the request is denied either
// way and Block is the real verdict. Otherwise any failed or unscanned chunk
// fails the evaluation, because a partial result never allows.
func (g *GuardEvaluator) scanChunks(ctx context.Context, cfg ActiveConfig, endpoints []ActiveEndpoint, chunks []string, chunkFields func(index int, chunk string) map[string]any) ([]*NormalizedResult, error) {
	scanCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make([]*NormalizedResult, len(chunks))
	var (
		next     atomic.Int64
		blocked  atomic.Bool
		failMu   sync.Mutex
		firstErr error
		wg       sync.WaitGroup
	)
	for range min(maxConcurrentChunks, len(chunks)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				index := int(next.Add(1) - 1)
				if index >= len(chunks) || scanCtx.Err() != nil {
					return
				}
				chunk := chunks[index]
				chunkStarted := g.clock.Now()
				LogInfo(EventChunkStarted, mergeLogFields(chunkFields(index, chunk), map[string]any{"status": "started"}))
				result, err := g.scanChunk(scanCtx, cfg, endpoints, chunk)
				if err != nil {
					if blocked.Load() {
						return
					}
					LogWarn(EventChunkFailed, mergeLogFields(chunkFields(index, chunk), map[string]any{
						"latency_ms": g.clock.Now().Sub(chunkStarted).Milliseconds(), "error_code": guardErrorCode(err), "status": "failed",
					}))
					failMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					failMu.Unlock()
					cancel()
					return
				}
				result.ChunkTotal = len(chunks)
				results[index] = result
				LogInfo(EventChunkCompleted, mergeLogFields(chunkFields(index, chunk), map[string]any{
					"guard_endpoint_id": result.GuardEndpointID, "action": result.Action,
					"latency_ms": g.clock.Now().Sub(chunkStarted).Milliseconds(), "status": "completed",
				}))
				if result.Action == ActionBlock {
					blocked.Store(true)
					cancel()
					return
				}
			}
		}()
	}
	wg.Wait()
	completed := make([]*NormalizedResult, 0, len(chunks))
	for _, result := range results {
		if result != nil {
			completed = append(completed, result)
		}
	}
	if blocked.Load() {
		return completed, nil
	}
	if firstErr != nil {
		return nil, firstErr
	}
	if len(completed) != len(chunks) {
		// The deadline or the caller ended the evaluation between chunks.
		return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: true, Timeout: errors.Is(ctx.Err(), context.DeadlineExceeded), Cause: ctx.Err()}
	}
	return completed, nil
}

func logGuardFailure(snapshot PromptSnapshot, cfg ActiveConfig, kind DecisionKind, code, guardEndpointID string, latency time.Duration) {
	fields := snapshotLogFields(snapshot)
	fields["config_version"] = cfg.ConfigVersion
	LogWarn(EventGuardFailed, mergeLogFields(fields, map[string]any{
		"decision": kind, "guard_endpoint_id": guardEndpointID, "latency_ms": latency.Milliseconds(),
		"status": "failed", "error_code": code, "upstream_dispatched": false, "billing_preconsumed": false,
	}))
}

func (g *GuardEvaluator) scanChunk(ctx context.Context, cfg ActiveConfig, endpoints []ActiveEndpoint, chunk string) (*NormalizedResult, error) {
	var lastErr error
	for index, endpoint := range endpoints {
		semaphore := g.nodeSemaphore(endpoint.ID)
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: true, Timeout: errors.Is(ctx.Err(), context.DeadlineExceeded), Cause: ctx.Err()}
		default:
			if g.metrics != nil {
				g.metrics.IncBulkheadFull()
			}
			lastErr = &GuardError{Code: ErrorCodeUnavailable, Retryable: true}
			if index < len(endpoints)-1 && g.metrics != nil {
				g.metrics.IncFailover()
			}
			continue
		}
		result, err := callPromptScanner(ctx, g.scanner, endpoint, chunk, cfg.Scanners)
		<-semaphore
		if err == nil && result != nil {
			return result, nil
		}
		if err == nil {
			err = &GuardError{Code: ErrorCodeInvalidResponse, Retryable: false}
		}
		lastErr = err
		var guardErr *GuardError
		if !errors.As(err, &guardErr) || !guardErr.Retryable {
			return nil, err
		}
		if index < len(endpoints)-1 && g.metrics != nil {
			g.metrics.IncFailover()
		}
	}
	if lastErr == nil {
		lastErr = &GuardError{Code: ErrorCodeUnavailable}
	}
	return nil, lastErr
}

func callPromptScanner(ctx context.Context, scanner PromptScanner, endpoint ActiveEndpoint, chunk string, scanners []string) (result *NormalizedResult, err error) {
	defer func() {
		if recover() != nil {
			result = nil
			err = &GuardError{Code: ErrorCodeUnavailable, Retryable: false}
		}
	}()
	return scanner.Scan(ctx, endpoint, chunk, scanners)
}

func (g *GuardEvaluator) nodeSemaphore(id string) chan struct{} {
	g.nodeMu.Lock()
	defer g.nodeMu.Unlock()
	semaphore := g.nodes[id]
	if semaphore == nil {
		semaphore = make(chan struct{}, g.perNodeLimit)
		g.nodes[id] = semaphore
	}
	return semaphore
}

func minimumInputLimit(endpoints []ActiveEndpoint) int {
	limit := DefaultInputLimit
	for index, endpoint := range endpoints {
		value := endpoint.InputLimit
		if value <= 0 {
			value = DefaultInputLimit
		}
		if index == 0 || value < limit {
			limit = value
		}
	}
	return limit
}

func guardErrorCode(err error) string {
	var guardErr *GuardError
	if errors.As(err, &guardErr) && guardErr.Code != "" {
		return guardErr.Code
	}
	return ErrorCodeUnavailable
}

func pointerLogID(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
