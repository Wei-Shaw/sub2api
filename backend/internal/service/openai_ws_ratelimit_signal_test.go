package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type openAIWSRateLimitSignalRepo struct {
	stubOpenAIAccountRepo
	rateLimitCalls []time.Time
	updateExtra    []map[string]any
REDACTED

type openAICodexSnapshotAsyncRepo struct {
	stubOpenAIAccountRepo
	updateExtraCh chan map[string]any
	rateLimitCh   chan time.Time
REDACTED

func (r *openAIWSRateLimitSignalRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	r.rateLimitCalls = append(r.rateLimitCalls, resetAt)
	return nil
REDACTED

func (r *openAIWSRateLimitSignalRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	copied := make(map[string]any, len(updates))
	for k, v := range updates {
		copied[k] = v
REDACTED
	r.updateExtra = append(r.updateExtra, copied)
	return nil
REDACTED

func (r *openAICodexSnapshotAsyncRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	if r.rateLimitCh != nil {
		r.rateLimitCh <- resetAt
REDACTED
	return nil
REDACTED

func (r *openAICodexSnapshotAsyncRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	if r.updateExtraCh != nil {
		copied := make(map[string]any, len(updates))
		for k, v := range updates {
			copied[k] = v
	REDACTED
		r.updateExtraCh <- copied
REDACTED
	return nil
REDACTED

func TestOpenAIGatewayService_Forward_WSv2ErrorEventUsageLimitPersistsRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	resetAt := time.Now().Add(2 * time.Hour).Unix()
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true REDACTEDREDACTED
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket failed: %v", err)
			return
	REDACTED
		defer func() { _ = conn.Close() REDACTED()

		var req map[string]any
		if err := conn.ReadJSON(&req); err != nil {
			t.Errorf("read ws request failed: %v", err)
			return
	REDACTED
		_ = conn.WriteJSON(map[string]any{
			"type": "error",
			"error": map[string]any{
				"code":      "rate_limit_exceeded",
				"type":      "usage_limit_reached",
				"message":   "The usage limit has been reached",
				"resets_at": resetAt,
		REDACTED,
	REDACTED)
REDACTED))
	defer wsServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "unit-test-agent/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_http_should_not_run"REDACTED`)),
	REDACTED,
REDACTED

	cfg := newOpenAIWSV2TestConfig()
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true

	account := Account{
		ID:          501,
		Name:        "openai-ws-rate-limit-event",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": wsServer.URL,
	REDACTED,
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
	REDACTED,
REDACTED
	repo := &openAIWSRateLimitSignalRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{accountREDACTEDREDACTEDREDACTED
	rateSvc := &RateLimitService{accountRepo: repoREDACTED
	svc := &OpenAIGatewayService{
		accountRepo:      repo,
		rateLimitService: rateSvc,
		httpUpstream:     upstream,
		cache:            &stubGatewayCache{REDACTED,
		cfg:              cfg,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
REDACTED

	body := []byte(`{"model":"gpt-5.1","stream":false,"input":[{"type":"input_text","text":"hello"REDACTED]REDACTED`)
	result, err := svc.Forward(context.Background(), c, &account, body)
REDACTED
	require.Nil(t, result)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Nil(t, upstream.lastReq, "WS 限流 error event 不应回退到同账号 HTTP")
	require.Len(t, repo.rateLimitCalls, 1)
	require.WithinDuration(t, time.Unix(resetAt, 0), repo.rateLimitCalls[0], 2*time.Second)
REDACTED

func TestOpenAIGatewayService_Forward_WSv2Handshake429PersistsRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-codex-primary-used-percent", "100")
		w.Header().Set("x-codex-primary-reset-after-seconds", "7200")
		w.Header().Set("x-codex-primary-window-minutes", "10080")
		w.Header().Set("x-codex-secondary-used-percent", "3")
		w.Header().Set("x-codex-secondary-reset-after-seconds", "1800")
		w.Header().Set("x-codex-secondary-window-minutes", "300")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"rate_limit_exceeded","message":"rate limited"REDACTEDREDACTED`))
REDACTED))
	defer server.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "unit-test-agent/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_http_should_not_run"REDACTED`)),
	REDACTED,
REDACTED

	cfg := newOpenAIWSV2TestConfig()
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true

	account := Account{
		ID:          502,
		Name:        "openai-ws-rate-limit-handshake",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": server.URL,
	REDACTED,
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
	REDACTED,
REDACTED
	repo := &openAIWSRateLimitSignalRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{accountREDACTEDREDACTEDREDACTED
	rateSvc := &RateLimitService{accountRepo: repoREDACTED
	svc := &OpenAIGatewayService{
		accountRepo:      repo,
		rateLimitService: rateSvc,
		httpUpstream:     upstream,
		cache:            &stubGatewayCache{REDACTED,
		cfg:              cfg,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
REDACTED

	body := []byte(`{"model":"gpt-5.1","stream":false,"input":[{"type":"input_text","text":"hello"REDACTED]REDACTED`)
	result, err := svc.Forward(context.Background(), c, &account, body)
REDACTED
	require.Nil(t, result)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Nil(t, upstream.lastReq, "WS 握手 429 不应回退到同账号 HTTP")
	require.Len(t, repo.rateLimitCalls, 1)
	require.NotEmpty(t, repo.updateExtra, "握手 429 的 x-codex 头应立即落库")
	require.Contains(t, repo.updateExtra[0], "codex_usage_updated_at")
REDACTED

func TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_ErrorEventUsageLimitPersistsRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := newOpenAIWSV2TestConfig()
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	resetAt := time.Now().Add(90 * time.Minute).Unix()
	captureConn := &openAIWSCaptureConn{
		events: [][]byte{
			[]byte(`{"type":"error","error":{"code":"rate_limit_exceeded","type":"usage_limit_reached","message":"The usage limit has been reached","resets_at":PLACEHOLDERREDACTEDREDACTED`),
	REDACTED,
REDACTED
	captureConn.events[0] = []byte(strings.ReplaceAll(string(captureConn.events[0]), "PLACEHOLDER", strconv.FormatInt(resetAt, 10)))
	captureDialer := &openAIWSCaptureDialer{conn: captureConnREDACTED
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(captureDialer)

	account := Account{
		ID:          503,
		Name:        "openai-ingress-rate-limit",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
REDACTED
			"api_key": "sk-test",
	REDACTED,
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
	REDACTED,
REDACTED
	repo := &openAIWSRateLimitSignalRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{accountREDACTEDREDACTEDREDACTED
	rateSvc := &RateLimitService{accountRepo: repoREDACTED
	svc := &OpenAIGatewayService{
		accountRepo:      repo,
		rateLimitService: rateSvc,
		httpUpstream:     &httpUpstreamRecorder{REDACTED,
		cache:            &stubGatewayCache{REDACTED,
		cfg:              cfg,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
REDACTED

	serverErrCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeoverREDACTED)
		if err != nil {
			serverErrCh <- err
			return
	REDACTED
		defer func() { _ = conn.CloseNow() REDACTED()

		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		req := r.Clone(r.Context())
		req.Header = req.Header.Clone()
		req.Header.Set("User-Agent", "unit-test-agent/1.0")
		ginCtx.Request = req

		readCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		msgType, firstMessage, readErr := conn.Read(readCtx)
		cancel()
		if readErr != nil {
			serverErrCh <- readErr
			return
	REDACTED
		if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
			serverErrCh <- io.ErrUnexpectedEOF
			return
	REDACTED

		serverErrCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, &account, "sk-test", firstMessage, nil)
REDACTED))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
REDACTED
	defer func() { _ = clientConn.CloseNow() REDACTED()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","stream":falseREDACTED`))
	cancelWrite()
REDACTED

	select {
	case serverErr := <-serverErrCh:
		require.Error(t, serverErr)
		require.Len(t, repo.rateLimitCalls, 1)
		require.WithinDuration(t, time.Unix(resetAt, 0), repo.rateLimitCalls[0], 2*time.Second)
	case <-time.After(5 * time.Second):
		t.Fatal("等待 ingress websocket 结束超时")
REDACTED
REDACTED

func TestOpenAIGatewayService_UpdateCodexUsageSnapshot_ExhaustedSnapshotSetsRateLimit(t *testing.T) {
	repo := &openAICodexSnapshotAsyncRepo{
		updateExtraCh: make(chan map[string]any, 1),
		rateLimitCh:   make(chan time.Time, 1),
REDACTED
	svc := &OpenAIGatewayService{accountRepo: repoREDACTED
	snapshot := &OpenAICodexUsageSnapshot{
		PrimaryUsedPercent:         ptrFloat64WS(100),
		PrimaryResetAfterSeconds:   ptrIntWS(3600),
		PrimaryWindowMinutes:       ptrIntWS(10080),
		SecondaryUsedPercent:       ptrFloat64WS(12),
		SecondaryResetAfterSeconds: ptrIntWS(1200),
		SecondaryWindowMinutes:     ptrIntWS(300),
REDACTED
	before := time.Now()
	svc.updateCodexUsageSnapshot(context.Background(), 601, snapshot)

	select {
	case updates := <-repo.updateExtraCh:
		require.Equal(t, 100.0, updates["codex_7d_used_percent"])
	case <-time.After(2 * time.Second):
		t.Fatal("等待 codex 快照落库超时")
REDACTED

	select {
	case resetAt := <-repo.rateLimitCh:
		require.WithinDuration(t, before.Add(time.Hour), resetAt, 2*time.Second)
	case <-time.After(2 * time.Second):
		t.Fatal("等待 codex 100% 自动切换限流超时")
REDACTED
REDACTED

func TestOpenAIGatewayService_UpdateCodexUsageSnapshot_NonExhaustedSnapshotDoesNotSetRateLimit(t *testing.T) {
	repo := &openAICodexSnapshotAsyncRepo{
		updateExtraCh: make(chan map[string]any, 1),
		rateLimitCh:   make(chan time.Time, 1),
REDACTED
	svc := &OpenAIGatewayService{accountRepo: repoREDACTED
	snapshot := &OpenAICodexUsageSnapshot{
		PrimaryUsedPercent:         ptrFloat64WS(94),
		PrimaryResetAfterSeconds:   ptrIntWS(3600),
		PrimaryWindowMinutes:       ptrIntWS(10080),
		SecondaryUsedPercent:       ptrFloat64WS(22),
		SecondaryResetAfterSeconds: ptrIntWS(1200),
		SecondaryWindowMinutes:     ptrIntWS(300),
REDACTED
	svc.updateCodexUsageSnapshot(context.Background(), 602, snapshot)

	select {
	case <-repo.updateExtraCh:
	case <-time.After(2 * time.Second):
		t.Fatal("等待 codex 快照落库超时")
REDACTED

	select {
	case resetAt := <-repo.rateLimitCh:
		t.Fatalf("unexpected rate limit reset at: %v", resetAt)
	case <-time.After(200 * time.Millisecond):
REDACTED
REDACTED

func ptrFloat64WS(v float64) *float64 { return &v REDACTED
func ptrIntWS(v int) *int             { return &v REDACTED

func TestOpenAIWSErrorHTTPStatusFromRaw_UsageLimitReachedIs429(t *testing.T) {
	require.Equal(t, http.StatusTooManyRequests, openAIWSErrorHTTPStatusFromRaw("", "usage_limit_reached"))
	require.Equal(t, http.StatusTooManyRequests, openAIWSErrorHTTPStatusFromRaw("rate_limit_exceeded", ""))
REDACTED
