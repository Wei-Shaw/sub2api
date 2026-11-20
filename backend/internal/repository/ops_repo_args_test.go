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

	require.Len(t, args, 40)
	encoded, ok := args[27].(sql.NullInt64)
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

func TestOpsInsertErrorLogArgsRequestParameters(t *testing.T) {
	args := opsInsertErrorLogArgs(&service.OpsInsertErrorLogInput{ReasoningEffort: "xhigh", ServiceTier: "priority"})
	require.Equal(t, sql.NullString{String: "xhigh", Valid: true}, args[38])
	require.Equal(t, sql.NullString{String: "priority", Valid: true}, args[39])
	missing := opsInsertErrorLogArgs(&service.OpsInsertErrorLogInput{})
	require.Equal(t, sql.NullString{}, missing[38])
	require.Equal(t, sql.NullString{}, missing[39])
}
