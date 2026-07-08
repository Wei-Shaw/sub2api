//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// anthropicTransportAccountRepoStub records SetTempUnschedulable calls. It embeds
// the (nil) AccountRepository interface so any other method call would panic —
// the helper under test must only touch SetTempUnschedulable. tempUnschedCall is
// shared with antigravity_internal500_penalty_test.go (same package).
type anthropicTransportAccountRepoStub struct {
	AccountRepository
	tempUnschedCalls []tempUnschedCall
}

func (r *anthropicTransportAccountRepoStub) SetTempUnschedulable(_ context.Context, id int64, until time.Time, reason string) error {
	r.tempUnschedCalls = append(r.tempUnschedCalls, tempUnschedCall{accountID: id, until: until, reason: reason})
	return nil
}

func newAnthropicTransportErrTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return c, rec
}

// A durable proxy/credential failure must (a) temporarily unschedule the account
// so it stops being hammered, and (b) return a failover error so the handler
// switches to a healthy account instead of writing a hard 502 itself.
func TestHandleAnthropicUpstreamTransportError_PersistentEvictsAndFailsOver(t *testing.T) {
	repo := &anthropicTransportAccountRepoStub{}
	svc := &GatewayService{accountRepo: repo}
	account := &Account{ID: 3301, Name: "proxy-expired", Platform: PlatformAnthropic}
	c, rec := newAnthropicTransportErrTestContext()

	before := time.Now()
	retErr := svc.handleAnthropicUpstreamTransportError(context.Background(), c, account,
		"https://api.anthropic.com/v1/messages",
		errors.New(`Post "https://api.anthropic.com/v1/messages": socks connect tcp 85.255.176.68:12324->api.anthropic.com:443: username/password authentication failed`), false)
	after := time.Now()

	// Failover error (handler will switch accounts), not a direct response.
	var fo *UpstreamFailoverError
	require.True(t, errors.As(retErr, &fo), "persistent error must return *UpstreamFailoverError")
	require.Equal(t, http.StatusBadGateway, fo.StatusCode)
	// Anthropic-format failover body so exhausted-failover passthrough rules see
	// the same client-visible payload as the legacy inline 502.
	require.JSONEq(t, `{"type":"error","error":{"type":"upstream_error","message":"Upstream request failed"}}`, string(fo.ResponseBody))

	// Persistent → account temporarily unscheduled for ~10min, reason carries cause.
	require.Len(t, repo.tempUnschedCalls, 1)
	require.Equal(t, int64(3301), repo.tempUnschedCalls[0].accountID)
	require.Contains(t, repo.tempUnschedCalls[0].reason, "authentication failed")
	require.True(t, repo.tempUnschedCalls[0].until.After(before.Add(anthropicTransportErrorTempUnschedDuration-time.Second)))
	require.True(t, repo.tempUnschedCalls[0].until.Before(after.Add(anthropicTransportErrorTempUnschedDuration+time.Second)))

	// Must NOT write a response body — the handler owns the (failover) response.
	require.Equal(t, 0, rec.Body.Len())
}

// A transient blip should fail over but must NOT evict the account.
func TestHandleAnthropicUpstreamTransportError_TransientFailsOverWithoutEviction(t *testing.T) {
	repo := &anthropicTransportAccountRepoStub{}
	svc := &GatewayService{accountRepo: repo}
	account := &Account{ID: 99, Name: "flaky", Platform: PlatformAnthropic}
	c, rec := newAnthropicTransportErrTestContext()

	err := svc.handleAnthropicUpstreamTransportError(context.Background(), c, account,
		"https://api.anthropic.com/v1/messages",
		errors.New(`Post "https://api.anthropic.com/v1/messages": read tcp 10.0.0.2:39724->160.79.104.10:443: read: connection reset by peer`), false)

	var fo *UpstreamFailoverError
	require.True(t, errors.As(err, &fo), "transient error must return *UpstreamFailoverError")
	require.Equal(t, http.StatusBadGateway, fo.StatusCode)

	// Transient → do NOT evict.
	require.Empty(t, repo.tempUnschedCalls)
	require.Equal(t, 0, rec.Body.Len())
}

// context.Canceled means the client disconnected — do NOT fail over to another
// account and do NOT temporarily evict this one.
func TestHandleAnthropicUpstreamTransportError_ContextCanceled_NoFailoverNoEviction(t *testing.T) {
	repo := &anthropicTransportAccountRepoStub{}
	svc := &GatewayService{accountRepo: repo}
	account := &Account{ID: 77, Name: "healthy", Platform: PlatformAnthropic}
	c, rec := newAnthropicTransportErrTestContext()

	err := svc.handleAnthropicUpstreamTransportError(context.Background(), c, account,
		"https://api.anthropic.com/v1/messages", context.Canceled, false)

	// Must NOT be a failover error.
	var fo *UpstreamFailoverError
	require.False(t, errors.As(err, &fo), "context.Canceled must NOT return *UpstreamFailoverError")
	require.NotNil(t, err, "must return a non-nil error")

	// Must NOT evict the account.
	require.Empty(t, repo.tempUnschedCalls, "context.Canceled must not trigger temp-unsched DB write")

	// Must NOT write a response body.
	require.Equal(t, 0, rec.Body.Len())
}

// context.Canceled wrapped inside another error must also avoid failover.
func TestHandleAnthropicUpstreamTransportError_WrappedContextCanceled_NoFailover(t *testing.T) {
	repo := &anthropicTransportAccountRepoStub{}
	svc := &GatewayService{accountRepo: repo}
	account := &Account{ID: 78, Name: "healthy2", Platform: PlatformAnthropic}
	c, _ := newAnthropicTransportErrTestContext()

	wrapped := fmt.Errorf("http request failed: %w", context.Canceled)
	err := svc.handleAnthropicUpstreamTransportError(context.Background(), c, account,
		"https://api.anthropic.com/v1/messages", wrapped, false)

	var fo *UpstreamFailoverError
	require.False(t, errors.As(err, &fo), "wrapped context.Canceled must NOT return *UpstreamFailoverError")
	require.Empty(t, repo.tempUnschedCalls)
}

// context.DeadlineExceeded is NOT special-cased — a hung upstream is worth failing over.
func TestHandleAnthropicUpstreamTransportError_DeadlineExceeded_StillFailsOver(t *testing.T) {
	repo := &anthropicTransportAccountRepoStub{}
	svc := &GatewayService{accountRepo: repo}
	account := &Account{ID: 79, Name: "slow", Platform: PlatformAnthropic}
	c, _ := newAnthropicTransportErrTestContext()

	err := svc.handleAnthropicUpstreamTransportError(context.Background(), c, account,
		"https://api.anthropic.com/v1/messages", context.DeadlineExceeded, false)

	var fo *UpstreamFailoverError
	require.True(t, errors.As(err, &fo), "context.DeadlineExceeded must still return *UpstreamFailoverError")
}

// A nil accountRepo (no DB) must not panic on a persistent fault; the error must
// still fail over so the request survives on another account.
func TestHandleAnthropicUpstreamTransportError_NilAccountRepo_StillFailsOver(t *testing.T) {
	svc := &GatewayService{accountRepo: nil}
	account := &Account{ID: 55, Name: "no-db", Platform: PlatformAnthropic}
	c, _ := newAnthropicTransportErrTestContext()

	err := svc.handleAnthropicUpstreamTransportError(context.Background(), c, account,
		"https://api.anthropic.com/v1/messages",
		errors.New(`Post "https://api.anthropic.com/v1/messages": connect: connection refused`), false)

	var fo *UpstreamFailoverError
	require.True(t, errors.As(err, &fo), "nil accountRepo must not prevent failover")
}
