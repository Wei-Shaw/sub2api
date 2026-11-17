//go:build unit

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestProxySampleSQLRandomizesWholePoolBeforeLimit(t *testing.T) {
	matcher := sqlmock.QueryMatcherFunc(func(expected, actual string) error {
		if !strings.Contains(actual, `"status"`) || !strings.Contains(actual, `"expires_at"`) {
			return fmt.Errorf("缺少可用池过滤: %s", actual)
		}
		if strings.Contains(actual, `"username"`) || strings.Contains(actual, `"password"`) {
			return fmt.Errorf("抽样不应查询认证字段: %s", actual)
		}
		if expected == "sample" {
			if !strings.Contains(actual, "ORDER BY RANDOM() LIMIT 50") || strings.Contains(actual, "OFFSET") || strings.Contains(actual, "ORDER BY \"proxies\".\"id\"") {
				return fmt.Errorf("未在全池随机后限制50: %s", actual)
			}
			if strings.Count(actual, "SELECT") != 1 {
				return fmt.Errorf("不得从预先截断的子查询抽样: %s", actual)
			}
		}
		return nil
	})
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	mock.ExpectQuery("count").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(20000))
	mock.ExpectQuery("sample").WillReturnRows(sqlmock.NewRows([]string{"id", "name", "host", "port", "status"}).AddRow(1234, "候选", "192.0.2.1", 8080, "active"))
	items, total, err := newProxyRepositoryWithSQL(client, db).SampleOptions(context.Background(), "region")
	require.NoError(t, err)
	require.EqualValues(t, 20000, total)
	require.Len(t, items, 1)
	require.EqualValues(t, 1234, items[0].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProxySamplePostgresLargePoolAndEligibility(t *testing.T) {
	db := scopedAdminKeyTestDB(t)
	_, err := db.Exec(`CREATE TABLE proxies(
 id BIGINT PRIMARY KEY,name TEXT DEFAULT 'pool',protocol TEXT DEFAULT 'http',host TEXT DEFAULT '192.0.2.1',port INT DEFAULT 8080,
 username TEXT DEFAULT 'SECRET_USER',password TEXT DEFAULT 'SECRET_PASSWORD',status TEXT DEFAULT 'active',
 expires_at TIMESTAMPTZ,deleted_at TIMESTAMPTZ,created_at TIMESTAMPTZ DEFAULT NOW(),updated_at TIMESTAMPTZ DEFAULT NOW(),
 fallback_mode TEXT DEFAULT 'none',backup_proxy_id BIGINT,expiry_warn_days INT DEFAULT 7);
 INSERT INTO proxies(id) SELECT generate_series(1,20005);
 UPDATE proxies SET status='inactive' WHERE id=1;
 UPDATE proxies SET status='inactive' WHERE id=2;
 UPDATE proxies SET expires_at=NOW()-INTERVAL '1 day' WHERE id=3;
 UPDATE proxies SET deleted_at=NOW() WHERE id=4;`)
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	repo := newProxyRepositoryWithSQL(client, db)
	for i := 0; i < 2; i++ {
		items, total, err := repo.SampleOptions(context.Background(), "")
		require.NoError(t, err)
		require.EqualValues(t, 20001, total)
		require.Len(t, items, 50)
		seen := map[int64]bool{}
		for _, p := range items {
			require.Greater(t, p.ID, int64(4))
			require.False(t, seen[p.ID])
			seen[p.ID] = true
			require.Empty(t, p.Username)
			require.Empty(t, p.Password)
		}
	}
	// 用唯一匹配 ID 证明老记录在全池内可达，不依赖随机概率作断言。
	_, err = db.Exec(`UPDATE proxies SET name='only-old-record' WHERE id=5`)
	require.NoError(t, err)
	items, total, err := repo.SampleOptions(context.Background(), "only-old-record")
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	require.EqualValues(t, 5, items[0].ID)
	items, total, err = repo.SampleOptions(context.Background(), "SECRET_USER")
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, items)
	page1, _, err := repo.ListOptions(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 50}, "")
	require.NoError(t, err)
	page2, _, err := repo.ListOptions(context.Background(), pagination.PaginationParams{Page: 2, PageSize: 50}, "")
	require.NoError(t, err)
	require.EqualValues(t, 20005, page1[0].ID)
	require.EqualValues(t, 19955, page2[0].ID)
	// 小池全部返回，空池返回空切片，均不填入无效代理。
	_, err = db.Exec(`UPDATE proxies SET status='inactive' WHERE id>10`)
	require.NoError(t, err)
	items, total, err = repo.SampleOptions(context.Background(), "")
	require.NoError(t, err)
	require.EqualValues(t, 6, total)
	require.Len(t, items, 6)
	_, err = db.Exec(`UPDATE proxies SET status='inactive'`)
	require.NoError(t, err)
	items, total, err = repo.SampleOptions(context.Background(), "")
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, items)
}
