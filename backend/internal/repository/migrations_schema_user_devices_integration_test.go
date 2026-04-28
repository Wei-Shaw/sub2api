//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigrationSchemaIncludesUserDevices(t *testing.T) {
	t.Parallel()

	var exists bool
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'user_devices'
		)
	`).Scan(&exists))
	require.True(t, exists, "user_devices migration should be applied")
}
