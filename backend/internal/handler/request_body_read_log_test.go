package handler

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type diagnosticFailingBody struct {
	payload []byte
	err     error
	read    bool
}

func (r *diagnosticFailingBody) Read(p []byte) (int, error) {
	if r.read {
		if r.err == nil {
			return 0, io.EOF
		}
		return 0, r.err
	}
	r.read = true
	return copy(p, r.payload), r.err
}

func (r *diagnosticFailingBody) Close() error { return nil }

func diagnosticLogFields(t *testing.T, logs *observer.ObservedLogs) (observer.LoggedEntry, map[string]any) {
	t.Helper()
	entries := logs.All()
	require.Len(t, entries, 1)
	return entries[0], diagnosticFields(entries[0])
}

func diagnosticFields(entry observer.LoggedEntry) map[string]any {
	fields := make(map[string]any, len(entry.Context))
	for _, field := range entry.Context {
		switch field.Type {
		case zapcore.StringType:
			fields[field.Key] = field.String
		case zapcore.Int64Type:
			fields[field.Key] = field.Integer
		default:
			fields[field.Key] = field.Interface
		}
	}
	return fields
}

func diagnosticWarnFields(t *testing.T, logs *observer.ObservedLogs) (observer.LoggedEntry, map[string]any) {
	t.Helper()
	entries := logs.FilterLevelExact(zap.WarnLevel).All()
	require.Len(t, entries, 1)
	return entries[0], diagnosticFields(entries[0])
}

func requireNoSensitiveDiagnosticData(t *testing.T, entry observer.LoggedEntry, sensitive ...string) {
	t.Helper()
	for _, field := range entry.Context {
		for _, value := range sensitive {
			require.NotContains(t, field.String, value)
			if field.Interface != nil {
				require.NotContains(t, fmt.Sprint(field.Interface), value)
			}
		}
	}
}

func newDiagnosticLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zap.DebugLevel)
	return zap.New(core), logs
}

func TestGatewayResponses_LogsRequestBodyReadFailureAndKeepsGeneric400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestLogger, logs := newDiagnosticLogger()
	bodySecret := "anthropic-body-secret"
	errorSecret := "anthropic-transport-secret"
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	req.Body = &diagnosticFailingBody{payload: []byte(bodySecret), err: errors.New(errorSecret)}
	req.ContentLength = 123
	req.Header.Set("Content-Encoding", "untrusted-secret-encoding")
	req.TransferEncoding = []string{"chunked"}
	req = req.WithContext(logger.IntoContext(req.Context(), requestLogger))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 7})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 9})

	(&GatewayHandler{}).Responses(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Failed to read request body")
	entry, fields := diagnosticLogFields(t, logs)
	require.Equal(t, zap.WarnLevel, entry.Level)
	require.Equal(t, "request body read failed", entry.Message)
	require.Equal(t, "transport_read", fields["error_stage"])
	require.Equal(t, "read_failed", fields["error_class"])
	require.Equal(t, int64(len(bodySecret)), fields["bytes_read"])
	require.Equal(t, "raw_transport", fields["bytes_read_kind"])
	require.Equal(t, int64(123), fields["content_length"])
	require.Equal(t, "other", fields["content_encoding"])
	require.Equal(t, "chunked", fields["transfer_encoding"])
	require.GreaterOrEqual(t, fields["elapsed_ms"].(int64), int64(0))
	requireNoSensitiveDiagnosticData(t, entry, bodySecret, errorSecret, "untrusted-secret-encoding")
}

func TestOpenAIResponses_LogsRequestBodyReadFailureAndKeepsGeneric400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestLogger, logs := newDiagnosticLogger()
	bodySecret := "openai-body-secret"
	errorSecret := "openai-transport-secret"
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	req.Body = &diagnosticFailingBody{payload: []byte(bodySecret), err: errors.New(errorSecret)}
	req.ContentLength = -1
	req.Header.Set("Content-Encoding", "gzip")
	req = req.WithContext(logger.IntoContext(req.Context(), requestLogger))

	h := newOpenAIReadDiagnosticHandler(t)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 8})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 10})

	h.Responses(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Failed to read request body")
	entry, fields := diagnosticLogFields(t, logs)
	require.Equal(t, zap.WarnLevel, entry.Level)
	require.Equal(t, "request body read failed", entry.Message)
	require.Equal(t, "transport_read", fields["error_stage"])
	require.Equal(t, "read_failed", fields["error_class"])
	require.Equal(t, int64(len(bodySecret)), fields["bytes_read"])
	require.Equal(t, "raw_transport", fields["bytes_read_kind"])
	require.Equal(t, int64(-1), fields["content_length"])
	require.Equal(t, "gzip", fields["content_encoding"])
	require.Equal(t, "unknown", fields["transfer_encoding"])
	require.GreaterOrEqual(t, fields["elapsed_ms"].(int64), int64(0))
	requireNoSensitiveDiagnosticData(t, entry, bodySecret, errorSecret)
}

func TestGatewayResponses_LogsCorruptedGzipAsDecompressionFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestLogger, logs := newDiagnosticLogger()
	var compressed bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressed)
	_, err := gzipWriter.Write([]byte("compressed-body-secret"))
	require.NoError(t, err)
	require.NoError(t, gzipWriter.Close())
	corrupted := compressed.Bytes()[:compressed.Len()-1]

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	req.Body = &diagnosticFailingBody{payload: corrupted, err: nil}
	req.ContentLength = int64(len(corrupted))
	req.Header.Set("Content-Encoding", "gzip")
	req = req.WithContext(logger.IntoContext(req.Context(), requestLogger))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 11})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 12})

	(&GatewayHandler{}).Responses(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Failed to read request body")
	entry, fields := diagnosticLogFields(t, logs)
	require.Equal(t, "decompress", fields["error_stage"])
	require.Equal(t, "decompression_failed", fields["error_class"])
	require.Equal(t, int64(len(corrupted)), fields["bytes_read"])
	require.Equal(t, "raw_transport", fields["bytes_read_kind"])
	require.Equal(t, int64(len(corrupted)), fields["content_length"])
	require.Equal(t, "gzip", fields["content_encoding"])
	require.Equal(t, "content_length", fields["transfer_encoding"])
	requireNoSensitiveDiagnosticData(t, entry, "compressed-body-secret")
}

func TestLogRequestBodyReadFailure_UsesSafeFramingValues(t *testing.T) {
	requestLogger, logs := newDiagnosticLogger()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	req.ContentLength = -1
	req.Header.Set("Content-Encoding", "gzip, secret-header-value")
	req.TransferEncoding = []string{"secret-transfer-value"}

	logRequestBodyReadFailure(
		requestLogger,
		newRequestBodyReadLogContext(req),
		errors.New("raw-error-secret"),
		time.Now().Add(-time.Millisecond),
	)

	entry, fields := diagnosticLogFields(t, logs)
	require.Equal(t, "transport_read", fields["error_stage"])
	require.Equal(t, "read_failed", fields["error_class"])
	require.Equal(t, int64(0), fields["bytes_read"])
	require.Equal(t, "raw_transport", fields["bytes_read_kind"])
	require.Equal(t, int64(-1), fields["content_length"])
	require.Equal(t, "other", fields["content_encoding"])
	require.Equal(t, "other", fields["transfer_encoding"])
	requireNoSensitiveDiagnosticData(t, entry, "secret-header-value", "secret-transfer-value", "raw-error-secret")
}

func TestRequestBodyReadFailureDiagnosticsCoverHighRelevanceEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		path   string
		invoke func(*testing.T, *gin.Context)
	}{
		{
			name: "gateway messages",
			path: "/v1/messages",
			invoke: func(_ *testing.T, c *gin.Context) {
				(&GatewayHandler{}).Messages(c)
			},
		},
		{
			name: "gateway chat completions",
			path: "/v1/chat/completions",
			invoke: func(_ *testing.T, c *gin.Context) {
				(&GatewayHandler{}).ChatCompletions(c)
			},
		},
		{
			name: "gateway count tokens",
			path: "/v1/messages/count_tokens",
			invoke: func(_ *testing.T, c *gin.Context) {
				(&GatewayHandler{}).CountTokens(c)
			},
		},
		{
			name: "openai messages",
			path: "/v1/messages",
			invoke: func(t *testing.T, c *gin.Context) {
				newOpenAIReadDiagnosticHandler(t).Messages(c)
			},
		},
		{
			name: "openai chat completions",
			path: "/openai/v1/chat/completions",
			invoke: func(t *testing.T, c *gin.Context) {
				newOpenAIReadDiagnosticHandler(t).ChatCompletions(c)
			},
		},
		{
			name: "openai grok count tokens",
			path: "/v1/messages/count_tokens",
			invoke: func(t *testing.T, c *gin.Context) {
				newOpenAIReadDiagnosticHandler(t).GrokCountTokens(c)
			},
		},
		{
			name: "openai count tokens",
			path: "/v1/messages/count_tokens",
			invoke: func(t *testing.T, c *gin.Context) {
				newOpenAIReadDiagnosticHandler(t).CountTokens(c)
			},
		},
	}

	failures := []struct {
		name           string
		err            error
		status         int
		class          string
		responseString string
	}{
		{
			name:           "generic 400",
			err:            errors.New("high-relevance-raw-error-secret"),
			status:         http.StatusBadRequest,
			class:          "read_failed",
			responseString: "Failed to read request body",
		},
		{
			name:           "max bytes 413",
			err:            &http.MaxBytesError{Limit: 4},
			status:         http.StatusRequestEntityTooLarge,
			class:          "body_too_large",
			responseString: "Request body too large",
		},
	}

	for _, tt := range tests {
		for _, failure := range failures {
			t.Run(tt.name+"/"+failure.name, func(t *testing.T) {
				requestLogger, logs := newDiagnosticLogger()
				bodySecret := "high-relevance-body-secret"
				errorSecret := "high-relevance-raw-error-secret"
				req := httptest.NewRequest(http.MethodPost, tt.path, nil)
				req.Body = &diagnosticFailingBody{payload: []byte(bodySecret), err: failure.err}
				req.ContentLength = int64(len(bodySecret) + 3)
				req.Header.Set("Content-Encoding", "gzip")
				req.TransferEncoding = []string{"chunked"}
				req = req.WithContext(logger.IntoContext(req.Context(), requestLogger))

				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = req
				c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 17})
				c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 19})

				tt.invoke(t, c)

				require.Equal(t, failure.status, recorder.Code)
				require.Contains(t, recorder.Body.String(), failure.responseString)
				require.NotContains(t, recorder.Body.String(), bodySecret)
				require.NotContains(t, recorder.Body.String(), errorSecret)

				entry, fields := diagnosticWarnFields(t, logs)
				require.Equal(t, zap.WarnLevel, entry.Level)
				require.Equal(t, "request body read failed", entry.Message)
				require.Equal(t, "transport_read", fields["error_stage"])
				require.Equal(t, failure.class, fields["error_class"])
				require.Equal(t, int64(len(bodySecret)), fields["bytes_read"])
				require.Equal(t, "raw_transport", fields["bytes_read_kind"])
				require.Equal(t, int64(len(bodySecret)+3), fields["content_length"])
				require.Equal(t, "gzip", fields["content_encoding"])
				require.Equal(t, "chunked", fields["transfer_encoding"])
				require.GreaterOrEqual(t, fields["elapsed_ms"].(int64), int64(0))
				requireNoSensitiveDiagnosticData(t, entry, bodySecret, errorSecret)
			})
		}
	}
}

func newOpenAIReadDiagnosticHandler(t *testing.T) *OpenAIGatewayHandler {
	t.Helper()
	cfg := &config.Config{RunMode: config.RunModeSimple}
	concurrencyService := service.NewConcurrencyService(nil)
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheService.Stop)
	apiKeyService := service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg)
	gatewayService := service.NewOpenAIGatewayService(
		nil, // accountRepo
		nil, // usageLogRepo
		nil, // usageBillingRepo
		nil, // userRepo
		nil, // userSubRepo
		nil, // userGroupRateRepo
		nil, // cache
		cfg,
		nil, // schedulerSnapshot
		concurrencyService,
		nil, // billingService
		nil, // rateLimitService
		billingCacheService,
		nil, // httpUpstream
		nil, // deferredService
		nil, // openAITokenProvider
		nil, // grokTokenProvider
		nil, // resolver
		nil, // channelService
		nil, // balanceNotifyService
		nil, // settingService
		nil, // userPlatformQuotaRepo
	)
	return NewOpenAIGatewayHandler(
		gatewayService,
		concurrencyService,
		billingCacheService,
		apiKeyService,
		nil,
		nil,
		nil,
		nil,
		cfg,
	)
}
