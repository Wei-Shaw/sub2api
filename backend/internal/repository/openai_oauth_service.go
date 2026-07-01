package repository

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
func NewOpenAIOAuthClient(httpUpstream service.HTTPUpstream) service.OpenAIOAuthClient {
	return &openaiOAuthService{tokenURL: openai.TokenURL, httpUpstream: httpUpstream}
}

type openaiOAuthService struct {
	tokenURL     string
	httpUpstream service.HTTPUpstream
}

func (s *openaiOAuthService) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL, clientID string, options ...service.OpenAIOAuthTokenRequestOptions) (*openai.TokenResponse, error) {
	if redirectURI == "" {
		redirectURI = openai.DefaultRedirectURI
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = openai.ClientID
	}

	formData := url.Values{}
	formData.Set("grant_type", "authorization_code")
	formData.Set("client_id", clientID)
	formData.Set("code", code)
	formData.Set("redirect_uri", redirectURI)
	formData.Set("code_verifier", codeVerifier)

	return s.postTokenForm(ctx, formData, proxyURL, "OPENAI_OAUTH_TOKEN_EXCHANGE_FAILED", "token exchange failed", options...)
}

func (s *openaiOAuthService) RefreshToken(ctx context.Context, refreshToken, proxyURL string, options ...service.OpenAIOAuthTokenRequestOptions) (*openai.TokenResponse, error) {
	return s.RefreshTokenWithClientID(ctx, refreshToken, proxyURL, "", options...)
}

func (s *openaiOAuthService) RefreshTokenWithClientID(ctx context.Context, refreshToken, proxyURL string, clientID string, options ...service.OpenAIOAuthTokenRequestOptions) (*openai.TokenResponse, error) {
	// 调用方应始终传入正确的 client_id；为兼容旧数据，未指定时默认使用 OpenAI ClientID
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = openai.ClientID
	}
	return s.refreshTokenWithClientID(ctx, refreshToken, proxyURL, clientID, options...)
}

func (s *openaiOAuthService) refreshTokenWithClientID(ctx context.Context, refreshToken, proxyURL, clientID string, options ...service.OpenAIOAuthTokenRequestOptions) (*openai.TokenResponse, error) {
	formData := url.Values{}
	formData.Set("grant_type", "refresh_token")
	formData.Set("refresh_token", refreshToken)
	formData.Set("client_id", clientID)
	formData.Set("scope", openai.RefreshScopes)

	return s.postTokenForm(ctx, formData, proxyURL, "OPENAI_OAUTH_TOKEN_REFRESH_FAILED", "token refresh failed", options...)
}

func (s *openaiOAuthService) postTokenForm(ctx context.Context, formData url.Values, proxyURL, failureReason, failureMessage string, options ...service.OpenAIOAuthTokenRequestOptions) (*openai.TokenResponse, error) {
	option := firstOpenAIOAuthTokenRequestOption(options)
	if option.TLSProfile != nil {
		return s.postTokenFormWithTLS(ctx, formData, proxyURL, failureReason, failureMessage, option)
	}

	client, err := createOpenAIReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}

	var tokenResp openai.TokenResponse

	resp, err := client.R().
		SetContext(ctx).
		SetHeader("User-Agent", resolveOpenAIOAuthTokenUserAgent(option)).
		SetFormDataFromValues(formData).
		SetSuccessResult(&tokenResp).
		Post(s.tokenURL)

	if err != nil {
		if shouldReturnOpenAINoProxyHint(ctx, proxyURL, err) {
			return nil, newOpenAINoProxyHintError(err)
		}
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_REQUEST_FAILED", "request failed: %v", err)
	}

	if !resp.IsSuccessState() {
		return nil, infraerrors.Newf(http.StatusBadGateway, failureReason, "%s: status %d, body: %s", failureMessage, resp.StatusCode, resp.String())
	}

	return &tokenResp, nil
}

func (s *openaiOAuthService) postTokenFormWithTLS(ctx context.Context, formData url.Values, proxyURL, failureReason, failureMessage string, option service.OpenAIOAuthTokenRequestOptions) (*openai.TokenResponse, error) {
	if s.httpUpstream == nil {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_OAUTH_CLIENT_INIT_FAILED", "HTTP upstream is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_REQUEST_FAILED", "build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", resolveOpenAIOAuthTokenUserAgent(option))
	req = req.WithContext(service.WithHTTPUpstreamProfile(req.Context(), service.HTTPUpstreamProfileOpenAI))

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, option.AccountID, option.AccountConcurrency, option.TLSProfile)
	if err != nil {
		if shouldReturnOpenAINoProxyHint(ctx, proxyURL, err) {
			return nil, newOpenAINoProxyHintError(err)
		}
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_REQUEST_FAILED", "request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_REQUEST_FAILED", "read response: %v", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, infraerrors.Newf(http.StatusBadGateway, failureReason, "%s: status %d, body: %s", failureMessage, resp.StatusCode, string(body))
	}

	var tokenResp openai.TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_REQUEST_FAILED", "decode token response: %v", err)
	}
	return &tokenResp, nil
}

func createOpenAIReqClient(proxyURL string) (*req.Client, error) {
	return getSharedReqClient(reqClientOptions{
		ProxyURL: proxyURL,
		Timeout:  120 * time.Second,
	})
}

func firstOpenAIOAuthTokenRequestOption(options []service.OpenAIOAuthTokenRequestOptions) service.OpenAIOAuthTokenRequestOptions {
	if len(options) == 0 {
		return service.OpenAIOAuthTokenRequestOptions{}
	}
	return options[0]
}

func resolveOpenAIOAuthTokenUserAgent(option service.OpenAIOAuthTokenRequestOptions) string {
	if ua := strings.TrimSpace(option.UserAgent); ua != "" {
		return ua
	}
	return "codex-cli/0.91.0"
}

func shouldReturnOpenAINoProxyHint(ctx context.Context, proxyURL string, err error) bool {
	if strings.TrimSpace(proxyURL) != "" || err == nil {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	return !errors.Is(err, context.Canceled)
}

func newOpenAINoProxyHintError(cause error) error {
	return infraerrors.New(
		http.StatusBadGateway,
		"OPENAI_OAUTH_PROXY_REQUIRED",
		"OpenAI OAuth request failed: no proxy is configured and this server could not reach OpenAI directly. Select a proxy that can access OpenAI, then retry; if the authorization code has expired, regenerate the authorization URL.",
	).WithCause(cause)
}
