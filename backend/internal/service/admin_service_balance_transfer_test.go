//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type balanceTransferUserRepoStub struct {
	*userRepoStub
	inputs []BalanceTransferInput
	result *BalanceTransfer
	err    error
}

func (s *balanceTransferUserRepoStub) TransferBalance(ctx context.Context, input BalanceTransferInput) (*BalanceTransfer, error) {
	s.inputs = append(s.inputs, input)
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

func TestAdminServiceTransferUserBalance_ValidatesInput(t *testing.T) {
	tests := []struct {
		name  string
		input BalanceTransferInput
		code  string
	}{
		{
			name: "external id required",
			input: BalanceTransferInput{
				FromUserID: 1,
				ToUserID:   2,
				Amount:     1,
				Reason:     "forum_read",
			},
			code: "BALANCE_TRANSFER_EXTERNAL_ID_REQUIRED",
		},
		{
			name: "distinct users required",
			input: BalanceTransferInput{
				ExternalID: "forum-read-1",
				FromUserID: 1,
				ToUserID:   1,
				Amount:     1,
				Reason:     "forum_read",
			},
			code: "BALANCE_TRANSFER_SAME_USER",
		},
		{
			name: "positive amount required",
			input: BalanceTransferInput{
				ExternalID: "forum-read-1",
				FromUserID: 1,
				ToUserID:   2,
				Reason:     "forum_read",
			},
			code: "BALANCE_TRANSFER_INVALID_AMOUNT",
		},
		{
			name: "reason required",
			input: BalanceTransferInput{
				ExternalID: "forum-read-1",
				FromUserID: 1,
				ToUserID:   2,
				Amount:     1,
			},
			code: "BALANCE_TRANSFER_REASON_REQUIRED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &balanceTransferUserRepoStub{userRepoStub: &userRepoStub{}}
			svc := &adminServiceImpl{userRepo: repo}
			_, err := svc.TransferUserBalance(context.Background(), tt.input)
			require.Error(t, err)
			require.Equal(t, tt.code, errors.Reason(err))
			require.Empty(t, repo.inputs)
		})
	}
}

func TestAdminServiceTransferUserBalance_TransfersAndInvalidates(t *testing.T) {
	repo := &balanceTransferUserRepoStub{
		userRepoStub: &userRepoStub{},
		result: &BalanceTransfer{
			ID:          1,
			ExternalID:  "forum-read-1",
			FromUserID:  1,
			ToUserID:    2,
			Amount:      0.25,
			Reason:      "forum_read",
			FromBalance: 9.75,
			ToBalance:   1.25,
		},
	}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{
		userRepo:             repo,
		authCacheInvalidator: invalidator,
	}

	got, err := svc.TransferUserBalance(context.Background(), BalanceTransferInput{
		ExternalID: " forum-read-1 ",
		FromUserID: 1,
		ToUserID:   2,
		Amount:     0.25,
		Reason:     " forum_read ",
	})
	require.NoError(t, err)
	require.Equal(t, repo.result, got)
	require.Len(t, repo.inputs, 1)
	require.Equal(t, "forum-read-1", repo.inputs[0].ExternalID)
	require.Equal(t, "forum_read", repo.inputs[0].Reason)
	require.Equal(t, []int64{1, 2}, invalidator.userIDs)
}

func TestAdminServiceTransferUserBalance_ReturnsConflictError(t *testing.T) {
	repo := &balanceTransferUserRepoStub{
		userRepoStub: &userRepoStub{},
		err:          ErrBalanceTransferConflict,
	}
	svc := &adminServiceImpl{userRepo: repo}

	_, err := svc.TransferUserBalance(context.Background(), BalanceTransferInput{
		ExternalID: "forum-read-1",
		FromUserID: 1,
		ToUserID:   2,
		Amount:     0.25,
		Reason:     "forum_read",
	})

	require.ErrorIs(t, err, ErrBalanceTransferConflict)
	require.Equal(t, "BALANCE_TRANSFER_CONFLICT", errors.Reason(err))
}
