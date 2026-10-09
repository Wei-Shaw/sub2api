package service

// Manual redemption of Claude native limit resets. The claim protocol (idempotent
// operation, account + organization leases, durable organization fence) is ported
// from upstream PR #7591 by korkin25, adapted so the server alone selects the grant.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

const (
	claudeResetOperationScope = "claude_reset_redeem"
	claudeResetFenceScope     = "claude_reset_org_fence"
	claudeResetProfileURL     = "https://api.anthropic.com/api/oauth/profile"
	claudeResetRedeemURLFmt   = "https://api.anthropic.com/api/organizations/%s/reset_rate_limits"
	claudeResetLeaseTTL       = 90 * time.Second
	claudeResetRecordTTL      = 365 * 24 * time.Hour
	// An unconfirmed claim blocks every further redemption of the organization until
	// the upstream outcome has certainly settled; after that a fresh query is
	// authoritative again (a consumed credit shows up as a lower count or cooldown).
	claudeResetUnknownFenceTTL = 24 * time.Hour
	// An explicit, well-formed "unavailable" answer means nothing was claimed, so it
	// only fences briefly instead of locking the organization out for a day.
	claudeResetUnavailableFenceTTL = 15 * time.Minute
	claudeResetReasonUnavailable   = "upstream_unavailable"

	ClaudeResetOutcomeReset       = "reset"
	ClaudeResetOutcomeAlreadyUsed = "already_used"
	ClaudeResetOutcomeNotLimited  = "not_limited"
	ClaudeResetOutcomeCooldown    = "cooldown"
	ClaudeResetOutcomeIneligible  = "ineligible"
	ClaudeResetOutcomeUnknown     = "unknown"
)

// Only known reason codes and window names reach clients, so an upstream value can
// never echo a grant ID or other identifier.
var (
	claudeResetKnownReasons = map[string]bool{
		"no_grant": true, "unknown_grant": true, "not_next_grant": true, "grant_id_required": true,
		"tenure": true, "other_experiment": true, "stamp_indeterminate": true, "reset_unconfirmed": true,
		"authorization_rejected": true, "claim_unconfirmed": true, claudeResetReasonUnavailable: true,
		"result_persistence_failed": true,
	}
	claudeResetKnownWindows = map[string]bool{"five_hour": true, "seven_day": true, "seven_day_overage_included": true}
)

// ClaudeResetOutcome is the sanitized redemption result. It never carries grant,
// organization, or upstream request IDs.
type ClaudeResetOutcome struct {
	Outcome       string              `json:"outcome"`
	Reason        string              `json:"reason,omitempty"`
	Cleared       []string            `json:"cleared,omitempty"`
	CooldownUntil *time.Time          `json:"cooldown_until,omitempty"`
	Credits       *ClaudeResetCredits `json:"credits,omitempty"`
	Replayed      bool                `json:"replayed"`
}

// claudeResetFence is stored in the idempotency table, one row per provider
// organization, so duplicate local accounts share it and account edits cannot erase it.
type claudeResetFence struct {
	Operation string    `json:"operation"`
	Outcome   string    `json:"outcome"`
	Reason    string    `json:"reason,omitempty"`
	At        time.Time `json:"at"`
}

// ConfigureRedemption enables Redeem. Without both stores Redeem refuses to run.
func (s *ClaudeResetCreditService) ConfigureRedemption(idem *IdempotencyCoordinator, locks LeaderLockCache) {
	s.idempotency = idem
	s.locks = locks
}

// Redeem consumes the upstream next reset grant of the account, if and only if a
// fresh query shows it redeemable. key identifies one operator confirmation: the
// same key replays the stored outcome and never sends a second claim.
func (s *ClaudeResetCreditService) Redeem(ctx context.Context, id int64, key string) (*ClaudeResetOutcome, error) {
	if strings.TrimSpace(key) == "" {
		return nil, ErrIdempotencyKeyRequired
	}
	normalized, err := NormalizeIdempotencyKey(key)
	if err != nil {
		return nil, err
	}
	if s.idempotency == nil || s.idempotency.repo == nil || s.locks == nil {
		return nil, ErrIdempotencyStoreUnavail
	}
	// Validate before entering the idempotency scope so a deleted or converted
	// account never replays. No token is stored anywhere.
	if _, _, _, err = s.account(ctx, id); err != nil {
		return nil, err
	}
	operation := HashIdempotencyKey(fmt.Sprintf("claude-reset:%d:%s", id, normalized))
	result, err := s.idempotency.Execute(ctx, IdempotencyExecuteOptions{
		Scope: claudeResetOperationScope, ActorScope: fmt.Sprintf("account:%d", id), Method: http.MethodPost,
		Route: "/admin/accounts/:id/claude/reset-credits/redeem", IdempotencyKey: operation,
		Payload: map[string]any{"account_id": id}, TTL: claudeResetRecordTTL, RequireKey: true, ExecutionTimeout: 60 * time.Second,
	}, func(exec context.Context) (any, error) { return s.redeemOnce(exec, id, operation) })
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(result.Data)
	if err != nil {
		return nil, err
	}
	var outcome ClaudeResetOutcome
	if err = json.Unmarshal(raw, &outcome); err != nil {
		return nil, err
	}
	outcome.Replayed = outcome.Replayed || result.Replayed
	return &outcome, nil
}

func (s *ClaudeResetCreditService) lease(ctx context.Context, key, owner string) (func(), error) {
	acquired, err := s.locks.TryAcquireLeaderLock(ctx, key, owner, claudeResetLeaseTTL)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("CLAUDE_RESET_LOCK_UNAVAILABLE", "reset coordination unavailable")
	}
	if !acquired {
		return nil, infraerrors.Conflict("CLAUDE_RESET_BUSY", "another reset is in progress")
	}
	return func() {
		release, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.locks.ReleaseLeaderLock(release, key, owner)
	}, nil
}

func (s *ClaudeResetCreditService) redeemOnce(ctx context.Context, id int64, operation string) (*ClaudeResetOutcome, error) {
	owner := uuid.NewString()
	release, err := s.lease(ctx, fmt.Sprintf("claude:reset-credit:account:%d", id), owner)
	if err != nil {
		return nil, err
	}
	defer release()
	_, token, proxy, err := s.account(ctx, id)
	if err != nil {
		return nil, err
	}
	org, err := s.organization(ctx, token, proxy)
	if err != nil {
		return nil, err
	}
	orgHash := HashIdempotencyKey("claude-org:" + org)
	releaseOrg, err := s.lease(ctx, "claude:reset-credit:organization:"+orgHash, owner)
	if err != nil {
		return nil, err
	}
	defer releaseOrg()

	fence, err := s.loadFence(ctx, orgHash)
	if err != nil {
		return nil, err
	}
	var prior claudeResetFence
	if fence.ResponseBody != nil {
		if json.Unmarshal([]byte(*fence.ResponseBody), &prior) != nil || prior.Operation == "" {
			return nil, infraerrors.Conflict("CLAUDE_RESET_UNRESOLVED", "previous reset requires reconciliation")
		}
		if prior.Operation == operation {
			// A crashed or interrupted attempt of this same confirmation: never resend.
			return &ClaudeResetOutcome{Outcome: prior.Outcome, Reason: prior.Reason, Replayed: true}, nil
		}
		if prior.Outcome == ClaudeResetOutcomeUnknown && prior.Reason == claudeResetReasonUnavailable {
			if s.now().Before(prior.At.Add(claudeResetUnavailableFenceTTL)) {
				return nil, infraerrors.Conflict("CLAUDE_RESET_UPSTREAM_UNAVAILABLE", "reset service was unavailable; retry after a while")
			}
		} else if prior.Outcome == ClaudeResetOutcomeUnknown && s.now().Before(prior.At.Add(claudeResetUnknownFenceTTL)) {
			return nil, infraerrors.Conflict("CLAUDE_RESET_UNRESOLVED", "previous reset outcome is unconfirmed; redemption is blocked for now")
		}
	}

	// Fresh eligibility check right before the irreversible call; the server alone
	// picks the grant, and only the upstream next grant can qualify.
	block, err := s.fetchBlock(ctx, token, proxy)
	if err != nil {
		return nil, err
	}
	var grant *claudeResetGrant
	if block != nil {
		for i := range block.Grants {
			if block.Grants[i].ID == block.NextGrantID && claudeResetGrantRedeemable(block, block.Grants[i], s.now()) {
				grant = &block.Grants[i]
				break
			}
		}
	}
	if grant == nil {
		return nil, infraerrors.Conflict("CLAUDE_RESET_NOT_AVAILABLE", "no reset is redeemable right now")
	}

	// Persist the unknown marker before sending: a crash after this point blocks
	// both a resend and another credit until the fence settles.
	marker := claudeResetFence{Operation: operation, Outcome: ClaudeResetOutcomeUnknown, Reason: "claim_unconfirmed", At: s.now().UTC()}
	if err = s.persistFence(ctx, fence.ID, marker); err != nil {
		return nil, err
	}
	outcome := s.claim(ctx, token, proxy, org, grant.ID, operation)
	marker.Outcome, marker.Reason = outcome.Outcome, outcome.Reason
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	persistErr := s.persistFence(persistCtx, fence.ID, marker)
	cancel()
	if persistErr != nil {
		return &ClaudeResetOutcome{Outcome: ClaudeResetOutcomeUnknown, Reason: "result_persistence_failed"}, nil
	}
	if outcome.Outcome != ClaudeResetOutcomeUnknown {
		if fresh, e := s.fetchBlock(ctx, token, proxy); e == nil {
			outcome.Credits = projectClaudeResetCredits(fresh, s.now())
		}
	}
	return outcome, nil
}

func (s *ClaudeResetCreditService) loadFence(ctx context.Context, orgHash string) (*IdempotencyRecord, error) {
	repo := s.idempotency.repo
	fence, err := repo.GetByScopeAndKeyHash(ctx, claudeResetFenceScope, orgHash)
	if err != nil {
		return nil, ErrIdempotencyStoreUnavail
	}
	if fence != nil {
		return fence, nil
	}
	row := &IdempotencyRecord{Scope: claudeResetFenceScope, IdempotencyKeyHash: orgHash, RequestFingerprint: orgHash, Status: IdempotencyStatusProcessing, ExpiresAt: s.now().Add(claudeResetRecordTTL)}
	if _, err = repo.CreateProcessing(ctx, row); err != nil {
		return nil, ErrIdempotencyStoreUnavail
	}
	fence, err = repo.GetByScopeAndKeyHash(ctx, claudeResetFenceScope, orgHash)
	if err != nil || fence == nil {
		return nil, ErrIdempotencyStoreUnavail
	}
	return fence, nil
}

func (s *ClaudeResetCreditService) persistFence(ctx context.Context, id int64, marker claudeResetFence) error {
	body, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	if err = s.idempotency.repo.MarkSucceeded(ctx, id, http.StatusOK, string(body), s.now().Add(claudeResetRecordTTL)); err != nil {
		return ErrIdempotencyStoreUnavail
	}
	return nil
}

func (s *ClaudeResetCreditService) organization(ctx context.Context, token, proxy string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, claudeResetProfileURL, nil)
	if err != nil {
		return "", err
	}
	s.headers(ctx, req, token)
	resp, err := s.do(req, proxy)
	if err != nil {
		return "", infraerrors.ServiceUnavailable("CLAUDE_RESET_PROFILE_FAILED", "OAuth profile unavailable")
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Organization struct {
			UUID string `json:"uuid"`
		} `json:"organization"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body) != nil {
		return "", infraerrors.New(http.StatusBadGateway, "CLAUDE_RESET_PROFILE_FAILED", "OAuth profile unavailable")
	}
	parsed, err := uuid.Parse(body.Organization.UUID)
	if err != nil || parsed == uuid.Nil {
		return "", infraerrors.New(http.StatusBadGateway, "CLAUDE_RESET_ORGANIZATION_INVALID", "OAuth organization unavailable")
	}
	return parsed.String(), nil
}

// claim sends the single irreversible request. Anything but a well-formed, known
// result is reported as unknown so the fence blocks a blind retry.
func (s *ClaudeResetCreditService) claim(ctx context.Context, token, proxy, org, grantID, operation string) *ClaudeResetOutcome {
	unknown := &ClaudeResetOutcome{Outcome: ClaudeResetOutcomeUnknown, Reason: "claim_unconfirmed"}
	// Deterministic per confirmation (64 hex chars, matches ^[A-Za-z0-9_-]{1,64}$).
	body, err := json.Marshal(map[string]string{"program": "cedar_ember", "grant_id": grantID, "request_id": operation})
	if err != nil {
		return unknown
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf(claudeResetRedeemURLFmt, org), bytes.NewReader(body))
	if err != nil {
		return unknown
	}
	s.headers(ctx, req, token)
	resp, err := s.do(req, proxy)
	if err != nil {
		return unknown
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return &ClaudeResetOutcome{Outcome: ClaudeResetOutcomeIneligible, Reason: "authorization_rejected"}
	}
	if resp.StatusCode != http.StatusOK {
		return unknown
	}
	var result struct {
		Result        string     `json:"result"`
		Reason        string     `json:"reason"`
		Cleared       []string   `json:"cleared"`
		CooldownUntil *time.Time `json:"cooldown_until"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result) != nil {
		return unknown
	}
	reason := ""
	if claudeResetKnownReasons[result.Reason] {
		reason = result.Reason
	}
	if reason == "stamp_indeterminate" || reason == "reset_unconfirmed" {
		return unknown
	}
	switch result.Result {
	case ClaudeResetOutcomeReset, ClaudeResetOutcomeAlreadyUsed, ClaudeResetOutcomeNotLimited, ClaudeResetOutcomeCooldown, ClaudeResetOutcomeIneligible:
		out := &ClaudeResetOutcome{Outcome: result.Result, Reason: reason, CooldownUntil: result.CooldownUntil}
		for _, w := range result.Cleared {
			if claudeResetKnownWindows[w] {
				out.Cleared = append(out.Cleared, w)
			}
		}
		return out
	case "unavailable":
		return &ClaudeResetOutcome{Outcome: ClaudeResetOutcomeUnknown, Reason: claudeResetReasonUnavailable}
	default:
		return unknown
	}
}
