package repository

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/imroc/req/v3"
)

type geminiOAuthClient struct {
	tokenURL string
	cfg      *config.Config
REDACTED

func NewGeminiOAuthClient(cfg *config.Config) service.GeminiOAuthClient {
	return &geminiOAuthClient{
		tokenURL: geminicli.TokenURL,
		cfg:      cfg,
REDACTED
REDACTED

func (c *geminiOAuthClient) ExchangeCode(ctx context.Context, oauthType, code, codeVerifier, redirectURI, proxyURL string) (*geminicli.TokenResponse, error) {
	client, err := createGeminiReqClient(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("create HTTP client: %w", err)
REDACTED

	// Use different OAuth clients based on oauthType:
	// - code_assist: always use built-in Gemini CLI OAuth client (public)
	// - google_one: always use built-in Gemini CLI OAuth client (public)
	// - ai_studio: requires a user-provided OAuth client
	oauthCfgInput := geminicli.OAuthConfig{
		ClientID:     c.cfg.Gemini.OAuth.ClientID,
		ClientSecret: c.cfg.Gemini.OAuth.ClientSecret,
		Scopes:       c.cfg.Gemini.OAuth.Scopes,
REDACTED
	if oauthType == "code_assist" || oauthType == "google_one" {
		// Force use of built-in Gemini CLI OAuth client
		oauthCfgInput.ClientID = ""
		oauthCfgInput.ClientSecret = ""
REDACTED

	oauthCfg, err := geminicli.EffectiveOAuthConfig(oauthCfgInput, oauthType)
	if err != nil {
		return nil, err
REDACTED

	formData := url.Values{REDACTED
	formData.Set("grant_type", "authorization_code")
	formData.Set("client_id", oauthCfg.ClientID)
	formData.Set("client_secret", oauthCfg.ClientSecret)
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

func (c *geminiOAuthClient) RefreshToken(ctx context.Context, oauthType, refreshToken, proxyURL string) (*geminicli.TokenResponse, error) {
	client, err := createGeminiReqClient(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("create HTTP client: %w", err)
REDACTED

	oauthCfgInput := geminicli.OAuthConfig{
		ClientID:     c.cfg.Gemini.OAuth.ClientID,
		ClientSecret: c.cfg.Gemini.OAuth.ClientSecret,
		Scopes:       c.cfg.Gemini.OAuth.Scopes,
REDACTED
	if oauthType == "code_assist" || oauthType == "google_one" {
		// Force use of built-in Gemini CLI OAuth client
		oauthCfgInput.ClientID = ""
		oauthCfgInput.ClientSecret = ""
REDACTED

	oauthCfg, err := geminicli.EffectiveOAuthConfig(oauthCfgInput, oauthType)
	if err != nil {
		return nil, err
REDACTED

	formData := url.Values{REDACTED
	formData.Set("grant_type", "refresh_token")
	formData.Set("refresh_token", refreshToken)
	formData.Set("client_id", oauthCfg.ClientID)
	formData.Set("client_secret", oauthCfg.ClientSecret)

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

func createGeminiReqClient(proxyURL string) (*req.Client, error) {
	return getSharedReqClient(reqClientOptions{
		ProxyURL: proxyURL,
		Timeout:  60 * time.Second,
REDACTED)
REDACTED
