//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// ─── Minimal stubs ────────────────────────────────────────────────────────────

// availUserRepoStub implements the subset of UserRepository used by
// GetAvailableGroupsWithSource / canUserBindGroup.
type availUserRepoStub struct {
	UserRepository
	getByID              func(ctx context.Context, id int64) (*User, error)
	canBind              func(ctx context.Context, userID, groupID int64, isExclusive bool, opts EffectiveAllowedGroupsOptions) (bool, error)
	getEffectiveSources  func(ctx context.Context, userIDs []int64, opts EffectiveAllowedGroupsOptions) (map[int64][]EffectiveAllowedGroupSource, error)
}

func (s *availUserRepoStub) GetByID(ctx context.Context, id int64) (*User, error) {
	if s.getByID != nil {
		return s.getByID(ctx, id)
	}
	return &User{ID: id, Status: StatusActive}, nil
}

func (s *availUserRepoStub) CanBindStandardGroupEffective(ctx context.Context, userID, groupID int64, isExclusive bool, opts EffectiveAllowedGroupsOptions) (bool, error) {
	if s.canBind != nil {
		return s.canBind(ctx, userID, groupID, isExclusive, opts)
	}
	return false, nil
}

func (s *availUserRepoStub) GetEffectiveAllowedGroupSources(ctx context.Context, userIDs []int64, opts EffectiveAllowedGroupsOptions) (map[int64][]EffectiveAllowedGroupSource, error) {
	if s.getEffectiveSources != nil {
		return s.getEffectiveSources(ctx, userIDs, opts)
	}
	return map[int64][]EffectiveAllowedGroupSource{}, nil
}

// availGroupRepoStub implements GroupRepository with only ListActive.
type availGroupRepoStub struct {
	GroupRepository
	groups []Group
	err    error
}

func (s *availGroupRepoStub) ListActive(ctx context.Context) ([]Group, error) {
	return s.groups, s.err
}

// availSubRepoStub implements UserSubscriptionRepository with only ListActiveByUserID
// and GetActiveByUserIDAndGroupID.
type availSubRepoStub struct {
	UserSubscriptionRepository
	subs    []UserSubscription
	listErr error
	getErr  error
}

func (s *availSubRepoStub) ListActiveByUserID(ctx context.Context, userID int64) ([]UserSubscription, error) {
	return s.subs, s.listErr
}

func (s *availSubRepoStub) GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*UserSubscription, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	for _, sub := range s.subs {
		if sub.UserID == userID && sub.GroupID == groupID {
			return &sub, nil
		}
	}
	return nil, errors.New("not found")
}

// ─── canUserBindGroup tests ───────────────────────────────────────────────────

// TestCanUserBindGroup_PropagatesPoolQueryFailure verifies that when
// CanBindStandardGroupEffective returns an error, canUserBindGroup returns the
// wrapped error (not a silent false).
func TestCanUserBindGroup_PropagatesPoolQueryFailure(t *testing.T) {
	poolErr := errors.New("pool query failure")

	userRepo := &availUserRepoStub{
		canBind: func(_ context.Context, _, _ int64, _ bool, _ EffectiveAllowedGroupsOptions) (bool, error) {
			return false, poolErr
		},
	}

	cfg := &config.Config{}
	svc := NewAPIKeyService(nil, userRepo, nil, nil, nil, nil, cfg)

	user := &User{ID: 1, Status: StatusActive, AllowedGroups: []int64{}}
	group := &Group{
		ID:               10,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeStandard,
		IsExclusive:      true,
	}

	ok, err := svc.canUserBindGroup(context.Background(), user, group)
	require.Error(t, err, "error must propagate from pool query failure")
	require.False(t, ok)
	require.ErrorContains(t, err, poolErr.Error())
}

// TestCanUserBindGroup_SubscriptionGroupOnlyChecksActiveSub verifies that for a
// subscription-type group, only an active subscription matters — Pool queries
// are not executed.
func TestCanUserBindGroup_SubscriptionGroupOnlyChecksActiveSub(t *testing.T) {
	// Pool query should never be called; panic if it is.
	userRepo := &availUserRepoStub{
		canBind: func(_ context.Context, _, _ int64, _ bool, _ EffectiveAllowedGroupsOptions) (bool, error) {
			panic("CanBindStandardGroupEffective must not be called for subscription groups")
		},
	}

	// User has an active subscription for group 20.
	subRepo := &availSubRepoStub{
		subs: []UserSubscription{{UserID: 1, GroupID: 20}},
	}

	cfg := &config.Config{}
	svc := NewAPIKeyService(nil, userRepo, nil, subRepo, nil, nil, cfg)

	user := &User{ID: 1, Status: StatusActive}
	group := &Group{
		ID:               20,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeSubscription,
		IsExclusive:      true,
	}

	ok, err := svc.canUserBindGroup(context.Background(), user, group)
	require.NoError(t, err)
	require.True(t, ok, "user with active subscription must be allowed to bind")
}

// TestCanUserBindGroup_SubscriptionGroupNoSub verifies that a user without an
// active subscription cannot bind a subscription-type group.
func TestCanUserBindGroup_SubscriptionGroupNoSub(t *testing.T) {
	subRepo := &availSubRepoStub{
		getErr: errors.New("not found"),
	}
	cfg := &config.Config{}
	svc := NewAPIKeyService(nil, nil, nil, subRepo, nil, nil, cfg)

	user := &User{ID: 1}
	group := &Group{
		ID:               20,
		SubscriptionType: SubscriptionTypeSubscription,
		IsExclusive:      true,
	}
	ok, err := svc.canUserBindGroup(context.Background(), user, group)
	require.NoError(t, err) // no error, just not allowed
	require.False(t, ok)
}

// ─── GetAvailableGroupsWithSource tests ──────────────────────────────────────

// TestGetAvailableGroupsWithSource_BulkQueryFailureReturnsError verifies that
// when GetEffectiveAllowedGroupSources fails the whole call returns an error
// instead of silently omitting pool-derived groups.
func TestGetAvailableGroupsWithSource_BulkQueryFailureReturnsError(t *testing.T) {
	bulkErr := errors.New("bulk effective sources failure")

	userRepo := &availUserRepoStub{
		getEffectiveSources: func(_ context.Context, _ []int64, _ EffectiveAllowedGroupsOptions) (map[int64][]EffectiveAllowedGroupSource, error) {
			return nil, bulkErr
		},
	}
	groupRepo := &availGroupRepoStub{
		groups: []Group{
			{ID: 1, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, IsExclusive: false},
		},
	}
	subRepo := &availSubRepoStub{}

	cfg := &config.Config{}
	svc := NewAPIKeyService(nil, userRepo, groupRepo, subRepo, nil, nil, cfg)

	_, err := svc.GetAvailableGroupsWithSource(context.Background(), 1)
	require.Error(t, err, "bulk query failure must surface as error")
	require.ErrorContains(t, err, bulkErr.Error())
}

// TestGetAvailableGroupsWithSource_PoolAlwaysIncluded verifies that IncludePool
// is always true (user_pool is now a default-enabled core feature).
func TestGetAvailableGroupsWithSource_PoolAlwaysIncluded(t *testing.T) {
	var capturedOpts EffectiveAllowedGroupsOptions

	userRepo := &availUserRepoStub{
		getEffectiveSources: func(_ context.Context, _ []int64, opts EffectiveAllowedGroupsOptions) (map[int64][]EffectiveAllowedGroupSource, error) {
			capturedOpts = opts
			return map[int64][]EffectiveAllowedGroupSource{}, nil
		},
	}
	groupRepo := &availGroupRepoStub{groups: []Group{}}
	subRepo := &availSubRepoStub{}

	cfg := &config.Config{}
	svc := NewAPIKeyService(nil, userRepo, groupRepo, subRepo, nil, nil, cfg)

	_, err := svc.GetAvailableGroupsWithSource(context.Background(), 1)
	require.NoError(t, err)
	require.True(t, capturedOpts.IncludePool,
		"IncludePool must always be true — user_pool is now a default-enabled core feature")
}
