package service

import (
	"bytes"
	"encoding/json"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const (
	codexAnalyticsEndpointDefault = "https://chatgpt.com/backend-api/codex/analytics-events/events"
	codexMetricsEndpointDefault   = "https://ab.chatgpt.com/otlp/v1/metrics"
	codexStatsigAPIKeyDefault     = "client-MkRuleRQBd6qakfnDYqJVR9JuXcY57Ljly3vi5JVUIO"
	codexTelemetryQueueSize       = 1024
	codexTelemetryWorkers         = 4
	codexTelemetryTimeout         = 10 * time.Second
	codexTelemetryDropLogInterval = time.Minute
	codexTelemetryStateTTL        = 5 * time.Minute
	codexTelemetryMaxEventBytes   = 8 << 20
)

var (
	codexAnalyticsEndpoint = codexAnalyticsEndpointDefault
	codexMetricsEndpoint   = codexMetricsEndpointDefault
	codexTelemetryRandIntN = rand.IntN
	codexTelemetryGlobal   = newCodexTelemetryManager()
)

type codexTelemetryClient struct {
	localID       int64
	accessToken   string
	accountID     string
	proxyURL      string
	concurrency   int
	userAgent     string
	originator    string
	version       string
	httpUpstream  HTTPUpstream
	statsigAPIKey string
}

type codexTelemetryProfile struct {
	client       codexTelemetryClient
	sessionID    string
	threadID     string
	turnID       string
	rootTurnID   string
	model        string
	effort       string
	serviceTier  string
	started      time.Time
	firstThread  bool
	dynamicTool  bool
	command      bool
	fileChange   bool
	turnMetadata gjson.Result
}

type codexTelemetryRequest struct {
	body      []byte
	headers   http.Header
	sessionID string
	ids       *codexFingerprintIDs
}

type codexTelemetryTerminal struct {
	status     string
	body       []byte
	firstEvent time.Time
	firstToken time.Time
}

type codexTelemetryAttempt struct {
	profile codexTelemetryProfile
	// mu protects firstEvent/firstToken plus the pending status: the stream
	// reader writes them in processEvent, while Close may finish from another
	// goroutine.
	mu         sync.Mutex
	firstEvent time.Time
	firstToken time.Time
	// pendingStatus is the terminal status recorded by a failed upstream
	// attempt. It is only emitted by finishPending at the end of the turn, so
	// an attempt that is about to be retried does not report a turn of its own.
	pendingStatus string
	// lastHTTPStatus keeps the most recent non-2xx upstream status. Not wired
	// into any event yet: codex_error_http_status_code / codex_error_kind are
	// hardcoded nil in codexTurnEventBase and codex_error_kind is a Codex
	// internal enum, where a guessed value is worse telemetry than nil.
	lastHTTPStatus int
	done           sync.Once
}

// codexTelemetryTurn binds one user turn's telemetry to the request.
//
// Every beginCodexTelemetry call site sits inside a retry loop or a tail
// recursion, so without this memo one user turn emits one turn's worth of
// telemetry per upstream attempt — with an identical turn_id whenever Codex
// fingerprint convergence is on, since stagedCodexFingerprintIDs is stable
// across retries. attempt == nil is a cached "not eligible" verdict that keeps
// retries from re-running codexTelemetryRequestExcluded's body scans.
type codexTelemetryTurn struct {
	accountID int64
	attempt   *codexTelemetryAttempt
}

// codexTelemetryTurnContextKey 暂存本请求的遥测回合。
// 与 codexFingerprintIDsContextKey 一样按账号校验：handler 会在同一个
// gin.Context 上换账号 failover，A 号的回合不得被 B 号复用。
const codexTelemetryTurnContextKey = "openai_codex_telemetry_turn"

func stagedCodexTelemetryTurn(c *gin.Context, account *Account) (*codexTelemetryTurn, bool) {
	if c == nil || account == nil {
		return nil, false
	}
	value, ok := c.Get(codexTelemetryTurnContextKey)
	if !ok {
		return nil, false
	}
	turn, ok := value.(*codexTelemetryTurn)
	if !ok || turn == nil || turn.accountID != account.ID {
		return nil, false
	}
	return turn, true
}

func stageCodexTelemetryTurn(c *gin.Context, account *Account, attempt *codexTelemetryAttempt) {
	if c == nil || account == nil {
		return
	}
	c.Set(codexTelemetryTurnContextKey, &codexTelemetryTurn{accountID: account.ID, attempt: attempt})
}

// finishCodexTelemetryTurn 结束并清除本请求暂存的遥测回合。
//
// 由四个最外层转发 frame 以 defer 调用。若回合已被流终止事件或 body Close
// 结束，finish 的 sync.Once 会吞掉本次调用；只有「所有 attempt 都失败、从未
// 拿到可观测的流」的回合才会在这里补发唯一一条终止事件。
//
// 清除 memo 是语义的一部分而非优化：handler 的同账号重试（failover_loop.go
// 的 FailoverContinue）会在同一个 gin.Context 上重入 Forward，不清除的话第二
// 次会拿到已 done 的 attempt，真正成功的回合会被永久记成 failed。多回合的
// WS HTTP bridge 也靠它让下一个回合重新开始。
func finishCodexTelemetryTurn(c *gin.Context) {
	if c == nil {
		return
	}
	value, ok := c.Get(codexTelemetryTurnContextKey)
	if !ok {
		return
	}
	c.Set(codexTelemetryTurnContextKey, (*codexTelemetryTurn)(nil))
	if turn, ok := value.(*codexTelemetryTurn); ok && turn != nil {
		turn.attempt.finishPending()
	}
}

type codexTelemetryJob struct {
	client  codexTelemetryClient
	url     string
	body    []byte
	metrics bool
}

type codexTelemetryManager struct {
	once        sync.Once
	queue       chan codexTelemetryJob
	mu          sync.Mutex
	threads     map[string]time.Time
	metrics     map[int64]*codexMetricState
	dropped     atomic.Int64
	dropLogUnix atomic.Int64
}

func newCodexTelemetryManager() *codexTelemetryManager {
	return &codexTelemetryManager{
		queue:   make(chan codexTelemetryJob, codexTelemetryQueueSize),
		threads: make(map[string]time.Time),
		metrics: make(map[int64]*codexMetricState),
	}
}

func newCodexTelemetryUUID() string {
	return uuid.NewString()
}

func (m *codexTelemetryManager) start() {
	m.once.Do(func() {
		for range codexTelemetryWorkers {
			go m.worker()
		}
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for now := range ticker.C {
				m.flushMetrics(now)
			}
		}()
	})
}

func (m *codexTelemetryManager) worker() {
	for job := range m.queue {
		if err := sendCodexTelemetryJob(job); err != nil {
			logger.L().Warn("codex telemetry send failed", zap.Error(err))
		}
	}
}

func (m *codexTelemetryManager) enqueue(job codexTelemetryJob) {
	m.start()
	select {
	case m.queue <- job:
	default:
		dropped := m.dropped.Add(1)
		now := time.Now().Unix()
		last := m.dropLogUnix.Load()
		if now-last >= int64(codexTelemetryDropLogInterval/time.Second) && m.dropLogUnix.CompareAndSwap(last, now) {
			logger.L().Warn("codex telemetry queue full, dropping batches", zap.Int64("dropped", dropped))
		}
	}
}

func (m *codexTelemetryManager) markThread(accountID int64, threadID string, now time.Time) bool {
	key := strconv.FormatInt(accountID, 10) + ":" + threadID
	m.mu.Lock()
	defer m.mu.Unlock()
	_, found := m.threads[key]
	if len(m.threads) >= 4096 {
		m.threads = make(map[string]time.Time)
		found = false
	}
	m.threads[key] = now
	return !found
}

func (s *OpenAIGatewayService) processCodexTelemetryEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.CodexTelemetryEnabled
}

func (s *OpenAIGatewayService) codexStatsigAPIKey() string {
	if s != nil && s.cfg != nil {
		if value := strings.TrimSpace(s.cfg.Gateway.CodexStatsigAPIKey); value != "" {
			return value
		}
	}
	return codexStatsigAPIKeyDefault
}

func ginRequestPath(c *gin.Context) string {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return ""
	}
	return c.Request.URL.Path
}

func codexTelemetryPathExcluded(c *gin.Context) bool {
	path := strings.ToLower(ginRequestPath(c))
	if path == "" {
		return false
	}
	return strings.Contains(path, "count_tokens") ||
		strings.Contains(path, "/embeddings") ||
		strings.Contains(path, "/images")
}

func turnMetadataIndicatesCompaction(c *gin.Context, body []byte) bool {
	hasCompaction := func(raw string) bool {
		return strings.Contains(strings.ToLower(raw), "compaction")
	}
	if c != nil && hasCompaction(c.GetHeader(codexTurnMetadataHeader)) {
		return true
	}
	return hasCompaction(gjson.GetBytes(body, "client_metadata.x-codex-turn-metadata").String())
}

func codexTelemetryRequestExcluded(c *gin.Context, body []byte) bool {
	if !gjson.ValidBytes(body) {
		return true
	}
	if isOpenAIResponsesCompactPath(c) || isOpenAINativeCompactionV2(c) || HasCompactionTriggerInInput(body) {
		return true
	}
	if codexTelemetryPathExcluded(c) {
		return true
	}
	if turnMetadataIndicatesCompaction(c, body) {
		return true
	}
	model := gjson.GetBytes(body, "model").String()
	return IsExplicitImageGenerationIntent(ginRequestPath(c), model, body)
}

// codexTelemetryEligible runs only the guards that depend on neither the
// request body nor the outbound headers, so a caller can skip expensive payload
// preparation when telemetry cannot run at all. Every check is in-memory:
// codexAccountIdentitySource is a gin.Context lookup whose DB resolution was
// already done by prepareCodexAccountIdentitySource at the forwarder entry.
//
// This must stay a superset of beginCodexTelemetry's body-independent guards —
// if it ever drifts into a subset, callers that gate on it silently drop real
// telemetry. TestCodexTelemetryEligiblePredicateMatchesBeginGuards pins that.
func (s *OpenAIGatewayService) codexTelemetryEligible(c *gin.Context, account *Account) bool {
	if s == nil || !s.processCodexTelemetryEnabled() || account == nil || account.IsGrok() {
		return false
	}
	source := codexAccountIdentitySource(c, account)
	return source != nil && source.IsCodexTelemetryEnabled() && !source.IsOpenAIAgentIdentity()
}

// beginCodexTelemetry returns this turn's telemetry attempt, creating it on the
// first upstream attempt and memoizing it for every retry that follows. See
// codexTelemetryTurn for why the memo exists.
func (s *OpenAIGatewayService) beginCodexTelemetry(c *gin.Context, account *Account, body []byte, headers http.Header) *codexTelemetryAttempt {
	if turn, ok := stagedCodexTelemetryTurn(c, account); ok {
		return turn.attempt
	}
	if !s.codexTelemetryEligible(c, account) {
		return nil
	}
	// The free checks run first: codexTelemetryRequestExcluded is the only
	// expensive guard here (several gjson scans of the whole body).
	if headers == nil || codexTelemetryRequestExcluded(c, body) {
		stageCodexTelemetryTurn(c, account, nil)
		return nil
	}
	source := codexAccountIdentitySource(c, account)
	client, ok := s.snapshotCodexTelemetryClient(account, source, headers)
	if !ok {
		stageCodexTelemetryTurn(c, account, nil)
		return nil
	}
	input := codexTelemetryRequest{
		body:      body,
		headers:   headers,
		sessionID: headerOrEmpty(headers, "session_id"),
		ids:       stagedCodexFingerprintIDs(c, account),
	}
	if input.sessionID == "" {
		input.sessionID = headerOrEmpty(headers, "session-id")
	}
	profile := buildCodexTelemetryProfile(client, input)
	profile.firstThread = codexTelemetryGlobal.markThread(client.localID, profile.threadID, profile.started)
	profile.dynamicTool = codexTelemetryRandIntN(5) < 2
	profile.command = profile.dynamicTool && codexTelemetryRandIntN(2) == 0
	profile.fileChange = codexTelemetryRandIntN(5) == 0
	attempt := &codexTelemetryAttempt{profile: profile}
	stageCodexTelemetryTurn(c, account, attempt)
	codexTelemetryGlobal.enqueueAnalytics(codexInitializationEvents(profile))
	codexTelemetryGlobal.touchMetrics(profile)
	return attempt
}

func (s *OpenAIGatewayService) snapshotCodexTelemetryClient(selected, source *Account, headers http.Header) (codexTelemetryClient, bool) {
	if selected == nil {
		selected = source
	}
	if source == nil {
		return codexTelemetryClient{}, false
	}
	client := codexTelemetryClient{
		localID:       source.ID,
		accessToken:   strings.TrimSpace(source.GetOpenAIAccessToken()),
		accountID:     strings.TrimSpace(source.GetChatGPTAccountID()),
		concurrency:   selected.Concurrency,
		httpUpstream:  s.httpUpstream,
		statsigAPIKey: s.codexStatsigAPIKey(),
	}
	if selected.ProxyID != nil && selected.Proxy != nil {
		client.proxyURL = selected.Proxy.URL()
	} else if source.ProxyID != nil && source.Proxy != nil {
		client.proxyURL = source.Proxy.URL()
	}
	if client.accessToken == "" || client.accountID == "" {
		return codexTelemetryClient{}, false
	}
	client.userAgent = strings.TrimSpace(headers.Get("user-agent"))
	client.originator = strings.TrimSpace(headers.Get("originator"))
	client.version = strings.TrimSpace(headers.Get("version"))
	if client.version == "" {
		client.version = strings.TrimSpace(headers.Get("Version"))
	}
	if client.originator == "" {
		client.originator = codexTelemetryOriginator(client.userAgent, headers)
	}
	if client.version == "" {
		client.version = openaiCodexVersionFromUserAgent(client.userAgent)
	}
	return client, client.userAgent != "" && client.originator != ""
}

func openaiCodexVersionFromUserAgent(userAgent string) string {
	return strings.TrimSpace(codexClientVersionFromUA(userAgent))
}

func codexTelemetryOriginator(userAgent string, headers http.Header) string {
	value := ""
	if headers != nil {
		value = strings.TrimSpace(headers.Get("originator"))
	}
	if openai.IsCodexOfficialClientByHeaders(userAgent, value) && value != "" {
		return value
	}
	originator, _, ok := openai.PairCodexClientIdentity(userAgent)
	if ok && originator != "" {
		return originator
	}
	return openai.CodexDefaultOriginator
}

func codexAnalyticsIsolatedEvent(eventType string) bool {
	return eventType == "codex_accepted_line_fingerprints"
}

func (m *codexTelemetryManager) enqueueAnalytics(events []codexAnalyticsEvent) {
	if len(events) == 0 {
		return
	}
	batch := make([]codexAnalyticsEvent, 0, len(events))
	flush := func() {
		if len(batch) == 0 {
			return
		}
		m.enqueueAnalyticsBatch(batch)
		batch = batch[:0]
	}
	for _, event := range events {
		if codexAnalyticsIsolatedEvent(event.EventType) {
			flush()
			m.enqueueAnalyticsBatch([]codexAnalyticsEvent{event})
			continue
		}
		batch = append(batch, event)
	}
	flush()
}

func (m *codexTelemetryManager) enqueueAnalyticsBatch(events []codexAnalyticsEvent) {
	if len(events) == 0 {
		return
	}
	body, err := json.Marshal(map[string]any{"events": events})
	if err == nil {
		m.enqueue(codexTelemetryJob{client: events[0].client, url: codexAnalyticsEndpoint, body: body})
	}
}

// observeResult records one upstream attempt of this turn.
//
// A failed attempt only records a pending status: every caller sits inside a
// retry loop, so emitting a terminal turn event here would report one turn per
// attempt. finishCodexTelemetryTurn emits the pending status once, at the end
// of the turn, if nothing better happened. A 2xx hands the turn to the stream,
// which reports the real terminal status through observeEventJSON / Close.
func (a *codexTelemetryAttempt) observeResult(resp *http.Response, err error) {
	if a == nil {
		return
	}
	if err != nil || resp == nil {
		a.markFailed(0)
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || resp.Body == nil {
		a.markFailed(resp.StatusCode)
		return
	}
	resp.Body = &codexTelemetryBody{ReadCloser: resp.Body, attempt: a}
}

func (a *codexTelemetryAttempt) markFailed(status int) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.pendingStatus = "failed"
	if status != 0 {
		a.lastHTTPStatus = status
	}
	a.mu.Unlock()
}

// finishPending ends a turn that no stream ever terminated. The fallback is
// "interrupted" rather than "completed" to match codexTelemetryBody.Close,
// which reports a 2xx whose stream never reached a terminal event the same way.
func (a *codexTelemetryAttempt) finishPending() {
	if a == nil {
		return
	}
	a.mu.Lock()
	status := a.pendingStatus
	a.mu.Unlock()
	if status == "" {
		status = "interrupted"
	}
	a.finish(status, nil)
}

func (a *codexTelemetryAttempt) finish(status string, terminal []byte) {
	if a == nil {
		return
	}
	a.done.Do(func() {
		a.mu.Lock()
		firstEvent, firstToken := a.firstEvent, a.firstToken
		a.mu.Unlock()
		result := codexTelemetryTerminal{status: status, body: terminal, firstEvent: firstEvent, firstToken: firstToken}
		codexTelemetryGlobal.enqueueAnalytics(codexTerminalEvents(a.profile, result))
		codexTelemetryGlobal.recordTurnMetrics(a.profile, result)
	})
}

func (a *codexTelemetryAttempt) observeEventJSON(data []byte) {
	if a == nil || !json.Valid(data) {
		return
	}
	now := time.Now()
	typ := gjson.GetBytes(data, "type").String()
	a.mu.Lock()
	if a.firstEvent.IsZero() {
		a.firstEvent = now
	}
	if a.firstToken.IsZero() && strings.HasSuffix(typ, ".delta") {
		a.firstToken = now
	}
	a.mu.Unlock()
	switch typ {
	case "response.completed":
		a.finish("completed", data)
	case "response.failed", "error":
		a.finish("failed", data)
	case "response.incomplete":
		a.finish("interrupted", data)
	}
}

type codexTelemetryBody struct {
	io.ReadCloser
	attempt  *codexTelemetryAttempt
	pending  []byte
	event    []byte
	dropping bool
}

func (b *codexTelemetryBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.observe(p[:n])
	}
	if err == io.EOF {
		b.flushJSON()
		b.attempt.finish("interrupted", nil)
	}
	return n, err
}

func (b *codexTelemetryBody) Close() error {
	b.attempt.finish("interrupted", nil)
	return b.ReadCloser.Close()
}

func (b *codexTelemetryBody) observe(data []byte) {
	for len(data) > 0 {
		i := bytes.IndexByte(data, '\n')
		part := data
		if i >= 0 {
			part = data[:i]
		}
		if !b.dropping && len(b.pending)+len(part) <= codexTelemetryMaxEventBytes {
			b.pending = append(b.pending, part...)
		} else {
			b.pending, b.event, b.dropping = nil, nil, true
		}
		if i < 0 {
			return
		}
		b.line(bytes.TrimSuffix(b.pending, []byte{'\r'}))
		if cap(b.pending) > 256<<10 {
			b.pending = nil
		} else {
			b.pending = b.pending[:0]
		}
		data = data[i+1:]
	}
}

func (b *codexTelemetryBody) line(line []byte) {
	if len(line) == 0 {
		b.attempt.observeEventJSON(b.event)
		if cap(b.event) > 256<<10 {
			b.event = nil
		} else {
			b.event = b.event[:0]
		}
		b.dropping = false
		return
	}
	if bytes.HasPrefix(line, []byte("data:")) && !b.dropping {
		part := bytes.TrimSpace(line[5:])
		if len(b.event)+len(part) <= codexTelemetryMaxEventBytes {
			b.event = append(b.event, part...)
		}
	}
}

func (b *codexTelemetryBody) flushJSON() {
	if len(b.event) > 0 {
		b.attempt.observeEventJSON(b.event)
		return
	}
	if json.Valid(b.pending) {
		status := gjson.GetBytes(b.pending, "status").String()
		if status == "completed" {
			b.attempt.finish("completed", b.pending)
		} else if status == "failed" {
			b.attempt.finish("failed", b.pending)
		}
	}
}
