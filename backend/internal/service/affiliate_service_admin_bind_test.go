package service

import (
	"context"
	"testing"
)

func TestAffiliateService_AdminBindInviter(t *testing.T) {
	ctx := context.Background()

	t.Run("binds unbound invitee", func(t *testing.T) {
		repo := newAffiliateAdminBindRepo()
		svc := NewAffiliateService(repo, nil, nil, nil)

		if err := svc.AdminBindInviter(ctx, 2, 1); err != nil {
			t.Fatalf("AdminBindInviter() error = %v", err)
		}
		if got := repo.binds[2]; got != 1 {
			t.Fatalf("bound inviter = %d, want 1", got)
		}
	})

	t.Run("rejects already bound invitee", func(t *testing.T) {
		repo := newAffiliateAdminBindRepo()
		inviterID := int64(1)
		repo.summaries[2].InviterID = &inviterID
		svc := NewAffiliateService(repo, nil, nil, nil)

		if err := svc.AdminBindInviter(ctx, 2, 3); err != ErrAffiliateAlreadyBound {
			t.Fatalf("AdminBindInviter() error = %v, want %v", err, ErrAffiliateAlreadyBound)
		}
	})

	t.Run("rejects descendant inviter", func(t *testing.T) {
		repo := newAffiliateAdminBindRepo()
		repo.descendants[[2]int64{1, 3}] = true
		svc := NewAffiliateService(repo, nil, nil, nil)

		if err := svc.AdminBindInviter(ctx, 1, 3); err != ErrAffiliateCycleDetected {
			t.Fatalf("AdminBindInviter() error = %v, want %v", err, ErrAffiliateCycleDetected)
		}
	})
}

type affiliateAdminBindRepo struct {
	summaries   map[int64]*AffiliateSummary
	descendants map[[2]int64]bool
	binds       map[int64]int64
}

func newAffiliateAdminBindRepo() *affiliateAdminBindRepo {
	return &affiliateAdminBindRepo{
		summaries: map[int64]*AffiliateSummary{
			1: {UserID: 1, AffCode: "AFF1"},
			2: {UserID: 2, AffCode: "AFF2"},
			3: {UserID: 3, AffCode: "AFF3"},
		},
		descendants: make(map[[2]int64]bool),
		binds:       make(map[int64]int64),
	}
}

func (r *affiliateAdminBindRepo) EnsureUserAffiliate(_ context.Context, userID int64) (*AffiliateSummary, error) {
	if summary, ok := r.summaries[userID]; ok {
		return summary, nil
	}
	return nil, ErrAffiliateProfileNotFound
}

func (r *affiliateAdminBindRepo) GetAffiliateByCode(_ context.Context, code string) (*AffiliateSummary, error) {
	for _, summary := range r.summaries {
		if summary != nil && summary.AffCode == code {
			return summary, nil
		}
	}
	return nil, ErrAffiliateProfileNotFound
}

func (r *affiliateAdminBindRepo) BindInviter(_ context.Context, userID, inviterID int64) (bool, error) {
	if r.summaries[userID].InviterID != nil {
		return false, nil
	}
	r.binds[userID] = inviterID
	r.summaries[userID].InviterID = &inviterID
	return true, nil
}

func (r *affiliateAdminBindRepo) IsAffiliateDescendant(_ context.Context, userID, candidateDescendantID int64) (bool, error) {
	return r.descendants[[2]int64{userID, candidateDescendantID}], nil
}

func (r *affiliateAdminBindRepo) AccrueQuota(context.Context, int64, int64, float64, int, *int64) (bool, error) {
	panic("unexpected AccrueQuota call")
}

func (r *affiliateAdminBindRepo) GetAccruedRebateFromInvitee(context.Context, int64, int64) (float64, error) {
	panic("unexpected GetAccruedRebateFromInvitee call")
}

func (r *affiliateAdminBindRepo) ThawFrozenQuota(context.Context, int64) (float64, error) {
	panic("unexpected ThawFrozenQuota call")
}

func (r *affiliateAdminBindRepo) TransferQuotaToBalance(context.Context, int64) (float64, float64, error) {
	panic("unexpected TransferQuotaToBalance call")
}

func (r *affiliateAdminBindRepo) ListInvitees(context.Context, int64, int) ([]AffiliateInvitee, error) {
	panic("unexpected ListInvitees call")
}

func (r *affiliateAdminBindRepo) UpdateUserAffCode(context.Context, int64, string) error {
	panic("unexpected UpdateUserAffCode call")
}

func (r *affiliateAdminBindRepo) ResetUserAffCode(context.Context, int64) (string, error) {
	panic("unexpected ResetUserAffCode call")
}

func (r *affiliateAdminBindRepo) SetUserRebateRate(context.Context, int64, *float64) error {
	panic("unexpected SetUserRebateRate call")
}

func (r *affiliateAdminBindRepo) BatchSetUserRebateRate(context.Context, []int64, *float64) error {
	panic("unexpected BatchSetUserRebateRate call")
}

func (r *affiliateAdminBindRepo) ListUsersWithCustomSettings(context.Context, AffiliateAdminFilter) ([]AffiliateAdminEntry, int64, error) {
	panic("unexpected ListUsersWithCustomSettings call")
}

func (r *affiliateAdminBindRepo) ListAffiliateInviteRecords(context.Context, AffiliateRecordFilter) ([]AffiliateInviteRecord, int64, error) {
	panic("unexpected ListAffiliateInviteRecords call")
}

func (r *affiliateAdminBindRepo) ListAffiliateRebateRecords(context.Context, AffiliateRecordFilter) ([]AffiliateRebateRecord, int64, error) {
	panic("unexpected ListAffiliateRebateRecords call")
}

func (r *affiliateAdminBindRepo) ListAffiliateTransferRecords(context.Context, AffiliateRecordFilter) ([]AffiliateTransferRecord, int64, error) {
	panic("unexpected ListAffiliateTransferRecords call")
}

func (r *affiliateAdminBindRepo) GetAffiliateUserOverview(context.Context, int64) (*AffiliateUserOverview, error) {
	panic("unexpected GetAffiliateUserOverview call")
}
