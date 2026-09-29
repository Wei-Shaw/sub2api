// Package muse contains the native consumer Muse provider's runtime contracts.
// It deliberately does not model the unverified Meta chat wire protocol.
package muse

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"
)

const Platform = "muse"

var (
	ErrInvalid              = errors.New("invalid muse runtime input")
	ErrNotFound             = errors.New("muse runtime record not found")
	ErrOwner                = errors.New("muse workspace belongs to another user")
	ErrBusy                 = errors.New("muse workspace has unresolved work")
	ErrLease                = errors.New("muse workspace lease is no longer owned")
	ErrGeneration           = errors.New("muse workspace identity changed")
	ErrTransition           = errors.New("invalid muse turn transition")
	ErrOwnerReview          = errors.New("muse turn needs owner review")
	ErrTransportUnqualified = errors.New("muse subscription transport is not qualified")
)

type State string

const (
	Reserved      State = "reserved"
	Submitting    State = "submitting"
	Accepted      State = "accepted"
	Running       State = "running"
	CancelPending State = "cancel_pending"
	Ambiguous     State = "ambiguous"
	OwnerReview   State = "owner_review"
	Completed     State = "completed"
	Failed        State = "failed"
	Cancelled     State = "cancelled"
	Rejected      State = "rejected"
)

func (s State) Terminal() bool {
	return s == Completed || s == Failed || s == Cancelled || s == Rejected
}

// CanTransition never converts uncertain accepted work into a new submission.
func CanTransition(from, to State) bool {
	switch from {
	case Reserved:
		return to == Submitting || to == Rejected
	case Submitting:
		return to == Accepted || to == Ambiguous || to == Rejected
	case Accepted:
		return to == Running || to == Completed || to == Failed || to == CancelPending || to == Ambiguous
	case Running:
		return to == Completed || to == Failed || to == CancelPending || to == Ambiguous
	case CancelPending:
		return to == Cancelled || to == Completed || to == Failed || to == Ambiguous
	case Ambiguous:
		return to == Running || to == Completed || to == Failed || to == Cancelled || to == OwnerReview
	default:
		return false
	}
}

// Identity must come from an authenticated provider observation, never directly
// from an inbound client request. Aliased local account rows share this identity.
type Identity struct {
	PrincipalID      string
	WorkspaceID      string
	OwnerUserID      int64
	AccountID        int64
	AccountUpdatedAt time.Time
}

func (i Identity) Validate() error {
	if !validID(i.PrincipalID, 256) || !validID(i.WorkspaceID, 256) || i.OwnerUserID <= 0 || i.AccountID <= 0 || i.AccountUpdatedAt.IsZero() {
		return ErrInvalid
	}
	return nil
}

type Workspace struct {
	ID           int64
	Identity     Identity
	Generation   int64
	Fence        int64
	ActiveTurnID string
	LeaseOwner   string
	LeaseUntil   time.Time
}

// Actor is resolved by Sub2API authentication, not the request's user field.
type Actor struct {
	UserID   int64 `json:"user_id"`
	APIKeyID int64 `json:"api_key_id"`
}

// Pricing freezes an explicit flat charge policy, not invented provider tokens.
type Pricing struct {
	Mode       string `json:"mode"`
	UnitPrice  string `json:"unit_price"`
	Multiplier string `json:"multiplier"`
}

var decimalPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,11})(\.[0-9]{1,8})?$`)

func (p Pricing) Validate() error {
	if !decimalPattern.MatchString(p.UnitPrice) || !decimalPattern.MatchString(p.Multiplier) {
		return fmt.Errorf("%w: pricing must use bounded decimal strings", ErrInvalid)
	}
	price, _ := new(big.Rat).SetString(p.UnitPrice)
	multiplier, _ := new(big.Rat).SetString(p.Multiplier)
	if (p.Mode != "test_free" && p.Mode != "flat_request") || multiplier.Sign() < 0 ||
		(p.Mode == "test_free" && price.Sign() != 0) || (p.Mode == "flat_request" && price.Sign() <= 0) {
		return fmt.Errorf("%w: explicit free or positive flat pricing required", ErrInvalid)
	}
	return nil
}

type Reservation struct {
	TurnID           string
	WorkspaceID      int64
	Generation       int64
	Actor            Actor
	AccountID        int64
	AccountUpdatedAt time.Time
	ProxyUpdatedAt   *time.Time
	LeaseOwner       string
	LeaseDuration    time.Duration
	Pricing          Pricing
}

func (r Reservation) Validate() error {
	if !validID(r.TurnID, 96) || !validID(r.LeaseOwner, 96) || r.WorkspaceID <= 0 || r.Generation <= 0 ||
		r.Actor.UserID <= 0 || r.Actor.APIKeyID <= 0 || r.AccountID <= 0 || r.AccountUpdatedAt.IsZero() {
		return ErrInvalid
	}
	if err := ValidateLease(r.LeaseDuration); err != nil {
		return err
	}
	return r.Pricing.Validate()
}

type Lease struct {
	WorkspaceID int64
	TurnID      string
	Owner       string
	Fence       int64
}

func (l Lease) Validate() error {
	if l.WorkspaceID <= 0 || l.Fence <= 0 || !validID(l.TurnID, 96) || !validID(l.Owner, 96) {
		return ErrInvalid
	}
	return nil
}

type Turn struct {
	ID               string     `json:"id"`
	WorkspaceID      int64      `json:"workspace_id"`
	Generation       int64      `json:"generation"`
	Actor            Actor      `json:"actor"`
	AccountID        int64      `json:"account_id"`
	AccountUpdatedAt time.Time  `json:"account_updated_at"`
	ProxyUpdatedAt   *time.Time `json:"proxy_updated_at,omitempty"`
	State            State      `json:"state"`
	ProviderTurnID   string     `json:"provider_turn_id,omitempty"`
	Pricing          Pricing    `json:"pricing"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func ValidateLease(d time.Duration) error {
	if d < time.Second || d > 5*time.Minute {
		return fmt.Errorf("%w: lease duration out of range", ErrInvalid)
	}
	return nil
}

func validID(s string, limit int) bool {
	return strings.TrimSpace(s) == s && s != "" && len(s) <= limit && !strings.ContainsAny(s, "\x00\r\n")
}

type RuntimeStore interface {
	Bind(context.Context, Identity) (*Workspace, error)
	Reserve(context.Context, Reservation) (*Turn, Lease, error)
	Advance(context.Context, Lease, State, State, string) error
	Renew(context.Context, Lease, time.Duration) error
	ClaimRecovery(context.Context, int64, string, time.Duration) (*Turn, Lease, error)
	GetTurn(context.Context, string, Actor) (*Turn, error)
}
