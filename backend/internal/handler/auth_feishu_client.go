package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// feishuClientConfig 是 FeishuClient 需要的最小配置子集。
type feishuClientConfig struct {
	AppID       string
	AppSecret   string
	TokenURL    string
	UserInfoURL string
}

// FeishuClient 封装飞书 OAuth2 的两次上游调用：v2 token 交换 + v1 user_info。
// 相比钉钉，飞书用户信息一次即可拿全（union_id/open_id/name/email/tenant_key），
// 无需 app_access_token 与多步链，因此 client 无需缓存任何令牌。
type FeishuClient struct {
	cfg        feishuClientConfig
	httpClient *http.Client
}

// FeishuAPIError 表示飞书上游返回的业务错误（code != 0 或 OAuth error）。
type FeishuAPIError struct {
	Code    int
	Message string
	HTTP    int
}

func (e *FeishuAPIError) Error() string {
	return fmt.Sprintf("feishu api error code=%d msg=%s http=%d", e.Code, e.Message, e.HTTP)
}

func parseFeishuErr(raw []byte, status int) error {
	var v struct {
		Code             int    `json:"code"`
		Msg              string `json:"msg"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	_ = json.Unmarshal(raw, &v)
	msg := v.Msg
	if strings.TrimSpace(msg) == "" {
		msg = strings.TrimSpace(v.Error)
	}
	if desc := strings.TrimSpace(v.ErrorDescription); desc != "" {
		if msg != "" {
			msg = msg + ": " + desc
		} else {
			msg = desc
		}
	}
	return &FeishuAPIError{Code: v.Code, Message: msg, HTTP: status}
}

// FeishuUserTokenResp 是 v2 token 端点的成功响应子集。
type FeishuUserTokenResp struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	TokenType    string
}

// ExchangeCodeForUserToken 用授权码换取 user_access_token。
// 飞书 v2 端点要求 application/json body（这也是无法直接复用通用 OIDC form 提交的原因）。
func (c *FeishuClient) ExchangeCodeForUserToken(ctx context.Context, code, redirectURI string) (*FeishuUserTokenResp, error) {
	body := map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     c.cfg.AppID,
		"client_secret": c.cfg.AppSecret,
		"code":          code,
		"redirect_uri":  redirectURI,
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.TokenURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)

	var v struct {
		Code         int    `json:"code"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, parseFeishuErr(raw, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK || v.Code != 0 || strings.TrimSpace(v.AccessToken) == "" {
		return nil, parseFeishuErr(raw, resp.StatusCode)
	}
	tokenType := v.TokenType
	if strings.TrimSpace(tokenType) == "" {
		tokenType = "Bearer"
	}
	return &FeishuUserTokenResp{
		AccessToken:  v.AccessToken,
		RefreshToken: v.RefreshToken,
		ExpiresIn:    v.ExpiresIn,
		TokenType:    tokenType,
	}, nil
}

// FeishuUserInfo 是 /authen/v1/user_info 返回的字段子集。
type FeishuUserInfo struct {
	OpenID          string
	UnionID         string
	Name            string
	Email           string
	EnterpriseEmail string
	AvatarURL       string
	TenantKey       string
}

// ResolvedEmail 返回可用于建号的邮箱：优先企业邮箱，回退个人邮箱。
func (u *FeishuUserInfo) ResolvedEmail() string {
	if e := strings.TrimSpace(u.EnterpriseEmail); e != "" {
		return e
	}
	return strings.TrimSpace(u.Email)
}

// GetUserInfo 用 user_access_token 拉取用户信息（Bearer 鉴权）。
func (c *FeishuClient) GetUserInfo(ctx context.Context, userToken string) (*FeishuUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.UserInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+userToken)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, parseFeishuErr(raw, resp.StatusCode)
	}
	var v struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OpenID          string `json:"open_id"`
			UnionID         string `json:"union_id"`
			Name            string `json:"name"`
			Email           string `json:"email"`
			EnterpriseEmail string `json:"enterprise_email"`
			AvatarURL       string `json:"avatar_url"`
			TenantKey       string `json:"tenant_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, parseFeishuErr(raw, resp.StatusCode)
	}
	if v.Code != 0 || strings.TrimSpace(v.Data.UnionID) == "" {
		return nil, parseFeishuErr(raw, resp.StatusCode)
	}
	return &FeishuUserInfo{
		OpenID:          v.Data.OpenID,
		UnionID:         v.Data.UnionID,
		Name:            v.Data.Name,
		Email:           v.Data.Email,
		EnterpriseEmail: v.Data.EnterpriseEmail,
		AvatarURL:       v.Data.AvatarURL,
		TenantKey:       v.Data.TenantKey,
	}, nil
}
