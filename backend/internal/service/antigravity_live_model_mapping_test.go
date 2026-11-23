package service

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

func antigravityModelInfo() antigravity.ModelInfo {
	return antigravity.ModelInfo{QuotaInfo: &antigravity.ModelQuotaInfo{RemainingFraction: 1}}
}

func antigravityFetchResponseForIDs(ids []string) *antigravity.FetchAvailableModelsResponse {
	resp := &antigravity.FetchAvailableModelsResponse{Models: make(map[string]antigravity.ModelInfo, len(ids))}
	for _, id := range ids {
		resp.Models[id] = antigravityModelInfo()
	}
	return resp
}

// antigravityUltraCatalogFixture is the fetchAvailableModels catalog three Ultra
// accounts returned on 2026-11-23.
var antigravityUltraCatalogFixture = []string{
	"claude-opus-5-5-high", "claude-opus-5-5-low", "claude-opus-5-5-medium",
	"claude-sonnet-5-5-high", "claude-sonnet-5-5-low", "claude-sonnet-5-5-medium",
	"gemini-2.5-flash", "gemini-2.5-flash-lite", "gemini-2.5-flash-thinking", "gemini-2.5-pro",
	"gemini-3-flash", "gemini-3.1-flash-image", "gemini-3.1-flash-lite", "gemini-3.5-flash-lite",
	"gemini-3.8-flash-high", "gemini-3.8-flash-low", "gemini-3.8-flash-medium", "gemini-3.8-flash-tiered",
	"gemini-4-argon-high", "gemini-4-argon-low", "gemini-4-argon-medium",
	"gemini-4-carbon-high", "gemini-4-carbon-low", "gemini-4-carbon-medium",
	"gemini-4-hydrogen-high", "gemini-4-hydrogen-low", "gemini-4-hydrogen-medium", "gemini-4-hydrogen-xhigh",
	"gemini-4-hydrogen-sonar", "gemini-4-hydrogen-tiered-high", "gemini-4-hydrogen-tiered-xhigh",
	"gemini-4-neon-high", "gemini-4-neon-low", "gemini-4-neon-medium", "gemini-4-neon-xhigh",
	"gpt-oss-120b-medium", "tab_flash_lite_preview", "tab_jump_flash_lite_preview",
}

// antigravityUltraLiveMapping is the mapping the fixture must derive to.
var antigravityUltraLiveMapping = map[string]string{
	"claude-opus-5-5":             "claude-opus-5-5-high",
	"claude-sonnet-5-5":           "claude-sonnet-5-5-high",
	"gemini-3.8-flash":            "gemini-3.8-flash-high",
	"gemini-4-argon":              "gemini-4-argon-high",
	"gemini-4-carbon":             "gemini-4-carbon-high",
	"gemini-4-hydrogen":           "gemini-4-hydrogen-high",
	"gemini-4-hydrogen-tiered":    "gemini-4-hydrogen-tiered-high",
	"gemini-4-neon":               "gemini-4-neon-high",
	"gemini-2.5-flash":            "gemini-2.5-flash",
	"gemini-2.5-flash-lite":       "gemini-2.5-flash-lite",
	"gemini-2.5-flash-thinking":   "gemini-2.5-flash-thinking",
	"gemini-2.5-pro":              "gemini-2.5-pro",
	"gemini-3-flash":              "gemini-3-flash",
	"gemini-3.1-flash-image":      "gemini-3.1-flash-image",
	"gemini-3.1-flash-lite":       "gemini-3.1-flash-lite",
	"gemini-3.5-flash-lite":       "gemini-3.5-flash-lite",
	"gemini-3.8-flash-tiered":     "gemini-3.8-flash-tiered",
	"gemini-4-hydrogen-sonar":     "gemini-4-hydrogen-sonar",
	"gpt-oss-120b-medium":         "gpt-oss-120b-medium",
	"tab_flash_lite_preview":      "tab_flash_lite_preview",
	"tab_jump_flash_lite_preview": "tab_jump_flash_lite_preview",
}

func newLiveAntigravityAccount(catalog any, explicit map[string]any) *Account {
	credentials := map[string]any{AntigravityModelMappingModeCredentialKey: AntigravityModelMappingModeLive}
	if explicit != nil {
		credentials["model_mapping"] = explicit
	}
	account := &Account{ID: 1, Platform: domain.PlatformAntigravity, Type: AccountTypeOAuth, Credentials: credentials}
	if catalog != nil {
		account.Extra = map[string]any{AntigravityModelCatalogExtraKey: catalog}
	}
	return account
}

// decodedCatalog returns the catalog in the shape Extra holds after a DB decode.
func decodedCatalog(ids []string, syncedAt string) map[string]any {
	models := make([]any, len(ids))
	for i, id := range ids {
		models[i] = id
	}
	return map[string]any{"models": models, "synced_at": syncedAt}
}

func TestAntigravityCatalogLevelFamilies(t *testing.T) {
	require.Equal(t, map[string][]string{
		"claude-opus-5-5":          {"low", "medium", "high"},
		"claude-sonnet-5-5":        {"low", "medium", "high"},
		"gemini-3.8-flash":         {"low", "medium", "high"},
		"gemini-4-argon":           {"low", "medium", "high"},
		"gemini-4-carbon":          {"low", "medium", "high"},
		"gemini-4-hydrogen":        {"low", "medium", "high", "xhigh"},
		"gemini-4-hydrogen-tiered": {"high", "xhigh"},
		"gemini-4-neon":            {"low", "medium", "high", "xhigh"},
	}, antigravityCatalogLevelFamilies(antigravityUltraCatalogFixture))
}

func TestDeriveAntigravityLiveModelMapping(t *testing.T) {
	t.Run("fixture", func(t *testing.T) {
		got := deriveAntigravityLiveModelMapping(antigravityUltraCatalogFixture, nil)
		require.Len(t, got, 21)
		require.Equal(t, antigravityUltraLiveMapping, got)
	})

	t.Run("explicit entries win and aliases are kept", func(t *testing.T) {
		got := deriveAntigravityLiveModelMapping(antigravityUltraCatalogFixture, map[string]string{
			"claude-haiku-4-5": "claude-sonnet-5-5",
			"gemini-4-neon":    "gemini-4-neon-xhigh",
			"gemini-3-flash":   "gemini-3.5-flash-lite",
			"claude-*":         "claude-opus-5-5",
		})
		want := maps.Clone(antigravityUltraLiveMapping)
		want["claude-haiku-4-5"] = "claude-sonnet-5-5"
		want["gemini-4-neon"] = "gemini-4-neon-xhigh"
		want["gemini-3-flash"] = "gemini-3.5-flash-lite"
		want["claude-*"] = "claude-opus-5-5"
		require.Equal(t, want, got)
	})

	t.Run("disabled placeholders are dropped", func(t *testing.T) {
		catalog := append(append([]string(nil), antigravityUltraCatalogFixture...), "chat_20706-disabled", "gemini-4-neon-max-disabled")
		require.Equal(t, antigravityUltraLiveMapping, deriveAntigravityLiveModelMapping(catalog, nil))
	})
}

func TestAccountGetModelMapping_LiveMode(t *testing.T) {
	t.Run("derived from a decoded catalog", func(t *testing.T) {
		account := newLiveAntigravityAccount(decodedCatalog(antigravityUltraCatalogFixture, "2026-11-23T10:00:00Z"), nil)
		require.Equal(t, antigravityUltraLiveMapping, account.GetModelMapping())
	})

	t.Run("explicit overrides apply on top", func(t *testing.T) {
		account := newLiveAntigravityAccount(
			AntigravityModelCatalog{Models: antigravityUltraCatalogFixture, SyncedAt: "2026-11-23T10:00:00Z"},
			map[string]any{"claude-haiku-4-5": "claude-sonnet-5-5"},
		)
		mapping := account.GetModelMapping()
		require.Equal(t, "claude-sonnet-5-5", mapping["claude-haiku-4-5"])
		require.Equal(t, "gemini-4-neon-high", mapping["gemini-4-neon"])
		require.NotContains(t, mapping, "gemini-4-neon-high")
		require.NotContains(t, mapping, "gemini-3.1-pro-high", "live mode adds no default passthroughs")
	})

	t.Run("no catalog falls back to defaults with the explicit overlay", func(t *testing.T) {
		account := newLiveAntigravityAccount(nil, map[string]any{"claude-haiku-4-5": "claude-sonnet-5-5", "team-alias": "gemini-4-neon"})
		want := maps.Clone(domain.DefaultAntigravityModelMapping)
		want["claude-haiku-4-5"] = "claude-sonnet-5-5"
		want["team-alias"] = "gemini-4-neon"
		require.Equal(t, want, account.GetModelMapping())
		require.NotContains(t, domain.DefaultAntigravityModelMapping, "team-alias", "the shared default table must stay untouched")
		require.Equal(t, "claude-sonnet-4-6", domain.DefaultAntigravityModelMapping["claude-haiku-4-5"])
	})

	t.Run("empty catalog falls back to defaults", func(t *testing.T) {
		account := newLiveAntigravityAccount(decodedCatalog(nil, "2026-11-23T10:00:00Z"), nil)
		require.Equal(t, domain.DefaultAntigravityModelMapping, account.GetModelMapping())
	})

	t.Run("other modes keep the existing behaviour", func(t *testing.T) {
		account := newLiveAntigravityAccount(decodedCatalog(antigravityUltraCatalogFixture, "2026-11-23T10:00:00Z"), nil)
		account.Credentials[AntigravityModelMappingModeCredentialKey] = "static"
		require.Equal(t, domain.DefaultAntigravityModelMapping, account.GetModelMapping())
	})
}

func TestAccountGetModelMapping_LiveCatalogUpdateInvalidatesCache(t *testing.T) {
	account := newLiveAntigravityAccount(decodedCatalog([]string{"gemini-3-flash", "gemini-4-neon-high", "gemini-4-neon-low"}, "2026-11-23T10:00:00Z"), nil)
	require.Equal(t, map[string]string{"gemini-3-flash": "gemini-3-flash", "gemini-4-neon": "gemini-4-neon-high"}, account.GetModelMapping())

	// Same slice, same length, one ID replaced in place: only the signature changes.
	models := account.Extra[AntigravityModelCatalogExtraKey].(map[string]any)["models"].([]any)
	models[0] = "gemini-3.5-flash-lite"
	require.Equal(t, map[string]string{"gemini-3.5-flash-lite": "gemini-3.5-flash-lite", "gemini-4-neon": "gemini-4-neon-high"}, account.GetModelMapping())

	// A save replaces the catalog value on the same Account.
	persistAntigravityModelCatalog(context.Background(), &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{1: {ID: 1}}}, account, AntigravityModelCatalog{
		Models:   []string{"gemini-4-carbon-high", "gemini-4-carbon-low"},
		SyncedAt: time.Now().UTC().Format(time.RFC3339),
	})
	require.Equal(t, map[string]string{"gemini-4-carbon": "gemini-4-carbon-high"}, account.GetModelMapping())
	families, ok := account.antigravityLiveLevelFamilies()
	require.True(t, ok)
	require.Equal(t, map[string][]string{"gemini-4-carbon": {"low", "high"}}, families)

	// Switching the mode off on the same credentials map returns to the defaults.
	delete(account.Credentials, AntigravityModelMappingModeCredentialKey)
	require.Equal(t, domain.DefaultAntigravityModelMapping, account.GetModelMapping())
}

func TestAccountSharedSnapshot_LiveCatalogReplacementRebuilds(t *testing.T) {
	master := newLiveAntigravityAccount(decodedCatalog([]string{"gemini-4-neon-high", "gemini-4-neon-low"}, "2026-11-23T10:00:00Z"), nil)
	master.PrepareSharedSnapshot()
	clone := *master
	require.Equal(t, map[string]string{"gemini-4-neon": "gemini-4-neon-high"}, clone.GetModelMapping())

	clone.Extra = map[string]any{AntigravityModelCatalogExtraKey: decodedCatalog([]string{"gemini-3-flash"}, "2026-11-23T11:00:00Z")}
	require.Equal(t, map[string]string{"gemini-3-flash": "gemini-3-flash"}, clone.GetModelMapping())
	require.Equal(t, map[string]string{"gemini-4-neon": "gemini-4-neon-high"}, master.GetModelMapping())
}

func TestAntigravityLiveModeEffortSelection(t *testing.T) {
	svc := &AntigravityGatewayService{}
	account := newLiveAntigravityAccount(
		decodedCatalog(antigravityUltraCatalogFixture, "2026-11-23T10:00:00Z"),
		map[string]any{"claude-haiku-4-5": "claude-sonnet-5-5"},
	)
	cases := []struct {
		model  string
		effort string
		want   string
	}{
		{"gemini-4-neon", "xhigh", "gemini-4-neon-xhigh"},
		{"gemini-4-neon", "max", "gemini-4-neon-xhigh"},
		{"gemini-4-carbon", "xhigh", "gemini-4-carbon-high"},
		{"claude-opus-5-5", "low", "claude-opus-5-5-low"},
		{"claude-opus-5-5", "minimal", "claude-opus-5-5-low"},
		{"gemini-3.8-flash", "medium", "gemini-3.8-flash-medium"},
		{"gemini-3.8-flash", "low", "gemini-3.8-flash-low"},
		{"gemini-3.8-flash", "xhigh", "gemini-3.8-flash-high"},
		{"gemini-4-argon", "", "gemini-4-argon-high"},
		{"gemini-4-hydrogen", "", "gemini-4-hydrogen-high"},
		{"gemini-4-hydrogen-tiered", "xhigh", "gemini-4-hydrogen-tiered-xhigh"},
		{"gemini-4-hydrogen-tiered", "low", "gemini-4-hydrogen-tiered-high"},
		{"claude-haiku-4-5", "low", "claude-sonnet-5-5-low"},
		{"gpt-oss-120b-medium", "high", "gpt-oss-120b-medium"},
		{"gemini-3.8-flash-tiered", "low", "gemini-3.8-flash-tiered"},
		{"gemini-3-flash", "low", "gemini-3-flash"},
		{"gemini-4-neon-high", "low", ""},
		{"claude-fable-5", "high", ""},
	}
	for _, tc := range cases {
		t.Run(tc.model+"/"+tc.effort, func(t *testing.T) {
			got := svc.getMappedModelForThinkingLevel(account, tc.model, "", antigravityLevelFromEffort(tc.effort))
			require.Equal(t, tc.want, got)
		})
	}

	t.Run("gemini native thinkingConfig selects the gemini-3.8-flash tier", func(t *testing.T) {
		for body, want := range map[string]string{
			`{"generationConfig":{"thinkingConfig":{"thinkingLevel":"medium"}}}`: "gemini-3.8-flash-medium",
			`{"generationConfig":{"thinkingConfig":{"thinkingBudget":0}}}`:       "gemini-3.8-flash-low",
			`{"generationConfig":{"thinkingConfig":{"thinkingBudget":-1}}}`:      "gemini-3.8-flash-high",
			`{}`: "gemini-3.8-flash-high",
		} {
			got := svc.getMappedModelForThinkingLevel(account, "gemini-3.8-flash", geminiThinkingLevelFromBody([]byte(body)), antigravityFamilyLevelFromGeminiBody([]byte(body)))
			require.Equal(t, want, got, body)
		}
	})
}

func TestAntigravityLiveModeHonoursPinsToSiblingModels(t *testing.T) {
	svc := &AntigravityGatewayService{}
	account := newLiveAntigravityAccount(
		decodedCatalog(antigravityUltraCatalogFixture, "2026-11-23T10:00:00Z"),
		map[string]any{
			"gemini-3.8-flash":  "gemini-3.8-flash-tiered",
			"gemini-4-hydrogen": "gemini-4-hydrogen-tiered",
			"gemini-4-neon":     "gemini-4-neon-low",
		},
	)
	cases := []struct {
		model  string
		effort string
		want   string
	}{
		{"gemini-3.8-flash", "low", "gemini-3.8-flash-tiered"},
		{"gemini-4-hydrogen", "xhigh", "gemini-4-hydrogen-tiered-xhigh"},
		{"gemini-4-hydrogen", "", "gemini-4-hydrogen-tiered-high"},
		// A pin to a tier of the same family still follows the request effort.
		{"gemini-4-neon", "xhigh", "gemini-4-neon-xhigh"},
	}
	for _, tc := range cases {
		t.Run(tc.model+"/"+tc.effort, func(t *testing.T) {
			require.Equal(t, tc.want, svc.getMappedModelForThinkingLevel(account, tc.model, "", antigravityLevelFromEffort(tc.effort)))
		})
	}
}

func TestAntigravityModelCatalogFromResponseDropsUncallableEntries(t *testing.T) {
	yes := true
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	resp := antigravityFetchResponseForIDs(antigravityUltraCatalogFixture)
	resp.Models["chat_20706-disabled"] = antigravityModelInfo()
	internal := antigravityModelInfo()
	internal.IsInternal = &yes
	resp.Models["internal-model"] = internal
	disabled := antigravityModelInfo()
	disabled.Disabled = &yes
	resp.Models["disabled-model"] = disabled

	catalog, ok := newAntigravityModelCatalog(resp, now)
	require.True(t, ok)
	require.Equal(t, "2026-11-23T10:00:00Z", catalog.SyncedAt)
	require.Equal(t, normalizeAntigravityCatalogIDs(antigravityUltraCatalogFixture), catalog.Models)
	require.Len(t, catalog.Models, 38)

	_, ok = newAntigravityModelCatalog(antigravityFetchResponseForIDs([]string{"a-disabled"}), now)
	require.False(t, ok, "a response without callable models never replaces a saved catalog")
}

func TestUpdateAccountKeepsAntigravityModelCatalogAndMode(t *testing.T) {
	const accountID = int64(31)
	stored := decodedCatalog(antigravityUltraCatalogFixture, "2026-11-23T10:00:00Z")
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:       accountID,
			Name:     "ag",
			Platform: PlatformAntigravity,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			Credentials: map[string]any{
				"access_token":                           "token-keep",
				AntigravityModelMappingModeCredentialKey: AntigravityModelMappingModeLive,
			},
			Extra: map[string]any{AntigravityModelCatalogExtraKey: stored},
		},
	}}

	got, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{
			"note":                          "from-form",
			AntigravityModelCatalogExtraKey: decodedCatalog([]string{"injected"}, "2030-01-01T00:00:00Z"),
		},
		Credentials: map[string]any{
			AntigravityModelMappingModeCredentialKey: AntigravityModelMappingModeLive,
			"model_mapping":                          map[string]any{"claude-haiku-4-5": "claude-sonnet-5-5"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, stored, got.Extra[AntigravityModelCatalogExtraKey])
	require.Equal(t, "from-form", got.Extra["note"])
	require.Equal(t, AntigravityModelMappingModeLive, got.GetCredential(AntigravityModelMappingModeCredentialKey))
	require.Equal(t, "token-keep", got.GetCredential("access_token"))
	require.Equal(t, "claude-sonnet-5-5", got.GetModelMapping()["claude-haiku-4-5"])
	require.Equal(t, "gemini-4-neon-high", got.GetModelMapping()["gemini-4-neon"])
}

func TestPersistAntigravityModelCatalogSkipsUnchangedFreshList(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	ids := []string{"gemini-4-neon-high", "gemini-4-neon-low"}
	cases := []struct {
		name      string
		savedIDs  []string
		savedAt   time.Time
		wantWrite bool
	}{
		{"unchanged and fresh", ids, now.Add(-time.Hour), false},
		{"unchanged but past half the refresh age", ids, now.Add(-4 * time.Hour), true},
		{"changed list", []string{"gemini-4-neon-high"}, now.Add(-time.Minute), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{1: {ID: 1}}}
			account := newLiveAntigravityAccount(decodedCatalog(tc.savedIDs, tc.savedAt.Format(time.RFC3339)), nil)
			persistAntigravityModelCatalog(context.Background(), repo, account, AntigravityModelCatalog{Models: ids, SyncedAt: now.Format(time.RFC3339)})
			_, written := repo.accounts[1].Extra[AntigravityModelCatalogExtraKey]
			require.Equal(t, tc.wantWrite, written)
		})
	}
}
