package service

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

type openaiOAuthClientAuthURLStub struct{REDACTED

func (s *openaiOAuthClientAuthURLStub) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL, clientID string) (*openai.TokenResponse, error) {
	return nil, errors.New("not implemented")
REDACTED

func (s *openaiOAuthClientAuthURLStub) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*openai.TokenResponse, error) {
	return nil, errors.New("not implemented")
REDACTED

func (s *openaiOAuthClientAuthURLStub) RefreshTokenWithClientID(ctx context.Context, refreshToken, proxyURL string, clientID string) (*openai.TokenResponse, error) {
	return nil, errors.New("not implemented")
REDACTED

func TestOpenAIOAuthService_GenerateAuthURL_OpenAIKeepsCodexFlow(t *testing.T) {
	svc := NewOpenAIOAuthService(nil, &openaiOAuthClientAuthURLStub{REDACTED)
	defer svc.Stop()

	result, err := svc.GenerateAuthURL(context.Background(), nil, "", PlatformOpenAI)
REDACTED
	require.NotEmpty(t, result.AuthURL)
	require.NotEmpty(t, result.SessionID)

	parsed, err := url.Parse(result.AuthURL)
REDACTED
	q := parsed.Query()
	require.Equal(t, openai.ClientID, q.Get("client_id"))
	require.Equal(t, "true", q.Get("codex_cli_simplified_flow"))

	session, ok := svc.sessionStore.Get(result.SessionID)
	require.True(t, ok)
	require.Equal(t, openai.ClientID, session.ClientID)
REDACTED

// TestOpenAIOAuthService_GenerateAuthURL_SoraUsesCodexClient 验证 Sora 平台复用 Codex CLI 的
// client_id（支持 localhost redirect_uri），但不启用 codex_cli_simplified_flow。
func TestOpenAIOAuthService_GenerateAuthURL_SoraUsesCodexClient(t *testing.T) {
	svc := NewOpenAIOAuthService(nil, &openaiOAuthClientAuthURLStub{REDACTED)
	defer svc.Stop()

	result, err := svc.GenerateAuthURL(context.Background(), nil, "", PlatformSora)
REDACTED
	require.NotEmpty(t, result.AuthURL)
	require.NotEmpty(t, result.SessionID)

	parsed, err := url.Parse(result.AuthURL)
REDACTED
	q := parsed.Query()
	require.Equal(t, openai.ClientID, q.Get("client_id"))
	require.Empty(t, q.Get("codex_cli_simplified_flow"))

	session, ok := svc.sessionStore.Get(result.SessionID)
	require.True(t, ok)
	require.Equal(t, openai.ClientID, session.ClientID)
REDACTED
