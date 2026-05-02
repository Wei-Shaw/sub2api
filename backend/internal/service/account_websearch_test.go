//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetWebSearchEmulationMode_Enabled(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyWebSearchEmulation: "enabled"},
	}
	require.Equal(t, WebSearchModeEnabled, a.GetWebSearchEmulationMode())
}

func TestGetWebSearchEmulationMode_Disabled(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyWebSearchEmulation: "disabled"},
	}
	require.Equal(t, WebSearchModeDisabled, a.GetWebSearchEmulationMode())
}

func TestGetWebSearchEmulationMode_Default(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyWebSearchEmulation: "default"},
	}
	require.Equal(t, WebSearchModeDefault, a.GetWebSearchEmulationMode())
}

func TestGetWebSearchEmulationMode_UnknownString(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyWebSearchEmulation: "unknown"},
	}
	require.Equal(t, WebSearchModeDefault, a.GetWebSearchEmulationMode())
}

func TestGetWebSearchEmulationMode_OldBoolTrue(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyWebSearchEmulation: true},
	}
	// bool true → tolerant fallback → enabled (not default)
	require.Equal(t, WebSearchModeEnabled, a.GetWebSearchEmulationMode())
}

func TestGetWebSearchEmulationMode_OldBoolFalse(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyWebSearchEmulation: false},
	}
	require.Equal(t, WebSearchModeDefault, a.GetWebSearchEmulationMode())
}

func TestGetWebSearchEmulationMode_NilAccount(t *testing.T) {
	var a *Account
	require.Equal(t, WebSearchModeDefault, a.GetWebSearchEmulationMode())
}

func TestGetWebSearchEmulationMode_NilExtra(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    nil,
	}
	require.Equal(t, WebSearchModeDefault, a.GetWebSearchEmulationMode())
}

func TestGetWebSearchEmulationMode_MissingField(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{},
	}
	require.Equal(t, WebSearchModeDefault, a.GetWebSearchEmulationMode())
}

func TestGetWebSearchEmulationMode_NonAnthropicPlatform(t *testing.T) {
	a := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyWebSearchEmulation: "enabled"},
	}
	require.Equal(t, WebSearchModeDefault, a.GetWebSearchEmulationMode())
}

func TestGetWebSearchEmulationMode_NonAPIKeyType(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{featureKeyWebSearchEmulation: "enabled"},
	}
	require.Equal(t, WebSearchModeDefault, a.GetWebSearchEmulationMode())
}
