//go:build integration

package repository

import (
	"context"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestWeb3IdentitySoftDeleteMigrationBackfillsDeletedUsers(t *testing.T) {
	ctx := context.Background()
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })

	_, err = tx.ExecContext(ctx, `
		CREATE TEMP TABLE users (
			id BIGINT PRIMARY KEY,
			deleted_at TIMESTAMPTZ
		) ON COMMIT DROP;
		CREATE TEMP TABLE web3_identities (
			user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			address VARCHAR(42) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT web3_identities_address_key UNIQUE (address)
		) ON COMMIT DROP;
		INSERT INTO users (id, deleted_at) VALUES (1, NOW());
		INSERT INTO web3_identities (user_id, address)
		VALUES (1, '0x1111111111111111111111111111111111111111');
	`)
	require.NoError(t, err)

	migrationSQL, err := dbmigrations.FS.ReadFile("195_web3_identity_soft_delete.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)

	var deleted bool
	require.NoError(t, tx.QueryRowContext(ctx, `
		SELECT deleted_at IS NOT NULL
		FROM web3_identities
		WHERE user_id = 1
	`).Scan(&deleted))
	require.True(t, deleted)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO users (id, deleted_at) VALUES (2, NULL);
		INSERT INTO web3_identities (user_id, address)
		VALUES (2, '0x1111111111111111111111111111111111111111');
	`)
	require.NoError(t, err)

	var totalCount, activeCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE deleted_at IS NULL)
		FROM web3_identities
		WHERE address = '0x1111111111111111111111111111111111111111'
	`).Scan(&totalCount, &activeCount))
	require.Equal(t, 2, totalCount)
	require.Equal(t, 1, activeCount)
}
