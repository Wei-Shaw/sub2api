//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type geminiCompositeRouteAccountRepo struct {
	*mockAccountRepoForGemini
	queriedGroups []int64
}

func (r *geminiCompositeRouteAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, groupID int64, platform string) ([]Account, error) {
	r.queriedGroups = append(r.queriedGroups, groupID)
	var accounts []Account
	for _, account := range r.accounts {
		if account.Platform != platform || !account.IsSchedulable() {
			continue
		}
		for _, membership := range account.AccountGroups {
			if membership.GroupID == groupID {
				accounts = append(accounts, account)
				break
			}
		}
	}
	return accounts, nil
}

func geminiCompositeRouteContext(sourceID int64, groupID, accountID *int64) context.Context {
	return WithCompositeRouteDecision(context.Background(), CompositeRouteDecision{
		Matched: true, Source: CompositeRouteSourceExplicit, GroupID: sourceID,
		PublicModel: "claude-sonnet-4-6", TargetPlatform: PlatformGemini,
		UpstreamModel: "gemini-2.5-flash", TargetGroupID: groupID, TargetAccountID: accountID,
	})
}

func newGeminiCompositeRouteService(accounts []Account, groups map[int64]*Group) (*GeminiMessagesCompatService, *geminiCompositeRouteAccountRepo, *mockGatewayCacheForGemini) {
	repo := &geminiCompositeRouteAccountRepo{mockAccountRepoForGemini: &mockAccountRepoForGemini{
		accounts: accounts, accountsByID: make(map[int64]*Account),
	}}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}
	cache := &mockGatewayCacheForGemini{}
	return &GeminiMessagesCompatService{accountRepo: repo, groupRepo: &mockGroupRepoForGemini{groups: groups}, cache: cache}, repo, cache
}

func TestGeminiCompositeRoute_TargetGroupIgnoresSourcePoolAndRejectsForeignSticky(t *testing.T) {
	sourceID, targetID := int64(10), int64(20)
	ctx := geminiCompositeRouteContext(sourceID, &targetID, nil)
	svc, repo, cache := newGeminiCompositeRouteService([]Account{
		{ID: 1, Platform: PlatformGemini, Status: StatusActive, Schedulable: true, Priority: 1, AccountGroups: []AccountGroup{{GroupID: sourceID}}},
		{ID: 2, Platform: PlatformGemini, Status: StatusActive, Schedulable: true, Priority: 9, AccountGroups: []AccountGroup{{GroupID: targetID}}},
	}, map[int64]*Group{targetID: {ID: targetID, Platform: PlatformGemini, Status: StatusActive}})
	key := CompositeRouteStickyKey(ctx, "gemini:shared-session")
	cache.sessionBindings = map[string]int64{key: 1}

	account, err := svc.SelectAccountForModelWithExclusions(ctx, &sourceID, "shared-session", "gemini-2.5-flash", nil)
	require.NoError(t, err)
	require.Equal(t, int64(2), account.ID)
	require.Equal(t, []int64{targetID}, repo.queriedGroups)
	require.Equal(t, int64(2), cache.sessionBindings[key])
	require.Equal(t, int64(10), sourceID)
}

func TestGeminiCompositeRoute_PinOutsideSourceGroupAndNoFailover(t *testing.T) {
	sourceID, accountID := int64(10), int64(2)
	ctx := geminiCompositeRouteContext(sourceID, nil, &accountID)
	svc, _, _ := newGeminiCompositeRouteService([]Account{
		{ID: 1, Platform: PlatformGemini, Status: StatusActive, Schedulable: true, Priority: 1, AccountGroups: []AccountGroup{{GroupID: sourceID}}},
		{ID: 2, Platform: PlatformGemini, Status: StatusActive, Schedulable: true, Priority: 9},
	}, nil)

	account, err := svc.SelectAccountForModelWithExclusions(ctx, &sourceID, "", "gemini-2.5-flash", nil)
	require.NoError(t, err)
	require.Equal(t, accountID, account.ID)
	account, err = svc.SelectAccountForModelWithExclusions(ctx, &sourceID, "", "gemini-2.5-flash", map[int64]struct{}{accountID: {}})
	require.Error(t, err)
	require.Nil(t, account)
}

func TestGeminiCompositeRoute_UnavailablePinDoesNotUseOtherAccounts(t *testing.T) {
	for _, unavailable := range []string{"disabled", "cooldown", "model", "deleted", "platform"} {
		t.Run(unavailable, func(t *testing.T) {
			sourceID, accountID := int64(10), int64(2)
			ctx := geminiCompositeRouteContext(sourceID, nil, &accountID)
			svc, repo, _ := newGeminiCompositeRouteService([]Account{
				{ID: 1, Platform: PlatformGemini, Status: StatusActive, Schedulable: true},
				{ID: 2, Platform: PlatformGemini, Status: StatusActive, Schedulable: true},
			}, nil)
			switch unavailable {
			case "disabled":
				repo.accountsByID[accountID].Schedulable = false
			case "cooldown":
				until := time.Now().Add(time.Hour)
				repo.accountsByID[accountID].RateLimitResetAt = &until
			case "model":
				repo.accountsByID[accountID].Credentials = map[string]any{"model_mapping": map[string]any{"another-model": "another-model"}}
			case "deleted":
				delete(repo.accountsByID, accountID)
			case "platform":
				repo.accountsByID[accountID].Platform = PlatformOpenAI
			}
			account, err := svc.SelectAccountForModelWithExclusions(ctx, &sourceID, "", "gemini-2.5-flash", nil)
			require.Error(t, err)
			require.Nil(t, account)
		})
	}
}

func TestGeminiCompositeRoute_EmptyOrInactiveGroupNeverFallsBack(t *testing.T) {
	for _, status := range []string{StatusActive, StatusDisabled} {
		t.Run(status, func(t *testing.T) {
			sourceID, targetID := int64(10), int64(20)
			ctx := geminiCompositeRouteContext(sourceID, &targetID, nil)
			svc, repo, _ := newGeminiCompositeRouteService([]Account{
				{ID: 1, Platform: PlatformGemini, Status: StatusActive, Schedulable: true, AccountGroups: []AccountGroup{{GroupID: sourceID}}},
			}, map[int64]*Group{targetID: {ID: targetID, Platform: PlatformGemini, Status: status}})
			repo.listByPlatformFunc = func(context.Context, []string) ([]Account, error) {
				t.Fatal("explicit destination escaped to global account lookup")
				return nil, errors.New("unexpected broader lookup")
			}
			account, err := svc.SelectAccountForModelWithExclusions(ctx, &sourceID, "", "gemini-2.5-flash", nil)
			require.Error(t, err)
			require.Nil(t, account)
		})
	}
}

func TestGeminiCompositeRoute_SourceGroupCannotChangeUnderGrant(t *testing.T) {
	sourceID, otherID, accountID := int64(10), int64(11), int64(2)
	ctx := geminiCompositeRouteContext(sourceID, nil, &accountID)
	svc, _, _ := newGeminiCompositeRouteService([]Account{
		{ID: accountID, Platform: PlatformGemini, Status: StatusActive, Schedulable: true},
	}, nil)
	account, err := svc.SelectAccountForModelWithExclusions(ctx, &otherID, "", "gemini-2.5-flash", nil)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Nil(t, account)
	account, err = svc.SelectAccountForModelsEndpoints(ctx, &otherID)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Nil(t, account)
}

func TestGeminiCompositeRoute_ModelEndpointStillRejectsDisabledPin(t *testing.T) {
	sourceID, accountID := int64(10), int64(2)
	ctx := geminiCompositeRouteContext(sourceID, nil, &accountID)
	svc, _, _ := newGeminiCompositeRouteService([]Account{
		{ID: accountID, Platform: PlatformGemini, Status: StatusActive, Schedulable: false},
	}, nil)
	account, err := svc.SelectAccountForModelsEndpoints(ctx, &sourceID)
	require.Error(t, err)
	require.Nil(t, account)
}
