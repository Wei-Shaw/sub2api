//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGatewayChatCredentialStopDoesNotSelectAnotherAccountAndReturnsSafe503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stopErr := &service.UpstreamFailoverError{
		Stage:             service.GatewayFailureStageAccountAuth,
		Scope:             service.GatewayFailureScopeProvider,
		Reason:            service.GrokCredentialReasonProviderConfig,
		NextAccountAction: service.NextAccountStop,
		ClientStatusCode:  http.StatusTeapot,
		ClientMessage:     "invalid_client client_secret=must-not-leak",
	}
	state := NewFailoverState(3, false)
	action := state.HandleFailoverError(context.Background(), &mockTempUnscheduler{}, 71, service.PlatformGrok, 0, stopErr)

	require.Equal(t, FailoverExhausted, action)
	require.Zero(t, state.SwitchCount)
	require.Empty(t, state.FailedAccountIDs)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	(&GatewayHandler{}).handleCCFailoverExhausted(c, state.LastFailoverErr, false)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), service.GrokCredentialUnavailableClientMessage)
	require.NotContains(t, recorder.Body.String(), "invalid_client")
	require.NotContains(t, recorder.Body.String(), "client_secret")
}

func TestGatewayChatAntigravityCredentialFailureReturnsActionableMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	(&GatewayHandler{}).handleCCFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:        http.StatusUnauthorized,
		Stage:             service.GatewayFailureStageAccountAuth,
		Scope:             service.GatewayFailureScopeAccount,
		Reason:            service.AntigravityCredentialRejectedReason,
		NextAccountAction: service.NextAccountRetry,
		ClientStatusCode:  http.StatusBadGateway,
		ClientMessage:     service.AntigravityCredentialRejectedClientMessage,
		ResponseBody:      []byte(`{"error":{"message":"Invalid bearer token","refresh_token":"must-not-leak"}}`),
	}, false)

	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.Contains(t, recorder.Body.String(), service.AntigravityCredentialRejectedClientMessage)
	require.NotContains(t, strings.ToLower(recorder.Body.String()), "bearer")
	require.NotContains(t, strings.ToLower(recorder.Body.String()), "refresh_token")
}

func TestOpenAIAccessStateCredentialFailureUsesTypedSafeResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:        http.StatusForbidden,
		Stage:             service.GatewayFailureStageAccountAuth,
		Scope:             service.GatewayFailureScopeAccount,
		Reason:            service.OpenAIUpstreamAccessStateReason,
		NextAccountAction: service.NextAccountRetry,
		ClientStatusCode:  http.StatusBadGateway,
		ClientMessage:     "Upstream access is temporarily unavailable, please retry later",
		ResponseBody:      []byte(`{"error":{"message":"Your workspace is deactivated","token":"must-not-leak"}}`),
	}, false)

	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Upstream access is temporarily unavailable")
	require.NotContains(t, strings.ToLower(recorder.Body.String()), "deactivated")
	require.NotContains(t, recorder.Body.String(), "must-not-leak")
}

func TestOpenAICapacityFailoverExhaustionPreservesMessageAsServerError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	message := "Our servers are currently overloaded. Please try again later."
	failoverErr := &service.UpstreamFailoverError{
		StatusCode:             http.StatusBadRequest,
		ResponseBody:           []byte(`{"error":{"code":"server_is_overloaded","message":"` + message + `"}}`),
		RetryableOnSameAccount: true,
		RequestScopedTransient: true,
		ClientStatusCode:       http.StatusServiceUnavailable,
		ClientMessage:          message,
	}

	t.Run("native_openai", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, failoverErr, false)
		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		require.Equal(t, "server_error", gjson.Get(recorder.Body.String(), "error.type").String())
		require.Equal(t, message, gjson.Get(recorder.Body.String(), "error.message").String())
		require.NotContains(t, recorder.Body.String(), "server_is_overloaded")
	})

	t.Run("responses_compat", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		(&GatewayHandler{}).handleResponsesFailoverExhausted(c, failoverErr, false)
		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		require.Equal(t, "server_error", gjson.Get(recorder.Body.String(), "error.code").String())
		require.Equal(t, message, gjson.Get(recorder.Body.String(), "error.message").String())
	})

	t.Run("anthropic_compat", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		(&OpenAIGatewayHandler{}).handleAnthropicFailoverExhausted(c, failoverErr, false)
		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		require.Equal(t, "api_error", gjson.Get(recorder.Body.String(), "error.type").String())
		require.Equal(t, message, gjson.Get(recorder.Body.String(), "error.message").String())
	})
}

func TestResponsesFailoverExhaustedAfterForwardedTerminalMarksOpsWithoutDuplicateFrame(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	official := "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"code\":\"server_error\",\"message\":\"official failure\"}}}\n\n"
	_, err := c.Writer.Write([]byte(official))
	require.NoError(t, err)
	service.MarkOpsStreamError(c, "server_error", "official failure", http.StatusBadGateway)

	(&GatewayHandler{}).handleResponsesFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:   http.StatusBadGateway,
		ResponseBody: []byte(`{"error":{"message":"fallback failure"}}`),
	}, true)

	require.Equal(t, official, recorder.Body.String())
	streamErr, ok := service.GetOpsStreamError(c)
	require.True(t, ok)
	require.Equal(t, "official failure", streamErr.Message)

	markerRecorder := httptest.NewRecorder()
	markerContext, _ := gin.CreateTestContext(markerRecorder)
	(&GatewayHandler{}).handleResponsesFailoverExhausted(markerContext, &service.UpstreamFailoverError{
		StatusCode: http.StatusTooManyRequests,
	}, true)
	require.Contains(t, markerRecorder.Body.String(), "event: response.failed")
	require.Equal(t, 1, strings.Count(markerRecorder.Body.String(), "event: response.failed"))
	streamErr, ok = service.GetOpsStreamError(markerContext)
	require.True(t, ok)
	require.Equal(t, http.StatusTooManyRequests, streamErr.IntendedStatus)
	require.Equal(t, "rate_limit_error", streamErr.ErrType)

	heartbeatRecorder := httptest.NewRecorder()
	heartbeatContext, _ := gin.CreateTestContext(heartbeatRecorder)
	heartbeat := ": keepalive\n\n"
	written, err := heartbeatRecorder.Write([]byte(heartbeat))
	require.NoError(t, err)
	recordGatewayStreamHeartbeat(heartbeatContext, written)
	(&GatewayHandler{}).handleResponsesFailoverExhausted(heartbeatContext, &service.UpstreamFailoverError{
		StatusCode: http.StatusBadGateway,
	}, true)
	require.True(t, strings.HasPrefix(heartbeatRecorder.Body.String(), heartbeat))
	require.Equal(t, 1, strings.Count(heartbeatRecorder.Body.String(), "event: response.failed"))
}

func TestGatewayChatInferenceExhaustionRestoresRetryAfter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	(&GatewayHandler{}).handleCCFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:      http.StatusTooManyRequests,
		ResponseHeaders: http.Header{"Retry-After": []string{"45"}},
	}, false)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, "45", recorder.Header().Get("Retry-After"))
}

func TestCredentialFailoverExhaustionReturnsFixedSafe503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	h := &OpenAIGatewayHandler{}

	h.handleFailoverExhausted(c, &service.UpstreamFailoverError{
		Stage:             service.GatewayFailureStageAccountAuth,
		Scope:             service.GatewayFailureScopeAccount,
		Reason:            service.GrokCredentialReasonRevoked,
		NextAccountAction: service.NextAccountRetry,
		ClientStatusCode:  http.StatusTeapot,
		ClientMessage:     "invalid_grant refresh_token=must-not-leak",
	}, false)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), service.GrokCredentialUnavailableClientMessage)
	require.NotContains(t, strings.ToLower(recorder.Body.String()), "invalid_grant")
	require.NotContains(t, strings.ToLower(recorder.Body.String()), "refresh_token")
	require.NotContains(t, recorder.Body.String(), "must-not-leak")
}

func TestInferenceFailoverExhaustionRestoresRetryAfter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	h := &OpenAIGatewayHandler{}

	h.handleFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:      http.StatusTooManyRequests,
		ResponseHeaders: http.Header{"Retry-After": []string{"17"}},
	}, false)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, "17", recorder.Header().Get("Retry-After"))
}

func TestFailoverExhaustionRejectsSecretBearingRetryAfter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	h := &OpenAIGatewayHandler{}

	h.handleFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:      http.StatusTooManyRequests,
		ResponseHeaders: http.Header{"Retry-After": []string{"refresh_token=must-not-leak"}},
	}, false)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Empty(t, recorder.Header().Get("Retry-After"))
	require.NotContains(t, recorder.Body.String(), "must-not-leak")
}

func TestFailoverExhaustionRejectsFarFutureRetryAfterDate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	h := &OpenAIGatewayHandler{}

	h.handleFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode: http.StatusTooManyRequests,
		ResponseHeaders: http.Header{
			"Retry-After": []string{time.Now().Add(30 * 24 * time.Hour).UTC().Format(http.TimeFormat)},
		},
	}, false)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Empty(t, recorder.Header().Get("Retry-After"))
}

func TestFailoverExhaustionAllowsBoundedRetryAfterDate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	h := &OpenAIGatewayHandler{}
	retryAfter := time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)

	h.handleFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:      http.StatusTooManyRequests,
		ResponseHeaders: http.Header{"Retry-After": []string{retryAfter}},
	}, false)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, retryAfter, recorder.Header().Get("Retry-After"))
}

func TestOpsClassificationTreatsCredentialFailureAsAuthNotInference(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(service.OpsUpstreamStatusCodeKey, http.StatusForbidden)
	c.Set(service.OpsUpstreamErrorMessageKey, "stale inference message")
	c.Set(service.OpsUpstreamErrorDetailKey, "stale inference detail")
	c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{
		{Stage: string(service.GatewayFailureStageInference), UpstreamStatusCode: http.StatusForbidden, Message: "stale inference message", Detail: "stale inference detail"},
		{
			Stage:              string(service.GatewayFailureStageAccountAuth),
			Scope:              string(service.GatewayFailureScopeAccount),
			Reason:             string(service.GrokCredentialReasonRevoked),
			UpstreamStatusCode: 0,
			Message:            "Grok OAuth credentials require account action",
		},
	})

	phase, _, owner, source := classifyOpsErrorLog(c, "upstream_error", service.GrokCredentialUnavailableClientMessage, "", http.StatusServiceUnavailable)
	require.Equal(t, "account_auth", phase)
	require.Equal(t, "provider", owner)
	require.Equal(t, "gateway", source)

	entry := &service.OpsInsertErrorLogInput{}
	applyOpsUpstreamFieldsFromContext(c, entry)
	require.NotNil(t, entry.UpstreamStatusCode)
	require.Zero(t, *entry.UpstreamStatusCode)
	require.NotNil(t, entry.UpstreamErrorMessage)
	require.Equal(t, "Grok OAuth credentials require account action", *entry.UpstreamErrorMessage)
	require.Nil(t, entry.UpstreamErrorDetail)
	require.Len(t, entry.UpstreamErrors, 2)
	require.Equal(t, http.StatusForbidden, entry.UpstreamErrors[0].UpstreamStatusCode)
}

func TestOpsRecoveredCredentialFailoverDoesNotCreateRequestError(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 2)
	gin.SetMode(gin.TestMode)
	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.GET("/openai/v1/responses", func(c *gin.Context) {
		c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{
			{Stage: string(service.GatewayFailureStageInference), UpstreamStatusCode: http.StatusForbidden, Message: "earlier inference failure"},
			{
				Stage: string(service.GatewayFailureStageAccountAuth), Scope: string(service.GatewayFailureScopeAccount),
				Reason: string(service.GrokCredentialReasonRevoked), Message: "Grok OAuth credentials require account action",
			},
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/openai/v1/responses", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(1), OpsErrorLogQueueLength())
	job := <-opsErrorLogQueue
	require.Equal(t, http.StatusOK, job.entry.StatusCode)
	require.Equal(t, string(service.GatewayFailureStageAccountAuth), job.entry.ErrorPhase)
	require.NotNil(t, job.entry.UpstreamErrorsJSON)
	events, err := service.ParseOpsUpstreamErrors(*job.entry.UpstreamErrorsJSON)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, string(service.GatewayFailureStageAccountAuth), events[1].Stage)
}

func TestOpsWebSocketCredentialFailoverSuccessDoesNotCreateRequestError(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 2)
	gin.SetMode(gin.TestMode)
	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.GET("/openai/v1/responses", func(c *gin.Context) {
		c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{{
			Stage: string(service.GatewayFailureStageAccountAuth), Scope: string(service.GatewayFailureScopeAccount),
			Reason: string(service.GrokCredentialReasonRevoked), Message: "Grok OAuth credentials require account action",
		}})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/openai/v1/responses", nil)
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(1), OpsErrorLogQueueLength())
	job := <-opsErrorLogQueue
	require.Equal(t, http.StatusOK, job.entry.StatusCode)
	require.Equal(t, string(service.GatewayFailureStageAccountAuth), job.entry.ErrorPhase)
	require.NotNil(t, job.entry.UpstreamErrorsJSON)
	events, err := service.ParseOpsUpstreamErrors(*job.entry.UpstreamErrorsJSON)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, string(service.GatewayFailureStageAccountAuth), events[0].Stage)
}

func TestOpsWebSocketCredentialFailoverExhaustedIsRecorded(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 2)
	gin.SetMode(gin.TestMode)
	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.GET("/openai/v1/responses", func(c *gin.Context) {
		c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{{
			Stage: string(service.GatewayFailureStageAccountAuth), Scope: string(service.GatewayFailureScopeAccount),
			Reason: string(service.GrokCredentialReasonRevoked), Message: "Grok OAuth credentials require account action",
		}})
		closeOpenAIWSFailoverExhausted(c, nil, &service.UpstreamFailoverError{
			Stage:             service.GatewayFailureStageAccountAuth,
			Scope:             service.GatewayFailureScopeAccount,
			Reason:            service.GrokCredentialReasonRevoked,
			NextAccountAction: service.NextAccountStop,
		})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/openai/v1/responses", nil)
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(1), OpsErrorLogQueueLength())
	job := <-opsErrorLogQueue
	require.Equal(t, "account_auth", job.entry.ErrorPhase)
	require.Equal(t, http.StatusServiceUnavailable, job.entry.StatusCode)
	require.Equal(t, service.GrokCredentialUnavailableClientMessage, job.entry.ErrorMessage)
}
