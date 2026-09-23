package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

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
