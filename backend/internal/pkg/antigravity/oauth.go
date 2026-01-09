package antigravity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// Google OAuth 端点
	AuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	TokenURL     = "https://oauth2.googleapis.com/token"
	UserInfoURL  = "https://www.googleapis.com/oauth2/v2/userinfo"

	// Antigravity OAuth 客户端凭证
	ClientID     = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
	ClientSecret = "REDACTED"

	// 固定的 redirect_uri（用户需手动复制 code）
	RedirectURI = "http://localhost:8085/callback"

	// OAuth scopes
	Scopes = "https://www.googleapis.com/auth/cloud-platform " +
		"https://www.googleapis.com/auth/userinfo.email " +
		"https://www.googleapis.com/auth/userinfo.profile " +
		"https://www.googleapis.com/auth/cclog " +
		"https://www.googleapis.com/auth/experimentsandconfigs"

	// User-Agent（模拟官方客户端）
	UserAgent = "antigravity/1.104.0 darwin/arm64"

	// Session 过期时间
	SessionTTL = 30 * time.Minute

	// URL 可用性 TTL（不可用 URL 的恢复时间）
	URLAvailabilityTTL = 5 * time.Minute
)

// BaseURLs 定义 Antigravity API 端点，按优先级排序
// fallback 顺序: sandbox → daily → prod
var BaseURLs = []string{
	"https://daily-cloudcode-pa.sandbox.googleapis.com", // sandbox
	"https://daily-cloudcode-pa.googleapis.com",         // daily
	"https://cloudcode-pa.googleapis.com",               // prod
REDACTED

// BaseURL 默认 URL（保持向后兼容）
var BaseURL = BaseURLs[0]

// URLAvailability 管理 URL 可用性状态（带 TTL 自动恢复）
type URLAvailability struct {
	mu          sync.RWMutex
	unavailable map[string]time.Time // URL -> 恢复时间
	ttl         time.Duration
REDACTED

// DefaultURLAvailability 全局 URL 可用性管理器
var DefaultURLAvailability = NewURLAvailability(URLAvailabilityTTL)

// NewURLAvailability 创建 URL 可用性管理器
func NewURLAvailability(ttl time.Duration) *URLAvailability {
	return &URLAvailability{
		unavailable: make(map[string]time.Time),
		ttl:         ttl,
REDACTED
REDACTED

// MarkUnavailable 标记 URL 临时不可用
func (u *URLAvailability) MarkUnavailable(url string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.unavailable[url] = time.Now().Add(u.ttl)
REDACTED

// IsAvailable 检查 URL 是否可用
func (u *URLAvailability) IsAvailable(url string) bool {
	u.mu.RLock()
	defer u.mu.RUnlock()
	expiry, exists := u.unavailable[url]
	if !exists {
		return true
REDACTED
	return time.Now().After(expiry)
REDACTED

// GetAvailableURLs 返回可用的 URL 列表（保持优先级顺序）
func (u *URLAvailability) GetAvailableURLs() []string {
	u.mu.RLock()
	defer u.mu.RUnlock()

	now := time.Now()
	result := make([]string, 0, len(BaseURLs))
	for _, url := range BaseURLs {
		expiry, exists := u.unavailable[url]
		if !exists || now.After(expiry) {
			result = append(result, url)
	REDACTED
REDACTED
	return result
REDACTED

// OAuthSession 保存 OAuth 授权流程的临时状态
type OAuthSession struct {
	State        string    `json:"state"`
	CodeVerifier string    `json:"code_verifier"`
	ProxyURL     string    `json:"proxy_url,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
REDACTED

// SessionStore OAuth session 存储
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*OAuthSession
	stopCh   chan struct{REDACTED
REDACTED

func NewSessionStore() *SessionStore {
	store := &SessionStore{
		sessions: make(map[string]*OAuthSession),
		stopCh:   make(chan struct{REDACTED),
REDACTED
	go store.cleanup()
	return store
REDACTED

func (s *SessionStore) Set(sessionID string, session *OAuthSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = session
REDACTED

func (s *SessionStore) Get(sessionID string) (*OAuthSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, false
REDACTED
	if time.Since(session.CreatedAt) > SessionTTL {
		return nil, false
REDACTED
	return session, true
REDACTED

func (s *SessionStore) Delete(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
REDACTED

func (s *SessionStore) Stop() {
	select {
	case <-s.stopCh:
		return
	default:
		close(s.stopCh)
REDACTED
REDACTED

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
			REDACTED
		REDACTED
			s.mu.Unlock()
	REDACTED
REDACTED
REDACTED

func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
REDACTED
	return b, nil
REDACTED

func GenerateState() (string, error) {
	bytes, err := GenerateRandomBytes(32)
	if err != nil {
		return "", err
REDACTED
	return base64URLEncode(bytes), nil
REDACTED

func GenerateSessionID() (string, error) {
	bytes, err := GenerateRandomBytes(16)
	if err != nil {
		return "", err
REDACTED
	return hex.EncodeToString(bytes), nil
REDACTED

func GenerateCodeVerifier() (string, error) {
	bytes, err := GenerateRandomBytes(32)
	if err != nil {
		return "", err
REDACTED
	return base64URLEncode(bytes), nil
REDACTED

func GenerateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64URLEncode(hash[:])
REDACTED

func base64URLEncode(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
REDACTED

// BuildAuthorizationURL 构建 Google OAuth 授权 URL
func BuildAuthorizationURL(state, codeChallenge string) string {
	params := url.Values{REDACTED
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
REDACTED

// GenerateMockProjectID 生成随机 project_id（当 API 不返回时使用）
// 格式：{形容词REDACTED-{名词REDACTED-{5位随机字符REDACTED
func GenerateMockProjectID() string {
	adjectives := []string{"useful", "bright", "swift", "calm", "bold"REDACTED
	nouns := []string{"fuze", "wave", "spark", "flow", "core"REDACTED

	randBytes, _ := GenerateRandomBytes(7)

	adj := adjectives[int(randBytes[0])%len(adjectives)]
	noun := nouns[int(randBytes[1])%len(nouns)]

	// 生成 5 位随机字符（a-z0-9）
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	suffix := make([]byte, 5)
	for i := 0; i < 5; i++ {
		suffix[i] = charset[int(randBytes[i+2])%len(charset)]
REDACTED

	return fmt.Sprintf("%s-%s-%s", adj, noun, string(suffix))
REDACTED
