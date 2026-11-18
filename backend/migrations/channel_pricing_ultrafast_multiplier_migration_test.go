package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelPricingUltrafastMultiplierMigration(t *testing.T) {
	content, err := FS.ReadFile("252_channel_pricing_ultrafast_multiplier.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ALTER TABLE channel_model_pricing ADD COLUMN IF NOT EXISTS ultrafast_multiplier NUMERIC(12,6);")
	require.Contains(t, sql, "conname = 'channel_model_pricing_ultrafast_multiplier_positive' AND conrelid = 'channel_model_pricing'::regclass")
	require.Contains(t, sql, "ALTER TABLE channel_model_pricing ADD CONSTRAINT channel_model_pricing_ultrafast_multiplier_positive"+
		" CHECK (ultrafast_multiplier IS NULL OR ultrafast_multiplier > 0)")
	// Existing rows must stay NULL so they keep the model-owned Ultrafast multiplier.
	require.NotContains(t, sql, "DEFAULT")
	require.NotContains(t, sql, "UPDATE channel_model_pricing")
}
