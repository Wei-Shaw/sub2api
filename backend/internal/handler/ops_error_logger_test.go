package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type ingressRejectSettingRepo struct {
	service.SettingRepository
	getValueCalls int
}

func (r *ingressRejectSettingRepo) GetValue(context.Context, string) (string, error) {
	r.getValueCalls++
	return "", service.ErrSettingNotFound
}

func (r *ingressRejectSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	r.getValueCalls++
	return map[string]string{}, nil
}

func (r *ingressRejectSettingRepo) Set(context.Context, string, string) error {
	return nil
}

type ingressRejectOpsRepo struct {
	service.OpsRepository
	insertCalls int
	entries     []*service.OpsInsertErrorLogInput
}

func (r *ingressRejectOpsRepo) InsertErrorLog(_ context.Context, entry *service.OpsInsertErrorLogInput) (int64, error) {
	r.insertCalls++
	r.entries = append(r.entries, entry)
	return 0, nil
}

func (r *ingressRejectOpsRepo) BatchInsertErrorLogs(_ context.Context, entries []*service.OpsInsertErrorLogInput) (int64, error) {
	r.insertCalls++
	r.entries = append(r.entries, entries...)
	return 0, nil
}

func TestOpsErrorLogQueueByteBudget(t *testing.T) {
	previousBytes := opsErrorLogQueueBytes.Load()
	previousLen := opsErrorLogQueueLen.Load()
	opsErrorLogQueueBytes.Store(0)
	opsErrorLogQueueLen.Store(0)
	t.Cleanup(func() {
		opsErrorLogQueueBytes.Store(previousBytes)
		opsErrorLogQueueLen.Store(previousLen)
	})

	if !reserveOpsErrorLogQueueBytes(opsErrorLogMaxQueueBytes - 1) {
		t.Fatal("first reservation within byte budget should succeed")
	}
	if reserveOpsErrorLogQueueBytes(2) {
		t.Fatal("reservation beyond byte budget should be rejected")
	}
	if got := OpsErrorLogQueueBytes(); got != opsErrorLogMaxQueueBytes-1 {
		t.Fatalf("queued bytes = %d, want %d", got, opsErrorLogMaxQueueBytes-1)
	}
	if got := OpsErrorLogQueueLength(); got != 1 {
		t.Fatalf("queue length = %d, want 1", got)
	}
}

func TestEstimateOpsErrorLogJobBytesIncludesVariablePayloads(t *testing.T) {
	base := estimateOpsErrorLogJobBytes(&service.OpsInsertErrorLogInput{})
	message := "upstream message"
	detail := "upstream detail"
	events := `[{"error":"x"}]`
	entry := &service.OpsInsertErrorLogInput{
		ErrorBody:            strings.Repeat("x", 1024),
		ErrorMessage:         "client error",
		UserAgent:            "test-agent",
		UpstreamErrorMessage: &message,
		UpstreamErrorDetail:  &detail,
		UpstreamErrorsJSON:   &events,
	}
	if got := estimateOpsErrorLogJobBytes(entry); got <= base+1024 {
		t.Fatalf("estimated bytes = %d, expected variable payloads above %d", got, base+1024)
	}
}

func resetOpsErrorLoggerStateForTest(t *testing.T) {
	t.Helper()

	opsErrorLogMu.Lock()
	ch := opsErrorLogQueue
	opsErrorLogQueue = nil
	opsErrorLogStopping = true
	opsErrorLogMu.Unlock()

	if ch != nil {
		close(ch)
	}
	opsErrorLogWorkersWg.Wait()

	opsErrorLogOnce = sync.Once{}
	opsErrorLogStopOnce = sync.Once{}
	opsErrorLogWorkersWg = sync.WaitGroup{}
	opsErrorLogMu = sync.RWMutex{}
	opsErrorLogStopping = false

	opsErrorLogQueueLen.Store(0)
	opsErrorLogEnqueued.Store(0)
	opsErrorLogDropped.Store(0)
	opsErrorLogProcessed.Store(0)
	opsErrorLogSanitized.Store(0)
	opsErrorLogLastDropLogAt.Store(0)

	opsErrorLogShutdownCh = make(chan struct{})
	opsErrorLogShutdownOnce = sync.Once{}
	opsErrorLogDrained.Store(false)
}

func TestEnqueueOpsErrorLog_QueueFullDrop(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)

	// 禁止 enqueueOpsErrorLog 触发 workers，使用测试队列验证满队列降级。
	opsErrorLogOnce.Do(func() {})

	opsErrorLogMu.Lock()
	opsErrorLogQueue = make(chan opsErrorLogJob, 1)
	opsErrorLogMu.Unlock()

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	entry := &service.OpsInsertErrorLogInput{ErrorPhase: "upstream", ErrorType: "upstream_error"}

	enqueueOpsErrorLog(ops, entry)
	enqueueOpsErrorLog(ops, entry)

	require.Equal(t, int64(1), OpsErrorLogEnqueuedTotal())
	require.Equal(t, int64(1), OpsErrorLogDroppedTotal())
	require.Equal(t, int64(1), OpsErrorLogQueueLength())
}

func TestEnqueueOpsErrorLog_EarlyReturnBranches(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	entry := &service.OpsInsertErrorLogInput{ErrorPhase: "upstream", ErrorType: "upstream_error"}

	// nil 入参分支
	enqueueOpsErrorLog(nil, entry)
	enqueueOpsErrorLog(ops, nil)
	require.Equal(t, int64(0), OpsErrorLogEnqueuedTotal())

	// shutdown 分支
	close(opsErrorLogShutdownCh)
	enqueueOpsErrorLog(ops, entry)
	require.Equal(t, int64(0), OpsErrorLogEnqueuedTotal())

	// stopping 分支
	resetOpsErrorLoggerStateForTest(t)
	opsErrorLogMu.Lock()
	opsErrorLogStopping = true
	opsErrorLogMu.Unlock()
	enqueueOpsErrorLog(ops, entry)
	require.Equal(t, int64(0), OpsErrorLogEnqueuedTotal())

	// queue nil 分支（防止启动 worker 干扰）
	resetOpsErrorLoggerStateForTest(t)
	opsErrorLogOnce.Do(func() {})
	opsErrorLogMu.Lock()
	opsErrorLogQueue = nil
	opsErrorLogMu.Unlock()
	enqueueOpsErrorLog(ops, entry)
	require.Equal(t, int64(0), OpsErrorLogEnqueuedTotal())
}

func TestOpsCaptureWriterPool_ResetOnRelease(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	writer := acquireOpsCaptureWriter(c.Writer)
	require.NotNil(t, writer)
	c.Writer.WriteHeader(http.StatusInternalServerError)
	_, err := writer.WriteString("temp-error-body")
	require.NoError(t, err)
	require.NotEmpty(t, writer.capturedBytes())

	releaseOpsCaptureWriter(writer)

	reused := acquireOpsCaptureWriter(c.Writer)
	defer releaseOpsCaptureWriter(reused)

	require.Empty(t, reused.capturedBytes(), "writer should be reset before reuse")
}

func TestOpsCaptureWriterPool_DropsLargeBuffers(t *testing.T) {
	state := &opsCaptureWriterState{}
	state.buf.Grow(opsCaptureWriterPoolMaxRetainedCapacity + 1)
	require.False(t, shouldPoolOpsCaptureWriterState(state))
}

func TestEnqueueOpsErrorLog_SanitizesAndBoundsBodyBeforeQueue(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 1)
	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	secret := strings.Repeat("s", service.OpsErrorLogQueueBodyMaxBytes)
	entry := &service.OpsInsertErrorLogInput{
		ErrorPhase: "request",
		ErrorType:  "api_error",
		ErrorBody:  `{"authorization":"Bearer ` + secret + `","message":"failed"}`,
	}

	enqueueOpsErrorLog(ops, entry)
	job := <-opsErrorLogQueue
	require.LessOrEqual(t, len(job.entry.ErrorBody), service.OpsErrorLogQueueBodyMaxBytes)
	require.NotContains(t, job.entry.ErrorBody, secret)
	require.Equal(t, int64(1), OpsErrorLogSanitizedTotal())
}

func TestOpsErrorLoggerMiddleware_DoesNotBreakOuterMiddlewares(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(middleware2.Recovery())
	r.Use(middleware2.RequestLogger())
	r.Use(middleware2.Logger())
	r.GET("/v1/messages", OpsErrorLoggerMiddleware(nil), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)

	require.NotPanics(t, func() {
		r.ServeHTTP(rec, req)
	})
	require.Equal(t, http.StatusNoContent, rec.Code)
}

// setupOpsErrorLogTestQueue 阻止 enqueueOpsErrorLog 启动真实 worker，改用可检查的测试队列。
func setupOpsErrorLogTestQueue(t *testing.T, size int) {
	t.Helper()
	resetOpsErrorLoggerStateForTest(t)
	opsErrorLogOnce.Do(func() {})
	opsErrorLogMu.Lock()
	opsErrorLogQueue = make(chan opsErrorLogJob, size)
	opsErrorLogMu.Unlock()
}

func TestOpsErrorLoggerMiddleware_HardSkipsIngressRejection(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 4)
	gin.SetMode(gin.TestMode)

	settings := &ingressRejectSettingRepo{}
	repo := &ingressRejectOpsRepo{}
	ops := service.NewOpsService(repo, settings, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	// Construction may read unrelated runtime settings; only request-path reads matter here.
	settings.getValueCalls = 0

	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.GET("/v1/messages", func(c *gin.Context) {
		middleware2.MarkIngressRejected(c, middleware2.IngressRejectInvalidAPIKey)
		c.JSON(http.StatusUnauthorized, gin.H{"code": "INVALID_API_KEY", "message": "Invalid API key"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.JSONEq(t, `{"code":"INVALID_API_KEY","message":"Invalid API key"}`, w.Body.String())
	require.Zero(t, settings.getValueCalls, "ingress rejection must bypass monitoring settings reads")
	require.Zero(t, repo.insertCalls, "ingress rejection must bypass inserts")
	require.Zero(t, OpsErrorLogEnqueuedTotal(), "ingress rejection must not enter the error queue")
}

func TestOpsErrorLoggerMiddleware_DedicatedCyberSessionBlockRecordsExactlyOnce(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 3)
	gin.SetMode(gin.TestMode)
	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := &OpenAIGatewayHandler{opsService: ops}
	apiKey := &service.APIKey{ID: 41, Key: "sk-dedicated-test"}
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.POST("/v1/responses", func(c *gin.Context) {
		h.enqueueCyberSessionBlockedOpsEntry(c, apiKey, "gpt-test", "session-block-hash")
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
			"type": "permission_error", "code": "session_blocked_by_cyber_policy", "message": "blocked",
		}})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Equal(t, int64(1), OpsErrorLogQueueLength())
	job := <-opsErrorLogQueue
	require.Equal(t, "cyber_policy_session_blocked", job.entry.ErrorType)
	require.Equal(t, http.StatusForbidden, job.entry.StatusCode)
}

func TestOpsErrorLoggerMiddleware_OrdinaryPermissionStillRecords(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 2)
	gin.SetMode(gin.TestMode)
	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.POST("/v1/responses", func(c *gin.Context) {
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
			"type": "permission_error", "code": "permission_denied", "message": "denied",
		}})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))

	require.Equal(t, int64(1), OpsErrorLogQueueLength())
	job := <-opsErrorLogQueue
	require.Equal(t, "permission_error", job.entry.ErrorType)
	require.Equal(t, http.StatusForbidden, job.entry.StatusCode)
}

func TestOpsErrorLoggerMiddleware_RecordsRecoveredUpstreamTelemetryOutsideFailureSLA(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 2)
	gin.SetMode(gin.TestMode)

	repo := &ingressRejectOpsRepo{}
	ops := service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.POST("/v1/responses", func(c *gin.Context) {
		c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{{
			UpstreamStatusCode: http.StatusTooManyRequests,
			Message:            "earlier attempt was rate limited",
		}})
		c.JSON(http.StatusOK, gin.H{"status": "completed"})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(1), OpsErrorLogQueueLength())
	job := <-opsErrorLogQueue
	require.Nil(t, job.entry.UpstreamErrors, "raw attempts must be released before async queueing")
	require.NotNil(t, job.entry.UpstreamErrorsJSON)
	queuedEvents, err := service.ParseOpsUpstreamErrors(*job.entry.UpstreamErrorsJSON)
	require.NoError(t, err)
	require.Len(t, queuedEvents, 1)
	require.Equal(t, http.StatusTooManyRequests, queuedEvents[0].UpstreamStatusCode)

	flushOpsErrorLogBatch([]opsErrorLogJob{job})
	require.Equal(t, 1, repo.insertCalls)
	require.Len(t, repo.entries, 1)
	persisted := repo.entries[0]
	require.Equal(t, http.StatusOK, persisted.StatusCode, "recovered telemetry must remain outside failed-request SLA")
	require.Equal(t, "upstream", persisted.ErrorPhase)
	require.Equal(t, "upstream_error", persisted.ErrorType)
	require.Equal(t, "Recovered upstream error 429: earlier attempt was rate limited", persisted.ErrorMessage)
	require.NotNil(t, persisted.UpstreamErrorsJSON)
	persistedEvents, err := service.ParseOpsUpstreamErrors(*persisted.UpstreamErrorsJSON)
	require.NoError(t, err)
	require.Len(t, persistedEvents, 1)
	require.Equal(t, http.StatusTooManyRequests, persistedEvents[0].UpstreamStatusCode)
}

func TestOpsErrorLoggerMiddleware_RecoveredTelemetryFiltersSkipMonitoringAttempts(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 2)
	gin.SetMode(gin.TestMode)

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.POST("/v1/responses", func(c *gin.Context) {
		c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{
			{UpstreamStatusCode: http.StatusTooManyRequests, Message: "visible retry"},
			{UpstreamStatusCode: http.StatusBadGateway, Message: "hidden retry", SkipMonitoring: true},
		})
		c.JSON(http.StatusOK, gin.H{"status": "completed"})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))

	require.Equal(t, int64(1), OpsErrorLogQueueLength())
	job := <-opsErrorLogQueue
	require.Equal(t, "Recovered upstream error 429: visible retry", job.entry.ErrorMessage)
	require.NotNil(t, job.entry.UpstreamErrorsJSON)
	events, err := service.ParseOpsUpstreamErrors(*job.entry.UpstreamErrorsJSON)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "visible retry", events[0].Message)
}

func TestOpsErrorLoggerMiddleware_RecoveredTelemetrySkipsAllHiddenAttempts(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 2)
	gin.SetMode(gin.TestMode)

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.POST("/v1/responses", func(c *gin.Context) {
		c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{{
			UpstreamStatusCode: http.StatusTooManyRequests,
			Message:            "hidden retry",
			SkipMonitoring:     true,
		}})
		c.JSON(http.StatusOK, gin.H{"status": "completed"})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))

	require.Equal(t, int64(0), OpsErrorLogQueueLength())
}

func TestOpsErrorLoggerMiddleware_IntermediateSkipMonitoringDoesNotHideFinalVisibleFailure(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 2)
	gin.SetMode(gin.TestMode)
	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.POST("/v1/responses", func(c *gin.Context) {
		c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{
			{UpstreamStatusCode: http.StatusBadGateway, Message: "hidden retry", SkipMonitoring: true},
			{UpstreamStatusCode: http.StatusServiceUnavailable, Message: "visible final"},
		})
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"type": "upstream_error", "message": "visible final"}})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))
	require.Equal(t, int64(1), OpsErrorLogQueueLength())
	job := <-opsErrorLogQueue
	require.Equal(t, http.StatusServiceUnavailable, job.entry.StatusCode)
	require.Equal(t, "visible final", job.entry.ErrorMessage)
}

func TestOpsErrorLoggerMiddleware_CapturesSplitResponsesFailedSSE(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 2)
	gin.SetMode(gin.TestMode)

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.POST("/v1/responses", func(c *gin.Context) {
		setOpsRequestContext(c, "gpt-5.5", true)
		c.Status(http.StatusOK)
		_, _ = c.Writer.Write([]byte("event: response."))
		_, _ = c.Writer.Write([]byte("failed\n"))
		_, _ = c.Writer.Write([]byte(`data: {"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","message":"Too many pending requests"}}}`))
		_, _ = c.Writer.Write([]byte("\n\n"))
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(1), OpsErrorLogQueueLength())
	job := <-opsErrorLogQueue
	require.Equal(t, http.StatusTooManyRequests, job.entry.StatusCode)
	require.Equal(t, "rate_limit_error", job.entry.ErrorType)
	require.Contains(t, job.entry.ErrorMessage, "Too many pending requests")
}

func TestOpsCaptureWriter_CapturesSplitDataOnlyTerminalMarkers(t *testing.T) {
	tests := []struct {
		name      string
		prefix    string
		suffix    string
		wantType  string
		wantCode  string
		wantError string
	}{
		{
			name:      "response failed with space",
			prefix:    `data: {"type":"response.`,
			suffix:    `failed","response":{"error":{"code":"server_is_overloaded","message":"busy"}}}`,
			wantType:  "overloaded_error",
			wantCode:  "server_is_overloaded",
			wantError: "busy",
		},
		{
			name:      "response failed without space",
			prefix:    `data:{"type":"response.`,
			suffix:    `failed","error":{"code":"rate_limit_exceeded","message":"slow down"}}`,
			wantType:  "rate_limit_error",
			wantCode:  "rate_limit_exceeded",
			wantError: "slow down",
		},
		{
			name:      "error with space",
			prefix:    `data: {"type":"er`,
			suffix:    `ror","error":{"type":"invalid_request_error","code":"invalid_request","message":"bad input"}}`,
			wantType:  "invalid_request_error",
			wantCode:  "invalid_request",
			wantError: "bad input",
		},
		{
			name:      "error without space",
			prefix:    `data:{"type":"er`,
			suffix:    `ror","error":{"type":"authentication_error","code":"authentication_failed","message":"sign in"}}`,
			wantType:  "authentication_error",
			wantCode:  "authentication_failed",
			wantError: "sign in",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &opsCaptureWriterState{limit: opsCaptureWriterLimit}
			state.captureResponseChunk([]byte(tt.prefix), http.StatusOK)
			require.Empty(t, state.buf.Bytes(), "partial frame must remain in the bounded probe")
			state.captureResponseChunk([]byte(tt.suffix+"\n\n"), http.StatusOK)

			parsed := parseOpsErrorResponse(state.buf.Bytes())
			require.True(t, state.sseCapturing)
			require.True(t, parsed.StreamFailure)
			require.Equal(t, tt.wantType, parsed.ErrorType)
			require.Equal(t, tt.wantCode, parsed.Code)
			require.Equal(t, tt.wantError, parsed.Message)
			require.LessOrEqual(t, len(state.probe), opsTerminalSSEFrameProbeLimit)
		})
	}
}

func TestOpsErrorLoggerMiddleware_StreamFailureUsesTerminalErrorOverAttemptContext(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 2)
	gin.SetMode(gin.TestMode)

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.POST("/v1/responses", func(c *gin.Context) {
		service.SetOpsUpstreamError(c, http.StatusBadGateway, "Upstream transport error", "earlier attempt failed")
		c.Status(http.StatusOK)
		_, _ = c.Writer.WriteString("event: er")
		_, _ = c.Writer.WriteString("ror\n")
		_, _ = c.Writer.WriteString(`data: {"type":"error","error":{"type":"invalid_request_error","code":"context_length_exceeded","message":"input exceeds the context window"}}`)
		_, _ = c.Writer.WriteString("\n\n")
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(1), OpsErrorLogQueueLength())
	job := <-opsErrorLogQueue
	require.Equal(t, http.StatusBadRequest, job.entry.StatusCode)
	require.Equal(t, "invalid_request_error", job.entry.ErrorType)
	require.NotNil(t, job.entry.UpstreamStatusCode)
	require.Equal(t, http.StatusBadRequest, *job.entry.UpstreamStatusCode)
	require.NotNil(t, job.entry.UpstreamErrorMessage)
	require.Equal(t, "input exceeds the context window", *job.entry.UpstreamErrorMessage)
}

func TestOpsErrorLoggerMiddleware_PrefersContextRequestID(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 2)
	gin.SetMode(gin.TestMode)

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.POST("/v1/responses", func(c *gin.Context) {
		c.Header("X-Request-Id", "response-header-id")
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "invalid_request_error", "message": "bad input"}})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	request = request.WithContext(context.WithValue(request.Context(), ctxkey.RequestID, "context-request-id"))
	router.ServeHTTP(recorder, request)

	require.Equal(t, int64(1), OpsErrorLogQueueLength())
	job := <-opsErrorLogQueue
	require.Equal(t, "context-request-id", job.entry.RequestID)
}

func TestNormalizeOpsPersistentUserAgentBoundsAndPreservesUTF8(t *testing.T) {
	value := strings.Repeat("a", opsErrorLogMaxUserAgentBytes-1) + "你" + strings.Repeat("b", 32)
	got := normalizeOpsPersistentUserAgent("  " + value + "  ")
	require.LessOrEqual(t, len(got), opsErrorLogMaxUserAgentBytes)
	require.True(t, utf8.ValidString(got))
	require.NotContains(t, got, "b")
}

// 就地(in-band) SSE 错误挂在已固化的 HTTP 200 流上：wire 状态码为 200，
// 常规 status>=400 采集路径不会触发。logOpsStreamError 必须据 MarkOpsStreamError
// 补记一条错误日志，且用 IntendedStatus(429) 分级、StatusCode 仍记 wire 的 200。
func TestLogOpsStreamError_RecordsInBandConcurrencyLimit(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 4)

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Set(opsModelKey, "test-model")

	service.MarkOpsStreamError(c, "rate_limit_error",
		"Concurrency limit exceeded for account, please retry later", http.StatusTooManyRequests)

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	logOpsStreamError(c, ops, http.StatusOK)

	require.Equal(t, int64(1), OpsErrorLogEnqueuedTotal())
	require.Equal(t, int64(1), OpsErrorLogQueueLength())

	job := <-opsErrorLogQueue
	require.NotNil(t, job.entry)
	require.Equal(t, "rate_limit_error", job.entry.ErrorType)
	require.Equal(t, "request", job.entry.ErrorPhase)
	require.True(t, job.entry.IsBusinessLimited)
	require.True(t, job.entry.Stream)
	require.Equal(t, http.StatusOK, job.entry.StatusCode) // wire 状态码保持 200
	require.Equal(t, "P1", job.entry.Severity)            // 用 IntendedStatus 429 分级
	require.Equal(t, "test-model", job.entry.Model)
	require.Equal(t, "Concurrency limit exceeded for account, please retry later", job.entry.ErrorMessage)
}

func TestLogOpsStreamError_UpstreamFailureCountsTowardsSLA(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 4)

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(opsModelKey, "gpt-5.6-sol")

	service.MarkOpsStreamFailure(
		c,
		"upstream_error",
		service.OpenAIUpstreamHTTP2StreamErrorCode,
		"Upstream HTTP/2 stream failed",
		http.StatusBadGateway,
	)

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	logOpsStreamError(c, ops, http.StatusOK)

	job := <-opsErrorLogQueue
	require.NotNil(t, job.entry)
	require.Equal(t, http.StatusBadGateway, job.entry.StatusCode)
	require.Equal(t, "upstream_error", job.entry.ErrorType)
	require.Equal(t, "upstream", job.entry.ErrorPhase)
	require.Equal(t, "provider", job.entry.ErrorOwner)
	require.False(t, job.entry.IsBusinessLimited)
	require.Contains(t, job.entry.ErrorBody, service.OpenAIUpstreamHTTP2StreamErrorCode)
}

// 未标记流内错误时 logOpsStreamError 必须是 no-op（不误记正常的 200 流）。
func TestLogOpsStreamError_NoopWhenNotMarked(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 4)

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	logOpsStreamError(c, ops, http.StatusOK)

	require.Equal(t, int64(0), OpsErrorLogEnqueuedTotal())
}

// 命中 skip_monitoring=true 透传规则时不落库，与其它采集分支一致。
func TestLogOpsStreamError_SkipWhenPassthroughSkipMonitoring(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 4)

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	service.MarkOpsStreamError(c, "upstream_error", "Upstream request failed", http.StatusBadGateway)
	c.Set(service.OpsSkipPassthroughKey, true)

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	logOpsStreamError(c, ops, http.StatusOK)

	require.Equal(t, int64(0), OpsErrorLogEnqueuedTotal())
}

func TestShouldSkipFinalOpsFailureUsesOnlyFinalAttemptRule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{
		{UpstreamStatusCode: http.StatusBadGateway, Message: "hidden intermediate", SkipMonitoring: true},
		{UpstreamStatusCode: http.StatusServiceUnavailable, Message: "visible final"},
	})
	require.False(t, shouldSkipFinalOpsFailure(c))

	c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{
		{UpstreamStatusCode: http.StatusBadGateway, Message: "visible intermediate"},
		nil,
		{UpstreamStatusCode: http.StatusServiceUnavailable, Message: "hidden final", SkipMonitoring: true},
	})
	require.True(t, shouldSkipFinalOpsFailure(c))
}

// MarkOpsStreamError 采用「首个标记生效」：后续的通用兜底帧不得覆盖根因错误。
func TestMarkOpsStreamError_FirstWins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	service.MarkOpsStreamError(c, "rate_limit_error", "Concurrency limit exceeded for account", http.StatusTooManyRequests)
	service.MarkOpsStreamError(c, "upstream_error", "Upstream request failed", http.StatusBadGateway)

	se, ok := service.GetOpsStreamError(c)
	require.True(t, ok)
	require.Equal(t, "rate_limit_error", se.ErrType)
	require.Equal(t, "Concurrency limit exceeded for account", se.Message)
	require.Equal(t, http.StatusTooManyRequests, se.IntendedStatus)
}

func TestLogOpsStreamError_RecordsOneFailurePerWebSocketTurn(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 4)
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	service.SetOpenAIClientTransport(c, service.OpenAIClientTransportWS)

	service.BeginOpsStreamTurn(c, 1)
	service.MarkOpsStreamFailure(c, "rate_limit_error", "rate_limit_exceeded", "turn one failed", http.StatusTooManyRequests)
	service.MarkOpsStreamError(c, "upstream_error", "generic duplicate for turn one", http.StatusBadGateway)
	service.BeginOpsStreamTurn(c, 2)
	service.MarkOpsStreamFailure(c, "permission_error", "permission_denied", "turn two failed", http.StatusForbidden)

	streamErrors := service.GetOpsStreamErrors(c)
	require.Len(t, streamErrors, 2)
	require.Equal(t, 1, streamErrors[0].Turn)
	require.Equal(t, 2, streamErrors[1].Turn)
	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	logOpsStreamError(c, ops, http.StatusSwitchingProtocols)

	require.Equal(t, int64(2), OpsErrorLogQueueLength())
	first := <-opsErrorLogQueue
	second := <-opsErrorLogQueue
	require.Equal(t, "turn one failed", first.entry.ErrorMessage)
	require.Equal(t, http.StatusTooManyRequests, first.entry.StatusCode)
	require.Equal(t, "turn two failed", second.entry.ErrorMessage)
	require.Equal(t, http.StatusForbidden, second.entry.StatusCode)
}

func TestIsKnownOpsErrorType(t *testing.T) {
	known := []string{
		"invalid_request_error",
		"authentication_error",
		"rate_limit_error",
		"billing_error",
		"subscription_error",
		"upstream_error",
		"overloaded_error",
		"api_error",
		"not_found_error",
		"forbidden_error",
	}
	for _, k := range known {
		require.True(t, isKnownOpsErrorType(k), "expected known: %s", k)
	}

	unknown := []string{"<nil>", "null", "", "random_error", "some_new_type", "<nil>\u003e"}
	for _, u := range unknown {
		require.False(t, isKnownOpsErrorType(u), "expected unknown: %q", u)
	}
}

func TestNormalizeOpsErrorType(t *testing.T) {
	tests := []struct {
		name    string
		errType string
		code    string
		want    string
	}{
		// Known types pass through.
		{"known invalid_request_error", "invalid_request_error", "", "invalid_request_error"},
		{"known rate_limit_error", "rate_limit_error", "", "rate_limit_error"},
		{"known upstream_error", "upstream_error", "", "upstream_error"},

		// Unknown/garbage types are rejected and fall through to code-based or default.
		{"nil literal from upstream", "<nil>", "", "api_error"},
		{"null string", "null", "", "api_error"},
		{"random string", "something_weird", "", "api_error"},

		// Unknown type but known code still maps correctly.
		{"nil with INSUFFICIENT_BALANCE code", "<nil>", "INSUFFICIENT_BALANCE", "billing_error"},
		{"nil with USAGE_LIMIT_EXCEEDED code", "<nil>", "USAGE_LIMIT_EXCEEDED", "subscription_error"},

		// Empty type falls through to code-based mapping.
		{"empty type with balance code", "", "INSUFFICIENT_BALANCE", "billing_error"},
		{"empty type with subscription code", "", "SUBSCRIPTION_NOT_FOUND", "subscription_error"},
		{"empty type no code", "", "", "api_error"},

		// Known type overrides conflicting code-based mapping.
		{"known type overrides conflicting code", "rate_limit_error", "INSUFFICIENT_BALANCE", "rate_limit_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeOpsErrorType(tt.errType, tt.code)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestClassifyOpsNoAvailableAccountsExcludedFromSLA(t *testing.T) {
	const message = "No available accounts"
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	markOpsRoutingCapacityLimited(c)

	errType := normalizeOpsErrorType("api_error", "")
	phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(c, errType, message, "", http.StatusServiceUnavailable)

	require.Equal(t, "api_error", errType)
	require.Equal(t, "routing", phase)
	require.True(t, isBusinessLimited)
	require.Equal(t, "platform", errorOwner)
	require.Equal(t, "gateway", errorSource)
}

func TestClassifyOpsRoutingCapacityMarkerExcludesMaskedSelectionFailureFromSLA(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	markOpsRoutingCapacityLimited(c)

	phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(
		c,
		"api_error",
		"Service temporarily unavailable",
		"",
		http.StatusServiceUnavailable,
	)

	require.Equal(t, "routing", phase)
	require.True(t, isBusinessLimited)
	require.Equal(t, "platform", errorOwner)
	require.Equal(t, "gateway", errorSource)
}

func TestClassifyOpsLocalModelConfigurationRejection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalModelConfiguration)

	phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(
		c,
		"model_not_found",
		"Model \"gpt-missing\" is not supported by any configured account in this group",
		"",
		http.StatusNotFound,
	)

	require.Equal(t, "routing", phase)
	require.True(t, isBusinessLimited)
	require.Equal(t, "platform", errorOwner)
	require.Equal(t, "gateway", errorSource)
}

func TestClassifyOpsLocalModelConfigurationOverridesStaleUpstreamMarkers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalModelConfiguration)
	c.Set(service.OpsUpstreamStatusCodeKey, http.StatusUnauthorized)
	c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{{
		Stage:              string(service.GatewayFailureStageAccountAuth),
		UpstreamStatusCode: http.StatusUnauthorized,
	}})

	phase, limited, owner, source := classifyOpsErrorLog(c, "model_not_found", "unsupported configured model", "", http.StatusNotFound)

	require.Equal(t, "routing", phase)
	require.True(t, limited)
	require.Equal(t, "platform", owner)
	require.Equal(t, "gateway", source)
}

func TestClassifyOpsLocalModelConfigurationRequiresMarkerAndReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(service.OpsClientBusinessLimitedReasonKey, service.OpsClientBusinessLimitedReasonLocalModelConfiguration)
	c.Set(service.OpsUpstreamStatusCodeKey, http.StatusBadGateway)

	phase, limited, owner, source := classifyOpsErrorLog(c, "upstream_error", "provider failed", "", http.StatusBadGateway)

	require.Equal(t, "upstream", phase)
	require.False(t, limited)
	require.Equal(t, "provider", owner)
	require.Equal(t, "upstream_http", source)
}

func TestOpsErrorLoggerMiddleware_LocalModelConfigurationFields(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 1)
	gin.SetMode(gin.TestMode)

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalModelConfiguration)
		c.Set(opsAccountIDKey, int64(99))
		c.Set(opsUpstreamModelKey, "stale-upstream-model")
		setActualUpstreamEndpoint(c, "/v1/chat/completions")
		c.Set(service.OpsUpstreamStatusCodeKey, http.StatusUnauthorized)
		c.Set(service.OpsUpstreamErrorMessageKey, "stale upstream error")
		c.Set(service.OpsUpstreamErrorDetailKey, "stale upstream detail")
		c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{{
			Stage:              string(service.GatewayFailureStageAccountAuth),
			UpstreamStatusCode: http.StatusUnauthorized,
			Message:            "stale auth failure",
		}})
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"type":    "model_not_found",
				"message": "Model \"gpt-missing\" is not supported by any configured account in this group",
			},
		})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	require.JSONEq(t, `{"error":{"type":"model_not_found","message":"Model \"gpt-missing\" is not supported by any configured account in this group"}}`, w.Body.String())
	job := <-opsErrorLogQueue
	require.Equal(t, http.StatusNotFound, job.entry.StatusCode)
	require.Equal(t, "routing", job.entry.ErrorPhase)
	require.True(t, job.entry.IsBusinessLimited)
	require.Equal(t, "platform", job.entry.ErrorOwner)
	require.Equal(t, "gateway", job.entry.ErrorSource)
	require.Nil(t, job.entry.AccountID)
	require.Nil(t, job.entry.UpstreamStatusCode)
	require.Nil(t, job.entry.UpstreamErrors)
	require.Nil(t, job.entry.UpstreamErrorMessage)
	require.Nil(t, job.entry.UpstreamErrorDetail)
	require.Empty(t, job.entry.UpstreamModel)
	require.Empty(t, job.entry.UpstreamEndpoint)
}

func TestClassifyOpsAuthClientErrorsExcludedFromSLA(t *testing.T) {
	tests := []struct {
		name    string
		errType string
		message string
		code    string
		status  int
	}{
		{
			name:    "standard invalid API key",
			errType: "api_error",
			message: "Invalid API key",
			code:    "INVALID_API_KEY",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "standard missing API key",
			errType: "api_error",
			message: "API key is required in Authorization header (Bearer scheme), x-api-key header, or x-goog-api-key header",
			code:    "API_KEY_REQUIRED",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "expired local API key",
			errType: "api_error",
			message: "API key 已过期",
			code:    "API_KEY_EXPIRED",
			status:  http.StatusForbidden,
		},
		{
			name:    "disabled local API key",
			errType: "api_error",
			message: "API key is disabled",
			code:    "API_KEY_DISABLED",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "local API key user missing",
			errType: "api_error",
			message: "User associated with API key not found",
			code:    "USER_NOT_FOUND",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "inactive local API key user",
			errType: "api_error",
			message: "User account is not active",
			code:    "USER_INACTIVE",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "deleted local API key group",
			errType: "api_error",
			message: "API Key 所属分组已删除",
			code:    "GROUP_DELETED",
			status:  http.StatusForbidden,
		},
		{
			name:    "disabled local API key group",
			errType: "api_error",
			message: "API Key 所属分组已停用",
			code:    "GROUP_DISABLED",
			status:  http.StatusForbidden,
		},
		{
			name:    "google deleted API key group message without semantic code",
			errType: "api_error",
			message: "API Key 所属分组已删除",
			code:    "403",
			status:  http.StatusForbidden,
		},
		{
			name:    "anthropic unassigned API key group",
			errType: "permission_error",
			message: "API Key is not assigned to any group and cannot be used. Please contact the administrator to assign it to a group.",
			code:    "",
			status:  http.StatusForbidden,
		},
		{
			name:    "google invalid API key",
			errType: "api_error",
			message: "Invalid API key",
			code:    "401",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "google missing API key",
			errType: "api_error",
			message: "API key is required",
			code:    "401",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "google disabled API key",
			errType: "api_error",
			message: "API key is disabled",
			code:    "401",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "google local API key user missing",
			errType: "api_error",
			message: "User associated with API key not found",
			code:    "401",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "google inactive local API key user",
			errType: "api_error",
			message: "User account is not active",
			code:    "401",
			status:  http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)

			errType := normalizeOpsErrorType(tt.errType, tt.code)
			phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(c, errType, tt.message, tt.code, tt.status)

			wantErrType := "api_error"
			if tt.errType == "permission_error" {
				wantErrType = "permission_error"
			}
			require.Equal(t, wantErrType, errType)
			require.Equal(t, "auth", phase)
			require.True(t, isBusinessLimited)
			require.Equal(t, "client", errorOwner)
			require.Equal(t, "client_request", errorSource)
		})
	}
}

func TestClassifyOpsLocalBusinessLimitErrorsExcludedFromSLA(t *testing.T) {
	tests := []struct {
		name        string
		errType     string
		message     string
		code        string
		status      int
		wantErrType string
		wantPhase   string
	}{
		{
			name:        "standard API key quota exhausted",
			errType:     "api_error",
			message:     "API key 额度已用完",
			code:        "API_KEY_QUOTA_EXHAUSTED",
			status:      http.StatusTooManyRequests,
			wantErrType: "api_error",
			wantPhase:   "request",
		},
		{
			name:        "standard query API key deprecated",
			errType:     "api_error",
			message:     "API key in query parameter is deprecated. Please use Authorization header instead.",
			code:        "api_key_in_query_deprecated",
			status:      http.StatusBadRequest,
			wantErrType: "api_error",
			wantPhase:   "request",
		},
		{
			name:        "google query API key deprecated",
			errType:     "api_error",
			message:     "Query parameter api_key is deprecated. Use Authorization header or key instead.",
			code:        "400",
			status:      http.StatusBadRequest,
			wantErrType: "api_error",
			wantPhase:   "request",
		},
		{
			name:        "google no active subscription",
			errType:     "api_error",
			message:     "No active subscription found for this group",
			code:        "403",
			status:      http.StatusForbidden,
			wantErrType: "api_error",
			wantPhase:   "request",
		},
		{
			name:        "gateway subscription invalid cache recheck",
			errType:     "billing_error",
			message:     "subscription is invalid or expired",
			code:        "billing_error",
			status:      http.StatusForbidden,
			wantErrType: "billing_error",
			wantPhase:   "request",
		},
		{
			name:        "google insufficient account balance",
			errType:     "api_error",
			message:     "Insufficient account balance",
			code:        "403",
			status:      http.StatusForbidden,
			wantErrType: "api_error",
			wantPhase:   "request",
		},
		{
			name:        "gateway billing cache insufficient balance",
			errType:     "billing_error",
			message:     "insufficient balance",
			code:        "",
			status:      http.StatusForbidden,
			wantErrType: "billing_error",
			wantPhase:   "request",
		},
		{
			name:        "gemini group platform mismatch",
			errType:     "api_error",
			message:     "API key group platform is not gemini",
			code:        "400",
			status:      http.StatusBadRequest,
			wantErrType: "api_error",
			wantPhase:   "request",
		},
		{
			name:        "gateway API key 5h rate limit",
			errType:     "api_error",
			message:     "api key 5小时限额已用完",
			code:        "rate_limit_exceeded",
			status:      http.StatusTooManyRequests,
			wantErrType: "api_error",
			wantPhase:   "request",
		},
		{
			name:        "gateway group RPM limit",
			errType:     "api_error",
			message:     "group requests-per-minute limit exceeded",
			code:        "rate_limit_exceeded",
			status:      http.StatusTooManyRequests,
			wantErrType: "api_error",
			wantPhase:   "request",
		},
		{
			name:        "google subscription daily limit",
			errType:     "api_error",
			message:     "daily usage limit exceeded",
			code:        "429",
			status:      http.StatusTooManyRequests,
			wantErrType: "api_error",
			wantPhase:   "request",
		},
		{
			name:        "user platform daily quota exhausted",
			errType:     "api_error",
			message:     "Daily usage quota exhausted for this platform.",
			code:        "rate_limit_exceeded",
			status:      http.StatusTooManyRequests,
			wantErrType: "api_error",
			wantPhase:   "request",
		},
		{
			name:        "local pending queue limit",
			errType:     "rate_limit_error",
			message:     "Too many pending requests, please retry later",
			code:        "",
			status:      http.StatusTooManyRequests,
			wantErrType: "rate_limit_error",
			wantPhase:   "request",
		},
		{
			name:        "local concurrency limit",
			errType:     "rate_limit_error",
			message:     "Concurrency limit exceeded for user, please retry later",
			code:        "",
			status:      http.StatusTooManyRequests,
			wantErrType: "rate_limit_error",
			wantPhase:   "request",
		},
		{
			name:        "group claude code only feature gate",
			errType:     "permission_error",
			message:     "This group is restricted to Claude Code clients (/v1/messages only)",
			code:        "",
			status:      http.StatusForbidden,
			wantErrType: "permission_error",
			wantPhase:   "request",
		},
		{
			name:        "group image generation feature gate",
			errType:     "permission_error",
			message:     "Image generation is not enabled for this group",
			code:        "",
			status:      http.StatusForbidden,
			wantErrType: "permission_error",
			wantPhase:   "request",
		},
		{
			name:        "route token counting platform unsupported",
			errType:     "not_found_error",
			message:     "Token counting is not supported for this platform",
			code:        "",
			status:      http.StatusNotFound,
			wantErrType: "not_found_error",
			wantPhase:   "request",
		},
		{
			name:        "route images API platform unsupported",
			errType:     "not_found_error",
			message:     "Images API is not supported for this platform",
			code:        "",
			status:      http.StatusNotFound,
			wantErrType: "not_found_error",
			wantPhase:   "request",
		},
		{
			name:        "antigravity model whitelist feature gate",
			errType:     "permission_error",
			message:     "model claude-3-5-sonnet not in whitelist",
			code:        "",
			status:      http.StatusForbidden,
			wantErrType: "permission_error",
			wantPhase:   "request",
		},
		{
			name:        "google antigravity model whitelist feature gate",
			errType:     "api_error",
			message:     "model gemini-2.5-pro not in whitelist",
			code:        "403",
			status:      http.StatusForbidden,
			wantErrType: "api_error",
			wantPhase:   "request",
		},
		{
			name:        "claude beta policy block",
			errType:     "invalid_request_error",
			message:     "beta feature interleaved-thinking-2025-05-14 is not allowed",
			code:        "",
			status:      http.StatusBadRequest,
			wantErrType: "invalid_request_error",
			wantPhase:   "request",
		},
		{
			name:        "openai fast policy block",
			errType:     "permission_error",
			message:     "openai service_tier=priority is not allowed for model gpt-5.5",
			code:        "",
			status:      http.StatusForbidden,
			wantErrType: "permission_error",
			wantPhase:   "request",
		},
		{
			name:        "codex official client policy block",
			errType:     "forbidden_error",
			message:     "This account only allows Codex official clients",
			code:        "",
			status:      http.StatusForbidden,
			wantErrType: "forbidden_error",
			wantPhase:   "request",
		},
		{
			name:        "openai wsv1 unsupported feature gate",
			errType:     "invalid_request_error",
			message:     "OpenAI WSv1 is temporarily unsupported. Please enable responses_websockets_v2.",
			code:        "",
			status:      http.StatusBadRequest,
			wantErrType: "invalid_request_error",
			wantPhase:   "request",
		},
		{
			name:        "openai passthrough instructions policy block",
			errType:     "forbidden_error",
			message:     "OpenAI codex passthrough requires a non-empty instructions field",
			code:        "",
			status:      http.StatusForbidden,
			wantErrType: "forbidden_error",
			wantPhase:   "request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)

			errType := normalizeOpsErrorType(tt.errType, tt.code)
			phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(c, errType, tt.message, tt.code, tt.status)

			require.Equal(t, tt.wantErrType, errType)
			require.Equal(t, tt.wantPhase, phase)
			require.True(t, isBusinessLimited)
			require.Equal(t, "client", errorOwner)
			require.Equal(t, "client_request", errorSource)
		})
	}
}

func TestClassifyOpsIPRestrictionAccessDeniedExcludedFromSLA(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonIPRestriction)

	errType := normalizeOpsErrorType("api_error", "ACCESS_DENIED")
	phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(c, errType, "Access denied", "ACCESS_DENIED", http.StatusForbidden)

	require.Equal(t, "api_error", errType)
	require.Equal(t, "auth", phase)
	require.True(t, isBusinessLimited)
	require.Equal(t, "client", errorOwner)
	require.Equal(t, "client_request", errorSource)
}

func TestClassifyOpsClientBusinessLimitedMarkerExcludesCustomPolicyDenialFromSLA(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalPolicyDenied)

	errType := normalizeOpsErrorType("invalid_request_error", "")
	phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(c, errType, "custom admin policy message", "", http.StatusBadRequest)

	require.Equal(t, "invalid_request_error", errType)
	require.Equal(t, "auth", phase)
	require.True(t, isBusinessLimited)
	require.Equal(t, "client", errorOwner)
	require.Equal(t, "client_request", errorSource)
}

func TestClassifyOpsOtherErrorsStillCountForSLA(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	errType := normalizeOpsErrorType("api_error", "INTERNAL_ERROR")
	phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(c, errType, "Failed to validate API key", "INTERNAL_ERROR", http.StatusInternalServerError)

	require.Equal(t, "api_error", errType)
	require.Equal(t, "internal", phase)
	require.False(t, isBusinessLimited)
	require.Equal(t, "platform", errorOwner)
	require.Equal(t, "gateway", errorSource)
}

func TestClassifyOpsUnsupportedModelExcludedFromSLA(t *testing.T) {
	tests := []string{
		"No available accounts: no available accounts supporting model: made-up-model",
		"No available accounts: no available OpenAI accounts supporting model: made-up-model",
		"No available Gemini accounts: no available Gemini accounts supporting model: made-up-model",
		"No available accounts: no available accounts supporting model: made-up-model (channel pricing restriction)",
	}

	for _, message := range tests {
		t.Run(message, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			markOpsRoutingCapacityLimited(c)

			errType := normalizeOpsErrorType("api_error", "")
			phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(c, errType, message, "", http.StatusServiceUnavailable)

			require.Equal(t, "api_error", errType)
			require.Equal(t, "routing", phase)
			require.True(t, isBusinessLimited)
			require.Equal(t, "platform", errorOwner)
			require.Equal(t, "gateway", errorSource)
		})
	}
}

func TestClassifyOpsUnmarkedNoAvailableTextStillCountsForSLA(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(
		c,
		"api_error",
		"No available accounts",
		"",
		http.StatusServiceUnavailable,
	)

	require.Equal(t, "routing", phase)
	require.False(t, isBusinessLimited)
	require.Equal(t, "platform", errorOwner)
	require.Equal(t, "gateway", errorSource)
}

func TestClassifyOpsUpstreamAuthTextStillCountsForSLA(t *testing.T) {
	tests := []struct {
		name    string
		message string
		code    string
		status  int
	}{
		{
			name:    "invalid API key",
			message: "Invalid API key",
			code:    "401",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "disabled API key",
			message: "API key is disabled",
			code:    "API_KEY_DISABLED",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "gemini group platform mismatch",
			message: "API key group platform is not gemini",
			code:    "400",
			status:  http.StatusBadRequest,
		},
		{
			name:    "provider balance error",
			message: "Insufficient account balance",
			code:    "INSUFFICIENT_BALANCE",
			status:  http.StatusForbidden,
		},
		{
			name:    "provider subscription error",
			message: "No active subscription found for this group",
			code:    "SUBSCRIPTION_NOT_FOUND",
			status:  http.StatusForbidden,
		},
		{
			name:    "provider quota error",
			message: "api key 额度已用完",
			code:    "API_KEY_QUOTA_EXHAUSTED",
			status:  http.StatusTooManyRequests,
		},
		{
			name:    "provider deleted group shaped error",
			message: "API Key 所属分组已删除",
			code:    "GROUP_DELETED",
			status:  http.StatusForbidden,
		},
		{
			name:    "provider unassigned group shaped error",
			message: "API Key is not assigned to any group and cannot be used. Please contact the administrator to assign it to a group.",
			code:    "403",
			status:  http.StatusForbidden,
		},
		{
			name:    "provider local quota shaped error",
			message: "Daily usage quota exhausted for this platform.",
			code:    "rate_limit_exceeded",
			status:  http.StatusTooManyRequests,
		},
		{
			name:    "provider feature gate shaped error",
			message: "Image generation is not enabled for this group",
			code:    "403",
			status:  http.StatusForbidden,
		},
		{
			name:    "provider token counting unsupported shaped error",
			message: "Token counting is not supported for this platform",
			code:    "404",
			status:  http.StatusNotFound,
		},
		{
			name:    "provider image API unsupported shaped error",
			message: "Images API is not supported for this platform",
			code:    "404",
			status:  http.StatusNotFound,
		},
		{
			name:    "provider antigravity whitelist shaped error",
			message: "model claude-3-5-sonnet not in whitelist",
			code:    "403",
			status:  http.StatusForbidden,
		},
		{
			name:    "provider beta policy shaped error",
			message: "beta feature interleaved-thinking-2025-05-14 is not allowed",
			code:    "400",
			status:  http.StatusBadRequest,
		},
		{
			name:    "provider openai fast policy shaped error",
			message: "openai service_tier=priority is not allowed for model gpt-5.5",
			code:    "403",
			status:  http.StatusForbidden,
		},
		{
			name:    "provider codex client policy shaped error",
			message: "This account only allows Codex official clients",
			code:    "403",
			status:  http.StatusForbidden,
		},
		{
			name:    "provider wsv1 unsupported shaped error",
			message: "OpenAI WSv1 is temporarily unsupported. Please enable responses_websockets_v2.",
			code:    "400",
			status:  http.StatusBadRequest,
		},
		{
			name:    "provider passthrough instructions shaped error",
			message: "OpenAI codex passthrough requires a non-empty instructions field",
			code:    "403",
			status:  http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			service.SetOpsUpstreamError(c, tt.status, tt.message, "")

			phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(
				c,
				"api_error",
				tt.message,
				tt.code,
				tt.status,
			)

			require.Equal(t, "upstream", phase)
			require.False(t, isBusinessLimited)
			require.Equal(t, "provider", errorOwner)
			require.Equal(t, "upstream_http", errorSource)
		})
	}
}

func TestClassifyOpsUpstreamNoAvailableTextStillCountsForSLA(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	service.SetOpsUpstreamError(c, http.StatusServiceUnavailable, "No available accounts", "")

	phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(
		c,
		"api_error",
		"No available accounts",
		"",
		http.StatusServiceUnavailable,
	)

	require.Equal(t, "upstream", phase)
	require.False(t, isBusinessLimited)
	require.Equal(t, "provider", errorOwner)
	require.Equal(t, "upstream_http", errorSource)
}

func TestParseOpsErrorResponsePreservesNestedStringCode(t *testing.T) {
	parsed := parseOpsErrorResponse([]byte(`{"error":{"type":"permission_error","code":"GROUP_DELETED","message":"API Key 所属分组已删除"}}`))

	require.Equal(t, "permission_error", parsed.ErrorType)
	require.Equal(t, "GROUP_DELETED", parsed.Code)
	require.Equal(t, "API Key 所属分组已删除", parsed.Message)
}

func TestParseOpsErrorResponsePreservesStructuredTopLevelSemantics(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantType string
		wantCode string
		wantMsg  string
	}{
		{
			name:     "model not found",
			body:     `{"type":"model_not_found","code":404,"message":"model unavailable"}`,
			wantType: "model_not_found",
			wantCode: "404",
			wantMsg:  "model unavailable",
		},
		{
			name:     "string error",
			body:     `{"type":"service_unavailable","code":"temporarily_unavailable","error":"capacity exhausted"}`,
			wantType: "service_unavailable",
			wantCode: "temporarily_unavailable",
			wantMsg:  "capacity exhausted",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := parseOpsErrorResponse([]byte(tt.body))
			require.Equal(t, tt.wantType, normalizeOpsErrorType(parsed.ErrorType, parsed.Code))
			require.Equal(t, tt.wantCode, parsed.Code)
			require.Equal(t, tt.wantMsg, parsed.Message)
		})
	}
}

func TestApplyOpsUpstreamFieldsUsesLastNonNilAttempt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	service.SetOpsUpstreamError(c, http.StatusUnauthorized, "stale context", "stale detail")
	c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{
		{UpstreamStatusCode: http.StatusTooManyRequests, Message: "first attempt", Detail: "first detail"},
		nil,
		{UpstreamStatusCode: http.StatusServiceUnavailable, Message: "final attempt", Detail: "final detail"},
		nil,
	})
	entry := &service.OpsInsertErrorLogInput{}

	applyOpsUpstreamFieldsFromContext(c, entry)

	require.NotNil(t, entry.UpstreamStatusCode)
	require.Equal(t, http.StatusServiceUnavailable, *entry.UpstreamStatusCode)
	require.NotNil(t, entry.UpstreamErrorMessage)
	require.Equal(t, "final attempt", *entry.UpstreamErrorMessage)
	require.NotNil(t, entry.UpstreamErrorDetail)
	require.Equal(t, "final detail", *entry.UpstreamErrorDetail)
	require.Len(t, entry.UpstreamErrors, 4)
}

func TestApplyOpsUpstreamFieldsFinalStatuslessAttemptClearsStaleContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	service.SetOpsUpstreamError(c, http.StatusBadGateway, "stale response", "stale body")
	c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{
		{UpstreamStatusCode: http.StatusBadGateway, Message: "first response"},
		{Kind: "request_error", Message: "final transport failure", Detail: "connection reset"},
	})
	entry := &service.OpsInsertErrorLogInput{}

	applyOpsUpstreamFieldsFromContext(c, entry)

	require.Nil(t, entry.UpstreamStatusCode)
	require.NotNil(t, entry.UpstreamErrorMessage)
	require.Equal(t, "final transport failure", *entry.UpstreamErrorMessage)
	require.NotNil(t, entry.UpstreamErrorDetail)
	require.Equal(t, "connection reset", *entry.UpstreamErrorDetail)
}

func TestOpsCaptureWriter_ProtocolLevelTerminalFrameDetection(t *testing.T) {
	state := &opsCaptureWriterState{limit: opsCaptureWriterLimit}
	chunks := []string{
		"event : response.failed\r\n",
		"data: { \"response\" : { \"error\" : { \"message\" : \"busy\", \"code\" : \"service_unavailable\" } },",
		" \"type\" : \"response.failed\" }\r\n\r\n",
	}
	for _, chunk := range chunks {
		state.captureResponseChunk([]byte(chunk), http.StatusOK)
	}

	parsed := parseOpsErrorResponse(state.buf.Bytes())
	require.True(t, state.sseCapturing)
	require.True(t, parsed.StreamFailure)
	require.Equal(t, "service_unavailable_error", parsed.ErrorType)
	require.Equal(t, "service_unavailable", parsed.Code)
	require.Equal(t, "busy", parsed.Message)
	require.Equal(t, http.StatusServiceUnavailable, inferStreamFailureStatus(nil, parsed))
}

func TestParseOpsSSEFailure_TopLevelErrorsAndUnknownStatus(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantType   string
		wantStatus int
	}{
		{
			name:       "top-level permission",
			body:       "event: error\ndata: {\"message\":\"denied\",\"code\":\"permission_denied\",\"type\":\"error\"}\n\n",
			wantType:   "permission_error",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "top-level unavailable",
			body:       "data: {\"message\":\"busy\",\"type\":\"error\",\"code\":\"service_unavailable\"}\n\n",
			wantType:   "service_unavailable_error",
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "unknown terminal",
			body:       "event: response.failed\ndata: {\"type\":\"response.failed\",\"error\":{\"code\":\"new_provider_code\",\"message\":\"failed\"}}\n\n",
			wantType:   "upstream_error",
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "explicit terminal status",
			body:       "event: error\ndata: {\"type\":\"error\",\"status_code\":429,\"code\":\"new_rate_code\",\"message\":\"slow down\"}\n\n",
			wantType:   "api_error",
			wantStatus: http.StatusTooManyRequests,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := parseOpsErrorResponse([]byte(tt.body))
			require.True(t, parsed.StreamFailure)
			require.Equal(t, tt.wantType, parsed.ErrorType)
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			service.SetOpsUpstreamError(c, http.StatusUnauthorized, "old attempt", "")
			require.Equal(t, tt.wantStatus, inferStreamFailureStatus(c, parsed), "terminal status must not inherit an earlier attempt")
		})
	}
}

func TestOpsCaptureWriter_OversizedNonTerminalFrameRemainsBounded(t *testing.T) {
	state := &opsCaptureWriterState{limit: opsCaptureWriterLimit}
	state.captureResponseChunk([]byte("data: "+strings.Repeat("x", opsTerminalSSEFrameProbeLimit*2)+"\n\n"), http.StatusOK)
	require.Empty(t, state.buf.Bytes())
	require.LessOrEqual(t, cap(state.probe), opsTerminalSSEFrameProbeLimit)

	state.captureResponseChunk([]byte("event: error\ndata: {\"type\":\"error\",\"code\":\"permission_denied\",\"message\":\"denied\"}\n\n"), http.StatusOK)
	require.True(t, state.sseCapturing)
	require.NotEmpty(t, state.buf.Bytes())
}

func TestOpsCaptureWriter_TerminalMetadataSurvivesBodyCaptureTruncation(t *testing.T) {
	state := &opsCaptureWriterState{limit: opsCaptureWriterLimit}
	frame := "event: response.failed\ndata: {\"padding\":\"" + strings.Repeat("x", opsCaptureWriterLimit) + "\",\"type\":\"response.failed\",\"error\":{\"code\":\"service_unavailable\",\"message\":\"busy\"}}\n\n"
	state.captureResponseChunk([]byte(frame), http.StatusOK)
	state.finalizeResponseCapture()

	require.Len(t, state.buf.Bytes(), opsCaptureWriterLimit)
	require.True(t, parseOpsErrorResponse(state.buf.Bytes()).StreamFailure, "the bounded parser must fail closed from the terminal event line")
	require.True(t, state.terminalFound)
	require.Equal(t, "service_unavailable_error", state.terminalError.ErrorType)
	require.Equal(t, "busy", state.terminalError.Message)
}

func TestOpsErrorLoggerMiddleware_LargeTerminalFrameUsesEventFallback(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 2)
	gin.SetMode(gin.TestMode)
	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.POST("/v1/responses", func(c *gin.Context) {
		c.Status(http.StatusOK)
		_, _ = c.Writer.WriteString("event: response.failed\n")
		_, _ = c.Writer.WriteString("data: {\"authorization\":\"Bearer must-not-persist\",\"padding\":\"" + strings.Repeat("x", opsTerminalSSEFrameProbeLimit*2) + "\"}")
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(1), OpsErrorLogQueueLength())
	job := <-opsErrorLogQueue
	require.Equal(t, http.StatusBadGateway, job.entry.StatusCode)
	require.Equal(t, "upstream_error", job.entry.ErrorType)
	require.Equal(t, "upstream stream failed", job.entry.ErrorMessage)
	require.NotContains(t, job.entry.ErrorMessage, "must-not-persist")
	require.NotContains(t, job.entry.ErrorBody, "must-not-persist")
	require.Contains(t, job.entry.ErrorBody, `"payload_truncated":true`)
}

func TestOpsErrorLoggerMiddleware_DetectsTerminalDataAtEOFWithoutBlankLine(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 2)
	gin.SetMode(gin.TestMode)
	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.POST("/v1/responses", func(c *gin.Context) {
		c.Status(http.StatusOK)
		_, _ = c.Writer.WriteString(`data: {"message":"denied","code":"permission_denied","type":"error"}`)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(1), OpsErrorLogQueueLength())
	job := <-opsErrorLogQueue
	require.Equal(t, http.StatusForbidden, job.entry.StatusCode)
	require.Equal(t, "permission_error", job.entry.ErrorType)
	require.Equal(t, "denied", job.entry.ErrorMessage)
}

func TestOpsCaptureWriter_DetectsCROnlySSEFrame(t *testing.T) {
	state := &opsCaptureWriterState{limit: opsCaptureWriterLimit}
	state.captureResponseChunk([]byte("data: {\"type\":\"error\",\"code\":\"service_unavailable\",\"message\":\"busy\"}\r\r"), http.StatusOK)

	require.True(t, state.sseCapturing)
	parsed := parseOpsErrorResponse(state.buf.Bytes())
	require.True(t, parsed.StreamFailure)
	require.Equal(t, "service_unavailable_error", parsed.ErrorType)
}

func TestSanitizeOpsSSEDataForPersistence_RedactsJSONFields(t *testing.T) {
	body := []byte("event: error\ndata: {\"type\":\"error\",\"authorization\":\"Bearer secret\",\ndata: \"nested\":{\"api_key\":\"sk-secret\"}}\n\n")
	sanitized := sanitizeOpsSSEDataForPersistence(body)
	require.NotContains(t, sanitized, "Bearer secret")
	require.NotContains(t, sanitized, "sk-secret")
	require.Contains(t, sanitized, `"authorization":"[REDACTED]"`)
	require.Contains(t, sanitized, `"api_key":"[REDACTED]"`)
}

func TestSanitizeOpsSSEDataForPersistence_DropsTruncatedJSONFragment(t *testing.T) {
	body := []byte("event: error\ndata: {\"type\":\"error\",\"authorization\":\"Bearer leaked")
	sanitized := sanitizeOpsSSEDataForPersistence(body)
	require.NotContains(t, sanitized, "Bearer leaked")
	require.Contains(t, sanitized, `data: {"payload_truncated":true}`)
}

func BenchmarkOpsCaptureWriterSuccessfulSSEFrames(b *testing.B) {
	frame := []byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n")
	state := &opsCaptureWriterState{limit: opsCaptureWriterLimit}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		state.captureResponseChunk(frame, http.StatusOK)
	}
	if state.buf.Len() != 0 {
		b.Fatal("successful frames must not be captured")
	}
}

func TestSetOpsEndpointContext_SetsContextKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	setOpsEndpointContext(c, "claude-3-5-sonnet-20241022", int16(2)) // stream

	v, ok := c.Get(opsUpstreamModelKey)
	require.True(t, ok)
	vStr, ok := v.(string)
	require.True(t, ok)
	require.Equal(t, "claude-3-5-sonnet-20241022", vStr)

	rt, ok := c.Get(opsRequestTypeKey)
	require.True(t, ok)
	rtVal, ok := rt.(int16)
	require.True(t, ok)
	require.Equal(t, int16(2), rtVal)
}

func TestSetOpsEndpointContext_EmptyModelNotStored(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	setOpsEndpointContext(c, "", int16(1))

	_, ok := c.Get(opsUpstreamModelKey)
	require.False(t, ok, "empty upstream model should not be stored")

	rt, ok := c.Get(opsRequestTypeKey)
	require.True(t, ok)
	rtVal, ok := rt.(int16)
	require.True(t, ok)
	require.Equal(t, int16(1), rtVal)
}

func TestSetOpsEndpointContext_NilContext(t *testing.T) {
	require.NotPanics(t, func() {
		setOpsEndpointContext(nil, "model", int16(1))
	})
}

func TestGetOpsAPIKeyFallsBackToOpsFallbackKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	// 主 key 缺席（鉴权早退场景）：返回 nil。
	require.Nil(t, getOpsAPIKey(c))

	// 写入 ops 专用 fallback key 后应能取到，且带齐 user/group。
	groupID := int64(55)
	apiKey := &service.APIKey{
		ID:      100,
		GroupID: &groupID,
		User:    &service.User{ID: 7},
		Group:   &service.Group{ID: groupID, Platform: service.PlatformAnthropic},
	}
	c.Set(string(middleware2.ContextKeyOpsFallbackAPIKey), apiKey)

	got := getOpsAPIKey(c)
	require.NotNil(t, got)
	require.Equal(t, int64(100), got.ID)
	require.NotNil(t, got.User)
	require.Equal(t, int64(7), got.User.ID)
	require.NotNil(t, got.Group)
	require.Equal(t, service.PlatformAnthropic, got.Group.Platform)
}

func TestGetOpsAPIKeyPrefersPrimaryContextKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	primary := &service.APIKey{ID: 1}
	fallback := &service.APIKey{ID: 2}
	c.Set(string(middleware2.ContextKeyAPIKey), primary)
	c.Set(string(middleware2.ContextKeyOpsFallbackAPIKey), fallback)

	got := getOpsAPIKey(c)
	require.NotNil(t, got)
	require.Equal(t, int64(1), got.ID, "已鉴权请求应优先使用正式 api key")
}
