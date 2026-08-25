package service

import (
	"context"
	"errors"
	"math"
	"sort"
	"strconv"
	"time"
)

// ContextPricingBasis 阶梯的计价基准。
type ContextPricingBasis string

const (
	// ContextPricingBasisWholeRequest 整单按所在档单价计价（目录阶梯、渠道区间）。
	ContextPricingBasisWholeRequest ContextPricingBasis = "whole_request"
	// ContextPricingBasisMarginal 仅超出阈值的部分按该档单价计价（平台旧规则）。
	ContextPricingBasisMarginal ContextPricingBasis = "marginal"
)

// ContextPricingTier (MinTokens, MaxTokens] 区间内的有效 per-token 单价（USD）。
// nil 表示该项无价/不计费；MaxTokens 为 nil 表示无上限。
type ContextPricingTier struct {
	MinTokens  int
	MaxTokens  *int
	Label      string
	Input      *float64
	Output     *float64
	CacheWrite *float64
	CacheRead  *float64
}

// TimePricingPeriod 分时倍率时段：配置时区当天 [StartTime, EndTime) 内整单费用乘 Multiplier。
type TimePricingPeriod struct {
	StartTime  string
	EndTime    string
	Multiplier float64
}

// TimePricingSchedule 分组+模型生效的分时倍率（仅列出倍率 ≠ 1 的时段，按开始时间升序）。
// WeekdaysOnly 为 true 时时段仅周一至周五生效，周末整天按标准价计费。
type TimePricingSchedule struct {
	Timezone     string
	WeekdaysOnly bool
	Periods      []TimePricingPeriod
}

// ContextPricingSchedule 分组+模型按上下文长度分档的有效单价表。
// 单价由真实计费函数探针得出，与扣费同源；单档表示无阶梯。
// Tiers 为标准时段单价；TimePricing 非 nil 时，落在时段内的请求整单再乘对应倍率。
type ContextPricingSchedule struct {
	Basis       ContextPricingBasis
	Tiers       []ContextPricingTier
	TimePricing *TimePricingSchedule
}

// ContextPricingScheduleInput 阶梯表查询输入。
type ContextPricingScheduleInput struct {
	Model string
	// Group 为 nil 表示查官方参考价：无分组、无渠道定价，也不套用平台旧规则。
	Group *Group
	// Platform 为请求的具体平台（composite 分组传模型所属平台），
	// 决定渠道定价查找与平台旧规则的适用。
	Platform string
}

var errContextPricingResolverRequired = errors.New("context pricing schedule: resolver is required")

// 探针步长：相邻两个探针点相差 contextProbeDelta 个 token，单价 = Δcost / Δtoken。
const contextProbeDelta = 1000

// ResolveContextPricingSchedule 解析分组+模型的上下文阶梯单价表。
//
// 解析链与扣费完全一致：Resolver.Resolve（分组卡 → 渠道 → 目录 → 策略）给出定价，
// CalculateTokenCostForRequest 给出路径（分组/渠道定价 → 平台旧规则 → 内置目录）。
// 断点只取自计费自身的规则输入（渠道区间边界、目录阶梯阈值、旧规则阈值），
// 每一段的单价由真实计费函数在该段内两点探针的差商得到，因此倍率、策略等
// 规则变更无需同步到这里；相邻同价段会合并。
//
// 非 token 计费模式返回 (nil, nil)；模型无任何定价来源时返回 ErrModelPricingUnavailable。
func (s *BillingService) ResolveContextPricingSchedule(ctx context.Context, resolver *ModelPricingResolver, in ContextPricingScheduleInput) (*ContextPricingSchedule, error) {
	if s == nil || resolver == nil {
		return nil, errContextPricingResolverRequired
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if in.Platform != "" {
		ctx = WithResolvedTargetPlatform(ctx, in.Platform)
	}

	pricingInput := PricingInput{Model: in.Model, Group: in.Group}
	if in.Group != nil {
		gid := in.Group.ID
		pricingInput.GroupID = &gid
	}
	resolved := resolver.Resolve(ctx, pricingInput)
	if resolved == nil {
		return nil, ErrModelPricingUnavailable
	}
	if resolved.Mode != "" && resolved.Mode != BillingModeToken {
		return nil, nil
	}

	var legacy *LegacyLongContextRule
	if in.Group != nil {
		legacy = s.LegacyLongContextRule(in.Platform)
	}
	if !legacyLongContextApplies(resolved, in.Group, legacy) {
		legacy = nil
	}

	req := TokenCostRequest{
		Ctx:               ctx,
		Model:             in.Model,
		Group:             in.Group,
		RateMultiplier:    1,
		Resolver:          resolver,
		Resolved:          resolved,
		LegacyLongContext: legacy,
	}
	probe := func(tokens UsageTokens) (*CostBreakdown, error) {
		r := req
		r.Tokens = tokens
		return s.CalculateTokenCostForRequest(r)
	}

	plan := s.contextPricingBreakpoints(resolver, resolved, in.Model, legacy)
	segments := buildContextSegments(plan.bounds)

	tiers := make([]ContextPricingTier, 0, len(segments))
	for _, seg := range segments {
		tier, err := probeContextTier(seg, resolved, probe)
		if err != nil {
			return nil, err
		}
		tiers = append(tiers, tier)
	}
	tiers = mergeEqualContextTiers(tiers)
	applyContextTierLabels(tiers, plan)

	basis := ContextPricingBasisWholeRequest
	if legacy != nil {
		basis = ContextPricingBasisMarginal
	}
	return &ContextPricingSchedule{Basis: basis, Tiers: tiers, TimePricing: resolvedTimePricingSchedule(resolved)}, nil
}

// resolvedTimePricingSchedule 列出计费会生效的分时倍率时段。
// 时段来自解析到的渠道定价配置，每个时段的倍率用计费自己的 resolvedChannelTimeMultiplier
// 在时段内取值：定价来源不是渠道（分组价卡覆盖）、配置非法等情况下计费按 1 计，
// 这里也就自然得到"无分时"。倍率为 1 的时段不列出。
func resolvedTimePricingSchedule(resolved *ResolvedPricing) *TimePricingSchedule {
	if resolved == nil || resolved.channelPricing == nil || resolved.channelPricing.TimePricing == nil {
		return nil
	}
	cfg := resolved.channelPricing.TimePricing
	location, err := loadChannelTimePricingLocation(cfg.Timezone)
	if err != nil {
		return nil
	}
	type probedPeriod struct {
		start  int
		period TimePricingPeriod
	}
	probed := make([]probedPeriod, 0, len(cfg.Periods))
	for _, period := range cfg.Periods {
		start, err := parseChannelTime(period.StartTime, false)
		if err != nil {
			continue
		}
		// 时段按每日循环，取该时段开始后 1 秒作为探针时刻。
		// 锚点日必须是工作日（2026-01-05 为周一）：weekdays_only 配置在周末恒为 1，
		// 锚点落在周末会把时段整组剔除。
		at := time.Date(2026, time.January, 5, 0, 0, start+1, 0, location)
		multiplier := resolvedChannelTimeMultiplier(resolved, at)
		if multiplier == 1 {
			continue
		}
		probed = append(probed, probedPeriod{start: start, period: TimePricingPeriod{
			StartTime:  period.StartTime,
			EndTime:    period.EndTime,
			Multiplier: multiplier,
		}})
	}
	if len(probed) == 0 {
		return nil
	}
	sort.SliceStable(probed, func(i, j int) bool { return probed[i].start < probed[j].start })
	out := &TimePricingSchedule{
		Timezone:     cfg.Timezone,
		WeekdaysOnly: cfg.WeekdaysOnly,
		Periods:      make([]TimePricingPeriod, 0, len(probed)),
	}
	for _, p := range probed {
		out.Periods = append(out.Periods, p.period)
	}
	return out
}

// contextBreakpointPlan 描述断点来源。
type contextBreakpointPlan struct {
	bounds []int
	// thresholdBound 为目录阶梯/旧规则的断点值（(0,b] / (b,∞)），0 表示无。
	thresholdBound int
	// thresholdInclusive 为真表示达到阈值即进入高档（断点 = 阈值-1）。
	thresholdInclusive bool
	threshold          int
}

// contextPricingBreakpoints 从计费自身的规则输入收集价格断点（不读取任何倍率）。
func (s *BillingService) contextPricingBreakpoints(resolver *ModelPricingResolver, resolved *ResolvedPricing, model string, legacy *LegacyLongContextRule) contextBreakpointPlan {
	plan := contextBreakpointPlan{}
	if legacy != nil {
		plan.bounds = []int{legacy.Threshold}
		plan.thresholdBound = legacy.Threshold
		plan.threshold = legacy.Threshold
		return plan
	}
	if !resolved.longContextPricingEnabled {
		return plan
	}
	if len(resolved.Intervals) > 0 {
		// 区间边界即断点；空洞段（含末档上限之外）由计费回落 base，探针会自然得到基础价。
		set := make(map[int]struct{}, len(resolved.Intervals)*2)
		for i := range resolved.Intervals {
			iv := &resolved.Intervals[i]
			if iv.MinTokens > 0 {
				set[iv.MinTokens] = struct{}{}
			}
			if iv.MaxTokens != nil {
				set[*iv.MaxTokens] = struct{}{}
			}
		}
		for b := range set {
			plan.bounds = append(plan.bounds, b)
		}
		sort.Ints(plan.bounds)
		return plan
	}
	pricing := resolver.GetIntervalPricing(resolved, 1)
	if pricing == nil {
		return plan
	}
	pricing = s.applyModelSpecificPricingPolicy(model, pricing)
	if pricing.LongContextInputThreshold <= 0 {
		return plan
	}
	bound := pricing.LongContextInputThreshold
	if pricing.LongContextThresholdInclusive {
		bound--
	}
	if bound <= 0 {
		return plan
	}
	plan.bounds = []int{bound}
	plan.thresholdBound = bound
	plan.threshold = pricing.LongContextInputThreshold
	plan.thresholdInclusive = pricing.LongContextThresholdInclusive
	return plan
}

// contextSegment 探针用的 (min, max] 段；max 为 nil 表示无上限。
type contextSegment struct {
	min int
	max *int
}

// buildContextSegments 把升序断点切成 (0,b1], (b1,b2], …, (bn,∞)；无断点时为单个开区间。
func buildContextSegments(bounds []int) []contextSegment {
	segments := make([]contextSegment, 0, len(bounds)+1)
	prev := 0
	for _, b := range bounds {
		if b <= prev {
			continue
		}
		upper := b
		segments = append(segments, contextSegment{min: prev, max: &upper})
		prev = b
	}
	segments = append(segments, contextSegment{min: prev})
	return segments
}

// probeContextTier 在段内两点探针，单价 = ΔActualCost / Δtoken（倍率固定为 1）。
// 每次只喂一种 token，ActualCost 即该项费用；旧边际规则的加倍只体现在 ActualCost
// 而不在分项费用里，因此统一读 ActualCost。整单阶梯与边际规则在同一段内都是
// 线性函数，差商同时适用，无需区分规则类型。
func probeContextTier(seg contextSegment, resolved *ResolvedPricing, probe func(UsageTokens) (*CostBreakdown, error)) (ContextPricingTier, error) {
	tier := ContextPricingTier{MinTokens: seg.min, MaxTokens: seg.max}
	c := seg.min + 1
	delta := contextProbeDelta
	if seg.max != nil {
		if width := *seg.max - seg.min; width-1 < delta {
			delta = width - 1
		}
	}

	var err error
	tier.Input, err = probeComponentPrice(func(n int) UsageTokens { return UsageTokens{InputTokens: n} }, c, delta, probe)
	if err != nil {
		return tier, err
	}
	tier.CacheRead, err = probeComponentPrice(func(n int) UsageTokens { return UsageTokens{CacheReadTokens: n} }, c, delta, probe)
	if err != nil {
		return tier, err
	}
	tier.CacheWrite, err = probeComponentPrice(func(n int) UsageTokens { return UsageTokens{CacheCreationTokens: n} }, c, delta, probe)
	if err != nil {
		return tier, err
	}
	// 输出价只随上下文所在档变化：固定上下文 c，对输出 token 数做差商（固定部分相减抵消）。
	tier.Output, err = probeComponentPrice(func(n int) UsageTokens { return UsageTokens{InputTokens: c, OutputTokens: n} }, 0, contextProbeDelta, probe)
	if err != nil {
		return tier, err
	}

	explicit := explicitContextPricingFields(resolved, c)
	tier.Input = contextPricePtr(tier.Input, explicit.input)
	tier.Output = contextPricePtr(tier.Output, explicit.output)
	tier.CacheWrite = contextPricePtr(tier.CacheWrite, explicit.cacheWrite)
	tier.CacheRead = contextPricePtr(tier.CacheRead, explicit.cacheRead)
	return tier, nil
}

// probeComponentPrice 返回 [from, from+delta] 上 ActualCost 的差商；delta 为 0（退化段）时退回平均单价。
func probeComponentPrice(tokensAt func(int) UsageTokens, from, delta int, probe func(UsageTokens) (*CostBreakdown, error)) (*float64, error) {
	if delta <= 0 {
		n := from
		if n <= 0 {
			n = 1
		}
		cost, err := probe(tokensAt(n))
		if err != nil {
			return nil, err
		}
		v := roundContextPrice(cost.ActualCost / float64(n))
		return &v, nil
	}
	lo, err := probe(tokensAt(from))
	if err != nil {
		return nil, err
	}
	hi, err := probe(tokensAt(from + delta))
	if err != nil {
		return nil, err
	}
	v := roundContextPrice((hi.ActualCost - lo.ActualCost) / float64(delta))
	return &v, nil
}

// roundContextPrice 去掉差商带来的浮点噪声（保留 12 位有效数字）。
func roundContextPrice(v float64) float64 {
	if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	r, err := strconv.ParseFloat(strconv.FormatFloat(v, 'g', 12, 64), 64)
	if err != nil {
		return v
	}
	return r
}

type explicitContextFields struct {
	input, output, cacheWrite, cacheRead bool
}

// explicitContextPricingFields 判断各项是否被分组卡/渠道定价（含命中区间）显式配置。
// 显式配置为 0 时计费按 $0 收，展示应为 $0 而非“无价”。
func explicitContextPricingFields(resolved *ResolvedPricing, contextTokens int) explicitContextFields {
	var out explicitContextFields
	if resolved == nil || resolved.channelPricing == nil {
		return out
	}
	cp := resolved.channelPricing
	out.input = cp.InputPrice != nil
	out.output = cp.OutputPrice != nil
	out.cacheWrite = cp.CacheWritePrice != nil
	out.cacheRead = cp.CacheReadPrice != nil
	if iv := FindMatchingInterval(resolved.Intervals, contextTokens); iv != nil {
		out.input = out.input || iv.InputPrice != nil
		out.output = out.output || iv.OutputPrice != nil
		out.cacheWrite = out.cacheWrite || iv.CacheWritePrice != nil
		out.cacheRead = out.cacheRead || iv.CacheReadPrice != nil
	}
	return out
}

func contextPricePtr(v *float64, explicit bool) *float64 {
	if v == nil {
		return nil
	}
	if *v == 0 && !explicit {
		return nil
	}
	return v
}

// mergeEqualContextTiers 合并相邻且四项单价相同的段（倍率 ≤1 的目录、关闭阶梯等场景塌成单档）。
func mergeEqualContextTiers(tiers []ContextPricingTier) []ContextPricingTier {
	if len(tiers) < 2 {
		return tiers
	}
	merged := make([]ContextPricingTier, 0, len(tiers))
	for _, t := range tiers {
		if n := len(merged); n > 0 && sameContextPrices(merged[n-1], t) {
			merged[n-1].MaxTokens = t.MaxTokens
			continue
		}
		merged = append(merged, t)
	}
	return merged
}

func sameContextPrices(a, b ContextPricingTier) bool {
	return samePricePtr(a.Input, b.Input) && samePricePtr(a.Output, b.Output) &&
		samePricePtr(a.CacheWrite, b.CacheWrite) && samePricePtr(a.CacheRead, b.CacheRead)
}

func samePricePtr(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if *a == *b {
		return true
	}
	scale := math.Max(math.Abs(*a), math.Abs(*b))
	return math.Abs(*a-*b) <= scale*1e-9
}

// applyContextTierLabels 给多档阶梯打统一形态的标签：有上限的档为「≤上限」，
// 末档为「>下限」；档位按上下文升序，因此相邻的 ≤100K / ≤200K 即表示 (100K,200K]。
// 目录阶梯/旧规则在"达到阈值即进高档"时改用 < / ≥ 表达阈值本身。
// 渠道区间上的 tier_label 不用于 token 档位（token 模式的管理表单不暴露该字段）。
func applyContextTierLabels(tiers []ContextPricingTier, plan contextBreakpointPlan) {
	if len(tiers) < 2 {
		return
	}
	for i := range tiers {
		t := &tiers[i]
		switch {
		case plan.thresholdInclusive && t.MaxTokens != nil && *t.MaxTokens == plan.thresholdBound:
			t.Label = "<" + formatContextTokenCount(plan.threshold)
		case plan.thresholdInclusive && t.MinTokens == plan.thresholdBound:
			t.Label = "≥" + formatContextTokenCount(plan.threshold)
		case t.MaxTokens != nil:
			t.Label = "≤" + formatContextTokenCount(*t.MaxTokens)
		default:
			t.Label = ">" + formatContextTokenCount(t.MinTokens)
		}
	}
}

// formatContextTokenCount 把 token 数格式化为 272K / 1M 等短标签。
func formatContextTokenCount(n int) string {
	switch {
	case n >= 1_000_000 && n%1_000_000 == 0:
		return strconv.Itoa(n/1_000_000) + "M"
	case n >= 1_000_000:
		return trimFloatLabel(float64(n)/1_000_000) + "M"
	case n >= 1_000:
		return trimFloatLabel(float64(n)/1_000) + "K"
	}
	return strconv.Itoa(n)
}

func trimFloatLabel(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}
