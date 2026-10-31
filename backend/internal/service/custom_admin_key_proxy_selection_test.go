//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type scopedSelectionProxyRepo struct {
	ProxyRepository
	calls int
	mode  string
	id    int64
}

func (r *scopedSelectionProxyRepo) SelectAccountProxy(ctx context.Context, mode string) (int64, error) {
	if dbent.TxFromContext(ctx) == nil {
		panic("选择必须位于账户事务中")
	}
	r.calls++
	r.mode = mode
	return r.id, nil
}

type scopedSelectionAccountRepo struct{ *scopedAdminWriteTestRepo }

func (r *scopedSelectionAccountRepo) Create(ctx context.Context, a *Account) error {
	if err := r.scopedAdminWriteTestRepo.Create(ctx, a); err != nil {
		return err
	}
	_, err := r.login.sql(ctx).ExecContext(ctx, `UPDATE accounts SET proxy_id=$2 WHERE id=$1`, a.ID, a.ProxyID)
	return err
}
func (r *scopedSelectionAccountRepo) Update(ctx context.Context, a *Account) error {
	if err := r.scopedAdminWriteTestRepo.Update(ctx, a); err != nil {
		return err
	}
	_, err := r.login.sql(ctx).ExecContext(ctx, `UPDATE accounts SET proxy_id=$2 WHERE id=$1`, a.ID, a.ProxyID)
	return err
}
func (r *scopedSelectionAccountRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	a, err := r.scopedAdminWriteTestRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	rows, err := r.login.sql(ctx).QueryContext(ctx, `SELECT proxy_id FROM accounts WHERE id=$1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		err = rows.Scan(&a.ProxyID)
	}
	if err != nil {
		return nil, err
	}
	return a, rows.Err()
}
func TestScopedAccountProxySelectionPermissionsAndRollback(t *testing.T) {
	login, db := scopedAdminAccountTestDB(t)
	_, err := db.Exec(`CREATE TABLE account_groups(account_id BIGINT,group_id BIGINT)`)
	require.NoError(t, err)
	repo := &scopedSelectionAccountRepo{&scopedAdminWriteTestRepo{login: login}}
	proxy := &scopedSelectionProxyRepo{id: 7}
	s := &adminServiceImpl{accountRepo: repo, proxyRepo: proxy, groupRepo: &scopedAdminWriteTestGroups{}, entClient: dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))}
	principal := &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.create", "accounts.credentials"}, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}}}}}
	ctx := adminaccess.WithPrincipal(context.Background(), principal)
	for _, field := range []string{`"proxy_id":7`, `"proxy_mode":"random"`, `"random_proxy":true`, `"proxy_mode":"least_used"`, `"proxy_mode":"direct"`} {
		var body map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(`{"name":"代理选择","platform":"openai","type":"oauth","group_ids":[1],"credentials":{"access_token":"fixture"},`+field+`}`), &body))
		_, err = s.ScopedAdminWrite(ctx, 0, "", body, nil)
		require.Equal(t, 403, infraerrors.Code(err))
		require.Zero(t, proxy.calls)
		principal.Policy.Rules[0].Permissions = append(principal.Policy.Rules[0].Permissions, "accounts.network")
		a, err := s.ScopedAdminWrite(ctx, 0, "", body, nil)
		require.NoError(t, err)
		var id *int64
		require.NoError(t, db.QueryRow(`SELECT proxy_id FROM accounts WHERE id=$1`, a.ID).Scan(&id))
		if field == `"proxy_mode":"direct"` {
			require.Nil(t, id)
		} else {
			require.NotNil(t, id)
			require.EqualValues(t, 7, *id)
		}
		principal.Policy.Rules[0].Permissions = principal.Policy.Rules[0].Permissions[:2]
		proxy.calls = 0
	}
	principal.Policy.Rules[0].Permissions = append(principal.Policy.Rules[0].Permissions, "accounts.network", "accounts.schedule", "accounts.groups")
	for _, mode := range []string{"random", "least_used"} {
		input := &CreateAccountInput{Name: "导入代理", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "fixture"}, GroupIDs: []int64{1}, ProxyMode: mode}
		a, err := s.ScopedAdminCreateCodex(ctx, input)
		require.NoError(t, err)
		require.Equal(t, mode, proxy.mode)
		require.EqualValues(t, 7, *a.ProxyID)
		repo.failBind = true
		_, err = s.ScopedAdminCreateCodex(ctx, input)
		require.Error(t, err)
		repo.failBind = false
	}
	_, err = db.Exec(`INSERT INTO accounts(id,name,platform,type,proxy_id) VALUES(1001,'可管理','openai','oauth',7),(1002,'组外','openai','oauth',7),(1003,'类型外','openai','apikey',7); INSERT INTO account_groups VALUES(1001,1),(1002,2),(1003,1);`)
	require.NoError(t, err)
	fields := map[string]json.RawMessage{"proxy_mode": json.RawMessage(`"least_used"`)}
	calls := proxy.calls
	for _, id := range []int64{1002, 1003} {
		_, err = s.ScopedAdminWrite(ctx, id, "", fields, nil)
		require.Equal(t, 404, infraerrors.Code(err))
	}
	require.Equal(t, calls, proxy.calls, "越权时不能进入代理选择")
	denied := adminaccess.WithPrincipal(context.Background(), &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.credentials", "accounts.schedule"}, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}}}}})
	_, err = s.ScopedAdminWrite(denied, 1001, "", fields, nil)
	require.Equal(t, 403, infraerrors.Code(err))
	_, err = s.ScopedAdminUpdateCodex(denied, 1001, &UpdateAccountInput{Credentials: map[string]any{"access_token": "fixture"}, ProxyMode: "random"}, func(_ *Account, _ *UpdateAccountInput) error {
		t.Fatal("缺网络权限不能执行更新回调")
		return nil
	})
	require.Equal(t, 403, infraerrors.Code(err))
	require.Equal(t, calls, proxy.calls)
	proxy.id = 8
	updated, err := s.ScopedAdminWrite(ctx, 1001, "", fields, nil)
	require.NoError(t, err)
	require.EqualValues(t, 8, *updated.ProxyID)
	updated, err = s.ScopedAdminWrite(ctx, 1001, "", map[string]json.RawMessage{"proxy_mode": json.RawMessage(`"direct"`)}, nil)
	require.NoError(t, err)
	require.Nil(t, updated.ProxyID)
	updated, err = s.ScopedAdminUpdateCodex(ctx, 1001, &UpdateAccountInput{Credentials: map[string]any{"access_token": "fixture"}, ProxyMode: "random"}, func(_ *Account, _ *UpdateAccountInput) error { return nil })
	require.NoError(t, err)
	require.EqualValues(t, 8, *updated.ProxyID)
	// 更新后的后续分组失败应回滚代理绑定。
	repo.failBind = true
	_, err = s.ScopedAdminWrite(ctx, 1001, "", map[string]json.RawMessage{"proxy_mode": json.RawMessage(`"direct"`), "group_ids": json.RawMessage(`[1]`)}, nil)
	require.Error(t, err)
	repo.failBind = false
	stored, err := repo.GetByID(ctx, 1001)
	require.NoError(t, err)
	require.EqualValues(t, 8, *stored.ProxyID)
	proxy.id = 0
	_, err = s.ScopedAdminCreateCodex(ctx, &CreateAccountInput{Name: "空池禁止直连", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "fixture"}, GroupIDs: []int64{1}, ProxyMode: "random"})
	require.Equal(t, 400, infraerrors.Code(err))
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM accounts WHERE name='空池禁止直连'`).Scan(&count))
	require.Zero(t, count)
}

func TestNormalizeAccountProxySelectionConflicts(t *testing.T) {
	id := int64(7)
	zero := int64(0)
	for _, tc := range []struct {
		mode    string
		id      *int64
		random  bool
		want    string
		invalid bool
	}{
		{"", &id, false, "", false}, {"specified", &id, false, "specified", false}, {"direct", nil, false, "direct", false}, {"", nil, true, "random", false}, {"random", nil, false, "random", false}, {"least_used", nil, false, "least_used", false},
		{"random", &id, false, "", true}, {"least_used", &id, false, "", true}, {"direct", &id, false, "", true}, {"specified", &zero, false, "", true}, {"unknown", nil, false, "", true}, {"least_used", nil, true, "", true},
	} {
		got, err := NormalizeAccountProxyMode(tc.mode, tc.id, tc.random)
		if tc.invalid {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		}
	}
}
