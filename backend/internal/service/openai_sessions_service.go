package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/imroc/req/v3"
)

// Confirmed from the ChatGPT account security page's network requests.
// Never use the global logout endpoint: it also revokes the current session.
const (
	openAISessionsURL      = "https://chatgpt.com/backend-api/accounts/sessions"
	openAISessionRevokeURL = openAISessionsURL + "/revoke"
	openAISessionsMaxBody  = 1 << 20
)

// OpenAIActiveSession exposes display metadata only, never upstream credentials.
type OpenAIActiveSession struct {
	SessionID    string   `json:"session_id"`
	Applications []string `json:"applications"`
	DeviceName   string   `json:"device_name"`
	Description  string   `json:"description,omitempty"`
	Platform     string   `json:"platform,omitempty"`
	OSVersion    string   `json:"os_version,omitempty"`
	City         string   `json:"city,omitempty"`
	Region       string   `json:"region,omitempty"`
	Country      string   `json:"country,omitempty"`
	LastSignedIn float64  `json:"last_signed_in,omitempty"`
	IsCurrent    bool     `json:"is_current"`
	CanRevoke    bool     `json:"can_revoke"`
}

type OpenAIActiveSessions struct {
	Sessions                 []OpenAIActiveSession `json:"sessions"`
	CurrentSessionIdentified bool                  `json:"current_session_identified"`
	FetchedAt                int64                 `json:"fetched_at"`
}

type openAISessionClient struct {
	client  *req.Client
	headers map[string]string
}

func (s *OpenAIQuotaService) sessionClient(ctx context.Context, accountID int64) (*openAISessionClient, error) {
	// Session/device metadata and revocation are not delegated by quota permissions.
	if adminaccess.RestrictedFromContext(ctx) != nil {
		return nil, infraerrors.New(http.StatusForbidden, "OPENAI_SESSIONS_FORBIDDEN", "当前管理密钥无权管理 ChatGPT 登录会话")
	}
	if s == nil || s.accountRepo == nil || s.tokenProvider == nil || s.privacyClientFactory == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "OPENAI_SESSIONS_NOT_CONFIGURED", "ChatGPT 活动会话服务暂不可用")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_SESSIONS_INVALID_ACCOUNT", "活动会话管理仅支持 OpenAI OAuth 账号")
	}
	if account.IsShadow() {
		return nil, infraerrors.New(http.StatusConflict, "OPENAI_SESSIONS_SHADOW_ACCOUNT", "请在母账号中管理 ChatGPT 活动会话")
	}
	if account.IsOpenAIAgentIdentity() {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_SESSIONS_UNSUPPORTED_AUTH", "当前账号授权方式不支持 ChatGPT 登录会话管理")
	}
	token, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil || strings.TrimSpace(token) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_SESSIONS_AUTH_ERROR", "无法获取账号登录凭据，请刷新凭据或重新授权后重试")
	}
	var proxyURL string
	if account.ProxyID != nil {
		proxy := account.Proxy
		if proxy == nil && s.proxyRepo != nil {
			proxy, err = s.proxyRepo.GetByID(ctx, *account.ProxyID)
			if err != nil {
				return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_SESSIONS_PROXY_ERROR", "无法读取账号代理，请检查代理配置")
			}
		}
		if proxy == nil {
			return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_SESSIONS_PROXY_ERROR", "账号代理不可用，请检查代理配置")
		}
		proxyURL = proxy.URL()
	}
	client, err := s.privacyClientFactory(proxyURL)
	if err != nil || client == nil {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_SESSIONS_CLIENT_ERROR", "无法连接 ChatGPT 会话服务，请检查账号代理")
	}
	client.SetRedirectPolicy(req.NoRedirectPolicy())
	headers := map[string]string{
		"Authorization":  "Bearer " + token,
		"Accept":         "application/json",
		"Origin":         "https://chatgpt.com",
		"Referer":        "https://chatgpt.com/",
		"sec-fetch-mode": "cors",
		"sec-fetch-site": "same-origin",
		"sec-fetch-dest": "empty",
	}
	if id := strings.TrimSpace(account.GetCredential("chatgpt_account_id")); id != "" {
		headers["ChatGPT-Account-ID"] = id
	}
	return &openAISessionClient{client: client, headers: headers}, nil
}

func (c *openAISessionClient) request(ctx context.Context, method, endpoint string, body any) ([]byte, error) {
	r := c.client.R().SetContext(ctx).SetHeaders(c.headers).SetRetryCount(0).DisableAutoReadResponse()
	if body != nil {
		r.SetBodyJsonMarshal(body)
	}
	resp, err := r.Send(method, endpoint)
	if resp != nil && resp.Response != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil || resp == nil || resp.Response == nil {
		if method == http.MethodPost {
			return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_SESSIONS_REVOKE_UNKNOWN", "退出请求结果未知，请刷新会话列表后再决定是否重试")
		}
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_SESSIONS_REQUEST_FAILED", "获取 ChatGPT 活动会话失败，请检查网络或账号代理")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, openAISessionsHTTPError(resp.StatusCode)
	}
	if resp.Body == nil {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_SESSIONS_INVALID_RESPONSE", "ChatGPT 会话服务返回了空响应，请刷新列表确认状态")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, openAISessionsMaxBody+1))
	if err != nil || len(data) > openAISessionsMaxBody {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_SESSIONS_INVALID_RESPONSE", "无法读取会话服务响应，请刷新列表确认最新状态")
	}
	return data, nil
}

func openAISessionsHTTPError(status int) error {
	// Do not return or log upstream bodies: they may contain credentials or PII.
	switch status {
	case http.StatusUnauthorized:
		return infraerrors.New(http.StatusBadGateway, "OPENAI_SESSIONS_AUTH_ERROR", "ChatGPT 会话认证失败，请刷新账号凭据或重新授权后重试")
	case http.StatusForbidden:
		return infraerrors.New(http.StatusBadGateway, "OPENAI_SESSIONS_ACCESS_DENIED", "ChatGPT 拒绝会话管理请求，当前授权可能不支持此功能，或账号代理被拦截")
	case http.StatusTooManyRequests:
		return infraerrors.New(http.StatusTooManyRequests, "OPENAI_SESSIONS_RATE_LIMITED", "会话操作过于频繁，请稍后重试")
	case http.StatusNotFound, http.StatusConflict, http.StatusBadRequest, http.StatusUnprocessableEntity:
		return infraerrors.New(http.StatusConflict, "OPENAI_SESSIONS_CHANGED", "会话状态已变更或请求被拒绝，请刷新会话列表后重试")
	default:
		return infraerrors.New(http.StatusBadGateway, "OPENAI_SESSIONS_UPSTREAM_ERROR", "ChatGPT 会话服务暂不可用，请稍后重试")
	}
}

func (c *openAISessionClient) list(ctx context.Context) (*OpenAIActiveSessions, error) {
	data, err := c.request(ctx, http.MethodGet, openAISessionsURL, nil)
	if err != nil {
		return nil, err
	}
	var payload struct {
		ShowSessionManager *bool `json:"show_session_manager"`
		Devices            *[]struct {
			SessionID    string  `json:"session_id"`
			DisplayName  string  `json:"display_name"`
			DeviceModel  string  `json:"device_model"`
			Description  string  `json:"human_readable_description"`
			Platform     string  `json:"platform"`
			OSVersion    string  `json:"os_version"`
			City         string  `json:"last_signed_in_city"`
			Region       string  `json:"last_signed_in_region_code"`
			Country      string  `json:"last_signed_in_country"`
			LastSignedIn float64 `json:"last_signed_in_timestamp_second"`
			IsCurrent    *bool   `json:"is_current_device"`
			AppSessions  []struct {
				ClientName string `json:"client_name"`
			} `json:"app_sessions"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.Devices == nil {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_SESSIONS_INVALID_RESPONSE", "ChatGPT 返回的会话列表格式无法识别")
	}
	if payload.ShowSessionManager != nil && !*payload.ShowSessionManager {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_SESSIONS_UNAVAILABLE", "此账号暂未开放 ChatGPT 活动会话管理")
	}
	result := &OpenAIActiveSessions{Sessions: make([]OpenAIActiveSession, 0, len(*payload.Devices)), FetchedAt: time.Now().Unix()}
	seen := make(map[string]bool)
	allCurrentMarkersPresent := true
	for _, device := range *payload.Devices {
		if !validOpenAISessionID(device.SessionID) || seen[device.SessionID] {
			return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_SESSIONS_INVALID_RESPONSE", "会话标识缺失或重复，已停止退出操作，请刷新列表")
		}
		seen[device.SessionID] = true
		current := device.IsCurrent != nil && *device.IsCurrent
		allCurrentMarkersPresent = allCurrentMarkersPresent && device.IsCurrent != nil
		result.CurrentSessionIdentified = result.CurrentSessionIdentified || current
		applications := make([]string, 0, len(device.AppSessions))
		for _, app := range device.AppSessions {
			if name := strings.TrimSpace(app.ClientName); name != "" {
				applications = append(applications, name)
			}
		}
		name := device.DisplayName
		if name == "" {
			name = device.DeviceModel
		}
		result.Sessions = append(result.Sessions, OpenAIActiveSession{
			SessionID: device.SessionID, Applications: applications,
			DeviceName: name, Description: device.Description, Platform: device.Platform, OSVersion: device.OSVersion,
			City: device.City, Region: device.Region, Country: device.Country,
			LastSignedIn: device.LastSignedIn, IsCurrent: current,
		})
	}
	result.CurrentSessionIdentified = result.CurrentSessionIdentified && allCurrentMarkersPresent
	for i := range result.Sessions {
		result.Sessions[i].CanRevoke = result.CurrentSessionIdentified && !result.Sessions[i].IsCurrent
	}
	return result, nil
}

func validOpenAISessionID(id string) bool {
	return id != "" && len(id) <= 256 && id == strings.TrimSpace(id) && !strings.ContainsAny(id, "\x00\r\n")
}

func (s *OpenAIQuotaService) ListActiveSessions(ctx context.Context, accountID int64) (*OpenAIActiveSessions, error) {
	ctx, cancel := context.WithTimeout(ctx, openaiQuotaUpstreamTimeout)
	defer cancel()
	client, err := s.sessionClient(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return client.list(ctx)
}

// RevokeActiveSession rechecks the list using the same authenticated client as
// the mutation. Neither a browser-supplied current flag nor a stale UI can
// authorize revoking the current session. Bulk logout calls this per session.
func (s *OpenAIQuotaService) RevokeActiveSession(ctx context.Context, accountID int64, sessionID string) error {
	if !validOpenAISessionID(sessionID) {
		return infraerrors.New(http.StatusBadRequest, "OPENAI_SESSIONS_INVALID_ID", "请选择有效的 ChatGPT 会话")
	}
	ctx, cancel := context.WithTimeout(ctx, openaiQuotaUpstreamTimeout)
	defer cancel()
	client, err := s.sessionClient(ctx, accountID)
	if err != nil {
		return err
	}
	list, err := client.list(ctx)
	if err != nil {
		return err
	}
	if !list.CurrentSessionIdentified {
		return infraerrors.New(http.StatusConflict, "OPENAI_SESSIONS_CURRENT_UNKNOWN", "无法可靠识别当前会话，已停止退出操作，请刷新列表或重新授权")
	}
	var target *OpenAIActiveSession
	for i := range list.Sessions {
		if list.Sessions[i].SessionID == sessionID {
			target = &list.Sessions[i]
			break
		}
	}
	if target == nil {
		return infraerrors.New(http.StatusConflict, "OPENAI_SESSIONS_CHANGED", "所选会话已不在活动列表中，请刷新列表")
	}
	if target.IsCurrent || !target.CanRevoke {
		return infraerrors.New(http.StatusConflict, "OPENAI_SESSIONS_CURRENT_PROTECTED", "当前会话已受保护，不能退出登录")
	}
	data, err := client.request(ctx, http.MethodPost, openAISessionRevokeURL, map[string]string{"session_id": sessionID})
	if err != nil {
		return err
	}
	// A successful revoke may be an empty response. Reject explicit failures and
	// non-JSON challenge pages even if upstream incorrectly labels them HTTP 200.
	if len(strings.TrimSpace(string(data))) > 0 {
		var result map[string]json.RawMessage
		if json.Unmarshal(data, &result) != nil || result == nil {
			return infraerrors.New(http.StatusBadGateway, "OPENAI_SESSIONS_REVOKE_UNKNOWN", "退出请求结果未知，请刷新会话列表确认")
		}
		if string(result["success"]) == "false" || (len(result["error"]) > 0 && string(result["error"]) != "null") {
			return infraerrors.New(http.StatusBadGateway, "OPENAI_SESSIONS_REVOKE_REJECTED", "ChatGPT 未确认退出成功，请刷新会话列表后重试")
		}
	}
	return nil
}
