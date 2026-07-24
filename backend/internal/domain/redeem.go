package domain

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Redeem-domain errors used by persistence and application layers.
// ErrInsufficientBalance stays in service: it is shared with billing paths and
// is not a redeem-code aggregate error.
var (
	ErrRedeemCodeNotFound = infraerrors.NotFound("REDEEM_CODE_NOT_FOUND", "redeem code not found")
	ErrRedeemCodeUsed     = infraerrors.Conflict("REDEEM_CODE_USED", "redeem code already used")
	ErrRedeemCodeExpired  = infraerrors.Conflict("REDEEM_CODE_EXPIRED", "redeem code expired")
	ErrRedeemRateLimited  = infraerrors.TooManyRequests("REDEEM_RATE_LIMITED", "too many failed attempts, please try again later")
	ErrRedeemCodeLocked   = infraerrors.Conflict("REDEEM_CODE_LOCKED", "redeem code is being processed, please try again")
)

// RedeemCode is a redeem / recharge code aggregate.
type RedeemCode struct {
	ID        int64
	Code      string
	Type      string
	Value     float64
	Status    string
	UsedBy    *int64
	UsedAt    *time.Time
	Notes     string
	CreatedAt time.Time
	ExpiresAt *time.Time

	GroupID      *int64
	ValidityDays int

	// Optional associations for list/detail views. Full User/Group aggregates
	// remain outside this BC until those entities are extracted to domain.
	User  *RedeemUsageUser
	Group *RedeemUsageGroup
}

// RedeemUsageUser is the minimal user projection needed by redeem list views.
type RedeemUsageUser struct {
	ID       int64
	Email    string
	Username string
	Role     string
	Balance  float64
	Status   string
}

// RedeemUsageGroup is the minimal group projection needed by redeem list views.
// Admin UI currently only needs id/name; other fields may be filled opportunistically.
type RedeemUsageGroup struct {
	ID               int64
	Name             string
	Platform         string
	Status           string
	SubscriptionType string
}

func (r *RedeemCode) IsUsed() bool {
	return r != nil && r.Status == StatusUsed
}

func (r *RedeemCode) IsExpired() bool {
	return r.IsExpiredAt(time.Now())
}

func (r *RedeemCode) IsExpiredAt(now time.Time) bool {
	if r == nil {
		return false
	}
	if r.Status == StatusExpired {
		return true
	}
	return r.Status == StatusUnused && r.ExpiresAt != nil && !r.ExpiresAt.After(now)
}

func (r *RedeemCode) CanUse() bool {
	return r != nil && r.Status == StatusUnused && !r.IsExpired()
}

// GenerateRedeemCode returns a random 32-char hex redeem code.
func GenerateRedeemCode() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
