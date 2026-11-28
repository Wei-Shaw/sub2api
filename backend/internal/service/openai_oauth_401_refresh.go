package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"github.com/tidwall/gjson"
)

var errOpenAIAccessTokenNotReplaced = errors.New("rejected OpenAI access token was not replaced")

// oauth401SameProxy reports whether both sides name the same proxy; nil and set differ.
func oauth401SameProxy(left, right *int64) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

// retryOpenAIOAuth401 replaces a rejected OpenAI OAuth access token and resends the
// request once. It returns resp untouched whenever a refresh cannot help, so the
// existing 401 handling stays in charge.
func (s *OpenAIGatewayService) retryOpenAIOAuth401(req *http.Request, proxyURL string, account *Account, resp *http.Response) (*http.Response, error) {
	if s.openAITokenProvider == nil || s.accountRepo == nil || account == nil || !account.IsOpenAIOAuth() {
		return resp, nil
	}
	scheme, rejected, found := strings.Cut(strings.TrimSpace(req.Header.Get("Authorization")), " ")
	rejected = strings.TrimSpace(rejected)
	if !found || !strings.EqualFold(scheme, "Bearer") || rejected == "" {
		return resp, nil
	}
	if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		return resp, nil
	}
	ctx := req.Context()
	body, upstreamMsg, _ := s.readOpenAIUpstreamErrorWithCompleteness(resp)
	// token_invalidated, token_revoked and detail Unauthorized are permanent in
	// ratelimit_service.go, account and workspace deactivation codes are handled by
	// handleOpenAIAccountUpstreamError, and a refresh keeps the grant's scopes, so these
	// bodies keep the existing handling.
	code := extractUpstreamErrorCode(body)
	if code == "token_invalidated" || code == "token_revoked" || code == "insufficient_scope" ||
		gjson.GetBytes(body, "detail").String() == "Unauthorized" ||
		strings.Contains(strings.ToLower(upstreamMsg), "missing scopes:") ||
		isOpenAIHTTPUpstreamAccessStateError(resp.StatusCode, "", body) {
		return resp, nil
	}

	// resolveCredentialAccount already requires the credential owner to be OpenAI OAuth;
	// a shadow account itself is never a PAT or agent identity, its parent can be.
	credAccount, err := resolveCredentialAccount(ctx, s.accountRepo, account)
	if err != nil || credAccount == nil ||
		credAccount.IsOpenAIPersonalAccessToken() || credAccount.IsOpenAIAgentIdentity() ||
		strings.TrimSpace(credAccount.GetOpenAIRefreshToken()) == "" {
		return resp, nil
	}
	var otherLastUsed *time.Time
	if account.ID != credAccount.ID {
		if own, readErr := s.accountRepo.GetByID(ctx, account.ID); readErr == nil && own != nil {
			otherLastUsed = own.LastUsedAt
		}
	}

	token, err := s.openAITokenProvider.replaceRejectedAccessToken(ctx, credAccount, account.ProxyID, rejected, otherLastUsed)
	if err != nil {
		// The shared attempt already classified, logged and reported the failure, or the
		// client went away, or the token was not replaced: the existing 401 handling stays
		// in charge.
		return resp, nil
	}

	resend := req.Clone(ctx)
	if req.GetBody != nil {
		replay, replayErr := req.GetBody()
		if replayErr != nil {
			slog.Warn("openai_oauth_401_resend_body_failed", "account_id", credAccount.ID, "error", logredact.RedactText(replayErr.Error()))
			return resp, nil
		}
		resend.Body = replay
	}
	resend.Header.Set("Authorization", "Bearer "+token)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	slog.Info("openai_oauth_401_token_replaced", "account_id", credAccount.ID)
	return s.roundTripOpenAIUpstream(resend, proxyURL, account)
}

// openAI401RefreshExecutor narrows OAuthRefreshAPI's decision to the rejected token.
// RefreshIfNeeded re-reads the account under its locks before NeedsRefresh runs.
type openAI401RefreshExecutor struct {
	OAuthRefreshExecutor
	rejected      string
	proxyID       *int64
	otherLastUsed *time.Time
	minted        bool // this provider minted the rejected token on an earlier 401
}

// NeedsRefresh reports whether the rejected token is still stored through the same proxy
// and can be stale.
func (e *openAI401RefreshExecutor) NeedsRefresh(account *Account, _ time.Duration) bool {
	if strings.TrimSpace(account.GetOpenAIAccessToken()) != e.rejected ||
		!oauth401SameProxy(account.ProxyID, e.proxyID) {
		return false
	}
	mintedAt := account.GetCredentialAsInt64("_token_version")
	if mintedAt == 0 {
		return true
	}
	minted := time.UnixMilli(mintedAt)
	lastSuccess := account.LastUsedAt
	if e.otherLastUsed != nil && (lastSuccess == nil || e.otherLastUsed.After(*lastSuccess)) {
		lastSuccess = e.otherLastUsed
	}
	if lastSuccess != nil && !lastSuccess.Before(minted) {
		return true
	}
	// No request has succeeded with this token. If the account was cooled down after it was
	// minted, or this provider minted it on a 401, the token itself was rejected: another
	// refresh would only rotate the refresh_token again.
	return !e.minted && (account.TempUnschedulableUntil == nil || !account.TempUnschedulableUntil.After(minted))
}

// replaceRejectedAccessToken returns a replacement for the rejected token. Concurrent
// requests that hold the same token for the same account and proxy share one attempt, and
// a caller that goes away stops waiting without cutting it.
func (p *OpenAITokenProvider) replaceRejectedAccessToken(ctx context.Context, account *Account, proxyID *int64, rejected string, otherLastUsed *time.Time) (string, error) {
	if p == nil || p.refreshAPI == nil || p.executor == nil || p.accountRepo == nil ||
		account == nil || !p.executor.CanRefresh(account) {
		return "", errOpenAIAccessTokenNotReplaced
	}
	proxyKey := "none"
	if proxyID != nil {
		proxyKey = fmt.Sprint(*proxyID)
	}
	attempt := p.refreshFlights.DoChan(fmt.Sprintf("%d|%s|%s", account.ID, proxyKey, rejected), func() (result any, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("openai_oauth_401_refresh_panicked", "account_id", account.ID, "panic", fmt.Sprint(recovered))
				result, err = nil, fmt.Errorf("openai oauth 401 refresh panicked: %v", recovered)
			}
		}()
		// WithoutCancel: the refresh rotates the refresh_token at the provider, so it must
		// complete and be persisted even if the client goes away.
		return p.refreshRejectedAccessToken(withOAuthRefreshLockRequired(withOAuthRefreshRequestPath(context.WithoutCancel(ctx))), account, proxyID, rejected, otherLastUsed)
	})
	select {
	case result := <-attempt:
		if result.Err != nil {
			return "", result.Err
		}
		token, _ := result.Val.(string)
		return token, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// storedReplacement returns the token another worker persisted for the account when it is
// usable here: the account is still active, it goes through the same proxy, and it is not
// the rejected token.
func (p *OpenAITokenProvider) storedReplacement(ctx context.Context, accountID int64, proxyID *int64, rejected string) string {
	stored, err := p.accountRepo.GetByID(ctx, accountID)
	if err != nil || stored == nil || !stored.IsActive() || !oauth401SameProxy(stored.ProxyID, proxyID) {
		return ""
	}
	if token := strings.TrimSpace(stored.GetOpenAIAccessToken()); token != "" && token != rejected {
		return token
	}
	return ""
}

// refreshFailed classifies a failed attempt once, for every caller that shares it. A
// terminal error is written to the account; a token an administrator refresh stored in the
// meantime is returned for the resend instead.
func (p *OpenAITokenProvider) refreshFailed(ctx context.Context, account *Account, proxyID *int64, rejected string, err error) (string, error) {
	terminal := isNonRetryableRefreshError(err) && !isSharedProviderRefreshError(err)
	fields := []any{"account_id", account.ID, "error", logredact.RedactText(err.Error())}
	if terminal {
		if token := p.storedReplacement(ctx, account.ID, proxyID, rejected); token != "" {
			return token, nil
		}
		if p.runtimeBlocker != nil {
			p.runtimeBlocker.BlockAccountScheduling(account, time.Time{}, "token_refresh_non_retryable")
		}
		message := nonRetryableRefreshErrorPrefix + logredact.RedactText(err.Error())
		if setErr := p.accountRepo.SetError(context.WithoutCancel(ctx), account.ID, message); setErr != nil {
			fields = append(fields, "set_error_failed", setErr)
		} else if p.tokenCache != nil {
			if delErr := p.tokenCache.DeleteAccessToken(context.WithoutCancel(ctx), OpenAITokenCacheKey(account)); delErr != nil {
				slog.Warn("openai_oauth_401_cache_delete_failed", "account_id", account.ID, "error", delErr)
			}
		}
	}
	slog.Warn("openai_oauth_401_refresh_failed", fields...)
	return "", err
}

// refreshRejectedAccessToken refreshes the rejected token and returns the token to resend
// with, or takes the token the worker holding the refresh lock persists instead.
func (p *OpenAITokenProvider) refreshRejectedAccessToken(ctx context.Context, account *Account, proxyID *int64, rejected string, otherLastUsed *time.Time) (string, error) {
	executor := &openAI401RefreshExecutor{
		OAuthRefreshExecutor: p.executor,
		rejected:             rejected,
		proxyID:              proxyID,
		otherLastUsed:        otherLastUsed,
	}
	if minted, ok := p.mintedAccessTokens.Load(account.ID); ok && minted == rejected {
		executor.minted = true
	}
	result, err := p.refreshAPI.RefreshIfNeeded(ctx, account, executor, 0)
	if err != nil {
		return p.refreshFailed(ctx, account, proxyID, rejected, err)
	}
	if result != nil && result.Account != nil {
		if !oauth401SameProxy(result.Account.ProxyID, proxyID) {
			return "", errOpenAIAccessTokenNotReplaced
		}
		if token := strings.TrimSpace(result.Account.GetOpenAIAccessToken()); token != "" && token != rejected {
			if result.Refreshed {
				p.mintedAccessTokens.Store(account.ID, token)
			}
			return token, nil
		}
	}
	if result == nil || !result.LockHeld {
		return "", errOpenAIAccessTokenNotReplaced
	}
	// Another worker holds the refresh lock: read the token it persists at most
	// openAILockMaxAttempts times, backing off from openAILockInitialWait to
	// openAILockMaxWait like waitForTokenAfterLockRace, instead of refreshing here.
	wait := openAILockInitialWait
	for i := 0; i < openAILockMaxAttempts; i++ {
		if token := p.storedReplacement(ctx, account.ID, proxyID, rejected); token != "" {
			return token, nil
		}
		if i < openAILockMaxAttempts-1 {
			time.Sleep(jitterLockWait(wait))
			wait = min(wait*2, openAILockMaxWait)
		}
	}
	return "", errOpenAIAccessTokenNotReplaced
}
