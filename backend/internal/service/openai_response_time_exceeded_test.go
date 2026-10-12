//go:build unit

package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsOpenAIUpstreamResponseTimeExceeded(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"observed channel timeout", http.StatusGatewayTimeout, `{"error":{"code":"channel:response_time_exceeded"}}`, true},
		{"nested response error", http.StatusServiceUnavailable, `{"response":{"error":{"code":"channel:response_time_exceeded"}}}`, true},
		{"ordinary timeout", http.StatusGatewayTimeout, `{"error":{"code":"gateway_timeout"}}`, false},
		{"echoed client text", http.StatusGatewayTimeout, `{"error":{"code":"server_error","message":"channel:response_time_exceeded"}}`, false},
		{"client error", http.StatusBadRequest, `{"error":{"code":"channel:response_time_exceeded"}}`, false},
		{"invalid JSON", http.StatusGatewayTimeout, `channel:response_time_exceeded`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isOpenAIUpstreamResponseTimeExceeded(tc.status, []byte(tc.body)))
		})
	}
	require.False(t, isOpenAIUpstreamResponseTimeExceeded(http.StatusGatewayTimeout, nil))
}
