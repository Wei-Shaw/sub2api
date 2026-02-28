package handler

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func resetOpsErrorLoggerStateForTest(t *testing.T) {
REDACTED

	opsErrorLogMu.Lock()
	ch := opsErrorLogQueue
	opsErrorLogQueue = nil
	opsErrorLogStopping = true
	opsErrorLogMu.Unlock()

	if ch != nil {
		close(ch)
REDACTED
	opsErrorLogWorkersWg.Wait()

	opsErrorLogOnce = sync.Once{REDACTED
	opsErrorLogStopOnce = sync.Once{REDACTED
	opsErrorLogWorkersWg = sync.WaitGroup{REDACTED
	opsErrorLogMu = sync.RWMutex{REDACTED
	opsErrorLogStopping = false

	opsErrorLogQueueLen.Store(0)
	opsErrorLogEnqueued.Store(0)
	opsErrorLogDropped.Store(0)
	opsErrorLogProcessed.Store(0)
	opsErrorLogSanitized.Store(0)
	opsErrorLogLastDropLogAt.Store(0)

	opsErrorLogShutdownCh = make(chan struct{REDACTED)
	opsErrorLogShutdownOnce = sync.Once{REDACTED
	opsErrorLogDrained.Store(false)
REDACTED

func TestAttachOpsRequestBodyToEntry_SanitizeAndTrim(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	raw := []byte(`{"access_token":"secret-token","messages":[{"role":"user","content":"hello"REDACTED]REDACTED`)
	setOpsRequestContext(c, "claude-3", false, raw)

	entry := &service.OpsInsertErrorLogInput{REDACTED
	attachOpsRequestBodyToEntry(c, entry)

	require.NotNil(t, entry.RequestBodyBytes)
	require.Equal(t, len(raw), *entry.RequestBodyBytes)
	require.NotNil(t, entry.RequestBodyJSON)
	require.NotContains(t, *entry.RequestBodyJSON, "secret-token")
	require.Contains(t, *entry.RequestBodyJSON, "[REDACTED]")
	require.Equal(t, int64(1), OpsErrorLogSanitizedTotal())
REDACTED

func TestAttachOpsRequestBodyToEntry_InvalidJSONKeepsSize(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	raw := []byte("not-json")
	setOpsRequestContext(c, "claude-3", false, raw)

	entry := &service.OpsInsertErrorLogInput{REDACTED
	attachOpsRequestBodyToEntry(c, entry)

	require.Nil(t, entry.RequestBodyJSON)
	require.NotNil(t, entry.RequestBodyBytes)
	require.Equal(t, len(raw), *entry.RequestBodyBytes)
	require.False(t, entry.RequestBodyTruncated)
	require.Equal(t, int64(1), OpsErrorLogSanitizedTotal())
REDACTED

func TestEnqueueOpsErrorLog_QueueFullDrop(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)

	// 禁止 enqueueOpsErrorLog 触发 workers，使用测试队列验证满队列降级。
	opsErrorLogOnce.Do(func() {REDACTED)

	opsErrorLogMu.Lock()
	opsErrorLogQueue = make(chan opsErrorLogJob, 1)
	opsErrorLogMu.Unlock()

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	entry := &service.OpsInsertErrorLogInput{ErrorPhase: "upstream", ErrorType: "upstream_error"REDACTED

	enqueueOpsErrorLog(ops, entry)
	enqueueOpsErrorLog(ops, entry)

	require.Equal(t, int64(1), OpsErrorLogEnqueuedTotal())
	require.Equal(t, int64(1), OpsErrorLogDroppedTotal())
	require.Equal(t, int64(1), OpsErrorLogQueueLength())
REDACTED

func TestAttachOpsRequestBodyToEntry_EarlyReturnBranches(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)
	gin.SetMode(gin.TestMode)

	entry := &service.OpsInsertErrorLogInput{REDACTED
	attachOpsRequestBodyToEntry(nil, entry)
	attachOpsRequestBodyToEntry(&gin.Context{REDACTED, nil)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	// 无请求体 key
	attachOpsRequestBodyToEntry(c, entry)
	require.Nil(t, entry.RequestBodyJSON)
	require.Nil(t, entry.RequestBodyBytes)
	require.False(t, entry.RequestBodyTruncated)

	// 错误类型
	c.Set(opsRequestBodyKey, "not-bytes")
	attachOpsRequestBodyToEntry(c, entry)
	require.Nil(t, entry.RequestBodyJSON)
	require.Nil(t, entry.RequestBodyBytes)

	// 空 bytes
	c.Set(opsRequestBodyKey, []byte{REDACTED)
	attachOpsRequestBodyToEntry(c, entry)
	require.Nil(t, entry.RequestBodyJSON)
	require.Nil(t, entry.RequestBodyBytes)

	require.Equal(t, int64(0), OpsErrorLogSanitizedTotal())
REDACTED

func TestEnqueueOpsErrorLog_EarlyReturnBranches(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	entry := &service.OpsInsertErrorLogInput{ErrorPhase: "upstream", ErrorType: "upstream_error"REDACTED

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
	opsErrorLogOnce.Do(func() {REDACTED)
	opsErrorLogMu.Lock()
	opsErrorLogQueue = nil
	opsErrorLogMu.Unlock()
	enqueueOpsErrorLog(ops, entry)
	require.Equal(t, int64(0), OpsErrorLogEnqueuedTotal())
REDACTED

func TestOpsCaptureWriterPool_ResetOnRelease(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	writer := acquireOpsCaptureWriter(c.Writer)
	require.NotNil(t, writer)
	_, err := writer.buf.WriteString("temp-error-body")
REDACTED

	releaseOpsCaptureWriter(writer)

	reused := acquireOpsCaptureWriter(c.Writer)
	defer releaseOpsCaptureWriter(reused)

	require.Zero(t, reused.buf.Len(), "writer should be reset before reuse")
REDACTED

func TestOpsErrorLoggerMiddleware_DoesNotBreakOuterMiddlewares(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(middleware2.Recovery())
	r.Use(middleware2.RequestLogger())
	r.Use(middleware2.Logger())
	r.GET("/v1/messages", OpsErrorLoggerMiddleware(nil), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
REDACTED)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)

	require.NotPanics(t, func() {
		r.ServeHTTP(rec, req)
REDACTED)
	require.Equal(t, http.StatusNoContent, rec.Code)
REDACTED
