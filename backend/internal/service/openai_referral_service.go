package service

import (
	"context"
	"net/http"
	"net/mail"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	openAIReferralSnapshotKey = "codex_referral_snapshot"
	openAIReferralConsumer    = "codex_referral_consumer"
	openAIReferralWorkspace   = "codex_referral_workspace"
)

type OpenAIReferralGrant struct {
	GrantType string  `json:"grant_type"`
	Recipient string  `json:"recipient"`
	Amount    float64 `json:"amount"`
}

type OpenAIReferralEligibility struct {
	ShouldShow                   bool                  `json:"should_show"`
	RemainingSendCapacity        *int                  `json:"remaining_send_capacity"`
	RemainingRewardCapacity      *int                  `json:"remaining_reward_capacity"`
	RequiresExplicitConfirmation *bool                 `json:"requires_explicit_confirmation"`
	OfferID                      string                `json:"offer_id,omitempty"`
	Title                        string                `json:"title,omitempty"`
	Description                  string                `json:"description,omitempty"`
	Rules                        []string              `json:"rules,omitempty"`
	Grants                       []OpenAIReferralGrant `json:"grants,omitempty"`
	ProgramID                    string                `json:"program_id"`
	AvailableInvites             *int                  `json:"available_invites"`
	FetchedAt                    int64                 `json:"fetched_at"`
}

// referralCapacity follows the desktop's send/reward capacity constraints.
// The desktop additionally caps one batch at five; this API sends one email.
func referralCapacity(e *OpenAIReferralEligibility) *int {
	if !e.ShouldShow {
		zero := 0
		return &zero
	}
	if e.RemainingSendCapacity == nil {
		return nil
	}
	count := max(0, *e.RemainingSendCapacity)
	if len(e.Grants) > 0 || (e.OfferID != "" && e.OfferID != "none") {
		if e.RemainingRewardCapacity == nil {
			return nil
		}
		count = min(count, max(0, *e.RemainingRewardCapacity))
	}
	return &count
}

func (s *OpenAIQuotaService) referralAccount(ctx context.Context, id int64, sending bool) (*Account, error) {
	if s == nil || s.accountRepo == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "OPENAI_REFERRAL_NOT_CONFIGURED", "referral service is unavailable")
	}
	a, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, ErrAccountNotFound
	}
	if a.Platform != PlatformOpenAI || a.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_REFERRAL_INVALID_ACCOUNT", "referrals require an OpenAI OAuth account")
	}
	if a.IsShadow() {
		if sending {
			return nil, infraerrors.New(http.StatusConflict, "OPENAI_REFERRAL_SHADOW_ACCOUNT", "send invitations from the parent account")
		}
		return resolveCredentialAccount(ctx, s.accountRepo, a)
	}
	return a, nil
}

func referralProgram(a *Account) string {
	if strings.EqualFold(a.GetCredential("account_type"), "workspace") {
		return openAIReferralWorkspace
	}
	switch strings.ToLower(strings.TrimSpace(a.GetCredential("plan_type"))) {
	case "team", "business", "self_serve_business_prolite", "self_serve_business_usage_based", "free_workspace", "enterprise", "enterprise_cbp_usage_based", "enterprise_cbp_automation", "edu", "education":
		return openAIReferralWorkspace
	default:
		return openAIReferralConsumer
	}
}

func (s *OpenAIQuotaService) referralCall(ctx context.Context, id int64, program string) (OpenAIReferralCall, error) {
	if s.referralClient == nil {
		return OpenAIReferralCall{}, infraerrors.New(http.StatusServiceUnavailable, "OPENAI_REFERRAL_NOT_CONFIGURED", "referral service is unavailable")
	}
	token, accountID, proxy, fedRAMP, err := s.prepareUpstreamCall(ctx, id)
	if err != nil {
		return OpenAIReferralCall{}, err
	}
	headers, _, err := s.buildCodexQuotaHeaders(ctx, id, token, accountID, fedRAMP)
	if err != nil {
		return OpenAIReferralCall{}, infraerrors.New(http.StatusBadGateway, "OPENAI_REFERRAL_AUTH_ERROR", "failed to authenticate referral request")
	}
	return OpenAIReferralCall{ProxyURL: proxy, Headers: headers, ProgramID: program}, nil
}

func (s *OpenAIQuotaService) QueryReferralEligibility(ctx context.Context, id int64) (*OpenAIReferralEligibility, error) {
	account, err := s.referralAccount(ctx, id, false)
	if err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, openaiQuotaUpstreamTimeout)
	defer cancel()
	program := referralProgram(account)
	call, err := s.referralCall(callCtx, id, program)
	if err != nil {
		return nil, err
	}
	result, err := s.referralClient.QueryEligibility(callCtx, call)
	if err != nil {
		return nil, err
	}
	result.ProgramID = program
	result.AvailableInvites = referralCapacity(result)
	result.FetchedAt = time.Now().Unix()
	return result, nil
}

func (s *OpenAIQuotaService) CacheReferralSnapshot(ctx context.Context, id int64, eligibility *OpenAIReferralEligibility) error {
	return s.accountRepo.UpdateExtra(ctx, id, map[string]any{openAIReferralSnapshotKey: eligibility})
}

type OpenAIReferralSendRequest struct {
	Email     string `json:"email"`
	ProgramID string `json:"program_id"`
	Confirmed bool   `json:"confirmed"`
}

type OpenAIReferralSendResult struct {
	Email string `json:"email"`
	Sent  bool   `json:"sent"`
}

func (s *OpenAIQuotaService) SendReferralInvite(ctx context.Context, id int64, input OpenAIReferralSendRequest) (*OpenAIReferralSendResult, error) {
	email := strings.TrimSpace(input.Email)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 || strings.ContainsAny(email, "\r\n") {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_REFERRAL_INVALID_EMAIL", "enter one valid email address")
	}
	account, err := s.referralAccount(ctx, id, true)
	if err != nil {
		return nil, err
	}
	if input.ProgramID != referralProgram(account) {
		return nil, infraerrors.New(http.StatusConflict, "OPENAI_REFERRAL_PROGRAM_CHANGED", "refresh invitation eligibility before sending")
	}
	// Recheck the server's current eligibility rather than trusting a UI cache.
	eligibility, err := s.QueryReferralEligibility(ctx, id)
	if err != nil {
		return nil, err
	}
	if !eligibility.ShouldShow || eligibility.AvailableInvites == nil || *eligibility.AvailableInvites <= 0 {
		return nil, infraerrors.New(http.StatusConflict, "OPENAI_REFERRAL_UNAVAILABLE", "no invitations are currently available")
	}
	if (eligibility.RequiresExplicitConfirmation == nil || *eligibility.RequiresExplicitConfirmation) && !input.Confirmed {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_REFERRAL_CONFIRMATION_REQUIRED", "confirm the recipient's consent before sending")
	}
	callCtx, cancel := context.WithTimeout(ctx, openaiQuotaUpstreamTimeout)
	defer cancel()
	call, err := s.referralCall(callCtx, id, eligibility.ProgramID)
	if err != nil {
		return nil, err
	}
	if err := s.referralClient.SendInvite(callCtx, call, email); err != nil {
		return nil, err
	}
	return &OpenAIReferralSendResult{Email: email, Sent: true}, nil
}
