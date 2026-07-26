//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type schedulablePlatformsRepo struct {
	*mockAccountRepoForPlatform
	all      []Account
	byGroup  map[int64][]Account
	listErr  error
	groupErr error
}

func (r *schedulablePlatformsRepo) ListSchedulable(context.Context) ([]Account, error) {
	return r.all, r.listErr
}

func (r *schedulablePlatformsRepo) ListSchedulableByGroupID(_ context.Context, groupID int64) ([]Account, error) {
	return r.byGroup[groupID], r.groupErr
}

func TestGatewayServiceGetSchedulablePlatforms(t *testing.T) {
	repo := &schedulablePlatformsRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{},
		all: []Account{
			{Platform: " openai "},
			{Platform: PlatformAnthropic},
			{Platform: "openai"},
			{Platform: ""},
		},
		byGroup: map[int64][]Account{
			73: {{Platform: PlatformGemini}, {Platform: "  "}},
		},
	}
	svc := &GatewayService{accountRepo: repo}

	require.Equal(t, map[string]struct{}{
		"openai":          {},
		PlatformAnthropic: {},
	}, svc.GetSchedulablePlatforms(context.Background(), nil))

	groupID := int64(73)
	require.Equal(t, map[string]struct{}{PlatformGemini: {}}, svc.GetSchedulablePlatforms(context.Background(), &groupID))

	repo.listErr = errors.New("list failed")
	require.Empty(t, svc.GetSchedulablePlatforms(context.Background(), nil))
	require.Empty(t, (*GatewayService)(nil).GetSchedulablePlatforms(context.Background(), nil))
}
