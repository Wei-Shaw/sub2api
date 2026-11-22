package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func keyGroupModelAccount(platform string, mapping map[string]string) Account {
	jsonMapping := make(map[string]any, len(mapping))
	for alias, target := range mapping {
		jsonMapping[alias] = target
	}
	return Account{ID: 5, Platform: platform, Type: "apikey", Status: StatusActive, Credentials: map[string]any{"model_mapping": jsonMapping}}
}

func TestKeyGroupModelsMergeRequestAliasesWithoutChangingTheirTarget(t *testing.T) {
	group := &Group{ID: 8, Platform: PlatformDeepseek}
	account := keyGroupModelAccount(PlatformDeepseek, map[string]string{
		"deepseek-flash":         "cn:deepseek-v4.1-flash",
		"deepseek-v4.1-flash":    "cn:deepseek-v4.1-flash",
		"cn:deepseek-v4.1-flash": "cn:deepseek-v4.1-flash",
	})

	models := keyGroupModelsFromAccounts(group, []Account{account})

	require.Equal(t, []KeyGroupModel{{
		ID: "cn:deepseek-v4.1-flash", UpstreamModel: "cn:deepseek-v4.1-flash",
		Aliases: []string{"cn:deepseek-v4.1-flash", "deepseek-flash", "deepseek-v4.1-flash"},
	}}, models)
}

func TestKeyGroupModelsUseOnlyCurrentlyAllowedRequestAliases(t *testing.T) {
	group := &Group{ID: 8, Platform: PlatformOpenAI, ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"public-model"}}}
	account := keyGroupModelAccount(PlatformOpenAI, map[string]string{
		"public-model": "actual-model", "other-alias": "actual-model", "actual-model": "actual-model",
	})

	models := keyGroupModelsFromAccounts(group, []Account{account})

	require.Equal(t, []KeyGroupModel{{ID: "public-model", UpstreamModel: "actual-model", Aliases: []string{"public-model"}}}, models)
}

func TestKeyGroupModelsDoNotCollapseDifferentActualModels(t *testing.T) {
	group := &Group{ID: 8, Platform: PlatformOpenAI}
	first := keyGroupModelAccount(PlatformOpenAI, map[string]string{"same-alias": "first-target"})
	second := keyGroupModelAccount(PlatformOpenAI, map[string]string{"same-alias": "second-target"})

	models := keyGroupModelsFromAccounts(group, []Account{first, second})

	require.Len(t, models, 2)
	require.Equal(t, "first-target", models[0].UpstreamModel)
	require.Equal(t, "second-target", models[1].UpstreamModel)
}

func TestKeyGroupModelsExcludeInactiveAndWrongPlatformAccounts(t *testing.T) {
	group := &Group{ID: 8, Platform: PlatformOpenAI}
	active := keyGroupModelAccount(PlatformOpenAI, map[string]string{"active-model": "active-target"})
	inactive := keyGroupModelAccount(PlatformOpenAI, map[string]string{"inactive-model": "inactive-target"})
	inactive.Status = StatusDisabled
	wrongPlatform := keyGroupModelAccount(PlatformAnthropic, map[string]string{"wrong-model": "wrong-target"})

	models := keyGroupModelsFromAccounts(group, []Account{inactive, wrongPlatform, active})

	require.Equal(t, []KeyGroupModel{{ID: "active-model", UpstreamModel: "active-target", Aliases: []string{"active-model"}}}, models)
}

func TestKeyGroupModelsResolveConcreteAllowlistAgainstWildcardMapping(t *testing.T) {
	group := &Group{ID: 8, Platform: PlatformOpenAI, ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"public-model-v2"}}}
	account := keyGroupModelAccount(PlatformOpenAI, map[string]string{"public-model-*": "actual-v2"})

	models := keyGroupModelsFromAccounts(group, []Account{account})

	require.Equal(t, []KeyGroupModel{{ID: "public-model-v2", UpstreamModel: "actual-v2", Aliases: []string{"public-model-v2"}}}, models)
}

func TestKeyGroupModelsListSingleAllowlistedModelOnUnmappedAccount(t *testing.T) {
	group := &Group{ID: 8, Platform: PlatformOpenAI, ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"gemini-4-hydrogen"}}}
	account := keyGroupModelAccount(PlatformOpenAI, nil)

	models := keyGroupModelsFromAccounts(group, []Account{account})

	require.Equal(t, []KeyGroupModel{{ID: "gemini-4-hydrogen", UpstreamModel: "gemini-4-hydrogen", Aliases: []string{"gemini-4-hydrogen"}}}, models)
}

func TestKeyGroupModelsDoNotReturnWildcardsAsRequestIDs(t *testing.T) {
	group := &Group{ID: 8, Platform: PlatformOpenAI}
	account := keyGroupModelAccount(PlatformOpenAI, map[string]string{"public-*": "actual-model"})

	models := keyGroupModelsFromAccounts(group, []Account{account})

	require.Empty(t, models)
}

func TestKeyGroupModelsUseAValidConcreteTargetCoveredByWildcardMapping(t *testing.T) {
	group := &Group{ID: 8, Platform: PlatformOpenAI}
	account := keyGroupModelAccount(PlatformOpenAI, map[string]string{"gemini-*": "gemini-4-hydrogen"})

	models := keyGroupModelsFromAccounts(group, []Account{account})

	require.Equal(t, []KeyGroupModel{{ID: "gemini-4-hydrogen", UpstreamModel: "gemini-4-hydrogen", Aliases: []string{"gemini-4-hydrogen"}}}, models)
}

func TestKeyGroupModelsRespectPassthroughIgnoringMappingTargets(t *testing.T) {
	group := &Group{ID: 8, Platform: PlatformOpenAI, ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"public-model"}}}
	account := keyGroupModelAccount(PlatformOpenAI, map[string]string{"public-model": "obsolete-target"})
	account.Extra = map[string]any{"openai_passthrough": true}

	models := keyGroupModelsFromAccounts(group, []Account{account})

	require.Equal(t, []KeyGroupModel{{ID: "public-model", UpstreamModel: "public-model", Aliases: []string{"public-model"}}}, models)
}
