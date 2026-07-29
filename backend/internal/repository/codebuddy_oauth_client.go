package repository

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/codebuddy"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"github.com/imroc/req/v3"
)

type codebuddyOAuthClient struct{}

// NewCodeBuddyOAuthClient 创建 CodeBuddy OAuth HTTP 客户端。
func NewCodeBuddyOAuthClient() service.CodeBuddyOAuthClient {
	return &codebuddyOAuthClient{}
}

// codebuddyEnvelope 是 CodeBuddy 接口的统一响应信封。
type codebuddyEnvelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func (c *codebuddyOAuthClient) userAgent() string {
	return "WorkBuddy/5.2.5 WorkBuddy/5.2.5 CLI/2.106.4"
}

func (c *codebuddyOAuthClient) decodeEnvelope(resp *req.Response, action string) (json.RawMessage, error) {
	if !resp.IsSuccessState() {
		return nil, codebuddyStatusError("CODEBUDDY_REQUEST_FAILED", action+" failed", resp)
	}
	body := resp.Bytes()
	var env codebuddyEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CODEBUDDY_BAD_RESPONSE",
			"%s: invalid json: %v", action, err)
	}
	if env.Code != 0 {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CODEBUDDY_API_ERROR",
			"%s: code=%d msg=%s", action, env.Code, logredact.RedactText(env.Msg))
	}
	return env.Data, nil
}

func (c *codebuddyOAuthClient) FetchState(ctx context.Context, proxyURL string) (*codebuddy.StateResult, error) {
	client, err := createCodeBuddyReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CODEBUDDY_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}
	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("X-Domain", codebuddy.PlatformDomain).
		SetHeader("X-No-Authorization", "true").
		SetHeader("X-No-User-Id", "true").
		SetHeader("X-No-Enterprise-Id", "true").
		SetHeader("X-No-Department-Info", "true").
		SetHeader("X-Product", "SaaS").
		SetHeader("User-Agent", c.userAgent()).
		SetBody("{}").
		Post(codebuddy.BuildAuthStateURL(codebuddy.DefaultBaseURL) + "?platform=" + codebuddy.PlatformWorkBuddy)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CODEBUDDY_STATE_FAILED", "request failed: %v", err)
	}
	data, err := c.decodeEnvelope(resp, "fetch state")
	if err != nil {
		return nil, err
	}
	var result codebuddy.StateResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CODEBUDDY_BAD_STATE", "decode state: %v", err)
	}
	return &result, nil
}

func (c *codebuddyOAuthClient) FetchToken(ctx context.Context, state, proxyURL string) (*codebuddy.TokenResponse, error) {
	client, err := createCodeBuddyReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CODEBUDDY_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}
	resp, err := client.R().
		SetContext(ctx).
		SetHeader("X-No-Authorization", "true").
		SetHeader("X-No-User-Id", "true").
		SetHeader("X-No-Enterprise-Id", "true").
		SetHeader("X-No-Department-Info", "true").
		SetHeader("X-Product", "SaaS").
		SetHeader("User-Agent", c.userAgent()).
		Get(codebuddy.BuildAuthTokenURL(codebuddy.DefaultBaseURL, state))
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CODEBUDDY_TOKEN_FAILED", "request failed: %v", err)
	}
	data, err := c.decodeEnvelope(resp, "fetch token")
	if err != nil {
		return nil, err
	}
	var token codebuddy.TokenResponse
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CODEBUDDY_BAD_TOKEN", "decode token: %v", err)
	}
	return &token, nil
}

func (c *codebuddyOAuthClient) GetAccountInfo(ctx context.Context, accessToken, state, proxyURL string) (*codebuddy.AccountInfo, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CODEBUDDY_NO_ACCESS_TOKEN", "access_token is required")
	}
	client, err := createCodeBuddyReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CODEBUDDY_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}
	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Authorization", "Bearer "+accessToken).
		SetHeader("X-Domain", codebuddy.DefaultDomain).
		SetHeader("X-No-User-Id", "true").
		SetHeader("X-No-Enterprise-Id", "true").
		SetHeader("X-No-Department-Info", "true").
		SetHeader("User-Agent", c.userAgent()).
		Get(codebuddy.BuildLoginAccountURL(codebuddy.DefaultBaseURL, state))
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CODEBUDDY_ACCOUNT_FAILED", "request failed: %v", err)
	}
	data, err := c.decodeEnvelope(resp, "get account")
	if err != nil {
		return nil, err
	}
	var info codebuddy.AccountInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CODEBUDDY_BAD_ACCOUNT", "decode account: %v", err)
	}
	return &info, nil
}

func (c *codebuddyOAuthClient) GetConfig(ctx context.Context, accessToken, uid, proxyURL string) ([]byte, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CODEBUDDY_NO_ACCESS_TOKEN", "access_token is required")
	}
	client, err := createCodeBuddyReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CODEBUDDY_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}
	reqBuilder := client.R().
		SetContext(ctx).
		SetHeader("Authorization", "Bearer "+accessToken).
		SetHeader("X-Domain", codebuddy.DefaultDomain).
		SetHeader("X-Product", "SaaS").
		SetHeader("User-Agent", c.userAgent())
	if uid != "" {
		reqBuilder = reqBuilder.SetHeader("X-User-Id", uid)
	}
	resp, err := reqBuilder.Get(codebuddy.BuildConfigURL(codebuddy.DefaultBaseURL))
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CODEBUDDY_CONFIG_FAILED", "request failed: %v", err)
	}
	data, err := c.decodeEnvelope(resp, "get config")
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (c *codebuddyOAuthClient) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*codebuddy.TokenResponse, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CODEBUDDY_NO_REFRESH_TOKEN", "refresh_token is required")
	}
	client, err := createCodeBuddyReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CODEBUDDY_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}
	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("X-Domain", codebuddy.DefaultDomain).
		SetHeader("X-Refresh-Token", refreshToken).
		SetHeader("X-Auth-Refresh-Source", codebuddy.AuthSourcePlugin).
		SetHeader("Authorization", "Bearer "+refreshToken).
		SetHeader("User-Agent", c.userAgent()).
		SetBody("{}").
		Post(codebuddy.BuildAuthTokenRefreshURL(codebuddy.DefaultBaseURL))
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CODEBUDDY_REFRESH_FAILED", "request failed: %v", err)
	}
	data, err := c.decodeEnvelope(resp, "refresh token")
	if err != nil {
		return nil, err
	}
	var token codebuddy.TokenResponse
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CODEBUDDY_BAD_REFRESH", "decode refresh: %v", err)
	}
	return &token, nil
}

func createCodeBuddyReqClient(proxyURL string) (*req.Client, error) {
	return getSharedReqClient(reqClientOptions{
		ProxyURL: proxyURL,
		Timeout:  60 * time.Second,
	})
}

func codebuddyStatusError(code, message string, resp *req.Response) error {
	statusCode := http.StatusBadGateway
	upstreamStatus := 0
	if resp != nil {
		upstreamStatus = resp.StatusCode
	}
	body := ""
	if resp != nil {
		body = logredact.RedactText(resp.String())
	}
	return infraerrors.Newf(statusCode, code, "%s: status %d, body: %s", message, upstreamStatus, body)
}
