package service

import (
	"context"
)

// enforceOpenAICodexClientAdmissionBeforeUpstream is the final local safety net
// for forwarding paths. The handler already performed the fresh post-slot
// check; this layer reuses the same request-frozen policy and admitted account
// object without adding another Redis or database round trip.
func (s *OpenAIGatewayService) enforceOpenAICodexClientAdmissionBeforeUpstream(
	ctx context.Context,
	selected *Account,
) (*Account, error) {
	if s == nil || selected == nil || !codexClientAdmissionActive(ctx) {
		return selected, nil
	}

	if vetoed, result := s.codexClientAdmissionVeto(ctx, selected); vetoed {
		return selected, &CodexClientAdmissionError{Result: result}
	}
	return selected, nil
}
