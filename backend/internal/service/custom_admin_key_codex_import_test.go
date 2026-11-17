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

type scopedCodexTestRepo struct{ *scopedAdminWriteTestRepo }

func (r *scopedCodexTestRepo) Create(ctx context.Context, a *Account) error {
	if err := r.scopedAdminWriteTestRepo.Create(ctx, a); err != nil {
		return err
	}
	return r.Update(ctx, a)
}
func (r *scopedCodexTestRepo) Update(ctx context.Context, a *Account) error {
	if err := r.scopedAdminWriteTestRepo.Update(ctx, a); err != nil {
		return err
	}
	extra, err := json.Marshal(a.Extra)
	if err != nil {
		return err
	}
	_, err = r.login.sql(ctx).ExecContext(ctx, `UPDATE accounts SET extra=$2,rate_multiplier=$3 WHERE id=$1`, a.ID, string(extra), a.BillingRateMultiplier())
	return err
}
func (r *scopedCodexTestRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	a, err := r.scopedAdminWriteTestRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	rows, err := r.login.sql(ctx).QueryContext(ctx, `SELECT extra,rate_multiplier FROM accounts WHERE id=$1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var extra []byte
	var rate float64
	if rows.Next() {
		if err = rows.Scan(&extra, &rate); err != nil {
			return nil, err
		}
	}
	if err = json.Unmarshal(extra, &a.Extra); err != nil {
		return nil, err
	}
	a.RateMultiplier = &rate
	return a, rows.Err()
}

func TestScopedCodexWriteAuthorizationAndTransactions(t *testing.T) {
	login, db := scopedAdminAccountTestDB(t)
	_, err := db.Exec(`CREATE TABLE account_groups(account_id BIGINT,group_id BIGINT);
 ALTER TABLE accounts ADD COLUMN extra JSONB DEFAULT '{}', ADD COLUMN rate_multiplier DOUBLE PRECISION DEFAULT 1;
 INSERT INTO accounts(id,name,platform,type,credentials,extra) VALUES
 (101,'共享','openai','oauth','{"access_token":"old","refresh_token":"keep"}','{"quota_used":7}'),
 (102,'其他组','openai','oauth','{}','{}'),(103,'API','openai','apikey','{}','{}'),(104,'其他平台','anthropic','oauth','{}','{}');
 INSERT INTO account_groups VALUES(101,1),(101,2),(102,2),(103,1),(104,1);`)
	require.NoError(t, err)
	repo := &scopedCodexTestRepo{&scopedAdminWriteTestRepo{login: login}}
	admin := &adminServiceImpl{accountRepo: repo, groupRepo: &scopedAdminWriteTestGroups{}, entClient: dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))}
	ctxFor := func(permissions ...string) context.Context {
		return adminaccess.WithPrincipal(context.Background(), &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: permissions, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}, Platforms: []string{"openai"}}}}})
	}
	all := []string{"accounts.create", "accounts.credentials", "accounts.schedule", "accounts.network", "accounts.groups"}
	input := func() *CreateAccountInput {
		rate := 1.0
		return &CreateAccountInput{Name: "导入创建", Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: []int64{1}, Credentials: map[string]any{"access_token": "new", "refresh_token": "refresh", "email": "test@example.com", "chatgpt_account_id": "acct"}, Extra: map[string]any{"import_source": "codex_session", "codex_fingerprint_mode": "off"}, Concurrency: 40, RateMultiplier: &rate}
	}
	for _, missing := range []string{"accounts.create", "accounts.credentials", "accounts.schedule", "accounts.network"} {
		t.Run("缺少"+missing, func(t *testing.T) {
			permissions := []string{}
			for _, p := range all {
				if p != missing {
					permissions = append(permissions, p)
				}
			}
			_, err := admin.ScopedAdminCreateCodex(ctxFor(permissions...), input())
			require.Equal(t, 403, infraerrors.Code(err))
		})
	}
	ctx := ctxFor(all...)
	for _, groups := range [][]int64{{1, 2}, {2}, {}, {0}} {
		v := input()
		v.GroupIDs = groups
		_, err = admin.ScopedAdminCreateCodex(ctx, v)
		require.Equal(t, 403, infraerrors.Code(err))
	}
	v := input()
	v.Type = AccountTypeAPIKey
	_, err = admin.ScopedAdminCreateCodex(ctx, v)
	require.Equal(t, 400, infraerrors.Code(err))
	v = input()
	rate := 0.0
	v.RateMultiplier = &rate
	_, err = admin.ScopedAdminCreateCodex(ctx, v)
	require.Equal(t, 403, infraerrors.Code(err))
	v = input()
	v.Extra["quota_used"] = 0
	_, err = admin.ScopedAdminCreateCodex(ctx, v)
	require.Equal(t, 400, infraerrors.Code(err))
	v = input()
	v.Credentials["model_mapping"] = map[string]any{"a": "b"}
	_, err = admin.ScopedAdminCreateCodex(ctx, v)
	require.Equal(t, 403, infraerrors.Code(err))
	repo.failBind = true
	_, err = admin.ScopedAdminCreateCodex(ctx, input())
	require.Error(t, err)
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM accounts WHERE name='导入创建'`).Scan(&count))
	require.Zero(t, count)
	repo.failBind = false
	created, err := admin.ScopedAdminCreateCodex(ctx, input())
	require.NoError(t, err)
	saved, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "new", saved.Credentials["access_token"])
	require.Equal(t, "off", saved.Extra["codex_fingerprint_mode"])
	require.Equal(t, []int64{1}, saved.GroupIDs)
	// 不需要账户读取权限；无分组授权显式控制，不自动绑定默认组。
	ungrouped := input()
	ungrouped.GroupIDs = []int64{}
	principal := *adminaccess.FromContext(ctx)
	principal.Policy.Rules = append([]adminaccess.Rule(nil), principal.Policy.Rules...)
	principal.Policy.Rules[0].IncludeUngrouped = true
	created, err = admin.ScopedAdminCreateCodex(adminaccess.WithPrincipal(context.Background(), &principal), ungrouped)
	require.NoError(t, err)
	saved, err = repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Empty(t, saved.GroupIDs)
	groups := []int64{1}
	update := &UpdateAccountInput{Credentials: map[string]any{"access_token": "changed"}, Extra: map[string]any{"import_source": "codex_session"}, GroupIDs: &groups}
	called := 0
	prepare := func(current *Account, safe *UpdateAccountInput) error {
		called++
		require.Equal(t, "old", current.Credentials["access_token"])
		safe.Credentials = map[string]any{"access_token": "changed", "refresh_token": current.Credentials["refresh_token"]}
		safe.Extra = map[string]any{"import_source": "codex_session", "quota_used": current.Extra["quota_used"]}
		return nil
	}
	for _, id := range []int64{102, 103, 104, 999} {
		_, err = admin.ScopedAdminUpdateCodex(ctx, id, update, prepare)
		require.Equal(t, 404, infraerrors.Code(err))
	}
	require.Zero(t, called)
	_, err = admin.ScopedAdminUpdateCodex(ctxFor("accounts.credentials", "accounts.schedule"), 101, update, prepare)
	require.Equal(t, 403, infraerrors.Code(err))
	require.Zero(t, called)
	_, err = admin.ScopedAdminUpdateCodex(ctx, 101, update, func(_ *Account, _ *UpdateAccountInput) error { return infraerrors.Conflict("CHANGED", "身份已变") })
	require.Equal(t, 409, infraerrors.Code(err))
	_, err = admin.ScopedAdminUpdateCodex(ctx, 101, update, prepare)
	require.NoError(t, err)
	require.Equal(t, 1, called)
	saved, err = repo.GetByID(ctx, 101)
	require.NoError(t, err)
	require.Equal(t, "changed", saved.Credentials["access_token"])
	require.Equal(t, "keep", saved.Credentials["refresh_token"])
	require.Equal(t, float64(7), saved.Extra["quota_used"])
	require.Equal(t, []int64{1, 2}, saved.GroupIDs)
	update.RateMultiplier = &rate
	_, err = admin.ScopedAdminUpdateCodex(ctx, 101, update, prepare)
	require.Equal(t, 403, infraerrors.Code(err))
	require.Equal(t, 1, called)
}
