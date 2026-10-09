//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRelayAdminCanMaintainPrivateGroupWithoutTurningItIntoPaidGroup(t *testing.T) {
	for _, tc := range []struct {
		name      string
		rate      float64
		wantError bool
	}{
		{"free name edit", 0, false}, {"paid private group", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &groupRepoStubForAdmin{getByID: &Group{ID: 1, OwnerUserID: relayTestID(7), Name: "private", Platform: PlatformOpenAI,
				Status: StatusActive, IsExclusive: true, SubscriptionType: SubscriptionTypeStandard}}
			s := &adminServiceImpl{groupRepo: repo}
			_, err := s.UpdateGroup(context.Background(), 1, &UpdateGroupInput{Name: "renamed private", RateMultiplier: &tc.rate})
			if tc.wantError {
				require.Error(t, err)
				require.Nil(t, repo.updated)
			} else {
				require.NoError(t, err)
				require.Zero(t, repo.updated.RateMultiplier)
			}
		})
	}
}
