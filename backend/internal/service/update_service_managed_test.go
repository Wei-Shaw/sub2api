//go:build unit

package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestManagedUpdateBlocksBinaryReplacementBeforeIO(t *testing.T) {
	// Nil dependencies prove the guard runs before release fetches/downloads.
	svc := NewUpdateService(nil, nil, "0.2.14-custom", "managed")
	require.ErrorIs(t, svc.PerformUpdate(context.Background()), ErrDeploymentManaged)
	require.ErrorIs(t, svc.Rollback(), ErrDeploymentManaged)
	require.ErrorIs(t, svc.RollbackToVersion(context.Background(), "0.2.13"), ErrDeploymentManaged)
	require.ErrorIs(t, svc.applyReleaseAssets(context.Background(), nil), ErrDeploymentManaged)
	_, err := svc.ListRollbackVersions(context.Background())
	require.ErrorIs(t, err, ErrDeploymentManaged)
}

func TestManagedUpdateCheckPreservesModeAndUpstreamVersion(t *testing.T) {
	cache := &updateServiceCacheStub{}
	github := &updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v0.2.15"}}
	svc := NewUpdateService(cache, github, "0.2.14-custom", "managed")
	for _, force := range []bool{true, false} {
		info, err := svc.CheckUpdate(context.Background(), force)
		require.NoError(t, err)
		require.Equal(t, "managed", info.BuildType)
		require.Equal(t, "0.2.15", info.LatestVersion)
		require.True(t, info.HasUpdate)
	}
}
