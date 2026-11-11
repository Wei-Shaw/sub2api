//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountRenameConcurrentPlansAndPreservedState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	repo := newAccountRepositoryWithSQL(testEntClient(t), integrationDB, nil)
	prefix := fmt.Sprintf("rename-%d-", time.Now().UnixNano())
	accounts := make([]*service.Account, 2)
	for i := range accounts {
		account := &service.Account{Name: fmt.Sprintf("%sold%d", prefix, i), Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: false, Credentials: map[string]any{"refresh_token": "test-refresh"}, Extra: map[string]any{"marker": "preserve"}, Concurrency: 2, Priority: 7}
		require.NoError(t, repo.Create(ctx, account))
		stored, err := repo.GetByID(ctx, account.ID)
		require.NoError(t, err)
		accounts[i] = stored
		t.Cleanup(func() {
			_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM scheduler_outbox WHERE account_id = $1", account.ID)
			_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM accounts WHERE id = $1", account.ID)
		})
	}
	// Two administrators reviewed the same available number for different accounts.
	// Exactly one transaction may apply it; the other must require another preview.
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]error, 2)
	for i := range accounts {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			a := accounts[index]
			results[index] = repo.ApplyAccountRenames(ctx, []service.AccountRenameItem{{AccountID: a.ID, OldName: a.Name, NewName: prefix + "01"}})
		}(i)
	}
	close(start)
	wg.Wait()
	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, service.ErrAccountRenameConflict)
		}
	}
	require.Equal(t, 1, successes)
	for _, original := range accounts {
		got, err := repo.GetByID(ctx, original.ID)
		require.NoError(t, err)
		require.Equal(t, original.Credentials, got.Credentials)
		require.Equal(t, original.Extra, got.Extra)
		require.Equal(t, original.Schedulable, got.Schedulable)
		require.Equal(t, original.Status, got.Status)
		require.Equal(t, original.Concurrency, got.Concurrency)
		require.Equal(t, original.Priority, got.Priority)
	}
}

func TestAccountRenameRejectsStaleBatchAtomically(t *testing.T) {
	ctx := context.Background()
	repo := newAccountRepositoryWithSQL(testEntClient(t), integrationDB, nil)
	prefix := fmt.Sprintf("rename-stale-%d-", time.Now().UnixNano())
	var accounts []*service.Account
	for i := 0; i < 2; i++ {
		account := &service.Account{Name: fmt.Sprintf("%sold%d", prefix, i), Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: false, Credentials: map[string]any{}, Extra: map[string]any{}}
		require.NoError(t, repo.Create(ctx, account))
		accounts = append(accounts, account)
		t.Cleanup(func() {
			_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM scheduler_outbox WHERE account_id = $1", account.ID)
			_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM accounts WHERE id = $1", account.ID)
		})
	}
	plan := []service.AccountRenameItem{{AccountID: accounts[0].ID, OldName: accounts[0].Name, NewName: prefix + "01"}, {AccountID: accounts[1].ID, OldName: "stale name", NewName: prefix + "02"}}
	require.ErrorIs(t, repo.ApplyAccountRenames(ctx, plan), service.ErrAccountRenameConflict)
	for _, a := range accounts {
		got, err := repo.GetByID(ctx, a.ID)
		require.NoError(t, err)
		require.Equal(t, a.Name, got.Name)
	}
	plan[1].OldName = accounts[1].Name
	require.NoError(t, repo.ApplyAccountRenames(ctx, plan))
	require.NoError(t, repo.ApplyAccountRenames(ctx, plan), "a lost response can be retried with the same plan")
}

func TestAccountRenameLookupUsesLiteralAffixes(t *testing.T) {
	ctx := context.Background()
	repo := newAccountRepositoryWithSQL(testEntClient(t), integrationDB, nil)
	prefix := fmt.Sprintf("rename-lookup-%d-%%_", time.Now().UnixNano())
	suffix := "尾_%"
	var accounts []*service.Account
	for _, name := range []string{prefix + "selected-email", prefix + "07" + suffix, prefix + "09" + suffix + "extra"} {
		account := &service.Account{Name: name, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Credentials: map[string]any{}, Extra: map[string]any{}}
		require.NoError(t, repo.Create(ctx, account))
		accounts = append(accounts, account)
		t.Cleanup(func() {
			_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM scheduler_outbox WHERE account_id = $1", account.ID)
			_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM accounts WHERE id = $1", account.ID)
		})
	}
	got, err := repo.ListAccountNamesForRename(ctx, []int64{accounts[0].ID}, prefix, suffix)
	require.NoError(t, err)
	require.Equal(t, []service.AccountName{{ID: accounts[0].ID, Name: accounts[0].Name}, {ID: accounts[1].ID, Name: accounts[1].Name}}, got)
}
