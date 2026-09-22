package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
)

const statusClientClosedRequest = 499

func concurrencyWSCloseStatus(err error) coderws.StatusCode {
	if errors.Is(err, service.ErrAPIKeyConcurrencyExceeded) {
		return coderws.StatusTryAgainLater
	}
	return coderws.StatusInternalError
}

func concurrencyWSReason(err error) string {
	if errors.Is(err, service.ErrAPIKeyConcurrencyExceeded) {
		return "api_key_concurrency_limit: too many concurrent requests, please retry later"
	}
	return "failed to acquire user concurrency slot"
}

const (
	gatewayQueueFullCode        = "gateway_queue_full"
	gatewayConcurrencyLimitCode = "gateway_concurrency_limit"
)

func concurrencyErrorResponse(err error, slotType string) (int, string, string, string) {
	if errors.Is(err, service.ErrAPIKeyConcurrencyExceeded) {
		return http.StatusTooManyRequests, "rate_limit_error", "api_key_concurrency_limit",
			"Concurrency limit exceeded for API key, please retry later"
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
