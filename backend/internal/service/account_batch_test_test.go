//go:build unit

package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestResolveBatchAccountTestConcurrencyUsesConfig(t *testing.T) {
	cfg := config.BatchAccountTestConfig{
		DefaultConcurrency:       2,
		MaxConcurrency:           3,
		MaxAccounts:              500,
		PerAccountTimeoutSeconds: 30,
	}

	require.Equal(t, 2, ResolveBatchAccountTestConcurrency(0, 5, cfg))
	require.Equal(t, 3, ResolveBatchAccountTestConcurrency(10, 5, cfg))
	require.Equal(t, 1, ResolveBatchAccountTestConcurrency(10, 1, cfg))
}

func TestBatchAccountTestProgressCountsUnauthorizedByHTTPStatus(t *testing.T) {
	var progress BatchAccountTestProgress
	resultItem := BatchAccountTestResult{
		AccountID:    1,
		Status:       batchAccountTestStatusFailed,
		ErrorMessage: "body mentions 401 but status is forbidden",
		HTTPStatus:   http.StatusForbidden,
	}

	updateBatchAccountTestProgress(&progress, resultItem)
	require.Equal(t, 1, progress.Failed)
	require.Equal(t, 0, progress.Unauthorized)

	resultItem.HTTPStatus = http.StatusUnauthorized
	updateBatchAccountTestProgress(&progress, resultItem)
	require.Equal(t, 2, progress.Failed)
	require.Equal(t, 1, progress.Unauthorized)
}

func TestRunTestBackgroundCapturesHTTPStatusInternally(t *testing.T) {
	recorderErr := &accountTestHTTPStatusError{
		statusCode: http.StatusUnauthorized,
		message:    "API returned 401",
	}

	var httpStatusErr *accountTestHTTPStatusError
	require.ErrorAs(t, recorderErr, &httpStatusErr)
	require.Equal(t, http.StatusUnauthorized, httpStatusErr.statusCode)
}

func TestAntigravityTestConnectionHTTPStatusError(t *testing.T) {
	recorderErr := &antigravityTestConnectionHTTPStatusError{
		statusCode: http.StatusUnauthorized,
		message:    "API returned 401",
	}

	var httpStatusErr *antigravityTestConnectionHTTPStatusError
	require.True(t, errors.As(recorderErr, &httpStatusErr))
	require.Equal(t, http.StatusUnauthorized, httpStatusErr.statusCode)
}
