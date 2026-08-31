//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCodexIdentityTemplateRepositoryUpdateLoadsProfilesAndSlotsOnPostgresTransaction(t *testing.T) {
	ctx := context.Background()
	repo := &codexIdentityTemplateRepository{db: integrationDB}
	template := &service.CodexIdentityTemplate{
		Name:               fmt.Sprintf("codex-template-update-%d", time.Now().UnixNano()),
		Description:        "initial",
		SessionPolicy:      service.CodexSessionPolicySpec{Mode: service.CodexSessionConversationIsolated},
		AffinityTTLSeconds: 3600,
		UnsupportedPolicy:  service.CodexUnsupportedProfileReject,
		Profiles: []service.CodexIdentityTemplateProfile{
			{
				OSClass: service.CodexOSWindows, CanonicalSurface: service.CodexSurfaceDesktop,
				Architecture: service.CodexArchX8664, ProxyMode: service.CodexProxyInherit,
				SlotCount: 1, CatalogVersion: 1,
				Slots: []service.CodexIdentityTemplateSlot{{Index: 0, ProxyMode: service.CodexProxyInherit}},
			},
			{
				OSClass: service.CodexOSWindows, CanonicalSurface: service.CodexSurfaceCLI,
				Architecture: service.CodexArchX8664, ProxyMode: service.CodexProxyInherit,
				SlotCount: 1, CatalogVersion: 1,
			},
		},
	}

	created, err := repo.CreateCodexIdentityTemplate(ctx, template)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM codex_identity_templates WHERE id=$1`, created.ID)
	})

	metadataUpdate := *created
	metadataUpdate.Description = "metadata-only"
	metadataUpdated, err := repo.UpdateCodexIdentityTemplate(ctx, &metadataUpdate, created.Revision, false)
	require.NoError(t, err)
	require.Equal(t, created.Revision, metadataUpdated.Revision)
	require.Len(t, metadataUpdated.Profiles, 2)
	var desktopProfile *service.CodexIdentityTemplateProfile
	for index := range metadataUpdated.Profiles {
		if metadataUpdated.Profiles[index].CanonicalSurface == service.CodexSurfaceDesktop {
			desktopProfile = &metadataUpdated.Profiles[index]
			break
		}
	}
	require.NotNil(t, desktopProfile)
	require.Len(t, desktopProfile.Slots, 1)

	runtimeUpdate := *metadataUpdated
	runtimeUpdate.Profiles = append([]service.CodexIdentityTemplateProfile(nil), metadataUpdated.Profiles...)
	runtimeUpdate.Profiles[0].SlotCount = 2
	runtimeUpdated, err := repo.UpdateCodexIdentityTemplate(ctx, &runtimeUpdate, metadataUpdated.Revision, false)
	require.NoError(t, err)
	require.Equal(t, metadataUpdated.Revision+1, runtimeUpdated.Revision)
	require.Len(t, runtimeUpdated.Profiles, 2)
}
