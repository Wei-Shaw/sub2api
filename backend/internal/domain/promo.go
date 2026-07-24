package domain

import (
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Promo-domain errors used by persistence and application layers.
var (
	ErrPromoCodeNotFound    = infraerrors.NotFound("PROMO_CODE_NOT_FOUND", "promo code not found")
	ErrPromoCodeExpired     = infraerrors.BadRequest("PROMO_CODE_EXPIRED", "promo code has expired")
	ErrPromoCodeDisabled    = infraerrors.BadRequest("PROMO_CODE_DISABLED", "promo code is disabled")
	ErrPromoCodeMaxUsed     = infraerrors.BadRequest("PROMO_CODE_MAX_USED", "promo code has reached maximum uses")
	ErrPromoCodeAlreadyUsed = infraerrors.Conflict("PROMO_CODE_ALREADY_USED", "you have already used this promo code")
	ErrPromoCodeInvalid     = infraerrors.BadRequest("PROMO_CODE_INVALID", "invalid promo code")
)

// PromoCode is a registration bonus code aggregate.
type PromoCode struct {
	ID          int64
	Code        string
	BonusAmount float64
	MaxUses     int
	UsedCount   int
	Status      string
	ExpiresAt   *time.Time
	Notes       string
	CreatedAt   time.Time
	UpdatedAt   time.Time

	// UsageRecords is an optional association, populated only when requested.
	UsageRecords []PromoCodeUsage
}

// PromoCodeUsage records one user redemption of a promo code.
type PromoCodeUsage struct {
	ID          int64
	PromoCodeID int64
	UserID      int64
	BonusAmount float64
	UsedAt      time.Time

	// Optional associations
	PromoCode *PromoCode
	// User is a shallow projection for admin usage lists. Full user ownership
	// remains outside this BC until User is extracted to domain.
	User *PromoUsageUser
}

// PromoUsageUser is the minimal user projection needed by promo usage views.
// It deliberately avoids depending on a full User aggregate still owned by service.
type PromoUsageUser struct {
	ID       int64
	Email    string
	Username string
	Role     string
	Balance  float64
	Status   string
}

// CanUse reports whether the promo code is currently redeemable.
func (p *PromoCode) CanUse() bool {
	if p == nil {
		return false
	}
	if p.Status != PromoCodeStatusActive {
		return false
	}
	if p.ExpiresAt != nil && time.Now().After(*p.ExpiresAt) {
		return false
	}
	if p.MaxUses > 0 && p.UsedCount >= p.MaxUses {
		return false
	}
	return true
}

// IsExpired reports whether the promo code is past its expiry time.
func (p *PromoCode) IsExpired() bool {
	if p == nil {
		return false
	}
	return p.ExpiresAt != nil && time.Now().After(*p.ExpiresAt)
}
