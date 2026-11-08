//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCreateWithAccountGroupsPersistsPausedCopyAtomically(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newAccountRepositoryWithSQL(client, integrationDB, nil)
	suffix := time.Now().UnixNano()

	group, err := client.Group.Create().
		SetName(fmt.Sprintf("duplicate-atomic-%d", suffix)).
		SetPlatform(service.PlatformAnthropic).
		Save(ctx)
	require.NoError(t, err)

	success := &service.Account{
		Name:        fmt.Sprintf("duplicate-success-%d", suffix),
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: false,
		Credentials: map[string]any{"api_key": "secret"},
		Extra:       map[string]any{},
	}
	require.NoError(t, repo.CreateWithAccountGroups(ctx, success, []service.AccountGroup{{GroupID: group.ID, Priority: 37}}))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM scheduler_outbox WHERE account_id = $1", success.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM account_groups WHERE account_id = $1", success.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM accounts WHERE id = $1", success.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM groups WHERE id = $1", group.ID)
	})

	var schedulable bool
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT schedulable FROM accounts WHERE id = $1", success.ID).Scan(&schedulable))
	require.False(t, schedulable)
	var priority int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT priority FROM account_groups WHERE account_id = $1 AND group_id = $2", success.ID, group.ID).Scan(&priority))
	require.Equal(t, 37, priority)
	var outboxCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM scheduler_outbox WHERE account_id = $1", success.ID).Scan(&outboxCount))
	require.Equal(t, 1, outboxCount)

	failure := &service.Account{
		Name:        fmt.Sprintf("duplicate-failure-%d", suffix),
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: false,
		Credentials: map[string]any{"api_key": "secret"},
		Extra:       map[string]any{},
	}
	err = repo.CreateWithAccountGroups(ctx, failure, []service.AccountGroup{{GroupID: int64(^uint64(0) >> 1), Priority: 1}})
	require.Error(t, err)

	var accountCount, groupCount, failedOutboxCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM accounts WHERE name = $1", failure.Name).Scan(&accountCount))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM account_groups WHERE account_id = $1", failure.ID).Scan(&groupCount))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM scheduler_outbox WHERE account_id = $1", failure.ID).Scan(&failedOutboxCount))
	require.Zero(t, accountCount)
	require.Zero(t, groupCount)
	require.Zero(t, failedOutboxCount)
}
