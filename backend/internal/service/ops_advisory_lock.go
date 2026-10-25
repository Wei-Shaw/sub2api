package service

import (
	"context"
	"database/sql"
	"fmt"
	"hash/fnv"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"
)

func hashAdvisoryLockID(key string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return int64(h.Sum64())
}

func tryAcquireDBAdvisoryLock(ctx context.Context, db *sql.DB, lockID int64) (func(), bool) {
	release, acquired, _ := tryAcquireDBAdvisoryLockWithError(ctx, db, lockID)
	return release, acquired
}

func tryAcquireDBAdvisoryLockWithError(ctx context.Context, db *sql.DB, lockID int64) (func(), bool, error) {
	if db == nil {
		return nil, false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if sqlDBIsMySQL(db) {
		return tryAcquireMySQLNamedLock(ctx, db, lockID)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("open advisory-lock connection: %w", err)
	}

	acquired := false
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&acquired); err != nil {
		_ = conn.Close()
		return nil, false, fmt.Errorf("query advisory lock: %w", err)
	}
	if !acquired {
		_ = conn.Close()
		return nil, false, nil
	}

	release := func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(unlockCtx, "SELECT pg_advisory_unlock($1)", lockID)
		_ = conn.Close()
	}
	return release, true, nil
}

// ---------------------------------------------------------------------------
// MySQL 方言实现（见 internal/repository/DIALECT_MYSQL_GUIDE.md「行锁与咨询锁」）。
// ---------------------------------------------------------------------------

// sqlDBIsMySQL 报告 *sql.DB 底层驱动是否为 go-sql-driver/mysql。
// service 包不能 import repository 取 IsMySQL()（repository 已 import service，
// 会构成 import cycle），*sql.DB 场景按驱动类型探测
// （先例：securityaudit 包 auditDBIsMySQL）。
func sqlDBIsMySQL(db *sql.DB) bool {
	if db == nil {
		return false
	}
	_, ok := db.Driver().(*mysql.MySQLDriver)
	return ok
}

// tryAcquireMySQLNamedLock 是 tryAcquireDBAdvisoryLockWithError 的 MySQL 实现：
// pg_try_advisory_lock/pg_advisory_unlock → GET_LOCK/RELEASE_LOCK（连接级命名锁，
// 仍以独立连接持有，参考 migrations_runner.go 的 mysqlGetLock 与
// scheduler_outbox_repo.go 先例）。GET_LOCK(name, 0) 为非阻塞尝试，返回
// 1=成功 / 0=被占用（对齐 pg_try_advisory_lock 语义）；锁名取 lockID 的十进制
// 字符串（hashAdvisoryLockID 输出 int64，映射一一对应，保证与 PG 版相同的
// 键空间互斥）。
func tryAcquireMySQLNamedLock(ctx context.Context, db *sql.DB, lockID int64) (func(), bool, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("open advisory-lock connection: %w", err)
	}
	name := strconv.FormatInt(lockID, 10)
	var locked sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", name).Scan(&locked); err != nil {
		_ = conn.Close()
		return nil, false, fmt.Errorf("query advisory lock: %w", err)
	}
	if !locked.Valid || locked.Int64 != 1 {
		_ = conn.Close()
		return nil, false, nil
	}

	release := func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(unlockCtx, "SELECT RELEASE_LOCK(?)", name)
		_ = conn.Close()
	}
	return release, true, nil
}
