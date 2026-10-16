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

func TestCreateGroupFromSourceRollsBackWhenBindingOrOutboxInsertFails(t *testing.T) {
	for _, table := range []string{"channel_groups", "scheduler_outbox"} {
		t.Run(table, func(t *testing.T) {
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

			var channelID int64
			require.NoError(t, integrationDB.QueryRowContext(ctx, "INSERT INTO channels (name) VALUES ($1) RETURNING id", fmt.Sprintf("duplicate-rollback-channel-%d", suffix)).Scan(&channelID))
			_, err = integrationDB.ExecContext(ctx, "INSERT INTO channel_groups (channel_id, group_id) VALUES ($1, $2)", channelID, source.ID)
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
				"CREATE TRIGGER %s BEFORE INSERT ON %s FOR EACH ROW EXECUTE FUNCTION %s()",
				triggerName,
				table,
				functionName,
			))
			require.NoError(t, err)

			duplicateName := fmt.Sprintf("duplicate-rollback-copy-%d", suffix)
			t.Cleanup(func() {
				_, _ = integrationDB.ExecContext(context.Background(), fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON %s", triggerName, table))
				_, _ = integrationDB.ExecContext(context.Background(), fmt.Sprintf("DROP FUNCTION IF EXISTS %s()", functionName))
				_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM scheduler_outbox WHERE group_id = $1", source.ID)
				_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM account_groups WHERE account_id = $1", account.ID)
				_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM channels WHERE id = $1", channelID)
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

			var groupCount, bindingCount, channelBindingCount, outboxCount int
			require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM groups WHERE name = $1", duplicateName).Scan(&groupCount))
			require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM account_groups WHERE group_id = $1", duplicate.ID).Scan(&bindingCount))
			require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM scheduler_outbox WHERE group_id = $1", duplicate.ID).Scan(&outboxCount))
			require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM channel_groups WHERE group_id = $1", duplicate.ID).Scan(&channelBindingCount))
			require.Zero(t, channelBindingCount)
			require.Zero(t, groupCount)
			require.Zero(t, bindingCount)
			require.Zero(t, outboxCount)
		})
	}
}
