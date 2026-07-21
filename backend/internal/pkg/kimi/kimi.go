// Package kimi 提供 Kimi（kimi.com 订阅）OAuth 设备码流程（RFC 8628）与
// API 网关共享的常量、会话存储、指纹头构造与 URL 校验工具。
//
// 协议要点（与 Grok 的 PKCE 授权码流程不同）：
//   - 无 authorization URL / redirect_uri / PKCE / scope 参数；
//   - POST /api/oauth/device_authorization 拿 device_code + user_code；
//   - 用户到 verification_uri 确认后，POST /api/oauth/token 轮询换 token；
//   - token 端点只认 application/x-www-form-urlencoded（JSON 会 400）；
//   - access_token 约 15 分钟有效，refresh_token 每次刷新轮换，必须原子写回。
package kimi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

const (
	// DefaultOAuthHost 是 Kimi OAuth 服务地址（可用环境变量覆盖，见下方 Env 常量）。
	DefaultOAuthHost = "https://auth.kimi.com"
	// DefaultDeviceAuthorizationURL 设备授权端点（RFC 8628 device_authorization endpoint）。
	DefaultDeviceAuthorizationURL = DefaultOAuthHost + "/api/oauth/device_authorization"
	// DefaultTokenURL token 端点（轮询换 token 与 refresh 共用）。
	DefaultTokenURL = DefaultOAuthHost + "/api/oauth/token"
	// DefaultBaseURL Kimi Coding API 基础地址（OpenAI 兼容）。
	DefaultBaseURL = "https://api.kimi.com/coding/v1"
	// DefaultClientID 官方 kimi-cli 的 public client_id（无 secret，无需注册）。
	DefaultClientID = "17e5f671-d194-4dfb-9706-5516cb48c098"
	// DeviceCodeGrantType 设备码授权的 grant_type（RFC 8628）。
	DeviceCodeGrantType = "urn:ietf:params:oauth:grant-type:device_code"

	// DefaultPollIntervalSeconds 默认轮询间隔（秒），上游未返回 interval 时使用。
	DefaultPollIntervalSeconds = 5
	// DefaultSessionTTL 设备码会话兜底 TTL（上游 expires_in 通常为 1800 秒）。
	DefaultSessionTTL = 30 * time.Minute
	// SlowDownExtraSeconds 收到 slow_down 后建议前端追加的轮询间隔（秒）。
	SlowDownExtraSeconds = 5

	// CLIVersion 是 Kimi Coding 套餐接口当前接受的兼容指纹版本。
	// User-Agent 与 X-Msh-Version 必须保持一致；升级前需用真实套餐账号做端到端验证。
	CLIVersion = "1.3"
	// UserAgent API 请求必须携带的 UA（前缀必须恰好是 "KimiCLI/"，否则 403/429）。
	UserAgent = "KimiCLI/" + CLIVersion
	// MshPlatform X-Msh-Platform 固定值（官方 CLI 为 kimi_cli）。
	MshPlatform = "kimi_cli"

	EnvOAuthHost               = "KIMI_OAUTH_HOST"
	EnvCodeOAuthHost           = "KIMI_CODE_OAUTH_HOST" // 兼容 kimi-cli 的变量名
	EnvDeviceAuthorizationURL  = "KIMI_OAUTH_DEVICE_AUTHORIZATION_URL"
	EnvTokenURL                = "KIMI_OAUTH_TOKEN_URL"
	EnvClientID                = "KIMI_OAUTH_CLIENT_ID"
	EnvBaseURL                 = "KIMI_BASE_URL"
	EnvAllowUnsafeURLOverrides = "KIMI_ALLOW_UNSAFE_URL_OVERRIDES"
	EnvDeviceName              = "KIMI_DEVICE_NAME"
	EnvDeviceModel             = "KIMI_DEVICE_MODEL"
	EnvOSVersion               = "KIMI_OS_VERSION"
)

var (
	oauthEndpointAllowedHosts = []string{"kimi.com", "*.kimi.com"}
	baseURLAllowedHosts       = []string{"api.kimi.com"}
)

// DeviceAuthResponse 是 POST /api/oauth/device_authorization 的成功响应。
type DeviceAuthResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int64  `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// TokenResponse 是 POST /api/oauth/token 的成功响应（轮询与 refresh 同构）。
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// OAuth 错误码（RFC 8628 / RFC 6749）。
const (
	OAuthErrorAuthorizationPending = "authorization_pending"
	OAuthErrorSlowDown             = "slow_down"
	OAuthErrorExpiredToken         = "expired_token"
	OAuthErrorAccessDenied         = "access_denied"
	OAuthErrorInvalidGrant         = "invalid_grant"
)

// OAuthError 是 Kimi OAuth 端点返回的结构化错误（{"error","error_description"}）。
type OAuthError struct {
	StatusCode  int    `json:"-"`
	Code        string `json:"error"`
	Description string `json:"error_description,omitempty"`
}

func (e *OAuthError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Description != "" {
		return fmt.Sprintf("kimi oauth error %q (status %d): %s", e.Code, e.StatusCode, e.Description)
	}
	return fmt.Sprintf("kimi oauth error %q (status %d)", e.Code, e.StatusCode)
}

func (e *OAuthError) IsPending() bool      { return e != nil && e.Code == OAuthErrorAuthorizationPending }
func (e *OAuthError) IsSlowDown() bool     { return e != nil && e.Code == OAuthErrorSlowDown }
func (e *OAuthError) IsExpiredToken() bool { return e != nil && e.Code == OAuthErrorExpiredToken }
func (e *OAuthError) IsAccessDenied() bool { return e != nil && e.Code == OAuthErrorAccessDenied }
func (e *OAuthError) IsInvalidGrant() bool { return e != nil && e.Code == OAuthErrorInvalidGrant }

// IsUnauthorized 判断刷新场景下的凭据作废（401/403 或 invalid_grant），需要用户重新登录。
func (e *OAuthError) IsUnauthorized() bool {
	if e == nil {
		return false
	}
	return e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden || e.IsInvalidGrant()
}

// ParseOAuthError 从非 2xx 响应体解析 OAuth 错误；解析不到 error 字段时返回 nil。
func ParseOAuthError(statusCode int, body []byte) *OAuthError {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return &OAuthError{StatusCode: statusCode}
	}
	var parsed OAuthError
	if err := json.Unmarshal(body, &parsed); err != nil || strings.TrimSpace(parsed.Code) == "" {
		return &OAuthError{StatusCode: statusCode}
	}
	parsed.StatusCode = statusCode
	return &parsed
}

// DeviceSession 存储一次设备码登录流程的会话状态。
type DeviceSession struct {
	DeviceCode              string    `json:"device_code"`
	UserCode                string    `json:"user_code"`
	VerificationURI         string    `json:"verification_uri"`
	VerificationURIComplete string    `json:"verification_uri_complete"`
	IntervalSeconds         int       `json:"interval_seconds"`
	ClientID                string    `json:"client_id,omitempty"`
	ProxyURL                string    `json:"proxy_url,omitempty"`
	DeviceID                string    `json:"device_id"`
	CreatedAt               time.Time `json:"created_at"`
	ExpiresAt               time.Time `json:"expires_at"`

	// 轮询成功后暂存 token，供 create-from-oauth 一步建号读取。
	Token     *TokenResponse `json:"token,omitempty"`
	Completed bool           `json:"completed"`
}

// SessionStore 管理 Kimi 设备码登录会话（内存态，TTL 取设备码 expires_in）。
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*DeviceSession
	stopOnce sync.Once
	stopCh   chan struct{}
}

func NewSessionStore() *SessionStore {
	store := &SessionStore{
		sessions: make(map[string]*DeviceSession),
		stopCh:   make(chan struct{}),
	}
	go store.cleanup()
	return store
}

func (s *SessionStore) Set(sessionID string, session *DeviceSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = session
}

func (s *SessionStore) Get(sessionID string) (*DeviceSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, false
	}
	if !session.ExpiresAt.IsZero() && time.Now().After(session.ExpiresAt) {
		return nil, false
	}
	return session, true
}

func (s *SessionStore) Delete(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

func (s *SessionStore) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

func (s *SessionStore) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.Lock()
			for id, session := range s.sessions {
				if !session.ExpiresAt.IsZero() && time.Now().After(session.ExpiresAt) {
					delete(s.sessions, id)
				}
			}
			s.mu.Unlock()
		}
	}
}

// SessionTTLFromExpiresIn 把上游 expires_in（秒）换算为会话 TTL，非法值回退默认 TTL。
func SessionTTLFromExpiresIn(expiresIn int64) time.Duration {
	if expiresIn <= 0 {
		return DefaultSessionTTL
	}
	return time.Duration(expiresIn) * time.Second
}

// EffectiveOAuthHost 返回 OAuth host（KIMI_CODE_OAUTH_HOST 优先，其次 KIMI_OAUTH_HOST）。
func EffectiveOAuthHost() string {
	if value := strings.TrimSpace(os.Getenv(EnvCodeOAuthHost)); value != "" {
		return strings.TrimRight(value, "/")
	}
	return strings.TrimRight(envOrDefault(EnvOAuthHost, DefaultOAuthHost), "/")
}

func EffectiveDeviceAuthorizationURL() string {
	if value := strings.TrimSpace(os.Getenv(EnvDeviceAuthorizationURL)); value != "" {
		return value
	}
	return EffectiveOAuthHost() + "/api/oauth/device_authorization"
}

func ValidatedDeviceAuthorizationURL() (string, error) {
	return ValidateOAuthEndpointURL(EffectiveDeviceAuthorizationURL())
}

func EffectiveTokenURL() string {
	if value := strings.TrimSpace(os.Getenv(EnvTokenURL)); value != "" {
		return value
	}
	return EffectiveOAuthHost() + "/api/oauth/token"
}

func ValidatedTokenURL() (string, error) {
	return ValidateOAuthEndpointURL(EffectiveTokenURL())
}

func EffectiveClientID() string {
	return envOrDefault(EnvClientID, DefaultClientID)
}

func EffectiveBaseURL(override string) string {
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		return strings.TrimRight(trimmed, "/")
	}
	return strings.TrimRight(envOrDefault(EnvBaseURL, DefaultBaseURL), "/")
}

func ValidatedBaseURL(override string) (string, error) {
	return ValidateBaseURL(EffectiveBaseURL(override))
}

func ValidateOAuthEndpointURL(raw string) (string, error) {
	if AllowUnsafeURLOverrides() {
		return urlvalidator.ValidateURLFormat(raw, true)
	}
	return urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
		AllowedHosts:     oauthEndpointAllowedHosts,
		RequireAllowlist: true,
		AllowPrivate:     false,
	})
}

// ValidateBaseURL 校验 API base URL，并把路径归一化到 /coding/v1。
func ValidateBaseURL(raw string) (string, error) {
	if AllowUnsafeURLOverrides() {
		return urlvalidator.ValidateURLFormat(raw, true)
	}
	normalized, err := urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
		AllowedHosts:     baseURLAllowedHosts,
		RequireAllowlist: true,
		AllowPrivate:     false,
	})
	if err != nil {
		return "", err
	}
	return normalizeKnownBaseURLPath(normalized)
}

func normalizeKnownBaseURLPath(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid url: %s", raw)
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		parsed.Path = "/coding/v1"
		parsed.RawPath = ""
		return strings.TrimRight(parsed.String(), "/"), nil
	}
	if path != "/coding/v1" {
		return "", fmt.Errorf("base URL path must be /coding/v1")
	}
	parsed.Path = path
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func BuildChatCompletionsURL(baseURL string) (string, error) {
	validatedBaseURL, err := ValidatedBaseURL(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base url: %w", err)
	}
	return validatedBaseURL + "/chat/completions", nil
}

func BuildModelsURL(baseURL string) (string, error) {
	validatedBaseURL, err := ValidatedBaseURL(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base url: %w", err)
	}
	return validatedBaseURL + "/models", nil
}

func AllowUnsafeURLOverrides() bool {
	return envBool(EnvAllowUnsafeURLOverrides)
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

func GenerateSessionID() (string, error) {
	bytes, err := GenerateRandomBytes(16)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// GenerateDeviceID 生成稳定设备指纹 ID（32 位 hex 无横线，等价 UUID 去横线格式）。
// 该 ID 会被绑进签发的 token，首次生成后必须持久化到账号 credentials 复用。
func GenerateDeviceID() (string, error) {
	bytes, err := GenerateRandomBytes(16)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// ──────────────────────────────────────────────────────────
// 设备指纹头（API 请求强制；OAuth 请求官方客户端也都携带，照带）
// ──────────────────────────────────────────────────────────

// FingerprintHeaders 返回 Kimi 上游要求的 8 个指纹头（含 User-Agent）。
// deviceID 必须是账号 credentials 中持久化的稳定值；头值全部保证纯 ASCII。
func FingerprintHeaders(deviceID string) map[string]string {
	return map[string]string{
		"User-Agent":         UserAgent,
		"X-Msh-Platform":     MshPlatform,
		"X-Msh-Version":      CLIVersion,
		"X-Msh-Device-Name":  DeviceName(),
		"X-Msh-Device-Model": DeviceModel(),
		"X-Msh-Os-Version":   OSVersion(),
		"X-Msh-Device-Id":    asciiHeaderValue(deviceID),
	}
}

// SetFingerprintHeaders 把 8 个指纹头写入 http.Header。
func SetFingerprintHeaders(header http.Header, deviceID string) {
	for key, value := range FingerprintHeaders(deviceID) {
		header.Set(key, value)
	}
}

// DeviceName 返回 X-Msh-Device-Name（主机名，仅 ASCII；可用 KIMI_DEVICE_NAME 覆盖）。
func DeviceName() string {
	if value := strings.TrimSpace(os.Getenv(EnvDeviceName)); value != "" {
		return asciiHeaderValue(value)
	}
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		return "sub2api-server"
	}
	return asciiHeaderValue(hostname)
}

// DeviceModel 返回 X-Msh-Device-Model（"系统名 版本 架构" 格式；可用 KIMI_DEVICE_MODEL 覆盖）。
// 格式不对会被上游 403，因此按 GOOS/GOARCH 给出符合官方客户端形态的默认值。
func DeviceModel() string {
	if value := strings.TrimSpace(os.Getenv(EnvDeviceModel)); value != "" {
		return asciiHeaderValue(value)
	}
	arch := runtime.GOARCH
	switch runtime.GOOS {
	case "darwin":
		return asciiHeaderValue("macOS 15.1.1 " + arch)
	case "windows":
		if arch == "amd64" {
			return "Windows 11 AMD64"
		}
		return asciiHeaderValue("Windows 11 " + strings.ToUpper(arch))
	case "linux":
		modelArch := arch
		if arch == "amd64" {
			modelArch = "x86_64"
		}
		return asciiHeaderValue("Linux 6.8.0 " + modelArch)
	default:
		return asciiHeaderValue(runtime.GOOS + " " + arch)
	}
}

// OSVersion 返回 X-Msh-Os-Version（内核版本串；可用 KIMI_OS_VERSION 覆盖）。
func OSVersion() string {
	if value := strings.TrimSpace(os.Getenv(EnvOSVersion)); value != "" {
		return asciiHeaderValue(value)
	}
	switch runtime.GOOS {
	case "darwin":
		return "Darwin Kernel Version 24.1.0"
	case "windows":
		return "10.0.22631"
	case "linux":
		return "6.8.0"
	default:
		return asciiHeaderValue(runtime.GOOS)
	}
}

// asciiHeaderValue 剥离非 ASCII 字符（对齐 kimi-cli _ascii_header_value 行为）。
func asciiHeaderValue(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= 32 && r <= 126 {
			return r
		}
		return -1
	}, strings.TrimSpace(value))
}
