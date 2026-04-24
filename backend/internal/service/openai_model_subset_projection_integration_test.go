//go:build unit

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
	}
	if c.assignedBuiltAt.IsZero() {
		c.assignedBuiltAt = time.Unix(1_716_000_000, 0).UTC()
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
