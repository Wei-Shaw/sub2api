//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

// The OpenAI gateway must honor a group's per-model account routing
// (groups.model_routing), the same way the Anthropic/Gemini gateway does.
// Regression: only the Anthropic/Gemini path applied the routing, so a
// configured per-model candidate order had no effect on GPT requests.
func openAIModelRoutingTestService(t *testing.T, routing map[string][]int64) (*OpenAIGatewayService, context.Context, int64) {
	t.Helper()

	groupID := int64(40)
	group := &Group{
		ID:                  groupID,
		Platform:            PlatformComposite,
		Status:              StatusActive,
		Hydrated:            true,
		ModelRoutingEnabled: len(routing) > 0,
		ModelRouting:        routing,
	}

	repo := &mockAccountRepoForPlatform{accountsByID: map[int64]*Account{}}
	repo.accounts = []Account{
		{ID: 1, Platform: PlatformOpenAI, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5},
		{ID: 2, Platform: PlatformOpenAI, Priority: 2, Status: StatusActive, Schedulable: true, Concurrency: 5},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}

	cfg := testConfig()
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(&mockConcurrencyCache{}),
	}
	ctx := context.WithValue(context.Background(), ctxkey.Group, group)
	return svc, ctx, groupID
}

func TestOpenAISelectAccountWithLoadAwareness_HonorsModelRouting(t *testing.T) {
	t.Parallel()

	svc, ctx, groupID := openAIModelRoutingTestService(t, map[string][]int64{"gpt-x": {2}})
	res, err := svc.selectAccountWithLoadAwareness(ctx, &groupID, PlatformOpenAI, "", "gpt-x", nil, false, OpenAIEndpointCapabilityChatCompletions, false)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, res.Account)
	require.Equal(t, int64(2), res.Account.ID,
		"the routed account (id 2) must win over the higher-priority unrouted account (id 1)")
}

func TestOpenAISelectBestAccount_HonorsModelRouting(t *testing.T) {
	t.Parallel()

	svc, ctx, groupID := openAIModelRoutingTestService(t, map[string][]int64{"gpt-x": {2}})
	accounts, err := svc.listSchedulableAccounts(ctx, &groupID, PlatformOpenAI)
	require.NoError(t, err)
	acc, _, _ := svc.selectBestAccount(ctx, &groupID, PlatformOpenAI, accounts, "gpt-x", nil, false, OpenAIEndpointCapabilityChatCompletions, false)
	require.NotNil(t, acc)
	require.Equal(t, int64(2), acc.ID)
}

func TestOpenAISelectAccountWithLoadAwareness_RoutingUsesPublicAlias(t *testing.T) {
	t.Parallel()

	svc, ctx, groupID := openAIModelRoutingTestService(t, map[string][]int64{
		"gpt-x":      {1},
		"gpt-x-full": {2},
	})
	// A composite group rewrites the request body to the upstream name ("gpt-x"),
	// while the public alias ("gpt-x-full") is carried in context. Routing must
	// use the public alias so the premium route stays isolated.
	ctx = context.WithValue(ctx, ctxkey.RequestedPublicModel, "gpt-x-full")

	res, err := svc.selectAccountWithLoadAwareness(ctx, &groupID, PlatformOpenAI, "", "gpt-x", nil, false, OpenAIEndpointCapabilityChatCompletions, false)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, res.Account)
	require.Equal(t, int64(2), res.Account.ID,
		"routing resolves the public alias, so the -full route account is selected")
}

func TestOpenAISelectAccountWithLoadAwareness_FallsBackWhenRouteUnavailable(t *testing.T) {
	t.Parallel()

	svc, ctx, groupID := openAIModelRoutingTestService(t, map[string][]int64{"gpt-x": {999}})
	res, err := svc.selectAccountWithLoadAwareness(ctx, &groupID, PlatformOpenAI, "", "gpt-x", nil, false, OpenAIEndpointCapabilityChatCompletions, false)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, res.Account)
	require.Equal(t, int64(1), res.Account.ID,
		"routing that points at unavailable accounts must fall back to the full pool (priority order)")
}
