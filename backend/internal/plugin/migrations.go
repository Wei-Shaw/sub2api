package plugin

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"
)

// pluginMigrationsTableDDL 存储所有插件的迁移记录,通过 plugin_name 区分归属。
// 与核心 schema_migrations 表分开,避免插件迁移污染核心迁移历史,也方便单独清理某个插件。
const pluginMigrationsTableDDL = `
CREATE TABLE IF NOT EXISTS plugin_migrations (
	plugin_name           TEXT NOT NULL,
	filename              TEXT NOT NULL,
	checksum              TEXT NOT NULL,
	applied_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	down_sql_cached       TEXT NULL,
	down_filename         TEXT NULL,
	down_checksum_sha256  TEXT NULL,
	PRIMARY KEY (plugin_name, filename)
);
`

const pluginMigrationLockRetryInterval = 500 * time.Millisecond

// MigrationFile 表示一个待执行的迁移文件,内容由插件通过 gRPC 提供。
type MigrationFile struct {
	Filename string
	Content  []byte
	// NonTransactional 标记此 SQL 含有 PostgreSQL 拒绝在显式事务里执行的语句
	// (例如 CREATE INDEX CONCURRENTLY)。host 会跳过 BEGIN/COMMIT,直接在
	// 连接上执行,并在记录到 plugin_migrations 表时使用独立语句。
	NonTransactional bool

	// DownFilename / DownChecksumSha256 / DownSQL 由 fetchAndRunPluginMigrations
	// 在 plugin enable 时拉取并校验, 一并写到 plugin_migrations 表中。Purge
	// 阶段 (插件硬卸载) 直接读取 down_sql_cached 执行回滚, 不再依赖 plugin
	// 进程仍然存活。
	//
	// 三个字段同进同出: 要么 plugin 没声明 down (全部为空), 要么三个都填好。
	// 空字符串 / 空 slice 表示该迁移不可逆, Purge 时跳过 SQL 执行, 仅删除
	// bookkeeping 行。
	DownFilename       string
	DownChecksumSha256 string
	DownSQL            []byte
}

// RunPluginMigrations 在事务中按 filename 顺序执行插件的迁移。
// 与核心迁移机制保持一致:
//   - 使用 PostgreSQL Advisory Lock 序列化执行(每个插件名独立锁 ID,避免互相阻塞)。
//   - 通过 SHA-256 校验和检测已应用迁移是否被篡改。
//   - 已应用的迁移自动跳过。
func RunPluginMigrations(ctx context.Context, db *sql.DB, pluginName string, migrations []MigrationFile) error {
	if db == nil {
		return errors.New("nil sql db")
	}
	if pluginName == "" {
		return errors.New("plugin name is required")
	}
	if len(migrations) == 0 {
		return nil
	}

	lockID := pluginMigrationLockID(pluginName)

	if err := pgPluginAdvisoryLock(ctx, db, lockID); err != nil {
		return err
	}
	defer func() {
		_ = pgPluginAdvisoryUnlock(context.Background(), db, lockID)
	}()

	if _, err := db.ExecContext(ctx, pluginMigrationsTableDDL); err != nil {
		return fmt.Errorf("create plugin_migrations: %w", err)
	}

	// 拷贝并按文件名升序,避免调用方提供的 slice 顺序不确定。
	files := make([]MigrationFile, len(migrations))
	copy(files, migrations)
	sort.Slice(files, func(i, j int) bool {
		return files[i].Filename < files[j].Filename
	})

	for _, m := range files {
		if err := applyOnePluginMigration(ctx, db, pluginName, m); err != nil {
			return err
		}
	}
	return nil
}

func applyOnePluginMigration(ctx context.Context, db *sql.DB, pluginName string, m MigrationFile) error {
	content := strings.TrimSpace(string(m.Content))
	if content == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(content))
	checksum := hex.EncodeToString(sum[:])

	var existing string
	err := db.QueryRowContext(ctx,
		"SELECT checksum FROM plugin_migrations WHERE plugin_name = $1 AND filename = $2",
		pluginName, m.Filename,
	).Scan(&existing)
	if err == nil {
		if existing != checksum {
			return fmt.Errorf(
				"plugin %s migration %s checksum mismatch (db=%s file=%s): "+
					"migration files are immutable once applied; create a new migration instead",
				pluginName, m.Filename, existing, checksum,
			)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check plugin migration %s/%s: %w", pluginName, m.Filename, err)
	}

	if m.NonTransactional {
		// 不允许在显式事务中执行(例如 CREATE INDEX CONCURRENTLY)。
		// 必须按 `;` 拆分逐条执行: lib/pq 在 simple-query 协议下会把
		// 多语句批量包进隐式事务, 触发 "CONCURRENTLY cannot run inside
		// a transaction block" 错误。
		// 失败时无法原子回滚,但已应用的语句会被 PostgreSQL 自身保留;
		// 重新启动时 checksum 校验仍会跳过已记录的迁移。
		for _, stmt := range splitSQLStatements(content) {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("apply plugin migration %s/%s (non_transactional): %w", pluginName, m.Filename, err)
			}
		}
		if _, err := db.ExecContext(ctx,
			`INSERT INTO plugin_migrations (
				plugin_name, filename, checksum,
				down_sql_cached, down_filename, down_checksum_sha256
			) VALUES ($1, $2, $3, $4, $5, $6)`,
			pluginName, m.Filename, checksum,
			nullableDownSQL(m.DownSQL),
			nullableString(m.DownFilename),
			nullableString(m.DownChecksumSha256),
		); err != nil {
			return fmt.Errorf("record plugin migration %s/%s (non_transactional): %w", pluginName, m.Filename, err)
		}
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin plugin migration %s/%s: %w", pluginName, m.Filename, err)
	}
	if _, err := tx.ExecContext(ctx, content); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("apply plugin migration %s/%s: %w", pluginName, m.Filename, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO plugin_migrations (
			plugin_name, filename, checksum,
			down_sql_cached, down_filename, down_checksum_sha256
		) VALUES ($1, $2, $3, $4, $5, $6)`,
		pluginName, m.Filename, checksum,
		nullableDownSQL(m.DownSQL),
		nullableString(m.DownFilename),
		nullableString(m.DownChecksumSha256),
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("record plugin migration %s/%s: %w", pluginName, m.Filename, err)
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("commit plugin migration %s/%s: %w", pluginName, m.Filename, err)
	}
	return nil
}

// nullableString returns sql.NullString matching s; empty -> {Valid:false}.
// Used so the down_filename / down_checksum_sha256 columns stay NULL when the
// plugin did not declare a reversible migration, instead of writing empty
// strings that would then need a special case in Purge.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// nullableDownSQL mirrors nullableString for the cached body. Bytes are stored
// as TEXT in PostgreSQL because the plugin guarantees UTF-8 SQL.
func nullableDownSQL(body []byte) any {
	if len(body) == 0 {
		return nil
	}
	return string(body)
}

// pluginMigrationLockID 用 fnv64 把 "plugin:<name>" 映射成 int64 advisory lock ID。
// fnv 输出 uint64,需要重新解读为 int64 以匹配 pg_try_advisory_lock(bigint)。
func pluginMigrationLockID(pluginName string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("plugin:"))
	_, _ = h.Write([]byte(pluginName))
	return int64(h.Sum64()) //nolint:gosec // 故意将 uint64 重新解读为 int64
}

func pgPluginAdvisoryLock(ctx context.Context, db *sql.DB, lockID int64) error {
	ticker := time.NewTicker(pluginMigrationLockRetryInterval)
	defer ticker.Stop()

	for {
		var locked bool
		if err := db.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&locked); err != nil {
			return fmt.Errorf("acquire plugin migration lock: %w", err)
		}
		if locked {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("acquire plugin migration lock: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func pgPluginAdvisoryUnlock(ctx context.Context, db *sql.DB, lockID int64) error {
	if _, err := db.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", lockID); err != nil {
		return fmt.Errorf("release plugin migration lock: %w", err)
	}
	return nil
}

// splitSQLStatements 把多语句 SQL 文本按 `;` 拆分为单条语句,
// 跳过纯空白和纯注释行。供 NonTransactional 迁移使用 ——
// CREATE INDEX CONCURRENTLY 等 DDL 必须独占连接 (无隐式事务),
// 而 lib/pq simple-query 协议会把同一 Exec 里的多语句包进隐式事务。
//
// 简单实现: 不识别字符串字面量里的 `;` (DDL migrations 极少包含)。
// 如需更鲁棒的解析,引入 pg_query_go 等专用 SQL 解析器。
func splitSQLStatements(content string) []string {
	parts := strings.Split(content, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		// 删除单行注释 -- ... 到行末
		var cleaned strings.Builder
		for _, line := range strings.Split(p, "\n") {
			if idx := strings.Index(line, "--"); idx >= 0 {
				line = line[:idx]
			}
			cleaned.WriteString(line)
			cleaned.WriteString("\n")
		}
		stmt := strings.TrimSpace(cleaned.String())
		if stmt == "" {
			continue
		}
		out = append(out, stmt)
	}
	return out
}
