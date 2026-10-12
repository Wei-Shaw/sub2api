//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCustomImageRejectsInPlaceUpdateAndRollbackBeforeIO(t *testing.T) {
	// Nil dependencies prove these paths cannot download, access backups or replace binaries.
	s := NewUpdateService(nil, nil, "0.2.5", "custom")
	require.ErrorIs(t, s.PerformUpdate(context.Background()), ErrCustomImageManaged)
	require.ErrorIs(t, s.Rollback(), ErrCustomImageManaged)
	require.ErrorIs(t, s.RollbackToVersion(context.Background(), "0.2.4"), ErrCustomImageManaged)
	versions, err := s.ListRollbackVersions(context.Background())
	require.NoError(t, err)
	require.Empty(t, versions)
}
