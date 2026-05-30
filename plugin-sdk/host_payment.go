package pluginsdk

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// HostClient extensions for the payment plugin.
//
// Idempotency: every method takes an IdempotencyKey that uniquely
// identifies the source operation (typically the payment order
// out_trade_no or order_id). The host stores the result of the first
// call keyed by (namespace, idempotency_key); a repeat call with the
// same key returns AlreadyApplied=true with the original result.
//
// codes.Unimplemented surfaces as a typed sentinel so callers can
// branch on host-feature-unavailable without stringly-typed checks.
// Other errors pass through unchanged.

const hostFulfillRPCTimeout = 10 * time.Second

type CreditBalanceInput struct {
	UserID         int64
	Amount         decimal.Decimal
	Reason         string
	IdempotencyKey string
	Source         string
}

type CreditBalanceResult struct {
	NewBalance     decimal.Decimal
	AlreadyApplied bool
}

type DeductBalanceInput struct {
	UserID         int64
	Amount         decimal.Decimal
	Reason         string
	IdempotencyKey string
	Source         string
	AllowNegative  bool
}

type DeductBalanceResult struct {
	NewBalance     decimal.Decimal
	AlreadyApplied bool
}

type AssignSubscriptionInput struct {
	UserID         int64
	PlanID         int64
	GroupID        int64
	Days           int
	Source         string
	IdempotencyKey string
}

type AssignSubscriptionResult struct {
	SubscriptionID int64
	ExpiresAtUnix  int64
	AlreadyApplied bool
}

type RevokeSubscriptionDaysInput struct {
	UserID         int64
	GroupID        int64
	Days           int
	Reason         string
	IdempotencyKey string
}

type RevokeSubscriptionDaysResult struct {
	ExpiresAtUnix  int64
	AlreadyApplied bool
}

type AccrueRebateInput struct {
	InviteeUserID  int64
	OrderAmount    decimal.Decimal
	IdempotencyKey string
}

type AccrueRebateResult struct {
	RebateAmount   decimal.Decimal
	InviterUserID  int64
	AlreadyApplied bool
}

type HostUserInfo struct {
	Found     bool
	ID        int64
	Email     string
	Username  string
	Balance   decimal.Decimal
	InviterID int64
	Role      string
}

var (
	ErrHostBalanceUnavailable      = errors.New("pluginsdk: host balance service not registered")
	ErrHostSubscriptionUnavailable = errors.New("pluginsdk: host subscription assigner not registered")
	ErrHostAffiliateUnavailable    = errors.New("pluginsdk: host affiliate accruer not registered")
	ErrHostUserLookupUnavailable   = errors.New("pluginsdk: host user lookup not registered")
	ErrHostUserDisableUnavailable  = errors.New("pluginsdk: host user-disable service not registered")
)

// Compile-time proof that the SDK's concrete types satisfy
// HostPaymentClient so PluginContext.HostPayment() never falls through
// to nilHostClient unexpectedly.
var (
	_ HostPaymentClient = (*hostClient)(nil)
	_ HostPaymentClient = nilHostClient{}
)

type HostPaymentClient interface {
	CreditBalance(ctx context.Context, in CreditBalanceInput) (*CreditBalanceResult, error)
	DeductBalance(ctx context.Context, in DeductBalanceInput) (*DeductBalanceResult, error)
	AssignSubscription(ctx context.Context, in AssignSubscriptionInput) (*AssignSubscriptionResult, error)
	RevokeSubscriptionDays(ctx context.Context, in RevokeSubscriptionDaysInput) (*RevokeSubscriptionDaysResult, error)
	AccrueRebate(ctx context.Context, in AccrueRebateInput) (*AccrueRebateResult, error)
	GetUserByID(ctx context.Context, userID int64) (*HostUserInfo, error)
	// SetUserDisabled bans (disabled=true) or unbans (disabled=false) a
	// user. The host also invalidates the user's auth cache so the change
	// takes effect on the next gateway request. reason is recorded on the
	// user's disable_reason field for audit. Used by the content-moderation
	// plugin for auto-ban and admin unban. Returns
	// ErrHostUserDisableUnavailable when the host has not registered the
	// extension.
	SetUserDisabled(ctx context.Context, userID int64, disabled bool, reason string) error
}

func (h *hostClient) CreditBalance(ctx context.Context, in CreditBalanceInput) (*CreditBalanceResult, error) {
	rpcCtx, cancel := context.WithTimeout(ctx, hostFulfillRPCTimeout)
	defer cancel()
	resp, err := h.grpc.CreditBalance(rpcCtx, &pb.CreditBalanceRequest{
		UserId:         in.UserID,
		Amount:         in.Amount.String(),
		Reason:         in.Reason,
		IdempotencyKey: in.IdempotencyKey,
		Source:         in.Source,
	})
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return nil, ErrHostBalanceUnavailable
		}
		return nil, err
	}
	bal, _ := decimal.NewFromString(resp.GetNewBalance())
	return &CreditBalanceResult{NewBalance: bal, AlreadyApplied: resp.GetAlreadyApplied()}, nil
}

func (h *hostClient) DeductBalance(ctx context.Context, in DeductBalanceInput) (*DeductBalanceResult, error) {
	rpcCtx, cancel := context.WithTimeout(ctx, hostFulfillRPCTimeout)
	defer cancel()
	resp, err := h.grpc.DeductBalance(rpcCtx, &pb.DeductBalanceRequest{
		UserId:         in.UserID,
		Amount:         in.Amount.String(),
		Reason:         in.Reason,
		IdempotencyKey: in.IdempotencyKey,
		Source:         in.Source,
		AllowNegative:  in.AllowNegative,
	})
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return nil, ErrHostBalanceUnavailable
		}
		return nil, err
	}
	bal, _ := decimal.NewFromString(resp.GetNewBalance())
	return &DeductBalanceResult{NewBalance: bal, AlreadyApplied: resp.GetAlreadyApplied()}, nil
}

func (h *hostClient) AssignSubscription(ctx context.Context, in AssignSubscriptionInput) (*AssignSubscriptionResult, error) {
	rpcCtx, cancel := context.WithTimeout(ctx, hostFulfillRPCTimeout)
	defer cancel()
	resp, err := h.grpc.AssignSubscription(rpcCtx, &pb.AssignSubscriptionRequest{
		UserId:         in.UserID,
		PlanId:         in.PlanID,
		GroupId:        in.GroupID,
		Days:           int32(in.Days),
		Source:         in.Source,
		IdempotencyKey: in.IdempotencyKey,
	})
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return nil, ErrHostSubscriptionUnavailable
		}
		return nil, err
	}
	return &AssignSubscriptionResult{
		SubscriptionID: resp.GetSubscriptionId(),
		ExpiresAtUnix:  resp.GetExpiresAtUnix(),
		AlreadyApplied: resp.GetAlreadyApplied(),
	}, nil
}

func (h *hostClient) RevokeSubscriptionDays(ctx context.Context, in RevokeSubscriptionDaysInput) (*RevokeSubscriptionDaysResult, error) {
	rpcCtx, cancel := context.WithTimeout(ctx, hostFulfillRPCTimeout)
	defer cancel()
	resp, err := h.grpc.RevokeSubscriptionDays(rpcCtx, &pb.RevokeSubscriptionDaysRequest{
		UserId:         in.UserID,
		GroupId:        in.GroupID,
		Days:           int32(in.Days),
		Reason:         in.Reason,
		IdempotencyKey: in.IdempotencyKey,
	})
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return nil, ErrHostSubscriptionUnavailable
		}
		return nil, err
	}
	return &RevokeSubscriptionDaysResult{
		ExpiresAtUnix:  resp.GetExpiresAtUnix(),
		AlreadyApplied: resp.GetAlreadyApplied(),
	}, nil
}

func (h *hostClient) AccrueRebate(ctx context.Context, in AccrueRebateInput) (*AccrueRebateResult, error) {
	rpcCtx, cancel := context.WithTimeout(ctx, hostFulfillRPCTimeout)
	defer cancel()
	resp, err := h.grpc.AccrueRebate(rpcCtx, &pb.AccrueRebateRequest{
		InviteeUserId:  in.InviteeUserID,
		OrderAmount:    in.OrderAmount.String(),
		IdempotencyKey: in.IdempotencyKey,
	})
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return nil, ErrHostAffiliateUnavailable
		}
		return nil, err
	}
	rebate, _ := decimal.NewFromString(resp.GetRebateAmount())
	return &AccrueRebateResult{
		RebateAmount:   rebate,
		InviterUserID:  resp.GetInviterUserId(),
		AlreadyApplied: resp.GetAlreadyApplied(),
	}, nil
}

func (h *hostClient) GetUserByID(ctx context.Context, userID int64) (*HostUserInfo, error) {
	rpcCtx, cancel := context.WithTimeout(ctx, hostFulfillRPCTimeout)
	defer cancel()
	resp, err := h.grpc.GetUserByID(rpcCtx, &pb.GetUserByIDRequest{UserId: userID})
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return nil, ErrHostUserLookupUnavailable
		}
		return nil, err
	}
	if !resp.GetFound() {
		return nil, nil
	}
	bal, _ := decimal.NewFromString(resp.GetBalance())
	return &HostUserInfo{
		Found:     true,
		ID:        resp.GetUserId(),
		Email:     resp.GetEmail(),
		Username:  resp.GetUsername(),
		Balance:   bal,
		InviterID: resp.GetInviterId(),
		Role:      resp.GetRole(),
	}, nil
}

func (h *hostClient) SetUserDisabled(ctx context.Context, userID int64, disabled bool, reason string) error {
	rpcCtx, cancel := context.WithTimeout(ctx, hostFulfillRPCTimeout)
	defer cancel()
	_, err := h.grpc.SetUserDisabled(rpcCtx, &pb.SetUserDisabledRequest{
		UserId:   userID,
		Disabled: disabled,
		Reason:   reason,
	})
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return ErrHostUserDisableUnavailable
		}
		return err
	}
	return nil
}

func (nilHostClient) CreditBalance(context.Context, CreditBalanceInput) (*CreditBalanceResult, error) {
	return nil, ErrHostBalanceUnavailable
}

func (nilHostClient) DeductBalance(context.Context, DeductBalanceInput) (*DeductBalanceResult, error) {
	return nil, ErrHostBalanceUnavailable
}

func (nilHostClient) AssignSubscription(context.Context, AssignSubscriptionInput) (*AssignSubscriptionResult, error) {
	return nil, ErrHostSubscriptionUnavailable
}

func (nilHostClient) RevokeSubscriptionDays(context.Context, RevokeSubscriptionDaysInput) (*RevokeSubscriptionDaysResult, error) {
	return nil, ErrHostSubscriptionUnavailable
}

func (nilHostClient) AccrueRebate(context.Context, AccrueRebateInput) (*AccrueRebateResult, error) {
	return nil, ErrHostAffiliateUnavailable
}

func (nilHostClient) GetUserByID(context.Context, int64) (*HostUserInfo, error) {
	return nil, ErrHostUserLookupUnavailable
}

func (nilHostClient) SetUserDisabled(context.Context, int64, bool, string) error {
	return ErrHostUserDisableUnavailable
}
