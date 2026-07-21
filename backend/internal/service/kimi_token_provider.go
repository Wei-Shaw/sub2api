package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

const (
	kimiTokenCacheSkew             = 5 * time.Minute
	kimiRequestRefreshTimeout      = 8 * time.Second
	kimiTokenProviderLogComponent  = "kimi_token_provider"
	kimiTempUnschedulableErrorCode = "token_refresh_failed"
)

type KimiTokenCache = GeminiTokenCache

type KimiTokenProvider struct {
	accountRepo      AccountRepository
	tokenCache       KimiTokenCache
	refreshAPI       *OAuthRefreshAPI
	executor         OAuthRefreshExecutor
	refreshPolicy    ProviderRefreshPolicy
	tempUnschedCache TempUnschedCache
}

func NewKimiTokenProvider(
	accountRepo AccountRepository,
	tokenCache KimiTokenCache,
) *KimiTokenProvider {
	return &KimiTokenProvider{
		accountRepo:   accountRepo,
		tokenCache:    tokenCache,
		refreshPolicy: AntigravityProviderRefreshPolicy(),
	}
}

func (p *KimiTokenProvider) SetRefreshAPI(api *OAuthRefreshAPI, executor OAuthRefreshExecutor) {
	p.refreshAPI = api
	p.executor = executor
}

func (p *KimiTokenProvider) SetRefreshPolicy(policy ProviderRefreshPolicy) {
	p.refreshPolicy = policy
}

func (p *KimiTokenProvider) SetTempUnschedCache(cache TempUnschedCache) {
	p.tempUnschedCache = cache
}

func (p *KimiTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}
	if account.Platform != PlatformKimi || account.Type != AccountTypeOAuth {
		return "", errors.New("not a kimi oauth account")
	}

	cacheKey := KimiTokenCacheKey(account)
	if p.tokenCache != nil {
		if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
			return token, nil
		}
	}

	expiresAt := account.GetCredentialAsTime("expires_at")
	needsRefresh := expiresAt == nil || time.Until(*expiresAt) <= kimiTokenRefreshSkew
	if needsRefresh && strings.TrimSpace(account.GetKimiRefreshToken()) == "" {
		if expiresAt == nil || !time.Now().Before(*expiresAt) {
			return "", errors.New("kimi access_token expired and refresh_token is missing")
		}
		needsRefresh = false
	}
	if needsRefresh && p.refreshAPI != nil && p.executor != nil {
		refreshCtx, cancel := context.WithTimeout(ctx, kimiRequestRefreshTimeout)
		defer cancel()
		result, err := p.refreshAPI.RefreshIfNeeded(refreshCtx, account, p.executor, kimiTokenRefreshSkew)
		if err != nil {
			p.markTempUnschedulable(account, err)
			if p.refreshPolicy.OnRefreshError == ProviderRefreshErrorReturn {
				return "", err
			}
		} else if !result.LockHeld && result.Account != nil {
			account = result.Account
			expiresAt = account.GetCredentialAsTime("expires_at")
		}
	}

	accessToken := account.GetKimiAccessToken()
	if strings.TrimSpace(accessToken) == "" {
		return "", errors.New("access_token not found in credentials")
	}

	if p.tokenCache != nil {
		latestAccount, isStale := CheckTokenVersion(ctx, account, p.accountRepo)
		if isStale && latestAccount != nil {
			accessToken = latestAccount.GetKimiAccessToken()
			if strings.TrimSpace(accessToken) == "" {
				return "", errors.New("access_token not found after version check")
			}
		} else {
			ttl := 30 * time.Minute
			if expiresAt != nil {
				until := time.Until(*expiresAt)
				switch {
				case until > kimiTokenCacheSkew:
					ttl = until - kimiTokenCacheSkew
				case until > 0:
					ttl = until
				default:
					ttl = time.Minute
				}
			}
			_ = p.tokenCache.SetAccessToken(ctx, cacheKey, accessToken, ttl)
		}
	}

	return accessToken, nil
}

func (p *KimiTokenProvider) markTempUnschedulable(account *Account, refreshErr error) {
	if p == nil || p.accountRepo == nil || account == nil {
		return
	}
	now := time.Now()
	until := now.Add(tokenRefreshTempUnschedDuration)
	redactedErr := "unknown error"
	if refreshErr != nil {
		redactedErr = logredact.RedactText(refreshErr.Error())
	}
	if isNonRetryableRefreshError(refreshErr) {
		if err := p.accountRepo.SetError(context.Background(), account.ID, "kimi token refresh failed (non-retryable): "+redactedErr); err != nil {
			slog.Warn(kimiTokenProviderLogComponent+".set_error_status_failed", "account_id", account.ID, "error", err)
		}
		return
	}
	reason := "kimi token refresh failed on request path: " + redactedErr
	bgCtx := context.Background()
	if err := p.accountRepo.SetTempUnschedulable(bgCtx, account.ID, until, reason); err != nil {
		slog.Warn(kimiTokenProviderLogComponent+".set_temp_unschedulable_failed", "account_id", account.ID, "error", err)
		return
	}
	if p.tempUnschedCache != nil {
		state := &TempUnschedState{
			UntilUnix:       until.Unix(),
			TriggeredAtUnix: now.Unix(),
			ErrorMessage:    kimiTempUnschedulableErrorCode + ": " + reason,
		}
		if err := p.tempUnschedCache.SetTempUnsched(bgCtx, account.ID, state); err != nil {
			slog.Warn(kimiTokenProviderLogComponent+".temp_unsched_cache_set_failed", "account_id", account.ID, "error", err)
		}
	}
}

func KimiTokenCacheKey(account *Account) string {
	if account == nil {
		return "kimi:account:0"
	}
	return "kimi:account:" + strconv.FormatInt(account.ID, 10)
}
