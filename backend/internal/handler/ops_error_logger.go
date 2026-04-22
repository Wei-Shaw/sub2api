package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
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
	opsModelKey       = "ops_model"
	opsStreamKey      = "ops_stream"
	opsRequestBodyKey = "ops_request_body"
	opsAccountIDKey   = "ops_account_id"

	opsUpstreamModelKey = "ops_upstream_model"
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
	opsCodeInsufficientBalance  = "INSUFFICIENT_BALANCE"
	opsCodeUsageLimitExceeded   = "USAGE_LIMIT_EXCEEDED"
	opsCodeSubscriptionNotFound = "SUBSCRIPTION_NOT_FOUND"
	opsCodeSubscriptionInvalid  = "SUBSCRIPTION_INVALID"
	opsCodeUserInactive         = "USER_INACTIVE"
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
)

type opsErrorLogJob struct {
	ops   *service.OpsService
	entry *service.OpsInsertErrorLogInput
}

var (
	opsErrorLogOnce  sync.Once
	opsErrorLogQueue chan opsErrorLogJob

	opsErrorLogStopOnce  sync.Once
	opsErrorLogWorkersWg sync.WaitGroup
	opsErrorLogMu        sync.RWMutex
	opsErrorLogStopping  bool
	opsErrorLogQueueLen  atomic.Int64
	opsErrorLogEnqueued  atomic.Int64
	opsErrorLogDropped   atomic.Int64
	opsErrorLogProcessed atomic.Int64
	opsErrorLogSanitized atomic.Int64

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

	select {
	case opsErrorLogQueue <- opsErrorLogJob{ops: ops, entry: entry}:
		opsErrorLogQueueLen.Add(1)
		opsErrorLogEnqueued.Add(1)
	default:
		// Queue is full; drop to avoid blocking request handling.
		opsErrorLogDropped.Add(1)
		maybeLogOpsErrorLogDrop()
	}
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
		return true
	case <-time.After(opsErrorLogDrainTimeout):
		return false
	}
}

func OpsErrorLogQueueLength() int64 {
	return opsErrorLogQueueLen.Load()
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
	queueCap := OpsErrorLogQueueCapacity()

	log.Printf(
		"[OpsErrorLogger] queue is full; dropping logs (queued=%d cap=%d enqueued_total=%d dropped_total=%d processed_total=%d sanitized_total=%d)",
		queued,
		queueCap,
		opsErrorLogEnqueued.Load(),
		opsErrorLogDropped.Load(),
		opsErrorLogProcessed.Load(),
		opsErrorLogSanitized.Load(),
	)
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

func setOpsRequestContext(c *gin.Context, model string, stream bool, requestBody []byte) {
	if c == nil {
		return
	}
	model = strings.TrimSpace(model)
	c.Set(opsModelKey, model)
	c.Set(opsStreamKey, stream)
	if len(requestBody) > 0 {
		c.Set(opsRequestBodyKey, requestBody)
	}
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

func attachOpsRequestBodyToEntry(c *gin.Context, entry *service.OpsInsertErrorLogInput) {
	if c == nil || entry == nil {
		return
	}
	v, ok := c.Get(opsRequestBodyKey)
	if !ok {
		return
	}
	raw, ok := v.([]byte)
	if !ok || len(raw) == 0 {
		return
	}
	entry.RequestBodyJSON, entry.RequestBodyTruncated, entry.RequestBodyBytes = service.PrepareOpsRequestBodyForQueue(raw)
	opsErrorLogSanitized.Add(1)
}

func setOpsSelectedAccount(c *gin.Context, accountID int64, platform ...string) {
	if c == nil || accountID <= 0 {
		return
	}
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

type opsCaptureWriter struct {
	gin.ResponseWriter
	limit int
	buf   bytes.Buffer
}

const opsCaptureWriterLimit = 64 * 1024

var opsCaptureWriterPool = sync.Pool{
	New: func() any {
		return &opsCaptureWriter{limit: opsCaptureWriterLimit}
	},
}

func acquireOpsCaptureWriter(rw gin.ResponseWriter) *opsCaptureWriter {
	w, ok := opsCaptureWriterPool.Get().(*opsCaptureWriter)
	if !ok || w == nil {
		w = &opsCaptureWriter{}
	}
	w.ResponseWriter = rw
	w.limit = opsCaptureWriterLimit
	w.buf.Reset()
	return w
}

func releaseOpsCaptureWriter(w *opsCaptureWriter) {
	if w == nil {
		return
	}
	w.ResponseWriter = nil
	w.limit = opsCaptureWriterLimit
	w.buf.Reset()
	opsCaptureWriterPool.Put(w)
}

func (w *opsCaptureWriter) Write(b []byte) (int, error) {
	if w.Status() >= 400 && w.limit > 0 && w.buf.Len() < w.limit {
		remaining := w.limit - w.buf.Len()
		if len(b) > remaining {
			_, _ = w.buf.Write(b[:remaining])
		} else {
			_, _ = w.buf.Write(b)
		}
	}
	return w.ResponseWriter.Write(b)
}

func (w *opsCaptureWriter) WriteString(s string) (int, error) {
	if w.Status() >= 400 && w.limit > 0 && w.buf.Len() < w.limit {
		remaining := w.limit - w.buf.Len()
		if len(s) > remaining {
			_, _ = w.buf.WriteString(s[:remaining])
		} else {
			_, _ = w.buf.WriteString(s)
		}
	}
	return w.ResponseWriter.WriteString(s)
}

// OpsErrorLoggerMiddleware records error responses (status >= 400) into ops_error_logs.
//
// Notes:
// - It buffers response bodies only when status >= 400 to avoid overhead for successful traffic.
// - Streaming errors after the response has started (SSE) may still need explicit logging.
func OpsErrorLoggerMiddleware(ops *service.OpsService) gin.HandlerFunc {
	return func(c *gin.Context) {
		originalWriter := c.Writer
		w := acquireOpsCaptureWriter(originalWriter)
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

		if ops == nil {
			return
		}
		if !ops.IsMonitoringEnabled(c.Request.Context()) {
			return
		}

		status := c.Writer.Status()
		if status < 400 {
			// Even when the client request succeeds, we still want to persist upstream error attempts
			// (retries/failover) so ops can observe upstream instability that gets "covered" by retries.
			var events []*service.OpsUpstreamErrorEvent
			if v, ok := c.Get(service.OpsUpstreamErrorsKey); ok {
				if arr, ok := v.([]*service.OpsUpstreamErrorEvent); ok && len(arr) > 0 {
					events = arr
				}
			}
			// Also accept single upstream fields set by gateway services (rare for successful requests).
			hasUpstreamContext := len(events) > 0
			if !hasUpstreamContext {
				if v, ok := c.Get(service.OpsUpstreamStatusCodeKey); ok {
					switch t := v.(type) {
					case int:
						hasUpstreamContext = t > 0
					case int64:
						hasUpstreamContext = t > 0
					}
				}
			}
			if !hasUpstreamContext {
				if v, ok := c.Get(service.OpsUpstreamErrorMessageKey); ok {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						hasUpstreamContext = true
					}
				}
			}
			if !hasUpstreamContext {
				if v, ok := c.Get(service.OpsUpstreamErrorDetailKey); ok {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						hasUpstreamContext = true
					}
				}
			}
			if !hasUpstreamContext {
				return
			}

			apiKey, _ := middleware2.GetAPIKeyFromContext(c)
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

			// Prefer showing the account that experienced the upstream error (if we have events),
			// otherwise fall back to the final selected account (best-effort).
			var accountID *int64
			if len(events) > 0 {
				if last := events[len(events)-1]; last != nil && last.AccountID > 0 {
					v := last.AccountID
					accountID = &v
				}
			}
			if accountID == nil {
				if v, ok := accountIDV.(int64); ok && v > 0 {
					accountID = &v
				}
			}

			fallbackPlatform := guessPlatformFromPath(c.Request.URL.Path)
			platform := resolveOpsPlatform(apiKey, fallbackPlatform)

			requestID := c.Writer.Header().Get("X-Request-Id")
			if requestID == "" {
				requestID = c.Writer.Header().Get("x-request-id")
			}

			// Best-effort backfill single upstream fields from the last event (if present).
			var upstreamStatusCode *int
			var upstreamErrorMessage *string
			var upstreamErrorDetail *string
			if len(events) > 0 {
				last := events[len(events)-1]
				if last != nil {
					if last.UpstreamStatusCode > 0 {
						code := last.UpstreamStatusCode
						upstreamStatusCode = &code
					}
					if msg := strings.TrimSpace(last.Message); msg != "" {
						upstreamErrorMessage = &msg
					}
					if detail := strings.TrimSpace(last.Detail); detail != "" {
						upstreamErrorDetail = &detail
					}
				}
			}

			if upstreamStatusCode == nil {
				if v, ok := c.Get(service.OpsUpstreamStatusCodeKey); ok {
					switch t := v.(type) {
					case int:
						if t > 0 {
							code := t
							upstreamStatusCode = &code
						}
					case int64:
						if t > 0 {
							code := int(t)
							upstreamStatusCode = &code
						}
					}
				}
			}
			if upstreamErrorMessage == nil {
				if v, ok := c.Get(service.OpsUpstreamErrorMessageKey); ok {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						msg := strings.TrimSpace(s)
						upstreamErrorMessage = &msg
					}
				}
			}
			if upstreamErrorDetail == nil {
				if v, ok := c.Get(service.OpsUpstreamErrorDetailKey); ok {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						detail := strings.TrimSpace(s)
						upstreamErrorDetail = &detail
					}
				}
			}

			// If we still have nothing meaningful, skip.
			if upstreamStatusCode == nil && upstreamErrorMessage == nil && upstreamErrorDetail == nil && len(events) == 0 {
				return
			}

			effectiveUpstreamStatus := 0
			if upstreamStatusCode != nil {
				effectiveUpstreamStatus = *upstreamStatusCode
			}

			recoveredMsg := "Recovered upstream error"
			if effectiveUpstreamStatus > 0 {
				recoveredMsg += " " + strconvItoa(effectiveUpstreamStatus)
			}
			if upstreamErrorMessage != nil && strings.TrimSpace(*upstreamErrorMessage) != "" {
				recoveredMsg += ": " + strings.TrimSpace(*upstreamErrorMessage)
			}
			recoveredMsg = truncateString(recoveredMsg, 2048)

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

				ErrorPhase: "upstream",
				ErrorType:  "upstream_error",
				// Severity/retryability should reflect the upstream failure, not the final client status (200).
				Severity:          classifyOpsSeverity("upstream_error", effectiveUpstreamStatus),
				StatusCode:        status,
				IsBusinessLimited: false,
				IsCountTokens:     isCountTokensRequest(c),

				ErrorMessage: recoveredMsg,
				ErrorBody:    "",

				ErrorSource: "upstream_http",
				ErrorOwner:  "provider",

				UpstreamStatusCode:   upstreamStatusCode,
				UpstreamErrorMessage: upstreamErrorMessage,
				UpstreamErrorDetail:  upstreamErrorDetail,
				UpstreamErrors:       events,

				IsRetryable: classifyOpsIsRetryable("upstream_error", effectiveUpstreamStatus),
				RetryCount:  0,
				CreatedAt:   time.Now(),
			}
			applyOpsLatencyFieldsFromContext(c, entry)

			if apiKey != nil {
				entry.APIKeyID = &apiKey.ID
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

			// Store request headers/body only when an upstream error occurred to keep overhead minimal.
			entry.RequestHeadersJSON = extractOpsRetryRequestHeaders(c)
			attachOpsRequestBodyToEntry(c, entry)

			// Skip logging if a passthrough rule with skip_monitoring=true matched.
			if v, ok := c.Get(service.OpsSkipPassthroughKey); ok {
				if skip, _ := v.(bool); skip {
					return
				}
			}

			enqueueOpsErrorLog(ops, entry)
			return
		}

		body := w.buf.Bytes()
		parsed := parseOpsErrorResponse(body)

		// Skip logging if a passthrough rule with skip_monitoring=true matched.
		if v, ok := c.Get(service.OpsSkipPassthroughKey); ok {
			if skip, _ := v.(bool); skip {
				return
			}
		}

		// Skip logging if the error should be filtered based on settings
		if shouldSkipOpsErrorLog(c.Request.Context(), ops, parsed.Message, string(body), c.Request.URL.Path) {
			return
		}

		apiKey, _ := middleware2.GetAPIKeyFromContext(c)

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
		platform := resolveOpsPlatform(apiKey, fallbackPlatform)

		requestID := c.Writer.Header().Get("X-Request-Id")
		if requestID == "" {
			requestID = c.Writer.Header().Get("x-request-id")
		}

		normalizedType := normalizeOpsErrorType(parsed.ErrorType, parsed.Code)

		phase := classifyOpsPhase(normalizedType, parsed.Message, parsed.Code)
		isBusinessLimited := classifyOpsIsBusinessLimited(normalizedType, phase, parsed.Code, status, parsed.Message)

		errorOwner := classifyOpsErrorOwner(phase, parsed.Message)
		errorSource := classifyOpsErrorSource(phase, parsed.Message)

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
			// Keep the full captured error body (capture is already capped at 64KB) so the
			// service layer can sanitize JSON before truncating for storage.
			ErrorBody:   string(body),
			ErrorSource: errorSource,
			ErrorOwner:  errorOwner,

			IsRetryable: classifyOpsIsRetryable(normalizedType, status),
			RetryCount:  0,
			CreatedAt:   time.Now(),
		}
		applyOpsLatencyFieldsFromContext(c, entry)

		// Capture upstream error context set by gateway services (if present).
		// This does NOT affect the client response; it enriches Ops troubleshooting data.
		{
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
				if s, ok := v.(string); ok {
					if msg := strings.TrimSpace(s); msg != "" {
						entry.UpstreamErrorMessage = &msg
					}
				}
			}
			if v, ok := c.Get(service.OpsUpstreamErrorDetailKey); ok {
				if s, ok := v.(string); ok {
					if detail := strings.TrimSpace(s); detail != "" {
						entry.UpstreamErrorDetail = &detail
					}
				}
			}
			if v, ok := c.Get(service.OpsUpstreamErrorsKey); ok {
				if events, ok := v.([]*service.OpsUpstreamErrorEvent); ok && len(events) > 0 {
					entry.UpstreamErrors = events
					// Best-effort backfill the single upstream fields from the last event when missing.
					last := events[len(events)-1]
					if last != nil {
						if entry.UpstreamStatusCode == nil && last.UpstreamStatusCode > 0 {
							code := last.UpstreamStatusCode
							entry.UpstreamStatusCode = &code
						}
						if entry.UpstreamErrorMessage == nil && strings.TrimSpace(last.Message) != "" {
							msg := strings.TrimSpace(last.Message)
							entry.UpstreamErrorMessage = &msg
						}
						if entry.UpstreamErrorDetail == nil && strings.TrimSpace(last.Detail) != "" {
							detail := strings.TrimSpace(last.Detail)
							entry.UpstreamErrorDetail = &detail
						}
					}
				}
			}
		}

		if apiKey != nil {
			entry.APIKeyID = &apiKey.ID
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

		// Persist only a minimal, whitelisted set of request headers to improve retry fidelity.
		// Do NOT store Authorization/Cookie/etc.
		entry.RequestHeadersJSON = extractOpsRetryRequestHeaders(c)
		attachOpsRequestBodyToEntry(c, entry)

		enqueueOpsErrorLog(ops, entry)
	}
}

var opsRetryRequestHeaderAllowlist = []string{
	"anthropic-beta",
	"anthropic-version",
}

// isCountTokensRequest checks if the request is a count_tokens request
func isCountTokensRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	return strings.Contains(c.Request.URL.Path, "/count_tokens")
}

func extractOpsRetryRequestHeaders(c *gin.Context) *string {
	if c == nil || c.Request == nil {
		return nil
	}

	headers := make(map[string]string, 4)
	for _, key := range opsRetryRequestHeaderAllowlist {
		v := strings.TrimSpace(c.GetHeader(key))
		if v == "" {
			continue
		}
		// Keep headers small even if a client sends something unexpected.
		headers[key] = truncateString(v, 512)
	}
	if len(headers) == 0 {
		return nil
	}

	raw, err := json.Marshal(headers)
	if err != nil {
		return nil
	}
	s := string(raw)
	return &s
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
	ErrorType string
	Message   string
	Code      string
}

func parseOpsErrorResponse(body []byte) parsedOpsError {
	if len(body) == 0 {
		return parsedOpsError{}
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
		// Gemini googleError also uses "error": { code, message, status }
		if msg == "" {
			if v, ok := errObj["message"]; ok {
				msg, _ = v.(string)
			}
		}
		if t == "" {
			// Gemini error does not have "type" field.
			t = "api_error"
		}
		// For gemini error, capture numeric code as string for business-limited mapping if needed.
		var code string
		if v, ok := errObj["code"]; ok {
			switch n := v.(type) {
			case float64:
				code = strconvItoa(int(n))
			case int:
				code = strconvItoa(n)
			}
		}
		return parsedOpsError{ErrorType: t, Message: msg, Code: code}
	}

	// APIKeyAuth-style: { code:"INSUFFICIENT_BALANCE", message:"..." }
	code, _ := m["code"].(string)
	msg, _ := m["message"].(string)
	if code != "" || msg != "" {
		return parsedOpsError{ErrorType: "api_error", Message: msg, Code: code}
	}

	return parsedOpsError{Message: truncateString(string(body), 1024)}
}

func resolveOpsPlatform(apiKey *service.APIKey, fallback string) string {
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
		"rate_limit_error",
		"billing_error",
		"subscription_error",
		"upstream_error",
		"overloaded_error",
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
	// Standardized phases: request|auth|routing|upstream|network|internal
	// Map billing/concurrency/response => request; scheduling => routing.
	switch strings.TrimSpace(code) {
	case opsCodeInsufficientBalance, opsCodeUsageLimitExceeded, opsCodeSubscriptionNotFound, opsCodeSubscriptionInvalid:
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
	case "invalid_request_error":
		return "request"
	case "upstream_error", "overloaded_error":
		return "upstream"
	case "api_error":
		if strings.Contains(msg, opsErrNoAvailableAccounts) {
			return "routing"
		}
		return "internal"
	default:
		return "internal"
	}
}

func classifyOpsSeverity(errType string, status int) string {
	switch errType {
	case "invalid_request_error", "authentication_error", "billing_error", "subscription_error":
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

func classifyOpsIsRetryable(errType string, statusCode int) bool {
	switch errType {
	case "authentication_error", "invalid_request_error":
		return false
	case "timeout_error":
		return true
	case "rate_limit_error":
		// May be transient (upstream or queue); retry can help.
		return true
	case "billing_error", "subscription_error":
		return false
	case "upstream_error", "overloaded_error":
		return statusCode >= 500 || statusCode == 429 || statusCode == 529
	default:
		return statusCode >= 500
	}
}

func classifyOpsIsBusinessLimited(errType, phase, code string, status int, message string) bool {
	switch strings.TrimSpace(code) {
	case opsCodeInsufficientBalance, opsCodeUsageLimitExceeded, opsCodeSubscriptionNotFound, opsCodeSubscriptionInvalid, opsCodeUserInactive:
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

func classifyOpsErrorOwner(phase string, message string) string {
	// Standardized owners: client|provider|platform
	switch phase {
	case "upstream", "network":
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
	settings, err := ops.GetOpsAdvancedSettings(ctx)
	if err != nil || settings == nil {
		// If we can't get settings, don't skip (fail open)
		return false
	}

	msgLower := strings.ToLower(message)
	bodyLower := strings.ToLower(body)

	// Check if count_tokens errors should be ignored
	if settings.IgnoreCountTokensErrors && strings.Contains(requestPath, "/count_tokens") {
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
