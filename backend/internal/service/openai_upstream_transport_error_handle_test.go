//go:build unit

package service

import (
	"context"
	"errors"
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

// openaiTransportAccountRepoStub records SetTempUnschedulable calls. It embeds the
// (nil) AccountRepository interface so any other method call would panic — the
// helper under test must only touch SetTempUnschedulable. tempUnschedCall is shared
// with antigravity_internal500_penalty_test.go (same package).
type openaiTransportAccountRepoStub struct {
	AccountRepository
	tempUnschedCalls []tempUnschedCall
}

func (r *openaiTransportAccountRepoStub) SetTempUnschedulable(_ context.Context, id int64, until time.Time, reason string) error {
	r.tempUnschedCalls = append(r.tempUnschedCalls, tempUnschedCall{accountID: id, until: until, reason: reason})
	return nil
}

func newOpenAITransportErrTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, rec
}

type failingOpenAIHTTPUpstream struct {
	err   error
	calls int
}

type failingResponseBody struct {
	err error
}

func (b failingResponseBody) Read([]byte) (int, error) { return 0, b.err }
func (b failingResponseBody) Close() error             { return nil }

func (u *failingOpenAIHTTPUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls++
	return nil, u.err
}

func (u *failingOpenAIHTTPUpstream) DoWithTLS(_ *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	u.calls++
	return nil, u.err
}

func TestHandleNonStreamingResponse_HTTP2ReadErrorFailsOverBeforeWrite(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := &Account{ID: 501, Name: "h2-read", Platform: PlatformOpenAI}
	c, rec := newOpenAITransportErrTestContext()
	readErr := errors.New("stream error: stream ID 37; INTERNAL_ERROR; received from peer")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       failingResponseBody{err: readErr},
	}

	_, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, account, "gpt-5.4", "gpt-5.4")

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Equal(t, 0, rec.Body.Len(), "下游尚未写入时必须交给 handler 切换账号")
}

func TestHandleNonStreamingResponsePassthrough_HTTP2ReadErrorFailsOverBeforeWrite(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := &Account{ID: 504, Name: "h2-read-passthrough", Platform: PlatformOpenAI}
	c, rec := newOpenAITransportErrTestContext()
	readErr := errors.New("stream error: stream ID 41; INTERNAL_ERROR; received from peer")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       failingResponseBody{err: readErr},
	}

	_, err := svc.handleNonStreamingResponsePassthrough(c.Request.Context(), resp, c, account, "gpt-5.4", "gpt-5.4")

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Equal(t, 0, rec.Body.Len())
}

func TestHandleNonStreamingResponse_ReadErrorAfterWriteDoesNotFailOver(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := &Account{ID: 502, Name: "h2-written", Platform: PlatformOpenAI}
	c, rec := newOpenAITransportErrTestContext()
	c.Data(http.StatusOK, "text/plain", []byte("already written"))
	readErr := errors.New("stream error: stream ID 39; INTERNAL_ERROR; received from peer")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       failingResponseBody{err: readErr},
	}

	_, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, account, "gpt-5.4", "gpt-5.4")

	require.ErrorIs(t, err, readErr)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "已写入下游后不能重放请求")
	require.Equal(t, "already written", rec.Body.String())
}

func TestHandleNonStreamingResponse_ContextCanceledDoesNotFailOver(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := &Account{ID: 503, Name: "client-gone", Platform: PlatformOpenAI}
	c, rec := newOpenAITransportErrTestContext()
	readErr := fmt.Errorf("read response body: %w", context.Canceled)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       failingResponseBody{err: readErr},
	}

	_, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, account, "gpt-5.4", "gpt-5.4")

	require.ErrorIs(t, err, context.Canceled)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Equal(t, 0, rec.Body.Len())
}

func TestHandleNonStreamingResponse_TooLargeDoesNotFailOver(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.UpstreamResponseReadMaxBytes = 1
	svc := &OpenAIGatewayService{cfg: cfg}
	account := &Account{ID: 505, Name: "too-large", Platform: PlatformOpenAI}
	c, rec := newOpenAITransportErrTestContext()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader("{}")),
	}

	_, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, account, "gpt-5.4", "gpt-5.4")

	require.ErrorIs(t, err, ErrUpstreamResponseBodyTooLarge)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "响应体超限已经写入格式化错误，不能重放请求")
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "Upstream response too large")
}

// A durable proxy/credential failure must (a) temporarily unschedule the account
// so it stops being hammered, and (b) return a failover error so the handler
// switches to a healthy account instead of writing a hard 502 itself.
func TestHandleOpenAIUpstreamTransportError_PersistentEvictsAndFailsOver(t *testing.T) {
	repo := &openaiTransportAccountRepoStub{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 4627, Name: "proxy-expired", Platform: PlatformOpenAI}
	c, rec := newOpenAITransportErrTestContext()

	before := time.Now()
	retErr := svc.handleOpenAIUpstreamTransportError(context.Background(), c, account,
		errors.New(`Post "https://chatgpt.com/backend-api/codex/responses": socks connect tcp 85.255.176.68:12324->chatgpt.com:443: username/password authentication failed`), false)
	after := time.Now()

	// Failover error (handler will switch accounts), not a direct response.
	var fo *UpstreamFailoverError
	require.True(t, errors.As(retErr, &fo), "persistent error must return *UpstreamFailoverError")
	require.Equal(t, http.StatusBadGateway, fo.StatusCode)

	// Persistent → account temporarily unscheduled for ~10min, reason carries cause.
	require.Len(t, repo.tempUnschedCalls, 1)
	require.Equal(t, int64(4627), repo.tempUnschedCalls[0].accountID)
	require.Contains(t, repo.tempUnschedCalls[0].reason, "authentication failed")
	require.True(t, repo.tempUnschedCalls[0].until.After(before.Add(openAITransportErrorTempUnschedDuration-time.Second)))
	require.True(t, repo.tempUnschedCalls[0].until.Before(after.Add(openAITransportErrorTempUnschedDuration+time.Second)))

	// Immediate in-memory effect so subsequent requests skip it before DB/cache catches up.
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))

	// Must NOT write a response body — the handler owns the (failover) response.
	require.Equal(t, 0, rec.Body.Len())
}

// A transient blip should fail over but must NOT evict the account.
func TestHandleOpenAIUpstreamTransportError_TransientFailsOverWithoutEviction(t *testing.T) {
	repo := &openaiTransportAccountRepoStub{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 99, Name: "flaky", Platform: PlatformOpenAI}
	c, rec := newOpenAITransportErrTestContext()

	err := svc.handleOpenAIUpstreamTransportError(context.Background(), c, account,
		errors.New(`Post "https://chatgpt.com/...": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`), false)

	var fo *UpstreamFailoverError
	require.True(t, errors.As(err, &fo), "transient error must return *UpstreamFailoverError")
	require.Equal(t, http.StatusBadGateway, fo.StatusCode)

	// Transient → do NOT evict.
	require.Empty(t, repo.tempUnschedCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.Equal(t, 0, rec.Body.Len())
}

// context.Canceled means the client disconnected — do NOT fail over to another
// account and do NOT temporarily evict this one.
func TestHandleOpenAIUpstreamTransportError_ContextCanceled_NoFailoverNoEviction(t *testing.T) {
	repo := &openaiTransportAccountRepoStub{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 77, Name: "healthy", Platform: PlatformOpenAI}
	c, rec := newOpenAITransportErrTestContext()

	err := svc.handleOpenAIUpstreamTransportError(context.Background(), c, account,
		context.Canceled, false)

	// Must NOT be a failover error.
	var fo *UpstreamFailoverError
	require.False(t, errors.As(err, &fo), "context.Canceled must NOT return *UpstreamFailoverError")
	require.NotNil(t, err, "must return a non-nil error")

	// Must NOT evict the account.
	require.Empty(t, repo.tempUnschedCalls, "context.Canceled must not trigger temp-unsched DB write")
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account), "context.Canceled must not block account in-memory")

	// Must NOT write a response body.
	require.Equal(t, 0, rec.Body.Len())
}

// context.Canceled wrapped inside another error must also avoid failover.
func TestHandleOpenAIUpstreamTransportError_WrappedContextCanceled_NoFailover(t *testing.T) {
	repo := &openaiTransportAccountRepoStub{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 78, Name: "healthy2", Platform: PlatformOpenAI}
	c, _ := newOpenAITransportErrTestContext()

	wrapped := fmt.Errorf("http request failed: %w", context.Canceled)
	err := svc.handleOpenAIUpstreamTransportError(context.Background(), c, account, wrapped, false)

	var fo *UpstreamFailoverError
	require.False(t, errors.As(err, &fo), "wrapped context.Canceled must NOT return *UpstreamFailoverError")
	require.Empty(t, repo.tempUnschedCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

// When accountRepo is nil (no DB), in-memory block must still happen but the
// success log "openai.account_temp_unscheduled_transport" must NOT fire (it
// would be misleading: the account is only blocked in memory, not persisted).
// We verify the in-memory block occurs and no DB call is made.
func TestTempUnscheduleOpenAITransportError_NilAccountRepo_InMemoryBlockOnly(t *testing.T) {
	// nil accountRepo → no DB write.
	svc := &OpenAIGatewayService{accountRepo: nil}
	account := &Account{ID: 55, Name: "no-db", Platform: PlatformOpenAI}

	svc.tempUnscheduleOpenAITransportError(context.Background(), account, "proxy refused")

	// In-memory block must still happen.
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account),
		"in-memory block must apply even when accountRepo is nil")
}

// context.DeadlineExceeded is NOT special-cased — a slow upstream is worth failing over.
func TestHandleOpenAIUpstreamTransportError_DeadlineExceeded_StillFailsOver(t *testing.T) {
	repo := &openaiTransportAccountRepoStub{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 79, Name: "slow", Platform: PlatformOpenAI}
	c, _ := newOpenAITransportErrTestContext()

	err := svc.handleOpenAIUpstreamTransportError(context.Background(), c, account,
		context.DeadlineExceeded, false)

	var fo *UpstreamFailoverError
	require.True(t, errors.As(err, &fo), "context.DeadlineExceeded must still return *UpstreamFailoverError")
}

func TestForwardAsRawChatCompletions_TransportErrorFailsOver(t *testing.T) {
	repo := &openaiTransportAccountRepoStub{}
	upstream := &failingOpenAIHTTPUpstream{
		err: errors.New(`Post "https://opencode.ai/zen/v1/chat/completions": EOF`),
	}
	svc := &OpenAIGatewayService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg: &config.Config{
			Security: config.SecurityConfig{
				URLAllowlist: config.URLAllowlistConfig{Enabled: false},
			},
		},
	}
	account := &Account{
		ID:          81,
		Name:        "oc-20053",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://opencode.ai/zen/v1"},
	}
	c, rec := newOpenAITransportErrTestContext()
	body := []byte(`{"model":"deepseek-v4-flash-free","messages":[{"role":"user","content":"hello"}]}`)

	_, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")

	require.Equal(t, 1, upstream.calls)
	var fo *UpstreamFailoverError
	require.True(t, errors.As(err, &fo), "transport error must trigger account failover")
	require.Equal(t, http.StatusBadGateway, fo.StatusCode)
	require.Empty(t, repo.tempUnschedCalls, "plain EOF is transient: fail over but do not evict")
	require.Equal(t, 0, rec.Body.Len(), "service must not write a hard 502 before handler can fail over")
}
