package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

type testLogSink struct {
	mu     sync.Mutex
	events []*logger.LogEvent
REDACTED

func (s *testLogSink) WriteLogEvent(event *logger.LogEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
REDACTED

func (s *testLogSink) list() []*logger.LogEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*logger.LogEvent, len(s.events))
	copy(out, s.events)
	return out
REDACTED

func initMiddlewareTestLogger(t *testing.T) *testLogSink {
	return initMiddlewareTestLoggerWithLevel(t, "debug")
REDACTED

func initMiddlewareTestLoggerWithLevel(t *testing.T, level string) *testLogSink {
REDACTED
	level = strings.TrimSpace(level)
	if level == "" {
		level = "debug"
REDACTED
	if err := logger.Init(logger.InitOptions{
		Level:       level,
		Format:      "json",
		ServiceName: "sub2api",
		Environment: "test",
		Output: logger.OutputOptions{
			ToStdout: false,
			ToFile:   false,
	REDACTED,
REDACTED); err != nil {
		t.Fatalf("init logger: %v", err)
REDACTED
	sink := &testLogSink{REDACTED
	logger.SetSink(sink)
	t.Cleanup(func() {
		logger.SetSink(nil)
REDACTED)
	return sink
REDACTED

func TestRequestLogger_GenerateAndPropagateRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestLogger())
	r.GET("/t", func(c *gin.Context) {
		reqID, ok := c.Request.Context().Value(ctxkey.RequestID).(string)
		if !ok || reqID == "" {
			t.Fatalf("request_id missing in context")
	REDACTED
		if got := c.Writer.Header().Get(requestIDHeader); got != reqID {
			t.Fatalf("response header request_id mismatch, header=%q ctx=%q", got, reqID)
	REDACTED
		c.Status(http.StatusOK)
REDACTED)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
REDACTED
	if w.Header().Get(requestIDHeader) == "" {
		t.Fatalf("X-Request-ID should be set")
REDACTED
REDACTED

func TestRequestLogger_KeepIncomingRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestLogger())
	r.GET("/t", func(c *gin.Context) {
		reqID, _ := c.Request.Context().Value(ctxkey.RequestID).(string)
		if reqID != "rid-fixed" {
			t.Fatalf("request_id=%q, want rid-fixed", reqID)
	REDACTED
		c.Status(http.StatusOK)
REDACTED)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set(requestIDHeader, "rid-fixed")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
REDACTED
	if got := w.Header().Get(requestIDHeader); got != "rid-fixed" {
		t.Fatalf("header=%q, want rid-fixed", got)
REDACTED
REDACTED

func TestRequestLoggerBoundsIncomingRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestLogger())
	r.GET("/t", func(c *gin.Context) {
		reqID, _ := c.Request.Context().Value(ctxkey.RequestID).(string)
		if len(reqID) != 36 {
			t.Fatalf("request_id length=%d", len(reqID))
	REDACTED
		c.Status(http.StatusOK)
REDACTED)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set(requestIDHeader, strings.Repeat("r", 1024))
	r.ServeHTTP(w, req)
	if got := len(w.Header().Get(requestIDHeader)); got != 36 {
		t.Fatalf("response request_id length=%d", got)
REDACTED
REDACTED

func TestLogger_AccessLogIncludesCoreFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sink := initMiddlewareTestLogger(t)

	r := gin.New()
	r.Use(Logger())
	r.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, ctxkey.AccountID, int64(101))
		ctx = context.WithValue(ctx, ctxkey.Platform, "openai")
		ctx = context.WithValue(ctx, ctxkey.Model, "gpt-5")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
REDACTED)
	r.GET("/api/test", func(c *gin.Context) {
		c.Status(http.StatusCreated)
REDACTED)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d", w.Code)
REDACTED

	events := sink.list()
	if len(events) == 0 {
		t.Fatalf("expected at least one log event")
REDACTED
	found := false
	for _, event := range events {
		if event == nil || event.Message != "http request completed" {
			continue
	REDACTED
		found = true
		switch v := event.Fields["status_code"].(type) {
		case int:
			if v != http.StatusCreated {
				t.Fatalf("status_code field mismatch: %v", v)
		REDACTED
		case int64:
			if v != int64(http.StatusCreated) {
				t.Fatalf("status_code field mismatch: %v", v)
		REDACTED
		default:
			t.Fatalf("status_code type mismatch: %T", v)
	REDACTED
		switch v := event.Fields["account_id"].(type) {
		case int64:
			if v != 101 {
				t.Fatalf("account_id field mismatch: %v", v)
		REDACTED
		case int:
			if v != 101 {
				t.Fatalf("account_id field mismatch: %v", v)
		REDACTED
		default:
			t.Fatalf("account_id type mismatch: %T", v)
	REDACTED
		if event.Fields["platform"] != "openai" || event.Fields["model"] != "gpt-5" {
			t.Fatalf("platform/model mismatch: %+v", event.Fields)
	REDACTED
REDACTED
	if !found {
		t.Fatalf("access log event not found")
REDACTED
REDACTED

func TestLogger_IngressRejectRemainsInStandardAccessLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sink := initMiddlewareTestLogger(t)
	r := gin.New()
	r.Use(Logger())
	r.GET("/v1/messages", func(c *gin.Context) {
		MarkIngressRejected(c, IngressRejectInvalidAPIKey)
		c.Status(http.StatusUnauthorized)
REDACTED)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
REDACTED
	events := sink.list()
	if len(events) != 1 {
		t.Fatalf("events=%d, want 1", len(events))
REDACTED
	if got := events[0].Fields["ingress_reject_reason"]; got != string(IngressRejectInvalidAPIKey) {
		t.Fatalf("ingress_reject_reason=%v", got)
REDACTED
	if got, _ := events[0].Fields[logger.OpsSystemLogSkipField].(bool); !got {
		t.Fatalf("%s must be true", logger.OpsSystemLogSkipField)
REDACTED
REDACTED

func TestLogger_AccessLogUsesForwardedClientIPFromTrustedProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sink := initMiddlewareTestLogger(t)

	r := gin.New()
	if err := r.SetTrustedProxies([]string{"104.23.251.120"REDACTED); err != nil {
		t.Fatalf("set trusted proxies: %v", err)
REDACTED
	r.Use(Logger())
	r.GET("/api/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
REDACTED)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "104.23.251.120:443"
	req.Header.Set("X-Forwarded-For", "203.0.113.42")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
REDACTED

	for _, event := range sink.list() {
		if event == nil || event.Message != "http request completed" {
			continue
	REDACTED
		if got := event.Fields["client_ip"]; got != "203.0.113.42" {
			t.Fatalf("client_ip=%q, want real forwarded ip", got)
	REDACTED
		return
REDACTED
	t.Fatalf("access log event not found")
REDACTED

func TestLogger_HealthPathSkipped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sink := initMiddlewareTestLogger(t)

	r := gin.New()
	r.Use(Logger())
	r.GET("/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
REDACTED)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
REDACTED
	if len(sink.list()) != 0 {
		t.Fatalf("health endpoint should not write access log")
REDACTED
REDACTED

func TestLogger_AccessLogDroppedWhenLevelWarn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sink := initMiddlewareTestLoggerWithLevel(t, "warn")

	r := gin.New()
	r.Use(RequestLogger())
	r.Use(Logger())
	r.GET("/api/test", func(c *gin.Context) {
		c.Status(http.StatusCreated)
REDACTED)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d", w.Code)
REDACTED

	events := sink.list()
	for _, event := range events {
		if event != nil && event.Message == "http request completed" {
			t.Fatalf("access log should not be indexed when level=warn: %+v", event)
	REDACTED
REDACTED
REDACTED
