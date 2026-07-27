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

	require.Len(t, args, 45)
	encoded, ok := args[34].(sql.NullInt64)
	require.True(t, ok)
	require.True(t, encoded.Valid)
	require.Zero(t, encoded.Int64)
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

func TestOpsNullIntPreservesObservedPointerZero(t *testing.T) {
	zeroInt := 0
	encodedInt := opsNullInt(&zeroInt).(sql.NullInt64)
	require.True(t, encodedInt.Valid)
	require.Zero(t, encodedInt.Int64)

	zeroInt64 := int64(0)
	encodedInt64 := opsNullInt(&zeroInt64).(sql.NullInt64)
	require.True(t, encodedInt64.Valid)
	require.Zero(t, encodedInt64.Int64)

	missing := opsNullInt((*int)(nil)).(sql.NullInt64)
	require.False(t, missing.Valid)
}
