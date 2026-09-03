package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAPIKeysKeyHashMigration 守护 239 的四条硬性要求。
//
// 这条迁移是"两阶段去明文"的第 1 阶段，任何一条被改坏都会造成线上事故：
// 摘要表达式漂移 → 存量 Key 全部认证失败；回填被删 → 同上；
// 部分唯一索引变成整表唯一 → 滚动升级窗口里老版本建 Key 直接报错；
// 明文列被顺手删掉 → 越权做了第 2 阶段的事，回滚即全量失效。
func TestAPIKeysKeyHashMigration(t *testing.T) {
	content, err := FS.ReadFile("239_add_api_keys_key_hash.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")

	// 1) 列必须是 NOT NULL DEFAULT ''：滚动升级窗口里老版本二进制不会写这一列。
	require.Contains(t, sql,
		"ADD COLUMN IF NOT EXISTS key_hash VARCHAR(64) NOT NULL DEFAULT ''")

	// 2) 回填表达式必须与 service.HashAPIKeyCredential 一致（小写 hex SHA-256）。
	require.Contains(t, sql,
		"SET key_hash = encode(sha256(convert_to(key, 'UTF8')), 'hex') WHERE key_hash = ''")

	// 3) 部分唯一索引，不是整表唯一约束。
	require.Contains(t, sql,
		"CREATE UNIQUE INDEX IF NOT EXISTS uniq_api_keys_key_hash ON api_keys (key_hash) WHERE key_hash <> ''")

	// 4) 第 1 阶段绝不能删除明文列——那是第 2 阶段的事，提前做会让回滚不可能。
	require.NotContains(t, sql, "DROP COLUMN")
	require.NotContains(t, strings.ToLower(sql), "drop column key")

	// 不得依赖 pgcrypto：sha256/convert_to/encode 都是内置函数。
	// （注释里出现 pgcrypto/hmac 是在解释为何不用它们，这里只查可执行语句。）
	require.NotContains(t, strings.ToLower(sql), "create extension")
}
