package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	grokTokenCacheSkew          = 5 * time.Minute
	grokRequestRefreshTimeout   = 8 * time.Second
	grokRefreshLockWaitTimeout  = 2 * time.Second
	grokRefreshLockPollInterval = 25 * time.Millisecond
)

var (
	errGrokOAuthRefreshNotConfigured = errors.New("grok oauth refresh is not configured")
	errGrokOAuthRefreshTokenMissing  = errors.New("grok oauth refresh token is missing")
	errGrokOAuthAccessTokenMissing   = errors.New("grok oauth access token is missing")
	errGrokOAuthAccessTokenExpired   = errors.New("grok oauth access token is expired")
	errGrokOAuthConfiguredProxyMiss  = errors.New("grok oauth configured proxy is missing")
	errGrokOAuthRefreshLockTimeout   = errors.New("grok oauth refresh lock wait timed out")
)

type GrokTokenCache = GeminiTokenCache

type GrokTokenProvider struct {
	accountRepo      AccountRepository
	tokenCache       GrokTokenCache
	refreshAPI       *OAuthRefreshAPI
	executor         OAuthRefreshExecutor
	refreshPolicy    ProviderRefreshPolicy
	tempUnschedCache TempUnschedCache
REDACTED

func NewGrokTokenProvider(
	accountRepo AccountRepository,
	tokenCache GrokTokenCache,
) *GrokTokenProvider {
	return &GrokTokenProvider{
		accountRepo:   accountRepo,
		tokenCache:    tokenCache,
		refreshPolicy: GrokProviderRefreshPolicy(),
REDACTED
REDACTED

func (p *GrokTokenProvider) SetRefreshAPI(api *OAuthRefreshAPI, executor OAuthRefreshExecutor) {
	p.refreshAPI = api
	p.executor = executor
REDACTED

func (p *GrokTokenProvider) SetRefreshPolicy(policy ProviderRefreshPolicy) {
	p.refreshPolicy = policy
REDACTED

func (p *GrokTokenProvider) SetTempUnschedCache(cache TempUnschedCache) {
	p.tempUnschedCache = cache
REDACTED

func (p *GrokTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
REDACTED
	if account.Platform != PlatformGrok || account.Type != AccountTypeOAuth {
		return "", errors.New("not a grok oauth account")
REDACTED
	selectedProxyID := cloneGrokProxyID(account.ProxyID)
	if eligibilityErr := grokOAuthRequestAccountEligibilityError(account); eligibilityErr != nil {
		return "", withGrokCredentialFailureSnapshot(eligibilityErr, account)
REDACTED

	expiresAt := account.GetCredentialAsTime("expires_at")
	accountAccessToken := strings.TrimSpace(account.GetGrokAccessToken())
	if accountAccessToken == "" {
		return "", withGrokCredentialFailureSnapshot(errGrokOAuthAccessTokenMissing, account)
REDACTED
	if strings.TrimSpace(account.GetGrokRefreshToken()) == "" {
		return "", withGrokCredentialFailureSnapshot(errGrokOAuthRefreshTokenMissing, account)
REDACTED
	cacheKey := GrokTokenCacheKey(account)
	if p.tokenCache != nil {
		if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil {
			cachedToken := strings.TrimSpace(token)
			if cachedToken != "" && accountAccessToken != "" && cachedToken == accountAccessToken &&
				expiresAt != nil && time.Until(*expiresAt) > grokTokenRefreshSkew {
				return cachedToken, nil
		REDACTED
	REDACTED
REDACTED

	needsRefresh := expiresAt == nil || time.Until(*expiresAt) <= grokTokenRefreshSkew
	if needsRefresh {
		if p.refreshAPI == nil || p.executor == nil {
			return "", errGrokOAuthRefreshNotConfigured
	REDACTED
		refreshCtx, cancel := context.WithTimeout(ctx, grokRequestRefreshTimeout)
		defer cancel()
		result, err := p.refreshAPI.RefreshIfNeeded(withOAuthRefreshRequestPath(refreshCtx), account, p.executor, grokTokenRefreshSkew)
		if err != nil {
			if p.refreshPolicy.OnRefreshError == ProviderRefreshErrorReturn {
				return "", err
		REDACTED
	REDACTED else if result != nil && result.LockHeld {
			if p.refreshPolicy.OnLockHeld == ProviderLockHeldWaitForCache {
				token, waitErr := p.waitForRefreshedToken(refreshCtx, account, cacheKey)
				return token, withGrokCredentialFailureSnapshot(waitErr, account)
		REDACTED
			if expiresAt == nil || !time.Now().Before(*expiresAt) {
				return "", withGrokCredentialFailureSnapshot(errGrokOAuthAccessTokenExpired, account)
		REDACTED
	REDACTED else if result != nil && result.Account != nil {
			if eligibilityErr := grokOAuthRequestAccountEligibilityError(result.Account); eligibilityErr != nil {
				return "", withGrokCredentialFailureSnapshot(eligibilityErr, result.Account)
		REDACTED
			if !grokCredentialProxyIDsEqual(result.Account.ProxyID, selectedProxyID) {
				return "", withGrokCredentialFailureSnapshot(errOAuthRefreshAccountStateChanged, result.Account)
		REDACTED
			account = result.Account
			expiresAt = account.GetCredentialAsTime("expires_at")
	REDACTED
REDACTED

	accessToken := account.GetGrokAccessToken()
	if strings.TrimSpace(accessToken) == "" {
		return "", withGrokCredentialFailureSnapshot(errGrokOAuthAccessTokenMissing, account)
REDACTED
	if expiresAt != nil && !time.Now().Before(*expiresAt) {
		return "", withGrokCredentialFailureSnapshot(errGrokOAuthAccessTokenExpired, account)
REDACTED

	if p.tokenCache != nil {
		latestAccount, isStale := CheckTokenVersion(ctx, account, p.accountRepo)
		if isStale && latestAccount != nil {
			if eligibilityErr := grokOAuthRequestAccountEligibilityError(latestAccount); eligibilityErr != nil {
				return "", withGrokCredentialFailureSnapshot(eligibilityErr, latestAccount)
		REDACTED
			if !grokCredentialProxyIDsEqual(latestAccount.ProxyID, selectedProxyID) {
				return "", withGrokCredentialFailureSnapshot(errOAuthRefreshAccountStateChanged, latestAccount)
		REDACTED
			accessToken = latestAccount.GetGrokAccessToken()
			if strings.TrimSpace(accessToken) == "" {
				return "", withGrokCredentialFailureSnapshot(errGrokOAuthAccessTokenMissing, latestAccount)
		REDACTED
			latestExpiry := latestAccount.GetCredentialAsTime("expires_at")
			if latestExpiry == nil || !time.Now().Before(*latestExpiry) {
				return "", withGrokCredentialFailureSnapshot(errGrokOAuthAccessTokenExpired, latestAccount)
		REDACTED
	REDACTED else {
			ttl := 30 * time.Minute
			if expiresAt != nil {
				until := time.Until(*expiresAt)
				switch {
				case until > grokTokenCacheSkew:
					ttl = until - grokTokenCacheSkew
				case until > 0:
					ttl = until
				default:
					ttl = time.Minute
			REDACTED
		REDACTED
			_ = p.tokenCache.SetAccessToken(ctx, cacheKey, accessToken, ttl)
	REDACTED
REDACTED

	return accessToken, nil
REDACTED

func (p *GrokTokenProvider) waitForRefreshedToken(ctx context.Context, account *Account, cacheKey string) (string, error) {
	waitCtx, cancel := context.WithTimeout(ctx, grokRefreshLockWaitTimeout)
	defer cancel()

	initialToken := strings.TrimSpace(account.GetGrokAccessToken())
	initialVersion := account.GetCredentialAsInt64("_token_version")
	selectedProxyID := cloneGrokProxyID(account.ProxyID)
	sawAuthoritativeState := false
	var lastAccountReadErr error
	ticker := time.NewTicker(grokRefreshLockPollInterval)
	defer ticker.Stop()

	for {
		cachedToken := ""
		if p.tokenCache != nil {
			if token, err := p.tokenCache.GetAccessToken(waitCtx, cacheKey); err == nil {
				cachedToken = strings.TrimSpace(token)
		REDACTED
	REDACTED

		if p.accountRepo != nil {
			latest, err := p.accountRepo.GetByID(waitCtx, account.ID)
			if err != nil {
				lastAccountReadErr = err
		REDACTED else if latest == nil {
				return "", errOAuthRefreshAccountStateChanged
		REDACTED else {
				sawAuthoritativeState = true
				if eligibilityErr := grokOAuthRequestAccountEligibilityError(latest); eligibilityErr != nil {
					return "", withGrokCredentialFailureSnapshot(eligibilityErr, latest)
			REDACTED
				if !grokCredentialProxyIDsEqual(latest.ProxyID, selectedProxyID) {
					return "", withGrokCredentialFailureSnapshot(errOAuthRefreshAccountStateChanged, latest)
			REDACTED
				token := strings.TrimSpace(latest.GetGrokAccessToken())
				version := latest.GetCredentialAsInt64("_token_version")
				expiresAt := latest.GetCredentialAsTime("expires_at")
				changed := token != initialToken || (version > 0 && version > initialVersion)
				valid := expiresAt != nil && time.Now().Before(*expiresAt)
				if token != "" && changed && valid {
					// The versioned DB credential is authoritative. A stale cache must
					// not hold the request on the old expired token; repair it best-effort.
					if cachedToken != "" && cachedToken != token {
						ttl := time.Until(*expiresAt)
						if ttl > grokTokenCacheSkew {
							ttl -= grokTokenCacheSkew
					REDACTED
						_ = p.tokenCache.SetAccessToken(waitCtx, cacheKey, token, ttl)
				REDACTED
					return token, nil
			REDACTED
		REDACTED
	REDACTED

		select {
		case <-waitCtx.Done():
			if ctx.Err() != nil {
				return "", ctx.Err()
		REDACTED
			if !sawAuthoritativeState {
				if lastAccountReadErr == nil {
					lastAccountReadErr = waitCtx.Err()
			REDACTED
				return "", fmt.Errorf("%w: %v", errOAuthRefreshAccountRereadFailed, lastAccountReadErr)
		REDACTED
			// Another worker still owns the refresh and the authoritative row is
			// unchanged. Do not quarantine the old credential: its refresh may
			// commit immediately after this bounded wait.
			return "", errOAuthRefreshAccountStateChanged
		case <-ticker.C:
	REDACTED
REDACTED
REDACTED

func grokOAuthRequestAccountEligibilityError(account *Account) error {
	if account == nil || !account.IsGrokOAuth() || !account.IsSchedulable() {
		return errOAuthRefreshAccountStateChanged
REDACTED
	if account.ProxyID != nil && account.Proxy == nil {
		return errGrokOAuthConfiguredProxyMiss
REDACTED
	return nil
REDACTED

func cloneGrokProxyID(proxyID *int64) *int64 {
	if proxyID == nil {
		return nil
REDACTED
	value := *proxyID
	return &value
REDACTED

func (p *GrokTokenProvider) InvalidateToken(ctx context.Context, account *Account) error {
	if p == nil || p.tokenCache == nil || account == nil {
		return nil
REDACTED
	return p.tokenCache.DeleteAccessToken(ctx, GrokTokenCacheKey(account))
REDACTED

func GrokTokenCacheKey(account *Account) string {
	if account == nil {
		return "grok:account:0"
REDACTED
	return "grok:account:" + strconv.FormatInt(account.ID, 10)
REDACTED
