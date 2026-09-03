package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMigrationsContainNoGooseDirectives 守护一条硬性约束：迁移文件里不允许出现 goose 指令。
//
// internal/repository/migrations_runner.go 并没有 goose 解析器：它把整个文件内容原样交给
// tx.ExecContext 在一个事务里执行。对 PostgreSQL 而言 `-- +goose Down` 只是一行普通注释，
// 于是 Down 块会紧接着 Up 块执行，迁移当场自我回滚——037 建完 ops_alert_silences 又 DROP 掉、
// 019 把 wechat 属性数据还原回 users.wechat、024 把刚写入的 tier_id 默认值删掉。
//
// `-- +goose Up` / `StatementBegin` / `StatementEnd` 同样会误导读者，让人以为这些文件由 goose
// 驱动，因此一并禁止。校验和不可变（schema_migrations.checksum）意味着这类错误一旦发布就只能
// 靠新增修复迁移补救（见 235/236/237），所以必须在 CI 阶段拦住。
func TestMigrationsContainNoGooseDirectives(t *testing.T) {
	err := fs.WalkDir(FS, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".sql") {
			return nil
		}

		content, readErr := FS.ReadFile(path)
		require.NoError(t, readErr)

		sql := string(content)
		require.NotContainsf(t, sql, "+goose Down",
			"%s contains a `+goose Down` block; the migration runner does not parse goose "+
				"directives, so the Down block would execute right after the Up block in the "+
				"same transaction and revert the migration", path)
		require.NotContainsf(t, sql, "+goose",
			"%s contains a goose directive; migrations are applied verbatim by "+
				"repository.ApplyMigrations, not by goose", path)

		return nil
	})
	require.NoError(t, err)
}
