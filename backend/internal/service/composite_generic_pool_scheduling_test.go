package service

// generic（Anthropic 原生）网关侧 composite 跨平台候选池（account_pool）测试。
//
// 镜像 openai_composite_pool_scheduling_test.go / composite_pool_contract_test.go
// 的构造手法（fake repo/cache + credentials 携带 model_mapping / api_protocol /
// api_base_urls）。覆盖：
//   a. {anthropic, kimi(adaptive)} 双平台池按 priority/LRU 跨平台选中；
//   b. claim 门：候选平台内不 claim 模型的账号不被选（非池请求保持
//      IsModelSupported 原语义）；
//   c. sticky 命中池外 / 已不 claim 账号时被池成员门拦下，绑定不清；
//   d. failover 排除后跨平台选中并粘新账号，池不塌缩；
//   e. 负载感知链路（SelectAccountWithLoadAwareness / resolvePlatform）放行池，
//      原 "composite candidate pool" 报错不再出现；
//   f. 池谓词契约：servable / countable / resolved 平台 pin 优先。
//   g. 转发 base 协议化：adaptive / anthropic-协议账号经 generic 转发打
//      {base}/v1/messages（无 ?beta=true）；classic anthropic 保持
//      base_url+/v1/messages?beta=true + x-api-key 不回归；count_tokens 与
//      passthrough 链同源。

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ---- 测试专用 fake ----

// genericPoolTestGroupRepo 提供 composite 分组解析（GetByIDLite / GetByID）。
type genericPoolTestGroupRepo struct {
	GroupRepository
	group *Group
}

func (r *genericPoolTestGroupRepo) GetByID(context.Context, int64) (*Group, error) {
	if r.group == nil {
		return nil, errors.New("group not found")
	}
	return r.group, nil
}

func (r *genericPoolTestGroupRepo) GetByIDLite(_ context.Context, _ int64) (*Group, error) {
	if r.group == nil {
		return nil, errors.New("group not found")
	}
	return r.group, nil
}

// genericPoolTestAccountRepo 平台过滤语义与生产 repo 一致；组维度按 GroupIDs /
// AccountGroups 过滤（对齐生产 ListSchedulableByGroupIDAndPlatform 的 SQL 语义），
// 指针接收者支持用例中途改账号状态。
type genericPoolTestAccountRepo struct {
	AccountRepository
	accounts []Account
}

func (r *genericPoolTestAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			return &r.accounts[i], nil
		}
	}
	return nil, errors.New("account not found")
}

func (r *genericPoolTestAccountRepo) listByPlatform(platform string) []Account {
	var out []Account
	for i := range r.accounts {
		if r.accounts[i].Platform == platform {
			out = append(out, r.accounts[i])
		}
	}
	return out
}

func (r *genericPoolTestAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, groupID int64, platform string) ([]Account, error) {
	var out []Account
	for i := range r.accounts {
		if r.accounts[i].Platform != platform {
			continue
		}
		for _, ag := range r.accounts[i].AccountGroups {
			if ag.GroupID == groupID {
				out = append(out, r.accounts[i])
				break
			}
		}
	}
	return out, nil
}

func (r *genericPoolTestAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]Account, error) {
	return r.listByPlatform(platform), nil
}

func (r *genericPoolTestAccountRepo) ListSchedulableUngroupedByPlatform(_ context.Context, platform string) ([]Account, error) {
	var out []Account
	for i := range r.accounts {
		if r.accounts[i].Platform == platform && len(r.accounts[i].AccountGroups) == 0 {
			out = append(out, r.accounts[i])
		}
	}
	return out, nil
}

// genericPoolTestAccount 构造池用 APIKey 账号：credentials 携带 model_mapping 与
// 多协议字段（api_protocol / api_base_urls），extra 透传（认证 scheme 覆写等）。
func genericPoolTestAccount(id int64, platform string, groupID int64, priority int, credentials map[string]any, extra map[string]any) Account {
	account := Account{
		ID:            id,
		Platform:      platform,
		Type:          AccountTypeAPIKey,
		Status:        StatusActive,
		Schedulable:   true,
		Concurrency:   1,
		Priority:      priority,
		AccountGroups: []AccountGroup{{AccountID: id, GroupID: groupID}},
		Credentials:   credentials,
	}
	if extra != nil {
		account.Extra = extra
	}
	return account
}

func newGenericPoolTestService(accounts []Account, group *Group, cache GatewayCache, cfg *config.Config) (*GatewayService, *genericPoolTestAccountRepo) {
	repo := &genericPoolTestAccountRepo{accounts: accounts}
	if cfg == nil {
		cfg = &config.Config{}
	}
	svc := &GatewayService{
		accountRepo: repo,
		groupRepo:   &genericPoolTestGroupRepo{group: group},
		cache:       cache,
		cfg:         cfg,
	}
	return svc, repo
}

func newGenericPoolTestGroup(id int64) *Group {
	return &Group{
		ID:       id,
		Platform: PlatformComposite,
		Status:   "active",
		Hydrated: true,
	}
}

func genericPoolTestContext(pool []string) context.Context {
	return WithCompositeCandidatePlatforms(context.Background(), pool)
}

// ---- a. 跨平台按 priority / LRU 选中 ----

func TestGenericCompositePool_SelectsAcrossPlatformsByPriorityAndLoad(t *testing.T) {
	groupID := int64(41001)
	group := newGenericPoolTestGroup(groupID)
	// 公开模型 my-model 由两个平台账号分别经显式映射 claim（claims 规则 A）。
	model := "my-model"
	pool := []string{PlatformAnthropic, PlatformKimi}
	ctx := genericPoolTestContext(pool)

	build := func(anthropicPriority, kimiPriority int, anthropicUsed, kimiUsed *time.Time) *GatewayService {
		anthropic := genericPoolTestAccount(44001, PlatformAnthropic, groupID, anthropicPriority,
			map[string]any{"model_mapping": map[string]any{"my-model": "claude-sonnet-4-5"}}, nil)
		kimi := genericPoolTestAccount(44002, PlatformKimi, groupID, kimiPriority,
			map[string]any{
				"model_mapping": map[string]any{"my-model": "kimi-k3"},
				"api_protocol":  APIProtocolAdaptive,
			}, nil)
		anthropic.LastUsedAt = anthropicUsed
		kimi.LastUsedAt = kimiUsed
		svc, _ := newGenericPoolTestService([]Account{anthropic, kimi}, group, &compositePoolTestCache{}, nil)
		return svc
	}

	// priority 更小者跨平台获胜：kimi(0) 压过 anthropic(5)。
	selected, err := build(5, 0, nil, nil).SelectAccountForModelWithExclusions(ctx, &groupID, "", model, nil)
	require.NoError(t, err)
	require.EqualValues(t, 44002, selected.ID)

	// 反转 priority：anthropic(0) 获胜。
	selected, err = build(0, 5, nil, nil).SelectAccountForModelWithExclusions(ctx, &groupID, "", model, nil)
	require.NoError(t, err)
	require.EqualValues(t, 44001, selected.ID)

	// 同 priority 时 LRU：从未使用的账号获胜。
	now := time.Now()
	selected, err = build(0, 0, &now, nil).SelectAccountForModelWithExclusions(ctx, &groupID, "", model, nil)
	require.NoError(t, err)
	require.EqualValues(t, 44002, selected.ID)

	// 更久未用（或从未使用）的账号获胜。
	stale := now.Add(-2 * time.Hour)
	recent := now.Add(-1 * time.Hour)
	selected, err = build(0, 0, &stale, &recent).SelectAccountForModelWithExclusions(ctx, &groupID, "", model, nil)
	require.NoError(t, err)
	require.EqualValues(t, 44001, selected.ID)

	// 池 immutable：ctx 候选池完整、未写 ResolvedTargetPlatform（不塌缩）。
	require.ElementsMatch(t, pool, CompositeCandidatePlatformsFromContext(ctx))
	_, resolved := ResolvedTargetPlatformFromContext(ctx)
	require.False(t, resolved)
}

// ---- b. claim 门：候选平台内不 claim 模型的账号不被选 ----

func TestGenericCompositePool_ClaimGateFiltersNonClaimingAccounts(t *testing.T) {
	groupID := int64(41002)
	group := newGenericPoolTestGroup(groupID)
	model := "my-model"
	pool := []string{PlatformAnthropic, PlatformKimi, PlatformZhipu}
	ctx := genericPoolTestContext(pool)

	// zhipu 在候选池中但不 claim my-model（mapping 未命中）→ 被 claims 谓词过滤。
	accounts := []Account{
		genericPoolTestAccount(44101, PlatformAnthropic, groupID, 5,
			map[string]any{"model_mapping": map[string]any{"my-model": "claude-sonnet-4-5"}}, nil),
		genericPoolTestAccount(44102, PlatformKimi, groupID, 1,
			map[string]any{"model_mapping": map[string]any{"my-model": "kimi-k3"}}, nil),
		genericPoolTestAccount(44103, PlatformZhipu, groupID, 0,
			map[string]any{"model_mapping": map[string]any{"other-model": "glm-5.2"}}, nil),
	}
	svc, _ := newGenericPoolTestService(accounts, group, &compositePoolTestCache{}, nil)

	selected, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, "", model, nil)
	require.NoError(t, err)
	require.EqualValues(t, 44102, selected.ID, "最低 priority 的 zhipu 不 claim 模型，应由 claim 命中的 kimi 入选")

	// 谓词直测：池请求用 claims（防 IsModelSupported 的 allow-all 冒领），非池请求
	// 保持 IsModelSupported 原语义。
	poolCtx := genericPoolTestContext([]string{PlatformKimi, PlatformZhipu})
	// 空 mapping 的 kimi 账号对无关平台模型是 allow-all（旧语义），但 claims 规则 B
	// 下 detector 平台不一致 → 池路径不得入选。
	kimiEmpty := genericPoolTestAccount(44104, PlatformKimi, groupID, 0, nil, nil)
	require.True(t, kimiEmpty.IsModelSupported("deepseek-chat"), "旧语义 allow-all 仍成立，池路径必须改用 claims 谓词")
	require.False(t, svc.gatewaySchedulingModelSupported(poolCtx, &kimiEmpty, "deepseek-chat"))
	require.True(t, svc.gatewaySchedulingModelSupported(context.Background(), &kimiEmpty, "deepseek-chat"))
	// 池内平台 + 显式 mapping 命中：claims 通过。
	require.True(t, svc.gatewaySchedulingModelSupported(poolCtx, &accounts[2], "other-model"))
}

// ---- c. sticky 命中池外 / 已不 claim 账号时被池成员门拦下 ----

func TestGenericCompositePool_StickyBlockedOutsidePoolOrClaimless(t *testing.T) {
	groupID := int64(41003)
	group := newGenericPoolTestGroup(groupID)
	sessionHash := "sessgenericpool01"
	model := "my-model"
	pool := []string{PlatformAnthropic, PlatformKimi}
	ctx := genericPoolTestContext(pool)

	// 池外账号（openai）也声明 my-model：仅池成员门能拦下它。
	accounts := []Account{
		genericPoolTestAccount(44201, PlatformAnthropic, groupID, 0,
			map[string]any{"model_mapping": map[string]any{"my-model": "claude-sonnet-4-5"}}, nil),
		genericPoolTestAccount(44202, PlatformKimi, groupID, 0,
			map[string]any{
				"model_mapping": map[string]any{"my-model": "kimi-k3"},
				"api_protocol":  APIProtocolAdaptive,
			}, nil),
		genericPoolTestAccount(44203, PlatformOpenAI, groupID, 0,
			map[string]any{"model_mapping": map[string]any{"my-model": "gpt-5.5"}}, nil),
	}
	cache := &compositePoolTestCache{}
	svc, _ := newGenericPoolTestService(accounts, group, cache, nil)

	// Case 1：绑定指向池外 openai 账号 → 池成员门拦下，改选池内账号；随后按既有
	// 选号绑定行为把绑定更新到新选中的账号（与 OpenAI 族 failover 语义一致）。
	require.NoError(t, svc.BindStickySession(ctx, &groupID, sessionHash, 44203))
	selected, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, sessionHash, model, nil)
	require.NoError(t, err)
	require.NotEqualValues(t, 44203, selected.ID)
	require.EqualValues(t, 44201, selected.ID)
	bound, ok := cache.binding(groupID, sessionHash)
	require.True(t, ok)
	require.EqualValues(t, 44201, bound)

	// Case 2：绑定指向 anthropic，但账号 mapping 已不 claim my-model → sticky miss，
	// 改选 claim 命中的池内账号（kimi）。
	require.NoError(t, svc.BindStickySession(ctx, &groupID, sessionHash, 44201))
	accounts[0].Credentials = map[string]any{"model_mapping": map[string]any{"other-model": "claude-sonnet-4-5"}}
	selected, err = svc.SelectAccountForModelWithExclusions(ctx, &groupID, sessionHash, model, nil)
	require.NoError(t, err)
	require.EqualValues(t, 44202, selected.ID)
}

// ---- d. failover 排除后跨平台选中并粘新账号，池不塌缩 ----

func TestGenericCompositePool_FailoverExcludesFailedAccountAndSticksToNewPlatform(t *testing.T) {
	groupID := int64(41004)
	group := newGenericPoolTestGroup(groupID)
	sessionHash := "sessgenericpool02"
	model := "my-model"
	pool := []string{PlatformAnthropic, PlatformKimi}
	ctx := genericPoolTestContext(pool)

	accounts := []Account{
		genericPoolTestAccount(44301, PlatformAnthropic, groupID, 0,
			map[string]any{"model_mapping": map[string]any{"my-model": "claude-sonnet-4-5"}}, nil),
		genericPoolTestAccount(44302, PlatformKimi, groupID, 0,
			map[string]any{
				"model_mapping": map[string]any{"my-model": "kimi-k3"},
				"api_protocol":  APIProtocolAdaptive,
			}, nil),
	}
	cache := &compositePoolTestCache{}
	svc, _ := newGenericPoolTestService(accounts, group, cache, nil)

	first, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, sessionHash, model, nil)
	require.NoError(t, err)
	require.EqualValues(t, 44301, first.ID)

	// 排除失败账号后跨平台选中 kimi，并更新同组绑定。
	excluded := map[int64]struct{}{44301: {}}
	second, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, sessionHash, model, excluded)
	require.NoError(t, err)
	require.EqualValues(t, 44302, second.ID)
	bound, ok := cache.binding(groupID, sessionHash)
	require.True(t, ok)
	require.EqualValues(t, 44302, bound)

	// 后续请求粘新账号；原报错文案不再出现。
	third, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, sessionHash, model, nil)
	require.NoError(t, err)
	require.EqualValues(t, 44302, third.ID)
	require.ElementsMatch(t, pool, CompositeCandidatePlatformsFromContext(ctx))
}

// ---- e. 负载感知链路放行池（resolvePlatform 池分支不再报错） ----

func TestGenericCompositePool_LoadAwareSelectsAcrossPlatforms(t *testing.T) {
	groupID := int64(41005)
	group := newGenericPoolTestGroup(groupID)
	model := "my-model"
	pool := []string{PlatformAnthropic, PlatformKimi}
	ctx := genericPoolTestContext(pool)

	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	accounts := []Account{
		genericPoolTestAccount(44401, PlatformAnthropic, groupID, 5,
			map[string]any{"model_mapping": map[string]any{"my-model": "claude-sonnet-4-5"}}, nil),
		genericPoolTestAccount(44402, PlatformKimi, groupID, 0,
			map[string]any{
				"model_mapping": map[string]any{"my-model": "kimi-k3"},
				"api_protocol":  APIProtocolAdaptive,
			}, nil),
	}
	svc, _ := newGenericPoolTestService(accounts, group, &compositePoolTestCache{}, cfg)
	svc.concurrencyService = NewConcurrencyService(schedulerTestConcurrencyCache{})

	selection, err := svc.SelectAccountWithLoadAwareness(ctx, &groupID, "", model, nil, "", 0)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.EqualValues(t, 44402, selection.Account.ID, "priority 更小的 kimi 跨平台获胜，resolvePlatform 池分支不再报 candidate pool 错误")

	// sticky 命中池内账号时正常复检通过。
	sessionHash := "sessgenericpool05"
	require.NoError(t, svc.BindStickySession(ctx, &groupID, sessionHash, 44401))
	selection, err = svc.SelectAccountWithLoadAwareness(ctx, &groupID, sessionHash, model, nil, "", 0)
	require.NoError(t, err)
	require.EqualValues(t, 44401, selection.Account.ID, "池内 sticky 正常命中")

	// sticky 命中的账号不再 claim 模型：Layer 1.5 模型门拦下，回退负载感知选中
	// 仍在 claim 的池内账号。
	accounts[0].Credentials = map[string]any{"model_mapping": map[string]any{"other-model": "claude-sonnet-4-5"}}
	selection, err = svc.SelectAccountWithLoadAwareness(ctx, &groupID, sessionHash, model, nil, "", 0)
	require.NoError(t, err)
	require.EqualValues(t, 44402, selection.Account.ID)
}

// ---- e2. 显式 resolved 平台 pin 优先于池（单平台旧行为保持） ----

func TestGenericCompositePool_ResolvedPlatformPinBeatsPool(t *testing.T) {
	groupID := int64(41006)
	group := newGenericPoolTestGroup(groupID)
	model := "my-model"

	accounts := []Account{
		genericPoolTestAccount(44501, PlatformAnthropic, groupID, 0,
			map[string]any{"model_mapping": map[string]any{"my-model": "claude-sonnet-4-5"}}, nil),
		genericPoolTestAccount(44502, PlatformKimi, groupID, 5,
			map[string]any{
				"model_mapping": map[string]any{"my-model": "kimi-k3"},
				"api_protocol":  APIProtocolAdaptive,
			}, nil),
	}
	svc, _ := newGenericPoolTestService(accounts, group, &compositePoolTestCache{}, nil)

	// pin 到 kimi：即便 anthropic priority 更高也按单平台旧行为选中 kimi。
	pinnedCtx := WithResolvedTargetPlatform(genericPoolTestContext([]string{PlatformAnthropic, PlatformKimi}), PlatformKimi)
	selected, err := svc.SelectAccountForModelWithExclusions(pinnedCtx, &groupID, "", model, nil)
	require.NoError(t, err)
	require.EqualValues(t, 44502, selected.ID)
}

// ---- f. 池谓词契约：servable / countable / pin 判定 ----

func TestCompositeGenericPoolPredicates(t *testing.T) {
	poolCtx := genericPoolTestContext([]string{PlatformAnthropic, PlatformKimi, PlatformOpenAI})

	anthropicNative := genericPoolTestAccount(1, PlatformAnthropic, 7, 0, nil, nil)
	anthropicOAuth := Account{ID: 2, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	kimiAdaptive := genericPoolTestAccount(3, PlatformKimi, 7, 0,
		map[string]any{"api_protocol": APIProtocolAdaptive}, nil)
	kimiChat := genericPoolTestAccount(4, PlatformKimi, 7, 0,
		map[string]any{"api_protocol": APIProtocolChatCompletions}, nil)
	zhipuAnthropic := genericPoolTestAccount(5, PlatformZhipu, 7, 0,
		map[string]any{"api_protocol": APIProtocolAnthropic}, nil)
	openai := genericPoolTestAccount(6, PlatformOpenAI, 7, 0, nil, nil)
	gemini := Account{ID: 7, Platform: PlatformGemini, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true}
	antigravity := Account{ID: 8, Platform: PlatformAntigravity, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true}

	// servable：anthropic 原生（含 OAuth）/ adaptive / anthropic-协议 / gemini /
	// antigravity / OpenAI 兼容（委派）均可入池。
	require.True(t, genericCompositePoolAccountServable(&anthropicNative))
	require.True(t, genericCompositePoolAccountServable(&anthropicOAuth))
	require.True(t, genericCompositePoolAccountServable(&kimiAdaptive))
	require.True(t, genericCompositePoolAccountServable(&zhipuAnthropic))
	require.True(t, genericCompositePoolAccountServable(&kimiChat), "纯 OpenAI 协议国产账号由 handler 委派转发，池成员资格保留")
	require.True(t, genericCompositePoolAccountServable(&openai))
	require.True(t, genericCompositePoolAccountServable(&gemini))
	require.True(t, genericCompositePoolAccountServable(&antigravity))

	// 池成员资格：平台 ∈ 候选池为准；池外平台一律拒绝。
	require.True(t, genericCompositePoolAllowsAccount(poolCtx, &kimiAdaptive))
	require.True(t, genericCompositePoolAllowsAccount(poolCtx, &anthropicNative))
	require.True(t, genericCompositePoolAllowsAccount(poolCtx, &openai))
	require.False(t, genericCompositePoolAllowsAccount(poolCtx, &gemini), "gemini 不在候选池")
	require.False(t, genericCompositePoolAllowsAccount(context.Background(), &kimiAdaptive), "无池恒 false")

	// countable：anthropic 原生 / anthropic-协议 / adaptive / gemini / antigravity
	// 可计数；纯 OpenAI 族（chat_completions 国产 / openai）不算。
	require.True(t, GenericCompositePoolCountableAccount(&anthropicNative))
	require.True(t, GenericCompositePoolCountableAccount(&anthropicOAuth))
	require.True(t, GenericCompositePoolCountableAccount(&kimiAdaptive))
	require.True(t, GenericCompositePoolCountableAccount(&zhipuAnthropic))
	require.True(t, GenericCompositePoolCountableAccount(&gemini))
	require.True(t, GenericCompositePoolCountableAccount(&antigravity))
	require.False(t, GenericCompositePoolCountableAccount(&kimiChat))
	require.False(t, GenericCompositePoolCountableAccount(&openai))

	// 池激活判定：resolved 平台优先于池；无池恒 false。
	require.True(t, GenericCompositePoolActive(poolCtx))
	require.False(t, GenericCompositePoolActive(WithResolvedTargetPlatform(poolCtx, PlatformKimi)))
	require.False(t, GenericCompositePoolActive(context.Background()))
}

// ---- g. 转发 base 协议化 ----

// newAdaptiveAnthropicBaseAccount 构造多协议 APIKey 账号，api_base_urls.anthropic
// 指向给定上游 base（adaptive），或 base_url + api_protocol=anthropic。
func newAdaptiveAnthropicBaseAccount(id int64, platform string, credentials map[string]any, extra map[string]any) *Account {
	account := genericPoolTestAccount(id, platform, 0, 0, credentials, extra)
	return &account
}

func newForwardTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return c, rec
}

func newForwardTestService(allowInsecureHTTP bool) *GatewayService {
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowInsecureHTTP = allowInsecureHTTP
	return &GatewayService{cfg: cfg}
}

// TestAnthropicProtocolUpstreamTargetURL_AdaptiveAndAnthropicProtocolNoBeta：
// adaptive / anthropic-协议账号经 generic 转发打 {base}/v1/messages，无 ?beta=true；
// 认证与实际 base 同源（默认 x-api-key，opt-in Bearer / ollama base 强制 Bearer）。
func TestAnthropicProtocolUpstreamTargetURL_AdaptiveAndAnthropicProtocolNoBeta(t *testing.T) {
	upstreamURL := "http://127.0.0.1:45991/anthropic"
	svc := newForwardTestService(true)
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":100}`)

	build := func(credentials map[string]any, extra map[string]any) (*http.Request, error) {
		account := newAdaptiveAnthropicBaseAccount(46001, PlatformKimi, credentials, extra)
		c, _ := newForwardTestContext()
		req, _, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "upstream-key", "apikey", "claude-sonnet-4-5", false, false)
		return req, err
	}

	// adaptive：api_base_urls.anthropic 生效，{base}/v1/messages 无 beta 参数。
	req, err := build(map[string]any{
		"api_key":       "upstream-key",
		"api_protocol":  APIProtocolAdaptive,
		"api_base_urls": map[string]any{"anthropic": upstreamURL},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, upstreamURL+"/v1/messages", req.URL.String())
	require.Empty(t, req.URL.RawQuery)
	require.Equal(t, "upstream-key", req.Header.Get("x-api-key"), "Kimi 默认保持 x-api-key 认证")
	require.Empty(t, req.Header.Get("Authorization"))

	// adaptive + opt-in Bearer：认证仍与同一 base 同源。
	req, err = build(map[string]any{
		"api_key":       "upstream-key",
		"api_protocol":  APIProtocolAdaptive,
		"api_base_urls": map[string]any{"anthropic": upstreamURL},
	}, map[string]any{anthropicAPIKeyAuthSchemeExtraKey: AnthropicAPIKeyAuthSchemeAuthorizationBearer})
	require.NoError(t, err)
	require.Equal(t, upstreamURL+"/v1/messages", req.URL.String())
	require.Equal(t, "Bearer upstream-key", req.Header.Get("Authorization"))
	require.Empty(t, req.Header.Get("x-api-key"))

	// anthropic-protocol：credential base_url 即 Anthropic 端点，朴素拼接无 beta。
	req, err = build(map[string]any{
		"api_key":      "upstream-key",
		"api_protocol": APIProtocolAnthropic,
		"base_url":     upstreamURL,
	}, nil)
	require.NoError(t, err)
	require.Equal(t, upstreamURL+"/v1/messages", req.URL.String())
	require.Empty(t, req.URL.RawQuery)

	// 协议化 base 指向 ollama.com：clamp 与认证判定同源（Bearer + max_tokens 压到 cap）。
	ollamaBody, _ := sjson.SetBytes(body, "model", "deepseek-v4")
	ollamaBody, _ = sjson.SetBytes(ollamaBody, "max_tokens", 200000)
	account := newAdaptiveAnthropicBaseAccount(46002, PlatformDeepseek, map[string]any{
		"api_key":       "upstream-key",
		"api_protocol":  APIProtocolAdaptive,
		"api_base_urls": map[string]any{"anthropic": "https://ollama.com"},
	}, nil)
	c, _ := newForwardTestContext()
	req, wireBody, err := svc.buildUpstreamRequest(context.Background(), c, account, ollamaBody, "upstream-key", "apikey", "deepseek-v4", false, false)
	require.NoError(t, err)
	require.Equal(t, "https://ollama.com/v1/messages", req.URL.String())
	require.Empty(t, req.URL.RawQuery)
	require.Equal(t, "Bearer upstream-key", req.Header.Get("Authorization"), "ollama 上游按实际 base 强制 Bearer")
	require.EqualValues(t, 65535, gjson.GetBytes(wireBody, "max_tokens").Int(), "clamp 与协议化 base 同源判定")
}

// TestAnthropicProtocolUpstreamTargetURL_ClassicAnthropicKeepsBeta：classic
// anthropic 账号保持 base_url+/v1/messages?beta=true + x-api-key 不回归；
// count_tokens 与 passthrough 链同源。
func TestAnthropicProtocolUpstreamTargetURL_ClassicAnthropicKeepsBeta(t *testing.T) {
	upstreamURL := "https://relay.example.com"
	svc := newForwardTestService(false)
	account := newAdaptiveAnthropicBaseAccount(46003, PlatformAnthropic, map[string]any{
		"api_key":  "anthropic-key",
		"base_url": upstreamURL,
	}, nil)
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":100}`)

	// messages：beta=true + x-api-key。
	c, _ := newForwardTestContext()
	req, _, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "anthropic-key", "apikey", "claude-sonnet-4-5", false, false)
	require.NoError(t, err)
	require.Equal(t, upstreamURL+"/v1/messages?beta=true", req.URL.String())
	require.Equal(t, "beta=true", req.URL.RawQuery)
	require.Equal(t, "anthropic-key", req.Header.Get("x-api-key"))
	require.Empty(t, req.Header.Get("Authorization"))

	// count_tokens：beta=true + x-api-key。
	c, _ = newForwardTestContext()
	ctReq, _, err := svc.buildCountTokensRequest(context.Background(), c, account, body, "anthropic-key", "apikey", "claude-sonnet-4-5", false)
	require.NoError(t, err)
	require.Equal(t, upstreamURL+"/v1/messages/count_tokens?beta=true", ctReq.URL.String())
	require.Equal(t, "anthropic-key", ctReq.Header.Get("x-api-key"))

	// passthrough 链：classic 账号不回归。
	c, _ = newForwardTestContext()
	ptReq, _, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(context.Background(), c, account, body, "anthropic-key")
	require.NoError(t, err)
	require.Equal(t, upstreamURL+"/v1/messages?beta=true", ptReq.URL.String())
	require.Equal(t, "anthropic-key", ptReq.Header.Get("x-api-key"))

	// count_tokens 协议化 base：{base}/v1/messages/count_tokens，无 beta 参数，
	// 认证与同一 base 同源（opt-in Bearer）。
	adaptiveAccount := newAdaptiveAnthropicBaseAccount(46004, PlatformKimi, map[string]any{
		"api_key":       "upstream-key",
		"api_protocol":  APIProtocolAdaptive,
		"api_base_urls": map[string]any{"anthropic": upstreamURL},
	}, map[string]any{anthropicAPIKeyAuthSchemeExtraKey: AnthropicAPIKeyAuthSchemeAuthorizationBearer})
	c, _ = newForwardTestContext()
	adaptiveCTReq, _, err := svc.buildCountTokensRequest(context.Background(), c, adaptiveAccount, body, "upstream-key", "apikey", "my-model", false)
	require.NoError(t, err)
	require.Equal(t, upstreamURL+"/v1/messages/count_tokens", adaptiveCTReq.URL.String())
	require.Empty(t, adaptiveCTReq.URL.RawQuery)
	require.Equal(t, "Bearer upstream-key", adaptiveCTReq.Header.Get("Authorization"))
}
