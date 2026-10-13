package handler

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// When every candidate is excluded only because it used up its account RPM limit,
// the client gets 429 rate_limit_error (retry later), not 503.
func TestClassifySelectionFailureError_AccountRPMLimitIs429(t *testing.T) {
	fallback := noAccountErrorClassification{Status: http.StatusServiceUnavailable, ErrType: "api_error", Message: "Service temporarily unavailable"}
	err := errors.New("no available OpenAI accounts supporting model: MiniMax-M3.1 (pool=4, filtered: rpm_rate_limited=4)")
	got := classifySelectionFailureError(err, fallback)
	require.Equal(t, http.StatusTooManyRequests, got.Status)
	require.Equal(t, "rate_limit_error", got.ErrType)
}
