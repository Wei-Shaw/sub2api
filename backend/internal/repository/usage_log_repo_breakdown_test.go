//go:build unit

package repository

import (
	"testing"

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
		{"", "ul.inbound_endpoint"REDACTED,           // default
		{"unknown", "ul.inbound_endpoint"REDACTED,     // fallback
REDACTED

	for _, tc := range tests {
		t.Run(tc.endpointType, func(t *testing.T) {
			got := resolveEndpointColumn(tc.endpointType)
			require.Equal(t, tc.want, got)
	REDACTED)
REDACTED
REDACTED
