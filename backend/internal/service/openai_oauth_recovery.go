package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// OpenAIOAuthRecoveryRepository keeps a late rejection/rotation from overwriting
// a concurrent refresh, reauthorization, or administrative state change.
type OpenAIOAuthRecoveryRepository interface {
	UpdateOpenAIOAuthCredentialsIfUnchanged(context.Context, int64, map[string]any, *int64, map[string]any) (bool, error)
	SetOpenAIOAuthRecoveryStateIfUnchanged(context.Context, *Account, string) (bool, error)
}

type openAI401RecoveryKey struct{}

const openAIReauthorizationRequired = "OpenAI OAuth reauthorization required"

func recoverableOpenAI401(body []byte) bool {
	switch strings.ToLower(extractUpstreamErrorCode(body)) {
	case "token_invalidated", "token_revoked", "account_deactivated", "account_disabled",
		"organization_deactivated", "insufficient_scope", "invalid_scope", "invalid_api_key":
		return false
	}
	message := strings.ToLower(extractUpstreamErrorMessage(body))
	return !strings.Contains(message, "missing scope") && !strings.Contains(message, "insufficient scope")
}

// Only the legacy account-test error is eligible for a probe while inactive.
// A successful refresh alone never clears it or enables scheduling.
func recoverableOpenAITestError(account *Account) bool {
	if account == nil || account.Status != StatusError {
		return false
	}
	body, ok := strings.CutPrefix(account.ErrorMessage, "Authentication failed (401): ")
	return ok && recoverableOpenAI401([]byte(body))
}

func openAIRecoveryAccountEligible(account *Account, test bool) bool {
	return account != nil && account.IsOpenAIOAuth() && !account.IsCredentialShadow() &&
		!account.IsOpenAIPersonalAccessToken() && !account.IsOpenAIAgentIdentity() &&
		strings.TrimSpace(account.GetOpenAIRefreshToken()) != "" &&
		(account.IsActive() || (test && recoverableOpenAITestError(account)))
}

func sameOpenAIRecoveryAccount(account, expected *Account, test bool) bool {
	if !openAIRecoveryAccountEligible(account, test) || account.ID != expected.ID ||
		account.Status != expected.Status || account.ErrorMessage != expected.ErrorMessage ||
		account.Schedulable != expected.Schedulable {
		return false
	}
	if account.ProxyID == nil || expected.ProxyID == nil {
		return account.ProxyID == nil && expected.ProxyID == nil
	}
	return *account.ProxyID == *expected.ProxyID
}

type openAIRejectedTokenExecutor struct {
	OAuthRefreshExecutor
	expected      *Account
	rejectedToken string
	test          bool
}

func (e openAIRejectedTokenExecutor) CanRefresh(account *Account) bool {
	return sameOpenAIRecoveryAccount(account, e.expected, e.test) && e.OAuthRefreshExecutor.CanRefresh(account)
}

func (e openAIRejectedTokenExecutor) NeedsRefresh(account *Account, _ time.Duration) bool {
	// Compare token identity and version under the shared locks. Do not rotate
	// again for a late 401 or consume a refresh token published by another writer.
	return account.GetOpenAIAccessToken() == e.rejectedToken &&
		account.GetCredentialAsInt64("_token_version") == e.expected.GetCredentialAsInt64("_token_version") &&
		account.GetOpenAIRefreshToken() == e.expected.GetOpenAIRefreshToken()
}

func (p *OpenAITokenProvider) refreshRejectedAccessToken(ctx context.Context, account *Account, rejectedToken string, test bool) (*Account, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p == nil || p.refreshAPI == nil || p.executor == nil || p.accountRepo == nil ||
		strings.TrimSpace(rejectedToken) == "" || !openAIRecoveryAccountEligible(account, test) {
		return nil, errors.New("OpenAI OAuth recovery is unavailable")
	}
	if _, ok := p.accountRepo.(OpenAIOAuthRecoveryRepository); !ok {
		return nil, errors.New("OpenAI OAuth recovery repository is unavailable")
	}
	// Stay within the shared distributed lock lease.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ctx = context.WithValue(withOAuthRefreshRequestPath(ctx), openAI401RecoveryKey{}, test)
	expected := snapshotOAuthRefreshAccount(account)
	executor := openAIRejectedTokenExecutor{p.executor, expected, rejectedToken, test}
	result, err := p.refreshAPI.RefreshIfNeeded(ctx, expected, executor, 0)
	if err != nil {
		if result != nil && result.Account != nil && isNonRetryableRefreshError(err) {
			p.setRecoveryState(ctx, result.Account, openAIReauthorizationRequired)
			return nil, errors.New(openAIReauthorizationRequired)
		}
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fresh := result.Account
	if result.LockHeld {
		// Read durable state, not a cache that may still contain the rejected
		// token. Lock contention is bounded and never triggers a second refresh.
		for attempt := 0; attempt < openAILockMaxAttempts; attempt++ {
			fresh, err = p.accountRepo.GetByID(ctx, expected.ID)
			if err != nil {
				return nil, err
			}
			if !sameOpenAIRecoveryAccount(fresh, expected, test) || fresh.GetOpenAIAccessToken() != rejectedToken {
				break
			}
			timer := time.NewTimer(openAILockMaxWait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	if !sameOpenAIRecoveryAccount(fresh, expected, test) ||
		strings.TrimSpace(fresh.GetOpenAIAccessToken()) == "" || fresh.GetOpenAIAccessToken() == rejectedToken {
		return nil, errors.New("OpenAI OAuth recovery did not publish a replacement token")
	}
	if p.tokenCache != nil {
		if err := p.tokenCache.DeleteAccessToken(ctx, OpenAITokenCacheKey(expected)); err != nil {
			return nil, errors.New("OpenAI OAuth recovery cache invalidation failed")
		}
	}
	return fresh, ctx.Err()
}

func (p *OpenAITokenProvider) setRecoveryState(ctx context.Context, account *Account, message string) {
	repo, ok := p.accountRepo.(OpenAIOAuthRecoveryRepository)
	if !ok {
		return
	}
	applied, err := repo.SetOpenAIOAuthRecoveryStateIfUnchanged(ctx, account, message)
	if err != nil {
		slog.Warn("openai_oauth_recovery_state_failed", "account_id", account.ID)
	}
	if applied && p.tokenCache != nil {
		_ = p.tokenCache.DeleteAccessToken(ctx, OpenAITokenCacheKey(account))
	}
}

func (s *OpenAIGatewayService) tryRefreshOpenAIHTTP401(ctx context.Context, account *Account, status int, body []byte, rejectedToken string) (string, bool) {
	if s == nil || s.openAITokenProvider == nil || account == nil ||
		status != http.StatusUnauthorized || !recoverableOpenAI401(body) {
		return "", false
	}
	owner := account
	if account.IsCredentialShadow() {
		var err error
		owner, err = resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil || owner == nil || !sameProxy(account, owner) {
			return "", false
		}
	}
	fresh, err := s.openAITokenProvider.refreshRejectedAccessToken(ctx, owner, rejectedToken, false)
	if err != nil {
		return "", false
	}
	return fresh.GetOpenAIAccessToken(), true
}

func sameProxy(a, b *Account) bool {
	if a.ProxyID == nil || b.ProxyID == nil {
		return a.ProxyID == nil && b.ProxyID == nil
	}
	return *a.ProxyID == *b.ProxyID
}
