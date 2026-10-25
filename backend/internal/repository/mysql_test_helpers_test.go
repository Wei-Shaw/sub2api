// 本文件不设 build 标签：skipOnMySQL 被无标签的 sqlmock 测试引用，需在
// unit / integration / 无标签（go test ./...）所有编译形态下可见；
// resetMySQLSchema 仅在 integration 运行被调用，其它形态下只是未被引用。

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"strconv"
	"testing"
	"time"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

// dbTimePrecision 返回当前方言时间戳列的存储精度：PostgreSQL timestamptz 为微秒；
// MySQL 按规范统一 DATETIME(3) 为毫秒。测试夹具用 time.Now().Truncate(dbTimePrecision())
// 构造可完整往返的时间值（PG 分支与原 Truncate(time.Microsecond) 行为一致）。
func dbTimePrecision() time.Duration {
	if IsMySQL() {
		return time.Millisecond
	}
	return time.Microsecond
}

// ph 返回测试夹具 SQL 中第 n 个位置参数的占位符：PostgreSQL "$n"、MySQL "?"。
// 用于把夹具里写死的 PG 占位符改为双方言；参数顺序与个数两侧必须一致
// （postgres 分支下产出的字符串与原夹具字节相同，PG 行为不变）。
func ph(n int) string {
	if IsMySQL() {
		return "?"
	}
	return "$" + strconv.Itoa(n)
}

// truncateTableSQL 返回清空表并重置自增主键的方言语句：
// PostgreSQL "TRUNCATE <t> RESTART IDENTITY"；MySQL "TRUNCATE TABLE <t>"
// （MySQL 的 TRUNCATE TABLE 本身就会重置 AUTO_INCREMENT）。
func truncateTableSQL(table string) string {
	if IsMySQL() {
		return "TRUNCATE TABLE " + table
	}
	return "TRUNCATE " + table + " RESTART IDENTITY"
}

// resetMySQLSchema 清空目标库的全部表与存储过程（仅限专用测试库！）。
// 供 MySQL 迁移验证（integration_mysql）与 MySQL 集成 harness（integration）共用。
// 全程使用单一专用连接：SET FOREIGN_KEY_CHECKS=0 是会话级设置，跨连接无效。
func resetMySQLSchema(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire reset connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=0"); err != nil {
		return err
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), "SET FOREIGN_KEY_CHECKS=1") }()

	rows, err := conn.QueryContext(ctx,
		"SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE()")
	if err != nil {
		return err
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return err
		}
		tables = append(tables, name)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, table := range tables {
		if _, err := conn.ExecContext(ctx, "DROP TABLE IF EXISTS `"+table+"`"); err != nil {
			return fmt.Errorf("drop table %s: %w", table, err)
		}
	}

	procRows, err := conn.QueryContext(ctx,
		"SELECT routine_name FROM information_schema.routines WHERE routine_schema = DATABASE() AND routine_type = 'PROCEDURE'")
	if err != nil {
		return err
	}
	var procs []string
	for procRows.Next() {
		var name string
		if err := procRows.Scan(&name); err != nil {
			_ = procRows.Close()
			return err
		}
		procs = append(procs, name)
	}
	_ = procRows.Close()
	for _, p := range procs {
		if _, err := conn.ExecContext(ctx, "DROP PROCEDURE IF EXISTS `"+p+"`"); err != nil {
			return fmt.Errorf("drop procedure %s: %w", p, err)
		}
	}

	log.Printf("mysql schema reset: dropped %d tables, %d procedures", len(tables), len(procs))
	return nil
}

// skipOnMySQL 在 MySQL 方言运行（SUB2API_TEST_DB=mysql）时跳过测试。
// 用于两类场景：sqlmock 模拟 PG 语句形状的测试、使用 PG 专属 DDL/语法夹具的测试。
// 默认（postgres）运行完全不受影响。
func skipOnMySQL(t testing.TB) {
	if IsMySQL() {
		t.Skip("test uses PostgreSQL-specific SQL shapes; not applicable to MySQL dialect")
	}
}

// testSQLDB 抽象 *sql.DB / *sql.Tx 在测试夹具里共用的执行方法。
type testSQLDB interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// migrationsFSForDialect 返回当前方言对应的迁移文件集：
// PostgreSQL 用 migrations.FS，MySQL 用 migrations.MySQLFS（mysql/ 子目录同名镜像）。
// 迁移等价性测试用它按方言加载同一编号迁移的对应版本。
func migrationsFSForDialect() fs.FS {
	if IsMySQL() {
		return dbmigrations.MySQLFS
	}
	return dbmigrations.FS
}

// readMigrationSQLForDialect 读取当前方言下指定迁移文件的内容（见 migrationsFSForDialect）。
func readMigrationSQLForDialect(t testing.TB, name string) string {
	t.Helper()
	content, err := fs.ReadFile(migrationsFSForDialect(), name)
	require.NoError(t, err, "read migration %s", name)
	return string(content)
}

// insertReturningID 执行单条 INSERT 并返回新生成的自增主键。
// query 不携带 RETURNING 子句：PostgreSQL 分支追加 " RETURNING id"（与原夹具
// "INSERT ... RETURNING id" 行为一致）；MySQL 无 RETURNING，按 DIALECT_MYSQL_GUIDE
// 规则 1 用 ExecContext + LastInsertId 等价实现。
func insertReturningID(ctx context.Context, db testSQLDB, query string, args ...any) (int64, error) {
	if IsMySQL() {
		res, err := db.ExecContext(ctx, query, args...)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	var id int64
	if err := db.QueryRowContext(ctx, query+" RETURNING id", args...).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// groupsTableRef 返回 groups 表在 SQL 语句中的方言语法引用：
// groups 是 MySQL 8 保留字需反引号；PostgreSQL 原样（见 DIALECT_MYSQL_GUIDE）。
func groupsTableRef() string {
	if IsMySQL() {
		return "`groups`"
	}
	return "groups"
}

// jsonTextExpr 返回把 JSON 列文本化的方言表达式：
// PostgreSQL "expr::text"；MySQL "CAST(expr AS CHAR)"。
func jsonTextExpr(expr string) string {
	if IsMySQL() {
		return "CAST(" + expr + " AS CHAR)"
	}
	return expr + "::text"
}

// jsonExtractTextExpr 返回「取 JSON 顶层键的文本值」的方言表达式：
// PostgreSQL `col->>'key'`；MySQL `JSON_UNQUOTE(JSON_EXTRACT(col, '$.key'))`。
func jsonExtractTextExpr(column, key string) string {
	if IsMySQL() {
		return "JSON_UNQUOTE(JSON_EXTRACT(" + column + ", '$." + key + "'))"
	}
	return column + "->>'" + key + "'"
}

// jsonHasPathExpr 返回「JSON 是否含顶层键」的方言表达式（结果为布尔值）：
// PostgreSQL `col ? 'key'`；MySQL `JSON_CONTAINS_PATH(col, 'one', '$.key')`。
func jsonHasPathExpr(column, key string) string {
	if IsMySQL() {
		return "JSON_CONTAINS_PATH(" + column + ", 'one', '$." + key + "')"
	}
	return column + " ? '" + key + "'"
}

// deleteAuthCacheOutboxByKeySQL 返回按 API key 明文清理失效 outbox 行的方言语句：
// PostgreSQL 用 encode(sha256(convert_to($1,'UTF8')),'hex') 计算 cache_key；
// MySQL 用 SHA2(?, 256)（同为 UTF-8 字节 SHA-256 的小写 hex）。
func deleteAuthCacheOutboxByKeySQL() string {
	if IsMySQL() {
		return "DELETE FROM auth_cache_invalidation_outbox WHERE cache_key = SHA2(?, 256)"
	}
	return "DELETE FROM auth_cache_invalidation_outbox WHERE cache_key = encode(sha256(convert_to($1, 'UTF8')), 'hex')"
}

// truncateSchedulerOutboxSQL 返回在测试事务内清空 scheduler_outbox 的方言语句。
// PG 用 TRUNCATE（可随测试事务回滚）；MySQL 的 TRUNCATE TABLE 是 DDL，会隐式提交
// 当前事务，把此前在本测试事务里创建的夹具行泄漏到共享库（破坏其它测试的
// 回滚隔离），改用事务性的 DELETE FROM。
func truncateSchedulerOutboxSQL() string {
	if IsMySQL() {
		return "DELETE FROM scheduler_outbox"
	}
	return "TRUNCATE scheduler_outbox"
}

// deleteByIDSQL 返回按主键删除单行的方言语句。
// groups 在 MySQL 8 是保留字，统一反引号（对其它表名无影响）。
func deleteByIDSQL(table string) string {
	if IsMySQL() {
		return "DELETE FROM `" + table + "` WHERE id = ?"
	}
	return "DELETE FROM " + table + " WHERE id = $1"
}
