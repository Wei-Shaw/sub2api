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
REDACTED{
		{"inbound", "ul.inbound_endpoint"REDACTED,
		{"upstream", "ul.upstream_endpoint"REDACTED,
		{"path", "ul.inbound_endpoint || ' -> ' || ul.upstream_endpoint"REDACTED,
		{"", "ul.inbound_endpoint"REDACTED,        // default
		{"unknown", "ul.inbound_endpoint"REDACTED, // fallback
REDACTED

	for _, tc := range tests {
		t.Run(tc.endpointType, func(t *testing.T) {
			got := resolveEndpointColumn(tc.endpointType)
			require.Equal(t, tc.want, got)
	REDACTED)
REDACTED
REDACTED

func TestResolveModelDimensionExpression(t *testing.T) {
	tests := []struct {
		modelType string
		want      string
REDACTED{
		{usagestats.ModelSourceRequested, "COALESCE(NULLIF(TRIM(requested_model), ''), model)"REDACTED,
		{usagestats.ModelSourceUpstream, "COALESCE(NULLIF(TRIM(upstream_model), ''), COALESCE(NULLIF(TRIM(requested_model), ''), model))"REDACTED,
		{usagestats.ModelSourceMapping, "(COALESCE(NULLIF(TRIM(requested_model), ''), model) || ' -> ' || COALESCE(NULLIF(TRIM(upstream_model), ''), COALESCE(NULLIF(TRIM(requested_model), ''), model)))"REDACTED,
		{"", "COALESCE(NULLIF(TRIM(requested_model), ''), model)"REDACTED,
		{"invalid", "COALESCE(NULLIF(TRIM(requested_model), ''), model)"REDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.modelType, func(t *testing.T) {
			got := resolveModelDimensionExpression(tc.modelType)
			require.Equal(t, tc.want, got)
	REDACTED)
REDACTED
REDACTED

func TestGetUserBreakdownStatsRequestTypeIncludesLegacyFallback(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: dbREDACTED
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	requestType := int16(service.RequestTypeStream)

	legacyFilter := `(ul.request_type = $3 OR (ul.request_type = 0 AND ul.stream = TRUE AND ul.openai_ws_mode = FALSE))`
	mock.ExpectQuery(regexp.QuoteMeta(legacyFilter)).
		WithArgs(start, end, requestType).
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "email", "requests", "input_tokens", "output_tokens",
			"cache_tokens", "total_tokens", "cost", "actual_cost", "account_cost",
	REDACTED))

	rows, err := repo.GetUserBreakdownStats(context.Background(), start, end, usagestats.UserBreakdownDimension{
		RequestType: &requestType,
REDACTED, 0)

REDACTED
	require.Empty(t, rows)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED
