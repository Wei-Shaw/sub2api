package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

const (
	OpenAIResetCreditExpiryTargetDefaultLeadTimeMinutes = 60
	OpenAIResetCreditExpiryTargetMinLeadTimeMinutes     = 5
	OpenAIResetCreditExpiryTargetMaxLeadTimeMinutes     = 7 * 24 * 60
)

type accountExtraCompareAndSwapper interface {
	CompareAndSwapExtra(ctx context.Context, id int64, key string, expected any, updates map[string]any) (bool, error)
}

func compareAndSwapAccountExtra(ctx context.Context, repo AccountRepository, id int64, key string, expected any, updates map[string]any) (bool, error) {
	cas, ok := repo.(accountExtraCompareAndSwapper)
	if !ok {
		return false, infraerrors.New(http.StatusInternalServerError, "ACCOUNT_EXTRA_CAS_UNAVAILABLE", "account repository does not support conditional extra updates")
	}
	return cas.CompareAndSwapExtra(ctx, id, key, expected, updates)
}

// OpenAIResetCreditExpiryTarget is one versioned authorization to consume a
// reset credit. PlanID prevents an older worker from clearing a replacement.
type OpenAIResetCreditExpiryTarget struct {
	PlanID          string `json:"plan_id"`
	CreditID        string `json:"credit_id"`
	ExpiresAt       string `json:"expires_at"`
	LeadTimeMinutes int    `json:"lead_time_minutes"`
}

func ResolveOpenAIResetCreditExpiryTarget(account *Account) *OpenAIResetCreditExpiryTarget {
	if !isOpenAIAutoResetCreditAccount(account) || len(account.Extra) == 0 {
		return nil
	}
	raw, ok := account.Extra[OpenAIAutoResetCreditExpiryTargetExtraKey]
	if !ok || raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var target OpenAIResetCreditExpiryTarget
	if err := json.Unmarshal(encoded, &target); err != nil || !validOpenAIResetCreditExpiryTarget(&target) {
		return nil
	}
	return &target
}

func (s *OpenAIQuotaService) SetResetCreditExpiryTarget(ctx context.Context, accountID int64, creditID string, leadTimeMinutes int) (*Account, error) {
	creditID = strings.TrimSpace(creditID)
	if creditID == "" {
		return nil, infraerrors.BadRequest("OPENAI_RESET_CREDIT_ID_INVALID", "credit_id is required")
	}
	if leadTimeMinutes < OpenAIResetCreditExpiryTargetMinLeadTimeMinutes || leadTimeMinutes > OpenAIResetCreditExpiryTargetMaxLeadTimeMinutes {
		return nil, infraerrors.BadRequest("OPENAI_RESET_CREDIT_LEAD_TIME_INVALID", "lead_time_minutes must be between 5 and 10080")
	}
	account, err := s.loadResetCreditExpiryTargetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	current := ResolveOpenAIResetCreditExpiryTarget(account)
	if current != nil && current.CreditID != creditID {
		return nil, infraerrors.Conflict("OPENAI_RESET_CREDIT_EXPIRY_TARGET_EXISTS", "another reset credit already has a scheduled-use plan; cancel it before scheduling a different credit")
	}
	snapshot := resolveOpenAIResetCreditSnapshot(account)
	candidate, ok := findOpenAIResetCreditByID(snapshot, creditID)
	if !ok {
		return nil, infraerrors.Conflict("OPENAI_RESET_CREDIT_TARGET_UNAVAILABLE", "the selected reset credit is not present in the latest complete snapshot; refresh the account quota")
	}
	expiresAt, err := time.Parse(time.RFC3339, candidate.ExpiresAt)
	if err != nil {
		return nil, infraerrors.Conflict("OPENAI_RESET_CREDIT_EXPIRY_INVALID", "the selected reset credit has an invalid expiration")
	}
	now := time.Now().UTC()
	if !expiresAt.After(now) {
		return nil, infraerrors.Conflict("OPENAI_RESET_CREDIT_EXPIRED", "the selected reset credit has expired")
	}

	target := &OpenAIResetCreditExpiryTarget{
		PlanID:          uuid.NewString(),
		CreditID:        creditID,
		ExpiresAt:       expiresAt.UTC().Format(time.RFC3339Nano),
		LeadTimeMinutes: leadTimeMinutes,
	}
	updates := map[string]any{OpenAIAutoResetCreditExpiryTargetExtraKey: target}
	if state := openAIAutoResetStateFromExtra(account.Extra); state != nil && resetOpenAIAutoResetExpiryState(state, snapshot.AvailableCount, now) {
		updates[OpenAIAutoResetCreditStateExtraKey] = state
	}
	var expected any
	if current != nil {
		expected = current
	}
	swapped, err := compareAndSwapAccountExtra(ctx, s.accountRepo, accountID, OpenAIAutoResetCreditExpiryTargetExtraKey, expected, updates)
	if err != nil {
		return nil, err
	}
	if !swapped {
		return nil, infraerrors.Conflict("OPENAI_RESET_CREDIT_EXPIRY_TARGET_CHANGED", "the scheduled-use plan changed; refresh and try again")
	}
	notifyOpenAIAutoReset(accountID)
	return s.accountRepo.GetByID(ctx, accountID)
}

func (s *OpenAIQuotaService) CancelResetCreditExpiryTarget(ctx context.Context, accountID int64) (*Account, error) {
	account, err := s.loadResetCreditExpiryTargetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	current := ResolveOpenAIResetCreditExpiryTarget(account)
	if current == nil {
		return account, nil
	}
	updates := map[string]any{OpenAIAutoResetCreditExpiryTargetExtraKey: nil}
	if state := openAIAutoResetStateFromExtra(account.Extra); state != nil && resetOpenAIAutoResetExpiryState(state, state.AvailableCount, time.Now().UTC()) {
		updates[OpenAIAutoResetCreditStateExtraKey] = state
	}
	swapped, err := compareAndSwapAccountExtra(ctx, s.accountRepo, accountID, OpenAIAutoResetCreditExpiryTargetExtraKey, current, updates)
	if err != nil {
		return nil, err
	}
	if !swapped {
		return nil, infraerrors.Conflict("OPENAI_RESET_CREDIT_EXPIRY_TARGET_CHANGED", "the scheduled-use plan changed; refresh and try again")
	}
	notifyOpenAIAutoReset(accountID)
	return s.accountRepo.GetByID(ctx, accountID)
}

func (s *OpenAIQuotaService) loadResetCreditExpiryTargetAccount(ctx context.Context, accountID int64) (*Account, error) {
	if s == nil || s.accountRepo == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_QUOTA_NOT_CONFIGURED", "openai quota service is not configured")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return nil, infraerrors.New(http.StatusNotFound, "OPENAI_QUOTA_ACCOUNT_NOT_FOUND", "account not found").WithCause(err)
	}
	if !isOpenAIAutoResetCreditAccount(account) {
		return nil, infraerrors.BadRequest("OPENAI_RESET_CREDIT_EXPIRY_TARGET_ACCOUNT_INVALID", "expiry targeting is only supported for OpenAI OAuth parent accounts")
	}
	return account, nil
}

func resolveOpenAIResetCreditSnapshot(account *Account) *OpenAIRateLimitResetCredits {
	if account == nil || len(account.Extra) == 0 {
		return nil
	}
	raw, ok := account.Extra[openaiQuotaResetCreditsKey]
	if !ok || raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var snapshot OpenAIRateLimitResetCredits
	if err := json.Unmarshal(encoded, &snapshot); err != nil || !completeOpenAIResetCreditSnapshot(&snapshot) {
		return nil
	}
	return &snapshot
}

func findOpenAIResetCreditByID(snapshot *OpenAIRateLimitResetCredits, creditID string) (OpenAIRateLimitResetCreditDetail, bool) {
	if snapshot == nil {
		return OpenAIRateLimitResetCreditDetail{}, false
	}
	for _, credit := range snapshot.Credits {
		if credit.ID == creditID {
			return credit, true
		}
	}
	return OpenAIRateLimitResetCreditDetail{}, false
}

func validOpenAIResetCreditExpiryTarget(target *OpenAIResetCreditExpiryTarget) bool {
	if target == nil || strings.TrimSpace(target.CreditID) == "" {
		return false
	}
	if _, err := uuid.Parse(target.PlanID); err != nil {
		return false
	}
	if target.LeadTimeMinutes < OpenAIResetCreditExpiryTargetMinLeadTimeMinutes || target.LeadTimeMinutes > OpenAIResetCreditExpiryTargetMaxLeadTimeMinutes {
		return false
	}
	_, err := time.Parse(time.RFC3339, target.ExpiresAt)
	return err == nil
}

func resetOpenAIAutoResetExpiryState(state *OpenAIAutoResetCreditState, available int, now time.Time) bool {
	if state == nil || state.TriggerReason != OpenAIAutoResetTriggerReasonExpiryTarget {
		return false
	}
	state.Status = OpenAIAutoResetStatusNoCredit
	if available > 0 {
		state.Status = OpenAIAutoResetStatusAvailable
	}
	state.TriggerReason = ""
	state.TriggerWindow = ""
	state.AvailableCount = available
	state.CheckedAt = now.UTC().Format(time.RFC3339)
	state.LastResultAt = ""
	state.ErrorCode = ""
	state.AttemptCycleHash = ""
	state.AttemptCreditHash = ""
	return true
}
