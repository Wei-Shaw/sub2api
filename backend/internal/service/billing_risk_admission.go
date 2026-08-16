package service

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const defaultBillingRiskMaxOutputTokens = 64 * 1024

// BillingRiskAdmissionInput 是网关请求前风险准入所需的计费快照。
// BillingModel 必须优先传渠道映射后的最终计费模型。
type BillingRiskAdmissionInput struct {
	APIKey          *APIKey
	Subscription    *UserSubscription
	Kind            BillingRiskRequestKind
	BillingModel    string
	InputTokens     int
	MaxOutputTokens int
	NoOutput        bool
	RequestCount    int
	SearchCalls     int
	WebSearchCalls  int
	AudioMode       string
	UsageUnits      float64
	SizeTier        string
	ServiceTier     string
	PricingAt       time.Time
	ForceProtect    bool
	// LongContextThreshold/Multiplier 用于 Gemini 等 handler 固定的长上下文计费规则。
	LongContextThreshold  int
	LongContextMultiplier float64
	// ConservativeUnknown 用于准入阶段无法覆盖全部计费维度的混合请求，按独占预算处理。
	ConservativeUnknown bool
	LeaseID             string
}

// BillingRiskAdmissionService 把网关请求快照转换成统一费用估算并获取风险许可。
// 高余额普通文本在读取价格和用户倍率前直接旁路。
type BillingRiskAdmissionService struct {
	guard        *BillingRiskGuard
	estimator    *BillingRiskEstimator
	rateResolver *userGroupRateResolver
	cfg          *config.Config
}

func NewBillingRiskAdmissionService(
	guard *BillingRiskGuard,
	estimator *BillingRiskEstimator,
	rateRepo UserGroupRateRepository,
	cfg *config.Config,
) *BillingRiskAdmissionService {
	return &BillingRiskAdmissionService{
		guard:        guard,
		estimator:    estimator,
		rateResolver: newUserGroupRateResolver(rateRepo, nil, resolveUserGroupRateCacheTTL(cfg), nil, "service.billing_risk"),
		cfg:          cfg,
	}
}

func (s *BillingRiskAdmissionService) IsEnabled() bool {
	if s == nil || s.guard == nil || (s.cfg != nil && s.cfg.RunMode == config.RunModeSimple) {
		return false
	}
	return s.guard.currentSettings().Enabled
}

func (s *BillingRiskAdmissionService) Acquire(ctx context.Context, input BillingRiskAdmissionInput) (*BillingRiskPermit, error) {
	if s == nil || s.guard == nil {
		return nil, nil
	}
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return nil, nil
	}
	settings := s.guard.currentSettings()
	if !settings.Enabled {
		return nil, nil
	}
	if input.APIKey == nil || input.APIKey.User == nil || input.APIKey.User.ID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_BILLING_RISK_REQUEST", "余额风险准入缺少用户信息")
	}

	apiKey := input.APIKey
	user := apiKey.User
	currentBalance := user.Balance
	subscriptionBilling := input.Subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
	if subscriptionBilling {
		return nil, nil
	}
	highBalanceTextBypass := (input.Kind == BillingRiskRequestText || input.Kind == BillingRiskRequestWebSocket) &&
		!input.ForceProtect && !input.ConservativeUnknown && currentBalance > settings.LowBalanceThreshold
	if user.BillingBalanceAuthoritative {
		user.BillingBalanceAuthoritative = false
		balanceVersion := user.BillingBalanceVersion
		user.BillingBalanceVersion = 0
		reset, err := s.guard.ResetBalance(ctx, user.ID, currentBalance, balanceVersion)
		if err != nil {
			if highBalanceTextBypass {
				return nil, nil
			}
			return nil, infraerrors.ServiceUnavailable("BILLING_RISK_GUARD_UNAVAILABLE", "余额风险保护暂时不可用").WithCause(err)
		}
		if reset != nil && !reset.Accepted {
			currentBalance = float64(reset.KnownBalanceMicros) / 1_000_000
			user.Balance = currentBalance
		}
		highBalanceTextBypass = (input.Kind == BillingRiskRequestText || input.Kind == BillingRiskRequestWebSocket) &&
			!input.ForceProtect && !input.ConservativeUnknown && currentBalance > settings.LowBalanceThreshold
	}
	if highBalanceTextBypass {
		return nil, nil
	}

	estimate := BillingRiskEstimate{}
	if s.estimator != nil && !input.ConservativeUnknown {
		rateMultiplier, rateCertain := s.resolveRateMultiplier(ctx, apiKey, input)
		if !rateCertain {
			return s.acquireEstimate(ctx, input, user, currentBalance, subscriptionBilling, estimate)
		}
		if input.SearchCalls > 0 && s.estimator.billing != nil {
			cost := s.estimator.billing.CalculateSearchCost(input.SearchCalls, groupSearchPricePer1kFromAPIKey(apiKey), rateMultiplier)
			if cost != nil {
				estimate.Cost = cost.ActualCost
				estimate.Certain = true
			}
		} else if input.WebSearchCalls > 0 && s.estimator.billing != nil {
			cost := s.estimator.billing.CalculateWebSearchCost(input.WebSearchCalls, webSearchPricePerCallFromAPIKey(apiKey), rateMultiplier)
			if cost != nil {
				estimate.Cost = cost.ActualCost
				estimate.Certain = true
			}
		} else if strings.TrimSpace(input.AudioMode) != "" && s.estimator.billing != nil {
			var resolved *ResolvedPricing
			if s.estimator.resolver != nil {
				resolved = s.estimator.resolver.Resolve(ctx, PricingInput{
					Model: strings.TrimSpace(input.BillingModel), GroupID: apiKey.GroupID, Group: apiKey.Group,
				})
			}
			if resolved != nil && resolved.Mode == BillingModePerRequest &&
				(resolved.Source == PricingSourceGroup || resolved.Source == PricingSourceChannel) {
				estimate, _ = s.estimator.Estimate(ctx, BillingRiskEstimateInput{
					Model: strings.TrimSpace(input.BillingModel), GroupID: apiKey.GroupID, Group: apiKey.Group,
					RequestCount: input.RequestCount, UsageUnits: input.UsageUnits,
					RateMultiplier: rateMultiplier, ServiceTier: input.ServiceTier,
				})
				return s.acquireEstimate(ctx, input, user, currentBalance, subscriptionBilling, estimate)
			}
			cost := s.estimator.billing.CalculateAudioCost(input.AudioMode, input.UsageUnits, groupAudioPriceConfigFromAPIKey(apiKey), rateMultiplier)
			if cost != nil {
				estimate.Cost = cost.ActualCost
				estimate.Certain = true
			}
		}
		if estimate.Certain {
			return s.acquireEstimate(ctx, input, user, currentBalance, subscriptionBilling, estimate)
		}
		if mediaEstimate, handled := s.estimateMediaCost(ctx, input, apiKey, rateMultiplier); handled {
			return s.acquireEstimate(ctx, input, user, currentBalance, subscriptionBilling, mediaEstimate)
		}
		requestCount := input.RequestCount
		if requestCount <= 0 {
			requestCount = 1
		}
		inputTokens := input.InputTokens
		if inputTokens < 0 {
			inputTokens = 0
		}
		maxOutputTokens := input.MaxOutputTokens
		if input.NoOutput {
			maxOutputTokens = 0
		} else if maxOutputTokens <= 0 && (input.Kind == BillingRiskRequestText || input.Kind == BillingRiskRequestWebSocket) {
			maxOutputTokens = defaultBillingRiskMaxOutputTokens
		}
		var err error
		estimate, err = s.estimator.Estimate(ctx, BillingRiskEstimateInput{
			Model:                 strings.TrimSpace(input.BillingModel),
			GroupID:               apiKey.GroupID,
			Group:                 apiKey.Group,
			Tokens:                UsageTokens{InputTokens: inputTokens, OutputTokens: maxOutputTokens},
			RequestCount:          requestCount,
			UsageUnits:            input.UsageUnits,
			SizeTier:              input.SizeTier,
			RateMultiplier:        rateMultiplier,
			ServiceTier:           input.ServiceTier,
			LongContextThreshold:  input.LongContextThreshold,
			LongContextMultiplier: input.LongContextMultiplier,
		})
		if err != nil {
			return nil, infraerrors.BadRequest("INVALID_BILLING_RISK_ESTIMATE", err.Error()).WithCause(err)
		}
	}

	return s.acquireEstimate(ctx, input, user, currentBalance, subscriptionBilling, estimate)
}

// estimateMediaCost 对齐最终媒体账单的模型级、分组平铺、渠道及默认价格优先级。
// 显式媒体 ModelPricing 仍交给统一 estimator，避免覆盖更具体的模型级/渠道价格。
func (s *BillingRiskAdmissionService) estimateMediaCost(
	ctx context.Context,
	input BillingRiskAdmissionInput,
	apiKey *APIKey,
	rateMultiplier float64,
) (BillingRiskEstimate, bool) {
	if s == nil || s.estimator == nil || s.estimator.billing == nil || apiKey == nil || apiKey.Group == nil {
		return BillingRiskEstimate{}, false
	}
	model := strings.TrimSpace(input.BillingModel)
	if model == "" {
		return BillingRiskEstimate{}, false
	}
	var resolved *ResolvedPricing
	if s.estimator.resolver != nil {
		resolved = s.estimator.resolver.Resolve(ctx, PricingInput{Model: model, GroupID: apiKey.GroupID, Group: apiKey.Group})
	}
	if resolved != nil && resolved.Mode == BillingModeToken &&
		(resolved.Source == PricingSourceGroup || resolved.Source == PricingSourceChannel) {
		// 图片/视频响应的 token 数在准入阶段没有可靠上界。显式配置为 token
		// 计费时不能退回按图/按秒默认价，否则会把高额 image output token
		// 费用误判为低成本请求；交给 guard 按不确定估价独占可用预算。
		return BillingRiskEstimate{Model: model, Source: resolved.Source, Mode: BillingModeToken}, true
	}
	requestCount := input.RequestCount
	if requestCount <= 0 {
		requestCount = 1
	}

	var cost *CostBreakdown
	var mode BillingMode
	switch input.Kind {
	case BillingRiskRequestSyncImage:
		if resolved != nil && resolved.Source == PricingSourceGroup &&
			(resolved.Mode == BillingModePerRequest || resolved.Mode == BillingModeImage) {
			return BillingRiskEstimate{}, false
		}
		sizeTier := NormalizeImageBillingTierOrDefault(input.SizeTier)
		if !apiKeyHasConfiguredImagePrice(apiKey, sizeTier) && resolved != nil && resolved.Source == PricingSourceChannel &&
			(resolved.Mode == BillingModePerRequest || resolved.Mode == BillingModeImage) {
			return BillingRiskEstimate{}, false
		}
		cost = s.estimator.billing.CalculateImageCost(model, sizeTier, requestCount, imagePriceConfigFromAPIKey(apiKey), rateMultiplier)
		mode = BillingModeImage
	case BillingRiskRequestVideo:
		if resolved != nil && resolved.Source == PricingSourceGroup && resolved.Mode == BillingModeVideo {
			return BillingRiskEstimate{}, false
		}
		resolution := NormalizeVideoBillingResolutionOrDefault(input.SizeTier)
		if !apiKeyHasConfiguredVideoPrice(apiKey, model, resolution) && resolved != nil && resolved.Source == PricingSourceChannel &&
			(resolved.Mode == BillingModePerRequest || resolved.Mode == BillingModeImage || resolved.Mode == BillingModeVideo) {
			if resolved.Mode == BillingModePerRequest || resolved.Mode == BillingModeImage {
				estimate, _ := s.estimator.Estimate(ctx, BillingRiskEstimateInput{
					Model: model, GroupID: apiKey.GroupID, Group: apiKey.Group,
					RequestCount: requestCount, UsageUnits: float64(requestCount), SizeTier: resolution,
					RateMultiplier: rateMultiplier, ServiceTier: input.ServiceTier,
				})
				return estimate, true
			}
			return BillingRiskEstimate{}, false
		}
		durationSeconds := NormalizeVideoBillingDurationSecondsOrDefault(int(math.Ceil(input.UsageUnits)))
		cost = s.estimator.billing.CalculateVideoCost(model, resolution, requestCount, durationSeconds, videoPriceConfigFromAPIKey(apiKey), rateMultiplier)
		mode = BillingModeVideo
	default:
		return BillingRiskEstimate{}, false
	}
	if cost == nil || math.IsNaN(cost.ActualCost) || math.IsInf(cost.ActualCost, 0) || cost.ActualCost < 0 {
		return BillingRiskEstimate{Model: model}, true
	}
	return BillingRiskEstimate{
		Model:   model,
		Cost:    cost.ActualCost,
		Certain: true,
		Source:  PricingSourceGroup,
		Mode:    mode,
	}, true
}

func (s *BillingRiskAdmissionService) acquireEstimate(
	ctx context.Context,
	input BillingRiskAdmissionInput,
	user *User,
	currentBalance float64,
	subscriptionBilling bool,
	estimate BillingRiskEstimate,
) (*BillingRiskPermit, error) {
	minimumBalanceReserve := 0.0
	if s.cfg != nil {
		minimumBalanceReserve = s.cfg.Billing.MinimumBalanceReserve
	}
	return s.guard.Acquire(ctx, BillingRiskRequest{
		UserID:                user.ID,
		Balance:               currentBalance,
		MinimumBalanceReserve: minimumBalanceReserve,
		SubscriptionBilling:   subscriptionBilling,
		Kind:                  input.Kind,
		EstimatedCost:         estimate.Cost,
		EstimateCertain:       estimate.Certain,
		ForceProtect:          input.ForceProtect,
		LeaseID:               input.LeaseID,
	})
}

func (s *BillingRiskAdmissionService) resolveRateMultiplier(ctx context.Context, apiKey *APIKey, input BillingRiskAdmissionInput) (float64, bool) {
	base := 1.0
	if s != nil && s.cfg != nil {
		base = s.cfg.Default.RateMultiplier
	}
	if apiKey != nil && apiKey.GroupID != nil && apiKey.Group != nil {
		var err error
		base, err = s.rateResolver.ResolveStrict(ctx, apiKey.UserID, *apiKey.GroupID, apiKey.Group.RateMultiplier)
		if err != nil {
			return 0, false
		}
	}
	if math.IsNaN(base) || math.IsInf(base, 0) || base < 0 {
		return 0, false
	}
	if input.PricingAt.IsZero() {
		input.PricingAt = time.Now()
	}
	switch {
	case input.AudioMode != "" || input.WebSearchCalls > 0:
		return base, true
	case input.Kind == BillingRiskRequestSyncImage:
		_, image := computePeakAwareMultipliers(apiKey, base, input.PricingAt)
		return image, true
	case input.Kind == BillingRiskRequestVideo:
		return resolveVideoRateMultiplier(apiKey, base), true
	default:
		text, _ := computePeakAwareMultipliers(apiKey, base, input.PricingAt)
		return text, true
	}
}

func (s *BillingRiskAdmissionService) RestorePermit(snapshot *BillingRiskPermitSnapshot) *BillingRiskPermit {
	if s == nil || s.guard == nil || snapshot == nil || snapshot.UserID <= 0 || strings.TrimSpace(snapshot.LeaseID) == "" {
		return nil
	}
	return &BillingRiskPermit{
		UserID:            snapshot.UserID,
		LeaseID:           strings.TrimSpace(snapshot.LeaseID),
		RiskMicros:        snapshot.RiskMicros,
		LeaseTTL:          time.Duration(snapshot.LeaseTTLSeconds) * time.Second,
		IdleTTL:           time.Duration(snapshot.IdleTTLSeconds) * time.Second,
		UncertainCooldown: time.Duration(snapshot.UncertainCooldownSeconds) * time.Second,
		RefreshInterval:   time.Duration(snapshot.RefreshIntervalSeconds) * time.Second,
		guard:             s.guard,
	}
}
