//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/userallowedgroup"
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

func TestGroupRepository_DeleteCascade_PreservesApiKeyGroupID(t *testing.T) {
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

	// API keys keep their group_id so auth can reject keys bound to a deleted group.
	keyAfter, err := apiKeyRepo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.NotNil(t, keyAfter.GroupID)
	require.Equal(t, targetGroup.ID, *keyAfter.GroupID)
	require.Nil(t, keyAfter.Group)
}

func TestGroupUpdateExclusiveAuthorizesOnlyActiveUndeletedKeyOwners(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	groupRepo := newGroupRepositoryWithSQL(client, tx)
	apiKeyRepo := newAPIKeyRepositoryWithSQL(client, tx)

	createUser := func(email string) *dbent.User {
		user, err := client.User.Create().
			SetEmail(email).
			SetPasswordHash("test-password-hash").
			SetRole(service.RoleUser).
			SetStatus(service.StatusActive).
			Save(ctx)
		require.NoError(t, err)
		return user
	}
	createGroup := func(name string) *service.Group {
		group, err := client.Group.Create().
			SetName(name).
			SetPlatform(service.PlatformAnthropic).
			SetStatus(service.StatusActive).
			SetSubscriptionType(service.SubscriptionTypeStandard).
			Save(ctx)
		require.NoError(t, err)
		return groupEntityToService(group)
	}
	createKey := func(userID, groupID int64, name string) *service.APIKey {
		key := &service.APIKey{
			UserID:  userID,
			GroupID: &groupID,
			Key:     uniqueTestValue(t, "sk-"+name),
			Name:    name,
			Status:  service.StatusActive,
		}
		require.NoError(t, apiKeyRepo.Create(ctx, key))
		return key
	}
	allowedCount := func(userID, groupID int64) int {
		count, err := client.UserAllowedGroup.Query().
			Where(userallowedgroup.UserIDEQ(userID), userallowedgroup.GroupIDEQ(groupID)).
			Count(ctx)
		require.NoError(t, err)
		return count
	}

	group := createGroup(uniqueTestValue(t, "exclusive-transition"))
	activeUser := createUser(uniqueTestValue(t, "exclusive-active") + "@example.com")
	disabledUser := createUser(uniqueTestValue(t, "exclusive-disabled") + "@example.com")
	deletedUser := createUser(uniqueTestValue(t, "exclusive-deleted") + "@example.com")

	createKey(activeUser.ID, group.ID, "active")
	disabledKey := createKey(disabledUser.ID, group.ID, "disabled")
	deletedKey := createKey(deletedUser.ID, group.ID, "deleted")

	_, err := client.APIKey.UpdateOneID(disabledKey.ID).SetStatus(service.StatusDisabled).Save(ctx)
	require.NoError(t, err)
	_, err = client.APIKey.UpdateOneID(deletedKey.ID).SetDeletedAt(time.Now().UTC()).Save(ctx)
	require.NoError(t, err)

	group.IsExclusive = true
	require.NoError(t, groupRepo.Update(ctx, group))

	require.Equal(t, 1, allowedCount(activeUser.ID, group.ID))
	require.Equal(t, 0, allowedCount(disabledUser.ID, group.ID))
	require.Equal(t, 0, allowedCount(deletedUser.ID, group.ID))

	// Repeating the update must remain idempotent under the composite key.
	require.NoError(t, groupRepo.Update(ctx, group))
	require.Equal(t, 1, allowedCount(activeUser.ID, group.ID))
}

func TestUserAllowedGroupsUpdatePreservesExclusiveGroupUsedByActiveKey(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	userRepo := newUserRepositoryWithSQL(client, tx)
	apiKeyRepo := newAPIKeyRepositoryWithSQL(client, tx)

	user, err := client.User.Create().
		SetEmail(uniqueTestValue(t, "allowed-groups-preserve") + "@example.com").
		SetPasswordHash("test-password-hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	exclusiveGroup, err := client.Group.Create().
		SetName(uniqueTestValue(t, "exclusive-preserve")).
		SetPlatform(service.PlatformAnthropic).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeStandard).
		Save(ctx)
	require.NoError(t, err)
	otherGroup, err := client.Group.Create().
		SetName(uniqueTestValue(t, "other-preserve")).
		SetPlatform(service.PlatformAnthropic).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeStandard).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.Group.UpdateOneID(exclusiveGroup.ID).SetIsExclusive(true).Save(ctx)
	require.NoError(t, err)
	key := &service.APIKey{
		UserID:  user.ID,
		GroupID: &exclusiveGroup.ID,
		Key:     uniqueTestValue(t, "sk-owner"),
		Name:    "owner",
		Status:  service.StatusActive,
	}
	require.NoError(t, apiKeyRepo.Create(ctx, key))
	require.NoError(t, userRepo.AddGroupToAllowedGroups(ctx, user.ID, otherGroup.ID))

	userIn := userEntityToService(user)
	userIn.AllowedGroups = nil
	require.NoError(t, userRepo.Update(ctx, userIn, service.UserUpdateFields{AllowedGroups: true}))

	count, err := client.UserAllowedGroup.Query().
		Where(userallowedgroup.UserIDEQ(user.ID), userallowedgroup.GroupIDEQ(exclusiveGroup.ID)).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	count, err = client.UserAllowedGroup.Query().
		Where(userallowedgroup.UserIDEQ(user.ID), userallowedgroup.GroupIDEQ(otherGroup.ID)).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	keys, err := client.APIKey.Query().
		Where(apikey.UserIDEQ(user.ID), apikey.GroupIDEQ(exclusiveGroup.ID), apikey.StatusEQ(service.StatusActive), apikey.DeletedAtIsNil()).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, keys, 1)
}

func TestRemoveUserAllowedGroupPreservesExclusiveGroupUsedByActiveKey(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	userRepo := newUserRepositoryWithSQL(client, tx)
	apiKeyRepo := newAPIKeyRepositoryWithSQL(client, tx)

	user, err := client.User.Create().
		SetEmail(uniqueTestValue(t, "remove-allowed-owner") + "@example.com").
		SetPasswordHash("test-password-hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetName(uniqueTestValue(t, "remove-allowed-exclusive")).
		SetPlatform(service.PlatformAnthropic).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeStandard).
		SetIsExclusive(true).
		Save(ctx)
	require.NoError(t, err)
	key := &service.APIKey{
		UserID:  user.ID,
		GroupID: &group.ID,
		Key:     uniqueTestValue(t, "sk-remove-allowed"),
		Name:    "owner",
		Status:  service.StatusActive,
	}
	require.NoError(t, apiKeyRepo.Create(ctx, key))
	require.NoError(t, userRepo.RemoveGroupFromUserAllowedGroups(ctx, user.ID, group.ID))

	count, err := client.UserAllowedGroup.Query().
		Where(userallowedgroup.UserIDEQ(user.ID), userallowedgroup.GroupIDEQ(group.ID)).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	_, err = client.APIKey.UpdateOneID(key.ID).SetStatus(service.StatusDisabled).Save(ctx)
	require.NoError(t, err)
	require.NoError(t, userRepo.RemoveGroupFromUserAllowedGroups(ctx, user.ID, group.ID))
	count, err = client.UserAllowedGroup.Query().
		Where(userallowedgroup.UserIDEQ(user.ID), userallowedgroup.GroupIDEQ(group.ID)).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, count)
}
