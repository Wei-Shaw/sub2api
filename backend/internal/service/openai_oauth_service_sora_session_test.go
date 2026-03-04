package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

type openaiOAuthClientNoopStub struct{REDACTED

func (s *openaiOAuthClientNoopStub) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL, clientID string) (*openai.TokenResponse, error) {
	return nil, errors.New("not implemented")
REDACTED

func (s *openaiOAuthClientNoopStub) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*openai.TokenResponse, error) {
	return nil, errors.New("not implemented")
REDACTED

func (s *openaiOAuthClientNoopStub) RefreshTokenWithClientID(ctx context.Context, refreshToken, proxyURL string, clientID string) (*openai.TokenResponse, error) {
	return nil, errors.New("not implemented")
REDACTED

func TestOpenAIOAuthService_ExchangeSoraSessionToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Contains(t, r.Header.Get("Cookie"), "__Secure-next-auth.session-token=st-token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accessToken":"at-token","expires":"2099-01-01T00:00:00Z","user":{"email":"demo@example.com"REDACTEDREDACTED`))
REDACTED))
	defer server.Close()

	origin := openAISoraSessionAuthURL
	openAISoraSessionAuthURL = server.URL
	defer func() { openAISoraSessionAuthURL = origin REDACTED()

	svc := NewOpenAIOAuthService(nil, &openaiOAuthClientNoopStub{REDACTED)
	defer svc.Stop()

	info, err := svc.ExchangeSoraSessionToken(context.Background(), "st-token", nil)
REDACTED
	require.NotNil(t, info)
	require.Equal(t, "at-token", info.AccessToken)
	require.Equal(t, "demo@example.com", info.Email)
	require.Greater(t, info.ExpiresAt, int64(0))
REDACTED

func TestOpenAIOAuthService_ExchangeSoraSessionToken_MissingAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"expires":"2099-01-01T00:00:00Z"REDACTED`))
REDACTED))
	defer server.Close()

	origin := openAISoraSessionAuthURL
	openAISoraSessionAuthURL = server.URL
	defer func() { openAISoraSessionAuthURL = origin REDACTED()

	svc := NewOpenAIOAuthService(nil, &openaiOAuthClientNoopStub{REDACTED)
	defer svc.Stop()

	_, err := svc.ExchangeSoraSessionToken(context.Background(), "st-token", nil)
REDACTED
	require.Contains(t, err.Error(), "missing access token")
REDACTED

func TestOpenAIOAuthService_ExchangeSoraSessionToken_AcceptsSetCookieLine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Contains(t, r.Header.Get("Cookie"), "__Secure-next-auth.session-token=st-cookie-value")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accessToken":"at-token","expires":"2099-01-01T00:00:00Z","user":{"email":"demo@example.com"REDACTEDREDACTED`))
REDACTED))
	defer server.Close()

	origin := openAISoraSessionAuthURL
	openAISoraSessionAuthURL = server.URL
	defer func() { openAISoraSessionAuthURL = origin REDACTED()

	svc := NewOpenAIOAuthService(nil, &openaiOAuthClientNoopStub{REDACTED)
	defer svc.Stop()

	raw := "__Secure-next-auth.session-token.0=st-cookie-value; Domain=.chatgpt.com; Path=/; HttpOnly; Secure; SameSite=Lax"
	info, err := svc.ExchangeSoraSessionToken(context.Background(), raw, nil)
REDACTED
	require.Equal(t, "at-token", info.AccessToken)
REDACTED

func TestOpenAIOAuthService_ExchangeSoraSessionToken_MergesChunkedSetCookieLines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Contains(t, r.Header.Get("Cookie"), "__Secure-next-auth.session-token=chunk-0chunk-1")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accessToken":"at-token","expires":"2099-01-01T00:00:00Z","user":{"email":"demo@example.com"REDACTEDREDACTED`))
REDACTED))
	defer server.Close()

	origin := openAISoraSessionAuthURL
	openAISoraSessionAuthURL = server.URL
	defer func() { openAISoraSessionAuthURL = origin REDACTED()

	svc := NewOpenAIOAuthService(nil, &openaiOAuthClientNoopStub{REDACTED)
	defer svc.Stop()

	raw := strings.Join([]string{
		"Set-Cookie: __Secure-next-auth.session-token.1=chunk-1; Path=/; HttpOnly",
		"Set-Cookie: __Secure-next-auth.session-token.0=chunk-0; Path=/; HttpOnly",
REDACTED, "\n")
	info, err := svc.ExchangeSoraSessionToken(context.Background(), raw, nil)
REDACTED
	require.Equal(t, "at-token", info.AccessToken)
REDACTED

func TestOpenAIOAuthService_ExchangeSoraSessionToken_PrefersLatestDuplicateChunks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Contains(t, r.Header.Get("Cookie"), "__Secure-next-auth.session-token=new-0new-1")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accessToken":"at-token","expires":"2099-01-01T00:00:00Z","user":{"email":"demo@example.com"REDACTEDREDACTED`))
REDACTED))
	defer server.Close()

	origin := openAISoraSessionAuthURL
	openAISoraSessionAuthURL = server.URL
	defer func() { openAISoraSessionAuthURL = origin REDACTED()

	svc := NewOpenAIOAuthService(nil, &openaiOAuthClientNoopStub{REDACTED)
	defer svc.Stop()

	raw := strings.Join([]string{
		"Set-Cookie: __Secure-next-auth.session-token.0=old-0; Path=/; HttpOnly",
		"Set-Cookie: __Secure-next-auth.session-token.1=old-1; Path=/; HttpOnly",
		"Set-Cookie: __Secure-next-auth.session-token.0=new-0; Path=/; HttpOnly",
		"Set-Cookie: __Secure-next-auth.session-token.1=new-1; Path=/; HttpOnly",
REDACTED, "\n")
	info, err := svc.ExchangeSoraSessionToken(context.Background(), raw, nil)
REDACTED
	require.Equal(t, "at-token", info.AccessToken)
REDACTED

func TestOpenAIOAuthService_ExchangeSoraSessionToken_UsesLatestCompleteChunkGroup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Contains(t, r.Header.Get("Cookie"), "__Secure-next-auth.session-token=ok-0ok-1")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accessToken":"at-token","expires":"2099-01-01T00:00:00Z","user":{"email":"demo@example.com"REDACTEDREDACTED`))
REDACTED))
	defer server.Close()

	origin := openAISoraSessionAuthURL
	openAISoraSessionAuthURL = server.URL
	defer func() { openAISoraSessionAuthURL = origin REDACTED()

	svc := NewOpenAIOAuthService(nil, &openaiOAuthClientNoopStub{REDACTED)
	defer svc.Stop()

	raw := strings.Join([]string{
		"set-cookie",
		"__Secure-next-auth.session-token.0=ok-0; Domain=.chatgpt.com; Path=/",
		"set-cookie",
		"__Secure-next-auth.session-token.1=ok-1; Domain=.chatgpt.com; Path=/",
		"set-cookie",
		"__Secure-next-auth.session-token.0=partial-0; Domain=.chatgpt.com; Path=/",
REDACTED, "\n")
	info, err := svc.ExchangeSoraSessionToken(context.Background(), raw, nil)
REDACTED
	require.Equal(t, "at-token", info.AccessToken)
REDACTED
