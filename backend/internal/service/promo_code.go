package service

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// Type aliases keep existing service call sites compiling while the promo BC
// owns its domain types. Mirror of announcement.go.

type PromoCode = domain.PromoCode

type PromoCodeUsage = domain.PromoCodeUsage

type PromoUsageUser = domain.PromoUsageUser

// CreatePromoCodeInput is an application-layer command DTO (stays in service).
type CreatePromoCodeInput struct {
	Code        string
	BonusAmount float64
	MaxUses     int
	ExpiresAt   *time.Time
	Notes       string
}

// UpdatePromoCodeInput is an application-layer command DTO (stays in service).
type UpdatePromoCodeInput struct {
	Code        *string
	BonusAmount *float64
	MaxUses     *int
	Status      *string
	ExpiresAt   *time.Time
	Notes       *string
}
