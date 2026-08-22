//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCreateGroupFromSourceRollsBackWhenOutboxInsertFails(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newGroupRepositoryWithSQL(client, integrationDB)
	suffix := time.Now().UnixNano()
	operationID := strings.Repeat("b", 64)

	source, err := client.Group.Create().
		SetName(fmt.Sprintf("duplicate-rollback-source-%d", suffix)).
		SetPlatform(service.PlatformAnthropic).
		Save(ctx)
	require.NoError(t, err)
	account, err := client.Account.Create().
		SetName(fmt.Sprintf("duplicate-rollback-account-%d", suffix)).
		SetPlatform(service.PlatformAnthropic).
		SetType(service.AccountTypeOAuth).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.AccountGroup.Create().
		SetAccountID(account.ID).
		SetGroupID(source.ID).
		SetPriority(29).
		Save(ctx)
	require.NoError(t, err)

	functionName := fmt.Sprintf("fail_group_duplicate_outbox_%d", suffix)
	triggerName := fmt.Sprintf("fail_group_duplicate_outbox_trigger_%d", suffix)
	_, err = integrationDB.ExecContext(ctx, fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.group_id IS NOT NULL AND EXISTS (
				SELECT 1 FROM groups
				WHERE id = NEW.group_id AND duplicate_operation_id = '%s'
			) THEN
				RAISE EXCEPTION 'forced duplicate outbox failure';
			END IF;
			RETURN NEW;
		END;
		$$`, functionName, operationID))
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, fmt.Sprintf(
		"CREATE TRIGGER %s BEFORE INSERT ON scheduler_outbox FOR EACH ROW EXECUTE FUNCTION %s()",
		triggerName,
		functionName,
	))
	require.NoError(t, err)

	duplicateName := fmt.Sprintf("duplicate-rollback-copy-%d", suffix)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON scheduler_outbox", triggerName))
		_, _ = integrationDB.ExecContext(context.Background(), fmt.Sprintf("DROP FUNCTION IF EXISTS %s()", functionName))
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM scheduler_outbox WHERE group_id = $1", source.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM account_groups WHERE account_id = $1", account.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM accounts WHERE id = $1", account.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM groups WHERE name IN ($1, $2)", source.Name, duplicateName)
	})

	duplicate := &service.Group{
		Name:                 duplicateName,
		Platform:             source.Platform,
		RateMultiplier:       1,
		Status:               "inactive",
		SubscriptionType:     service.SubscriptionTypeStandard,
		DuplicateOperationID: operationID,
	}
	err = repo.CreateFromSource(ctx, duplicate, source.ID)
	require.ErrorContains(t, err, "forced duplicate outbox failure")

	var groupCount, bindingCount, outboxCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM groups WHERE name = $1", duplicateName).Scan(&groupCount))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM account_groups WHERE group_id = $1", duplicate.ID).Scan(&bindingCount))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM scheduler_outbox WHERE group_id = $1", duplicate.ID).Scan(&outboxCount))
	require.Zero(t, groupCount)
	require.Zero(t, bindingCount)
	require.Zero(t, outboxCount)
}
