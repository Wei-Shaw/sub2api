//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestRedeemService_BatchUpdate_PartialFields(t *testing.T) {
	status := StatusDisabled
	notes := "maintenance window"
	expiresAt := time.Now().UTC().Add(24 * time.Hour)
	repo := &redeemRepoStub{}
	svc := &RedeemService{redeemRepo: repo}

	result, err := svc.BatchUpdate(context.Background(), &RedeemCodeBatchUpdateInput{
		IDs: []int64{1, 2, 2},
		Fields: RedeemCodeBatchUpdateFields{
			Status:    &status,
			ExpiresAt: NullableTimeUpdate{Set: true, Value: &expiresAt},
			Notes:     &notes,
		},
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), result.Updated)
	require.True(t, repo.batchUpdateCalled)
	require.Equal(t, []int64{1, 2}, repo.batchUpdateIDs)
	require.Equal(t, &status, repo.batchUpdateFields.Status)
	require.True(t, repo.batchUpdateFields.ExpiresAt.Set)
	require.WithinDuration(t, expiresAt, *repo.batchUpdateFields.ExpiresAt.Value, time.Second)
	require.Equal(t, &notes, repo.batchUpdateFields.Notes)
	require.False(t, repo.batchUpdateFields.GroupID.Set)
	require.Nil(t, repo.batchUpdateFields.Type)
	require.Nil(t, repo.batchUpdateFields.Value)
}

func TestRedeemService_BatchUpdate_RejectsInvalidID(t *testing.T) {
	repo := &redeemRepoStub{}
	svc := &RedeemService{redeemRepo: repo}
	notes := "bad id"

	result, err := svc.BatchUpdate(context.Background(), &RedeemCodeBatchUpdateInput{
		IDs:    []int64{1, 0},
		Fields: RedeemCodeBatchUpdateFields{Notes: &notes},
	})

	require.Nil(t, result)
	require.Error(t, err)
	require.True(t, infraerrors.IsBadRequest(err))
	require.False(t, repo.batchUpdateCalled)
}

func TestRedeemService_BatchUpdate_RejectsCoreFieldsForUsedCodes(t *testing.T) {
	repo := &redeemRepoStub{}
	svc := &RedeemService{redeemRepo: repo}
	newValue := 100.0

	result, err := svc.BatchUpdate(context.Background(), &RedeemCodeBatchUpdateInput{
		IDs: []int64{42},
		Fields: RedeemCodeBatchUpdateFields{
			Value: &newValue,
		},
	})

	require.Nil(t, result)
	require.Error(t, err)
	require.True(t, infraerrors.IsBadRequest(err))
	require.False(t, repo.batchUpdateCalled)
}
