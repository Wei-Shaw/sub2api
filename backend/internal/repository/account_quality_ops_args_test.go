package repository

import (
	"database/sql"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAccountQualityOpsConditionArgumentsAreIndependentFromErrorBodies(t *testing.T) {
	entry := &service.OpsInsertErrorLogInput{QualityReasoningEffort: "high", QualityServiceTier: "priority", QualityScopeKnown: true, ErrorBody: "redacted body"}
	args := opsInsertErrorLogArgs(entry)
	require.Len(t, args, 41)
	require.Equal(t, sql.NullString{String: "high", Valid: true}, args[38])
	require.Equal(t, sql.NullString{String: "priority", Valid: true}, args[39])
	require.Equal(t, true, args[40])
	entry.QualityScopeKnown = false
	require.Equal(t, false, opsInsertErrorLogArgs(entry)[40])
}
