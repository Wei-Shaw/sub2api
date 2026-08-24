package kiro

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/google/uuid"
)

const (
	socialAuthPortalURL = "https://app.kiro.dev"
	socialAuthEndpoint  = "https://prod.us-east-1.auth.desktop.kiro.dev"
	defaultIDCRegion    = "us-east-1"
	BuilderIDStartURL   = "https://view.awsapps.com/start"
	sessionTTL          = 10 * time.Minute
	sessionCleanupEvery = 32
	sessionCleanupMin   = 32
)

var allowedExternalIdpHostSuffixes = []string{
	".microsoftonline.com",
	".microsoftonline.us",
	".microsoftonline.cn",
}

var (
	socialAuthEndpointURL = socialAuthEndpoint
	oidcEndpointOverride  = ""
)

type SocialProvider string

const (
	SocialProviderGoogle SocialProvider = "Google"
	SocialProviderGitHub SocialProvider = "Github"
)

// Kiro 账号 provider 白名单。社交登录为 Google/Github;
// IDC 登录按 startURL 区分为 BuilderId(个人 Builder ID)/ Enterprise(企业自建 IAM Identity Center)。
const (
	ProviderGoogle      = "Google"
	ProviderGithub      = "Github"
	ProviderBuilderId   = "BuilderId"
	ProviderEnterprise  = "Enterprise"
	ProviderExternalIdp = "ExternalIdp"
)

// IsValidKiroProvider 校验 provider 是否在白名单内。
func IsValidKiroProvider(p string) bool {
	switch strings.TrimSpace(p) {
	case ProviderGoogle, ProviderGithub, ProviderBuilderId, ProviderEnterprise, ProviderExternalIdp:
		return true
	default:
		return false
	}
}

// resolveIDCProvider 按 startURL 推导 IDC 子类型:
// startURL 为空或等于默认 Builder ID start URL → BuilderId;其余视为企业自建 → Enterprise。
// 仅用于「有 startURL」的写入路径(IDC 登录),刷新/导入路径不得调用本函数推导。
func resolveIDCProvider(startURL string) string {
	if strings.TrimSpace(startURL) == "" || strings.TrimSpace(startURL) == BuilderIDStartURL {
		return ProviderBuilderId
	}
	return ProviderEnterprise
}

// normalizeKiroExpiresAt 把导入的 expiresAt 归一化为带本地时区偏移的 RFC3339。
// 兼容多种来源格式:带 Z(UTC)、带毫秒、带时区偏移、naive(无时区,按 UTC 处理)。
// 输出对齐 OAuth 登录流程(time.RFC3339 + 服务器本地时区)。
func normalizeKiroExpiresAt(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("expiresAt is empty")
	}
	// 优先尝试带时区的标准格式(含 Z 或 ±hh:mm 偏移)。
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999999", // naive,无时区
		"2006-01-02T15:04:05",
	}
	for i, layout := range layouts {
		var (
			t   time.Time
			err error
		)
		if i >= 2 {
			// naive 格式按 UTC 解析。
			t, err = time.ParseInLocation(layout, value, time.UTC)
		} else {
			t, err = time.Parse(layout, value)
		}
		if err == nil {
			return t.Local().Format(time.RFC3339), nil
		}
	}
	return "", fmt.Errorf("invalid expiresAt format: %q", raw)
}

type AuthSession struct {
	State         string
	CodeVerifier  string
	ProxyURL      string
	CreatedAt     time.Time
	AuthType      string
	Provider      string
	RedirectURI   string
	ClientID      string
	ClientSecret  string
	Region        string
	StartURL      string
	TokenEndpoint string
	IssuerURL     string
	Scopes        string
	LoginHint     string
}

type SessionStore struct {
	mu       sync.RWMutex
	data     map[string]*AuthSession
	setCount uint64
}

func NewSessionStore() *SessionStore {
	return &SessionStore{data: make(map[string]*AuthSession)}
}

func (s *SessionStore) Get(id string) (*AuthSession, bool) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.data[id]
	if ok && sessionExpired(session, now) {
		delete(s.data, id)
		return nil, false
	}
	return session, ok
}

func (s *SessionStore) Set(id string, session *AuthSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setCount++
	if len(s.data) >= sessionCleanupMin && s.setCount%sessionCleanupEvery == 0 {
		s.pruneExpiredLocked(time.Now())
	}
	s.data[id] = session
}

func (s *SessionStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, id)
}

func (s *SessionStore) pruneExpiredLocked(now time.Time) {
	for id, session := range s.data {
		if sessionExpired(session, now) {
			delete(s.data, id)
		}
	}
}

func sessionExpired(session *AuthSession, now time.Time) bool {
	if session == nil {
		return true
	}
	if session.CreatedAt.IsZero() {
		return true
	}
	return now.After(session.CreatedAt.Add(sessionTTL))
}

type TokenData struct {
	AccessToken   string `json:"accessToken"`
	RefreshToken  string `json:"refreshToken"`
	ProfileArn    string `json:"profileArn,omitempty"`
	ExpiresAt     string `json:"expiresAt,omitempty"`
	AuthMethod    string `json:"authMethod,omitempty"`
	Provider      string `json:"provider,omitempty"`
	ClientID      string `json:"clientId,omitempty"`
	ClientSecret  string `json:"clientSecret,omitempty"`
	ClientIDHash  string `json:"clientIdHash,omitempty"`
	Email         string `json:"email,omitempty"`
	StartURL      string `json:"startUrl,omitempty"`
	Region        string `json:"region,omitempty"`
	TokenEndpoint string `json:"tokenEndpoint,omitempty"`
	IssuerURL     string `json:"issuerUrl,omitempty"`
	Scopes        string `json:"scopes,omitempty"`
}

type socialTokenResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ProfileArn   string `json:"profileArn"`
	ExpiresIn    int    `json:"expiresIn"`
}

type registerClientResponse struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

type createTokenResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ProfileArn   string `json:"profileArn"`
	ExpiresIn    int    `json:"expiresIn"`
}

type externalIdpTokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type externalIdpDiscoveryResponse struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

type userInfoResponse struct {
	Email string `json:"email"`
}

type deviceRegistration struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

type RefreshTokenInvalidError struct {
	StatusCode int
	Body       string
}

func (e *RefreshTokenInvalidError) Error() string {
	if e == nil {
		return ""
	}
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return "kiro refresh token invalid (invalid_grant)"
	}
	return fmt.Sprintf("kiro refresh token invalid (invalid_grant, status %d): %s", e.StatusCode, body)
}

func GenerateSessionID() string {
	return uuid.NewString()
}

func GenerateState() (string, error) {
	return randomURLSafe(16)
}

func GenerateCodeVerifier() (string, error) {
	return randomURLSafe(32)
}

func randomURLSafe(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func GenerateCodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func BuildSocialSignInURL(redirectURI, codeChallenge, state string) string {
	params := url.Values{}
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	params.Set("redirect_uri", redirectURI)
	params.Set("redirect_from", "KiroIDE")
	return fmt.Sprintf("%s/signin?%s", socialAuthPortalURL, params.Encode())
}

func BuildSocialTokenRedirectURI(baseRedirectURI, callbackPath, loginOption string) string {
	redirectURI := strings.TrimRight(strings.TrimSpace(baseRedirectURI), "/")
	if redirectURI == "" {
		return ""
	}
	path := strings.TrimSpace(callbackPath)
	if path == "" {
		path = "/oauth/callback"
	} else if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	fullRedirectURI := redirectURI + path
	if option := strings.TrimSpace(loginOption); option != "" {
		return fullRedirectURI + "?login_option=" + url.QueryEscape(option)
	}
	return fullRedirectURI
}

func CreateSocialToken(ctx context.Context, proxyURL, code, codeVerifier, redirectURI string) (*TokenData, error) {
	payload := map[string]string{
		"code":          code,
		"code_verifier": codeVerifier,
		"redirect_uri":  redirectURI,
	}
	var resp socialTokenResponse
	if err := doJSON(ctx, proxyURL, http.MethodPost, socialAuthEndpointURL+"/oauth/token", payload, &resp, BuildLoginHeaders(shortSHA(codeVerifier), BuildMachineID("", "", "codeVerifier:"+codeVerifier))); err != nil {
		return nil, err
	}
	expiresIn := resp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	return &TokenData{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ProfileArn:   resp.ProfileArn,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second).Format(time.RFC3339),
		AuthMethod:   "social",
		Region:       defaultIDCRegion,
	}, nil
}

func RefreshSocialToken(ctx context.Context, proxyURL, refreshToken, provider string) (*TokenData, error) {
	payload := map[string]string{
		"refreshToken": refreshToken,
	}
	var resp socialTokenResponse
	accountKey := BuildAccountKey("", "", refreshToken, "", 0)
	if err := doJSON(ctx, proxyURL, http.MethodPost, socialAuthEndpointURL+"/refreshToken", payload, &resp, BuildLoginHeaders(accountKey, BuildMachineID(refreshToken, "", accountKey))); err != nil {
		return nil, err
	}
	expiresIn := resp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	return &TokenData{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ProfileArn:   resp.ProfileArn,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second).Format(time.RFC3339),
		AuthMethod:   "social",
		Provider:     provider,
		Region:       defaultIDCRegion,
	}, nil
}

func BuildExternalIdpAuthURL(authEndpoint, clientID, redirectURI, scopes, codeChallenge, state, loginHint string) string {
	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", scopes)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	params.Set("response_mode", "query")
	params.Set("state", state)
	if strings.TrimSpace(loginHint) != "" {
		params.Set("login_hint", strings.TrimSpace(loginHint))
	}
	return strings.TrimSpace(authEndpoint) + "?" + params.Encode()
}

func DiscoverExternalIdp(ctx context.Context, proxyURL, issuerURL string) (*externalIdpDiscoveryResponse, error) {
	issuer := strings.TrimRight(strings.TrimSpace(issuerURL), "/")
	if issuer == "" {
		return nil, fmt.Errorf("kiro external_idp issuer_url is required")
	}
	if err := validateExternalIdpEndpoint(issuer); err != nil {
		return nil, err
	}
	var resp externalIdpDiscoveryResponse
	if err := doJSON(ctx, proxyURL, http.MethodGet, issuer+"/.well-known/openid-configuration", nil, &resp, nil); err != nil {
		return nil, err
	}
	resp.AuthorizationEndpoint = strings.TrimSpace(resp.AuthorizationEndpoint)
	resp.TokenEndpoint = strings.TrimSpace(resp.TokenEndpoint)
	if resp.AuthorizationEndpoint == "" || resp.TokenEndpoint == "" {
		return nil, fmt.Errorf("external IdP discovery document missing authorization_endpoint or token_endpoint")
	}
	if err := validateExternalIdpEndpoint(resp.AuthorizationEndpoint); err != nil {
		return nil, err
	}
	if err := validateExternalIdpEndpoint(resp.TokenEndpoint); err != nil {
		return nil, err
	}
	return &resp, nil
}

func validateExternalIdpEndpoint(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("invalid external IdP URL %q: %w", rawURL, err)
	}
	if strings.ToLower(parsed.Scheme) != "https" {
		return fmt.Errorf("external IdP URL must use https: %q", rawURL)
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return fmt.Errorf("external IdP URL has no host: %q", rawURL)
	}
	if net.ParseIP(host) != nil {
		return fmt.Errorf("external IdP URL host must not be an IP literal: %q", rawURL)
	}
	for _, suffix := range allowedExternalIdpHostSuffixes {
		if strings.HasSuffix(host, suffix) {
			return nil
		}
	}
	return fmt.Errorf("external IdP host %q is not allow-listed", host)
}

func ExchangeExternalIdpAuthCode(ctx context.Context, proxyURL, tokenEndpoint, clientID, code, codeVerifier, redirectURI, scopes, issuerURL string) (*TokenData, error) {
	form := url.Values{}
	form.Set("client_id", strings.TrimSpace(clientID))
	form.Set("grant_type", "authorization_code")
	form.Set("code", strings.TrimSpace(code))
	form.Set("redirect_uri", strings.TrimSpace(redirectURI))
	form.Set("code_verifier", strings.TrimSpace(codeVerifier))
	if strings.TrimSpace(scopes) != "" {
		form.Set("scope", strings.TrimSpace(scopes))
	}
	var resp externalIdpTokenResponse
	if err := doForm(ctx, proxyURL, strings.TrimSpace(tokenEndpoint), form, &resp); err != nil {
		return nil, err
	}
	if strings.TrimSpace(resp.AccessToken) == "" {
		return nil, fmt.Errorf("external IdP token exchange returned empty access_token")
	}
	return buildExternalIdpTokenData(resp, strings.TrimSpace(resp.RefreshToken), clientID, tokenEndpoint, issuerURL, scopes), nil
}

func RefreshExternalIdpToken(ctx context.Context, proxyURL, clientID, refreshToken, tokenEndpoint, issuerURL, scopes string) (*TokenData, error) {
	form := url.Values{}
	form.Set("client_id", strings.TrimSpace(clientID))
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", strings.TrimSpace(refreshToken))
	if strings.TrimSpace(scopes) != "" {
		form.Set("scope", strings.TrimSpace(scopes))
	}
	var resp externalIdpTokenResponse
	if err := doForm(ctx, proxyURL, strings.TrimSpace(tokenEndpoint), form, &resp); err != nil {
		return nil, err
	}
	if strings.TrimSpace(resp.AccessToken) == "" {
		return nil, fmt.Errorf("external IdP refresh returned empty access_token")
	}
	return buildExternalIdpTokenData(resp, strings.TrimSpace(refreshToken), clientID, tokenEndpoint, issuerURL, scopes), nil
}

func buildExternalIdpTokenData(resp externalIdpTokenResponse, fallbackRefreshToken, clientID, tokenEndpoint, issuerURL, scopes string) *TokenData {
	expiresIn := resp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	refreshToken := strings.TrimSpace(resp.RefreshToken)
	if refreshToken == "" {
		refreshToken = strings.TrimSpace(fallbackRefreshToken)
	}
	return &TokenData{
		AccessToken:   strings.TrimSpace(resp.AccessToken),
		RefreshToken:  refreshToken,
		ExpiresAt:     time.Now().Add(time.Duration(expiresIn) * time.Second).Format(time.RFC3339),
		AuthMethod:    "external_idp",
		Provider:      ProviderExternalIdp,
		ClientID:      strings.TrimSpace(clientID),
		TokenEndpoint: strings.TrimSpace(tokenEndpoint),
		IssuerURL:     strings.TrimSpace(issuerURL),
		Scopes:        strings.TrimSpace(scopes),
		Region:        defaultIDCRegion,
	}
}

func RegisterIDCClient(ctx context.Context, proxyURL, redirectURI, issuerURL, region string) (*registerClientResponse, error) {
	if region == "" {
		region = defaultIDCRegion
	}
	payload := map[string]any{
		"clientName":   "Kiro IDE",
		"clientType":   "public",
		"scopes":       []string{"codewhisperer:completions", "codewhisperer:analysis", "codewhisperer:conversations", "codewhisperer:transformations", "codewhisperer:taskassist"},
		"grantTypes":   []string{"authorization_code", "refresh_token"},
		"redirectUris": []string{redirectURI},
		"issuerUrl":    issuerURL,
	}
	var resp registerClientResponse
	headers := oidcHeaders("", BuildMachineID("", "", "register-idc-client"))
	if err := doJSON(ctx, proxyURL, http.MethodPost, getOIDCEndpoint(region)+"/client/register", payload, &resp, headers); err != nil {
		return nil, err
	}
	return &resp, nil
}

func BuildIDCAuthURL(clientID, redirectURI, state, codeChallenge, region string) string {
	if region == "" {
		region = defaultIDCRegion
	}
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("scopes", strings.Join([]string{
		"codewhisperer:completions",
		"codewhisperer:analysis",
		"codewhisperer:conversations",
		"codewhisperer:transformations",
		"codewhisperer:taskassist",
	}, " "))
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	return fmt.Sprintf("%s/authorize?%s", getOIDCEndpoint(region), params.Encode())
}

func ExchangeIDCAuthCode(ctx context.Context, proxyURL, clientID, clientSecret, code, codeVerifier, redirectURI, region, startURL string) (*TokenData, error) {
	if region == "" {
		region = defaultIDCRegion
	}
	payload := map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"code":         code,
		"codeVerifier": codeVerifier,
		"redirectUri":  redirectURI,
		"grantType":    "authorization_code",
	}
	var resp createTokenResponse
	accountKey := BuildAccountKey(clientID, "", "", "", 0)
	headers := oidcHeaders(accountKey, BuildMachineID("", "", "clientID:"+clientID))
	if err := doJSON(ctx, proxyURL, http.MethodPost, getOIDCEndpoint(region)+"/token", payload, &resp, headers); err != nil {
		return nil, err
	}
	expiresIn := resp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	token := &TokenData{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ProfileArn:   resp.ProfileArn,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second).Format(time.RFC3339),
		AuthMethod:   "idc",
		Provider:     resolveIDCProvider(startURL),
		ClientID:     clientID,
		ClientSecret: clientSecret,
		StartURL:     startURL,
		Region:       region,
	}
	token.Email = FetchOIDCUserEmail(ctx, proxyURL, token.AccessToken, region)
	return token, nil
}

func RefreshIDCToken(ctx context.Context, proxyURL, clientID, clientSecret, refreshToken, region, startURL, provider string) (*TokenData, error) {
	if region == "" {
		region = defaultIDCRegion
	}
	payload := map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"refreshToken": refreshToken,
		"grantType":    "refresh_token",
	}
	var resp createTokenResponse
	accountKey := BuildAccountKey(clientID, "", refreshToken, "", 0)
	headers := oidcHeaders(accountKey, BuildMachineID(refreshToken, "", accountKey))
	if err := doJSON(ctx, proxyURL, http.MethodPost, getOIDCEndpoint(region)+"/token", payload, &resp, headers); err != nil {
		return nil, err
	}
	expiresIn := resp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	token := &TokenData{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ProfileArn:   resp.ProfileArn,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second).Format(time.RFC3339),
		AuthMethod:   "idc",
		// 刷新路径优先保留存量 provider(导入的 Enterprise 账号无 startURL,
		// 不得用 startURL 重新推导,否则会退化为 BuilderId)。仅当存量为空时才按 startURL 兜底。
		Provider:     strings.TrimSpace(provider),
		ClientID:     clientID,
		ClientSecret: clientSecret,
		StartURL:     startURL,
		Region:       region,
	}
	if token.Provider == "" {
		token.Provider = resolveIDCProvider(startURL)
	}
	token.Email = FetchOIDCUserEmail(ctx, proxyURL, token.AccessToken, region)
	return token, nil
}

func FetchOIDCUserEmail(ctx context.Context, proxyURL, accessToken, region string) string {
	if strings.TrimSpace(accessToken) == "" {
		return ""
	}
	var resp userInfoResponse
	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
	}
	if err := doJSON(ctx, proxyURL, http.MethodGet, getOIDCEndpoint(region)+"/userinfo", nil, &resp, headers); err != nil {
		return ""
	}
	return strings.TrimSpace(resp.Email)
}

func ParseImportedToken(tokenJSON string, deviceRegistrationJSON string) (*TokenData, error) {
	var token TokenData
	if err := json.Unmarshal([]byte(tokenJSON), &token); err != nil {
		return nil, fmt.Errorf("failed to parse kiro token: %w", err)
	}
	token.AuthMethod = strings.ToLower(strings.TrimSpace(token.AuthMethod))
	if strings.TrimSpace(token.AccessToken) == "" {
		return nil, fmt.Errorf("access token is empty")
	}
	if token.ClientIDHash != "" && (token.ClientID == "" || token.ClientSecret == "") && strings.TrimSpace(deviceRegistrationJSON) != "" {
		var reg deviceRegistration
		if err := json.Unmarshal([]byte(deviceRegistrationJSON), &reg); err != nil {
			return nil, fmt.Errorf("failed to parse device registration: %w", err)
		}
		if reg.ClientID != "" {
			token.ClientID = reg.ClientID
		}
		if reg.ClientSecret != "" {
			token.ClientSecret = reg.ClientSecret
		}
	}
	if token.AuthMethod == "" && strings.TrimSpace(token.ClientID) != "" && strings.TrimSpace(token.ClientSecret) != "" {
		token.AuthMethod = "idc"
	}
	// provider 严格校验:必须显式提供且属于白名单(Google/Github/BuilderId/Enterprise/ExternalIdp),
	// 空值或非法值一律拒绝,不再兜底为 AWS。
	token.Provider = strings.TrimSpace(token.Provider)
	if !IsValidKiroProvider(token.Provider) {
		return nil, fmt.Errorf("unsupported or missing kiro provider: %q (must be one of Google/Github/BuilderId/Enterprise/ExternalIdp)", token.Provider)
	}
	switch token.AuthMethod {
	case "idc":
		if strings.TrimSpace(token.Region) == "" {
			token.Region = defaultIDCRegion
		}
	case "external_idp":
		token.Provider = ProviderExternalIdp
		token.ClientID = strings.TrimSpace(token.ClientID)
		token.TokenEndpoint = strings.TrimSpace(token.TokenEndpoint)
		token.IssuerURL = strings.TrimSpace(token.IssuerURL)
		token.Scopes = strings.TrimSpace(token.Scopes)
		if strings.TrimSpace(token.RefreshToken) == "" || token.ClientID == "" || token.TokenEndpoint == "" {
			return nil, fmt.Errorf("kiro external_idp import requires refreshToken, clientId, and tokenEndpoint")
		}
		if strings.TrimSpace(token.Region) == "" {
			token.Region = defaultIDCRegion
		}
	}
	// expiresAt 归一化为带本地时区偏移的 RFC3339,对齐 OAuth 登录流程。
	if strings.TrimSpace(token.ExpiresAt) != "" {
		normalized, err := normalizeKiroExpiresAt(token.ExpiresAt)
		if err != nil {
			return nil, fmt.Errorf("failed to parse kiro token expiresAt: %w", err)
		}
		token.ExpiresAt = normalized
	}
	return &token, nil
}

func getOIDCEndpoint(region string) string {
	if strings.TrimSpace(oidcEndpointOverride) != "" {
		return strings.TrimRight(strings.TrimSpace(oidcEndpointOverride), "/")
	}
	if region == "" {
		region = defaultIDCRegion
	}
	return fmt.Sprintf("https://oidc.%s.amazonaws.com", region)
}

func oidcHeaders(accountKey, machineID string) map[string]string {
	headers := BuildOIDCHeaders(accountKey, machineID)
	if headers["amz-sdk-invocation-id"] == "" {
		headers["amz-sdk-invocation-id"] = uuid.NewString()
	}
	if headers["amz-sdk-request"] == "" {
		headers["amz-sdk-request"] = "attempt=1; max=4"
	}
	return headers
}

func doJSON(ctx context.Context, proxyURL, method, rawURL string, payload any, out any, extraHeaders map[string]string) error {
	client, err := newHTTPClient(proxyURL)
	if err != nil {
		return err
	}

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return err
	}

	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range extraHeaders {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyText := strings.TrimSpace(string(respBody))
		if resp.StatusCode == http.StatusBadRequest && strings.Contains(strings.ToLower(bodyText), "invalid_grant") {
			return &RefreshTokenInvalidError{StatusCode: resp.StatusCode, Body: bodyText}
		}
		return fmt.Errorf("upstream request failed (status %d): %s", resp.StatusCode, bodyText)
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	return json.Unmarshal(respBody, out)
}

func doForm(ctx context.Context, proxyURL, rawURL string, form url.Values, out any) error {
	client, err := newHTTPClient(proxyURL)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyText := strings.TrimSpace(string(respBody))
		if resp.StatusCode == http.StatusBadRequest && strings.Contains(strings.ToLower(bodyText), "invalid_grant") {
			return &RefreshTokenInvalidError{StatusCode: resp.StatusCode, Body: bodyText}
		}
		return fmt.Errorf("upstream request failed (status %d): %s", resp.StatusCode, bodyText)
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	return json.Unmarshal(respBody, out)
}

func newHTTPClient(rawProxyURL string) (*http.Client, error) {
	_, parsed, err := proxyurl.Parse(rawProxyURL)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{}
	if parsed != nil {
		transport.Proxy = http.ProxyURL(parsed)
	}
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}, nil
}
