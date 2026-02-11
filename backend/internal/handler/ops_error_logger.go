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
)

const (
	opsErrorLogTimeout      = 5 * time.Second
	opsErrorLogDrainTimeout = 10 * time.Second

	opsErrorLogMinWorkerCount = 4
	opsErrorLogMaxWorkerCount = 32

	opsErrorLogQueueSizePerWorker = 128
	opsErrorLogMinQueueSize       = 256
	opsErrorLogMaxQueueSize       = 8192
)

type opsErrorLogJob struct {
	ops         *service.OpsService
	entry       *service.OpsInsertErrorLogInput
	requestBody []byte
REDACTED

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

	opsErrorLogLastDropLogAt atomic.Int64

	opsErrorLogShutdownCh   = make(chan struct{REDACTED)
	opsErrorLogShutdownOnce sync.Once
	opsErrorLogDrained      atomic.Bool
)

func startOpsErrorLogWorkers() {
	opsErrorLogMu.Lock()
	defer opsErrorLogMu.Unlock()

	if opsErrorLogStopping {
		return
REDACTED

	workerCount, queueSize := opsErrorLogConfig()
	opsErrorLogQueue = make(chan opsErrorLogJob, queueSize)
	opsErrorLogQueueLen.Store(0)

	opsErrorLogWorkersWg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer opsErrorLogWorkersWg.Done()
			for job := range opsErrorLogQueue {
				opsErrorLogQueueLen.Add(-1)
				if job.ops == nil || job.entry == nil {
					continue
			REDACTED
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("[OpsErrorLogger] worker panic: %v\n%s", r, debug.Stack())
					REDACTED
				REDACTED()
					ctx, cancel := context.WithTimeout(context.Background(), opsErrorLogTimeout)
					_ = job.ops.RecordError(ctx, job.entry, job.requestBody)
					cancel()
					opsErrorLogProcessed.Add(1)
			REDACTED()
		REDACTED
	REDACTED()
REDACTED
REDACTED

func enqueueOpsErrorLog(ops *service.OpsService, entry *service.OpsInsertErrorLogInput, requestBody []byte) {
	if ops == nil || entry == nil {
		return
REDACTED
	select {
	case <-opsErrorLogShutdownCh:
		return
	default:
REDACTED

	opsErrorLogMu.RLock()
	stopping := opsErrorLogStopping
	opsErrorLogMu.RUnlock()
	if stopping {
		return
REDACTED

	opsErrorLogOnce.Do(startOpsErrorLogWorkers)

	opsErrorLogMu.RLock()
	defer opsErrorLogMu.RUnlock()
	if opsErrorLogStopping || opsErrorLogQueue == nil {
		return
REDACTED

	select {
	case opsErrorLogQueue <- opsErrorLogJob{ops: ops, entry: entry, requestBody: requestBodyREDACTED:
		opsErrorLogQueueLen.Add(1)
		opsErrorLogEnqueued.Add(1)
	default:
		// Queue is full; drop to avoid blocking request handling.
		opsErrorLogDropped.Add(1)
		maybeLogOpsErrorLogDrop()
REDACTED
REDACTED

func StopOpsErrorLogWorkers() bool {
	opsErrorLogStopOnce.Do(func() {
		opsErrorLogShutdownOnce.Do(func() {
			close(opsErrorLogShutdownCh)
	REDACTED)
		opsErrorLogDrained.Store(stopOpsErrorLogWorkers())
REDACTED)
	return opsErrorLogDrained.Load()
REDACTED

func stopOpsErrorLogWorkers() bool {
	opsErrorLogMu.Lock()
	opsErrorLogStopping = true
	ch := opsErrorLogQueue
	if ch != nil {
		close(ch)
REDACTED
	opsErrorLogQueue = nil
	opsErrorLogMu.Unlock()

	if ch == nil {
		opsErrorLogQueueLen.Store(0)
		return true
REDACTED

	done := make(chan struct{REDACTED)
	go func() {
		opsErrorLogWorkersWg.Wait()
		close(done)
REDACTED()

	select {
	case <-done:
		opsErrorLogQueueLen.Store(0)
		return true
	case <-time.After(opsErrorLogDrainTimeout):
		return false
REDACTED
REDACTED

func OpsErrorLogQueueLength() int64 {
	return opsErrorLogQueueLen.Load()
REDACTED

func OpsErrorLogQueueCapacity() int {
	opsErrorLogMu.RLock()
	ch := opsErrorLogQueue
	opsErrorLogMu.RUnlock()
	if ch == nil {
		return 0
REDACTED
	return cap(ch)
REDACTED

func OpsErrorLogDroppedTotal() int64 {
	return opsErrorLogDropped.Load()
REDACTED

func OpsErrorLogEnqueuedTotal() int64 {
	return opsErrorLogEnqueued.Load()
REDACTED

func OpsErrorLogProcessedTotal() int64 {
	return opsErrorLogProcessed.Load()
REDACTED

func maybeLogOpsErrorLogDrop() {
	now := time.Now().Unix()

	for {
		last := opsErrorLogLastDropLogAt.Load()
		if last != 0 && now-last < 60 {
			return
	REDACTED
		if opsErrorLogLastDropLogAt.CompareAndSwap(last, now) {
			break
	REDACTED
REDACTED

	queued := opsErrorLogQueueLen.Load()
	queueCap := OpsErrorLogQueueCapacity()

	log.Printf(
		"[OpsErrorLogger] queue is full; dropping logs (queued=%d cap=%d enqueued_total=%d dropped_total=%d processed_total=%d)",
		queued,
		queueCap,
		opsErrorLogEnqueued.Load(),
		opsErrorLogDropped.Load(),
		opsErrorLogProcessed.Load(),
	)
REDACTED

func opsErrorLogConfig() (workerCount int, queueSize int) {
	workerCount = runtime.GOMAXPROCS(0) * 2
	if workerCount < opsErrorLogMinWorkerCount {
		workerCount = opsErrorLogMinWorkerCount
REDACTED
	if workerCount > opsErrorLogMaxWorkerCount {
		workerCount = opsErrorLogMaxWorkerCount
REDACTED

	queueSize = workerCount * opsErrorLogQueueSizePerWorker
	if queueSize < opsErrorLogMinQueueSize {
		queueSize = opsErrorLogMinQueueSize
REDACTED
	if queueSize > opsErrorLogMaxQueueSize {
		queueSize = opsErrorLogMaxQueueSize
REDACTED

	return workerCount, queueSize
REDACTED

func setOpsRequestContext(c *gin.Context, model string, stream bool, requestBody []byte) {
	if c == nil {
		return
REDACTED
	c.Set(opsModelKey, model)
	c.Set(opsStreamKey, stream)
	if len(requestBody) > 0 {
		c.Set(opsRequestBodyKey, requestBody)
REDACTED
REDACTED

func setOpsSelectedAccount(c *gin.Context, accountID int64) {
	if c == nil || accountID <= 0 {
		return
REDACTED
	c.Set(opsAccountIDKey, accountID)
REDACTED

type opsCaptureWriter struct {
	gin.ResponseWriter
	limit int
	buf   bytes.Buffer
REDACTED

func (w *opsCaptureWriter) Write(b []byte) (int, error) {
	if w.Status() >= 400 && w.limit > 0 && w.buf.Len() < w.limit {
		remaining := w.limit - w.buf.Len()
		if len(b) > remaining {
			_, _ = w.buf.Write(b[:remaining])
	REDACTED else {
			_, _ = w.buf.Write(b)
	REDACTED
REDACTED
	return w.ResponseWriter.Write(b)
REDACTED

func (w *opsCaptureWriter) WriteString(s string) (int, error) {
	if w.Status() >= 400 && w.limit > 0 && w.buf.Len() < w.limit {
		remaining := w.limit - w.buf.Len()
		if len(s) > remaining {
			_, _ = w.buf.WriteString(s[:remaining])
	REDACTED else {
			_, _ = w.buf.WriteString(s)
	REDACTED
REDACTED
	return w.ResponseWriter.WriteString(s)
REDACTED

// OpsErrorLoggerMiddleware records error responses (status >= 400) into ops_error_logs.
//
// Notes:
// - It buffers response bodies only when status >= 400 to avoid overhead for successful traffic.
// - Streaming errors after the response has started (SSE) may still need explicit logging.
func OpsErrorLoggerMiddleware(ops *service.OpsService) gin.HandlerFunc {
	return func(c *gin.Context) {
		w := &opsCaptureWriter{ResponseWriter: c.Writer, limit: 64 * 1024REDACTED
		c.Writer = w
		c.Next()

		if ops == nil {
			return
	REDACTED
		if !ops.IsMonitoringEnabled(c.Request.Context()) {
			return
	REDACTED

		status := c.Writer.Status()
		if status < 400 {
			// Even when the client request succeeds, we still want to persist upstream error attempts
			// (retries/failover) so ops can observe upstream instability that gets "covered" by retries.
			var events []*service.OpsUpstreamErrorEvent
			if v, ok := c.Get(service.OpsUpstreamErrorsKey); ok {
				if arr, ok := v.([]*service.OpsUpstreamErrorEvent); ok && len(arr) > 0 {
					events = arr
			REDACTED
		REDACTED
			// Also accept single upstream fields set by gateway services (rare for successful requests).
			hasUpstreamContext := len(events) > 0
			if !hasUpstreamContext {
				if v, ok := c.Get(service.OpsUpstreamStatusCodeKey); ok {
					switch t := v.(type) {
					case int:
						hasUpstreamContext = t > 0
					case int64:
						hasUpstreamContext = t > 0
				REDACTED
			REDACTED
		REDACTED
			if !hasUpstreamContext {
				if v, ok := c.Get(service.OpsUpstreamErrorMessageKey); ok {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						hasUpstreamContext = true
				REDACTED
			REDACTED
		REDACTED
			if !hasUpstreamContext {
				if v, ok := c.Get(service.OpsUpstreamErrorDetailKey); ok {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						hasUpstreamContext = true
				REDACTED
			REDACTED
		REDACTED
			if !hasUpstreamContext {
				return
		REDACTED

			apiKey, _ := middleware2.GetAPIKeyFromContext(c)
			clientRequestID, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string)

			model, _ := c.Get(opsModelKey)
			streamV, _ := c.Get(opsStreamKey)
			accountIDV, _ := c.Get(opsAccountIDKey)

			var modelName string
			if s, ok := model.(string); ok {
				modelName = s
		REDACTED
			stream := false
			if b, ok := streamV.(bool); ok {
				stream = b
		REDACTED

			// Prefer showing the account that experienced the upstream error (if we have events),
			// otherwise fall back to the final selected account (best-effort).
			var accountID *int64
			if len(events) > 0 {
				if last := events[len(events)-1]; last != nil && last.AccountID > 0 {
					v := last.AccountID
					accountID = &v
			REDACTED
		REDACTED
			if accountID == nil {
				if v, ok := accountIDV.(int64); ok && v > 0 {
					accountID = &v
			REDACTED
		REDACTED

			fallbackPlatform := guessPlatformFromPath(c.Request.URL.Path)
			platform := resolveOpsPlatform(apiKey, fallbackPlatform)

			requestID := c.Writer.Header().Get("X-Request-Id")
			if requestID == "" {
				requestID = c.Writer.Header().Get("x-request-id")
		REDACTED

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
				REDACTED
					if msg := strings.TrimSpace(last.Message); msg != "" {
						upstreamErrorMessage = &msg
				REDACTED
					if detail := strings.TrimSpace(last.Detail); detail != "" {
						upstreamErrorDetail = &detail
				REDACTED
			REDACTED
		REDACTED

			if upstreamStatusCode == nil {
				if v, ok := c.Get(service.OpsUpstreamStatusCodeKey); ok {
					switch t := v.(type) {
					case int:
						if t > 0 {
							code := t
							upstreamStatusCode = &code
					REDACTED
					case int64:
						if t > 0 {
							code := int(t)
							upstreamStatusCode = &code
					REDACTED
				REDACTED
			REDACTED
		REDACTED
			if upstreamErrorMessage == nil {
				if v, ok := c.Get(service.OpsUpstreamErrorMessageKey); ok {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						msg := strings.TrimSpace(s)
						upstreamErrorMessage = &msg
				REDACTED
			REDACTED
		REDACTED
			if upstreamErrorDetail == nil {
				if v, ok := c.Get(service.OpsUpstreamErrorDetailKey); ok {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						detail := strings.TrimSpace(s)
						upstreamErrorDetail = &detail
				REDACTED
			REDACTED
		REDACTED

			// If we still have nothing meaningful, skip.
			if upstreamStatusCode == nil && upstreamErrorMessage == nil && upstreamErrorDetail == nil && len(events) == 0 {
				return
		REDACTED

			effectiveUpstreamStatus := 0
			if upstreamStatusCode != nil {
				effectiveUpstreamStatus = *upstreamStatusCode
		REDACTED

			recoveredMsg := "Recovered upstream error"
			if effectiveUpstreamStatus > 0 {
				recoveredMsg += " " + strconvItoa(effectiveUpstreamStatus)
		REDACTED
			if upstreamErrorMessage != nil && strings.TrimSpace(*upstreamErrorMessage) != "" {
				recoveredMsg += ": " + strings.TrimSpace(*upstreamErrorMessage)
		REDACTED
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
				REDACTED
					return ""
			REDACTED(),
				Stream:    stream,
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
		REDACTED

			if apiKey != nil {
				entry.APIKeyID = &apiKey.ID
				if apiKey.User != nil {
					entry.UserID = &apiKey.User.ID
			REDACTED
				if apiKey.GroupID != nil {
					entry.GroupID = apiKey.GroupID
			REDACTED
				// Prefer group platform if present (more stable than inferring from path).
				if apiKey.Group != nil && apiKey.Group.Platform != "" {
					entry.Platform = apiKey.Group.Platform
			REDACTED
		REDACTED

			var clientIP string
			if ip := strings.TrimSpace(ip.GetClientIP(c)); ip != "" {
				clientIP = ip
				entry.ClientIP = &clientIP
		REDACTED

			var requestBody []byte
			if v, ok := c.Get(opsRequestBodyKey); ok {
				if b, ok := v.([]byte); ok && len(b) > 0 {
					requestBody = b
			REDACTED
		REDACTED
			// Store request headers/body only when an upstream error occurred to keep overhead minimal.
			entry.RequestHeadersJSON = extractOpsRetryRequestHeaders(c)

			// Skip logging if a passthrough rule with skip_monitoring=true matched.
			if v, ok := c.Get(service.OpsSkipPassthroughKey); ok {
				if skip, _ := v.(bool); skip {
					return
			REDACTED
		REDACTED

			enqueueOpsErrorLog(ops, entry, requestBody)
			return
	REDACTED

		body := w.buf.Bytes()
		parsed := parseOpsErrorResponse(body)

		// Skip logging if a passthrough rule with skip_monitoring=true matched.
		if v, ok := c.Get(service.OpsSkipPassthroughKey); ok {
			if skip, _ := v.(bool); skip {
				return
		REDACTED
	REDACTED

		// Skip logging if the error should be filtered based on settings
		if shouldSkipOpsErrorLog(c.Request.Context(), ops, parsed.Message, string(body), c.Request.URL.Path) {
			return
	REDACTED

		apiKey, _ := middleware2.GetAPIKeyFromContext(c)

		clientRequestID, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string)

		model, _ := c.Get(opsModelKey)
		streamV, _ := c.Get(opsStreamKey)
		accountIDV, _ := c.Get(opsAccountIDKey)

		var modelName string
		if s, ok := model.(string); ok {
			modelName = s
	REDACTED
		stream := false
		if b, ok := streamV.(bool); ok {
			stream = b
	REDACTED
		var accountID *int64
		if v, ok := accountIDV.(int64); ok && v > 0 {
			accountID = &v
	REDACTED

		fallbackPlatform := guessPlatformFromPath(c.Request.URL.Path)
		platform := resolveOpsPlatform(apiKey, fallbackPlatform)

		requestID := c.Writer.Header().Get("X-Request-Id")
		if requestID == "" {
			requestID = c.Writer.Header().Get("x-request-id")
	REDACTED

		phase := classifyOpsPhase(parsed.ErrorType, parsed.Message, parsed.Code)
		isBusinessLimited := classifyOpsIsBusinessLimited(parsed.ErrorType, phase, parsed.Code, status, parsed.Message)

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
			REDACTED
				return ""
		REDACTED(),
			Stream:    stream,
			UserAgent: c.GetHeader("User-Agent"),

			ErrorPhase:        phase,
			ErrorType:         normalizeOpsErrorType(parsed.ErrorType, parsed.Code),
			Severity:          classifyOpsSeverity(parsed.ErrorType, status),
			StatusCode:        status,
			IsBusinessLimited: isBusinessLimited,
			IsCountTokens:     isCountTokensRequest(c),

			ErrorMessage: parsed.Message,
			// Keep the full captured error body (capture is already capped at 64KB) so the
			// service layer can sanitize JSON before truncating for storage.
			ErrorBody:   string(body),
			ErrorSource: errorSource,
			ErrorOwner:  errorOwner,

			IsRetryable: classifyOpsIsRetryable(parsed.ErrorType, status),
			RetryCount:  0,
			CreatedAt:   time.Now(),
	REDACTED

		// Capture upstream error context set by gateway services (if present).
		// This does NOT affect the client response; it enriches Ops troubleshooting data.
		{
			if v, ok := c.Get(service.OpsUpstreamStatusCodeKey); ok {
				switch t := v.(type) {
				case int:
					if t > 0 {
						code := t
						entry.UpstreamStatusCode = &code
				REDACTED
				case int64:
					if t > 0 {
						code := int(t)
						entry.UpstreamStatusCode = &code
				REDACTED
			REDACTED
		REDACTED
			if v, ok := c.Get(service.OpsUpstreamErrorMessageKey); ok {
				if s, ok := v.(string); ok {
					if msg := strings.TrimSpace(s); msg != "" {
						entry.UpstreamErrorMessage = &msg
				REDACTED
			REDACTED
		REDACTED
			if v, ok := c.Get(service.OpsUpstreamErrorDetailKey); ok {
				if s, ok := v.(string); ok {
					if detail := strings.TrimSpace(s); detail != "" {
						entry.UpstreamErrorDetail = &detail
				REDACTED
			REDACTED
		REDACTED
			if v, ok := c.Get(service.OpsUpstreamErrorsKey); ok {
				if events, ok := v.([]*service.OpsUpstreamErrorEvent); ok && len(events) > 0 {
					entry.UpstreamErrors = events
					// Best-effort backfill the single upstream fields from the last event when missing.
					last := events[len(events)-1]
					if last != nil {
						if entry.UpstreamStatusCode == nil && last.UpstreamStatusCode > 0 {
							code := last.UpstreamStatusCode
							entry.UpstreamStatusCode = &code
					REDACTED
						if entry.UpstreamErrorMessage == nil && strings.TrimSpace(last.Message) != "" {
							msg := strings.TrimSpace(last.Message)
							entry.UpstreamErrorMessage = &msg
					REDACTED
						if entry.UpstreamErrorDetail == nil && strings.TrimSpace(last.Detail) != "" {
							detail := strings.TrimSpace(last.Detail)
							entry.UpstreamErrorDetail = &detail
					REDACTED
				REDACTED
			REDACTED
		REDACTED
	REDACTED

		if apiKey != nil {
			entry.APIKeyID = &apiKey.ID
			if apiKey.User != nil {
				entry.UserID = &apiKey.User.ID
		REDACTED
			if apiKey.GroupID != nil {
				entry.GroupID = apiKey.GroupID
		REDACTED
			// Prefer group platform if present (more stable than inferring from path).
			if apiKey.Group != nil && apiKey.Group.Platform != "" {
				entry.Platform = apiKey.Group.Platform
		REDACTED
	REDACTED

		var clientIP string
		if ip := strings.TrimSpace(ip.GetClientIP(c)); ip != "" {
			clientIP = ip
			entry.ClientIP = &clientIP
	REDACTED

		var requestBody []byte
		if v, ok := c.Get(opsRequestBodyKey); ok {
			if b, ok := v.([]byte); ok && len(b) > 0 {
				requestBody = b
		REDACTED
	REDACTED
		// Persist only a minimal, whitelisted set of request headers to improve retry fidelity.
		// Do NOT store Authorization/Cookie/etc.
		entry.RequestHeadersJSON = extractOpsRetryRequestHeaders(c)

		enqueueOpsErrorLog(ops, entry, requestBody)
REDACTED
REDACTED

var opsRetryRequestHeaderAllowlist = []string{
	"anthropic-beta",
	"anthropic-version",
REDACTED

// isCountTokensRequest checks if the request is a count_tokens request
func isCountTokensRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
REDACTED
	return strings.Contains(c.Request.URL.Path, "/count_tokens")
REDACTED

func extractOpsRetryRequestHeaders(c *gin.Context) *string {
	if c == nil || c.Request == nil {
		return nil
REDACTED

	headers := make(map[string]string, 4)
	for _, key := range opsRetryRequestHeaderAllowlist {
		v := strings.TrimSpace(c.GetHeader(key))
		if v == "" {
			continue
	REDACTED
		// Keep headers small even if a client sends something unexpected.
		headers[key] = truncateString(v, 512)
REDACTED
	if len(headers) == 0 {
		return nil
REDACTED

	raw, err := json.Marshal(headers)
	if err != nil {
		return nil
REDACTED
	s := string(raw)
	return &s
REDACTED

type parsedOpsError struct {
	ErrorType string
	Message   string
	Code      string
REDACTED

func parseOpsErrorResponse(body []byte) parsedOpsError {
	if len(body) == 0 {
		return parsedOpsError{REDACTED
REDACTED

	// Fast path: attempt to decode into a generic map.
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return parsedOpsError{Message: truncateString(string(body), 1024)REDACTED
REDACTED

	// Claude/OpenAI-style gateway error: { type:"error", error:{ type, message REDACTED REDACTED
	if errObj, ok := m["error"].(map[string]any); ok {
		t, _ := errObj["type"].(string)
		msg, _ := errObj["message"].(string)
		// Gemini googleError also uses "error": { code, message, status REDACTED
		if msg == "" {
			if v, ok := errObj["message"]; ok {
				msg, _ = v.(string)
		REDACTED
	REDACTED
		if t == "" {
			// Gemini error does not have "type" field.
			t = "api_error"
	REDACTED
		// For gemini error, capture numeric code as string for business-limited mapping if needed.
		var code string
		if v, ok := errObj["code"]; ok {
			switch n := v.(type) {
			case float64:
				code = strconvItoa(int(n))
			case int:
				code = strconvItoa(n)
		REDACTED
	REDACTED
		return parsedOpsError{ErrorType: t, Message: msg, Code: codeREDACTED
REDACTED

	// APIKeyAuth-style: { code:"INSUFFICIENT_BALANCE", message:"..." REDACTED
	code, _ := m["code"].(string)
	msg, _ := m["message"].(string)
	if code != "" || msg != "" {
		return parsedOpsError{ErrorType: "api_error", Message: msg, Code: codeREDACTED
REDACTED

	return parsedOpsError{Message: truncateString(string(body), 1024)REDACTED
REDACTED

func resolveOpsPlatform(apiKey *service.APIKey, fallback string) string {
	if apiKey != nil && apiKey.Group != nil && apiKey.Group.Platform != "" {
		return apiKey.Group.Platform
REDACTED
	return fallback
REDACTED

func guessPlatformFromPath(path string) string {
	p := strings.ToLower(path)
	switch {
	case strings.HasPrefix(p, "/antigravity/"):
		return service.PlatformAntigravity
	case strings.HasPrefix(p, "/v1beta/"):
		return service.PlatformGemini
	case strings.Contains(p, "/responses"):
		return service.PlatformOpenAI
	default:
		return ""
REDACTED
REDACTED

func normalizeOpsErrorType(errType string, code string) string {
	if errType != "" {
		return errType
REDACTED
	switch strings.TrimSpace(code) {
	case "INSUFFICIENT_BALANCE":
		return "billing_error"
	case "USAGE_LIMIT_EXCEEDED", "SUBSCRIPTION_NOT_FOUND", "SUBSCRIPTION_INVALID":
		return "subscription_error"
	default:
		return "api_error"
REDACTED
REDACTED

func classifyOpsPhase(errType, message, code string) string {
	msg := strings.ToLower(message)
	// Standardized phases: request|auth|routing|upstream|network|internal
	// Map billing/concurrency/response => request; scheduling => routing.
	switch strings.TrimSpace(code) {
	case "INSUFFICIENT_BALANCE", "USAGE_LIMIT_EXCEEDED", "SUBSCRIPTION_NOT_FOUND", "SUBSCRIPTION_INVALID":
		return "request"
REDACTED

	switch errType {
	case "authentication_error":
		return "auth"
	case "billing_error", "subscription_error":
		return "request"
	case "rate_limit_error":
		if strings.Contains(msg, "concurrency") || strings.Contains(msg, "pending") || strings.Contains(msg, "queue") {
			return "request"
	REDACTED
		return "upstream"
	case "invalid_request_error":
		return "request"
	case "upstream_error", "overloaded_error":
		return "upstream"
	case "api_error":
		if strings.Contains(msg, "no available accounts") {
			return "routing"
	REDACTED
		return "internal"
	default:
		return "internal"
REDACTED
REDACTED

func classifyOpsSeverity(errType string, status int) string {
	switch errType {
	case "invalid_request_error", "authentication_error", "billing_error", "subscription_error":
		return "P3"
REDACTED
	if status >= 500 {
		return "P1"
REDACTED
	if status == 429 {
		return "P1"
REDACTED
	if status >= 400 {
		return "P2"
REDACTED
	return "P3"
REDACTED

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
REDACTED
REDACTED

func classifyOpsIsBusinessLimited(errType, phase, code string, status int, message string) bool {
	switch strings.TrimSpace(code) {
	case "INSUFFICIENT_BALANCE", "USAGE_LIMIT_EXCEEDED", "SUBSCRIPTION_NOT_FOUND", "SUBSCRIPTION_INVALID", "USER_INACTIVE":
		return true
REDACTED
	if phase == "billing" || phase == "concurrency" {
		// SLA/错误率排除“用户级业务限制”
		return true
REDACTED
	// Avoid treating upstream rate limits as business-limited.
	if errType == "rate_limit_error" && strings.Contains(strings.ToLower(message), "upstream") {
		return false
REDACTED
	_ = status
	return false
REDACTED

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
	REDACTED
		return "platform"
REDACTED
REDACTED

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
	REDACTED
		return "gateway"
REDACTED
REDACTED

func truncateString(s string, max int) string {
	if max <= 0 {
		return ""
REDACTED
	if len(s) <= max {
		return s
REDACTED
	cut := s[:max]
	// Ensure truncation does not split multi-byte characters.
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
REDACTED
	return cut
REDACTED

func strconvItoa(v int) string {
	return strconv.Itoa(v)
REDACTED

// shouldSkipOpsErrorLog determines if an error should be skipped from logging based on settings.
// Returns true for errors that should be filtered according to OpsAdvancedSettings.
func shouldSkipOpsErrorLog(ctx context.Context, ops *service.OpsService, message, body, requestPath string) bool {
	if ops == nil {
		return false
REDACTED

	// Get advanced settings to check filter configuration
	settings, err := ops.GetOpsAdvancedSettings(ctx)
	if err != nil || settings == nil {
		// If we can't get settings, don't skip (fail open)
		return false
REDACTED

	msgLower := strings.ToLower(message)
	bodyLower := strings.ToLower(body)

	// Check if count_tokens errors should be ignored
	if settings.IgnoreCountTokensErrors && strings.Contains(requestPath, "/count_tokens") {
		return true
REDACTED

	// Check if context canceled errors should be ignored (client disconnects)
	if settings.IgnoreContextCanceled {
		if strings.Contains(msgLower, "context canceled") || strings.Contains(bodyLower, "context canceled") {
			return true
	REDACTED
REDACTED

	// Check if "no available accounts" errors should be ignored
	if settings.IgnoreNoAvailableAccounts {
		if strings.Contains(msgLower, "no available accounts") || strings.Contains(bodyLower, "no available accounts") {
			return true
	REDACTED
REDACTED

	// Check if invalid/missing API key errors should be ignored (user misconfiguration)
	if settings.IgnoreInvalidApiKeyErrors {
		if strings.Contains(bodyLower, "invalid_api_key") || strings.Contains(bodyLower, "api_key_required") {
			return true
	REDACTED
REDACTED

	return false
REDACTED
