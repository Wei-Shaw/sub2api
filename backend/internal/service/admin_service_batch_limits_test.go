//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type batchLimitsUserRepoStub struct {
	*userRepoStub
	calls       int
	userIDs     []int64
	concurrency *int
	rpmLimit    *int
	affected    int
	err         error
REDACTED

func (s *batchLimitsUserRepoStub) BatchUpdateLimits(_ context.Context, userIDs []int64, concurrency, rpmLimit *int) (int, error) {
	s.calls++
	s.userIDs = append([]int64(nil), userIDs...)
	s.concurrency = cloneBatchLimitValue(concurrency)
	s.rpmLimit = cloneBatchLimitValue(rpmLimit)
	return s.affected, s.err
REDACTED

func cloneBatchLimitValue(value *int) *int {
	if value == nil {
		return nil
REDACTED
	cloned := *value
	return &cloned
REDACTED

func TestAdminServiceBatchUpdateLimitsPassesOnlyProvidedFields(t *testing.T) {
	concurrency := 0
	repo := &batchLimitsUserRepoStub{
		userRepoStub: &userRepoStub{REDACTED,
		affected:     2,
REDACTED
	invalidator := &authCacheInvalidatorStub{REDACTED
	service := &adminServiceImpl{userRepo: repo, authCacheInvalidator: invalidatorREDACTED

	affected, err := service.BatchUpdateLimits(
		context.Background(),
		[]int64{3, 0, 3, 7, -1REDACTED,
		&concurrency,
		nil,
	)

REDACTED
	require.Equal(t, 2, affected)
	require.Equal(t, []int64{3, 7REDACTED, repo.userIDs)
	require.Equal(t, pointerToInt(0), repo.concurrency)
	require.Nil(t, repo.rpmLimit)
	require.Equal(t, []int64{3, 7REDACTED, invalidator.userIDs)
REDACTED

func TestAdminServiceBatchUpdateLimitsDoesNotInvalidateCacheOnRepositoryError(t *testing.T) {
	rpmLimit := 60
	repo := &batchLimitsUserRepoStub{
		userRepoStub: &userRepoStub{REDACTED,
		err:          errors.New("database unavailable"),
REDACTED
	invalidator := &authCacheInvalidatorStub{REDACTED
	service := &adminServiceImpl{userRepo: repo, authCacheInvalidator: invalidatorREDACTED

	affected, err := service.BatchUpdateLimits(context.Background(), []int64{1, 2REDACTED, nil, &rpmLimit)

	require.EqualError(t, err, "database unavailable")
	require.Zero(t, affected)
	require.Empty(t, invalidator.userIDs)
REDACTED

func TestAdminServiceBatchUpdateLimitsRequiresAField(t *testing.T) {
	repo := &batchLimitsUserRepoStub{userRepoStub: &userRepoStub{REDACTEDREDACTED
	service := &adminServiceImpl{userRepo: repo, authCacheInvalidator: &authCacheInvalidatorStub{REDACTEDREDACTED

	affected, err := service.BatchUpdateLimits(context.Background(), []int64{1REDACTED, nil, nil)

REDACTED
	require.Zero(t, affected)
	require.Zero(t, repo.calls)
REDACTED

func pointerToInt(value int) *int {
	return &value
REDACTED
