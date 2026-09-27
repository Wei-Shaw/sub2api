package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// StudentVerificationStatusResult is the user-facing view of the feature.
type StudentVerificationStatusResult struct {
	Enabled      bool                 `json:"enabled"`
	Verification *StudentVerification `json:"verification,omitempty"`
	RebateRate   float64              `json:"rebate_rate"`
	GroupCount   int                  `json:"group_count"`
}

// StudentVerificationService grants student entitlements (exclusive groups +
// exclusive affiliate rebate rate) after a school-email code check.
type StudentVerificationService struct {
	repo                 StudentVerificationRepository
	userRepo             StudentVerificationGroupGranter
	groupReader          DefaultSubscriptionGroupReader
	affiliateRepo        StudentVerificationAffiliateRepo
	settingService       *SettingService
	emailService         StudentVerificationEmailVerifier
	entClient            *dbent.Client
	authCacheInvalidator APIKeyAuthCacheInvalidator

	interval time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func NewStudentVerificationService(
	repo StudentVerificationRepository,
	userRepo StudentVerificationGroupGranter,
	groupReader DefaultSubscriptionGroupReader,
	affiliateRepo StudentVerificationAffiliateRepo,
	settingService *SettingService,
	emailService StudentVerificationEmailVerifier,
	entClient *dbent.Client,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
) *StudentVerificationService {
	return &StudentVerificationService{
		repo:                 repo,
		userRepo:             userRepo,
		groupReader:          groupReader,
		affiliateRepo:        affiliateRepo,
		settingService:       settingService,
		emailService:         emailService,
		entClient:            entClient,
		authCacheInvalidator: authCacheInvalidator,
		interval:             24 * time.Hour,
		stopCh:               make(chan struct{}),
	}
}

// Start launches the periodic expiry sweeper. Safe to leave unstarted in tests.
func (s *StudentVerificationService) Start() {
	if s == nil || s.repo == nil || s.interval <= 0 {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.sweepOnce()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.sweepOnce()
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *StudentVerificationService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { close(s.stopCh) })
	s.wg.Wait()
}

func (s *StudentVerificationService) sweepOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Note: not gated on the feature flag — expired entitlements must be
	// stripped even while the feature is disabled.
	processed, err := s.SweepExpired(ctx, 500)
	if err != nil {
		slog.Error("[StudentVerification] sweep failed", "error", err)
		return
	}
	if processed > 0 {
		slog.Info("[StudentVerification] expired verifications processed", "count", processed)
	}
}

// GetStatus returns the current user's verification record (if any) plus the
// effective feature configuration.
func (s *StudentVerificationService) GetStatus(ctx context.Context, userID int64) (*StudentVerificationStatusResult, error) {
	result := &StudentVerificationStatusResult{
		Enabled: s.isEnabled(ctx),
	}
	if s.settingService != nil {
		result.RebateRate = s.settingService.GetStudentVerificationRebateRatePercent(ctx)
		result.GroupCount = len(s.resolveGrantableGroupIDs(ctx))
	}
	v, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrStudentVerificationNotFound) {
			return result, nil
		}
		return nil, err
	}
	// Surface expired state immediately even before the sweeper runs.
	if v.Status == StudentVerificationStatusActive && !v.IsEntitled(time.Now().UTC()) {
		v.Status = StudentVerificationStatusExpired
	}
	result.Verification = v
	return result, nil
}

// SendCode validates the candidate school email and dispatches a verification
// code. The email must pass the admin-configured domain whitelist and must not
// be claimed by another account.
func (s *StudentVerificationService) SendCode(ctx context.Context, userID int64, email string, locale ...string) error {
	if s == nil || s.repo == nil {
		return ErrServiceUnavailable
	}
	if !s.isEnabled(ctx) {
		return ErrStudentVerificationDisabled
	}
	normalized, raw, err := normalizeStudentVerificationEmail(email)
	if err != nil {
		return err
	}
	if s.settingService == nil || !s.settingService.IsStudentVerificationEmailAllowed(ctx, normalized) {
		return ErrStudentEmailDomainNotAllowed
	}
	if s.emailService == nil {
		return ErrServiceUnavailable
	}
	if err := s.ensureUserNotRevoked(ctx, userID); err != nil {
		return err
	}
	if err := s.ensureEmailClaimableBy(ctx, userID, normalized); err != nil {
		return err
	}
	siteName := "Sub2API"
	if s.settingService != nil {
		siteName = s.settingService.GetSiteName(ctx)
	}
	return s.emailService.SendVerifyCode(ctx, raw, siteName, firstEmailLocale(locale))
}

// Verify checks the emailed code, claims the school email, and grants student
// entitlements atomically. Re-verification by the same user renews the period
// or switches to a new school email (releasing the previous claim).
func (s *StudentVerificationService) Verify(ctx context.Context, userID int64, email, code string) (*StudentVerification, error) {
	if s == nil || s.repo == nil {
		return nil, ErrServiceUnavailable
	}
	if !s.isEnabled(ctx) {
		return nil, ErrStudentVerificationDisabled
	}
	normalized, raw, err := normalizeStudentVerificationEmail(email)
	if err != nil {
		return nil, err
	}
	if s.settingService == nil || !s.settingService.IsStudentVerificationEmailAllowed(ctx, normalized) {
		return nil, ErrStudentEmailDomainNotAllowed
	}
	if s.emailService == nil {
		return nil, ErrServiceUnavailable
	}
	if err := s.ensureUserNotRevoked(ctx, userID); err != nil {
		return nil, err
	}
	if err := s.ensureEmailClaimableBy(ctx, userID, normalized); err != nil {
		return nil, err
	}
	if err := s.emailService.VerifyCode(ctx, raw, code); err != nil {
		return nil, err
	}

	validityDays := s.settingService.GetStudentVerificationValidityDays(ctx)
	now := time.Now().UTC()
	groupIDs := s.resolveGrantableGroupIDs(ctx)
	rebateRate := s.settingService.GetStudentVerificationRebateRatePercent(ctx)

	apply := func(txCtx context.Context) (*StudentVerification, error) {
		v := &StudentVerification{
			UserID:          userID,
			Email:           normalized,
			EmailRaw:        raw,
			Status:          StudentVerificationStatusActive,
			VerifiedAt:      now,
			ExpiresAt:       now.AddDate(0, 0, validityDays),
			GrantedGroupIDs: groupIDs,
		}

		var existing *StudentVerification
		existing, err := s.repo.GetByUserID(txCtx, userID)
		switch {
		case err == nil:
		case errors.Is(err, ErrStudentVerificationNotFound):
			existing = nil
		default:
			return nil, err
		}
		// A revoked record blocks re-verification until an admin deletes it;
		// otherwise revocation would be trivially bypassed by re-verifying.
		if existing != nil && existing.Status == StudentVerificationStatusRevoked {
			return nil, ErrStudentVerificationRevoked
		}

		if rebateRate > 0 && s.affiliateRepo != nil {
			summary, err := s.affiliateRepo.EnsureUserAffiliate(txCtx, userID)
			if err != nil {
				return nil, fmt.Errorf("ensure affiliate profile: %w", err)
			}
			current := summary.AffRebateRatePercent
			if existing != nil && existing.RebateRateApplied != nil &&
				current != nil && *current == *existing.RebateRateApplied {
				// The previous student rate is still in effect — preserve the
				// pre-student baseline rather than treating our own rate as it.
				v.PreviousRebateRate = existing.PreviousRebateRate
			} else {
				v.PreviousRebateRate = current
			}
			applied := rebateRate
			v.RebateRateApplied = &applied
		}

		if err := s.repo.UpsertActive(txCtx, v); err != nil {
			return nil, err
		}
		for _, gid := range groupIDs {
			if err := s.userRepo.AddGroupToAllowedGroups(txCtx, userID, gid); err != nil {
				return nil, fmt.Errorf("grant student group %d: %w", gid, err)
			}
		}
		if v.RebateRateApplied != nil && s.affiliateRepo != nil {
			if err := s.affiliateRepo.SetUserRebateRate(txCtx, userID, v.RebateRateApplied); err != nil {
				return nil, fmt.Errorf("set student rebate rate: %w", err)
			}
		}
		return v, nil
	}

	v, err := s.inTx(ctx, apply)
	if err != nil {
		return nil, err
	}
	s.invalidateUserAuthCache(ctx, userID)
	return v, nil
}

// AdminList returns paginated verification records for the admin console.
func (s *StudentVerificationService) AdminList(ctx context.Context, params pagination.PaginationParams, status, keyword string) ([]StudentVerification, *pagination.PaginationResult, error) {
	if s == nil || s.repo == nil {
		return nil, nil, ErrServiceUnavailable
	}
	if status != "" {
		switch status {
		case StudentVerificationStatusActive, StudentVerificationStatusExpired, StudentVerificationStatusRevoked:
		default:
			return nil, nil, ErrStudentVerificationInvalidStatus
		}
	}
	return s.repo.List(ctx, params, status, keyword)
}

// AdminRevoke strips entitlements and marks the record revoked. The email claim
// stays in place — only AdminDelete releases the email for another account.
func (s *StudentVerificationService) AdminRevoke(ctx context.Context, id int64, adminID int64, reason string) error {
	v, err := s.transitionLocked(ctx, id, StudentVerificationStatusRevoked, &adminID, strPtrOrNil(reason))
	if err != nil {
		return err
	}
	s.invalidateUserAuthCache(ctx, v.UserID)
	return nil
}

// AdminDelete removes the record entirely, releasing the email claim. If the
// record still grants entitlements they are stripped first.
func (s *StudentVerificationService) AdminDelete(ctx context.Context, id int64) error {
	if s == nil || s.repo == nil {
		return ErrServiceUnavailable
	}
	v, err := s.inTx(ctx, func(txCtx context.Context) (*StudentVerification, error) {
		v, err := s.repo.GetByID(txCtx, id)
		if err != nil {
			return nil, err
		}
		if v.Status == StudentVerificationStatusActive {
			if err := s.removeEntitlements(txCtx, v); err != nil {
				return nil, err
			}
		}
		if err := s.repo.Delete(txCtx, id); err != nil {
			return nil, err
		}
		return v, nil
	})
	if err != nil {
		return err
	}
	s.invalidateUserAuthCache(ctx, v.UserID)
	return nil
}

// SweepExpired transitions due records to expired and strips entitlements.
// Returns the number of records processed.
func (s *StudentVerificationService) SweepExpired(ctx context.Context, limit int) (int, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	expired, err := s.repo.ListActiveExpired(ctx, time.Now().UTC(), limit)
	if err != nil {
		return 0, err
	}
	for i := range expired {
		if _, err := s.transitionLocked(ctx, expired[i].ID, StudentVerificationStatusExpired, nil, nil); err != nil {
			slog.Error("[StudentVerification] expire record failed", "id", expired[i].ID, "error", err)
			continue
		}
		s.invalidateUserAuthCache(ctx, expired[i].UserID)
	}
	return len(expired), nil
}

// transitionLocked runs the terminal-state transition plus entitlement removal
// in one transaction. status is "expired" or "revoked".
func (s *StudentVerificationService) transitionLocked(ctx context.Context, id int64, status string, revokedBy *int64, reason *string) (*StudentVerification, error) {
	return s.inTx(ctx, func(txCtx context.Context) (*StudentVerification, error) {
		v, err := s.repo.GetByID(txCtx, id)
		if err != nil {
			return nil, err
		}
		if v.Status != StudentVerificationStatusActive {
			return nil, ErrStudentVerificationInvalidStatus
		}
		if err := s.removeEntitlements(txCtx, v); err != nil {
			return nil, err
		}
		if err := s.repo.UpdateStatus(txCtx, id, status, revokedBy, reason); err != nil {
			return nil, err
		}
		v.Status = status
		return v, nil
	})
}

// removeEntitlements drops the groups granted at verify time and restores the
// pre-verification affiliate rate. An admin-set rate different from the applied
// student rate is left untouched. Orphaned records (user deleted, user_id==0)
// only need the status flip — their groups/affiliate rows are already gone.
func (s *StudentVerificationService) removeEntitlements(ctx context.Context, v *StudentVerification) error {
	if v.UserID <= 0 {
		return nil
	}
	for _, gid := range v.GrantedGroupIDs {
		if err := s.userRepo.RemoveGroupFromUserAllowedGroups(ctx, v.UserID, gid); err != nil {
			return fmt.Errorf("remove student group %d: %w", gid, err)
		}
	}
	if v.RebateRateApplied != nil && s.affiliateRepo != nil {
		summary, err := s.affiliateRepo.EnsureUserAffiliate(ctx, v.UserID)
		if err != nil {
			return fmt.Errorf("ensure affiliate profile: %w", err)
		}
		if summary.AffRebateRatePercent != nil && *summary.AffRebateRatePercent == *v.RebateRateApplied {
			if err := s.affiliateRepo.SetUserRebateRate(ctx, v.UserID, v.PreviousRebateRate); err != nil {
				return fmt.Errorf("restore rebate rate: %w", err)
			}
		}
	}
	return nil
}

// resolveGrantableGroupIDs filters the configured student group IDs down to
// groups that still qualify (exclusive standard groups that are not deleted).
func (s *StudentVerificationService) resolveGrantableGroupIDs(ctx context.Context) []int64 {
	if s.settingService == nil || s.groupReader == nil {
		return []int64{}
	}
	ids := s.settingService.GetStudentVerificationGroupIDs(ctx)
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		group, err := s.groupReader.GetByID(ctx, id)
		if err != nil || group == nil {
			slog.Warn("[StudentVerification] configured group missing", "group_id", id)
			continue
		}
		if !group.IsExclusive || group.IsSubscriptionType() || strings.EqualFold(group.Status, "deleted") {
			slog.Warn("[StudentVerification] configured group not exclusive standard", "group_id", id)
			continue
		}
		out = append(out, id)
	}
	return out
}

// ensureUserNotRevoked rejects verify attempts when the user's own record was
// revoked by an admin. Expired records may renew freely.
func (s *StudentVerificationService) ensureUserNotRevoked(ctx context.Context, userID int64) error {
	existing, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrStudentVerificationNotFound) {
			return nil
		}
		return err
	}
	if existing.Status == StudentVerificationStatusRevoked {
		return ErrStudentVerificationRevoked
	}
	return nil
}

// ensureEmailClaimableBy rejects emails already claimed by a different account.
func (s *StudentVerificationService) ensureEmailClaimableBy(ctx context.Context, userID int64, normalized string) error {
	existing, err := s.repo.GetByEmail(ctx, normalized)
	if err != nil {
		if errors.Is(err, ErrStudentVerificationNotFound) {
			return nil
		}
		return err
	}
	if existing.UserID != userID {
		return ErrStudentEmailClaimed
	}
	return nil
}

func (s *StudentVerificationService) isEnabled(ctx context.Context) bool {
	if s == nil || s.settingService == nil {
		return false
	}
	return s.settingService.IsStudentVerificationEnabled(ctx)
}

func (s *StudentVerificationService) invalidateUserAuthCache(ctx context.Context, userID int64) {
	if s != nil && s.authCacheInvalidator != nil && userID > 0 {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
}

// inTx runs fn inside an ent transaction when a client is available; without
// one it runs the callback directly (single-node/test deployments).
func (s *StudentVerificationService) inTx(ctx context.Context, fn func(txCtx context.Context) (*StudentVerification, error)) (*StudentVerification, error) {
	if s.entClient == nil {
		logger.LegacyPrintf("service.student_verification", "Warning: entClient is nil, student verification write is not transactional")
		return fn(ctx)
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	v, err := fn(dbent.NewTxContext(ctx, tx))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}
	return v, nil
}

func strPtrOrNil(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	v := strings.TrimSpace(s)
	return &v
}

// normalizeStudentVerificationEmail lowercases/trims the raw input for delivery
// (raw) and produces the canonical inbox identity (normalized) used for the
// uniqueness claim.
func normalizeStudentVerificationEmail(email string) (normalized string, raw string, err error) {
	raw = strings.ToLower(strings.TrimSpace(email))
	if raw == "" {
		return "", "", infraerrors.BadRequest("STUDENT_EMAIL_REQUIRED", "school email is required")
	}
	if _, _, ok := splitEmailForPolicy(raw); !ok {
		return "", "", infraerrors.BadRequest("STUDENT_EMAIL_INVALID", "invalid school email")
	}
	normalized = NormalizeEmailForAliasDedup(raw)
	if normalized == "" {
		return "", "", infraerrors.BadRequest("STUDENT_EMAIL_INVALID", "invalid school email")
	}
	return normalized, raw, nil
}
