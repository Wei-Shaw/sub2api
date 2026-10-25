//go:build integration_mysql

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/migrations"
	mysql "github.com/go-sql-driver/mysql"
)

// TestMySQLMigrationsApply 按顺序执行全部 MySQL 方言迁移文件，用于翻译验证迭代。
//
// 目标库通过环境变量 SUB2API_TEST_MYSQL_DSN 指定，例如：
//
//	user:pass@tcp(localhost:3306)/sub2api_test?parseTime=true
//
// 执行前会清空目标库中全部表/存储过程/触发器（仅限测试库使用！），
// 然后逐文件执行；单个文件失败不中断，最后汇总所有失败，便于批量修复。
func TestMySQLMigrationsApply(t *testing.T) {
	dsn := os.Getenv("SUB2API_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SUB2API_TEST_MYSQL_DSN not set; skipping mysql migration verification")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse SUB2API_TEST_MYSQL_DSN: %v", err)
	}
	if cfg.DBName == "" {
		t.Fatal("SUB2API_TEST_MYSQL_DSN must include a database name")
	}
	cfg.MultiStatements = true
	cfg.ParseTime = true
	// 与生产 DSN（config.MySQLConfig）保持一致，避免连接默认 collation
	// （utf8mb4_general_ci）与表列（utf8mb4_0900_ai_ci）混合导致的假阳性错误。
	if cfg.Collation == "" || cfg.Collation == "utf8mb4_general_ci" {
		cfg.Collation = "utf8mb4_0900_ai_ci"
	}

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}
	// 多语句批量执行中途失败会残留未读结果集；空闲连接置 0 让坏连接用后即弃，
	// 避免复用导致协议失步死锁（代价：每次建新连接，验证场景可接受）。
	db.SetMaxIdleConns(0)
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	if err := resetMySQLSchema(ctx, db); err != nil {
		t.Fatalf("reset schema: %v", err)
	}

	// 预建迁移记录表（与迁移执行器 applyMigrationsFSWithDialect 的行为一致），
	// 个别迁移（如 123）会引用该表判断历史迁移是否已应用。
	if _, err := db.ExecContext(ctx, schemaMigrationsTableDDLMySQL); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}

	// 默认容错：无 SUPER 权限的测试库跳过触发器 DDL（1419）；
	// SUB2API_TEST_MYSQL_STRICT_TRIGGERS=true 时严格验证全部语句（需 DBA 预开权限）。
	mysqlTriggerFailureTolerant = !strings.EqualFold(strings.TrimSpace(os.Getenv("SUB2API_TEST_MYSQL_STRICT_TRIGGERS")), "true")

	files, err := fs.Glob(migrations.MySQLFS, "*.sql")
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	sort.Strings(files)

	type migrationFailure struct {
		name string
		err  error
	}
	var failures []migrationFailure
	applied := 0
	for _, name := range files {
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		content, err := fs.ReadFile(migrations.MySQLFS, name)
		if err != nil {
			failures = append(failures, migrationFailure{name, err})
			continue
		}
		// 容错模式：无 SUPER 权限的测试库预先剥掉触发器 DDL。
		// 必须预剥离——多语句批量中途失败（1419）会在连接上残留未读结果集，
		// 复用该连接会协议失步死锁。
		stmtContent := string(content)
		if mysqlTriggerFailureTolerant && strings.Contains(strings.ToUpper(stmtContent), "CREATE TRIGGER") {
			stmtContent = stripMySQLTriggerDDL(stmtContent)
			t.Logf("WARN %-60s trigger DDL skipped (test-tolerant mode)", name)
			if strings.TrimSpace(stripSQLLineComment(stmtContent)) == "" {
				continue // 整个文件只有触发器
			}
		}

		start := time.Now()
		if _, err := db.ExecContext(ctx, stmtContent); err != nil {
			failures = append(failures, migrationFailure{name, err})
			t.Logf("FAIL %-60s %v", name, err)
			continue
		}
		applied++
		t.Logf("ok   %-60s (%dms)", name, time.Since(start).Milliseconds())
	}

	if len(failures) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "%d/%d migrations failed:\n", len(failures), len(files))
		for _, f := range failures {
			fmt.Fprintf(&b, "  - %s: %v\n", f.name, f.err)
		}
		t.Fatalf("%s", b.String())
	}
	t.Logf("all %d mysql migrations applied", applied)
}
