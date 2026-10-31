//go:build unit

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"os"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func scopedPermissionsTestDB(t *testing.T, options ...dbent.Option) (*dbent.Client, *sql.DB) {
	t.Helper()
	dsn := os.Getenv("SCOPED_ADMIN_TEST_DSN")
	if dsn == "" {
		t.Skip("需要独立 SCOPED_ADMIN_TEST_DSN")
	}
	base, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	schema := fmt.Sprintf("permissions_test_%d", time.Now().UnixNano())
	_, err = base.Exec(`CREATE SCHEMA ` + schema)
	require.NoError(t, err)
	db, err := sql.Open("postgres", dsn+" search_path="+schema)
	require.NoError(t, err)
	client := dbent.NewClient(append([]dbent.Option{dbent.Driver(entsql.OpenDB(dialect.Postgres, db))}, options...)...)
	t.Cleanup(func() { _ = client.Close(); _, _ = base.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); _ = base.Close() })
	_, err = db.Exec(`CREATE TABLE groups(id BIGINT PRIMARY KEY, name TEXT DEFAULT '分组', platform TEXT DEFAULT 'openai', status TEXT DEFAULT 'active', model_allowlist JSONB DEFAULT '{}', deleted_at TIMESTAMPTZ);
	CREATE TABLE accounts(id BIGINT PRIMARY KEY, name TEXT DEFAULT '账户', notes TEXT, platform TEXT DEFAULT 'openai', type TEXT DEFAULT 'oauth', status TEXT DEFAULT 'active', schedulable BOOLEAN DEFAULT true, concurrency INT DEFAULT 1, priority INT DEFAULT 1, credentials JSONB DEFAULT '{}', extra JSONB DEFAULT '{}', expires_at TIMESTAMPTZ, rate_limit_reset_at TIMESTAMPTZ, overload_until TIMESTAMPTZ, temp_unschedulable_until TIMESTAMPTZ, created_at TIMESTAMPTZ DEFAULT NOW(), deleted_at TIMESTAMPTZ);
	CREATE TABLE account_groups(account_id BIGINT, group_id BIGINT, priority INT DEFAULT 50, created_at TIMESTAMPTZ DEFAULT NOW());
	INSERT INTO groups(id,platform,status) VALUES (1,'openai','active'),(2,'openai','active'),(3,'anthropic','active'),(4,'openai','inactive');
	INSERT INTO accounts(id,credentials,extra) VALUES (1,'{"email":"test@example.com","access_token":"SECRET"}','{}'),(2,'{}','{"email":"test@example.com"}'),(3,'{"email":"test@example.com"}','{}');
	INSERT INTO account_groups(account_id,group_id) VALUES (1,1),(1,9),(2,2),(3,3);`)
	require.NoError(t, err)
	return client, db
}

func TestScopedAdminPermissionsPostgresFilters(t *testing.T) {
	client, db := scopedPermissionsTestDB(t)
	policy := adminaccess.Policy{Rules: []adminaccess.Rule{
		{Permissions: []string{"accounts.read", "groups.list.read", "groups.capacity.read"}, GroupIDs: []int64{1, 2, 3, 4}, AccountTypes: []string{"oauth"}, Platforms: []string{"openai"}},
		{Permissions: []string{"accounts.identity.read"}, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}, Platforms: []string{"openai"}},
		{Permissions: []string{"groups.capacity.read"}, GroupIDs: []int64{2}, Platforms: []string{"anthropic"}},
	}}
	ctx := adminaccess.WithPrincipal(context.Background(), &adminaccess.Principal{Policy: policy})
	groups := newGroupRepositoryWithSQL(client, db)
	ids, err := groups.ListScopedAdminCapacityGroupIDs(ctx)
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, ids, "平台和分组不能跨规则拼接，停用组不能参与容量统计")
	rows, total, err := groups.ScopedAdminListGroupModels(ctx, pagination.PaginationParams{Page: 1, PageSize: 1}, false, "openai", "active", "")
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, rows, 1)
	require.Equal(t, "active", rows[0].Status)
	accounts := newAccountRepositoryWithSQL(client, db, nil)
	for _, tc := range []struct {
		group  int64
		search string
		ids    []int64
	}{
		{2, "", []int64{2}}, {1, "TEST@example.com", []int64{1}}, {2, "test@example.com", []int64{}}, {0, "test@example.com", []int64{1}}, {9, "", []int64{}},
	} {
		rows, total, err := accounts.ScopedAdminList(ctx, pagination.PaginationParams{Page: 1, PageSize: 100}, "openai", "oauth", "", tc.search, tc.group)
		require.NoError(t, err)
		require.EqualValues(t, len(tc.ids), total)
		ids := []int64{}
		for _, row := range rows {
			ids = append(ids, row.ID)
			require.NotContains(t, row.Credentials, "access_token")
			if row.ID == 1 {
				require.Equal(t, "test@example.com", row.Credentials["email"])
				require.False(t, row.CreatedAt.IsZero())
			} else {
				require.NotContains(t, row.Credentials, "email")
			}
		}

		require.Equal(t, tc.ids, ids)
	}
	_, err = groups.ListScopedAdminCapacityGroupIDs(context.Background())
	require.Error(t, err)
}

func TestScopedAdminPermissionsWeeklySnapshotPostgres(t *testing.T) {
	client, db := scopedPermissionsTestDB(t)
	_, err := db.Exec(`UPDATE accounts SET credentials = '{"email":"test@example.com","access_token":"SECRET"}', extra = '{"email":"test@example.com","codex_usage_updated_at":"2026-10-25T00:00:00Z","codex_7d_used_percent":90,"codex_7d_reset_at":"2026-10-30T00:00:00Z","private":"SECRET"}'`)
	require.NoError(t, err)
	repo := newAccountRepositoryWithSQL(client, db, nil)
	for _, tc := range []struct {
		name     string
		usage    adminaccess.Rule
		identity bool
		want     bool
	}{
		{name: "无额度权限"},
		{name: "仅身份权限", identity: true},
		{name: "仅额度权限", usage: adminaccess.Rule{Permissions: []string{"accounts.usage.read"}, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}, Platforms: []string{"openai"}}, want: true},
		{name: "身份和额度权限", identity: true, usage: adminaccess.Rule{Permissions: []string{"accounts.usage.read"}, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}, Platforms: []string{"openai"}}, want: true},
		{name: "额度分组不匹配", usage: adminaccess.Rule{Permissions: []string{"accounts.usage.read"}, GroupIDs: []int64{99}, AccountTypes: []string{"oauth"}, Platforms: []string{"openai"}}},
		{name: "额度类型不匹配", usage: adminaccess.Rule{Permissions: []string{"accounts.usage.read"}, GroupIDs: []int64{1}, AccountTypes: []string{"apikey"}, Platforms: []string{"openai"}}},
		{name: "额度平台不匹配", usage: adminaccess.Rule{Permissions: []string{"accounts.usage.read"}, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}, Platforms: []string{"anthropic"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			read := adminaccess.Rule{Permissions: []string{"accounts.read"}, GroupIDs: []int64{1, 2}, AccountTypes: []string{"oauth"}, Platforms: []string{"openai"}}
			if tc.identity {
				read.Permissions = append(read.Permissions, "accounts.identity.read")
			}
			policy := adminaccess.Policy{Rules: []adminaccess.Rule{read}}
			if len(tc.usage.Permissions) > 0 {
				policy.Rules = append(policy.Rules, tc.usage)
			}
			ctx := adminaccess.WithPrincipal(context.Background(), &adminaccess.Principal{Policy: policy})
			rows, total, err := repo.ScopedAdminList(ctx, pagination.PaginationParams{Page: 1, PageSize: 100}, "openai", "oauth", "", "", 0)
			require.NoError(t, err)
			require.EqualValues(t, 2, total)
			require.Len(t, rows, 2)
			for _, row := range rows {
				want := map[string]any{}
				if tc.identity {
					want["email"] = "test@example.com"
					require.Equal(t, "test@example.com", row.Credentials["email"])
				}
				if tc.want && row.ID == 1 {
					want["codex_usage_updated_at"] = "2026-10-25T00:00:00Z"
					want["codex_7d_used_percent"] = float64(90)
					want["codex_7d_reset_at"] = "2026-10-30T00:00:00Z"
				}
				if len(want) == 0 {
					require.Empty(t, row.Extra)
				} else {
					require.Equal(t, want, row.Extra, "列表必须保留已授权的周额度快照，必须按行应用额度权限")
				}
				require.NotContains(t, row.Credentials, "access_token")
			}
		})
	}
}

func TestScopedAdminPermissionBasicGroupRepositoryRedactsModels(t *testing.T) {
	client, db := scopedPermissionsTestDB(t)
	_, err := db.Exec(`UPDATE groups SET model_allowlist='{"enabled":true,"models":["private-model"]}'`)
	require.NoError(t, err)
	repo := newGroupRepositoryWithSQL(client, db)
	p := &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{
		{Permissions: []string{"groups.list.read"}, GroupIDs: []int64{1, 2}, Platforms: []string{"openai"}},
		{Permissions: []string{"groups.models.read"}, GroupIDs: []int64{2}, Platforms: []string{"openai"}},
	}}}
	rows, total, err := repo.ScopedAdminListGroupModels(adminaccess.WithPrincipal(context.Background(), p), pagination.DefaultPagination(), false, "", "", "")
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, rows, 2)
	require.Empty(t, rows[0].ModelAllowlist.Models, "基本分组权限不能加载模型配置")
	require.Equal(t, []string{"private-model"}, rows[1].ModelAllowlist.Models)
}

func TestScopedAdminGroupModelsUpdatePreservesStatusPostgres(t *testing.T) {
	client, db := scopedPermissionsTestDB(t)
	_, err := db.Exec(`ALTER TABLE groups ADD COLUMN updated_at TIMESTAMPTZ;
 CREATE TABLE scheduler_outbox(id BIGSERIAL PRIMARY KEY,event_type TEXT,account_id BIGINT,group_id BIGINT,payload JSONB,dedup_key TEXT);
 CREATE UNIQUE INDEX outbox_dedup ON scheduler_outbox(dedup_key) WHERE dedup_key IS NOT NULL;`)
	require.NoError(t, err)
	repo := newGroupRepositoryWithSQL(client, db)
	ctx := adminaccess.WithPrincipal(context.Background(), &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"groups.models.write"}, GroupIDs: []int64{1}, Platforms: []string{"openai"}}}}})
	cfg := service.GroupModelAllowlist{Enabled: true, Models: []string{"model"}}
	for _, status := range []string{"active", "inactive"} {
		t.Run(status, func(t *testing.T) {
			_, err := db.Exec(`UPDATE groups SET status=$1 WHERE id=1`, status)
			require.NoError(t, err)
			updated, err := repo.ScopedAdminUpdateGroupModels(ctx, 1, cfg)
			require.NoError(t, err)
			require.Equal(t, status, updated.Status)
			var stored string
			require.NoError(t, db.QueryRow(`SELECT status FROM groups WHERE id=1`).Scan(&stored))
			require.Equal(t, status, stored)
		})
	}
}

func TestScopedAdminIdentityProjectionBatchesAuthorizedPagePostgres(t *testing.T) {
	identityQueries := []string{}
	client, db := scopedPermissionsTestDB(t, dbent.Debug(), dbent.Log(func(args ...any) {
		query := fmt.Sprint(args...)
		if strings.Contains(query, "subscription_expires_at") {
			identityQueries = append(identityQueries, query)
		}
	}))
	_, err := db.Exec(`INSERT INTO accounts(id,credentials,extra) SELECT i,'{"email":"batch@example.com","access_token":"SECRET","model_mapping":{"alias":"target"}}'::jsonb,'{"email":"batch@example.com","codex_7d_used_percent":90,"private":"SECRET"}'::jsonb FROM generate_series(100,199) i;
 INSERT INTO account_groups(account_id,group_id) SELECT i,CASE WHEN i%2=0 THEN 1 ELSE 2 END FROM generate_series(100,199) i;`)
	require.NoError(t, err)
	policy := adminaccess.Policy{Rules: []adminaccess.Rule{
		{Permissions: []string{"accounts.read"}, GroupIDs: []int64{1, 2}, AccountTypes: []string{"oauth"}, Platforms: []string{"openai"}},
		{Permissions: []string{"accounts.identity.read"}, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}, Platforms: []string{"openai"}},
		{Permissions: []string{"accounts.usage.read"}, GroupIDs: []int64{2}, AccountTypes: []string{"oauth"}, Platforms: []string{"openai"}},
	}}
	ctx := adminaccess.WithPrincipal(context.Background(), &adminaccess.Principal{Policy: policy})
	repo := newAccountRepositoryWithSQL(client, db, nil)
	rows, total, err := repo.ScopedAdminList(ctx, pagination.PaginationParams{Page: 1, PageSize: 100}, "openai", "oauth", "", "", 0)
	require.NoError(t, err)
	require.EqualValues(t, 102, total)
	require.Len(t, rows, 100)
	for _, row := range rows {
		resource := adminaccess.Account{ID: row.ID, Type: row.Type, Platform: row.Platform, GroupIDs: row.GroupIDs}
		if policy.AllowsAccount("accounts.identity.read", resource) {
			require.Contains(t, row.Credentials, "email")
			require.False(t, row.CreatedAt.IsZero())
		} else {
			require.NotContains(t, row.Credentials, "email")
			require.NotContains(t, row.Extra, "email")
			require.True(t, row.CreatedAt.IsZero())
		}
		if row.ID >= 100 {
			require.Equal(t, map[string]string{"alias": "target"}, row.Credentials["model_mapping"])
			require.Equal(t, policy.AllowsAccount("accounts.usage.read", resource), row.Extra["codex_7d_used_percent"] != nil)
		}
		require.NotContains(t, row.Credentials, "access_token")
		require.NotContains(t, row.Extra, "private")
	}
	require.Equal(t, 1, len(identityQueries), "同一分页的身份信息必须单次批量读取")
	require.Contains(t, identityQueries[0], `"accounts"."id" IN (`)
}
