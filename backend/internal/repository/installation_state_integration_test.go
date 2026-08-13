//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBootstrapCompletionStateControlsResidentAdmission(t *testing.T) {
	ctx := context.Background()
	_, err := integrationDB.ExecContext(ctx, "DELETE FROM installation_state WHERE id = 1")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM installation_state WHERE id = 1")
	})

	require.ErrorIs(t, RequireBootstrapComplete(ctx, integrationDB), ErrBootstrapIncomplete)

	_, err = integrationDB.ExecContext(ctx, "INSERT INTO installation_state (id) VALUES (1)")
	require.NoError(t, err)

	require.NoError(t, RequireBootstrapComplete(ctx, integrationDB))
}
