// dialect.go 提供进程级 SQL 方言（PostgreSQL / MySQL）的统一管理。
//
// 背景与约束：
//   - ent ORM 部分自带方言能力（entsql.Open 时指定 dialect），无需本文件；
//   - 仓库层存在大量手写原生 SQL（经 *sql.DB / *sql.Tx 执行），这些语句
//     没有可靠的运行时方言来源（*sql.DB 抽象掉了底层驱动）。由于应用的
//     数据库类型在进程生命周期内不变，这里用进程级单例保存方言，
//     由 InitEnt 在建立连接时设置一次。
//   - 默认值为 postgres：保证既有单元测试（sqlite/ent）与既有 PG 行为
//     完全不受影响。
package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync/atomic"

	"entgo.io/ent/dialect"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/go-sql-driver/mysql"
)

// activeDialect 保存当前 SQL 方言（dialect.Postgres / dialect.MySQL）。
var activeDialect atomic.Value

// SetDialect 设置进程级 SQL 方言，仅在应用启动（InitEnt / 集成测试 harness）时调用。
func SetDialect(name string) {
	if name != dialect.Postgres && name != dialect.MySQL {
		return
	}
	activeDialect.Store(name)
}

// Dialect 返回当前 SQL 方言，未设置时默认 postgres。
func Dialect() string {
	if v, ok := activeDialect.Load().(string); ok && v != "" {
		return v
	}
	return dialect.Postgres
}

// IsMySQL 报告当前 SQL 方言是否为 MySQL。
func IsMySQL() bool {
	return Dialect() == dialect.MySQL
}

// Placeholders 生成 n 个 "?" 占位符组成的列表（如 n=3 → "?,?,?"），
// 用于把 PostgreSQL 的 `= ANY($1)` 数组参数展开为 MySQL 可用的 `IN (...)`。
func Placeholders(n int) string {
	if n <= 0 {
		return "NULL"
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// isMySQLUniqueViolation 精确检测 MySQL 唯一约束冲突（错误码 1062）。
func isMySQLUniqueViolation(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

// isMySQLTransientError 检测 MySQL 瞬态错误（连接建立/服务可用类，可重试）。
// 参考：2003 无法连接到服务器、2006 server has gone away、1040 连接数满。
// 锁等待超时（1205）与死锁（1213）属于业务语义层，不在此判定。
func isMySQLTransientError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		switch mysqlErr.Number {
		case 1040, 2003, 2006:
			return true
		}
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "server has gone away") ||
		strings.Contains(msg, "i/o timeout")
}

// sqlAtomicScope 提供「同一连接上顺序执行多条语句」的作用域，
// 用于把 PostgreSQL 的单条 UPDATE..RETURNING / 可写 CTE 拆解为
// MySQL 上的多条等价语句时保持原子语义：
//   - ctx 已携带 ent 事务 → 直接复用该事务的连接；
//   - 否则在 *sql.DB 上开启独立事务（sqlq 非 *sql.DB 时退化为顺序执行，
//     仅测试 mock 场景会出现）。
//
// 使用模式：
//
//	scope, err := beginSQLAtomicScope(ctx, client, sqlq)
//	if err != nil { return err }
//	defer scope.rollback() // commit 之后调用是无害的 no-op
//	... scope.exec / scope.query ...
//	if err := scope.commit(); err != nil { return err }
type sqlAtomicScope struct {
	tx     *sql.Tx
	exec   func(context.Context, string, ...any) (sql.Result, error)
	query  func(context.Context, string, ...any) (*sql.Rows, error)
	commit func() error
}

func beginSQLAtomicScope(ctx context.Context, client *dbent.Client, sqlq sqlExecutor) (*sqlAtomicScope, error) {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		q := tx.Client()
		return &sqlAtomicScope{
			exec:   q.ExecContext,
			query:  q.QueryContext,
			commit: func() error { return nil },
		}, nil
	}
	if db, ok := sqlq.(*sql.DB); ok {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		return &sqlAtomicScope{
			tx:     tx,
			exec:   tx.ExecContext,
			query:  tx.QueryContext,
			commit: func() error { return tx.Commit() },
		}, nil
	}
	return &sqlAtomicScope{
		exec:   sqlq.ExecContext,
		query:  sqlq.QueryContext,
		commit: func() error { return nil },
	}, nil
}

// rollback 回滚未提交的事务；已提交或未开启事务时为 no-op。
func (s *sqlAtomicScope) rollback() {
	if s != nil && s.tx != nil {
		_ = s.tx.Rollback()
	}
}
