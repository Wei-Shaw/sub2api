package antigravity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	// Google OAuth 端点
	AuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	TokenURL     = "https://oauth2.googleapis.com/token"
	UserInfoURL  = "https://www.googleapis.com/oauth2/v2/userinfo"

	// Antigravity OAuth 客户端凭证
	ClientID = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"

	// AntigravityOAuthClientSecretEnv 是 Antigravity OAuth client_secret 的环境变量名。
	AntigravityOAuthClientSecretEnv = "ANTIGRAVITY_OAUTH_CLIENT_SECRET"

	// 固定的 redirect_uri（用户需手动复制 code）
	RedirectURI = "http://localhost:8085/callback"

	// OAuth scopes
	Scopes = "https://www.googleapis.com/auth/cloud-platform " +
		"https://www.googleapis.com/auth/userinfo.email " +
		"https://www.googleapis.com/auth/userinfo.profile " +
		"https://www.googleapis.com/auth/cclog " +
		"https://www.googleapis.com/auth/experimentsandconfigs"

	// Session 过期时间
	SessionTTL = 30 * time.Minute

	// URL 可用性 TTL（不可用 URL 的恢复时间）
	URLAvailabilityTTL = 5 * time.Minute

	// Antigravity API 端点
	antigravityProdBaseURL  = "https://cloudcode-pa.googleapis.com"
	antigravityDailyBaseURL = "https://daily-cloudcode-pa.sandbox.googleapis.com"
)

// defaultUserAgentVersion 可通过环境变量 ANTIGRAVITY_USER_AGENT_VERSION 配置，默认 1.20.5
var defaultUserAgentVersion = "1.21.9"

// defaultClientSecret 可通过环境变量 ANTIGRAVITY_OAUTH_CLIENT_SECRET 配置
var defaultClientSecret = "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"

func init() {
	// 从环境变量读取版本号，未设置则使用默认值
	if version := os.Getenv("ANTIGRAVITY_USER_AGENT_VERSION"); version != "" {
		defaultUserAgentVersion = version
	}
	// 从环境变量读取 client_secret，未设置则使用默认值
	if secret := os.Getenv(AntigravityOAuthClientSecretEnv); secret != "" {
		defaultClientSecret = secret
	}
}

// GetUserAgent 返回当前配置的 User-Agent
func GetUserAgent() string {
	return fmt.Sprintf("antigravity/%s windows/amd64", defaultUserAgentVersion)
}

func getClientSecret() (string, error) {
	if v := strings.TrimSpace(defaultClientSecret); v != "" {
		return v, nil
	}
	return "", infraerrors.Newf(http.StatusBadRequest, "ANTIGRAVITY_OAUTH_CLIENT_SECRET_MISSING", "missing antigravity oauth client_secret; set %s", AntigravityOAuthClientSecretEnv)
}

// BaseURLs 定义 Antigravity API 端点（与 Antigravity-Manager 保持一致）
var BaseURLs = []string{
	antigravityProdBaseURL,  // prod (优先)
	antigravityDailyBaseURL, // daily sandbox (备用)
}

// BaseURL 默认 URL（保持向后兼容）
var BaseURL = BaseURLs[0]

// ForwardBaseURLs 返回 API 转发用的 URL 顺序（daily 优先）
func ForwardBaseURLs() []string {
	if len(BaseURLs) == 0 {
		return nil
	}
	urls := append([]string(nil), BaseURLs...)
	dailyIndex := -1
	for i, url := range urls {
		if url == antigravityDailyBaseURL {
			dailyIndex = i
			break
		}
	}
	if dailyIndex <= 0 {
		return urls
	}
	reordered := make([]string, 0, len(urls))
	reordered = append(reordered, urls[dailyIndex])
	for i, url := range urls {
		if i == dailyIndex {
			continue
		}
		reordered = append(reordered, url)
	}
	return reordered
}

// URLAvailability 管理 URL 可用性状态（带 TTL 自动恢复和动态优先级）
type URLAvailability struct {
	mu          sync.RWMutex
	unavailable map[string]time.Time // URL -> 恢复时间
	ttl         time.Duration
	lastSuccess string // 最近成功请求的 URL，优先使用
}

// DefaultURLAvailability 全局 URL 可用性管理器
var DefaultURLAvailability = NewURLAvailability(URLAvailabilityTTL)

// NewURLAvailability 创建 URL 可用性管理器
func NewURLAvailability(ttl time.Duration) *URLAvailability {
	return &URLAvailability{
		unavailable: make(map[string]time.Time),
		ttl:         ttl,
	}
}

// MarkUnavailable 标记 URL 临时不可用
func (u *URLAvailability) MarkUnavailable(url string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.unavailable[url] = time.Now().Add(u.ttl)
}

// MarkSuccess 标记 URL 请求成功，将其设为优先使用
func (u *URLAvailability) MarkSuccess(url string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.lastSuccess = url
	// 成功后清除该 URL 的不可用标记
	delete(u.unavailable, url)
}

// IsAvailable 检查 URL 是否可用
func (u *URLAvailability) IsAvailable(url string) bool {
	u.mu.RLock()
	defer u.mu.RUnlock()
	expiry, exists := u.unavailable[url]
	if !exists {
		return true
	}
	return time.Now().After(expiry)
}

// GetAvailableURLs 返回可用的 URL 列表
// 最近成功的 URL 优先，其他按默认顺序
func (u *URLAvailability) GetAvailableURLs() []string {
	return u.GetAvailableURLsWithBase(BaseURLs)
}

// GetAvailableURLsWithBase 返回可用的 URL 列表（使用自定义顺序）
// 最近成功的 URL 优先，其他按传入顺序
func (u *URLAvailability) GetAvailableURLsWithBase(baseURLs []string) []string {
	u.mu.RLock()
	defer u.mu.RUnlock()

	now := time.Now()
	result := make([]string, 0, len(baseURLs))

	// 如果有最近成功的 URL 且可用，放在最前面
	if u.lastSuccess != "" {
		found := false
		for _, url := range baseURLs {
			if url == u.lastSuccess {
				found = true
				break
			}
		}
		if found {
			expiry, exists := u.unavailable[u.lastSuccess]
			if !exists || now.After(expiry) {
				result = append(result, u.lastSuccess)
			}
		}
	}

	// 添加其他可用的 URL（按传入顺序）
	for _, url := range baseURLs {
		// 跳过已添加的 lastSuccess
		if url == u.lastSuccess {
			continue
		}
		expiry, exists := u.unavailable[url]
		if !exists || now.After(expiry) {
			result = append(result, url)
		}
	}
	return result
}

// OAuthSession 保存 OAuth 授权流程的临时状态
type OAuthSession struct {
	State        string    `json:"state"`
	CodeVerifier string    `json:"code_verifier"`
	ProxyURL     string    `json:"proxy_url,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// SessionStore OAuth session 存储
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*OAuthSession
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
	select {
	case <-s.stopCh:
		return
	default:
		close(s.stopCh)
	}
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

func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func GenerateState() (string, error) {
	bytes, err := GenerateRandomBytes(32)
	if err != nil {
		return "", err
	}
	return base64URLEncode(bytes), nil
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

// BuildAuthorizationURL 构建 Google OAuth 授权 URL
func BuildAuthorizationURL(state, codeChallenge string) string {
	params := url.Values{}
	params.Set("client_id", ClientID)
	params.Set("redirect_uri", RedirectURI)
	params.Set("response_type", "code")
	params.Set("scope", Scopes)
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	params.Set("access_type", "offline")
	params.Set("prompt", "consent")
	params.Set("include_granted_scopes", "true")

	return fmt.Sprintf("%s?%s", AuthorizeURL, params.Encode())
}
