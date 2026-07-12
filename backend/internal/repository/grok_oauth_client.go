package repository

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"github.com/imroc/req/v3"
)

type grokOAuthClient struct {
	tokenURL string
}

func NewGrokOAuthClient() service.GrokOAuthClient {
	return &grokOAuthClient{tokenURL: xai.EffectiveTokenURL()}
}

func (c *grokOAuthClient) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL, clientID string) (*xai.TokenResponse, error) {
	client, err := createGrokReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}

	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = xai.EffectiveClientID()
	}

	formData := url.Values{}
	formData.Set("grant_type", "authorization_code")
	formData.Set("client_id", clientID)
	formData.Set("code", code)
	formData.Set("redirect_uri", xai.EffectiveRedirectURI(redirectURI))
	formData.Set("code_verifier", codeVerifier)

	var tokenResp xai.TokenResponse
	resp, err := client.R().
		SetContext(ctx).
		SetHeader("User-Agent", "sub2api-grok-oauth/1.0").
		SetFormDataFromValues(formData).
		SetSuccessResult(&tokenResp).
		Post(c.tokenURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_OAUTH_REQUEST_FAILED", "request failed: %v", err)
	}
	if !resp.IsSuccessState() {
		return nil, grokOAuthStatusError("GROK_OAUTH_TOKEN_EXCHANGE_FAILED", "token exchange failed", resp)
	}
	return &tokenResp, nil
}

func (c *grokOAuthClient) RefreshToken(ctx context.Context, refreshToken, proxyURL, clientID string) (*xai.TokenResponse, error) {
	client, err := createGrokReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}

	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = xai.EffectiveClientID()
	}

	formData := url.Values{}
	formData.Set("grant_type", "refresh_token")
	formData.Set("client_id", clientID)
	formData.Set("refresh_token", refreshToken)

	var tokenResp xai.TokenResponse
	resp, err := client.R().
		SetContext(ctx).
		SetHeader("User-Agent", "sub2api-grok-oauth/1.0").
		SetFormDataFromValues(formData).
		SetSuccessResult(&tokenResp).
		Post(c.tokenURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_OAUTH_REQUEST_FAILED", "request failed: %v", err)
	}
	if !resp.IsSuccessState() {
		return nil, grokOAuthStatusError("GROK_OAUTH_TOKEN_REFRESH_FAILED", "token refresh failed", resp)
	}
	return &tokenResp, nil
}

func createGrokReqClient(proxyURL string) (*req.Client, error) {
	return getSharedReqClient(reqClientOptions{
		ProxyURL: proxyURL,
		Timeout:  60 * time.Second,
	})
}

func grokOAuthStatusError(code, message string, resp *req.Response) error {
	statusCode := http.StatusBadGateway
	errorCode := code
	upstreamStatus := 0
	if resp != nil && resp.StatusCode == http.StatusForbidden {
		statusCode = http.StatusForbidden
		errorCode = "GROK_OAUTH_ENTITLEMENT_DENIED"
	}
	body := ""
	if resp != nil {
		upstreamStatus = resp.StatusCode
		body = logredact.RedactText(resp.String())
	}
	return infraerrors.Newf(statusCode, errorCode, "%s: status %d, body: %s", message, upstreamStatus, body)
}
