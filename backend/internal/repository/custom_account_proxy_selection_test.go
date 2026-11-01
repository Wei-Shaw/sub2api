//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestAccountProxySelectionLargePoolAndTransactionalLeastUsed(t *testing.T) {
	db := scopedAdminKeyTestDB(t)
	_, err := db.Exec(`CREATE TABLE proxies(
 id BIGINT PRIMARY KEY,name TEXT DEFAULT '',protocol TEXT DEFAULT 'http',host TEXT DEFAULT '192.0.2.1',port INT DEFAULT 8080,
 username TEXT DEFAULT 'SECRET_USER',password TEXT DEFAULT 'SECRET_PASSWORD',status TEXT DEFAULT 'active',
 expires_at TIMESTAMPTZ,deleted_at TIMESTAMPTZ,created_at TIMESTAMPTZ DEFAULT NOW(),updated_at TIMESTAMPTZ DEFAULT NOW(),
 fallback_mode TEXT DEFAULT 'none',backup_proxy_id BIGINT,expiry_warn_days INT DEFAULT 7);
 CREATE TABLE accounts(id BIGINT PRIMARY KEY,proxy_id BIGINT,deleted_at TIMESTAMPTZ);
 INSERT INTO proxies(id) SELECT generate_series(1,20005);
 UPDATE proxies SET status='inactive' WHERE id=1;
 UPDATE proxies SET status='inactive' WHERE id=2;
 UPDATE proxies SET expires_at=NOW()-INTERVAL '1 hour' WHERE id=3;
 UPDATE proxies SET deleted_at=NOW() WHERE id=4;
 INSERT INTO accounts VALUES(1,5,NULL),(2,6,NULL),(3,7,NULL),(4,8,NOW());
`)
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	repo := newProxyRepositoryWithSQL(client, db)
	// 真实 2 万条池只返回指定页；不用账户聚合管理列表。
	items, page, err := repo.ListOptions(context.Background(), pagination.PaginationParams{Page: 2, PageSize: 100}, "")
	require.NoError(t, err)
	require.Len(t, items, 100)
	require.EqualValues(t, 20002, page.Total)
	require.EqualValues(t, 19905, items[0].ID)
	_, err = repo.SelectAccountProxy(context.Background(), "random")
	require.Error(t, err)
	tx, err := client.Tx(context.Background())
	require.NoError(t, err)
	defer tx.Rollback()
	ctx := dbent.NewTxContext(context.Background(), tx)
	selected, err := repo.SelectAccountProxy(ctx, "least_used")
	require.NoError(t, err)
	require.EqualValues(t, 8, selected, "软删账户不占用")
	_, err = tx.Client().ExecContext(ctx, `INSERT INTO accounts VALUES(10,$1,NULL)`, selected)
	require.NoError(t, err)
	// 第二个事务必须等待第一个事务绑定提交，随后观察到新的计数。
	started := make(chan struct{})
	done := make(chan struct {
		id  int64
		err error
	}, 1)
	go func() {
		other, err := client.Tx(context.Background())
		if err != nil {
			done <- struct {
				id  int64
				err error
			}{0, err}
			return
		}
		defer other.Rollback()
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		c = dbent.NewTxContext(c, other)
		close(started)
		id, err := repo.SelectAccountProxy(c, "least_used")
		done <- struct {
			id  int64
			err error
		}{id, err}
	}()
	<-started
	select {
	case result := <-done:
		t.Fatalf("选择不应越过尚未提交的分配: %+v", result)
	case <-time.After(30 * time.Millisecond):
	}
	require.NoError(t, tx.Commit())
	result := <-done
	require.NoError(t, result.err)
	require.EqualValues(t, 9, result.id)
	// 候选查询不能通过用户名搜索探测认证字段。
	scopedCtx := adminaccess.WithPrincipal(context.Background(), &adminaccess.Principal{Policy: adminaccess.Policy{GlobalPermissions: []string{"proxies.read"}}})
	matches, meta, err := repo.ListOptions(scopedCtx, pagination.PaginationParams{Page: 1, PageSize: 100}, "SECRET_USER")
	require.NoError(t, err)
	require.Empty(t, matches)
	require.Zero(t, meta.Total)
	matches, meta, err = repo.ListOptions(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 100}, "SECRET_USER")
	require.NoError(t, err)
	require.Empty(t, matches)
	require.Zero(t, meta.Total)
	// 限定唯一可用候选，确定性验证随机排除过期、停用及已删除。
	_, err = db.Exec(`UPDATE proxies SET status='inactive' WHERE id>9;UPDATE proxies SET status='inactive' WHERE id IN (5,6,7,8)`)
	require.NoError(t, err)
	tx, err = client.Tx(context.Background())
	require.NoError(t, err)
	ctx = dbent.NewTxContext(context.Background(), tx)
	selected, err = repo.SelectAccountProxy(ctx, "random")
	require.NoError(t, err)
	require.EqualValues(t, 9, selected)
	require.NoError(t, tx.Rollback())
	// 绑定数相同时按 ID 稳定选择。
	_, err = db.Exec(`UPDATE proxies SET status='active' WHERE id IN(5,6,7);UPDATE proxies SET status='inactive' WHERE id=9`)
	require.NoError(t, err)
	tx, err = client.Tx(context.Background())
	require.NoError(t, err)
	ctx = dbent.NewTxContext(context.Background(), tx)
	selected, err = repo.SelectAccountProxy(ctx, "least_used")
	require.NoError(t, err)
	require.EqualValues(t, 5, selected)
	require.NoError(t, tx.Rollback())
	_, err = db.Exec(`UPDATE proxies SET status='inactive'`)
	require.NoError(t, err)
	tx, err = client.Tx(context.Background())
	require.NoError(t, err)
	ctx = dbent.NewTxContext(context.Background(), tx)
	selected, err = repo.SelectAccountProxy(ctx, "random")
	require.NoError(t, err)
	require.Zero(t, selected)
	require.NoError(t, tx.Rollback())
	require.NoError(t, db.Ping(), "测试结束连接可用")
}
