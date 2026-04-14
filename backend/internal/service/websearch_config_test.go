package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// --- validateWebSearchConfig ---

func TestValidateWebSearchConfig_Nil(t *testing.T) {
	require.NoError(t, validateWebSearchConfig(nil))
}

func TestValidateWebSearchConfig_Valid(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Enabled: true,
		Providers: []WebSearchProviderConfig{
			{Type: "brave", QuotaLimit: int64Ptr(1000)},
			{Type: "tavily", QuotaLimit: int64Ptr(500)},
		},
	}
	require.NoError(t, validateWebSearchConfig(cfg))
}

func TestValidateWebSearchConfig_TooManyProviders(t *testing.T) {
	cfg := &WebSearchEmulationConfig{Providers: make([]WebSearchProviderConfig, 11)}
	for i := range cfg.Providers {
		cfg.Providers[i] = WebSearchProviderConfig{Type: "brave"}
	}
	err := validateWebSearchConfig(cfg)
	require.ErrorContains(t, err, "too many providers")
}

func TestValidateWebSearchConfig_InvalidType(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{{Type: "bing"}},
	}
	require.ErrorContains(t, validateWebSearchConfig(cfg), "invalid type")
}

func TestValidateWebSearchConfig_NegativeQuotaLimit(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{{Type: "brave", QuotaLimit: int64Ptr(-1)}},
	}
	require.ErrorContains(t, validateWebSearchConfig(cfg), "quota_limit must be > 0 or null")
}

func TestValidateWebSearchConfig_DuplicateType(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{
			{Type: "brave"},
			{Type: "brave"},
		},
	}
	require.ErrorContains(t, validateWebSearchConfig(cfg), "duplicate type")
}

func TestValidateWebSearchConfig_NilQuotaLimit(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{{Type: "brave", QuotaLimit: nil}},
	}
	require.NoError(t, validateWebSearchConfig(cfg))
}

// --- parseWebSearchConfigJSON ---

func TestParseWebSearchConfigJSON_ValidJSON(t *testing.T) {
	raw := `{"enabled":true,"providers":[{"type":"brave","api_key":"sk-xxx"}]}`
	cfg := parseWebSearchConfigJSON(raw)
	require.True(t, cfg.Enabled)
	require.Len(t, cfg.Providers, 1)
	require.Equal(t, "brave", cfg.Providers[0].Type)
}

func TestParseWebSearchConfigJSON_EmptyString(t *testing.T) {
	cfg := parseWebSearchConfigJSON("")
	require.False(t, cfg.Enabled)
	require.Empty(t, cfg.Providers)
}

func TestParseWebSearchConfigJSON_InvalidJSON(t *testing.T) {
	cfg := parseWebSearchConfigJSON("not{json")
	require.False(t, cfg.Enabled)
	require.Empty(t, cfg.Providers)
}

func TestParseWebSearchConfigJSON_BackwardCompatibility(t *testing.T) {
	// Old config with priority and quota_refresh_interval should parse without error
	raw := `{"enabled":true,"providers":[{"type":"brave","priority":1,"quota_refresh_interval":"monthly","quota_limit":1000}]}`
	cfg := parseWebSearchConfigJSON(raw)
	require.True(t, cfg.Enabled)
	require.Len(t, cfg.Providers, 1)
	require.Equal(t, int64(1000), *cfg.Providers[0].QuotaLimit)
}

// --- SanitizeWebSearchConfig ---

func TestSanitizeWebSearchConfig_MaskAPIKey(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Enabled: true,
		Providers: []WebSearchProviderConfig{
			{Type: "brave", APIKey: "sk-secret-xxx"},
		},
	}
	out := SanitizeWebSearchConfig(context.Background(), cfg)
	require.Equal(t, "", out.Providers[0].APIKey)
	require.True(t, out.Providers[0].APIKeyConfigured)
}

func TestSanitizeWebSearchConfig_NoAPIKey(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: ""}},
	}
	out := SanitizeWebSearchConfig(context.Background(), cfg)
	require.Equal(t, "", out.Providers[0].APIKey)
	require.False(t, out.Providers[0].APIKeyConfigured)
}

func TestSanitizeWebSearchConfig_Nil(t *testing.T) {
	require.Nil(t, SanitizeWebSearchConfig(context.Background(), nil))
}

func TestSanitizeWebSearchConfig_PreservesOtherFields(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Enabled: true,
		Providers: []WebSearchProviderConfig{
			{Type: "brave", APIKey: "secret", QuotaLimit: int64Ptr(1000)},
		},
	}
	out := SanitizeWebSearchConfig(context.Background(), cfg)
	require.True(t, out.Enabled)
	require.Equal(t, int64(1000), *out.Providers[0].QuotaLimit)
}

func TestSanitizeWebSearchConfig_DoesNotMutateOriginal(t *testing.T) {
	cfg := &WebSearchEmulationConfig{
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "secret"}},
	}
	_ = SanitizeWebSearchConfig(context.Background(), cfg)
	require.Equal(t, "secret", cfg.Providers[0].APIKey)
}
