//go:build unit

package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestResolveEndpointColumn(t *testing.T) {
	tests := []struct {
		endpointType string
		want         string
	}{
		{"inbound", "ul.inbound_endpoint"},
		{"upstream", "ul.upstream_endpoint"},
		{"path", "ul.inbound_endpoint || ' -> ' || ul.upstream_endpoint"},
		{"", "ul.inbound_endpoint"},        // default
		{"unknown", "ul.inbound_endpoint"}, // fallback
	}

	for _, tc := range tests {
		t.Run(tc.endpointType, func(t *testing.T) {
			got := resolveEndpointColumn(tc.endpointType)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestResolveModelDimensionExpression(t *testing.T) {
	tests := []struct {
		modelType string
		want      string
	}{
		{usagestats.ModelSourceRequested, "COALESCE(NULLIF(TRIM(requested_model), ''), model)"},
		{usagestats.ModelSourceUpstream, "COALESCE(NULLIF(TRIM(upstream_model), ''), COALESCE(NULLIF(TRIM(requested_model), ''), model))"},
		{usagestats.ModelSourceMapping, "(COALESCE(NULLIF(TRIM(requested_model), ''), model) || ' -> ' || COALESCE(NULLIF(TRIM(upstream_model), ''), COALESCE(NULLIF(TRIM(requested_model), ''), model)))"},
		{"", "COALESCE(NULLIF(TRIM(requested_model), ''), model)"},
		{"invalid", "COALESCE(NULLIF(TRIM(requested_model), ''), model)"},
	}

	for _, tc := range tests {
		t.Run(tc.modelType, func(t *testing.T) {
			got := resolveModelDimensionExpression(tc.modelType)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestGetUserBreakdownStatsRequestTypeIncludesLegacyFallback(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	requestType := int16(service.RequestTypeStream)

	legacyFilter := `(ul.request_type = $3 OR (ul.request_type = 0 AND ul.stream = TRUE AND ul.openai_ws_mode = FALSE))`
	mock.ExpectQuery(regexp.QuoteMeta(legacyFilter)).
		WithArgs(start, end, requestType).
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "email", "requests", "input_tokens", "output_tokens",
			"cache_tokens", "total_tokens", "cost", "actual_cost", "account_cost",
		}))

	rows, err := repo.GetUserBreakdownStats(context.Background(), start, end, usagestats.UserBreakdownDimension{
		RequestType: &requestType,
	}, 0)

	require.NoError(t, err)
	require.Empty(t, rows)
	require.NoError(t, mock.ExpectationsWereMet())
}
