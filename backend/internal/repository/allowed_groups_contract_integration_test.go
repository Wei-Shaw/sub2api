//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func uniqueTestValue(t *testing.T, prefix string) string {
	t.Helper()
	safeName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	return fmt.Sprintf("%s-%s", prefix, safeName)
}

func TestUserRepository_RemoveGroupFromAllowedGroups_RemovesAllOccurrences(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	entClient := tx.Client()

	targetGroup, err := entClient.Group.Create().
		SetName(uniqueTestValue(t, "target-group")).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	otherGroup, err := entClient.Group.Create().
		SetName(uniqueTestValue(t, "other-group")).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	repo := newUserRepositoryWithSQL(entClient, tx)

	u1 := &service.User{
		Email:         uniqueTestValue(t, "u1") + "@example.com",
		PasswordHash:  "test-password-hash",
		Role:          service.RoleUser,
		Status:        service.StatusActive,
		Concurrency:   5,
		AllowedGroups: []int64{targetGroup.ID, otherGroup.ID},
	}
	require.NoError(t, repo.Create(ctx, u1))

	u2 := &service.User{
		Email:         uniqueTestValue(t, "u2") + "@example.com",
		PasswordHash:  "test-password-hash",
		Role:          service.RoleUser,
		Status:        service.StatusActive,
		Concurrency:   5,
		AllowedGroups: []int64{targetGroup.ID},
	}
	require.NoError(t, repo.Create(ctx, u2))

	u3 := &service.User{
		Email:         uniqueTestValue(t, "u3") + "@example.com",
		PasswordHash:  "test-password-hash",
		Role:          service.RoleUser,
		Status:        service.StatusActive,
		Concurrency:   5,
		AllowedGroups: []int64{otherGroup.ID},
	}
	require.NoError(t, repo.Create(ctx, u3))

	affected, err := repo.RemoveGroupFromAllowedGroups(ctx, targetGroup.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), affected)

	u1After, err := repo.GetByID(ctx, u1.ID)
	require.NoError(t, err)
	require.NotContains(t, u1After.AllowedGroups, targetGroup.ID)
	require.Contains(t, u1After.AllowedGroups, otherGroup.ID)

	u2After, err := repo.GetByID(ctx, u2.ID)
	require.NoError(t, err)
	require.NotContains(t, u2After.AllowedGroups, targetGroup.ID)
}

func TestGroupRepository_DeleteCascade_RemovesAllowedGroupsAndClearsApiKeys(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	entClient := tx.Client()

	targetGroup, err := entClient.Group.Create().
		SetName(uniqueTestValue(t, "delete-cascade-target")).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	otherGroup, err := entClient.Group.Create().
		SetName(uniqueTestValue(t, "delete-cascade-other")).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	userRepo := newUserRepositoryWithSQL(entClient, tx)
	groupRepo := newGroupRepositoryWithSQL(entClient, tx)
	apiKeyRepo := newAPIKeyRepositoryWithSQL(entClient, tx)

	u := &service.User{
		Email:         uniqueTestValue(t, "cascade-user") + "@example.com",
		PasswordHash:  "test-password-hash",
		Role:          service.RoleUser,
		Status:        service.StatusActive,
		Concurrency:   5,
		AllowedGroups: []int64{targetGroup.ID, otherGroup.ID},
	}
	require.NoError(t, userRepo.Create(ctx, u))

	key := &service.APIKey{
		UserID:  u.ID,
		Key:     uniqueTestValue(t, "sk-test-delete-cascade"),
		Name:    "test key",
		GroupID: &targetGroup.ID,
		Status:  service.StatusActive,
	}
	require.NoError(t, apiKeyRepo.Create(ctx, key))

	_, err = groupRepo.DeleteCascade(ctx, targetGroup.ID)
	require.NoError(t, err)

	// Deleted group should be hidden by default queries (soft-delete semantics).
	_, err = groupRepo.GetByID(ctx, targetGroup.ID)
	require.ErrorIs(t, err, service.ErrGroupNotFound)

	activeGroups, err := groupRepo.ListActive(ctx)
	require.NoError(t, err)
	for _, g := range activeGroups {
		require.NotEqual(t, targetGroup.ID, g.ID)
	}

	// User.allowed_groups should no longer include the deleted group.
	uAfter, err := userRepo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.NotContains(t, uAfter.AllowedGroups, targetGroup.ID)
	require.Contains(t, uAfter.AllowedGroups, otherGroup.ID)

	// API keys bound to the deleted group should have group_id cleared.
	keyAfter, err := apiKeyRepo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.Nil(t, keyAfter.GroupID)
}
