package middleware

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIAsyncManagementSkipsInferenceBillingOnly(t *testing.T) {
	for _, path := range []string{"/v1/files", "/v1/files/file-local-123", "/v1/files/file-local-123/content", "/v1/batches", "/v1/batches/batch_123"} {
		require.True(t, isOpenAIAsyncManagement("GET", path))
	}
	require.True(t, isOpenAIAsyncManagement("POST", "/v1/batches/batch_123/cancel"))
	require.False(t, isOpenAIAsyncManagement("POST", "/v1/batches"))
	require.False(t, isOpenAIAsyncManagement("POST", "/v1/responses"))
	require.False(t, isOpenAIAsyncManagement("POST", "/v1/batches/batch_123"))
}
