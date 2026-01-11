//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type accountRepoStubForBulkUpdate struct {
	accountRepoStub
	bulkUpdateErr     error
	bulkUpdateIDs     []int64
	bindGroupErrByID  map[int64]error
REDACTED

func (s *accountRepoStubForBulkUpdate) BulkUpdate(_ context.Context, ids []int64, _ AccountBulkUpdate) (int64, error) {
	s.bulkUpdateIDs = append([]int64{REDACTED, ids...)
	if s.bulkUpdateErr != nil {
		return 0, s.bulkUpdateErr
REDACTED
	return int64(len(ids)), nil
REDACTED

func (s *accountRepoStubForBulkUpdate) BindGroups(_ context.Context, accountID int64, _ []int64) error {
	if err, ok := s.bindGroupErrByID[accountID]; ok {
		return err
REDACTED
	return nil
REDACTED

// TestAdminService_BulkUpdateAccounts_AllSuccessIDs 验证批量更新成功时返回 success_ids/failed_ids。
func TestAdminService_BulkUpdateAccounts_AllSuccessIDs(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{REDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	schedulable := true
	input := &BulkUpdateAccountsInput{
		AccountIDs:  []int64{1, 2, 3REDACTED,
		Schedulable: &schedulable,
REDACTED

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
REDACTED
	require.Equal(t, 3, result.Success)
	require.Equal(t, 0, result.Failed)
	require.ElementsMatch(t, []int64{1, 2, 3REDACTED, result.SuccessIDs)
	require.Empty(t, result.FailedIDs)
	require.Len(t, result.Results, 3)
REDACTED

// TestAdminService_BulkUpdateAccounts_PartialFailureIDs 验证部分失败时 success_ids/failed_ids 正确。
func TestAdminService_BulkUpdateAccounts_PartialFailureIDs(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		bindGroupErrByID: map[int64]error{
			2: errors.New("bind failed"),
	REDACTED,
REDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	groupIDs := []int64{10REDACTED
	schedulable := false
	input := &BulkUpdateAccountsInput{
		AccountIDs:            []int64{1, 2, 3REDACTED,
		GroupIDs:              &groupIDs,
		Schedulable:           &schedulable,
		SkipMixedChannelCheck: true,
REDACTED

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
REDACTED
	require.Equal(t, 2, result.Success)
	require.Equal(t, 1, result.Failed)
	require.ElementsMatch(t, []int64{1, 3REDACTED, result.SuccessIDs)
	require.ElementsMatch(t, []int64{2REDACTED, result.FailedIDs)
	require.Len(t, result.Results, 3)
REDACTED
