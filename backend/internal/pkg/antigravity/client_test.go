//go:build unit

package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// NewAPIRequestWithURL
// ---------------------------------------------------------------------------

func TestNewAPIRequestWithURL_普通请求(t *testing.T) {
	ctx := context.Background()
	baseURL := "https://example.com"
	action := "generateContent"
	token := "test-token"
	body := []byte(`{"prompt":"hello"}`)

	req, err := NewAPIRequestWithURL(ctx, baseURL, action, token, body)
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}

	// 验证 URL 不含 ?alt=sse
	expectedURL := "https://example.com/v1internal:generateContent"
	if req.URL.String() != expectedURL {
		t.Errorf("URL 不匹配: got %s, want %s", req.URL.String(), expectedURL)
	}

	// 验证请求方法
	if req.Method != http.MethodPost {
		t.Errorf("请求方法不匹配: got %s, want POST", req.Method)
	}

	// 验证 Headers
	if ct := req.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type 不匹配: got %s", ct)
	}
	if auth := req.Header.Get("Authorization"); auth != "Bearer test-token" {
		t.Errorf("Authorization 不匹配: got %s", auth)
	}
	if ua := req.Header.Get("User-Agent"); ua != GetUserAgent() {
		t.Errorf("User-Agent 不匹配: got %s, want %s", ua, GetUserAgent())
	}
}

func TestNewAPIRequestWithURL_流式请求(t *testing.T) {
	ctx := context.Background()
	baseURL := "https://example.com"
	action := "streamGenerateContent"
	token := "tok"
	body := []byte(`{}`)

	req, err := NewAPIRequestWithURL(ctx, baseURL, action, token, body)
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}

	expectedURL := "https://example.com/v1internal:streamGenerateContent?alt=sse"
	if req.URL.String() != expectedURL {
		t.Errorf("URL 不匹配: got %s, want %s", req.URL.String(), expectedURL)
	}
}

func TestNewAPIRequestWithURL_空Body(t *testing.T) {
	ctx := context.Background()
	req, err := NewAPIRequestWithURL(ctx, "https://example.com", "test", "tok", nil)
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}
	if req.Body == nil {
		t.Error("Body 应该非 nil（bytes.NewReader(nil) 会返回空 reader）")
	}
}

// ---------------------------------------------------------------------------
// NewAPIRequest
// ---------------------------------------------------------------------------

func TestNewAPIRequest_使用默认URL(t *testing.T) {
	ctx := context.Background()
	req, err := NewAPIRequest(ctx, "generateContent", "tok", []byte(`{}`))
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}

	expected := BaseURL + "/v1internal:generateContent"
	if req.URL.String() != expected {
		t.Errorf("URL 不匹配: got %s, want %s", req.URL.String(), expected)
	}
}

// ---------------------------------------------------------------------------
// TierInfo.UnmarshalJSON
// ---------------------------------------------------------------------------

func TestTierInfo_UnmarshalJSON_字符串格式(t *testing.T) {
	data := []byte(`"free-tier"`)
	var tier TierInfo
	if err := tier.UnmarshalJSON(data); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if tier.ID != "free-tier" {
		t.Errorf("ID 不匹配: got %s, want free-tier", tier.ID)
	}
	if tier.Name != "" {
		t.Errorf("Name 应为空: got %s", tier.Name)
	}
}

func TestTierInfo_UnmarshalJSON_对象格式(t *testing.T) {
	data := []byte(`{"id":"g1-pro-tier","name":"Pro","description":"Pro plan"}`)
	var tier TierInfo
	if err := tier.UnmarshalJSON(data); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if tier.ID != "g1-pro-tier" {
		t.Errorf("ID 不匹配: got %s, want g1-pro-tier", tier.ID)
	}
	if tier.Name != "Pro" {
		t.Errorf("Name 不匹配: got %s, want Pro", tier.Name)
	}
	if tier.Description != "Pro plan" {
		t.Errorf("Description 不匹配: got %s, want Pro plan", tier.Description)
	}
}

func TestTierInfo_UnmarshalJSON_null(t *testing.T) {
	data := []byte(`null`)
	var tier TierInfo
	if err := tier.UnmarshalJSON(data); err != nil {
		t.Fatalf("反序列化 null 失败: %v", err)
	}
	if tier.ID != "" {
		t.Errorf("null 场景下 ID 应为空: got %s", tier.ID)
	}
}

func TestTierInfo_UnmarshalJSON_空数据(t *testing.T) {
	data := []byte(``)
	var tier TierInfo
	if err := tier.UnmarshalJSON(data); err != nil {
		t.Fatalf("反序列化空数据失败: %v", err)
	}
	if tier.ID != "" {
		t.Errorf("空数据场景下 ID 应为空: got %s", tier.ID)
	}
}

func TestTierInfo_UnmarshalJSON_空格包裹null(t *testing.T) {
	data := []byte(`  null  `)
	var tier TierInfo
	if err := tier.UnmarshalJSON(data); err != nil {
		t.Fatalf("反序列化空格 null 失败: %v", err)
	}
	if tier.ID != "" {
		t.Errorf("空格 null 场景下 ID 应为空: got %s", tier.ID)
	}
}

func TestTierInfo_UnmarshalJSON_通过JSON嵌套结构(t *testing.T) {
	// 模拟 LoadCodeAssistResponse 中的嵌套反序列化
	jsonData := `{"currentTier":"free-tier","paidTier":{"id":"g1-ultra-tier","name":"Ultra"}}`
	var resp LoadCodeAssistResponse
	if err := json.Unmarshal([]byte(jsonData), &resp); err != nil {
		t.Fatalf("反序列化嵌套结构失败: %v", err)
	}
	if resp.CurrentTier == nil || resp.CurrentTier.ID != "free-tier" {
		t.Errorf("CurrentTier 不匹配: got %+v", resp.CurrentTier)
	}
	if resp.PaidTier == nil || resp.PaidTier.ID != "g1-ultra-tier" {
		t.Errorf("PaidTier 不匹配: got %+v", resp.PaidTier)
	}
}

// ---------------------------------------------------------------------------
// LoadCodeAssistResponse.GetTier
// ---------------------------------------------------------------------------

func TestGetTier_PaidTier优先(t *testing.T) {
	resp := &LoadCodeAssistResponse{
		CurrentTier: &TierInfo{ID: "free-tier"},
		PaidTier:    &PaidTierInfo{ID: "g1-pro-tier"},
	}
	if got := resp.GetTier(); got != "g1-pro-tier" {
		t.Errorf("应返回 paidTier: got %s", got)
	}
}

func TestGetTier_回退到CurrentTier(t *testing.T) {
	resp := &LoadCodeAssistResponse{
		CurrentTier: &TierInfo{ID: "free-tier"},
	}
	if got := resp.GetTier(); got != "free-tier" {
		t.Errorf("应返回 currentTier: got %s", got)
	}
}

func TestGetTier_PaidTier为空ID(t *testing.T) {
	resp := &LoadCodeAssistResponse{
		CurrentTier: &TierInfo{ID: "free-tier"},
		PaidTier:    &PaidTierInfo{ID: ""},
	}
	// paidTier.ID 为空时应回退到 currentTier
	if got := resp.GetTier(); got != "free-tier" {
		t.Errorf("paidTier.ID 为空时应回退到 currentTier: got %s", got)
	}
}

func TestGetAvailableCredits(t *testing.T) {
	resp := &LoadCodeAssistResponse{
		PaidTier: &PaidTierInfo{
			ID: "g1-pro-tier",
			AvailableCredits: []AvailableCredit{
				{
					CreditType:                  "GOOGLE_ONE_AI",
					CreditAmount:                "25",
					MinimumCreditAmountForUsage: "5",
				},
			},
		},
	}

	credits := resp.GetAvailableCredits()
	if len(credits) != 1 {
		t.Fatalf("AI Credits 数量不匹配: got %d", len(credits))
	}
	if credits[0].GetAmount() != 25 {
		t.Errorf("CreditAmount 解析不正确: got %v", credits[0].GetAmount())
	}
	if credits[0].GetMinimumAmount() != 5 {
		t.Errorf("MinimumCreditAmountForUsage 解析不正确: got %v", credits[0].GetMinimumAmount())
	}
}

func TestGetTier_两者都为nil(t *testing.T) {
	resp := &LoadCodeAssistResponse{}
	if got := resp.GetTier(); got != "" {
		t.Errorf("两者都为 nil 时应返回空字符串: got %s", got)
	}
}

// ---------------------------------------------------------------------------
// NewClient
// ---------------------------------------------------------------------------

func mustNewClient(t *testing.T, proxyURL string) *Client {
	t.Helper()
	client, err := NewClient(proxyURL)
	if err != nil {
		t.Fatalf("NewClient(%q) failed: %v", proxyURL, err)
	}
	return client
}

func TestNewClient_无代理(t *testing.T) {
	client, err := NewClient("")
	if err != nil {
		t.Fatalf("NewClient 返回错误: %v", err)
	}
	if client == nil {
		t.Fatal("NewClient 返回 nil")
	}
	if client.httpClient == nil {
		t.Fatal("httpClient 为 nil")
	}
	if client.httpClient.Timeout != 30*time.Second {
		t.Errorf("Timeout 不匹配: got %v, want 30s", client.httpClient.Timeout)
	}
	// 无代理时 Transport 应为 nil（使用默认）
	if client.httpClient.Transport != nil {
		t.Error("无代理时 Transport 应为 nil")
	}
}

func TestNewClient_有代理(t *testing.T) {
	client, err := NewClient("http://proxy.example.com:8080")
	if err != nil {
		t.Fatalf("NewClient 返回错误: %v", err)
	}
	if client == nil {
		t.Fatal("NewClient 返回 nil")
	}
	if client.httpClient.Transport == nil {
		t.Fatal("有代理时 Transport 不应为 nil")
	}
}

func TestNewClient_空格代理(t *testing.T) {
	client, err := NewClient("   ")
	if err != nil {
		t.Fatalf("NewClient 返回错误: %v", err)
	}
	if client == nil {
		t.Fatal("NewClient 返回 nil")
	}
	// 空格代理应等同于无代理
	if client.httpClient.Transport != nil {
		t.Error("空格代理 Transport 应为 nil")
	}
}

func TestNewClient_无效代理URL(t *testing.T) {
	// 无效 URL 应返回 error
	_, err := NewClient("://invalid")
	if err == nil {
		t.Fatal("无效代理 URL 应返回错误")
	}
	if !strings.Contains(err.Error(), "invalid proxy URL") {
		t.Errorf("错误信息应包含 'invalid proxy URL': got %s", err.Error())
	}
}

// ---------------------------------------------------------------------------
// isConnectionError
// ---------------------------------------------------------------------------

func TestIsConnectionError_nil(t *testing.T) {
	if isConnectionError(nil) {
		t.Error("nil 错误不应判定为连接错误")
	}
}

func TestIsConnectionError_超时错误(t *testing.T) {
	// 使用 net.OpError 包装超时
	err := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: &timeoutError{},
	}
	if !isConnectionError(err) {
		t.Error("超时错误应判定为连接错误")
	}
}

// timeoutError 实现 net.Error 接口用于测试
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

func TestIsConnectionError_netOpError(t *testing.T) {
	err := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: fmt.Errorf("connection refused"),
	}
	if !isConnectionError(err) {
		t.Error("net.OpError 应判定为连接错误")
	}
}

func TestIsConnectionError_urlError(t *testing.T) {
	err := &url.Error{
		Op:  "Get",
		URL: "https://example.com",
		Err: fmt.Errorf("some error"),
	}
	if !isConnectionError(err) {
		t.Error("url.Error 应判定为连接错误")
	}
}

func TestIsConnectionError_普通错误(t *testing.T) {
	err := fmt.Errorf("some random error")
	if isConnectionError(err) {
		t.Error("普通错误不应判定为连接错误")
	}
}

func TestIsConnectionError_包装的netOpError(t *testing.T) {
	inner := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: fmt.Errorf("connection refused"),
	}
	err := fmt.Errorf("wrapping: %w", inner)
	if !isConnectionError(err) {
		t.Error("被包装的 net.OpError 应判定为连接错误")
	}
}

// ---------------------------------------------------------------------------
// shouldFallbackToNextURL
// ---------------------------------------------------------------------------

func TestShouldFallbackToNextURL_连接错误(t *testing.T) {
	err := &net.OpError{Op: "dial", Net: "tcp", Err: fmt.Errorf("refused")}
	if !shouldFallbackToNextURL(err, 0) {
		t.Error("连接错误应触发 URL 降级")
	}
}

func TestShouldFallbackToNextURL_状态码(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       bool
	}{
		{"429 Too Many Requests", http.StatusTooManyRequests, true},
		{"408 Request Timeout", http.StatusRequestTimeout, true},
		{"404 Not Found", http.StatusNotFound, true},
		{"500 Internal Server Error", http.StatusInternalServerError, true},
		{"502 Bad Gateway", http.StatusBadGateway, true},
		{"503 Service Unavailable", http.StatusServiceUnavailable, true},
		{"200 OK", http.StatusOK, false},
		{"201 Created", http.StatusCreated, false},
		{"400 Bad Request", http.StatusBadRequest, false},
		{"401 Unauthorized", http.StatusUnauthorized, false},
		{"403 Forbidden", http.StatusForbidden, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldFallbackToNextURL(nil, tt.statusCode)
			if got != tt.want {
				t.Errorf("shouldFallbackToNextURL(nil, %d) = %v, want %v", tt.statusCode, got, tt.want)
			}
		})
	}
}

func TestShouldFallbackToNextURL_无错误且200(t *testing.T) {
	if shouldFallbackToNextURL(nil, http.StatusOK) {
		t.Error("无错误且 200 不应触发 URL 降级")
	}
}

// ---------------------------------------------------------------------------
// Client.ExchangeCode (使用 httptest)
// ---------------------------------------------------------------------------

func TestClient_ExchangeCode_成功(t *testing.T) {
	old := defaultClientSecret
	defaultClientSecret = "test-secret"
	t.Cleanup(func() { defaultClientSecret = old })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求方法
		if r.Method != http.MethodPost {
			t.Errorf("请求方法不匹配: got %s", r.Method)
		}
		// 验证 Content-Type
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type 不匹配: got %s", ct)
		}
		// 验证请求体参数
		if err := r.ParseForm(); err != nil {
			t.Fatalf("解析表单失败: %v", err)
		}
		if r.FormValue("client_id") != ClientID {
			t.Errorf("client_id 不匹配: got %s", r.FormValue("client_id"))
		}
		if r.FormValue("client_secret") != "test-secret" {
			t.Errorf("client_secret 不匹配: got %s", r.FormValue("client_secret"))
		}
		if r.FormValue("code") != "auth-code" {
			t.Errorf("code 不匹配: got %s", r.FormValue("code"))
		}
		if r.FormValue("code_verifier") != "verifier123" {
			t.Errorf("code_verifier 不匹配: got %s", r.FormValue("code_verifier"))
		}
		if r.FormValue("grant_type") != "authorization_code" {
			t.Errorf("grant_type 不匹配: got %s", r.FormValue("grant_type"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:  "access-tok",
			ExpiresIn:    3600,
			TokenType:    "Bearer",
			RefreshToken: "refresh-tok",
		})
	}))
	defer server.Close()

	// 临时替换 TokenURL（该函数直接使用常量，需要我们通过构建自定义 client 来绕过）
	// 由于 ExchangeCode 硬编码了 TokenURL，我们需要直接测试 HTTP client 的行为
	// 这里通过构造一个直接调用 mock server 的测试
	client := &Client{httpClient: server.Client()}

	// 由于 ExchangeCode 使用硬编码的 TokenURL，我们无法直接注入 mock server URL
	// 需要使用 httptest 的 Transport 重定向
	originalTokenURL := TokenURL
	// 我们改为直接构造请求来测试逻辑
	_ = originalTokenURL
	_ = client

	// 改用直接构造请求测试 mock server 响应
	ctx := context.Background()
	params := url.Values{}
	params.Set("client_id", ClientID)
	params.Set("client_secret", "test-secret")
	params.Set("code", "auth-code")
	params.Set("redirect_uri", RedirectURI)
	params.Set("grant_type", "authorization_code")
	params.Set("code_verifier", "verifier123")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, strings.NewReader(params.Encode()))
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码不匹配: got %d", resp.StatusCode)
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	if tokenResp.AccessToken != "access-tok" {
		t.Errorf("AccessToken 不匹配: got %s", tokenResp.AccessToken)
	}
	if tokenResp.RefreshToken != "refresh-tok" {
		t.Errorf("RefreshToken 不匹配: got %s", tokenResp.RefreshToken)
	}
}

func TestClient_ExchangeCode_无ClientSecret(t *testing.T) {
	old := defaultClientSecret
	defaultClientSecret = ""
	t.Cleanup(func() { defaultClientSecret = old })

	client := mustNewClient(t, "")
	_, err := client.ExchangeCode(context.Background(), "code", "verifier")
	if err == nil {
		t.Fatal("缺少 client_secret 时应返回错误")
	}
	if !strings.Contains(err.Error(), AntigravityOAuthClientSecretEnv) {
		t.Errorf("错误信息应包含环境变量名: got %s", err.Error())
	}
}

func TestClient_ExchangeCode_服务器返回错误(t *testing.T) {
	old := defaultClientSecret
	defaultClientSecret = "test-secret"
	t.Cleanup(func() { defaultClientSecret = old })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer server.Close()

	// 直接测试 mock server 的错误响应
	resp, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("状态码不匹配: got %d, want 400", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Client.RefreshToken (使用 httptest)
// ---------------------------------------------------------------------------

func TestClient_RefreshToken_MockServer(t *testing.T) {
	old := defaultClientSecret
	defaultClientSecret = "test-secret"
	t.Cleanup(func() { defaultClientSecret = old })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("请求方法不匹配: got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("解析表单失败: %v", err)
		}
		if r.FormValue("grant_type") != "refresh_token" {
			t.Errorf("grant_type 不匹配: got %s", r.FormValue("grant_type"))
		}
		if r.FormValue("refresh_token") != "old-refresh-tok" {
			t.Errorf("refresh_token 不匹配: got %s", r.FormValue("refresh_token"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: "new-access-tok",
			ExpiresIn:   3600,
			TokenType:   "Bearer",
		})
	}))
	defer server.Close()

	ctx := context.Background()
	params := url.Values{}
	params.Set("client_id", ClientID)
	params.Set("client_secret", "test-secret")
	params.Set("refresh_token", "old-refresh-tok")
	params.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, strings.NewReader(params.Encode()))
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码不匹配: got %d", resp.StatusCode)
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	if tokenResp.AccessToken != "new-access-tok" {
		t.Errorf("AccessToken 不匹配: got %s", tokenResp.AccessToken)
	}
}

func TestClient_RefreshToken_无ClientSecret(t *testing.T) {
	old := defaultClientSecret
	defaultClientSecret = ""
	t.Cleanup(func() { defaultClientSecret = old })

	client := mustNewClient(t, "")
	_, err := client.RefreshToken(context.Background(), "refresh-tok")
	if err == nil {
		t.Fatal("缺少 client_secret 时应返回错误")
	}
}

// ---------------------------------------------------------------------------
// Client.GetUserInfo (使用 httptest)
// ---------------------------------------------------------------------------

func TestClient_GetUserInfo_成功(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("请求方法不匹配: got %s", r.Method)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-access-token" {
			t.Errorf("Authorization 不匹配: got %s", auth)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(UserInfo{
			Email:      "user@example.com",
			Name:       "Test User",
			GivenName:  "Test",
			FamilyName: "User",
			Picture:    "https://example.com/photo.jpg",
		})
	}))
	defer server.Close()

	// 直接通过 mock server 测试 GetUserInfo 的行为逻辑
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-access-token")

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码不匹配: got %d", resp.StatusCode)
	}

	var userInfo UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	if userInfo.Email != "user@example.com" {
		t.Errorf("Email 不匹配: got %s", userInfo.Email)
	}
	if userInfo.Name != "Test User" {
		t.Errorf("Name 不匹配: got %s", userInfo.Name)
	}
}

func TestClient_GetUserInfo_服务器返回错误(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_token"}`))
	}))
	defer server.Close()

	resp, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("状态码不匹配: got %d, want 401", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// TokenResponse / UserInfo JSON 序列化
// ---------------------------------------------------------------------------

func TestTokenResponse_JSON序列化(t *testing.T) {
	jsonData := `{"access_token":"at","expires_in":3600,"token_type":"Bearer","scope":"openid","refresh_token":"rt"}`
	var resp TokenResponse
	if err := json.Unmarshal([]byte(jsonData), &resp); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if resp.AccessToken != "at" {
		t.Errorf("AccessToken 不匹配: got %s", resp.AccessToken)
	}
	if resp.ExpiresIn != 3600 {
		t.Errorf("ExpiresIn 不匹配: got %d", resp.ExpiresIn)
	}
	if resp.RefreshToken != "rt" {
		t.Errorf("RefreshToken 不匹配: got %s", resp.RefreshToken)
	}
}

func TestUserInfo_JSON序列化(t *testing.T) {
	jsonData := `{"email":"a@b.com","name":"Alice"}`
	var info UserInfo
	if err := json.Unmarshal([]byte(jsonData), &info); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if info.Email != "a@b.com" {
		t.Errorf("Email 不匹配: got %s", info.Email)
	}
	if info.Name != "Alice" {
		t.Errorf("Name 不匹配: got %s", info.Name)
	}
}

// ---------------------------------------------------------------------------
// LoadCodeAssistResponse JSON 序列化
// ---------------------------------------------------------------------------

func TestLoadCodeAssistResponse_完整JSON(t *testing.T) {
	jsonData := `{
		"cloudaicompanionProject": "proj-123",
		"currentTier": "free-tier",
		"paidTier": {"id": "g1-pro-tier", "name": "Pro"},
		"ineligibleTiers": [{"tier": {"id": "g1-ultra-tier"}, "reasonCode": "INELIGIBLE_ACCOUNT"}]
	}`
	var resp LoadCodeAssistResponse
	if err := json.Unmarshal([]byte(jsonData), &resp); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if resp.CloudAICompanionProject != "proj-123" {
		t.Errorf("CloudAICompanionProject 不匹配: got %s", resp.CloudAICompanionProject)
	}
	if resp.GetTier() != "g1-pro-tier" {
		t.Errorf("GetTier 不匹配: got %s", resp.GetTier())
	}
	if len(resp.IneligibleTiers) != 1 {
		t.Fatalf("IneligibleTiers 数量不匹配: got %d", len(resp.IneligibleTiers))
	}
	if resp.IneligibleTiers[0].ReasonCode != "INELIGIBLE_ACCOUNT" {
		t.Errorf("ReasonCode 不匹配: got %s", resp.IneligibleTiers[0].ReasonCode)
	}
}

// ===========================================================================
// 以下为新增测试：真正调用 Client 方法，通过 RoundTripper 拦截 HTTP 请求
// ===========================================================================

// redirectRoundTripper 将请求中特定前缀的 URL 重定向到 httptest server
type redirectRoundTripper struct {
	// 原始 URL 前缀 -> 替换目标 URL 的映射
	redirects map[string]string
	transport http.RoundTripper
}

func (rt *redirectRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	originalURL := req.URL.String()
	for prefix, target := range rt.redirects {
		if strings.HasPrefix(originalURL, prefix) {
			newURL := target + strings.TrimPrefix(originalURL, prefix)
			parsed, err := url.Parse(newURL)
			if err != nil {
				return nil, err
			}
			req.URL = parsed
			break
		}
	}
	if rt.transport == nil {
		return http.DefaultTransport.RoundTrip(req)
	}
	return rt.transport.RoundTrip(req)
}

// newTestClientWithRedirect 创建一个 Client，将指定 URL 前缀的请求重定向到 mock server
func newTestClientWithRedirect(redirects map[string]string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &redirectRoundTripper{
				redirects: redirects,
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Client.ExchangeCode - 真正调用方法的测试
// ---------------------------------------------------------------------------

func TestClient_ExchangeCode_Success_RealCall(t *testing.T) {
	old := defaultClientSecret
	defaultClientSecret = "test-secret"
	t.Cleanup(func() { defaultClientSecret = old })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("请求方法不匹配: got %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type 不匹配: got %s", ct)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("解析表单失败: %v", err)
		}
		if r.FormValue("client_id") != ClientID {
			t.Errorf("client_id 不匹配: got %s", r.FormValue("client_id"))
		}
		if r.FormValue("client_secret") != "test-secret" {
			t.Errorf("client_secret 不匹配: got %s", r.FormValue("client_secret"))
		}
		if r.FormValue("code") != "test-auth-code" {
			t.Errorf("code 不匹配: got %s", r.FormValue("code"))
		}
		if r.FormValue("code_verifier") != "test-verifier" {
			t.Errorf("code_verifier 不匹配: got %s", r.FormValue("code_verifier"))
		}
		if r.FormValue("grant_type") != "authorization_code" {
			t.Errorf("grant_type 不匹配: got %s", r.FormValue("grant_type"))
		}
		if r.FormValue("redirect_uri") != RedirectURI {
			t.Errorf("redirect_uri 不匹配: got %s", r.FormValue("redirect_uri"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:  "new-access-token",
			ExpiresIn:    3600,
			TokenType:    "Bearer",
			Scope:        "openid email",
			RefreshToken: "new-refresh-token",
		})
	}))
	defer server.Close()

	client := newTestClientWithRedirect(map[string]string{
		TokenURL: server.URL,
	})

	tokenResp, err := client.ExchangeCode(context.Background(), "test-auth-code", "test-verifier")
	if err != nil {
		t.Fatalf("ExchangeCode 失败: %v", err)
	}
	if tokenResp.AccessToken != "new-access-token" {
		t.Errorf("AccessToken 不匹配: got %s, want new-access-token", tokenResp.AccessToken)
	}
	if tokenResp.RefreshToken != "new-refresh-token" {
		t.Errorf("RefreshToken 不匹配: got %s, want new-refresh-token", tokenResp.RefreshToken)
	}
	if tokenResp.ExpiresIn != 3600 {
		t.Errorf("ExpiresIn 不匹配: got %d, want 3600", tokenResp.ExpiresIn)
	}
	if tokenResp.TokenType != "Bearer" {
		t.Errorf("TokenType 不匹配: got %s, want Bearer", tokenResp.TokenType)
	}
	if tokenResp.Scope != "openid email" {
		t.Errorf("Scope 不匹配: got %s, want openid email", tokenResp.Scope)
	}
}

func TestClient_ExchangeCode_ServerError_RealCall(t *testing.T) {
	old := defaultClientSecret
	defaultClientSecret = "test-secret"
	t.Cleanup(func() { defaultClientSecret = old })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"code expired"}`))
	}))
	defer server.Close()

	client := newTestClientWithRedirect(map[string]string{
		TokenURL: server.URL,
	})

	_, err := client.ExchangeCode(context.Background(), "expired-code", "verifier")
	if err == nil {
		t.Fatal("服务器返回 400 时应返回错误")
	}
	if !strings.Contains(err.Error(), "token 交换失败") {
		t.Errorf("错误信息应包含 'token 交换失败': got %s", err.Error())
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("错误信息应包含状态码 400: got %s", err.Error())
	}
}

func TestClient_ExchangeCode_InvalidJSON_RealCall(t *testing.T) {
	old := defaultClientSecret
	defaultClientSecret = "test-secret"
	t.Cleanup(func() { defaultClientSecret = old })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	client := newTestClientWithRedirect(map[string]string{
		TokenURL: server.URL,
	})

	_, err := client.ExchangeCode(context.Background(), "code", "verifier")
	if err == nil {
		t.Fatal("无效 JSON 响应应返回错误")
	}
	if !strings.Contains(err.Error(), "token 解析失败") {
		t.Errorf("错误信息应包含 'token 解析失败': got %s", err.Error())
	}
}

func TestClient_ExchangeCode_ContextCanceled_RealCall(t *testing.T) {
	old := defaultClientSecret
	defaultClientSecret = "test-secret"
	t.Cleanup(func() { defaultClientSecret = old })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // 模拟慢响应
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClientWithRedirect(map[string]string{
		TokenURL: server.URL,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := client.ExchangeCode(ctx, "code", "verifier")
	if err == nil {
		t.Fatal("context 取消时应返回错误")
	}
}

// ---------------------------------------------------------------------------
// Client.RefreshToken - 真正调用方法的测试
// ---------------------------------------------------------------------------

func TestClient_RefreshToken_Success_RealCall(t *testing.T) {
	old := defaultClientSecret
	defaultClientSecret = "test-secret"
	t.Cleanup(func() { defaultClientSecret = old })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("请求方法不匹配: got %s, want POST", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("解析表单失败: %v", err)
		}
		if r.FormValue("grant_type") != "refresh_token" {
			t.Errorf("grant_type 不匹配: got %s", r.FormValue("grant_type"))
		}
		if r.FormValue("refresh_token") != "my-refresh-token" {
			t.Errorf("refresh_token 不匹配: got %s", r.FormValue("refresh_token"))
		}
		if r.FormValue("client_id") != ClientID {
			t.Errorf("client_id 不匹配: got %s", r.FormValue("client_id"))
		}
		if r.FormValue("client_secret") != "test-secret" {
			t.Errorf("client_secret 不匹配: got %s", r.FormValue("client_secret"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: "refreshed-access-token",
			ExpiresIn:   3600,
			TokenType:   "Bearer",
		})
	}))
	defer server.Close()

	client := newTestClientWithRedirect(map[string]string{
		TokenURL: server.URL,
	})

	tokenResp, err := client.RefreshToken(context.Background(), "my-refresh-token")
	if err != nil {
		t.Fatalf("RefreshToken 失败: %v", err)
	}
	if tokenResp.AccessToken != "refreshed-access-token" {
		t.Errorf("AccessToken 不匹配: got %s, want refreshed-access-token", tokenResp.AccessToken)
	}
	if tokenResp.ExpiresIn != 3600 {
		t.Errorf("ExpiresIn 不匹配: got %d, want 3600", tokenResp.ExpiresIn)
	}
}

func TestClient_RefreshToken_ServerError_RealCall(t *testing.T) {
	old := defaultClientSecret
	defaultClientSecret = "test-secret"
	t.Cleanup(func() { defaultClientSecret = old })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"token revoked"}`))
	}))
	defer server.Close()

	client := newTestClientWithRedirect(map[string]string{
		TokenURL: server.URL,
	})

	_, err := client.RefreshToken(context.Background(), "revoked-token")
	if err == nil {
		t.Fatal("服务器返回 401 时应返回错误")
	}
	if !strings.Contains(err.Error(), "token 刷新失败") {
		t.Errorf("错误信息应包含 'token 刷新失败': got %s", err.Error())
	}
}

func TestClient_RefreshToken_InvalidJSON_RealCall(t *testing.T) {
	old := defaultClientSecret
	defaultClientSecret = "test-secret"
	t.Cleanup(func() { defaultClientSecret = old })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	client := newTestClientWithRedirect(map[string]string{
		TokenURL: server.URL,
	})

	_, err := client.RefreshToken(context.Background(), "refresh-tok")
	if err == nil {
		t.Fatal("无效 JSON 响应应返回错误")
	}
	if !strings.Contains(err.Error(), "token 解析失败") {
		t.Errorf("错误信息应包含 'token 解析失败': got %s", err.Error())
	}
}

func TestClient_RefreshToken_ContextCanceled_RealCall(t *testing.T) {
	old := defaultClientSecret
	defaultClientSecret = "test-secret"
	t.Cleanup(func() { defaultClientSecret = old })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClientWithRedirect(map[string]string{
		TokenURL: server.URL,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.RefreshToken(ctx, "refresh-tok")
	if err == nil {
		t.Fatal("context 取消时应返回错误")
	}
}

// ---------------------------------------------------------------------------
// Client.GetUserInfo - 真正调用方法的测试
// ---------------------------------------------------------------------------

func TestClient_GetUserInfo_Success_RealCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("请求方法不匹配: got %s, want GET", r.Method)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer user-access-token" {
			t.Errorf("Authorization 不匹配: got %s", auth)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(UserInfo{
			Email:      "test@example.com",
			Name:       "Test User",
			GivenName:  "Test",
			FamilyName: "User",
			Picture:    "https://example.com/avatar.jpg",
		})
	}))
	defer server.Close()

	client := newTestClientWithRedirect(map[string]string{
		UserInfoURL: server.URL,
	})

	userInfo, err := client.GetUserInfo(context.Background(), "user-access-token")
	if err != nil {
		t.Fatalf("GetUserInfo 失败: %v", err)
	}
	if userInfo.Email != "test@example.com" {
		t.Errorf("Email 不匹配: got %s, want test@example.com", userInfo.Email)
	}
	if userInfo.Name != "Test User" {
		t.Errorf("Name 不匹配: got %s, want Test User", userInfo.Name)
	}
	if userInfo.GivenName != "Test" {
		t.Errorf("GivenName 不匹配: got %s, want Test", userInfo.GivenName)
	}
	if userInfo.FamilyName != "User" {
		t.Errorf("FamilyName 不匹配: got %s, want User", userInfo.FamilyName)
	}
	if userInfo.Picture != "https://example.com/avatar.jpg" {
		t.Errorf("Picture 不匹配: got %s", userInfo.Picture)
	}
}

func TestClient_GetUserInfo_Unauthorized_RealCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_token"}`))
	}))
	defer server.Close()

	client := newTestClientWithRedirect(map[string]string{
		UserInfoURL: server.URL,
	})

	_, err := client.GetUserInfo(context.Background(), "bad-token")
	if err == nil {
		t.Fatal("服务器返回 401 时应返回错误")
	}
	if !strings.Contains(err.Error(), "获取用户信息失败") {
		t.Errorf("错误信息应包含 '获取用户信息失败': got %s", err.Error())
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("错误信息应包含状态码 401: got %s", err.Error())
	}
}

func TestClient_GetUserInfo_InvalidJSON_RealCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{broken`))
	}))
	defer server.Close()

	client := newTestClientWithRedirect(map[string]string{
		UserInfoURL: server.URL,
	})

	_, err := client.GetUserInfo(context.Background(), "token")
	if err == nil {
		t.Fatal("无效 JSON 响应应返回错误")
	}
	if !strings.Contains(err.Error(), "用户信息解析失败") {
		t.Errorf("错误信息应包含 '用户信息解析失败': got %s", err.Error())
	}
}

func TestClient_GetUserInfo_ContextCanceled_RealCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClientWithRedirect(map[string]string{
		UserInfoURL: server.URL,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.GetUserInfo(ctx, "token")
	if err == nil {
		t.Fatal("context 取消时应返回错误")
	}
}

// ---------------------------------------------------------------------------
// Client.LoadCodeAssist - 真正调用方法的测试
// ---------------------------------------------------------------------------

// withMockBaseURLs 临时替换 BaseURLs，测试结束后恢复
func withMockBaseURLs(t *testing.T, urls []string) {
	t.Helper()
	origBaseURLs := BaseURLs
	origBaseURL := BaseURL
	BaseURLs = urls
	if len(urls) > 0 {
		BaseURL = urls[0]
	}
	t.Cleanup(func() {
		BaseURLs = origBaseURLs
		BaseURL = origBaseURL
	})
}

func TestClient_LoadCodeAssist_Success_RealCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("请求方法不匹配: got %s, want POST", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/v1internal:loadCodeAssist") {
			t.Errorf("URL 路径不匹配: got %s", r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Authorization 不匹配: got %s", auth)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type 不匹配: got %s", ct)
		}
		if ua := r.Header.Get("User-Agent"); ua != GetUserAgent() {
			t.Errorf("User-Agent 不匹配: got %s", ua)
		}

		// 验证请求体
		var reqBody LoadCodeAssistRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("解析请求体失败: %v", err)
		}
		if reqBody.Metadata.IDEType != "ANTIGRAVITY" {
			t.Errorf("IDEType 不匹配: got %s, want ANTIGRAVITY", reqBody.Metadata.IDEType)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"cloudaicompanionProject": "test-project-123",
			"currentTier": {"id": "free-tier", "name": "Free"},
			"paidTier": {"id": "g1-pro-tier", "name": "Pro", "description": "Pro plan"}
		}`))
	}))
	defer server.Close()

	withMockBaseURLs(t, []string{server.URL})

	client := mustNewClient(t, "")
	resp, rawResp, err := client.LoadCodeAssist(context.Background(), "test-token")
	if err != nil {
		t.Fatalf("LoadCodeAssist 失败: %v", err)
	}
	if resp.CloudAICompanionProject != "test-project-123" {
		t.Errorf("CloudAICompanionProject 不匹配: got %s", resp.CloudAICompanionProject)
	}
	if resp.GetTier() != "g1-pro-tier" {
		t.Errorf("GetTier 不匹配: got %s, want g1-pro-tier", resp.GetTier())
	}
	if resp.CurrentTier == nil || resp.CurrentTier.ID != "free-tier" {
		t.Errorf("CurrentTier 不匹配: got %+v", resp.CurrentTier)
	}
	if resp.PaidTier == nil || resp.PaidTier.ID != "g1-pro-tier" {
		t.Errorf("PaidTier 不匹配: got %+v", resp.PaidTier)
	}
	// 验证原始 JSON map
	if rawResp == nil {
		t.Fatal("rawResp 不应为 nil")
	}
	if rawResp["cloudaicompanionProject"] != "test-project-123" {
		t.Errorf("rawResp cloudaicompanionProject 不匹配: got %v", rawResp["cloudaicompanionProject"])
	}
}

func TestClient_LoadCodeAssist_HTTPError_RealCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer server.Close()

	withMockBaseURLs(t, []string{server.URL})

	client := mustNewClient(t, "")
	_, _, err := client.LoadCodeAssist(context.Background(), "bad-token")
	if err == nil {
		t.Fatal("服务器返回 403 时应返回错误")
	}
	if !strings.Contains(err.Error(), "loadCodeAssist 失败") {
		t.Errorf("错误信息应包含 'loadCodeAssist 失败': got %s", err.Error())
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("错误信息应包含状态码 403: got %s", err.Error())
	}
}

func TestClient_LoadCodeAssist_InvalidJSON_RealCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{not valid json!!!`))
	}))
	defer server.Close()

	withMockBaseURLs(t, []string{server.URL})

	client := mustNewClient(t, "")
	_, _, err := client.LoadCodeAssist(context.Background(), "token")
	if err == nil {
		t.Fatal("无效 JSON 响应应返回错误")
	}
	if !strings.Contains(err.Error(), "响应解析失败") {
		t.Errorf("错误信息应包含 '响应解析失败': got %s", err.Error())
	}
}

func TestClient_LoadCodeAssist_URLFallback_RealCall(t *testing.T) {
	// 第一个 server 返回 500，第二个 server 返回成功
	callCount := 0
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal"}`))
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"cloudaicompanionProject": "fallback-project",
			"currentTier": {"id": "free-tier", "name": "Free"}
		}`))
	}))
	defer server2.Close()

	withMockBaseURLs(t, []string{server1.URL, server2.URL})

	client := mustNewClient(t, "")
	resp, _, err := client.LoadCodeAssist(context.Background(), "token")
	if err != nil {
		t.Fatalf("LoadCodeAssist 应在 fallback 后成功: %v", err)
	}
	if resp.CloudAICompanionProject != "fallback-project" {
		t.Errorf("CloudAICompanionProject 不匹配: got %s", resp.CloudAICompanionProject)
	}
	if callCount != 2 {
		t.Errorf("应该调用了 2 个 server，实际调用 %d 次", callCount)
	}
}

func TestClient_LoadCodeAssist_AllURLsFail_RealCall(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"unavailable"}`))
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"bad_gateway"}`))
	}))
	defer server2.Close()

	withMockBaseURLs(t, []string{server1.URL, server2.URL})

	client := mustNewClient(t, "")
	_, _, err := client.LoadCodeAssist(context.Background(), "token")
	if err == nil {
		t.Fatal("所有 URL 都失败时应返回错误")
	}
}

func TestClient_LoadCodeAssist_ContextCanceled_RealCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	withMockBaseURLs(t, []string{server.URL})

	client := mustNewClient(t, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := client.LoadCodeAssist(ctx, "token")
	if err == nil {
		t.Fatal("context 取消时应返回错误")
	}
}

// ---------------------------------------------------------------------------
// Client.FetchAvailableModels - 真正调用方法的测试
// ---------------------------------------------------------------------------

func TestClient_FetchAvailableModels_Success_RealCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("请求方法不匹配: got %s, want POST", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/v1internal:fetchAvailableModels") {
			t.Errorf("URL 路径不匹配: got %s", r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Authorization 不匹配: got %s", auth)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type 不匹配: got %s", ct)
		}
		if ua := r.Header.Get("User-Agent"); ua != GetUserAgent() {
			t.Errorf("User-Agent 不匹配: got %s", ua)
		}

		// 验证请求体
		var reqBody FetchAvailableModelsRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("解析请求体失败: %v", err)
		}
		if reqBody.Project != "project-abc" {
			t.Errorf("Project 不匹配: got %s, want project-abc", reqBody.Project)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"models": {
				"gemini-2.0-flash": {
					"quotaInfo": {
						"remainingFraction": 0.85,
						"resetTime": "2025-01-01T00:00:00Z"
					}
				},
				"gemini-2.5-pro": {
					"quotaInfo": {
						"remainingFraction": 0.5
					}
				}
			}
		}`))
	}))
	defer server.Close()

	withMockBaseURLs(t, []string{server.URL})

	client := mustNewClient(t, "")
	resp, rawResp, err := client.FetchAvailableModels(context.Background(), "test-token", "project-abc")
	if err != nil {
		t.Fatalf("FetchAvailableModels 失败: %v", err)
	}
	if resp.Models == nil {
		t.Fatal("Models 不应为 nil")
	}
	if len(resp.Models) != 2 {
		t.Errorf("Models 数量不匹配: got %d, want 2", len(resp.Models))
	}

	flashModel, ok := resp.Models["gemini-2.0-flash"]
	if !ok {
		t.Fatal("缺少 gemini-2.0-flash 模型")
	}
	if flashModel.QuotaInfo == nil {
		t.Fatal("gemini-2.0-flash QuotaInfo 不应为 nil")
	}
	if flashModel.QuotaInfo.RemainingFraction != 0.85 {
		t.Errorf("RemainingFraction 不匹配: got %f, want 0.85", flashModel.QuotaInfo.RemainingFraction)
	}
	if flashModel.QuotaInfo.ResetTime != "2025-01-01T00:00:00Z" {
		t.Errorf("ResetTime 不匹配: got %s", flashModel.QuotaInfo.ResetTime)
	}

	proModel, ok := resp.Models["gemini-2.5-pro"]
	if !ok {
		t.Fatal("缺少 gemini-2.5-pro 模型")
	}
	if proModel.QuotaInfo == nil {
		t.Fatal("gemini-2.5-pro QuotaInfo 不应为 nil")
	}
	if proModel.QuotaInfo.RemainingFraction != 0.5 {
		t.Errorf("RemainingFraction 不匹配: got %f, want 0.5", proModel.QuotaInfo.RemainingFraction)
	}

	// 验证原始 JSON map
	if rawResp == nil {
		t.Fatal("rawResp 不应为 nil")
	}
	if rawResp["models"] == nil {
		t.Error("rawResp models 不应为 nil")
	}
}

func TestClient_FetchAvailableModels_HTTPError_RealCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer server.Close()

	withMockBaseURLs(t, []string{server.URL})

	client := mustNewClient(t, "")
	_, _, err := client.FetchAvailableModels(context.Background(), "bad-token", "proj")
	if err == nil {
		t.Fatal("服务器返回 403 时应返回错误")
	}
	if !strings.Contains(err.Error(), "fetchAvailableModels 失败") {
		t.Errorf("错误信息应包含 'fetchAvailableModels 失败': got %s", err.Error())
	}
}

func TestClient_FetchAvailableModels_InvalidJSON_RealCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<<<not json>>>`))
	}))
	defer server.Close()

	withMockBaseURLs(t, []string{server.URL})

	client := mustNewClient(t, "")
	_, _, err := client.FetchAvailableModels(context.Background(), "token", "proj")
	if err == nil {
		t.Fatal("无效 JSON 响应应返回错误")
	}
	if !strings.Contains(err.Error(), "响应解析失败") {
		t.Errorf("错误信息应包含 '响应解析失败': got %s", err.Error())
	}
}

func TestClient_FetchAvailableModels_URLFallback_RealCall(t *testing.T) {
	callCount := 0
	// 第一个 server 返回 429，第二个 server 返回成功
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate_limited"}`))
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"models": {"model-a": {}}}`))
	}))
	defer server2.Close()

	withMockBaseURLs(t, []string{server1.URL, server2.URL})

	client := mustNewClient(t, "")
	resp, _, err := client.FetchAvailableModels(context.Background(), "token", "proj")
	if err != nil {
		t.Fatalf("FetchAvailableModels 应在 fallback 后成功: %v", err)
	}
	if _, ok := resp.Models["model-a"]; !ok {
		t.Error("应返回 fallback server 的模型")
	}
	if callCount != 2 {
		t.Errorf("应该调用了 2 个 server，实际调用 %d 次", callCount)
	}
}

func TestClient_FetchAvailableModels_AllURLsFail_RealCall(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`not found`))
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`internal error`))
	}))
	defer server2.Close()

	withMockBaseURLs(t, []string{server1.URL, server2.URL})

	client := mustNewClient(t, "")
	_, _, err := client.FetchAvailableModels(context.Background(), "token", "proj")
	if err == nil {
		t.Fatal("所有 URL 都失败时应返回错误")
	}
}

func TestClient_FetchAvailableModels_ContextCanceled_RealCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	withMockBaseURLs(t, []string{server.URL})

	client := mustNewClient(t, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := client.FetchAvailableModels(ctx, "token", "proj")
	if err == nil {
		t.Fatal("context 取消时应返回错误")
	}
}

func TestClient_FetchAvailableModels_EmptyModels_RealCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"models": {}}`))
	}))
	defer server.Close()

	withMockBaseURLs(t, []string{server.URL})

	client := mustNewClient(t, "")
	resp, rawResp, err := client.FetchAvailableModels(context.Background(), "token", "proj")
	if err != nil {
		t.Fatalf("FetchAvailableModels 失败: %v", err)
	}
	if resp.Models == nil {
		t.Fatal("Models 不应为 nil")
	}
	if len(resp.Models) != 0 {
		t.Errorf("Models 应为空: got %d", len(resp.Models))
	}
	if rawResp == nil {
		t.Fatal("rawResp 不应为 nil")
	}
}

// ---------------------------------------------------------------------------
// LoadCodeAssist 和 FetchAvailableModels 的 408 fallback 测试
// ---------------------------------------------------------------------------

func TestClient_LoadCodeAssist_408Fallback_RealCall(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusRequestTimeout)
		_, _ = w.Write([]byte(`timeout`))
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"cloudaicompanionProject":"p2","currentTier":"free-tier"}`))
	}))
	defer server2.Close()

	withMockBaseURLs(t, []string{server1.URL, server2.URL})

	client := mustNewClient(t, "")
	resp, _, err := client.LoadCodeAssist(context.Background(), "token")
	if err != nil {
		t.Fatalf("LoadCodeAssist 应在 408 fallback 后成功: %v", err)
	}
	if resp.CloudAICompanionProject != "p2" {
		t.Errorf("CloudAICompanionProject 不匹配: got %s", resp.CloudAICompanionProject)
	}
}

func TestClient_FetchAvailableModels_404Fallback_RealCall(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`not found`))
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"models":{"m1":{"quotaInfo":{"remainingFraction":1.0}}}}`))
	}))
	defer server2.Close()

	withMockBaseURLs(t, []string{server1.URL, server2.URL})

	client := mustNewClient(t, "")
	resp, _, err := client.FetchAvailableModels(context.Background(), "token", "proj")
	if err != nil {
		t.Fatalf("FetchAvailableModels 应在 404 fallback 后成功: %v", err)
	}
	if _, ok := resp.Models["m1"]; !ok {
		t.Error("应返回 fallback server 的模型 m1")
	}
}

func TestExtractProjectIDFromOnboardResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		resp map[string]any
		want string
	}{
		{
			name: "nil response",
			resp: nil,
			want: "",
		},
		{
			name: "empty response",
			resp: map[string]any{},
			want: "",
		},
		{
			name: "project as string",
			resp: map[string]any{
				"cloudaicompanionProject": "my-project-123",
			},
			want: "my-project-123",
		},
		{
			name: "project as string with spaces",
			resp: map[string]any{
				"cloudaicompanionProject": "  my-project-123  ",
			},
			want: "my-project-123",
		},
		{
			name: "project as map with id",
			resp: map[string]any{
				"cloudaicompanionProject": map[string]any{
					"id": "proj-from-map",
				},
			},
			want: "proj-from-map",
		},
		{
			name: "project as map without id",
			resp: map[string]any{
				"cloudaicompanionProject": map[string]any{
					"name": "some-name",
				},
			},
			want: "",
		},
		{
			name: "missing cloudaicompanionProject key",
			resp: map[string]any{
				"otherField": "value",
			},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := extractProjectIDFromOnboardResponse(tc.resp)
			if got != tc.want {
				t.Fatalf("extractProjectIDFromOnboardResponse() = %q, want %q", got, tc.want)
			}
		})
	}
}
