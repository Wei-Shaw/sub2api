package migrations

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestKiroAccountTypeBackfillMigrationCoversBothDirections 确认迁移文件
// 覆盖 kiro 账号 type 回填的两个方向（api_key 之外的鉴权方式 → oauth；
// api_key → apikey），且两条 UPDATE 都限定在 platform = 'kiro'，不会
// 误改其它平台的账号。
func TestKiroAccountTypeBackfillMigrationCoversBothDirections(t *testing.T) {
	raw, err := os.ReadFile("235_kiro_account_type_backfill.sql")
	require.NoError(t, err)
	sql := string(raw)

	require.Equal(t, 2, strings.Count(sql, "platform = 'kiro'"),
		"两条 UPDATE 都必须限定 platform = 'kiro'，不能误改其它平台")

	require.Contains(t, sql, "SET type = 'oauth'")
	require.Contains(t, sql, "SET type = 'apikey'")

	// 默认鉴权方式是 social，需要与 Account.KiroAuthMethod() 的缺省值保持一致。
	require.Contains(t, sql, "COALESCE(credentials ->> 'auth_method', 'social')")
	require.Contains(t, sql, "credentials ->> 'auth_method' = 'api_key'")
}
