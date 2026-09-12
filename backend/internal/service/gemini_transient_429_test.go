//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 生产复现：Gemini service_account (account_id=6) 上游第一次返回 429，Sub2API 立刻
// 把账号 SetRateLimited 到次日 PST 午夜，随后 retry 1/5 成功、客户端拿到 200，但
// rate_limit_reset_at 仍留在账号上，账号被停止调度到第二天。
//
// 两条不变量：
//  1. 重试期间不写账号级限流；任意一次重试成功则不得留下 rate_limit 状态。
//  2. 只有最终一次仍是 429 才写，且只写一次。
//
// Claude-compatible / Gemini native / Chat Completions 三条转发路径都要成立。
// ---------------------------------------------------------------------------

// geminiSequenceUpstreamStub 按顺序返回预置响应，用于驱动真实的重试循环。
type geminiSequenceUpstreamStub struct {
	statuses []int
	bodies   []string
	headers  []http.Header
	calls    int
}

func (s *geminiSequenceUpstreamStub) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	if s.calls >= len(s.statuses) {
		return nil, fmt.Errorf("unexpected upstream call #%d (only %d responses staged)", s.calls+1, len(s.statuses))
	}
	idx := s.calls
	s.calls++
	header := http.Header{"Content-Type": []string{"application/json"}}
	if idx < len(s.headers) && s.headers[idx] != nil {
		header = s.headers[idx].Clone()
		header.Set("Content-Type", "application/json")
	}
	return &http.Response{
		StatusCode: s.statuses[idx],
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(s.bodies[idx])),
	}, nil
}

func (s *geminiSequenceUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

// geminiTransient429Body 是生产上观测到的 Vertex 瞬时 429：RESOURCE_EXHAUSTED，
// 没有 quotaResetDelay，也没有 "per day" 字样。
const geminiTransient429Body = `{"error":{"code":429,"message":"Resource exhausted, please try again later.","status":"RESOURCE_EXHAUSTED"}}`

const geminiNativeSuccessBody = `{"candidates":[{"content":{"role":"model","parts":[{"text":"hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`

// shrinkGeminiRetryBackoff 把指数退避压到毫秒级，避免单测真的睡 1+2+4+8 秒。
func shrinkGeminiRetryBackoff(t *testing.T) {
	t.Helper()
	original := geminiRetryBaseDelay
	geminiRetryBaseDelay = time.Millisecond
	t.Cleanup(func() { geminiRetryBaseDelay = original })
}

func geminiVertexServiceAccount(id int64) *Account {
	return &Account{
		ID:       id,
		Platform: PlatformGemini,
		Type:     AccountTypeServiceAccount,
		Credentials: map[string]any{
			"service_account_json": map[string]any{
				"type":         "service_account",
				"project_id":   "proj",
				"private_key":  "-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n",
				"client_email": "svc@proj.iam.gserviceaccount.com",
			},
		},
	}
}

func newGeminiTransient429Service(stub *geminiSequenceUpstreamStub) (*GeminiMessagesCompatService, *rateLimit429AccountRepoStub) {
	repo := &rateLimit429AccountRepoStub{}
	return &GeminiMessagesCompatService{
		httpUpstream:     stub,
		cfg:              &config.Config{},
		accountRepo:      repo,
		rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil),
		tokenProvider:    NewGeminiTokenProvider(repo, &fakeGeminiTokenCache{token: "ya29.test-token"}, nil),
	}, repo
}

func newGeminiTransient429Context(t *testing.T, path string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
	return c, rec
}

// ---------------------------------------------------------------------------
// 1. 第一次 429、第二次 200：三条路径都必须成功返回且 SetRateLimited 调用次数为 0。
// ---------------------------------------------------------------------------

func TestGeminiServiceAccount_Transient429ThenSuccessLeavesNoRateLimit(t *testing.T) {
	shrinkGeminiRetryBackoff(t)

	tests := []struct {
		name    string
		path    string
		forward func(svc *GeminiMessagesCompatService, c *gin.Context, account *Account) (*ForwardResult, error)
	}{
		{
			name: "claude_compatible",
			path: "/v1/messages",
			forward: func(svc *GeminiMessagesCompatService, c *gin.Context, account *Account) (*ForwardResult, error) {
				body := []byte(`{"model":"gemini-2.5-pro","max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`)
				return svc.Forward(context.Background(), c, account, body)
			},
		},
		{
			name: "gemini_native",
			path: "/v1beta/models/gemini-2.5-pro:generateContent",
			forward: func(svc *GeminiMessagesCompatService, c *gin.Context, account *Account) (*ForwardResult, error) {
				body := []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`)
				return svc.ForwardNative(context.Background(), c, account, "gemini-2.5-pro", "generateContent", false, body)
			},
		},
		{
			name: "chat_completions",
			path: "/v1/chat/completions",
			forward: func(svc *GeminiMessagesCompatService, c *gin.Context, account *Account) (*ForwardResult, error) {
				body := []byte(`{"model":"gemini-2.5-pro","messages":[{"role":"user","content":"hello"}]}`)
				return svc.ForwardAsChatCompletions(context.Background(), c, account, body)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &geminiSequenceUpstreamStub{
				statuses: []int{http.StatusTooManyRequests, http.StatusOK},
				bodies:   []string{geminiTransient429Body, geminiNativeSuccessBody},
			}
			svc, repo := newGeminiTransient429Service(stub)
			c, rec := newGeminiTransient429Context(t, tt.path)

			result, err := tt.forward(svc, c, geminiVertexServiceAccount(6))

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 2, stub.calls, "应该重试一次后成功")
			require.Equal(t, http.StatusOK, rec.Code)
			require.Zero(t, repo.rateLimitCalls,
				"重试成功后账号不得留下任何 rate_limit 状态（生产缺陷：429 后重试成功，账号仍被冷却到 PST 午夜）")
		})
	}
}

// ---------------------------------------------------------------------------
// 2. 连续 5 次 429：最终失败，且 SetRateLimited 只调用一次（而不是每次重试一次）。
// ---------------------------------------------------------------------------

func TestGeminiServiceAccount_AllAttempts429MarksRateLimitOnce(t *testing.T) {
	shrinkGeminiRetryBackoff(t)

	stub := &geminiSequenceUpstreamStub{
		statuses: []int{
			http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests,
			http.StatusTooManyRequests, http.StatusTooManyRequests,
		},
		bodies: []string{
			geminiTransient429Body, geminiTransient429Body, geminiTransient429Body,
			geminiTransient429Body, geminiTransient429Body,
		},
	}
	svc, repo := newGeminiTransient429Service(stub)
	c, _ := newGeminiTransient429Context(t, "/v1/messages")

	body := []byte(`{"model":"gemini-2.5-pro","max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`)
	before := time.Now()
	_, err := svc.Forward(context.Background(), c, geminiVertexServiceAccount(6), body)

	require.Error(t, err, "5 次都 429 应向上返回 failover 错误")
	require.Equal(t, geminiMaxRetries, stub.calls)
	require.Equal(t, 1, repo.rateLimitCalls, "只有最终结果确定后写一次账号限流")
	require.Equal(t, int64(6), repo.lastRateLimitID)
	// service_account 用短冷却，不是 PST 午夜。
	require.WithinDuration(t, before.Add(geminiServiceAccountFallbackCooldown), repo.lastRateLimitReset, 5*time.Second)
}

// ---------------------------------------------------------------------------
// 3~5. RetryInfo.retryDelay 解析：整数秒、小数秒向上取整、异常长值截断。
// ---------------------------------------------------------------------------

func geminiRetryInfoBody(retryDelay string) []byte {
	return []byte(`{"error":{"code":429,"message":"Resource exhausted","status":"RESOURCE_EXHAUSTED","details":[` +
		`{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"RATE_LIMIT_EXCEEDED"},` +
		`{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"` + retryDelay + `"}]}}`)
}

func TestParseGeminiRetryInfoDelay(t *testing.T) {
	tests := []struct {
		name       string
		retryDelay string
		wantFound  bool
		want       time.Duration
	}{
		{name: "integer_seconds", retryDelay: "39s", wantFound: true, want: 39 * time.Second},
		{name: "fractional_seconds_round_up", retryDelay: "1.5s", wantFound: true, want: 2 * time.Second},
		{name: "sub_second_rounds_up_to_one", retryDelay: "0.2s", wantFound: true, want: time.Second},
		{name: "absurdly_long_is_capped", retryDelay: "86400s", wantFound: true, want: geminiMaxRetryDelay},
		{name: "just_over_cap_is_capped", retryDelay: "16m", wantFound: true, want: geminiMaxRetryDelay},
		{name: "zero_is_ignored", retryDelay: "0s", wantFound: false},
		{name: "unparsable_is_ignored", retryDelay: "soon", wantFound: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseGeminiRetryInfoDelay(geminiRetryInfoBody(tt.retryDelay))
			require.Equal(t, tt.wantFound, ok)
			if tt.wantFound {
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseGeminiRetryInfoDelay_IgnoresOtherDetailTypes(t *testing.T) {
	body := []byte(`{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","retryDelay":"39s"}]}}`)
	_, ok := parseGeminiRetryInfoDelay(body)
	require.False(t, ok, "retryDelay 只在 google.rpc.RetryInfo 里有意义")
}

func TestParseGeminiRateLimitResetTime_UsesRetryInfo(t *testing.T) {
	now := time.Now().Unix()
	got := ParseGeminiRateLimitResetTime(geminiRetryInfoBody("39s"))
	require.NotNil(t, got)
	require.InDelta(t, now+39, *got, 2)
}

func TestHandleGeminiUpstreamError_ServiceAccountUsesRetryInfoDelay(t *testing.T) {
	repo := &rateLimit429AccountRepoStub{}
	svc := &GeminiMessagesCompatService{
		accountRepo:      repo,
		rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil),
	}

	before := time.Now()
	svc.handleGeminiUpstreamError(context.Background(), geminiVertexServiceAccount(6),
		http.StatusTooManyRequests, http.Header{}, geminiRetryInfoBody("39s"))

	require.Equal(t, 1, repo.rateLimitCalls)
	require.WithinDuration(t, before.Add(39*time.Second), repo.lastRateLimitReset, 3*time.Second)
}

func TestHandleGeminiUpstreamError_ServiceAccountCapsAbsurdRetryInfoDelay(t *testing.T) {
	repo := &rateLimit429AccountRepoStub{}
	svc := &GeminiMessagesCompatService{
		accountRepo:      repo,
		rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil),
	}

	before := time.Now()
	svc.handleGeminiUpstreamError(context.Background(), geminiVertexServiceAccount(6),
		http.StatusTooManyRequests, http.Header{}, geminiRetryInfoBody("86400s"))

	require.Equal(t, 1, repo.rateLimitCalls)
	require.WithinDuration(t, before.Add(geminiMaxRetryDelay), repo.lastRateLimitReset, 3*time.Second)
}

// ---------------------------------------------------------------------------
// 6. service_account 无任何可解析 reset 信息 → 短冷却，绝不是 PST 午夜。
// ---------------------------------------------------------------------------

func TestHandleGeminiUpstreamError_ServiceAccountFallsBackToShortCooldown(t *testing.T) {
	// MODEL_CAPACITY_EXHAUSTED 对按量计费的 Vertex 项目是瞬时容量问题，不是日配额。
	capacityExhausted := []byte(`{"error":{"code":429,"message":"No capacity available for model gemini-3.1-pro-preview on the server","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"MODEL_CAPACITY_EXHAUSTED"}]}}`)
	// 中转/上游偶尔会带 "per day" 文案；service_account 依然不能套用 AI Studio 的日配额语义。
	dailyQuotaWording := []byte(`{"error":{"code":429,"message":"Quota exceeded for quota metric 'Generate requests per day'","status":"RESOURCE_EXHAUSTED"}}`)

	tests := []struct {
		name string
		body []byte
	}{
		{name: "plain_resource_exhausted", body: []byte(geminiTransient429Body)},
		{name: "model_capacity_exhausted", body: capacityExhausted},
		{name: "daily_quota_wording", body: dailyQuotaWording},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &rateLimit429AccountRepoStub{}
			svc := &GeminiMessagesCompatService{
				accountRepo:      repo,
				rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil),
			}

			before := time.Now()
			svc.handleGeminiUpstreamError(context.Background(), geminiVertexServiceAccount(6),
				http.StatusTooManyRequests, http.Header{}, tt.body)

			require.Equal(t, 1, repo.rateLimitCalls)
			require.WithinDuration(t, before.Add(geminiServiceAccountFallbackCooldown), repo.lastRateLimitReset, 5*time.Second)
			require.True(t, repo.lastRateLimitReset.Before(geminiDailyResetTime(before)),
				"Vertex 按量账号不得冷却到 PST 午夜")
		})
	}
}

func TestHandleGeminiUpstreamError_ServiceAccountShortCooldownReadsSettings(t *testing.T) {
	repo := &rateLimit429AccountRepoStub{}
	settingRepo := newMockSettingRepo()
	data, _ := json.Marshal(RateLimit429CooldownSettings{Enabled: true, CooldownSeconds: 12})
	settingRepo.data[SettingKeyRateLimit429CooldownSettings] = string(data)

	rlSvc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	rlSvc.SetSettingService(NewSettingService(settingRepo, &config.Config{}))
	svc := &GeminiMessagesCompatService{accountRepo: repo, rateLimitService: rlSvc}

	before := time.Now()
	svc.handleGeminiUpstreamError(context.Background(), geminiVertexServiceAccount(6),
		http.StatusTooManyRequests, http.Header{}, []byte(geminiTransient429Body))

	require.Equal(t, 1, repo.rateLimitCalls)
	require.WithinDuration(t, before.Add(12*time.Second), repo.lastRateLimitReset, 3*time.Second)
}

func TestHandleGeminiUpstreamError_ServiceAccountShortCooldownDisabledSkipsWrite(t *testing.T) {
	repo := &rateLimit429AccountRepoStub{}
	settingRepo := newMockSettingRepo()
	data, _ := json.Marshal(RateLimit429CooldownSettings{Enabled: false, CooldownSeconds: 12})
	settingRepo.data[SettingKeyRateLimit429CooldownSettings] = string(data)

	rlSvc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	rlSvc.SetSettingService(NewSettingService(settingRepo, &config.Config{}))
	svc := &GeminiMessagesCompatService{accountRepo: repo, rateLimitService: rlSvc}

	svc.handleGeminiUpstreamError(context.Background(), geminiVertexServiceAccount(6),
		http.StatusTooManyRequests, http.Header{}, []byte(geminiTransient429Body))

	require.Zero(t, repo.rateLimitCalls, "管理端关闭 429 兜底冷却时保持既有语义：不标记账号")
}

// ---------------------------------------------------------------------------
// Retry-After：上游显式给出的时间优先于本地启发式兜底。
// ---------------------------------------------------------------------------

func TestParseGeminiRetryAfterHeader(t *testing.T) {
	httpDate := time.Now().Add(90 * time.Second).UTC().Format(http.TimeFormat)

	tests := []struct {
		name      string
		value     string
		wantFound bool
		want      time.Duration
		delta     time.Duration
	}{
		{name: "seconds", value: "39", wantFound: true, want: 39 * time.Second},
		{name: "fractional_seconds_round_up", value: "1.5", wantFound: true, want: 2 * time.Second},
		{name: "absurdly_long_is_capped", value: "86400", wantFound: true, want: geminiMaxRetryDelay},
		{name: "http_date", value: httpDate, wantFound: true, want: 90 * time.Second, delta: 2 * time.Second},
		{name: "past_http_date_ignored", value: time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)},
		{name: "zero_ignored", value: "0"},
		{name: "garbage_ignored", value: "later"},
		{name: "empty_ignored", value: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			if tt.value != "" {
				headers.Set("Retry-After", tt.value)
			}
			got, ok := parseGeminiRetryAfterHeader(headers)
			require.Equal(t, tt.wantFound, ok)
			if !tt.wantFound {
				return
			}
			if tt.delta > 0 {
				require.InDelta(t, tt.want.Seconds(), got.Seconds(), tt.delta.Seconds())
				return
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestHandleGeminiUpstreamError_ServiceAccountHonorsRetryAfterHeader(t *testing.T) {
	repo := &rateLimit429AccountRepoStub{}
	svc := &GeminiMessagesCompatService{
		accountRepo:      repo,
		rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil),
	}
	headers := http.Header{"Retry-After": []string{"42"}}

	before := time.Now()
	svc.handleGeminiUpstreamError(context.Background(), geminiVertexServiceAccount(6),
		http.StatusTooManyRequests, headers, []byte(geminiTransient429Body))

	require.Equal(t, 1, repo.rateLimitCalls)
	require.WithinDuration(t, before.Add(42*time.Second), repo.lastRateLimitReset, 3*time.Second)
}

func TestHandleGeminiUpstreamError_APIKeyHonorsRetryAfterInsteadOfPSTMidnight(t *testing.T) {
	repo := &rateLimit429AccountRepoStub{}
	svc := &GeminiMessagesCompatService{
		accountRepo:      repo,
		rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil),
	}
	account := &Account{ID: 7, Platform: PlatformGemini, Type: AccountTypeAPIKey}
	headers := http.Header{"Retry-After": []string{"30"}}

	before := time.Now()
	svc.handleGeminiUpstreamError(context.Background(), account,
		http.StatusTooManyRequests, headers, []byte(geminiTransient429Body))

	require.Equal(t, 1, repo.rateLimitCalls)
	require.WithinDuration(t, before.Add(30*time.Second), repo.lastRateLimitReset, 3*time.Second)
}

// ---------------------------------------------------------------------------
// 7. API Key 的日配额 / PST 午夜语义不回归。
// ---------------------------------------------------------------------------

func TestHandleGeminiUpstreamError_APIKeyDailyQuotaStillResetsAtPSTMidnight(t *testing.T) {
	tests := []struct {
		name string
		body []byte
	}{
		{
			name: "explicit_daily_quota_message",
			body: []byte(`{"error":{"code":429,"message":"Quota exceeded for quota metric 'Generate requests per day'","status":"RESOURCE_EXHAUSTED"}}`),
		},
		{
			name: "no_parsable_reset_info",
			body: []byte(geminiTransient429Body),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &rateLimit429AccountRepoStub{}
			svc := &GeminiMessagesCompatService{
				accountRepo:      repo,
				rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil),
			}
			account := &Account{ID: 7, Platform: PlatformGemini, Type: AccountTypeAPIKey}

			now := time.Now()
			svc.handleGeminiUpstreamError(context.Background(), account,
				http.StatusTooManyRequests, http.Header{}, tt.body)

			require.Equal(t, 1, repo.rateLimitCalls)
			require.WithinDuration(t, geminiDailyResetTime(now), repo.lastRateLimitReset, 2*time.Second)
		})
	}
}
