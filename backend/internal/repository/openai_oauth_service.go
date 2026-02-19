package repository

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/imroc/req/v3"
)

// NewOpenAIOAuthClient creates a new OpenAI OAuth client
func NewOpenAIOAuthClient() service.OpenAIOAuthClient {
	return &openaiOAuthService{tokenURL: openai.TokenURLREDACTED
REDACTED

type openaiOAuthService struct {
	tokenURL string
REDACTED

func (s *openaiOAuthService) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL string) (*openai.TokenResponse, error) {
	client := createOpenAIReqClient(proxyURL)

	if redirectURI == "" {
		redirectURI = openai.DefaultRedirectURI
REDACTED

	formData := url.Values{REDACTED
	formData.Set("grant_type", "authorization_code")
	formData.Set("client_id", openai.ClientID)
	formData.Set("code", code)
	formData.Set("redirect_uri", redirectURI)
	formData.Set("code_verifier", codeVerifier)

	var tokenResp openai.TokenResponse

	resp, err := client.R().
		SetContext(ctx).
		SetHeader("User-Agent", "codex-cli/0.91.0").
		SetFormDataFromValues(formData).
		SetSuccessResult(&tokenResp).
		Post(s.tokenURL)

	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_REQUEST_FAILED", "request failed: %v", err)
REDACTED

	if !resp.IsSuccessState() {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_TOKEN_EXCHANGE_FAILED", "token exchange failed: status %d, body: %s", resp.StatusCode, resp.String())
REDACTED

	return &tokenResp, nil
REDACTED

func (s *openaiOAuthService) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*openai.TokenResponse, error) {
	return s.RefreshTokenWithClientID(ctx, refreshToken, proxyURL, "")
REDACTED

func (s *openaiOAuthService) RefreshTokenWithClientID(ctx context.Context, refreshToken, proxyURL string, clientID string) (*openai.TokenResponse, error) {
	if strings.TrimSpace(clientID) != "" {
		return s.refreshTokenWithClientID(ctx, refreshToken, proxyURL, strings.TrimSpace(clientID))
REDACTED

	clientIDs := []string{
		openai.ClientID,
		openai.SoraClientID,
REDACTED
	seen := make(map[string]struct{REDACTED, len(clientIDs))
	var lastErr error
	for _, clientID := range clientIDs {
		clientID = strings.TrimSpace(clientID)
		if clientID == "" {
			continue
	REDACTED
		if _, ok := seen[clientID]; ok {
			continue
	REDACTED
		seen[clientID] = struct{REDACTED{REDACTED

		tokenResp, err := s.refreshTokenWithClientID(ctx, refreshToken, proxyURL, clientID)
		if err == nil {
			return tokenResp, nil
	REDACTED
		lastErr = err
REDACTED
	if lastErr != nil {
		return nil, lastErr
REDACTED
	return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_OAUTH_TOKEN_REFRESH_FAILED", "token refresh failed")
REDACTED

func (s *openaiOAuthService) refreshTokenWithClientID(ctx context.Context, refreshToken, proxyURL, clientID string) (*openai.TokenResponse, error) {
	client := createOpenAIReqClient(proxyURL)

	formData := url.Values{REDACTED
	formData.Set("grant_type", "refresh_token")
	formData.Set("refresh_token", refreshToken)
	formData.Set("client_id", clientID)
	formData.Set("scope", openai.RefreshScopes)

	var tokenResp openai.TokenResponse

	resp, err := client.R().
		SetContext(ctx).
		SetHeader("User-Agent", "codex-cli/0.91.0").
		SetFormDataFromValues(formData).
		SetSuccessResult(&tokenResp).
		Post(s.tokenURL)

	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_REQUEST_FAILED", "request failed: %v", err)
REDACTED

	if !resp.IsSuccessState() {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_TOKEN_REFRESH_FAILED", "token refresh failed: status %d, body: %s", resp.StatusCode, resp.String())
REDACTED

	return &tokenResp, nil
REDACTED

func createOpenAIReqClient(proxyURL string) *req.Client {
	return getSharedReqClient(reqClientOptions{
		ProxyURL: proxyURL,
		Timeout:  120 * time.Second,
REDACTED)
REDACTED
