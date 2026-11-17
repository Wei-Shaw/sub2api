//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestScopedAdminAccountFieldsRejectUnknown(t *testing.T) {
	for _, body := range []string{`{"credentials":{}}`, `{"extra":{}}`, `{"platform":"openai"}`} {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal([]byte(body), &fields)
		if _, err := scopedAccountFieldPermissions(fields, false); err == nil {
			t.Fatalf("应拒绝字段: %s", body)
		}
	}
}

func TestScopedAdminAccountModelMappingFieldPermissions(t *testing.T) {
	for _, create := range []bool{false, true} {
		for _, mapping := range []string{`{"gpt-6":"gpt-6","alias":"gpt-6"}`, `{}`, `null`} {
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(`{"credentials":{"model_mapping":`+mapping+`}}`), &fields))
			permissions, err := scopedAccountFieldPermissions(fields, create)
			require.NoError(t, err)
			require.Equal(t, []string{"accounts.models.write"}, permissions)
		}
	}
	for _, mapping := range []string{`[]`, `"gpt-6"`, `{"gpt-6":42}`, `{"gpt-6":null}`, `{"gpt-6":{}}`, `{"":"gpt-6"}`, `{"gpt-6":" "}`, `{"gpt-6":"` + strings.Repeat("x", 65536) + `"}`} {
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(`{"credentials":{"model_mapping":`+mapping+`}}`), &fields))
		_, err := scopedAccountFieldPermissions(fields, false)
		require.Equal(t, 400, infraerrors.Code(err), mapping)
	}
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(`{"credentials":{"model_mapping":{},"access_token":"secret","base_url":"https://example.com"}}`), &fields))
	permissions, err := scopedAccountFieldPermissions(fields, false)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"accounts.models.write", "accounts.credentials", "accounts.network"}, permissions)
}

func TestScopedAdminWriteModelMappingEnforcesScopeAndPreservesCredentials(t *testing.T) {
	login, db := scopedAdminAccountTestDB(t)
	_, err := db.Exec(`CREATE TABLE account_groups(account_id BIGINT,group_id BIGINT);
	INSERT INTO accounts(id,name,platform,type,credentials) VALUES
	(101,'共享','openai','oauth','{"access_token":"original-token","refresh_token":"original-refresh","base_url":"https://example.com","model_mapping":{"old":"old"}}'),
	(102,'API','openai','apikey','{}'),(103,'其他组','openai','oauth','{}'),(104,'其他平台','anthropic','oauth','{}');
	INSERT INTO account_groups VALUES(101,1),(101,2),(102,1),(103,2),(104,1);`)
	require.NoError(t, err)
	repo := &scopedAdminWriteTestRepo{login: login}
	admin := &adminServiceImpl{accountRepo: repo, entClient: dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))}
	fields := func(body string) map[string]json.RawMessage {
		var result map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(body), &result))
		return result
	}
	ctxFor := func(permissions ...string) context.Context {
		return adminaccess.WithPrincipal(context.Background(), &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: permissions, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}, Platforms: []string{"openai"}}}}})
	}
	body := `{"credentials":{"model_mapping":{"gpt-6":"gpt-6","alias":"gpt-6"}}}`
	_, err = admin.ScopedAdminWrite(ctxFor("accounts.credentials", "accounts.update"), 101, "", fields(body), nil)
	require.Equal(t, 403, infraerrors.Code(err), "写凭证权限不能隐式修改模型")
	ctx := ctxFor("accounts.models.write")
	for _, id := range []int64{102, 103, 104, 999} {
		_, err = admin.ScopedAdminWrite(ctx, id, "", fields(body), nil)
		require.Equal(t, 404, infraerrors.Code(err))
	}
	for _, forbidden := range []string{`{"credentials":{"model_mapping":{},"access_token":"changed"}}`, `{"credentials":{"model_mapping":{},"base_url":"https://evil.example"}}`} {
		_, err = admin.ScopedAdminWrite(ctx, 101, "", fields(forbidden), nil)
		require.Equal(t, 403, infraerrors.Code(err), "模型权限不能混写凭证或网络")
	}
	for _, mapping := range []string{`{"gpt-6":"gpt-6","alias":"gpt-6"}`, `{}`, `null`} {
		_, err = admin.ScopedAdminWrite(ctx, 101, "", fields(`{"credentials":{"model_mapping":`+mapping+`}}`), nil)
		require.NoError(t, err)
		a, err := repo.GetByID(ctx, 101)
		require.NoError(t, err)
		require.Equal(t, "original-token", a.Credentials["access_token"])
		require.Equal(t, "original-refresh", a.Credentials["refresh_token"])
		require.Equal(t, "https://example.com", a.Credentials["base_url"])
		var want any
		require.NoError(t, json.Unmarshal([]byte(mapping), &want))
		require.Equal(t, want, a.Credentials["model_mapping"])
		require.Equal(t, []int64{1, 2}, a.GroupIDs)
	}
}

// 用真实数据库事务承载最小仓储，直接验证授权服务到业务写入的完整调用链。
type scopedAdminWriteTestRepo struct {
	AccountRepository
	login    *scopedTestDB
	failBind bool
}

func (r *scopedAdminWriteTestRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	a := &Account{ID: id}
	var credentials []byte
	rows, err := r.login.sql(ctx).QueryContext(ctx, `SELECT name,platform,type,status,credentials FROM accounts WHERE id=$1`, id)
	if err != nil {
		return nil, err
	}
	if !rows.Next() {
		_ = rows.Close()
		return nil, ErrAccountNotFound
	}
	err = rows.Scan(&a.Name, &a.Platform, &a.Type, &a.Status, &credentials)
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(credentials, &a.Credentials); err != nil {
		return nil, err
	}
	rows, err = r.login.sql(ctx).QueryContext(ctx, `SELECT group_id FROM account_groups WHERE account_id=$1 ORDER BY group_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var group int64
		if err = rows.Scan(&group); err != nil {
			return nil, err
		}
		a.GroupIDs = append(a.GroupIDs, group)
	}
	return a, rows.Err()
}
func (r *scopedAdminWriteTestRepo) ListShadowsByParent(context.Context, int64) ([]*Account, error) {
	return nil, nil
}
func (r *scopedAdminWriteTestRepo) Update(ctx context.Context, a *Account) error {
	credentials, err := json.Marshal(a.Credentials)
	if err != nil {
		return err
	}
	_, err = r.login.sql(ctx).ExecContext(ctx, `UPDATE accounts SET name=$2,credentials=$3 WHERE id=$1`, a.ID, a.Name, string(credentials))
	return err
}
func (r *scopedAdminWriteTestRepo) Create(ctx context.Context, a *Account) error {
	rows, err := r.login.sql(ctx).QueryContext(ctx, `INSERT INTO accounts(name,platform,type) VALUES($1,$2,$3) RETURNING id`, a.Name, a.Platform, a.Type)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		return errors.New("创建未返回 ID")
	}
	return rows.Scan(&a.ID)
}
func (r *scopedAdminWriteTestRepo) BindGroups(ctx context.Context, id int64, groups []int64) error {
	if r.failBind {
		return errors.New("模拟绑定失败")
	}
	if _, err := r.login.sql(ctx).ExecContext(ctx, `DELETE FROM account_groups WHERE account_id=$1`, id); err != nil {
		return err
	}
	for _, group := range groups {
		if _, err := r.login.sql(ctx).ExecContext(ctx, `INSERT INTO account_groups(account_id,group_id) VALUES($1,$2)`, id, group); err != nil {
			return err
		}
	}
	return nil
}

type scopedAdminWriteTestGroups struct{ GroupRepository }

func (*scopedAdminWriteTestGroups) GetByID(_ context.Context, id int64) (*Group, error) {
	return &Group{ID: id}, nil
}

func TestScopedAdminWriteEnforcesScopeAndAtomicCreate(t *testing.T) {
	login, db := scopedAdminAccountTestDB(t)
	_, err := db.Exec(`CREATE TABLE account_groups(account_id BIGINT,group_id BIGINT);
	INSERT INTO accounts(id,name,type) VALUES(101,'共享','oauth'),(102,'API','apikey'),(103,'其他组','oauth');
	INSERT INTO account_groups VALUES(101,1),(101,2),(102,1),(103,2);`)
	require.NoError(t, err)
	repo := &scopedAdminWriteTestRepo{login: login}
	admin := &adminServiceImpl{accountRepo: repo, groupRepo: &scopedAdminWriteTestGroups{}, entClient: dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))}
	principal := &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.groups", "accounts.update", "accounts.create", "accounts.credentials"}, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}}}}}
	ctx := adminaccess.WithPrincipal(context.Background(), principal)
	fields := func(body string) map[string]json.RawMessage {
		var result map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(body), &result))
		return result
	}
	_, err = admin.ScopedAdminWrite(ctx, 101, "", fields(`{"name":"已修改","group_ids":[]}`), nil)
	require.NoError(t, err)
	a, err := repo.GetByID(ctx, 101)
	require.NoError(t, err)
	require.Equal(t, "已修改", a.Name)
	require.Equal(t, []int64{2}, a.GroupIDs, "移除 A 时保留无权操作的 B")
	for _, tc := range []struct {
		id   int64
		body string
	}{
		{102, `{"name":"禁止"}`},
		{103, `{"group_ids":[1]}`},
		{103, `{"name":"禁止"}`},
	} {
		_, err = admin.ScopedAdminWrite(ctx, tc.id, "", fields(tc.body), nil)
		require.Error(t, err)
		require.Equal(t, 404, infraerrors.Code(err))
	}
	// 恢复 A 仅用于验证有范围但无字段权限的拒绝。
	_, err = db.Exec(`INSERT INTO account_groups VALUES(101,1)`)
	require.NoError(t, err)
	_, err = admin.ScopedAdminWrite(ctx, 101, "", fields(`{"status":"inactive"}`), nil)
	require.Equal(t, 403, infraerrors.Code(err))
	_, err = admin.ScopedAdminWrite(ctx, 101, "", fields(`{"extra":{"secret":"禁止"}}`), nil)
	require.Equal(t, 400, infraerrors.Code(err))
	repo.failBind = true
	_, err = admin.ScopedAdminWrite(ctx, 0, "", fields(`{"name":"必须回滚","type":"oauth","platform":"openai","credentials":{"access_token":"测试"},"group_ids":[1]}`), nil)
	require.ErrorContains(t, err, "模拟绑定失败")
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM accounts WHERE name='必须回滚'`).Scan(&count))
	require.Zero(t, count, "绑定失败不能留下未分组账户")
}

func TestScopedAdminWriteHidesAccountsOutsideGrantedScope(t *testing.T) {
	login, db := scopedAdminAccountTestDB(t)
	_, err := db.Exec(`CREATE TABLE account_groups(account_id BIGINT,group_id BIGINT);
	INSERT INTO accounts(id,name,platform,type) VALUES
	(101,'范围内','openai','oauth'),(102,'其他组','openai','oauth'),
	(103,'其他类型','openai','apikey'),(104,'其他平台','anthropic','oauth'),
	(105,'未分组','openai','oauth'),(106,'仅分组模型权限','openai','oauth');
	INSERT INTO account_groups VALUES(101,1),(102,2),(103,1),(104,1),(106,3);`)
	require.NoError(t, err)
	repo := &scopedAdminWriteTestRepo{login: login}
	admin := &adminServiceImpl{accountRepo: repo, entClient: dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))}
	principal := &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{
		{Permissions: []string{"accounts.update"}, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}, Platforms: []string{"openai"}},
		// 只有范围但未授予权限的规则不能暴露账户存在性。
		{GroupIDs: []int64{2}, IncludeUngrouped: true, AccountTypes: []string{"oauth"}},
		// 分组模型权限不能使账户进入可操作范围。
		{Permissions: []string{"groups.models.read"}, GroupIDs: []int64{3}, AccountTypes: []string{"oauth"}},
	}}}
	ctx := adminaccess.WithPrincipal(context.Background(), principal)
	for _, tc := range []struct {
		name string
		id   int64
		body string
		code int
	}{
		{"范围内缺少字段权限", 101, `{"status":"inactive"}`, 403},
		{"其他组且仅匹配空权限规则", 102, `{"name":"禁止"}`, 404},
		{"其他账户类型", 103, `{"name":"禁止"}`, 404},
		{"其他平台", 104, `{"name":"禁止"}`, 404},
		{"未分组且仅匹配空权限规则", 105, `{"name":"禁止"}`, 404},
		{"仅匹配分组模型权限", 106, `{"name":"禁止"}`, 404},
		{"账户不存在", 999, `{"name":"禁止"}`, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(tc.body), &fields))
			_, err := admin.ScopedAdminWrite(ctx, tc.id, "", fields, nil)
			require.Error(t, err)
			require.Equal(t, tc.code, infraerrors.Code(err))
		})
	}
}

func TestScopedAdminGroupReplacementPreservesForeignBindings(t *testing.T) {
	policy := adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.groups"}, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}}}}
	resource := adminaccess.Account{ID: 9, Type: "oauth", GroupIDs: []int64{1}}
	for _, tc := range []struct {
		request []int64
		want    []int64
		denied  bool
	}{{[]int64{1}, []int64{2, 1}, false}, {[]int64{}, []int64{2}, false}, {[]int64{2}, nil, true}} {
		got, err := scopedAdminMergeGroups(policy, resource, []int64{1, 2}, tc.request)
		if (err != nil) != tc.denied || (!tc.denied && !reflect.DeepEqual(got, tc.want)) {
			t.Fatalf("request=%v got=%v err=%v", tc.request, got, err)
		}
	}
}

func TestScopedAdminAccountCredentialsNeedIndependentPermissions(t *testing.T) {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal([]byte(`{"credentials":{"api_key":"secret","base_url":"https://example.com"},"status":"active","concurrency":2}`), &fields)
	permissions, err := scopedAccountFieldPermissions(fields, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"accounts.credentials", "accounts.network", "accounts.status", "accounts.schedule"} {
		found := false
		for _, p := range permissions {
			if p == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("缺少独立权限 %s", want)
		}
	}
}

func TestScopedAdminCreateWithCredentialWriteDoesNotRequireRead(t *testing.T) {
	login, db := scopedAdminAccountTestDB(t)
	_, err := db.Exec(`CREATE TABLE account_groups(account_id BIGINT,group_id BIGINT)`)
	require.NoError(t, err)
	repo := &scopedAdminWriteTestRepo{login: login}
	s := &adminServiceImpl{accountRepo: repo, groupRepo: &scopedAdminWriteTestGroups{}, entClient: dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))}
	p := &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.create", "accounts.credentials"}, GroupIDs: []int64{2}, AccountTypes: []string{"oauth"}}}}}
	ctx := adminaccess.WithPrincipal(context.Background(), p)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(`{"name":"仅写凭证也可创建","platform":"openai","type":"oauth","group_ids":[2],"credentials":{"access_token":"TEST_TOKEN"}}`), &fields))
	account, err := s.ScopedAdminWrite(ctx, 0, "", fields, nil)
	require.NoError(t, err)
	require.Positive(t, account.ID)
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM accounts WHERE id=$1`, account.ID).Scan(&count))
	require.Equal(t, 1, count)
	require.False(t, p.Policy.AllowsAccount("accounts.read", scopedAccountResource(account)))
}
