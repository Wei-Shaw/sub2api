package repository

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/service/ports"

	"github.com/imroc/req/v3"
)

type geminiOAuthClient struct {
	tokenURL string
	cfg      *config.Config
REDACTED

func NewGeminiOAuthClient(cfg *config.Config) ports.GeminiOAuthClient {
	return &geminiOAuthClient{
		tokenURL: geminicli.TokenURL,
		cfg:      cfg,
REDACTED
REDACTED

func (c *geminiOAuthClient) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL string) (*geminicli.TokenResponse, error) {
	client := createGeminiReqClient(proxyURL)

	formData := url.Values{REDACTED
	formData.Set("grant_type", "authorization_code")
	formData.Set("client_id", c.cfg.Gemini.OAuth.ClientID)
	formData.Set("client_secret", c.cfg.Gemini.OAuth.ClientSecret)
	formData.Set("code", code)
	formData.Set("code_verifier", codeVerifier)
	formData.Set("redirect_uri", redirectURI)

	var tokenResp geminicli.TokenResponse
	resp, err := client.R().
		SetContext(ctx).
		SetFormDataFromValues(formData).
		SetSuccessResult(&tokenResp).
		Post(c.tokenURL)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
REDACTED
	if !resp.IsSuccessState() {
		return nil, fmt.Errorf("token exchange failed: status %d, body: %s", resp.StatusCode, geminicli.SanitizeBodyForLogs(resp.String()))
REDACTED
	return &tokenResp, nil
REDACTED

func (c *geminiOAuthClient) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*geminicli.TokenResponse, error) {
	client := createGeminiReqClient(proxyURL)

	formData := url.Values{REDACTED
	formData.Set("grant_type", "refresh_token")
	formData.Set("refresh_token", refreshToken)
	formData.Set("client_id", c.cfg.Gemini.OAuth.ClientID)
	formData.Set("client_secret", c.cfg.Gemini.OAuth.ClientSecret)

	var tokenResp geminicli.TokenResponse
	resp, err := client.R().
		SetContext(ctx).
		SetFormDataFromValues(formData).
		SetSuccessResult(&tokenResp).
		Post(c.tokenURL)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
REDACTED
	if !resp.IsSuccessState() {
		return nil, fmt.Errorf("token refresh failed: status %d, body: %s", resp.StatusCode, geminicli.SanitizeBodyForLogs(resp.String()))
REDACTED
	return &tokenResp, nil
REDACTED

func createGeminiReqClient(proxyURL string) *req.Client {
	client := req.C().SetTimeout(60 * time.Second)
	if proxyURL != "" {
		client.SetProxyURL(proxyURL)
REDACTED
	return client
REDACTED
