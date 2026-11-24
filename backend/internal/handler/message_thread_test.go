//go:build unit

package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// These tests drive GatewayHandler.Messages against two local fake upstreams
// that implement the message-thread protocol, a miniredis and no real account.
// Each fake keeps threads per caller like the real API does: a continue request
// that reaches the wrong upstream, or the right one with a changed beta set,
// model, system or tools, fails there too.

const (
	threadTestBeta    = "message-threads-2026-08-12"
	threadTestModel   = "claude-sonnet-4-5"
	threadTestTools   = `[{"name":"lookup","description":"Look something up","input_schema":{"type":"object"}}]`
	threadTestSystem  = `"You are a support agent for Acme."`
	threadTestSecret  = "do-not-store-this-conversation"
	threadTestSession = "0f1e2d3c-4b5a-4978-8a6b-5c4d3e2f1a0b"

	// Deliberately oddly formatted: the client must receive these exact bytes.
	threadTestNotFound    = "{ \"type\":\"error\", \"error\":{\"type\":\"not_found_error\",\"message\":\"original upstream wording\",\"details\":{\"error_code\":\"thread_not_found\"}},\"request_id\":\"req_original\" }\n"
	threadTestUnsupported = "{\"type\": \"error\",\n \"error\": {\"type\":\"invalid_request_error\",\"message\":\"thread: original wording\",\"details\":{\"error_code\":\"thread_unsupported_request\",\"unsupported\":[\"compaction\"]}},\"request_id\":\"req_second\"}\n"
)

// threadRouter is the gateway's HTTP transport. It sends each request to the
// fake that stands for the selected account and to nothing else.
type threadRouter struct {
	service.HTTPUpstream
	routes map[int64]*threadFakeUpstream
	hosts  sync.Map // account id → host the gateway addressed
	stray  atomic.Int32
}

func (r *threadRouter) DoWithTLS(req *http.Request, _ string, accountID int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	target := r.routes[accountID]
	if target == nil {
		r.stray.Add(1)
		return nil, errors.New("no local upstream for this account")
	}
	r.hosts.Store(accountID, req.URL.Host)
	local, _ := url.Parse(target.srv.URL)
	out := req.Clone(req.Context())
	out.URL.Scheme, out.URL.Host, out.Host, out.RequestURI = local.Scheme, local.Host, local.Host, ""
	return target.srv.Client().Do(out)
}

type threadAccountRepo struct {
	service.AccountRepository
	mu          sync.Mutex
	rows        map[int64]*service.Account
	script      []*service.Account
	modelLimits []string
}

func (r *threadAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// script, when set, is what successive reads of the row return: a row
	// that changes between two reads within one request.
	if len(r.script) > 0 {
		row := r.script[0]
		r.script = r.script[1:]
		return row, nil
	}
	if account := r.rows[id]; account != nil {
		return account, nil
	}
	return nil, service.ErrAccountNotFound
}

func (r *threadAccountRepo) set(id int64, account *service.Account) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if account == nil {
		delete(r.rows, id)
		return
	}
	r.rows[id] = account
}

func (r *threadAccountRepo) SetRateLimited(_ context.Context, id int64, resetAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if account := r.rows[id]; account != nil {
		account.RateLimitResetAt = &resetAt
	}
	return nil
}

func (r *threadAccountRepo) SetOverloaded(_ context.Context, id int64, until time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if account := r.rows[id]; account != nil {
		account.OverloadUntil = &until
	}
	return nil
}

func (r *threadAccountRepo) UpdateExtra(context.Context, int64, map[string]any) error { return nil }

func (r *threadAccountRepo) SetError(context.Context, int64, string) error { return nil }

func (r *threadAccountRepo) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	return nil
}

func (r *threadAccountRepo) ListModelAvailabilityCandidates(context.Context, *int64, []string, bool) ([]service.Account, error) {
	return nil, nil
}

// A missing model is recorded against the account's model, as for any request.
func (r *threadAccountRepo) SetModelRateLimit(_ context.Context, id int64, scope string, _ time.Time, _ ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modelLimits = append(r.modelLimits, fmt.Sprintf("%d:%s", id, scope))
	return nil
}

// threadSettings serves the two settings these tests change at run time.
type threadSettings struct {
	service.SettingRepository
	policy    string
	rectifier string
	// The beta policy can be unconfigured (policyMissing) or unreadable: every
	// read from the policyFailFrom-th on fails, counted in policyReads.
	policyMissing  bool
	policyFailFrom int
	policyReads    int
}

func (r *threadSettings) GetValue(_ context.Context, key string) (string, error) {
	switch {
	case key == service.SettingKeyBetaPolicySettings:
		r.policyReads++
		if r.policyFailFrom > 0 && r.policyReads >= r.policyFailFrom {
			return "", errors.New("settings store unavailable")
		}
		if r.policyMissing {
			return "", service.ErrSettingNotFound
		}
		return r.policy, nil
	case key == service.SettingKeyRectifierSettings && r.rectifier != "":
		return r.rectifier, nil
	}
	return "", service.ErrSettingNotFound
}

func (r *threadSettings) Get(ctx context.Context, key string) (*service.Setting, error) {
	value, err := r.GetValue(ctx, key)
	return &service.Setting{Key: key, Value: value}, err
}

func (r *threadSettings) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

type threadScheduler struct {
	*fakeSchedulerCache
	lists atomic.Int32
}

func (s *threadScheduler) GetSnapshot(ctx context.Context, bucket service.SchedulerBucket) ([]*service.Account, bool, error) {
	s.lists.Add(1)
	return s.fakeSchedulerCache.GetSnapshot(ctx, bucket)
}

type threadSessions struct {
	service.SessionLimitCache
	mu       sync.Mutex
	sessions map[int64][]string
	// fail makes the store unreachable; full refuses every registration.
	fail atomic.Bool
	full atomic.Bool
	// The cached window cost per account: absent is a cache miss, costFail
	// makes the cache unreachable. costReads counts the single reads.
	costs     sync.Map
	costFail  atomic.Bool
	costReads atomic.Int32
}

func (s *threadSessions) GetWindowCost(_ context.Context, accountID int64) (float64, bool, error) {
	s.costReads.Add(1)
	if s.costFail.Load() {
		return 0, false, errors.New("window cost cache unavailable")
	}
	if value, ok := s.costs.Load(accountID); ok {
		return value.(float64), true, nil
	}
	return 0, false, nil
}

func (s *threadSessions) SetWindowCost(_ context.Context, accountID int64, cost float64) error {
	if s.costFail.Load() {
		return errors.New("window cost cache unavailable")
	}
	s.costs.Store(accountID, cost)
	return nil
}

func (s *threadSessions) GetWindowCostBatch(_ context.Context, accountIDs []int64) (map[int64]float64, error) {
	if s.costFail.Load() {
		return nil, errors.New("window cost cache unavailable")
	}
	out := map[int64]float64{}
	for _, id := range accountIDs {
		if value, ok := s.costs.Load(id); ok {
			out[id] = value.(float64)
		}
	}
	return out, nil
}

// threadWindowCosts is the durable window cost state behind the cache.
type threadWindowCosts struct {
	*openAIWSUsageHandlerUsageLogRepoStub
	costs sync.Map
	fail  atomic.Bool
	reads atomic.Int32
}

func (r *threadWindowCosts) GetAccountWindowStats(_ context.Context, id int64, _ time.Time) (*usagestats.AccountStats, error) {
	r.reads.Add(1)
	if r.fail.Load() {
		return nil, errors.New("window cost state unavailable")
	}
	out := &usagestats.AccountStats{}
	if value, ok := r.costs.Load(id); ok {
		out.StandardCost = value.(float64)
	}
	return out, nil
}

func (s *threadSessions) RegisterSession(_ context.Context, accountID int64, session string, _ int, _ time.Duration) (bool, error) {
	if s.fail.Load() {
		return false, errors.New("session limit store unavailable")
	}
	if s.full.Load() {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[accountID] = append(s.sessions[accountID], session)
	return true, nil
}

func (s *threadSessions) UnregisterSession(context.Context, int64, string) error { return nil }

func (s *threadSessions) distinct(accountID int64) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := slices.Clone(s.sessions[accountID])
	slices.Sort(out)
	return slices.Compact(out)
}

// threadSlots refuses the first account slot requests, so a request has to
// wait, and refuses every request for an account marked busy.
type threadSlots struct {
	fakeConcurrencyCache
	refuse    atomic.Int32
	busy      sync.Map
	queueFull atomic.Bool
}

func (s *threadSlots) IncrementAccountWaitCount(context.Context, int64, int) (bool, error) {
	return !s.queueFull.Load(), nil
}

// threadRPM reports a fixed request rate per account. failRead makes a read
// of one account fail, failBatch the batched read the scheduler prefetches
// with; reads counts the single reads.
type threadRPM struct {
	counts    sync.Map
	failRead  atomic.Bool
	failBatch atomic.Bool
	// staleBatch makes the batched read answer with counts from before any
	// request was counted: a prefetch that is older than the single read.
	staleBatch atomic.Bool
	reads      atomic.Int32
}

func (r *threadRPM) count(accountID int64) int {
	if value, ok := r.counts.Load(accountID); ok {
		return value.(int)
	}
	return 0
}

func (r *threadRPM) IncrementRPM(_ context.Context, accountID int64) (int, error) {
	return r.count(accountID), nil
}

func (r *threadRPM) GetRPM(_ context.Context, accountID int64) (int, error) {
	r.reads.Add(1)
	if r.failRead.Load() {
		return 0, errors.New("rpm store unavailable")
	}
	return r.count(accountID), nil
}

func (r *threadRPM) GetRPMBatch(_ context.Context, accountIDs []int64) (map[int64]int, error) {
	if r.failBatch.Load() {
		return nil, errors.New("rpm store unavailable")
	}
	out := make(map[int64]int, len(accountIDs))
	for _, id := range accountIDs {
		out[id] = r.count(id)
		if r.staleBatch.Load() {
			out[id] = 0
		}
	}
	return out, nil
}

func (s *threadSlots) AcquireAccountSlot(_ context.Context, accountID int64, _ int, _ string) (bool, error) {
	if _, busy := s.busy.Load(accountID); busy {
		return false, nil
	}
	return s.refuse.Add(-1) < 0, nil
}

// threadGroupRepo can fail, to stand for a group lookup that errors.
type threadGroupRepo struct {
	*fakeGroupRepo
	fail atomic.Bool
}

func (r *threadGroupRepo) GetByID(ctx context.Context, id int64) (*service.Group, error) {
	if r.fail.Load() {
		return nil, errors.New("group store unavailable")
	}
	return r.fakeGroupRepo.GetByID(ctx, id)
}

func (r *threadGroupRepo) GetByIDLite(ctx context.Context, id int64) (*service.Group, error) {
	return r.GetByID(ctx, id)
}

// threadWriter records, for every write to the client, how many bindings the
// store held at that moment.
type threadWriter struct {
	*httptest.ResponseRecorder
	db     *miniredis.Miniredis
	mu     sync.Mutex
	writes []threadWrite
	// onWrite runs before each write reaches the client.
	onWrite func(data string)
}

type threadWrite struct {
	data     string
	bindings int
}

func (w *threadWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.writes = append(w.writes, threadWrite{data: string(p), bindings: threadBindingCount(w.db)})
	w.mu.Unlock()
	if w.onWrite != nil {
		w.onWrite(string(p))
	}
	return w.ResponseRecorder.Write(p)
}

func (w *threadWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

func threadBindingKeys(db *miniredis.Miniredis) []string {
	var keys []string
	for _, key := range db.Keys() {
		if strings.HasPrefix(key, "gateway:claude:thread:v1:") && strings.Contains(key, "}:b:") {
			keys = append(keys, key)
		}
	}
	return keys
}

func threadBindingCount(db *miniredis.Miniredis) int { return len(threadBindingKeys(db)) }

type threadHarness struct {
	t        *testing.T
	kind     string
	db       *miniredis.Miniredis
	a, b     *threadFakeUpstream
	accountA *service.Account
	accountB *service.Account
	repo     *threadAccountRepo
	router   *threadRouter
	sched    *threadScheduler
	sessions *threadSessions
	slots    *threadSlots
	bill     *compatStreamUsageBillingRepo
	funds    *handlerInflightCache
	settings *threadSettings
	group    *service.Group
	groups   *threadGroupRepo
	rpm      *threadRPM
	costs    *threadWindowCosts
	// realSessions is the production session limit cache, when the harness
	// was built with it instead of the recording double.
	realSessions service.SessionLimitCache
	// betaExtra is added to the client's anthropic-beta on every request.
	betaExtra string
	cfg       *config.Config
	handler   *GatewayHandler
	userID    int64
	keyID     int64
	// fixed is sent on every thread request of this harness: the fixed fields
	// of a thread have to be the same on each of its requests.
	fixed string
}

// kinds: "apikey", "passthrough" (API-key passthrough), "oauth-cli" (subscription
// account, Claude Code client), "oauth" and "setup-token" (subscription account,
// any other client).
func threadTestAccount(kind string, id int64, secret string, groupID int64) *service.Account {
	account := &service.Account{
		ID: id, Name: fmt.Sprintf("account-%d", id), Platform: service.PlatformAnthropic, Status: service.StatusActive,
		Schedulable: true, Concurrency: 4, Priority: 1,
		AccountGroups: []service.AccountGroup{{AccountID: id, GroupID: groupID}},
	}
	switch kind {
	case "apikey", "passthrough":
		account.Type = service.AccountTypeAPIKey
		account.Credentials = map[string]any{"api_key": secret}
		account.Extra = map[string]any{"anthropic_passthrough": kind == "passthrough"}
	default:
		account.Type = service.AccountTypeOAuth
		if kind == "setup-token" {
			account.Type = service.AccountTypeSetupToken
		}
		account.Credentials = map[string]any{"access_token": secret}
		account.Extra = map[string]any{"account_uuid": "uuid-" + secret, "org_uuid": "org-" + secret, "max_sessions": 5}
	}
	return account
}

func newThreadHarness(t *testing.T, kind string) *threadHarness {
	t.Helper()
	return newThreadHarnessFor(t, kind, true)
}

// loadBatch selects the load-aware scheduler (the default in production) or
// the legacy one; both have to keep a thread on its account.
func newThreadHarnessFor(t *testing.T, kind string, loadBatch bool) *threadHarness {
	t.Helper()
	return newThreadHarnessWith(t, kind, threadHarnessOptions{legacyScheduler: !loadBatch})
}

type threadHarnessOptions struct {
	legacyScheduler bool
	// productionHTTP sends upstream requests through the gateway's real HTTP
	// client, to the fakes' own addresses, instead of through threadRouter.
	productionHTTP bool
	// productionSessions uses the real session limit cache on the test Redis.
	productionSessions bool
	productionRPM      bool // use the RPM adapter on the same miniredis clock
	// noSessionStore builds the gateway without a session limit store.
	noSessionStore bool
	// noRPMStore and noWindowCostState build it without those stores.
	noRPMStore        bool
	noWindowCostState bool
}

func newThreadHarnessWith(t *testing.T, kind string, options threadHarnessOptions) *threadHarness {
	t.Helper()
	loadBatch := !options.legacyScheduler
	gin.SetMode(gin.TestMode)
	h := &threadHarness{t: t, kind: kind, userID: 52, keyID: 47}
	h.db = miniredis.RunT(t)
	kv := redis.NewClient(&redis.Options{Addr: h.db.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = kv.Close() })

	h.a, h.b = newThreadFakeUpstream(t, "a"), newThreadFakeUpstream(t, "b")
	h.a.principals["secret-a"], h.b.principals["secret-b"] = "caller-a", "caller-b"
	groupID := int64(941)
	h.group = &service.Group{ID: groupID, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive, RateMultiplier: 1}
	h.accountA = threadTestAccount(kind, 71, "secret-a", groupID)
	h.accountB = threadTestAccount(kind, 72, "secret-b", groupID)
	h.accountB.Priority = 50
	h.repo = &threadAccountRepo{rows: map[int64]*service.Account{71: h.accountA, 72: h.accountB}}
	h.router = &threadRouter{routes: map[int64]*threadFakeUpstream{71: h.a, 72: h.b}}
	h.sched = &threadScheduler{fakeSchedulerCache: &fakeSchedulerCache{accounts: []*service.Account{h.accountA, h.accountB}}}
	h.sessions = &threadSessions{sessions: map[int64][]string{}}
	h.slots = &threadSlots{}
	h.bill = &compatStreamUsageBillingRepo{}
	h.funds = newHandlerInflightCache(100)
	h.settings = &threadSettings{policy: `{"rules":[]}`}
	h.rpm = &threadRPM{}

	cfg := &config.Config{}
	h.cfg = cfg
	var upstream service.HTTPUpstream = h.router
	if options.productionHTTP {
		// Each account is addressed where its fake really listens.
		cfg.Security.URLAllowlist.AllowInsecureHTTP = true
		for account, fake := range map[*service.Account]*threadFakeUpstream{h.accountA: h.a, h.accountB: h.b} {
			account.Extra[service.MessageThreadAccountFlag] = true
			if account.Type == service.AccountTypeAPIKey {
				account.Credentials["base_url"] = fake.srv.URL
			} else {
				account.Extra["custom_base_url_enabled"], account.Extra["custom_base_url"] = true, fake.srv.URL
			}
		}
		upstream = repository.NewHTTPUpstream(cfg)
	}
	var sessions service.SessionLimitCache = h.sessions
	if options.productionSessions {
		h.realSessions = repository.NewSessionLimitCache(kv, 1) // one minute idle timeout
		sessions = h.realSessions
	}
	if options.noSessionStore {
		sessions = nil
	}
	h.groups = &threadGroupRepo{fakeGroupRepo: &fakeGroupRepo{group: h.group}}
	cfg.Billing.InflightReservation = config.InflightReservationConfig{Enabled: true, TTLSeconds: 60}
	cfg.Gateway.Scheduling = config.GatewaySchedulingConfig{
		StickySessionMaxWaiting: 3, StickySessionWaitTimeout: 45 * time.Second,
		FallbackMaxWaiting: 100, FallbackWaitTimeout: 30 * time.Second, LoadBatchEnabled: loadBatch,
	}
	settings := service.NewSettingService(h.settings, cfg)
	snapshot := service.NewSchedulerSnapshotService(h.sched, nil, nil, nil, nil)
	concurrency := service.NewConcurrencyService(h.slots)
	billing := service.NewBillingCacheService(h.funds, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	rateLimit := service.NewRateLimitService(h.repo, nil, cfg, nil, nil)
	h.costs = &threadWindowCosts{openAIWSUsageHandlerUsageLogRepoStub: &openAIWSUsageHandlerUsageLogRepoStub{}}
	var logs service.UsageLogRepository = h.costs
	if options.noWindowCostState {
		// The public usage repository exposes the window reader directly. A
		// missing durable backend reports an error rather than an empty window.
		h.costs.fail.Store(true)
	}
	var rpm service.RPMCache = h.rpm
	if options.productionRPM {
		rpm = repository.NewRPMCache(kv)
	}
	if options.noRPMStore {
		rpm = nil
	}
	gateway := service.NewGatewayService(
		h.repo, h.groups, logs, h.bill, nil, nil, nil, repository.NewGatewayCache(kv), cfg, snapshot,
		concurrency, service.NewBillingService(cfg, nil), rateLimit, nil, nil, upstream, &service.DeferredService{}, nil, sessions, rpm, nil, settings,
		nil, nil, nil, nil, nil, nil,
	)
	h.handler = &GatewayHandler{
		gatewayService: gateway, billingCacheService: billing, settingService: settings, cfg: cfg, maxAccountSwitches: 3,
		concurrencyHelper: NewConcurrencyHelper(concurrency, SSEPingFormatClaude, time.Millisecond),
	}
	return h
}

type threadTurn struct {
	previous  string
	stream    bool
	noThread  bool
	model     string
	beta      *string
	betaLines []string
	extra     string
	messages  string
	omitDefs  bool // create without system and tools
	plain     bool // send as an ordinary SDK client, whatever the harness kind
	failAt    int  // the client connection breaks at this write (1-based)
	// fixed replaces the harness's fixed fields for this request; tools
	// replaces the declared tools of a create request.
	fixed     *string
	tools     string
	maxTokens int
	// composite stands for a composite route that resolved the public model
	// to the model in the request body, as the route middleware does.
	composite *service.CompositeRouteDecision
	userID    int64
	keyID     int64
	groupID   int64
	writer    *threadWriter
}

func (h *threadHarness) claudeCode() bool { return h.kind == "oauth-cli" }

// threadBrokenWriter is a client that goes away in the middle of a stream.
type threadBrokenWriter struct {
	*httptest.ResponseRecorder
	failAt int
	writes int
}

func (w *threadBrokenWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes >= w.failAt {
		return 0, errors.New("client connection closed")
	}
	return w.ResponseRecorder.Write(p)
}

func (w *threadBrokenWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

func (h *threadHarness) send(turn threadTurn) *httptest.ResponseRecorder {
	h.t.Helper()
	model := turn.model
	if model == "" {
		model = threadTestModel
	}
	maxTokens := 64
	if turn.maxTokens > 0 {
		maxTokens = turn.maxTokens
	}
	fields := []string{fmt.Sprintf(`"model":%q`, model), fmt.Sprintf(`"max_tokens":%d`, maxTokens), fmt.Sprintf(`"stream":%t`, turn.stream)}
	messages := turn.messages
	claudeCode := h.claudeCode() && !turn.plain
	if turn.previous == "" {
		system := threadTestSystem
		if claudeCode {
			system = `[{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude."},{"type":"text","text":"Project notes."}]`
		}
		if !turn.omitDefs {
			tools := threadTestTools
			if turn.tools != "" {
				tools = turn.tools
			}
			fields = append(fields, `"system":`+system, `"tools":`+tools)
		}
		if messages == "" {
			messages = fmt.Sprintf(`[{"role":"user","content":%q}]`, threadTestSecret)
		}
		if !turn.noThread {
			fields = append(fields, `"thread":{"type":"create"}`)
		}
	} else {
		if messages == "" {
			messages = `[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"done"}]}]`
		}
		fields = append(fields, fmt.Sprintf(`"thread":{"type":"continue","previous_message_id":%q}`, turn.previous))
	}
	fields = append(fields, `"messages":`+messages)
	if claudeCode {
		fields = append(fields, fmt.Sprintf(`"metadata":{"user_id":"{\"device_id\":\"%s\",\"account_uuid\":\"\",\"session_id\":\"%s\"}"}`, strings.Repeat("ab", 32), threadTestSession))
	}
	fixed := h.fixed
	if turn.fixed != nil {
		fixed = *turn.fixed
	}
	if fixed != "" && !turn.noThread {
		fields = append(fields, fixed)
	}
	if turn.extra != "" {
		fields = append(fields, turn.extra)
	}
	body := "{" + strings.Join(fields, ",") + "}"

	rec := httptest.NewRecorder()
	var writer http.ResponseWriter = rec
	if turn.writer != nil {
		turn.writer.ResponseRecorder, turn.writer.db = rec, h.db
		writer = turn.writer
	}
	if turn.failAt > 0 {
		writer = &threadBrokenWriter{ResponseRecorder: rec, failAt: turn.failAt}
	}
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Anthropic-Version", "2023-06-01")
	beta := threadTestBeta + ",custom-beta-2026-01-01"
	if h.claudeCode() {
		beta = "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14," + threadTestBeta
	}
	if claudeCode {
		c.Request.Header.Set("User-Agent", "claude-cli/2.1.300 (external, cli)")
		c.Request.Header.Set("X-App", "cli")
	}
	if h.betaExtra != "" {
		beta += "," + h.betaExtra
	}
	if turn.beta != nil {
		beta = *turn.beta
	}
	if beta != "" {
		c.Request.Header.Set("Anthropic-Beta", beta)
	}
	for _, line := range turn.betaLines {
		c.Request.Header.Add("Anthropic-Beta", line)
	}
	userID, keyID, groupID := h.userID, h.keyID, h.group.ID
	if turn.userID != 0 {
		userID = turn.userID
	}
	if turn.keyID != 0 {
		keyID = turn.keyID
	}
	group := h.group
	if turn.groupID != 0 {
		groupID = turn.groupID
		copied := *h.group
		copied.ID = groupID
		group = &copied
	}
	key := &service.APIKey{ID: keyID, UserID: userID, Group: group, GroupID: &groupID, User: &service.User{ID: userID, Balance: 100}}
	c.Set(string(middleware.ContextKeyAPIKey), key)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: userID})
	if turn.composite != nil {
		c.Request = c.Request.WithContext(service.WithCompositeRouteDecision(c.Request.Context(), *turn.composite))
	}
	h.handler.Messages(c)
	return rec
}

func (h *threadHarness) commands() []*service.UsageBillingCommand {
	h.bill.mu.Lock()
	defer h.bill.mu.Unlock()
	return slices.Clone(h.bill.commands)
}

func (h *threadHarness) requireCommands(n int) []*service.UsageBillingCommand {
	h.t.Helper()
	require.Eventually(h.t, func() bool { return len(h.commands()) >= n }, 2*time.Second, 5*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	commands := h.commands()
	require.Len(h.t, commands, n)
	return commands
}

func threadResponseID(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	if id := gjson.Get(rec.Body.String(), "id").Str; id != "" {
		return id
	}
	for _, event := range threadReadClientEvents(rec.Body.String()) {
		if event.name == "message_start" && gjson.Get(event.data, "type").Str == "message_start" {
			return gjson.Get(event.data, "message.id").Str
		}
	}
	t.Fatalf("no response id in %s", rec.Body.String())
	return ""
}

func (h *threadHarness) requireNotFound(turn threadTurn) {
	h.t.Helper()
	a, b := h.a.count(), h.b.count()
	rec := h.send(turn)
	require.Equal(h.t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Equal(h.t, "not_found_error", gjson.Get(rec.Body.String(), "error.type").Str)
	require.Equal(h.t, "thread_not_found", gjson.Get(rec.Body.String(), "error.details.error_code").Str)
	require.NotEqual(h.t, threadTestNotFound, rec.Body.String(), "must be the gateway's own answer")
	require.Equal(h.t, a, h.a.count(), "bound upstream must not be called")
	require.Equal(h.t, b, h.b.count(), "another account must never receive a continue request")
}

func threadTestKinds() []string {
	return []string{"apikey", "passthrough", "oauth-cli", "oauth", "setup-token"}
}

// Both directions use the client's spelling on threads, even when a name
// overlaps with the ordinary request/response name rewrite tables.
func TestMessageThreadSubscriptionToolNames(t *testing.T) {
	for _, kind := range []string{"oauth", "setup-token"} {
		for _, name := range []string{"sessions_lookup", "cc_sess_lookup"} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/create_stream=%t", kind, name, stream), func(t *testing.T) {
					h := newThreadHarness(t, kind)
					h.fixed = fmt.Sprintf(`"tool_choice":{"type":"tool","name":%q}`, name)
					created := h.send(threadTurn{stream: stream, tools: strings.Replace(threadTestTools, "lookup", name, 1)})
					id := threadResponseID(t, created)
					require.Equal(t, name, gjson.GetBytes(h.a.last().body, "tools.0.name").Str)
					require.Contains(t, created.Body.String(), fmt.Sprintf(`"name":%q`, name))
					next := h.send(threadTurn{previous: id, stream: !stream})
					threadResponseID(t, next)
					wire := h.a.last().body
					require.Equal(t, name, gjson.GetBytes(wire, "tool_choice.name").Str)
					require.Equal(t, "toolu_1", gjson.GetBytes(wire, "messages.0.content.0.tool_use_id").Str)
					require.False(t, gjson.GetBytes(wire, "tools").Exists())
					require.False(t, gjson.GetBytes(wire, "system").Exists())
					require.Contains(t, next.Body.String(), fmt.Sprintf(`"name":%q`, name))
					require.Equal(t, 2, h.a.count())
					require.Zero(t, h.b.count())
				})
			}
		}
	}
}

// A complete thread: create, two continues with a tool round trip, and a reuse
// of a response id the upstream has already continued.
func TestMessageThreadCreateAndContinue(t *testing.T) {
	for _, kind := range threadTestKinds() {
		for _, firstStream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/create_stream=%t", kind, firstStream), func(t *testing.T) {
				// The streaming variant also runs on the legacy scheduler.
				h := newThreadHarnessFor(t, kind, !firstStream)
				// Fixed for the whole thread, as the protocol requires.
				h.fixed = `"tool_choice":{"type":"tool","name":"lookup"},"cache_control":{"type":"ephemeral"}`
				wireTool := "lookup"

				// An id the gateway never issued is answered before any scheduling.
				h.requireNotFound(threadTurn{previous: "msg_unknown", stream: true})
				require.Zero(t, h.sched.lists.Load(), "an unknown id must not reach the scheduler")

				created := h.send(threadTurn{stream: firstStream})
				first := threadResponseID(t, created)
				require.Contains(t, created.Body.String(), `"name":"lookup"`, "the client sees its own tool name")
				require.Equal(t, 1, h.a.count())
				create := h.a.last()
				createBeta := threadTestCanonicalBeta(create.header)
				require.Contains(t, createBeta, threadTestBeta)
				require.Equal(t, "create", gjson.GetBytes(create.body, "thread.type").Str)
				require.Equal(t, wireTool, gjson.GetBytes(create.body, "tools.0.name").Str)
				require.Equal(t, wireTool, gjson.GetBytes(create.body, "tool_choice.name").Str)
				host, _ := h.router.hosts.Load(int64(71))
				require.Equal(t, "api.anthropic.com", host)
				switch kind {
				case "apikey", "passthrough":
					require.Equal(t, "secret-a", create.header.Get("X-Api-Key"))
					require.Contains(t, createBeta, "custom-beta-2026-01-01")
					require.Equal(t, strings.Trim(threadTestSystem, `"`), gjson.GetBytes(create.body, "system").Str)
				case "oauth-cli":
					require.Equal(t, "Bearer secret-a", create.header.Get("Authorization"))
					require.Equal(t, "Project notes.", gjson.GetBytes(create.body, "system.1.text").Str, "a Claude Code request is not rewritten")
				default:
					require.Equal(t, "Bearer secret-a", create.header.Get("Authorization"))
					require.NotContains(t, createBeta, "custom-beta-2026-01-01")
					require.True(t, gjson.GetBytes(create.body, "system").IsArray())
				}

				// One binding, holding routing metadata only.
				keys := threadBindingKeys(h.db)
				require.Len(t, keys, 1)
				stored, err := h.db.Get(keys[0])
				require.NoError(t, err)
				require.EqualValues(t, 71, gjson.Get(stored, "a").Int())
				require.EqualValues(t, 91020, gjson.Get(stored, "t").Int())
				for _, key := range h.db.Keys() {
					value, err := h.db.Get(key)
					if err != nil {
						continue
					}
					for _, forbidden := range []string{"secret-a", threadTestSecret, "support agent", "Look something up", first} {
						require.NotContains(t, key+value, forbidden)
					}
				}

				// Turn 2 answers the tool call; only free fields change.
				second := h.send(threadTurn{previous: first, stream: !firstStream, extra: `"stop_sequences":["STOP"],"service_tier":"auto"`})
				secondID := threadResponseID(t, second)
				require.Contains(t, second.Body.String(), `"name":"lookup"`)
				require.Equal(t, 2, h.a.count())
				next := h.a.last()
				require.Equal(t, createBeta, threadTestCanonicalBeta(next.header), "the beta set of a thread never changes")
				require.Equal(t, first, gjson.GetBytes(next.body, "thread.previous_message_id").Str)
				require.False(t, gjson.GetBytes(next.body, "system").Exists(), string(next.body))
				require.False(t, gjson.GetBytes(next.body, "tools").Exists(), string(next.body))
				require.Equal(t, wireTool, gjson.GetBytes(next.body, "tool_choice.name").Str)
				require.Equal(t, "toolu_1", gjson.GetBytes(next.body, "messages.0.content.0.tool_use_id").Str)
				require.Equal(t, gjson.GetBytes(create.body, "model").Str, gjson.GetBytes(next.body, "model").Str)

				// Turn 3 continues from the latest response.
				third := h.send(threadTurn{previous: secondID, stream: firstStream})
				thirdID := threadResponseID(t, third)
				require.Equal(t, 3, h.a.count())
				require.Equal(t, createBeta, threadTestCanonicalBeta(h.a.last().header))
				require.False(t, gjson.GetBytes(h.a.last().body, "tools").Exists())

				// Billing follows the upstream usage of every turn, cache included.
				commands := h.requireCommands(3)
				for _, command := range commands {
					require.Equal(t, 10000, command.InputTokens)
					require.Equal(t, 80000, command.CacheReadTokens)
					require.Equal(t, 1000, command.CacheCreationTokens)
					require.Equal(t, 20, command.OutputTokens)
					require.Positive(t, command.BalanceCost)
				}

				// The upstream's own thread error reaches the client unchanged.
				reused := h.send(threadTurn{previous: first, stream: true})
				require.Equal(t, http.StatusBadRequest, reused.Code)
				require.Equal(t, "thread_already_continued", gjson.Get(reused.Body.String(), "error.details.error_code").Str)
				require.Equal(t, "req_fake", gjson.Get(reused.Body.String(), "request_id").Str)
				require.Equal(t, 4, h.a.count())

				// A response id belongs to the caller that received it.
				h.requireNotFound(threadTurn{previous: thirdID, userID: 99})
				h.requireNotFound(threadTurn{previous: thirdID, keyID: 99})
				h.requireNotFound(threadTurn{previous: thirdID, groupID: 99})

				require.Zero(t, h.b.count(), "the other account saw no request at all")
				require.Zero(t, h.router.stray.Load())
				h.requireCommands(3)
				if kind != "apikey" && kind != "passthrough" {
					require.Len(t, h.sessions.distinct(71), 1, "a thread is one session for the account's session limit")
				}
			})
		}
	}
}

// The binding is written before the client can read the end of the response,
// and not before the response is complete.
func TestMessageThreadBindsBeforeTerminalEvent(t *testing.T) {
	for _, kind := range threadTestKinds() {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			writer := &threadWriter{}
			created := h.send(threadTurn{stream: true, writer: writer})
			first := threadResponseID(t, created)
			sawStart, sawStop := false, false
			for _, write := range writer.writes {
				if strings.Contains(write.data, "message_stop") {
					require.Equal(t, 1, write.bindings, "bound before message_stop is written")
					sawStop = true
					continue
				}
				if strings.Contains(write.data, "message_start") {
					sawStart = true
				}
				require.False(t, sawStop)
				require.Zero(t, write.bindings, "not bound before the response is complete: %q", write.data)
			}
			require.True(t, sawStart && sawStop)

			// A client that continues as soon as it knows the id, before the
			// response is complete, is answered locally: the id is not a thread
			// yet, for the gateway as for the upstream.
			early := 0
			writer = &threadWriter{}
			writer.onWrite = func(data string) {
				for _, line := range strings.Split(data, "\n") {
					payload, ok := strings.CutPrefix(line, "data: ")
					if !ok || gjson.Get(payload, "type").Str != "message_start" {
						continue
					}
					early++
					h.requireNotFound(threadTurn{previous: gjson.Get(payload, "message.id").Str})
				}
			}
			first = threadResponseID(t, h.send(threadTurn{previous: first, stream: true, writer: writer}))
			require.Equal(t, 1, early)

			writer = &threadWriter{}
			second := h.send(threadTurn{previous: first, writer: writer})
			threadResponseID(t, second)
			require.Len(t, writer.writes, 1)
			require.Equal(t, 3, writer.writes[0].bindings, "a non-streaming response is bound before it is written")
		})
	}
}

// Responses the upstream does not keep, or that the client must not continue,
// are delivered and billed but leave no binding.
func TestMessageThreadUnbindableResponses(t *testing.T) {
	for _, kind := range []string{"apikey", "oauth"} {
		for _, mode := range []string{"refusal", "truncated", "fallback", "cut"} {
			for _, stream := range []bool{false, true} {
				if mode == "cut" && !stream {
					continue
				}
				t.Run(fmt.Sprintf("%s/%s/stream=%t", kind, mode, stream), func(t *testing.T) {
					h := newThreadHarness(t, kind)
					first := threadResponseID(t, h.send(threadTurn{}))
					h.requireCommands(1)
					h.a.setMode(mode)
					rec := h.send(threadTurn{previous: first, stream: stream})
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					if mode == "cut" {
						require.NotContains(t, rec.Body.String(), "message_stop")
					}
					commands := h.requireCommands(2)
					require.Equal(t, 10000, commands[1].InputTokens, "an interrupted response is still billed for what the upstream metered")
					require.Equal(t, 1, threadBindingCount(h.db))
					h.a.setMode("")
					h.requireNotFound(threadTurn{previous: "msg_a_2"})
				})
			}
		}
	}
}

// A paused turn is resumed with an empty message list.
func TestMessageThreadResumesPausedTurn(t *testing.T) {
	for _, kind := range []string{"passthrough", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			h.a.setMode("pause")
			first := threadResponseID(t, h.send(threadTurn{stream: true}))
			h.a.setMode("")
			threadResponseID(t, h.send(threadTurn{previous: first, messages: `[]`}))
			require.Equal(t, 2, h.a.count())
			require.Equal(t, `[]`, gjson.GetBytes(h.a.last().body, "messages").Raw)
			require.False(t, gjson.GetBytes(h.a.last().body, "tools").Exists())
		})
	}
}

// When the account that holds the thread cannot be used, the new messages are
// not sent anywhere: the client is told to replay, and a replay may then be
// served by another account.
func TestMessageThreadBoundAccountUnavailable(t *testing.T) {
	for _, kind := range []string{"apikey", "passthrough", "oauth-cli", "oauth", "legacy-scheduler/apikey", "legacy-scheduler/oauth"} {
		t.Run(kind, func(t *testing.T) {
			kind, legacy := strings.CutPrefix(kind, "legacy-scheduler/")
			h := newThreadHarnessFor(t, kind, !legacy)
			first := threadResponseID(t, h.send(threadTurn{}))
			h.accountB.Priority = -100 // B is now the scheduler's first choice
			apiKey := kind == "apikey" || kind == "passthrough"
			later := time.Now().Add(time.Hour)
			copyRow := func(mutate func(*service.Account)) *service.Account {
				row := *h.accountA
				row.Credentials, row.Extra = map[string]any{}, map[string]any{}
				for k, v := range h.accountA.Credentials {
					row.Credentials[k] = v
				}
				for k, v := range h.accountA.Extra {
					row.Extra[k] = v
				}
				mutate(&row)
				return &row
			}

			cases := map[string]func() func(){
				"rate limited": func() func() {
					h.accountA.RateLimitResetAt = &later
					return func() { h.accountA.RateLimitResetAt = nil }
				},
				"overloaded": func() func() {
					h.accountA.OverloadUntil = &later
					return func() { h.accountA.OverloadUntil = nil }
				},
				"unschedulable": func() func() {
					h.accountA.Schedulable = false
					return func() { h.accountA.Schedulable = true }
				},
				"disabled": func() func() {
					h.accountA.Status = service.StatusDisabled
					return func() { h.accountA.Status = service.StatusActive }
				},
				"removed from the group": func() func() {
					groups := h.accountA.AccountGroups
					h.accountA.AccountGroups = []service.AccountGroup{{AccountID: 71, GroupID: 7}}
					return func() { h.accountA.AccountGroups = groups }
				},
				// The scheduler snapshot still lists the account; only the
				// authoritative row has changed.
				"deleted after scheduling": func() func() {
					h.repo.set(71, nil)
					return func() { h.repo.set(71, h.accountA) }
				},
				"moved after scheduling": func() func() {
					h.repo.set(71, copyRow(func(a *service.Account) { a.AccountGroups = []service.AccountGroup{{AccountID: 71, GroupID: 7}} }))
					return func() { h.repo.set(71, h.accountA) }
				},
				"rate limited after scheduling": func() func() {
					h.repo.set(71, copyRow(func(a *service.Account) { a.RateLimitResetAt = &later }))
					return func() { h.repo.set(71, h.accountA) }
				},
				// The row now holds another credential than the one in hand.
				"credential replaced after scheduling": func() func() {
					h.repo.set(71, copyRow(func(a *service.Account) {
						if apiKey {
							a.Credentials["api_key"] = "another-key"
						} else {
							a.Credentials["access_token"] = "another-token"
						}
					}))
					return func() { h.repo.set(71, h.accountA) }
				},
			}
			if apiKey {
				cases["key rotated"] = func() func() {
					h.accountA.Credentials["api_key"] = "rotated"
					return func() { h.accountA.Credentials["api_key"] = "secret-a" }
				}
				cases["origin changed"] = func() func() {
					h.accountA.Credentials["base_url"] = "https://relay.example.com"
					h.accountA.Extra[service.MessageThreadAccountFlag] = true
					return func() {
						delete(h.accountA.Credentials, "base_url")
						delete(h.accountA.Extra, service.MessageThreadAccountFlag)
					}
				}
				cases["quota exhausted"] = func() func() {
					h.accountA.Extra["quota_limit"], h.accountA.Extra["quota_used"] = 1.0, 1.0
					return func() { delete(h.accountA.Extra, "quota_limit"); delete(h.accountA.Extra, "quota_used") }
				}
			} else {
				// Same row, same recorded account and organization, another token:
				// nothing but the credential shows that it was authorized again.
				cases["authorized again"] = func() func() {
					h.accountA.Credentials["access_token"] = "token-of-another-login"
					return func() { h.accountA.Credentials["access_token"] = "secret-a" }
				}
				cases["routed through a relay"] = func() func() {
					h.accountA.Extra["custom_base_url_enabled"], h.accountA.Extra["custom_base_url"] = true, "https://relay.example.com"
					h.accountA.Extra[service.MessageThreadAccountFlag] = true
					return func() {
						delete(h.accountA.Extra, "custom_base_url_enabled")
						delete(h.accountA.Extra, "custom_base_url")
						delete(h.accountA.Extra, service.MessageThreadAccountFlag)
					}
				}
			}
			for name, apply := range cases {
				restore := apply()
				h.requireNotFound(threadTurn{previous: first, stream: true})
				restore()
				if t.Failed() {
					t.Fatalf("case %q", name)
				}
			}
			require.Zero(t, h.b.count())

			// Metadata that is not the credential or the origin changes nothing.
			h.accountA.Extra["session_window_utilization"] = 0.5
			if !apiKey {
				h.accountA.Extra["account_uuid"], h.accountA.Extra["org_uuid"] = "re-synced", "re-synced"
				h.accountA.Credentials["refresh_token"] = "rotated-refresh-token"
			}
			second := threadResponseID(t, h.send(threadTurn{previous: first}))
			require.Equal(t, 2, h.a.count())

			// The replay is an ordinary request and may use the better account.
			h.accountA.RateLimitResetAt = &later
			h.requireNotFound(threadTurn{previous: second})
			replayed := threadResponseID(t, h.send(threadTurn{}))
			require.Equal(t, 1, h.b.count())
			threadResponseID(t, h.send(threadTurn{previous: replayed}))
			require.Equal(t, 2, h.b.count())
			require.Equal(t, 2, h.a.count())
			require.Zero(t, h.router.stray.Load())
		})
	}
}

// Thread state that cannot be read is a failure, not a miss, and an expired
// binding is a miss.
func TestMessageThreadStorageFailureAndExpiry(t *testing.T) {
	for _, kind := range []string{"apikey", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			first := threadResponseID(t, h.send(threadTurn{}))

			h.db.SetError("ERR storage unavailable")
			for _, previous := range []string{first, "msg_unknown"} {
				rec := h.send(threadTurn{previous: previous, stream: true})
				require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
				require.Equal(t, "api_error", gjson.Get(rec.Body.String(), "error.type").Str)
				require.False(t, gjson.Get(rec.Body.String(), "error.details").Exists(), "a storage failure is not reported as a missing thread")
			}
			require.Equal(t, 1, h.a.count())

			// A create request still works while the store is down. Its response
			// cannot be bound, so it is simply not continuable.
			unbound := threadResponseID(t, h.send(threadTurn{stream: true}))
			require.Equal(t, 2, h.a.count())
			h.requireCommands(2)
			h.db.SetError("")
			h.requireNotFound(threadTurn{previous: unbound})

			// The first binding survived the outage and expires with the thread.
			h.db.FastForward(service.MessageThreadTTL - time.Second)
			second := threadResponseID(t, h.send(threadTurn{previous: first}))
			h.db.FastForward(service.MessageThreadTTL - time.Second)
			h.requireNotFound(threadTurn{previous: first})
			third := threadResponseID(t, h.send(threadTurn{previous: second}))
			h.db.FastForward(service.MessageThreadTTL)
			h.requireNotFound(threadTurn{previous: third})
			require.Zero(t, h.b.count())
		})
	}
}

// Upstream answers about the thread are relayed byte for byte and never held
// against the account; other upstream errors keep their normal handling but a
// continue request is not moved to another account.
func TestMessageThreadUpstreamErrors(t *testing.T) {
	for _, kind := range []string{"apikey", "passthrough", "oauth-cli", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			first := threadResponseID(t, h.send(threadTurn{}))
			h.requireCommands(1)
			h.accountB.Priority = -100

			h.a.setMode("thread404")
			for _, stream := range []bool{true, false} {
				rec := h.send(threadTurn{previous: first, stream: stream})
				require.Equal(t, http.StatusNotFound, rec.Code)
				require.Equal(t, threadTestNotFound, rec.Body.String())
				require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
			}
			require.Equal(t, 3, h.a.count(), "one upstream request per attempt, no retry")
			require.True(t, h.accountA.IsSchedulable(), "a thread error says nothing about the account")
			limits := func() int {
				h.repo.mu.Lock()
				defer h.repo.mu.Unlock()
				return len(h.repo.modelLimits)
			}
			require.Zero(t, limits(), "and nothing about its models")

			// A missing model is also a 404, and must not look like a lost thread.
			h.a.setMode("model404")
			rec := h.send(threadTurn{previous: first, stream: true})
			require.NotEqual(t, http.StatusOK, rec.Code)
			require.NotContains(t, rec.Body.String(), "thread_not_found")
			require.Equal(t, "error", gjson.Get(rec.Body.String(), "type").Str)
			require.Equal(t, 1, limits(), "an unknown model keeps its ordinary handling")
			h.repo.mu.Lock()
			h.repo.modelLimits = nil
			h.repo.mu.Unlock()

			// The account is rate limited mid-thread. The client gets that
			// answer; the next attempt is told to replay.
			h.a.setMode("ratelimit")
			before := h.a.count()
			rec = h.send(threadTurn{previous: first, stream: true})
			require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
			require.Equal(t, before+1, h.a.count())
			h.a.setMode("")
			h.requireNotFound(threadTurn{previous: first})
			h.accountA.RateLimitResetAt = nil

			require.Zero(t, h.b.count(), "no continue request was retried on another account")
			h.requireCommands(1)

			// After all of that the thread itself is intact.
			threadResponseID(t, h.send(threadTurn{previous: first}))
			require.Zero(t, h.b.count())
		})
	}
}

// A create request is an ordinary full request and keeps ordinary failover.
func TestMessageThreadCreateKeepsFailover(t *testing.T) {
	for _, kind := range []string{"apikey", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			h.a.setMode("overloaded")
			first := threadResponseID(t, h.send(threadTurn{stream: true}))
			require.Positive(t, h.a.count())
			require.Equal(t, 1, h.b.count())
			require.True(t, strings.HasPrefix(first, "msg_b_"))
			h.a.setMode("")
			h.accountA.OverloadUntil = nil
			second := threadResponseID(t, h.send(threadTurn{previous: first}))
			require.True(t, strings.HasPrefix(second, "msg_b_"), "the thread stays with the account that created it")
			require.Equal(t, 2, h.b.count())
		})
	}
}

// Fixed fields: the gateway neither changes them behind the client's back nor
// hides a change the client made.
func TestMessageThreadFixedFields(t *testing.T) {
	for _, kind := range []string{"apikey", "passthrough", "oauth-cli", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			first := threadResponseID(t, h.send(threadTurn{}))
			calls := h.a.count()
			mismatch := func(turn threadTurn, field string) {
				t.Helper()
				rec := h.send(turn)
				require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
				require.Equal(t, "thread_fingerprint_mismatch", gjson.Get(rec.Body.String(), "error.details.error_code").Str)
				require.Contains(t, gjson.Get(rec.Body.String(), "error.message").Str, field)
				require.Equal(t, calls, h.a.count())
			}
			changed := threadTestBeta + ",another-beta-2026-02-02"
			if h.claudeCode() {
				changed = "claude-code-20250219,oauth-2025-04-20," + threadTestBeta
			}
			mismatch(threadTurn{previous: first, beta: &changed}, "anthropic-beta")
			mismatch(threadTurn{previous: first, model: "claude-opus-4-8"}, "model")

			for _, extra := range []string{`"system":"new"`, `"system":null`, `"tools":[]`, `"tools":null`} {
				rec := h.send(threadTurn{previous: first, extra: extra})
				require.Equal(t, http.StatusBadRequest, rec.Code, extra)
				require.False(t, gjson.Get(rec.Body.String(), "error.details").Exists())
			}
			for _, body := range []string{`"thread":"continue"`, `"thread":{"type":"fork"}`, `"thread":{"type":"create","previous_message_id":"x"}`, `"thread":{"type":"continue"}`} {
				rec := h.send(threadTurn{noThread: true, extra: body})
				require.Equal(t, http.StatusBadRequest, rec.Code, body)
			}
			// Two thread fields would let two parsers route differently.
			rec := h.send(threadTurn{extra: fmt.Sprintf(`"thread":{"type":"continue","previous_message_id":%q}`, first)})
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, calls, h.a.count())
			require.Zero(t, h.b.count())

			// The untouched thread still continues.
			threadResponseID(t, h.send(threadTurn{previous: first}))
			require.Equal(t, calls+1, h.a.count())
		})
	}
}

// A thread needs its beta on every request; the gateway does not invent it.
func TestMessageThreadRequiresBeta(t *testing.T) {
	for _, kind := range []string{"apikey", "passthrough", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			none := "custom-beta-2026-01-01"
			rec := h.send(threadTurn{beta: &none})
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), threadTestBeta)
			require.Zero(t, h.a.count()+h.b.count())
			require.Zero(t, threadBindingCount(h.db))
		})
	}
}

// The balance reservation of a continue request covers the conversation the
// upstream holds, and nothing stays reserved afterwards.
func TestMessageThreadReservationCoversStoredContext(t *testing.T) {
	h := newThreadHarness(t, "apikey")
	first := threadResponseID(t, h.send(threadTurn{}))
	h.requireCommands(1)
	reserved := func() int {
		h.funds.mu.Lock()
		defer h.funds.mu.Unlock()
		return len(h.funds.res)
	}
	require.Eventually(t, func() bool { return reserved() == 0 }, 2*time.Second, 5*time.Millisecond)

	// The few new bytes would fit this balance; the stored 91k tokens do not.
	h.funds.mu.Lock()
	h.funds.balance = 0.01
	h.funds.res["another-request"] = 0.001
	h.funds.mu.Unlock()
	rec := h.send(threadTurn{previous: first, stream: true})
	require.NotEqual(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, 1, h.a.count())
	// The same small request without a thread is admitted.
	plain := h.send(threadTurn{noThread: true, omitDefs: true})
	require.Equal(t, http.StatusOK, plain.Code, plain.Body.String())
	require.Equal(t, 2, h.a.count())

	h.funds.mu.Lock()
	h.funds.balance = 100
	delete(h.funds.res, "another-request")
	h.funds.mu.Unlock()
	threadResponseID(t, h.send(threadTurn{previous: first, stream: true}))
	commands := h.requireCommands(3)
	require.Equal(t, 80000, commands[2].CacheReadTokens)
	require.Eventually(t, func() bool { return reserved() == 0 }, 2*time.Second, 5*time.Millisecond, "no reservation is left behind")
}

// While a thread request waits for an account slot the response is not
// started, so the upstream's status and body still reach the client.
func TestMessageThreadQueueDoesNotCommitResponse(t *testing.T) {
	for _, kind := range []string{"apikey", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			first := threadResponseID(t, h.send(threadTurn{}))

			// Control: an ordinary queued stream is kept alive with pings.
			h.slots.refuse.Store(4)
			plain := h.send(threadTurn{noThread: true, stream: true})
			require.Equal(t, http.StatusOK, plain.Code)
			require.Contains(t, plain.Body.String(), `"type": "ping"`)

			h.slots.refuse.Store(4)
			h.a.setMode("thread404")
			rec := h.send(threadTurn{previous: first, stream: true})
			require.Equal(t, http.StatusNotFound, rec.Code)
			require.Equal(t, threadTestNotFound, rec.Body.String())

			h.slots.refuse.Store(4)
			h.a.setMode("")
			queued := h.send(threadTurn{previous: first, stream: true})
			threadResponseID(t, queued)
			require.NotContains(t, queued.Body.String(), `"type": "ping"`)
			require.Zero(t, h.b.count())
		})
	}
}

// Requests without a thread behave exactly as before on the same accounts.
func TestMessageThreadLeavesOrdinaryRequestsAlone(t *testing.T) {
	for _, kind := range threadTestKinds() {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", kind, stream), func(t *testing.T) {
				h := newThreadHarness(t, kind)
				plainBeta := "custom-beta-2026-01-01"
				if h.claudeCode() {
					plainBeta = "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14"
				}
				rec := h.send(threadTurn{noThread: true, stream: stream, omitDefs: true, beta: &plainBeta})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				sent := h.a.last()
				require.False(t, gjson.GetBytes(sent.body, "thread").Exists())
				require.NotContains(t, threadTestCanonicalBeta(sent.header), threadTestBeta)
				if kind == "oauth" || kind == "setup-token" {
					require.Equal(t, `[]`, gjson.GetBytes(sent.body, "tools").Raw, "the existing rewrite of ordinary requests is unchanged")
				} else {
					require.False(t, gjson.GetBytes(sent.body, "tools").Exists())
				}
				for _, key := range h.db.Keys() {
					require.NotContains(t, key, "thread", "an ordinary request stores no thread state")
				}
				h.requireCommands(1)

				// Ordinary requests still fail over between accounts.
				h.a.setMode("overloaded")
				rec = h.send(threadTurn{noThread: true, stream: stream, omitDefs: true, beta: &plainBeta})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.Equal(t, 1, h.b.count())
			})
		}
	}
}

// A group restricted to Claude Code keeps its restriction for the whole
// thread: the continue request cannot repeat the validated system prompt, so
// that one check is inherited, and nothing else is.
func TestMessageThreadClaudeCodeOnlyGroup(t *testing.T) {
	h := newThreadHarness(t, "oauth-cli")
	h.group.ClaudeCodeOnly = true

	// Not a Claude Code request: refused as before, with or without a thread.
	for _, noThread := range []bool{true, false} {
		rec := h.send(threadTurn{plain: true, noThread: noThread})
		require.NotEqual(t, http.StatusOK, rec.Code)
	}
	require.Zero(t, h.a.count()+h.b.count())

	first := threadResponseID(t, h.send(threadTurn{}))
	stored, err := h.db.Get(threadBindingKeys(h.db)[0])
	require.NoError(t, err)
	require.True(t, gjson.Get(stored, "cc").Bool(), "the create request was validated as Claude Code")

	// The same thread continued by something that is not the Claude Code
	// client is not served, and its messages reach no upstream.
	h.requireNotFound(threadTurn{previous: first, plain: true})

	second := threadResponseID(t, h.send(threadTurn{previous: first, stream: true}))
	require.False(t, gjson.GetBytes(h.a.last().body, "system").Exists())
	threadResponseID(t, h.send(threadTurn{previous: second}))
	require.Equal(t, 3, h.a.count())
	require.Zero(t, h.b.count())
}

// Only an upstream known to keep threads is used for one. A custom origin or a
// pooled upstream needs the operator's explicit declaration.
func TestMessageThreadAccountCapability(t *testing.T) {
	relay := func(a *service.Account) {
		if a.Type == service.AccountTypeAPIKey {
			a.Credentials["base_url"] = "https://relay.example.com"
		} else {
			a.Extra["custom_base_url_enabled"], a.Extra["custom_base_url"] = true, "https://relay.example.com"
		}
	}
	for _, kind := range []string{"apikey", "passthrough", "oauth"} {
		for _, loadBatch := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/load_batch=%t", kind, loadBatch), func(t *testing.T) {
				h := newThreadHarnessFor(t, kind, loadBatch)
				relay(h.accountA) // the preferred account is behind a relay

				first := threadResponseID(t, h.send(threadTurn{stream: true}))
				require.Zero(t, h.a.count(), "an undeclared relay never receives a thread")
				require.Equal(t, 1, h.b.count())
				threadResponseID(t, h.send(threadTurn{previous: first}))
				require.Equal(t, 2, h.b.count())

				// Ordinary requests still use the relay account.
				require.Equal(t, http.StatusOK, h.send(threadTurn{noThread: true, omitDefs: true}).Code)
				require.Equal(t, 1, h.a.count())

				// No capable account at all: an explicit, actionable refusal.
				relay(h.accountB)
				before := h.a.count() + h.b.count()
				rec := h.send(threadTurn{})
				require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
				require.Equal(t, "thread_unsupported_request", gjson.Get(rec.Body.String(), "error.details.error_code").Str)
				require.Equal(t, before, h.a.count()+h.b.count())
				// The thread created on B is no longer continuable there either.
				h.requireNotFound(threadTurn{previous: "msg_b_2"})

				// Declared by the operator: the relay is used.
				h.accountA.Extra[service.MessageThreadAccountFlag] = true
				declared := threadResponseID(t, h.send(threadTurn{}))
				require.True(t, strings.HasPrefix(declared, "msg_a_"))
				host, _ := h.router.hosts.Load(int64(71))
				require.Equal(t, "relay.example.com", host)
				threadResponseID(t, h.send(threadTurn{previous: declared, stream: true}))
				require.Zero(t, h.router.stray.Load())
			})
		}
	}

	t.Run("pool mode", func(t *testing.T) {
		h := newThreadHarness(t, "apikey")
		h.accountA.Credentials["pool_mode"] = true
		first := threadResponseID(t, h.send(threadTurn{}))
		require.True(t, strings.HasPrefix(first, "msg_b_"))
		require.Zero(t, h.a.count())
	})

	t.Run("other account kinds", func(t *testing.T) {
		for _, change := range []func(*service.Account){
			func(a *service.Account) { a.Type = service.AccountTypeBedrock },
			func(a *service.Account) { a.Type = service.AccountTypeServiceAccount },
			func(a *service.Account) { a.Type = service.AccountTypeUpstream },
		} {
			h := newThreadHarness(t, "apikey")
			change(h.accountA)
			first := threadResponseID(t, h.send(threadTurn{}))
			require.True(t, strings.HasPrefix(first, "msg_b_"))
			require.Zero(t, h.a.count())
		}
	})
}

// Routes that cannot carry a thread say so instead of answering without the
// conversation.
func TestMessageThreadUnsupportedRoutes(t *testing.T) {
	t.Run("converted platform", func(t *testing.T) {
		for _, platform := range []string{service.PlatformGemini, service.PlatformAntigravity} {
			h := newThreadHarness(t, "apikey")
			first := threadResponseID(t, h.send(threadTurn{}))
			h.group.Platform = platform
			for _, turn := range []threadTurn{{}, {previous: first}, {previous: "msg_unknown", stream: true}} {
				rec := h.send(turn)
				require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
				require.Equal(t, "thread_unsupported_request", gjson.Get(rec.Body.String(), "error.details.error_code").Str)
			}
			require.Equal(t, 1, h.a.count())
			require.Zero(t, h.b.count())
		}
	})

	threadBodies := []string{
		`{"model":"claude-sonnet-4-5","max_tokens":8,"messages":[{"role":"user","content":"hi"}],"thread":{"type":"create"}}`,
		`{"model":"claude-sonnet-4-5","max_tokens":8,"messages":[{"role":"user","content":"hi"}],"thread":{"type":"continue","previous_message_id":"msg_1"}}`,
	}
	newContext := func(path, body, platform string) (*gin.Context, *httptest.ResponseRecorder) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		c.Request.Header.Set("Anthropic-Beta", threadTestBeta)
		groupID := int64(941)
		group := &service.Group{ID: groupID, Hydrated: true, Platform: platform, Status: service.StatusActive, RateMultiplier: 1}
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 47, UserID: 52, Group: group, GroupID: &groupID, User: &service.User{ID: 52, Balance: 100}})
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 52})
		return c, rec
	}

	t.Run("token counting", func(t *testing.T) {
		h := newThreadHarness(t, "apikey")
		openAI := &OpenAIGatewayHandler{cfg: &config.Config{}}
		for _, body := range threadBodies {
			for name, call := range map[string]func(*gin.Context){
				"anthropic": h.handler.CountTokens,
				"grok":      openAI.GrokCountTokens,
			} {
				c, rec := newContext("/v1/messages/count_tokens", body, service.PlatformAnthropic)
				call(c)
				require.Equal(t, http.StatusBadRequest, rec.Code, name+": "+rec.Body.String())
				require.Equal(t, "invalid_request_error", gjson.Get(rec.Body.String(), "error.type").Str)
				require.Contains(t, rec.Body.String(), "token counting")
			}
		}
		require.Zero(t, h.a.count()+h.b.count())
	})

	// The OpenAI-side dispatcher either converts the request or reaches a
	// third-party Messages-compatible endpoint; it has no thread binding.
	t.Run("openai-side dispatcher", func(t *testing.T) {
		upstream := &nativeEndpointUpstream{}
		cfg := &config.Config{}
		gateway := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil)
		billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
		t.Cleanup(billing.Stop)
		openAI := &OpenAIGatewayHandler{
			gatewayService: gateway, billingCacheService: billing, apiKeyService: &service.APIKeyService{}, cfg: cfg,
			concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(&fakeConcurrencyCache{}), SSEPingFormatClaude, time.Millisecond),
		}
		for _, platform := range []string{service.PlatformKimi, service.PlatformGrok} {
			for _, body := range threadBodies {
				c, rec := newContext("/v1/messages", body, platform)
				openAI.Messages(c)
				require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
				require.Equal(t, "thread_unsupported_request", gjson.Get(rec.Body.String(), "error.details.error_code").Str)

				c, rec = newContext("/v1/messages/count_tokens", body, platform)
				openAI.CountTokens(c)
				require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
				require.Contains(t, rec.Body.String(), "token counting")
			}
		}
		require.Empty(t, upstream.paths, "nothing was sent upstream")
	})
}

// On the path that rewrites requests for a subscription account, the rewrite
// of a continue request has to agree with what was sent on the create request.
func TestMessageThreadRewrittenContinueRequest(t *testing.T) {
	for _, kind := range []string{"oauth", "setup-token"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			h.fixed = `"cache_control":{"type":"ephemeral"},"tool_choice":{"type":"auto"}`
			first := threadResponseID(t, h.send(threadTurn{}))
			create := h.a.last()
			reserved := strings.Count(gjson.GetBytes(create.body, "system").Raw, `"cache_control"`) +
				strings.Count(gjson.GetBytes(create.body, "tools").Raw, `"cache_control"`)
			require.Positive(t, reserved, "the create request carries the gateway's own breakpoints")
			stored, err := h.db.Get(threadBindingKeys(h.db)[0])
			require.NoError(t, err)
			require.EqualValues(t, reserved, gjson.Get(stored, "bp").Int())
			require.True(t, gjson.Get(stored, "ht").Bool())
			require.True(t, gjson.Get(stored, "mi").Bool())

			// Four client breakpoints, the top-level one and those the thread
			// already holds are more than the upstream allows; the fake upstream
			// counts them the same way and would refuse the request.
			blocks := make([]string, 4)
			for i := range blocks {
				blocks[i] = fmt.Sprintf(`{"type":"text","text":"part %d","cache_control":{"type":"ephemeral"}}`, i)
			}
			second := h.send(threadTurn{
				previous: first, stream: true,
				messages: `[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"done"},` + strings.Join(blocks, ",") + `]}]`,
			})
			secondID := threadResponseID(t, second)
			sent := h.a.last().body
			require.True(t, gjson.GetBytes(sent, "cache_control").Exists(), "the client's top-level cache_control is a fixed field and is kept")
			require.Equal(t, 4-1-reserved, strings.Count(gjson.GetBytes(sent, "messages").Raw, `"cache_control"`))
			require.Equal(t, "auto", gjson.GetBytes(sent, "tool_choice.type").Str, "tool_choice is kept: the thread has tools")
			require.False(t, gjson.GetBytes(sent, "tools").Exists())
			require.False(t, gjson.GetBytes(sent, "system").Exists())
			require.Equal(t, gjson.GetBytes(create.body, "model").Str, gjson.GetBytes(sent, "model").Str)

			third := h.send(threadTurn{previous: secondID})
			threadResponseID(t, third)
			require.Contains(t, third.Body.String(), `"name":"lookup"`, "the client tool name is unchanged")
			require.Equal(t, 3, h.a.count())
			require.Zero(t, h.b.count())
		})
	}

	// A thread created without tools: the create request lost its tool_choice,
	// so the continue request loses it the same way.
	t.Run("no tools", func(t *testing.T) {
		h := newThreadHarness(t, "oauth")
		h.fixed = `"tool_choice":{"type":"auto"}`
		first := threadResponseID(t, h.send(threadTurn{omitDefs: true}))
		require.False(t, gjson.GetBytes(h.a.last().body, "tool_choice").Exists())
		require.Equal(t, `[]`, gjson.GetBytes(h.a.last().body, "tools").Raw)
		threadResponseID(t, h.send(threadTurn{previous: first, messages: `[{"role":"user","content":"next"}]`}))
		require.False(t, gjson.GetBytes(h.a.last().body, "tool_choice").Exists())
		require.False(t, gjson.GetBytes(h.a.last().body, "tools").Exists())
	})
}

// A client that disconnects before the end of a stream cannot have seen
// message_stop: the response is billed in full and not bound.
func TestMessageThreadClientDisconnect(t *testing.T) {
	for _, kind := range []string{"apikey", "passthrough", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			first := threadResponseID(t, h.send(threadTurn{}))
			h.requireCommands(1)
			rec := h.send(threadTurn{previous: first, stream: true, failAt: 2})
			require.NotContains(t, rec.Body.String(), "message_stop")
			commands := h.requireCommands(2)
			require.Equal(t, 20, commands[1].OutputTokens, "the upstream was read to the end for its usage")
			require.Equal(t, 1, threadBindingCount(h.db))
			h.requireNotFound(threadTurn{previous: "msg_a_2"})
		})
	}
}

// The current beta policy is never bypassed for a thread.
func TestMessageThreadBetaPolicy(t *testing.T) {
	for _, kind := range []string{"apikey", "passthrough", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			first := threadResponseID(t, h.send(threadTurn{}))
			calls := h.a.count()

			// The thread beta is filtered now. It is not put back; the client
			// is told to replay, and a replay is refused explicitly.
			h.settings.policy = `{"rules":[{"beta_token":"` + threadTestBeta + `","action":"filter","scope":"all"}]}`
			h.requireNotFound(threadTurn{previous: first})
			rec := h.send(threadTurn{})
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Equal(t, "thread_unsupported_request", gjson.Get(rec.Body.String(), "error.details.error_code").Str)

			// A block rule answers as it does for any request.
			h.settings.policy = `{"rules":[{"beta_token":"` + threadTestBeta + `","action":"block","scope":"all","error_message":"threads are disabled here"}]}`
			for _, turn := range []threadTurn{{}, {previous: first}} {
				rec := h.send(turn)
				require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
				require.Contains(t, rec.Body.String(), "threads are disabled here")
			}
			require.Equal(t, calls, h.a.count())
			require.Zero(t, h.b.count())

			// With the policy lifted the thread is as it was.
			h.settings.policy = `{"rules":[]}`
			threadResponseID(t, h.send(threadTurn{previous: first}))
			require.Equal(t, calls+1, h.a.count())
		})
	}
}

// When the gateway had to rewrite a create request to get it accepted (here:
// dropping a thinking block with an invalid signature), the upstream stored a
// thread whose fixed fields are no longer the client's. The answer is
// delivered; the thread is not offered for continuation.
func TestMessageThreadRectifiedCreateIsNotBound(t *testing.T) {
	h := newThreadHarness(t, "oauth")
	h.a.setMode("signature-once")
	rec := h.send(threadTurn{
		messages: `[{"role":"user","content":"hi"},{"role":"assistant","content":[{"type":"thinking","thinking":"hm","signature":"bad"},{"type":"text","text":"hello"}]},{"role":"user","content":"again"}]`,
		extra:    `"thinking":{"type":"enabled","budget_tokens":1024}`,
	})
	id := threadResponseID(t, rec)
	require.Equal(t, 2, h.a.count(), "rejected once, accepted after the rewrite")
	require.NotEqual(t, "enabled", gjson.GetBytes(h.a.last().body, "thinking.type").Str, "the accepted request differs in a fixed field")
	require.Zero(t, threadBindingCount(h.db))
	h.requireNotFound(threadTurn{previous: id})
	h.requireCommands(1)

	// The same request without that history is bound as usual.
	second := threadResponseID(t, h.send(threadTurn{}))
	require.Equal(t, 1, threadBindingCount(h.db))
	threadResponseID(t, h.send(threadTurn{previous: second}))
}

// A continue request is sent upstream once. Where an ordinary request is
// retried on the same account, the upstream may already have taken the turn,
// so the gateway does not repeat it; the client replays what it owns.
func TestMessageThreadContinueIsSentOnce(t *testing.T) {
	for _, kind := range []string{"apikey", "passthrough", "oauth-cli", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			h := newThreadHarness(t, kind)
			mode := "forbidden"
			if kind == "apikey" || kind == "passthrough" {
				// This account retries every status it was not told to handle.
				mode = "server-error"
				h.accountA.Credentials["custom_error_codes_enabled"] = true
				h.accountA.Credentials["custom_error_codes"] = []any{float64(429)}
			}
			first := threadResponseID(t, h.send(threadTurn{}))
			h.accountB.Priority = -100

			h.a.setMode(mode)
			before := h.a.count()
			rec := h.send(threadTurn{previous: first, stream: true})
			require.GreaterOrEqual(t, rec.Code, 400, rec.Body.String())
			require.NotContains(t, rec.Body.String(), "thread_not_found")
			require.Equal(t, before+1, h.a.count(), "exactly one upstream request")
			require.Zero(t, h.b.count())

			// Control: the same failure on an ordinary request is retried.
			h.repo.set(72, nil)
			h.sched.accounts = []*service.Account{h.accountA}
			before = h.a.count()
			rec = h.send(threadTurn{noThread: true, omitDefs: true})
			require.GreaterOrEqual(t, rec.Code, 400)
			require.Greater(t, h.a.count(), before+1, "ordinary requests keep their retries")
			h.requireCommands(1)
		})
	}
}
