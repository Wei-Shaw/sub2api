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
REDACTED
	safeName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	return fmt.Sprintf("%s-%s", prefix, safeName)
REDACTED

func TestUserRepository_RemoveGroupFromAllowedGroups_RemovesAllOccurrences(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	entClient := tx.Client()

	targetGroup, err := entClient.Group.Create().
		SetName(uniqueTestValue(t, "target-group")).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED
	otherGroup, err := entClient.Group.Create().
		SetName(uniqueTestValue(t, "other-group")).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	repo := newUserRepositoryWithSQL(entClient, tx)

	u1 := &service.User{
		Email:         uniqueTestValue(t, "u1") + "@example.com",
		PasswordHash:  "test-password-hash",
		Role:          service.RoleUser,
		Status:        service.StatusActive,
		Concurrency:   5,
		AllowedGroups: []int64{targetGroup.ID, otherGroup.IDREDACTED,
REDACTED
	require.NoError(t, repo.Create(ctx, u1))

	u2 := &service.User{
		Email:         uniqueTestValue(t, "u2") + "@example.com",
		PasswordHash:  "test-password-hash",
		Role:          service.RoleUser,
		Status:        service.StatusActive,
		Concurrency:   5,
		AllowedGroups: []int64{targetGroup.IDREDACTED,
REDACTED
	require.NoError(t, repo.Create(ctx, u2))

	u3 := &service.User{
		Email:         uniqueTestValue(t, "u3") + "@example.com",
		PasswordHash:  "test-password-hash",
		Role:          service.RoleUser,
		Status:        service.StatusActive,
		Concurrency:   5,
		AllowedGroups: []int64{otherGroup.IDREDACTED,
REDACTED
	require.NoError(t, repo.Create(ctx, u3))

	affected, err := repo.RemoveGroupFromAllowedGroups(ctx, targetGroup.ID)
REDACTED
	require.Equal(t, int64(2), affected)

	u1After, err := repo.GetByID(ctx, u1.ID)
REDACTED
	require.NotContains(t, u1After.AllowedGroups, targetGroup.ID)
	require.Contains(t, u1After.AllowedGroups, otherGroup.ID)

	u2After, err := repo.GetByID(ctx, u2.ID)
REDACTED
	require.NotContains(t, u2After.AllowedGroups, targetGroup.ID)
REDACTED

func TestGroupRepository_DeleteCascade_PreservesApiKeyGroupID(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	entClient := tx.Client()

	targetGroup, err := entClient.Group.Create().
		SetName(uniqueTestValue(t, "delete-cascade-target")).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED
	otherGroup, err := entClient.Group.Create().
		SetName(uniqueTestValue(t, "delete-cascade-other")).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	userRepo := newUserRepositoryWithSQL(entClient, tx)
	groupRepo := newGroupRepositoryWithSQL(entClient, tx)
	apiKeyRepo := newAPIKeyRepositoryWithSQL(entClient, tx)

	u := &service.User{
		Email:         uniqueTestValue(t, "cascade-user") + "@example.com",
		PasswordHash:  "test-password-hash",
		Role:          service.RoleUser,
		Status:        service.StatusActive,
		Concurrency:   5,
		AllowedGroups: []int64{targetGroup.ID, otherGroup.IDREDACTED,
REDACTED
	require.NoError(t, userRepo.Create(ctx, u))

	key := &service.APIKey{
		UserID:  u.ID,
		Key:     uniqueTestValue(t, "sk-test-delete-cascade"),
		Name:    "test key",
		GroupID: &targetGroup.ID,
		Status:  service.StatusActive,
REDACTED
	require.NoError(t, apiKeyRepo.Create(ctx, key))

	_, err = groupRepo.DeleteCascade(ctx, targetGroup.ID)
REDACTED

	// Deleted group should be hidden by default queries (soft-delete semantics).
	_, err = groupRepo.GetByID(ctx, targetGroup.ID)
	require.ErrorIs(t, err, service.ErrGroupNotFound)

	activeGroups, err := groupRepo.ListActive(ctx)
REDACTED
	for _, g := range activeGroups {
		require.NotEqual(t, targetGroup.ID, g.ID)
REDACTED

	// User.allowed_groups should no longer include the deleted group.
	uAfter, err := userRepo.GetByID(ctx, u.ID)
REDACTED
	require.NotContains(t, uAfter.AllowedGroups, targetGroup.ID)
	require.Contains(t, uAfter.AllowedGroups, otherGroup.ID)

	// API keys keep their group_id so auth can reject keys bound to a deleted group.
	keyAfter, err := apiKeyRepo.GetByID(ctx, key.ID)
REDACTED
	require.NotNil(t, keyAfter.GroupID)
	require.Equal(t, targetGroup.ID, *keyAfter.GroupID)
	require.Nil(t, keyAfter.Group)
REDACTED
