//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigrationsRunner_AcceptsPreReleaseMainChecksumsWithoutReplaying(t *testing.T) {
	ctx := context.Background()
	preReleaseMainChecksums := map[string]string{
		"217_group_video_model_prices.sql":                          "e335f1b68ed1349661fab51bf4669619b7b116df31c1fb974c844b1c8a2f84d3",
		"220_clear_non_grok_video_generation_config.sql":            "cf4dbfa75ac27d93a30a6a14439fe7dccfc911c043358363d5ec47946aa0e28b",
		"221_channel_monitor_v2.sql":                                "efe5668b284b3e0e7a27a6ebb138fb03aeeeb3593117232733b1f4cc13c9e2c1",
		"223_channel_monitor_v2_ignored_error_categories.sql":       "75520645fc43e15b8d48bb28ce49c0e1633414db4267cba99ed1e4b2b334b7c4",
		"224_channel_monitor_v2_seed_popular_models.sql":            "23d0489c1b421bc6d7c91bbfcb7006eb49a0d107f1e5ee441cc6502f8b280cbb",
		"225_channel_monitor_v2_health_thresholds.sql":              "3e996b6f92520e657905964c9e9fc7f2a370e00ebcf1d43c9ad5df0412fb4e71",
		"226_channel_monitor_v2_fixed_rollups.sql":                  "6b11bb6342c385b3ecb27ab42d2ddf3d7eeda0d36d0dd804c66b33fb6310c7b5",
		"227_channel_monitor_v2_rollup_permissions.sql":             "86d16854b17a096c8e1dbd7e4897b30aabcd74adaee1390d7e7c5b0c4cea8e84",
		"228_channel_monitor_v2_refresh_5m.sql":                     "d5f3cf24c2ee94eb872cc9fa6508929886bcf779f5b0d7ba71ef450c58699601",
		"229_channel_monitor_v2_full_table_permissions.sql":         "7d944e39abd71fee4845dac933536f3f891fcfd7fb926e1187b4e8a49b6edc3e",
		"230_channel_monitor_v2_default_ignore_and_cache.sql":       "ae54ca5d660438136f559cf2b65dae15399281de195a19159d03ec596de4bb98",
		"232_channel_monitor_v2_reset_factory_cache_thresholds.sql": "9cc91f869c03ba5cc46696575959cbd1b74ba0e9ad380bcbc3a422259eddff80",
		"233_channel_monitor_v2_privacy_defaults.sql":               "e2bdbcafac7f07aa9eebc804dde013e3eef506a6b3a323d8548b01b8796905cd",
	}

	t.Cleanup(func() {
		for filename := range preReleaseMainChecksums {
			rule := migrationChecksumCompatibilityRules[filename]
			_, _ = integrationDB.ExecContext(context.Background(), `
UPDATE schema_migrations
SET checksum = $2
WHERE filename = $1
`, filename, rule.fileChecksum)
		}
	})

	for filename, checksum := range preReleaseMainChecksums {
		result, err := integrationDB.ExecContext(ctx, `
UPDATE schema_migrations
SET checksum = $2
WHERE filename = $1
`, filename, checksum)
		require.NoError(t, err)
		rowsAffected, err := result.RowsAffected()
		require.NoError(t, err)
		require.Equal(t, int64(1), rowsAffected, "expected applied migration record for %s", filename)
	}

	// The explicit CREATE TABLE statements in 221 and 226 would fail if an
	// already-applied migration were replayed instead of checksum-skipped.
	require.NoError(t, ApplyMigrations(ctx, integrationDB))

	for _, index := range []string{
		"idx_channel_monitor_v2_metrics_platform_time",
		"idx_channel_monitor_v2_metrics_group_time",
		"idx_channel_monitor_v2_metrics_model_time",
		"idx_channel_monitor_v2_user_metrics_user_time",
		"idx_channel_monitor_v2_user_metrics_time",
		"idx_channel_monitor_v2_errors_time",
		"idx_channel_monitor_v2_errors_category_time",
		"idx_channel_monitor_v2_histograms_time",
		"idx_channel_monitor_v2_metrics_rollup_platform_time",
		"idx_channel_monitor_v2_metrics_rollup_group_time",
		"idx_channel_monitor_v2_metrics_rollup_model_time",
		"idx_channel_monitor_v2_user_rollup_user_time",
		"idx_channel_monitor_v2_user_rollup_time",
		"idx_channel_monitor_v2_errors_rollup_time",
		"idx_channel_monitor_v2_errors_rollup_category_time",
		"idx_channel_monitor_v2_histograms_rollup_time",
	} {
		var count int
		require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM pg_class
WHERE relkind = 'i' AND relname = $1
`, index).Scan(&count))
		require.Equal(t, 1, count, "expected exactly one index named %s", index)
	}
}
