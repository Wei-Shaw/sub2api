package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRuntimeHardeningBalanceSkipsSameAccountRetry(t *testing.T) {
	err := &service.UpstreamFailoverError{StatusCode: 403, ResponseBody: []byte(`{"code":"INSUFFICIENT_BALANCE"}`), RetryableOnSameAccount: true}
	require.False(t, sameAccountRetryAllowed(err, 0, 3))
	require.True(t, err.ShouldRetryNextAccount())
	err.ResponseBody = []byte(`{"code":"temporarily_unavailable"}`)
	require.True(t, sameAccountRetryAllowed(err, 0, 3))
}
