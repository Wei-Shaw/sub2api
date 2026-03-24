//go:build unit

package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
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
