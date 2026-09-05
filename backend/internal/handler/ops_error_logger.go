package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	opsModelKey                  = "ops_model"
	opsStreamKey                 = "ops_stream"
	opsAccountIDKey              = "ops_account_id"
	opsRoutingCapacityLimitedKey = "ops_routing_capacity_limited"
	opsDedicatedErrorRecordedKey = "ops_dedicated_error_recorded"

	opsUpstreamModelKey = service.OpsUpstreamModelKey
	opsRequestTypeKey   = "ops_request_type"

	// 错误过滤匹配常量 — shouldSkipOpsErrorLog 和错误分类共用
	opsErrContextCanceled            = "context canceled"
	opsErrNoAvailableAccounts        = "no available accounts"
	opsErrInvalidAPIKey              = "invalid_api_key"
	opsErrAPIKeyRequired             = "api_key_required"
	opsErrInsufficientBalance        = "insufficient balance"
	opsErrInsufficientAccountBalance = "insufficient account balance"
	opsErrInsufficientQuota          = "insufficient_quota"

	// 上游错误码常量 — 错误分类 (normalizeOpsErrorType / classifyOpsPhase / classifyOpsIsBusinessLimited)
	opsCodeInsufficientBalance   = "INSUFFICIENT_BALANCE"
	opsCodeUsageLimitExceeded    = "USAGE_LIMIT_EXCEEDED"
	opsCodeSubscriptionNotFound  = "SUBSCRIPTION_NOT_FOUND"
	opsCodeSubscriptionInvalid   = "SUBSCRIPTION_INVALID"
	opsCodeUserInactive          = "USER_INACTIVE"
	opsCodeInvalidAPIKey         = "INVALID_API_KEY"
	opsCodeAPIKeyRequired        = "API_KEY_REQUIRED"
	opsCodeAPIKeyExpired         = "API_KEY_EXPIRED"
	opsCodeAPIKeyDisabled        = "API_KEY_DISABLED"
	opsCodeUserNotFound          = "USER_NOT_FOUND"
	opsCodeAPIKeyQuotaExhausted  = "API_KEY_QUOTA_EXHAUSTED"
	opsCodeAPIKeyQueryDeprecated = "api_key_in_query_deprecated"
	opsCodeGroupDeleted          = "GROUP_DELETED"
	opsCodeGroupDisabled         = "GROUP_DISABLED"
)

const (
	opsErrorLogTimeout      = 5 * time.Second
	opsErrorLogDrainTimeout = 10 * time.Second
	opsErrorLogBatchWindow  = 200 * time.Millisecond

	opsErrorLogMinWorkerCount = 4
	opsErrorLogMaxWorkerCount = 32

	opsErrorLogQueueSizePerWorker = 128
	opsErrorLogMinQueueSize       = 256
	opsErrorLogMaxQueueSize       = 8192
	opsErrorLogBatchSize          = 32
	opsErrorLogMaxQueueBytes      = 32 * 1024 * 1024
	opsErrorLogMaxUserAgentBytes  = 512
)

// keyPrefix 返回脱敏前缀(前 n 个字符);不足 n 则原样返回。
func keyPrefix(key string, n int) string {
	if len(key) <= n {
		return key
	}
	return key[:n]
}

type opsErrorLogJob struct {
	ops         *service.OpsService
	entry       *service.OpsInsertErrorLogInput
	queuedBytes int64
}

var (
	opsErrorLogOnce  sync.Once
	opsErrorLogQueue chan opsErrorLogJob

	opsErrorLogStopOnce   sync.Once
	opsErrorLogWorkersWg  sync.WaitGroup
	opsErrorLogMu         sync.RWMutex
	opsErrorLogStopping   bool
	opsErrorLogQueueLen   atomic.Int64
	opsErrorLogQueueBytes atomic.Int64
	opsErrorLogEnqueued   atomic.Int64
	opsErrorLogDropped    atomic.Int64
	opsErrorLogProcessed  atomic.Int64
	opsErrorLogSanitized  atomic.Int64

	opsErrorLogLastDropLogAt atomic.Int64

	opsErrorLogShutdownCh   = make(chan struct{})
	opsErrorLogShutdownOnce sync.Once
	opsErrorLogDrained      atomic.Bool
)

func startOpsErrorLogWorkers() {
	opsErrorLogMu.Lock()
	defer opsErrorLogMu.Unlock()

	if opsErrorLogStopping {
		return
	}

	workerCount, queueSize := opsErrorLogConfig()
	opsErrorLogQueue = make(chan opsErrorLogJob, queueSize)
	opsErrorLogQueueLen.Store(0)
	opsErrorLogQueueBytes.Store(0)

	opsErrorLogWorkersWg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer opsErrorLogWorkersWg.Done()
			for {
				job, ok := <-opsErrorLogQueue
				if !ok {
					return
				}
				opsErrorLogQueueLen.Add(-1)
				opsErrorLogQueueBytes.Add(-job.queuedBytes)
				batch := make([]opsErrorLogJob, 0, opsErrorLogBatchSize)
				batch = append(batch, job)

				timer := time.NewTimer(opsErrorLogBatchWindow)
			batchLoop:
				for len(batch) < opsErrorLogBatchSize {
					select {
					case nextJob, ok := <-opsErrorLogQueue:
						if !ok {
							if !timer.Stop() {
								select {
								case <-timer.C:
								default:
								}
							}
							flushOpsErrorLogBatch(batch)
							return
						}
						opsErrorLogQueueLen.Add(-1)
						opsErrorLogQueueBytes.Add(-nextJob.queuedBytes)
						batch = append(batch, nextJob)
					case <-timer.C:
						break batchLoop
					}
				}
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				flushOpsErrorLogBatch(batch)
			}
		}()
	}
}

func flushOpsErrorLogBatch(batch []opsErrorLogJob) {
	if len(batch) == 0 {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[OpsErrorLogger] worker panic: %v\n%s", r, debug.Stack())
		}
	}()

	grouped := make(map[*service.OpsService][]*service.OpsInsertErrorLogInput, len(batch))
	var processed int64
	for _, job := range batch {
		if job.ops == nil || job.entry == nil {
			continue
		}
		grouped[job.ops] = append(grouped[job.ops], job.entry)
		processed++
	}
	if processed == 0 {
		return
	}

	for opsSvc, entries := range grouped {
		if opsSvc == nil || len(entries) == 0 {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), opsErrorLogTimeout)
		_ = opsSvc.RecordErrorBatch(ctx, entries)
		cancel()
	}
	opsErrorLogProcessed.Add(processed)
}

func enqueueOpsErrorLog(ops *service.OpsService, entry *service.OpsInsertErrorLogInput) {
	if ops == nil || entry == nil {
		return
	}
	entry.UserAgent = normalizeOpsPersistentUserAgent(entry.UserAgent)
	if entry.ErrorBody != "" {
		originalBody := entry.ErrorBody
		body, truncated := service.SanitizeOpsErrorBodyForQueue(originalBody)
		entry.ErrorBody = body
		if truncated || body != originalBody {
			opsErrorLogSanitized.Add(1)
		}
	}
	if err := service.SanitizeOpsUpstreamErrorsForQueue(entry); err != nil {
		opsErrorLogDropped.Add(1)
		maybeLogOpsErrorLogDrop()
		return
	}
	select {
	case <-opsErrorLogShutdownCh:
		return
	default:
	}

	opsErrorLogMu.RLock()
	stopping := opsErrorLogStopping
	opsErrorLogMu.RUnlock()
	if stopping {
		return
	}

	opsErrorLogOnce.Do(startOpsErrorLogWorkers)

	opsErrorLogMu.RLock()
	defer opsErrorLogMu.RUnlock()
	if opsErrorLogStopping || opsErrorLogQueue == nil {
		return
	}
	queuedBytes := estimateOpsErrorLogJobBytes(entry)
	if !reserveOpsErrorLogQueueBytes(queuedBytes) {
		opsErrorLogDropped.Add(1)
		maybeLogOpsErrorLogDrop()
		return
	}

	select {
	case opsErrorLogQueue <- opsErrorLogJob{ops: ops, entry: entry, queuedBytes: queuedBytes}:
		opsErrorLogEnqueued.Add(1)
	default:
		opsErrorLogQueueLen.Add(-1)
		opsErrorLogQueueBytes.Add(-queuedBytes)
		// Queue is full; drop to avoid blocking request handling.
		opsErrorLogDropped.Add(1)
		maybeLogOpsErrorLogDrop()
	}
}

func normalizeOpsPersistentUserAgent(value string) string {
	return truncateString(strings.TrimSpace(strings.ToValidUTF8(value, "")), opsErrorLogMaxUserAgentBytes)
}

func StopOpsErrorLogWorkers() bool {
	opsErrorLogStopOnce.Do(func() {
		opsErrorLogShutdownOnce.Do(func() {
			close(opsErrorLogShutdownCh)
		})
		opsErrorLogDrained.Store(stopOpsErrorLogWorkers())
	})
	return opsErrorLogDrained.Load()
}

func stopOpsErrorLogWorkers() bool {
	opsErrorLogMu.Lock()
	opsErrorLogStopping = true
	ch := opsErrorLogQueue
	if ch != nil {
		close(ch)
	}
	opsErrorLogQueue = nil
	opsErrorLogMu.Unlock()

	if ch == nil {
		opsErrorLogQueueLen.Store(0)
		opsErrorLogQueueBytes.Store(0)
		return true
	}

	done := make(chan struct{})
	go func() {
		opsErrorLogWorkersWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		opsErrorLogQueueLen.Store(0)
		opsErrorLogQueueBytes.Store(0)
		return true
	case <-time.After(opsErrorLogDrainTimeout):
		return false
	}
}

func OpsErrorLogQueueLength() int64 {
	return opsErrorLogQueueLen.Load()
}

func OpsErrorLogQueueBytes() int64 {
	return opsErrorLogQueueBytes.Load()
}

func OpsErrorLogQueueBytesCapacity() int64 {
	return opsErrorLogMaxQueueBytes
}

func OpsErrorLogQueueCapacity() int {
	opsErrorLogMu.RLock()
	ch := opsErrorLogQueue
	opsErrorLogMu.RUnlock()
	if ch == nil {
		return 0
	}
	return cap(ch)
}

func OpsErrorLogDroppedTotal() int64 {
	return opsErrorLogDropped.Load()
}

func OpsErrorLogEnqueuedTotal() int64 {
	return opsErrorLogEnqueued.Load()
}

func OpsErrorLogProcessedTotal() int64 {
	return opsErrorLogProcessed.Load()
}

func OpsErrorLogSanitizedTotal() int64 {
	return opsErrorLogSanitized.Load()
}

func maybeLogOpsErrorLogDrop() {
	now := time.Now().Unix()

	for {
		last := opsErrorLogLastDropLogAt.Load()
		if last != 0 && now-last < 60 {
			return
		}
		if opsErrorLogLastDropLogAt.CompareAndSwap(last, now) {
			break
		}
	}

	queued := opsErrorLogQueueLen.Load()
	queuedBytes := opsErrorLogQueueBytes.Load()
	queueCap := OpsErrorLogQueueCapacity()

	log.Printf(
		"[OpsErrorLogger] queue is full; dropping logs (queued=%d cap=%d queued_bytes=%d bytes_cap=%d enqueued_total=%d dropped_total=%d processed_total=%d sanitized_total=%d)",
		queued,
		queueCap,
		queuedBytes,
		opsErrorLogMaxQueueBytes,
		opsErrorLogEnqueued.Load(),
		opsErrorLogDropped.Load(),
		opsErrorLogProcessed.Load(),
		opsErrorLogSanitized.Load(),
	)
}

func reserveOpsErrorLogQueueBytes(size int64) bool {
	if size < 1 {
		size = 1
	}
	for {
		current := opsErrorLogQueueBytes.Load()
		if current > opsErrorLogMaxQueueBytes-size {
			return false
		}
		if opsErrorLogQueueBytes.CompareAndSwap(current, current+size) {
			opsErrorLogQueueLen.Add(1)
			return true
		}
	}
}

func estimateOpsErrorLogJobBytes(entry *service.OpsInsertErrorLogInput) int64 {
	if entry == nil {
		return 1
	}
	const fixedOverhead = 512
	size := fixedOverhead + len(entry.RequestID) + len(entry.ClientRequestID) +
		len(entry.Platform) + len(entry.Model) + len(entry.RequestPath) +
		len(entry.InboundEndpoint) + len(entry.UpstreamEndpoint) +
		len(entry.RequestedModel) + len(entry.UpstreamModel) + len(entry.UserAgent) +
		len(entry.ErrorPhase) + len(entry.ErrorType) + len(entry.Severity) +
		len(entry.ErrorMessage) + len(entry.ErrorBody) + len(entry.ErrorSource) +
		len(entry.ErrorOwner) + len(entry.APIKeyPrefix)
	if entry.UpstreamErrorMessage != nil {
		size += len(*entry.UpstreamErrorMessage)
	}
	if entry.UpstreamErrorDetail != nil {
		size += len(*entry.UpstreamErrorDetail)
	}
	if entry.UpstreamErrorsJSON != nil {
		size += len(*entry.UpstreamErrorsJSON)
	}
	return int64(size)
}

func opsErrorLogConfig() (workerCount int, queueSize int) {
	workerCount = runtime.GOMAXPROCS(0) * 2
	if workerCount < opsErrorLogMinWorkerCount {
		workerCount = opsErrorLogMinWorkerCount
	}
	if workerCount > opsErrorLogMaxWorkerCount {
		workerCount = opsErrorLogMaxWorkerCount
	}

	queueSize = workerCount * opsErrorLogQueueSizePerWorker
	if queueSize < opsErrorLogMinQueueSize {
		queueSize = opsErrorLogMinQueueSize
	}
	if queueSize > opsErrorLogMaxQueueSize {
		queueSize = opsErrorLogMaxQueueSize
	}

	return workerCount, queueSize
}

func setOpsRequestContext(c *gin.Context, model string, stream bool) {
	if c == nil {
		return
	}
	model = strings.TrimSpace(model)
	c.Set(opsModelKey, model)
	c.Set(opsStreamKey, stream)
	if c.Request != nil && model != "" {
		ctx := context.WithValue(c.Request.Context(), ctxkey.Model, model)
		c.Request = c.Request.WithContext(ctx)
	}
}

// setOpsEndpointContext stores upstream model and request type for ops error logging.
// Called by handlers after model mapping and request type determination.
func setOpsEndpointContext(c *gin.Context, upstreamModel string, requestType int16) {
	if c == nil {
		return
	}
	if upstreamModel = strings.TrimSpace(upstreamModel); upstreamModel != "" {
		c.Set(opsUpstreamModelKey, upstreamModel)
	}
	c.Set(opsRequestTypeKey, requestType)
}

func setOpsSelectedAccount(c *gin.Context, accountID int64, platform ...string) {
	if c == nil || accountID <= 0 {
		return
	}
	service.ClearOpsUpstreamModel(c)
	c.Set(opsAccountIDKey, accountID)
	if c.Request != nil {
		ctx := context.WithValue(c.Request.Context(), ctxkey.AccountID, accountID)
		if len(platform) > 0 {
			p := strings.TrimSpace(platform[0])
			if p != "" {
				ctx = context.WithValue(ctx, ctxkey.Platform, p)
			}
		}
		c.Request = c.Request.WithContext(ctx)
	}
}

func markOpsRoutingCapacityLimited(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(opsRoutingCapacityLimitedKey, true)
}

func markOpsRoutingCapacityLimitedIfNoAvailable(c *gin.Context, err error) {
	if !isOpsNoAvailableAccountError(err) {
		return
	}
	markOpsRoutingCapacityLimited(c)
}

func isOpsRoutingCapacityLimited(c *gin.Context) bool {
	if c == nil {
		return false
	}
	v, ok := c.Get(opsRoutingCapacityLimitedKey)
	if !ok {
		return false
	}
	marked, _ := v.(bool)
	return marked
}

func isOpsNoAvailableAccountError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, service.ErrNoAvailableAccounts) || errors.Is(err, service.ErrNoAvailableCompactAccounts) {
		return true
	}
	return isOpsNoAvailableAccountMessage(err.Error())
}

type opsCaptureWriter struct {
	// Handles are never pooled. A generation binds each handle to exactly one
	// pooled state lease, so a stale handle cannot reach a later request.
	state      *opsCaptureWriterState
	generation uint64
	pool       opsCaptureWriterStatePool
}

type opsCaptureWriterState struct {
	mu             sync.RWMutex
	inFlight       sync.WaitGroup
	generation     uint64
	responseWriter gin.ResponseWriter
	limit          int
	buf            bytes.Buffer
	probe          []byte
	lineProbe      []byte
	frameLineLen   int
	frameTruncated bool
	lineTruncated  bool
	skipLF         bool
	sseCapturing   bool
	terminalError  parsedOpsError
	terminalFound  bool
	ctx            *gin.Context
}

const (
	opsCaptureWriterLimit         = service.OpsErrorLogQueueBodyMaxBytes
	opsTerminalSSEFrameProbeLimit = 16 * 1024
)

const opsCaptureWriterPoolMaxRetainedCapacity = service.OpsErrorLogQueueBodyMaxBytes

type opsCaptureWriterStatePool interface {
	Get() any
	Put(any)
}

var opsCaptureWriterPool opsCaptureWriterStatePool = &sync.Pool{
	New: func() any {
		return &opsCaptureWriterState{limit: opsCaptureWriterLimit}
	},
}

func acquireOpsCaptureWriter(rw gin.ResponseWriter) *opsCaptureWriter {
	return acquireOpsCaptureWriterFromPool(opsCaptureWriterPool, rw)
}

func acquireOpsCaptureWriterFromPool(pool opsCaptureWriterStatePool, rw gin.ResponseWriter) *opsCaptureWriter {
	var pooled any
	if pool != nil {
		pooled = pool.Get()
	}
	state, ok := pooled.(*opsCaptureWriterState)
	if !ok || state == nil {
		state = &opsCaptureWriterState{}
	}
	state.mu.Lock()
	state.generation++
	state.responseWriter = rw
	state.limit = opsCaptureWriterLimit
	state.buf.Reset()
	state.probe = state.probe[:0]
	state.lineProbe = state.lineProbe[:0]
	state.frameLineLen = 0
	state.frameTruncated = false
	state.lineTruncated = false
	state.skipLF = false
	state.sseCapturing = false
	state.terminalError = parsedOpsError{}
	state.terminalFound = false
	state.ctx = nil
	generation := state.generation
	state.mu.Unlock()
	return &opsCaptureWriter{state: state, generation: generation, pool: pool}
}

func releaseOpsCaptureWriter(w *opsCaptureWriter) {
	if w == nil || w.state == nil {
		return
	}
	state := w.state
	state.mu.Lock()
	if state.generation != w.generation {
		state.mu.Unlock()
		return
	}
	// Invalidate the lease before waiting. No new delegated calls can start for
	// this handle, while calls that already copied the writer keep it alive via
	// inFlight until their network operation returns.
	state.generation++
	state.responseWriter = nil
	state.ctx = nil
	state.mu.Unlock()
	state.inFlight.Wait()
	state.mu.Lock()
	state.limit = opsCaptureWriterLimit
	state.probe = state.probe[:0]
	state.lineProbe = state.lineProbe[:0]
	state.frameLineLen = 0
	state.frameTruncated = false
	state.lineTruncated = false
	state.skipLF = false
	state.sseCapturing = false
	state.terminalError = parsedOpsError{}
	state.terminalFound = false
	poolable := shouldPoolOpsCaptureWriterState(state)
	state.buf.Reset()
	state.mu.Unlock()
	if poolable && w.pool != nil {
		w.pool.Put(state)
	}
}

func shouldPoolOpsCaptureWriterState(state *opsCaptureWriterState) bool {
	return state != nil && state.buf.Cap() <= opsCaptureWriterPoolMaxRetainedCapacity &&
		cap(state.probe) <= opsTerminalSSEFrameProbeLimit && cap(state.lineProbe) <= 256
}

func (w *opsCaptureWriter) lockActive() (*opsCaptureWriterState, gin.ResponseWriter) {
	if w == nil || w.state == nil {
		return nil, nil
	}
	state := w.state
	state.mu.RLock()
	if state.generation != w.generation || state.responseWriter == nil {
		state.mu.RUnlock()
		return nil, nil
	}
	return state, state.responseWriter
}

func (w *opsCaptureWriter) lockActiveWrite() (*opsCaptureWriterState, gin.ResponseWriter) {
	if w == nil || w.state == nil {
		return nil, nil
	}
	state := w.state
	state.mu.Lock()
	if state.generation != w.generation || state.responseWriter == nil {
		state.mu.Unlock()
		return nil, nil
	}
	return state, state.responseWriter
}

func (w *opsCaptureWriter) beginDelegatedCall() (*opsCaptureWriterState, gin.ResponseWriter) {
	if w == nil || w.state == nil {
		return nil, nil
	}
	state := w.state
	state.mu.Lock()
	if state.generation != w.generation || state.responseWriter == nil {
		state.mu.Unlock()
		return nil, nil
	}
	rw := state.responseWriter
	state.inFlight.Add(1)
	return state, rw
}

func finishDelegatedCall(state *opsCaptureWriterState) {
	if state != nil {
		state.inFlight.Done()
	}
}

func (w *opsCaptureWriter) setContext(ctx *gin.Context) {
	state, _ := w.lockActiveWrite()
	if state == nil {
		return
	}
	state.ctx = ctx
	state.mu.Unlock()
}

func (w *opsCaptureWriter) capturedBytes() []byte {
	state, _ := w.lockActive()
	if state == nil {
		return nil
	}
	defer state.mu.RUnlock()
	return append([]byte(nil), state.buf.Bytes()...)
}

func (w *opsCaptureWriter) capturedTerminalError() (parsedOpsError, bool) {
	state, _ := w.lockActive()
	if state == nil {
		return parsedOpsError{}, false
	}
	defer state.mu.RUnlock()
	return state.terminalError, state.terminalFound
}

func (w *opsCaptureWriter) finalizeCapture() {
	state, _ := w.lockActiveWrite()
	if state == nil {
		return
	}
	defer state.mu.Unlock()
	state.finalizeResponseCapture()
}

func (w *opsCaptureWriter) Header() http.Header {
	state, rw := w.lockActive()
	if state == nil {
		return http.Header{}
	}
	defer state.mu.RUnlock()
	return rw.Header()
}
func (w *opsCaptureWriter) WriteHeader(code int) {
	state, rw := w.beginDelegatedCall()
	if state == nil {
		return
	}
	state.mu.Unlock()
	defer finishDelegatedCall(state)
	rw.WriteHeader(code)
}
func (w *opsCaptureWriter) WriteHeaderNow() {
	state, rw := w.beginDelegatedCall()
	if state == nil {
		return
	}
	state.mu.Unlock()
	defer finishDelegatedCall(state)
	rw.WriteHeaderNow()
}
func (w *opsCaptureWriter) Status() int {
	state, rw := w.lockActive()
	if state == nil {
		return 0
	}
	defer state.mu.RUnlock()
	return rw.Status()
}
func (w *opsCaptureWriter) Size() int {
	state, rw := w.lockActive()
	if state == nil {
		return -1
	}
	defer state.mu.RUnlock()
	return rw.Size()
}
func (w *opsCaptureWriter) Written() bool {
	state, rw := w.lockActive()
	if state == nil {
		return false
	}
	defer state.mu.RUnlock()
	return rw.Written()
}
func (w *opsCaptureWriter) Flush() {
	state, rw := w.beginDelegatedCall()
	if state == nil {
		return
	}
	state.mu.Unlock()
	defer finishDelegatedCall(state)
	rw.Flush()
}
func (w *opsCaptureWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	state, rw := w.beginDelegatedCall()
	if state == nil {
		return nil, nil, errors.New("response writer released")
	}
	state.mu.Unlock()
	defer finishDelegatedCall(state)
	return rw.Hijack()
}
func (w *opsCaptureWriter) CloseNotify() <-chan bool {
	state, rw := w.lockActive()
	if state == nil {
		ch := make(chan bool)
		close(ch)
		return ch
	}
	defer state.mu.RUnlock()
	return rw.CloseNotify()
}
func (w *opsCaptureWriter) Pusher() http.Pusher {
	state, rw := w.lockActive()
	if state == nil {
		return nil
	}
	defer state.mu.RUnlock()
	return rw.Pusher()
}

func (w *opsCaptureWriter) Write(b []byte) (int, error) {
	state, rw := w.beginDelegatedCall()
	if state == nil {
		return 0, nil
	}
	if state.shouldCapture() {
		state.captureResponseChunk(b, rw.Status())
	}
	state.mu.Unlock()
	defer finishDelegatedCall(state)
	return rw.Write(b)
}

func (w *opsCaptureWriter) WriteString(s string) (int, error) {
	state, rw := w.beginDelegatedCall()
	if state == nil {
		return 0, nil
	}
	if state.shouldCapture() {
		state.captureResponseChunk([]byte(s), rw.Status())
	}
	state.mu.Unlock()
	defer finishDelegatedCall(state)
	return rw.WriteString(s)
}

var _ gin.ResponseWriter = (*opsCaptureWriter)(nil)

func isOpsTerminalSSEFrame(frame []byte) bool {
	eventType, payload := parseOpsSSEFrameEnvelope(frame)
	if bytes.Equal(eventType, []byte("response.failed")) || bytes.Equal(eventType, []byte("error")) {
		return true
	}
	if len(payload) == 0 {
		return false
	}
	// Most successful frames cannot be terminal. Avoid JSON decoding on this
	// hot path while still validating any plausible terminal payload below.
	if !bytes.Contains(payload, []byte("response.failed")) && !bytes.Contains(payload, []byte(`"error"`)) {
		return false
	}
	var event struct {
		Type string `json:"type"`
	}
	return json.Unmarshal(payload, &event) == nil &&
		(event.Type == "response.failed" || event.Type == "error")
}

func parseOpsSSEFrameEnvelope(frame []byte) ([]byte, []byte) {
	var eventType []byte
	var data []byte
	dataOwned := false
	dataSeen := false
	for len(frame) > 0 {
		line := frame
		lf := bytes.IndexByte(frame, '\n')
		cr := bytes.IndexByte(frame, '\r')
		idx := lf
		if idx < 0 || (cr >= 0 && cr < idx) {
			idx = cr
		}
		if idx >= 0 {
			line = frame[:idx]
			consume := idx + 1
			if frame[idx] == '\r' && consume < len(frame) && frame[consume] == '\n' {
				consume++
			}
			frame = frame[consume:]
		} else {
			frame = nil
		}
		if len(line) == 0 || line[0] == ':' {
			continue
		}
		field, value, found := bytes.Cut(line, []byte{':'})
		if !found {
			value = nil
		}
		field = bytes.TrimSpace(field)
		value = bytes.TrimSpace(value)
		switch {
		case bytes.Equal(field, []byte("event")):
			eventType = value
		case bytes.Equal(field, []byte("data")):
			if !dataSeen {
				data = value
				dataSeen = true
				continue
			}
			if !dataOwned {
				data = append([]byte(nil), data...)
				dataOwned = true
			}
			data = append(data, '\n')
			data = append(data, value...)
		}
	}
	return bytes.TrimSpace(eventType), data
}

func (state *opsCaptureWriterState) captureResponseChunk(chunk []byte, status int) {
	if state == nil || state.limit <= 0 || len(chunk) == 0 {
		return
	}
	if status >= 400 {
		state.appendCapturedResponse(chunk)
		return
	}
	if state.sseCapturing {
		state.appendTerminalProbe(chunk)
		state.appendCapturedResponse(chunk)
		return
	}
	// Most stream writes contain one or more complete successful SSE frames.
	// Skip the byte-wise frame parser when the chunk cannot contain a terminal
	// event and leaves no split frame to carry into the next write.
	if len(state.probe) == 0 && len(state.lineProbe) == 0 && endsAtOpsSSEFrameBoundary(chunk) &&
		!mayContainOpsTerminalSSE(chunk) {
		return
	}
	for i, b := range chunk {
		if state.skipLF {
			state.skipLF = false
			if b == '\n' {
				continue
			}
		}
		if !state.lineTruncated {
			if len(state.lineProbe) < 256 {
				state.lineProbe = append(state.lineProbe, b)
			} else {
				state.lineTruncated = true
			}
		}
		if !state.frameTruncated {
			if len(state.probe) < opsTerminalSSEFrameProbeLimit {
				state.probe = append(state.probe, b)
			} else {
				state.frameTruncated = true
			}
		}
		if b != '\n' && b != '\r' {
			state.frameLineLen++
			continue
		}
		if b == '\r' {
			state.skipLF = true
		}
		if !state.lineTruncated && isOpsTerminalSSEEventLine(state.lineProbe) {
			state.sseCapturing = true
			state.terminalError = parsedOpsError{ErrorType: "upstream_error", StreamFailure: true}
			state.terminalFound = true
			state.appendCapturedResponse(state.lineProbe)
			state.probe = state.probe[:0]
			state.appendTerminalProbe(state.lineProbe)
			state.lineProbe = state.lineProbe[:0]
			state.frameLineLen = 0
			state.frameTruncated = false
			state.lineTruncated = false
			state.skipLF = false
			state.appendTerminalProbe(chunk[i+1:])
			state.appendCapturedResponse(chunk[i+1:])
			return
		}
		if state.frameLineLen != 0 {
			state.frameLineLen = 0
			state.lineProbe = state.lineProbe[:0]
			state.lineTruncated = false
			continue
		}
		if !state.frameTruncated && isOpsTerminalSSEFrame(state.probe) {
			state.sseCapturing = true
			state.terminalError, state.terminalFound = parseOpsSSEFailure(state.probe)
			state.appendCapturedResponse(state.probe)
			state.probe = state.probe[:0]
			state.appendCapturedResponse(chunk[i+1:])
			return
		}
		state.probe = state.probe[:0]
		state.lineProbe = state.lineProbe[:0]
		state.frameTruncated = false
		state.lineTruncated = false
	}
}

func endsAtOpsSSEFrameBoundary(chunk []byte) bool {
	return bytes.HasSuffix(chunk, []byte("\n\n")) ||
		bytes.HasSuffix(chunk, []byte("\r\n\r\n")) ||
		bytes.HasSuffix(chunk, []byte("\r\r"))
}

func mayContainOpsTerminalSSE(chunk []byte) bool {
	if bytes.Contains(chunk, []byte("response.failed")) {
		return true
	}
	return bytes.Contains(chunk, []byte("error")) &&
		(bytes.Contains(chunk, []byte("event")) || bytes.Contains(chunk, []byte(`"type"`)))
}

func isOpsTerminalSSEEventLine(line []byte) bool {
	line = bytes.TrimSpace(line)
	field, value, found := bytes.Cut(line, []byte{':'})
	return found && bytes.Equal(bytes.TrimSpace(field), []byte("event")) &&
		(bytes.Equal(bytes.TrimSpace(value), []byte("response.failed")) ||
			bytes.Equal(bytes.TrimSpace(value), []byte("error")))
}

func (state *opsCaptureWriterState) appendTerminalProbe(chunk []byte) {
	remaining := opsTerminalSSEFrameProbeLimit - len(state.probe)
	if remaining <= 0 {
		state.frameTruncated = true
		return
	}
	if len(chunk) > remaining {
		chunk = chunk[:remaining]
		state.frameTruncated = true
	}
	state.probe = append(state.probe, chunk...)
}

func (state *opsCaptureWriterState) finalizeResponseCapture() {
	if state == nil {
		return
	}
	if state.terminalFound {
		if parsed, ok := parseOpsSSEFailure(state.probe); ok {
			state.terminalError = parsed
		}
		return
	}
	if state.frameTruncated || len(state.probe) == 0 || !isOpsTerminalSSEFrame(state.probe) {
		return
	}
	state.sseCapturing = true
	state.appendCapturedResponse(state.probe)
	state.terminalError, state.terminalFound = parseOpsSSEFailure(state.probe)
	if !state.terminalFound {
		state.terminalError = parsedOpsError{ErrorType: "upstream_error", StreamFailure: true}
		state.terminalFound = true
	}
}

func (state *opsCaptureWriterState) appendCapturedResponse(chunk []byte) {
	remaining := state.limit - state.buf.Len()
	if remaining <= 0 {
		return
	}
	if len(chunk) > remaining {
		chunk = chunk[:remaining]
	}
	_, _ = state.buf.Write(chunk)
}

func (state *opsCaptureWriterState) shouldCapture() bool {
	if state.ctx == nil {
		return true
	}
	_, rejected := middleware2.GetIngressRejectReason(state.ctx)
	return !rejected
}

// OpsErrorLoggerMiddleware records error responses (status >= 400) into ops_error_logs.
//
// Notes:
// - It buffers response bodies only for status >= 400 or terminal SSE frames.
// - Streaming errors after the response has started (SSE) may still need explicit logging.
func OpsErrorLoggerMiddleware(ops *service.OpsService) gin.HandlerFunc {
	return func(c *gin.Context) {
		originalWriter := c.Writer
		w := acquireOpsCaptureWriter(originalWriter)
		w.setContext(c)
		defer func() {
			// Restore the original writer before returning so outer middlewares
			// don't observe a pooled wrapper that has been released.
			if c.Writer == w {
				c.Writer = originalWriter
			}
			releaseOpsCaptureWriter(w)
		}()
		c.Writer = w
		c.Next()
		w.finalizeCapture()

		if _, rejected := middleware2.GetIngressRejectReason(c); rejected {
			return
		}

		if ops == nil {
			return
		}
		if !ops.IsMonitoringEnabled(c.Request.Context()) {
			return
		}
		if c.GetBool(opsDedicatedErrorRecordedKey) {
			return
		}

		if shouldSkipOpsErrorLogForCyber(c) {
			return
		}

		status := c.Writer.Status()
		body := w.capturedBytes()
		parsed := parseOpsErrorResponse(body)
		if !parsed.StreamFailure {
			if terminal, ok := w.capturedTerminalError(); ok {
				parsed = terminal
			}
		}
		if status < 400 {
			if parsed.StreamFailure {
				status = inferStreamFailureStatus(c, parsed)
			} else {
				// A marked in-band error is a visible request failure even though its
				// wire status is already 200. Otherwise retain recovered attempts as a
				// provider-health row whose 2xx status keeps it outside request SLA.
				if len(service.GetOpsStreamErrors(c)) > 0 {
					logOpsStreamError(c, ops, status)
				} else {
					logOpsRecoveredUpstream(c, ops, status)
				}
				return
			}
		}

		// Skip logging if a passthrough rule with skip_monitoring=true matched.
		if shouldSkipFinalOpsFailure(c) {
			return
		}

		// Skip logging if the error should be filtered based on settings
		if shouldSkipOpsErrorLog(c.Request.Context(), ops, parsed.Message, string(body), c.Request.URL.Path) {
			return
		}

		apiKey := getOpsAPIKey(c)

		clientRequestID, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string)

		model, _ := c.Get(opsModelKey)
		streamV, _ := c.Get(opsStreamKey)
		accountIDV, _ := c.Get(opsAccountIDKey)

		var modelName string
		if s, ok := model.(string); ok {
			modelName = s
		}
		stream := false
		if b, ok := streamV.(bool); ok {
			stream = b
		}
		var accountID *int64
		if v, ok := accountIDV.(int64); ok && v > 0 {
			accountID = &v
		}

		fallbackPlatform := guessPlatformFromPath(c.Request.URL.Path)
		platform := resolveOpsPlatform(c.Request.Context(), apiKey, fallbackPlatform)

		requestID, _ := c.Request.Context().Value(ctxkey.RequestID).(string)
		requestID = strings.TrimSpace(requestID)
		if requestID == "" {
			requestID = c.Writer.Header().Get("X-Request-Id")
			if requestID == "" {
				requestID = c.Writer.Header().Get("x-request-id")
			}
		}

		normalizedType := normalizeOpsErrorType(parsed.ErrorType, parsed.Code)

		phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(c, normalizedType, parsed.Message, parsed.Code, status)

		entry := &service.OpsInsertErrorLogInput{
			RequestID:       requestID,
			ClientRequestID: clientRequestID,

			AccountID: accountID,
			Platform:  platform,
			Model:     modelName,
			RequestPath: func() string {
				if c.Request != nil && c.Request.URL != nil {
					return c.Request.URL.Path
				}
				return ""
			}(),
			Stream:           stream,
			InboundEndpoint:  GetInboundEndpoint(c),
			UpstreamEndpoint: GetUpstreamEndpoint(c, platform),
			RequestedModel:   modelName,
			UpstreamModel: func() string {
				if v, ok := c.Get(opsUpstreamModelKey); ok {
					if s, ok := v.(string); ok {
						return strings.TrimSpace(s)
					}
				}
				return ""
			}(),
			RequestType: func() *int16 {
				if v, ok := c.Get(opsRequestTypeKey); ok {
					switch t := v.(type) {
					case int16:
						return &t
					case int:
						v16 := int16(t)
						return &v16
					}
				}
				return nil
			}(),
			UserAgent: c.GetHeader("User-Agent"),

			ErrorPhase:        phase,
			ErrorType:         normalizedType,
			Severity:          classifyOpsSeverity(normalizedType, status),
			StatusCode:        status,
			IsBusinessLimited: isBusinessLimited,
			IsCountTokens:     isCountTokensRequest(c),

			ErrorMessage: parsed.Message,
			// Sanitize each SSE data payload before the body enters the async queue.
			ErrorBody:   sanitizeOpsSSEDataForPersistence(body),
			ErrorSource: errorSource,
			ErrorOwner:  errorOwner,

			CreatedAt: time.Now(),
		}
		applyOpsLatencyFieldsFromContext(c, entry)
		applyOpsUpstreamFieldsFromContext(c, entry)
		if parsed.StreamFailure {
			if message := strings.TrimSpace(parsed.Message); message != "" {
				entry.UpstreamErrorMessage = &message
			}
			if status >= 400 {
				finalStatus := status
				entry.UpstreamStatusCode = &finalStatus
			}
		}
		suppressOpsUpstreamAttributionForLocalModelConfiguration(c, entry)

		if apiKey != nil {
			entry.APIKeyID = &apiKey.ID
			// 有效 key 报错时快照前缀，key 之后被删也保留。
			entry.APIKeyPrefix = keyPrefix(apiKey.Key, 8)
			if apiKey.User != nil {
				entry.UserID = &apiKey.User.ID
			}
			if apiKey.GroupID != nil {
				entry.GroupID = apiKey.GroupID
			}
			// Prefer group platform if present (more stable than inferring from path).
			if apiKey.Group != nil && apiKey.Group.Platform != "" {
				entry.Platform = apiKey.Group.Platform
			}
		}

		var clientIP string
		if ip := strings.TrimSpace(ip.GetClientIP(c)); ip != "" {
			clientIP = ip
			entry.ClientIP = &clientIP
		}

		enqueueOpsErrorLog(ops, entry)
	}
}

func logOpsRecoveredUpstream(c *gin.Context, ops *service.OpsService, finalStatus int) {
	if c == nil || ops == nil || finalStatus >= 400 {
		return
	}

	entry := &service.OpsInsertErrorLogInput{StatusCode: finalStatus}
	applyOpsUpstreamFieldsFromContext(c, entry)
	if len(entry.UpstreamErrors) > 0 {
		visibleEvents := make([]*service.OpsUpstreamErrorEvent, 0, len(entry.UpstreamErrors))
		for _, event := range entry.UpstreamErrors {
			if event != nil && !event.SkipMonitoring {
				visibleEvents = append(visibleEvents, event)
			}
		}
		if len(visibleEvents) == 0 {
			return
		}
		applyOpsUpstreamErrorEvents(entry, visibleEvents)
	}
	if entry.UpstreamStatusCode == nil && entry.UpstreamErrorMessage == nil &&
		entry.UpstreamErrorDetail == nil && len(entry.UpstreamErrors) == 0 {
		return
	}

	lastStatus := 0
	if entry.UpstreamStatusCode != nil {
		lastStatus = *entry.UpstreamStatusCode
	}
	lastStage := ""
	for i := len(entry.UpstreamErrors) - 1; i >= 0; i-- {
		if event := entry.UpstreamErrors[i]; event != nil {
			lastStage = event.Stage
			if event.AccountID > 0 {
				accountID := event.AccountID
				entry.AccountID = &accountID
			}
			break
		}
	}
	if entry.AccountID == nil {
		if accountID, ok := c.Get(opsAccountIDKey); ok {
			if value, ok := accountID.(int64); ok && value > 0 {
				entry.AccountID = &value
			}
		}
	}

	entry.ErrorPhase = "upstream"
	entry.ErrorType = "upstream_error"
	entry.ErrorSource = "upstream_http"
	entry.ErrorOwner = "provider"
	entry.Severity = classifyOpsSeverity(entry.ErrorType, lastStatus)
	entry.IsCountTokens = isCountTokensRequest(c)
	entry.CreatedAt = time.Now()
	entry.ErrorMessage = "Recovered upstream error"
	if lastStage == string(service.GatewayFailureStageAccountAuth) {
		entry.ErrorPhase = string(service.GatewayFailureStageAccountAuth)
		entry.ErrorMessage = "Recovered account authentication failure"
	} else if lastStatus > 0 {
		entry.ErrorMessage += " " + strconv.Itoa(lastStatus)
	}
	if entry.UpstreamErrorMessage != nil && strings.TrimSpace(*entry.UpstreamErrorMessage) != "" {
		entry.ErrorMessage += ": " + strings.TrimSpace(*entry.UpstreamErrorMessage)
	}
	entry.ErrorMessage = truncateString(entry.ErrorMessage, 2048)

	if c.Request != nil {
		entry.UserAgent = c.GetHeader("User-Agent")
		if c.Request.URL != nil {
			entry.RequestPath = c.Request.URL.Path
		}
		if c.Request.Context() != nil {
			entry.ClientRequestID, _ = c.Request.Context().Value(ctxkey.ClientRequestID).(string)
			entry.RequestID, _ = c.Request.Context().Value(ctxkey.RequestID).(string)
		}
	}
	entry.RequestID = strings.TrimSpace(entry.RequestID)
	if entry.RequestID == "" {
		entry.RequestID = c.Writer.Header().Get("X-Request-Id")
	}
	entry.Model = c.GetString(opsModelKey)
	entry.RequestedModel = entry.Model
	entry.Stream = c.GetBool(opsStreamKey)
	entry.InboundEndpoint = GetInboundEndpoint(c)
	entry.UpstreamModel = c.GetString(opsUpstreamModelKey)
	entry.RequestType = opsRequestTypeFromContext(c)

	apiKey := getOpsAPIKey(c)
	fallbackPlatform := guessPlatformFromPath(entry.RequestPath)
	requestContext := context.Background()
	if c.Request != nil {
		requestContext = c.Request.Context()
	}
	entry.Platform = resolveOpsPlatform(requestContext, apiKey, fallbackPlatform)
	entry.UpstreamEndpoint = GetUpstreamEndpoint(c, entry.Platform)
	if apiKey != nil {
		entry.APIKeyID = &apiKey.ID
		entry.APIKeyPrefix = keyPrefix(apiKey.Key, 8)
		if apiKey.User != nil {
			entry.UserID = &apiKey.User.ID
		}
		if apiKey.GroupID != nil {
			entry.GroupID = apiKey.GroupID
		}
		if apiKey.Group != nil && apiKey.Group.Platform != "" {
			entry.Platform = apiKey.Group.Platform
		}
	}
	if clientIP := strings.TrimSpace(ip.GetClientIP(c)); clientIP != "" {
		entry.ClientIP = &clientIP
	}
	applyOpsLatencyFieldsFromContext(c, entry)
	enqueueOpsErrorLog(ops, entry)
}

func opsRequestTypeFromContext(c *gin.Context) *int16 {
	if c == nil {
		return nil
	}
	if value, ok := c.Get(opsRequestTypeKey); ok {
		switch typed := value.(type) {
		case int16:
			result := typed
			return &result
		case int:
			result := int16(typed)
			return &result
		}
	}
	return nil
}

// logOpsStreamError 记录一次挂在已固化 HTTP 200 SSE 流上的就地错误。
// 由于 wire 状态码停留在 200，常规的 status>=400 捕获路径永远不会触发；
// handleStreamingAwareError 通过 service.MarkOpsStreamError 标记这类错误，
// 此函数据此补记一条错误日志，让并发限流/流内失败在错误看板里可见。
//
// 仅在 status<400 且不存在上游错误上下文时调用：上游透传错误已由中间件的
// upstream-context 分支落库，无需在此重复记录。
func logOpsStreamError(c *gin.Context, ops *service.OpsService, wireStatus int) {
	for _, streamErr := range service.GetOpsStreamErrors(c) {
		logOpsStreamErrorValue(c, ops, wireStatus, streamErr)
	}
}

func logOpsStreamErrorValue(c *gin.Context, ops *service.OpsService, wireStatus int, streamErr service.OpsStreamError) {
	// 命中 skip_monitoring=true 透传规则的请求跳过落库，与其它分支一致。
	if streamErr.SkipMonitoring || (streamErr.Turn == 0 && shouldSkipFinalOpsFailure(c)) {
		return
	}

	// 复用与 status>=400 分支相同的设置过滤（context canceled / 无可用账号等）。
	if shouldSkipOpsErrorLog(c.Request.Context(), ops, streamErr.Message, streamErr.Message, c.Request.URL.Path) {
		return
	}

	// 分级用「本应返回的状态码」(如并发限流 429)，wire 状态码缺省时回退。
	classifyStatus := streamErr.IntendedStatus
	if classifyStatus <= 0 {
		classifyStatus = wireStatus
	}
	normalizedType := normalizeOpsErrorType(streamErr.ErrType, streamErr.Code)
	phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(c, normalizedType, streamErr.Message, streamErr.Code, classifyStatus)
	recordedStatus := wireStatus
	if streamErr.CountTowardsSLA && streamErr.IntendedStatus >= 400 {
		recordedStatus = streamErr.IntendedStatus
	}
	errorBody := ""
	if streamErr.Code != "" {
		if payload, err := json.Marshal(gin.H{"error": gin.H{
			"type": normalizedType, "code": streamErr.Code, "message": streamErr.Message,
		}}); err == nil {
			errorBody = string(payload)
		}
	}

	apiKey := getOpsAPIKey(c)
	clientRequestID, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string)

	model, _ := c.Get(opsModelKey)
	var modelName string
	if s, ok := model.(string); ok {
		modelName = s
	}
	accountIDV, _ := c.Get(opsAccountIDKey)
	var accountID *int64
	if v, ok := accountIDV.(int64); ok && v > 0 {
		accountID = &v
	}

	fallbackPlatform := guessPlatformFromPath(c.Request.URL.Path)
	platform := resolveOpsPlatform(c.Request.Context(), apiKey, fallbackPlatform)

	requestID, _ := c.Request.Context().Value(ctxkey.RequestID).(string)
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		requestID = c.Writer.Header().Get("X-Request-Id")
		if requestID == "" {
			requestID = c.Writer.Header().Get("x-request-id")
		}
	}

	entry := &service.OpsInsertErrorLogInput{
		RequestID:       requestID,
		ClientRequestID: clientRequestID,

		AccountID: accountID,
		Platform:  platform,
		Model:     modelName,
		RequestPath: func() string {
			if c.Request != nil && c.Request.URL != nil {
				return c.Request.URL.Path
			}
			return ""
		}(),
		// 就地 SSE 错误只出现在流式请求上。
		Stream:           true,
		InboundEndpoint:  GetInboundEndpoint(c),
		UpstreamEndpoint: GetUpstreamEndpoint(c, platform),
		RequestedModel:   modelName,
		UpstreamModel: func() string {
			if v, ok := c.Get(opsUpstreamModelKey); ok {
				if s, ok := v.(string); ok {
					return strings.TrimSpace(s)
				}
			}
			return ""
		}(),
		RequestType: func() *int16 {
			if v, ok := c.Get(opsRequestTypeKey); ok {
				switch t := v.(type) {
				case int16:
					return &t
				case int:
					v16 := int16(t)
					return &v16
				}
			}
			return nil
		}(),
		UserAgent: c.GetHeader("User-Agent"),

		ErrorPhase:        phase,
		ErrorType:         normalizedType,
		Severity:          classifyOpsSeverity(normalizedType, classifyStatus),
		StatusCode:        recordedStatus,
		IsBusinessLimited: isBusinessLimited,
		IsCountTokens:     isCountTokensRequest(c),

		ErrorMessage: streamErr.Message,
		ErrorBody:    errorBody,
		ErrorSource:  errorSource,
		ErrorOwner:   errorOwner,

		CreatedAt: time.Now(),
	}
	applyOpsLatencyFieldsFromContext(c, entry)
	applyOpsUpstreamFieldsFromContext(c, entry)
	if streamErr.Turn > 0 {
		applyOpsStreamErrorSnapshot(entry, streamErr)
	}

	if apiKey != nil {
		entry.APIKeyID = &apiKey.ID
		entry.APIKeyPrefix = keyPrefix(apiKey.Key, 8)
		if apiKey.User != nil {
			entry.UserID = &apiKey.User.ID
		}
		if apiKey.GroupID != nil {
			entry.GroupID = apiKey.GroupID
		}
		if apiKey.Group != nil && apiKey.Group.Platform != "" {
			entry.Platform = apiKey.Group.Platform
		}
	}

	if clientIP := strings.TrimSpace(ip.GetClientIP(c)); clientIP != "" {
		entry.ClientIP = &clientIP
	}

	enqueueOpsErrorLog(ops, entry)
}

func applyOpsStreamErrorSnapshot(entry *service.OpsInsertErrorLogInput, streamErr service.OpsStreamError) {
	if entry == nil {
		return
	}
	if streamErr.AccountID > 0 {
		accountID := streamErr.AccountID
		entry.AccountID = &accountID
	}
	entry.UpstreamModel = strings.TrimSpace(streamErr.UpstreamModel)
	entry.UpstreamStatusCode = nil
	if streamErr.UpstreamStatus > 0 {
		status := streamErr.UpstreamStatus
		entry.UpstreamStatusCode = &status
	}
	entry.UpstreamErrorMessage = nil
	if message := strings.TrimSpace(streamErr.UpstreamMessage); message != "" {
		entry.UpstreamErrorMessage = &message
	}
	entry.UpstreamErrorDetail = nil
	if detail := strings.TrimSpace(streamErr.UpstreamDetail); detail != "" {
		entry.UpstreamErrorDetail = &detail
	}
	entry.UpstreamErrors = streamErr.UpstreamErrors
	lastStage := ""
	for i := len(streamErr.UpstreamErrors) - 1; i >= 0; i-- {
		if streamErr.UpstreamErrors[i] != nil {
			lastStage = streamErr.UpstreamErrors[i].Stage
			break
		}
	}
	if lastStage == string(service.GatewayFailureStageAccountAuth) {
		entry.ErrorPhase = string(service.GatewayFailureStageAccountAuth)
		entry.ErrorOwner = "provider"
		entry.ErrorSource = "gateway"
		entry.IsBusinessLimited = false
	} else if streamErr.UpstreamStatus > 0 || len(streamErr.UpstreamErrors) > 0 {
		entry.ErrorPhase = "upstream"
		entry.ErrorOwner = "provider"
		entry.ErrorSource = "upstream_http"
		entry.IsBusinessLimited = false
	}
}

func shouldSkipFinalOpsFailure(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if v, ok := c.Get(service.OpsSkipPassthroughKey); ok {
		if skip, _ := v.(bool); skip {
			return true
		}
	}
	if v, ok := c.Get(service.OpsUpstreamErrorsKey); ok {
		if events, ok := v.([]*service.OpsUpstreamErrorEvent); ok {
			for i := len(events) - 1; i >= 0; i-- {
				if events[i] != nil {
					return events[i].SkipMonitoring
				}
			}
		}
	}
	return false
}

// isCountTokensRequest checks if the request is a count_tokens request
func isCountTokensRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	return isTokenCountRequestPath(c.Request.URL.Path)
}

func isTokenCountRequestPath(path string) bool {
	return strings.Contains(path, "/count_tokens") || strings.Contains(path, "/responses/input_tokens")
}

func applyOpsLatencyFieldsFromContext(c *gin.Context, entry *service.OpsInsertErrorLogInput) {
	if c == nil || entry == nil {
		return
	}
	entry.AuthLatencyMs = getContextLatencyMs(c, service.OpsAuthLatencyMsKey)
	entry.RoutingLatencyMs = getContextLatencyMs(c, service.OpsRoutingLatencyMsKey)
	entry.UpstreamLatencyMs = getContextLatencyMs(c, service.OpsUpstreamLatencyMsKey)
	entry.ResponseLatencyMs = getContextLatencyMs(c, service.OpsResponseLatencyMsKey)
	entry.TimeToFirstTokenMs = getContextLatencyMs(c, service.OpsTimeToFirstTokenMsKey)
}

// applyOpsUpstreamFieldsFromContext captures attempt-level upstream context.
// A final account_auth event owns the top-level status and forces it to zero;
// prior inference statuses remain available in UpstreamErrors.
func applyOpsUpstreamFieldsFromContext(c *gin.Context, entry *service.OpsInsertErrorLogInput) {
	if c == nil || entry == nil {
		return
	}
	if v, ok := c.Get(service.OpsUpstreamStatusCodeKey); ok {
		switch t := v.(type) {
		case int:
			if t > 0 {
				code := t
				entry.UpstreamStatusCode = &code
			}
		case int64:
			if t > 0 {
				code := int(t)
				entry.UpstreamStatusCode = &code
			}
		}
	}
	if v, ok := c.Get(service.OpsUpstreamErrorMessageKey); ok {
		if value, ok := v.(string); ok {
			if message := strings.TrimSpace(value); message != "" {
				entry.UpstreamErrorMessage = &message
			}
		}
	}
	if v, ok := c.Get(service.OpsUpstreamErrorDetailKey); ok {
		if value, ok := v.(string); ok {
			if detail := strings.TrimSpace(value); detail != "" {
				entry.UpstreamErrorDetail = &detail
			}
		}
	}
	if v, ok := c.Get(service.OpsUpstreamErrorsKey); ok {
		if events, ok := v.([]*service.OpsUpstreamErrorEvent); ok && len(events) > 0 {
			applyOpsUpstreamErrorEvents(entry, events)
		}
	}
}

func applyOpsUpstreamErrorEvents(entry *service.OpsInsertErrorLogInput, events []*service.OpsUpstreamErrorEvent) {
	entry.UpstreamErrors = events
	var last *service.OpsUpstreamErrorEvent
	for i := len(events) - 1; i >= 0; i-- {
		if events[i] != nil {
			last = events[i]
			break
		}
	}
	if last == nil {
		return
	}

	entry.UpstreamStatusCode = nil
	entry.UpstreamErrorMessage = nil
	entry.UpstreamErrorDetail = nil
	if last.Stage == string(service.GatewayFailureStageAccountAuth) {
		code := 0
		entry.UpstreamStatusCode = &code
	} else if last.UpstreamStatusCode > 0 {
		code := last.UpstreamStatusCode
		entry.UpstreamStatusCode = &code
	}
	if message := strings.TrimSpace(last.Message); message != "" {
		entry.UpstreamErrorMessage = &message
	}
	if detail := strings.TrimSpace(last.Detail); detail != "" {
		entry.UpstreamErrorDetail = &detail
	}
}

func suppressOpsUpstreamAttributionForLocalModelConfiguration(c *gin.Context, entry *service.OpsInsertErrorLogInput) {
	if entry == nil || !service.HasOpsClientBusinessLimited(c) || service.OpsClientBusinessLimitedReason(c) != service.OpsClientBusinessLimitedReasonLocalModelConfiguration {
		return
	}
	entry.AccountID = nil
	entry.UpstreamEndpoint = ""
	entry.UpstreamModel = ""
	entry.UpstreamStatusCode = nil
	entry.UpstreamErrorMessage = nil
	entry.UpstreamErrorDetail = nil
	entry.UpstreamErrors = nil
}

func getContextLatencyMs(c *gin.Context, key string) *int64 {
	if c == nil || strings.TrimSpace(key) == "" {
		return nil
	}
	v, ok := c.Get(key)
	if !ok {
		return nil
	}
	var ms int64
	switch t := v.(type) {
	case int:
		ms = int64(t)
	case int32:
		ms = int64(t)
	case int64:
		ms = t
	case float64:
		ms = int64(t)
	default:
		return nil
	}
	if ms < 0 {
		return nil
	}
	return &ms
}

type parsedOpsError struct {
	ErrorType     string
	Message       string
	Code          string
	StatusCode    int
	StreamFailure bool
}

func parseOpsErrorResponse(body []byte) parsedOpsError {
	if len(body) == 0 {
		return parsedOpsError{}
	}
	if parsed, ok := parseOpsSSEFailure(body); ok {
		return parsed
	}

	// Fast path: attempt to decode into a generic map.
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return parsedOpsError{Message: truncateString(string(body), 1024)}
	}

	// Claude/OpenAI-style gateway error: { type:"error", error:{ type, message } }
	if errObj, ok := m["error"].(map[string]any); ok {
		t, _ := errObj["type"].(string)
		msg, _ := errObj["message"].(string)
		if t == "" {
			t = "api_error"
		}
		code := opsJSONScalarString(errObj["code"])
		return parsedOpsError{ErrorType: t, Message: msg, Code: code}
	}
	if errMessage, ok := m["error"].(string); ok && strings.TrimSpace(errMessage) != "" {
		t, _ := m["type"].(string)
		if t == "" || t == "error" {
			t = "api_error"
		}
		return parsedOpsError{ErrorType: t, Message: strings.TrimSpace(errMessage), Code: opsJSONScalarString(m["code"])}
	}

	// APIKeyAuth-style: { code:"INSUFFICIENT_BALANCE", message:"..." }
	code := opsJSONScalarString(m["code"])
	msg, _ := m["message"].(string)
	if code != "" || msg != "" {
		t, _ := m["type"].(string)
		if t == "" || t == "error" {
			t = "api_error"
		}
		return parsedOpsError{ErrorType: t, Message: msg, Code: code}
	}

	return parsedOpsError{Message: truncateString(string(body), 1024)}
}

func opsJSONScalarString(value any) string {
	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value)
	case float64:
		return strconvItoa(int(value))
	case json.Number:
		return strings.TrimSpace(value.String())
	case int:
		return strconvItoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	default:
		return ""
	}
}

func opsJSONInt(value any) int {
	switch value := value.(type) {
	case float64:
		return int(value)
	case json.Number:
		parsed, _ := strconv.Atoi(value.String())
		return parsed
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(value))
		return parsed
	case int:
		return value
	case int64:
		return int(value)
	default:
		return 0
	}
}

func parseOpsSSEFailure(body []byte) (parsedOpsError, bool) {
	normalized := strings.ReplaceAll(string(body), "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	var errorCandidate *parsedOpsError
	for _, frame := range strings.Split(normalized, "\n\n") {
		frame = strings.TrimSpace(frame)
		if frame == "" {
			continue
		}
		eventTypeBytes, payloadBytes := parseOpsSSEFrameEnvelope([]byte(frame))
		eventType := string(eventTypeBytes)
		if eventType != "response.failed" && eventType != "error" && len(payloadBytes) == 0 {
			continue
		}

		payload := string(payloadBytes)
		var event map[string]any
		if err := json.Unmarshal(payloadBytes, &event); err == nil {
			if eventType == "" {
				eventType, _ = event["type"].(string)
			}
		}
		if eventType != "response.failed" && eventType != "error" {
			continue
		}

		parsed := parsedOpsError{ErrorType: "upstream_error", StreamFailure: true}
		if eventType == "error" {
			parsed.ErrorType = "api_error"
		}
		errObj := opsSSEErrorObject(event)
		if errObj == nil && event != nil && (event["message"] != nil || event["code"] != nil) {
			errObj = event
		}
		if errObj != nil {
			parsed.ErrorType, _ = errObj["type"].(string)
			if parsed.ErrorType == "error" || parsed.ErrorType == "response.failed" {
				parsed.ErrorType = ""
			}
			parsed.Message, _ = errObj["message"].(string)
			switch code := errObj["code"].(type) {
			case string:
				parsed.Code = strings.TrimSpace(code)
			case float64:
				parsed.Code = strconvItoa(int(code))
			}
			parsed.StatusCode = opsJSONInt(errObj["status_code"])
			if parsed.StatusCode == 0 {
				parsed.StatusCode = opsJSONInt(errObj["status"])
			}
			if parsed.StatusCode == 0 {
				parsed.StatusCode = opsJSONInt(event["status_code"])
			}
			if parsed.StatusCode == 0 {
				parsed.StatusCode = opsJSONInt(event["status"])
			}
			if parsed.ErrorType == "" {
				parsed.ErrorType = inferResponsesFailedOpsErrorType(parsed.Code)
			}
			if parsed.ErrorType == "" {
				if eventType == "error" {
					parsed.ErrorType = "api_error"
				} else {
					parsed.ErrorType = "upstream_error"
				}
			}
		}
		if strings.TrimSpace(parsed.Message) == "" && payload != "" {
			trimmedPayload := strings.TrimSpace(payload)
			if strings.HasPrefix(trimmedPayload, "{") || strings.HasPrefix(trimmedPayload, "[") {
				parsed.Message = "upstream stream failed"
			} else {
				parsed.Message = truncateString(trimmedPayload, 1024)
			}
		}
		if eventType == "response.failed" {
			return parsed, true
		}
		candidate := parsed
		errorCandidate = &candidate
	}
	if errorCandidate != nil {
		return *errorCandidate, true
	}
	return parsedOpsError{}, false
}

func opsSSEErrorObject(event map[string]any) map[string]any {
	if event == nil {
		return nil
	}
	if errObj, ok := event["error"].(map[string]any); ok {
		return errObj
	}
	if response, ok := event["response"].(map[string]any); ok {
		if errObj, ok := response["error"].(map[string]any); ok {
			return errObj
		}
	}
	// Some providers flatten error fields onto the terminal event itself:
	// {"type":"error","code":"service_unavailable","message":"..."}.
	if eventType, _ := event["type"].(string); eventType == "error" {
		return event
	}
	return nil
}

func sanitizeOpsSSEDataForPersistence(body []byte) string {
	if len(body) == 0 || !bytes.Contains(body, []byte("data")) {
		return string(body)
	}
	normalized := bytes.ReplaceAll(body, []byte("\r\n"), []byte{'\n'})
	normalized = bytes.ReplaceAll(normalized, []byte{'\r'}, []byte{'\n'})
	frames := bytes.Split(normalized, []byte("\n\n"))
	var out bytes.Buffer
	out.Grow(len(body))
	for frameIndex, frame := range frames {
		if frameIndex > 0 {
			_, _ = out.WriteString("\n\n")
		}
		_, payload := parseOpsSSEFrameEnvelope(frame)
		trimmedPayload := bytes.TrimSpace(payload)
		replacement := ""
		if json.Valid(trimmedPayload) {
			replacement, _ = service.SanitizeOpsErrorBodyForQueue(string(trimmedPayload))
		} else if len(trimmedPayload) > 0 && (trimmedPayload[0] == '{' || trimmedPayload[0] == '[') {
			// Captured terminal frames can be truncated at the queue bound. Never
			// persist a JSON-looking fragment that could contain an unredacted key.
			replacement = `{"payload_truncated":true}`
		}
		if replacement == "" {
			_, _ = out.Write(frame)
			continue
		}
		wroteData := false
		emittedLine := false
		for _, line := range bytes.Split(frame, []byte{'\n'}) {
			field, _, found := bytes.Cut(line, []byte{':'})
			if found && bytes.Equal(bytes.TrimSpace(field), []byte("data")) {
				if wroteData {
					continue
				}
				line = append([]byte("data: "), replacement...)
				wroteData = true
			}
			if emittedLine {
				_ = out.WriteByte('\n')
			}
			_, _ = out.Write(line)
			emittedLine = true
		}
	}
	return out.String()
}

func inferResponsesFailedOpsErrorType(code string) string {
	switch strings.TrimSpace(code) {
	case "rate_limit_exceeded":
		return "rate_limit_error"
	case "permission_denied", "permission_error", "insufficient_permissions", "cyber_policy", "content_policy":
		return "permission_error"
	case "invalid_request", "context_length_exceeded":
		return "invalid_request_error"
	case "server_is_overloaded":
		return "overloaded_error"
	case "service_unavailable", "service_unavailable_error", "server_error":
		return "service_unavailable_error"
	case "authentication_failed":
		return "authentication_error"
	default:
		return ""
	}
}

func inferStreamFailureStatus(_ *gin.Context, parsed parsedOpsError) int {
	if parsed.StatusCode >= 400 && parsed.StatusCode <= 599 {
		return parsed.StatusCode
	}
	switch strings.TrimSpace(parsed.Code) {
	case "rate_limit_exceeded":
		return http.StatusTooManyRequests
	case "permission_denied", "permission_error", "insufficient_permissions", "cyber_policy", "content_policy":
		return http.StatusForbidden
	case "invalid_request", "context_length_exceeded":
		return http.StatusBadRequest
	case "server_is_overloaded":
		return http.StatusServiceUnavailable
	case "service_unavailable", "service_unavailable_error", "server_error":
		return http.StatusServiceUnavailable
	case "authentication_failed":
		return http.StatusUnauthorized
	}

	switch strings.TrimSpace(parsed.ErrorType) {
	case "rate_limit_error":
		return http.StatusTooManyRequests
	case "permission_error", "forbidden_error":
		return http.StatusForbidden
	case "authentication_error":
		return http.StatusUnauthorized
	case "invalid_request_error":
		return http.StatusBadRequest
	case "overloaded_error", "service_unavailable_error":
		return http.StatusServiceUnavailable
	}

	return http.StatusBadGateway
}

// getOpsAPIKey 返回用于 Ops 错误日志的 API Key：优先取已鉴权写入的正式 key；
// 鉴权早退（分组停用/删除、Key 停用/过期/额度、用户停用、IP 限制等）时，
// 正式 key 尚未写入，回退到 middleware 写入的 ops fallback key
// （含 User/Group/Platform），从而让日志能展示 用户/分组/平台。
func getOpsAPIKey(c *gin.Context) *service.APIKey {
	if apiKey, ok := middleware2.GetAPIKeyFromContext(c); ok && apiKey != nil {
		return apiKey
	}
	if apiKey, ok := middleware2.GetOpsFallbackAPIKey(c); ok && apiKey != nil {
		return apiKey
	}
	return nil
}

func resolveOpsPlatform(ctx context.Context, apiKey *service.APIKey, fallback string) string {
	if platform, ok := service.ResolvedTargetPlatformFromContext(ctx); ok {
		return platform
	}
	if apiKey != nil && apiKey.Group != nil && apiKey.Group.Platform != "" {
		return apiKey.Group.Platform
	}
	return fallback
}

func guessPlatformFromPath(path string) string {
	p := strings.ToLower(path)
	switch {
	case strings.HasPrefix(p, "/antigravity/"):
		return service.PlatformAntigravity
	case strings.HasPrefix(p, "/v1beta/"):
		return service.PlatformGemini
	case strings.Contains(p, "/responses"), strings.Contains(p, "/images/"):
		return service.PlatformOpenAI
	default:
		return ""
	}
}

// isKnownOpsErrorType returns true if t is a recognized error type used by the
// ops classification pipeline.  Upstream proxies sometimes return garbage values
// (e.g. the Go-serialized literal "<nil>") which would pollute phase/severity
// classification if accepted blindly.
func isKnownOpsErrorType(t string) bool {
	switch t {
	case "invalid_request_error",
		"authentication_error",
		"permission_error",
		"model_not_found",
		"service_unavailable",
		"rate_limit_error",
		"billing_error",
		"subscription_error",
		"upstream_error",
		"overloaded_error",
		"service_unavailable_error",
		"api_error",
		"not_found_error",
		"forbidden_error":
		return true
	}
	return false
}

func normalizeOpsErrorType(errType string, code string) string {
	if errType != "" && isKnownOpsErrorType(errType) {
		return errType
	}
	switch strings.TrimSpace(code) {
	case opsCodeInsufficientBalance:
		return "billing_error"
	case opsCodeUsageLimitExceeded, opsCodeSubscriptionNotFound, opsCodeSubscriptionInvalid:
		return "subscription_error"
	default:
		return "api_error"
	}
}

func classifyOpsPhase(errType, message, code string) string {
	msg := strings.ToLower(message)
	// Standardized phases: request|auth|account_auth|routing|upstream|network|internal
	// Map billing/concurrency/response => request; scheduling => routing.
	if isOpsClientAuthError(code, msg) {
		return "auth"
	}
	if isOpsLocalBusinessLimitError(code, msg) {
		return "request"
	}

	switch errType {
	case "authentication_error":
		return "auth"
	case "billing_error", "subscription_error":
		return "request"
	case "rate_limit_error":
		if strings.Contains(msg, "concurrency") || strings.Contains(msg, "pending") || strings.Contains(msg, "queue") {
			return "request"
		}
		return "upstream"
	case "invalid_request_error", "permission_error", "forbidden_error", "not_found_error", "model_not_found":
		return "request"
	case "upstream_error", "overloaded_error":
		return "upstream"
	case "api_error":
		if isOpsNoAvailableAccountMessage(msg) {
			return "routing"
		}
		return "internal"
	default:
		return "internal"
	}
}

func classifyOpsSeverity(errType string, status int) string {
	switch errType {
	case "invalid_request_error", "authentication_error", "permission_error", "forbidden_error", "not_found_error", "model_not_found", "billing_error", "subscription_error":
		return "P3"
	}
	if status >= 500 {
		return "P1"
	}
	if status == 429 {
		return "P1"
	}
	if status >= 400 {
		return "P2"
	}
	return "P3"
}

func classifyOpsErrorLog(c *gin.Context, errType, message, code string, status int) (phase string, isBusinessLimited bool, errorOwner string, errorSource string) {
	phase = classifyOpsPhase(errType, message, code)
	routingCapacityLimited := isOpsRoutingCapacityLimited(c)
	clientBusinessLimited := service.HasOpsClientBusinessLimited(c)
	localModelConfiguration := clientBusinessLimited && service.OpsClientBusinessLimitedReason(c) == service.OpsClientBusinessLimitedReasonLocalModelConfiguration
	upstreamError := hasOpsUpstreamErrorContext(c)
	accountAuthFailure := hasOpsAccountAuthFailure(c)
	if localModelConfiguration {
		phase = "routing"
	} else if accountAuthFailure && !routingCapacityLimited {
		phase = "account_auth"
	} else if upstreamError && !routingCapacityLimited {
		phase = "upstream"
	}
	if clientBusinessLimited && !upstreamError && !routingCapacityLimited && !localModelConfiguration {
		phase = "auth"
	}
	if routingCapacityLimited {
		phase = "routing"
	}
	msg := strings.ToLower(message)
	effectiveUpstreamError := upstreamError && !localModelConfiguration
	localClientAuthError := !effectiveUpstreamError && phase == "auth" && isOpsClientAuthError(code, msg)
	localBusinessLimited := !effectiveUpstreamError && classifyOpsIsBusinessLimited(errType, phase, code, status, message, localClientAuthError)
	isBusinessLimited = localModelConfiguration || routingCapacityLimited || (clientBusinessLimited && !effectiveUpstreamError) || localBusinessLimited
	errorOwner = classifyOpsErrorOwner(phase, message)
	errorSource = classifyOpsErrorSource(phase, message)
	return phase, isBusinessLimited, errorOwner, errorSource
}

func classifyOpsIsBusinessLimited(errType, phase, code string, status int, message string, localClientAuthError ...bool) bool {
	if len(localClientAuthError) > 0 && localClientAuthError[0] {
		return true
	}
	if isOpsLocalBusinessLimitError(code, strings.ToLower(message)) {
		return true
	}
	if phase == "billing" || phase == "concurrency" {
		// SLA/错误率排除“用户级业务限制”
		return true
	}
	// Avoid treating upstream rate limits as business-limited.
	if errType == "rate_limit_error" && strings.Contains(strings.ToLower(message), "upstream") {
		return false
	}
	_ = status
	return false
}

func isOpsClientAuthError(code string, msg string) bool {
	switch strings.TrimSpace(code) {
	case opsCodeInvalidAPIKey,
		opsCodeAPIKeyRequired,
		opsCodeAPIKeyExpired,
		opsCodeAPIKeyDisabled,
		opsCodeUserNotFound,
		opsCodeUserInactive,
		opsCodeGroupDeleted,
		opsCodeGroupDisabled:
		return true
	}
	return strings.Contains(msg, "invalid api key") ||
		strings.Contains(msg, "api key is required") ||
		strings.Contains(msg, "api key is disabled") ||
		strings.Contains(msg, "user associated with api key not found") ||
		strings.Contains(msg, "user account is not active") ||
		strings.Contains(msg, "api key 所属分组已删除") ||
		strings.Contains(msg, "api key 所属分组已停用") ||
		strings.Contains(msg, "api key is not assigned to any group")
}

func isOpsLocalBusinessLimitError(code string, msg string) bool {
	switch strings.TrimSpace(code) {
	case opsCodeInsufficientBalance,
		opsCodeUsageLimitExceeded,
		opsCodeSubscriptionNotFound,
		opsCodeSubscriptionInvalid,
		opsCodeAPIKeyQuotaExhausted,
		opsCodeAPIKeyQueryDeprecated:
		return true
	}
	return strings.Contains(msg, "api key in query parameter is deprecated") ||
		strings.Contains(msg, "query parameter api_key is deprecated") ||
		strings.Contains(msg, "no active subscription found for this group") ||
		strings.Contains(msg, "subscription is invalid or expired") ||
		strings.Contains(msg, opsErrInsufficientBalance) ||
		strings.Contains(msg, "insufficient account balance") ||
		strings.Contains(msg, "api key group platform is not gemini") ||
		strings.Contains(msg, "api key 额度已用完") ||
		strings.Contains(msg, "api key 5小时限额已用完") ||
		strings.Contains(msg, "api key 日限额已用完") ||
		strings.Contains(msg, "api key 7天限额已用完") ||
		strings.Contains(msg, "daily usage limit exceeded") ||
		strings.Contains(msg, "weekly usage limit exceeded") ||
		strings.Contains(msg, "monthly usage limit exceeded") ||
		strings.Contains(msg, "usage quota exhausted for this platform") ||
		strings.Contains(msg, "requests-per-minute limit exceeded") ||
		strings.Contains(msg, "too many pending requests") ||
		strings.Contains(msg, "concurrency limit exceeded") ||
		strings.Contains(msg, "image generation concurrency limit exceeded") ||
		strings.Contains(msg, "this group is restricted to claude code clients") ||
		strings.Contains(msg, "this group does not allow /v1/messages dispatch") ||
		strings.Contains(msg, "image generation is not enabled for this group") ||
		(strings.Contains(msg, "reasoning effort") && strings.Contains(msg, "exceeds this group's limit")) ||
		strings.Contains(msg, "token counting is not supported for this platform") ||
		strings.Contains(msg, "images api is not supported for this platform") ||
		(strings.Contains(msg, "model ") && strings.Contains(msg, " not in whitelist")) ||
		(strings.Contains(msg, "beta feature ") && strings.Contains(msg, " is not allowed")) ||
		(strings.Contains(msg, "openai service_tier=") && strings.Contains(msg, " is not allowed for model")) ||
		strings.Contains(msg, "this account only allows codex official clients") ||
		strings.Contains(msg, "openai wsv1 is temporarily unsupported") ||
		strings.Contains(msg, "openai codex passthrough requires a non-empty instructions field")
}

func hasOpsUpstreamErrorContext(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if v, ok := c.Get(service.OpsUpstreamStatusCodeKey); ok {
		switch code := v.(type) {
		case int:
			if code > 0 {
				return true
			}
		case int64:
			if code > 0 {
				return true
			}
		}
	}
	if v, ok := c.Get(service.OpsUpstreamErrorsKey); ok {
		if events, ok := v.([]*service.OpsUpstreamErrorEvent); ok && len(events) > 0 {
			return true
		}
	}
	return false
}

func hasOpsAccountAuthFailure(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if v, ok := c.Get(service.OpsUpstreamErrorsKey); ok {
		if events, ok := v.([]*service.OpsUpstreamErrorEvent); ok {
			for i := len(events) - 1; i >= 0; i-- {
				if events[i] != nil {
					return events[i].Stage == string(service.GatewayFailureStageAccountAuth)
				}
			}
		}
	}
	return false
}

func isOpsNoAvailableAccountMessage(message string) bool {
	msg := strings.ToLower(message)
	return strings.Contains(msg, opsErrNoAvailableAccounts) ||
		strings.Contains(msg, "no available account") ||
		strings.Contains(msg, "no available gemini accounts") ||
		strings.Contains(msg, "no available openai accounts") ||
		strings.Contains(msg, "no available compatible accounts")
}

func classifyOpsErrorOwner(phase string, message string) string {
	// Standardized owners: client|provider|platform
	switch phase {
	case "upstream", "network":
		return "provider"
	case "account_auth":
		return "provider"
	case "request", "auth":
		return "client"
	case "routing", "internal":
		return "platform"
	default:
		if strings.Contains(strings.ToLower(message), "upstream") {
			return "provider"
		}
		return "platform"
	}
}

func classifyOpsErrorSource(phase string, message string) string {
	// Standardized sources: client_request|upstream_http|gateway
	switch phase {
	case "upstream":
		return "upstream_http"
	case "account_auth":
		return "gateway"
	case "network":
		return "gateway"
	case "request", "auth":
		return "client_request"
	case "routing", "internal":
		return "gateway"
	default:
		if strings.Contains(strings.ToLower(message), "upstream") {
			return "upstream_http"
		}
		return "gateway"
	}
}

func truncateString(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	// Ensure truncation does not split multi-byte characters.
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}

func strconvItoa(v int) string {
	return strconv.Itoa(v)
}

// shouldSkipOpsErrorLog determines if an error should be skipped from logging based on settings.
// Returns true for errors that should be filtered according to OpsAdvancedSettings.
func shouldSkipOpsErrorLog(ctx context.Context, ops *service.OpsService, message, body, requestPath string) bool {
	if ops == nil {
		return false
	}

	// Get advanced settings to check filter configuration
	_ = ctx
	settings := ops.OpsAdvancedSettingsSnapshot()

	msgLower := strings.ToLower(message)
	bodyLower := strings.ToLower(body)

	// Check if count_tokens errors should be ignored
	if settings.IgnoreCountTokensErrors && isTokenCountRequestPath(requestPath) {
		return true
	}

	// Check if context canceled errors should be ignored (client disconnects)
	if settings.IgnoreContextCanceled {
		if strings.Contains(msgLower, opsErrContextCanceled) || strings.Contains(bodyLower, opsErrContextCanceled) {
			return true
		}
	}

	// Check if "no available accounts" errors should be ignored
	if settings.IgnoreNoAvailableAccounts {
		if strings.Contains(msgLower, opsErrNoAvailableAccounts) || strings.Contains(bodyLower, opsErrNoAvailableAccounts) {
			return true
		}
	}

	// Check if invalid/missing API key errors should be ignored (user misconfiguration)
	if settings.IgnoreInvalidApiKeyErrors {
		if strings.Contains(bodyLower, opsErrInvalidAPIKey) || strings.Contains(bodyLower, opsErrAPIKeyRequired) {
			return true
		}
	}

	// Check if insufficient balance errors should be ignored
	if settings.IgnoreInsufficientBalanceErrors {
		if strings.Contains(bodyLower, opsErrInsufficientBalance) || strings.Contains(bodyLower, opsErrInsufficientAccountBalance) ||
			strings.Contains(bodyLower, opsErrInsufficientQuota) ||
			strings.Contains(msgLower, opsErrInsufficientBalance) || strings.Contains(msgLower, opsErrInsufficientAccountBalance) {
			return true
		}
	}

	return false
}

// shouldSkipOpsErrorLogForCyber：cyber_policy 命中的请求由 recordCyberPolicyIfMarked
// 统一落一条 status=403 的错误请求，故中间件跳过自身落库，避免双写。
func shouldSkipOpsErrorLogForCyber(c *gin.Context) bool {
	return service.GetOpsCyberPolicy(c) != nil
}
