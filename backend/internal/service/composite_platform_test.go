package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

type compositeOwnershipAccountRepo struct {
	AccountRepository
	accounts             []Account
	cooledAccountIDs     map[int64]struct{}
	disabledAccountIDs   map[int64]struct{}
	listSchedulableCalls int
	listCandidateCalls   int
	candidatePlatforms   []string
	candidateGroupID     *int64
}

func (r *compositeOwnershipAccountRepo) ListSchedulableByGroupID(context.Context, int64) ([]Account, error) {
	r.listSchedulableCalls++
	return r.filteredAccounts(true), nil
}

func (r *compositeOwnershipAccountRepo) ListModelAvailabilityCandidates(_ context.Context, groupID *int64, platforms []string, _ bool) ([]Account, error) {
	r.listCandidateCalls++
	r.candidatePlatforms = append([]string(nil), platforms...)
	if groupID != nil {
		groupIDCopy := *groupID
		r.candidateGroupID = &groupIDCopy
	}
	return r.filteredAccounts(false), nil
}

func (r *compositeOwnershipAccountRepo) filteredAccounts(ignoreTransientState bool) []Account {
	accounts := make([]Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if _, disabled := r.disabledAccountIDs[account.ID]; disabled {
			continue
		}
		if ignoreTransientState {
			if _, cooled := r.cooledAccountIDs[account.ID]; cooled {
				continue
			}
		}
		accounts = append(accounts, account)
	}
	return accounts
}

// Scenario: 唯一平台的精确别名可路由
func TestResolveCompositeModelOwnershipKeepsProviderAccountsIsolated(t *testing.T) {
	groupID := int64(7)
	repo := &compositeOwnershipAccountRepo{
		accounts: []Account{
			{
				ID:       1,
				Platform: PlatformOpenAI,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"gpt-public": "gpt-5"},
				},
			},
			{
				ID:       2,
				Platform: PlatformDeepseek,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"reasoning-alias": "deepseek-v4-pro"},
				},
			},
		},
	}
	svc := &GatewayService{accountRepo: repo}

	deepSeekOwnership, err := svc.resolveCompositeModelOwnership(context.Background(), groupID, "reasoning-alias")
	require.NoError(t, err)
	require.Equal(t, CompositeModelOwnership{TargetPlatform: PlatformDeepseek, Matched: true}, deepSeekOwnership)

	openAIOwnership, err := svc.resolveCompositeModelOwnership(context.Background(), groupID, "gpt-public")
	require.NoError(t, err)
	require.Equal(t, CompositeModelOwnership{TargetPlatform: PlatformOpenAI, Matched: true}, openAIOwnership)
}

// Scenario: 通配符和空映射不声明所有权
func TestResolveCompositeModelOwnershipRequiresNonEmptyExactMappings(t *testing.T) {
	groupID := int64(7)
	repo := &compositeOwnershipAccountRepo{
		accounts: []Account{
			{
				ID:       1,
				Platform: PlatformOpenAI,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"*": "gpt-5", "gpt-*": "gpt-5", "empty-alias": ""},
				},
			},
			{
				ID:       2,
				Platform: PlatformGrok,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"grok-public": "grok-4"},
				},
			},
		},
	}
	svc := &GatewayService{accountRepo: repo}

	for _, model := range []string{"gpt-5", "empty-alias", "unknown-alias"} {
		ownership, err := svc.resolveCompositeModelOwnership(context.Background(), groupID, model)
		require.NoError(t, err)
		require.Equal(t, CompositeModelOwnership{}, ownership, "model=%s", model)
	}

	ownership, err := svc.resolveCompositeModelOwnership(context.Background(), groupID, "grok-public")
	require.NoError(t, err)
	require.Equal(t, CompositeModelOwnership{TargetPlatform: PlatformGrok, Matched: true}, ownership)
}

func TestResolveCompositeModelOwnershipAllowsSamePlatformAndRejectsCrossPlatformAliases(t *testing.T) {
	groupID := int64(7)
	repo := &compositeOwnershipAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{"shared-openai": "gpt-5", "ambiguous": "gpt-5"}}},
			{ID: 2, Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{"shared-openai": "gpt-5.1"}}},
			{ID: 3, Platform: PlatformDeepseek, Credentials: map[string]any{"model_mapping": map[string]any{"ambiguous": "deepseek-v4-pro"}}},
		},
	}
	svc := &GatewayService{accountRepo: repo}

	samePlatform, err := svc.resolveCompositeModelOwnership(context.Background(), groupID, "shared-openai")
	require.NoError(t, err)
	require.Equal(t, CompositeModelOwnership{TargetPlatform: PlatformOpenAI, Matched: true}, samePlatform)

	ambiguous, err := svc.resolveCompositeModelOwnership(context.Background(), groupID, "ambiguous")
	require.NoError(t, err)
	require.Equal(t, CompositeModelOwnership{Ambiguous: true}, ambiguous)
}

func TestResolveCompositeModelOwnershipIgnoresCooledOwnerForAccountModelSource(t *testing.T) {
	const model = "cmdc/deepseek-v4.1-flash"
	groupID := int64(7)
	repo := &compositeOwnershipAccountRepo{
		accounts: []Account{{
			ID:          1,
			Platform:    PlatformOpenAI,
			Credentials: map[string]any{"model_mapping": map[string]any{model: "gpt-5"}},
		}},
		cooledAccountIDs: map[int64]struct{}{1: {}},
	}
	detected, ok := DetectModelPlatform(model)
	require.True(t, ok)
	require.Equal(t, PlatformDeepseek, detected)

	decision := resolveCompositeOwnershipDecision(t, repo, groupID, model)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceAccount, decision.Source)
	require.Equal(t, PlatformOpenAI, decision.TargetPlatform)
	require.Equal(t, 1, repo.listCandidateCalls)
	require.Zero(t, repo.listSchedulableCalls)
	requirePersistentOwnershipCandidates(t, repo, groupID)
}

func TestResolveCompositeModelOwnershipExcludesPermanentlyDisabledAccounts(t *testing.T) {
	const model = "disabled-owner-alias"
	groupID := int64(7)
	repo := &compositeOwnershipAccountRepo{
		accounts: []Account{{
			ID:          1,
			Platform:    PlatformOpenAI,
			Credentials: map[string]any{"model_mapping": map[string]any{model: "gpt-5"}},
		}},
		disabledAccountIDs: map[int64]struct{}{1: {}},
	}

	decision := resolveCompositeOwnershipDecision(t, repo, groupID, model)
	require.False(t, decision.Matched)
	require.Empty(t, decision.TargetPlatform)
	require.Equal(t, "no explicit route or built-in detector match", decision.Reason)
	require.Equal(t, 1, repo.listCandidateCalls)
	require.Zero(t, repo.listSchedulableCalls)
	requirePersistentOwnershipCandidates(t, repo, groupID)
}

func TestResolveCompositeModelOwnershipRemainsAmbiguousWhenOneOwnerCooled(t *testing.T) {
	const model = "shared-provider-alias"
	groupID := int64(7)
	repo := &compositeOwnershipAccountRepo{
		accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformOpenAI,
				Credentials: map[string]any{"model_mapping": map[string]any{model: "gpt-5"}},
			},
			{
				ID:          2,
				Platform:    PlatformDeepseek,
				Credentials: map[string]any{"model_mapping": map[string]any{model: "deepseek-v4-pro"}},
			},
		},
		cooledAccountIDs: map[int64]struct{}{1: {}},
	}

	decision := resolveCompositeOwnershipDecision(t, repo, groupID, model)
	require.False(t, decision.Matched)
	require.Empty(t, decision.Source)
	require.Equal(t, "model is exposed by multiple provider platforms", decision.Reason)
	require.Equal(t, 1, repo.listCandidateCalls)
	require.Zero(t, repo.listSchedulableCalls)
	requirePersistentOwnershipCandidates(t, repo, groupID)
}

func requirePersistentOwnershipCandidates(t *testing.T, repo *compositeOwnershipAccountRepo, groupID int64) {
	t.Helper()
	require.NotEmpty(t, repo.candidatePlatforms)
	expectedPlatforms := schedulerSnapshotPlatforms()
	require.ElementsMatch(t, expectedPlatforms[:], repo.candidatePlatforms)
	require.NotNil(t, repo.candidateGroupID)
	require.Equal(t, groupID, *repo.candidateGroupID)
}

func resolveCompositeOwnershipDecision(t *testing.T, repo AccountRepository, groupID int64, model string) CompositeRouteDecision {
	t.Helper()
	resolver := NewCompositeRouteResolver(nil)
	svc := &GatewayService{accountRepo: repo}
	resolver.SetModelOwnershipResolver(svc.resolveCompositeModelOwnership)
	decision, err := resolver.Resolve(context.Background(), groupID, model, CompositeRouteEndpointResponses)
	require.NoError(t, err)
	return decision
}

func TestNewGatewayServiceWiresCompositeModelOwnershipResolver(t *testing.T) {
	groupID := int64(7)
	repo := &compositeOwnershipAccountRepo{
		accounts: []Account{{
			ID:          1,
			Platform:    PlatformDeepseek,
			Credentials: map[string]any{"model_mapping": map[string]any{"reasoning-alias": "deepseek-v4-pro"}},
		}},
	}
	resolver := NewCompositeRouteResolver(nil)
	svc := NewGatewayService(
		repo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		resolver,
		nil,
		nil,
	)
	require.Same(t, resolver, svc.compositeResolver)

	decision, err := resolver.Resolve(context.Background(), groupID, "reasoning-alias", CompositeRouteEndpointResponses)
	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceAccount, decision.Source)
	require.Equal(t, PlatformDeepseek, decision.TargetPlatform)
}

func TestDetectModelPlatform(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		platform string
		ok       bool
	}{
		{name: "claude", model: "claude-sonnet-4-5", platform: PlatformAnthropic, ok: true},
		{name: "anthropic prefix", model: "anthropic/claude-opus-4-5", platform: PlatformAnthropic, ok: true},
		{name: "gpt", model: "gpt-5.1", platform: PlatformOpenAI, ok: true},
		{name: "o series", model: "o3-mini", platform: PlatformOpenAI, ok: true},
		{name: "embedding", model: "text-embedding-3-large", platform: PlatformOpenAI, ok: true},
		{name: "gemini", model: "gemini-3-pro", platform: PlatformGemini, ok: true},
		{name: "gemini models prefix", model: "models/gemini-2.5-flash", platform: PlatformGemini, ok: true},
		{name: "learnlm", model: "learnlm-2.0-flash-experimental", platform: PlatformGemini, ok: true},
		{name: "grok", model: "grok-4", platform: PlatformGrok, ok: true},
		{name: "xai prefix", model: "xai/grok-4", platform: PlatformGrok, ok: true},
		{name: "kimi", model: "kimi-k2-thinking", platform: PlatformKimi, ok: true},
		{name: "kimi code bare k3", model: "K3", platform: PlatformKimi, ok: true},
		{name: "kimi code bare k3 256k", model: "k3-256k", platform: PlatformKimi, ok: true},
		{name: "kimi code provider prefix", model: "kimi-code/k3", platform: PlatformKimi, ok: true},
		{name: "moonshot prefix", model: "moonshot/moonshot-v1-32k", platform: PlatformKimi, ok: true},
		{name: "zhipu", model: "glm-5.2", platform: PlatformZhipu, ok: true},
		{name: "deepseek", model: "deepseek-v4-pro", platform: PlatformDeepseek, ok: true},
		{name: "minimax", model: "MiniMax-M3", platform: PlatformMiniMax, ok: true},
		{name: "minimax prefix", model: "minimax/MiniMax-M2.5", platform: PlatformMiniMax, ok: true},
		{name: "abab legacy", model: "abab6.5-chat", platform: PlatformMiniMax, ok: true},
		{name: "abab7 legacy", model: "abab7-chat-preview", platform: PlatformMiniMax, ok: true},
		{name: "jev", model: "jev-latest", platform: PlatformTypeSafe, ok: true},
		{name: "typesafe prefix", model: "typesafe/jev-latest", platform: PlatformTypeSafe, ok: true},
		{name: "abab unrelated namespace", model: "abab-other", ok: false},
		{name: "unknown k3 alias", model: "k3-preview", ok: false},
		{name: "unknown", model: "llama-4-maverick", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platform, ok := DetectModelPlatform(tt.model)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.platform, platform)
		})
	}
}

func TestQuotaPlatformCompositeUsesResolvedOrForceOnly(t *testing.T) {
	apiKey := &APIKey{Group: &Group{Platform: PlatformComposite}}

	require.Equal(t, "", QuotaPlatform(context.Background(), apiKey))
	require.Equal(t, PlatformGemini, QuotaPlatform(WithResolvedTargetPlatform(context.Background(), PlatformGemini), apiKey))
	require.Equal(t, PlatformAntigravity, QuotaPlatform(context.WithValue(context.Background(), ctxkey.ForcePlatform, PlatformAntigravity), apiKey))

	ctx := WithResolvedTargetPlatform(context.Background(), PlatformAnthropic)
	ctx = context.WithValue(ctx, ctxkey.ForcePlatform, PlatformAntigravity)
	require.Equal(t, PlatformAntigravity, QuotaPlatform(ctx, apiKey))
}

func TestCompositeGroupSchedulerHasAllCanonicalPlatformBuckets(t *testing.T) {
	seen := make(map[string]struct{})
	for _, bucket := range schedulerCanonicalBuckets(99) {
		seen[bucket.Platform] = struct{}{}
	}
	platforms := make([]string, 0, len(seen))
	for platform := range seen {
		platforms = append(platforms, platform)
	}
	require.ElementsMatch(t,
		[]string{PlatformAnthropic, PlatformGemini, PlatformOpenAI, PlatformAntigravity, PlatformGrok, PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo, PlatformTypeSafe},
		platforms,
	)
}

func TestCompositeConcretePlatformsIncludeCNProviders(t *testing.T) {
	for _, platform := range []string{PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo, PlatformTypeSafe} {
		require.True(t, isConcreteRequestPlatform(platform))
		require.True(t, canCopyAccountsFromGroupPlatform(PlatformComposite, platform))
	}
}
