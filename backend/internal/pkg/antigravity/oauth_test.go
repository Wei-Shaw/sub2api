//go:build unit

package antigravity

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// getClientSecret
// ---------------------------------------------------------------------------

func TestGetClientSecret_环境变量设置(t *testing.T) {
	old := defaultClientSecret
	defaultClientSecret = ""
	t.Cleanup(func() { defaultClientSecret = old })
	t.Setenv(AntigravityOAuthClientSecretEnv, "my-secret-value")

	// 需要重新触发 init 逻辑：手动从环境变量读取
	defaultClientSecret = os.Getenv(AntigravityOAuthClientSecretEnv)

	secret, err := getClientSecret()
	if err != nil {
		t.Fatalf("获取 client_secret 失败: %v", err)
	}
	if secret != "my-secret-value" {
		t.Errorf("client_secret 不匹配: got %s, want my-secret-value", secret)
	}
}

func TestGetClientSecret_环境变量为空(t *testing.T) {
	old := defaultClientSecret
	defaultClientSecret = ""
	t.Cleanup(func() { defaultClientSecret = old })

	_, err := getClientSecret()
	if err == nil {
		t.Fatal("defaultClientSecret 为空时应返回错误")
	}
	if !strings.Contains(err.Error(), AntigravityOAuthClientSecretEnv) {
		t.Errorf("错误信息应包含环境变量名: got %s", err.Error())
	}
}

func TestGetClientSecret_环境变量未设置(t *testing.T) {
	old := defaultClientSecret
	defaultClientSecret = ""
	t.Cleanup(func() { defaultClientSecret = old })

	_, err := getClientSecret()
	if err == nil {
		t.Fatal("defaultClientSecret 为空时应返回错误")
	}
}

func TestGetClientSecret_环境变量含空格(t *testing.T) {
	old := defaultClientSecret
	defaultClientSecret = "   "
	t.Cleanup(func() { defaultClientSecret = old })

	_, err := getClientSecret()
	if err == nil {
		t.Fatal("defaultClientSecret 仅含空格时应返回错误")
	}
}

func TestGetClientSecret_环境变量有前后空格(t *testing.T) {
	old := defaultClientSecret
	defaultClientSecret = "  valid-secret  "
	t.Cleanup(func() { defaultClientSecret = old })

	secret, err := getClientSecret()
	if err != nil {
		t.Fatalf("获取 client_secret 失败: %v", err)
	}
	if secret != "valid-secret" {
		t.Errorf("应去除前后空格: got %q, want %q", secret, "valid-secret")
	}
}

// ---------------------------------------------------------------------------
// ForwardBaseURLs
// ---------------------------------------------------------------------------

func TestForwardBaseURLs_Daily优先(t *testing.T) {
	urls := ForwardBaseURLs()
	if len(urls) == 0 {
		t.Fatal("ForwardBaseURLs 返回空列表")
	}

	// daily URL 应排在第一位
	if urls[0] != antigravityDailyBaseURL {
		t.Errorf("第一个 URL 应为 daily: got %s, want %s", urls[0], antigravityDailyBaseURL)
	}

	// 应包含所有 URL
	if len(urls) != len(BaseURLs) {
		t.Errorf("URL 数量不匹配: got %d, want %d", len(urls), len(BaseURLs))
	}

	// 验证 prod URL 也在列表中
	found := false
	for _, u := range urls {
		if u == antigravityProdBaseURL {
			found = true
			break
		}
	}
	if !found {
		t.Error("ForwardBaseURLs 中缺少 prod URL")
	}
}

func TestForwardBaseURLs_不修改原切片(t *testing.T) {
	originalFirst := BaseURLs[0]
	_ = ForwardBaseURLs()
	// 确保原始 BaseURLs 未被修改
	if BaseURLs[0] != originalFirst {
		t.Errorf("ForwardBaseURLs 不应修改原始 BaseURLs: got %s, want %s", BaseURLs[0], originalFirst)
	}
}

// ---------------------------------------------------------------------------
// URLAvailability
// ---------------------------------------------------------------------------

func TestNewURLAvailability(t *testing.T) {
	ua := NewURLAvailability(5 * time.Minute)
	if ua == nil {
		t.Fatal("NewURLAvailability 返回 nil")
	}
	if ua.ttl != 5*time.Minute {
		t.Errorf("TTL 不匹配: got %v, want 5m", ua.ttl)
	}
	if ua.unavailable == nil {
		t.Error("unavailable map 不应为 nil")
	}
}

func TestURLAvailability_MarkUnavailable(t *testing.T) {
	ua := NewURLAvailability(5 * time.Minute)
	testURL := "https://example.com"

	ua.MarkUnavailable(testURL)

	if ua.IsAvailable(testURL) {
		t.Error("标记为不可用后 IsAvailable 应返回 false")
	}
}

func TestURLAvailability_MarkSuccess(t *testing.T) {
	ua := NewURLAvailability(5 * time.Minute)
	testURL := "https://example.com"

	// 先标记为不可用
	ua.MarkUnavailable(testURL)
	if ua.IsAvailable(testURL) {
		t.Error("标记为不可用后应不可用")
	}

	// 标记成功后应恢复可用
	ua.MarkSuccess(testURL)
	if !ua.IsAvailable(testURL) {
		t.Error("MarkSuccess 后应恢复可用")
	}

	// 验证 lastSuccess 被设置
	ua.mu.RLock()
	if ua.lastSuccess != testURL {
		t.Errorf("lastSuccess 不匹配: got %s, want %s", ua.lastSuccess, testURL)
	}
	ua.mu.RUnlock()
}

func TestURLAvailability_IsAvailable_TTL过期(t *testing.T) {
	// 使用极短的 TTL
	ua := NewURLAvailability(1 * time.Millisecond)
	testURL := "https://example.com"

	ua.MarkUnavailable(testURL)
	// 等待 TTL 过期
	time.Sleep(5 * time.Millisecond)

	if !ua.IsAvailable(testURL) {
		t.Error("TTL 过期后 URL 应恢复可用")
	}
}

func TestURLAvailability_IsAvailable_未标记的URL(t *testing.T) {
	ua := NewURLAvailability(5 * time.Minute)
	if !ua.IsAvailable("https://never-marked.com") {
		t.Error("未标记的 URL 应默认可用")
	}
}

func TestURLAvailability_GetAvailableURLs(t *testing.T) {
	ua := NewURLAvailability(10 * time.Minute)

	// 默认所有 URL 都可用
	urls := ua.GetAvailableURLs()
	if len(urls) != len(BaseURLs) {
		t.Errorf("可用 URL 数量不匹配: got %d, want %d", len(urls), len(BaseURLs))
	}
}

func TestURLAvailability_GetAvailableURLs_标记一个不可用(t *testing.T) {
	ua := NewURLAvailability(10 * time.Minute)

	if len(BaseURLs) < 2 {
		t.Skip("BaseURLs 少于 2 个，跳过此测试")
	}

	ua.MarkUnavailable(BaseURLs[0])
	urls := ua.GetAvailableURLs()

	// 标记的 URL 不应出现在可用列表中
	for _, u := range urls {
		if u == BaseURLs[0] {
			t.Errorf("被标记不可用的 URL 不应出现在可用列表中: %s", BaseURLs[0])
		}
	}
}

func TestURLAvailability_GetAvailableURLsWithBase(t *testing.T) {
	ua := NewURLAvailability(10 * time.Minute)
	customURLs := []string{"https://a.com", "https://b.com", "https://c.com"}

	urls := ua.GetAvailableURLsWithBase(customURLs)
	if len(urls) != 3 {
		t.Errorf("可用 URL 数量不匹配: got %d, want 3", len(urls))
	}
}

func TestURLAvailability_GetAvailableURLsWithBase_LastSuccess优先(t *testing.T) {
	ua := NewURLAvailability(10 * time.Minute)
	customURLs := []string{"https://a.com", "https://b.com", "https://c.com"}

	ua.MarkSuccess("https://c.com")

	urls := ua.GetAvailableURLsWithBase(customURLs)
	if len(urls) != 3 {
		t.Fatalf("可用 URL 数量不匹配: got %d, want 3", len(urls))
	}
	// c.com 应排在第一位
	if urls[0] != "https://c.com" {
		t.Errorf("lastSuccess 应排在第一位: got %s, want https://c.com", urls[0])
	}
	// 其余按原始顺序
	if urls[1] != "https://a.com" {
		t.Errorf("第二个应为 a.com: got %s", urls[1])
	}
	if urls[2] != "https://b.com" {
		t.Errorf("第三个应为 b.com: got %s", urls[2])
	}
}

func TestURLAvailability_GetAvailableURLsWithBase_LastSuccess不可用(t *testing.T) {
	ua := NewURLAvailability(10 * time.Minute)
	customURLs := []string{"https://a.com", "https://b.com"}

	ua.MarkSuccess("https://b.com")
	ua.MarkUnavailable("https://b.com")

	urls := ua.GetAvailableURLsWithBase(customURLs)
	// b.com 被标记不可用，不应出现
	if len(urls) != 1 {
		t.Fatalf("可用 URL 数量不匹配: got %d, want 1", len(urls))
	}
	if urls[0] != "https://a.com" {
		t.Errorf("仅 a.com 应可用: got %s", urls[0])
	}
}

func TestURLAvailability_GetAvailableURLsWithBase_LastSuccess不在列表中(t *testing.T) {
	ua := NewURLAvailability(10 * time.Minute)
	customURLs := []string{"https://a.com", "https://b.com"}

	ua.MarkSuccess("https://not-in-list.com")

	urls := ua.GetAvailableURLsWithBase(customURLs)
	// lastSuccess 不在自定义列表中，不应被添加
	if len(urls) != 2 {
		t.Fatalf("可用 URL 数量不匹配: got %d, want 2", len(urls))
	}
}

// ---------------------------------------------------------------------------
// SessionStore
// ---------------------------------------------------------------------------

func TestNewSessionStore(t *testing.T) {
	store := NewSessionStore()
	defer store.Stop()

	if store == nil {
		t.Fatal("NewSessionStore 返回 nil")
	}
	if store.sessions == nil {
		t.Error("sessions map 不应为 nil")
	}
}

func TestSessionStore_SetAndGet(t *testing.T) {
	store := NewSessionStore()
	defer store.Stop()

	session := &OAuthSession{
		State:        "test-state",
		CodeVerifier: "test-verifier",
		ProxyURL:     "http://proxy.example.com",
		CreatedAt:    time.Now(),
	}

	store.Set("session-1", session)

	got, ok := store.Get("session-1")
	if !ok {
		t.Fatal("Get 应返回 true")
	}
	if got.State != "test-state" {
		t.Errorf("State 不匹配: got %s", got.State)
	}
	if got.CodeVerifier != "test-verifier" {
		t.Errorf("CodeVerifier 不匹配: got %s", got.CodeVerifier)
	}
	if got.ProxyURL != "http://proxy.example.com" {
		t.Errorf("ProxyURL 不匹配: got %s", got.ProxyURL)
	}
}

func TestSessionStore_Get_不存在(t *testing.T) {
	store := NewSessionStore()
	defer store.Stop()

	_, ok := store.Get("nonexistent")
	if ok {
		t.Error("不存在的 session 应返回 false")
	}
}

func TestSessionStore_Get_过期(t *testing.T) {
	store := NewSessionStore()
	defer store.Stop()

	session := &OAuthSession{
		State:     "expired-state",
		CreatedAt: time.Now().Add(-SessionTTL - time.Minute), // 已过期
	}

	store.Set("expired-session", session)

	_, ok := store.Get("expired-session")
	if ok {
		t.Error("过期的 session 应返回 false")
	}
}

func TestSessionStore_Delete(t *testing.T) {
	store := NewSessionStore()
	defer store.Stop()

	session := &OAuthSession{
		State:     "to-delete",
		CreatedAt: time.Now(),
	}

	store.Set("del-session", session)
	store.Delete("del-session")

	_, ok := store.Get("del-session")
	if ok {
		t.Error("删除后 Get 应返回 false")
	}
}

func TestSessionStore_Delete_不存在(t *testing.T) {
	store := NewSessionStore()
	defer store.Stop()

	// 删除不存在的 session 不应 panic
	store.Delete("nonexistent")
}

func TestSessionStore_Stop(t *testing.T) {
	store := NewSessionStore()
	store.Stop()

	// 多次 Stop 不应 panic
	store.Stop()
}

func TestSessionStore_多个Session(t *testing.T) {
	store := NewSessionStore()
	defer store.Stop()

	for i := 0; i < 10; i++ {
		session := &OAuthSession{
			State:     "state-" + string(rune('0'+i)),
			CreatedAt: time.Now(),
		}
		store.Set("session-"+string(rune('0'+i)), session)
	}

	// 验证都能取到
	for i := 0; i < 10; i++ {
		_, ok := store.Get("session-" + string(rune('0'+i)))
		if !ok {
			t.Errorf("session-%d 应存在", i)
		}
	}
}

// ---------------------------------------------------------------------------
// GenerateRandomBytes
// ---------------------------------------------------------------------------

func TestGenerateRandomBytes_长度正确(t *testing.T) {
	sizes := []int{0, 1, 16, 32, 64, 128}
	for _, size := range sizes {
		b, err := GenerateRandomBytes(size)
		if err != nil {
			t.Fatalf("GenerateRandomBytes(%d) 失败: %v", size, err)
		}
		if len(b) != size {
			t.Errorf("长度不匹配: got %d, want %d", len(b), size)
		}
	}
}

func TestGenerateRandomBytes_不同调用产生不同结果(t *testing.T) {
	b1, err := GenerateRandomBytes(32)
	if err != nil {
		t.Fatalf("第一次调用失败: %v", err)
	}
	b2, err := GenerateRandomBytes(32)
	if err != nil {
		t.Fatalf("第二次调用失败: %v", err)
	}
	// 两次生成的随机字节应该不同（概率上几乎不可能相同）
	if string(b1) == string(b2) {
		t.Error("两次生成的随机字节相同，概率极低，可能有问题")
	}
}

// ---------------------------------------------------------------------------
// GenerateState
// ---------------------------------------------------------------------------

func TestGenerateState_返回值格式(t *testing.T) {
	state, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState 失败: %v", err)
	}
	if state == "" {
		t.Error("GenerateState 返回空字符串")
	}
	// base64url 编码不应包含 +, /, =
	if strings.ContainsAny(state, "+/=") {
		t.Errorf("GenerateState 返回值包含非 base64url 字符: %s", state)
	}
	// 32 字节的 base64url 编码长度应为 43（去掉了尾部 = 填充）
	if len(state) != 43 {
		t.Errorf("GenerateState 返回值长度不匹配: got %d, want 43", len(state))
	}
}

func TestGenerateState_唯一性(t *testing.T) {
	s1, _ := GenerateState()
	s2, _ := GenerateState()
	if s1 == s2 {
		t.Error("两次 GenerateState 结果相同")
	}
}

// ---------------------------------------------------------------------------
// GenerateSessionID
// ---------------------------------------------------------------------------

func TestGenerateSessionID_返回值格式(t *testing.T) {
	id, err := GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID 失败: %v", err)
	}
	if id == "" {
		t.Error("GenerateSessionID 返回空字符串")
	}
	// 16 字节的 hex 编码长度应为 32
	if len(id) != 32 {
		t.Errorf("GenerateSessionID 返回值长度不匹配: got %d, want 32", len(id))
	}
	// 验证是合法的 hex 字符串
	if _, err := hex.DecodeString(id); err != nil {
		t.Errorf("GenerateSessionID 返回值不是合法的 hex 字符串: %s, err: %v", id, err)
	}
}

func TestGenerateSessionID_唯一性(t *testing.T) {
	id1, _ := GenerateSessionID()
	id2, _ := GenerateSessionID()
	if id1 == id2 {
		t.Error("两次 GenerateSessionID 结果相同")
	}
}

// ---------------------------------------------------------------------------
// GenerateCodeVerifier
// ---------------------------------------------------------------------------

func TestGenerateCodeVerifier_返回值格式(t *testing.T) {
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier 失败: %v", err)
	}
	if verifier == "" {
		t.Error("GenerateCodeVerifier 返回空字符串")
	}
	// base64url 编码不应包含 +, /, =
	if strings.ContainsAny(verifier, "+/=") {
		t.Errorf("GenerateCodeVerifier 返回值包含非 base64url 字符: %s", verifier)
	}
	// 32 字节的 base64url 编码长度应为 43
	if len(verifier) != 43 {
		t.Errorf("GenerateCodeVerifier 返回值长度不匹配: got %d, want 43", len(verifier))
	}
}

func TestGenerateCodeVerifier_唯一性(t *testing.T) {
	v1, _ := GenerateCodeVerifier()
	v2, _ := GenerateCodeVerifier()
	if v1 == v2 {
		t.Error("两次 GenerateCodeVerifier 结果相同")
	}
}

// ---------------------------------------------------------------------------
// GenerateCodeChallenge
// ---------------------------------------------------------------------------

func TestGenerateCodeChallenge_SHA256_Base64URL(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"

	challenge := GenerateCodeChallenge(verifier)

	// 手动计算预期值
	hash := sha256.Sum256([]byte(verifier))
	expected := strings.TrimRight(base64.URLEncoding.EncodeToString(hash[:]), "=")

	if challenge != expected {
		t.Errorf("CodeChallenge 不匹配: got %s, want %s", challenge, expected)
	}
}

func TestGenerateCodeChallenge_不含填充字符(t *testing.T) {
	challenge := GenerateCodeChallenge("test-verifier")
	if strings.Contains(challenge, "=") {
		t.Errorf("CodeChallenge 不应包含 = 填充字符: %s", challenge)
	}
}

func TestGenerateCodeChallenge_不含非URL安全字符(t *testing.T) {
	challenge := GenerateCodeChallenge("another-verifier")
	if strings.ContainsAny(challenge, "+/") {
		t.Errorf("CodeChallenge 不应包含 + 或 / 字符: %s", challenge)
	}
}

func TestGenerateCodeChallenge_相同输入相同输出(t *testing.T) {
	c1 := GenerateCodeChallenge("same-verifier")
	c2 := GenerateCodeChallenge("same-verifier")
	if c1 != c2 {
		t.Errorf("相同输入应产生相同输出: got %s and %s", c1, c2)
	}
}

func TestGenerateCodeChallenge_不同输入不同输出(t *testing.T) {
	c1 := GenerateCodeChallenge("verifier-1")
	c2 := GenerateCodeChallenge("verifier-2")
	if c1 == c2 {
		t.Error("不同输入应产生不同输出")
	}
}

// ---------------------------------------------------------------------------
// BuildAuthorizationURL
// ---------------------------------------------------------------------------

func TestBuildAuthorizationURL_参数验证(t *testing.T) {
	state := "test-state-123"
	codeChallenge := "test-challenge-abc"

	authURL := BuildAuthorizationURL(state, codeChallenge)

	// 验证以 AuthorizeURL 开头
	if !strings.HasPrefix(authURL, AuthorizeURL+"?") {
		t.Errorf("URL 应以 %s? 开头: got %s", AuthorizeURL, authURL)
	}

	// 解析 URL 并验证参数
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("解析 URL 失败: %v", err)
	}

	params := parsed.Query()

	expectedParams := map[string]string{
		"client_id":              ClientID,
		"redirect_uri":           RedirectURI,
		"response_type":          "code",
		"scope":                  Scopes,
		"state":                  state,
		"code_challenge":         codeChallenge,
		"code_challenge_method":  "S256",
		"access_type":            "offline",
		"prompt":                 "consent",
		"include_granted_scopes": "true",
	}

	for key, want := range expectedParams {
		got := params.Get(key)
		if got != want {
			t.Errorf("参数 %s 不匹配: got %q, want %q", key, got, want)
		}
	}
}

func TestBuildAuthorizationURL_参数数量(t *testing.T) {
	authURL := BuildAuthorizationURL("s", "c")
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("解析 URL 失败: %v", err)
	}

	params := parsed.Query()
	// 应包含 10 个参数
	expectedCount := 10
	if len(params) != expectedCount {
		t.Errorf("参数数量不匹配: got %d, want %d", len(params), expectedCount)
	}
}

func TestBuildAuthorizationURL_特殊字符编码(t *testing.T) {
	state := "state+with/special=chars"
	codeChallenge := "challenge+value"

	authURL := BuildAuthorizationURL(state, codeChallenge)

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("解析 URL 失败: %v", err)
	}

	// 解析后应正确还原特殊字符
	if got := parsed.Query().Get("state"); got != state {
		t.Errorf("state 参数编码/解码不匹配: got %q, want %q", got, state)
	}
}

// ---------------------------------------------------------------------------
// 常量值验证
// ---------------------------------------------------------------------------

func TestConstants_值正确(t *testing.T) {
	if AuthorizeURL != "https://accounts.google.com/o/oauth2/v2/auth" {
		t.Errorf("AuthorizeURL 不匹配: got %s", AuthorizeURL)
	}
	if TokenURL != "https://oauth2.googleapis.com/token" {
		t.Errorf("TokenURL 不匹配: got %s", TokenURL)
	}
	if UserInfoURL != "https://www.googleapis.com/oauth2/v2/userinfo" {
		t.Errorf("UserInfoURL 不匹配: got %s", UserInfoURL)
	}
	if ClientID != "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com" {
		t.Errorf("ClientID 不匹配: got %s", ClientID)
	}
	secret, err := getClientSecret()
	if err != nil {
		t.Fatalf("getClientSecret 应返回默认值，但报错: %v", err)
	}
	if secret != "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" {
		t.Errorf("默认 client_secret 不匹配: got %s", secret)
	}
	if RedirectURI != "http://localhost:8085/callback" {
		t.Errorf("RedirectURI 不匹配: got %s", RedirectURI)
	}
	if GetUserAgent() != "antigravity/1.21.9 windows/amd64" {
		t.Errorf("UserAgent 不匹配: got %s", GetUserAgent())
	}
	if SessionTTL != 30*time.Minute {
		t.Errorf("SessionTTL 不匹配: got %v", SessionTTL)
	}
	if URLAvailabilityTTL != 5*time.Minute {
		t.Errorf("URLAvailabilityTTL 不匹配: got %v", URLAvailabilityTTL)
	}
}

func TestScopes_包含必要范围(t *testing.T) {
	expectedScopes := []string{
		"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/userinfo.email",
		"https://www.googleapis.com/auth/userinfo.profile",
		"https://www.googleapis.com/auth/cclog",
		"https://www.googleapis.com/auth/experimentsandconfigs",
	}

	for _, scope := range expectedScopes {
		if !strings.Contains(Scopes, scope) {
			t.Errorf("Scopes 缺少 %s", scope)
		}
	}
}
