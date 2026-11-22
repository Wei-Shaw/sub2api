package repository

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestCodexPoolQuotaPostgresMigration(t *testing.T) {
	dsn := os.Getenv("CODEX_POOL_QUOTA_TEST_DSN")
	if dsn == "" {
		t.Skip("set CODEX_POOL_QUOTA_TEST_DSN to a PostgreSQL instance with permission to create test databases")
	}
	ctx := context.Background()
	admin, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	names, err := fs.Glob(migrations.FS, "*.sql")
	require.NoError(t, err)
	for _, baseline := range []string{"", "218_", "253_"} {
		t.Run("baseline_"+baseline, func(t *testing.T) {
			name := fmt.Sprintf("codex_pool_quota_%d", time.Now().UnixNano())
			_, err := admin.ExecContext(ctx, "CREATE DATABASE "+pq.QuoteIdentifier(name))
			require.NoError(t, err)
			t.Cleanup(func() {
				_, err := admin.ExecContext(ctx, "DROP DATABASE "+pq.QuoteIdentifier(name))
				require.NoError(t, err)
			})
			db, err := sql.Open("postgres", dsn+" dbname="+name)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Close()) })

			var legacyGroupID int64
			oldMigrations := fstest.MapFS{}
			if baseline != "" {
				for _, name := range names {
					if strings.Compare(name, baseline) >= 0 {
						continue
					}
					body, err := fs.ReadFile(migrations.FS, name)
					require.NoError(t, err)
					oldMigrations[name] = &fstest.MapFile{Data: body}
				}
				require.NoError(t, applyMigrationsFS(ctx, db, oldMigrations))
				require.NoError(t, db.QueryRowContext(ctx,
					"INSERT INTO groups (name, platform) VALUES ('legacy-codex', 'openai') RETURNING id").Scan(&legacyGroupID))
			}
			require.NoError(t, ApplyMigrations(ctx, db))

			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			groups := NewGroupRepository(client, db)
			if legacyGroupID != 0 {
				group, err := groups.GetByIDLite(ctx, legacyGroupID)
				require.NoError(t, err)
				require.False(t, group.CodexPoolQuotaErrorEnabled)
			}
			group := &service.Group{Name: "codex-enabled", Platform: service.PlatformOpenAI, Status: service.StatusActive, CodexPoolQuotaErrorEnabled: true}
			require.NoError(t, groups.Create(ctx, group))

			// A rollback uses the earlier migration runner and writes only columns
			// it knows about. Re-upgrading must preserve the newer setting.
			if baseline != "" {
				require.NoError(t, applyMigrationsFS(ctx, db, oldMigrations))
			}
			_, err = db.ExecContext(ctx, "UPDATE groups SET description = 'legacy writer' WHERE id = $1", group.ID)
			require.NoError(t, err)
			require.NoError(t, ApplyMigrations(ctx, db))
			persisted, err := groups.GetByIDLite(ctx, group.ID)
			require.NoError(t, err)
			require.True(t, persisted.CodexPoolQuotaErrorEnabled)
			require.Equal(t, "legacy writer", persisted.Description)
			persisted.CodexPoolQuotaErrorEnabled = false
			require.NoError(t, groups.Update(ctx, persisted))
			persisted, err = groups.GetByIDLite(ctx, group.ID)
			require.NoError(t, err)
			require.False(t, persisted.CodexPoolQuotaErrorEnabled)

			var id int64
			require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO accounts (name, platform, type, extra)
				VALUES ('codex-quota-proof','openai','oauth','{"unrelated":true}') RETURNING id`).Scan(&id))
			accounts, ok := NewAccountRepository(client, db, nil).(*accountRepository)
			require.True(t, ok)
			reset := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
			require.NoError(t, accounts.SetCodexQuotaRateLimited(ctx, id, reset, reset, true))
			account, err := accounts.GetByID(ctx, id)
			require.NoError(t, err)
			require.True(t, reset.Equal(*account.RateLimitResetAt))
			require.Equal(t, true, account.Extra["unrelated"])
			proof, ok := account.Extra[service.OpenAICodexQuotaCooldownExtraKey].(map[string]any)
			require.True(t, ok)
			limitedAt, ok := proof["limited_at"].(string)
			require.True(t, ok)
			observed, err := time.Parse(time.RFC3339Nano, limitedAt)
			require.NoError(t, err)
			require.True(t, observed.Equal(*account.RateLimitedAt))
			require.NoError(t, accounts.ClearRateLimit(ctx, id))
			account, err = accounts.GetByID(ctx, id)
			require.NoError(t, err)
			require.Nil(t, account.RateLimitResetAt)
		})
	}
}
