package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// Retry only an HTTP authentication rejection before any response is published.
// API keys, setup/PAT/agent-identity credentials and permanent/scope failures keep
// their existing error handling. WebSocket recovery is deliberately separate.
func (s *OpenAIGatewayService) tryRefreshOpenAIHTTP401(ctx context.Context, account *Account, status int, body []byte, rejectedToken string) (string, bool) {
	if s == nil || s.openAITokenProvider == nil || status != http.StatusUnauthorized || account == nil {
		return "", false
	}
	switch extractUpstreamErrorCode(body) {
	case "token_invalidated", "token_revoked", "insufficient_scope":
		return "", false
	}
	if gjson.GetBytes(body, "detail").String() == "Unauthorized" ||
		strings.Contains(strings.ToLower(extractUpstreamErrorMessage(body)), "missing scopes:") {
		return "", false
	}
	if account.IsShadow() {
		if s.accountRepo == nil {
			return "", false
		}
		owner, err := resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil || owner == nil {
			return "", false
		}
		// A shadow inherits its owner's proxy. Keep the transport used by
		// the rejected request if the owner changed after selection.
		if (account.ProxyID == nil) != (owner.ProxyID == nil) ||
			(account.ProxyID != nil && *account.ProxyID != *owner.ProxyID) {
			return "", false
		}
		account = owner
	}
	token, err := s.openAITokenProvider.refreshRejectedAccessToken(ctx, account, rejectedToken)
	if err != nil {
		// Neither provider error bodies nor credentials belong in the recovery log.
		slog.Debug("openai_http_401_recovery_unavailable", "account_id", account.ID)
		return "", false
	}
	return token, true
}

type openAIRejectedTokenExecutor struct {
	OAuthRefreshExecutor
	expected      *Account
	rejectedToken string
}

func (e openAIRejectedTokenExecutor) CanRefresh(account *Account) bool {
	return sameOpenAI401RecoveryAccount(account, e.expected) && e.OAuthRefreshExecutor.CanRefresh(account)
}

func (e openAIRejectedTokenExecutor) NeedsRefresh(account *Account, _ time.Duration) bool {
	// Under the existing refresh locks, use the durable token identity rather
	// than expiry. A late 401 must reuse a winner's token, not rotate it again.
	return strings.TrimSpace(account.GetOpenAIAccessToken()) == e.rejectedToken
}

func sameOpenAI401RecoveryAccount(account, expected *Account) bool {
	if account == nil || expected == nil || account.ID != expected.ID ||
		!account.IsActive() || !account.IsOpenAIOAuth() || account.IsOpenAIPersonalAccessToken() ||
		account.IsOpenAIAgentIdentity() || strings.TrimSpace(account.GetOpenAIRefreshToken()) == "" {
		return false
	}
	if account.ProxyID == nil || expected.ProxyID == nil {
		return account.ProxyID == nil && expected.ProxyID == nil
	}
	return *account.ProxyID == *expected.ProxyID
}

func (p *OpenAITokenProvider) refreshRejectedAccessToken(ctx context.Context, account *Account, rejectedToken string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	rejectedToken = strings.TrimSpace(rejectedToken)
	if p == nil || p.refreshAPI == nil || p.executor == nil || p.accountRepo == nil ||
		rejectedToken == "" || !sameOpenAI401RecoveryAccount(account, account) {
		return "", errors.New("OpenAI OAuth 401 recovery is unavailable")
	}
	expected := snapshotOAuthRefreshAccount(account)
	executor := openAIRejectedTokenExecutor{OAuthRefreshExecutor: p.executor, expected: expected, rejectedToken: rejectedToken}
	p.ensureMetrics()
	p.metrics.refreshRequests.Add(1)
	p.metrics.touchNow()
	result, err := p.refreshAPI.RefreshIfNeeded(withOAuthRefreshRequestPath(ctx), account, executor, 0)
	if err != nil {
		p.metrics.refreshFailure.Add(1)
		return "", err
	}
	// Reusing an already rotated token can bypass the executor's cancellation
	// check. The caller must not retry a request that has ended.
	if err := ctx.Err(); err != nil {
		return "", err
	}
	fresh := result.Account
	if result.LockHeld {
		p.metrics.lockContention.Add(1)
		// Never accept the rejected token still in cache as the lock winner.
		fresh, err = p.waitForOpenAI401Rotation(ctx, expected, rejectedToken)
		if err != nil {
			return "", err
		}
	} else if result.Refreshed {
		p.metrics.refreshSuccess.Add(1)
	}
	if !sameOpenAI401RecoveryAccount(fresh, expected) {
		return "", errors.New("OpenAI OAuth account changed during 401 recovery")
	}
	token := strings.TrimSpace(fresh.GetOpenAIAccessToken())
	if token == "" || token == rejectedToken {
		return "", errors.New("OpenAI OAuth refresh did not replace the rejected token")
	}
	if p.tokenCache != nil {
		if err := p.tokenCache.DeleteAccessToken(ctx, OpenAITokenCacheKey(expected)); err != nil {
			return "", errors.New("OpenAI OAuth cache invalidation failed after 401 recovery")
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return token, nil
}

func (p *OpenAITokenProvider) waitForOpenAI401Rotation(ctx context.Context, expected *Account, rejectedToken string) (*Account, error) {
	wait := openAILockInitialWait
	for i := 0; i < openAILockMaxAttempts; i++ {
		fresh, err := p.accountRepo.GetByID(ctx, expected.ID)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !sameOpenAI401RecoveryAccount(fresh, expected) {
			return nil, errors.New("OpenAI OAuth account changed while waiting for refresh")
		}
		if token := strings.TrimSpace(fresh.GetOpenAIAccessToken()); token != "" && token != rejectedToken {
			return fresh, nil
		}
		if i == openAILockMaxAttempts-1 {
			break
		}
		timer := time.NewTimer(jitterLockWait(wait))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
		wait *= 2
		if wait > openAILockMaxWait {
			wait = openAILockMaxWait
		}
	}
	return nil, errors.New("OpenAI OAuth refresh lock winner has not replaced the rejected token")
}
