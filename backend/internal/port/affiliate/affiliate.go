// Package affiliate contains the port interfaces (repository abstractions)
// for the affiliate bounded context. DTO/value types live in internal/domain;
// this package only owns the persistence port contracts and query filters.
package affiliate

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// AdminFilter is the list filter for users with custom affiliate settings.
type AdminFilter struct {
	Search   string
	Page     int
	PageSize int
}

// RecordFilter is the shared filter for admin invite/rebate/transfer records.
type RecordFilter struct {
	Search   string
	Page     int
	PageSize int
	StartAt  *time.Time
	EndAt    *time.Time
	SortBy   string
	SortDesc bool
}

// Repository persists affiliate profiles, rebates, and admin projections.
type Repository interface {
	EnsureUserAffiliate(ctx context.Context, userID int64) (*domain.AffiliateSummary, error)
	GetAffiliateByCode(ctx context.Context, code string) (*domain.AffiliateSummary, error)
	BindInviter(ctx context.Context, userID, inviterID int64) (bool, error)
	AccrueQuota(ctx context.Context, inviterID, inviteeUserID int64, amount float64, freezeHours int, sourceOrderID *int64) (bool, error)
	GetAccruedRebateFromInvitee(ctx context.Context, inviterID, inviteeUserID int64) (float64, error)
	ThawFrozenQuota(ctx context.Context, userID int64) (float64, error)
	TransferQuotaToBalance(ctx context.Context, userID int64) (float64, float64, error)
	ListInvitees(ctx context.Context, inviterID int64, limit int) ([]domain.AffiliateInvitee, error)

	// Admin: per-user exclusive configuration
	UpdateUserAffCode(ctx context.Context, userID int64, newCode string) error
	ResetUserAffCode(ctx context.Context, userID int64) (string, error)
	SetUserRebateRate(ctx context.Context, userID int64, ratePercent *float64) error
	BatchSetUserRebateRate(ctx context.Context, userIDs []int64, ratePercent *float64) error
	ListUsersWithCustomSettings(ctx context.Context, filter AdminFilter) ([]domain.AffiliateAdminEntry, int64, error)
	ListAffiliateInviteRecords(ctx context.Context, filter RecordFilter) ([]domain.AffiliateInviteRecord, int64, error)
	ListAffiliateRebateRecords(ctx context.Context, filter RecordFilter) ([]domain.AffiliateRebateRecord, int64, error)
	ListAffiliateTransferRecords(ctx context.Context, filter RecordFilter) ([]domain.AffiliateTransferRecord, int64, error)
	GetAffiliateUserOverview(ctx context.Context, userID int64) (*domain.AffiliateUserOverview, error)
}
