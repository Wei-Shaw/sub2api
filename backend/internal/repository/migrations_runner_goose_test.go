package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigrationExecutableContent(t *testing.T) {
	t.Run("plain migration is unchanged", func(t *testing.T) {
		const sql = "CREATE TABLE example(id bigint);"
		got, err := migrationExecutableContent(sql)
		require.NoError(t, err)
		require.Equal(t, sql, got)
	})

	t.Run("legacy Goose migration executes only Up", func(t *testing.T) {
		const sql = `-- migration description
-- +goose Up
-- +goose StatementBegin
CREATE TABLE example(id bigint);
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP TABLE example;
-- +goose StatementEnd`

		got, err := migrationExecutableContent(sql)
		require.NoError(t, err)
		require.Contains(t, got, "CREATE TABLE example")
		require.NotContains(t, got, "DROP TABLE example")
		require.NotContains(t, got, "+goose")
	})

	t.Run("rejects malformed markers", func(t *testing.T) {
		_, err := migrationExecutableContent("-- +goose Down\nDROP TABLE example;")
		require.ErrorContains(t, err, "Down section")

		_, err = migrationExecutableContent("-- +goose Up\n-- +goose StatementEnd")
		require.ErrorContains(t, err, "empty")
	})
}
