package migrations

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDisplayPricingMigrationIsIsolatedAndSeedsExactCatalog(t *testing.T) {
	content, err := FS.ReadFile("232_display_pricing_catalog.sql")
	require.NoError(t, err)
	sql := string(content)
	normalized := strings.Join(strings.Fields(sql), " ")

	require.Contains(t, normalized, "CREATE TABLE IF NOT EXISTS display_pricing_settings")
	require.Contains(t, normalized, "CREATE TABLE IF NOT EXISTS display_pricing_providers")
	require.Contains(t, normalized, "CREATE TABLE IF NOT EXISTS display_model_prices")
	require.Contains(t, normalized, "global_multiplier NUMERIC(12, 6) NOT NULL DEFAULT 1.000000")
	require.Contains(t, normalized, "logo_key VARCHAR(64) NOT NULL DEFAULT ''")
	require.Contains(t, normalized, "logo_url TEXT NOT NULL DEFAULT ''")
	require.Contains(t, normalized, "ON UPDATE CASCADE ON DELETE CASCADE")
	require.NotContains(t, strings.ToLower(sql), "update groups")
	require.NotContains(t, strings.ToLower(sql), "update channels")
	require.NotContains(t, strings.ToLower(sql), "channel_model_pricing")

	seedRow := regexp.MustCompile(`\('[^']+',\s*'[^']+',\s*'[^']+',\s*'(?:token|per_request|image)'`)
	require.Len(t, seedRow.FindAllString(sql, -1), 48)

	for _, model := range []string{
		"deepseek-v4-flash-vision-exp", "kimi-k3", "qwen3.7-max", "claude-fable-5",
		"gpt-5.6-luna", "gemini-3.1-pro-preview", "grok-4.6", "gpt-image-2",
	} {
		require.Contains(t, sql, "'"+model+"'")
	}
	for _, model := range []string{"qwen3.8-coder", "qwen3.8-plus", "gpt-5.4", "grok-4.5-fast"} {
		require.NotContains(t, sql, "'"+model+"'")
	}
	require.Contains(t, normalized, "per_request_lte_256k")
	require.Contains(t, normalized, "per_request_256k_512k_override")
	require.Contains(t, normalized, "per_request_gt_512k_override")
	require.Contains(t, normalized, "'deepseek-v4-flash-vision-exp', 'deepseek', 'token', 'CNY', 1.6, 4.7, NULL, 0.1, 0.46875")
}

func TestDisplayPricingNotesMigrationIsPresentationOnly(t *testing.T) {
	content, err := FS.ReadFile("233_display_pricing_notes.sql")
	require.NoError(t, err)
	sql := string(content)
	normalized := strings.Join(strings.Fields(sql), " ")

	require.Contains(t, normalized, "ADD COLUMN IF NOT EXISTS provider_note VARCHAR(4000) NOT NULL DEFAULT ''")
	require.Contains(t, normalized, "ADD COLUMN IF NOT EXISTS model_note VARCHAR(1000) NOT NULL DEFAULT ''")
	require.Contains(t, sql, "DeepSeek 平常价格展示")
	require.Contains(t, sql, "deepseek-v4-flash-vision-exp")
	require.NotContains(t, strings.ToLower(sql), "update groups")
	require.NotContains(t, strings.ToLower(sql), "update channels")
}

func TestDisplayPricingOpenAICompatiblePlatformMigrationIsPresentationOnly(t *testing.T) {
	content, err := FS.ReadFile("234_display_pricing_openai_compatible_platform.sql")
	require.NoError(t, err)
	sql := string(content)
	normalized := strings.Join(strings.Fields(sql), " ")

	require.Contains(t, normalized, "UPDATE display_model_prices AS target")
	require.Contains(t, normalized, "SET platform = 'openai'")
	require.Contains(t, normalized, "target.provider IN ('anthropic', 'gemini', 'grok')")
	require.Contains(t, normalized, "existing.model_name = target.model_name")
	require.Contains(t, normalized, "existing.billing_mode = target.billing_mode")
	require.NotContains(t, strings.ToLower(sql), "update groups")
	require.NotContains(t, strings.ToLower(sql), "update channels")
	require.NotContains(t, strings.ToLower(sql), "channel_model_pricing")
}

func TestDisplayPricingProviderModeNotesMigrationIsPresentationOnly(t *testing.T) {
	content, err := FS.ReadFile("237_display_pricing_provider_mode_notes.sql")
	require.NoError(t, err)
	sql := string(content)
	normalized := strings.Join(strings.Fields(sql), " ")

	require.Contains(t, normalized, "ALTER TABLE display_pricing_providers")
	require.Contains(t, normalized, "ADD COLUMN IF NOT EXISTS per_request_note VARCHAR(4000) NOT NULL DEFAULT ''")
	require.Contains(t, normalized, "ADD COLUMN IF NOT EXISTS image_note VARCHAR(4000) NOT NULL DEFAULT ''")
	require.NotContains(t, strings.ToLower(sql), "update groups")
	require.NotContains(t, strings.ToLower(sql), "update channels")
	require.NotContains(t, strings.ToLower(sql), "channel_model_pricing")
}

func TestDisplayOfficialPriceSourceMigrationIsPresentationOnly(t *testing.T) {
	content, err := FS.ReadFile("238_display_official_price_source.sql")
	require.NoError(t, err)
	sql := string(content)
	normalized := strings.Join(strings.Fields(sql), " ")

	require.Contains(t, normalized, "ALTER TABLE display_model_prices")
	require.Contains(t, normalized, "official_price_source VARCHAR(64) NOT NULL DEFAULT 'manual'")
	require.Contains(t, normalized, "official_price_source_url TEXT NOT NULL DEFAULT ''")
	require.Contains(t, normalized, "official_price_synced_at TIMESTAMPTZ")
	require.NotContains(t, strings.ToLower(sql), "update groups")
	require.NotContains(t, strings.ToLower(sql), "update channels")
	require.NotContains(t, strings.ToLower(sql), "channel_model_pricing")
}
