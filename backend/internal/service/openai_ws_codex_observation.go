package service

import (
	"context"
	"net/http"
)

// Successful native WS headers describe the handshake, not subsequent turns.
// Capture their relative reset time at dial completion, before a connection is
// prewarmed or leased. Reusing those headers later would invent new boundaries.
func (s *OpenAIGatewayService) captureOpenAIWSHandshakeQuota(ctx context.Context, account *Account, headers http.Header) {
	if s == nil || !account.UsesOpenAICodexProtocol() || (account.Platform != PlatformOpenAI && account.Platform != "") || account.IsShadow() {
		return
	}
	if snapshot := ParseCodexRateLimitHeaders(headers); snapshot != nil {
		s.updateCodexUsageSnapshot(ctx, account.ID, snapshot)
	}
}
