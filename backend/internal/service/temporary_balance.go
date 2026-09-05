package service

import (
	"context"
	"math"
	"net/http"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	// Keep grants bounded so a malformed admin request cannot create an
	// effectively unbounded billing liability.
	maxTemporaryBalanceAmount   = 1_000_000_000.0
	maxTemporaryBalanceLifetime = 10 * 365 * 24 * time.Hour
)

var (
	ErrTemporaryBalanceInvalidAmount = infraerrors.BadRequest("TEMPORARY_BALANCE_INVALID_AMOUNT", "temporary balance amount must be finite and greater than zero")
	ErrTemporaryBalanceInvalidExpiry = infraerrors.BadRequest("TEMPORARY_BALANCE_INVALID_EXPIRY", "temporary balance expiry must be in the future and within ten years")
	ErrTemporaryBalanceUnavailable   = infraerrors.New(http.StatusNotImplemented, "TEMPORARY_BALANCE_UNAVAILABLE", "temporary balance capability is not configured")
)

func validateTemporaryBalanceGrant(amount float64, expiresAt, now time.Time) error {
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 || amount > maxTemporaryBalanceAmount {
		return ErrTemporaryBalanceInvalidAmount
	}
	if expiresAt.IsZero() || !expiresAt.After(now) || expiresAt.After(now.Add(maxTemporaryBalanceLifetime)) {
		return ErrTemporaryBalanceInvalidExpiry
	}
	return nil
}

// GrantUserTemporaryBalance atomically adds an expiring grant and returns the
// refreshed user. The repository implementation also writes an immutable audit
// row; service-level validation prevents malformed values reaching SQL.
func (s *adminServiceImpl) GrantUserTemporaryBalance(ctx context.Context, userID int64, amount float64, expiresAt time.Time, actorAdminID int64, notes string) (*User, error) {
	if userID <= 0 {
		return nil, ErrUserNotFound
	}
	if err := validateTemporaryBalanceGrant(amount, expiresAt.UTC(), time.Now().UTC()); err != nil {
		return nil, err
	}
	repo, ok := s.userRepo.(TemporaryBalanceRepository)
	if !ok {
		return nil, ErrTemporaryBalanceUnavailable
	}
	if _, err := repo.GrantTemporaryBalance(ctx, userID, amount, expiresAt.UTC(), actorAdminID, notes); err != nil {
		return nil, err
	}
	// Cache invalidation must not inherit a canceled admin request context: the
	// grant is already committed and stale auth/billing entries would otherwise
	// remain valid until their TTL expires.
	cacheCtx, cancelCache := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelCache()
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(cacheCtx, userID)
	}
	if s.billingCacheService != nil {
		// Invalidate synchronously so a subsequent request cannot observe the
		// pre-grant balance. The database write remains authoritative if Redis is
		// unavailable or the invalidation call fails.
		_ = s.billingCacheService.InvalidateUserBalance(cacheCtx, userID)
	}
	return s.GetUser(ctx, userID)
}

// ClearExpiredTemporaryBalances clears expired grants in a bounded batch. It
// is intended for a scheduled maintenance worker, not a request hot path.
func (s *adminServiceImpl) ClearExpiredTemporaryBalances(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 1000
	}
	repo, ok := s.userRepo.(TemporaryBalanceRepository)
	if !ok {
		return 0, ErrTemporaryBalanceUnavailable
	}
	if users, usersOK := repo.(temporaryBalanceCleanupUsers); usersOK {
		ids, err := users.ClearExpiredTemporaryBalanceUsers(ctx, limit)
		if err != nil {
			return 0, err
		}
		for _, userID := range ids {
			if s.billingCacheService != nil {
				_ = s.billingCacheService.InvalidateUserBalance(ctx, userID)
			}
			if s.authCacheInvalidator != nil {
				s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
			}
		}
		return len(ids), nil
	}
	return repo.ClearExpiredTemporaryBalances(ctx, limit)
}

func hydrateTemporaryBalance(ctx context.Context, repo UserRepository, user *User) error {
	if user == nil {
		return nil
	}
	reader, ok := repo.(TemporaryBalanceRepository)
	if !ok {
		return nil
	}
	grant, err := reader.GetTemporaryBalance(ctx, user.ID)
	if err != nil {
		return err
	}
	applyTemporaryBalanceGrant(user, grant)
	return nil
}

func hydrateTemporaryBalances(ctx context.Context, repo UserRepository, users []User) error {
	if len(users) == 0 {
		return nil
	}
	reader, ok := repo.(TemporaryBalanceRepository)
	if !ok {
		return nil
	}
	ids := make([]int64, 0, len(users))
	for i := range users {
		ids = append(ids, users[i].ID)
	}
	grants, err := reader.GetTemporaryBalances(ctx, ids)
	if err != nil {
		return err
	}
	for i := range users {
		applyTemporaryBalanceGrant(&users[i], grants[users[i].ID])
	}
	return nil
}

func applyTemporaryBalanceGrant(user *User, grant *TemporaryBalanceGrant) {
	if user == nil {
		return
	}
	user.TemporaryBalance = 0
	user.ActiveTemporaryBalance = 0
	user.TemporaryBalanceExpiresAt = nil
	if grant == nil {
		return
	}
	user.TemporaryBalance = grant.Amount
	user.ActiveTemporaryBalance = grant.ActiveAmount
	user.TemporaryBalanceExpiresAt = grant.ExpiresAt
}
