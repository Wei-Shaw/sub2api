//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenDesignUsagePreservedByOrdinaryEditAndClearedOnCredentialChange(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "opendesign-usage", Platform: service.PlatformOpenDesign, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "original"},
		Extra:       map[string]any{"unrelated": "kept"},
	})
	binding := json.RawMessage(`{"workspace_id":"fixture","ciphertext":"encrypted-fixture","status":"ok"}`)
	applied, err := repo.CompareAndSwapOpenDesignUsage(ctx, account, binding)
	require.NoError(t, err)
	require.True(t, applied)
	applied, err = repo.CompareAndSwapOpenDesignUsage(ctx, account, nil)
	require.NoError(t, err)
	require.False(t, applied, "基于绑定前版本的并发请求不能清掉当前绑定")

	account.Name = "renamed"
	require.NoError(t, repo.Update(ctx, account))
	loaded, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, "renamed", loaded.Name)
	require.Equal(t, "fixture", service.OpenDesignUsageStateFromAccount(loaded).WorkspaceID)

	_, err = repo.BulkUpdate(ctx, []int64{account.ID}, service.AccountBulkUpdate{Credentials: map[string]any{"api_key": "replacement"}})
	require.NoError(t, err)
	loaded, err = repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Nil(t, service.OpenDesignUsageStateFromAccount(loaded))
	require.Equal(t, "kept", loaded.Extra["unrelated"])
}
