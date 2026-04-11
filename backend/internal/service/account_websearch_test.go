package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccount_IsWebSearchEmulationEnabled_Enabled(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyWebSearchEmulation: true},
	}
	require.True(t, a.IsWebSearchEmulationEnabled())
}

func TestAccount_IsWebSearchEmulationEnabled_Disabled(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyWebSearchEmulation: false},
	}
	require.False(t, a.IsWebSearchEmulationEnabled())
}

func TestAccount_IsWebSearchEmulationEnabled_MissingField(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{},
	}
	require.False(t, a.IsWebSearchEmulationEnabled())
}

func TestAccount_IsWebSearchEmulationEnabled_WrongType(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyWebSearchEmulation: "true"},
	}
	require.False(t, a.IsWebSearchEmulationEnabled())
}

func TestAccount_IsWebSearchEmulationEnabled_NilExtra(t *testing.T) {
	a := &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Extra: nil}
	require.False(t, a.IsWebSearchEmulationEnabled())
}

func TestAccount_IsWebSearchEmulationEnabled_NilAccount(t *testing.T) {
	var a *Account
	require.False(t, a.IsWebSearchEmulationEnabled())
}

func TestAccount_IsWebSearchEmulationEnabled_NonAnthropicPlatform(t *testing.T) {
	a := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyWebSearchEmulation: true},
	}
	require.False(t, a.IsWebSearchEmulationEnabled())
}

func TestAccount_IsWebSearchEmulationEnabled_NonAPIKeyType(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{featureKeyWebSearchEmulation: true},
	}
	require.False(t, a.IsWebSearchEmulationEnabled())
}
