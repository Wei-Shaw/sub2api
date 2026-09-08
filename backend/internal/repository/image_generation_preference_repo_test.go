package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserRepositorySetLastImageAPIKeyIDAcceptsOwnedActiveKey(t *testing.T) {
	_, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	owner := mustCreateAPIKeyRepoUser(t, ctx, client, "image-preference-owner@example.com")
	key, err := client.APIKey.Create().
		SetUserID(owner.ID).
		SetKey("sk-image-preference-owner").
		SetName("image preference owner").
		SetStatus(service.StatusAPIKeyActive).
		Save(ctx)
	require.NoError(t, err)

	repo := newUserRepositoryWithSQL(client, nil)
	persisted, err := repo.SetLastImageAPIKeyID(ctx, owner.ID, &key.ID)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	require.Equal(t, key.ID, *persisted)

	loaded, err := repo.GetLastImageAPIKeyID(ctx, owner.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, key.ID, *loaded)
}

func TestUserRepositorySetLastImageAPIKeyIDRejectsForeignAndDeletedKeys(t *testing.T) {
	_, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	owner := mustCreateAPIKeyRepoUser(t, ctx, client, "image-preference-owner@example.com")
	other := mustCreateAPIKeyRepoUser(t, ctx, client, "image-preference-other@example.com")
	foreignKey, err := client.APIKey.Create().
		SetUserID(other.ID).
		SetKey("sk-image-preference-foreign").
		SetName("image preference foreign").
		SetStatus(service.StatusAPIKeyActive).
		Save(ctx)
	require.NoError(t, err)
	deletedKey, err := client.APIKey.Create().
		SetUserID(owner.ID).
		SetKey("sk-image-preference-deleted").
		SetName("image preference deleted").
		SetStatus(service.StatusAPIKeyActive).
		Save(ctx)
	require.NoError(t, err)
	require.NoError(t, client.APIKey.UpdateOneID(deletedKey.ID).SetDeletedAt(time.Now().UTC()).Exec(ctx))

	repo := newUserRepositoryWithSQL(client, nil)
	_, err = repo.SetLastImageAPIKeyID(ctx, owner.ID, &foreignKey.ID)
	require.ErrorIs(t, err, service.ErrAPIKeyNotFound)
	_, err = repo.SetLastImageAPIKeyID(ctx, owner.ID, &deletedKey.ID)
	require.ErrorIs(t, err, service.ErrAPIKeyNotFound)
}

func TestUserRepositorySetLastImageAPIKeyIDClearsExistingPreference(t *testing.T) {
	_, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	owner := mustCreateAPIKeyRepoUser(t, ctx, client, "image-preference-clear@example.com")
	key, err := client.APIKey.Create().
		SetUserID(owner.ID).
		SetKey("sk-image-preference-clear").
		SetName("image preference clear").
		SetStatus(service.StatusAPIKeyActive).
		Save(ctx)
	require.NoError(t, err)

	repo := newUserRepositoryWithSQL(client, nil)
	_, err = repo.SetLastImageAPIKeyID(ctx, owner.ID, &key.ID)
	require.NoError(t, err)
	persisted, err := repo.SetLastImageAPIKeyID(ctx, owner.ID, nil)
	require.NoError(t, err)
	require.Nil(t, persisted)

	loaded, err := repo.GetLastImageAPIKeyID(ctx, owner.ID)
	require.NoError(t, err)
	require.Nil(t, loaded)
}
