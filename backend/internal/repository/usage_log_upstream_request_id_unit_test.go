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

func newUpstreamRequestIDUsageLog(upstreamRequestID *string) *service.UsageLog {
	return &service.UsageLog{
		UserID:            1,
		APIKeyID:          2,
		AccountID:         3,
		RequestID:         "client:req-upstream-id",
		Model:             "claude-3",
		InputTokens:       10,
		OutputTokens:      5,
		TotalCost:         1.0,
		ActualCost:        1.0,
		UpstreamRequestID: upstreamRequestID,
		CreatedAt:         time.Now().UTC(),
	}
}

// upstreamRequestIDArgIndex 是 upstream_request_id 在插入参数表中的位置：
// 紧跟 upstream_model_mismatch（boolean）之后。
func upstreamRequestIDArgIndex(t *testing.T) int {
	t.Helper()
	for i, typ := range usageLogInsertArgTypes {
		if typ == "boolean" {
			return i + 1
		}
	}
	t.Fatal("upstream_model_mismatch boolean arg not found")
	return -1
}

// TestPrepareUsageLogInsert_UpstreamRequestIDArgWiring pins upstream_request_id to
// the arg slice / arg-type table so the five INSERT column lists stay in sync.
func TestPrepareUsageLogInsert_UpstreamRequestIDArgWiring(t *testing.T) {
	idx := upstreamRequestIDArgIndex(t)
	require.Equal(t, "text", usageLogInsertArgTypes[idx],
		"upstream_request_id arg type must be text")

	upstreamRequestID := "req_011CRJabc"
	prepared := prepareUsageLogInsert(newUpstreamRequestIDUsageLog(&upstreamRequestID))
	require.Len(t, prepared.args, len(usageLogInsertArgTypes))

	arg := prepared.args[idx]
	ns, ok := arg.(sql.NullString)
	require.True(t, ok, "upstream_request_id arg should be a sql.NullString, got %T", arg)
	require.True(t, ns.Valid)
	require.Equal(t, upstreamRequestID, ns.String)
}

// TestPrepareUsageLogInsert_UpstreamRequestIDNullWhenAbsent proves an absent
// upstream request id is persisted as SQL NULL rather than an empty string.
func TestPrepareUsageLogInsert_UpstreamRequestIDNullWhenAbsent(t *testing.T) {
	idx := upstreamRequestIDArgIndex(t)

	prepared := prepareUsageLogInsert(newUpstreamRequestIDUsageLog(nil))
	ns, ok := prepared.args[idx].(sql.NullString)
	require.True(t, ok, "upstream_request_id arg should be a sql.NullString, got %T", prepared.args[idx])
	require.False(t, ns.Valid, "absent upstream request id must be NULL, not empty string")

	empty := ""
	preparedEmpty := prepareUsageLogInsert(newUpstreamRequestIDUsageLog(&empty))
	nsEmpty := preparedEmpty.args[idx].(sql.NullString)
	require.False(t, nsEmpty.Valid, "empty upstream request id must also be NULL")
}

// TestUsageLogQueries_IncludeUpstreamRequestID guards that every generated INSERT
// path and the SELECT column list reference upstream_request_id.
func TestUsageLogQueries_IncludeUpstreamRequestID(t *testing.T) {
	require.Contains(t, usageLogSelectColumns, "upstream_request_id",
		"SELECT column list must include upstream_request_id")

	upstreamRequestID := "req-in-query"
	log := newUpstreamRequestIDUsageLog(&upstreamRequestID)
	prepared := prepareUsageLogInsert(log)
	key := usageLogBatchKey(log.RequestID, log.APIKeyID)

	batchQuery, batchArgs := buildUsageLogBatchInsertQuery([]string{key},
		map[string]usageLogInsertPrepared{key: prepared})
	// CTE 定义 + INSERT 列清单 + SELECT ... FROM input 三处引用。
	require.GreaterOrEqual(t, strings.Count(batchQuery, "upstream_request_id"), 3)
	require.Len(t, batchArgs, len(prepared.args)+1)

	bestEffortQuery, bestEffortArgs := buildUsageLogBestEffortInsertQuery([]usageLogInsertPrepared{prepared})
	require.GreaterOrEqual(t, strings.Count(bestEffortQuery, "upstream_request_id"), 3)
	require.Len(t, bestEffortArgs, len(prepared.args))
}
