// Package redeem contains the port interfaces (repository/cache abstractions)
// for the redeem-code bounded context. DTO/value types live in internal/domain;
// this package only owns the persistence/cache port contracts and related
// update-field value objects used by those contracts.
package redeem

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// NullableTimeUpdate models an optional time pointer update (set/clear).
type NullableTimeUpdate struct {
	Set   bool
	Value *time.Time
}

// NullableInt64Update models an optional int64 pointer update (set/clear).
type NullableInt64Update struct {
	Set   bool
	Value *int64
}

// RedeemCodeBatchUpdateFields holds partial update fields for bulk redeem edits.
type RedeemCodeBatchUpdateFields struct {
	Status    *string
	ExpiresAt NullableTimeUpdate
	Notes     *string
	GroupID   NullableInt64Update

	// Core fields are intentionally modeled only so service validation can
	// reject payloads that try to mutate redemption value semantics in bulk.
	Type  *string
	Value *float64
}

func (f RedeemCodeBatchUpdateFields) HasChanges() bool {
	return f.Status != nil ||
		f.ExpiresAt.Set ||
		f.Notes != nil ||
		f.GroupID.Set ||
		f.Type != nil ||
		f.Value != nil
}

func (f RedeemCodeBatchUpdateFields) HasCoreFieldChanges() bool {
	return f.Type != nil || f.Value != nil
}

func (f RedeemCodeBatchUpdateFields) TouchesUsedSensitiveFields() bool {
	return f.Status != nil || f.ExpiresAt.Set || f.GroupID.Set
}

// RedeemCodeRepository persists redeem codes.
type RedeemCodeRepository interface {
	Create(ctx context.Context, code *domain.RedeemCode) error
	CreateBatch(ctx context.Context, codes []domain.RedeemCode) error
	GetByID(ctx context.Context, id int64) (*domain.RedeemCode, error)
	GetByCode(ctx context.Context, code string) (*domain.RedeemCode, error)
	Update(ctx context.Context, code *domain.RedeemCode) error
	BatchUpdate(ctx context.Context, ids []int64, fields RedeemCodeBatchUpdateFields) (int64, error)
	Delete(ctx context.Context, id int64) error
	Use(ctx context.Context, id, userID int64) error

	List(ctx context.Context, params pagination.PaginationParams) ([]domain.RedeemCode, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, codeType, status, search string) ([]domain.RedeemCode, *pagination.PaginationResult, error)
	ListByUser(ctx context.Context, userID int64, limit int) ([]domain.RedeemCode, error)
	// ListByUserPaginated returns paginated balance/concurrency history for a specific user.
	// codeType filter is optional - pass empty string to return all types.
	ListByUserPaginated(ctx context.Context, userID int64, params pagination.PaginationParams, codeType string) ([]domain.RedeemCode, *pagination.PaginationResult, error)
	// SumPositiveBalanceByUser returns the total recharged amount (sum of positive balance values) for a user.
	SumPositiveBalanceByUser(ctx context.Context, userID int64) (float64, error)
}

// RedeemCache defines cache operations for redeem service (rate limit + locks).
type RedeemCache interface {
	GetRedeemAttemptCount(ctx context.Context, userID int64) (int, error)
	IncrementRedeemAttemptCount(ctx context.Context, userID int64) error

	AcquireRedeemLock(ctx context.Context, code string, ttl time.Duration) (bool, error)
	ReleaseRedeemLock(ctx context.Context, code string) error
}
