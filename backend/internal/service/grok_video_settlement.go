package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	GrokVideoSettlementStatusPending   = "pending"
	GrokVideoSettlementStatusSettled   = "settled"
	GrokVideoSettlementStatusFailed    = "failed"
	GrokVideoSettlementStatusExpired   = "expired"
	GrokVideoSettlementStatusCancelled = "cancelled"

	GrokVideoUpstreamStatusPending = "pending"
	GrokVideoUpstreamStatusDone    = "done"
	GrokVideoUpstreamStatusFailed  = "failed"
	GrokVideoUpstreamStatusExpired = "expired"

	GrokVideoPricingSnapshotVersion   = 1
	GrokVideoPricingBasisVideoSecond  = "video_second"
	GrokVideoPricingBasisFixedRequest = "fixed_request"
	GrokVideoPricingBasisToken        = "token"
)

var (
	ErrGrokVideoSettlementNotFound      = errors.New("grok video settlement not found")
	ErrGrokVideoSettlementConflict      = errors.New("grok video settlement fingerprint conflict")
	ErrGrokVideoSettlementNotPending    = errors.New("grok video settlement is not pending")
	ErrGrokVideoTerminalConflict        = errors.New("grok video settlement terminal status conflict")
	ErrGrokVideoTokenPricingUnsupported = errors.New("token-priced grok video cannot be deferred without billable usage")
)

// GrokVideoSettlement is the immutable billing snapshot captured when xAI
// accepts an asynchronous video request.
type GrokVideoSettlement struct {
	ID                        int64
	RequestID                 string
	RequestFingerprint        string
	UserID                    int64
	APIKeyID                  int64
	GroupID                   *int64
	AccountID                 int64
	AccountType               string
	SubscriptionID            *int64
	RequestedModel            string
	BillingModel              string
	UpstreamModel             string
	Usage                     OpenAIUsage
	PricingSnapshotVersion    int
	PricingBasis              string
	BillingMode               string
	BillingType               int8
	InputCost                 float64
	ImageInputCost            float64
	OutputCost                float64
	ImageOutputCost           float64
	CacheCreationCost         float64
	CacheReadCost             float64
	TotalCost                 float64
	ActualCost                float64
	RateMultiplier            float64
	AccountRateMultiplier     float64
	LongContextBillingApplied bool
	AccountStatsCost          *float64
	VideoResolution           string
	VideoDurationSeconds      int
	RequestDuration           time.Duration
	RequestPayloadHash        string
	InboundEndpoint           string
	UpstreamEndpoint          string
	UserAgent                 string
	IPAddress                 string
	QuotaPlatform             string
	ChannelUsageFields
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	TerminalAt *time.Time
	SettledAt  *time.Time
}

func (s *GrokVideoSettlement) Normalize() {
	if s == nil {
		return
	}
	s.RequestID = strings.TrimSpace(s.RequestID)
	s.AccountType = strings.TrimSpace(s.AccountType)
	s.RequestedModel = strings.TrimSpace(s.RequestedModel)
	s.BillingModel = strings.TrimSpace(s.BillingModel)
	if s.BillingModel == "" {
		s.BillingModel = s.RequestedModel
	}
	s.UpstreamModel = strings.TrimSpace(s.UpstreamModel)
	s.PricingBasis = strings.TrimSpace(s.PricingBasis)
	s.BillingMode = strings.TrimSpace(s.BillingMode)
	s.VideoResolution = NormalizeVideoBillingResolutionOrDefault(s.VideoResolution)
	s.VideoDurationSeconds = NormalizeVideoBillingDurationSecondsOrDefault(s.VideoDurationSeconds)
	s.RequestPayloadHash = strings.TrimSpace(s.RequestPayloadHash)
	s.InboundEndpoint = strings.TrimSpace(s.InboundEndpoint)
	s.UpstreamEndpoint = strings.TrimSpace(s.UpstreamEndpoint)
	s.UserAgent = strings.TrimSpace(s.UserAgent)
	s.IPAddress = strings.TrimSpace(s.IPAddress)
	s.QuotaPlatform = strings.TrimSpace(s.QuotaPlatform)
	s.ChannelMappedModel = strings.TrimSpace(s.ChannelMappedModel)
	s.BillingModelSource = strings.TrimSpace(s.BillingModelSource)
	s.ModelMappingChain = strings.TrimSpace(s.ModelMappingChain)
	if strings.TrimSpace(s.Status) == "" {
		s.Status = GrokVideoSettlementStatusPending
	}
	if s.RequestFingerprint == "" {
		s.RequestFingerprint = grokVideoSettlementFingerprint(s)
	}
}

func grokVideoSettlementFingerprint(s *GrokVideoSettlement) string {
	if s == nil {
		return ""
	}
	payload := struct {
		RequestID                   string
		UserID                      int64
		APIKeyID                    int64
		GroupID                     *int64
		AccountID                   int64
		AccountType                 string
		SubscriptionID              *int64
		RequestedModel              string
		BillingModel                string
		UpstreamModel               string
		Usage                       OpenAIUsage
		PricingSnapshotVersion      int
		PricingBasis                string
		BillingMode                 string
		BillingType                 int8
		InputCost                   float64
		ImageInputCost              float64
		OutputCost                  float64
		ImageOutputCost             float64
		CacheCreationCost           float64
		CacheReadCost               float64
		TotalCost                   float64
		ActualCost                  float64
		RateMultiplier              float64
		AccountRateMultiplier       float64
		LongContextBillingApplied   bool
		AccountStatsCost            *float64
		VideoResolution             string
		VideoDurationSeconds        int
		RequestDurationMilliseconds int64
		RequestPayloadHash          string
		InboundEndpoint             string
		UpstreamEndpoint            string
		UserAgent                   string
		IPAddress                   string
		QuotaPlatform               string
		ChannelUsageFields          ChannelUsageFields
	}{
		RequestID: s.RequestID, UserID: s.UserID, APIKeyID: s.APIKeyID, GroupID: s.GroupID,
		AccountID: s.AccountID, AccountType: s.AccountType, SubscriptionID: s.SubscriptionID,
		RequestedModel: s.RequestedModel, BillingModel: s.BillingModel, UpstreamModel: s.UpstreamModel,
		Usage: s.Usage, PricingSnapshotVersion: s.PricingSnapshotVersion, PricingBasis: s.PricingBasis,
		BillingMode: s.BillingMode, BillingType: s.BillingType, InputCost: s.InputCost,
		ImageInputCost: s.ImageInputCost, OutputCost: s.OutputCost, ImageOutputCost: s.ImageOutputCost,
		CacheCreationCost: s.CacheCreationCost, CacheReadCost: s.CacheReadCost, TotalCost: s.TotalCost,
		ActualCost: s.ActualCost, RateMultiplier: s.RateMultiplier,
		AccountRateMultiplier: s.AccountRateMultiplier, LongContextBillingApplied: s.LongContextBillingApplied,
		AccountStatsCost: s.AccountStatsCost, VideoResolution: s.VideoResolution,
		VideoDurationSeconds: s.VideoDurationSeconds, RequestDurationMilliseconds: s.RequestDuration.Milliseconds(),
		RequestPayloadHash: s.RequestPayloadHash, InboundEndpoint: s.InboundEndpoint,
		UpstreamEndpoint: s.UpstreamEndpoint, UserAgent: s.UserAgent, IPAddress: s.IPAddress,
		QuotaPlatform: s.QuotaPlatform, ChannelUsageFields: s.ChannelUsageFields,
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

type DeferredUsageBillingSnapshot struct {
	Version               int
	PricingBasis          string
	Cost                  CostBreakdown
	BillingType           int8
	RateMultiplier        float64
	AccountRateMultiplier float64
	AccountStatsCost      *float64
}

func (s *GrokVideoSettlement) DeferredBillingSnapshot(actualDurationSeconds int) *DeferredUsageBillingSnapshot {
	if s == nil || s.PricingSnapshotVersion <= 0 || strings.TrimSpace(s.PricingBasis) == "" {
		return nil
	}
	cost := CostBreakdown{
		InputCost: s.InputCost, ImageInputCost: s.ImageInputCost, OutputCost: s.OutputCost,
		ImageOutputCost: s.ImageOutputCost, CacheCreationCost: s.CacheCreationCost,
		CacheReadCost: s.CacheReadCost, TotalCost: s.TotalCost, ActualCost: s.ActualCost,
		BillingMode: s.BillingMode, LongContextBillingApplied: s.LongContextBillingApplied,
	}
	if s.PricingBasis == GrokVideoPricingBasisVideoSecond && actualDurationSeconds > 0 && s.VideoDurationSeconds > 0 {
		factor := float64(NormalizeVideoBillingDurationSecondsOrDefault(actualDurationSeconds)) / float64(s.VideoDurationSeconds)
		cost.InputCost *= factor
		cost.ImageInputCost *= factor
		cost.OutputCost *= factor
		cost.ImageOutputCost *= factor
		cost.CacheCreationCost *= factor
		cost.CacheReadCost *= factor
		cost.TotalCost *= factor
		cost.ActualCost *= factor
	}
	var accountStatsCost *float64
	if s.AccountStatsCost != nil {
		value := *s.AccountStatsCost
		accountStatsCost = &value
	}
	return &DeferredUsageBillingSnapshot{
		Version: s.PricingSnapshotVersion, PricingBasis: s.PricingBasis, Cost: cost,
		BillingType: s.BillingType, RateMultiplier: s.RateMultiplier,
		AccountRateMultiplier: s.AccountRateMultiplier, AccountStatsCost: accountStatsCost,
	}
}

// GrokVideoBillingRequestID produces a stable, usage-log-safe idempotency key.
func GrokVideoBillingRequestID(requestID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(requestID)))
	return "grok-video:" + hex.EncodeToString(sum[:])[:52]
}

type GrokVideoSettlementRepository interface {
	CreateGrokVideoSettlement(ctx context.Context, settlement *GrokVideoSettlement) (*GrokVideoSettlement, error)
	GetGrokVideoSettlementForOwner(ctx context.Context, requestID string, userID, apiKeyID int64) (*GrokVideoSettlement, error)
	MarkGrokVideoSettlementTerminal(ctx context.Context, id int64, status string) error
	MarkGrokVideoSettlementSettled(ctx context.Context, id int64) error
}

type GrokVideoStatusSettlementInput struct {
	RequestID             string
	Status                string
	ActualDurationSeconds int
	APIKey                *APIKey
	Account               *Account
	APIKeyService         APIKeyQuotaUpdater
}

func NormalizeGrokVideoUpstreamStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending", "queued", "in_progress", "processing":
		return GrokVideoUpstreamStatusPending
	case "done", "completed", "complete", "succeeded", "success":
		return GrokVideoUpstreamStatusDone
	case "failed", "failure", "error":
		return GrokVideoUpstreamStatusFailed
	case "expired":
		return GrokVideoUpstreamStatusExpired
	case "cancelled", "canceled":
		return GrokVideoSettlementStatusCancelled
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func (s *OpenAIGatewayService) PrepareGrokVideoSettlement(
	ctx context.Context,
	settlement *GrokVideoSettlement,
	apiKey *APIKey,
	account *Account,
	subscription *UserSubscription,
) error {
	if s == nil || s.grokVideoSettlementRepo == nil {
		return errors.New("grok video settlement repository is unavailable")
	}
	if settlement == nil || apiKey == nil || apiKey.User == nil || account == nil {
		return errors.New("grok video settlement registration is incomplete")
	}
	if account.Platform != PlatformGrok {
		return errors.New("grok video settlement account platform mismatch")
	}
	if subscription != nil && (subscription.UserID != apiKey.User.ID || apiKey.GroupID == nil || subscription.GroupID != *apiKey.GroupID) {
		return errors.New("grok video settlement subscription owner mismatch")
	}
	settlement.UserID = apiKey.User.ID
	settlement.APIKeyID = apiKey.ID
	settlement.GroupID = cloneInt64Ptr(apiKey.GroupID)
	settlement.AccountID = account.ID
	settlement.AccountType = account.Type
	settlement.SubscriptionID = nil
	if subscription != nil {
		settlement.SubscriptionID = cloneInt64Ptr(&subscription.ID)
	}
	settlement.Normalize()
	if settlement.UserID <= 0 || settlement.APIKeyID <= 0 || settlement.AccountID <= 0 || settlement.RequestedModel == "" {
		return errors.New("grok video settlement is incomplete")
	}
	if settlement.PricingSnapshotVersion == 0 {
		if err := s.captureGrokVideoBillingSnapshot(ctx, settlement, apiKey, account, subscription); err != nil {
			return err
		}
	} else if settlement.PricingSnapshotVersion != GrokVideoPricingSnapshotVersion {
		return errors.New("grok video pricing snapshot version is unsupported")
	}
	settlement.RequestFingerprint = ""
	return nil
}

func (s *OpenAIGatewayService) RegisterGrokVideoSettlement(
	ctx context.Context,
	settlement *GrokVideoSettlement,
	apiKey *APIKey,
	account *Account,
	subscription *UserSubscription,
) error {
	if err := s.PrepareGrokVideoSettlement(ctx, settlement, apiKey, account, subscription); err != nil {
		return err
	}
	if settlement.RequestID == "" {
		return errors.New("grok video settlement request id is required")
	}
	settlement.Normalize()
	_, err := s.grokVideoSettlementRepo.CreateGrokVideoSettlement(ctx, settlement)
	return err
}

func (s *OpenAIGatewayService) SettleGrokVideoStatus(ctx context.Context, input GrokVideoStatusSettlementInput) error {
	if s == nil || s.grokVideoSettlementRepo == nil {
		return errors.New("grok video settlement repository is unavailable")
	}
	if input.APIKey == nil || input.APIKey.User == nil || input.Account == nil {
		return errors.New("grok video settlement billing context is incomplete")
	}
	status := NormalizeGrokVideoUpstreamStatus(input.Status)
	if status == "" || status == GrokVideoUpstreamStatusPending {
		return nil
	}
	switch status {
	case GrokVideoUpstreamStatusDone, GrokVideoUpstreamStatusFailed, GrokVideoUpstreamStatusExpired, GrokVideoSettlementStatusCancelled:
	default:
		return nil
	}

	settlement, err := s.grokVideoSettlementRepo.GetGrokVideoSettlementForOwner(
		ctx,
		input.RequestID,
		input.APIKey.User.ID,
		input.APIKey.ID,
	)
	if err != nil {
		return err
	}
	if settlement.AccountID != input.Account.ID {
		return errors.New("grok video settlement account mismatch")
	}
	if settlement.AccountType != "" && settlement.AccountType != input.Account.Type {
		return errors.New("grok video settlement account type mismatch")
	}
	if settlement.Status != GrokVideoSettlementStatusPending {
		return validateGrokVideoTerminalObservation(settlement.Status, status)
	}

	switch status {
	case GrokVideoUpstreamStatusFailed, GrokVideoUpstreamStatusExpired, GrokVideoSettlementStatusCancelled:
		err := s.grokVideoSettlementRepo.MarkGrokVideoSettlementTerminal(ctx, settlement.ID, status)
		if !errors.Is(err, ErrGrokVideoSettlementNotPending) {
			return err
		}
		current, getErr := s.grokVideoSettlementRepo.GetGrokVideoSettlementForOwner(
			ctx, settlement.RequestID, settlement.UserID, settlement.APIKeyID,
		)
		if getErr != nil {
			return err
		}
		return validateGrokVideoTerminalObservation(current.Status, status)
	case GrokVideoUpstreamStatusDone:
		// Continue below.
	}
	frozenBilling := settlement.DeferredBillingSnapshot(input.ActualDurationSeconds)
	if frozenBilling == nil && !sameGrokVideoSettlementGroup(settlement.GroupID, input.APIKey.GroupID) {
		return errors.New("grok video settlement group mismatch")
	}
	if frozenBilling != nil {
		switch frozenBilling.BillingType {
		case BillingTypeBalance:
		case BillingTypeSubscription:
			if settlement.SubscriptionID == nil {
				return errors.New("grok video subscription billing snapshot has no subscription")
			}
		default:
			return errors.New("grok video billing snapshot has invalid billing type")
		}
	}
	if s.usageBillingRepo == nil {
		return errors.New("grok video settlement requires idempotent usage billing")
	}

	var subscription *UserSubscription
	if settlement.SubscriptionID != nil {
		if s.userSubRepo == nil {
			return errors.New("grok video settlement subscription repository is unavailable")
		}
		subscription, err = s.userSubRepo.GetByIDIncludeDeleted(ctx, *settlement.SubscriptionID)
		if err != nil {
			return fmt.Errorf("load grok video settlement subscription: %w", err)
		}
	}

	durationSeconds := settlement.VideoDurationSeconds
	if input.ActualDurationSeconds > 0 {
		durationSeconds = NormalizeVideoBillingDurationSecondsOrDefault(input.ActualDurationSeconds)
	}
	result := &OpenAIForwardResult{
		RequestID:            settlement.RequestID,
		ResponseID:           settlement.RequestID,
		Usage:                settlement.Usage,
		Model:                settlement.RequestedModel,
		BillingModel:         settlement.BillingModel,
		UpstreamModel:        settlement.UpstreamModel,
		Duration:             settlement.RequestDuration,
		ImageCount:           1,
		VideoCount:           1,
		VideoResolution:      settlement.VideoResolution,
		VideoDurationSeconds: durationSeconds,
	}
	billingAPIKey := input.APIKey
	if frozenBilling != nil {
		clone := *input.APIKey
		clone.GroupID = cloneInt64Ptr(settlement.GroupID)
		clone.Group = nil
		billingAPIKey = &clone
	}
	if err := s.RecordUsage(ctx, &OpenAIRecordUsageInput{
		Result:                result,
		BillingRequestID:      GrokVideoBillingRequestID(settlement.RequestID),
		GrokVideoSettlementID: settlement.ID,
		FrozenBilling:         frozenBilling,
		BillingGroupID:        cloneInt64Ptr(settlement.GroupID),
		APIKey:                billingAPIKey,
		User:                  input.APIKey.User,
		Account:               input.Account,
		Subscription:          subscription,
		InboundEndpoint:       settlement.InboundEndpoint,
		UpstreamEndpoint:      settlement.UpstreamEndpoint,
		UserAgent:             settlement.UserAgent,
		IPAddress:             settlement.IPAddress,
		RequestPayloadHash:    settlement.RequestPayloadHash,
		APIKeyService:         input.APIKeyService,
		QuotaPlatform:         settlement.QuotaPlatform,
		ChannelUsageFields:    settlement.ChannelUsageFields,
	}); err != nil {
		if errors.Is(err, ErrUsageBillingRequestConflict) || errors.Is(err, ErrGrokVideoSettlementNotPending) {
			current, getErr := s.grokVideoSettlementRepo.GetGrokVideoSettlementForOwner(
				ctx, settlement.RequestID, settlement.UserID, settlement.APIKeyID,
			)
			if getErr == nil {
				if terminalErr := validateGrokVideoTerminalObservation(current.Status, status); terminalErr == nil {
					return nil
				}
			}
		}
		return err
	}
	return s.grokVideoSettlementRepo.MarkGrokVideoSettlementSettled(ctx, settlement.ID)
}

func validateGrokVideoTerminalObservation(storedStatus, observedStatus string) error {
	storedStatus = strings.ToLower(strings.TrimSpace(storedStatus))
	observedStatus = NormalizeGrokVideoUpstreamStatus(observedStatus)
	if observedStatus == GrokVideoUpstreamStatusDone && storedStatus == GrokVideoSettlementStatusSettled {
		return nil
	}
	if observedStatus != GrokVideoUpstreamStatusDone && storedStatus == observedStatus {
		return nil
	}
	return fmt.Errorf("%w: stored=%s observed=%s", ErrGrokVideoTerminalConflict, storedStatus, observedStatus)
}

func sameGrokVideoSettlementGroup(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
