package xai

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

const (
	OAuthIssuer         = "https://auth.x.ai"
	DiscoveryURL        = OAuthIssuer + "/.well-known/openid-configuration"
	DefaultAuthorizeURL = OAuthIssuer + "/oauth2/authorize"
	DefaultTokenURL     = OAuthIssuer + "/oauth2/token"
	DefaultBaseURL      = "https://api.x.ai/v1"
	DefaultCLIBaseURL   = "https://cli-chat-proxy.grok.com/v1"
	DefaultClientID     = "b1a00492-073a-47ea-816f-4c329264a828"
	DefaultScope        = "openid profile email offline_access grok-cli:access api:access"
	DefaultRedirectURI  = "http://127.0.0.1:56121/callback"
	SessionTTL          = 30 * time.Minute

	EnvAuthorizeURL               = "XAI_OAUTH_AUTHORIZE_URL"
	EnvTokenURL                   = "XAI_OAUTH_TOKEN_URL"
	EnvClientID                   = "XAI_OAUTH_CLIENT_ID"
	EnvScope                      = "XAI_OAUTH_SCOPE"
	EnvRedirectURI                = "XAI_OAUTH_REDIRECT_URI"
	EnvBaseURL                    = "XAI_BASE_URL"
	EnvAllowUnsafeURLOverrides    = "XAI_ALLOW_UNSAFE_URL_OVERRIDES"
	EnvUnsafeAllowHighConcurrency = "XAI_GROK_UNSAFE_ALLOW_CONCURRENCY_GT_ONE"
)

var (
	oauthEndpointAllowedHosts = []string{"x.ai", "*.x.ai"}
	// *.api.x.ai 覆盖 xAI 区域端点（us-east-1/us-west-2/eu-west-1 等），
	// 运营方可在端点间手动切换以规避单点不可用。
	baseURLAllowedHosts = []string{"api.x.ai", "*.api.x.ai", "cli-chat-proxy.grok.com"}
)

// OAuthSession stores one PKCE OAuth flow.
type OAuthSession struct {
	State         string    `json:"state"`
	CodeVerifier  string    `json:"code_verifier"`
	CodeChallenge string    `json:"code_challenge"`
	ClientID      string    `json:"client_id,omitempty"`
	Scope         string    `json:"scope,omitempty"`
	ProxyURL      string    `json:"proxy_url,omitempty"`
	RedirectURI   string    `json:"redirect_uri"`
	CreatedAt     time.Time `json:"created_at"`
}

// SessionStore manages xAI OAuth sessions in memory.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*OAuthSession
	stopOnce sync.Once
	stopCh   chan struct{}
}

func NewSessionStore() *SessionStore {
	store := &SessionStore{
		sessions: make(map[string]*OAuthSession),
		stopCh:   make(chan struct{}),
	}
	go store.cleanup()
	return store
}

func (s *SessionStore) Set(sessionID string, session *OAuthSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = session
}

func (s *SessionStore) Get(sessionID string) (*OAuthSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, false
	}
	if time.Since(session.CreatedAt) > SessionTTL {
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
				if time.Since(session.CreatedAt) > SessionTTL {
					delete(s.sessions, id)
				}
			}
			s.mu.Unlock()
		}
	}
}

func EffectiveAuthorizeURL() string {
	return envOrDefault(EnvAuthorizeURL, DefaultAuthorizeURL)
}

func ValidatedAuthorizeURL() (string, error) {
	return ValidateOAuthEndpointURL(EffectiveAuthorizeURL())
}

func EffectiveTokenURL() string {
	return envOrDefault(EnvTokenURL, DefaultTokenURL)
}

func ValidatedTokenURL() (string, error) {
	return ValidateOAuthEndpointURL(EffectiveTokenURL())
}

func EffectiveClientID() string {
	return envOrDefault(EnvClientID, DefaultClientID)
}

func EffectiveScope() string {
	return envOrDefault(EnvScope, DefaultScope)
}

func EffectiveRedirectURI(override string) string {
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		return trimmed
	}
	return envOrDefault(EnvRedirectURI, DefaultRedirectURI)
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

// BaseURLValidator applies the caller's outbound URL trust policy before xAI
// endpoint paths are appended. The service layer uses this for API-key accounts
// so the global security.url_allowlist policy remains the single source of
// truth; OAuth callers keep using the strict trusted-host validator.
type BaseURLValidator func(string) (string, error)

func validatedBaseURLWithValidator(override string, validator BaseURLValidator) (string, error) {
	if validator == nil {
		return ValidatedBaseURL(override)
	}
	raw := EffectiveBaseURL(override)
	validated, err := validator(raw)
	if err != nil {
		return "", err
	}
	return normalizeKnownBaseURLPath(validated)
}

type RuntimeSanityCheck struct {
	Value     string `json:"value"`
	Valid     bool   `json:"valid"`
	Error     string `json:"error,omitempty"`
	IsDefault bool   `json:"is_default,omitempty"`
}

type RuntimeSanityReport struct {
	BaseURL               RuntimeSanityCheck `json:"base_url"`
	OAuthAuthorizeURL     RuntimeSanityCheck `json:"oauth_authorize_url"`
	OAuthTokenURL         RuntimeSanityCheck `json:"oauth_token_url"`
	OAuthRedirectURI      RuntimeSanityCheck `json:"oauth_redirect_uri"`
	UnsafeURLOverrides    bool               `json:"unsafe_url_overrides"`
	UnsafeHighConcurrency bool               `json:"unsafe_high_concurrency"`
	PublicGatewayScope    string             `json:"public_gateway_scope"`
	ProxyPolicy           string             `json:"proxy_policy"`
}

func RuntimeSanity() RuntimeSanityReport {
	return RuntimeSanityReport{
		BaseURL:               runtimeSanityCheck(EffectiveBaseURL(""), EnvBaseURL, ValidatedBaseURL),
		OAuthAuthorizeURL:     runtimeSanityCheck(EffectiveAuthorizeURL(), EnvAuthorizeURL, func(string) (string, error) { return ValidatedAuthorizeURL() }),
		OAuthTokenURL:         runtimeSanityCheck(EffectiveTokenURL(), EnvTokenURL, func(string) (string, error) { return ValidatedTokenURL() }),
		OAuthRedirectURI:      runtimeSanityCheck(EffectiveRedirectURI(""), EnvRedirectURI, validateRedirectURI),
		UnsafeURLOverrides:    AllowUnsafeURLOverrides(),
		UnsafeHighConcurrency: AllowUnsafeHighConcurrency(),
		PublicGatewayScope:    "responses_only",
		ProxyPolicy:           "account_proxy_optional; OAuth URLs use trusted-host allowlists; API-key base URLs require public HTTPS unless unsafe overrides are enabled",
	}
}

func runtimeSanityCheck(value string, envKey string, validate func(string) (string, error)) RuntimeSanityCheck {
	normalized, err := validate(value)
	check := RuntimeSanityCheck{
		Value:     sanitizeRuntimeURLValue(normalized),
		Valid:     err == nil,
		IsDefault: strings.TrimSpace(os.Getenv(envKey)) == "",
	}
	if err != nil {
		check.Value = sanitizeRuntimeURLValue(value)
		check.Error = sanitizeRuntimeError(err.Error(), value)
	}
	return check
}

func validateRedirectURI(raw string) (string, error) {
	return urlvalidator.ValidateURLFormat(raw, true)
}

func sanitizeRuntimeURLValue(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return trimmed
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func sanitizeRuntimeError(rawErr string, rawValue string) string {
	redacted := logredact.RedactText(rawErr)
	trimmedValue := strings.TrimSpace(rawValue)
	if trimmedValue == "" {
		return redacted
	}
	sanitizedValue := sanitizeRuntimeURLValue(trimmedValue)
	redacted = strings.ReplaceAll(redacted, trimmedValue, sanitizedValue)
	redacted = strings.ReplaceAll(redacted, logredact.RedactText(trimmedValue), sanitizedValue)
	return redacted
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

func ValidateBaseURL(raw string) (string, error) {
	if AllowUnsafeURLOverrides() {
		return urlvalidator.ValidateURLFormat(raw, true)
	}
	normalized, err := urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
		AllowPrivate: false,
	})
	if err != nil {
		return "", err
	}
	return normalizeKnownBaseURLPath(normalized)
}

func ValidateTrustedBaseURL(raw string) (string, error) {
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

// normalizeKnownBaseURLPath 规范化 base URL 的 path 部分：
//   - 官方主机固定使用 /v1 前缀（空 path 自动补齐，其余 path 拒绝）；
//   - 其他主机保留管理员配置的任意 path 前缀（第三方转发地址常见
//     /xxx/v1 之类的路由前缀），空 path 仍按惯例补 /v1。
//
// 所有主机统一禁止 userinfo/query/fragment，并去除尾部斜杠。
func normalizeKnownBaseURLPath(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("invalid base URL")
	}
	if parsed.User != nil {
		return "", errors.New("base URL must not include userinfo")
	}
	if parsed.ForceQuery || parsed.RawQuery != "" {
		return "", errors.New("base URL must not include a query")
	}
	if parsed.Fragment != "" {
		return "", errors.New("base URL must not include a fragment")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		parsed.Path = "/v1"
		parsed.RawPath = ""
		return strings.TrimRight(parsed.String(), "/"), nil
	}
	if path != "/v1" && IsOfficialBaseURLHost(parsed.Hostname()) {
		return "", fmt.Errorf("base URL path must be /v1")
	}
	parsed.Path = path
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

// IsOfficialBaseURLHost 报告 host 是否属于官方 API / 区域 API / CLI 网关主机。
func IsOfficialBaseURLHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, allowed := range baseURLAllowedHosts {
		if strings.HasPrefix(allowed, "*.") {
			suffix := strings.TrimPrefix(allowed, "*.")
			if host == suffix || strings.HasSuffix(host, "."+suffix) {
				return true
			}
			continue
		}
		if host == allowed {
			return true
		}
	}
	return false
}

// IsParseableBaseURL 报告 raw 是否能解析出 host。
// 供读取路径判定存量脏数据：无法解析的值应回落默认端点，而不是把流量发往未定义目标。
func IsParseableBaseURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	parsed, err := url.Parse(trimmed)
	return err == nil && parsed.Host != ""
}

// IsOfficialBaseURL 报告 raw 是否指向官方主机（api.x.ai / *.api.x.ai 区域端点 / CLI 网关），
// 容忍存量凭证中的历史变体（大小写、显式 443 端口、百分号编码 path 等）。
// 无法解析的值一并视为官方，调用方据此回落默认端点而不是把流量发往未定义目标。
func IsOfficialBaseURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return true
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return true
	}
	return IsOfficialBaseURLHost(parsed.Hostname())
}

func AllowUnsafeURLOverrides() bool {
	return envBool(EnvAllowUnsafeURLOverrides)
}

func AllowUnsafeHighConcurrency() bool {
	return envBool(EnvUnsafeAllowHighConcurrency)
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

func GenerateState() (string, error) {
	bytes, err := GenerateRandomBytes(32)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func GenerateNonce() (string, error) {
	bytes, err := GenerateRandomBytes(16)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func GenerateSessionID() (string, error) {
	bytes, err := GenerateRandomBytes(16)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func GenerateCodeVerifier() (string, error) {
	bytes, err := GenerateRandomBytes(32)
	if err != nil {
		return "", err
	}
	return base64URLEncode(bytes), nil
}

func GenerateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64URLEncode(hash[:])
}

func base64URLEncode(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}

func BuildAuthorizationURL(state, codeChallenge, redirectURI, nonce string) (string, error) {
	redirectURI = EffectiveRedirectURI(redirectURI)
	authorizeURL, err := ValidatedAuthorizeURL()
	if err != nil {
		return "", fmt.Errorf("invalid authorize url: %w", err)
	}

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", EffectiveClientID())
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", EffectiveScope())
	params.Set("state", state)
	params.Set("nonce", nonce)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	params.Set("plan", "generic")
	params.Set("referrer", "sub2api")

	return fmt.Sprintf("%s?%s", authorizeURL, params.Encode()), nil
}

// AuthorizationInput is a parsed manual OAuth callback input.
type AuthorizationInput struct {
	Code          string
	State         string
	RequiresState bool
}

// ParseAuthorizationInput accepts a full callback URL, query string, or bare code.
func ParseAuthorizationInput(raw string) AuthorizationInput {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return AuthorizationInput{}
	}

	if parsed, err := url.Parse(trimmed); err == nil && parsed != nil {
		values := parsed.Query()
		if code := strings.TrimSpace(values.Get("code")); code != "" {
			return AuthorizationInput{
				Code:          code,
				State:         strings.TrimSpace(values.Get("state")),
				RequiresState: true,
			}
		}
	}

	queryCandidate := strings.TrimPrefix(trimmed, "?")
	if strings.Contains(queryCandidate, "=") {
		if values, err := url.ParseQuery(queryCandidate); err == nil {
			if code := strings.TrimSpace(values.Get("code")); code != "" {
				return AuthorizationInput{
					Code:          code,
					State:         strings.TrimSpace(values.Get("state")),
					RequiresState: true,
				}
			}
		}
	}

	return AuthorizationInput{Code: trimmed}
}

func BuildResponsesURL(baseURL string) (string, error) {
	return BuildResponsesURLWithValidator(baseURL, nil)
}

func BuildResponsesURLWithValidator(baseURL string, validator BaseURLValidator) (string, error) {
	validatedBaseURL, err := validatedBaseURLWithValidator(baseURL, validator)
	if err != nil {
		return "", fmt.Errorf("invalid base url: %w", err)
	}
	return validatedBaseURL + "/responses", nil
}

func BuildChatCompletionsURL(baseURL string) (string, error) {
	return BuildChatCompletionsURLWithValidator(baseURL, nil)
}

func BuildChatCompletionsURLWithValidator(baseURL string, validator BaseURLValidator) (string, error) {
	validatedBaseURL, err := validatedBaseURLWithValidator(baseURL, validator)
	if err != nil {
		return "", fmt.Errorf("invalid base url: %w", err)
	}
	return validatedBaseURL + "/chat/completions", nil
}

func BuildImagesGenerationsURL(baseURL string) (string, error) {
	return BuildImagesGenerationsURLWithValidator(baseURL, nil)
}

func BuildImagesGenerationsURLWithValidator(baseURL string, validator BaseURLValidator) (string, error) {
	validatedBaseURL, err := validatedBaseURLWithValidator(baseURL, validator)
	if err != nil {
		return "", fmt.Errorf("invalid base url: %w", err)
	}
	return validatedBaseURL + "/images/generations", nil
}

func BuildImagesEditsURL(baseURL string) (string, error) {
	return BuildImagesEditsURLWithValidator(baseURL, nil)
}

func BuildImagesEditsURLWithValidator(baseURL string, validator BaseURLValidator) (string, error) {
	validatedBaseURL, err := validatedBaseURLWithValidator(baseURL, validator)
	if err != nil {
		return "", fmt.Errorf("invalid base url: %w", err)
	}
	return validatedBaseURL + "/images/edits", nil
}

func BuildVideosGenerationsURL(baseURL string) (string, error) {
	return BuildVideosGenerationsURLWithValidator(baseURL, nil)
}

func BuildVideosGenerationsURLWithValidator(baseURL string, validator BaseURLValidator) (string, error) {
	validatedBaseURL, err := validatedBaseURLWithValidator(baseURL, validator)
	if err != nil {
		return "", fmt.Errorf("invalid base url: %w", err)
	}
	return validatedBaseURL + "/videos/generations", nil
}

func BuildVideosEditsURL(baseURL string) (string, error) {
	return BuildVideosEditsURLWithValidator(baseURL, nil)
}

func BuildVideosEditsURLWithValidator(baseURL string, validator BaseURLValidator) (string, error) {
	validatedBaseURL, err := validatedBaseURLWithValidator(baseURL, validator)
	if err != nil {
		return "", fmt.Errorf("invalid base url: %w", err)
	}
	return validatedBaseURL + "/videos/edits", nil
}

func BuildVideosExtensionsURL(baseURL string) (string, error) {
	return BuildVideosExtensionsURLWithValidator(baseURL, nil)
}

func BuildVideosExtensionsURLWithValidator(baseURL string, validator BaseURLValidator) (string, error) {
	validatedBaseURL, err := validatedBaseURLWithValidator(baseURL, validator)
	if err != nil {
		return "", fmt.Errorf("invalid base url: %w", err)
	}
	return validatedBaseURL + "/videos/extensions", nil
}

func BuildVideoURL(baseURL, requestID string) (string, error) {
	return BuildVideoURLWithValidator(baseURL, requestID, nil)
}

func BuildVideoURLWithValidator(baseURL, requestID string, validator BaseURLValidator) (string, error) {
	validatedBaseURL, err := validatedBaseURLWithValidator(baseURL, validator)
	if err != nil {
		return "", fmt.Errorf("invalid base url: %w", err)
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return "", fmt.Errorf("request id is required")
	}
	return validatedBaseURL + "/videos/" + url.PathEscape(requestID), nil
}

// TokenResponse represents xAI OAuth token responses.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
	Scope        string `json:"scope,omitempty"`
}
