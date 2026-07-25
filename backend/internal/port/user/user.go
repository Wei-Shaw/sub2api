// Package user contains the port interfaces (repository abstractions)
// for the user bounded context. DTO/value types live in internal/domain;
// this package only owns the persistence port contracts.
package user

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// Repository persists users and related profile/identity operations used by
// the user application service.
type Repository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, id int64) (*domain.User, error)
	// GetByIDIncludeDeleted bypasses soft-delete filtering (admin audit/usage).
	GetByIDIncludeDeleted(ctx context.Context, id int64) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetFirstAdmin(ctx context.Context) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
	Delete(ctx context.Context, id int64) error
	GetUserAvatar(ctx context.Context, userID int64) (*domain.UserAvatar, error)
	UpsertUserAvatar(ctx context.Context, userID int64, input domain.UpsertUserAvatarInput) (*domain.UserAvatar, error)
	DeleteUserAvatar(ctx context.Context, userID int64) error

	List(ctx context.Context, params pagination.PaginationParams) ([]domain.User, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters domain.UserListFilters) ([]domain.User, *pagination.PaginationResult, error)
	GetLatestUsedAtByUserIDs(ctx context.Context, userIDs []int64) (map[int64]*time.Time, error)
	GetLatestUsedAtByUserID(ctx context.Context, userID int64) (*time.Time, error)
	UpdateUserLastActiveAt(ctx context.Context, userID int64, activeAt time.Time) error

	UpdateBalance(ctx context.Context, id int64, amount float64) error
	DeductBalance(ctx context.Context, id int64, amount float64) error
	UpdateConcurrency(ctx context.Context, id int64, amount int) error
	BatchSetConcurrency(ctx context.Context, userIDs []int64, value int) (int, error)
	BatchAddConcurrency(ctx context.Context, userIDs []int64, delta int) (int, error)
	BatchUpdateLimits(ctx context.Context, userIDs []int64, concurrency, rpmLimit *int) (int, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error)
	// AddGroupToAllowedGroups incrementally adds a group to allowed_groups (idempotent).
	AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error
	// RemoveGroupFromUserAllowedGroups removes one group from a single user.
	RemoveGroupFromUserAllowedGroups(ctx context.Context, userID int64, groupID int64) error
	ListUserAuthIdentities(ctx context.Context, userID int64) ([]domain.UserAuthIdentityRecord, error)
	UnbindUserAuthProvider(ctx context.Context, userID int64, provider string) error

	// TOTP
	UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error
	EnableTotp(ctx context.Context, userID int64) error
	DisableTotp(ctx context.Context, userID int64) error
}

// RedeemAdjustmentRepository provides atomic floor-at-zero updates used by
// negative-value redeem codes. Narrower than Repository because normal usage
// billing is allowed to overdraw.
type RedeemAdjustmentRepository interface {
	ApplyRedeemBalanceAdjustment(ctx context.Context, id int64, delta float64) error
	ApplyRedeemConcurrencyAdjustment(ctx context.Context, id int64, delta int) error
}
