package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestConcurrencyErrorResponse(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		slotType    string
		wantStatus  int
		wantType    string
		wantCode    string
		wantMessage string
	}{
		{
			name:        "true concurrency timeout remains rate limit",
			err:         &ConcurrencyError{SlotType: "account", IsTimeout: true},
			slotType:    "user",
			wantStatus:  http.StatusTooManyRequests,
			wantType:    "rate_limit_error",
			wantCode:    gatewayConcurrencyLimitCode,
			wantMessage: "Concurrency limit exceeded for account, please retry later",
		},
		{
			name:        "full local wait queue has gateway code",
			err:         &WaitQueueFullError{SlotType: "account"},
			slotType:    "account",
			wantStatus:  http.StatusTooManyRequests,
			wantType:    "rate_limit_error",
			wantCode:    gatewayQueueFullCode,
			wantMessage: "Too many pending requests, please retry later",
		},
		{
			name:        "client cancellation is not classified as concurrency limit",
			err:         context.Canceled,
			slotType:    "user",
			wantStatus:  statusClientClosedRequest,
			wantType:    "api_error",
			wantMessage: "context canceled",
		},
		{
			name:        "deadline exceeded is service unavailable",
			err:         context.DeadlineExceeded,
			slotType:    "user",
			wantStatus:  http.StatusServiceUnavailable,
			wantType:    "api_error",
			wantMessage: "Service temporarily unavailable, please retry later",
		},
		{
			name:        "redis acquire error is service unavailable",
			err:         errors.New("redis unavailable"),
			slotType:    "user",
			wantStatus:  http.StatusServiceUnavailable,
			wantType:    "api_error",
			wantMessage: "Service temporarily unavailable, please retry later",
		},
		{
			name: "queued configuration change stays retryable",
			err: &service.APIKeyQueueError{
				Kind:  service.APIKeyQueueErrorAuthRejected,
				Cause: infraerrors.ServiceUnavailable("API_KEY_GROUP_CHANGED", "API key configuration changed; please retry"),
			},
			slotType:    "API key",
			wantStatus:  http.StatusServiceUnavailable,
			wantType:    "api_error",
			wantCode:    "API_KEY_GROUP_CHANGED",
			wantMessage: "API key configuration changed; please retry",
		},
		{
			name: "queued permission revocation keeps its business status",
			err: &service.APIKeyQueueError{
				Kind:  service.APIKeyQueueErrorAuthRejected,
				Cause: infraerrors.Forbidden("LIVE_NOT_ALLOWED", "Live is not enabled for this group"),
			},
			slotType:    "API key",
			wantStatus:  http.StatusForbidden,
			wantType:    "permission_error",
			wantCode:    "LIVE_NOT_ALLOWED",
			wantMessage: "Live is not enabled for this group",
		},
		{
			name: "queued model revocation keeps the allowlist not-found shape",
			err: &service.APIKeyQueueError{
				Kind:  service.APIKeyQueueErrorAuthRejected,
				Cause: infraerrors.NotFound("MODEL_NOT_ALLOWED", `Model "gpt-5.4" is not available for this group`),
			},
			slotType:    "API key",
			wantStatus:  http.StatusNotFound,
			wantType:    "not_found_error",
			wantCode:    "MODEL_NOT_ALLOWED",
			wantMessage: `Model "gpt-5.4" is not available for this group`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, errType, code, message := concurrencyErrorResponse(tt.err, tt.slotType)
			require.Equal(t, tt.wantStatus, status)
			require.Equal(t, tt.wantType, errType)
			require.Equal(t, tt.wantCode, code)
			require.Equal(t, tt.wantMessage, message)
		})
	}
}
