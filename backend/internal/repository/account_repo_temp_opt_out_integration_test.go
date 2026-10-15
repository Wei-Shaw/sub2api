//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountTempOptOutPersistsAndRejectsStaleWriters(t *testing.T) {
	tx := testEntTx(t)
	client := tx.Client()
	repo := newAccountRepositoryWithSQL(client, tx, nil)
	ctx := context.Background()
	account := mustCreateAccount(t, client, &service.Account{Name: "temp-opt-out", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Schedulable: true})
	until := time.Now().Add(time.Hour)
	require.NoError(t, repo.SetTempUnschedulable(ctx, account.ID, until, "before opt-out"))
	account, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, account.TempUnschedulableUntil)
	if account.Credentials == nil {
		account.Credentials = map[string]any{}
	}
	account.Credentials["temp_unschedulable_disabled"] = true
	require.NoError(t, repo.Update(ctx, account))
	saved, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.True(t, saved.IsTempUnschedulableDisabled())
	require.Nil(t, saved.TempUnschedulableUntil)
	require.Empty(t, saved.TempUnschedulableReason)
	// A request started before the edit can still attempt an ID-only write.
	require.NoError(t, repo.SetTempUnschedulable(ctx, account.ID, until.Add(time.Hour), "old request"))
	saved, err = repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Nil(t, saved.TempUnschedulableUntil)
	require.True(t, saved.IsSchedulable())
	// Turning the option off restores the existing cooldown behavior.
	saved.Credentials["temp_unschedulable_disabled"] = false
	require.NoError(t, repo.Update(ctx, saved))
	require.NoError(t, repo.SetTempUnschedulable(ctx, account.ID, until, "after opt-out"))
	saved, err = repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, saved.TempUnschedulableUntil)
	require.False(t, saved.IsSchedulable())
}
