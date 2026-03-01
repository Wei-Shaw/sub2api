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
	bulkUpdateErr    error
	bulkUpdateIDs    []int64
	bindGroupErrByID map[int64]error
	bindGroupsCalls  []int64
	getByIDsAccounts []*Account
	getByIDsErr      error
	getByIDsCalled   bool
	getByIDsIDs      []int64
	getByIDAccounts  map[int64]*Account
	getByIDErrByID   map[int64]error
	getByIDCalled    []int64
	listByGroupData  map[int64][]Account
	listByGroupErr   map[int64]error
REDACTED

func (s *accountRepoStubForBulkUpdate) BulkUpdate(_ context.Context, ids []int64, _ AccountBulkUpdate) (int64, error) {
	s.bulkUpdateIDs = append([]int64{REDACTED, ids...)
	if s.bulkUpdateErr != nil {
		return 0, s.bulkUpdateErr
REDACTED
	return int64(len(ids)), nil
REDACTED

func (s *accountRepoStubForBulkUpdate) BindGroups(_ context.Context, accountID int64, _ []int64) error {
	s.bindGroupsCalls = append(s.bindGroupsCalls, accountID)
	if err, ok := s.bindGroupErrByID[accountID]; ok {
		return err
REDACTED
	return nil
REDACTED

func (s *accountRepoStubForBulkUpdate) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	s.getByIDsCalled = true
	s.getByIDsIDs = append([]int64{REDACTED, ids...)
	if s.getByIDsErr != nil {
		return nil, s.getByIDsErr
REDACTED
	return s.getByIDsAccounts, nil
REDACTED

func (s *accountRepoStubForBulkUpdate) GetByID(_ context.Context, id int64) (*Account, error) {
	s.getByIDCalled = append(s.getByIDCalled, id)
	if err, ok := s.getByIDErrByID[id]; ok {
		return nil, err
REDACTED
	if account, ok := s.getByIDAccounts[id]; ok {
		return account, nil
REDACTED
	return nil, errors.New("account not found")
REDACTED

func (s *accountRepoStubForBulkUpdate) ListByGroup(_ context.Context, groupID int64) ([]Account, error) {
	if err, ok := s.listByGroupErr[groupID]; ok {
		return nil, err
REDACTED
	if rows, ok := s.listByGroupData[groupID]; ok {
		return rows, nil
REDACTED
	return nil, nil
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
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo:   &groupRepoStubForAdmin{getByID: &Group{ID: 10, Name: "g10"REDACTEDREDACTED,
REDACTED

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

func TestAdminService_BulkUpdateAccounts_NilGroupRepoReturnsError(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{REDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	groupIDs := []int64{10REDACTED
	input := &BulkUpdateAccountsInput{
		AccountIDs: []int64{1REDACTED,
		GroupIDs:   &groupIDs,
REDACTED

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.Nil(t, result)
REDACTED
	require.Contains(t, err.Error(), "group repository not configured")
REDACTED

// TestAdminService_BulkUpdateAccounts_MixedChannelPreCheckBlocksOnExistingConflict verifies
// that the global pre-check detects a conflict with existing group members and returns an
// error before any DB write is performed.
func TestAdminService_BulkUpdateAccounts_MixedChannelPreCheckBlocksOnExistingConflict(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		getByIDsAccounts: []*Account{
			{ID: 1, Platform: PlatformAntigravityREDACTED,
	REDACTED,
		// Group 10 already contains an Anthropic account.
		listByGroupData: map[int64][]Account{
			10: {{ID: 99, Platform: PlatformAnthropicREDACTEDREDACTED,
	REDACTED,
REDACTED
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo:   &groupRepoStubForAdmin{getByID: &Group{ID: 10, Name: "target-group"REDACTEDREDACTED,
REDACTED

	groupIDs := []int64{10REDACTED
	input := &BulkUpdateAccountsInput{
		AccountIDs: []int64{1REDACTED,
		GroupIDs:   &groupIDs,
REDACTED

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.Nil(t, result)
REDACTED
	require.Contains(t, err.Error(), "mixed channel")
	// No BindGroups should have been called since the check runs before any write.
	require.Empty(t, repo.bindGroupsCalls)
REDACTED
