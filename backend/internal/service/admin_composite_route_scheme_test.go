//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminService_CreateCompositeRouteScheme_CopiesTemplateRoutes(t *testing.T) {
	routeRepo := &compositeRouteRepoStubForAdmin{
		nextID: 11,
		routes: []CompositeModelRoute{
			{
				ID:             1,
				SchemeID:       5,
				PublicModel:    "openrouter/gpt-5",
				MatchType:      CompositeRouteMatchExact,
				TargetPlatform: PlatformOpenAI,
				UpstreamModel:  "gpt-5",
				Endpoint:       CompositeRouteEndpointResponses,
				Priority:       10,
				Enabled:        true,
			},
		},
	}
	svc := &adminServiceImpl{compositeRouteRepo: routeRepo}

	scheme, err := svc.CreateCompositeRouteScheme(context.Background(), CompositeRouteSchemeInput{
		Name:             "  production  ",
		Description:      " from template ",
		CopyFromSchemeID: 5,
	})

	require.NoError(t, err)
	require.Equal(t, "production", scheme.Name)
	require.Equal(t, "from template", scheme.Description)
	require.NotNil(t, routeRepo.created)
	require.Equal(t, scheme.ID, routeRepo.created.SchemeID)
	require.Equal(t, "openrouter/gpt-5", routeRepo.created.PublicModel)
	require.Equal(t, PlatformOpenAI, routeRepo.created.TargetPlatform)
}

func TestAdminService_DeleteCompositeRouteScheme_RejectsInUse(t *testing.T) {
	routeRepo := &compositeRouteRepoStubForAdmin{nextID: 3}
	routeRepo.countGroups = 2
	svc := &adminServiceImpl{compositeRouteRepo: routeRepo}

	err := svc.DeleteCompositeRouteScheme(context.Background(), 3)
	require.ErrorIs(t, err, ErrCompositeRouteSchemeInUse)
}

func TestAdminService_CreateGroup_BindsCompositeRouteScheme(t *testing.T) {
	groupRepo := &groupRepoStubForAdmin{createID: 8}
	routeRepo := &compositeRouteRepoStubForAdmin{nextID: 4}
	svc := &adminServiceImpl{groupRepo: groupRepo, compositeRouteRepo: routeRepo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                   "composite-prod",
		Platform:               PlatformComposite,
		RateMultiplier:         1,
		CompositeRouteSchemeID: schemeIDPtr(4),
	})
	require.NoError(t, err)
	require.NotNil(t, group.CompositeRouteSchemeID)
	require.Equal(t, int64(4), *group.CompositeRouteSchemeID)
}

func schemeIDPtr(v int64) *int64 { return &v }
