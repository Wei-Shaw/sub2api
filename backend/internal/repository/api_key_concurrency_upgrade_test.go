package repository

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Wei-Shaw/sub2api/migrations"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Requires an explicitly supplied disposable PostgreSQL server. Each path gets
// its own database; this must never point to an application database/server.
func TestAPIKeyConcurrencyUpgradePaths(t *testing.T) {
	dsn := os.Getenv("SUB2API_UPGRADE_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("set SUB2API_UPGRADE_TEST_POSTGRES_URL to a disposable PostgreSQL server")
	}
	admin, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	ctx := context.Background()
	const filename = "237_add_api_key_concurrency_limit.sql"
	const checksum = "64659971d796ca6f80bca02642a5c54939943d40d071e1bee8b3661ca33631c2"
	for _, deployed := range []bool{false, true} {
		t.Run(fmt.Sprintf("deployed_pr=%t", deployed), func(t *testing.T) {
			name := fmt.Sprintf("key_upgrade_%d", time.Now().UnixNano())
			_, err := admin.ExecContext(ctx, "CREATE DATABASE "+name)
			require.NoError(t, err)
			t.Cleanup(func() {
				_, err := admin.ExecContext(ctx, "DROP DATABASE "+name+" WITH (FORCE)")
				require.NoError(t, err)
			})
			u, err := url.Parse(dsn)
			require.NoError(t, err)
			u.Path = "/" + name
			db, err := sql.Open("postgres", u.String())
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			baseline := fstest.MapFS{}
			files, err := fs.Glob(migrations.FS, "*.sql")
			require.NoError(t, err)
			for _, file := range files {
				if file == filename && !deployed {
					continue
				}
				data, err := fs.ReadFile(migrations.FS, file)
				require.NoError(t, err)
				baseline[file] = &fstest.MapFile{Data: data}
			}
			require.NoError(t, applyMigrationsFS(ctx, db, baseline))
			var userID, keyID int64
			require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO users(email,password_hash) VALUES ('upgrade@example.test','test') RETURNING id").Scan(&userID))
			require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO api_keys(user_id,key,name) VALUES ($1,'upgrade-key','upgrade') RETURNING id", userID).Scan(&keyID))
			want := 0
			if deployed {
				want = 9
				_, err := db.ExecContext(ctx, "UPDATE api_keys SET concurrency_limit=9 WHERE id=$1", keyID)
				require.NoError(t, err)
			}
			for repeat := 0; repeat < 2; repeat++ {
				require.NoError(t, applyMigrationsFS(ctx, db, migrations.FS))
				var got int
				require.NoError(t, db.QueryRowContext(ctx, "SELECT concurrency_limit FROM api_keys WHERE id=$1", keyID).Scan(&got))
				require.Equal(t, want, got)
				var recorded string
				require.NoError(t, db.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE filename=$1", filename).Scan(&recorded))
				require.Equal(t, checksum, recorded)
				var count int
				require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE filename IN ($1,'237_add_minimax_platform.sql','240_affiliate_ledger_operation_id.sql')", filename).Scan(&count))
				require.Equal(t, 3, count)
			}
			_, err = db.ExecContext(ctx, "UPDATE api_keys SET concurrency_limit=-1 WHERE id=$1", keyID)
			require.ErrorContains(t, err, "api_keys_concurrency_limit_nonnegative")
			var defaultLimit int
			require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO api_keys(user_id,key,name) VALUES ($1,'new-upgrade-key','new') RETURNING concurrency_limit", userID).Scan(&defaultLimit))
			require.Zero(t, defaultLimit)
		})
	}
}
