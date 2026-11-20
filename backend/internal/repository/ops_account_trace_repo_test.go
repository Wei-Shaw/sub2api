package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// 按账号反查时，一个账号的多条尝试来自不同请求，每条都要带上自己的 request_id。
func TestQueryTraceAttemptsScansRequestID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC().Truncate(time.Second)
	cols := []string{
		"attempt_no", "account_id", "retry_from", "platform", "model_req", "model_upstream",
		"upstream_url", "upstream_status", "failure_kind", "error_code", "error_msg",
		"account_state", "cooling_until", "credential_fp", "proxy_id", "request_id", "created_at",
	}
	rows := sqlmock.NewRows(cols).
		AddRow(1, int64(9201), nil, "anthropic", "claude-x", nil,
			nil, 401, "auth", "invalid_api_key", nil,
			nil, nil, "5e6a7944dd4a0d16", nil, "req-aaa", now).
		AddRow(2, int64(9201), int64(9100), "anthropic", "claude-x", nil,
			nil, 429, "rate_limit", nil, nil,
			"cooling", now.Add(time.Minute), nil, nil, "req-bbb", now)
	mock.ExpectQuery("SELECT (.+) request_id, created_at\\s+FROM upstream_attempts").
		WithArgs(int64(9201)).
		WillReturnRows(rows)

	query := `SELECT ` + traceAttemptColumns + ` FROM upstream_attempts WHERE account_id = $1`
	items, err := queryTraceAttempts(context.Background(), db, query, int64(9201))
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "req-aaa", items[0].RequestID)
	require.Equal(t, "req-bbb", items[1].RequestID)
	require.Equal(t, now, items[0].CreatedAt)
	require.NotNil(t, items[1].RetryFrom)
	require.Equal(t, int64(9100), *items[1].RetryFrom)
	require.NoError(t, mock.ExpectationsWereMet())
}
