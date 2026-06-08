// host_service_adapters.go wires the HostServiceServer interfaces (see
// grpc_server_host.go) to concrete host services via SQL-backed
// idempotency. Each adapter is the smallest possible bridge: validate
// inputs, run the service call inside an idempotency-protected
// LookupOrApply, encode the response so replays return the original
// outcome.
//
// Decoupling: gRPC handler → adapter → host service. Adapters depend on
// concrete host services (UserRepository, SubscriptionService, …) but
// implement the HostService* interfaces so the handler stays free of
// business-layer imports.
//
// All adapters require *HostIdempotencyStore. Without it, replays would
// double-credit / double-assign on retry.
package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/shopspring/decimal"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// hostBalanceUserRepo is the minimal UserRepository surface the balance
// adapter needs. Declaring the smaller interface here documents the
// dependency edge and keeps the adapter unit-testable without faking
// the entire UserRepository.
type hostBalanceUserRepo interface {
	GetByID(ctx context.Context, id int64) (*service.User, error)
	UpdateBalance(ctx context.Context, id int64, amount float64) error
	DeductBalance(ctx context.Context, id int64, amount float64) error
}

// HostBalanceAdapter implements HostBalanceService by delegating to the
// host UserRepository inside an idempotency window.
type HostBalanceAdapter struct {
	repo hostBalanceUserRepo
	idem *HostIdempotencyStore
	log  *slog.Logger
}

// NewHostBalanceAdapter constructs an adapter; both arguments are
// required.
func NewHostBalanceAdapter(repo hostBalanceUserRepo, idem *HostIdempotencyStore) *HostBalanceAdapter {
	return &HostBalanceAdapter{
		repo: repo,
		idem: idem,
		log:  slog.Default().With("component", "host_balance_adapter"),
	}
}

// balancePayload is the JSON shape persisted in plugin_idempotency for
// CreditBalance / DeductBalance replays.
type balancePayload struct {
	NewBalance string `json:"new_balance"`
}

// CreditBalance implements HostBalanceService.CreditBalance.
func (a *HostBalanceAdapter) CreditBalance(
	ctx context.Context, userID int64, amount decimal.Decimal,
	reason, idempotencyKey, source string,
) (decimal.Decimal, bool, error) {
	return a.applyBalanceOp(ctx, IdempotencyNamespaceCreditBalance, idempotencyKey,
		func(ctx context.Context) (decimal.Decimal, error) {
			amt, _ := amount.Float64()
			if err := a.repo.UpdateBalance(ctx, userID, amt); err != nil {
				return decimal.Decimal{}, fmt.Errorf("credit balance: %w", err)
			}
			return a.fetchBalance(ctx, userID)
		})
}

// DeductBalance implements HostBalanceService.DeductBalance. The repo's
// DeductBalance allows balance to go negative (transactional overdraft);
// callers that disable allowNegative must pre-check via fetchBalance.
func (a *HostBalanceAdapter) DeductBalance(
	ctx context.Context, userID int64, amount decimal.Decimal,
	reason, idempotencyKey, source string, allowNegative bool,
) (decimal.Decimal, bool, error) {
	return a.applyBalanceOp(ctx, IdempotencyNamespaceDeductBalance, idempotencyKey,
		func(ctx context.Context) (decimal.Decimal, error) {
			if !allowNegative {
				cur, err := a.fetchBalance(ctx, userID)
				if err != nil {
					return decimal.Decimal{}, err
				}
				if cur.LessThan(amount) {
					return decimal.Decimal{}, fmt.Errorf("insufficient balance: have %s, need %s",
						cur.String(), amount.String())
				}
			}
			amt, _ := amount.Float64()
			if err := a.repo.DeductBalance(ctx, userID, amt); err != nil {
				return decimal.Decimal{}, fmt.Errorf("deduct balance: %w", err)
			}
			return a.fetchBalance(ctx, userID)
		})
}

// applyBalanceOp wraps the supplied mutation in idempotency bookkeeping
// and JSON-encodes the resulting balance into plugin_idempotency.
func (a *HostBalanceAdapter) applyBalanceOp(
	ctx context.Context, namespace, key string,
	op func(ctx context.Context) (decimal.Decimal, error),
) (decimal.Decimal, bool, error) {
	payload, applied, err := a.idem.LookupOrApply(ctx, namespace, key,
		func(ctx context.Context) ([]byte, error) {
			newBal, err := op(ctx)
			if err != nil {
				return nil, err
			}
			return json.Marshal(balancePayload{NewBalance: newBal.String()})
		})
	if err != nil {
		return decimal.Decimal{}, false, err
	}
	var decoded balancePayload
	if jerr := json.Unmarshal(payload, &decoded); jerr != nil {
		return decimal.Decimal{}, applied, fmt.Errorf("decode payload: %w", jerr)
	}
	bal, err := decimal.NewFromString(decoded.NewBalance)
	if err != nil {
		return decimal.Decimal{}, applied, fmt.Errorf("parse balance: %w", err)
	}
	return bal, applied, nil
}

// fetchBalance loads the user's current balance as a decimal.
func (a *HostBalanceAdapter) fetchBalance(ctx context.Context, userID int64) (decimal.Decimal, error) {
	user, err := a.repo.GetByID(ctx, userID)
	if err != nil {
		return decimal.Decimal{}, err
	}
	return decimal.NewFromFloat(user.Balance), nil
}

// hostSubscriptionService is the minimal SubscriptionService surface the
// subscription adapter needs.
type hostSubscriptionService interface {
	AssignOrExtendSubscription(ctx context.Context, input *service.AssignSubscriptionInput) (*service.UserSubscription, bool, error)
	ExtendSubscription(ctx context.Context, subscriptionID int64, days int) (*service.UserSubscription, error)
	GetActiveSubscription(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error)
	RevokeSubscription(ctx context.Context, subscriptionID int64) error
}

// HostSubscriptionAdapter implements HostSubscriptionAssigner.
type HostSubscriptionAdapter struct {
	svc  hostSubscriptionService
	idem *HostIdempotencyStore
	log  *slog.Logger
}

// NewHostSubscriptionAdapter constructs an adapter; both arguments are
// required.
func NewHostSubscriptionAdapter(svc hostSubscriptionService, idem *HostIdempotencyStore) *HostSubscriptionAdapter {
	return &HostSubscriptionAdapter{svc: svc, idem: idem, log: slog.Default()}
}

// assignPayload is persisted on AssignSubscription replays.
type assignPayload struct {
	SubscriptionID int64 `json:"subscription_id"`
	ExpiresAtUnix  int64 `json:"expires_at_unix"`
}

// revokePayload is persisted on RevokeSubscriptionDays replays.
type revokePayload struct {
	ExpiresAtUnix int64 `json:"expires_at_unix"`
}

// AssignSubscription implements HostSubscriptionAssigner.AssignSubscription.
// PlanID is currently unused at the SubscriptionService boundary (the
// service identifies subscriptions by group_id only); we surface it on
// the wire for forward-compat with the future plan-aware refactor.
func (a *HostSubscriptionAdapter) AssignSubscription(
	ctx context.Context, userID, planID, groupID int64, days int,
	source, idempotencyKey string,
) (int64, int64, bool, error) {
	payload, applied, err := a.idem.LookupOrApply(ctx,
		IdempotencyNamespaceAssignSub, idempotencyKey,
		func(ctx context.Context) ([]byte, error) {
			sub, _, err := a.svc.AssignOrExtendSubscription(ctx, &service.AssignSubscriptionInput{
				UserID:       userID,
				GroupID:      groupID,
				ValidityDays: days,
				Notes:        fmt.Sprintf("plugin assign: source=%s key=%s", source, idempotencyKey),
			})
			if err != nil {
				return nil, fmt.Errorf("assign subscription: %w", err)
			}
			return json.Marshal(assignPayload{
				SubscriptionID: sub.ID,
				ExpiresAtUnix:  sub.ExpiresAt.Unix(),
			})
		})
	if err != nil {
		return 0, 0, false, err
	}
	var decoded assignPayload
	if jerr := json.Unmarshal(payload, &decoded); jerr != nil {
		return 0, 0, applied, fmt.Errorf("decode payload: %w", jerr)
	}
	return decoded.SubscriptionID, decoded.ExpiresAtUnix, applied, nil
}

// RevokeSubscriptionDays shortens an active subscription by the
// requested number of days. The underlying service.ExtendSubscription
// accepts negative day counts as "shorten"; we negate the value here so
// the adapter contract stays positive.
func (a *HostSubscriptionAdapter) RevokeSubscriptionDays(
	ctx context.Context, userID, groupID int64, days int,
	reason, idempotencyKey string,
) (int64, bool, error) {
	payload, applied, err := a.idem.LookupOrApply(ctx,
		IdempotencyNamespaceRevokeSub, idempotencyKey,
		func(ctx context.Context) ([]byte, error) {
			sub, err := a.svc.GetActiveSubscription(ctx, userID, groupID)
			if err != nil || sub == nil {
				return nil, fmt.Errorf("subscription not found: %w", err)
			}
			updated, err := a.svc.ExtendSubscription(ctx, sub.ID, -days)
			if err != nil {
				// Fallback: when the deduction would push the
				// subscription past its expiry, revoke the
				// whole subscription instead of failing the
				// refund. This mirrors the legacy host-side
				// behaviour from before the plugin migration.
				if errors.Is(err, service.ErrAdjustWouldExpire) {
					if a.log != nil {
						a.log.Info("subscription deduction would expire; revoking entire subscription",
							"subscription_id", sub.ID,
							"user_id", userID,
							"group_id", groupID,
							"requested_days", days,
							"reason", reason,
						)
					}
					if revokeErr := a.svc.RevokeSubscription(ctx, sub.ID); revokeErr != nil {
						return nil, fmt.Errorf("revoke subscription fallback: %w", revokeErr)
					}
					// Subscription removed — surface
					// expires_at = 0 to signal "no longer
					// active" to the caller.
					return json.Marshal(revokePayload{ExpiresAtUnix: 0})
				}
				return nil, fmt.Errorf("revoke days: %w", err)
			}
			return json.Marshal(revokePayload{ExpiresAtUnix: updated.ExpiresAt.Unix()})
		})
	if err != nil {
		return 0, false, err
	}
	var decoded revokePayload
	if jerr := json.Unmarshal(payload, &decoded); jerr != nil {
		return 0, applied, fmt.Errorf("decode payload: %w", jerr)
	}
	return decoded.ExpiresAtUnix, applied, nil
}

// hostAffiliateService is the minimal AffiliateService surface needed
// by the rebate adapter.
type hostAffiliateService interface {
	AccrueInviteRebate(ctx context.Context, inviteeUserID int64, baseRechargeAmount float64) (float64, error)
	EnsureUserAffiliate(ctx context.Context, userID int64) (*service.AffiliateSummary, error)
}

// HostAffiliateAdapter implements HostAffiliateAccruer.
type HostAffiliateAdapter struct {
	svc  hostAffiliateService
	idem *HostIdempotencyStore
}

// NewHostAffiliateAdapter constructs an adapter; both arguments are
// required.
func NewHostAffiliateAdapter(svc hostAffiliateService, idem *HostIdempotencyStore) *HostAffiliateAdapter {
	return &HostAffiliateAdapter{svc: svc, idem: idem}
}

// rebatePayload is persisted on AccrueRebate replays.
type rebatePayload struct {
	RebateAmount  string `json:"rebate_amount"`
	InviterUserID int64  `json:"inviter_user_id"`
}

// AccrueRebate implements HostAffiliateAccruer.AccrueRebate.
func (a *HostAffiliateAdapter) AccrueRebate(
	ctx context.Context, inviteeUserID int64, orderAmount decimal.Decimal,
	idempotencyKey string,
) (decimal.Decimal, int64, bool, error) {
	payload, applied, err := a.idem.LookupOrApply(ctx,
		IdempotencyNamespaceRebate, idempotencyKey,
		func(ctx context.Context) ([]byte, error) {
			amt, _ := orderAmount.Float64()
			rebate, err := a.svc.AccrueInviteRebate(ctx, inviteeUserID, amt)
			if err != nil {
				return nil, fmt.Errorf("accrue rebate: %w", err)
			}
			inviterID, _ := a.lookupInviterID(ctx, inviteeUserID)
			rebateDec := decimal.NewFromFloat(rebate)
			return json.Marshal(rebatePayload{
				RebateAmount:  rebateDec.String(),
				InviterUserID: inviterID,
			})
		})
	if err != nil {
		return decimal.Decimal{}, 0, false, err
	}
	var decoded rebatePayload
	if jerr := json.Unmarshal(payload, &decoded); jerr != nil {
		return decimal.Decimal{}, 0, applied, fmt.Errorf("decode payload: %w", jerr)
	}
	rebate, _ := decimal.NewFromString(decoded.RebateAmount)
	return rebate, decoded.InviterUserID, applied, nil
}

// lookupInviterID resolves the inviter for the given invitee. Errors
// degrade to (0, err); the rebate flow does not strictly require the
// inviter ID for correctness.
func (a *HostAffiliateAdapter) lookupInviterID(ctx context.Context, inviteeUserID int64) (int64, error) {
	summary, err := a.svc.EnsureUserAffiliate(ctx, inviteeUserID)
	if err != nil || summary == nil || summary.InviterID == nil {
		return 0, err
	}
	return *summary.InviterID, nil
}

// hostUserLookupRepo is the minimal UserRepository surface the user
// lookup adapter needs.
type hostUserLookupRepo interface {
	GetByID(ctx context.Context, id int64) (*service.User, error)
}

// HostUserLookupAdapter implements HostUserLookup. Inviter resolution
// is optional: if affiliateService is nil the InviterID field stays 0.
type HostUserLookupAdapter struct {
	repo      hostUserLookupRepo
	affiliate hostAffiliateService
}

// NewHostUserLookupAdapter constructs the adapter. affiliateService
// may be nil — the only consequence is that GetUserByID returns
// InviterID=0.
func NewHostUserLookupAdapter(repo hostUserLookupRepo, affiliateService hostAffiliateService) *HostUserLookupAdapter {
	return &HostUserLookupAdapter{repo: repo, affiliate: affiliateService}
}

// GetUserByID implements HostUserLookup.GetUserByID. Service-level
// ErrUserNotFound is converted to (nil, nil) so the gRPC layer reports
// found=false rather than NotFound.
func (a *HostUserLookupAdapter) GetUserByID(ctx context.Context, userID int64) (*HostUserInfo, error) {
	user, err := a.repo.GetByID(ctx, userID)
	if err != nil {
		// Treat "not found" as soft success; any other error bubbles up.
		if isNotFoundErr(err) {
			return nil, nil
		}
		return nil, err
	}
	info := &HostUserInfo{
		ID:       user.ID,
		Email:    user.Email,
		Username: user.Username,
		Role:     user.Role,
		Balance:  decimal.NewFromFloat(user.Balance),
	}
	if a.affiliate != nil {
		summary, err := a.affiliate.EnsureUserAffiliate(ctx, userID)
		if err == nil && summary != nil && summary.InviterID != nil {
			info.InviterID = *summary.InviterID
		}
	}
	return info, nil
}

// isNotFoundErr inspects an error for the host's ErrUserNotFound. We
// intentionally match by string because service.ErrUserNotFound is a
// wrapped infraerrors value; errors.Is would require importing the
// concrete type.
func isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	return err == service.ErrUserNotFound
}
