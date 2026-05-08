//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type snapshotHydrationCache struct {
	snapshot     []*Account
	accounts     map[int64]*Account
	openAIStates map[string]*OpenAISchedulerBucketState
}

func (c *snapshotHydrationCache) GetSnapshot(ctx context.Context, bucket SchedulerBucket) ([]*Account, bool, error) {
	return c.snapshot, true, nil
}

func (c *snapshotHydrationCache) SetSnapshot(ctx context.Context, bucket SchedulerBucket, accounts []Account) error {
	return nil
}

func (c *snapshotHydrationCache) GetOpenAIBucketState(ctx context.Context, bucket SchedulerBucket) (*OpenAISchedulerBucketState, bool, error) {
	if c.openAIStates == nil {
		return nil, false, nil
	}
	state, ok := c.openAIStates[bucket.String()]
	return state, ok, nil
}

func (c *snapshotHydrationCache) SetOpenAIBucketState(ctx context.Context, bucket SchedulerBucket, state *OpenAISchedulerBucketState) error {
	return nil
}

func (c *snapshotHydrationCache) GetAccount(ctx context.Context, accountID int64) (*Account, error) {
	if c.accounts == nil {
		return nil, nil
	}
	return c.accounts[accountID], nil
}

func (c *snapshotHydrationCache) SetAccount(ctx context.Context, account *Account) error {
	return nil
}

func (c *snapshotHydrationCache) DeleteAccount(ctx context.Context, accountID int64) error {
	return nil
}

func (c *snapshotHydrationCache) UpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	return nil
}

func (c *snapshotHydrationCache) TryLockBucket(ctx context.Context, bucket SchedulerBucket, ttl time.Duration) (bool, error) {
	return true, nil
}

func (c *snapshotHydrationCache) UnlockBucket(ctx context.Context, bucket SchedulerBucket) error {
	return nil
}

func (c *snapshotHydrationCache) ListBuckets(ctx context.Context) ([]SchedulerBucket, error) {
	return nil, nil
}

func (c *snapshotHydrationCache) GetOutboxWatermark(ctx context.Context) (int64, error) {
	return 0, nil
}

func (c *snapshotHydrationCache) SetOutboxWatermark(ctx context.Context, id int64) error {
	return nil
}

func TestOpenAISelectAccountWithLoadAwareness_HydratesSelectedAccountFromSchedulerSnapshot(t *testing.T) {
	cache := &snapshotHydrationCache{
		snapshot: []*Account{
			{
				ID:          1,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Priority:    1,
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"gpt-4": "gpt-4",
					},
				},
			},
		},
		accounts: map[int64]*Account{
			1: {
				ID:          1,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Priority:    1,
				Credentials: map[string]any{
					"api_key":       "sk-live",
					"model_mapping": map[string]any{"gpt-4": "gpt-4"},
				},
			},
		},
	}

	schedulerSnapshot := NewSchedulerSnapshotService(cache, nil, nil, nil, nil)
	groupID := int64(2)
	svc := &OpenAIGatewayService{
		schedulerSnapshot: schedulerSnapshot,
		cache:             &stubGatewayCache{},
	}

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if selection == nil || selection.Account == nil {
		t.Fatalf("expected selected account")
	}
	if got := selection.Account.GetOpenAIApiKey(); got != "sk-live" {
		t.Fatalf("expected hydrated api key, got %q", got)
	}
}

func TestGatewaySelectAccountWithLoadAwareness_HydratesSelectedAccountFromSchedulerSnapshot(t *testing.T) {
	cache := &snapshotHydrationCache{
		snapshot: []*Account{
			{
				ID:          9,
				Platform:    PlatformAnthropic,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Priority:    1,
			},
		},
		accounts: map[int64]*Account{
			9: {
				ID:          9,
				Platform:    PlatformAnthropic,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Priority:    1,
				Credentials: map[string]any{
					"api_key": "anthropic-live-key",
				},
			},
		},
	}

	schedulerSnapshot := NewSchedulerSnapshotService(cache, nil, nil, nil, nil)
	svc := &GatewayService{
		schedulerSnapshot: schedulerSnapshot,
		cache:             &mockGatewayCacheForPlatform{},
		cfg:               testConfig(),
	}

	result, err := svc.SelectAccountWithLoadAwareness(context.Background(), nil, "", "claude-3-5-sonnet-20241022", nil, "", 0)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
	}
	if result == nil || result.Account == nil {
		t.Fatalf("expected selected account")
	}
	if got := result.Account.GetCredential("api_key"); got != "anthropic-live-key" {
		t.Fatalf("expected hydrated api key, got %q", got)
	}
}

func TestSchedulerSnapshotHydration_PreservesOpenAIProjectionFields(t *testing.T) {
	bucket := SchedulerBucket{GroupID: 2, Platform: PlatformOpenAI, Mode: SchedulerModeSingle}
	builtAt := time.Unix(1_715_000_000, 0).UTC()
	cache := &snapshotHydrationCache{
		openAIStates: map[string]*OpenAISchedulerBucketState{
			bucket.String(): {
				Accounts: []*Account{{
					ID:          7,
					Platform:    PlatformOpenAI,
					Type:        AccountTypeAPIKey,
					Status:      StatusActive,
					Schedulable: true,
				}},
				ProjectionAccounts: []*Account{{
					ID:          7,
					Platform:    PlatformOpenAI,
					Type:        AccountTypeAPIKey,
					Status:      StatusActive,
					Schedulable: true,
				}},
				Projection: &OpenAIModelSubsetProjection{
					Bucket:            bucket,
					AccountReserveIDs: map[int64]struct{}{7: {}},
					Models: map[string]OpenAIModelRoleView{
						"gpt-5.4": {
							CanonicalModel:     "gpt-5.4",
							ReserveOverflowIDs: []int64{7},
						},
					},
				},
				ProjectionVersion: 7,
				BuiltAt:           builtAt,
			},
		},
	}

	schedulerSnapshot := NewSchedulerSnapshotService(cache, nil, nil, nil, nil)
	state, ok, err := schedulerSnapshot.GetOpenAIBucketState(context.Background(), bucket)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, state)
	require.Equal(t, int64(7), state.ProjectionVersion)
	require.True(t, state.BuiltAt.Equal(builtAt))
	require.Len(t, state.ProjectionAccounts, 1)
	require.Equal(t, int64(7), state.ProjectionAccounts[0].ID)
	view, found := state.Projection.ViewForModel("gpt-5.4")
	require.True(t, found)
	require.Equal(t, []int64{7}, view.ReserveOverflowIDs)
}
