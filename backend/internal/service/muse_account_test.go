//go:build unit

package service

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMuseAccountCreationCannotEnableUnverifiedTraffic(t *testing.T) {
	input := &CreateAccountInput{Name: "Muse", Platform: PlatformMuse, Type: AccountTypeSession, Credentials: map[string]any{"muse_session": map[string]any{"opaque": "fixture"}}, Extra: map[string]any{MuseOwnerUserIDKey: float64(1)}, Concurrency: 20}
	a, err := buildAccountForCreate(input, input.Extra)
	require.NoError(t, err)
	require.False(t, a.Schedulable)
	require.Equal(t, 1, a.Concurrency)
	require.True(t, a.IsOpenAICompatible())
	require.False(t, a.IsOAuth())
	require.Empty(t, a.GetOpenAIBaseURL())
	input.Type = AccountTypeAPIKey
	_, err = buildAccountForCreate(input, input.Extra)
	require.Error(t, err)
}

func TestMuseSessionTypeCannotBeAppliedToOtherProviders(t *testing.T) {
	require.Error(t, ValidateMuseAccount(PlatformOpenAI, AccountTypeSession, map[string]any{}, nil))
	require.NoError(t, ValidateMuseAccount(PlatformOpenAI, AccountTypeAPIKey, map[string]any{}, nil))
	require.Error(t, ValidateMuseAccount(PlatformMuse, AccountTypeSession, map[string]any{"muse_session": map[string]any{"opaque": "fixture"}}, map[string]any{MuseOwnerUserIDKey: float64(1.5)}))
}

func TestMuseNamespaceDoesNotHijackOpenCodeModels(t *testing.T) {
	platform, ok := DetectModelPlatform("muse/assistant")
	require.True(t, ok)
	require.Equal(t, PlatformMuse, platform)
	_, ok = DetectModelPlatform("muse-spark-1.3-contributor")
	require.False(t, ok)
	require.Equal(t, PlatformMuse, NormalizeOpenAICompatiblePlatform(PlatformMuse))
}
