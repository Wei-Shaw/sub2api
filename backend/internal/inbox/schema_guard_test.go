//go:build unit

package inbox

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestInboxMigration_NoSensitiveColumns 是 schema 泄露门禁（tasks 2.16）：
// 信箱三张表只应存放"通知信封"（seq / namespace / dedup_key / targeting / payload /
// 时间戳 / user_id），绝不应出现凭据类字段名。本测试解析迁移 SQL，断言其中不含
// token / password / secret / authorization / credential 等敏感列名，防止后续演进
// 误把敏感数据落进信箱表。
func TestInboxMigration_NoSensitiveColumns(t *testing.T) {
	// 测试工作目录为包目录 backend/internal/inbox，迁移文件在 backend/migrations。
	path := filepath.Join("..", "..", "migrations", "184_add_general_inbox.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取迁移文件失败: %v", err)
	}
	sql := strings.ToLower(string(raw))

	forbidden := []string{
		"token",
		"password",
		"passwd",
		"secret",
		"authorization",
		"credential",
		"api_key",
		"apikey",
		"access_key",
		"private_key",
	}
	for _, word := range forbidden {
		// 用单词边界匹配，避免误伤（当前无合法用途，但保持严格）。
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`)
		if re.MatchString(sql) {
			t.Errorf("信箱迁移 SQL 中出现敏感字段名 %q，禁止在信箱表存放凭据类数据", word)
		}
	}
}
