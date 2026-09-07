package kiro

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// BuilderIDStartURL 是 AWS Builder ID（个人账号）的 SSO 门户地址。
const BuilderIDStartURL = "https://view.awsapps.com/start"

// deviceCodeGrant 是设备码授权的 grant type。
const deviceCodeGrant = "urn:ietf:params:oauth:grant-type:device_code"

// DefaultScopes 是 Kiro 需要的 CodeWhisperer 权限范围。
var DefaultScopes = []string{
	"codewhisperer:completions",
	"codewhisperer:analysis",
	"codewhisperer:conversations",
	"codewhisperer:transformations",
	"codewhisperer:taskassist",
}

// 设备码轮询的非终态错误。调用方据此决定继续等待、放慢频率还是中止。
var (
	ErrAuthorizationPending = errors.New("kiro: authorization pending")
	ErrSlowDown             = errors.New("kiro: slow down polling")
	ErrDeviceCodeExpired    = errors.New("kiro: device code expired")
)

// AuthMethod 是账号的凭证接入方式，存于 credentials["auth_method"]。
type AuthMethod string

const (
	// AuthSocial 走 Kiro 自家认证服务刷新，只需 refreshToken。
	AuthSocial AuthMethod = "social"
	// AuthBuilderID 是个人 AWS Builder ID，初始授权走设备码。
	AuthBuilderID AuthMethod = "builder_id"
	// AuthIdC 是企业 IAM Identity Center，初始授权走 PKCE 授权码。
	AuthIdC AuthMethod = "idc"
	// AuthAPIKey 不刷新，直接用 API Key 作 Bearer。
	AuthAPIKey AuthMethod = "api_key"
)

// ParseAuthMethod 解析 credentials 里的值。未知值退回 social ——
// 历史账号多数是以 social 形态导入的。
func ParseAuthMethod(s string) AuthMethod {
	switch AuthMethod(strings.ToLower(strings.TrimSpace(s))) {
	case AuthBuilderID:
		return AuthBuilderID
	case AuthIdC:
		return AuthIdC
	case AuthAPIKey:
		return AuthAPIKey
	default:
		return AuthSocial
	}
}

// UsesOIDCRefresh 返回该方式是否走 AWS SSO OIDC 的 /token 刷新。
func (m AuthMethod) UsesOIDCRefresh() bool {
	return m == AuthBuilderID || m == AuthIdC
}

// TokenSet 是一次刷新或授权换取到的凭证。
type TokenSet struct {
	AccessToken  string
	RefreshToken string
	// ProfileArn 必须回写到账号 credentials —— 漏写会导致一段时间后 403。
	ProfileArn string
	ExpiresAt  time.Time
}

// OIDCBase 返回 AWS SSO OIDC 的基地址。
func OIDCBase(region string) string {
	if region == "" {
		region = defaultRegion
	}
	return fmt.Sprintf("https://oidc.%s.amazonaws.com", region)
}

// SocialBase 返回 Kiro 自家认证服务的基地址。
func SocialBase(region string) string {
	if region == "" {
		region = defaultRegion
	}
	return fmt.Sprintf("https://prod.%s.auth.desktop.kiro.dev", region)
}

// tokenResponse 是三个 token 端点的公共响应形态。
type tokenResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int    `json:"expiresIn"`
	ProfileArn   string `json:"profileArn"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
	Message      string `json:"message"`
}

func (r *tokenResponse) toTokenSet(fallbackRefresh string) *TokenSet {
	ts := &TokenSet{
		AccessToken:  r.AccessToken,
		RefreshToken: r.RefreshToken,
		ProfileArn:   r.ProfileArn,
	}
	if ts.RefreshToken == "" {
		ts.RefreshToken = fallbackRefresh
	}
	if r.ExpiresIn > 0 {
		ts.ExpiresAt = time.Now().Add(time.Duration(r.ExpiresIn) * time.Second)
	}
	return ts
}

// postJSON 发送 JSON 请求并解析响应，非 2xx 时把响应体带进错误。
func postJSON(ctx context.Context, hc *http.Client, endpoint string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	// 限制读取量，防止畸形响应撑爆内存。
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("kiro: %s returned %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("kiro: decode response from %s: %w", endpoint, err)
	}
	return nil
}

// RefreshSocial 走 Kiro 自家认证服务刷新，请求体只有 refreshToken。
func RefreshSocial(ctx context.Context, hc *http.Client, base, refreshToken string) (*TokenSet, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, errors.New("kiro: social refresh requires refreshToken")
	}

	var out tokenResponse
	if err := postJSON(ctx, hc, base+"/refreshToken",
		map[string]string{"refreshToken": refreshToken}, &out); err != nil {
		return nil, err
	}
	return out.toTokenSet(refreshToken), nil
}

// RefreshOIDC 走 AWS SSO OIDC 刷新，需要注册时拿到的 clientId/clientSecret。
func RefreshOIDC(ctx context.Context, hc *http.Client, base, clientID, clientSecret, refreshToken string) (*TokenSet, error) {
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(clientSecret) == "" {
		return nil, errors.New("kiro: OIDC refresh requires clientId and clientSecret")
	}
	if strings.TrimSpace(refreshToken) == "" {
		return nil, errors.New("kiro: OIDC refresh requires refreshToken")
	}

	var out tokenResponse
	payload := map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"refreshToken": refreshToken,
		"grantType":    "refresh_token",
	}
	if err := postJSON(ctx, hc, base+"/token", payload, &out); err != nil {
		return nil, err
	}
	return out.toTokenSet(refreshToken), nil
}

// ClientRegistration 是动态注册得到的客户端凭据，长期有效，需持久化。
type ClientRegistration struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

// RegisterOIDCClient 动态注册一个 OIDC 客户端。
//
// deviceFlow 为 true 时申请设备码授权（Builder ID 路径），
// 否则申请授权码 + PKCE（IdC 路径，需提供 redirectURI）。
func RegisterOIDCClient(ctx context.Context, hc *http.Client, base, issuerURL, redirectURI string, deviceFlow bool) (*ClientRegistration, error) {
	grantTypes := []string{"authorization_code", "refresh_token"}
	if deviceFlow {
		grantTypes = []string{deviceCodeGrant, "refresh_token"}
	}

	payload := map[string]any{
		"clientName": "Kiro",
		"clientType": "public",
		"scopes":     DefaultScopes,
		"grantTypes": grantTypes,
		"issuerUrl":  issuerURL,
	}
	if redirectURI != "" {
		payload["redirectUris"] = []string{redirectURI}
	}

	var out ClientRegistration
	if err := postJSON(ctx, hc, base+"/client/register", payload, &out); err != nil {
		return nil, err
	}
	if out.ClientID == "" || out.ClientSecret == "" {
		return nil, errors.New("kiro: client registration returned empty credentials")
	}
	return &out, nil
}

// PKCE 是一对 code_verifier / code_challenge。
type PKCE struct {
	Verifier  string
	Challenge string
}

// NewPKCE 生成一对 PKCE 参数（S256）。
func NewPKCE() (*PKCE, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("kiro: generate PKCE verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(verifier))
	return &PKCE{
		Verifier:  verifier,
		Challenge: base64.RawURLEncoding.EncodeToString(sum[:]),
	}, nil
}

// BuildAuthorizeURL 拼出 IdC 的授权跳转地址。
// 管理员在浏览器打开它，用组织的用户名/密码在 AWS 门户登录后跳回 redirectURI。
func BuildAuthorizeURL(base, clientID, redirectURI, state, challenge string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scopes", strings.Join(DefaultScopes, ","))
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	return base + "/authorize?" + q.Encode()
}

// ExchangeAuthorizationCode 用回调拿到的 code 换取 token。
func ExchangeAuthorizationCode(ctx context.Context, hc *http.Client, base, clientID, clientSecret, code, verifier, redirectURI string) (*TokenSet, error) {
	payload := map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"grantType":    "authorization_code",
		"code":         code,
		"codeVerifier": verifier,
		"redirectUri":  redirectURI,
	}

	var out tokenResponse
	if err := postJSON(ctx, hc, base+"/token", payload, &out); err != nil {
		return nil, err
	}
	if out.AccessToken == "" {
		return nil, errors.New("kiro: authorization code exchange returned no access token")
	}
	return out.toTokenSet(""), nil
}

// DeviceAuth 是设备码授权的第一步结果。
type DeviceAuth struct {
	DeviceCode              string `json:"deviceCode"`
	UserCode                string `json:"userCode"`
	VerificationURI         string `json:"verificationUri"`
	VerificationURIComplete string `json:"verificationUriComplete"`
	ExpiresIn               int    `json:"expiresIn"`
	Interval                int    `json:"interval"`
}

// StartDeviceAuthorization 发起设备码授权（Builder ID 路径）。
func StartDeviceAuthorization(ctx context.Context, hc *http.Client, base, clientID, clientSecret, startURL string) (*DeviceAuth, error) {
	if startURL == "" {
		startURL = BuilderIDStartURL
	}

	payload := map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"startUrl":     startURL,
	}

	var out DeviceAuth
	if err := postJSON(ctx, hc, base+"/device_authorization", payload, &out); err != nil {
		return nil, err
	}
	if out.DeviceCode == "" {
		return nil, errors.New("kiro: device authorization returned no device code")
	}
	if out.Interval <= 0 {
		out.Interval = 5
	}
	return &out, nil
}

// PollDeviceToken 轮询设备码换取 token。
//
// 返回 ErrAuthorizationPending 表示用户尚未完成授权，应按 Interval 继续轮询；
// ErrSlowDown 表示应放慢频率；ErrDeviceCodeExpired 表示应中止并让用户重新发起。
func PollDeviceToken(ctx context.Context, hc *http.Client, base, clientID, clientSecret, deviceCode string) (*TokenSet, error) {
	payload := map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"grantType":    deviceCodeGrant,
		"deviceCode":   deviceCode,
	}

	var out tokenResponse
	err := postJSON(ctx, hc, base+"/token", payload, &out)
	if err != nil {
		// 非终态错误以 error code 形式出现在 4xx 响应体里。
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "authorization_pending"):
			return nil, ErrAuthorizationPending
		case strings.Contains(msg, "slow_down"):
			return nil, ErrSlowDown
		case strings.Contains(msg, "expired_token"), strings.Contains(msg, "expired"):
			return nil, ErrDeviceCodeExpired
		}
		return nil, err
	}

	if out.AccessToken == "" {
		return nil, ErrAuthorizationPending
	}
	return out.toTokenSet(""), nil
}
