package service

import (
	"context"
	"strings"
)

// PeekStickyOpenAIAccount best-effort resolves the sticky OpenAI account for the
// current session hash. It is intended for diagnostics on selection-failure
// paths where no account was successfully selected in-band.
func (s *OpenAIGatewayService) PeekStickyOpenAIAccount(ctx context.Context, groupID *int64, sessionHash string) (*Account, error) {
	if s == nil {
		return nil, nil
	}

	sessionHash = strings.TrimSpace(sessionHash)
	if sessionHash == "" || s.cache == nil {
		return nil, nil
	}

	accountID, err := s.getStickySessionAccountID(ctx, groupID, sessionHash)
	if err != nil || accountID <= 0 {
		return nil, err
	}

	account, err := s.getSchedulableAccount(ctx, accountID)
	if err != nil || account == nil {
		return nil, err
	}
	if !account.IsOpenAI() {
		return nil, nil
	}
	return account, nil
}
