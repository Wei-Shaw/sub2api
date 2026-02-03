package repository

import (
	"context"
	"net/http"
	"net/url"
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
	client := createOpenAIReqClient(proxyURL)

	formData := url.Values{REDACTED
	formData.Set("grant_type", "refresh_token")
	formData.Set("refresh_token", refreshToken)
	formData.Set("client_id", openai.ClientID)
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
