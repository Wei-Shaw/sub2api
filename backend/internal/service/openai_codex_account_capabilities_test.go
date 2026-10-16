//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 2026-10-16 实测目录样本的能力相关子集：Blue 账号在 gpt-6-sol 上带 daybreak_blue，
// gpt-6.1-sol 上带 ultrafast；无项目账号每个模型都只带 standard 与 priority。
const codexCapabilitiesBlueAccountManifest = `{"models":[
  {"slug":"gpt-6-sol","service_tiers":[{"id":"priority","name":"Fast"}],"available_access_programs":{"cyber":["standard","daybreak_blue"]}},
  {"slug":"gpt-6.1-sol","service_tiers":[{"id":"priority","name":"Fast"},{"id":"ultrafast","name":"Ultrafast"}],"available_access_programs":{"cyber":["standard"]}},
  {"slug":"gpt-daybreak-blue-latest","service_tiers":[],"available_access_programs":{"cyber":["daybreak_blue"]}}
]}`

const codexCapabilitiesStandardAccountManifest = `{"models":[
  {"slug":"gpt-6-sol","service_tiers":[{"id":"priority","name":"Fast"}],"available_access_programs":{"cyber":["standard"]}},
  {"slug":"gpt-6.1-sol","service_tiers":[{"id":"priority","name":"Fast"}],"available_access_programs":{"cyber":["standard"]}}
]}`

func TestDeriveCodexAccountCapabilities_BlueAccount(t *testing.T) {
	caps, ok := DeriveCodexAccountCapabilities([]byte(codexCapabilitiesBlueAccountManifest))
	require.True(t, ok)
	require.Equal(t, []string{"daybreak_blue"}, caps.CyberPrograms, "账号级集合不含 standard")
	require.Equal(t, []string{"priority", "ultrafast"}, caps.ServiceTiers)
	require.Equal(t, CodexModelCapabilities{ServiceTiers: []string{"priority"}, CyberPrograms: []string{"daybreak_blue", "standard"}}, caps.Models["gpt-6-sol"])
	require.Equal(t, CodexModelCapabilities{ServiceTiers: []string{"priority", "ultrafast"}, CyberPrograms: []string{"standard"}}, caps.Models["gpt-6.1-sol"])
	require.Equal(t, CodexModelCapabilities{CyberPrograms: []string{"daybreak_blue"}}, caps.Models["gpt-daybreak-blue-latest"])
	require.False(t, caps.SyncedAt.IsZero())
	require.True(t, caps.SupportsCyberProgram("daybreak_blue"))
	require.False(t, caps.SupportsCyberProgram("daybreak_red"))
	require.True(t, caps.SupportsServiceTier("ultrafast"))
}

func TestDeriveCodexAccountCapabilities_StandardAccount(t *testing.T) {
	caps, ok := DeriveCodexAccountCapabilities([]byte(codexCapabilitiesStandardAccountManifest))
	require.True(t, ok)
	require.Empty(t, caps.CyberPrograms)
	require.Equal(t, []string{"priority"}, caps.ServiceTiers)
	require.Len(t, caps.Models, 2)
	require.False(t, caps.SupportsServiceTier("ultrafast"))
}

func TestDeriveCodexAccountCapabilities_EmptyOrInvalid(t *testing.T) {
	for name, body := range map[string]string{
		"empty list":   `{"models":[]}`,
		"missing":      `{"metadata":{}}`,
		"not array":    `{"models":{}}`,
		"invalid json": `{"models":[`,
		"no slugs":     `{"models":[{"display_name":"x"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, ok := DeriveCodexAccountCapabilities([]byte(body))
			require.False(t, ok)
		})
	}
}

func TestDeriveCodexAccountCapabilities_EquivalentIgnoresSyncedAt(t *testing.T) {
	first, ok := DeriveCodexAccountCapabilities([]byte(codexCapabilitiesBlueAccountManifest))
	require.True(t, ok)
	second, ok := DeriveCodexAccountCapabilities([]byte(codexCapabilitiesBlueAccountManifest))
	require.True(t, ok)
	second.SyncedAt = second.SyncedAt.Add(1)
	require.True(t, first.EquivalentTo(&second))

	standard, ok := DeriveCodexAccountCapabilities([]byte(codexCapabilitiesStandardAccountManifest))
	require.True(t, ok)
	require.False(t, first.EquivalentTo(&standard))
}

func TestAccountCodexCapabilities_ReadsStructAndMapForms(t *testing.T) {
	derived, ok := DeriveCodexAccountCapabilities([]byte(codexCapabilitiesBlueAccountManifest))
	require.True(t, ok)

	t.Run("struct value", func(t *testing.T) {
		account := &Account{Extra: map[string]any{CodexCapabilitiesExtraKey: derived}}
		require.True(t, account.CodexCapabilities().SupportsCyberProgram("daybreak_blue"))
	})
	t.Run("map value from JSONB", func(t *testing.T) {
		account := &Account{Extra: map[string]any{CodexCapabilitiesExtraKey: map[string]any{
			"models":         map[string]any{"gpt-6-sol": map[string]any{"service_tiers": []any{"priority"}, "cyber_programs": []any{"standard", "daybreak_red"}}},
			"service_tiers":  []any{"priority"},
			"cyber_programs": []any{"daybreak_red"},
			"synced_at":      "2026-10-16T00:00:00Z",
		}}}
		caps := account.CodexCapabilities()
		require.NotNil(t, caps)
		require.True(t, caps.SupportsCyberProgram("daybreak_red"))
		require.Equal(t, []string{"standard", "daybreak_red"}, caps.Models["gpt-6-sol"].CyberPrograms)
	})
	t.Run("missing or empty", func(t *testing.T) {
		require.Nil(t, (&Account{}).CodexCapabilities())
		require.Nil(t, (&Account{Extra: map[string]any{CodexCapabilitiesExtraKey: nil}}).CodexCapabilities())
		require.Nil(t, (&Account{Extra: map[string]any{CodexCapabilitiesExtraKey: map[string]any{"models": map[string]any{}}}}).CodexCapabilities())
		require.Nil(t, (&Account{Extra: map[string]any{CodexCapabilitiesExtraKey: "garbage"}}).CodexCapabilities())
		var nilCaps *CodexAccountCapabilities
		require.False(t, nilCaps.SupportsServiceTier("priority"))
	})
}
