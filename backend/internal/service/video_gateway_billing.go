package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// VideoBudgetGuard is the VA1 per-call budget cap gate for
// the video gateway. It is intentionally a narrow interface so the gateway can
// enforce a budget without depending on the concrete balance/credits backend.
// Real balance deduction is wired separately through UserRepository so the static
// guard stays a cap/interception primitive rather than a billing backend.
type VideoBudgetGuard interface {
	// CheckBudget is the FAIL-CLOSED pre-call gate. It MUST return a non-nil error
	// when the subject cannot afford estimatedCost OR when affordability cannot be
	// determined; the gateway rejects the create (no row persisted, no provider
	// dispatch) on ANY error.
	CheckBudget(ctx context.Context, userID int64, estimatedCost float64) error
	// Charge is retained as a legacy post-success hook for tests/telemetry.
	// StaticBudgetGuard implements it as a no-op; real balance charging must not
	// live behind this method.
	Charge(ctx context.Context, userID int64, cost float64, taskID int64) error
}

// SetBudgetGuard wires the VA1 budget guard into the service. Passing nil disables
// the pre-call cap gate; real post-delivery balance billing is wired separately.
func (s *VideoGatewayService) SetBudgetGuard(g VideoBudgetGuard) {
	s.budget = g
}

// SetBalanceBillingDependencies wires the real user-balance billing path used
// after terminal successful video tasks. Passing nil leaves that dependency inert.
func (s *VideoGatewayService) SetBalanceBillingDependencies(userRepo UserRepository, settingService *SettingService, billingCacheService *BillingCacheService) {
	s.userRepo = userRepo
	s.settingService = settingService
	s.billingCacheService = billingCacheService
}

func (s *VideoGatewayService) videoReliabilityCoreEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.ReliabilityCore.VideoEnabled
}

// estimateVideoCost estimates a task's monetary cost for the budget gate. Seedance
// returns no cost field, so it is duration 脳 the configured per-second rate. With
// the default rate of 0 the estimate is 0 (gate inert) until a real rate is set.
func (s *VideoGatewayService) estimateVideoCost(task *VideoTask) float64 {
	if task == nil {
		return 0
	}
	rate := 0.0
	if s.cfg != nil {
		rate = s.cfg.VideoGateway.CostPerSecond
	}
	if rate < 0 {
		rate = 0
	}
	duration := task.Duration
	if duration == -1 {
		duration = 5
	}
	if rate == 0 {
		rate = s.estimateSeedanceCostPerSecond(task)
	}
	return rate * float64(duration)
}

// calculateVideoActualCost converts provider-reported usage tokens to the official
// Seedance CNY cost. Failed tasks and tasks without provider usage cost 0.
func (s *VideoGatewayService) calculateVideoActualCost(task *VideoTask) float64 {
	if task == nil || task.Status != VideoStatusSucceeded || task.UsageTotalTokens == nil || *task.UsageTotalTokens <= 0 {
		return 0
	}
	rate, _, ok := s.videoPricingCatalog().RateCNYPerMillionTokens(task)
	if !ok || rate <= 0 {
		return 0
	}
	return float64(*task.UsageTotalTokens) / videoTokensPerMillion * rate
}

// EstimatePrice adapts the existing video pricing algorithms to the reliability
// core's decimal Money boundary. The pricing math remains owned by
// estimateVideoCost/VideoPricingCatalog; only currency conversion and snapshot
// capture happen here.
func (s *VideoGatewayService) EstimatePrice(ctx context.Context, task *VideoTask) (Money, PricingSnapshot, error) {
	return s.videoPrice(ctx, task, s.estimateVideoCost(task))
}

// ActualPrice shares the same Money + PricingSnapshot boundary as creation so
// finalization cannot accidentally introduce a second financial algorithm.
func (s *VideoGatewayService) ActualPrice(ctx context.Context, task *VideoTask) (Money, PricingSnapshot, error) {
	return s.videoPrice(ctx, task, s.chargeableVideoCost(task))
}

func (s *VideoGatewayService) videoPrice(ctx context.Context, task *VideoTask, rawAmount float64) (Money, PricingSnapshot, error) {
	if task == nil {
		zero := MustUSD("0")
		return zero, PricingSnapshot{AmountOriginal: zero, ExchangeRate: "1.0000000000"}, nil
	}
	s.applyVideoBillingMetadata(task)
	currency := Currency(NormalizeBillingCurrency(task.Currency))
	originalDecimal := decimal.NewFromFloat(rawAmount).Round(ledgerDecimalPlaces)
	if originalDecimal.IsNegative() {
		return Money{}, PricingSnapshot{}, fmt.Errorf("video price must not be negative")
	}
	original, err := NewMoney(originalDecimal.String(), currency)
	if err != nil {
		return Money{}, PricingSnapshot{}, fmt.Errorf("build original video price: %w", err)
	}

	exchangeRate := decimal.NewFromInt(1)
	amountUSDDecimal := originalDecimal
	if currency == Currency(BillingCurrencyCNY) {
		rate := DefaultUSDCNYRate
		if s.settingService != nil {
			rate = s.settingService.GetUSDCNYRate(ctx)
		}
		if rate <= 0 {
			rate = DefaultUSDCNYRate
		}
		exchangeRate = decimal.NewFromFloat(rate).Round(ledgerDecimalPlaces)
		amountUSDDecimal = originalDecimal.Div(exchangeRate).Round(ledgerDecimalPlaces)
	}
	amountUSD, err := NewMoney(amountUSDDecimal.String(), CurrencyUSD)
	if err != nil {
		return Money{}, PricingSnapshot{}, fmt.Errorf("build USD video price: %w", err)
	}
	return amountUSD, PricingSnapshot{
		AmountOriginal: original,
		ExchangeRate:   exchangeRate.StringFixed(ledgerDecimalPlaces),
		PricingSource:  task.PricingSource,
		PricingVersion: task.PricingVersion,
	}, nil
}

func (s *VideoGatewayService) estimateSeedanceCostPerSecond(task *VideoTask) float64 {
	if task == nil || task.Provider != VideoProviderSeedance {
		return 0
	}
	model := normalizeVideoPricingModel(task.Model)
	resolution := strings.ToLower(strings.TrimSpace(task.Resolution))
	if resolution == "" {
		resolution = "720p"
	}
	var base float64
	var noVideoTokenRate float64
	var actualTokenRate float64
	if strings.Contains(model, "seedance-2-0-fast") {
		noVideoTokenRate = 37
		actualTokenRate = 37
		if task.HasVideoInput {
			actualTokenRate = 22
		}
		switch resolution {
		case "480p":
			base = 0.4
		case "720p", "1080p":
			base = 0.8
		}
	} else if strings.Contains(model, "seedance-2-0") {
		noVideoTokenRate = 46
		actualTokenRate = 46
		if task.HasVideoInput {
			actualTokenRate = 28
		}
		switch resolution {
		case "480p":
			base = 0.5
		case "720p":
			base = 1.0
		case "1080p":
			base = 2.5
		}
	}
	if base <= 0 {
		return 0
	}
	if noVideoTokenRate > 0 && actualTokenRate > 0 && actualTokenRate != noVideoTokenRate {
		base *= actualTokenRate / noVideoTokenRate
	}
	return base
}

func (s *VideoGatewayService) chargeableVideoCost(task *VideoTask) float64 {
	if task == nil || task.Status != VideoStatusSucceeded {
		return 0
	}
	if task.Provider == VideoProviderSeedance {
		return s.calculateVideoActualCost(task)
	}
	if task.CostEstimate > 0 {
		return task.CostEstimate
	}
	return s.estimateVideoCost(task)
}

// chargeForVideo deducts the USD user balance for a delivered video exactly once.
// Seedance provider-usage prices are stored in CNY and converted through the
// configured usd_cny_rate (default 7.20) before deducting users.balance.
// Billing errors are logged, never fatal (the video is already delivered).
func (s *VideoGatewayService) chargeForVideo(ctx context.Context, task *VideoTask) {
	if task == nil || task.Status != VideoStatusSucceeded || task.Provider == VideoProviderMock {
		return
	}
	amountUSD, snapshot, err := s.videoTaskPricing().ActualPrice(ctx, task)
	if err != nil {
		slog.Warn("video_gateway: calculate Money charge failed", "task_id", task.ID, "error", err)
		return
	}
	s.chargeForVideoMoney(ctx, task, amountUSD, snapshot)
}

// chargeForVideoMoney keeps the new billing input decimal-safe. Conversion to
// float64 occurs only at the legacy UserRepository/cache/budget adapter calls.
func (s *VideoGatewayService) chargeForVideoMoney(ctx context.Context, task *VideoTask, amountUSD Money, snapshot PricingSnapshot) {
	// The reliability-core path settles the reservation, immutable ledger and
	// DECIMAL balance inside VideoTaskFinalizer. Reaching the legacy claim-based
	// adapter while the flag is on would reintroduce a second charging path.
	if s.videoReliabilityCoreEnabled() {
		return
	}
	if task == nil || task.Status != VideoStatusSucceeded || task.Provider == VideoProviderMock || !amountUSD.Decimal().IsPositive() {
		return
	}

	claimedAt, claimed, err := s.repo.ClaimVideoBalanceCharge(ctx, task.ID)
	if err != nil {
		slog.Warn("video_gateway: claim balance charge failed", "task_id", task.ID, "error", err)
		return
	}
	if !claimed {
		return
	}

	legacyUSD, err := ProjectVideoMoneyToLegacyFloat(amountUSD)
	if err != nil {
		slog.Warn("video_gateway: project USD charge to legacy repository", "task_id", task.ID, "error", err)
		s.releaseVideoBalanceClaim(ctx, task.ID, claimedAt)
		return
	}
	if s.userRepo != nil {
		if err := s.userRepo.DeductBalance(ctx, task.CreatedBy, legacyUSD); err != nil {
			slog.Warn("video_gateway: user balance deduction failed", "task_id", task.ID, "user_id", task.CreatedBy, "error", err)
			s.releaseVideoBalanceClaim(ctx, task.ID, claimedAt)
			return
		}
		if s.billingCacheService != nil {
			s.billingCacheService.QueueDeductBalance(task.CreatedBy, legacyUSD)
		}
	}

	if s.budget != nil {
		legacyOriginal, projectionErr := ProjectVideoMoneyToLegacyFloat(snapshot.AmountOriginal)
		if projectionErr != nil {
			slog.Warn("video_gateway: project original charge to legacy budget hook", "task_id", task.ID, "error", projectionErr)
			return
		}
		if err := s.budget.Charge(ctx, task.CreatedBy, legacyOriginal, task.ID); err != nil {
			slog.Warn("video_gateway: post-success budget hook failed", "task_id", task.ID, "error", err)
		}
	}
}

func (s *VideoGatewayService) releaseVideoBalanceClaim(ctx context.Context, taskID int64, claimedAt time.Time) {
	if s == nil || s.repo == nil || taskID <= 0 || claimedAt.IsZero() {
		return
	}
	cleared, err := s.repo.ClearVideoBalanceChargeIfClaimedAt(ctx, taskID, claimedAt)
	if err != nil {
		slog.Warn("video_gateway: release balance charge claim failed", "task_id", taskID, "error", err)
		return
	}
	if !cleared {
		slog.Warn("video_gateway: balance charge claim was not released", "task_id", taskID)
	}
}
