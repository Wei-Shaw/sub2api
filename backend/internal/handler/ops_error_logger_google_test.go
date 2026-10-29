package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// recordGoogleFormatOpsError 让 handle 以 Google 错误格式回写错误，返回 ops 中间件记录的错误日志。
func recordGoogleFormatOpsError(t *testing.T, handle gin.HandlerFunc) *service.OpsInsertErrorLogInput {
	t.Helper()
	setupOpsErrorLogTestQueue(t, 1)
	gin.SetMode(gin.TestMode)
	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.POST("/v1beta/cachedContents", handle)

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1beta/cachedContents", nil))

	require.Equal(t, int64(1), OpsErrorLogQueueLength())
	return (<-opsErrorLogQueue).entry
}

func TestOpsErrorLoggerMiddleware_GoogleClientErrorsClassifiedAsClient(t *testing.T) {
	for _, tc := range []struct {
		status  int
		errType string
		phase   string
	}{
		{status: http.StatusBadRequest, errType: "invalid_request_error", phase: "request"},
		{status: http.StatusRequestEntityTooLarge, errType: "invalid_request_error", phase: "request"},
		{status: http.StatusUnauthorized, errType: "authentication_error", phase: "auth"},
		{status: http.StatusForbidden, errType: "permission_error", phase: "request"},
		{status: http.StatusNotFound, errType: "not_found_error", phase: "request"},
	} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			entry := recordGoogleFormatOpsError(t, func(c *gin.Context) {
				googleError(c, tc.status, "client request problem")
			})
			require.Equal(t, tc.status, entry.StatusCode)
			require.Equal(t, tc.errType, entry.ErrorType)
			require.Equal(t, tc.phase, entry.ErrorPhase)
			require.Equal(t, "client", entry.ErrorOwner)
			require.Equal(t, "client_request", entry.ErrorSource)
			require.Equal(t, "P3", entry.Severity)
		})
	}
}

func TestOpsErrorLoggerMiddleware_GooglePlatformErrorStaysPlatform(t *testing.T) {
	entry := recordGoogleFormatOpsError(t, func(c *gin.Context) {
		googlePlatformError(c, http.StatusBadRequest, "Explicit context caching is not available for model m: storage pricing is not configured")
	})
	require.Equal(t, "api_error", entry.ErrorType)
	require.Equal(t, "internal", entry.ErrorPhase)
	require.Equal(t, "platform", entry.ErrorOwner)
	require.Equal(t, "gateway", entry.ErrorSource)
}

func TestOpsErrorLoggerMiddleware_GoogleServerErrorsNotTypedByStatus(t *testing.T) {
	entry := recordGoogleFormatOpsError(t, func(c *gin.Context) {
		googleError(c, http.StatusInternalServerError, "Failed to persist cached content")
	})
	require.Equal(t, "api_error", entry.ErrorType)
	require.Equal(t, "platform", entry.ErrorOwner)
}

func TestOpsErrorLoggerMiddleware_GoogleBillingErrorMatchesClaudeFormat(t *testing.T) {
	entry := recordGoogleFormatOpsError(t, func(c *gin.Context) {
		status, code, message, _ := billingErrorDetails(service.ErrInsufficientBalance)
		googleErrorWithType(c, status, code, "", message)
	})
	require.Equal(t, "billing_error", entry.ErrorType)
	require.Equal(t, "request", entry.ErrorPhase)
	require.Equal(t, "client", entry.ErrorOwner)
	require.True(t, entry.IsBusinessLimited)
}

func TestOpsErrorLoggerMiddleware_GoogleConcurrencyErrorClassifiedAsRequest(t *testing.T) {
	entry := recordGoogleFormatOpsError(t, func(c *gin.Context) {
		googleConcurrencyError(c, &ConcurrencyError{SlotType: "user"}, "user")
	})
	require.Equal(t, http.StatusTooManyRequests, entry.StatusCode)
	require.Equal(t, "rate_limit_error", entry.ErrorType)
	require.Equal(t, "request", entry.ErrorPhase)
	require.Equal(t, "client", entry.ErrorOwner)
}

func TestOpsErrorLoggerMiddleware_GoogleErrorAfterUpstreamFailureKeepsUpstreamAttribution(t *testing.T) {
	entry := recordGoogleFormatOpsError(t, func(c *gin.Context) {
		service.SetOpsUpstreamError(c, http.StatusBadRequest, "upstream rejected", "")
		googleError(c, http.StatusBadRequest, "upstream rejected")
	})
	require.Equal(t, "upstream", entry.ErrorPhase)
	require.Equal(t, "provider", entry.ErrorOwner)
	require.Equal(t, "upstream_http", entry.ErrorSource)
}

func TestOpsErrorLoggerMiddleware_RequestScopedUpstreamErrorClassifiedAsClient(t *testing.T) {
	entry := recordGoogleFormatOpsError(t, func(c *gin.Context) {
		service.SetOpsUpstreamError(c, http.StatusBadRequest, "Invalid resource state for cache content 8079767076522164224.", "")
		c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{{
			Platform: service.PlatformGemini, AccountID: 173, UpstreamStatusCode: http.StatusBadRequest, Kind: "http_error",
			Message: "Invalid resource state for cache content 8079767076522164224.",
		}})
		service.MarkOpsRequestScopedError(c, "permission_error")
		c.Data(http.StatusForbidden, "application/json", []byte(service.GeminiCachedContentNotFoundResponse))
	})
	require.Equal(t, http.StatusForbidden, entry.StatusCode)
	require.Equal(t, "permission_error", entry.ErrorType)
	require.Equal(t, "request", entry.ErrorPhase)
	require.Equal(t, "client", entry.ErrorOwner)
	require.Equal(t, "client_request", entry.ErrorSource)
	require.False(t, entry.IsBusinessLimited)
	require.NotNil(t, entry.UpstreamStatusCode, "上游原始错误保留为明细")
	require.Equal(t, http.StatusBadRequest, *entry.UpstreamStatusCode)
}

func TestSetOpsLocalErrorTypeClearsRequestScopedMark(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	service.MarkOpsRequestScopedError(c, "permission_error")
	require.True(t, service.IsOpsRequestScopedError(c))

	service.SetOpsLocalErrorType(c, "", "")
	require.False(t, service.IsOpsRequestScopedError(c))
	errType, code := service.OpsLocalErrorType(c)
	require.Empty(t, errType)
	require.Empty(t, code)
}
