//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

// 编译期接口断言
var _ HTTPUpstream = (*stubAntigravityUpstream)(nil)
var _ HTTPUpstream = (*recordingOKUpstream)(nil)
var _ AccountRepository = (*stubAntigravityAccountRepo)(nil)
var _ SchedulerCache = (*stubSchedulerCache)(nil)

type stubAntigravityUpstream struct {
	firstBase  string
	secondBase string
	calls      []string
}

type recordingOKUpstream struct {
	calls int
}

func (r *recordingOKUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	r.calls++
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("ok")),
	}, nil
}

func (r *recordingOKUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, enableTLSFingerprint bool) (*http.Response, error) {
	return r.Do(req, proxyURL, accountID, accountConcurrency)
}

func (s *stubAntigravityUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	url := req.URL.String()
	s.calls = append(s.calls, url)
	if strings.HasPrefix(url, s.firstBase) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"Resource has been exhausted"}}`)),
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("ok")),
	}, nil
}

func (s *stubAntigravityUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, enableTLSFingerprint bool) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

type rateLimitCall struct {
	accountID int64
	resetAt   time.Time
}

type modelRateLimitCall struct {
	accountID int64
	modelKey  string // 存储的 key（应该是官方模型 ID，如 "claude-sonnet-4-5"）
	resetAt   time.Time
}

type extraUpdateCall struct {
	accountID int64
	updates   map[string]any
}

type stubAntigravityAccountRepo struct {
	AccountRepository
	rateCalls           []rateLimitCall
	modelRateLimitCalls []modelRateLimitCall
	extraUpdateCalls    []extraUpdateCall
}

func (s *stubAntigravityAccountRepo) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	s.rateCalls = append(s.rateCalls, rateLimitCall{accountID: id, resetAt: resetAt})
	return nil
}

func (s *stubAntigravityAccountRepo) SetModelRateLimit(ctx context.Context, id int64, modelKey string, resetAt time.Time) error {
	s.modelRateLimitCalls = append(s.modelRateLimitCalls, modelRateLimitCall{accountID: id, modelKey: modelKey, resetAt: resetAt})
	return nil
}

func (s *stubAntigravityAccountRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	s.extraUpdateCalls = append(s.extraUpdateCalls, extraUpdateCall{accountID: id, updates: updates})
	return nil
}

func TestAntigravityRetryLoop_NoURLFallback_UsesConfiguredBaseURL(t *testing.T) {
	t.Setenv(antigravityForwardBaseURLEnv, "")

	oldBaseURLs := append([]string(nil), antigravity.BaseURLs...)
	oldAvailability := antigravity.DefaultURLAvailability
	defer func() {
		antigravity.BaseURLs = oldBaseURLs
		antigravity.DefaultURLAvailability = oldAvailability
	}()

	base1 := "https://ag-1.test"
	base2 := "https://ag-2.test"
	antigravity.BaseURLs = []string{base1, base2}
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)

	upstream := &stubAntigravityUpstream{firstBase: base1, secondBase: base2}
	account := &Account{
		ID:          1,
		Name:        "acc-1",
		Platform:    PlatformAntigravity,
		Schedulable: true,
		Status:      StatusActive,
		Concurrency: 1,
	}

	var handleErrorCalled bool
	svc := &AntigravityGatewayService{}
	result, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		prefix:         "[test]",
		ctx:            context.Background(),
		account:        account,
		proxyURL:       "",
		accessToken:    "token",
		action:         "generateContent",
		body:           []byte(`{"input":"test"}`),
		httpUpstream:   upstream,
		requestedModel: "claude-sonnet-4-5",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			handleErrorCalled = true
			return nil
		},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.resp)
	defer func() { _ = result.resp.Body.Close() }()
	require.Equal(t, http.StatusTooManyRequests, result.resp.StatusCode)
	require.True(t, handleErrorCalled)
	require.Len(t, upstream.calls, antigravityMaxRetries)
	for _, callURL := range upstream.calls {
		require.True(t, strings.HasPrefix(callURL, base1))
	}

	available := antigravity.DefaultURLAvailability.GetAvailableURLs()
	require.NotEmpty(t, available)
	require.Equal(t, base1, available[0])
}

// TestHandleUpstreamError_429_ModelRateLimit 测试 429 模型限流场景
func TestHandleUpstreamError_429_ModelRateLimit(t *testing.T) {
	repo := &stubAntigravityAccountRepo{}
	svc := &AntigravityGatewayService{accountRepo: repo}
	account := &Account{ID: 1, Name: "acc-1", Platform: PlatformAntigravity}

	// 429 + RATE_LIMIT_EXCEEDED + 模型名 → 模型限流
	body := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-5"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "15s"}
			]
		}
	}`)

	result := svc.handleUpstreamError(context.Background(), "[test]", account, http.StatusTooManyRequests, http.Header{}, body, "claude-sonnet-4-5", 0, "", false)

	// 应该触发模型限流
	require.NotNil(t, result)
	require.True(t, result.Handled)
	require.NotNil(t, result.SwitchError)
	require.Equal(t, "claude-sonnet-4-5", result.SwitchError.RateLimitedModel)
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "claude-sonnet-4-5", repo.modelRateLimitCalls[0].modelKey)
}

// TestHandleUpstreamError_429_NonModelRateLimit 测试 429 非模型限流场景（走模型级限流兜底）
func TestHandleUpstreamError_429_NonModelRateLimit(t *testing.T) {
	repo := &stubAntigravityAccountRepo{}
	svc := &AntigravityGatewayService{accountRepo: repo}
	account := &Account{ID: 2, Name: "acc-2", Platform: PlatformAntigravity}

	// 429 + 普通限流响应（无 RATE_LIMIT_EXCEEDED reason）→ 走模型级限流兜底
	body := buildGeminiRateLimitBody("5s")

	result := svc.handleUpstreamError(context.Background(), "[test]", account, http.StatusTooManyRequests, http.Header{}, body, "claude-sonnet-4-5", 0, "", false)

	// handleModelRateLimit 不会处理（因为没有 RATE_LIMIT_EXCEEDED），
	// 但 429 兜底逻辑会使用 requestedModel 设置模型级限流
	require.Nil(t, result)
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "claude-sonnet-4-5", repo.modelRateLimitCalls[0].modelKey)
}

// TestHandleUpstreamError_429_NonModelRateLimit_UsesMappedModelKey 测试 429 非模型限流场景
// 验证：requestedModel 会被映射到 Antigravity 最终模型（例如 claude-opus-4-6 -> claude-opus-4-6-thinking）
func TestHandleUpstreamError_429_NonModelRateLimit_UsesMappedModelKey(t *testing.T) {
	repo := &stubAntigravityAccountRepo{}
	svc := &AntigravityGatewayService{accountRepo: repo}
	account := &Account{ID: 20, Name: "acc-20", Platform: PlatformAntigravity}

	body := buildGeminiRateLimitBody("5s")

	result := svc.handleUpstreamError(context.Background(), "[test]", account, http.StatusTooManyRequests, http.Header{}, body, "claude-opus-4-6", 0, "", false)

	require.Nil(t, result)
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "claude-opus-4-6-thinking", repo.modelRateLimitCalls[0].modelKey)
}

// TestHandleUpstreamError_503_ModelCapacityExhausted 测试 503 模型容量不足场景
// MODEL_CAPACITY_EXHAUSTED 时应等待重试，不切换账号
func TestHandleUpstreamError_503_ModelCapacityExhausted(t *testing.T) {
	repo := &stubAntigravityAccountRepo{}
	svc := &AntigravityGatewayService{accountRepo: repo}
	account := &Account{ID: 3, Name: "acc-3", Platform: PlatformAntigravity}

	// 503 + MODEL_CAPACITY_EXHAUSTED → 等待重试，不切换账号
	body := []byte(`{
		"error": {
			"status": "UNAVAILABLE",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-pro-high"}, "reason": "MODEL_CAPACITY_EXHAUSTED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "30s"}
			]
		}
	}`)

	result := svc.handleUpstreamError(context.Background(), "[test]", account, http.StatusServiceUnavailable, http.Header{}, body, "gemini-3-pro-high", 0, "", false)

	// MODEL_CAPACITY_EXHAUSTED 应该标记为已处理，不切换账号，不设置模型限流
	// 实际重试由 handleSmartRetry 处理
	require.NotNil(t, result)
	require.True(t, result.Handled)
	require.False(t, result.ShouldRetry, "MODEL_CAPACITY_EXHAUSTED should not trigger retry from handleModelRateLimit path")
	require.Nil(t, result.SwitchError, "MODEL_CAPACITY_EXHAUSTED should not trigger account switch")
	require.Empty(t, repo.modelRateLimitCalls, "MODEL_CAPACITY_EXHAUSTED should not set model rate limit")
}

// TestHandleUpstreamError_503_NonModelRateLimit 测试 503 非模型限流场景（不处理）
func TestHandleUpstreamError_503_NonModelRateLimit(t *testing.T) {
	repo := &stubAntigravityAccountRepo{}
	svc := &AntigravityGatewayService{accountRepo: repo}
	account := &Account{ID: 4, Name: "acc-4", Platform: PlatformAntigravity}

	// 503 + 普通错误（非 MODEL_CAPACITY_EXHAUSTED）→ 不做任何处理
	body := []byte(`{
		"error": {
			"status": "UNAVAILABLE",
			"message": "Service temporarily unavailable",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason": "SERVICE_UNAVAILABLE"}
			]
		}
	}`)

	result := svc.handleUpstreamError(context.Background(), "[test]", account, http.StatusServiceUnavailable, http.Header{}, body, "gemini-3-pro-high", 0, "", false)

	// 503 非模型限流不应该做任何处理
	require.Nil(t, result)
	require.Empty(t, repo.modelRateLimitCalls, "503 non-model rate limit should not trigger model rate limit")
	require.Empty(t, repo.rateCalls, "503 non-model rate limit should not trigger account rate limit")
}

// TestHandleUpstreamError_503_EmptyBody 测试 503 空响应体（不处理）
func TestHandleUpstreamError_503_EmptyBody(t *testing.T) {
	repo := &stubAntigravityAccountRepo{}
	svc := &AntigravityGatewayService{accountRepo: repo}
	account := &Account{ID: 5, Name: "acc-5", Platform: PlatformAntigravity}

	// 503 + 空响应体 → 不做任何处理
	body := []byte(`{}`)

	result := svc.handleUpstreamError(context.Background(), "[test]", account, http.StatusServiceUnavailable, http.Header{}, body, "gemini-3-pro-high", 0, "", false)

	// 503 空响应不应该做任何处理
	require.Nil(t, result)
	require.Empty(t, repo.modelRateLimitCalls)
	require.Empty(t, repo.rateCalls)
}

func TestAccountIsSchedulableForModel_AntigravityRateLimits(t *testing.T) {
	now := time.Now()
	future := now.Add(10 * time.Minute)

	account := &Account{
		ID:          1,
		Name:        "acc",
		Platform:    PlatformAntigravity,
		Status:      StatusActive,
		Schedulable: true,
	}

	account.RateLimitResetAt = &future
	require.False(t, account.IsSchedulableForModel("claude-sonnet-4-5"))
	require.False(t, account.IsSchedulableForModel("gemini-3-flash"))

	account.RateLimitResetAt = nil
	require.True(t, account.IsSchedulableForModel("claude-sonnet-4-5"))
	require.True(t, account.IsSchedulableForModel("gemini-3-flash"))
}

func buildGeminiRateLimitBody(delay string) []byte {
	return []byte(fmt.Sprintf(`{"error":{"message":"too many requests","details":[{"metadata":{"quotaResetDelay":%q}}]}}`, delay))
}

func TestParseGeminiRateLimitResetTime_QuotaResetDelay_RoundsUp(t *testing.T) {
	// Avoid flakiness around Unix second boundaries.
	for {
		now := time.Now()
		if now.Nanosecond() < 800*1e6 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	baseUnix := time.Now().Unix()
	ts := ParseGeminiRateLimitResetTime(buildGeminiRateLimitBody("0.1s"))
	require.NotNil(t, ts)
	require.Equal(t, baseUnix+1, *ts, "fractional seconds should be rounded up to the next second")
}

func TestParseAntigravitySmartRetryInfo(t *testing.T) {
	tests := []struct {
		name                             string
		body                             string
		expectedDelay                    time.Duration
		expectedModel                    string
		expectedNil                      bool
		expectedIsModelCapacityExhausted bool
	}{
		{
			name: "valid complete response with RATE_LIMIT_EXCEEDED",
			body: `{
				"error": {
					"code": 429,
					"details": [
						{
							"@type": "type.googleapis.com/google.rpc.ErrorInfo",
							"domain": "cloudcode-pa.googleapis.com",
							"metadata": {
								"model": "claude-sonnet-4-5",
								"quotaResetDelay": "201.506475ms"
							},
							"reason": "RATE_LIMIT_EXCEEDED"
						},
						{
							"@type": "type.googleapis.com/google.rpc.RetryInfo",
							"retryDelay": "0.201506475s"
						}
					],
					"message": "You have exhausted your capacity on this model.",
					"status": "RESOURCE_EXHAUSTED"
				}
			}`,
			expectedDelay: 201506475 * time.Nanosecond,
			expectedModel: "claude-sonnet-4-5",
		},
		{
			name: "429 RESOURCE_EXHAUSTED without RATE_LIMIT_EXCEEDED - should return nil",
			body: `{
				"error": {
					"code": 429,
					"status": "RESOURCE_EXHAUSTED",
					"details": [
						{
							"@type": "type.googleapis.com/google.rpc.ErrorInfo",
							"metadata": {"model": "claude-sonnet-4-5"},
							"reason": "QUOTA_EXCEEDED"
						},
						{
							"@type": "type.googleapis.com/google.rpc.RetryInfo",
							"retryDelay": "3s"
						}
					]
				}
			}`,
			expectedNil: true,
		},
		{
			name: "503 UNAVAILABLE with MODEL_CAPACITY_EXHAUSTED - long delay",
			body: `{
				"error": {
					"code": 503,
					"status": "UNAVAILABLE",
					"details": [
						{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-pro-high"}, "reason": "MODEL_CAPACITY_EXHAUSTED"},
						{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "39s"}
					],
					"message": "No capacity available for model gemini-3-pro-high on the server"
				}
			}`,
			expectedDelay:                    39 * time.Second,
			expectedModel:                    "gemini-3-pro-high",
			expectedIsModelCapacityExhausted: true,
		},
		{
			name: "503 UNAVAILABLE without MODEL_CAPACITY_EXHAUSTED - should return nil",
			body: `{
				"error": {
					"code": 503,
					"status": "UNAVAILABLE",
					"details": [
						{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-pro"}, "reason": "SERVICE_UNAVAILABLE"},
						{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "5s"}
					]
				}
			}`,
			expectedNil: true,
		},
		{
			name: "wrong status - should return nil",
			body: `{
				"error": {
					"code": 429,
					"status": "INVALID_ARGUMENT",
					"details": [
						{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "3s"}
					]
				}
			}`,
			expectedNil: true,
		},
		{
			name: "missing status - should return nil",
			body: `{
				"error": {
					"code": 429,
					"details": [
						{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "3s"}
					]
				}
			}`,
			expectedNil: true,
		},
		{
			name: "milliseconds format is now supported",
			body: `{
				"error": {
					"code": 429,
					"status": "RESOURCE_EXHAUSTED",
					"details": [
						{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "test-model"}, "reason": "RATE_LIMIT_EXCEEDED"},
						{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "500ms"}
					]
				}
			}`,
			expectedDelay: 500 * time.Millisecond,
			expectedModel: "test-model",
		},
		{
			name: "minutes format is supported",
			body: `{
				"error": {
					"code": 429,
					"status": "RESOURCE_EXHAUSTED",
					"details": [
						{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-pro"}, "reason": "RATE_LIMIT_EXCEEDED"},
						{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "4m50s"}
					]
				}
			}`,
			expectedDelay: 4*time.Minute + 50*time.Second,
			expectedModel: "gemini-3-pro",
		},
		{
			name: "missing model name - should return nil",
			body: `{
				"error": {
					"code": 429,
					"status": "RESOURCE_EXHAUSTED",
					"details": [
						{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason": "RATE_LIMIT_EXCEEDED"},
						{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "3s"}
					]
				}
			}`,
			expectedNil: true,
		},
		{
			name:        "invalid JSON",
			body:        `not json`,
			expectedNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAntigravitySmartRetryInfo([]byte(tt.body))
			if tt.expectedNil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Errorf("expected non-nil result")
				return
			}
			if result.RetryDelay != tt.expectedDelay {
				t.Errorf("RetryDelay = %v, want %v", result.RetryDelay, tt.expectedDelay)
			}
			if result.ModelName != tt.expectedModel {
				t.Errorf("ModelName = %q, want %q", result.ModelName, tt.expectedModel)
			}
			if result.IsModelCapacityExhausted != tt.expectedIsModelCapacityExhausted {
				t.Errorf("IsModelCapacityExhausted = %v, want %v", result.IsModelCapacityExhausted, tt.expectedIsModelCapacityExhausted)
			}
		})
	}
}

func TestShouldTriggerAntigravitySmartRetry(t *testing.T) {
	oauthAccount := &Account{Type: AccountTypeOAuth, Platform: PlatformAntigravity}
	setupTokenAccount := &Account{Type: AccountTypeSetupToken, Platform: PlatformAntigravity}
	upstreamAccount := &Account{Type: AccountTypeUpstream, Platform: PlatformAntigravity}
	apiKeyAccount := &Account{Type: AccountTypeAPIKey}

	tests := []struct {
		name                             string
		account                          *Account
		body                             string
		expectedShouldRetry              bool
		expectedShouldRateLimit          bool
		expectedIsModelCapacityExhausted bool
		minWait                          time.Duration
		modelName                        string
	}{
		{
			name:    "OAuth account with short delay (< 7s) - smart retry",
			account: oauthAccount,
			body: `{
				"error": {
					"status": "RESOURCE_EXHAUSTED",
					"details": [
						{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-opus-4"}, "reason": "RATE_LIMIT_EXCEEDED"},
						{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.5s"}
					]
				}
			}`,
			expectedShouldRetry:     true,
			expectedShouldRateLimit: false,
			minWait:                 1 * time.Second, // 0.5s < 1s, 使用最小等待时间 1s
			modelName:               "claude-opus-4",
		},
		{
			name:    "SetupToken account with short delay - smart retry",
			account: setupTokenAccount,
			body: `{
				"error": {
					"status": "RESOURCE_EXHAUSTED",
					"details": [
						{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-flash"}, "reason": "RATE_LIMIT_EXCEEDED"},
						{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "3s"}
					]
				}
			}`,
			expectedShouldRetry:     true,
			expectedShouldRateLimit: false,
			minWait:                 3 * time.Second,
			modelName:               "gemini-3-flash",
		},
		{
			name:    "OAuth account with long delay (>= 7s) - direct rate limit",
			account: oauthAccount,
			body: `{
				"error": {
					"status": "RESOURCE_EXHAUSTED",
					"details": [
						{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-5"}, "reason": "RATE_LIMIT_EXCEEDED"},
						{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "15s"}
					]
				}
			}`,
			expectedShouldRetry:     false,
			expectedShouldRateLimit: true,
			modelName:               "claude-sonnet-4-5",
		},
		{
			name:    "Upstream account with short delay - smart retry",
			account: upstreamAccount,
			body: `{
				"error": {
					"status": "RESOURCE_EXHAUSTED",
					"details": [
						{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-5"}, "reason": "RATE_LIMIT_EXCEEDED"},
						{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "2s"}
					]
				}
			}`,
			expectedShouldRetry:     true,
			expectedShouldRateLimit: false,
			minWait:                 2 * time.Second,
			modelName:               "claude-sonnet-4-5",
		},
		{
			name:    "API Key account - should not trigger",
			account: apiKeyAccount,
			body: `{
				"error": {
					"status": "RESOURCE_EXHAUSTED",
					"details": [
						{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "test"}, "reason": "RATE_LIMIT_EXCEEDED"},
						{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.5s"}
					]
				}
			}`,
			expectedShouldRetry:     false,
			expectedShouldRateLimit: false,
		},
		{
			name:    "OAuth account with exactly 7s delay - direct rate limit",
			account: oauthAccount,
			body: `{
				"error": {
					"status": "RESOURCE_EXHAUSTED",
					"details": [
						{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-pro"}, "reason": "RATE_LIMIT_EXCEEDED"},
						{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "7s"}
					]
				}
			}`,
			expectedShouldRetry:     false,
			expectedShouldRateLimit: true,
			minWait:                 7 * time.Second,
			modelName:               "gemini-pro",
		},
		{
			name:    "503 UNAVAILABLE with MODEL_CAPACITY_EXHAUSTED - long delay",
			account: oauthAccount,
			body: `{
				"error": {
					"code": 503,
					"status": "UNAVAILABLE",
					"details": [
						{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-pro-high"}, "reason": "MODEL_CAPACITY_EXHAUSTED"},
						{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "39s"}
					]
				}
			}`,
			expectedShouldRetry:              true,
			expectedShouldRateLimit:          false,
			expectedIsModelCapacityExhausted: true,
			minWait:                          1 * time.Second,
			modelName:                        "gemini-3-pro-high",
		},
		{
			name:    "503 UNAVAILABLE with MODEL_CAPACITY_EXHAUSTED - no retryDelay - use fixed wait",
			account: oauthAccount,
			body: `{
				"error": {
					"code": 503,
					"status": "UNAVAILABLE",
					"details": [
						{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-2.5-flash"}, "reason": "MODEL_CAPACITY_EXHAUSTED"}
					],
					"message": "No capacity available for model gemini-2.5-flash on the server"
				}
			}`,
			expectedShouldRetry:              true,
			expectedShouldRateLimit:          false,
			expectedIsModelCapacityExhausted: true,
			minWait:                          1 * time.Second,
			modelName:                        "gemini-2.5-flash",
		},
		{
			name:    "429 RESOURCE_EXHAUSTED with RATE_LIMIT_EXCEEDED - no retryDelay - use default rate limit",
			account: oauthAccount,
			body: `{
				"error": {
					"code": 429,
					"status": "RESOURCE_EXHAUSTED",
					"details": [
						{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-5"}, "reason": "RATE_LIMIT_EXCEEDED"}
					],
					"message": "You have exhausted your capacity on this model."
				}
			}`,
			expectedShouldRetry:     false,
			expectedShouldRateLimit: true,
			minWait:                 30 * time.Second,
			modelName:               "claude-sonnet-4-5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldRetry, shouldRateLimit, wait, model, isModelCapacityExhausted := shouldTriggerAntigravitySmartRetry(tt.account, []byte(tt.body))
			if shouldRetry != tt.expectedShouldRetry {
				t.Errorf("shouldRetry = %v, want %v", shouldRetry, tt.expectedShouldRetry)
			}
			if shouldRateLimit != tt.expectedShouldRateLimit {
				t.Errorf("shouldRateLimit = %v, want %v", shouldRateLimit, tt.expectedShouldRateLimit)
			}
			if isModelCapacityExhausted != tt.expectedIsModelCapacityExhausted {
				t.Errorf("isModelCapacityExhausted = %v, want %v", isModelCapacityExhausted, tt.expectedIsModelCapacityExhausted)
			}
			if shouldRetry {
				if wait < tt.minWait {
					t.Errorf("wait = %v, want >= %v", wait, tt.minWait)
				}
			}
			if shouldRateLimit && tt.minWait > 0 {
				if wait < tt.minWait {
					t.Errorf("rate limit wait = %v, want >= %v", wait, tt.minWait)
				}
			}
			if (shouldRetry || shouldRateLimit) && model != tt.modelName {
				t.Errorf("modelName = %q, want %q", model, tt.modelName)
			}
		})
	}
}

// TestSetModelRateLimitByModelName_UsesOfficialModelID 验证写入端使用官方模型 ID
func TestSetModelRateLimitByModelName_UsesOfficialModelID(t *testing.T) {
	tests := []struct {
		name             string
		modelName        string
		expectedModelKey string
		expectedSuccess  bool
	}{
		{
			name:             "claude-sonnet-4-5 should be stored as-is",
			modelName:        "claude-sonnet-4-5",
			expectedModelKey: "claude-sonnet-4-5",
			expectedSuccess:  true,
		},
		{
			name:             "gemini-3-pro-high should be stored as-is",
			modelName:        "gemini-3-pro-high",
			expectedModelKey: "gemini-3-pro-high",
			expectedSuccess:  true,
		},
		{
			name:             "gemini-3-flash should be stored as-is",
			modelName:        "gemini-3-flash",
			expectedModelKey: "gemini-3-flash",
			expectedSuccess:  true,
		},
		{
			name:             "empty model name should fail",
			modelName:        "",
			expectedModelKey: "",
			expectedSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubAntigravityAccountRepo{}
			resetAt := time.Now().Add(30 * time.Second)

			success := setModelRateLimitByModelName(
				context.Background(),
				repo,
				123, // accountID
				tt.modelName,
				"[test]",
				429,
				resetAt,
				false, // afterSmartRetry
			)

			require.Equal(t, tt.expectedSuccess, success)

			if tt.expectedSuccess {
				require.Len(t, repo.modelRateLimitCalls, 1)
				call := repo.modelRateLimitCalls[0]
				require.Equal(t, int64(123), call.accountID)
				// 关键断言：存储的 key 应该是官方模型 ID，而不是 scope
				require.Equal(t, tt.expectedModelKey, call.modelKey, "should store official model ID, not scope")
				require.WithinDuration(t, resetAt, call.resetAt, time.Second)
			} else {
				require.Empty(t, repo.modelRateLimitCalls)
			}
		})
	}
}

// TestSetModelRateLimitByModelName_NotConvertToScope 验证不会将模型名转换为 scope
func TestSetModelRateLimitByModelName_NotConvertToScope(t *testing.T) {
	repo := &stubAntigravityAccountRepo{}
	resetAt := time.Now().Add(30 * time.Second)

	// 调用 setModelRateLimitByModelName，传入官方模型 ID
	success := setModelRateLimitByModelName(
		context.Background(),
		repo,
		456,
		"claude-sonnet-4-5", // 官方模型 ID
		"[test]",
		429,
		resetAt,
		true, // afterSmartRetry
	)

	require.True(t, success)
	require.Len(t, repo.modelRateLimitCalls, 1)

	call := repo.modelRateLimitCalls[0]
	// 关键断言：存储的应该是 "claude-sonnet-4-5"，而不是 "claude_sonnet"
	require.Equal(t, "claude-sonnet-4-5", call.modelKey, "should NOT convert to scope like claude_sonnet")
	require.NotEqual(t, "claude_sonnet", call.modelKey, "should NOT be scope")
}

func TestAntigravityRetryLoop_PreCheck_SwitchesWhenRateLimited(t *testing.T) {
	upstream := &recordingOKUpstream{}
	account := &Account{
		ID:          1,
		Name:        "acc-1",
		Platform:    PlatformAntigravity,
		Schedulable: true,
		Status:      StatusActive,
		Concurrency: 1,
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				"claude-sonnet-4-5": map[string]any{
					"rate_limit_reset_at": time.Now().Add(2 * time.Second).Format(time.RFC3339),
				},
			},
		},
	}

	svc := &AntigravityGatewayService{}
	result, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:             context.Background(),
		prefix:          "[test]",
		account:         account,
		accessToken:     "token",
		action:          "generateContent",
		body:            []byte(`{"input":"test"}`),
		requestedModel:  "claude-sonnet-4-5",
		httpUpstream:    upstream,
		isStickySession: true,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	})

	require.Nil(t, result)
	var switchErr *AntigravityAccountSwitchError
	require.ErrorAs(t, err, &switchErr)
	require.Equal(t, account.ID, switchErr.OriginalAccountID)
	require.Equal(t, "claude-sonnet-4-5", switchErr.RateLimitedModel)
	require.True(t, switchErr.IsStickySession)
	require.Equal(t, 0, upstream.calls, "should not call upstream when switching on pre-check")
}

func TestAntigravityRetryLoop_PreCheck_SwitchesWhenRemainingLong(t *testing.T) {
	upstream := &recordingOKUpstream{}
	account := &Account{
		ID:          2,
		Name:        "acc-2",
		Platform:    PlatformAntigravity,
		Schedulable: true,
		Status:      StatusActive,
		Concurrency: 1,
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				"claude-sonnet-4-5": map[string]any{
					"rate_limit_reset_at": time.Now().Add(11 * time.Second).Format(time.RFC3339),
				},
			},
		},
	}

	svc := &AntigravityGatewayService{}
	result, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:             context.Background(),
		prefix:          "[test]",
		account:         account,
		accessToken:     "token",
		action:          "generateContent",
		body:            []byte(`{"input":"test"}`),
		requestedModel:  "claude-sonnet-4-5",
		httpUpstream:    upstream,
		isStickySession: true,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	})

	require.Nil(t, result)
	var switchErr *AntigravityAccountSwitchError
	require.ErrorAs(t, err, &switchErr)
	require.Equal(t, account.ID, switchErr.OriginalAccountID)
	require.Equal(t, "claude-sonnet-4-5", switchErr.RateLimitedModel)
	require.True(t, switchErr.IsStickySession)
	require.Equal(t, 0, upstream.calls, "should not call upstream when switching on pre-check")
}

func TestIsAntigravityAccountSwitchError(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		expectedOK    bool
		expectedID    int64
		expectedModel string
	}{
		{
			name:       "nil error",
			err:        nil,
			expectedOK: false,
		},
		{
			name:       "generic error",
			err:        fmt.Errorf("some error"),
			expectedOK: false,
		},
		{
			name: "account switch error",
			err: &AntigravityAccountSwitchError{
				OriginalAccountID: 123,
				RateLimitedModel:  "claude-sonnet-4-5",
				IsStickySession:   true,
			},
			expectedOK:    true,
			expectedID:    123,
			expectedModel: "claude-sonnet-4-5",
		},
		{
			name: "wrapped account switch error",
			err: fmt.Errorf("wrapped: %w", &AntigravityAccountSwitchError{
				OriginalAccountID: 456,
				RateLimitedModel:  "gemini-3-flash",
				IsStickySession:   false,
			}),
			expectedOK:    true,
			expectedID:    456,
			expectedModel: "gemini-3-flash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switchErr, ok := IsAntigravityAccountSwitchError(tt.err)
			require.Equal(t, tt.expectedOK, ok)
			if tt.expectedOK {
				require.NotNil(t, switchErr)
				require.Equal(t, tt.expectedID, switchErr.OriginalAccountID)
				require.Equal(t, tt.expectedModel, switchErr.RateLimitedModel)
			} else {
				require.Nil(t, switchErr)
			}
		})
	}
}

func TestResolveAntigravityForwardBaseURL_DefaultDaily(t *testing.T) {
	t.Setenv(antigravityForwardBaseURLEnv, "")

	oldBaseURLs := append([]string(nil), antigravity.BaseURLs...)
	defer func() {
		antigravity.BaseURLs = oldBaseURLs
	}()

	prodURL := "https://prod.test"
	dailyURL := "https://daily.test"
	antigravity.BaseURLs = []string{dailyURL, prodURL}

	resolved := resolveAntigravityForwardBaseURL()
	require.Equal(t, dailyURL, resolved)
}

func TestAntigravityAccountSwitchError_Error(t *testing.T) {
	err := &AntigravityAccountSwitchError{
		OriginalAccountID: 789,
		RateLimitedModel:  "claude-opus-4-5",
		IsStickySession:   true,
	}
	msg := err.Error()
	require.Contains(t, msg, "789")
	require.Contains(t, msg, "claude-opus-4-5")
}

// stubSchedulerCache 用于测试的 SchedulerCache 实现
type stubSchedulerCache struct {
	SchedulerCache
	setAccountCalls []*Account
	setAccountErr   error
}

func (s *stubSchedulerCache) SetAccount(ctx context.Context, account *Account) error {
	s.setAccountCalls = append(s.setAccountCalls, account)
	return s.setAccountErr
}

// TestUpdateAccountModelRateLimitInCache_UpdatesExtraAndCallsCache 测试模型限流后更新缓存
func TestUpdateAccountModelRateLimitInCache_UpdatesExtraAndCallsCache(t *testing.T) {
	cache := &stubSchedulerCache{}
	snapshotService := &SchedulerSnapshotService{cache: cache}
	svc := &AntigravityGatewayService{
		schedulerSnapshot: snapshotService,
	}

	account := &Account{
		ID:       100,
		Name:     "test-account",
		Platform: PlatformAntigravity,
	}
	modelKey := "claude-sonnet-4-5"
	resetAt := time.Now().Add(30 * time.Second)

	svc.updateAccountModelRateLimitInCache(context.Background(), account, modelKey, resetAt)

	// 验证 Extra 字段被正确更新
	require.NotNil(t, account.Extra)
	limits, ok := account.Extra["model_rate_limits"].(map[string]any)
	require.True(t, ok)
	modelLimit, ok := limits[modelKey].(map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, modelLimit["rate_limited_at"])
	require.NotEmpty(t, modelLimit["rate_limit_reset_at"])

	// 验证 cache.SetAccount 被调用
	require.Len(t, cache.setAccountCalls, 1)
	require.Equal(t, account.ID, cache.setAccountCalls[0].ID)
}

// TestUpdateAccountModelRateLimitInCache_NilSchedulerSnapshot 测试 schedulerSnapshot 为 nil 时不 panic
func TestUpdateAccountModelRateLimitInCache_NilSchedulerSnapshot(t *testing.T) {
	svc := &AntigravityGatewayService{
		schedulerSnapshot: nil,
	}

	account := &Account{ID: 1, Name: "test"}

	// 不应 panic
	svc.updateAccountModelRateLimitInCache(context.Background(), account, "claude-sonnet-4-5", time.Now().Add(30*time.Second))

	// Extra 不应被更新（因为函数提前返回）
	require.Nil(t, account.Extra)
}

// TestUpdateAccountModelRateLimitInCache_PreservesExistingExtra 测试保留已有的 Extra 数据
func TestUpdateAccountModelRateLimitInCache_PreservesExistingExtra(t *testing.T) {
	cache := &stubSchedulerCache{}
	snapshotService := &SchedulerSnapshotService{cache: cache}
	svc := &AntigravityGatewayService{
		schedulerSnapshot: snapshotService,
	}

	account := &Account{
		ID:       200,
		Name:     "test-account",
		Platform: PlatformAntigravity,
		Extra: map[string]any{
			"existing_key": "existing_value",
			"model_rate_limits": map[string]any{
				"gemini-3-flash": map[string]any{
					"rate_limited_at":     "2024-01-01T00:00:00Z",
					"rate_limit_reset_at": "2024-01-01T00:05:00Z",
				},
			},
		},
	}

	svc.updateAccountModelRateLimitInCache(context.Background(), account, "claude-sonnet-4-5", time.Now().Add(30*time.Second))

	// 验证已有数据被保留
	require.Equal(t, "existing_value", account.Extra["existing_key"])
	limits := account.Extra["model_rate_limits"].(map[string]any)
	require.NotNil(t, limits["gemini-3-flash"])
	require.NotNil(t, limits["claude-sonnet-4-5"])
}

// TestSchedulerSnapshotService_UpdateAccountInCache 测试 UpdateAccountInCache 方法
func TestSchedulerSnapshotService_UpdateAccountInCache(t *testing.T) {
	t.Run("calls cache.SetAccount", func(t *testing.T) {
		cache := &stubSchedulerCache{}
		svc := &SchedulerSnapshotService{cache: cache}

		account := &Account{ID: 123, Name: "test"}
		err := svc.UpdateAccountInCache(context.Background(), account)

		require.NoError(t, err)
		require.Len(t, cache.setAccountCalls, 1)
		require.Equal(t, int64(123), cache.setAccountCalls[0].ID)
	})

	t.Run("returns nil when cache is nil", func(t *testing.T) {
		svc := &SchedulerSnapshotService{cache: nil}

		err := svc.UpdateAccountInCache(context.Background(), &Account{ID: 1})

		require.NoError(t, err)
	})

	t.Run("returns nil when account is nil", func(t *testing.T) {
		cache := &stubSchedulerCache{}
		svc := &SchedulerSnapshotService{cache: cache}

		err := svc.UpdateAccountInCache(context.Background(), nil)

		require.NoError(t, err)
		require.Empty(t, cache.setAccountCalls)
	})

	t.Run("propagates cache error", func(t *testing.T) {
		expectedErr := fmt.Errorf("cache error")
		cache := &stubSchedulerCache{setAccountErr: expectedErr}
		svc := &SchedulerSnapshotService{cache: cache}

		err := svc.UpdateAccountInCache(context.Background(), &Account{ID: 1})

		require.ErrorIs(t, err, expectedErr)
	})
}
