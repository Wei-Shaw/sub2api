package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

type stubOpenAIAccountRepo struct {
	AccountRepository
	accounts []Account
REDACTED

func (r stubOpenAIAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	return append([]Account(nil), r.accounts...), nil
REDACTED

func (r stubOpenAIAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return append([]Account(nil), r.accounts...), nil
REDACTED

type stubConcurrencyCache struct {
	ConcurrencyCache
REDACTED

type cancelReadCloser struct{REDACTED

func (c cancelReadCloser) Read(p []byte) (int, error) { return 0, context.Canceled REDACTED
func (c cancelReadCloser) Close() error               { return nil REDACTED

type failingGinWriter struct {
	gin.ResponseWriter
	failAfter int
	writes    int
REDACTED

func (w *failingGinWriter) Write(p []byte) (int, error) {
	if w.writes >= w.failAfter {
		return 0, errors.New("write failed")
REDACTED
	w.writes++
	return w.ResponseWriter.Write(p)
REDACTED

func (c stubConcurrencyCache) AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
	return true, nil
REDACTED

func (c stubConcurrencyCache) ReleaseAccountSlot(ctx context.Context, accountID int64, requestID string) error {
	return nil
REDACTED

func (c stubConcurrencyCache) GetAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	out := make(map[int64]*AccountLoadInfo, len(accounts))
	for _, acc := range accounts {
		out[acc.ID] = &AccountLoadInfo{AccountID: acc.ID, LoadRate: 0REDACTED
REDACTED
	return out, nil
REDACTED

func TestOpenAIGatewayService_GenerateSessionHash_Priority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)

	svc := &OpenAIGatewayService{REDACTED

	// 1) session_id header wins
	c.Request.Header.Set("session_id", "sess-123")
	c.Request.Header.Set("conversation_id", "conv-456")
	h1 := svc.GenerateSessionHash(c, map[string]any{"prompt_cache_key": "ses_aaa"REDACTED)
	if h1 == "" {
		t.Fatalf("expected non-empty hash")
REDACTED

	// 2) conversation_id used when session_id absent
	c.Request.Header.Del("session_id")
	h2 := svc.GenerateSessionHash(c, map[string]any{"prompt_cache_key": "ses_aaa"REDACTED)
	if h2 == "" {
		t.Fatalf("expected non-empty hash")
REDACTED
	if h1 == h2 {
		t.Fatalf("expected different hashes for different keys")
REDACTED

	// 3) prompt_cache_key used when both headers absent
	c.Request.Header.Del("conversation_id")
	h3 := svc.GenerateSessionHash(c, map[string]any{"prompt_cache_key": "ses_aaa"REDACTED)
	if h3 == "" {
		t.Fatalf("expected non-empty hash")
REDACTED
	if h2 == h3 {
		t.Fatalf("expected different hashes for different keys")
REDACTED

	// 4) empty when no signals
	h4 := svc.GenerateSessionHash(c, map[string]any{REDACTED)
	if h4 != "" {
		t.Fatalf("expected empty hash when no signals")
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_FiltersUnschedulable(t *testing.T) {
	now := time.Now()
	resetAt := now.Add(10 * time.Minute)
	groupID := int64(1)

	rateLimited := Account{
		ID:               1,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeAPIKey,
		Status:           StatusActive,
		Schedulable:      true,
		Concurrency:      1,
		Priority:         0,
		RateLimitResetAt: &resetAt,
REDACTED
	available := Account{
		ID:          2,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    1,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{rateLimited, availableREDACTEDREDACTED,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{REDACTED),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-5.2", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.Account == nil {
		t.Fatalf("expected selection with account")
REDACTED
	if selection.Account.ID != available.ID {
		t.Fatalf("expected account %d, got %d", available.ID, selection.Account.ID)
REDACTED
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_FiltersUnschedulableWhenNoConcurrencyService(t *testing.T) {
	now := time.Now()
	resetAt := now.Add(10 * time.Minute)
	groupID := int64(1)

	rateLimited := Account{
		ID:               1,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeAPIKey,
		Status:           StatusActive,
		Schedulable:      true,
		Concurrency:      1,
		Priority:         0,
		RateLimitResetAt: &resetAt,
REDACTED
	available := Account{
		ID:          2,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    1,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{rateLimited, availableREDACTEDREDACTED,
		// concurrencyService is nil, forcing the non-load-batch selection path.
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-5.2", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.Account == nil {
		t.Fatalf("expected selection with account")
REDACTED
	if selection.Account.ID != available.ID {
		t.Fatalf("expected account %d, got %d", available.ID, selection.Account.ID)
REDACTED
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
REDACTED
REDACTED

func TestOpenAIStreamingTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 1,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{REDACTED,
REDACTED

	start := time.Now()
	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1REDACTED, start, "model", "model")
	_ = pw.Close()
	_ = pr.Close()

	if err == nil || !strings.Contains(err.Error(), "stream data interval timeout") {
		t.Fatalf("expected stream timeout error, got %v", err)
REDACTED
	if !strings.Contains(rec.Body.String(), "\"type\":\"error\"") || !strings.Contains(rec.Body.String(), "stream_timeout") {
		t.Fatalf("expected OpenAI-compatible error SSE event, got %q", rec.Body.String())
REDACTED
REDACTED

func TestOpenAIStreamingContextCanceledDoesNotInjectErrorEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       cancelReadCloser{REDACTED,
		Header:     http.Header{REDACTED,
REDACTED

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1REDACTED, time.Now(), "model", "model")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
REDACTED
	if strings.Contains(rec.Body.String(), "event: error") || strings.Contains(rec.Body.String(), "stream_read_error") {
		t.Fatalf("expected no injected SSE error event, got %q", rec.Body.String())
REDACTED
REDACTED

func TestOpenAIStreamingClientDisconnectDrainsUpstreamUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Writer = &failingGinWriter{ResponseWriter: c.Writer, failAfter: 0REDACTED

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{REDACTED,
REDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		_, _ = pw.Write([]byte("data: {\"type\":\"response.in_progress\",\"response\":{REDACTEDREDACTED\n\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":3,\"output_tokens\":5,\"input_tokens_details\":{\"cached_tokens\":1REDACTEDREDACTEDREDACTEDREDACTED\n\n"))
REDACTED()

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1REDACTED, time.Now(), "model", "model")
	_ = pr.Close()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
REDACTED
	if result == nil || result.usage == nil {
		t.Fatalf("expected usage result")
REDACTED
	if result.usage.InputTokens != 3 || result.usage.OutputTokens != 5 || result.usage.CacheReadInputTokens != 1 {
		t.Fatalf("unexpected usage: %+v", *result.usage)
REDACTED
	if strings.Contains(rec.Body.String(), "event: error") || strings.Contains(rec.Body.String(), "write_failed") {
		t.Fatalf("expected no injected SSE error event, got %q", rec.Body.String())
REDACTED
REDACTED

func TestOpenAIStreamingTooLong(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               64 * 1024,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{REDACTED,
REDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		// 写入超过 MaxLineSize 的单行数据，触发 ErrTooLong
		payload := "data: " + strings.Repeat("a", 128*1024) + "\n"
		_, _ = pw.Write([]byte(payload))
REDACTED()

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 2REDACTED, time.Now(), "model", "model")
	_ = pr.Close()

	if !errors.Is(err, bufio.ErrTooLong) {
		t.Fatalf("expected ErrTooLong, got %v", err)
REDACTED
	if !strings.Contains(rec.Body.String(), "\"type\":\"error\"") || !strings.Contains(rec.Body.String(), "response_too_large") {
		t.Fatalf("expected OpenAI-compatible error SSE event, got %q", rec.Body.String())
REDACTED
REDACTED

func TestOpenAINonStreamingContentTypePassThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			ResponseHeaders: config.ResponseHeaderConfig{Enabled: falseREDACTED,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	body := []byte(`{"usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0REDACTEDREDACTEDREDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/vnd.test+json"REDACTEDREDACTED,
REDACTED

	_, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{REDACTED, "model", "model")
	if err != nil {
		t.Fatalf("handleNonStreamingResponse error: %v", err)
REDACTED

	if !strings.Contains(rec.Header().Get("Content-Type"), "application/vnd.test+json") {
		t.Fatalf("expected Content-Type passthrough, got %q", rec.Header().Get("Content-Type"))
REDACTED
REDACTED

func TestOpenAINonStreamingContentTypeDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			ResponseHeaders: config.ResponseHeaderConfig{Enabled: falseREDACTED,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	body := []byte(`{"usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0REDACTEDREDACTEDREDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{REDACTED,
REDACTED

	_, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{REDACTED, "model", "model")
	if err != nil {
		t.Fatalf("handleNonStreamingResponse error: %v", err)
REDACTED

	if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("expected default Content-Type, got %q", rec.Header().Get("Content-Type"))
REDACTED
REDACTED

func TestOpenAIStreamingHeadersOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			ResponseHeaders: config.ResponseHeaderConfig{Enabled: falseREDACTED,
	REDACTED,
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header: http.Header{
			"Cache-Control": []string{"upstream"REDACTED,
			"X-Request-Id":  []string{"req-123"REDACTED,
			"Content-Type":  []string{"application/custom"REDACTED,
	REDACTED,
REDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		_, _ = pw.Write([]byte("data: {REDACTED\n\n"))
REDACTED()

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1REDACTED, time.Now(), "model", "model")
	_ = pr.Close()
	if err != nil {
		t.Fatalf("handleStreamingResponse error: %v", err)
REDACTED

	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("expected Cache-Control override, got %q", rec.Header().Get("Cache-Control"))
REDACTED
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected Content-Type override, got %q", rec.Header().Get("Content-Type"))
REDACTED
	if rec.Header().Get("X-Request-Id") != "req-123" {
		t.Fatalf("expected X-Request-Id passthrough, got %q", rec.Header().Get("X-Request-Id"))
REDACTED
REDACTED

func TestOpenAIInvalidBaseURLWhenAllowlistDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTED,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
REDACTED"base_url": "://invalid-url"REDACTED,
REDACTED

	_, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte("{REDACTED"), "token", false, "", false)
	if err == nil {
		t.Fatalf("expected error for invalid base_url when allowlist disabled")
REDACTED
REDACTED

func TestOpenAIValidateUpstreamBaseURLDisabledRequiresHTTPS(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTED,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	if _, err := svc.validateUpstreamBaseURL("http://not-https.example.com"); err == nil {
		t.Fatalf("expected http to be rejected when allow_insecure_http is false")
REDACTED
	normalized, err := svc.validateUpstreamBaseURL("https://example.com")
	if err != nil {
		t.Fatalf("expected https to be allowed when allowlist disabled, got %v", err)
REDACTED
	if normalized != "https://example.com" {
		t.Fatalf("expected raw url passthrough, got %q", normalized)
REDACTED
REDACTED

func TestOpenAIValidateUpstreamBaseURLDisabledAllowsHTTP(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:           false,
				AllowInsecureHTTP: true,
		REDACTED,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	normalized, err := svc.validateUpstreamBaseURL("http://not-https.example.com")
	if err != nil {
		t.Fatalf("expected http allowed when allow_insecure_http is true, got %v", err)
REDACTED
	if normalized != "http://not-https.example.com" {
		t.Fatalf("expected raw url passthrough, got %q", normalized)
REDACTED
REDACTED

func TestOpenAIValidateUpstreamBaseURLEnabledEnforcesAllowlist(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:       true,
				UpstreamHosts: []string{"example.com"REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	if _, err := svc.validateUpstreamBaseURL("https://example.com"); err != nil {
		t.Fatalf("expected allowlisted host to pass, got %v", err)
REDACTED
	if _, err := svc.validateUpstreamBaseURL("https://evil.com"); err == nil {
		t.Fatalf("expected non-allowlisted host to fail")
REDACTED
REDACTED
