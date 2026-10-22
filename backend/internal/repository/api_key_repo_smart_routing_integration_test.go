//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyRepositorySmartRoutingPersistenceAndGroupIndex(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newAPIKeyRepositoryWithSQL(client, tx)

	user, err := client.User.Create().
		SetEmail(uniqueTestValue(t, "smart-routing-user") + "@example.com").
		SetPasswordHash("test-password-hash").
		SetStatus(service.StatusActive).
		SetRole(service.RoleUser).
		Save(ctx)
	require.NoError(t, err)
	first, err := client.Group.Create().SetName(uniqueTestValue(t, "smart-routing-first")).SetStatus(service.StatusActive).Save(ctx)
	require.NoError(t, err)
	second, err := client.Group.Create().SetName(uniqueTestValue(t, "smart-routing-second")).SetStatus(service.StatusActive).Save(ctx)
	require.NoError(t, err)

	key := &service.APIKey{
		UserID:          user.ID,
		Key:             uniqueTestValue(t, "sk-smart-routing"),
		Name:            "smart routing",
		GroupID:         &first.ID,
		RoutingGroupIDs: []int64{first.ID, second.ID},
		Status:          service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, key))

	got, err := repo.GetByKeyForAuth(ctx, key.Key)
	require.NoError(t, err)
	require.Equal(t, []int64{first.ID, second.ID}, got.RoutingGroupIDs)

	keys, page, err := repo.ListByGroupID(ctx, second.ID, pagination.PaginationParams{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, int64(1), page.Total)
	require.Len(t, keys, 1)
	require.Equal(t, key.ID, keys[0].ID)

	count, err := repo.CountByGroupID(ctx, second.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	groupKeys, err := repo.ListKeysByGroupID(ctx, second.ID)
	require.NoError(t, err)
	require.Equal(t, []string{key.Key}, groupKeys)

	key.RoutingGroupIDs = []int64{first.ID}
	require.NoError(t, repo.Update(ctx, key, service.APIKeyUpdateFields{RoutingGroupIDs: true}))
	got, err = repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.Equal(t, []int64{first.ID}, got.RoutingGroupIDs)

	// Removing a secondary group must leave the primary binding intact while
	// removing the deleted group from the ordered route list.
	key.RoutingGroupIDs = []int64{first.ID, second.ID}
	require.NoError(t, repo.Update(ctx, key, service.APIKeyUpdateFields{RoutingGroupIDs: true}))
	removed, err := repo.ClearGroupIDByGroupID(ctx, second.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), removed)
	got, err = repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.Equal(t, []int64{first.ID}, got.RoutingGroupIDs)
	require.NotNil(t, got.GroupID)
	require.Equal(t, first.ID, *got.GroupID)
}

func TestAPIKeyRepositorySmartRoutingGroupMigrationRewritesPrimaryRoute(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newAPIKeyRepositoryWithSQL(client, tx)

	user, err := client.User.Create().
		SetEmail(uniqueTestValue(t, "smart-routing-migrate-user") + "@example.com").
		SetPasswordHash("test-password-hash").
		SetStatus(service.StatusActive).
		SetRole(service.RoleUser).
		Save(ctx)
	require.NoError(t, err)
	oldGroup, err := client.Group.Create().SetName(uniqueTestValue(t, "smart-routing-old")).SetStatus(service.StatusActive).Save(ctx)
	require.NoError(t, err)
	newGroup, err := client.Group.Create().SetName(uniqueTestValue(t, "smart-routing-new")).SetStatus(service.StatusActive).Save(ctx)
	require.NoError(t, err)
	otherGroup, err := client.Group.Create().SetName(uniqueTestValue(t, "smart-routing-other")).SetStatus(service.StatusActive).Save(ctx)
	require.NoError(t, err)

	key := &service.APIKey{
		UserID:          user.ID,
		Key:             uniqueTestValue(t, "sk-smart-routing-migrate"),
		Name:            "smart routing migrate",
		GroupID:         &oldGroup.ID,
		RoutingGroupIDs: []int64{oldGroup.ID, otherGroup.ID},
		Status:          service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, key))
	secondaryKey := &service.APIKey{
		UserID:          user.ID,
		Key:             uniqueTestValue(t, "sk-smart-routing-secondary-migrate"),
		Name:            "smart routing secondary migrate",
		GroupID:         &otherGroup.ID,
		RoutingGroupIDs: []int64{otherGroup.ID, oldGroup.ID, newGroup.ID, oldGroup.ID},
		Status:          service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, secondaryKey))

	changed, err := repo.UpdateGroupIDByUserAndGroup(ctx, user.ID, oldGroup.ID, newGroup.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), changed)
	got, err := repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.Equal(t, newGroup.ID, *got.GroupID)
	require.Equal(t, []int64{newGroup.ID, otherGroup.ID}, got.RoutingGroupIDs)
	got, err = repo.GetByID(ctx, secondaryKey.ID)
	require.NoError(t, err)
	require.Equal(t, otherGroup.ID, *got.GroupID)
	require.Equal(t, []int64{otherGroup.ID, newGroup.ID}, got.RoutingGroupIDs)
}
