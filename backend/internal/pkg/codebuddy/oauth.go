package codebuddy

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultBaseURL 是 CodeBuddy（腾讯 copilot）API 的默认上游基地址。
	DefaultBaseURL = "https://copilot.tencent.com"
	// DefaultDomain 是登录态相关接口使用的 X-Domain 头值。
	DefaultDomain = "www.codebuddy.cn"
	// PlatformDomain 是插件/推理接口使用的 X-Domain 头值。
	PlatformDomain = "copilot.tencent.com"

	// PluginAuthStatePath 生成登录态（state）接口路径。
	PluginAuthStatePath = "/v2/plugin/auth/state"
	// PluginAuthTokenPath 用 state 换取 token 的接口路径。
	PluginAuthTokenPath = "/v2/plugin/auth/token"
	// PluginAuthTokenRefreshPath 刷新 token 的接口路径。
	PluginAuthTokenRefreshPath = "/v2/plugin/auth/token/refresh"
	// LoginAccountPath 获取登录账号信息的接口路径。
	LoginAccountPath = "/v2/plugin/login/account"
	// ConfigPath 获取用户配置（可用模型）的接口路径。
	ConfigPath = "/v3/config"

	// ChatCompletionsPath OpenAI 兼容的对话补全接口路径。
	ChatCompletionsPath = "/v2/chat/completions"
	// ResponsesPath OpenAI Responses 兼容接口路径（CodeBuddy 暂无公开端点，保留扩展）。
	ResponsesPath = "/v1/responses"
	// ImagesGenerationsPath 图片生成接口路径（扩展用，待端点确认）。
	ImagesGenerationsPath = "/v1/images/generations"
	// VideosGenerationsPath 视频生成接口路径（扩展用，待端点确认）。
	VideosGenerationsPath = "/v1/videos/generations"

	// AuthSourcePlugin 刷新接口要求的 X-Auth-Refresh-Source 头值。
	AuthSourcePlugin = "plugin"
	// PlatformWorkBuddy 生成 state 时使用的 platform 查询参数。
	PlatformWorkBuddy = "workbuddy"
)

// TokenResponse 表示 CodeBuddy 的 token 响应（/v2/plugin/auth/token 与 /refresh 共用结构）。
// 注意：CodeBuddy 接口返回的字段均为驼峰命名（如 accessToken / expiresIn / refreshToken），
// 必须使用对应的 json tag 才能正确解析。
type TokenResponse struct {
	AccessToken       string `json:"accessToken,omitempty"`
	ExpiresIn         int64  `json:"expiresIn,omitempty"`
	RefreshExpiresIn  int64  `json:"refreshExpiresIn,omitempty"`
	RefreshToken      string `json:"refreshToken,omitempty"`
	TokenType         string `json:"tokenType,omitempty"`
	SessionState      string `json:"sessionState,omitempty"`
	// Domain 是 /v2/plugin/auth/token 返回的用户域标识。
	Domain string `json:"domain,omitempty"`
	// Scope 是 /v2/plugin/auth/token 返回的授权范围。
	Scope string `json:"scope,omitempty"`
}

// StateResult 表示 /v2/plugin/auth/state 的返回数据。
type StateResult struct {
	State   string `json:"state"`
	AuthURL string `json:"authUrl"`
	Domain  string `json:"domain,omitempty"`
}

// AccountInfo 表示 /v2/plugin/login/account 返回的账号信息（仅取网关需要的字段）。
type AccountInfo struct {
	UID         string `json:"uid"`
	Nickname    string `json:"nickname"`
	Type        string `json:"type"`
	UIN         string `json:"uin"`
	PhoneNumber string `json:"phoneNumber"`
}

// ConfigResponse 表示 /v3/config 返回的用户配置（仅取网关需要的字段）。
type ConfigResponse struct {
	Agents []ConfigAgent `json:"agents"`
	Models []ConfigModel `json:"models"`
}

// ConfigAgent 表示 /v3/config 中某个 agent 的配置。
type ConfigAgent struct {
	Name   string   `json:"name"`
	AsTool bool     `json:"asTool"`
	Models []string `json:"models"`
}

// ConfigModel 表示 /v3/config 顶层 models 列表中的单个模型条目
// （比 agent.models 更全面，是 CodeBuddy 后端真实可用的完整模型目录）。
type ConfigModel struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Credits           string `json:"credits"`
	DescriptionZh     string `json:"descriptionZh"`
	DisabledMultimodal bool  `json:"disabledMultimodal"`
	MaxAllowedSize    int    `json:"maxAllowedSize"`
	MaxInputTokens    int    `json:"maxInputTokens"`
	MaxOutputTokens   int    `json:"maxOutputTokens"`
	SupportsImages    bool   `json:"supportsImages"`
	SupportsReasoning bool   `json:"supportsReasoning"`
	SupportsToolCall  bool   `json:"supportsToolCall"`
}

// EffectiveBaseURL 返回规范化后的基地址，空值回退到默认地址。
func EffectiveBaseURL(override string) string {
	trimmed := strings.TrimSpace(override)
	if trimmed == "" {
		return DefaultBaseURL
	}
	return strings.TrimRight(trimmed, "/")
}

// buildURL 将基地址与相对路径拼接为完整 URL。
func buildURL(baseURL, path string) string {
	return EffectiveBaseURL(baseURL) + path
}

// BuildAuthStateURL 生成登录态（state）获取接口地址。
func BuildAuthStateURL(baseURL string) string {
	return buildURL(baseURL, PluginAuthStatePath)
}

// BuildAuthTokenURL 生成用 state 换取 token 的接口地址。
func BuildAuthTokenURL(baseURL, state string) string {
	return buildURL(baseURL, PluginAuthTokenPath) + "?state=" + state
}

// BuildAuthTokenRefreshURL 生成刷新 token 的接口地址。
func BuildAuthTokenRefreshURL(baseURL string) string {
	return buildURL(baseURL, PluginAuthTokenRefreshPath)
}

// BuildLoginAccountURL 生成账号信息接口地址。
func BuildLoginAccountURL(baseURL, state string) string {
	return buildURL(baseURL, LoginAccountPath) + "?state=" + state
}

// BuildConfigURL 生成用户配置接口地址。
func BuildConfigURL(baseURL string) string {
	return buildURL(baseURL, ConfigPath)
}

// BuildChatCompletionsURL 生成对话补全接口地址（OpenAI 兼容）。
func BuildChatCompletionsURL(baseURL string) (string, error) {
	return buildURL(baseURL, ChatCompletionsPath), nil
}

// BuildResponsesURL 生成 Responses 接口地址（扩展用）。
func BuildResponsesURL(baseURL string) (string, error) {
	return buildURL(baseURL, ResponsesPath), nil
}

// BuildImagesGenerationsURL 生成图片生成接口地址（扩展用）。
func BuildImagesGenerationsURL(baseURL string) (string, error) {
	return buildURL(baseURL, ImagesGenerationsPath), nil
}

// BuildVideosGenerationsURL 生成视频生成接口地址（扩展用）。
func BuildVideosGenerationsURL(baseURL string) (string, error) {
	return buildURL(baseURL, VideosGenerationsPath), nil
}

// generateRandomHex 生成 16 字节的随机十六进制字符串。
func generateRandomHex() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GenerateState 生成随机 state 字符串。
func GenerateState() (string, error) {
	return generateRandomHex()
}

// GenerateSessionID 生成随机 session ID，用于在服务端关联 state 与代理配置。
func GenerateSessionID() (string, error) {
	return generateRandomHex()
}

// OAuthSession 保存单次 CodeBuddy OAuth 流程的会话（state -> proxyURL）。
type OAuthSession struct {
	State     string    `json:"state"`
	ProxyURL  string    `json:"proxy_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// SessionStore 在内存中管理 CodeBuddy OAuth 会话。
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*OAuthSession
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewSessionStore 创建会话存储并启动清理协程。
func NewSessionStore() *SessionStore {
	store := &SessionStore{
		sessions: make(map[string]*OAuthSession),
		stopCh:   make(chan struct{}),
	}
	go store.cleanup()
	return store
}

// Set 保存会话。
func (s *SessionStore) Set(sessionID string, session *OAuthSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = session
}

// Get 获取会话，超时返回 false。
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

// Delete 删除会话。
func (s *SessionStore) Delete(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

// Stop 停止清理协程。
func (s *SessionStore) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
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

// SessionTTL 是 CodeBuddy OAuth 会话的存活时间。
const SessionTTL = 30 * time.Minute

// RuntimeSanityReport 是 CodeBuddy 运行期端点校验报告。
type RuntimeSanityReport struct {
	BaseURL         string `json:"base_url"`
	AuthState       string `json:"auth_state_url"`
	ChatCompletions string `json:"chat_completions_url"`
	Config          string `json:"config_url"`
}

// RuntimeSanity 返回 CodeBuddy 当前生效的端点配置（用于后台自检）。
func RuntimeSanity() RuntimeSanityReport {
	base := DefaultBaseURL
	chat, _ := BuildChatCompletionsURL(base)
	return RuntimeSanityReport{
		BaseURL:         base,
		AuthState:       BuildAuthStateURL(base) + "?platform=" + PlatformWorkBuddy,
		ChatCompletions: chat,
		Config:          BuildConfigURL(base),
	}
}
