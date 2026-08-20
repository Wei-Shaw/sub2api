package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPlatformDefaultModelIDs_IncludesCatalogOnlyModels(t *testing.T) {
	ids := PlatformDefaultModelIDs(PlatformAntigravity)
	require.Contains(t, ids, "gemini-3.7-flash")
	require.Contains(t, ids, "gemini-3.7-flash-high")

	gemini := PlatformDefaultModelIDs(PlatformGemini)
	require.Contains(t, gemini, "gemini-3.7-flash")
	require.Contains(t, gemini, "gemini-2.5-flash")
}

func TestApplyCatalogDefaultMappings_OnlyWhenRenameOnly(t *testing.T) {
	legacy := &Account{Platform: PlatformAntigravity}
	require.Nil(t, applyCatalogDefaultMappings(legacy, nil))

	renameOnly := &Account{
		Platform: PlatformAntigravity,
		Credentials: map[string]any{
			CredentialKeyModelMappingRestricts: false,
		},
	}
	got := applyCatalogDefaultMappings(renameOnly, map[string]string{"keep": "keep"})
	require.Equal(t, "keep", got["keep"])
	require.Equal(t, "gemini-pro-agent", got["gemini-3.1-pro-high"])
}

func TestApplyCatalogDefaultMappings_DoesNotMutateSharedDefaults(t *testing.T) {
	account := &Account{
		Platform: PlatformAntigravity,
		Credentials: map[string]any{
			CredentialKeyModelMappingRestricts: false,
		},
	}
	shared := map[string]string{"keep": "keep"}
	got := applyCatalogDefaultMappings(account, shared)
	require.Equal(t, "keep", shared["keep"])
	require.NotContains(t, shared, "gemini-3.1-pro-high")
	require.Equal(t, "gemini-pro-agent", got["gemini-3.1-pro-high"])
}
