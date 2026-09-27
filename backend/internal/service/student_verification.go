package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// Student verification status constants.
const (
	StudentVerificationStatusActive  = "active"
	StudentVerificationStatusExpired = "expired"
	StudentVerificationStatusRevoked = "revoked"
)

const (
	// StudentVerificationValidityDaysDefault is applied when the admin has not
	// configured a validity period.
	StudentVerificationValidityDaysDefault = 365
	StudentVerificationValidityDaysMin     = 1
	StudentVerificationValidityDaysMax     = 3650
)

var (
	ErrStudentVerificationDisabled      = infraerrors.Forbidden("STUDENT_VERIFICATION_DISABLED", "student verification is not enabled")
	ErrStudentEmailDomainNotAllowed     = infraerrors.BadRequest("STUDENT_EMAIL_DOMAIN_NOT_ALLOWED", "email domain is not eligible for student verification")
	ErrStudentEmailClaimed              = infraerrors.Conflict("STUDENT_EMAIL_CLAIMED", "this email has already been verified by another account")
	ErrStudentVerificationNotFound      = infraerrors.NotFound("STUDENT_VERIFICATION_NOT_FOUND", "student verification record not found")
	ErrStudentVerificationInvalidStatus = infraerrors.BadRequest("STUDENT_VERIFICATION_INVALID_STATUS", "operation not allowed for the current verification status")
	ErrStudentVerificationRevoked       = infraerrors.Forbidden("STUDENT_VERIFICATION_REVOKED", "your student verification was revoked; contact an administrator")
)

// StudentVerification is the domain projection of a verified student claim.
type StudentVerification struct {
	ID                 int64
	UserID             int64
	Email              string // alias-normalized school email; globally unique
	EmailRaw           string // as entered by the user, for display
	Status             string // active / expired / revoked
	VerifiedAt         time.Time
	ExpiresAt          time.Time
	RevokedAt          *time.Time
	RevokedBy          *int64
	RevokeReason       *string
	RebateRateApplied  *float64 // exclusive rebate rate granted by this verification
	PreviousRebateRate *float64 // user's own exclusive rate before verification, for restore
	GrantedGroupIDs    []int64  // student groups actually granted at verify time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// IsEntitled reports whether the record currently grants student benefits.
func (v *StudentVerification) IsEntitled(now time.Time) bool {
	return v != nil && v.Status == StudentVerificationStatusActive && now.Before(v.ExpiresAt)
}

// StudentVerificationGroupGranter is the narrow slice of UserRepository the
// service needs to grant/revoke student groups.
type StudentVerificationGroupGranter interface {
	AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error
	RemoveGroupFromUserAllowedGroups(ctx context.Context, userID int64, groupID int64) error
}

// StudentVerificationAffiliateRepo is the narrow slice of AffiliateRepository
// the service needs to apply/restore the exclusive rebate rate.
type StudentVerificationAffiliateRepo interface {
	EnsureUserAffiliate(ctx context.Context, userID int64) (*AffiliateSummary, error)
	SetUserRebateRate(ctx context.Context, userID int64, ratePercent *float64) error
}

// StudentVerificationEmailVerifier is the narrow slice of EmailService used
// for school-email verification codes.
type StudentVerificationEmailVerifier interface {
	SendVerifyCode(ctx context.Context, email, siteName string, locale ...string) error
	VerifyCode(ctx context.Context, email, code string) error
}

// StudentVerificationRepository persists verification records. Implementations
// must honor ent transactions attached to ctx (dbent.NewTxContext) so the
// service can commit record + entitlements atomically.
type StudentVerificationRepository interface {
	GetByID(ctx context.Context, id int64) (*StudentVerification, error)
	GetByUserID(ctx context.Context, userID int64) (*StudentVerification, error)
	GetByEmail(ctx context.Context, normalizedEmail string) (*StudentVerification, error)
	// UpsertActive inserts a new record or rewrites the caller's existing row
	// (unique on user_id). A unique conflict on email returns
	// ErrStudentEmailClaimed.
	UpsertActive(ctx context.Context, v *StudentVerification) error
	// UpdateStatus transitions a record to a terminal/non-active status.
	UpdateStatus(ctx context.Context, id int64, status string, revokedBy *int64, reason *string) error
	// ListActiveExpired returns active records whose expires_at <= now, for the
	// periodic sweeper.
	ListActiveExpired(ctx context.Context, now time.Time, limit int) ([]StudentVerification, error)
	// List returns paginated records for admin, newest first.
	List(ctx context.Context, params pagination.PaginationParams, status string, keyword string) ([]StudentVerification, *pagination.PaginationResult, error)
	Delete(ctx context.Context, id int64) error
}
