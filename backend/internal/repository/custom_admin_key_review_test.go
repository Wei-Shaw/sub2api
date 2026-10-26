//go:build unit

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/schema"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/migrate"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestScopedAdminReviewGroupPriorityAndStatus(t *testing.T) {
	dsn := os.Getenv("SCOPED_ADMIN_TEST_DSN")
	if dsn == "" {
		t.Skip("需要独立 SCOPED_ADMIN_TEST_DSN")
	}
	base, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	name := fmt.Sprintf("scoped_review_%d", time.Now().UnixNano())
	_, err = base.Exec(`CREATE SCHEMA ` + name)
	require.NoError(t, err)
	db, err := sql.Open("postgres", dsn+" search_path="+name)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close(); _, _ = base.Exec(`DROP SCHEMA ` + name + ` CASCADE`); _ = base.Close() })
	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	require.NoError(t, migrate.Create(context.Background(), migrate.NewSchema(driver), []*schema.Table{migrate.ProxiesTable, migrate.AccountsTable, migrate.GroupsTable, migrate.AccountGroupsTable}, migrate.WithForeignKeys(false)))
	_, err = db.Exec(`CREATE TABLE scheduler_outbox(id BIGSERIAL PRIMARY KEY,event_type TEXT,account_id BIGINT,group_id BIGINT,payload JSONB,dedup_key TEXT,created_at TIMESTAMPTZ DEFAULT NOW());CREATE UNIQUE INDEX review_outbox_dedup ON scheduler_outbox(dedup_key) WHERE dedup_key IS NOT NULL`)
	require.NoError(t, err)
	cache := &scopedReviewSchedulerCache{accounts: make(map[int64]*service.Account)}
	repo := NewAdminAccountRepository(client, db, cache)
	groups := NewAdminGroupRepository(client, db)
	admin := service.NewAdminService(nil, nil, groups, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, client, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	scoped := admin.(service.ScopedAdminAccounts)
	ctx := context.Background()
	a, err := client.Group.Create().SetName("授权组").Save(ctx)
	require.NoError(t, err)
	b, err := client.Group.Create().SetName("未授权组").Save(ctx)
	require.NoError(t, err)
	c, err := client.Group.Create().SetName("新增授权组").Save(ctx)
	require.NoError(t, err)
	account, err := admin.CreateAccount(ctx, &service.CreateAccountInput{Name: "共享账户", Platform: "openai", Type: "apikey", Credentials: map[string]any{"api_key": "test"}, GroupIDs: []int64{a.ID, b.ID}, Concurrency: 1, Priority: 1})
	require.NoError(t, err)
	originalTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	_, err = db.Exec(`UPDATE account_groups SET priority=73,created_at=$2 WHERE group_id=$1`, b.ID, originalTime)
	require.NoError(t, err)
	principal := &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.groups", "accounts.status"}, GroupIDs: []int64{a.ID, c.ID}, AccountTypes: []string{"apikey"}}}}}
	ctx = adminaccess.WithPrincipal(ctx, principal)
	t.Run("非法状态不得落库", func(t *testing.T) {
		for _, raw := range []string{`"garbage"`, `""`, `null`, `12`, `" active "`} {
			_, err := scoped.ScopedAdminWrite(ctx, account.ID, "", map[string]json.RawMessage{"status": json.RawMessage(raw)}, nil)
			require.Equal(t, 400, infraerrors.Code(err), raw)
		}
		stored, err := repo.GetByID(ctx, account.ID)
		require.NoError(t, err)
		require.Equal(t, "active", stored.Status)
		for _, status := range []string{"inactive", "error", "active"} {
			raw, _ := json.Marshal(status)
			_, err := scoped.ScopedAdminWrite(ctx, account.ID, "", map[string]json.RawMessage{"status": raw}, nil)
			require.NoError(t, err)
		}
	})
	t.Run("修改授权关联保留其他关联元数据", func(t *testing.T) {
		for _, ids := range [][]int64{{a.ID, c.ID}, {c.ID}, {}} {
			raw, _ := json.Marshal(ids)
			_, err := scoped.ScopedAdminWrite(ctx, account.ID, "", map[string]json.RawMessage{"group_ids": raw}, nil)
			require.NoError(t, err)
			var priority int
			var created time.Time
			require.NoError(t, db.QueryRow(`SELECT priority,created_at FROM account_groups WHERE account_id=$1 AND group_id=$2`, account.ID, b.ID).Scan(&priority, &created))
			require.Equal(t, 73, priority, "未授权分组调度优先级不能变化")
			require.True(t, originalTime.Equal(created), "未授权关联不应被重建")
		}
	})
	t.Run("受限创建绑定与失败回滚", func(t *testing.T) {
		createCtx := adminaccess.WithPrincipal(context.Background(), &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.create", "accounts.credentials"}, GroupIDs: []int64{a.ID, 999999}, AccountTypes: []string{"apikey"}}}}})
		for _, groupID := range []int64{a.ID, 999999} {
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(`{"name":"创建验收-%d","platform":"openai","type":"apikey","credentials":{"api_key":"test"},"group_ids":[%d]}`, groupID, groupID)), &fields))
			created, createErr := scoped.ScopedAdminWrite(createCtx, 0, "", fields, nil)
			if groupID == a.ID {
				require.NoError(t, createErr)
				stored, getErr := repo.GetByID(createCtx, created.ID)
				require.NoError(t, getErr)
				require.Equal(t, []int64{a.ID}, stored.GroupIDs)
			} else {
				require.Error(t, createErr)
				var count int
				require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM accounts WHERE name=$1`, fmt.Sprintf("创建验收-%d", groupID)).Scan(&count))
				require.Zero(t, count)
			}
		}
	})
	t.Run("删除提交后立即清缓存且失败保留缓存", func(t *testing.T) {
		_, err := db.Exec(`CREATE TABLE scheduled_test_plans(account_id BIGINT);
  CREATE FUNCTION reject_review_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION '模拟提交失败'; RETURN NULL; END $$;
  CREATE CONSTRAINT TRIGGER reject_review_delete AFTER UPDATE ON accounts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN (NEW.name='删除回滚' AND NEW.deleted_at IS NOT NULL) EXECUTE FUNCTION reject_review_delete();`)
		require.NoError(t, err)
		deleteCtx := adminaccess.WithPrincipal(context.Background(), &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.delete"}, GroupIDs: []int64{a.ID}, AccountTypes: []string{"apikey"}}}}})
		for _, name := range []string{"删除回滚", "删除成功"} {
			created, err := admin.CreateAccount(context.Background(), &service.CreateAccountInput{Name: name, Platform: "openai", Type: "apikey", Credentials: map[string]any{"api_key": "test"}, GroupIDs: []int64{a.ID}, Concurrency: 1, Priority: 1})
			require.NoError(t, err)
			cache.accounts[created.ID] = created
			cache.beforeDelete = func(id int64) {
				var deleted bool
				require.NoError(t, db.QueryRow(`SELECT deleted_at IS NOT NULL FROM accounts WHERE id=$1`, id).Scan(&deleted))
				require.True(t, deleted, "缓存只能在提交后删除")
			}
			_, err = scoped.ScopedAdminWrite(deleteCtx, created.ID, "accounts.delete", nil, nil)
			if name == "删除回滚" {
				require.ErrorContains(t, err, "模拟提交失败")
				require.Contains(t, cache.accounts, created.ID)
				_, err = repo.GetByID(context.Background(), created.ID)
				require.NoError(t, err)
			} else {
				require.NoError(t, err)
				require.NotContains(t, cache.accounts, created.ID, "返回成功前应清除已删除账户的调度快照")
				require.Contains(t, cache.deleted, created.ID)
			}
		}
	})

}

type scopedRotationBarrier struct {
	service.ScopedAdminKeyRepository
	arrived chan struct{}
	release chan struct{}
}

func (r *scopedRotationBarrier) GetScopedAdminKey(ctx context.Context, id string) (*service.ScopedAdminKey, error) {
	item, err := r.ScopedAdminKeyRepository.GetScopedAdminKey(ctx, id)
	r.arrived <- struct{}{}
	select {
	case <-r.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return item, err
}
func TestScopedAdminReviewConcurrentRotation(t *testing.T) {
	db := scopedAdminKeyTestDB(t)
	repo := NewScopedAdminKeyRepository(db)
	settings := service.NewSettingService(nil, nil)
	settings.SetScopedAdminKeyRepository(repo)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, item, err := settings.CreateScopedAdminKey(ctx, service.ScopedAdminKeyInput{Name: "并发轮换", Enabled: true}, 9)
	require.NoError(t, err)
	barrier := &scopedRotationBarrier{ScopedAdminKeyRepository: repo, arrived: make(chan struct{}, 2), release: make(chan struct{})}
	settings.SetScopedAdminKeyRepository(barrier)
	type result struct {
		key string
		err error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key, _, e := settings.RotateScopedAdminKey(ctx, item.ID)
			results <- result{key, e}
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-barrier.arrived:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	close(barrier.release)
	wg.Wait()
	close(results)
	settings.SetScopedAdminKeyRepository(repo)
	successes, conflicts := 0, 0
	for result := range results {
		if result.err == nil {
			successes++
			_, e := settings.AuthenticateScopedAdminKey(ctx, result.key, "192.0.2.1")
			require.NoError(t, e, "成功返回的密钥必须仍有效")
		} else {
			conflicts++
			require.Equal(t, 409, infraerrors.Code(result.err))
			require.Empty(t, result.key)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
}

type scopedReviewSchedulerCache struct {
	service.SchedulerCache
	accounts     map[int64]*service.Account
	deleted      []int64
	beforeDelete func(int64)
}

func (c *scopedReviewSchedulerCache) SetAccount(_ context.Context, a *service.Account) error {
	c.accounts[a.ID] = a
	return nil
}
func (c *scopedReviewSchedulerCache) DeleteAccount(_ context.Context, id int64) error {
	if c.beforeDelete != nil {
		c.beforeDelete(id)
	}
	delete(c.accounts, id)
	c.deleted = append(c.deleted, id)
	return nil
}
