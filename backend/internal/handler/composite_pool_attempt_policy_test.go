package handler

// composite 账号池 per-attempt 平台策略（handler 侧辅助文件）的单元测试：
// attempt 局部 ctx 纯度、被拒平台的整平台 mask 与重选 ctx、配额/渠道限制的终止
// 错误语义。池完整链路（选号→策略→forward→计费）的行为测试在
// internal/server/routes/gateway_composite_pool_attempt_policy_test.go（待 2B
// handler 文件移交接线后补齐）。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ── fakes：只覆盖测试涉及的缓存/仓储入口，其余方法内嵌接口零值 ────────────────

type poolAttemptFakeQuotaCache struct {
	service.BillingCache
	entry *service.UserPlatformQuotaCacheEntry
}

func (f *poolAttemptFakeQuotaCache) GetUserPlatformQuotaCache(context.Context, int64, string) (*service.UserPlatformQuotaCacheEntry, bool, error) {
	if f.entry == nil {
		return nil, false, nil
	}
	return f.entry, true, nil
}

func (f *poolAttemptFakeQuotaCache) SetUserPlatformQuotaCache(context.Context, int64, string, *service.UserPlatformQuotaCacheEntry, time.Duration) error {
	return nil
}

type poolAttemptFakeQuotaRepo struct {
	service.UserPlatformQuotaRepository
	rec *service.UserPlatformQuotaRecord
}

func (f *poolAttemptFakeQuotaRepo) GetByUserPlatform(context.Context, int64, string) (*service.UserPlatformQuotaRecord, error) {
	return f.rec, nil
}

type poolAttemptFakeChannelRepo struct {
	service.ChannelRepository
	channels   []service.Channel
	platforms  map[int64]string
	listAllCnt int
}

func (f *poolAttemptFakeChannelRepo) ListAll(context.Context) ([]service.Channel, error) {
	f.listAllCnt++
	return f.channels, nil
}

func (f *poolAttemptFakeChannelRepo) GetGroupPlatforms(context.Context, []int64) (map[int64]string, error) {
	return f.platforms, nil
}

func newPoolAttemptBilling(t *testing.T, entry *service.UserPlatformQuotaCacheEntry) *service.BillingCacheService {
	t.Helper()
	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Billing.UserPlatformQuotaCacheTTLSeconds = 60
	return service.NewBillingCacheService(
		&poolAttemptFakeQuotaCache{entry: entry}, nil, nil, nil, nil, nil, cfg,
		&poolAttemptFakeQuotaRepo{})
}

// ── compositePoolAttemptContext ──────────────────────────────────────────────

func TestCompositePoolAttemptContextIsolatesPlatform(t *testing.T) {
	base := service.WithCompositeCandidatePlatforms(context.Background(), []string{"deepseek", "openai"})

	attemptCtx := compositePoolAttemptContext(base, &service.Account{Platform: "deepseek"})

	// attempt ctx 携带选中平台，base 候选池保持 immutable 且不携带 resolved。
	platform, ok := service.ResolvedTargetPlatformFromContext(attemptCtx)
	require.True(t, ok)
	require.Equal(t, "deepseek", platform)
	require.Equal(t, []string{"deepseek", "openai"}, service.CompositeCandidatePlatformsFromContext(attemptCtx))

	_, resolved := service.ResolvedTargetPlatformFromContext(base)
	require.False(t, resolved, "original request ctx must stay pool-shaped (no resolved target)")
	require.Equal(t, []string{"deepseek", "openai"}, service.CompositeCandidatePlatformsFromContext(base))
}

func TestCompositePoolAttemptContextGuards(t *testing.T) {
	var nilCtx context.Context
	require.Nil(t, compositePoolAttemptContext(nilCtx, &service.Account{Platform: "openai"}))

	base := context.Background()
	require.Equal(t, base, compositePoolAttemptContext(base, nil))
	require.Equal(t, base, compositePoolAttemptContext(base, &service.Account{Platform: ""}))

	alreadyResolved := service.WithResolvedTargetPlatform(base, "grok")
	require.Equal(t, alreadyResolved, compositePoolAttemptContext(alreadyResolved, &service.Account{Platform: "openai"}),
		"resolved single-target requests must not be overridden by pool attempt semantics")
}

// ── compositePoolPlatformDenials / compositePoolSelectionRetryContext ────────

func TestCompositePoolPlatformDenialsMaskWholePlatform(t *testing.T) {
	denials := newCompositePoolPlatformDenials()
	denials.deny("openai")
	denials.deny("openai") // 幂等

	remaining := denials.filterCandidates([]string{"deepseek", "grok", "openai"})
	require.Equal(t, []string{"deepseek", "grok"}, remaining)
	require.Len(t, denials.filterCandidates([]string{"deepseek", "openai"}), 1)
}

func TestCompositePoolSelectionRetryContextMasksCandidates(t *testing.T) {
	base := service.WithCompositeCandidatePlatforms(context.Background(), []string{"deepseek", "openai"})
	denials := newCompositePoolPlatformDenials()

	// 无 deny：原样返回 base（候选池不变）。
	retryCtx, ok := compositePoolSelectionRetryContext(base, denials)
	require.True(t, ok)
	require.Equal(t, base, retryCtx)

	denials.deny("openai")
	retryCtx, ok = compositePoolSelectionRetryContext(base, denials)
	require.True(t, ok)
	// 候选池保持 immutable（不收缩），仅叠加请求局部 deniedPlatforms 列表。
	require.Equal(t, []string{"deepseek", "openai"}, service.CompositeCandidatePlatformsFromContext(retryCtx))
	require.Equal(t, []string{"openai"}, service.CompositePoolDeniedPlatformsFromContext(retryCtx))
	require.Equal(t, []string{"deepseek", "openai"}, service.CompositeCandidatePlatformsFromContext(base),
		"original request ctx candidates must stay immutable")
	require.Nil(t, service.CompositePoolDeniedPlatformsFromContext(base),
		"base request ctx must not carry the denied overlay")
	_, resolved := service.ResolvedTargetPlatformFromContext(retryCtx)
	require.False(t, resolved, "retry ctx must not collapse into the last attempted account's platform")

	denials.deny("deepseek")
	_, ok = compositePoolSelectionRetryContext(base, denials)
	require.False(t, ok, "all candidates business-limited → terminate instead of re-selecting")
}

// ── respondCompositePoolAttemptPolicyFailure ─────────────────────────────────

func TestRespondCompositePoolAttemptPolicyFailure_QuotaMapsToBillingErrorDetails(t *testing.T) {
	var gotStatus int
	var gotCode string
	write := func(c *gin.Context, status int, code, message string) {
		gotStatus = status
		gotCode = code
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	respondCompositePoolAttemptPolicyFailure(c, compositePoolAttemptPolicyFailure{
		QuotaErr: service.ErrUserPlatformDailyQuotaExhausted,
	}, write)

	require.Equal(t, http.StatusTooManyRequests, gotStatus)
	require.Equal(t, "rate_limit_exceeded", gotCode)
	require.NotEmpty(t, c.Writer.Header().Get("Retry-After"), "quota denial must keep the Retry-After semantics of billingErrorDetails")
}

func TestRespondCompositePoolAttemptPolicyFailure_ChannelRestrictionStays503(t *testing.T) {
	var gotStatus int
	var gotErrType string
	write := func(c *gin.Context, status int, code, message string) {
		gotStatus = status
		gotErrType = code
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	respondCompositePoolAttemptPolicyFailure(c, compositePoolAttemptPolicyFailure{ChannelRestricted: true}, write)

	require.Equal(t, http.StatusServiceUnavailable, gotStatus,
		"channel restriction must keep the selection-stage channel-restriction classification")
	require.Equal(t, "api_error", gotErrType)
	require.Empty(t, c.Writer.Header().Get("Retry-After"))
}

func TestRespondCompositePoolAttemptPolicyFailure_NotDeniedDoesNotWrite(t *testing.T) {
	called := false
	write := func(c *gin.Context, status int, code, message string) { called = true }
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	respondCompositePoolAttemptPolicyFailure(c, compositePoolAttemptPolicyFailure{}, write)
	require.False(t, called)
}

// ── compositePoolAttemptPolicyApplies ────────────────────────────────────────

func poolAttemptGinContext(base context.Context) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(base)
	return c
}

func TestCompositePoolAttemptPolicyAppliesOnlyToActivePools(t *testing.T) {
	compositeKey := &service.APIKey{Group: &service.Group{ID: 1, Platform: service.PlatformComposite}}

	pool := poolAttemptGinContext(service.WithCompositeCandidatePlatforms(context.Background(), []string{"openai"}))
	require.True(t, compositePoolAttemptPolicyApplies(pool, compositeKey))

	// 显式 resolved 单目标（pin/detector）与普通分组保持旧行为。
	resolved := poolAttemptGinContext(service.WithCompositeCandidatePlatforms(
		service.WithResolvedTargetPlatform(context.Background(), "openai"), []string{"openai"}))
	require.False(t, compositePoolAttemptPolicyApplies(resolved, compositeKey))

	plain := poolAttemptGinContext(context.Background())
	require.False(t, compositePoolAttemptPolicyApplies(plain, compositeKey))

	openAIKey := &service.APIKey{Group: &service.Group{ID: 2, Platform: service.PlatformOpenAI}}
	require.False(t, compositePoolAttemptPolicyApplies(pool, openAIKey))
}

// ── evaluateCompositePoolAttemptPolicy ───────────────────────────────────────

func TestEvaluateCompositePoolAttemptPolicy_QuotaDeniedBlocksBeforeForward(t *testing.T) {
	zero := 0.0
	now := time.Now().UTC()
	billing := newPoolAttemptBilling(t, &service.UserPlatformQuotaCacheEntry{
		DailyUsageUSD:    0,
		DailyLimitUSD:    &zero,
		DailyWindowStart: &now,
		SchemaVersion:    service.UserPlatformQuotaCacheSchemaV1,
	})
	base := service.WithCompositeCandidatePlatforms(context.Background(), []string{"openai", "deepseek"})
	apiKey := &service.APIKey{GroupID: &[]int64{1}[0], Group: &service.Group{ID: 1, Platform: service.PlatformComposite}, User: &service.User{ID: 42}}

	attempt := evaluateCompositePoolAttemptPolicy(
		base, billing, nil, apiKey, nil, &service.Account{Platform: "openai"}, "gpt-4", false)

	require.ErrorIs(t, attempt.Failure.QuotaErr, service.ErrUserPlatformDailyQuotaExhausted)
	require.True(t, attempt.Failure.denied())
	_, resolved := service.ResolvedTargetPlatformFromContext(attempt.AttemptCtx)
	require.True(t, resolved)
	_, baseResolved := service.ResolvedTargetPlatformFromContext(base)
	require.False(t, baseResolved, "base request ctx must stay untouched")
}

func TestEvaluateCompositePoolAttemptPolicy_ChannelRestrictionDeniesSelectedPlatform(t *testing.T) {
	now := time.Now().UTC()
	billing := newPoolAttemptBilling(t, &service.UserPlatformQuotaCacheEntry{
		DailyWindowStart: &now,
		SchemaVersion:    service.UserPlatformQuotaCacheSchemaV1, // limits 全 nil → 配额放行
	})
	channelSvc := service.NewChannelService(&poolAttemptFakeChannelRepo{
		channels: []service.Channel{{
			ID:                 1,
			Status:             service.StatusActive,
			GroupIDs:           []int64{4311},
			RestrictModels:     true,
			BillingModelSource: service.BillingModelSourceRequested,
			ModelPricing: []service.ChannelModelPricing{
				{Platform: service.PlatformOpenAI, Models: []string{"gpt-*"}},
			},
		}},
		platforms: map[int64]string{4311: service.PlatformComposite},
	}, nil, nil, nil, nil)
	gateway := service.NewOpenAIGatewayService(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, channelSvc, nil, nil, nil,
	)
	groupID := int64(4311)
	apiKey := &service.APIKey{GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformComposite}}
	base := service.WithCompositeCandidatePlatforms(context.Background(), []string{"openai", "deepseek"})

	// openai 账号 + openai 作用域内的模型 → 放行且映射直通。
	allowed := evaluateCompositePoolAttemptPolicy(
		base, billing, gateway, apiKey, nil, &service.Account{Platform: service.PlatformOpenAI}, "gpt-4", false)
	require.False(t, allowed.Failure.denied())

	// deepseek 账号：gpt-4 不在 deepseek 作用域定价内 → 渠道限制拒绝该平台，
	// 且不能把 openai 行跨平台扫进来误放行。
	denied := evaluateCompositePoolAttemptPolicy(
		base, billing, gateway, apiKey, nil, &service.Account{Platform: service.PlatformDeepseek}, "gpt-4", false)
	require.NoError(t, denied.Failure.QuotaErr)
	require.True(t, denied.Failure.ChannelRestricted)
	require.False(t, denied.Mapping.Mapped,
		"channel mapping must be scoped to the selected platform, not resolved across pool platforms")
}

// TestCompositePoolAttemptChannelPolicyIsPureAndLazy 复核渠道策略 helper 的纯度：
// 只做纯读 + lazy cache 加载——重复评估结论稳定；cache 只在首次查询时从仓储构建
// 一次，后续评估走进程内快照，不产生仓储重复读、不写任何状态（无计数、无扣费
// 入口可触达）。
func TestCompositePoolAttemptChannelPolicyIsPureAndLazy(t *testing.T) {
	repo := &poolAttemptFakeChannelRepo{
		channels: []service.Channel{{
			ID:                 1,
			Status:             service.StatusActive,
			GroupIDs:           []int64{4312},
			RestrictModels:     true,
			BillingModelSource: service.BillingModelSourceRequested,
			ModelPricing: []service.ChannelModelPricing{
				{Platform: service.PlatformOpenAI, Models: []string{"gpt-*"}},
			},
		}},
		platforms: map[int64]string{4312: service.PlatformComposite},
	}
	channelSvc := service.NewChannelService(repo, nil, nil, nil, nil)
	gateway := service.NewOpenAIGatewayService(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, channelSvc, nil, nil, nil,
	)
	groupID := int64(4312)
	attemptCtx := service.WithResolvedTargetPlatform(context.Background(), service.PlatformDeepseek)
	account := &service.Account{Platform: service.PlatformDeepseek}

	first := gateway.CompositePoolAttemptChannelRestricted(attemptCtx, &groupID, account, "gpt-4", false)
	second := gateway.CompositePoolAttemptChannelRestricted(attemptCtx, &groupID, account, "gpt-4", false)
	mapping := gateway.CompositePoolAttemptChannelMapping(attemptCtx, &groupID, "gpt-4")

	require.True(t, first)
	require.True(t, second, "repeated evaluation must be stable (pure read, no state change)")
	require.False(t, mapping.Mapped)
	require.LessOrEqual(t, repo.listAllCnt, 1,
		"channel cache must be built lazily once; repeat checks must read the in-process snapshot")
}
