//go:build unit

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestScopedAdminAccountListScopesCountAndPageInSQL(t *testing.T) {
	var countScope string
	matcher := sqlmock.QueryMatcherFunc(func(expected, actual string) error {
		if !strings.Contains(actual, `"type"`) || !strings.Contains(actual, `"account_groups"`) || !strings.Contains(actual, " OR ") {
			return fmt.Errorf("查询缺少完整规则: %s", actual)
		}
		where := actual[strings.Index(actual, " WHERE "):]
		if i := strings.Index(where, " ORDER BY "); i >= 0 {
			where = where[:i]
		}
		if i := strings.Index(where, " LIMIT "); i >= 0 {
			where = where[:i]
		}
		if expected == "count" {
			countScope = where
		} else {
			if where != countScope {
				return fmt.Errorf("分页和统计范围不同")
			}
			// 允许模型映射子字段投影，禁止读取完整 credentials。
			projected := strings.ReplaceAll(actual, `"accounts"."credentials"->'model_mapping' AS model_mapping`, "model_mapping")
			if strings.Contains(projected, `"credentials"`) || strings.Contains(projected, `"extra"`) {
				return fmt.Errorf("读取了敏感字段")
			}
		}
		return nil
	})
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	defer client.Close()
	repo := newAccountRepositoryWithSQL(client, db, nil)
	mock.ExpectQuery("count").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("page").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	principal := &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{
		{Permissions: []string{"accounts.read"}, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}},
		{Permissions: []string{"accounts.read"}, GroupIDs: []int64{2}, AccountTypes: []string{"apikey"}},
	}}}
	_, total, err := repo.ScopedAdminList(adminaccess.WithPrincipal(context.Background(), principal), pagination.PaginationParams{Page: 1, PageSize: 20}, "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Fatal(total)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestScopedAdminAccountListPostgresRulesAndPagination(t *testing.T) {
	dsn := os.Getenv("SCOPED_ADMIN_TEST_DSN")
	if dsn == "" {
		t.Skip("需要独立 SCOPED_ADMIN_TEST_DSN")
	}
	base, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	schema := fmt.Sprintf("scoped_list_test_%d", time.Now().UnixNano())
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
	// 最小数据集刻意交叉分组、类型和平台，防止不同规则被拆开组合。
	_, err = db.Exec(`CREATE TABLE accounts (
		id BIGINT PRIMARY KEY, name TEXT NOT NULL DEFAULT '', notes TEXT, platform TEXT NOT NULL,
		type TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'active', schedulable BOOLEAN NOT NULL DEFAULT true,
		concurrency INT NOT NULL DEFAULT 1, priority INT NOT NULL DEFAULT 1, credentials JSONB DEFAULT '{}', deleted_at TIMESTAMPTZ
	);
	CREATE TABLE account_groups(account_id BIGINT, group_id BIGINT, priority INT DEFAULT 50, created_at TIMESTAMPTZ DEFAULT NOW());
	INSERT INTO accounts(id,platform,type) VALUES
	(1,'openai','oauth'), (2,'openai','apikey'), (3,'openai','oauth'),
	(4,'openai','apikey'), (5,'anthropic','oauth'), (6,'openai','oauth');
	INSERT INTO account_groups(account_id,group_id) VALUES (1,1),(1,2),(2,1),(3,2),(4,2),(5,1);`)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE accounts SET credentials = '{"model_mapping":{"alias":"target","invalid":{"token":"NESTED_SECRET"}},"access_token":"TOKEN_SECRET","refresh_token":"REFRESH_SECRET","api_key":"KEY_SECRET","base_url":"URL_SECRET"}' WHERE id=1`)
	require.NoError(t, err)
	repo := newAccountRepositoryWithSQL(client, db, nil)
	principal := &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{
		{Permissions: []string{"accounts.read"}, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}, Platforms: []string{"openai"}},
		{Permissions: []string{"accounts.read"}, GroupIDs: []int64{2}, AccountTypes: []string{"apikey"}, Platforms: []string{"openai"}},
	}}}
	ctx := adminaccess.WithPrincipal(context.Background(), principal)
	for page, wantID := range []int64{1, 4} {
		rows, total, listErr := repo.ScopedAdminList(ctx, pagination.PaginationParams{Page: page + 1, PageSize: 1}, "", "", "", "")
		require.NoError(t, listErr)
		require.EqualValues(t, 2, total)
		require.Len(t, rows, 1)
		require.Equal(t, wantID, rows[0].ID)
		if wantID == 1 {
			require.Equal(t, map[string]any{"model_mapping": map[string]string{"alias": "target"}}, rows[0].Credentials)
		} else {
			require.Empty(t, rows[0].Credentials)
		}
	}
	// 缺失、清空及错误形状都应保留可读取列表，不回传嵌套对象或凭证。
	for _, credentials := range []string{`{}`, `{"model_mapping":null}`, `{"model_mapping":{}}`, `{"model_mapping":[]}`, `{"model_mapping":"invalid"}`, `{"model_mapping":{"bad":{"token":"SECRET"},"number":1},"access_token":"SECRET"}`} {
		_, err = db.Exec(`UPDATE accounts SET credentials=$1::jsonb WHERE id=4`, credentials)
		require.NoError(t, err)
		rows, total, listErr := repo.ScopedAdminList(ctx, pagination.PaginationParams{Page: 2, PageSize: 1}, "", "", "", "")
		require.NoError(t, listErr)
		require.EqualValues(t, 2, total)
		require.Len(t, rows, 1)
		require.EqualValues(t, 4, rows[0].ID)
		require.Empty(t, rows[0].Credentials)
	}
	principal.Policy.Rules = []adminaccess.Rule{{Permissions: []string{"accounts.read"}, AllGroups: true, AccountTypes: []string{"oauth"}}}
	rows, total, err := repo.ScopedAdminList(ctx, pagination.DefaultPagination(), "", "", "", "")
	require.NoError(t, err)
	require.EqualValues(t, 3, total, "全部分组不包含无分组账户")
	require.Len(t, rows, 3)
	principal.Policy.Rules[0].IncludeUngrouped = true
	_, total, err = repo.ScopedAdminList(ctx, pagination.DefaultPagination(), "", "", "", "")
	require.NoError(t, err)
	require.EqualValues(t, 4, total)
}

func TestScopedAdminCodexCandidatesQueryIsBounded(t *testing.T) {
	matcher := sqlmock.QueryMatcherFunc(func(_, actual string) error {
		where := actual[strings.Index(actual, " WHERE "):]
		if !strings.Contains(where, "credentials") || !strings.Contains(actual, " LIMIT ") {
			return fmt.Errorf("导入候选未按身份筛选并限制数量: %s", actual)
		}
		return nil
	})
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	require.NoError(t, err)
	defer db.Close()
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	defer client.Close()
	repo := newAccountRepositoryWithSQL(client, db, nil)
	ctx := adminaccess.WithPrincipal(context.Background(), &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.credentials"}, AccountTypes: []string{"oauth"}, AllGroups: true, IncludeUngrouped: true}}}})
	mock.ExpectQuery("候选身份").WillReturnRows(sqlmock.NewRows([]string{"id", "credentials"}))
	_, err = repo.ScopedAdminCodexCandidates(ctx, service.ScopedCodexIdentity{AccessToken: "inside", AccessTokenOnly: true})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestScopedAdminCodexCandidatesUseCredentialWriteScope(t *testing.T) {
	db := scopedAdminKeyTestDB(t)
	_, err := db.Exec(`CREATE TABLE accounts(id BIGINT PRIMARY KEY,platform TEXT,type TEXT,credentials JSONB DEFAULT '{}',deleted_at TIMESTAMPTZ);
 CREATE TABLE account_groups(account_id BIGINT,group_id BIGINT);
 INSERT INTO accounts(id,platform,type,credentials) VALUES
 (1,'openai','oauth','{"access_token":"inside"}'),(2,'openai','oauth','{"access_token":"outside"}'),
 (3,'openai','apikey','{}'),(4,'anthropic','oauth','{}'),(5,'openai','oauth','{}'),(6,'openai','oauth','{}');
 INSERT INTO account_groups VALUES(1,1),(1,2),(2,2),(3,1),(4,1),(6,1);
 UPDATE accounts SET deleted_at=NOW() WHERE id=6;
 UPDATE accounts SET credentials='{"access_token":"inside"}';`)
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	repo := newAccountRepositoryWithSQL(client, db, nil)
	principal := &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.credentials"}, AccountTypes: []string{"oauth"}, GroupIDs: []int64{1}}, {Permissions: []string{"accounts.read"}, AccountTypes: []string{"oauth"}, GroupIDs: []int64{2}}}}}
	ctx := adminaccess.WithPrincipal(context.Background(), principal)
	rows, err := repo.ScopedAdminCodexCandidates(ctx, service.ScopedCodexIdentity{AccessToken: "inside", AccessTokenOnly: true})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 1, rows[0].ID)
	require.Equal(t, "inside", rows[0].Credentials["access_token"])
	principal.Policy.Rules[0].IncludeUngrouped = true
	rows, err = repo.ScopedAdminCodexCandidates(ctx, service.ScopedCodexIdentity{AccessToken: "inside", AccessTokenOnly: true})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 5, rows[0].ID)
	principal.Policy.Rules[0].Permissions = []string{"accounts.read"}
	rows, err = repo.ScopedAdminCodexCandidates(ctx, service.ScopedCodexIdentity{AccessToken: "inside", AccessTokenOnly: true})
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestScopedAdminCodexCandidatesMatchIdentityWithoutLoadingUnrelatedCredentials(t *testing.T) {
	db := scopedAdminKeyTestDB(t)
	_, err := db.Exec(`CREATE TABLE accounts(id BIGINT PRIMARY KEY,platform TEXT,type TEXT,credentials JSONB DEFAULT '{}',deleted_at TIMESTAMPTZ);
 CREATE TABLE account_groups(account_id BIGINT,group_id BIGINT);
 INSERT INTO accounts(id,platform,type,credentials) VALUES
 (1,'openai','oauth','{"chatgpt_account_id":"workspace","chatgpt_user_id":"member","access_token":"old","refresh_token":"must-not-load","large_extra":"must-not-load"}'),
 (2,'openai','oauth','{"chatgpt_account_id":"workspace","chatgpt_user_id":"other","access_token":"shared"}'),
 (3,'openai','oauth','{"chatgpt_account_id":"workspace","access_token":"legacy"}'),
 (4,'openai','oauth','{"chatgpt_user_id":"member","access_token":"user-only"}'),
 (5,'openai','oauth','{"email":" Test@Example.COM ","access_token":"email-only"}'),
 (6,'openai','oauth','{"chatgpt_account_id":"elsewhere","chatgpt_user_id":"member","email":"test@example.com","access_token":"different-workspace"}'),
 (7,'openai','oauth','{"chatgpt_account_id":123.00,"chatgpt_user_id":9007199254740993,"access_token":"numeric-legacy"}');
 INSERT INTO accounts SELECT id,'openai','oauth',jsonb_build_object('access_token','unrelated-'||id,'refresh_token',repeat('x',8192)),NULL FROM generate_series(100,1100) AS id;`)
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	repo := newAccountRepositoryWithSQL(client, db, nil)
	ctx := adminaccess.WithPrincipal(context.Background(), &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.credentials"}, AccountTypes: []string{"oauth"}, AllGroups: true, IncludeUngrouped: true}}}})
	for _, tc := range []struct {
		name     string
		identity service.ScopedCodexIdentity
		want     int64
	}{
		{"精确组合优先于token及account回退", service.ScopedCodexIdentity{AccountID: "workspace", UserID: "member", AccessToken: "shared"}, 1},
		{"仅token不能借用相同workspace", service.ScopedCodexIdentity{AccountID: "workspace", UserID: "member", AccessToken: "missing", AccessTokenOnly: true}, 0},
		{"token命中不施加account用户冲突", service.ScopedCodexIdentity{AccountID: "workspace", UserID: "member", AccessToken: "shared", AccessTokenOnly: true}, 2},
		{"account回退允许缺失用户", service.ScopedCodexIdentity{AccountID: "workspace", UserID: "new-member", AccessToken: "missing"}, 3},
		{"仅user不匹配其他workspace", service.ScopedCodexIdentity{UserID: "member", AccessToken: "missing"}, 4},
		{"email忽略大小写空格但不借用workspace", service.ScopedCodexIdentity{Email: "test@example.com", AccessToken: "missing"}, 5},
		{"AgentIdentity仅account", service.ScopedCodexIdentity{AccountID: "workspace", AccessToken: "different-workspace", AgentIdentity: true}, 3},
		{"AgentIdentity不回退token", service.ScopedCodexIdentity{AccountID: "missing", AccessToken: "shared", AgentIdentity: true}, 0},
		{"空身份不查询全量", service.ScopedCodexIdentity{}, 0},
		{"数值身份遵循原float64归一化", service.ScopedCodexIdentity{AccountID: "123", UserID: "9007199254740992", AccessToken: "rotated-token"}, 7},
		{"不同workspace不能以user合并", service.ScopedCodexIdentity{AccountID: "new-workspace", UserID: "member"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := repo.ScopedAdminCodexCandidates(ctx, tc.identity)
			require.NoError(t, err)
			if tc.want == 0 {
				require.Empty(t, rows)
				return
			}
			require.Len(t, rows, 1)
			require.Equal(t, tc.want, rows[0].ID)
			require.NotContains(t, rows[0].Credentials, "refresh_token")
			require.NotContains(t, rows[0].Credentials, "large_extra")
		})
	}
	// 同一身份有大量旧副本时仍只取 ID 最大者；首尾 Unicode 空白与导入器一致。
	_, err = db.Exec(`INSERT INTO accounts SELECT id,'openai','oauth',jsonb_build_object('chatgpt_account_id', E'\tworkspace　','chatgpt_user_id','member','access_token','duplicate','refresh_token',repeat('x',8192)),NULL FROM generate_series(2000,3000) AS id;`)
	require.NoError(t, err)
	rows, err := repo.ScopedAdminCodexCandidates(ctx, service.ScopedCodexIdentity{AccountID: "workspace", UserID: "member"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 3000, rows[0].ID)
	require.Len(t, rows[0].Credentials, 4)
}
