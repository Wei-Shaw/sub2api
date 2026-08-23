//go:build unit

package service

import (
	"context"
	"math"
	"math/rand"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// scheduleScenario 阶梯表测试场景：同一份场景既做断言也做与计费函数的对账。
type scheduleScenario struct {
	name          string
	model         string
	platform      string
	groupPlatform string
	group         *Group
	channel       []ChannelModelPricing
	catalog       *PricingService
	wantErr       bool
	wantNil       bool
	wantBasis     ContextPricingBasis
	check         func(t *testing.T, s *ContextPricingSchedule)
REDACTED

func enabledGroup(platform string) *Group {
	return &Group{ID: 100, Platform: platform, LongContextPricingEnabled: trueREDACTED
REDACTED

func disabledGroup(platform string) *Group {
	return &Group{ID: 100, Platform: platform, LongContextPricingEnabled: falseREDACTED
REDACTED

func sonnetChannel(iv ...PricingInterval) []ChannelModelPricing {
	return []ChannelModelPricing{{
		Platform: PlatformAnthropic, Models: []string{"claude-sonnet-4"REDACTED, BillingMode: BillingModeToken,
		InputPrice: testPtrFloat64(2e-6), Intervals: iv,
REDACTEDREDACTED
REDACTED

func requireTier(t *testing.T, tier ContextPricingTier, min int, max *int, label string, input, output, cacheWrite, cacheRead *float64) {
REDACTED
	require.Equal(t, min, tier.MinTokens, "min")
	if max == nil {
		require.Nil(t, tier.MaxTokens, "max should be unbounded")
REDACTED else {
		require.NotNil(t, tier.MaxTokens, "max")
		require.Equal(t, *max, *tier.MaxTokens, "max")
REDACTED
	require.Equal(t, label, tier.Label, "label")
	requirePrice(t, input, tier.Input, "input")
	requirePrice(t, output, tier.Output, "output")
	requirePrice(t, cacheWrite, tier.CacheWrite, "cache_write")
	requirePrice(t, cacheRead, tier.CacheRead, "cache_read")
REDACTED

func requirePrice(t *testing.T, want, got *float64, field string) {
REDACTED
	if want == nil {
		require.Nil(t, got, field)
		return
REDACTED
	require.NotNil(t, got, field)
	require.InDelta(t, *want, *got, 1e-15, field)
REDACTED

func scheduleScenarios() []scheduleScenario {
	p := testPtrFloat64
	return []scheduleScenario{
		{
			name: "官方阶梯 gpt-5.4 整单两档", model: "gpt-5.4", platform: PlatformOpenAI, groupPlatform: PlatformOpenAI,
			group: enabledGroup(PlatformOpenAI), wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 2)
				requireTier(t, s.Tiers[0], 0, intPtr(272000), "≤272K", p(2.5e-6), p(15e-6), p(2.5e-6), p(0.25e-6))
				requireTier(t, s.Tiers[1], 272000, nil, ">272K", p(5e-6), p(22.5e-6), p(5e-6), p(0.5e-6))
		REDACTED,
	REDACTED,
		{
			name: "Grok 达到阈值即进高档", model: "grok-4.5", platform: PlatformGrok, groupPlatform: PlatformGrok,
			group: enabledGroup(PlatformGrok), wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 2)
				requireTier(t, s.Tiers[0], 0, intPtr(199999), "<200K", p(2e-6), p(6e-6), nil, p(0.3e-6))
				requireTier(t, s.Tiers[1], 199999, nil, "≥200K", p(4e-6), p(12e-6), nil, p(0.6e-6))
		REDACTED,
	REDACTED,
		{
			name: "分组关闭阶梯只剩基础档", model: "gpt-5.4", platform: PlatformOpenAI, groupPlatform: PlatformOpenAI,
			group: disabledGroup(PlatformOpenAI), wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 1)
				requireTier(t, s.Tiers[0], 0, nil, "", p(2.5e-6), p(15e-6), p(2.5e-6), p(0.25e-6))
		REDACTED,
	REDACTED,
		{
			name: "官方参考价（无分组）带目录阶梯", model: "gpt-5.4", platform: "", groupPlatform: PlatformOpenAI,
			group: nil, wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 2)
				requireTier(t, s.Tiers[1], 272000, nil, ">272K", p(5e-6), p(22.5e-6), p(5e-6), p(0.5e-6))
		REDACTED,
	REDACTED,
		{
			name: "渠道倍率区间按渠道平价覆盖后的 base 折算", model: "claude-sonnet-4", platform: PlatformAnthropic, groupPlatform: PlatformAnthropic,
			group: enabledGroup(PlatformAnthropic), wantBasis: ContextPricingBasisWholeRequest,
			channel: sonnetChannel(
				PricingInterval{MinTokens: 0, MaxTokens: intPtr(200000), InputMultiplier: p(1)REDACTED,
				PricingInterval{MinTokens: 200000, TierLabel: "long", InputMultiplier: p(2), OutputMultiplier: p(1.5), CacheWriteMultiplier: p(2), CacheReadMultiplier: p(2)REDACTED,
			),
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 2)
				requireTier(t, s.Tiers[0], 0, intPtr(200000), "", p(2e-6), p(15e-6), p(3.75e-6), p(0.3e-6))
				requireTier(t, s.Tiers[1], 200000, nil, "long", p(4e-6), p(22.5e-6), p(7.5e-6), p(0.6e-6))
		REDACTED,
	REDACTED,
		{
			name: "区间显式价优先于倍率", model: "claude-sonnet-4", platform: PlatformAnthropic, groupPlatform: PlatformAnthropic,
			group: enabledGroup(PlatformAnthropic), wantBasis: ContextPricingBasisWholeRequest,
			channel: sonnetChannel(
				PricingInterval{MinTokens: 0, MaxTokens: intPtr(200000), InputMultiplier: p(1)REDACTED,
				PricingInterval{MinTokens: 200000, InputPrice: p(9e-6), InputMultiplier: p(2)REDACTED,
			),
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 2)
				requirePrice(t, p(9e-6), s.Tiers[1].Input, "input")
		REDACTED,
	REDACTED,
		{
			name: "区间空洞按 base 补档且同价段合并", model: "claude-sonnet-4", platform: PlatformAnthropic, groupPlatform: PlatformAnthropic,
			group: enabledGroup(PlatformAnthropic), wantBasis: ContextPricingBasisWholeRequest,
			channel: sonnetChannel(
				PricingInterval{MinTokens: 0, MaxTokens: intPtr(100000), InputMultiplier: p(0.5)REDACTED,
				PricingInterval{MinTokens: 200000, InputMultiplier: p(2)REDACTED,
			),
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 3)
				requireTier(t, s.Tiers[0], 0, intPtr(100000), "", p(1e-6), p(15e-6), p(3.75e-6), p(0.3e-6))
				requireTier(t, s.Tiers[1], 100000, intPtr(200000), "", p(2e-6), p(15e-6), p(3.75e-6), p(0.3e-6))
				requireTier(t, s.Tiers[2], 200000, nil, "", p(4e-6), p(15e-6), p(3.75e-6), p(0.3e-6))
		REDACTED,
	REDACTED,
		{
			name: "首档同价空洞被合并", model: "claude-sonnet-4", platform: PlatformAnthropic, groupPlatform: PlatformAnthropic,
			group: enabledGroup(PlatformAnthropic), wantBasis: ContextPricingBasisWholeRequest,
			channel: sonnetChannel(
				PricingInterval{MinTokens: 0, MaxTokens: intPtr(100000), InputMultiplier: p(1)REDACTED,
				PricingInterval{MinTokens: 200000, InputMultiplier: p(2)REDACTED,
			),
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 2)
				requireTier(t, s.Tiers[0], 0, intPtr(200000), "", p(2e-6), p(15e-6), p(3.75e-6), p(0.3e-6))
				requireTier(t, s.Tiers[1], 200000, nil, "", p(4e-6), p(15e-6), p(3.75e-6), p(0.3e-6))
		REDACTED,
	REDACTED,
		{
			name: "末档有上限时尾段回落 base", model: "claude-sonnet-4", platform: PlatformAnthropic, groupPlatform: PlatformAnthropic,
			group: enabledGroup(PlatformAnthropic), wantBasis: ContextPricingBasisWholeRequest,
			channel: sonnetChannel(
				PricingInterval{MinTokens: 0, MaxTokens: intPtr(200000), InputMultiplier: p(1)REDACTED,
				PricingInterval{MinTokens: 200000, MaxTokens: intPtr(1000000), InputMultiplier: p(2)REDACTED,
			),
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 3)
				requireTier(t, s.Tiers[1], 200000, intPtr(1000000), "", p(4e-6), p(15e-6), p(3.75e-6), p(0.3e-6))
				requireTier(t, s.Tiers[2], 1000000, nil, "", p(2e-6), p(15e-6), p(3.75e-6), p(0.3e-6))
		REDACTED,
	REDACTED,
		{
			name: "分组关闭时渠道区间折叠到 1-token 档", model: "claude-sonnet-4", platform: PlatformAnthropic, groupPlatform: PlatformAnthropic,
			group: disabledGroup(PlatformAnthropic), wantBasis: ContextPricingBasisWholeRequest,
			channel: sonnetChannel(
				PricingInterval{MinTokens: 0, MaxTokens: intPtr(200000), InputMultiplier: p(0.5)REDACTED,
				PricingInterval{MinTokens: 200000, InputMultiplier: p(2)REDACTED,
			),
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 1)
				requireTier(t, s.Tiers[0], 0, nil, "", p(1e-6), p(15e-6), p(3.75e-6), p(0.3e-6))
		REDACTED,
	REDACTED,
		{
			name: "分组关闭且首档不从 0 起时平价为 base", model: "claude-sonnet-4", platform: PlatformAnthropic, groupPlatform: PlatformAnthropic,
			group: disabledGroup(PlatformAnthropic), wantBasis: ContextPricingBasisWholeRequest,
			channel: sonnetChannel(PricingInterval{MinTokens: 200000, InputMultiplier: p(2)REDACTED),
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 1)
				requirePrice(t, p(2e-6), s.Tiers[0].Input, "input")
		REDACTED,
	REDACTED,
		{
			name: "分组 token 价卡整张替换渠道定价并剥区间", model: "claude-sonnet-4", platform: PlatformAnthropic, groupPlatform: PlatformAnthropic,
			group: &Group{ID: 100, Platform: PlatformAnthropic, LongContextPricingEnabled: true, ModelPricing: []ChannelModelPricing{{
				Models: []string{"claude-sonnet-*"REDACTED, BillingMode: BillingModeToken, InputPrice: p(1e-6),
				Intervals: []PricingInterval{{MinTokens: 200000, InputMultiplier: p(5)REDACTEDREDACTED,
		REDACTEDREDACTEDREDACTED,
			channel:   sonnetChannel(PricingInterval{MinTokens: 200000, InputMultiplier: p(3)REDACTED),
			wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 1)
				requireTier(t, s.Tiers[0], 0, nil, "", p(1e-6), p(15e-6), p(3.75e-6), p(0.3e-6))
		REDACTED,
	REDACTED,
		{
			name: "分组价卡之上叠加官方阶梯", model: "gpt-5.4", platform: PlatformOpenAI, groupPlatform: PlatformOpenAI,
			group: &Group{ID: 100, Platform: PlatformOpenAI, LongContextPricingEnabled: true, ModelPricing: []ChannelModelPricing{{
				Models: []string{"gpt-5.4"REDACTED, BillingMode: BillingModeToken, InputPrice: p(1e-6),
		REDACTEDREDACTEDREDACTED,
			wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 2)
				requirePrice(t, p(1e-6), s.Tiers[0].Input, "input")
				requirePrice(t, p(2e-6), s.Tiers[1].Input, "input")
				requirePrice(t, p(22.5e-6), s.Tiers[1].Output, "output")
		REDACTED,
	REDACTED,
		{
			name: "Gemini 旧规则按超出部分计价", model: "gemini-2.5-pro", platform: PlatformGemini, groupPlatform: PlatformGemini,
			group: enabledGroup(PlatformGemini), catalog: geminiCatalogStub(), wantBasis: ContextPricingBasisMarginal,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 2)
				requireTier(t, s.Tiers[0], 0, intPtr(200000), "≤200K", p(1.25e-6), p(10e-6), nil, p(0.3125e-6))
				requireTier(t, s.Tiers[1], 200000, nil, ">200K", p(2.5e-6), p(10e-6), nil, p(0.625e-6))
		REDACTED,
	REDACTED,
		{
			name: "Gemini 分组关闭时不用旧规则", model: "gemini-2.5-pro", platform: PlatformGemini, groupPlatform: PlatformGemini,
			group: disabledGroup(PlatformGemini), catalog: geminiCatalogStub(), wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 1)
		REDACTED,
	REDACTED,
		{
			name: "Gemini 有渠道定价时旧规则让位", model: "gemini-2.5-pro", platform: PlatformGemini, groupPlatform: PlatformGemini,
			group: enabledGroup(PlatformGemini), catalog: geminiCatalogStub(),
			channel: []ChannelModelPricing{{
		REDACTED Models: []string{"gemini-2.5-pro"REDACTED, BillingMode: BillingModeToken, InputPrice: p(3e-6),
	REDACTED
			wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 1)
				requirePrice(t, p(3e-6), s.Tiers[0].Input, "input")
		REDACTED,
	REDACTED,
		{
			name: "Gemini 官方参考不套用站内旧规则", model: "gemini-2.5-pro", platform: "", groupPlatform: PlatformGemini,
			group: nil, catalog: geminiCatalogStub(), wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 1)
		REDACTED,
	REDACTED,
		{
			name: "composite 分组按模型平台取渠道定价并叠加官方阶梯", model: "gpt-5.4", platform: PlatformOpenAI, groupPlatform: PlatformComposite,
			group: enabledGroup(PlatformComposite),
			channel: []ChannelModelPricing{{
				Platform: PlatformOpenAI, Models: []string{"gpt-5.4"REDACTED, BillingMode: BillingModeToken, InputPrice: p(1e-6),
	REDACTED
			wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 2)
				requirePrice(t, p(1e-6), s.Tiers[0].Input, "input")
				requirePrice(t, p(2e-6), s.Tiers[1].Input, "input")
		REDACTED,
	REDACTED,
		{
			name: "渠道显式 cache_write=0 保留为 0", model: "claude-sonnet-4", platform: PlatformAnthropic, groupPlatform: PlatformAnthropic,
			group: enabledGroup(PlatformAnthropic),
			channel: []ChannelModelPricing{{
				Platform: PlatformAnthropic, Models: []string{"claude-sonnet-4"REDACTED, BillingMode: BillingModeToken,
				InputPrice: p(2e-6), CacheWritePrice: p(0),
	REDACTED
			wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 1)
				require.NotNil(t, s.Tiers[0].CacheWrite)
				require.Zero(t, *s.Tiers[0].CacheWrite)
		REDACTED,
	REDACTED,
		{
			name: "gpt-5.6 缺 cache_write 时按策略补 1.25 倍并带阶梯", model: "gpt-5.6-sol", platform: PlatformOpenAI, groupPlatform: PlatformOpenAI,
			group: enabledGroup(PlatformOpenAI),
			catalog: newStubPricingServiceFromMap(map[string]*LiteLLMModelPricing{
				"gpt-5.6-sol": {Mode: "chat", InputCostPerToken: 5e-6, OutputCostPerToken: 30e-6, CacheReadInputTokenCost: 0.5e-6REDACTED,
		REDACTED),
			wantBasis: ContextPricingBasisWholeRequest,
			check: func(t *testing.T, s *ContextPricingSchedule) {
				require.Len(t, s.Tiers, 2)
				requireTier(t, s.Tiers[0], 0, intPtr(272000), "≤272K", p(5e-6), p(30e-6), p(6.25e-6), p(0.5e-6))
				requireTier(t, s.Tiers[1], 272000, nil, ">272K", p(10e-6), p(45e-6), p(12.5e-6), p(1e-6))
		REDACTED,
	REDACTED,
		{
			name: "图片模式返回 nil", model: "gpt-image-2", platform: PlatformOpenAI, groupPlatform: PlatformOpenAI,
			group: enabledGroup(PlatformOpenAI),
			channel: []ChannelModelPricing{{
				Platform: PlatformOpenAI, Models: []string{"gpt-image-2"REDACTED, BillingMode: BillingModeImage, PerRequestPrice: p(0.04),
	REDACTED
			wantNil: true,
	REDACTED,
		{
			name: "无任何定价来源报错", model: "unknown-model-xyz", platform: PlatformOpenAI, groupPlatform: PlatformOpenAI,
			group: enabledGroup(PlatformOpenAI), wantErr: true,
	REDACTED,
REDACTED
REDACTED

func newScheduleTestEnv(t *testing.T, sc scheduleScenario) (*BillingService, *ModelPricingResolver) {
REDACTED
	return newTokenCostTestEnv(t, sc.groupPlatform, sc.channel, sc.catalog)
REDACTED

func TestResolveContextPricingSchedule_Scenarios(t *testing.T) {
	for _, sc := range scheduleScenarios() {
		t.Run(sc.name, func(t *testing.T) {
			bs, resolver := newScheduleTestEnv(t, sc)
			sched, err := bs.ResolveContextPricingSchedule(context.Background(), resolver, ContextPricingScheduleInput{
				Model: sc.model, Group: sc.group, Platform: sc.platform,
		REDACTED)
			if sc.wantErr {
			REDACTED
				require.Nil(t, sched)
				return
		REDACTED
		REDACTED
			if sc.wantNil {
				require.Nil(t, sched)
				return
		REDACTED
			require.NotNil(t, sched)
			require.Equal(t, sc.wantBasis, sched.Basis)
			require.NotEmpty(t, sched.Tiers)
			require.Zero(t, sched.Tiers[0].MinTokens, "首档从 0 起")
			for i := 1; i < len(sched.Tiers); i++ {
				require.NotNil(t, sched.Tiers[i-1].MaxTokens)
				require.Equal(t, *sched.Tiers[i-1].MaxTokens, sched.Tiers[i].MinTokens, "档位连续")
		REDACTED
			if sc.check != nil {
				sc.check(t, sched)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestResolveContextPricingSchedule_NilResolver(t *testing.T) {
	bs := NewBillingService(&config.Config{REDACTED, nil)
	sched, err := bs.ResolveContextPricingSchedule(context.Background(), nil, ContextPricingScheduleInput{Model: "gpt-5.4"REDACTED)
REDACTED
	require.Nil(t, sched)
REDACTED

// --- 对账：阶梯表推算的费用必须等于真实计费函数 ---

type tokenKind int

const (
	kindInput tokenKind = iota
	kindOutput
	kindCacheRead
	kindCacheWrite
)

func tierPrice(tier ContextPricingTier, kind tokenKind) float64 {
	var p *float64
	switch kind {
	case kindInput:
		p = tier.Input
	case kindOutput:
		p = tier.Output
	case kindCacheRead:
		p = tier.CacheRead
	case kindCacheWrite:
		p = tier.CacheWrite
REDACTED
	if p == nil {
		return 0
REDACTED
	return *p
REDACTED

func tierAt(tiers []ContextPricingTier, contextTokens int) ContextPricingTier {
	for _, tier := range tiers {
		if contextTokens > tier.MinTokens && (tier.MaxTokens == nil || contextTokens <= *tier.MaxTokens) {
			return tier
	REDACTED
REDACTED
	return tiers[len(tiers)-1]
REDACTED

// expectedCostFromSchedule 按阶梯表推算 contextTokens 个某类 token 的费用：
// 整单基准取所在档单价 × 全量；边际基准逐段累加。
func expectedCostFromSchedule(s *ContextPricingSchedule, kind tokenKind, contextTokens int) float64 {
	if s.Basis == ContextPricingBasisMarginal {
		total := 0.0
		for _, tier := range s.Tiers {
			if contextTokens <= tier.MinTokens {
				break
		REDACTED
			upper := contextTokens
			if tier.MaxTokens != nil && *tier.MaxTokens < upper {
				upper = *tier.MaxTokens
		REDACTED
			total += float64(upper-tier.MinTokens) * tierPrice(tier, kind)
	REDACTED
		return total
REDACTED
	return float64(contextTokens) * tierPrice(tierAt(s.Tiers, contextTokens), kind)
REDACTED

func TestResolveContextPricingSchedule_ParityWithBilling(t *testing.T) {
	const outputProbe = 1000
	rng := rand.New(rand.NewSource(20260823))
	for _, sc := range scheduleScenarios() {
		if sc.wantErr || sc.wantNil {
			continue
	REDACTED
		t.Run(sc.name, func(t *testing.T) {
			bs, resolver := newScheduleTestEnv(t, sc)
			ctx := context.Background()
			if sc.platform != "" {
				ctx = WithResolvedTargetPlatform(ctx, sc.platform)
		REDACTED
			sched, err := bs.ResolveContextPricingSchedule(ctx, resolver, ContextPricingScheduleInput{
				Model: sc.model, Group: sc.group, Platform: sc.platform,
		REDACTED)
		REDACTED
			require.NotNil(t, sched)

			// 消费方视角重建同一份计费请求（与网关一致）。
			pricingInput := PricingInput{Model: sc.model, Group: sc.groupREDACTED
			if sc.group != nil {
				gid := sc.group.ID
				pricingInput.GroupID = &gid
		REDACTED
			resolved := resolver.Resolve(ctx, pricingInput)
			var legacy *LegacyLongContextRule
			if sc.group != nil {
				legacy = bs.LegacyLongContextRule(sc.platform)
		REDACTED
			if !legacyLongContextApplies(resolved, sc.group, legacy) {
				legacy = nil
		REDACTED
			cost := func(tokens UsageTokens) float64 {
				bd, err := bs.CalculateTokenCostForRequest(TokenCostRequest{
					Ctx: ctx, Model: sc.model, Group: sc.group, Tokens: tokens, RateMultiplier: 1,
					Resolver: resolver, Resolved: resolved, LegacyLongContext: legacy,
			REDACTED)
			REDACTED
				return bd.ActualCost
		REDACTED

			probes := []int{1, 2, 999, 1000, 1001, 10_000_000REDACTED
			for _, tier := range sched.Tiers {
				for _, b := range []*int{tier.MaxTokensREDACTED {
					if b == nil {
						continue
				REDACTED
					probes = append(probes, *b-1, *b, *b+1)
			REDACTED
		REDACTED
			for i := 0; i < 8; i++ {
				probes = append(probes, 1+rng.Intn(3_000_000))
		REDACTED

			for _, c := range probes {
				if c <= 0 {
					continue
			REDACTED
				assertCostClose(t, expectedCostFromSchedule(sched, kindInput, c), cost(UsageTokens{InputTokens: cREDACTED), "input@%d", c)
				assertCostClose(t, expectedCostFromSchedule(sched, kindCacheRead, c), cost(UsageTokens{CacheReadTokens: cREDACTED), "cache_read@%d", c)
				assertCostClose(t, expectedCostFromSchedule(sched, kindCacheWrite, c), cost(UsageTokens{CacheCreationTokens: cREDACTED), "cache_write@%d", c)
				wantOutput := float64(outputProbe) * tierPrice(tierAt(sched.Tiers, c), kindOutput)
				gotOutput := cost(UsageTokens{InputTokens: c, OutputTokens: outputProbeREDACTED) - cost(UsageTokens{InputTokens: cREDACTED)
				assertCostClose(t, wantOutput, gotOutput, "output@%d", c)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func assertCostClose(t *testing.T, want, got float64, format string, args ...any) {
REDACTED
	tolerance := 1e-12 + 1e-9*math.Abs(want)
	require.InDeltaf(t, want, got, tolerance, format, args...)
REDACTED
