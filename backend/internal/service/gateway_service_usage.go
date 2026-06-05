package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// RecordUsageInput 记录使用量的输入参数
type RecordUsageInput struct {
	Result              *ForwardResult
	ParsedRequest       *ParsedRequest
	APIKey              *APIKey
	User                *User
	Account             *Account
	Subscription        *UserSubscription  // 可选：订阅信息
	InboundEndpoint     string             // 入站端点（客户端请求路径）
	UpstreamEndpoint    string             // 上游端点（标准化后的上游路径）
	UserAgent           string             // 请求的 User-Agent
	IPAddress           string             // 请求的客户端 IP 地址
	RequestPayloadHash  string             // 请求体语义哈希，用于降低 request_id 误复用时的静默误去重风险
	ForceCacheBilling   bool               // 强制缓存计费：将 input_tokens 转为 cache_read 计费（用于粘性会话切换）
	APIKeyService       APIKeyQuotaUpdater // 可选：用于更新API Key配额
	ServiceQuotaRequest ServiceQuotaCheckRequest

	ChannelUsageFields // 渠道映射信息（由 handler 在 Forward 前解析）
}

// RecordUsageLongContextInput 记录使用量的输入参数（支持长上下文双倍计费）
type RecordUsageLongContextInput struct {
	Result                *ForwardResult
	APIKey                *APIKey
	User                  *User
	Account               *Account
	Subscription          *UserSubscription  // 可选：订阅信息
	InboundEndpoint       string             // 入站端点（客户端请求路径）
	UpstreamEndpoint      string             // 上游端点（标准化后的上游路径）
	UserAgent             string             // 请求的 User-Agent
	IPAddress             string             // 请求的客户端 IP 地址
	RequestPayloadHash    string             // 请求体语义哈希，用于降低 request_id 误复用时的静默误去重风险
	LongContextThreshold  int                // 长上下文阈值（如 200000）
	LongContextMultiplier float64            // 超出阈值部分的倍率（如 2.0）
	ForceCacheBilling     bool               // 强制缓存计费：将 input_tokens 转为 cache_read 计费（用于粘性会话切换）
	APIKeyService         APIKeyQuotaUpdater // API Key 配额服务（可选）

	ChannelUsageFields // 渠道映射信息（由 handler 在 Forward 前解析）
}

// recordUsageCoreInput 是 recordUsageCore 的公共输入字段，从两种输入结构体中提取。
type recordUsageCoreInput struct {
	Result             *ForwardResult
	APIKey             *APIKey
	User               *User
	Account            *Account
	Subscription       *UserSubscription
	InboundEndpoint    string
	UpstreamEndpoint   string
	UserAgent          string
	IPAddress          string
	RequestPayloadHash string
	ForceCacheBilling  bool
	APIKeyService      APIKeyQuotaUpdater
	ChannelUsageFields
	ServiceQuotaRequest ServiceQuotaCheckRequest
}

// recordUsageOpts 内部选项，参数化 RecordUsage 与 RecordUsageWithLongContext 的差异点。
type recordUsageOpts struct {
	// Claude Max 策略所需的 ParsedRequest（可选，仅 Claude 路径传入）
	ParsedRequest *ParsedRequest

	// EnableClaudePath 启用 Claude 路径特有逻辑：
	// - Claude Max 缓存计费策略
	EnableClaudePath bool

	// 长上下文计费（仅 Gemini 路径需要）
	LongContextThreshold  int
	LongContextMultiplier float64
}

// RecordUsage 记录使用量并扣费（或更新订阅用量）
func (s *GatewayService) RecordUsage(ctx context.Context, input *RecordUsageInput) error {
	return s.recordUsageCore(ctx, &recordUsageCoreInput{
		Result:              input.Result,
		APIKey:              input.APIKey,
		User:                input.User,
		Account:             input.Account,
		Subscription:        input.Subscription,
		InboundEndpoint:     input.InboundEndpoint,
		UpstreamEndpoint:    input.UpstreamEndpoint,
		UserAgent:           input.UserAgent,
		IPAddress:           input.IPAddress,
		RequestPayloadHash:  input.RequestPayloadHash,
		ForceCacheBilling:   input.ForceCacheBilling,
		APIKeyService:       input.APIKeyService,
		ServiceQuotaRequest: input.ServiceQuotaRequest,
		ChannelUsageFields:  input.ChannelUsageFields,
	}, &recordUsageOpts{
		EnableClaudePath: true,
	})
}

// RecordUsageWithLongContext 记录使用量并扣费，支持长上下文双倍计费（用于 Gemini）
func (s *GatewayService) RecordUsageWithLongContext(ctx context.Context, input *RecordUsageLongContextInput) error {
	return s.recordUsageCore(ctx, &recordUsageCoreInput{
		Result:             input.Result,
		APIKey:             input.APIKey,
		User:               input.User,
		Account:            input.Account,
		Subscription:       input.Subscription,
		InboundEndpoint:    input.InboundEndpoint,
		UpstreamEndpoint:   input.UpstreamEndpoint,
		UserAgent:          input.UserAgent,
		IPAddress:          input.IPAddress,
		RequestPayloadHash: input.RequestPayloadHash,
		ForceCacheBilling:  input.ForceCacheBilling,
		APIKeyService:      input.APIKeyService,
		ChannelUsageFields: input.ChannelUsageFields,
	}, &recordUsageOpts{
		LongContextThreshold:  input.LongContextThreshold,
		LongContextMultiplier: input.LongContextMultiplier,
	})
}

func (s *GatewayService) billingDeps() *billingDeps {
	return &billingDeps{
		accountRepo:          s.accountRepo,
		userRepo:             s.userRepo,
		userSubRepo:          s.userSubRepo,
		billingCacheService:  s.billingCacheService,
		deferredService:      s.deferredService,
		balanceNotifyService: s.balanceNotifyService,
	}
}

// SetAccountStatsResolver wires an optional plugin-supplied account-stats
// cost resolver. Passing nil disables the hook so the host falls back to
// the default formula (total_cost × account_rate_multiplier).
//
// Safe for concurrent calls: the resolver is read under a RW mutex on
// every recordUsage path so swapping it out (plugin restart, disable)
// does not race with in-flight requests.
func (s *GatewayService) SetAccountStatsResolver(resolver AccountStatsCostResolver) {
	if s == nil {
		return
	}
	s.accountStatsMu.Lock()
	defer s.accountStatsMu.Unlock()
	s.accountStatsResolver = resolver
}

// loadAccountStatsResolver returns the currently registered resolver
// (possibly nil). Hot path read; the lock cost is negligible vs the
// gRPC RPC the resolver itself makes.
func (s *GatewayService) loadAccountStatsResolver() AccountStatsCostResolver {
	if s == nil {
		return nil
	}
	s.accountStatsMu.RLock()
	defer s.accountStatsMu.RUnlock()
	return s.accountStatsResolver
}

// resolveAccountStatsCost calls the registered AccountStatsCostResolver
// (typically the channel-management plugin) to get the per-request
// account-stats cost override. nil resolver, error, or HasCost=false →
// returns nil so the caller leaves usage_logs.account_stats_cost NULL
// and aggregation queries fall back to total_cost via COALESCE.
//
// The resolver is given the customer total cost (before multiplier) so
// it can implement "use customer cost" priority levels without re-doing
// the gateway's pricing math.
func (s *GatewayService) resolveAccountStatsCost(
	ctx context.Context,
	requestID string,
	channelID, accountID, groupID int64,
	upstreamModel string,
	usage ClaudeUsage,
	requestCount int,
	totalCost float64,
) *float64 {
	resolver := s.loadAccountStatsResolver()
	if resolver == nil {
		return nil
	}
	if upstreamModel == "" {
		return nil
	}
	tokens := AccountStatsCostTokens{
		InputTokens:         int64(usage.InputTokens),
		OutputTokens:        int64(usage.OutputTokens),
		CacheCreationTokens: int64(usage.CacheCreationInputTokens),
		CacheReadTokens:     int64(usage.CacheReadInputTokens),
		ImageOutputTokens:   int64(usage.ImageOutputTokens),
	}
	if requestCount <= 0 {
		requestCount = 1
	}
	res, err := resolver.ResolveAccountStatsCost(ctx, AccountStatsCostInput{
		RequestID:     requestID,
		ChannelID:     channelID,
		AccountID:     accountID,
		GroupID:       groupID,
		UpstreamModel: upstreamModel,
		Tokens:        tokens,
		RequestCount:  requestCount,
		TotalCost:     totalCost,
	})
	if err != nil {
		logger.LegacyPrintf("service.gateway",
			"resolve_account_stats_cost: plugin error (channel=%d account=%d): %v",
			channelID, accountID, err)
		return nil
	}
	if !res.HasCost {
		return nil
	}
	cost := res.Cost
	return &cost
}

// recordUsageCore 是 RecordUsage 和 RecordUsageWithLongContext 的统一实现。
// opts 中的字段控制两者之间的差异行为：
// - ParsedRequest != nil → 启用 Claude Max 缓存计费策略
// - LongContextThreshold > 0 → Token 计费回退走 CalculateCostWithLongContext
func (s *GatewayService) recordUsageCore(ctx context.Context, input *recordUsageCoreInput, opts *recordUsageOpts) error {
	result := input.Result
	apiKey := input.APIKey
	user := input.User
	account := input.Account
	subscription := input.Subscription

	// 强制缓存计费：将 input_tokens 转为 cache_read_input_tokens
	// 用于粘性会话切换时的特殊计费处理
	if input.ForceCacheBilling && result.Usage.InputTokens > 0 {
		logger.LegacyPrintf("service.gateway", "force_cache_billing: %d input_tokens → cache_read_input_tokens (account=%d)",
			result.Usage.InputTokens, account.ID)
		result.Usage.CacheReadInputTokens += result.Usage.InputTokens
		result.Usage.InputTokens = 0
	}

	// Cache TTL Override: 确保计费时 token 分类与账号设置一致。
	// 账号级设置优先；全局 1h 请求注入开启时，默认把 usage 计费归回 5m。
	cacheTTLOverridden := false
	if overrideTarget, ok := s.resolveCacheTTLUsageOverrideTarget(ctx, account); ok {
		applyCacheTTLOverride(&result.Usage, overrideTarget)
		cacheTTLOverridden = (result.Usage.CacheCreation5mTokens + result.Usage.CacheCreation1hTokens) > 0
	}

	// 获取费率倍数（优先级：用户专属 > 分组默认 > 系统默认）
	multiplier := 1.0
	if s.cfg != nil {
		multiplier = s.cfg.Default.RateMultiplier
	}
	if apiKey.GroupID != nil && apiKey.Group != nil {
		groupDefault := apiKey.Group.RateMultiplier
		multiplier = s.getUserGroupRateMultiplier(ctx, user.ID, *apiKey.GroupID, groupDefault)
	}
	imageMultiplier := resolveImageRateMultiplier(apiKey, multiplier)

	// 确定计费模型
	billingModel := forwardResultBillingModel(result.Model, result.UpstreamModel)
	if input.BillingModelSource == BillingModelSourceChannelMapped && input.ChannelMappedModel != "" {
		billingModel = input.ChannelMappedModel
	}
	if input.BillingModelSource == BillingModelSourceRequested && input.OriginalModel != "" {
		billingModel = input.OriginalModel
	}

	// 确定 RequestedModel（渠道映射前的原始模型）
	requestedModel := result.Model
	if input.OriginalModel != "" {
		requestedModel = input.OriginalModel
	}

	// 计算费用
	cost := s.calculateRecordUsageCost(ctx, result, apiKey, billingModel, multiplier, imageMultiplier, opts)

	// 判断计费方式：订阅模式 vs 余额模式
	isSubscriptionBilling := subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
	billingType := BillingTypeBalance
	if isSubscriptionBilling {
		billingType = BillingTypeSubscription
	}

	// 创建使用日志
	accountRateMultiplier := account.BillingRateMultiplier()
	usageLog := s.buildRecordUsageLog(ctx, input, result, apiKey, user, account, subscription,
		requestedModel, multiplier, imageMultiplier, accountRateMultiplier, billingType, cacheTTLOverridden, cost, opts)

	// 询问插件层是否有渠道级账号统计定价覆写。优先级：
	//   1. 自定义规则命中 → cost
	//   2. 渠道开启 ApplyPricingToAccountStats 且 totalCost > 0 → 客户计费
	//   3. nil → 默认公式 COALESCE(account_stats_cost, total_cost) * account_rate_multiplier
	// upstream model 用 result.UpstreamModel（如果空则 fallback 到 result.Model）
	// 以匹配 release 行为（commit 11c460687）。
	upstreamModel := result.UpstreamModel
	if upstreamModel == "" {
		upstreamModel = result.Model
	}
	totalCost := 0.0
	if cost != nil {
		totalCost = cost.TotalCost
	}
	requestCount := 1
	if result.ImageCount > 0 {
		requestCount = result.ImageCount
	}
	groupID := int64(0)
	if apiKey.GroupID != nil {
		groupID = *apiKey.GroupID
	}
	usageLog.AccountStatsCost = s.resolveAccountStatsCost(
		ctx, usageLog.RequestID,
		input.ChannelID, account.ID, groupID,
		upstreamModel, result.Usage, requestCount, totalCost,
	)

	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.gateway")
		logger.LegacyPrintf("service.gateway", "[SIMPLE MODE] Usage recorded (not billed): user=%d, tokens=%d", usageLog.UserID, usageLog.TotalTokens())
		s.deferredService.ScheduleLastUsedUpdate(account.ID)
		return nil
	}

	requestID := usageLog.RequestID
	_, billingErr := applyUsageBilling(ctx, requestID, usageLog, &postUsageBillingParams{
		Cost:                  cost,
		User:                  user,
		APIKey:                apiKey,
		Account:               account,
		Subscription:          subscription,
		RequestPayloadHash:    resolveUsageBillingPayloadFingerprint(ctx, input.RequestPayloadHash),
		IsSubscriptionBill:    isSubscriptionBilling,
		AccountRateMultiplier: accountRateMultiplier,
		APIKeyService:         input.APIKeyService,
		ServiceQuotaRequest:   input.ServiceQuotaRequest,
		InputTokens:           int64(usageLog.InputTokens),
		OutputTokens:          int64(usageLog.OutputTokens),
		CacheCreationTokens:   int64(usageLog.CacheCreationTokens),
		CacheReadTokens:       int64(usageLog.CacheReadTokens),
	}, s.billingDeps(), s.usageBillingRepo)

	if billingErr != nil {
		return billingErr
	}
	writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.gateway")

	return nil
}

// calculateRecordUsageCost 根据请求类型和选项计算费用。
func (s *GatewayService) calculateRecordUsageCost(
	ctx context.Context,
	result *ForwardResult,
	apiKey *APIKey,
	billingModel string,
	multiplier float64,
	imageMultiplier float64,
	opts *recordUsageOpts,
) *CostBreakdown {
	// 图片生成计费
	if result.ImageCount > 0 {
		return s.calculateImageCost(ctx, result, apiKey, billingModel, imageMultiplier)
	}

	// Token 计费
	return s.calculateTokenCost(ctx, result, apiKey, billingModel, multiplier, opts)
}

// resolveChannelPricing 检查指定模型是否存在渠道级别定价。
// 返回非 nil 的 ResolvedPricing 表示有渠道定价，nil 表示走默认定价路径。
func (s *GatewayService) resolveChannelPricing(ctx context.Context, billingModel string, apiKey *APIKey) *ResolvedPricing {
	if s.resolver == nil || apiKey.Group == nil {
		return nil
	}
	gid := apiKey.Group.ID
	resolved := s.resolver.Resolve(ctx, PricingInput{
		Model:    billingModel,
		GroupID:  &gid,
		Platform: apiKey.Group.Platform,
	})
	if resolved.Source == PricingSourceChannel {
		return resolved
	}
	return nil
}

// calculateImageCost 计算图片生成费用：渠道级别定价优先，否则走按次计费。
func (s *GatewayService) calculateImageCost(
	ctx context.Context,
	result *ForwardResult,
	apiKey *APIKey,
	billingModel string,
	multiplier float64,
) *CostBreakdown {
	if resolved := s.resolveChannelPricing(ctx, billingModel, apiKey); resolved != nil {
		tokens := UsageTokens{
			InputTokens:       result.Usage.InputTokens,
			OutputTokens:      result.Usage.OutputTokens,
			ImageOutputTokens: result.Usage.ImageOutputTokens,
		}
		gid := apiKey.Group.ID
		cost, err := s.billingService.CalculateCostUnified(CostInput{
			Ctx:            ctx,
			Model:          billingModel,
			GroupID:        &gid,
			Tokens:         tokens,
			RequestCount:   result.ImageCount,
			SizeTier:       result.ImageSize,
			RateMultiplier: multiplier,
			Resolver:       s.resolver,
			Resolved:       resolved,
		})
		if err != nil {
			logger.LegacyPrintf("service.gateway", "Calculate image token cost failed: %v", err)
			return &CostBreakdown{ActualCost: 0}
		}
		return cost
	}

	var groupConfig *ImagePriceConfig
	if apiKey.Group != nil {
		cfg := apiKey.Group.ImageConfig()
		groupConfig = &ImagePriceConfig{
			Price1K: cfg.Price1K,
			Price2K: cfg.Price2K,
			Price4K: cfg.Price4K,
		}
	}
	return s.billingService.CalculateImageCost(billingModel, result.ImageSize, result.ImageCount, groupConfig, multiplier)
}

// calculateTokenCost 计算 Token 计费：根据 opts 决定走普通/长上下文/渠道统一计费。
func (s *GatewayService) calculateTokenCost(
	ctx context.Context,
	result *ForwardResult,
	apiKey *APIKey,
	billingModel string,
	multiplier float64,
	opts *recordUsageOpts,
) *CostBreakdown {
	tokens := UsageTokens{
		InputTokens:           result.Usage.InputTokens,
		OutputTokens:          result.Usage.OutputTokens,
		CacheCreationTokens:   result.Usage.CacheCreationInputTokens,
		CacheReadTokens:       result.Usage.CacheReadInputTokens,
		CacheCreation5mTokens: result.Usage.CacheCreation5mTokens,
		CacheCreation1hTokens: result.Usage.CacheCreation1hTokens,
		ImageOutputTokens:     result.Usage.ImageOutputTokens,
	}

	var cost *CostBreakdown
	var err error

	// 优先尝试渠道定价 → CalculateCostUnified
	if resolved := s.resolveChannelPricing(ctx, billingModel, apiKey); resolved != nil {
		gid := apiKey.Group.ID
		cost, err = s.billingService.CalculateCostUnified(CostInput{
			Ctx:            ctx,
			Model:          billingModel,
			GroupID:        &gid,
			Tokens:         tokens,
			RequestCount:   1,
			RateMultiplier: multiplier,
			Resolver:       s.resolver,
			Resolved:       resolved,
		})
	} else if opts.LongContextThreshold > 0 {
		// 长上下文双倍计费（如 Gemini 200K 阈值）
		cost, err = s.billingService.CalculateCostWithLongContext(
			billingModel, tokens, multiplier,
			opts.LongContextThreshold, opts.LongContextMultiplier,
		)
	} else {
		cost, err = s.billingService.CalculateCost(billingModel, tokens, multiplier)
	}
	if err != nil {
		logger.LegacyPrintf("service.gateway", "Calculate cost failed: %v", err)
		return &CostBreakdown{ActualCost: 0}
	}
	return cost
}

// buildRecordUsageLog 构建使用日志并设置计费模式。
func (s *GatewayService) buildRecordUsageLog(
	ctx context.Context,
	input *recordUsageCoreInput,
	result *ForwardResult,
	apiKey *APIKey,
	user *User,
	account *Account,
	subscription *UserSubscription,
	requestedModel string,
	multiplier float64,
	imageMultiplier float64,
	accountRateMultiplier float64,
	billingType int8,
	cacheTTLOverridden bool,
	cost *CostBreakdown,
	opts *recordUsageOpts,
) *UsageLog {
	durationMs := int(result.Duration.Milliseconds())
	requestID := resolveUsageBillingRequestID(ctx, result.RequestID)
	usageLog := &UsageLog{
		UserID:                user.ID,
		APIKeyID:              apiKey.ID,
		AccountID:             account.ID,
		RequestID:             requestID,
		Model:                 result.Model,
		RequestedModel:        requestedModel,
		UpstreamModel:         optionalNonEqualStringPtr(result.UpstreamModel, result.Model),
		ReasoningEffort:       result.ReasoningEffort,
		InboundEndpoint:       optionalTrimmedStringPtr(input.InboundEndpoint),
		UpstreamEndpoint:      optionalTrimmedStringPtr(input.UpstreamEndpoint),
		InputTokens:           result.Usage.InputTokens,
		OutputTokens:          result.Usage.OutputTokens,
		CacheCreationTokens:   result.Usage.CacheCreationInputTokens,
		CacheReadTokens:       result.Usage.CacheReadInputTokens,
		CacheCreation5mTokens: result.Usage.CacheCreation5mTokens,
		CacheCreation1hTokens: result.Usage.CacheCreation1hTokens,
		ImageOutputTokens:     result.Usage.ImageOutputTokens,
		RateMultiplier:        multiplier,
		AccountRateMultiplier: &accountRateMultiplier,
		BillingType:           billingType,
		BillingMode:           resolveBillingMode(result, cost),
		Stream:                result.Stream,
		DurationMs:            &durationMs,
		FirstTokenMs:          result.FirstTokenMs,
		ImageCount:            result.ImageCount,
		ImageSize:             optionalTrimmedStringPtr(result.ImageSize),
		CacheTTLOverridden:    cacheTTLOverridden,
		ChannelID:             optionalInt64Ptr(input.ChannelID),
		ModelMappingChain:     optionalTrimmedStringPtr(input.ModelMappingChain),
		UserAgent:             optionalTrimmedStringPtr(input.UserAgent),
		IPAddress:             optionalTrimmedStringPtr(input.IPAddress),
		GroupID:               apiKey.GroupID,
		SubscriptionID:        optionalSubscriptionID(subscription),
		CreatedAt:             time.Now(),
	}
	if result.ImageCount > 0 {
		usageLog.RateMultiplier = imageMultiplier
	}
	if cost != nil {
		usageLog.InputCost = cost.InputCost
		usageLog.OutputCost = cost.OutputCost
		usageLog.ImageOutputCost = cost.ImageOutputCost
		usageLog.CacheCreationCost = cost.CacheCreationCost
		usageLog.CacheReadCost = cost.CacheReadCost
		usageLog.TotalCost = cost.TotalCost
		usageLog.ActualCost = cost.ActualCost
	}

	return usageLog
}

// resolveBillingMode 根据计费结果和请求类型确定计费模式。
func resolveBillingMode(result *ForwardResult, cost *CostBreakdown) *string {
	var mode string
	switch {
	case cost != nil && cost.BillingMode != "":
		mode = cost.BillingMode
	case result.ImageCount > 0:
		mode = string(BillingModeImage)
	default:
		mode = string(BillingModeToken)
	}
	return &mode
}

func optionalSubscriptionID(subscription *UserSubscription) *int64 {
	if subscription != nil {
		return &subscription.ID
	}
	return nil
}
