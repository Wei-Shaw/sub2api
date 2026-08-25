package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOnlyKnownLegacyMigrationsUseGooseSections(t *testing.T) {
	allowed := map[string]struct{}{
		"019_migrate_wechat_to_attributes.sql": {},
		"024_add_gemini_tier_id.sql":           {},
		"037_ops_alert_silences.sql":           {},
	}

	files, err := fs.Glob(FS, "*.sql")
	require.NoError(t, err)
	for _, name := range files {
		content, readErr := FS.ReadFile(name)
		require.NoError(t, readErr)
		if !strings.Contains(string(content), "-- +goose") {
			continue
		}
		_, ok := allowed[name]
		require.Truef(t, ok, "%s uses unsupported Goose sections; add a forward-only migration instead", name)
	}
}

func TestLegacyGooseRepairMigrationCoversAffectedState(t *testing.T) {
	content, err := FS.ReadFile("192_repair_legacy_goose_migration_outcomes.sql")
	require.NoError(t, err)
	sql := string(content)
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS ops_alert_silences")
	require.Contains(t, sql, "credentials->>'tier_id' IS NULL")
	require.Contains(t, sql, "INSERT INTO user_attribute_values")
	require.Contains(t, sql, "ALTER TABLE users DROP COLUMN wechat")
}
