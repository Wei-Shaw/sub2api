package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// Scenario: mixed groups prefer capability metadata synced for the routed account.
func TestBuildCodexModelsManifestForGroupUsesSyncedAccountMetadata(t *testing.T) {
	t.Parallel()

	const groupID int64 = 735
	account := Account{
		ID:       25,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url":      "https://opencode.ai/zen/v1",
			"model_mapping": map[string]any{"x-preview-f-free": "x-preview-f-free"},
		},
		Extra: map[string]any{
			UpstreamModelMetadataExtraKey: map[string]any{
				"source": "models.dev",
				"models": map[string]any{
					"x-preview-f-free": map[string]any{
						"id":                         "x-preview-f-free",
						"display_name":               "Ox Alpha Free (Unlimited)",
						"description":                "Stealth reasoning model",
						"reasoning":                  true,
						"supported_reasoning_levels": []any{"low", "high", "max"},
						"input_modalities":           []any{"text", "image"},
						"context_window":             float64(1_000_000),
						"max_output_tokens":          float64(131_072),
					},
				},
			},
		},
	}
	svc := &GatewayService{accountRepo: codexModelsVisibilityAccountRepo{byGroup: map[int64][]Account{
		groupID: {account},
	}}}

	body, err := svc.BuildCodexModelsManifestForGroup(
		context.Background(),
		&Group{ID: groupID, Platform: PlatformComposite},
		"",
		[]string{"x-preview-f-free"},
	)
	require.NoError(t, err)

	models := decodeCodexManifestModels(t, body)
	require.Len(t, models, 1)
	require.Equal(t, "Ox Alpha Free (Unlimited)", models[0]["display_name"])
	require.Equal(t, "low", models[0]["default_reasoning_level"])
	require.Equal(t, []string{"low", "high", "max"}, effortsFromManifestModel(t, models[0]))
	require.Equal(t, []any{"text", "image"}, models[0]["input_modalities"])
	require.EqualValues(t, 1_000_000, models[0]["context_window"])
}

// Scenario: an explicitly non-reasoning model remains directly selectable in Codex.
func TestBuildCodexModelsManifestForGroupUsesNoneForExplicitNonReasoningMetadata(t *testing.T) {
	t.Parallel()

	const groupID int64 = 737
	reasoning := false
	account := Account{
		ID: 28, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url":      "https://provider.example/v1",
			"model_mapping": map[string]any{"company-coding-model": "company-coding-model"},
		},
	}
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"company-coding-model": {
			ID: "company-coding-model", Reasoning: &reasoning,
			InputModalities: []string{"text"}, ContextWindow: 64_000,
		},
	}})
	svc := &GatewayService{accountRepo: codexModelsVisibilityAccountRepo{byGroup: map[int64][]Account{
		groupID: {account},
	}}}

	body, err := svc.BuildCodexModelsManifestForGroup(
		context.Background(), &Group{ID: groupID, Platform: PlatformComposite}, "", []string{"company-coding-model"},
	)
	require.NoError(t, err)
	models := decodeCodexManifestModels(t, body)
	require.Len(t, models, 1)
	require.Equal(t, "none", models[0]["default_reasoning_level"])
	require.Equal(t, []string{"none"}, effortsFromManifestModel(t, models[0]))
}

// Scenario: multiple schedulable accounts advertise only their shared capabilities.
func TestBuildCodexModelsManifestForGroupIntersectsSyncedAccountMetadata(t *testing.T) {
	t.Parallel()

	const groupID int64 = 736
	reasoning := true
	newAccount := func(id int64, levels, modalities []string, contextWindow int64) Account {
		account := Account{
			ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Credentials: map[string]any{
				"base_url":      "https://provider.example/v1",
				"model_mapping": map[string]any{"shared-model": "shared-model"},
			},
		}
		account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
			"shared-model": {
				ID: "shared-model", Reasoning: &reasoning,
				SupportedReasoningLevels: levels,
				InputModalities:          modalities,
				ContextWindow:            contextWindow,
			},
		}})
		return account
	}
	svc := &GatewayService{accountRepo: codexModelsVisibilityAccountRepo{byGroup: map[int64][]Account{
		groupID: {
			newAccount(26, []string{"low", "high"}, []string{"text", "image"}, 256_000),
			newAccount(27, []string{"high", "max"}, []string{"text"}, 128_000),
		},
	}}}

	body, err := svc.BuildCodexModelsManifestForGroup(
		context.Background(), &Group{ID: groupID, Platform: PlatformComposite}, "", []string{"shared-model"},
	)
	require.NoError(t, err)
	models := decodeCodexManifestModels(t, body)
	require.Len(t, models, 1)
	require.Equal(t, []string{"high"}, effortsFromManifestModel(t, models[0]))
	require.Equal(t, "high", models[0]["default_reasoning_level"])
	require.Equal(t, []any{"text"}, models[0]["input_modalities"])
	require.EqualValues(t, 128_000, models[0]["context_window"])
}

// Scenario: the same public alias may target different models on one platform when complete snapshots can be intersected.
func TestBuildCodexModelsManifestForGroupIntersectsDifferentMappedTargetsWithoutLeakingAlias(t *testing.T) {
	t.Parallel()

	const groupID int64 = 739
	reasoning := true
	newAccount := func(id int64, target, displayName, description string, levels, modalities []string, contextWindow int64) Account {
		account := Account{
			ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Credentials: map[string]any{
				"base_url":      "https://provider.example/v1",
				"model_mapping": map[string]any{"my-coder": target},
			},
		}
		account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
			target: {
				ID: target, DisplayName: displayName, Description: description, Reasoning: &reasoning,
				SupportedReasoningLevels: levels,
				InputModalities:          modalities,
				ContextWindow:            contextWindow,
			},
		}})
		return account
	}
	openAIAccount := newAccount(
		31,
		"gpt-5.6-sol",
		"GPT-5.6 Sol",
		"OpenAI upstream model",
		[]string{"low", "medium", "high", "xhigh"},
		[]string{"text", "image"},
		272_000,
	)
	arkAccount := newAccount(
		32,
		"glm-5.3",
		"GLM 5.3",
		"Ark upstream model",
		[]string{"low", "medium", "high"},
		[]string{"text"},
		1_000_000,
	)

	for _, accounts := range [][]Account{{openAIAccount, arkAccount}, {arkAccount, openAIAccount}} {
		svc := &GatewayService{accountRepo: codexModelsVisibilityAccountRepo{byGroup: map[int64][]Account{
			groupID: accounts,
		}}}
		body, err := svc.BuildCodexModelsManifestForGroup(
			context.Background(), &Group{ID: groupID, Platform: PlatformOpenAI}, "", []string{"my-coder"},
		)
		require.NoError(t, err)
		models := decodeCodexManifestModels(t, body)
		require.Len(t, models, 1)
		require.Equal(t, "my-coder", models[0]["slug"])
		require.Equal(t, "my-coder", models[0]["display_name"])
		require.Equal(t, "Custom model routed through Sub2API.", models[0]["description"])
		require.Equal(t, []string{"low", "medium", "high"}, effortsFromManifestModel(t, models[0]))
		require.Equal(t, []any{"text"}, models[0]["input_modalities"])
		require.EqualValues(t, 272_000, models[0]["context_window"])
	}
}

// Scenario: accounts that leave the current scheduling pool only because of
// transient state still participate in capability intersection.
func TestBuildCodexModelsManifestForGroupIntersectsTransientlyUnschedulableMappedAccounts(t *testing.T) {
	t.Parallel()

	const groupID int64 = 741
	schedulable := newCodexCatalogMappedAccount(
		41,
		"gpt-5.6-sol",
		"GPT-5.6 Sol",
		[]string{"low", "medium", "high", "xhigh"},
		[]string{"text", "image"},
		1_000_000,
		true,
		nil,
	)
	transientlyUnschedulable := newCodexCatalogMappedAccount(
		42,
		"glm-5.3",
		"GLM 5.3",
		[]string{"low", "medium", "high"},
		[]string{"text"},
		272_000,
		true,
		map[string]any{"exclusive-model": "exclusive-upstream"},
	)
	svc := &GatewayService{accountRepo: splitCodexModelsAccountRepo{
		schedulable: map[int64][]Account{groupID: {schedulable}},
		catalog:     map[int64][]Account{groupID: {schedulable, transientlyUnschedulable}},
	}}

	body, err := svc.BuildCodexModelsManifestForGroup(
		context.Background(), &Group{ID: groupID, Platform: PlatformOpenAI}, "", []string{"my-coder"},
	)
	require.NoError(t, err)
	models := decodeCodexManifestModels(t, body)
	require.Len(t, models, 1)
	require.Equal(t, "my-coder", models[0]["slug"])
	require.Equal(t, "my-coder", models[0]["display_name"])
	require.Equal(t, []string{"low", "medium", "high"}, effortsFromManifestModel(t, models[0]))
	require.Equal(t, []any{"text"}, models[0]["input_modalities"])
	require.EqualValues(t, 272_000, models[0]["context_window"])
}

// Scenario: a persistently disabled account cannot narrow the advertised contract.
func TestBuildCodexModelsManifestForGroupIgnoresPersistentlyDisabledMappedAccounts(t *testing.T) {
	t.Parallel()

	const groupID int64 = 742
	remaining := newCodexCatalogMappedAccount(
		41,
		"gpt-5.6-sol",
		"GPT-5.6 Sol",
		[]string{"low", "medium", "high", "xhigh"},
		[]string{"text", "image"},
		1_000_000,
		true,
		nil,
	)
	disabled := newCodexCatalogMappedAccount(
		42,
		"glm-5.3",
		"GLM 5.3",
		[]string{"low", "medium", "high"},
		[]string{"text"},
		272_000,
		false,
		nil,
	)
	svc := &GatewayService{accountRepo: splitCodexModelsAccountRepo{
		schedulable: map[int64][]Account{groupID: {remaining}},
		catalog:     map[int64][]Account{groupID: {remaining}},
		all:         map[int64][]Account{groupID: {remaining, disabled}},
	}}

	body, err := svc.BuildCodexModelsManifestForGroup(
		context.Background(), &Group{ID: groupID, Platform: PlatformOpenAI}, "", []string{"my-coder"},
	)
	require.NoError(t, err)
	models := decodeCodexManifestModels(t, body)
	require.Len(t, models, 1)
	require.Equal(t, []any{"text", "image"}, models[0]["input_modalities"])
	require.EqualValues(t, 1_000_000, models[0]["context_window"])
}

func TestBuildCodexModelsManifestForGroupFallsBackToSchedulableWhenAvailabilityLookupFails(t *testing.T) {
	t.Parallel()

	const groupID int64 = 743
	repo := &countingCodexModelsAccountRepo{
		accounts: []Account{newCodexCatalogMappedAccount(
			41,
			"gpt-5.6-sol",
			"GPT-5.6 Sol",
			[]string{"low", "medium", "high", "xhigh"},
			[]string{"text", "image"},
			1_000_000,
			true,
			nil,
		)},
		availabilityErr: errors.New("group listing unavailable"),
	}
	svc := &GatewayService{accountRepo: repo}

	body, err := svc.BuildCodexModelsManifestForGroup(
		context.Background(), &Group{ID: groupID, Platform: PlatformOpenAI}, "", []string{"my-coder"},
	)
	require.NoError(t, err)
	require.Equal(t, int32(1), repo.calls.Load())
	models := decodeCodexManifestModels(t, body)
	require.Len(t, models, 1)
	require.Equal(t, []any{"text", "image"}, models[0]["input_modalities"])
	require.EqualValues(t, 1_000_000, models[0]["context_window"])
}

// Scenario: a Composite alias claimed across platforms remains ambiguous and fails closed.
func TestBuildCodexModelsManifestForGroupKeepsCrossPlatformAliasAmbiguityClosed(t *testing.T) {
	t.Parallel()

	const groupID int64 = 740
	reasoning := true
	newAccount := func(id int64, platform, target string) Account {
		account := Account{
			ID: id, Platform: platform, Type: AccountTypeAPIKey,
			Credentials: map[string]any{"model_mapping": map[string]any{"shared-alias": target}},
		}
		account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
			target: {
				ID: target, DisplayName: target, Reasoning: &reasoning,
				SupportedReasoningLevels: []string{"low", "high"},
				InputModalities:          []string{"text", "image"},
				ContextWindow:            128_000,
			},
		}})
		return account
	}
	svc := &GatewayService{accountRepo: codexModelsVisibilityAccountRepo{byGroup: map[int64][]Account{
		groupID: {
			newAccount(33, PlatformOpenAI, "gpt-5.6-sol"),
			newAccount(34, PlatformGrok, "grok-4.6"),
		},
	}}}

	body, err := svc.BuildCodexModelsManifestForGroup(
		context.Background(), &Group{ID: groupID, Platform: PlatformComposite}, "", []string{"shared-alias"},
	)
	require.NoError(t, err)
	models := decodeCodexManifestModels(t, body)
	require.Len(t, models, 1)
	require.Equal(t, "shared-alias", models[0]["display_name"])
	require.Empty(t, effortsFromManifestModel(t, models[0]))
	require.Equal(t, []any{"text"}, models[0]["input_modalities"])
}

func TestBuildCodexModelsManifestForGroupDoesNotAdvertiseNoneWhenAccountReasoningConflicts(t *testing.T) {
	t.Parallel()

	const groupID int64 = 738
	reasoning := true
	noReasoning := false
	newAccount := func(id int64, metadata UpstreamModelMetadata) Account {
		account := Account{
			ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Credentials: map[string]any{
				"base_url":      "https://provider.example/v1",
				"model_mapping": map[string]any{"shared-model": "shared-model"},
			},
		}
		account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
			"shared-model": metadata,
		}})
		return account
	}
	svc := &GatewayService{accountRepo: codexModelsVisibilityAccountRepo{byGroup: map[int64][]Account{
		groupID: {
			newAccount(29, UpstreamModelMetadata{
				ID: "shared-model", Reasoning: &reasoning,
				SupportedReasoningLevels: []string{"low", "high"},
				InputModalities:          []string{"text"}, ContextWindow: 128_000,
			}),
			newAccount(30, UpstreamModelMetadata{
				ID: "shared-model", Reasoning: &noReasoning,
				InputModalities: []string{"text"}, ContextWindow: 128_000,
			}),
		},
	}}}

	body, err := svc.BuildCodexModelsManifestForGroup(
		context.Background(), &Group{ID: groupID, Platform: PlatformComposite}, "", []string{"shared-model"},
	)
	require.NoError(t, err)
	models := decodeCodexManifestModels(t, body)
	require.Len(t, models, 1)
	_, hasDefault := models[0]["default_reasoning_level"]
	require.False(t, hasDefault)
	require.Empty(t, models[0]["supported_reasoning_levels"])
}
