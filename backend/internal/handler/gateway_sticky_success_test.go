//go:build unit

package handler

// 利润控制下按成功结果维护可迁移会话粘性的 handler 级回归。
//
// 全部走真实 GatewayHandler + 真实选号/failover 循环，出站在 HTTPUpstream 边界
// 被脚本化拦截；账号与会话均为合成数据。

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 假上游：按账号给出可编程的响应
// ---------------------------------------------------------------------------

type stickyScript struct {
	statusCode int
	// streamBody 非空时返回 SSE；否则返回 JSON body。
	streamBody string
	jsonBody   string
	// streamHead / streamTail / onHeadWritten 用于「上游已交出用量之后客户端才断开」：
	// 写完 head（含带 usage 的 message_delta）后回调，再写 tail 让上游流自然结束。
	streamHead    string
	streamTail    string
	onHeadWritten func()
}

type stickyUpstream struct {
	service.HTTPUpstream
	mu      sync.Mutex
	hits    []int64
	scripts map[int64][]stickyScript
}

func (u *stickyUpstream) respond(accountID int64) (*http.Response, error) {
	u.mu.Lock()
	u.hits = append(u.hits, accountID)
	scripts := u.scripts[accountID]
	var script stickyScript
	switch {
	case len(scripts) == 0:
		script = stickyScript{statusCode: http.StatusInternalServerError, jsonBody: `{"error":{"message":"no script"}}`}
	case len(scripts) == 1:
		script = scripts[0]
	default:
		script = scripts[0]
		u.scripts[accountID] = scripts[1:]
	}
	u.mu.Unlock()

	header := http.Header{"X-Request-Id": []string{"sticky-upstream-req-id"}}
	if script.streamHead != "" {
		header.Set("Content-Type", "text/event-stream")
		reader, writer := io.Pipe()
		go func() {
			defer func() { _ = writer.Close() }()
			_, _ = io.WriteString(writer, script.streamHead)
			if script.onHeadWritten != nil {
				// 让网关先读完 head 再断开，模拟真实的「已计量后断连」。
				time.Sleep(80 * time.Millisecond)
				script.onHeadWritten()
				time.Sleep(40 * time.Millisecond)
			}
			_, _ = io.WriteString(writer, script.streamTail)
		}()
		return &http.Response{StatusCode: script.statusCode, Header: header, Body: reader}, nil
	}
	if script.streamBody != "" {
		header.Set("Content-Type", "text/event-stream")
		reader, writer := io.Pipe()
		go func() {
			defer func() { _ = writer.Close() }()
			_, _ = io.WriteString(writer, script.streamBody)
		}()
		return &http.Response{StatusCode: script.statusCode, Header: header, Body: reader}, nil
	}
	header.Set("Content-Type", "application/json")
	return &http.Response{
		StatusCode: script.statusCode,
		Header:     header,
		Body:       io.NopCloser(bytes.NewBufferString(script.jsonBody)),
	}, nil
}

func (u *stickyUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	if req != nil && req.Body != nil {
		_, _ = io.ReadAll(req.Body)
	}
	return u.respond(accountID)
}

func (u *stickyUpstream) DoWithTLS(req *http.Request, _ string, accountID int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	if req != nil && req.Body != nil {
		_, _ = io.ReadAll(req.Body)
	}
	return u.respond(accountID)
}

func (u *stickyUpstream) hitCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.hits)
}

func (u *stickyUpstream) resetHits() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.hits = nil
}

func (u *stickyUpstream) hitOrder() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.hits...)
}

// Anthropic 流：message_start → 一个 text delta → message_delta(usage) → message_stop。
const stickyAnthropicStream = "event: message_start\n" +
	`data: {"type":"message_start","message":{"id":"msg_sticky","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-5","usage":{"input_tokens":7,"output_tokens":1}}}` + "\n\n" +
	"event: content_block_start\n" +
	`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
	"event: content_block_delta\n" +
	`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}` + "\n\n" +
	"event: content_block_stop\n" +
	`data: {"type":"content_block_stop","index":0}` + "\n\n" +
	"event: message_delta\n" +
	`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}` + "\n\n" +
	"event: message_stop\n" +
	`data: {"type":"message_stop"}` + "\n\n"

func stickyOKScript() stickyScript {
	return stickyScript{statusCode: http.StatusOK, streamBody: stickyAnthropicStream}
}

func stickyFailScript() stickyScript {
	return stickyScript{statusCode: http.StatusBadGateway, jsonBody: `{"error":{"message":"upstream unavailable"}}`}
}

// repeatedStickyScript 让同一账号在多次请求里重复给出同一个脚本。
func repeatedStickyScript(script stickyScript, n int) []stickyScript {
	out := make([]stickyScript, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, script)
	}
	return out
}

// ---------------------------------------------------------------------------
// 内存 GatewayCache：旧粘性绑定 + 成功偏好（含 CAS），两者都按分组建键
// ---------------------------------------------------------------------------

type stickyGatewayTestCache struct {
	testutil.StubGatewayCache
	mu        sync.Mutex
	bindings  map[string]int64
	successes map[string]service.GatewayStickySuccessBinding
	// writtenKeys 记录本缓存收到的全部写入键。
	writtenKeys []string
}

func newStickyGatewayTestCache() *stickyGatewayTestCache {
	return &stickyGatewayTestCache{
		bindings:  map[string]int64{},
		successes: map[string]service.GatewayStickySuccessBinding{},
	}
}

func stickySuccessTestKey(groupID int64, sessionHash, model string) string {
	return "success:" + strconv.FormatInt(groupID, 10) + ":" + sessionHash + ":" + model
}

func stickyBindingTestKey(groupID int64, sessionHash string) string {
	return "binding:" + strconv.FormatInt(groupID, 10) + ":" + sessionHash
}

func (c *stickyGatewayTestCache) GetSessionAccountID(_ context.Context, groupID int64, sessionHash string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if id, ok := c.bindings[stickyBindingTestKey(groupID, sessionHash)]; ok {
		return id, nil
	}
	return 0, service.ErrStickySessionNotFound
}

func (c *stickyGatewayTestCache) SetSessionAccountID(_ context.Context, groupID int64, sessionHash string, accountID int64, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := stickyBindingTestKey(groupID, sessionHash)
	c.bindings[key] = accountID
	c.writtenKeys = append(c.writtenKeys, key)
	return nil
}

func (c *stickyGatewayTestCache) RefreshSessionTTL(_ context.Context, groupID int64, sessionHash string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writtenKeys = append(c.writtenKeys, "refresh:"+stickyBindingTestKey(groupID, sessionHash))
	return nil
}

func (c *stickyGatewayTestCache) DeleteSessionAccountID(_ context.Context, groupID int64, sessionHash string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.bindings, stickyBindingTestKey(groupID, sessionHash))
	return nil
}

func (c *stickyGatewayTestCache) GetGatewayStickySuccess(_ context.Context, groupID int64, sessionHash, model string) (service.GatewayStickySuccessBinding, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	binding, ok := c.successes[stickySuccessTestKey(groupID, sessionHash, model)]
	if !ok {
		return binding, service.ErrStickySessionNotFound
	}
	return binding, nil
}

func (c *stickyGatewayTestCache) CompareAndSwapGatewayStickySuccess(_ context.Context, groupID int64, sessionHash, model string, expected, next service.GatewayStickySuccessBinding, _ time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := stickySuccessTestKey(groupID, sessionHash, model)
	if c.successes[key] != expected {
		return false, nil
	}
	c.successes[key] = next
	c.writtenKeys = append(c.writtenKeys, key)
	return true, nil
}

// preference 返回本组下唯一一条会话的成功偏好（测试里只用一条合成会话）。
func (c *stickyGatewayTestCache) preference(t *testing.T) service.GatewayStickySuccessBinding {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	require.Len(t, c.successes, 1, "合成用例只应产生一条会话偏好")
	for _, binding := range c.successes {
		return binding
	}
	return service.GatewayStickySuccessBinding{}
}

func (c *stickyGatewayTestCache) preferenceCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.successes)
}

// bindingGroups 返回写过旧 sticky 键的分组 ID（升序），用于断言写入归属。
func (c *stickyGatewayTestCache) bindingGroups() []int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	seen := map[int64]struct{}{}
	out := make([]int64, 0, len(c.bindings))
	for key := range c.bindings {
		parts := strings.Split(key, ":")
		if len(parts) < 3 || parts[0] != "binding" {
			continue
		}
		groupID, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		if _, ok := seen[groupID]; ok {
			continue
		}
		seen[groupID] = struct{}{}
		out = append(out, groupID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// bindingFor 返回某个分组下唯一一条会话的旧 sticky 绑定。
func (c *stickyGatewayTestCache) bindingFor(groupID int64) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := "binding:" + strconv.FormatInt(groupID, 10) + ":"
	for key, id := range c.bindings {
		if strings.HasPrefix(key, prefix) {
			return id
		}
	}
	return 0
}

func (c *stickyGatewayTestCache) legacyBinding() (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, id := range c.bindings {
		return id, true
	}
	return 0, false
}

func (c *stickyGatewayTestCache) keys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.writtenKeys...)
}

// ---------------------------------------------------------------------------
// 夹具：真实 GatewayHandler + /v1/messages
// ---------------------------------------------------------------------------

const stickyEntryGroupID = int64(20)

type stickyFixture struct {
	router  *gin.Engine
	group   *service.Group
	usage   *stickyUsageLogRepo
	cleanup func()
}

// stickyUsageLogRepo 只截获 Create：其余方法在本用例里不会被调用。
type stickyUsageLogRepo struct {
	service.UsageLogRepository
	mu   sync.Mutex
	logs []*service.UsageLog
}

func (r *stickyUsageLogRepo) Create(_ context.Context, log *service.UsageLog) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copied := *log
	r.logs = append(r.logs, &copied)
	return true, nil
}

func (r *stickyUsageLogRepo) created() []*service.UsageLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*service.UsageLog(nil), r.logs...)
}

// stickyTestAccount 是带上游倍率的合成账号；priority 小的先被选中。
func stickyTestAccount(id int64, priority int, rate float64) *service.Account {
	value := rate
	return &service.Account{
		ID: id, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, Concurrency: 4, Priority: priority,
		RateMultiplier: &value,
		Credentials:    map[string]any{"api_key": "test-key", "base_url": "https://upstream.invalid"},
		AccountGroups:  []service.AccountGroup{{AccountID: id, GroupID: stickyEntryGroupID}},
	}
}

func setAccountRate(account *service.Account, rate float64) {
	value := rate
	account.RateMultiplier = &value
}

type stickyFixtureOptions struct {
	// profitControl 打开入口分组的利润控制（售价有效倍率 1、零最低利润与安全
	// 缓冲，阈值 = 1.0），使成功粘性偏好在真实 handler 上装配。
	profitControl bool
	// mutateGroup 在入口分组装配完成后进一步定制（例如打开 claude_code_only
	// 与降级分组，驱动调度器内部的分组降级）。
	mutateGroup func(*service.Group)
	// groupRepo 覆盖默认的单分组替身，用于多分组场景。
	groupRepo service.GroupRepository
}

// newStickyFixture 装配真实 handler + /v1/messages 路由。
func newStickyFixture(t *testing.T, accounts []*service.Account, upstream *stickyUpstream, cache service.GatewayCache, opts stickyFixtureOptions) *stickyFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	groupID := stickyEntryGroupID
	group := &service.Group{ID: groupID, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive}
	if opts.profitControl {
		group.RateMultiplier = 1.0
		group.SubscriptionType = service.SubscriptionTypeStandard
		group.ProfitControlEnabled = true
		group.ProfitMinMargin = 0
		group.ProfitSafetyBuffer = 0
	}
	if opts.mutateGroup != nil {
		opts.mutateGroup(group)
	}
	var groupRepo service.GroupRepository = &fakeGroupRepo{group: group}
	if opts.groupRepo != nil {
		groupRepo = opts.groupRepo
	}
	repoAccounts := make([]service.Account, 0, len(accounts))
	for _, account := range accounts {
		repoAccounts = append(repoAccounts, *account)
	}
	repo := &openAIImagesFailoverAccountRepo{accounts: repoAccounts}
	snapshot := service.NewSchedulerSnapshotService(&fakeSchedulerCache{accounts: accounts}, nil, nil, nil, nil)
	concurrency := service.NewConcurrencyService(&fakeConcurrencyCache{})
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.Scheduling.FallbackWaitTimeout = time.Second
	cfg.Gateway.Scheduling.FallbackMaxWaiting = 5
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	usageLogs := &stickyUsageLogRepo{}
	gateway := service.NewGatewayService(repo, groupRepo, usageLogs, nil, nil, nil, nil, cache, cfg, snapshot, concurrency,
		service.NewBillingService(cfg, nil), nil, billing, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := &GatewayHandler{
		gatewayService:           gateway,
		billingCacheService:      billing,
		concurrencyHelper:        NewConcurrencyHelper(concurrency, SSEPingFormatClaude, 0),
		cfg:                      cfg,
		maxAccountSwitches:       3,
		maxAccountSwitchesGemini: 3,
	}

	apiKey := &service.APIKey{
		ID: 30, UserID: 40, GroupID: &groupID, Group: group,
		User: &service.User{ID: 40, Concurrency: 4, Balance: 100, Status: service.StatusActive}, Status: service.StatusActive,
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.Group, group))
		c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 40, Concurrency: 4})
		c.Next()
	})
	router.POST("/v1/messages", h.Messages)

	return &stickyFixture{router: router, group: group, usage: usageLogs, cleanup: billing.Stop}
}

func newStickyProfitFixture(t *testing.T, accounts []*service.Account, upstream *stickyUpstream, cache service.GatewayCache) *stickyFixture {
	t.Helper()
	return newStickyFixture(t, accounts, upstream, cache, stickyFixtureOptions{profitControl: true})
}

func (f *stickyFixture) messages(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		bytes.NewBufferString(`{"model":"claude-sonnet-4-5","stream":true,"max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	f.router.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// T1：A 502 → B 成功；下一次相同会话/模型优先 B
// ---------------------------------------------------------------------------

func TestGatewayStickySuccessPrefersTheAccountThatActuallySucceeded(t *testing.T) {
	a, b := int64(9101), int64(9102)
	accounts := []*service.Account{stickyTestAccount(a, 1, 0.5), stickyTestAccount(b, 2, 0.5)}
	upstream := &stickyUpstream{scripts: map[int64][]stickyScript{
		a: repeatedStickyScript(stickyFailScript(), 4),
		b: repeatedStickyScript(stickyOKScript(), 4),
	}}
	cache := newStickyGatewayTestCache()
	fixture := newStickyProfitFixture(t, accounts, upstream, cache)
	defer fixture.cleanup()

	require.Equal(t, http.StatusOK, fixture.messages(t).Code)
	require.Equal(t, []int64{a, b}, upstream.hitOrder(), "首次请求按优先级先试 A，failover 到 B 成功")
	require.Equal(t, b, cache.preference(t).AccountID, "只有真正成功的账号才写成功偏好")
	legacy, ok := cache.legacyBinding()
	require.True(t, ok)
	require.Equal(t, b, legacy, "旧键同步到最后一次成功的账号")

	upstream.resetHits()
	require.Equal(t, http.StatusOK, fixture.messages(t).Code)
	require.Equal(t, []int64{b}, upstream.hitOrder(), "第二次请求必须直接从成功过的 B 开始，不再固定从 A 起")
}

// ---------------------------------------------------------------------------
// T2：全部失败 / 请求取消都不建立成功偏好
// ---------------------------------------------------------------------------

func TestGatewayStickySuccessNotWrittenWhenNothingSucceeds(t *testing.T) {
	a, b := int64(9201), int64(9202)
	accounts := []*service.Account{stickyTestAccount(a, 1, 0.5), stickyTestAccount(b, 2, 0.5)}
	upstream := &stickyUpstream{scripts: map[int64][]stickyScript{
		a: repeatedStickyScript(stickyFailScript(), 4),
		b: repeatedStickyScript(stickyFailScript(), 4),
	}}
	cache := newStickyGatewayTestCache()
	fixture := newStickyProfitFixture(t, accounts, upstream, cache)
	defer fixture.cleanup()

	require.NotEqual(t, http.StatusOK, fixture.messages(t).Code)
	require.Zero(t, cache.preferenceCount(), "全部候选失败不得留下会被反复续命的成功偏好")
	_, bound := cache.legacyBinding()
	require.False(t, bound, "失败尝试不得建立或续期旧绑定")
	require.Empty(t, cache.keys(), "失败请求在粘性缓存上零写入")
}

func TestGatewayStickySuccessNotWrittenWhenClientCancels(t *testing.T) {
	a := int64(9301)
	accounts := []*service.Account{stickyTestAccount(a, 1, 0.5)}

	ctx, cancel := context.WithCancel(context.Background())
	head := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_sticky_cancel","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-5","usage":{"input_tokens":11,"output_tokens":1}}}` + "\n\n" +
		"event: content_block_start\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":23}}` + "\n\n"
	tail := "event: message_stop\n" + `data: {"type":"message_stop"}` + "\n\n"

	upstream := &stickyUpstream{scripts: map[int64][]stickyScript{
		a: {{statusCode: http.StatusOK, streamHead: head, streamTail: tail, onHeadWritten: cancel}},
	}}
	cache := newStickyGatewayTestCache()
	fixture := newStickyProfitFixture(t, accounts, upstream, cache)
	defer fixture.cleanup()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		bytes.NewBufferString(`{"model":"claude-sonnet-4-5","stream":true,"max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	fixture.router.ServeHTTP(rec, req)

	require.Equal(t, 1, upstream.hitCount(), "取消的请求不得重放")
	require.Zero(t, cache.preferenceCount(), "客户端取消不算可确认成功，不写成功偏好")

	// 计费不受影响：断连仍按真实用量入账。
	require.Eventually(t, func() bool { return len(fixture.usage.created()) > 0 }, 3*time.Second, 20*time.Millisecond)
}

// ---------------------------------------------------------------------------
// T3：同一账号连续成功 → 复用并按成功规则续期（revision 更新）
// ---------------------------------------------------------------------------

func TestGatewayStickySuccessRenewsRevisionOnRepeatedSuccess(t *testing.T) {
	a, b := int64(9401), int64(9402)
	accounts := []*service.Account{stickyTestAccount(a, 1, 0.5), stickyTestAccount(b, 2, 0.5)}
	upstream := &stickyUpstream{scripts: map[int64][]stickyScript{
		a: repeatedStickyScript(stickyOKScript(), 4),
		b: repeatedStickyScript(stickyOKScript(), 4),
	}}
	cache := newStickyGatewayTestCache()
	fixture := newStickyProfitFixture(t, accounts, upstream, cache)
	defer fixture.cleanup()

	require.Equal(t, http.StatusOK, fixture.messages(t).Code)
	first := cache.preference(t)
	require.Equal(t, a, first.AccountID)
	require.NotEmpty(t, first.Revision)

	upstream.resetHits()
	require.Equal(t, http.StatusOK, fixture.messages(t).Code)
	require.Equal(t, []int64{a}, upstream.hitOrder(), "仍合格的成功账号继续复用")
	renewed := cache.preference(t)
	require.Equal(t, a, renewed.AccountID)
	require.NotEqual(t, first.Revision, renewed.Revision, "再次成功按成功规则续期")

	// 计费链路不受影响：两次请求都要有真实入账记录，且归属到实际成功的账号。
	require.Eventually(t, func() bool { return len(fixture.usage.created()) >= 2 }, 3*time.Second, 20*time.Millisecond)
	for _, log := range fixture.usage.created() {
		require.Equal(t, a, log.AccountID, "入账必须归属真正成功的账号")
		require.Equal(t, "claude-sonnet-4-5", log.Model)
		require.Greater(t, log.OutputTokens, 0, "假上游已给出 usage")
		require.Greater(t, log.TotalCost, 0.0, "成功请求不得按零费用入账")
	}
}

// ---------------------------------------------------------------------------
// T4 + T6：倍率交换链与「便宜不夺粘性」
// ---------------------------------------------------------------------------

func TestGatewayStickySuccessFollowsEligibilityAcrossRateSwaps(t *testing.T) {
	a, b := int64(9501), int64(9502)
	accountA := stickyTestAccount(a, 1, 0.8)
	accountB := stickyTestAccount(b, 2, 1.2)
	upstream := &stickyUpstream{scripts: map[int64][]stickyScript{
		a: repeatedStickyScript(stickyOKScript(), 6),
		b: repeatedStickyScript(stickyOKScript(), 6),
	}}
	cache := newStickyGatewayTestCache()
	fixture := newStickyProfitFixture(t, []*service.Account{accountA, accountB}, upstream, cache)
	defer fixture.cleanup()

	// 阈值为 1.0：A=0.8 合格、B=1.2 不合格；A 成功。
	require.Equal(t, http.StatusOK, fixture.messages(t).Code)
	require.Equal(t, []int64{a}, upstream.hitOrder())
	require.Equal(t, a, cache.preference(t).AccountID)

	// 交换倍率：A 变成利润不合格，B 合格。
	setAccountRate(accountA, 1.2)
	setAccountRate(accountB, 0.8)
	upstream.resetHits()
	require.Equal(t, http.StatusOK, fixture.messages(t).Code)
	require.Equal(t, []int64{b}, upstream.hitOrder(), "偏好账号已不合格：不得调用、也不得排队等待它")
	require.Equal(t, b, cache.preference(t).AccountID, "合格并成功的 B 接替成功偏好")

	// A 重新降价到与 B 同价：不因为它是更早的绑定就自动抢回。
	setAccountRate(accountA, 0.8)
	upstream.resetHits()
	require.Equal(t, http.StatusOK, fixture.messages(t).Code)
	require.Equal(t, []int64{b}, upstream.hitOrder(), "原账号价格恢复后不自动回粘")
	require.Equal(t, b, cache.preference(t).AccountID)

	// A 比 B 贵但两者都合格：也不为省成本主动打散 B 的成功粘性。
	setAccountRate(accountA, 0.7)
	upstream.resetHits()
	require.Equal(t, http.StatusOK, fixture.messages(t).Code)
	require.Equal(t, []int64{b}, upstream.hitOrder(), "更便宜的候选不夺走已成功账号的粘性")
}

// ---------------------------------------------------------------------------
// T7：偏好账号被停用 → 现有硬否决与重选保留，偏好不得绕门
// ---------------------------------------------------------------------------

func TestGatewayStickySuccessCannotBypassAdministrativeSuspension(t *testing.T) {
	a, b := int64(9601), int64(9602)
	accountA := stickyTestAccount(a, 1, 0.5)
	accountB := stickyTestAccount(b, 2, 0.5)
	upstream := &stickyUpstream{scripts: map[int64][]stickyScript{
		a: repeatedStickyScript(stickyOKScript(), 4),
		b: repeatedStickyScript(stickyOKScript(), 4),
	}}
	cache := newStickyGatewayTestCache()
	fixture := newStickyProfitFixture(t, []*service.Account{accountA, accountB}, upstream, cache)
	defer fixture.cleanup()

	require.Equal(t, http.StatusOK, fixture.messages(t).Code)
	require.Equal(t, a, cache.preference(t).AccountID)

	accountA.Schedulable = false
	upstream.resetHits()
	require.Equal(t, http.StatusOK, fixture.messages(t).Code)
	require.Equal(t, []int64{b}, upstream.hitOrder(), "被停用的偏好账号不得被调用")
	require.Equal(t, b, cache.preference(t).AccountID)
}

// ---------------------------------------------------------------------------
// 对照：无利润门时保持原有行为
// ---------------------------------------------------------------------------

func TestGatewayStickyWithoutProfitGateKeepsLegacyBehavior(t *testing.T) {
	a, b := int64(9801), int64(9802)
	accounts := []*service.Account{stickyTestAccount(a, 1, 0.5), stickyTestAccount(b, 2, 0.5)}
	upstream := &stickyUpstream{scripts: map[int64][]stickyScript{
		a: repeatedStickyScript(stickyFailScript(), 4),
		b: repeatedStickyScript(stickyOKScript(), 4),
	}}
	cache := newStickyGatewayTestCache()
	fixture := newStickyFixture(t, accounts, upstream, cache, stickyFixtureOptions{})
	defer fixture.cleanup()

	require.Equal(t, http.StatusOK, fixture.messages(t).Code)
	require.Equal(t, []int64{a, b}, upstream.hitOrder())
	require.Zero(t, cache.preferenceCount(), "无门路径不装配成功偏好")
	// 原有行为：选号阶段对每个取得槽位的候选做 eager 绑定，成功后再按请求开始
	// 的绑定快照补一次绑定。本次修复没有触碰这条路径，绑定仍落在 B。
	legacy, ok := cache.legacyBinding()
	require.True(t, ok, "无门路径保持 eager 绑定")
	require.Equal(t, b, legacy)
}

// ---------------------------------------------------------------------------
// 跨分组：调度器内部的 Claude Code 降级
// ---------------------------------------------------------------------------

// stickyMultiGroupRepo 按 ID 返回多个分组，用于驱动 resolveGatewayGroup 的降级链。
type stickyMultiGroupRepo struct {
	service.GroupRepository
	groups map[int64]*service.Group
}

func (r *stickyMultiGroupRepo) GetByID(_ context.Context, id int64) (*service.Group, error) {
	return r.groups[id], nil
}

func (r *stickyMultiGroupRepo) GetByIDLite(_ context.Context, id int64) (*service.Group, error) {
	return r.groups[id], nil
}

// stickyDowngradeFixture 搭出「入口分组 claude_code_only → 降级分组」的真实链路：
// 两个合成账号只属于降级分组，A 固定 502、B 成功。entryGate / fallbackGate 分别
// 控制两个分组的利润控制开关。
func stickyDowngradeFixture(t *testing.T, entryGate, fallbackGate bool) (*stickyFixture, *stickyGatewayTestCache, *stickyUpstream) {
	t.Helper()
	const fallbackID = int64(21)
	a, b := int64(9951), int64(9952)
	fallback := &service.Group{
		ID: fallbackID, Hydrated: true, Platform: service.PlatformAnthropic,
		Status: service.StatusActive, RateMultiplier: 1,
		SubscriptionType: service.SubscriptionTypeStandard, ProfitControlEnabled: fallbackGate,
	}
	accounts := []*service.Account{stickyTestAccount(a, 1, 0.5), stickyTestAccount(b, 2, 0.5)}
	for _, account := range accounts {
		account.AccountGroups = []service.AccountGroup{{AccountID: account.ID, GroupID: fallbackID}}
		account.GroupIDs = []int64{fallbackID}
	}
	upstream := &stickyUpstream{scripts: map[int64][]stickyScript{
		a: repeatedStickyScript(stickyFailScript(), 6),
		b: repeatedStickyScript(stickyOKScript(), 6),
	}}
	cache := newStickyGatewayTestCache()
	fixture := newStickyFixture(t, accounts, upstream, cache, stickyFixtureOptions{
		profitControl: entryGate,
		mutateGroup: func(group *service.Group) {
			group.RateMultiplier = 1
			group.SubscriptionType = service.SubscriptionTypeStandard
			group.ClaudeCodeOnly = true
			id := fallbackID
			group.FallbackGroupID = &id
		},
		groupRepo: &stickyMultiGroupRepo{groups: map[int64]*service.Group{fallbackID: fallback}},
	})
	t.Cleanup(fixture.cleanup)
	return fixture, cache, upstream
}

// 降级分组开启利润控制时必须自己学习成功偏好，下一次相同请求直接复用真正成功
// 的账号。
func TestGatewayStickySuccessDowngradePrefersSuccessfulFallbackAccount(t *testing.T) {
	fixture, cache, upstream := stickyDowngradeFixture(t, true, true)
	first := fixture.messages(t)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	require.Equal(t, []int64{9951, 9952}, upstream.hitOrder())
	assert.Equal(t, 1, cache.preferenceCount(), "the effective profit-controlled group must record its successful account")
	assert.Equal(t, int64(9952), cache.bindingFor(21), "only the actual fallback group should receive the successful legacy binding")
	assert.Zero(t, cache.bindingFor(20), "the entry group must not receive fallback bindings")

	upstream.resetHits()
	second := fixture.messages(t)
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	assert.Equal(t, []int64{9952}, upstream.hitOrder(), "a second identical request must prefer the successful account, not replay the failing first hop")
}

// 入口分组关门、降级分组开门时，所有粘性写入都必须归属真实生效的分组。
func TestGatewayStickySuccessDowngradeFromUngatedEntryWritesOnlyEffectiveGroup(t *testing.T) {
	fixture, cache, upstream := stickyDowngradeFixture(t, false, true)
	response := fixture.messages(t)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, []int64{9951, 9952}, upstream.hitOrder())
	assert.Zero(t, cache.bindingFor(20), "the ungated entry group must not receive a binding to an account serving only the fallback group")
	assert.Equal(t, []int64{21}, cache.bindingGroups(), "all sticky writes must belong to the effective group")
	assert.Equal(t, 1, cache.preferenceCount(), "the effective fallback group has profit control enabled and must commit success")
}

// 入口/降级两个分组的利润门四种组合：入口分组的开关只决定入口分组自己，降级
// 分组是否拥有成功偏好由它自己的开关决定；两种情况下写入都只归属有效分组。
func TestGatewayStickySuccessAcrossDowngradeGateCombinations(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		entryGate            bool
		fallbackGate         bool
		expectPreferences    int
		expectSecondHitOrder []int64
	}{
		{name: "entry closed / fallback open", entryGate: false, fallbackGate: true, expectPreferences: 1, expectSecondHitOrder: []int64{9952}},
		{name: "entry open / fallback open", entryGate: true, fallbackGate: true, expectPreferences: 1, expectSecondHitOrder: []int64{9952}},
		{name: "entry open / fallback closed", entryGate: true, fallbackGate: false, expectPreferences: 0, expectSecondHitOrder: []int64{9952}},
		{name: "entry closed / fallback closed", entryGate: false, fallbackGate: false, expectPreferences: 0, expectSecondHitOrder: []int64{9952}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture, cache, upstream := stickyDowngradeFixture(t, tc.entryGate, tc.fallbackGate)

			require.Equal(t, http.StatusOK, fixture.messages(t).Code)
			require.Equal(t, []int64{9951, 9952}, upstream.hitOrder(), "首次请求按优先级先试 A，failover 到 B 成功")

			// 成功偏好：只有降级分组自己开门时才有，且必然记在降级分组上。
			require.Equal(t, tc.expectPreferences, cache.preferenceCount())
			if tc.expectPreferences > 0 {
				require.Equal(t, int64(9952), cache.preference(t).AccountID)
			}

			// 旧 sticky 键：只归属真实生效的降级分组，入口分组一律为空。
			require.Equal(t, []int64{21}, cache.bindingGroups(), "粘性写入只能归属有效分组")
			require.Equal(t, int64(9952), cache.bindingFor(21))
			require.Zero(t, cache.bindingFor(stickyEntryGroupID), "入口分组不得拿到降级分组请求的粘性")

			// 第二次相同请求：无论哪种组合都必须从真正成功过的 B 开始。
			upstream.resetHits()
			require.Equal(t, http.StatusOK, fixture.messages(t).Code)
			require.Equal(t, tc.expectSecondHitOrder, upstream.hitOrder())

			// 计费归属真实账号，不受分组降级影响。
			require.Eventually(t, func() bool { return len(fixture.usage.created()) >= 2 }, 3*time.Second, 20*time.Millisecond)
			for _, log := range fixture.usage.created() {
				require.Equal(t, int64(9952), log.AccountID)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 身份模型 vs 调度模型：渠道映射后不得合并不同请求模型的成功偏好
// ---------------------------------------------------------------------------

// Gemini 原生流：一帧带 usageMetadata 的候选内容即为完整成功终态。
const stickyGeminiStream = `data: {"candidates":[{"content":{"parts":[{"text":"hi"}],"role":"model"},"finishReason":"STOP"}],` +
	`"modelVersion":"gemini-2.5-pro","usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":5,"totalTokenCount":12}}` + "\n\n"

// stickyGeminiTestAccount 与 stickyTestAccount 同构，只是平台为 Gemini。
func stickyGeminiTestAccount(id int64, priority int, rate float64) *service.Account {
	value := rate
	return &service.Account{
		ID: id, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, Concurrency: 4, Priority: priority,
		RateMultiplier: &value,
		Credentials:    map[string]any{"api_key": "test-key", "base_url": "https://upstream.invalid"},
		AccountGroups:  []service.AccountGroup{{AccountID: id, GroupID: stickyEntryGroupID}},
	}
}

// Gemini 原生入口把渠道映射后的 modelName 传给调度器（gemini_v1beta_handler.go
// 的 Select 调用），而成功偏好键必须用映射前的客户端请求模型。两个别名映射到同一
// 个上游模型时，各自学习、各自复用，一个别名的 failover 不得覆盖另一个的偏好。
func TestGatewayStickySuccessKeepsRequestedModelsIsolatedAfterChannelMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const aliasA = "gemini-alias-a"
	const aliasB = "gemini-alias-b"
	const mappedModel = "gemini-2.5-pro"
	const accountA = int64(9981)
	const accountB = int64(9982)
	groupID := stickyEntryGroupID
	group := &service.Group{
		ID: groupID, Hydrated: true, Platform: service.PlatformGemini,
		Status: service.StatusActive, RateMultiplier: 1,
		SubscriptionType: service.SubscriptionTypeStandard, ProfitControlEnabled: true,
	}
	a := stickyGeminiTestAccount(accountA, 1, 0.5)
	b := stickyGeminiTestAccount(accountB, 2, 0.5)
	accounts := []*service.Account{a, b}
	ok := stickyScript{statusCode: http.StatusOK, streamBody: stickyGeminiStream}
	fail := stickyScript{statusCode: http.StatusForbidden, jsonBody: `{"error":{"message":"temporary upstream refusal"}}`}
	upstream := &stickyUpstream{scripts: map[int64][]stickyScript{
		accountA: {ok, fail, ok, ok},
		accountB: repeatedStickyScript(ok, 4),
	}}
	cache := newStickyGatewayTestCache()
	repo := &openAIImagesFailoverAccountRepo{accounts: []service.Account{*a, *b}}
	groupRepo := &fakeGroupRepo{group: group}
	snapshot := service.NewSchedulerSnapshotService(&fakeSchedulerCache{accounts: accounts}, nil, nil, nil, nil)
	concurrency := service.NewConcurrencyService(&fakeConcurrencyCache{})
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.Scheduling.FallbackWaitTimeout = time.Second
	cfg.Gateway.Scheduling.FallbackMaxWaiting = 5
	channel := service.NewChannelService(&openAIWSUsageHandlerChannelRepoStub{
		channels: []service.Channel{{
			ID: 9980, Status: service.StatusActive, GroupIDs: []int64{groupID},
			ModelMapping: map[string]map[string]string{
				service.PlatformGemini: {aliasA: mappedModel, aliasB: mappedModel},
			},
		}},
		groupPlatforms: map[int64]string{groupID: service.PlatformGemini},
	}, nil, nil, nil, nil)
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	usage := &stickyUsageLogRepo{}
	gateway := service.NewGatewayService(
		repo, groupRepo, usage, nil, nil, nil, nil, cache, cfg, snapshot, concurrency,
		service.NewBillingService(cfg, nil), nil, billing, nil, upstream,
		nil, nil, nil, nil, nil, nil, nil, channel, nil, nil, nil, nil,
	)
	gemini := service.NewGeminiMessagesCompatService(repo, groupRepo, nil, snapshot, nil, nil, upstream, nil, cfg)
	h := &GatewayHandler{
		gatewayService: gateway, geminiCompatService: gemini, billingCacheService: billing,
		concurrencyHelper: NewConcurrencyHelper(concurrency, SSEPingFormatNone, 0),
		cfg:               cfg, maxAccountSwitchesGemini: 3,
	}
	apiKey := &service.APIKey{
		ID: 30, UserID: 40, GroupID: &groupID, Group: group, Status: service.StatusActive,
		User: &service.User{ID: 40, Concurrency: 4, Balance: 100, Status: service.StatusActive},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.Group, group))
		c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 40, Concurrency: 4})
		c.Next()
	})
	router.POST("/v1beta/models/*modelAction", h.GeminiV1BetaModels)
	hash := strings.Repeat("a", 64)
	send := func(model string) {
		t.Helper()
		body := `{"contents":[{"role":"user","parts":[{"text":"hello /.gemini/tmp/` + hash + `"}]}]}`
		req := httptest.NewRequest(http.MethodPost, "/v1beta/models/"+model+":streamGenerateContent?alt=sse", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}

	send(aliasA)
	require.Equal(t, []int64{accountA}, upstream.hitOrder())
	upstream.resetHits()
	send(aliasB)
	require.Equal(t, []int64{accountA, accountB}, upstream.hitOrder())
	t.Logf("after the two aliases succeeded: keys=%v", cache.keys())
	assert.Equal(t, 2, cache.preferenceCount(), "the two requested models must retain separate success preferences")
	aliasAPref, err := cache.GetGatewayStickySuccess(context.Background(), groupID, "gemini:"+hash, aliasA)
	assert.NoError(t, err)
	assert.Equal(t, accountA, aliasAPref.AccountID, "alias B failover must not change alias A's preference")
	aliasBPref, err := cache.GetGatewayStickySuccess(context.Background(), groupID, "gemini:"+hash, aliasB)
	assert.NoError(t, err)
	assert.Equal(t, accountB, aliasBPref.AccountID)

	// 两条 success key 各自存在，且都按映射前的别名成键——没有任何一条落在映射后的
	// 上游模型上。
	assert.Contains(t, cache.keys(), stickySuccessTestKey(groupID, "gemini:"+hash, aliasA))
	assert.Contains(t, cache.keys(), stickySuccessTestKey(groupID, "gemini:"+hash, aliasB))
	assert.NotContains(t, cache.keys(), stickySuccessTestKey(groupID, "gemini:"+hash, mappedModel),
		"成功偏好键不得使用渠道映射后的调度模型")

	upstream.resetHits()
	send(aliasA)
	assert.Equal(t, []int64{accountA}, upstream.hitOrder(), "alias A's still eligible successful account must not be replaced by alias B's fallback")
}
