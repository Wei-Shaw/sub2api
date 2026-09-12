package service

import (
	"context"
	"net/http"
)

type capturedCodexObservationKey struct{}

// Response-local metadata must not be placed in upstream headers: those can be
// copied to clients or request logs. The snapshot remains anchored to header
// arrival even when error handling runs after a slow response body.
type capturedCodexObservation struct {
	accountID int64
	snapshot  *OpenAICodexUsageSnapshot
}

func codexObservationFromResponse(response *http.Response) *capturedCodexObservation {
	if response == nil || response.Request == nil {
		return nil
	}
	observation, _ := response.Request.Context().Value(capturedCodexObservationKey{}).(*capturedCodexObservation)
	return observation
}

func codexObservationResponseContext(ctx context.Context, response *http.Response) context.Context {
	if observation := codexObservationFromResponse(response); observation != nil {
		return context.WithValue(ctx, capturedCodexObservationKey{}, observation)
	}
	return ctx
}

func capturedCodexSnapshot(ctx context.Context, accountID int64) *OpenAICodexUsageSnapshot {
	if ctx != nil {
		if observation, ok := ctx.Value(capturedCodexObservationKey{}).(*capturedCodexObservation); ok && observation.accountID == accountID {
			return observation.snapshot
		}
	}
	return nil
}

func codexRateLimitSnapshot(ctx context.Context, accountID int64, headers http.Header) *OpenAICodexUsageSnapshot {
	if snapshot := capturedCodexSnapshot(ctx, accountID); snapshot != nil {
		return snapshot
	}
	return ParseCodexRateLimitHeaders(headers)
}

func (s *OpenAIGatewayService) handleOpenAIResponseUpstreamError(ctx context.Context, account *Account, response *http.Response, responseBody []byte, canonicalModel ...string) bool {
	return s.handleOpenAIAccountUpstreamError(codexObservationResponseContext(ctx, response), account, response.StatusCode, response.Header, responseBody, canonicalModel...)
}
