//go:build unit

package service

import (
	"context"
	"database/sql"
	"fmt"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
	"time"
)

type scopedTestSQL interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}
type scopedTestDB struct{ db *sql.DB }

func (s *scopedTestDB) sql(ctx context.Context) scopedTestSQL {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return tx.Client()
	}
	return s.db
}
func scopedAdminAccountTestDB(t *testing.T) (*scopedTestDB, *sql.DB) {
	t.Helper()
	dsn := os.Getenv("SCOPED_ADMIN_TEST_DSN")
	if dsn == "" {
		t.Skip("需要独立 SCOPED_ADMIN_TEST_DSN")
	}
	base, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	name := fmt.Sprintf("scoped_accounts_%d", time.Now().UnixNano())
	_, err = base.Exec(`CREATE SCHEMA ` + name)
	require.NoError(t, err)
	db, err := sql.Open("postgres", dsn+" search_path="+name)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close(); _, _ = base.Exec(`DROP SCHEMA ` + name + ` CASCADE`); _ = base.Close() })
	_, err = db.Exec(`CREATE TABLE accounts(id BIGSERIAL PRIMARY KEY,name TEXT DEFAULT '',platform TEXT DEFAULT 'openai',type TEXT DEFAULT 'oauth',credentials JSONB DEFAULT '{}',status TEXT DEFAULT 'active',schedulable BOOLEAN DEFAULT true,error_message TEXT,temp_unschedulable_until TIMESTAMPTZ,temp_unschedulable_reason TEXT,deleted_at TIMESTAMPTZ,parent_account_id BIGINT,proxy_id BIGINT,expires_at TIMESTAMPTZ,updated_at TIMESTAMPTZ DEFAULT NOW());
 CREATE TABLE proxies(id BIGSERIAL PRIMARY KEY,protocol TEXT,host TEXT,port INT,username TEXT,password TEXT,status TEXT DEFAULT 'active',expires_at TIMESTAMPTZ,deleted_at TIMESTAMPTZ);`)
	require.NoError(t, err)
	return &scopedTestDB{db: db}, db
}
