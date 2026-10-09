package service

import (
	"context"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/google/uuid"
)

// InflightBalanceReservationCache 余额在途预留的缓存能力（可选）。
// BillingCache 的 Redis 实现同时实现此接口；未实现时在途预留自动关闭（fail-open）。
type InflightBalanceReservationCache interface {
	// ReserveInflightBalance 原子地：清理过期预留；若用户已有在途预留且
	// balance - sum(在途) < amount 则拒绝；否则登记 requestID 的预留（ttl 后自动失效）。
	// 返回是否放行以及登记前的在途合计。
	ReserveInflightBalance(ctx context.Context, userID int64, requestID string, amount, balance float64, ttl time.Duration) (bool, float64, error)
	// ReleaseInflightBalance 释放 requestID 的预留（幂等）。
	ReleaseInflightBalance(ctx context.Context, userID int64, requestID string) error
}

// InflightBalanceReservationRenewer 可选：续期仍存活的预留（流式长请求期间防止 TTL 过期）。
type InflightBalanceReservationRenewer interface {
	// RenewInflightBalance 若 requestID 仍存在则把其过期时间推迟到 now+ttl；返回是否仍存在。
	RenewInflightBalance(ctx context.Context, userID int64, requestID string, ttl time.Duration) (bool, error)
}

const (
	defaultInflightReservationTTL      = 15 * time.Minute
	defaultInflightDefaultMaxTokens    = 8192
	inflightReservationReleaseTimeout  = 2 * time.Second
	inflightReservationReserveTimeout  = 2 * time.Second
	inflightReservationRenewTimeout    = 2 * time.Second
	inflightInputBytesPerTokenEstimate = 4
	inflightUnpricedLogInterval        = time.Minute
)

// InflightReservation 一次请求的在途预留句柄。
//
// 生命周期（引用计数，归零时释放一次）：
//   - 创建时持有 1 个「handler」引用，并启动续期协程（每 ttl/3 续期一次）。
//   - handler 提交计费任务时通过 Acquire 再取一个引用，由计费任务在余额缓存
//     实际扣减之后归还（任务被丢弃时由提交方立即归还）。
//   - handler 返回时调用 HandlerDone：停止续期并归还 handler 引用。
//
// 因此预留会一直保持到「handler 结束 且 所有计费任务已完成扣减」，
// 从而消除「handler 已返回、异步计费尚未落地」窗口内的透支。handler 结束后
// 不再续期，所以最长持有时间受 TTL 约束（计费任务卡死时预留自动过期）。
// 所有方法对 nil 接收者安全。
type InflightReservation struct {
	cache     InflightBalanceReservationCache
	userID    int64
	requestID string
	amount    float64
	ttl       time.Duration

	refs        atomic.Int64
	releaseOnce sync.Once
	stopOnce    sync.Once
	closeOnce   sync.Once
	stopRenew   chan struct{}
	renewDone   chan struct{}
}

// Amount 预留金额。
func (r *InflightReservation) Amount() float64 {
	if r == nil {
		return 0
	}
	return r.amount
}

// Acquire 为一个异步计费任务增加引用；返回的 done 幂等，必须在任务结束（或被丢弃）时调用。
// 预留已释放时返回 no-op。
func (r *InflightReservation) Acquire() func() {
	if r == nil {
		return noopRelease
	}
	for {
		cur := r.refs.Load()
		if cur <= 0 {
			return noopRelease
		}
		if r.refs.CompareAndSwap(cur, cur+1) {
			break
		}
	}
	var once sync.Once
	return func() { once.Do(r.decRef) }
}

// HandlerDone handler 结束：停止续期并归还 handler 引用（幂等）。
func (r *InflightReservation) HandlerDone() {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() {
		r.stopRenewal()
		r.decRef()
	})
}

// Release 立即释放预留（幂等），不论引用计数。
func (r *InflightReservation) Release() {
	if r == nil {
		return
	}
	r.stopRenewal()
	r.releaseOnce.Do(func() {
		r.refs.Store(0)
		relCtx, relCancel := context.WithTimeout(context.Background(), inflightReservationReleaseTimeout)
		defer relCancel()
		if err := r.cache.ReleaseInflightBalance(relCtx, r.userID, r.requestID); err != nil {
			logger.LegacyPrintf("service.billing_cache", "Warning: inflight reservation release failed for user %d (expires by ttl): %v", r.userID, err)
		}
	})
}

func (r *InflightReservation) decRef() {
	if r.refs.Add(-1) <= 0 {
		r.Release()
	}
}

func (r *InflightReservation) stopRenewal() {
	if r.stopRenew == nil {
		return
	}
	r.closeOnce.Do(func() { close(r.stopRenew) })
	<-r.renewDone
}

func (r *InflightReservation) startRenewal(renewer InflightBalanceReservationRenewer) {
	interval := r.ttl / 3
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	r.stopRenew = make(chan struct{})
	r.renewDone = make(chan struct{})
	go func() {
		defer close(r.renewDone)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-r.stopRenew:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), inflightReservationRenewTimeout)
				alive, err := renewer.RenewInflightBalance(ctx, r.userID, r.requestID, r.ttl)
				cancel()
				if err != nil {
					logger.LegacyPrintf("service.billing_cache", "Warning: inflight reservation renew failed for user %d: %v", r.userID, err)
					continue
				}
				if !alive {
					return
				}
			}
		}
	}()
}

type inflightReservationCtxKey struct{}

// WithInflightReservation 把预留句柄挂到 context 上，供计费任务提交时交接。
func WithInflightReservation(ctx context.Context, r *InflightReservation) context.Context {
	if r == nil {
		return ctx
	}
	return context.WithValue(ctx, inflightReservationCtxKey{}, r)
}

// InflightReservationFromContext 读取 context 中的预留句柄（可能为 nil）。
func InflightReservationFromContext(ctx context.Context) *InflightReservation {
	if ctx == nil {
		return nil
	}
	r, _ := ctx.Value(inflightReservationCtxKey{}).(*InflightReservation)
	return r
}

func noopRelease() {}

func (s *BillingCacheService) inflightReservationConfig() (config.InflightReservationConfig, bool) {
	if s == nil || s.cfg == nil {
		return config.InflightReservationConfig{}, false
	}
	cfg := s.cfg.Billing.InflightReservation
	if !cfg.Enabled || s.cfg.RunMode == config.RunModeSimple {
		return cfg, false
	}
	return cfg, true
}

// InflightReservationEnabled 是否启用余额在途预留。
func (s *BillingCacheService) InflightReservationEnabled() bool {
	_, ok := s.inflightReservationConfig()
	return ok
}

// InflightReservationFailClosedOnUnpriced 无法估算费用时是否拒绝请求（默认 false = fail-open）。
func (s *BillingCacheService) InflightReservationFailClosedOnUnpriced() bool {
	cfg, ok := s.inflightReservationConfig()
	return ok && cfg.FailClosedOnUnpriced
}

// ReserveInflightBalance 简化封装：返回释放函数，不续期、不做计费交接（预留最长存活 TTL）。
func (s *BillingCacheService) ReserveInflightBalance(ctx context.Context, user *User, group *Group, subscription *UserSubscription, estimate float64) (func(), error) {
	r, err := s.reserveInflight(ctx, user, group, subscription, estimate, false)
	if err != nil {
		return noopRelease, err
	}
	if r == nil {
		return noopRelease, nil
	}
	return r.Release, nil
}

// ReserveInflight 在余额模式下为本次请求登记在途预留。
//
// 必须在 CheckBillingEligibility 通过后调用。返回的句柄可能为 nil（未预留）；
// 调用方须在 handler 结束时调用 HandlerDone（nil 安全）。
//
// 以下情况直接放行且不登记预留（fail-open，保持旧行为）：
// 开关关闭 / 简易模式 / 订阅模式 / estimate <= 0 / 缓存不支持 / 余额读取失败 / Redis 执行失败。
// 仅当 Redis 明确判定 缓存余额 - 在途合计 < estimate（且已有在途请求）时返回 ErrInsufficientBalance。
func (s *BillingCacheService) ReserveInflight(ctx context.Context, user *User, group *Group, subscription *UserSubscription, estimate float64) (*InflightReservation, error) {
	return s.reserveInflight(ctx, user, group, subscription, estimate, true)
}

func (s *BillingCacheService) reserveInflight(ctx context.Context, user *User, group *Group, subscription *UserSubscription, estimate float64, renew bool) (*InflightReservation, error) {
	cfg, ok := s.inflightReservationConfig()
	if !ok || user == nil {
		return nil, nil
	}
	if group != nil && group.IsSubscriptionType() && subscription != nil {
		return nil, nil
	}
	if cfg.MaxReservationUSD > 0 && estimate > cfg.MaxReservationUSD {
		estimate = cfg.MaxReservationUSD
	}
	if estimate <= 0 || math.IsNaN(estimate) || math.IsInf(estimate, 0) {
		return nil, nil
	}
	rc, ok := s.cache.(InflightBalanceReservationCache)
	if !ok || rc == nil {
		return nil, nil
	}

	balance, err := s.GetUserBalance(ctx, user.ID)
	if err != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: inflight reservation balance read failed for user %d (fail-open): %v", user.ID, err)
		return nil, nil
	}

	ttl := defaultInflightReservationTTL
	if cfg.TTLSeconds > 0 {
		ttl = time.Duration(cfg.TTLSeconds) * time.Second
	}
	requestID := uuid.NewString()
	reserveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), inflightReservationReserveTimeout)
	allowed, inflight, err := rc.ReserveInflightBalance(reserveCtx, user.ID, requestID, estimate, balance, ttl)
	cancel()
	if err != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: inflight reservation failed for user %d (fail-open): %v", user.ID, err)
		return nil, nil
	}
	if !allowed {
		logger.LegacyPrintf("service.billing_cache", "inflight reservation rejected: user=%d balance=%.6f inflight=%.6f estimate=%.6f", user.ID, balance, inflight, estimate)
		return nil, ErrInsufficientBalance
	}

	r := &InflightReservation{cache: rc, userID: user.ID, requestID: requestID, amount: estimate, ttl: ttl}
	r.refs.Store(1)
	if renewer, ok := s.cache.(InflightBalanceReservationRenewer); renew && ok && renewer != nil {
		r.startRenewal(renewer)
	}
	return r, nil
}

// ============================================
// 费用估算
// ============================================

// InflightEstimateKind 估算口径。
type InflightEstimateKind int

const (
	// InflightEstimateToken 文本/对话类：输入估算 + 输出上限（按次渠道定价时按次计）。
	InflightEstimateToken InflightEstimateKind = iota
	// InflightEstimateImage 图片生成：按张计（取所有尺寸档最高单价）。
	InflightEstimateImage
	// InflightEstimatePerRequest 按次类（独立搜索等）：仅按渠道/分组按次价与搜索附加费，不做 token 估算。
	InflightEstimatePerRequest
	// InflightEstimateVideo 视频生成：按秒 × 条数（分组/模型视频价）。
	InflightEstimateVideo
	// InflightEstimateAudio 语音（tts/stt/realtime）：按 AudioMode/AudioUnits 计。
	InflightEstimateAudio
)

// InflightEstimateRequest 单请求估算输入。
type InflightEstimateRequest struct {
	Model     string
	BodyBytes int
	MaxTokens int
	Kind      InflightEstimateKind
	// Units 按次/按张数量（<=0 视为 1）。
	Units int
	// SearchCalls 叠加的搜索次数（按分组 search_price_per_1k 计）。
	SearchCalls int
	// 视频：分辨率与时长（秒）。
	VideoResolution      string
	VideoDurationSeconds int
	// 音频：模式（tts/stt/realtime）与计量单位（百万字符/小时/分钟）。
	AudioMode  string
	AudioUnits float64
}

// inflightEstimateDeps 两种网关 service 共用的估算依赖。
type inflightEstimateDeps struct {
	cfg            *config.Config
	billing        *BillingService
	resolver       *ModelPricingResolver
	resolveMapping func(ctx context.Context, groupID int64, model string) ChannelMappingResult
	userGroupRate  func(ctx context.Context, userID, groupID int64, groupDefault float64) float64
	// accountMappedModels 返回调度器可选账号对 model 的账号级映射结果（去重，不含 model 本身）。
	// 准入时账号尚未选定，计费侧 billableModelWithFallback 会回退到实际转发模型（UpstreamModel，
	// 即账号映射后的模型），因此这里取所有候选映射模型的最高估算。
	// 仅在首选/渠道候选均无法定价（或 composite 分组）时才调用；实现只读调度器快照，
	// 不在请求路径上直接查库，也不按模型名缓存（内存不随请求模型名增长）。
	accountMappedModels func(ctx context.Context, apiKey *APIKey, model string) []string
}

// inflightSnapshotLister 调度器快照读取（与账号选择同源：Redis 快照，未命中时由快照服务自身回源并回填）。
type inflightSnapshotLister func(ctx context.Context, groupID *int64, platform string, hasForcePlatform bool) ([]Account, error)

// inflightAccountMappedModelsFromSnapshot 基于调度器快照在内存中匹配账号级映射（支持通配规则）。
// resolvePlatform 返回与调度器一致的平台；无分组 API Key 使用调度器的未分组账号池（groupID=nil）。
func inflightAccountMappedModelsFromSnapshot(list inflightSnapshotLister, resolvePlatform func(ctx context.Context, apiKey *APIKey, model string) (string, bool, bool)) func(ctx context.Context, apiKey *APIKey, model string) []string {
	if list == nil || resolvePlatform == nil {
		return nil
	}
	return func(ctx context.Context, apiKey *APIKey, model string) []string {
		model = strings.TrimSpace(model)
		if apiKey == nil || model == "" {
			return nil
		}
		platform, forced, ok := resolvePlatform(ctx, apiKey, model)
		if !ok || platform == "" {
			return nil
		}
		accounts, err := list(ctx, apiKey.GroupID, platform, forced)
		if err != nil {
			return nil
		}
		return accountMappedModelsFrom(accounts, model)
	}
}

func accountMappedModelsFrom(accounts []Account, model string) []string {
	var seen map[string]struct{}
	var out []string
	for i := range accounts {
		mapped, matched := accounts[i].ResolveMappedModel(model)
		mapped = strings.TrimSpace(mapped)
		if !matched || mapped == "" || mapped == model {
			continue
		}
		if seen == nil {
			seen = map[string]struct{}{}
		}
		if _, dup := seen[mapped]; dup {
			continue
		}
		seen[mapped] = struct{}{}
		out = append(out, mapped)
	}
	return out
}

var inflightUnpricedLastLog atomic.Int64

func logInflightUnpriced(model string, groupID *int64) {
	now := time.Now().UnixNano()
	last := inflightUnpricedLastLog.Load()
	if now-last < int64(inflightUnpricedLogInterval) || !inflightUnpricedLastLog.CompareAndSwap(last, now) {
		return
	}
	var gid int64
	if groupID != nil {
		gid = *groupID
	}
	logger.LegacyPrintf("service.billing_cache", "Warning: inflight reservation cannot price model=%q group=%d; request admitted without reservation (fail-open, throttled log)", model, gid)
}

// inflightBillingModelCandidates 与计费路径一致地挑选计费模型：
// 返回 primary（计费侧首选的计费模型）与 fallbacks（计费侧 billableModelWithFallback
// 在首选模型查无价时回退到的实际转发模型：渠道映射模型 → 账号级映射模型）。
//   - channel_mapped（默认）→ 映射后模型（同时估算请求模型，取较高者）；
//   - requested → 请求模型；
//   - upstream / response_model 在准入时未知 → 取请求模型与映射模型两者较高估算。
func inflightBillingModelCandidates(ctx context.Context, deps inflightEstimateDeps, apiKey *APIKey, model string) (primary, fallbacks []string, upstreamInput string) {
	upstreamInput = model
	primary = []string{model}
	if apiKey == nil || apiKey.GroupID == nil {
		return primary, nil, upstreamInput
	}
	if deps.resolveMapping != nil {
		m := deps.resolveMapping(ctx, *apiKey.GroupID, model)
		if mapped := m.MappedModel; mapped != "" && mapped != model {
			upstreamInput = mapped
			if m.BillingModelSource == BillingModelSourceRequested {
				fallbacks = append(fallbacks, mapped)
			} else {
				primary = []string{mapped, model}
			}
		}
	}
	return primary, fallbacks, upstreamInput
}

func (d inflightEstimateDeps) rates(ctx context.Context, apiKey *APIKey) (text, image float64) {
	rate := 1.0
	if d.cfg != nil && d.cfg.Default.RateMultiplier > 0 {
		rate = d.cfg.Default.RateMultiplier
	}
	if apiKey != nil && apiKey.GroupID != nil && apiKey.Group != nil {
		rate = apiKey.Group.RateMultiplier
		if d.userGroupRate != nil && apiKey.User != nil {
			rate = d.userGroupRate(ctx, apiKey.User.ID, *apiKey.GroupID, rate)
		}
	}
	return computePeakAwareMultipliers(apiKey, rate, timezone.Now())
}

func tokenCounts(cfg config.InflightReservationConfig, bodyBytes, maxTokens int) (int, int) {
	inputTokens := 0
	if bodyBytes > 0 {
		inputTokens = bodyBytes / inflightInputBytesPerTokenEstimate
	}
	if cfg.MaxInputTokens > 0 && inputTokens > cfg.MaxInputTokens {
		inputTokens = cfg.MaxInputTokens
	}
	outputTokens := maxTokens
	if outputTokens <= 0 {
		outputTokens = cfg.DefaultMaxTokens
		if outputTokens <= 0 {
			outputTokens = defaultInflightDefaultMaxTokens
		}
	}
	if cfg.MaxOutputTokens > 0 && outputTokens > cfg.MaxOutputTokens {
		outputTokens = cfg.MaxOutputTokens
	}
	return inputTokens, outputTokens
}

func maxPerRequestPrice(resolved *ResolvedPricing) float64 {
	if resolved == nil {
		return 0
	}
	p := resolved.DefaultPerRequestPrice
	for _, tier := range resolved.RequestTiers {
		if tier.PerRequestPrice != nil && *tier.PerRequestPrice > p {
			p = *tier.PerRequestPrice
		}
	}
	return p
}

func validCost(v float64) bool { return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }

// estimateOne 估算单个候选计费模型（未乘倍率的 token 部分与按次部分分开返回，便于套用不同倍率）。
func (d inflightEstimateDeps) estimateOne(ctx context.Context, apiKey *APIKey, model string, req InflightEstimateRequest, textRate, imageRate float64) float64 {
	cfg := inflightReservationCfg(d.cfg)
	units := req.Units
	if units <= 0 {
		units = 1
	}
	var resolved *ResolvedPricing
	if d.resolver != nil {
		in := PricingInput{Model: model}
		if apiKey != nil {
			in.GroupID = apiKey.GroupID
			in.Group = apiKey.Group
		}
		resolved = d.resolver.Resolve(ctx, in)
	}

	inputTokens, outputTokens := tokenCounts(cfg, req.BodyBytes, req.MaxTokens)
	tokenCost := func() float64 {
		var pricing *ModelPricing
		if resolved != nil && (resolved.Mode == BillingModeToken || resolved.Mode == "") && d.resolver != nil {
			pricing = d.resolver.GetIntervalPricing(resolved, inputTokens)
		}
		if pricing == nil && d.billing != nil {
			pricing, _ = d.billing.GetModelPricing(model)
		}
		if pricing == nil {
			return 0
		}
		return (float64(inputTokens)*pricing.InputPricePerToken + float64(outputTokens)*pricing.OutputPricePerToken) * textRate
	}

	var cost float64
	perRequestMode := resolved != nil && (resolved.Mode == BillingModePerRequest || resolved.Mode == BillingModeImage || resolved.Mode == BillingModeVideo)
	switch req.Kind {
	case InflightEstimateImage:
		if perRequestMode {
			cost = maxPerRequestPrice(resolved) * float64(units) * imageRate
		}
		if d.billing != nil {
			cfgImg := imagePriceConfigFromAPIKey(apiKey)
			for _, tier := range []string{ImageBillingSize1K, ImageBillingSize2K, ImageBillingSize4K} {
				if b := d.billing.CalculateImageCost(model, tier, units, cfgImg, imageRate); b != nil && b.ActualCost > cost {
					cost = b.ActualCost
				}
			}
		}
		if cost <= 0 {
			cost = tokenCost()
		}
	case InflightEstimateVideo:
		if perRequestMode {
			cost = maxPerRequestPrice(resolved) * float64(units) * math.Max(textRate, imageRate)
		}
		if d.billing != nil {
			if b := d.billing.CalculateVideoCost(model, req.VideoResolution, units, req.VideoDurationSeconds, videoPriceConfigFromAPIKey(apiKey), math.Max(textRate, imageRate)); b != nil && b.ActualCost > cost {
				cost = b.ActualCost
			}
		}
	case InflightEstimateAudio:
		if perRequestMode && resolved.Mode == BillingModePerRequest {
			u := req.AudioUnits
			if u <= 0 {
				u = 1
			}
			cost = maxPerRequestPrice(resolved) * u * textRate
		}
		if d.billing != nil && req.AudioUnits > 0 {
			if b := d.billing.CalculateAudioCost(req.AudioMode, req.AudioUnits, groupAudioPriceConfigFromAPIKey(apiKey), textRate); b != nil && b.ActualCost > cost {
				cost = b.ActualCost
			}
		}
	default:
		if perRequestMode {
			rate := textRate
			if resolved.Mode == BillingModeImage {
				rate = imageRate
			}
			cost = maxPerRequestPrice(resolved) * float64(units) * rate
		} else if req.Kind != InflightEstimatePerRequest {
			cost = tokenCost()
		}
	}
	if req.SearchCalls > 0 {
		if d.billing != nil {
			if b := d.billing.CalculateSearchCost(req.SearchCalls, groupSearchPricePer1kFromAPIKey(apiKey), textRate); b != nil && b.ActualCost > 0 {
				cost += b.ActualCost
			}
		}
	}
	if !validCost(cost) {
		return 0
	}
	return cost
}

// estimate 返回保守的单请求费用（USD，已乘倍率）；无法定价返回 (0,false)。
func (d inflightEstimateDeps) estimate(ctx context.Context, apiKey *APIKey, req InflightEstimateRequest) (float64, bool) {
	if apiKey == nil || apiKey.User == nil {
		return 0, false
	}
	if req.Model == "" || (req.Kind == InflightEstimateAudio && req.AudioUnits <= 0) {
		// 非计量请求（媒体状态查询、custom-voices 等）：无需预留，也不算「无法定价」。
		return 0, true
	}
	textRate, imageRate := d.rates(ctx, apiKey)
	if textRate <= 0 && imageRate <= 0 {
		// 免费分组：不计费，也无需预留。
		return 0, true
	}
	primary, fallbacks, upstreamInput := inflightBillingModelCandidates(ctx, d, apiKey, req.Model)
	bestOf := func(models []string) float64 {
		best := 0.0
		for _, m := range models {
			if c := d.estimateOne(ctx, apiKey, m, req, textRate, imageRate); c > best {
				best = c
			}
		}
		return best
	}
	best := bestOf(primary)
	// composite 分组：计费侧除非别名有显式渠道价，否则按实际转发的具体模型计费；
	// 别名本身可能命中家族模糊价（低估），因此与候选具体模型一起取最高。
	composite := apiKey.Group != nil && apiKey.Group.Platform == PlatformComposite
	if best <= 0 || composite {
		// 与 billableModelWithFallback 同口径：首选模型无价时回退到实际转发模型。
		if c := bestOf(fallbacks); c > best {
			best = c
		}
		// 账号级映射候选（读调度器快照）仅在仍无法定价或 composite 时才查，已定价模型不触发。
		if (best <= 0 || composite) && d.accountMappedModels != nil {
			if c := bestOf(d.accountMappedModels(ctx, apiKey, upstreamInput)); c > best {
				best = c
			}
		}
	}
	if best <= 0 {
		logInflightUnpriced(req.Model, apiKey.GroupID)
		return 0, false
	}
	return best, true
}

// EstimateInflightReservationCost 仅用基础定价的简化估算（保留给无 resolver 的调用方/测试）。
//
//	input_tokens  = min(bodyBytes / 4, max_input_tokens)
//	output_tokens = min(max_tokens 或 default_max_tokens, max_output_tokens)
//	cost = (input_tokens × 输入单价 + output_tokens × 输出单价) × rateMultiplier
//
// 无法取得定价时返回 (0, false)，调用方应 fail-open。
func EstimateInflightReservationCost(billing *BillingService, cfg config.InflightReservationConfig, model string, bodyBytes, maxTokens int, rateMultiplier float64) (float64, bool) {
	if billing == nil || model == "" || rateMultiplier <= 0 {
		return 0, false
	}
	pricing, err := billing.GetModelPricing(model)
	if err != nil || pricing == nil {
		return 0, false
	}
	inputTokens, outputTokens := tokenCounts(cfg, bodyBytes, maxTokens)
	cost := (float64(inputTokens)*pricing.InputPricePerToken + float64(outputTokens)*pricing.OutputPricePerToken) * rateMultiplier
	if !validCost(cost) {
		return 0, false
	}
	return cost, true
}

func inflightReservationCfg(cfg *config.Config) config.InflightReservationConfig {
	if cfg == nil {
		return config.InflightReservationConfig{}
	}
	return cfg.Billing.InflightReservation
}

func (s *GatewayService) inflightEstimateDeps() inflightEstimateDeps {
	d := inflightEstimateDeps{cfg: s.cfg, billing: s.billingService, resolver: s.resolver}
	if s.channelService != nil {
		d.resolveMapping = s.channelService.ResolveChannelMapping
	}
	d.userGroupRate = s.getUserGroupRateMultiplier
	if s.schedulerSnapshot != nil {
		snap := s.schedulerSnapshot
		d.accountMappedModels = inflightAccountMappedModelsFromSnapshot(
			func(ctx context.Context, groupID *int64, platform string, forced bool) ([]Account, error) {
				accounts, _, err := snap.ListSchedulableAccounts(ctx, groupID, platform, forced)
				return accounts, err
			},
			func(ctx context.Context, apiKey *APIKey, model string) (string, bool, bool) {
				platform, forced, err := s.resolvePlatform(ctx, apiKey.GroupID, apiKey.Group, model)
				return platform, forced, err == nil
			},
		)
	}
	return d
}

// EstimateInflightReservation 与计费路径同口径（计费模型 / ModelPricingResolver / 倍率）估算在途预留金额。
// 第二个返回值为 false 表示无法定价（调用方 fail-open 或按配置 fail-closed）。
func (s *GatewayService) EstimateInflightReservation(ctx context.Context, apiKey *APIKey, req InflightEstimateRequest) (float64, bool) {
	if s == nil {
		return 0, false
	}
	return s.inflightEstimateDeps().estimate(ctx, apiKey, req)
}

func (s *OpenAIGatewayService) inflightEstimateDeps() inflightEstimateDeps {
	d := inflightEstimateDeps{cfg: s.cfg, billing: s.billingService, resolver: s.resolver}
	if s.channelService != nil {
		d.resolveMapping = s.channelService.ResolveChannelMapping
	}
	d.userGroupRate = s.ResolveUserGroupRateMultiplier
	if s.schedulerSnapshot != nil {
		snap := s.schedulerSnapshot
		d.accountMappedModels = inflightAccountMappedModelsFromSnapshot(
			func(ctx context.Context, groupID *int64, platform string, forced bool) ([]Account, error) {
				accounts, _, err := snap.ListSchedulableAccounts(ctx, groupID, platform, forced)
				return accounts, err
			},
			func(ctx context.Context, apiKey *APIKey, model string) (string, bool, bool) {
				platform := PlatformOpenAI
				if apiKey.Group != nil && apiKey.Group.Platform != "" && apiKey.Group.Platform != PlatformComposite {
					platform = apiKey.Group.Platform
				}
				return NormalizeOpenAICompatiblePlatform(platform), false, true
			},
		)
	}
	return d
}

// EstimateInflightReservation 同 GatewayService.EstimateInflightReservation（OpenAI 网关倍率口径）。
func (s *OpenAIGatewayService) EstimateInflightReservation(ctx context.Context, apiKey *APIKey, req InflightEstimateRequest) (float64, bool) {
	if s == nil {
		return 0, false
	}
	return s.inflightEstimateDeps().estimate(ctx, apiKey, req)
}
