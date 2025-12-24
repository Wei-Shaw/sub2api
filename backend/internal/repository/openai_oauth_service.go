package repository

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/service/ports"

	"github.com/imroc/req/v3"
)

type openaiOAuthService struct{REDACTED

// NewOpenAIOAuthClient creates a new OpenAI OAuth client
func NewOpenAIOAuthClient() ports.OpenAIOAuthClient {
	return &openaiOAuthService{REDACTED
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
		SetFormDataFromValues(formData).
		SetSuccessResult(&tokenResp).
		Post(openai.TokenURL)

	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
REDACTED

	if !resp.IsSuccessState() {
		return nil, fmt.Errorf("token exchange failed: status %d, body: %s", resp.StatusCode, resp.String())
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
		SetFormDataFromValues(formData).
		SetSuccessResult(&tokenResp).
		Post(openai.TokenURL)

	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
REDACTED

	if !resp.IsSuccessState() {
		return nil, fmt.Errorf("token refresh failed: status %d, body: %s", resp.StatusCode, resp.String())
REDACTED

	return &tokenResp, nil
REDACTED

func createOpenAIReqClient(proxyURL string) *req.Client {
	client := req.C().
		SetTimeout(60 * time.Second)

	if proxyURL != "" {
		client.SetProxyURL(proxyURL)
REDACTED

	return client
REDACTED
