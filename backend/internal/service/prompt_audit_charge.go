package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// PromptAuditCharge collects the prompt-audit fees of one client request until
// they are billed. Every audit adds its fee; a usage record takes what has
// accumulated since the previous one. WebSocket turns share one request, and
// each turn is audited before it is forwarded and billed after, so each usage
// record carries the fee of its own turn. Fees no usage record takes, because
// the upstream call failed, the client left or no usage was recorded, are
// billed on their own when the request ends (Finish).
type PromptAuditCharge struct {
	mu       sync.Mutex
	pending  float64
	sequence int
	unbilled func(ctx context.Context, cost float64) error
}

func NewPromptAuditCharge() *PromptAuditCharge { return &PromptAuditCharge{} }

// Add records the fee of one audit whose request goes on to the upstream, and
// how to bill it by itself if no usage record takes it.
func (c *PromptAuditCharge) Add(cost float64, chargeUnbilled func(ctx context.Context, cost float64) error) {
	if c == nil || cost <= 0 {
		return
	}
	c.mu.Lock()
	c.pending += cost
	if chargeUnbilled != nil {
		c.unbilled = chargeUnbilled
	}
	c.mu.Unlock()
}

// Take returns the fees added since the previous Take and clears them.
func (c *PromptAuditCharge) Take() float64 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	cost := c.pending
	c.pending = 0
	return cost
}

// Finish bills the fees that no usage record took. The caller runs it once the
// request has ended, so any usage record has already taken its fee.
func (c *PromptAuditCharge) Finish(ctx context.Context) (float64, error) {
	if c == nil {
		return 0, nil
	}
	c.mu.Lock()
	cost, charge := c.pending, c.unbilled
	c.pending = 0
	c.mu.Unlock()
	if cost <= 0 {
		return 0, nil
	}
	if charge == nil {
		return cost, errors.New("prompt audit fee has no charger")
	}
	return cost, charge(ctx, cost)
}

// nextSequence numbers the separate charges of one request, so each blocked
// WebSocket turn and the unbilled fees get their own billing request IDs.
func (c *PromptAuditCharge) nextSequence() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sequence++
	return c.sequence
}

type promptAuditChargeKey struct{}
type promptAuditCostKey struct{}

func WithPromptAuditCharge(ctx context.Context, charge *PromptAuditCharge) context.Context {
	return context.WithValue(ctx, promptAuditChargeKey{}, charge)
}

func PromptAuditChargeFromContext(ctx context.Context) *PromptAuditCharge {
	if ctx == nil {
		return nil
	}
	charge, _ := ctx.Value(promptAuditChargeKey{}).(*PromptAuditCharge)
	return charge
}

// WithPromptAuditCost fixes the audit fee a usage record bills. Usage records
// are billed by workers on a background context, so the fee is taken from the
// request's PromptAuditCharge when the record is submitted.
func WithPromptAuditCost(ctx context.Context, cost float64) context.Context {
	return context.WithValue(ctx, promptAuditCostKey{}, cost)
}

// PromptAuditCostFromContext returns the fee fixed for a usage record by
// WithPromptAuditCost.
func PromptAuditCostFromContext(ctx context.Context) (float64, bool) {
	if ctx == nil {
		return 0, false
	}
	cost, ok := ctx.Value(promptAuditCostKey{}).(float64)
	return cost, ok
}

// promptAuditCostForBilling returns the fee fixed for this usage record, or
// takes the pending fee when the record is billed on the request context.
func promptAuditCostForBilling(ctx context.Context) float64 {
	if cost, ok := PromptAuditCostFromContext(ctx); ok {
		return cost
	}
	return PromptAuditChargeFromContext(ctx).Take()
}

// PromptAuditFeeChargeInput describes an audit fee no usage record carries:
// the audit stopped the request, or the request ended without a usage record.
type PromptAuditFeeChargeInput struct {
	RequestID     string
	Charge        *PromptAuditCharge
	Cost          float64
	APIKey        *APIKey
	User          *User
	Subscription  *UserSubscription
	APIKeyService APIKeyQuotaUpdater
	Platform      string
}

// chargePromptAuditFee bills the fee at its list price through the same
// idempotent billing transaction as usage records: from the subscription when
// the group bills by subscription, otherwise from the balance, and against the
// API key's quota and rate limits.
func chargePromptAuditFee(ctx context.Context, input PromptAuditFeeChargeInput, deps *billingDeps, repo UsageBillingRepository) error {
	if input.Cost <= 0 || input.APIKey == nil || input.User == nil || deps == nil || repo == nil {
		return nil
	}
	requestID := strings.TrimSpace(input.RequestID)
	if requestID == "" {
		return ErrUsageBillingRequestIDRequired
	}
	sequence := 1
	if input.Charge != nil {
		sequence = input.Charge.nextSequence()
	}
	cost := &CostBreakdown{ActualCost: input.Cost}
	p := &postUsageBillingParams{
		Cost: cost, User: input.User, APIKey: input.APIKey, Subscription: input.Subscription,
		IsSubscriptionBill: input.Subscription != nil && input.APIKey.Group != nil && input.APIKey.Group.IsSubscriptionType(),
		APIKeyService:      input.APIKeyService, Platform: input.Platform, PromptAuditCost: input.Cost,
	}
	cmd := &UsageBillingCommand{
		RequestID: fmt.Sprintf("prompt_audit:%s:%d", requestID, sequence),
		APIKeyID:  input.APIKey.ID, UserID: input.User.ID, Model: "prompt-audit",
	}
	if p.IsSubscriptionBill && p.Subscription != nil {
		cmd.SubscriptionID = &p.Subscription.ID
		cmd.SubscriptionCost = input.Cost
	} else {
		cmd.BalanceCost = input.Cost
	}
	if p.shouldDeductAPIKeyQuota() {
		cmd.APIKeyQuotaCost = input.Cost
	}
	if p.shouldUpdateRateLimits() {
		cmd.APIKeyRateLimitCost = input.Cost
	}
	cmd.Normalize()

	applyCtx, cancelApply := detachedBillingContext(ctx)
	result, err := repo.Apply(applyCtx, cmd)
	cancelApply()
	if err != nil || result == nil || !result.Applied {
		return err
	}
	billingCtx, cancel := detachedBillingContext(ctx)
	defer cancel()
	if result.APIKeyQuotaExhausted {
		if invalidator, ok := p.APIKeyService.(apiKeyAuthCacheInvalidator); ok && p.APIKey.Key != "" {
			invalidator.InvalidateAuthCacheByKey(billingCtx, p.APIKey.Key)
		}
	}
	syncPromptAuditChargeCaches(billingCtx, p, deps, result)
	return nil
}

// syncPromptAuditChargeCaches mirrors the cache updates finalizePostUsageBilling
// makes after a usage charge; the fee is not tied to an upstream account.
func syncPromptAuditChargeCaches(ctx context.Context, p *postUsageBillingParams, deps *billingDeps, result *UsageBillingApplyResult) {
	if deps.billingCacheService == nil {
		return
	}
	if p.IsSubscriptionBill {
		if p.APIKey.GroupID != nil {
			deps.billingCacheService.QueueUpdateSubscriptionUsage(p.User.ID, *p.APIKey.GroupID, p.Cost.ActualCost)
		}
	} else {
		syncBalanceCacheAfterDeduction(ctx, p, deps, result)
		if p.Platform != "" && deps.userPlatformQuotaRepo != nil && deps.billingCacheService.HasUserPlatformQuotaLimit(ctx, p.User.ID, p.Platform) {
			deps.billingCacheService.IncrementUserPlatformQuotaUsage(p.User.ID, p.Platform, p.Cost.ActualCost)
			if deps.cfg == nil || !deps.cfg.Database.UserPlatformQuotaFlusherEnabled {
				persistPostUsagePlatformQuota(ctx, deps.userPlatformQuotaRepo, p.User.ID, p.Platform, p.Cost.ActualCost)
			}
		}
	}
	if p.APIKey.HasRateLimits() {
		deps.billingCacheService.QueueUpdateAPIKeyRateLimitUsage(p.APIKey.ID, p.Cost.ActualCost)
	}
}

// ChargePromptAuditFee bills an audit fee that no usage record carries.
func (s *GatewayService) ChargePromptAuditFee(ctx context.Context, input PromptAuditFeeChargeInput) error {
	return chargePromptAuditFee(ctx, input, s.billingDeps(), s.usageBillingRepo)
}

// ChargePromptAuditFee bills an audit fee that no usage record carries.
func (s *OpenAIGatewayService) ChargePromptAuditFee(ctx context.Context, input PromptAuditFeeChargeInput) error {
	return chargePromptAuditFee(ctx, input, s.billingDeps(), s.usageBillingRepo)
}
