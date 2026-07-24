// Package promo contains the port interfaces (repository abstractions)
// for the promo-code bounded context. DTO/value types live in internal/domain;
// this package only owns the persistence port contracts.
package promo

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// PromoCodeRepository persists promo codes and their usage records.
type PromoCodeRepository interface {
	// Basic CRUD
	Create(ctx context.Context, code *domain.PromoCode) error
	GetByID(ctx context.Context, id int64) (*domain.PromoCode, error)
	GetByCode(ctx context.Context, code string) (*domain.PromoCode, error)
	// GetByCodeForUpdate loads a promo code with a row lock for concurrent redeem paths.
	GetByCodeForUpdate(ctx context.Context, code string) (*domain.PromoCode, error)
	Update(ctx context.Context, code *domain.PromoCode) error
	Delete(ctx context.Context, id int64) error

	// List queries
	List(ctx context.Context, params pagination.PaginationParams) ([]domain.PromoCode, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, status, search string) ([]domain.PromoCode, *pagination.PaginationResult, error)

	// Usage records
	CreateUsage(ctx context.Context, usage *domain.PromoCodeUsage) error
	GetUsageByPromoCodeAndUser(ctx context.Context, promoCodeID, userID int64) (*domain.PromoCodeUsage, error)
	ListUsagesByPromoCode(ctx context.Context, promoCodeID int64, params pagination.PaginationParams) ([]domain.PromoCodeUsage, *pagination.PaginationResult, error)

	// Counters
	IncrementUsedCount(ctx context.Context, id int64) error
}
