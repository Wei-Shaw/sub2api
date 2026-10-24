//go:build unit

package service

import (
	"context"
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

// geminiTransient429UpstreamStub 按顺序返回预置响应，用于驱动真实的重试循环。
type geminiTransient429UpstreamStub struct {
	statuses []int
	bodies   []string
	headers  []http.Header
	calls    int
}

func (s *geminiTransient429UpstreamStub) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
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

func (s *geminiTransient429UpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

// Use a project quota 429 with RetryInfo so upstream shared-capacity exemptions
// cannot mask premature account-level rate-limit writes.
const geminiTransient429Body = `{"error":{"code":429,"message":"Project quota exceeded","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"39s"}]}}`

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

func newGeminiTransient429Service(stub *geminiTransient429UpstreamStub) (*GeminiMessagesCompatService, *rateLimit429AccountRepoStub) {
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

type geminiTransient429Forwarder struct {
	name    string
	path    string
	forward func(svc *GeminiMessagesCompatService, c *gin.Context, account *Account) (*ForwardResult, error)
}

func geminiTransient429Forwarders() []geminiTransient429Forwarder {
	return []geminiTransient429Forwarder{
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
}

func TestGeminiServiceAccount_Transient429ThenSuccessLeavesNoRateLimit(t *testing.T) {
	shrinkGeminiRetryBackoff(t)

	for _, tt := range geminiTransient429Forwarders() {
		t.Run(tt.name, func(t *testing.T) {
			stub := &geminiTransient429UpstreamStub{
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

	for _, tt := range geminiTransient429Forwarders() {
		t.Run(tt.name, func(t *testing.T) {
			stub := &geminiTransient429UpstreamStub{}
			for range geminiMaxRetries {
				stub.statuses = append(stub.statuses, http.StatusTooManyRequests)
				stub.bodies = append(stub.bodies, geminiTransient429Body)
			}
			svc, repo := newGeminiTransient429Service(stub)
			c, _ := newGeminiTransient429Context(t, tt.path)
			before := time.Now()
			_, err := tt.forward(svc, c, geminiVertexServiceAccount(6))

			require.Error(t, err, "5 次都 429 应向上返回 failover 错误")
			require.Equal(t, geminiMaxRetries, stub.calls)
			require.Equal(t, 1, repo.rateLimitCalls, "只有最终结果确定后写一次账号限流")
			require.Equal(t, int64(6), repo.lastRateLimitID)
			// Keep upstream's RetryInfo cooldown after the final failed attempt.
			require.WithinDuration(t, before.Add(39*time.Second), repo.lastRateLimitReset, 5*time.Second)
		})
	}
}
