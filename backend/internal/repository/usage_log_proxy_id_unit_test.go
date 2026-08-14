//go:build unit

package repository

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func newProxyIDUsageLog(proxyID *int64) *service.UsageLog {
	return &service.UsageLog{
		UserID:       1,
		APIKeyID:     2,
		AccountID:    3,
		RequestID:    "req-proxy-id",
		Model:        "claude-3",
		InputTokens:  10,
		OutputTokens: 5,
		TotalCost:    1.0,
		ActualCost:   1.0,
		ProxyID:      proxyID,
		CreatedAt:    time.Now().UTC(),
	}
}

func TestPrepareUsageLogInsert_ProxyIDArgWiring(t *testing.T) {
	require.Len(t, usageLogInsertArgTypes, 60, "arg-type table must include proxy_id")
	require.Equal(t, "bigint", usageLogInsertArgTypes[len(usageLogInsertArgTypes)-2],
		"proxy_id arg type must be bigint")

	proxyID := int64(42)
	prepared := prepareUsageLogInsert(newProxyIDUsageLog(&proxyID))
	require.Len(t, prepared.args, len(usageLogInsertArgTypes))

	proxyArg := prepared.args[len(prepared.args)-2]
	ni, ok := proxyArg.(sql.NullInt64)
	require.True(t, ok, "proxy_id arg should be a sql.NullInt64, got %T", proxyArg)
	require.True(t, ni.Valid)
	require.Equal(t, proxyID, ni.Int64)
}

func TestPrepareUsageLogInsert_ProxyIDNullWhenAbsent(t *testing.T) {
	prepared := prepareUsageLogInsert(newProxyIDUsageLog(nil))
	proxyArg := prepared.args[len(prepared.args)-2]
	ni, ok := proxyArg.(sql.NullInt64)
	require.True(t, ok, "proxy_id arg should be a sql.NullInt64, got %T", proxyArg)
	require.False(t, ni.Valid, "absent proxy id must be NULL")
}

func TestUsageLogInsertQueries_IncludeProxyID(t *testing.T) {
	require.Contains(t, usageLogSelectColumns, "proxy_id",
		"SELECT column list must include proxy_id")

	proxyID := int64(7)
	log := newProxyIDUsageLog(&proxyID)
	prepared := prepareUsageLogInsert(log)
	key := usageLogBatchKey(log.RequestID, log.APIKeyID)

	batchQuery, batchArgs := buildUsageLogBatchInsertQuery([]string{key},
		map[string]usageLogInsertPrepared{key: prepared})
	require.Contains(t, batchQuery, "proxy_id")
	require.GreaterOrEqual(t, strings.Count(batchQuery, "proxy_id"), 3)
	require.Len(t, batchArgs, len(prepared.args)+1)

	bestEffortQuery, bestEffortArgs := buildUsageLogBestEffortInsertQuery([]usageLogInsertPrepared{prepared})
	require.Contains(t, bestEffortQuery, "proxy_id")
	require.Len(t, bestEffortArgs, len(prepared.args))
}
