package service

import (
	"context"
	"net/http"
	"time"
)

type codexQuotaOverdraftAccountTestCoordinator interface {
	ObserveAccount(account *Account, preferredModel string)
	ObserveBusinessSuccess(account *Account, preferredModel string)
	HandleQuota429(ctx context.Context, account *Account, headers http.Header, body []byte, preferredModel string) bool
}

func (s *AccountTestService) SetCodexQuotaOverdraftCoordinator(coordinator *CodexQuotaOverdraftCoordinator) {
	if s != nil {
		s.codexQuotaOverdraft = coordinator
	}
}

func (s *AccountTestService) prepareCodexQuotaOverdraftTestRequest(
	ctx context.Context,
	account *Account,
	payload []byte,
) (context.Context, []byte, bool) {
	if !s.codexQuotaOverdraftTestEnabled(account) || !codexQuotaOverdraftInjectionEligible(account, time.Now().UTC()) {
		return ctx, payload, false
	}
	ctx = WithCodexQuotaOverdraftScheduling(ctx)
	updated, changed, err := injectCodexQuotaOverdraft(payload)
	if err != nil || !changed {
		return ctx, payload, false
	}
	markCodexQuotaOverdraftInjected(ctx, account.ID)
	return ctx, updated, true
}

func (s *AccountTestService) handleCodexQuotaOverdraftTest429(
	ctx context.Context,
	account *Account,
	headers http.Header,
	body []byte,
	preferredModel string,
) bool {
	return s.codexQuotaOverdraft != nil && s.codexQuotaOverdraftTestEnabled(account) &&
		s.codexQuotaOverdraft.HandleQuota429(ctx, account, headers, body, preferredModel)
}

func (s *AccountTestService) observeCodexQuotaOverdraftTestResult(
	account *Account,
	preferredModel string,
	injected bool,
) {
	if s.codexQuotaOverdraft == nil || !s.codexQuotaOverdraftTestEnabled(account) {
		return
	}
	if injected {
		s.codexQuotaOverdraft.ObserveBusinessSuccess(account, preferredModel)
		return
	}
	s.codexQuotaOverdraft.ObserveAccount(account, preferredModel)
}

func (s *AccountTestService) codexQuotaOverdraftTestEnabled(account *Account) bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.CodexQuotaOverdraftEnabled &&
		isCodexQuotaOverdraftAccount(account)
}
