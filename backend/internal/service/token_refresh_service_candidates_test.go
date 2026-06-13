package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

type tokenRefreshCandidateRepo struct {
	AccountRepository
	accounts              []Account
	updatedCredentialIDs  []int64
	setErrorCalls         int
	setTempUnschedCalls   int
	lastTempUnschedReason string
	listActiveCalls       int
REDACTED

func (r *tokenRefreshCandidateRepo) ListActive(context.Context) ([]Account, error) {
	r.listActiveCalls++
	return r.accounts, nil
REDACTED

func (r *tokenRefreshCandidateRepo) ListOAuthRefreshCandidates(context.Context) ([]Account, error) {
	candidates := make([]Account, 0, len(r.accounts))
	now := time.Now()
	for _, account := range r.accounts {
		refreshToken, _ := account.Credentials["refresh_token"].(string)
		inRetryCooldown := account.TempUnschedulableUntil != nil &&
			account.TempUnschedulableUntil.After(now) &&
			strings.HasPrefix(account.TempUnschedulableReason, tokenRefreshRetryExhaustedReasonPrefix)
		if account.Status != StatusActive ||
			account.Type != AccountTypeOAuth ||
			!isOAuthRefreshPlatform(account.Platform) ||
			strings.TrimSpace(refreshToken) == "" ||
			inRetryCooldown {
			continue
	REDACTED
		candidates = append(candidates, account)
REDACTED
	return candidates, nil
REDACTED

func (r *tokenRefreshCandidateRepo) UpdateCredentials(_ context.Context, id int64, _ map[string]any) error {
	r.updatedCredentialIDs = append(r.updatedCredentialIDs, id)
	return nil
REDACTED

func (r *tokenRefreshCandidateRepo) SetError(context.Context, int64, string) error {
	r.setErrorCalls++
	return nil
REDACTED

func (r *tokenRefreshCandidateRepo) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, reason string) error {
	r.setTempUnschedCalls++
	r.lastTempUnschedReason = reason
	return nil
REDACTED

func isOAuthRefreshPlatform(platform string) bool {
	switch platform {
	case PlatformAnthropic, PlatformOpenAI, PlatformGemini, PlatformAntigravity:
		return true
	default:
		return false
REDACTED
REDACTED

type tokenRefreshTestRefresher struct {
	err error
REDACTED

func (r *tokenRefreshTestRefresher) CanRefresh(*Account) bool { return true REDACTED

func (r *tokenRefreshTestRefresher) NeedsRefresh(*Account, time.Duration) bool { return true REDACTED

func (r *tokenRefreshTestRefresher) Refresh(context.Context, *Account) (map[string]any, error) {
	if r.err != nil {
		return nil, r.err
REDACTED
	return map[string]any{"access_token": "new-access-token", "refresh_token": "new-refresh-token"REDACTED, nil
REDACTED

func TestTokenRefreshService_ProcessRefreshUsesOAuthRefreshCandidates(t *testing.T) {
	future := time.Now().Add(10 * time.Minute)
	repo := &tokenRefreshCandidateRepo{
		accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Status:      StatusActive,
		REDACTED"refresh_token": "refresh-token"REDACTED,
		REDACTED,
			{
				ID:          2,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Status:      StatusActive,
		REDACTEDREDACTED,
		REDACTED,
			{
				ID:          3,
				Platform:    PlatformGemini,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
		REDACTED"refresh_token": "refresh-token"REDACTED,
		REDACTED,
			{
				ID:                      4,
				Platform:                PlatformAntigravity,
				Type:                    AccountTypeOAuth,
				Status:                  StatusActive,
				Credentials:             map[string]any{"refresh_token": "refresh-token"REDACTED,
				TempUnschedulableUntil:  &future,
				TempUnschedulableReason: tokenRefreshRetryExhaustedReasonPrefix + " network timeout",
		REDACTED,
			{
				ID:          5,
				Platform:    "other",
				Type:        AccountTypeOAuth,
				Status:      StatusActive,
		REDACTED"refresh_token": "refresh-token"REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	svc := &TokenRefreshService{
		accountRepo:   repo,
		refreshers:    []TokenRefresher{&tokenRefreshTestRefresher{REDACTEDREDACTED,
		refreshPolicy: DefaultBackgroundRefreshPolicy(),
		cfg:           &config.TokenRefreshConfig{RefreshBeforeExpiryHours: 1, MaxRetries: 1REDACTED,
REDACTED

	svc.processRefresh()

	require.Zero(t, repo.listActiveCalls, "TokenRefreshService should not use the broad active-account query")
	require.Equal(t, []int64{1REDACTED, repo.updatedCredentialIDs)
REDACTED

func TestTokenRefreshService_RefreshFailureDoesNotCallPrivacy(t *testing.T) {
	tests := []struct {
		name string
		err  error
REDACTED{
		{name: "retry exhausted", err: errors.New("temporary upstream timeout")REDACTED,
		{name: "non retryable", err: errors.New("invalid_grant: token revoked")REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &tokenRefreshCandidateRepo{REDACTED
			svc := &TokenRefreshService{
				accountRepo:   repo,
				refreshPolicy: DefaultBackgroundRefreshPolicy(),
				cfg:           &config.TokenRefreshConfig{MaxRetries: 1, RetryBackoffSeconds: 0REDACTED,
				privacyClientFactory: func(string) (*req.Client, error) {
					t.Fatalf("privacy client factory must not be called on refresh failure")
					return nil, errors.New("unexpected privacy call")
			REDACTED,
		REDACTED
			account := &Account{
				ID:       11,
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
		REDACTED
					"access_token":  "old-access-token",
					"refresh_token": "refresh-token",
			REDACTED,
		REDACTED

			err := svc.refreshWithRetry(context.Background(), account, &tokenRefreshTestRefresher{err: tt.errREDACTED, nil, time.Hour)

		REDACTED
			if isNonRetryableRefreshError(tt.err) {
				require.Equal(t, 1, repo.setErrorCalls)
				require.Zero(t, repo.setTempUnschedCalls)
		REDACTED else {
				require.Zero(t, repo.setErrorCalls)
				require.Equal(t, 1, repo.setTempUnschedCalls)
				require.True(t, strings.HasPrefix(repo.lastTempUnschedReason, tokenRefreshRetryExhaustedReasonPrefix))
		REDACTED
	REDACTED)
REDACTED
REDACTED
