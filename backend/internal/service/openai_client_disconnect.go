package service

import (
	"context"
	"errors"
	"net/http"
)

// Cancellation forbids replay, but must not erase an already confirmed upstream
// failure. A transport cancellation alone is not evidence of account failure.
func (s *OpenAIGatewayService) ReportOpenAIFailureAfterDisconnect(account *Account, model string, err error) {
	if shouldReportOpenAIFailureAfterDisconnect(err) {
		s.ReportOpenAIAccountScheduleResult(account, model, false, nil, err)
	}
}

func shouldReportOpenAIFailureAfterDisconnect(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var failover *UpstreamFailoverError
	if errors.As(err, &failover) {
		if failover.Scope == GatewayFailureScopeProvider {
			return false
		}
		if !failover.IsCredentialFailure() && isOpenAICapacityHealthPayload(failover.ResponseBody) {
			return true
		}
		return failover.Scope != GatewayFailureScopeRequest && !failover.RequestScopedTransient &&
			failover.ShouldReportAccountScheduleFailure() &&
			(failover.StatusCode == http.StatusTooManyRequests || failover.StatusCode >= http.StatusInternalServerError)
	}
	var terminal *OpenAIStreamTerminalError
	return errors.As(err, &terminal) && terminal.StatusCode >= http.StatusInternalServerError
}
