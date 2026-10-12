package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// VideoTaskCostSnapshot contains only billing data, never API credentials or
// provider URLs. Persist it before acknowledging creation. Completion metadata
// may enrich the usage row but cannot change this accepted quote or command.
type VideoTaskCostSnapshot struct {
	Version       int                 `json:"version"`
	Command       UsageBillingCommand `json:"command"`
	Log           UsageLog            `json:"log"`
	Cost          CostBreakdown       `json:"cost"`
	Platform      string              `json:"platform"`
	QuotaPlatform string              `json:"quota_platform"`
	RateLimit5h   float64             `json:"rate_limit_5h"`
	RateLimit1d   float64             `json:"rate_limit_1d"`
	RateLimit7d   float64             `json:"rate_limit_7d"`
}

// ResolveVideoTaskCost resolves an accepted fixed-quantity video using the
// existing pricing calculators. This is read-only: no charge, dedup or log write.
// Token-priced video cannot be quoted from unknown completion token counts.
func (s *OpenAIGatewayService) ResolveVideoTaskCost(ctx context.Context, input *OpenAIRecordUsageInput) (*VideoTaskCostSnapshot, error) {
	if s == nil || s.billingService == nil || input == nil || input.Result == nil || input.APIKey == nil || input.Account == nil {
		return nil, errors.New("video cost snapshot requires result, key, account and billing service")
	}
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return nil, errors.New("atomic video usage billing is unavailable in simple mode")
	}
	key, account, result := input.APIKey, input.Account, input.Result
	user := input.User
	if user == nil {
		user = &User{ID: key.UserID}
	}
	if user.ID <= 0 || key.ID <= 0 || account.ID <= 0 || (key.UserID != 0 && key.UserID != user.ID) {
		return nil, errors.New("video cost snapshot owner mismatch")
	}
	if key.GroupID != nil && (key.Group == nil || key.Group.ID != *key.GroupID) {
		return nil, errors.New("video cost snapshot requires the accepted group")
	}
	if input.Subscription != nil && (key.GroupID == nil || input.Subscription.UserID != user.ID || input.Subscription.GroupID != *key.GroupID) {
		return nil, errors.New("video cost snapshot subscription mismatch")
	}
	model := firstNonEmpty(result.BillingModel, forwardResultBillingModel(result.Model, result.UpstreamModel))
	if input.BillingModelSource == BillingModelSourceChannelMapped && input.ChannelMappedModel != "" && input.ChannelMappedModel != input.OriginalModel {
		model = strings.TrimSpace(input.ChannelMappedModel)
	}
	if input.BillingModelSource == BillingModelSourceRequested && input.OriginalModel != "" {
		model = strings.TrimSpace(input.OriginalModel)
	}
	if model == "" || result.VideoCount <= 0 {
		return nil, errors.New("video cost snapshot requires model and positive count")
	}
	resolution, seconds, err := videoTaskBillingUnits(account.Platform, model, result.VideoResolution, result.VideoDurationSeconds)
	if err != nil {
		return nil, err
	}
	multiplier := 1.0
	if s.cfg != nil {
		multiplier = s.cfg.Default.RateMultiplier
	}
	if key.GroupID != nil {
		multiplier = s.ResolveUserGroupRateMultiplier(ctx, user.ID, *key.GroupID, key.Group.RateMultiplier)
	}
	multiplier = resolveVideoRateMultiplier(key, multiplier)
	if !validVideoMoney(multiplier) || !validVideoMoney(account.BillingRateMultiplier()) {
		return nil, errors.New("invalid video billing multiplier")
	}
	pricingAt := openAIUsagePricingAt(input)
	cost, err := s.resolveVideoTaskPrice(ctx, key, model, resolution, result.VideoCount, seconds, multiplier, pricingAt)
	if err != nil {
		return nil, err
	}
	if cost == nil || !validVideoMoney(cost.TotalCost) || !validVideoMoney(cost.ActualCost) {
		return nil, errors.New("invalid video cost")
	}
	cost.ActualCost = QuantizeUsageBillingAmount(cost.ActualCost)
	accountRate := account.BillingRateMultiplier()
	log := UsageLog{
		UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID,
		Model: firstNonEmpty(result.Model, model), RequestedModel: firstNonEmpty(input.OriginalModel, result.Model, model),
		UpstreamModel: optionalTrimmedStringPtr(result.UpstreamModel),
		GroupID:       copyVideoID(key.GroupID), VideoCount: result.VideoCount,
		VideoResolution: &resolution, VideoDurationSeconds: &seconds,
		TotalCost: cost.TotalCost, ActualCost: cost.ActualCost, BillingMode: optionalTrimmedStringPtr(cost.BillingMode),
		RateMultiplier: multiplier, AccountRateMultiplier: &accountRate,
		ChannelID: optionalInt64Ptr(input.ChannelID), ModelMappingChain: optionalTrimmedStringPtr(input.ModelMappingChain),
		InboundEndpoint: optionalTrimmedStringPtr(input.InboundEndpoint), UpstreamEndpoint: optionalTrimmedStringPtr(input.UpstreamEndpoint),
	}
	isSubscription := input.Subscription != nil && key.Group != nil && key.Group.IsSubscriptionType()
	if isSubscription {
		log.BillingType = BillingTypeSubscription
		log.SubscriptionID = copyVideoID(&input.Subscription.ID)
	}
	if key.GroupID != nil {
		applyAccountStatsCost(ctx, &log, s.channelService, s.billingService, account.ID, *key.GroupID,
			result.UpstreamModel, result.Model, UsageTokens{}, cost.TotalCost, pricingAt, true)
	}
	p := &postUsageBillingParams{
		Cost: cost, User: user, APIKey: key, Account: account, Subscription: input.Subscription,
		RequestPayloadHash: strings.TrimSpace(input.RequestPayloadHash), IsSubscriptionBill: isSubscription,
		AccountRateMultiplier: accountRate, APIKeyService: input.APIKeyService,
	}
	cmd := buildUsageBillingCommand("video-cost-snapshot", &log, p)
	// SQL quota updates need no APIKeyService. Freeze their participation now,
	// even if the caller only has the authenticated key snapshot at acceptance.
	if key.Quota > 0 {
		cmd.APIKeyQuotaCost = cost.ActualCost
	}
	if key.HasRateLimits() {
		cmd.APIKeyRateLimitCost = cost.ActualCost
	}
	cmd.RequestID, cmd.RequestFingerprint = "", ""
	cmd.SubscriptionID = copyVideoID(log.SubscriptionID)
	cmd.MediaType = fmt.Sprintf("video:%s:%s:%d:%d", model, resolution, seconds, result.VideoCount)
	cmd.Normalize()
	cmd.RequestFingerprint = ""
	platform := firstNonEmpty(input.QuotaPlatform, PlatformFromAPIKey(key))
	snapshot := &VideoTaskCostSnapshot{
		Version: 1, Command: *cmd, Log: log, Cost: *cost, Platform: account.Platform, QuotaPlatform: platform,
		RateLimit5h: key.RateLimit5h, RateLimit1d: key.RateLimit1d, RateLimit7d: key.RateLimit7d,
	}
	if err := snapshot.validate(); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func videoTaskBillingUnits(platform, model, resolution string, seconds int) (string, int, error) {
	if platform != PlatformOpenAI {
		return NormalizeVideoBillingResolutionOrDefault(resolution), NormalizeVideoBillingDurationSecondsOrDefault(seconds), nil
	}
	// Known xAI models retain their documented defaults through OpenAI transport,
	// but OpenAI/Seedance durations must never be clamped to xAI's 15-second cap.
	_, knownGrok := getDefaultGrokImagineVideoPrice(model, VideoBillingResolution480P)
	resolution = strings.ToLower(strings.TrimSpace(resolution))
	if resolution == "" && knownGrok {
		resolution = VideoBillingResolution480P
	}
	if seconds <= 0 && knownGrok {
		seconds = VideoBillingDefaultDurationSeconds
	}
	if normalized, ok := LookupVideoBillingResolution(resolution); ok {
		resolution = normalized
	}
	if resolution == "" || seconds <= 0 {
		return "", 0, errors.New("video cost snapshot requires accepted resolution and duration")
	}
	return resolution, seconds, nil
}

func (s *OpenAIGatewayService) resolveVideoTaskPrice(ctx context.Context, key *APIKey, model, resolution string, count, seconds int, multiplier float64, pricingAt time.Time) (*CostBreakdown, error) {
	resolved := s.resolveOpenAIChannelPricing(ctx, model, key)
	if resolved != nil && resolved.Mode == BillingModeToken {
		return nil, errors.New("cannot freeze video token pricing before completion usage is known")
	}
	configured := func() (*CostBreakdown, error) {
		if resolved.Mode != BillingModeVideo && resolved.Mode != BillingModePerRequest && resolved.Mode != BillingModeImage {
			return nil, fmt.Errorf("%w: unsupported video billing mode", ErrModelPricingUnavailable)
		}
		identified := resolved.channelPricing != nil && resolved.channelPricing.PerRequestPrice != nil
		for _, tier := range resolved.RequestTiers {
			if tier.PerRequestPrice != nil && VideoTierLabelsMatch(tier.TierLabel, resolution) {
				identified = true
			}
		}
		if tier := FindMatchingInterval(resolved.RequestTiers, 0); tier != nil && tier.TierLabel == "" && tier.PerRequestPrice != nil {
			identified = true
		}
		if !identified {
			return nil, fmt.Errorf("%w: no configured video rate for %s/%s", ErrModelPricingUnavailable, model, resolution)
		}
		units := float64(count)
		if resolved.Mode == BillingModeVideo {
			units *= float64(seconds)
		}
		return s.billingService.CalculateCostUnified(CostInput{
			Ctx: ctx, Model: model, GroupID: key.GroupID, Group: key.Group,
			RequestCount: count, UsageUnits: units, SizeTier: resolution, RateMultiplier: multiplier,
			PricingAt: pricingAt, Resolver: s.resolver, Resolved: resolved,
		})
	}
	if resolved != nil && resolved.Source == PricingSourceGroup {
		return configured()
	}
	// Legacy tier cards only have xAI resolution names. Never silently map an
	// OpenAI/Seedance tier such as 4k to the lowest-priced legacy tier.
	_, legacyTier := LookupVideoBillingResolution(resolution)
	if legacyTier && apiKeyHasConfiguredVideoPrice(key, model, resolution) {
		cost := s.billingService.CalculateVideoCost(model, resolution, count, 1, videoPriceConfigFromAPIKey(key), multiplier)
		applyCostBreakdownMultiplier(cost, float64(seconds))
		return cost, nil
	}
	if resolved != nil && resolved.Source == PricingSourceChannel {
		return configured()
	}
	if _, known := getDefaultGrokImagineVideoPrice(model, resolution); !known || !legacyTier {
		return nil, fmt.Errorf("%w: no identified video rate for %s/%s", ErrModelPricingUnavailable, model, resolution)
	}
	// Reuse the known model's calculator at one second, then apply the accepted
	// duration so the legacy calculator cannot truncate an OpenAI transport task.
	cost := s.billingService.CalculateVideoCost(model, resolution, count, 1, nil, multiplier)
	applyCostBreakdownMultiplier(cost, float64(seconds))
	return cost, nil
}

func validVideoMoney(v float64) bool {
	return v >= 0 && !math.IsNaN(v) && !math.IsInf(v, 0)
}

func copyVideoID(id *int64) *int64 {
	if id == nil {
		return nil
	}
	v := *id
	return &v
}

func (snapshot *VideoTaskCostSnapshot) validate() error {
	if snapshot == nil || snapshot.Version != 1 {
		return errors.New("missing or unsupported video cost snapshot")
	}
	c, log := &snapshot.Command, &snapshot.Log
	if c.UserID <= 0 || c.APIKeyID <= 0 || c.AccountID <= 0 || c.VideoUsageLog != nil ||
		log.UserID != c.UserID || log.APIKeyID != c.APIKeyID || log.AccountID != c.AccountID ||
		log.Model != c.Model || log.BillingType != c.BillingType || valueOrZero(log.SubscriptionID) != valueOrZero(c.SubscriptionID) ||
		log.VideoCount <= 0 || log.VideoDurationSeconds == nil || *log.VideoDurationSeconds <= 0 || log.VideoResolution == nil ||
		strings.TrimSpace(*log.VideoResolution) == "" || log.AccountRateMultiplier == nil || log.BillingMode == nil ||
		*log.BillingMode != snapshot.Cost.BillingMode || log.TotalCost != snapshot.Cost.TotalCost || log.ActualCost != snapshot.Cost.ActualCost {
		return errors.New("inconsistent video cost snapshot")
	}
	for _, amount := range []float64{snapshot.Cost.TotalCost, snapshot.Cost.ActualCost, c.BalanceCost, c.SubscriptionCost,
		c.APIKeyQuotaCost, c.APIKeyRateLimitCost, c.AccountQuotaCost, log.RateMultiplier, *log.AccountRateMultiplier} {
		if !validVideoMoney(amount) {
			return errors.New("invalid video snapshot amount")
		}
	}
	if c.BalanceCost+c.SubscriptionCost != snapshot.Cost.ActualCost ||
		(c.BillingType == BillingTypeSubscription && (c.SubscriptionID == nil || c.BalanceCost != 0)) ||
		(c.BillingType == BillingTypeBalance && c.SubscriptionCost != 0) {
		return errors.New("video snapshot charge does not match usage cost")
	}
	return nil
}

func (s *OpenAIGatewayService) recordVideoTaskCost(ctx context.Context, input *OpenAIRecordUsageInput) error {
	snapshot := input.VideoTaskCost
	if err := snapshot.validate(); err != nil {
		return err
	}
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return errors.New("atomic video usage billing is unavailable in simple mode")
	}
	cmd, log, cost := snapshot.Command, snapshot.Log, snapshot.Cost
	if input.APIKey == nil || input.User == nil || input.Account == nil || input.APIKey.ID != cmd.APIKeyID ||
		input.User.ID != cmd.UserID || (input.APIKey.UserID != 0 && input.APIKey.UserID != cmd.UserID) ||
		input.Account.ID != cmd.AccountID || input.Account.Platform != snapshot.Platform {
		return errors.New("video cost snapshot owner or account mismatch")
	}
	if cmd.SubscriptionID != nil && (input.Subscription == nil || input.Subscription.ID != *cmd.SubscriptionID ||
		input.Subscription.UserID != cmd.UserID || input.Subscription.GroupID != valueOrZero(log.GroupID)) {
		return errors.New("video cost snapshot subscription mismatch")
	}
	result := input.Result
	if result.VideoCount != log.VideoCount {
		return errors.New("video completion count differs from accepted quote")
	}
	// The saved accepted model/resolution/duration govern the fixed quote. The
	// worker preserves the observed completion separately for upstream auditing.
	id := firstNonEmpty(result.RequestID, result.ResponseID)
	if id == "" {
		return ErrUsageBillingRequestIDRequired
	}
	cmd.RequestID = StableGrokVideoBillingRequestID(strings.TrimPrefix(strings.TrimSpace(id), "grok-video:"))
	cmd.RequestFingerprint = ""
	cmd.Normalize()
	log.RequestID, log.ID, log.CreatedAt = cmd.RequestID, 0, time.Now()
	duration := int(result.Duration.Milliseconds())
	log.DurationMs = &duration
	log.UpstreamResponseModel = optionalTrimmedStringPtr(result.UpstreamResponseModel)
	log.SessionID = optionalTrimmedStringPtr(input.SessionID)
	key := *input.APIKey
	key.GroupID = copyVideoID(log.GroupID)
	key.RateLimit5h, key.RateLimit1d, key.RateLimit7d = snapshot.RateLimit5h, snapshot.RateLimit1d, snapshot.RateLimit7d
	p := &postUsageBillingParams{
		Cost: &cost, User: input.User, APIKey: &key, Account: input.Account, Subscription: input.Subscription,
		IsSubscriptionBill: cmd.BillingType == BillingTypeSubscription, AccountRateMultiplier: *log.AccountRateMultiplier,
		APIKeyService: input.APIKeyService, Platform: snapshot.QuotaPlatform,
	}
	_, err := applyVideoUsageBillingCommand(ctx, &cmd, &log, p, s.billingDeps(), s.usageBillingRepo)
	return err
}
