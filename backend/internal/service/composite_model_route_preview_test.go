//go:build unit

package service

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

// accountRepoStubForCompositePreview filters by platform like the real repository.
type accountRepoStubForCompositePreview struct {
	AccountRepository
	accounts      []Account
	gotGroupID    int64
	gotPlatforms  []string
	listCallCount int
}

func (s *accountRepoStubForCompositePreview) ListSchedulableByGroupIDAndPlatforms(_ context.Context, groupID int64, platforms []string) ([]Account, error) {
	s.listCallCount++
	s.gotGroupID = groupID
	s.gotPlatforms = append([]string(nil), platforms...)
	out := make([]Account, 0, len(s.accounts))
	for _, acc := range s.accounts {
		if slices.Contains(platforms, acc.Platform) {
			out = append(out, acc)
		}
	}
	return out, nil
}

func newCompositePreviewService(accounts []Account) (*adminServiceImpl, *accountRepoStubForCompositePreview) {
	accountRepo := &accountRepoStubForCompositePreview{accounts: accounts}
	return &adminServiceImpl{
		groupRepo: &groupRepoStubForAdmin{getByID: &Group{ID: 7, Platform: PlatformComposite}},
		compositeRouteRepo: &compositeRouteRepoStubForAdmin{routes: []CompositeModelRoute{{
			ID: 11, GroupID: 7, PublicModel: "all/claude", MatchType: CompositeRouteMatchExact,
			TargetPlatform: PlatformAnthropic, UpstreamModel: "claude-sonnet-4-6",
			Endpoint: CompositeRouteEndpointAny, Priority: 100, Enabled: true,
		}}},
		accountRepo: accountRepo,
	}, accountRepo
}

func TestAdminService_PreviewCompositeRoute_ZeroAvailableAccountsWithoutTargetPlatform(t *testing.T) {
	svc, accountRepo := newCompositePreviewService([]Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
	})

	decision, err := svc.PreviewCompositeRoute(context.Background(), 7, CompositeRoutePreviewRequest{Model: "all/claude"})

	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.NotNil(t, decision.AvailableAccounts)
	require.Equal(t, 0, *decision.AvailableAccounts)
	require.Equal(t, int64(7), accountRepo.gotGroupID)
	require.Equal(t, []string{PlatformAnthropic, PlatformAntigravity}, accountRepo.gotPlatforms)
}

func TestAdminService_PreviewCompositeRoute_CountsOnlyAccountsServingUpstreamModel(t *testing.T) {
	sonnetOnly := map[string]any{"model_mapping": map[string]any{"claude-sonnet-4-6": "claude-sonnet-4-6"}}
	opusOnly := map[string]any{"model_mapping": map[string]any{"claude-opus-4-6": "claude-opus-4-6"}}
	svc, _ := newCompositePreviewService([]Account{
		{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey},                           // no mapping: all models
		{ID: 2, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: opusOnly},    // other model only
		{ID: 3, Platform: PlatformAntigravity, Type: AccountTypeOAuth, Credentials: sonnetOnly}, // mixed scheduling off
		{ID: 4, Platform: PlatformAntigravity, Type: AccountTypeOAuth, Credentials: sonnetOnly,
			Extra: map[string]any{"mixed_scheduling": true}},
		{ID: 5, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
	})

	decision, err := svc.PreviewCompositeRoute(context.Background(), 7, CompositeRoutePreviewRequest{Model: "all/claude"})

	require.NoError(t, err)
	require.NotNil(t, decision.AvailableAccounts)
	require.Equal(t, 2, *decision.AvailableAccounts)
}

func TestAdminService_PreviewCompositeRoute_UnmatchedSkipsAccountCount(t *testing.T) {
	svc, accountRepo := newCompositePreviewService(nil)

	decision, err := svc.PreviewCompositeRoute(context.Background(), 7, CompositeRoutePreviewRequest{Model: "unknown-model-xyz"})

	require.NoError(t, err)
	require.False(t, decision.Matched)
	require.Nil(t, decision.AvailableAccounts)
	require.Zero(t, accountRepo.listCallCount)
}
