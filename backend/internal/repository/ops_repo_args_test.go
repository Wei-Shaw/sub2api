//go:build unit

package repository

import (
	"database/sql"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsInsertErrorLogArgsPreservesExplicitZeroUpstreamStatus(t *testing.T) {
	zero := 0
	args := opsInsertErrorLogArgs(&service.OpsInsertErrorLogInput{UpstreamStatusCode: &zero})

	// 41 = 原始 38 + 上游商户两列(upstream_supplier_id/name) + 上游请求 ID 一列。
	// 这个数字必须与 insertOpsErrorLogSQL 的 VALUES 占位符数一致，
	// 否则运行期才会以「参数个数不匹配」暴露。
	require.Len(t, args, 41)
	encoded, ok := args[27].(sql.NullInt64)
	require.True(t, ok)
	require.True(t, encoded.Valid)
	require.Zero(t, encoded.Int64)
}

// TestOpsInsertErrorLogArgsCarriesUpstreamSupplier 守住「失败归因到商户」这条链路的落点。
//
// 这条链路较长（探针失败 → 写 ops 错误日志 → 对账循环按上游请求 ID 反查 → 回填商户），
// 中间任何一处把字段漏掉都不会报错，只会表现为「页面上永远没有商户」——所以在这里钉住。
func TestOpsInsertErrorLogArgsCarriesUpstreamSupplier(t *testing.T) {
	supplierID := 568
	args := opsInsertErrorLogArgs(&service.OpsInsertErrorLogInput{
		UpstreamSupplierID:   &supplierID,
		UpstreamSupplierName: "mist",
		UpstreamRequestID:    "202610101146012103035508268d9d6UZCjQqh0",
	})

	id, ok := args[30].(sql.NullInt64)
	require.True(t, ok)
	require.True(t, id.Valid)
	require.EqualValues(t, supplierID, id.Int64)

	name, ok := args[31].(sql.NullString)
	require.True(t, ok)
	require.True(t, name.Valid)
	require.Equal(t, "mist", name.String)

	reqID, ok := args[32].(sql.NullString)
	require.True(t, ok)
	require.True(t, reqID.Valid)
	require.Equal(t, "202610101146012103035508268d9d6UZCjQqh0", reqID.String)
}

func TestOpsNullableIntPointerDistinguishesNilZeroAndStatus(t *testing.T) {
	missing := opsNullableIntPointer(nil).(sql.NullInt64)
	require.False(t, missing.Valid)

	zeroValue := 0
	zero := opsNullableIntPointer(&zeroValue).(sql.NullInt64)
	require.True(t, zero.Valid)
	require.Zero(t, zero.Int64)

	statusValue := 503
	status := opsNullableIntPointer(&statusValue).(sql.NullInt64)
	require.True(t, status.Valid)
	require.EqualValues(t, 503, status.Int64)
}
