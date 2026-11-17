//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type emailBlacklistUserRepoStub struct {
	*userRepoStub
	probeKeys   []string
	candidates  []User
	listErr     error
	disableIDs  []int64
	disabled    []int64
	disableErr  error
	listCalls   int
	disableCall int
}

func (s *emailBlacklistUserRepoStub) ListActiveUsersByEmailAliasProbeKeys(_ context.Context, keys []string) ([]User, error) {
	s.listCalls++
	s.probeKeys = append([]string(nil), keys...)
	return s.candidates, s.listErr
}

func (s *emailBlacklistUserRepoStub) BatchDisableUsers(_ context.Context, userIDs []int64) ([]int64, error) {
	s.disableCall++
	s.disableIDs = append([]int64(nil), userIDs...)
	return s.disabled, s.disableErr
}

func TestAdminService_FindActiveUsersByBlacklistedEmails_FiltersProbeOvermatch(t *testing.T) {
	repo := &emailBlacklistUserRepoStub{
		userRepoStub: &userRepoStub{},
		candidates: []User{
			{ID: 1, Email: "a.b@gmail.com"},
			{ID: 2, Email: "ab+x@googlemail.com"},
			{ID: 3, Email: "first.last@company.example"},
			{ID: 4, Email: "firstlast@company.example"},
		},
	}
	svc := &adminServiceImpl{userRepo: repo}

	users, err := svc.FindActiveUsersByBlacklistedEmails(context.Background(), []string{
		"AB@googlemail.com", "firstlast@company.example", "not-an-email", "ab@gmail.com",
	})
	require.NoError(t, err)
	ids := make([]int64, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.ID)
	}
	require.Equal(t, []int64{1, 2, 4}, ids)
	require.ElementsMatch(t, []string{"ab@gmailcom", "ab@googlemailcom", "firstlast@companyexample"}, repo.probeKeys)
}

func TestAdminService_FindActiveUsersByBlacklistedEmails_NoValidEmailSkipsQuery(t *testing.T) {
	repo := &emailBlacklistUserRepoStub{userRepoStub: &userRepoStub{}}
	svc := &adminServiceImpl{userRepo: repo}

	users, err := svc.FindActiveUsersByBlacklistedEmails(context.Background(), []string{"", "bad"})
	require.NoError(t, err)
	require.Empty(t, users)
	require.Zero(t, repo.listCalls)
}

func TestAdminService_FindActiveUsersByBlacklistedEmails_RequiresRepositoryCapability(t *testing.T) {
	svc := &adminServiceImpl{userRepo: &userRepoStub{}}
	_, err := svc.FindActiveUsersByBlacklistedEmails(context.Background(), []string{"a@gmail.com"})
	require.ErrorIs(t, err, ErrServiceUnavailable)
}

func TestAdminService_BatchDisableUsers_InvalidatesOnlyDisabledUsers(t *testing.T) {
	repo := &emailBlacklistUserRepoStub{userRepoStub: &userRepoStub{}, disabled: []int64{7}}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{userRepo: repo, authCacheInvalidator: invalidator}

	affected, err := svc.BatchDisableUsers(context.Background(), []int64{7, 0, 7, 9, -3})
	require.NoError(t, err)
	require.Equal(t, 1, affected)
	require.Equal(t, []int64{7, 9}, repo.disableIDs)
	require.Equal(t, []int64{7}, invalidator.userIDs)
}

func TestAdminService_BatchDisableUsers_EmptyAndErrors(t *testing.T) {
	repo := &emailBlacklistUserRepoStub{userRepoStub: &userRepoStub{}}
	svc := &adminServiceImpl{userRepo: repo}

	affected, err := svc.BatchDisableUsers(context.Background(), []int64{0, -1})
	require.NoError(t, err)
	require.Zero(t, affected)
	require.Zero(t, repo.disableCall)

	repo.disableErr = errors.New("db down")
	_, err = svc.BatchDisableUsers(context.Background(), []int64{5})
	require.Error(t, err)
}

type ctxRecordingAuthCacheInvalidator struct {
	authCacheInvalidatorStub
	ctxErrs []error
}

func (s *ctxRecordingAuthCacheInvalidator) InvalidateAuthCacheByUserID(ctx context.Context, userID int64) {
	s.ctxErrs = append(s.ctxErrs, ctx.Err())
	s.authCacheInvalidatorStub.InvalidateAuthCacheByUserID(ctx, userID)
}

func TestAdminService_BatchDisableUsers_InvalidatesCacheAfterRequestCanceled(t *testing.T) {
	repo := &emailBlacklistUserRepoStub{userRepoStub: &userRepoStub{}, disabled: []int64{4, 5}}
	invalidator := &ctxRecordingAuthCacheInvalidator{}
	svc := &adminServiceImpl{userRepo: repo, authCacheInvalidator: invalidator}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	affected, err := svc.BatchDisableUsers(ctx, []int64{4, 5})
	require.NoError(t, err)
	require.Equal(t, 2, affected)
	require.Equal(t, []int64{4, 5}, invalidator.userIDs)
	require.Equal(t, []error{nil, nil}, invalidator.ctxErrs)
}
