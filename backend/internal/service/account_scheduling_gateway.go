package service

import "context"

// Selection capability is request-local: Live cannot use a short HTTP/turn
// lease, while the same governed account remains eligible for text requests.
type accountSchedulingSelectionCapabilityKey struct{}

func (s *OpenAIGatewayService) SetAccountSchedulingPools(pools *AccountSchedulingPoolService) {
	if s != nil {
		s.accountSchedulingPools = pools
	}
}

func (s *OpenAIGatewayService) AcquireSchedulingPoolLease(ctx context.Context, account *Account, actualProxyURL string) (*AccountSchedulingLease, error) {
	if s == nil || s.accountSchedulingPools == nil || account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		return nil, nil
	}
	return s.accountSchedulingPools.Acquire(ctx, account.ID, actualProxyURL)
}

type accountSchedulingLeaseProviderKey struct{}
type accountSchedulingLeaseProvider struct {
	accountID int64
	proxyURL  string
	current   func() *AccountSchedulingLease
}

// WithAccountSchedulingLeaseProvider lets the HTTP bridge reuse its current WS
// turn lease. The provider is evaluated for each send, never captured at turn 1.
func WithAccountSchedulingLeaseProvider(ctx context.Context, accountID int64, actualProxyURL string, current func() *AccountSchedulingLease) context.Context {
	return context.WithValue(ctx, accountSchedulingLeaseProviderKey{}, accountSchedulingLeaseProvider{accountID: accountID, proxyURL: actualProxyURL, current: current})
}

// AccountSchedulingProvidedLease only reuses an exact account/route match. An
// expired lease or changed proxy cannot silently send under the old budget.
func AccountSchedulingProvidedLease(ctx context.Context, accountID int64, actualProxyURL string) (*AccountSchedulingLease, bool, error) {
	p, ok := ctx.Value(accountSchedulingLeaseProviderKey{}).(accountSchedulingLeaseProvider)
	if !ok {
		return nil, false, nil
	}
	if p.accountID != accountID || p.proxyURL != actualProxyURL {
		return nil, true, ErrAccountSchedulingUnavailable
	}
	lease := p.current()
	if lease == nil {
		return nil, false, nil
	}
	if lease.Context().Err() != nil {
		return nil, true, ErrAccountSchedulingLeaseLost
	}
	return lease, true, nil
}

func (s *OpenAIGatewayService) filterAccountSchedulingPools(ctx context.Context, accounts []Account) ([]Account, error) {
	if s == nil || s.accountSchedulingPools == nil {
		return accounts, nil
	}
	return s.accountSchedulingPools.EligibleAccounts(ctx, accounts)
}
