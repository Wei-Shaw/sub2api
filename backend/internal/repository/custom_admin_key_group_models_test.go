//go:build unit

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestScopedAdminGroupModelsPostgresScopeAndAtomicWrite(t *testing.T) {
	dsn := os.Getenv("SCOPED_ADMIN_TEST_DSN")
	if dsn == "" {
		t.Skip("需要独立 SCOPED_ADMIN_TEST_DSN")
	}
	base, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	schema := fmt.Sprintf("scoped_models_test_%d", time.Now().UnixNano())
	_, err = base.Exec(`CREATE SCHEMA ` + schema)
	require.NoError(t, err)
	db, err := sql.Open("postgres", dsn+" search_path="+schema)
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() {
		_ = client.Close()
		_, _ = base.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
		_ = base.Close()
	})
	_, err = db.Exec(`CREATE TABLE groups(id BIGINT PRIMARY KEY, name TEXT DEFAULT '模型组', platform TEXT DEFAULT 'openai',
		model_allowlist JSONB DEFAULT '{}', rate_multiplier NUMERIC DEFAULT 9, status TEXT DEFAULT 'active', updated_at TIMESTAMPTZ DEFAULT NOW(), deleted_at TIMESTAMPTZ);
		CREATE TABLE scheduler_outbox(id BIGSERIAL PRIMARY KEY, event_type TEXT, account_id BIGINT, group_id BIGINT, payload JSONB, dedup_key TEXT);
		CREATE UNIQUE INDEX outbox_dedup ON scheduler_outbox(dedup_key) WHERE dedup_key IS NOT NULL;
		INSERT INTO groups(id,platform,deleted_at) VALUES (2,'openai',NULL),(3,'openai',NULL),(4,'anthropic',NULL),(5,'openai',NOW());`)
	require.NoError(t, err)
	repo := newGroupRepositoryWithSQL(client, db)
	principal := &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"groups.models.write"}, GroupIDs: []int64{2, 4, 5}, Platforms: []string{"openai"}}}}}
	ctx := adminaccess.WithPrincipal(context.Background(), principal)
	// 列表范围与总数必须一起过滤，不能将不同规则的平台和分组交叉拼接。
	readPrincipal := &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{
		{Permissions: []string{"groups.models.read"}, GroupIDs: []int64{2, 4, 5}, Platforms: []string{"openai"}},
		{Permissions: []string{"groups.models.read"}, GroupIDs: []int64{3}, Platforms: []string{"anthropic"}},
	}}}
	readctx := adminaccess.WithPrincipal(context.Background(), readPrincipal)
	params := pagination.PaginationParams{Page: 1, PageSize: 1}
	rows, total, err := repo.ScopedAdminListGroupModels(readctx, params, false, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	require.Equal(t, int64(2), rows[0].ID)
	rows, total, err = repo.ScopedAdminListGroupModels(readctx, pagination.PaginationParams{Page: 2, PageSize: 1}, false, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Empty(t, rows)
	for _, filter := range []struct{ platform, status, search string }{{"anthropic", "", ""}, {"", "inactive", ""}, {"", "", "未匹配"}} {
		rows, total, err = repo.ScopedAdminListGroupModels(readctx, params, true, filter.platform, filter.status, filter.search)
		require.NoError(t, err)
		require.Zero(t, total)
		require.Empty(t, rows)
	}
	rows, total, err = repo.ScopedAdminListGroupModels(ctx, params, true, "", "", "")
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, rows, "只写权限不能读取列表")
	_, _, err = repo.ScopedAdminListGroupModels(context.Background(), params, true, "", "", "")
	require.Equal(t, 403, infraerrors.Code(err))
	cfg := service.GroupModelAllowlist{Enabled: true, Models: []string{"custom-model", "other-model"}}
	for _, id := range []int64{3, 4, 5, 99} {
		_, err = repo.ScopedAdminUpdateGroupModels(ctx, id, cfg)
		require.Equal(t, 404, infraerrors.Code(err))
	}
	_, err = repo.ScopedAdminUpdateGroupModels(context.Background(), 2, cfg)
	require.Equal(t, 403, infraerrors.Code(err))
	updated, err := repo.ScopedAdminUpdateGroupModels(ctx, 2, cfg)
	require.NoError(t, err)
	require.Equal(t, cfg, updated.ModelAllowlist)
	var configJSON, status string
	var rate float64
	require.NoError(t, db.QueryRow(`SELECT model_allowlist::text,rate_multiplier,status FROM groups WHERE id=2`).Scan(&configJSON, &rate, &status))
	require.JSONEq(t, `{"enabled":true,"models":["custom-model","other-model"]}`, configJSON)
	require.Equal(t, 9.0, rate)
	require.Equal(t, "active", status)
	var events int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM scheduler_outbox WHERE group_id=2`).Scan(&events))
	require.Equal(t, 1, events)
	// outbox 写入失败必须一起回滚模型配置。
	_, err = db.Exec(`DROP TABLE scheduler_outbox`)
	require.NoError(t, err)
	_, err = repo.ScopedAdminUpdateGroupModels(ctx, 2, service.GroupModelAllowlist{Models: []string{"must-rollback"}})
	require.Error(t, err)
	var after string
	require.NoError(t, db.QueryRow(`SELECT model_allowlist::text FROM groups WHERE id=2`).Scan(&after))
	require.JSONEq(t, configJSON, after)
}
