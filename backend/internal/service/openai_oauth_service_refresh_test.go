package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

type openaiOAuthClientRefreshStub struct {
	refreshCalls int32
REDACTED

func (s *openaiOAuthClientRefreshStub) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL, clientID string) (*openai.TokenResponse, error) {
	return nil, errors.New("not implemented")
REDACTED

func (s *openaiOAuthClientRefreshStub) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*openai.TokenResponse, error) {
	atomic.AddInt32(&s.refreshCalls, 1)
	return nil, errors.New("not implemented")
REDACTED

func (s *openaiOAuthClientRefreshStub) RefreshTokenWithClientID(ctx context.Context, refreshToken, proxyURL string, clientID string) (*openai.TokenResponse, error) {
	atomic.AddInt32(&s.refreshCalls, 1)
	return nil, errors.New("not implemented")
REDACTED

func TestOpenAIOAuthService_RefreshAccountToken_NoRefreshTokenUsesExistingAccessToken(t *testing.T) {
	client := &openaiOAuthClientRefreshStub{REDACTED
	svc := NewOpenAIOAuthService(nil, client)
	var privacyClientCalls int32
	svc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		atomic.AddInt32(&privacyClientCalls, 1)
		return nil, errors.New("stop before request")
REDACTED)

	expiresAt := time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339)
	account := &Account{
		ID:       77,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token": "existing-access-token",
			"expires_at":   expiresAt,
			"client_id":    "client-id-1",
	REDACTED,
REDACTED

	info, err := svc.RefreshAccountToken(context.Background(), account)
REDACTED
	require.NotNil(t, info)
	require.Equal(t, "existing-access-token", info.AccessToken)
	require.Equal(t, "client-id-1", info.ClientID)
	require.Zero(t, atomic.LoadInt32(&client.refreshCalls), "existing access token should be reused without calling refresh")
	require.Positive(t, atomic.LoadInt32(&privacyClientCalls), "existing access token should still run enrichment")
REDACTED

func TestOpenAIOAuthService_RefreshAccountToken_PATIgnoresStaleRefreshToken(t *testing.T) {
	client := &openaiOAuthClientRefreshStub{REDACTED
	var whoamiCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&whoamiCalls, 1)
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"email":"user@example.com",
			"chatgpt_user_id":"user-123",
			"chatgpt_account_id":"acct-123",
			"chatgpt_plan_type":"plus",
			"chatgpt_account_is_fedramp":false
	REDACTED`))
REDACTED))
	defer server.Close()

	originalURL := openAICodexPATWhoamiURL
	openAICodexPATWhoamiURL = server.URL
	defer func() { openAICodexPATWhoamiURL = originalURL REDACTED()

	svc := NewOpenAIOAuthService(nil, client)
	defer svc.Stop()

	account := &Account{
		ID:       77,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token":  "at-test-token",
			"refresh_token": "stale-refresh-token",
			"expires_at":    time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
			"auth_mode":     "personal_access_token",
	REDACTED,
REDACTED

	info, err := svc.RefreshAccountToken(context.Background(), account)
REDACTED
	require.Equal(t, OpenAIAuthModePersonalAccessToken, info.AuthMode)
	require.Equal(t, "at-test-token", info.AccessToken)
	require.Empty(t, info.RefreshToken)
	require.Equal(t, int32(1), atomic.LoadInt32(&whoamiCalls))
	require.Zero(t, atomic.LoadInt32(&client.refreshCalls), "PAT accounts must not call OAuth refresh even if stale refresh_token remains")
REDACTED

func TestOpenAITokenRefresher_NeedsRefresh_SkipsAccountWithoutRefreshToken(t *testing.T) {
	refresher := NewOpenAITokenRefresher(nil, nil)
	expiresAt := time.Now().Add(time.Minute).UTC().Format(time.RFC3339)

	withoutRT := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token": "access-token",
			"expires_at":   expiresAt,
	REDACTED,
REDACTED
	require.False(t, refresher.NeedsRefresh(withoutRT, 5*time.Minute))

	withRT := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
			"expires_at":    expiresAt,
	REDACTED,
REDACTED
	require.True(t, refresher.NeedsRefresh(withRT, 5*time.Minute))

	patWithStaleRT := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token":  "at-test-token",
			"refresh_token": "stale-refresh-token",
			"expires_at":    expiresAt,
			"auth_mode":     OpenAIAuthModePersonalAccessToken,
	REDACTED,
REDACTED
	require.False(t, refresher.NeedsRefresh(patWithStaleRT, 5*time.Minute))
REDACTED

func TestOpenAITokenRefresher_Refresh_PATRemovesStaleOAuthFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"email":"user@example.com",
			"chatgpt_user_id":"user-123",
			"chatgpt_account_id":"acct-123",
			"chatgpt_plan_type":"plus",
			"chatgpt_account_is_fedramp":true
	REDACTED`))
REDACTED))
	defer server.Close()

	originalURL := openAICodexPATWhoamiURL
	openAICodexPATWhoamiURL = server.URL
	defer func() { openAICodexPATWhoamiURL = originalURL REDACTED()

	svc := NewOpenAIOAuthService(nil, nil)
	defer svc.Stop()
	refresher := NewOpenAITokenRefresher(svc, nil)

	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token":  "at-test-token",
			"refresh_token": "stale-refresh-token",
			"id_token":      "stale-id-token",
			"expires_at":    time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
			"expires_in":    3600,
			"client_id":     "stale-client",
			"auth_mode":     OpenAIAuthModePersonalAccessToken,
			"model_mapping": map[string]any{"gpt-5": "gpt-5-codex"REDACTED,
	REDACTED,
REDACTED

	credentials, err := refresher.Refresh(context.Background(), account)
REDACTED
	require.Equal(t, "at-test-token", credentials["access_token"])
	require.Equal(t, OpenAIAuthModePersonalAccessToken, credentials["auth_mode"])
	require.Equal(t, "personal_access_token", credentials["openai_auth_mode"])
	require.NotContains(t, credentials, "refresh_token")
	require.NotContains(t, credentials, "id_token")
	require.NotContains(t, credentials, "expires_at")
	require.NotContains(t, credentials, "expires_in")
	require.NotContains(t, credentials, "client_id")
	require.Equal(t, map[string]any{"gpt-5": "gpt-5-codex"REDACTED, credentials["model_mapping"])
REDACTED

func TestOpenAITokenProvider_NoRefreshTokenExpiredAccessTokenReturnsError(t *testing.T) {
	provider := NewOpenAITokenProvider(nil, nil, nil)
	expiresAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token": "expired-access-token",
			"expires_at":   expiresAt,
	REDACTED,
REDACTED

	token, err := provider.GetAccessToken(context.Background(), account)
REDACTED
	require.Empty(t, token)
	require.Contains(t, err.Error(), "refresh_token is missing")
REDACTED
