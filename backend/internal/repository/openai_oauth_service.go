package repository

import (
	"context"
	"errors"
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

func (s *openaiOAuthService) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL, clientID string) (*openai.TokenResponse, error) {
	client, err := createOpenAIReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
REDACTED

	if redirectURI == "" {
		redirectURI = openai.DefaultRedirectURI
REDACTED
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = openai.ClientID
REDACTED

	formData := url.Values{REDACTED
	formData.Set("grant_type", "authorization_code")
	formData.Set("client_id", clientID)
	formData.Set("code", code)
	formData.Set("redirect_uri", redirectURI)
	formData.Set("code_verifier", codeVerifier)

	var tokenResp openai.TokenResponse

	resp, err := client.R().
		SetContext(ctx).
		SetHeader("User-Agent", service.CodexCanonicalUserAgent()).
		SetFormDataFromValues(formData).
		SetSuccessResult(&tokenResp).
		Post(s.tokenURL)

	if err != nil {
		if shouldReturnOpenAINoProxyHint(ctx, proxyURL, err) {
			return nil, newOpenAINoProxyHintError(err)
	REDACTED
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
	// 调用方应始终传入正确的 client_id；为兼容旧数据，未指定时默认使用 OpenAI ClientID
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = openai.ClientID
REDACTED
	return s.refreshTokenWithClientID(ctx, refreshToken, proxyURL, clientID)
REDACTED

func (s *openaiOAuthService) refreshTokenWithClientID(ctx context.Context, refreshToken, proxyURL, clientID string) (*openai.TokenResponse, error) {
	client, err := createOpenAIReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
REDACTED

	formData := url.Values{REDACTED
	formData.Set("grant_type", "refresh_token")
	formData.Set("refresh_token", refreshToken)
	formData.Set("client_id", clientID)
	formData.Set("scope", openai.RefreshScopes)

	var tokenResp openai.TokenResponse

	resp, err := client.R().
		SetContext(ctx).
		SetHeader("User-Agent", service.CodexCanonicalUserAgent()).
		SetFormDataFromValues(formData).
		SetSuccessResult(&tokenResp).
		Post(s.tokenURL)

	if err != nil {
		if shouldReturnOpenAINoProxyHint(ctx, proxyURL, err) {
			return nil, newOpenAINoProxyHintError(err)
	REDACTED
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_REQUEST_FAILED", "request failed: %v", err)
REDACTED

	if !resp.IsSuccessState() {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_TOKEN_REFRESH_FAILED", "token refresh failed: status %d, body: %s", resp.StatusCode, resp.String())
REDACTED

	return &tokenResp, nil
REDACTED

func createOpenAIReqClient(proxyURL string) (*req.Client, error) {
	return getSharedReqClient(reqClientOptions{
		ProxyURL: proxyURL,
		Timeout:  120 * time.Second,
REDACTED)
REDACTED

func shouldReturnOpenAINoProxyHint(ctx context.Context, proxyURL string, err error) bool {
	if strings.TrimSpace(proxyURL) != "" || err == nil {
		return false
REDACTED
	if ctx != nil && ctx.Err() != nil {
		return false
REDACTED
	return !errors.Is(err, context.Canceled)
REDACTED

func newOpenAINoProxyHintError(cause error) error {
	return infraerrors.New(
		http.StatusBadGateway,
		"OPENAI_OAUTH_PROXY_REQUIRED",
		"OpenAI OAuth request failed: no proxy is configured and this server could not reach OpenAI directly. Select a proxy that can access OpenAI, then retry; if the authorization code has expired, regenerate the authorization URL.",
	).WithCause(cause)
REDACTED
