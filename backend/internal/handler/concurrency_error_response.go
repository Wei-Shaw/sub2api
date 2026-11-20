package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
)

const statusClientClosedRequest = 499

func openAIWSUserSlotAcquireError(err error) *service.OpenAIWSClientCloseError {
	var queueErr *service.APIKeyQueueError
	if errors.As(err, &queueErr) {
		switch queueErr.Kind {
		case service.APIKeyQueueErrorFull:
			return service.NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, "API key wait queue is full; please retry later", err)
		case service.APIKeyQueueErrorTimeout:
			return service.NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, "timed out waiting for API key concurrency slot; please retry later", err)
		case service.APIKeyQueueErrorAuthRejected:
			return openAIWSQueueAuthRejectedClose(queueErr.Cause, err)
		default:
			// Capacity, policy and temporary verification failures are retryable.
			return service.NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, "API key wait queue is temporarily unavailable; please retry later", err)
		}
	}
	var limitErr *ConcurrencyError
	if errors.As(err, &limitErr) && limitErr.SlotType == "API key" {
		return service.NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, "API key concurrency limit reached; please retry later", err)
	}
	return service.NewOpenAIWSClientCloseError(coderws.StatusInternalError, "failed to acquire concurrency slot", err)
}

// openAIWSQueueAuthRejectedClose keeps the WS close code aligned with the
// business nature of a queued rejection: a definite identity/permission/model
// denial closes with 1008, while a retryable service or configuration change
// closes with 1013. Both 5xx flavors are retryable but must stay distinct: only
// a core API_KEY_GROUP_CHANGED is a configuration change; a temporary
// authentication read failure keeps a service-unavailable message.
func openAIWSQueueAuthRejectedClose(cause error, wrapper error) *service.OpenAIWSClientCloseError {
	status := infraerrors.Code(cause)
	switch {
	case status == http.StatusServiceUnavailable, status >= 500:
		message := "API key authentication is temporarily unavailable; please reconnect"
		if cause != nil && infraerrors.Reason(cause) == "API_KEY_GROUP_CHANGED" {
			message = "API key configuration changed; please reconnect"
		}
		return service.NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, message, wrapper)
	default:
		message := "API key authentication changed; please reconnect"
		if cause != nil {
			if text := strings.TrimSpace(infraerrors.Message(cause)); text != "" && text != infraerrors.UnknownMessage {
				message = text
			}
		}
		return service.NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, message, wrapper)
	}
}

const (
	gatewayQueueFullCode        = "gateway_queue_full"
	gatewayConcurrencyLimitCode = "gateway_concurrency_limit"
	apiKeyQueueFullCode         = "api_key_queue_full"
	apiKeyQueueTimeoutCode      = "api_key_queue_timeout"
)

// queueAuthRejectedResponse reuses the application error produced by the
// authentication middleware so a queued request that lost its authorization
// returns the same envelope as a fresh authentication attempt.
func queueAuthRejectedResponse(err error) (int, string, string, string) {
	ackErr := infraerrors.FromError(err)
	status := int(ackErr.Code)
	if status < 400 || status >= 600 || (status == infraerrors.UnknownCode && ackErr.Reason == "") {
		status = http.StatusUnauthorized
	}
	errType := "authentication_error"
	switch status {
	case http.StatusForbidden:
		errType = "permission_error"
	case http.StatusNotFound:
		// Model-level denial reuses the initial allowlist gate's not-found shape.
		errType = "not_found_error"
	case http.StatusTooManyRequests:
		errType = "rate_limit_error"
	case http.StatusServiceUnavailable:
		errType = "api_error"
	}
	message := ackErr.Message
	if message == "" || message == infraerrors.UnknownMessage {
		message = "API key authentication changed; please retry"
	}
	return status, errType, ackErr.Reason, message
}

func concurrencyErrorResponse(err error, slotType string) (int, string, string, string) {
	var queueErr *service.APIKeyQueueError
	if errors.As(err, &queueErr) {
		switch queueErr.Kind {
		case service.APIKeyQueueErrorFull:
			return http.StatusTooManyRequests, "rate_limit_error", apiKeyQueueFullCode,
				"API key wait queue is full, please retry later"
		case service.APIKeyQueueErrorTimeout:
			return http.StatusTooManyRequests, "rate_limit_error", apiKeyQueueTimeoutCode,
				"Timed out waiting for API key concurrency slot, please retry later"
		case service.APIKeyQueueErrorAuthRejected:
			if queueErr.Cause != nil {
				return queueAuthRejectedResponse(queueErr.Cause)
			}
			return http.StatusUnauthorized, "authentication_error", "", "API key authentication changed while waiting"
		case service.APIKeyQueueErrorPolicyChanged:
			return http.StatusServiceUnavailable, "api_error", "",
				"API key concurrency policy changed, please retry"
		default:
			return http.StatusServiceUnavailable, "api_error", "",
				"Service temporarily unavailable, please retry later"
		}
	}

	var waitQueueFullErr *WaitQueueFullError
	if errors.As(err, &waitQueueFullErr) {
		return http.StatusTooManyRequests, "rate_limit_error", gatewayQueueFullCode,
			"Too many pending requests, please retry later"
	}

	var concurrencyErr *ConcurrencyError
	if errors.As(err, &concurrencyErr) {
		if concurrencyErr.SlotType != "" {
			slotType = concurrencyErr.SlotType
		}
		return http.StatusTooManyRequests, "rate_limit_error", gatewayConcurrencyLimitCode,
			fmt.Sprintf("Concurrency limit exceeded for %s, please retry later", slotType)
	}

	if errors.Is(err, context.Canceled) {
		return statusClientClosedRequest, "api_error", "", "context canceled"
	}

	return http.StatusServiceUnavailable, "api_error", "", "Service temporarily unavailable, please retry later"
}
