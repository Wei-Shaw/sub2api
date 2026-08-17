package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAsyncMediaTaskRepositoryTerminalUsageLogUpsertsZeroCostTimeoutRecord(t *testing.T) {
	var capturedSQL string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureEntQueryMatcher{actual: &capturedSQL}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := &asyncMediaTaskRepository{sql: db}
	mock.ExpectExec("insert terminal usage log").
		WillReturnResult(sqlmock.NewResult(0, 1))

	inserted, err := repo.InsertTerminalUsageLog(context.Background(), &service.TerminalUsageLogInput{
		UserID:         1,
		APIKeyID:       2,
		AccountID:      3,
		RequestID:      "req-timeout-then-success",
		Model:          "openai/gpt-image-2",
		RequestedModel: "gpt-image-2",
		UpstreamModel:  "openai/gpt-image-2",
		TotalCost:      0.05,
		ActualCost:     0.05,
		RateMultiplier: 1,
		BillingType:    service.BillingTypeBalance,
		RequestType:    int16(service.RequestTypeSync),
		ImageCount:     1,
		TaskID:         9,
		ImageURLs:      []string{"https://fal.media/out.png"},
		BillingStatus:  service.BillingStatusCharged,
	})

	require.NoError(t, err)
	require.True(t, inserted)
	require.NoError(t, mock.ExpectationsWereMet())

	normalized := normalizeSQLWhitespace(capturedSQL)
	require.Contains(t, normalized, "ON CONFLICT (request_id, api_key_id) DO UPDATE SET")
	require.Contains(t, normalized, "image_input_size")
	require.Contains(t, normalized, "image_output_size")
	require.Contains(t, normalized, "image_size_breakdown")
	require.Contains(t, normalized, "EXCLUDED.billing_status = 'charged'")
	require.Contains(t, normalized, "COALESCE(usage_logs.actual_cost, 0) = 0")
	require.Contains(t, normalized, "COALESCE(usage_logs.total_cost, 0) = 0")
	require.NotContains(t, normalized, "COALESCE(usage_logs.image_count, 0) = 0")
}
