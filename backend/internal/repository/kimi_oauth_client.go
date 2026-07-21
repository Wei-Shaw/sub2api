package repository

import (
	"context"
	"net/http"
	"net/url"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kimi"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/imroc/req/v3"
)

// kimiOAuthClient 实现 Kimi OAuth 设备码流程的 HTTP 调用。
// token 端点只认 application/x-www-form-urlencoded（SetFormDataFromValues），
// 所有请求携带 X-Msh-* 指纹头（对齐官方 kimi-cli 行为）。
type kimiOAuthClient struct {
	deviceAuthorizationURL string
	tokenURL               string
}

func NewKimiOAuthClient() service.KimiOAuthClient {
	return &kimiOAuthClient{
		deviceAuthorizationURL: kimi.EffectiveDeviceAuthorizationURL(),
		tokenURL:               kimi.EffectiveTokenURL(),
	}
}

func (c *kimiOAuthClient) DeviceAuthorization(ctx context.Context, proxyURL, deviceID string) (*kimi.DeviceAuthResponse, error) {
	client, err := createKimiReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}

	formData := url.Values{}
	formData.Set("client_id", kimi.EffectiveClientID())

	var deviceResp kimi.DeviceAuthResponse
	resp, err := client.R().
		SetContext(ctx).
		SetHeaders(kimi.FingerprintHeaders(deviceID)).
		SetFormDataFromValues(formData).
		SetSuccessResult(&deviceResp).
		Post(c.deviceAuthorizationURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_OAUTH_REQUEST_FAILED", "device authorization request failed: %v", err)
	}
	if !resp.IsSuccessState() {
		return nil, kimi.ParseOAuthError(resp.StatusCode, resp.Bytes())
	}
	return &deviceResp, nil
}

func (c *kimiOAuthClient) PollDeviceToken(ctx context.Context, deviceCode, proxyURL, deviceID string) (*kimi.TokenResponse, error) {
	client, err := createKimiReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}

	formData := url.Values{}
	formData.Set("client_id", kimi.EffectiveClientID())
	formData.Set("device_code", deviceCode)
	formData.Set("grant_type", kimi.DeviceCodeGrantType)

	var tokenResp kimi.TokenResponse
	resp, err := client.R().
		SetContext(ctx).
		SetHeaders(kimi.FingerprintHeaders(deviceID)).
		SetFormDataFromValues(formData).
		SetSuccessResult(&tokenResp).
		Post(c.tokenURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_OAUTH_REQUEST_FAILED", "device token poll request failed: %v", err)
	}
	if !resp.IsSuccessState() {
		return nil, kimi.ParseOAuthError(resp.StatusCode, resp.Bytes())
	}
	return &tokenResp, nil
}

func (c *kimiOAuthClient) RefreshToken(ctx context.Context, refreshToken, proxyURL, deviceID string) (*kimi.TokenResponse, error) {
	client, err := createKimiReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}

	formData := url.Values{}
	formData.Set("client_id", kimi.EffectiveClientID())
	formData.Set("grant_type", "refresh_token")
	formData.Set("refresh_token", refreshToken)

	var tokenResp kimi.TokenResponse
	resp, err := client.R().
		SetContext(ctx).
		SetHeaders(kimi.FingerprintHeaders(deviceID)).
		SetFormDataFromValues(formData).
		SetSuccessResult(&tokenResp).
		Post(c.tokenURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_OAUTH_REQUEST_FAILED", "token refresh request failed: %v", err)
	}
	if !resp.IsSuccessState() {
		return nil, kimi.ParseOAuthError(resp.StatusCode, resp.Bytes())
	}
	return &tokenResp, nil
}

func createKimiReqClient(proxyURL string) (*req.Client, error) {
	return getSharedReqClient(reqClientOptions{
		ProxyURL: proxyURL,
		Timeout:  60 * time.Second,
	})
}
