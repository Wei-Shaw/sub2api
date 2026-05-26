package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestClassifyOpenAIWSAcquireError(t *testing.T) {
	t.Run("dial_426_upgrade_required", func(t *testing.T) {
		err := &openAIWSDialError{StatusCode: 426, Err: errors.New("upgrade required")}
		require.Equal(t, "upgrade_required", classifyOpenAIWSAcquireError(err))
	})

	t.Run("queue_full", func(t *testing.T) {
		require.Equal(t, "conn_queue_full", classifyOpenAIWSAcquireError(errOpenAIWSConnQueueFull))
	})

	t.Run("preferred_conn_unavailable", func(t *testing.T) {
		require.Equal(t, "preferred_conn_unavailable", classifyOpenAIWSAcquireError(errOpenAIWSPreferredConnUnavailable))
	})

	t.Run("acquire_timeout", func(t *testing.T) {
		require.Equal(t, "acquire_timeout", classifyOpenAIWSAcquireError(context.DeadlineExceeded))
	})

	t.Run("auth_failed_401", func(t *testing.T) {
		err := &openAIWSDialError{StatusCode: 401, Err: errors.New("unauthorized")}
		require.Equal(t, "auth_failed", classifyOpenAIWSAcquireError(err))
	})

	t.Run("upstream_rate_limited", func(t *testing.T) {
		err := &openAIWSDialError{StatusCode: 429, Err: errors.New("rate limited")}
		require.Equal(t, "upstream_rate_limited", classifyOpenAIWSAcquireError(err))
	})

	t.Run("upstream_5xx", func(t *testing.T) {
		err := &openAIWSDialError{StatusCode: 502, Err: errors.New("bad gateway")}
		require.Equal(t, "upstream_5xx", classifyOpenAIWSAcquireError(err))
	})

	t.Run("dial_failed_other_status", func(t *testing.T) {
		err := &openAIWSDialError{StatusCode: 418, Err: errors.New("teapot")}
		require.Equal(t, "dial_failed", classifyOpenAIWSAcquireError(err))
	})

	t.Run("other", func(t *testing.T) {
		require.Equal(t, "acquire_conn", classifyOpenAIWSAcquireError(errors.New("x")))
	})

	t.Run("nil", func(t *testing.T) {
		require.Equal(t, "acquire_conn", classifyOpenAIWSAcquireError(nil))
	})
}

func TestClassifyOpenAIWSDialError(t *testing.T) {
	t.Run("handshake_not_finished", func(t *testing.T) {
		err := &openAIWSDialError{
			StatusCode: http.StatusBadGateway,
			Err:        errors.New("WebSocket protocol error: Handshake not finished"),
		}
		require.Equal(t, "handshake_not_finished", classifyOpenAIWSDialError(err))
	})

	t.Run("context_deadline", func(t *testing.T) {
		err := &openAIWSDialError{
			StatusCode: 0,
			Err:        context.DeadlineExceeded,
		}
		require.Equal(t, "ctx_deadline_exceeded", classifyOpenAIWSDialError(err))
	})
}

func TestSummarizeOpenAIWSDialError(t *testing.T) {
	err := &openAIWSDialError{
		StatusCode: http.StatusBadGateway,
		ResponseHeaders: http.Header{
			"Server":       []string{"cloudflare"},
			"Via":          []string{"1.1 example"},
			"Cf-Ray":       []string{"abcd1234"},
			"X-Request-Id": []string{"req_123"},
		},
		Err: errors.New("WebSocket protocol error: Handshake not finished"),
	}

	status, class, closeStatus, closeReason, server, via, cfRay, reqID := summarizeOpenAIWSDialError(err)
	require.Equal(t, http.StatusBadGateway, status)
	require.Equal(t, "handshake_not_finished", class)
	require.Equal(t, "-", closeStatus)
	require.Equal(t, "-", closeReason)
	require.Equal(t, "cloudflare", server)
	require.Equal(t, "1.1 example", via)
	require.Equal(t, "abcd1234", cfRay)
	require.Equal(t, "req_123", reqID)
}

func TestClassifyOpenAIWSErrorEvent(t *testing.T) {
	reason, recoverable := classifyOpenAIWSErrorEvent([]byte(`{"type":"error","error":{"code":"upgrade_required","message":"Upgrade required"}}`))
	require.Equal(t, "upgrade_required", reason)
	require.True(t, recoverable)

	reason, recoverable = classifyOpenAIWSErrorEvent([]byte(`{"type":"error","error":{"code":"previous_response_not_found","message":"not found"}}`))
	require.Equal(t, "previous_response_not_found", reason)
	require.True(t, recoverable)
}

func TestClassifyOpenAIWSReconnectReason(t *testing.T) {
	reason, retryable := classifyOpenAIWSReconnectReason(wrapOpenAIWSFallback("policy_violation", errors.New("policy")))
	require.Equal(t, "policy_violation", reason)
	require.False(t, retryable)

	reason, retryable = classifyOpenAIWSReconnectReason(wrapOpenAIWSFallback("read_event", errors.New("io")))
	require.Equal(t, "read_event", reason)
	require.True(t, retryable)
}

func TestOpenAIWSErrorHTTPStatus(t *testing.T) {
	require.Equal(t, http.StatusBadRequest, openAIWSErrorHTTPStatus([]byte(`{"type":"error","error":{"type":"invalid_request_error","code":"invalid_request","message":"invalid input"}}`)))
	require.Equal(t, http.StatusUnauthorized, openAIWSErrorHTTPStatus([]byte(`{"type":"error","error":{"type":"authentication_error","code":"invalid_api_key","message":"auth failed"}}`)))
	require.Equal(t, http.StatusForbidden, openAIWSErrorHTTPStatus([]byte(`{"type":"error","error":{"type":"permission_error","code":"forbidden","message":"forbidden"}}`)))
	require.Equal(t, http.StatusTooManyRequests, openAIWSErrorHTTPStatus([]byte(`{"type":"error","error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"rate limited"}}`)))
	require.Equal(t, http.StatusBadGateway, openAIWSErrorHTTPStatus([]byte(`{"type":"error","error":{"type":"server_error","code":"server_error","message":"server"}}`)))
}

func TestResolveOpenAIWSFallbackErrorResponse(t *testing.T) {
	t.Run("previous_response_not_found", func(t *testing.T) {
		statusCode, errType, clientMessage, upstreamMessage, ok := resolveOpenAIWSFallbackErrorResponse(
			wrapOpenAIWSFallback("previous_response_not_found", errors.New("previous response not found")),
		)
		require.True(t, ok)
		require.Equal(t, http.StatusBadRequest, statusCode)
		require.Equal(t, "invalid_request_error", errType)
		require.Equal(t, "previous response not found", clientMessage)
		require.Equal(t, "previous response not found", upstreamMessage)
	})

	t.Run("auth_failed_uses_dial_status", func(t *testing.T) {
		statusCode, errType, clientMessage, upstreamMessage, ok := resolveOpenAIWSFallbackErrorResponse(
			wrapOpenAIWSFallback("auth_failed", &openAIWSDialError{
				StatusCode: http.StatusForbidden,
				Err:        errors.New("forbidden"),
			}),
		)
		require.True(t, ok)
		require.Equal(t, http.StatusForbidden, statusCode)
		require.Equal(t, "upstream_error", errType)
		require.Equal(t, "forbidden", clientMessage)
		require.Equal(t, "forbidden", upstreamMessage)
	})

	t.Run("non_fallback_error_not_resolved", func(t *testing.T) {
		_, _, _, _, ok := resolveOpenAIWSFallbackErrorResponse(errors.New("plain error"))
		require.False(t, ok)
	})
}

func TestOpenAIWSFallbackCooling(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAIWS.FallbackCooldownSeconds = 1

	require.False(t, svc.isOpenAIWSFallbackCooling(1))
	svc.markOpenAIWSFallbackCooling(1, "upgrade_required")
	require.True(t, svc.isOpenAIWSFallbackCooling(1))

	svc.clearOpenAIWSFallbackCooling(1)
	require.False(t, svc.isOpenAIWSFallbackCooling(1))

	svc.markOpenAIWSFallbackCooling(2, "x")
	time.Sleep(1200 * time.Millisecond)
	require.False(t, svc.isOpenAIWSFallbackCooling(2))
}

func TestOpenAIWSRetryBackoff(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAIWS.RetryBackoffInitialMS = 100
	svc.cfg.Gateway.OpenAIWS.RetryBackoffMaxMS = 400
	svc.cfg.Gateway.OpenAIWS.RetryJitterRatio = 0

	require.Equal(t, time.Duration(100)*time.Millisecond, svc.openAIWSRetryBackoff(1))
	require.Equal(t, time.Duration(200)*time.Millisecond, svc.openAIWSRetryBackoff(2))
	require.Equal(t, time.Duration(400)*time.Millisecond, svc.openAIWSRetryBackoff(3))
	require.Equal(t, time.Duration(400)*time.Millisecond, svc.openAIWSRetryBackoff(4))
}

func TestOpenAIWSRetryTotalBudget(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAIWS.RetryTotalBudgetMS = 1200
	require.Equal(t, 1200*time.Millisecond, svc.openAIWSRetryTotalBudget())

	svc.cfg.Gateway.OpenAIWS.RetryTotalBudgetMS = 0
	require.Equal(t, time.Duration(0), svc.openAIWSRetryTotalBudget())
}

func TestClassifyOpenAIWSReadFallbackReason(t *testing.T) {
	require.Equal(t, "policy_violation", classifyOpenAIWSReadFallbackReason(coderws.CloseError{Code: coderws.StatusPolicyViolation}))
	require.Equal(t, "message_too_big", classifyOpenAIWSReadFallbackReason(coderws.CloseError{Code: coderws.StatusMessageTooBig}))
	require.Equal(t, "read_event", classifyOpenAIWSReadFallbackReason(errors.New("io")))
}

func TestOpenAIWSStoreDisabledConnMode(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAIWS.StoreDisabledForceNewConn = true
	require.Equal(t, openAIWSStoreDisabledConnModeStrict, svc.openAIWSStoreDisabledConnMode())

	svc.cfg.Gateway.OpenAIWS.StoreDisabledConnMode = "adaptive"
	require.Equal(t, openAIWSStoreDisabledConnModeAdaptive, svc.openAIWSStoreDisabledConnMode())

	svc.cfg.Gateway.OpenAIWS.StoreDisabledConnMode = ""
	svc.cfg.Gateway.OpenAIWS.StoreDisabledForceNewConn = false
	require.Equal(t, openAIWSStoreDisabledConnModeOff, svc.openAIWSStoreDisabledConnMode())
}

func TestShouldForceNewConnOnStoreDisabled(t *testing.T) {
	require.True(t, shouldForceNewConnOnStoreDisabled(openAIWSStoreDisabledConnModeStrict, ""))
	require.False(t, shouldForceNewConnOnStoreDisabled(openAIWSStoreDisabledConnModeOff, "policy_violation"))

	require.True(t, shouldForceNewConnOnStoreDisabled(openAIWSStoreDisabledConnModeAdaptive, "policy_violation"))
	require.True(t, shouldForceNewConnOnStoreDisabled(openAIWSStoreDisabledConnModeAdaptive, "prewarm_message_too_big"))
	require.False(t, shouldForceNewConnOnStoreDisabled(openAIWSStoreDisabledConnModeAdaptive, "read_event"))
}

func TestOpenAIWSRetryMetricsSnapshot(t *testing.T) {
	svc := &OpenAIGatewayService{}
	svc.recordOpenAIWSRetryAttempt(150 * time.Millisecond)
	svc.recordOpenAIWSRetryAttempt(0)
	svc.recordOpenAIWSRetryExhausted()
	svc.recordOpenAIWSNonRetryableFastFallback()

	snapshot := svc.SnapshotOpenAIWSRetryMetrics()
	require.Equal(t, int64(2), snapshot.RetryAttemptsTotal)
	require.Equal(t, int64(150), snapshot.RetryBackoffMsTotal)
	require.Equal(t, int64(1), snapshot.RetryExhaustedTotal)
	require.Equal(t, int64(1), snapshot.NonRetryableFastFallbackTotal)
}

func TestShouldLogOpenAIWSPayloadSchema(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}

	svc.cfg.Gateway.OpenAIWS.PayloadLogSampleRate = 0
	require.True(t, svc.shouldLogOpenAIWSPayloadSchema(1), "首次尝试应始终记录 payload_schema")
	require.False(t, svc.shouldLogOpenAIWSPayloadSchema(2))

	svc.cfg.Gateway.OpenAIWS.PayloadLogSampleRate = 1
	require.True(t, svc.shouldLogOpenAIWSPayloadSchema(2))
}
