//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// userGroupRateRepoStubForGroupRate implements UserGroupRateRepository for group rate tests.
type userGroupRateRepoStubForGroupRate struct {
	getByGroupIDData map[int64][]UserGroupRateEntry
	getByGroupIDErr  error

	deletedGroupIDs  []int64
	deleteByGroupErr error

	syncedGroupID int64
	syncedEntries []GroupRateMultiplierInput
	syncGroupErr  error
REDACTED

func (s *userGroupRateRepoStubForGroupRate) GetByUserID(_ context.Context, _ int64) (map[int64]float64, error) {
	panic("unexpected GetByUserID call")
REDACTED

func (s *userGroupRateRepoStubForGroupRate) GetByUserAndGroup(_ context.Context, _, _ int64) (*float64, error) {
	panic("unexpected GetByUserAndGroup call")
REDACTED

func (s *userGroupRateRepoStubForGroupRate) GetByGroupID(_ context.Context, groupID int64) ([]UserGroupRateEntry, error) {
	if s.getByGroupIDErr != nil {
		return nil, s.getByGroupIDErr
REDACTED
	return s.getByGroupIDData[groupID], nil
REDACTED

func (s *userGroupRateRepoStubForGroupRate) SyncUserGroupRates(_ context.Context, _ int64, _ map[int64]*float64) error {
	panic("unexpected SyncUserGroupRates call")
REDACTED

func (s *userGroupRateRepoStubForGroupRate) SyncGroupRateMultipliers(_ context.Context, groupID int64, entries []GroupRateMultiplierInput) error {
	s.syncedGroupID = groupID
	s.syncedEntries = entries
	return s.syncGroupErr
REDACTED

func (s *userGroupRateRepoStubForGroupRate) DeleteByGroupID(_ context.Context, groupID int64) error {
	s.deletedGroupIDs = append(s.deletedGroupIDs, groupID)
	return s.deleteByGroupErr
REDACTED

func (s *userGroupRateRepoStubForGroupRate) DeleteByUserID(_ context.Context, _ int64) error {
	panic("unexpected DeleteByUserID call")
REDACTED

func TestAdminService_GetGroupRateMultipliers(t *testing.T) {
	t.Run("returns entries for group", func(t *testing.T) {
		repo := &userGroupRateRepoStubForGroupRate{
			getByGroupIDData: map[int64][]UserGroupRateEntry{
				10: {
					{UserID: 1, UserName: "alice", UserEmail: "alice@test.com", RateMultiplier: 1.5REDACTED,
					{UserID: 2, UserName: "bob", UserEmail: "bob@test.com", RateMultiplier: 0.8REDACTED,
			REDACTED,
		REDACTED,
	REDACTED
		svc := &adminServiceImpl{userGroupRateRepo: repoREDACTED

		entries, err := svc.GetGroupRateMultipliers(context.Background(), 10)
	REDACTED
		require.Len(t, entries, 2)
		require.Equal(t, int64(1), entries[0].UserID)
		require.Equal(t, "alice", entries[0].UserName)
		require.Equal(t, 1.5, entries[0].RateMultiplier)
		require.Equal(t, int64(2), entries[1].UserID)
		require.Equal(t, 0.8, entries[1].RateMultiplier)
REDACTED)

	t.Run("returns nil when repo is nil", func(t *testing.T) {
		svc := &adminServiceImpl{userGroupRateRepo: nilREDACTED

		entries, err := svc.GetGroupRateMultipliers(context.Background(), 10)
	REDACTED
		require.Nil(t, entries)
REDACTED)

	t.Run("returns empty slice for group with no entries", func(t *testing.T) {
		repo := &userGroupRateRepoStubForGroupRate{
			getByGroupIDData: map[int64][]UserGroupRateEntry{REDACTED,
	REDACTED
		svc := &adminServiceImpl{userGroupRateRepo: repoREDACTED

		entries, err := svc.GetGroupRateMultipliers(context.Background(), 99)
	REDACTED
		require.Nil(t, entries)
REDACTED)

	t.Run("propagates repo error", func(t *testing.T) {
		repo := &userGroupRateRepoStubForGroupRate{
			getByGroupIDErr: errors.New("db error"),
	REDACTED
		svc := &adminServiceImpl{userGroupRateRepo: repoREDACTED

		_, err := svc.GetGroupRateMultipliers(context.Background(), 10)
	REDACTED
		require.Contains(t, err.Error(), "db error")
REDACTED)
REDACTED

func TestAdminService_ClearGroupRateMultipliers(t *testing.T) {
	t.Run("deletes by group ID", func(t *testing.T) {
		repo := &userGroupRateRepoStubForGroupRate{REDACTED
		svc := &adminServiceImpl{userGroupRateRepo: repoREDACTED

		err := svc.ClearGroupRateMultipliers(context.Background(), 42)
	REDACTED
		require.Equal(t, []int64{42REDACTED, repo.deletedGroupIDs)
REDACTED)

	t.Run("returns nil when repo is nil", func(t *testing.T) {
		svc := &adminServiceImpl{userGroupRateRepo: nilREDACTED

		err := svc.ClearGroupRateMultipliers(context.Background(), 42)
	REDACTED
REDACTED)

	t.Run("propagates repo error", func(t *testing.T) {
		repo := &userGroupRateRepoStubForGroupRate{
			deleteByGroupErr: errors.New("delete failed"),
	REDACTED
		svc := &adminServiceImpl{userGroupRateRepo: repoREDACTED

		err := svc.ClearGroupRateMultipliers(context.Background(), 42)
	REDACTED
		require.Contains(t, err.Error(), "delete failed")
REDACTED)
REDACTED

func TestAdminService_BatchSetGroupRateMultipliers(t *testing.T) {
	t.Run("syncs entries to repo", func(t *testing.T) {
		repo := &userGroupRateRepoStubForGroupRate{REDACTED
		svc := &adminServiceImpl{userGroupRateRepo: repoREDACTED

		entries := []GroupRateMultiplierInput{
			{UserID: 1, RateMultiplier: 1.5REDACTED,
			{UserID: 2, RateMultiplier: 0.8REDACTED,
	REDACTED
		err := svc.BatchSetGroupRateMultipliers(context.Background(), 10, entries)
	REDACTED
		require.Equal(t, int64(10), repo.syncedGroupID)
		require.Equal(t, entries, repo.syncedEntries)
REDACTED)

	t.Run("returns nil when repo is nil", func(t *testing.T) {
		svc := &adminServiceImpl{userGroupRateRepo: nilREDACTED

		err := svc.BatchSetGroupRateMultipliers(context.Background(), 10, nil)
	REDACTED
REDACTED)

	t.Run("propagates repo error", func(t *testing.T) {
		repo := &userGroupRateRepoStubForGroupRate{
			syncGroupErr: errors.New("sync failed"),
	REDACTED
		svc := &adminServiceImpl{userGroupRateRepo: repoREDACTED

		err := svc.BatchSetGroupRateMultipliers(context.Background(), 10, []GroupRateMultiplierInput{
			{UserID: 1, RateMultiplier: 1.0REDACTED,
	REDACTED)
	REDACTED
		require.Contains(t, err.Error(), "sync failed")
REDACTED)
REDACTED
