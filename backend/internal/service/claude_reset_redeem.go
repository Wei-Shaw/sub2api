package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

const claudeResetPendingScope = "claude_reset_account_fence"

type ClaudeResetOutcome struct {
	Outcome  string              `json:"outcome"`
	Reason   string              `json:"reason,omitempty"`
	Credits  *ClaudeResetCredits `json:"credits,omitempty"`
	Replayed bool                `json:"replayed"`
}

type claudeResetWriter interface {
	GetByID(context.Context, int64) (*Account, error)
	UpdateExtra(context.Context, int64, map[string]any) error
}

type claudePreparedClaim struct {
	Selection      string `json:"selection"`
	GrantID        string `json:"grant_id"`
	OrganizationID string `json:"organization_id"`
	RequestID      string `json:"request_id"`
}

type claudePendingClaim struct {
	Selection string `json:"selection"`
	Outcome   string `json:"outcome"`
	Reason    string `json:"reason,omitempty"`
}

// ConfigureRedemption is wired separately from the read-only adapter. Both the
// PostgreSQL idempotency store and Redis lease are mandatory, never fail-open.
func (s *ClaudeResetCreditService) ConfigureRedemption(repo AccountRepository, idem *IdempotencyCoordinator, locks LeaderLockCache) {
	s.writer = repo
	s.idempotency = idem
	s.locks = locks
}

func (s *ClaudeResetCreditService) Redeem(ctx context.Context, id int64, selection, key string) (*ClaudeResetOutcome, error) {
	if strings.TrimSpace(key) == "" {
		return nil, ErrIdempotencyKeyRequired
	}
	if _, err := NormalizeIdempotencyKey(key); err != nil {
		return nil, err
	}
	if decoded, err := hex.DecodeString(selection); err != nil || len(decoded) != 32 {
		return nil, infraerrors.BadRequest("CLAUDE_RESET_SELECTION_INVALID", "select an available reset")
	}
	if s.idempotency == nil || s.idempotency.repo == nil || s.writer == nil || s.locks == nil {
		return nil, ErrIdempotencyStoreUnavail
	}
	// Validation before entering either idempotency scope prevents replay for a
	// deleted/replaced/non-Claude account. No OAuth token is stored in either row.
	if _, _, _, err := s.account(ctx, id); err != nil {
		return nil, err
	}
	result, err := s.idempotency.Execute(ctx, IdempotencyExecuteOptions{
		Scope: "claude_reset_manual", ActorScope: fmt.Sprintf("account:%d", id), Method: http.MethodPost,
		Route: "/admin/accounts/claude/reset-credits", IdempotencyKey: HashIdempotencyKey(fmt.Sprintf("%d:%s", id, key)),
		Payload: map[string]any{"account_id": id, "selection": selection}, TTL: 365 * 24 * time.Hour, RequireKey: true, ExecutionTimeout: 60 * time.Second,
	}, func(exec context.Context) (any, error) { return s.redeemOnce(exec, id, selection, key) })
	if err != nil {
		return nil, err
	}
	var outcome ClaudeResetOutcome
	raw, err := json.Marshal(result.Data)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &outcome); err != nil {
		return nil, err
	}
	outcome.Replayed = outcome.Replayed || result.Replayed
	return &outcome, nil
}

func (s *ClaudeResetCreditService) redeemOnce(ctx context.Context, id int64, selection string, operationKey string) (*ClaudeResetOutcome, error) {
	lockKey := fmt.Sprintf("claude:reset-credit:account:%d", id)
	owner := uuid.NewString()
	acquired, err := s.locks.TryAcquireLeaderLock(ctx, lockKey, owner, 90*time.Second)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("CLAUDE_RESET_LOCK_UNAVAILABLE", "reset coordination unavailable")
	}
	if !acquired {
		return nil, infraerrors.Conflict("CLAUDE_RESET_BUSY", "another reset is in progress")
	}
	defer func() {
		release, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.locks.ReleaseLeaderLock(release, lockKey, owner)
	}()
	account, err := s.writer.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}
	_, token, proxy, err := s.account(ctx, id)
	if err != nil {
		return nil, err
	}
	org, err := s.organization(ctx, token, proxy)
	if err != nil {
		return nil, err
	}
	orgHash := HashIdempotencyKey("claude-org:" + org)
	orgLockKey := "claude:reset-credit:organization:" + orgHash
	acquired, err = s.locks.TryAcquireLeaderLock(ctx, orgLockKey, owner, 90*time.Second)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("CLAUDE_RESET_LOCK_UNAVAILABLE", "reset coordination unavailable")
	}
	if !acquired {
		return nil, infraerrors.Conflict("CLAUDE_RESET_BUSY", "another reset is in progress")
	}
	defer func() {
		release, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.locks.ReleaseLeaderLock(release, orgLockKey, owner)
	}()
	// The durable provider-organization fence lives in the operation table, not editable
	// account.Extra. Ordinary edits/imports cannot erase an ambiguous claim.
	fenceKey := orgHash
	fence, err := s.idempotency.repo.GetByScopeAndKeyHash(ctx, claudeResetPendingScope, fenceKey)
	if err != nil {
		return nil, ErrIdempotencyStoreUnavail
	}
	if fence == nil {
		row := &IdempotencyRecord{Scope: claudeResetPendingScope, IdempotencyKeyHash: fenceKey, RequestFingerprint: orgHash, Status: IdempotencyStatusProcessing, ExpiresAt: s.now().AddDate(100, 0, 0)}
		_, err = s.idempotency.repo.CreateProcessing(ctx, row)
		if err != nil {
			return nil, ErrIdempotencyStoreUnavail
		}
		fence, err = s.idempotency.repo.GetByScopeAndKeyHash(ctx, claudeResetPendingScope, fenceKey)
		if err != nil || fence == nil {
			return nil, ErrIdempotencyStoreUnavail
		}
	}
	var pending claudePendingClaim
	if fence.ResponseBody != nil {
		if json.Unmarshal([]byte(*fence.ResponseBody), &pending) != nil || pending.Selection == "" {
			return nil, infraerrors.Conflict("CLAUDE_RESET_UNRESOLVED", "previous reset requires reconciliation")
		}
		if pending.Outcome == "unknown" || pending.Outcome == "" {
			if pending.Selection == selection {
				return &ClaudeResetOutcome{Outcome: "unknown", Reason: pending.Reason, Replayed: true}, nil
			}
			return nil, infraerrors.Conflict("CLAUDE_RESET_UNRESOLVED", "previous reset outcome is unknown; another credit cannot be used")
		}
		if pending.Selection == selection && (pending.Outcome == "reset" || pending.Outcome == "already_used") {
			return &ClaudeResetOutcome{Outcome: pending.Outcome, Reason: pending.Reason, Replayed: true}, nil
		}
	}
	status, block, err := s.queryWithToken(ctx, token, proxy)
	if err != nil {
		return nil, err
	}
	var grant *claudeResetGrant
	for _, credit := range status.Credits {
		if credit.SelectionToken == selection && credit.Redeemable {
			for i := range block.Grants {
				if claudeGrantSelection(block.Grants[i]) == selection {
					grant = &block.Grants[i]
					break
				}
			}
		}
	}
	if grant == nil {
		return nil, infraerrors.Conflict("CLAUDE_RESET_NOT_AVAILABLE", "selected reset is no longer available")
	}
	planResult, err := s.idempotency.Execute(ctx, IdempotencyExecuteOptions{
		Scope: "claude_reset_prepared", ActorScope: fmt.Sprintf("account:%d", id), Method: http.MethodPost, Route: "/system/claude/reset-plan",
		IdempotencyKey: HashIdempotencyKey(fmt.Sprintf("%d:%s:%s", id, selection, operationKey)), Payload: map[string]any{"account_id": id, "selection": selection}, TTL: 365 * 24 * time.Hour, RequireKey: true,
	}, func(context.Context) (any, error) {
		return claudePreparedClaim{Selection: selection, GrantID: grant.ID, OrganizationID: org, RequestID: uuid.NewString()}, nil
	})
	if err != nil {
		return nil, err
	}
	var plan claudePreparedClaim
	data, err := json.Marshal(planResult.Data)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, &plan); err != nil {
		return nil, err
	}
	if plan.OrganizationID != org || plan.GrantID != grant.ID || plan.Selection != selection {
		return nil, infraerrors.Conflict("CLAUDE_RESET_IDENTITY_CHANGED", "prepared reset identity changed")
	}
	// Recheck the native temporal gates immediately before the irreversible call.
	if grant.EndsAt != nil && !s.now().Before(*grant.EndsAt) {
		return nil, infraerrors.Conflict("CLAUDE_RESET_NOT_AVAILABLE", "selected reset expired")
	}
	if block.CooldownUntil != nil && s.now().Before(*block.CooldownUntil) {
		return nil, infraerrors.Conflict("CLAUDE_RESET_NOT_AVAILABLE", "reset is cooling down")
	}
	// Persist before the irreversible request. A crash anywhere after this write
	// leaves an unknown marker that forbids both another send and another credit.
	marker := claudePendingClaim{Selection: selection, Outcome: "unknown", Reason: "claim_unconfirmed"}
	if err = s.persistFence(ctx, fence.ID, marker); err != nil {
		return nil, err
	}
	outcome := s.claim(ctx, token, proxy, plan)
	marker.Outcome = outcome.Outcome
	marker.Reason = outcome.Reason
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	persistErr := s.persistFence(persistCtx, fence.ID, marker)
	cancel()
	if persistErr != nil {
		return &ClaudeResetOutcome{Outcome: "unknown", Reason: "result_persistence_failed"}, nil
	}
	if outcome.Outcome == "reset" {
		fresh, e := s.Query(ctx, id)
		if e == nil {
			outcome.Credits = fresh
		} else {
			outcome.Reason = "post_reset_query_failed"
		}
	}
	return outcome, nil
}

func (s *ClaudeResetCreditService) organization(ctx context.Context, token, proxy string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.anthropic.com/api/oauth/profile", nil)
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
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body) != nil {
		return "", infraerrors.New(502, "CLAUDE_RESET_PROFILE_FAILED", "OAuth profile unavailable")
	}
	parsed, err := uuid.Parse(body.Organization.UUID)
	if err != nil || parsed == uuid.Nil {
		return "", infraerrors.New(502, "CLAUDE_RESET_ORGANIZATION_INVALID", "OAuth organization unavailable")
	}
	return parsed.String(), nil
}

func (s *ClaudeResetCreditService) claim(ctx context.Context, token, proxy string, p claudePreparedClaim) *ClaudeResetOutcome {
	unknown := &ClaudeResetOutcome{Outcome: "unknown", Reason: "claim_unconfirmed"}
	body, _ := json.Marshal(map[string]string{"program": "cedar_ember", "grant_id": p.GrantID, "request_id": p.RequestID})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/api/organizations/"+p.OrganizationID+"/reset_rate_limits", bytes.NewReader(body))
	s.headers(ctx, req, token)
	resp, err := s.do(req, proxy)
	if err != nil {
		return unknown
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return &ClaudeResetOutcome{Outcome: "ineligible", Reason: "authorization_rejected"}
	}
	if resp.StatusCode != 200 {
		return unknown
	}
	var result struct {
		Result string `json:"result"`
		Reason string `json:"reason"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result) != nil {
		return unknown
	}
	if result.Reason == "stamp_indeterminate" || result.Reason == "reset_unconfirmed" {
		return unknown
	}
	switch result.Result {
	case "reset", "already_used", "not_limited", "cooldown", "ineligible":
		return &ClaudeResetOutcome{Outcome: result.Result}
	default:
		return unknown
	}
}

func (s *ClaudeResetCreditService) persistFence(ctx context.Context, id int64, marker claudePendingClaim) error {
	body, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	return s.idempotency.repo.MarkSucceeded(ctx, id, http.StatusOK, string(body), s.now().AddDate(100, 0, 0))
}
