package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type responseEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
REDACTED

type opsSystemLogCaptureRepo struct {
	service.OpsRepository
	listFilter    *service.OpsSystemLogFilter
	cleanupFilter *service.OpsSystemLogCleanupFilter
REDACTED

func (r *opsSystemLogCaptureRepo) ListSystemLogs(_ context.Context, filter *service.OpsSystemLogFilter) (*service.OpsSystemLogList, error) {
	r.listFilter = filter
	return &service.OpsSystemLogList{Logs: []*service.OpsSystemLog{REDACTED, Page: filter.Page, PageSize: filter.PageSizeREDACTED, nil
REDACTED

func (r *opsSystemLogCaptureRepo) DeleteSystemLogs(_ context.Context, filter *service.OpsSystemLogCleanupFilter) (int64, error) {
	r.cleanupFilter = filter
	return 1, nil
REDACTED

func (r *opsSystemLogCaptureRepo) InsertSystemLogCleanupAudit(_ context.Context, _ *service.OpsSystemLogCleanupAudit) error {
	return nil
REDACTED

func newOpsSystemLogTestRouter(handler *OpsHandler, withUser bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if withUser {
		r.Use(func(c *gin.Context) {
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 99REDACTED)
			c.Next()
	REDACTED)
REDACTED
	r.GET("/logs", handler.ListSystemLogs)
	r.POST("/logs/cleanup", handler.CleanupSystemLogs)
	r.GET("/logs/health", handler.GetSystemLogIngestionHealth)
	return r
REDACTED

func TestOpsSystemLogHandler_ListUnavailable(t *testing.T) {
	h := NewOpsHandler(nil)
	r := newOpsSystemLogTestRouter(h, false)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/logs", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503", w.Code)
REDACTED
REDACTED

func TestOpsSystemLogHandler_ListInvalidUserID(t *testing.T) {
	svc := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpsHandler(svc)
	r := newOpsSystemLogTestRouter(h, false)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/logs?user_id=abc", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
REDACTED
REDACTED

func TestOpsSystemLogHandler_ListInvalidAccountID(t *testing.T) {
	svc := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpsHandler(svc)
	r := newOpsSystemLogTestRouter(h, false)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/logs?account_id=-1", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
REDACTED
REDACTED

func TestOpsSystemLogHandler_ListInvalidAPIKeyID(t *testing.T) {
	svc := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpsHandler(svc)
	r := newOpsSystemLogTestRouter(h, false)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/logs?api_key_id=abc", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
REDACTED
REDACTED

func TestOpsSystemLogHandler_ListMonitoringDisabled(t *testing.T) {
	svc := service.NewOpsService(nil, nil, &config.Config{
		Ops: config.OpsConfig{Enabled: falseREDACTED,
REDACTED, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpsHandler(svc)
	r := newOpsSystemLogTestRouter(h, false)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/logs", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
REDACTED
REDACTED

func TestOpsSystemLogHandler_ListSuccess(t *testing.T) {
	svc := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpsHandler(svc)
	r := newOpsSystemLogTestRouter(h, false)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/logs?time_range=30m&page=1&page_size=20", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", w.Code)
REDACTED

	var resp responseEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
REDACTED
	if resp.Code != 0 {
		t.Fatalf("unexpected response code: %+v", resp)
REDACTED
REDACTED

func TestOpsSystemLogHandler_ListAcceptsHost(t *testing.T) {
	repo := &opsSystemLogCaptureRepo{REDACTED
	svc := service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpsHandler(svc)
	r := newOpsSystemLogTestRouter(h, false)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/logs?host=api-node-1", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", w.Code)
REDACTED
	if repo.listFilter == nil || repo.listFilter.Host != "api-node-1" {
		t.Fatalf("host filter = %+v, want api-node-1", repo.listFilter)
REDACTED
REDACTED

func TestOpsSystemLogHandler_CleanupUnauthorized(t *testing.T) {
	svc := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpsHandler(svc)
	r := newOpsSystemLogTestRouter(h, false)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/logs/cleanup", bytes.NewBufferString(`{"request_id":"r1"REDACTED`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", w.Code)
REDACTED
REDACTED

func TestOpsSystemLogHandler_CleanupInvalidPayload(t *testing.T) {
	svc := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpsHandler(svc)
	r := newOpsSystemLogTestRouter(h, true)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/logs/cleanup", bytes.NewBufferString(`{bad-json`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
REDACTED
REDACTED

func TestOpsSystemLogHandler_CleanupInvalidTime(t *testing.T) {
	svc := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpsHandler(svc)
	r := newOpsSystemLogTestRouter(h, true)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/logs/cleanup", bytes.NewBufferString(`{"start_time":"bad","request_id":"r1"REDACTED`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
REDACTED
REDACTED

func TestOpsSystemLogHandler_CleanupInvalidEndTime(t *testing.T) {
	svc := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpsHandler(svc)
	r := newOpsSystemLogTestRouter(h, true)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/logs/cleanup", bytes.NewBufferString(`{"end_time":"bad","request_id":"r1"REDACTED`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
REDACTED
REDACTED

func TestOpsSystemLogHandler_CleanupServiceUnavailable(t *testing.T) {
	svc := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpsHandler(svc)
	r := newOpsSystemLogTestRouter(h, true)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/logs/cleanup", bytes.NewBufferString(`{"request_id":"r1"REDACTED`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503", w.Code)
REDACTED
REDACTED

func TestOpsSystemLogHandler_CleanupAcceptsAPIKeyID(t *testing.T) {
	svc := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpsHandler(svc)
	r := newOpsSystemLogTestRouter(h, true)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/logs/cleanup", bytes.NewBufferString(`{"api_key_id":123REDACTED`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503", w.Code)
REDACTED
REDACTED

func TestOpsSystemLogHandler_CleanupAcceptsHost(t *testing.T) {
	repo := &opsSystemLogCaptureRepo{REDACTED
	svc := service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpsHandler(svc)
	r := newOpsSystemLogTestRouter(h, true)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/logs/cleanup", bytes.NewBufferString(`{"host":"api-node-1"REDACTED`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", w.Code)
REDACTED
	if repo.cleanupFilter == nil || repo.cleanupFilter.Host != "api-node-1" {
		t.Fatalf("host filter = %+v, want api-node-1", repo.cleanupFilter)
REDACTED
REDACTED

func TestOpsSystemLogHandler_CleanupInvalidAPIKeyID(t *testing.T) {
	svc := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpsHandler(svc)
	r := newOpsSystemLogTestRouter(h, true)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/logs/cleanup", bytes.NewBufferString(`{"api_key_id":0REDACTED`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
REDACTED
REDACTED

func TestOpsSystemLogHandler_CleanupMonitoringDisabled(t *testing.T) {
	svc := service.NewOpsService(nil, nil, &config.Config{
		Ops: config.OpsConfig{Enabled: falseREDACTED,
REDACTED, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpsHandler(svc)
	r := newOpsSystemLogTestRouter(h, true)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/logs/cleanup", bytes.NewBufferString(`{"request_id":"r1"REDACTED`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
REDACTED
REDACTED

func TestOpsSystemLogHandler_Health(t *testing.T) {
	sink := service.NewOpsSystemLogSink(nil)
	svc := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, sink)
	h := NewOpsHandler(svc)
	r := newOpsSystemLogTestRouter(h, false)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/logs/health", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", w.Code)
REDACTED
REDACTED

func TestOpsSystemLogHandler_HealthUnavailableAndMonitoringDisabled(t *testing.T) {
	h := NewOpsHandler(nil)
	r := newOpsSystemLogTestRouter(h, false)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/logs/health", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503", w.Code)
REDACTED

	svc := service.NewOpsService(nil, nil, &config.Config{
		Ops: config.OpsConfig{Enabled: falseREDACTED,
REDACTED, nil, nil, nil, nil, nil, nil, nil, nil)
	h = NewOpsHandler(svc)
	r = newOpsSystemLogTestRouter(h, false)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/logs/health", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
REDACTED
REDACTED
