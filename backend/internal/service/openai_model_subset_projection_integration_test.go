//go:build integration

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type openAIBucketStateCacheRecorder struct {
	SchedulerCache
	openAIState          *OpenAISchedulerBucketState
	setOpenAIStateCalls  int
	setSnapshotCalls     int
	sawUnsetVersion      bool
	sawUnsetBuiltAt      bool
	assignedVersion      int64
	assignedBuiltAt      time.Time
	returnedSnapshotMiss bool
}

func (c *openAIBucketStateCacheRecorder) GetSnapshot(ctx context.Context, bucket SchedulerBucket) ([]*Account, bool, error) {
	if c.returnedSnapshotMiss || c.openAIState == nil {
		return nil, false, nil
	}
	return c.openAIState.Accounts, true, nil
}

func (c *openAIBucketStateCacheRecorder) SetSnapshot(ctx context.Context, bucket SchedulerBucket, accounts []Account) error {
	c.setSnapshotCalls++
	return nil
}

func (c *openAIBucketStateCacheRecorder) GetOpenAIBucketState(ctx context.Context, bucket SchedulerBucket) (*OpenAISchedulerBucketState, bool, error) {
	if c.openAIState == nil {
		return nil, false, nil
	}
	return c.openAIState, true, nil
}

func (c *openAIBucketStateCacheRecorder) SetOpenAIBucketState(ctx context.Context, bucket SchedulerBucket, state *OpenAISchedulerBucketState) error {
	c.setOpenAIStateCalls++
	c.sawUnsetVersion = state != nil && state.ProjectionVersion == 0
	c.sawUnsetBuiltAt = state != nil && state.BuiltAt.IsZero()
	if c.assignedVersion == 0 {
		c.assignedVersion = 11
	} else {
		c.assignedVersion++
	}
	if c.assignedBuiltAt.IsZero() {
		c.assignedBuiltAt = time.Unix(1_716_000_000, 0).UTC()
	} else {
		c.assignedBuiltAt = c.assignedBuiltAt.Add(time.Second)
	}
	if state != nil {
		state.ProjectionVersion = c.assignedVersion
		state.BuiltAt = c.assignedBuiltAt
	}
	c.openAIState = state
	return nil
}

func (c *openAIBucketStateCacheRecorder) GetAccount(ctx context.Context, accountID int64) (*Account, error) {
	if c.openAIState == nil {
		return nil, nil
	}
	for _, account := range c.openAIState.Accounts {
		if account != nil && account.ID == accountID {
			return account, nil
		}
	}
	return nil, nil
}

func (c *openAIBucketStateCacheRecorder) SetAccount(ctx context.Context, account *Account) error {
	return nil
}

func (c *openAIBucketStateCacheRecorder) DeleteAccount(ctx context.Context, accountID int64) error {
	return nil
}

func (c *openAIBucketStateCacheRecorder) UpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	return nil
}

func (c *openAIBucketStateCacheRecorder) TryLockBucket(ctx context.Context, bucket SchedulerBucket, ttl time.Duration) (bool, error) {
	return true, nil
}

func (c *openAIBucketStateCacheRecorder) ListBuckets(ctx context.Context) ([]SchedulerBucket, error) {
	return nil, nil
}

func (c *openAIBucketStateCacheRecorder) GetOutboxWatermark(ctx context.Context) (int64, error) {
	return 0, nil
}

func (c *openAIBucketStateCacheRecorder) SetOutboxWatermark(ctx context.Context, id int64) error {
	return nil
}

type mixedReadOpenAIAccountRepo struct {
	stubOpenAIAccountRepo
	projectionAccounts []Account
	broadAccounts      []Account
	schedulableCalls   int
}

type mutableOpenAIProjectionRepo struct {
	stubOpenAIAccountRepo
	accounts         []Account
	updateExtraCalls int
	lastExtraByID    map[int64]map[string]any
}

func newMutableOpenAIProjectionRepo(accounts []Account) *mutableOpenAIProjectionRepo {
	r := &mutableOpenAIProjectionRepo{
		accounts: append([]Account(nil), accounts...),
		lastExtraByID: make(map[int64]map[string]any),
	}
	return r
}

func (r *mutableOpenAIProjectionRepo) schedulableAccounts(platform string) []Account {
	out := make([]Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if account.Platform != platform || !account.IsSchedulable() {
			continue
		}
		out = append(out, account)
	}
	return out
}

func (r *mutableOpenAIProjectionRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	return r.schedulableAccounts(platform), nil
}

func (r *mutableOpenAIProjectionRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return r.schedulableAccounts(platform), nil
}

func (r *mutableOpenAIProjectionRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return r.schedulableAccounts(platform), nil
}

func (r *mutableOpenAIProjectionRepo) ListByPlatform(ctx context.Context, platform string) ([]Account, error) {
	out := make([]Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if account.Platform == platform {
			out = append(out, account)
		}
	}
	return out, nil
}

func (r *mutableOpenAIProjectionRepo) ListByGroup(ctx context.Context, groupID int64) ([]Account, error) {
	return append([]Account(nil), r.accounts...), nil
}

func (r *mutableOpenAIProjectionRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			cloned := r.accounts[i]
			return &cloned, nil
		}
	}
	return nil, nil
}

func (r *mutableOpenAIProjectionRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	r.updateExtraCalls++
	clonedUpdates := make(map[string]any, len(updates))
	for key, value := range updates {
		clonedUpdates[key] = value
	}
	r.lastExtraByID[id] = clonedUpdates
	for i := range r.accounts {
		if r.accounts[i].ID != id {
			continue
		}
		if r.accounts[i].Extra == nil {
			r.accounts[i].Extra = map[string]any{}
		}
		for key, value := range updates {
			r.accounts[i].Extra[key] = value
		}
		return nil
	}
	return nil
}

func (r *mixedReadOpenAIAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	r.schedulableCalls++
	return append([]Account(nil), r.projectionAccounts...), nil
}

func (r *mixedReadOpenAIAccountRepo) ListByGroup(ctx context.Context, groupID int64) ([]Account, error) {
	return append([]Account(nil), r.broadAccounts...), nil
}

func TestOpenAIModelSubsetProjectionIntegration_RebuildBucketPublishesSingleBundle(t *testing.T) {
	cache := &openAIBucketStateCacheRecorder{}
	svc := NewSchedulerSnapshotService(
		cache,
		nil,
		stubOpenAIAccountRepo{accounts: []Account{{
			ID:          1,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    1,
			Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4"}},
		}}},
		nil,
		nil,
	)
	bucket := SchedulerBucket{GroupID: 2, Platform: PlatformOpenAI, Mode: SchedulerModeSingle}

	require.NoError(t, svc.rebuildBucket(context.Background(), bucket, "unit_test"))
	require.Equal(t, 1, cache.setOpenAIStateCalls)
	require.Zero(t, cache.setSnapshotCalls)
	require.True(t, cache.sawUnsetVersion)
	require.True(t, cache.sawUnsetBuiltAt)
	require.NotNil(t, cache.openAIState)
	require.NotNil(t, cache.openAIState.Projection)
	require.Equal(t, cache.assignedVersion, cache.openAIState.ProjectionVersion)
	require.True(t, cache.openAIState.BuiltAt.Equal(cache.assignedBuiltAt))
}

func TestOpenAIModelSubsetProjectionIntegration_PublishBucketSnapshotUsesSamePrimarySnapshotForAccountsAndProjection(t *testing.T) {
	bucket := SchedulerBucket{GroupID: 2, Platform: PlatformOpenAI, Mode: SchedulerModeSingle}
	primaryAccounts := []Account{newOpenAIProjectionExhaustedAccount(1, 1, []string{"gpt-5.4"})}
	repo := &mixedReadOpenAIAccountRepo{
		projectionAccounts: []Account{newOpenAIProjectionExhaustedAccount(2, 1, []string{"gpt-5.4"})},
		broadAccounts:      append([]Account(nil), primaryAccounts...),
	}
	cache := &openAIBucketStateCacheRecorder{}
	svc := NewSchedulerSnapshotService(cache, nil, repo, nil, nil)

	require.NoError(t, svc.publishBucketSnapshot(context.Background(), bucket, primaryAccounts))
	require.Zero(t, repo.schedulableCalls)
	require.NotNil(t, cache.openAIState)
	require.Len(t, cache.openAIState.Accounts, 1)
	require.Equal(t, int64(1), cache.openAIState.Accounts[0].ID)

	view, ok := cache.openAIState.Projection.ViewForModel("gpt-5.4")
	require.True(t, ok)
	require.Equal(t, []int64{1}, view.ExhaustedBaseIDs)
	require.Empty(t, view.ReserveOverflowIDs)
}

func TestOpenAIModelSubsetProjectionIntegration_ListSchedulableAccountsPublishesBundleOnFallback(t *testing.T) {
	cache := &openAIBucketStateCacheRecorder{returnedSnapshotMiss: true}
	svc := NewSchedulerSnapshotService(
		cache,
		nil,
		stubOpenAIAccountRepo{accounts: []Account{{
			ID:          2,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    1,
			Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4"}},
		}}},
		nil,
		nil,
	)
	groupID := int64(2)

	accounts, _, err := svc.ListSchedulableAccounts(context.Background(), &groupID, PlatformOpenAI, false)
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, int64(2), accounts[0].ID)
	require.Equal(t, 1, cache.setOpenAIStateCalls)
	require.Zero(t, cache.setSnapshotCalls)
}

func TestOpenAIModelSubsetProjectionIntegration_GetOpenAIBucketStateFailsClosedOnIncompleteBundle(t *testing.T) {
	bucket := SchedulerBucket{GroupID: 2, Platform: PlatformOpenAI, Mode: SchedulerModeSingle}
	cache := &openAIBucketStateCacheRecorder{
		openAIState: &OpenAISchedulerBucketState{
			Accounts: []*Account{{ID: 1, Platform: PlatformOpenAI}},
		},
	}
	svc := NewSchedulerSnapshotService(cache, nil, nil, nil, nil)

	state, ok, err := svc.GetOpenAIBucketState(context.Background(), bucket)
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, state)
}

func TestUnknownModel_IsConservativelyExcludedUntilCapabilitySnapshotRefresh(t *testing.T) {
	ctx := context.Background()
	account := newOpenAIProjectionActiveAccount(301, 1, 20, nil)
	account.Credentials["model_mapping"] = map[string]any{}
	account.Extra[openAICapabilityWildcardRulesExtraKey] = []string{"gpt-5.*"}
	repo := newMutableOpenAIProjectionRepo([]Account{account})
	cache := &openAIBucketStateCacheRecorder{}
	snapshot := NewSchedulerSnapshotService(cache, nil, repo, nil, nil)
	gateway := &OpenAIGatewayService{schedulerSnapshot: snapshot, accountRepo: repo}

	_, _, err := snapshot.ListSchedulableAccounts(ctx, nil, PlatformOpenAI, false)
	require.NoError(t, err)
	baseline := cache.openAIState.ProjectionVersion
	_, ok := cache.openAIState.Projection.ViewForModel("gpt-5.unknown")
	require.False(t, ok)
	require.Zero(t, repo.updateExtraCalls)

	view, err := gateway.getOpenAIProjectionView(ctx, nil, "gpt-5.unknown")
	require.NoError(t, err)
	require.NotNil(t, view)
	require.Equal(t, "gpt-5.unknown", view.canonicalModel)
	require.Equal(t, 1, repo.updateExtraCalls)
	require.Equal(t, []string{"gpt-5.unknown"}, repo.lastExtraByID[account.ID][openAICapabilityCatalogModelsExtraKey])
	require.NotEmpty(t, repo.lastExtraByID[account.ID][openAICapabilityLastRefreshAtExtraKey])
	require.NotEmpty(t, repo.lastExtraByID[account.ID][openAICapabilityLastSuccessfulRefreshAtExtraKey])
	require.Greater(t, cache.openAIState.ProjectionVersion, baseline)
	require.Equal(t, []string{"gpt-5.unknown"}, cache.openAIState.Accounts[0].Extra[openAICapabilityCatalogModelsExtraKey])
	require.Contains(t, view.view.ReserveOverflowIDs, account.ID)
}

func TestProjectionVersion_ChangesAtomicallyWithBucketState(t *testing.T) {
	ctx := context.Background()
	account := newOpenAIProjectionActiveAccount(302, 1, 20, []string{"gpt-5.4"})
	account.Credentials["model_mapping"] = map[string]any{}
	account.Extra[openAICapabilityExplicitModelsExtraKey] = nil
	account.Extra[openAICapabilityWildcardRulesExtraKey] = []string{"gpt-5.*"}
	repo := newMutableOpenAIProjectionRepo([]Account{account})
	cache := &openAIBucketStateCacheRecorder{}
	snapshot := NewSchedulerSnapshotService(cache, nil, repo, nil, nil)
	gateway := &OpenAIGatewayService{schedulerSnapshot: snapshot, accountRepo: repo}

	_, _, err := snapshot.ListSchedulableAccounts(ctx, nil, PlatformOpenAI, false)
	require.NoError(t, err)
	before := cache.openAIState
	require.NotNil(t, before)
	beforeVersion := before.ProjectionVersion
	_, ok := before.Projection.ViewForModel("gpt-5.unknown")
	require.False(t, ok)

	view, err := gateway.getOpenAIProjectionView(ctx, nil, "gpt-5.unknown")
	require.NoError(t, err)
	require.NotNil(t, view)

	after := cache.openAIState
	require.NotNil(t, after)
	require.Greater(t, after.ProjectionVersion, beforeVersion)
	require.Equal(t, 1, repo.updateExtraCalls)
	require.Equal(t, []string{"gpt-5.unknown"}, repo.lastExtraByID[account.ID][openAICapabilityCatalogModelsExtraKey])
	require.NotEmpty(t, repo.lastExtraByID[account.ID][openAICapabilityLastRefreshAtExtraKey])
	require.NotEmpty(t, repo.lastExtraByID[account.ID][openAICapabilityLastSuccessfulRefreshAtExtraKey])
	require.Len(t, after.Accounts, 1)
	require.Equal(t, []string{"gpt-5.unknown"}, after.Accounts[0].Extra[openAICapabilityCatalogModelsExtraKey])
	unknownView, ok := after.Projection.ViewForModel("gpt-5.unknown")
	require.True(t, ok)
	require.Contains(t, unknownView.ReserveOverflowIDs, account.ID)
	require.Equal(t, after.ProjectionVersion, view.state.ProjectionVersion)
}
