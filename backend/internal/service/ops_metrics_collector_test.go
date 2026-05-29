package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWriteOpenAIFastPolicyBlockedResponseMarksBusinessLimited(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	writeOpenAIFastPolicyBlockedResponse(c, &OpenAIFastBlockedError{Message: "custom fast policy block"})

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.True(t, HasOpsClientBusinessLimited(c))
	reason, ok := c.Get(OpsClientBusinessLimitedReasonKey)
	require.True(t, ok)
	require.Equal(t, OpsClientBusinessLimitedReasonLocalPolicyDenied, reason)
}

func TestOpsMetricsCollectorQueryErrorCountsExcludesCountTokens(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	collector := &OpsMetricsCollector{db: db}
	start := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	mock.ExpectQuery(`(?s)FROM ops_error_logs\s+WHERE created_at >= \$1 AND created_at < \$2\s+AND is_count_tokens = FALSE`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{
			"error_total",
			"business_limited",
			"error_sla",
			"upstream_excl",
			"upstream_429",
			"upstream_529",
		}).AddRow(int64(5), int64(2), int64(3), int64(1), int64(1), int64(1)))

	errorTotal, businessLimited, errorSLA, upstreamExcl429529, upstream429, upstream529, err := collector.queryErrorCounts(context.Background(), start, end)
	require.NoError(t, err)
	require.Equal(t, int64(5), errorTotal)
	require.Equal(t, int64(2), businessLimited)
	require.Equal(t, int64(3), errorSLA)
	require.Equal(t, int64(1), upstreamExcl429529)
	require.Equal(t, int64(1), upstream429)
	require.Equal(t, int64(1), upstream529)
	require.NoError(t, mock.ExpectationsWereMet())
	mock.ExpectClose()
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}
