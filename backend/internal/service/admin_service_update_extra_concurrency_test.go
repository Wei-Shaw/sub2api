//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type extraConcurrencyHistoryRepoStub struct {
	*balanceRedeemRepoStub
}

func (s *extraConcurrencyHistoryRepoStub) ListByUserPaginated(_ context.Context, userID int64, _ pagination.PaginationParams, codeType string) ([]RedeemCode, *pagination.PaginationResult, error) {
	codes := make([]RedeemCode, 0, len(s.created))
	for _, code := range s.created {
		if code == nil || code.UsedBy == nil || *code.UsedBy != userID || (codeType != "" && code.Type != codeType) {
			continue
		}
		codes = append(codes, *code)
	}
	return codes, &pagination.PaginationResult{Total: int64(len(codes))}, nil
}

func (s *extraConcurrencyHistoryRepoStub) SumPositiveBalanceByUser(context.Context, int64) (float64, error) {
	return 0, nil
}

func TestAdminService_UpdateUserExtraConcurrencyRecordsAdjustment(t *testing.T) {
	baseRepo := &userRepoStub{user: &User{
		ID:               7,
		Email:            "extra-update@test.com",
		PasswordHash:     "hash",
		Role:             RoleUser,
		Status:           StatusActive,
		Concurrency:      5,
		ExtraConcurrency: 1,
	}}
	repo := &balanceUserRepoStub{userRepoStub: baseRepo}
	redeemRepo := &extraConcurrencyHistoryRepoStub{
		balanceRedeemRepoStub: &balanceRedeemRepoStub{redeemRepoStub: &redeemRepoStub{}},
	}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{
		userRepo:             repo,
		redeemCodeRepo:       redeemRepo,
		authCacheInvalidator: invalidator,
	}
	extraConcurrency := 3

	updated, err := svc.UpdateUser(context.Background(), 7, &UpdateUserInput{
		ExtraConcurrency: &extraConcurrency,
	})

	require.NoError(t, err)
	require.Equal(t, 5, updated.Concurrency)
	require.Equal(t, 3, updated.ExtraConcurrency)
	require.Equal(t, []int64{7}, invalidator.userIDs)
	require.Len(t, redeemRepo.created, 1)
	require.Equal(t, AdjustmentTypeAdminExtraConcurrency, redeemRepo.created[0].Type)
	require.Equal(t, float64(2), redeemRepo.created[0].Value)

	history, total, _, err := svc.GetUserBalanceHistory(context.Background(), 7, 1, 10, AdjustmentTypeAdminExtraConcurrency)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, history, 1)
	require.Equal(t, AdjustmentTypeAdminExtraConcurrency, history[0].Type)
	require.Equal(t, float64(2), history[0].Value)
}

func TestAdminService_UpdateUserRejectsNegativeExtraConcurrency(t *testing.T) {
	svc := &adminServiceImpl{userRepo: &userRepoStub{}}
	extraConcurrency := -1

	updated, err := svc.UpdateUser(context.Background(), 7, &UpdateUserInput{
		ExtraConcurrency: &extraConcurrency,
	})

	require.Nil(t, updated)
	require.ErrorContains(t, err, "extra concurrency must be non-negative")
}
