// Package apikey contains the port interfaces (repository abstractions)
// for the API-key bounded context. DTO/value types live in internal/domain;
// this package only owns the persistence port contracts.
package apikey

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// Repository persists API keys and related quota/rate-limit operations used by
// the API-key application service.
type Repository interface {
	Create(ctx context.Context, key *domain.APIKey) error
	GetByID(ctx context.Context, id int64) (*domain.APIKey, error)
	// GetKeyAndOwnerID fetches only the key credential and owner id (lightweight delete path).
	GetKeyAndOwnerID(ctx context.Context, id int64) (string, int64, error)
	GetByKey(ctx context.Context, key string) (*domain.APIKey, error)
	// GetByKeyForAuth is the auth-path query that returns a minimal field set.
	GetByKeyForAuth(ctx context.Context, key string) (*domain.APIKey, error)
	Update(ctx context.Context, key *domain.APIKey) error
	Delete(ctx context.Context, id int64) error
	// DeleteWithAudit keeps the legacy interface name for rolling-upgrade compatibility.
	// Implementations must tombstone the key and soft-delete it atomically without
	// retaining the deleted credential material.
	DeleteWithAudit(ctx context.Context, id int64) error

	ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams, filters domain.APIKeyListFilters) ([]domain.APIKey, *pagination.PaginationResult, error)
	VerifyOwnership(ctx context.Context, userID int64, apiKeyIDs []int64) ([]int64, error)
	CountByUserID(ctx context.Context, userID int64) (int64, error)
	ExistsByKey(ctx context.Context, key string) (bool, error)
	ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]domain.APIKey, *pagination.PaginationResult, error)
	SearchAPIKeys(ctx context.Context, userID int64, keyword string, limit int) ([]domain.APIKey, error)
	ClearGroupIDByGroupID(ctx context.Context, groupID int64) (int64, error)
	// UpdateGroupIDByUserAndGroup migrates all keys for a user from oldGroupID to newGroupID.
	UpdateGroupIDByUserAndGroup(ctx context.Context, userID, oldGroupID, newGroupID int64) (int64, error)
	CountByGroupID(ctx context.Context, groupID int64) (int64, error)
	ListKeysByUserID(ctx context.Context, userID int64) ([]string, error)
	ListKeysByGroupID(ctx context.Context, groupID int64) ([]string, error)

	// Quota methods
	IncrementQuotaUsed(ctx context.Context, id int64, amount float64) (float64, error)
	UpdateLastUsed(ctx context.Context, id int64, usedAt time.Time) error

	// Rate limit methods
	IncrementRateLimitUsage(ctx context.Context, id int64, cost float64) error
	ResetRateLimitWindows(ctx context.Context, id int64) error
	GetRateLimitData(ctx context.Context, id int64) (*domain.APIKeyRateLimitData, error)
}
