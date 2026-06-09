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
