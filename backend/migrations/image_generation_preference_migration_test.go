package migrations

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestImageGenerationPreferenceMigrationAddsNullableUserAPIKeyID(t *testing.T) {
	contents, err := FS.ReadFile("170_add_user_last_image_api_key_id.sql")
	require.NoError(t, err)
	migration := strings.ToLower(string(contents))
	require.Contains(t, migration, "alter table users")
	require.Contains(t, migration, "add column if not exists last_image_api_key_id bigint")
	require.NotContains(t, migration, "not null")
}

func TestImageGenerationPreferenceMigrationPreservesExistingUsersAndAllowsSelection(t *testing.T) {
	db, err := sql.Open("sqlite", "file:image_generation_preference_migration?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO users (id, email) VALUES (1, 'existing@example.com')`)
	require.NoError(t, err)

	require.NoError(t, applyImageGenerationPreferenceMigration(db))

	var existingPreference sql.NullInt64
	require.NoError(t, db.QueryRow(`SELECT last_image_api_key_id FROM users WHERE id = 1`).Scan(&existingPreference))
	require.False(t, existingPreference.Valid)

	_, err = db.Exec(`UPDATE users SET last_image_api_key_id = 42 WHERE id = 1`)
	require.NoError(t, err)
	require.NoError(t, db.QueryRow(`SELECT last_image_api_key_id FROM users WHERE id = 1`).Scan(&existingPreference))
	require.True(t, existingPreference.Valid)
	require.Equal(t, int64(42), existingPreference.Int64)
}

func applyImageGenerationPreferenceMigration(db *sql.DB) error {
	contents, err := FS.ReadFile("170_add_user_last_image_api_key_id.sql")
	if err != nil {
		return err
	}
	migration := strings.Replace(
		string(contents),
		"ADD COLUMN IF NOT EXISTS last_image_api_key_id BIGINT",
		"ADD COLUMN last_image_api_key_id BIGINT",
		1,
	)
	_, err = db.Exec(migration)
	return err
}
