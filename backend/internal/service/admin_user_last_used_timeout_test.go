//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAdminService_ListUsers_LastUsedTimeoutKeepsUsersAndParentContext(t *testing.T) {
	parent := context.Background()
	var lookupCtx context.Context
	userRepo := &userRepoStubForListUsers{
		users: []User{{ID: 101}},
		lastUsedLookup: func(ctx context.Context) error {
			lookupCtx = ctx
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			require.Positive(t, time.Until(deadline))
			require.LessOrEqual(t, time.Until(deadline), adminUserLastUsedTimeout)
			return context.DeadlineExceeded
		},
	}
	rateRepo := &userGroupRateRepoStubForListUsers{batchData: map[int64]map[int64]float64{101: {11: 1.1}}}
	svc := &adminServiceImpl{userRepo: userRepo, userGroupRateRepo: rateRepo}
	users, total, err := svc.ListUsers(parent, 1, 20, UserListFilters{}, "created_at", "desc")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, users, 1)
	require.Nil(t, users[0].LastUsedAt)
	require.Equal(t, 1.1, users[0].GroupRates[11])
	require.NoError(t, parent.Err())
	require.ErrorIs(t, lookupCtx.Err(), context.Canceled)
}

func TestAdminService_ListUsers_LastUsedHonorsEarlierParentDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	deadline, _ := parent.Deadline()
	called := false
	userRepo := &userRepoStubForListUsers{
		users: []User{{ID: 101}},
		lastUsedLookup: func(ctx context.Context) error {
			called = true
			got, ok := ctx.Deadline()
			require.True(t, ok)
			require.Equal(t, deadline, got)
			return nil
		},
	}
	svc := &adminServiceImpl{userRepo: userRepo}
	_, _, err := svc.ListUsers(parent, 1, 20, UserListFilters{}, "", "")
	require.NoError(t, err)
	require.True(t, called)
	require.NoError(t, parent.Err())
}
