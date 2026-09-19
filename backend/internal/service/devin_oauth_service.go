// devin_oauth_service.go 实现 Devin CLI 的 PKCE 登录链（手动粘贴授权码形态）：
// 生成 app.devin.ai/auth/cli/continue 授权 URL → 用户粘贴授权码 →
// ExchangePKCEAuthorizationCode / ExchangeDevinCLIPKCECode 换 Connect
// api key → GetUserStatus 拉取账号信息。
//
// 注意：不要用 POST api.devin.ai/auth/cli/token——它返回的是
// server.codeium.com 拒收的 web-session JWT（插件实测）。
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
)

// DevinOAuthService 处理 Devin 账号的 PKCE 授权流程。
type DevinOAuthService struct {
	sessionStore *devin.AuthSessionStore
	proxyRepo    ProxyRepository
	upstream     HTTPUpstream
}

// NewDevinOAuthService 创建 Devin OAuth 服务。
func NewDevinOAuthService(proxyRepo ProxyRepository, upstream HTTPUpstream) *DevinOAuthService {
	return &DevinOAuthService{
		sessionStore: devin.NewAuthSessionStore(),
		proxyRepo:    proxyRepo,
		upstream:     upstream,
	}
}

// Stop 停止会话清理协程（服务退出时调用）。
func (s *DevinOAuthService) Stop() {
	if s == nil {
		return
	}
	s.sessionStore.Stop()
}

// DevinAuthURLResult 是生成授权链接的结果。
type DevinAuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
	State     string `json:"state"`
}

// GenerateAuthURL 生成 PKCE 授权链接（手动粘贴形态，无回调）。
func (s *DevinOAuthService) GenerateAuthURL(ctx context.Context, proxyID *int64) (*DevinAuthURLResult, error) {
	pair := devin.GeneratePKCE()
	sessionID := devin.UUID()

	var proxyURL string
	if proxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
		}
	}

	s.sessionStore.Set(sessionID, &devin.AuthSession{
		State:     pair.State,
		Verifier:  pair.Verifier,
		ProxyURL:  proxyURL,
		CreatedAt: time.Now(),
	})

	return &DevinAuthURLResult{
		AuthURL:   devin.BuildAuthURL(pair.Challenge, pair.State),
		SessionID: sessionID,
		State:     pair.State,
	}, nil
}

// DevinExchangeCodeInput 是授权码交换的输入。
type DevinExchangeCodeInput struct {
	SessionID string
	State     string
	// Code 是用户粘贴的授权码；若粘贴值本身已是 api key
	// （devin-session-token$…/JWT/长 hex）则跳过交换直接使用。
	Code    string
	ProxyID *int64
}

// DevinTokenInfo 是交换/校验得到的账号凭据与身份信息。
type DevinTokenInfo struct {
	// AccessToken 是 Connect api key（devin-session-token$…）。
	AccessToken string `json:"access_token"`
	// APIServerURL 是上游返回的 api_server_url（空表示默认）。
	APIServerURL string `json:"api_server_url,omitempty"`

	// 以下字段来自 GetUserStatus；拉取失败时留空不影响建号。
	Name               string `json:"name,omitempty"`
	Email              string `json:"email,omitempty"`
	OrgID              string `json:"org_id,omitempty"`
	TeamID             string `json:"team_id,omitempty"`
	PlanName           string `json:"plan_name,omitempty"`
	AccountDisplayName string `json:"account_display_name,omitempty"`
	CanUseCLI          bool   `json:"can_use_cli"`
	IsEnterprise       bool   `json:"is_enterprise"`
}

// ExchangeCode 用粘贴的授权码交换 token 并拉取账号信息。
func (s *DevinOAuthService) ExchangeCode(ctx context.Context, input *DevinExchangeCodeInput) (*DevinTokenInfo, error) {
	code := strings.TrimSpace(input.Code)
	if code == "" {
		return nil, fmt.Errorf("code 不能为空")
	}
	// 粘贴值本身已是 api key（devin-session-token$…/JWT/长 hex）时跳过
	// 交换，也无需会话——token 不需要 verifier，直接可用。
	directToken := devin.LooksLikeAPIKey(code) && !strings.Contains(code, " ")

	var proxyURL string
	var session *devin.AuthSession
	if !directToken {
		var ok bool
		session, ok = s.sessionStore.Get(input.SessionID)
		if !ok {
			return nil, fmt.Errorf("session 不存在或已过期")
		}
		if strings.TrimSpace(input.State) == "" || input.State != session.State {
			return nil, fmt.Errorf("state 无效")
		}
		proxyURL = session.ProxyURL
	}
	if input.ProxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *input.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
		}
	}

	var creds devin.ExchangedCredentials
	if directToken {
		creds = devin.ExchangedCredentials{Token: devin.NormalizeToken(code)}
	} else {
		exchanged, err := s.exchangePKCE(ctx, code, session.Verifier, proxyURL)
		if err != nil {
			return nil, err
		}
		creds = exchanged
		s.sessionStore.Delete(input.SessionID)
	}

	info := &DevinTokenInfo{
		AccessToken:  creds.Token,
		APIServerURL: creds.APIServerURL,
	}

	// GetUserStatus 拉账号信息（email/org/plan）；失败不阻断建号。
	baseURL := strings.TrimSpace(creds.APIServerURL)
	if baseURL == "" {
		baseURL = devin.DefaultBaseURL
	}
	if status, err := s.getUserStatus(ctx, creds.Token, baseURL, proxyURL); err == nil && status != nil {
		info.Name = status.Name
		info.Email = status.Email
		info.OrgID = status.OrgID
		info.TeamID = status.TeamID
		info.PlanName = status.PlanName
		info.AccountDisplayName = status.AccountDisplayName
		info.CanUseCLI = status.CanUseCLI
		info.IsEnterprise = status.IsEnterprise
	}
	return info, nil
}

// exchangePKCE 依次尝试两个交换端点（与插件 resolvePastedToToken 一致）：
//  1. ExchangePKCEAuthorizationCode → api_key（credentials.toml 存的形态）
//  2. ExchangeDevinCLIPKCECode → session_token（兜底，JWT 归一加前缀）
func (s *DevinOAuthService) exchangePKCE(ctx context.Context, code, verifier, proxyURL string) (devin.ExchangedCredentials, error) {
	body := devin.MarshalExchangePKCE(code, verifier)

	// 1) ExchangePKCEAuthorizationCode → api_key
	if raw, err := s.doNoAuth(ctx, devin.PathExchangePKCEAuthCode, body, proxyURL); err == nil {
		if creds, decErr := devin.DecodeExchangeResponse(raw); decErr == nil && devin.LooksLikeAPIKey(creds.Token) {
			return creds, nil
		}
	}
	// 2) ExchangeDevinCLIPKCECode → session_token
	raw, err := s.doNoAuth(ctx, devin.PathExchangeDevinCLIPKCE, body, proxyURL)
	if err != nil {
		return devin.ExchangedCredentials{}, fmt.Errorf("token 交换失败: %s", connectErrText(err))
	}
	creds, err := devin.DecodeCLIPKCEResponse(raw)
	if err != nil {
		return devin.ExchangedCredentials{}, fmt.Errorf("token 交换失败: %s", connectErrText(err))
	}
	return creds, nil
}

// doNoAuth 发送无认证的 unary Connect 请求。
func (s *DevinOAuthService) doNoAuth(ctx context.Context, path string, body []byte, proxyURL string) ([]byte, error) {
	req, err := devin.NewUnaryRequestNoAuth(devin.DefaultBaseURL, path, body)
	if err != nil {
		return nil, err
	}
	resp, err := s.upstream.Do(req.WithContext(ctx), proxyURL, 0, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return devin.ReadUnaryResponse(resp)
}

// getUserStatus 拉取账号信息（SeatManagementService/GetUserStatus，Basic 认证）。
func (s *DevinOAuthService) getUserStatus(ctx context.Context, token, baseURL, proxyURL string) (*devin.UserStatus, error) {
	body := devin.MarshalGetUserStatus(token, devin.DefaultClientVersion, devin.ClientOS())
	req, err := devin.NewUnaryRequest(baseURL, devin.PathGetUserStatus, token, body)
	if err != nil {
		return nil, err
	}
	resp, err := s.upstream.Do(req.WithContext(ctx), proxyURL, 0, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := devin.ReadUnaryResponse(resp)
	if err != nil {
		return nil, err
	}
	return devin.DecodeUserStatus(data), nil
}

// connectErrText 提取 ConnectError 的 code: message 文案。
func connectErrText(err error) string {
	var connectErr *devin.ConnectError
	if err != nil && errors.As(err, &connectErr) {
		if connectErr.Message != "" {
			return connectErr.Code + ": " + connectErr.Message
		}
		return connectErr.Error()
	}
	if err == nil {
		return ""
	}
	return err.Error()
}
