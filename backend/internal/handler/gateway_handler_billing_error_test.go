package handler

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBillingErrorDetails_MapsGroupRPMExceededToTooManyRequests(t *testing.T) {
	status, code, msg, md := billingErrorDetails(service.ErrGroupRPMExceeded)
	require.Equal(t, http.StatusTooManyRequests, status)
	require.Equal(t, "rate_limit_exceeded", code)
	require.NotEmpty(t, msg)
	retryAfter, _ := strconv.Atoi(md["retry_after"])
	require.Greater(t, retryAfter, 0, "RPM exceeded should return positive Retry-After")
	require.LessOrEqual(t, retryAfter, 60)
}

func TestBillingErrorDetails_MapsUserRPMExceededToTooManyRequests(t *testing.T) {
	status, code, msg, md := billingErrorDetails(service.ErrUserRPMExceeded)
	require.Equal(t, http.StatusTooManyRequests, status)
	require.Equal(t, "rate_limit_exceeded", code)
	require.NotEmpty(t, msg)
	retryAfter, _ := strconv.Atoi(md["retry_after"])
	require.Greater(t, retryAfter, 0, "RPM exceeded should return positive Retry-After")
	require.LessOrEqual(t, retryAfter, 60)
}

func TestBillingErrorDetails_APIKeyRateLimitStillMaps(t *testing.T) {
	// 回归保护：加 RPM 分支后不应影响已有 APIKey rate limit 的映射。
	for _, err := range []error{
		service.ErrAPIKeyRateLimit5hExceeded,
		service.ErrAPIKeyRateLimit1dExceeded,
		service.ErrAPIKeyRateLimit7dExceeded,
	} {
		status, code, _, _ := billingErrorDetails(err)
		require.Equal(t, http.StatusTooManyRequests, status, "status for %v", err)
		require.Equal(t, "rate_limit_exceeded", code)
	}
}

func TestBillingErrorDetails_BillingServiceUnavailableMapsTo503(t *testing.T) {
	status, code, _, md := billingErrorDetails(service.ErrBillingServiceUnavailable)
	require.Equal(t, http.StatusServiceUnavailable, status)
	require.Equal(t, "billing_service_error", code)
	require.Nil(t, md, "non-RPM errors should not set Retry-After metadata")
}

func TestBillingErrorDetails_UnknownErrorFallsBackTo403(t *testing.T) {
	status, code, msg, _ := billingErrorDetails(service.ErrInsufficientBalance)
	require.Equal(t, http.StatusForbidden, status)
	require.Equal(t, "billing_error", code)
	require.NotEmpty(t, msg)
}
