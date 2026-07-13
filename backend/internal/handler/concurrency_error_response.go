package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const statusClientClosedRequest = 499

func isGatewayAdmissionConcurrencyError(err error) bool {
	var extraUnavailable *service.ExtraConcurrencyUnavailableError
	var timeout *service.GatewayAdmissionTimeoutError
	var queueFull *service.GatewayAdmissionQueueFullError
	return errors.As(err, &extraUnavailable) || errors.As(err, &timeout) || errors.As(err, &queueFull)
}

func concurrencyErrorResponse(err error, slotType string) (int, string, string) {
	var extraUnavailableErr *service.ExtraConcurrencyUnavailableError
	if errors.As(err, &extraUnavailableErr) {
		return http.StatusTooManyRequests, "EXTRA_CONCURRENCY_UNAVAILABLE",
			"Extra concurrency is unavailable, please retry later"
	}

	var admissionQueueFullErr *service.GatewayAdmissionQueueFullError
	if errors.As(err, &admissionQueueFullErr) {
		return http.StatusTooManyRequests, "rate_limit_error",
			"Too many pending requests, please retry later"
	}

	var admissionTimeoutErr *service.GatewayAdmissionTimeoutError
	if errors.As(err, &admissionTimeoutErr) {
		if admissionTimeoutErr.SlotType != "" {
			slotType = admissionTimeoutErr.SlotType
		}
		return http.StatusTooManyRequests, "rate_limit_error",
			fmt.Sprintf("Concurrency limit exceeded for %s, please retry later", slotType)
	}

	var waitQueueFullErr *WaitQueueFullError
	if errors.As(err, &waitQueueFullErr) {
		return http.StatusTooManyRequests, "rate_limit_error",
			"Too many pending requests, please retry later"
	}

	var concurrencyErr *ConcurrencyError
	if errors.As(err, &concurrencyErr) {
		if concurrencyErr.SlotType != "" {
			slotType = concurrencyErr.SlotType
		}
		return http.StatusTooManyRequests, "rate_limit_error",
			fmt.Sprintf("Concurrency limit exceeded for %s, please retry later", slotType)
	}

	if errors.Is(err, context.Canceled) {
		return statusClientClosedRequest, "api_error", "context canceled"
	}

	return http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable, please retry later"
}
