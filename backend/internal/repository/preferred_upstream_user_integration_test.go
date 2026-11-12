//go:build integration

package repository

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPreferredUpstreamUserPersistence(t *testing.T) {
	tx := testEntTx(t)
	ctx := dbent.NewTxContext(context.Background(), tx)
	client := tx.Client()
	accountRepo := newAccountRepositoryWithSQL(client, tx, nil)
	userRepo := newUserRepositoryWithSQL(client, tx)
	user := mustCreateUser(t, client, &service.User{})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "preferred-upstream", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
	})
	stale, err := accountRepo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Nil(t, stale.PreferredUserID)

	require.NoError(t, accountRepo.UpdateWithAccountBillingSettings(ctx, account, nil, nil, nil, &user.ID))
	require.Equal(t, &user.ID, account.PreferredUserID)

	stale.Name = "name-only-update"
	require.NoError(t, accountRepo.Update(ctx, stale))
	require.Equal(t, &user.ID, stale.PreferredUserID, "an update from an old snapshot must not erase the preference")

	require.NoError(t, userRepo.Delete(ctx, user.ID))
	current, err := accountRepo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Nil(t, current.PreferredUserID)
	require.ErrorIs(t, accountRepo.UpdateWithAccountBillingSettings(ctx, current, nil, nil, nil, &user.ID), service.ErrUserNotFound)
}
