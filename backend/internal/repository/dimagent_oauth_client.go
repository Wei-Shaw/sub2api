package repository

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const dimAgentDefaultTokenEndpoint = "https://dimagent.cn/oauth/token"

type dimAgentOAuthClient struct{}

func NewDimAgentOAuthClient() service.DimAgentOAuthClient {
	return &dimAgentOAuthClient{}
}

func (c *dimAgentOAuthClient) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL string) (*service.DimAgentOAuthTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", service.DimAgentOAuthClientID)
	form.Set("code", strings.TrimSpace(code))
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", codeVerifier)
	return c.postToken(ctx, dimAgentDefaultTokenEndpoint, proxyURL, form, "DIMAGENT_OAUTH_TOKEN_EXCHANGE_FAILED")
}

func (c *dimAgentOAuthClient) RefreshToken(ctx context.Context, refreshToken, tokenEndpoint, proxyURL string) (*service.DimAgentOAuthTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", service.DimAgentOAuthClientID)
	form.Set("refresh_token", strings.TrimSpace(refreshToken))
	return c.postToken(ctx, dimAgentTokenEndpointOrDefault(tokenEndpoint), proxyURL, form, "DIMAGENT_OAUTH_TOKEN_REFRESH_FAILED")
}

func (c *dimAgentOAuthClient) postToken(ctx context.Context, endpoint, proxyURL string, form url.Values, reason string) (*service.DimAgentOAuthTokenResponse, error) {
	client, err := getSharedReqClient(reqClientOptions{ProxyURL: proxyURL, Timeout: 60 * time.Second})
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "DIMAGENT_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}
	var token service.DimAgentOAuthTokenResponse
	response, err := client.R().
		SetContext(ctx).
		SetHeader("X-Title", "DeepSeek Harness").
		SetHeader("HTTP-Referer", service.DimAgentOAuthIssuer+"/").
		SetFormDataFromValues(form).
		SetSuccessResult(&token).
		Post(endpoint)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "DIMAGENT_OAUTH_REQUEST_FAILED", "DimAgent OAuth request failed: %v", err)
	}
	if !response.IsSuccessState() {
		return nil, infraerrors.Newf(http.StatusBadGateway, reason, "DimAgent OAuth token request failed: status %d", response.StatusCode)
	}
	if strings.TrimSpace(token.AccessToken) == "" || !strings.EqualFold(strings.TrimSpace(token.TokenType), "bearer") {
		return nil, infraerrors.New(http.StatusBadGateway, "DIMAGENT_OAUTH_INVALID_TOKEN_RESPONSE", "DimAgent OAuth returned an invalid token response")
	}
	return &token, nil
}

func dimAgentTokenEndpointOrDefault(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, "dimagent.cn") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return dimAgentDefaultTokenEndpoint
	}
	if strings.TrimRight(parsed.Path, "/") != "/oauth/token" {
		return dimAgentDefaultTokenEndpoint
	}
	return strings.TrimRight(parsed.String(), "/")
}
