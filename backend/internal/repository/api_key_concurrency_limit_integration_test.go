//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyConcurrencyLimitMigration(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	requireColumn(t, tx, "api_keys", "concurrency_limit", "bigint", 0, false)
	requireConstraintDefinitionContains(t, tx, "api_keys", "api_keys_concurrency_limit_nonnegative", "concurrency_limit", ">= 0")
	var userID, keyID int64
	require.NoError(t, tx.QueryRowContext(ctx,
		`INSERT INTO users (email, password_hash) VALUES ('concurrency-migration@test.com', 'test') RETURNING id`).Scan(&userID))
	var limit int
	require.NoError(t, tx.QueryRowContext(ctx,
		`INSERT INTO api_keys (user_id, key, name) VALUES ($1, 'sk-concurrency-migration', 'test') RETURNING id, concurrency_limit`, userID).Scan(&keyID, &limit))
	require.Zero(t, limit, "database default applies without Ent")
	for _, want := range []int{9, 0} {
		require.NoError(t, tx.QueryRowContext(ctx,
			`UPDATE api_keys SET concurrency_limit = $1 WHERE id = $2 RETURNING concurrency_limit`, want, keyID).Scan(&limit))
		require.Equal(t, want, limit)
	}
	_, err := tx.ExecContext(ctx, `UPDATE api_keys SET concurrency_limit = -1 WHERE id = $1`, keyID)
	require.ErrorContains(t, err, "api_keys_concurrency_limit_nonnegative")
}
