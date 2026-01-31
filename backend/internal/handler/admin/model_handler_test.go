package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

func TestModelHandlerListSoraSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"m1"REDACTED,{"id":"m2"REDACTED]REDACTED`))
REDACTED))
	t.Cleanup(upstream.Close)

	cfg := &config.Config{REDACTED
	cfg.Sora2API.BaseURL = upstream.URL
	cfg.Sora2API.APIKey = "test-key"
	soraService := service.NewSora2APIService(cfg)

	h := NewModelHandler(soraService)
	router := gin.New()
	router.GET("/admin/models", h.List)

	req := httptest.NewRequest(http.MethodGet, "/admin/models?platform=sora", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
REDACTED
	var resp response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
REDACTED
	if resp.Code != 0 {
		t.Fatalf("响应 code=%d", resp.Code)
REDACTED
	data, ok := resp.Data.([]any)
	if !ok {
		t.Fatalf("响应 data 类型错误")
REDACTED
	if len(data) != 2 {
		t.Fatalf("模型数量不符: %d", len(data))
REDACTED
REDACTED

func TestModelHandlerListSoraNotConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewModelHandler(&service.Sora2APIService{REDACTED)
	router := gin.New()
	router.GET("/admin/models", h.List)

	req := httptest.NewRequest(http.MethodGet, "/admin/models?platform=sora", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
REDACTED
REDACTED

func TestModelHandlerListInvalidPlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewModelHandler(&service.Sora2APIService{REDACTED)
	router := gin.New()
	router.GET("/admin/models", h.List)

	req := httptest.NewRequest(http.MethodGet, "/admin/models?platform=unknown", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
REDACTED
REDACTED
