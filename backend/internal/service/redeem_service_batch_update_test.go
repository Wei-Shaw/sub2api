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
	repo := &redeemRepoStub{REDACTED
	svc := &RedeemService{redeemRepo: repoREDACTED

	result, err := svc.BatchUpdate(context.Background(), &RedeemCodeBatchUpdateInput{
		IDs: []int64{1, 2, 2REDACTED,
		Fields: RedeemCodeBatchUpdateFields{
			Status:    &status,
			ExpiresAt: NullableTimeUpdate{Set: true, Value: &expiresAtREDACTED,
			Notes:     &notes,
	REDACTED,
REDACTED)

REDACTED
	require.Equal(t, int64(2), result.Updated)
	require.True(t, repo.batchUpdateCalled)
	require.Equal(t, []int64{1, 2REDACTED, repo.batchUpdateIDs)
	require.Equal(t, &status, repo.batchUpdateFields.Status)
	require.True(t, repo.batchUpdateFields.ExpiresAt.Set)
	require.WithinDuration(t, expiresAt, *repo.batchUpdateFields.ExpiresAt.Value, time.Second)
	require.Equal(t, &notes, repo.batchUpdateFields.Notes)
	require.False(t, repo.batchUpdateFields.GroupID.Set)
	require.Nil(t, repo.batchUpdateFields.Type)
	require.Nil(t, repo.batchUpdateFields.Value)
REDACTED

func TestRedeemService_BatchUpdate_RejectsInvalidID(t *testing.T) {
	repo := &redeemRepoStub{REDACTED
	svc := &RedeemService{redeemRepo: repoREDACTED
	notes := "bad id"

	result, err := svc.BatchUpdate(context.Background(), &RedeemCodeBatchUpdateInput{
		IDs:    []int64{1, 0REDACTED,
		Fields: RedeemCodeBatchUpdateFields{Notes: &notesREDACTED,
REDACTED)

	require.Nil(t, result)
REDACTED
	require.True(t, infraerrors.IsBadRequest(err))
	require.False(t, repo.batchUpdateCalled)
REDACTED

func TestRedeemService_BatchUpdate_RejectsCoreFieldsForUsedCodes(t *testing.T) {
	repo := &redeemRepoStub{REDACTED
	svc := &RedeemService{redeemRepo: repoREDACTED
	newValue := 100.0

	result, err := svc.BatchUpdate(context.Background(), &RedeemCodeBatchUpdateInput{
		IDs: []int64{42REDACTED,
		Fields: RedeemCodeBatchUpdateFields{
			Value: &newValue,
	REDACTED,
REDACTED)

	require.Nil(t, result)
REDACTED
	require.True(t, infraerrors.IsBadRequest(err))
	require.False(t, repo.batchUpdateCalled)
REDACTED
