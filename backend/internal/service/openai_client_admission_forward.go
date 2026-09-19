package service

import (
	"context"
	"fmt"
)

// enforceOpenAICodexClientAdmissionBeforeUpstream is the final local safety net
// for forwarding paths. The handler already performed the fresh post-slot
// check; this layer reuses the same request-frozen policy and admitted account
// object without adding another Redis or database round trip.
func (s *OpenAIGatewayService) enforceOpenAICodexClientAdmissionBeforeUpstream(
	ctx context.Context,
	selected *Account,
) (*Account, error) {
	if s == nil || selected == nil || !codexClientAdmissionAppliesToAccount(ctx, selected) {
		return selected, nil
	}

	snapshot, ok := codexClientAdmissionSnapshotFromContext(ctx)
	if !ok || !snapshot.terminalAdmissionRecorded(selected) {
		if ok {
			snapshot.recordUnavailable()
		}
		return selected, fmt.Errorf("%w: missing terminal admission grant for account %d", ErrCodexClientAdmissionUnavailable, selected.ID)
	}
	return selected, nil
}
