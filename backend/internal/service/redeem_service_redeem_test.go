package service

import (
	"context"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type redeemRejectRepo struct {
	code      RedeemCode
	useCalled bool
}

func (r *redeemRejectRepo) Create(ctx context.Context, code *RedeemCode) error {
	panic("unexpected Create call")
}

func (r *redeemRejectRepo) CreateBatch(ctx context.Context, codes []RedeemCode) error {
	panic("unexpected CreateBatch call")
}

func (r *redeemRejectRepo) GetByID(ctx context.Context, id int64) (*RedeemCode, error) {
	if r.code.ID != id {
		return nil, ErrRedeemCodeNotFound
	}
	clone := r.code
	return &clone, nil
}

func (r *redeemRejectRepo) GetByCode(ctx context.Context, code string) (*RedeemCode, error) {
	if r.code.Code != code {
		return nil, ErrRedeemCodeNotFound
	}
	clone := r.code
	return &clone, nil
}

func (r *redeemRejectRepo) Update(ctx context.Context, code *RedeemCode) error {
	panic("unexpected Update call")
}

func (r *redeemRejectRepo) BatchUpdate(ctx context.Context, ids []int64, fields RedeemCodeBatchUpdateFields) (int64, error) {
	panic("unexpected BatchUpdate call")
}

func (r *redeemRejectRepo) Delete(ctx context.Context, id int64) error {
	panic("unexpected Delete call")
}

func (r *redeemRejectRepo) Use(ctx context.Context, id, userID int64) error {
	r.useCalled = true
	r.code.Status = StatusUsed
	r.code.UsedBy = &userID
	return nil
}

func (r *redeemRejectRepo) List(ctx context.Context, params pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (r *redeemRejectRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, codeType, status, search string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (r *redeemRejectRepo) ListByUser(ctx context.Context, userID int64, limit int) ([]RedeemCode, error) {
	panic("unexpected ListByUser call")
}

func (r *redeemRejectRepo) ListByUserPaginated(ctx context.Context, userID int64, params pagination.PaginationParams, codeType string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserPaginated call")
}

func (r *redeemRejectRepo) SumPositiveBalanceByUser(ctx context.Context, userID int64) (float64, error) {
	panic("unexpected SumPositiveBalanceByUser call")
}

func (r *redeemRejectRepo) HasUserRedeemedBatch(ctx context.Context, batchID string, userID int64) (bool, error) {
	if r.code.BatchID != nil && *r.code.BatchID == batchID && r.code.OnlyOncePerAccount {
		if r.code.UsedBy != nil && *r.code.UsedBy == userID {
			return true, nil
		}
	}
	return false, nil
}

func TestRedeemRejectsInvitationCodeBeforeTransaction(t *testing.T) {
	ctx := context.Background()
	redeemRepo := &redeemRejectRepo{
		code: RedeemCode{
			ID:     1,
			Code:   "INVITE-001",
			Type:   RedeemTypeInvitation,
			Status: StatusUnused,
		},
	}
	redeemService := NewRedeemService(redeemRepo, nil, nil, nil, nil, nil, nil, nil)

	got, err := redeemService.Redeem(ctx, 2, redeemRepo.code.Code)

	require.Nil(t, got)
	require.Error(t, err)
	require.True(t, infraerrors.IsBadRequest(err))
	require.Equal(t, "REDEEM_CODE_UNSUPPORTED_TYPE", infraerrors.Reason(err))
	require.Equal(t, "invitation codes can only be used during registration", infraerrors.Message(err))
	require.False(t, redeemRepo.useCalled)
	require.Equal(t, StatusUnused, redeemRepo.code.Status)
	require.Nil(t, redeemRepo.code.UsedBy)
}

func TestRedeemBatchOncePerAccountConstraint(t *testing.T) {
	ctx := context.Background()
	batchID := "batch_test_123"
	userID := int64(999)

	redeemRepo := &redeemRejectRepo{
		code: RedeemCode{
			ID:                 10,
			Code:               "BATCH-LIMITED-001",
			Type:               RedeemTypeBalance,
			Value:              10.0,
			Status:             StatusUnused,
			BatchID:            &batchID,
			OnlyOncePerAccount: true,
			UsedBy:             &userID, // 模拟该用户在此批次已经核销过一张码
		},
	}
	redeemService := NewRedeemService(redeemRepo, nil, nil, nil, nil, nil, nil, nil)

	got, err := redeemService.Redeem(ctx, userID, redeemRepo.code.Code)

	require.Nil(t, got)
	require.Error(t, err)
	require.True(t, infraerrors.IsConflict(err))
	require.Equal(t, "REDEEM_BATCH_ALREADY_USED", infraerrors.Reason(err))
	require.False(t, redeemRepo.useCalled)
}
