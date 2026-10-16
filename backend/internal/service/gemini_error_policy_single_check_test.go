//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
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
// Gemini 三条转发路径对每个上游错误响应只检查一次错误策略：
//   - 重试循环内已检查的最终响应，循环外的错误处理复用该结果；
//   - 账号级 401 规则首次命中只临时不可调度，账号上已有 401 暂停记录时才升级为 error；
//   - 循环内未检查的最终响应（如 Claude 兼容层的非签名 400）在循环外检查。
// ---------------------------------------------------------------------------

type geminiSingleCheckRepoStub struct {
	errorPolicyRepoStub
	tempReason string
}

func (r *geminiSingleCheckRepoStub) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, reason string) error {
	r.tempCalls++
	r.tempReason = reason
	return nil
}

func (r *geminiSingleCheckRepoStub) GetByID(_ context.Context, id int64) (*Account, error) {
	return &Account{ID: id, TempUnschedulableReason: r.tempReason}, nil
}

type geminiSequenceUpstreamResponse struct {
	status int
	body   string
}

type geminiSequenceUpstreamStub struct {
	responses []geminiSequenceUpstreamResponse
	calls     int
}

func (s *geminiSequenceUpstreamStub) Do(*http.Request, string, int64, int) (*http.Response, error) {
	r := s.responses[min(s.calls, len(s.responses)-1)]
	s.calls++
	return &http.Response{
		StatusCode: r.status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(r.body)),
	}, nil
}

func (s *geminiSequenceUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, concurrency)
}

func newGeminiSingleCheckService(responses ...geminiSequenceUpstreamResponse) (*GeminiMessagesCompatService, *geminiSequenceUpstreamStub, *geminiSingleCheckRepoStub) {
	repo := &geminiSingleCheckRepoStub{}
	rateLimit := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	upstream := &geminiSequenceUpstreamStub{responses: responses}
	svc := &GeminiMessagesCompatService{
		accountRepo:      repo,
		httpUpstream:     upstream,
		rateLimitService: rateLimit,
		cfg:              &config.Config{},
	}
	return svc, upstream, repo
}

func geminiAPIKeyAccountWithTempRule(rule map[string]any) *Account {
	return &Account{
		ID:       830,
		Platform: PlatformGemini,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":                    "test-key",
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules":   []any{rule},
		},
	}
}

const geminiSingleCheck401Body = `{"error":{"code":401,"message":"API key expired. Please renew the API key.","status":"UNAUTHENTICATED"}}`

func geminiAccountScoped401Rule() map[string]any {
	return map[string]any{
		"error_code":       float64(401),
		"keywords":         []any{"expired"},
		"duration_minutes": float64(30),
		"scope":            TempUnschedScopeAccount,
	}
}

// geminiSingleCheckForwarders 以三条转发路径各发一次请求，返回转发错误与 gin.Context。
var geminiSingleCheckForwarders = []struct {
	name    string
	forward func(t *testing.T, svc *GeminiMessagesCompatService, account *Account) (error, *gin.Context)
}{
	{
		name: "native",
		forward: func(t *testing.T, svc *GeminiMessagesCompatService, account *Account) (error, *gin.Context) {
			c, _ := newGeminiNativeTestContext(t)
			_, err := svc.ForwardNative(context.Background(), c, account, "gemini-2.5-flash", "generateContent", false,
				[]byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
			return err, c
		},
	},
	{
		name: "claude_compat",
		forward: func(t *testing.T, svc *GeminiMessagesCompatService, account *Account) (error, *gin.Context) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			body := []byte(`{"model":"gemini-2.5-flash","max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
			_, err := svc.Forward(context.Background(), c, account, body)
			return err, c
		},
	},
	{
		name: "chat_completions_compat",
		forward: func(t *testing.T, svc *GeminiMessagesCompatService, account *Account) (error, *gin.Context) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			body := []byte(`{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"hi"}]}`)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body)
			return err, c
		},
	},
}

func TestGeminiForward_First401RuleHitOnlyTempUnschedules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, fw := range geminiSingleCheckForwarders {
		t.Run(fw.name, func(t *testing.T) {
			svc, upstream, repo := newGeminiSingleCheckService(geminiSequenceUpstreamResponse{status: http.StatusUnauthorized, body: geminiSingleCheck401Body})

			err, _ := fw.forward(t, svc, geminiAPIKeyAccountWithTempRule(geminiAccountScoped401Rule()))

			var failoverErr *UpstreamFailoverError
			require.True(t, errors.As(err, &failoverErr), "规则命中应换号，got %T: %v", err, err)
			require.Equal(t, http.StatusUnauthorized, failoverErr.StatusCode)
			require.Equal(t, 1, upstream.calls)
			require.Equal(t, 1, repo.tempCalls, "首次 401 命中规则应临时不可调度")
			require.Zero(t, repo.setErrCalls, "首次 401 命中规则不应升级为 error")
		})
	}
}

func TestGeminiForward_Repeated401RuleHitEscalatesToError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, fw := range geminiSingleCheckForwarders {
		t.Run(fw.name, func(t *testing.T) {
			svc, _, repo := newGeminiSingleCheckService(geminiSequenceUpstreamResponse{status: http.StatusUnauthorized, body: geminiSingleCheck401Body})
			repo.tempReason = `{"until_unix":1,"triggered_at_unix":1,"status_code":401,"matched_keyword":"expired","rule_index":0,"scope":"account"}`

			_, _ = fw.forward(t, svc, geminiAPIKeyAccountWithTempRule(geminiAccountScoped401Rule()))

			require.Zero(t, repo.tempCalls, "已有 401 暂停记录时直接升级，不临时不可调度")
			require.Equal(t, 1, repo.setErrCalls, "已有 401 暂停记录时应升级为 error")
			require.Contains(t, repo.lastErrorMsg, "Authentication failed (401)")
		})
	}
}

func TestGeminiForward_UncheckedFinalResponseIsCheckedAfterLoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if testing.Short() {
		t.Skip("包含一次 1s 的重试退避")
	}
	// 第一次 503 在循环内检查后重试；第二次非签名 400 走 Claude 兼容层的签名分支，循环内不检查。
	svc, upstream, repo := newGeminiSingleCheckService(
		geminiSequenceUpstreamResponse{status: http.StatusServiceUnavailable, body: `{"error":{"code":503,"message":"backend busy","status":"UNAVAILABLE"}}`},
		geminiSequenceUpstreamResponse{status: http.StatusBadRequest, body: `{"error":{"code":400,"message":"quota project misconfigured","status":"FAILED_PRECONDITION"}}`},
	)
	account := geminiAPIKeyAccountWithTempRule(map[string]any{
		"error_code":       float64(400),
		"keywords":         []any{"misconfigured"},
		"duration_minutes": float64(30),
		"scope":            TempUnschedScopeAccount,
	})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := []byte(`{"model":"gemini-2.5-flash","max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))

	_, err := svc.Forward(context.Background(), c, account, body)

	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "400 规则命中应换号，got %T: %v", err, err)
	require.Equal(t, 2, upstream.calls)
	require.Equal(t, 1, repo.tempCalls, "循环内未检查的最终响应应在循环外检查")
}
