package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type gatewayTransportAccountRepoStub struct {
	AccountRepository
	tempUnschedCalls []gatewayTempUnschedCall
}

type gatewayTempUnschedCall struct {
	accountID int64
	until     time.Time
	reason    string
}

func (r *gatewayTransportAccountRepoStub) SetTempUnschedulable(_ context.Context, id int64, until time.Time, reason string) error {
	r.tempUnschedCalls = append(r.tempUnschedCalls, gatewayTempUnschedCall{accountID: id, until: until, reason: reason})
	return nil
}

func newGatewayTransportErrTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return c, rec
}

func TestHandleGatewayUpstreamTransportError_PersistentFailsOverAndUnschedules(t *testing.T) {
	repo := &gatewayTransportAccountRepoStub{}
	svc := &GatewayService{accountRepo: repo}
	account := &Account{ID: 4627, Name: "local-llm-down", Platform: PlatformAnthropic}
	c, rec := newGatewayTransportErrTestContext()

	before := time.Now()
	err := svc.handleGatewayUpstreamTransportError(
		context.Background(),
		c,
		account,
		"http://127.0.0.1:30001/v1/messages",
		errors.New(`dial tcp 127.0.0.1:30001: connect: connection refused`),
		true,
	)
	after := time.Now()

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
	require.JSONEq(t, string(gatewayTransportFailoverBody), string(failoverErr.ResponseBody))
	require.Len(t, repo.tempUnschedCalls, 1)
	require.Equal(t, int64(4627), repo.tempUnschedCalls[0].accountID)
	require.Contains(t, repo.tempUnschedCalls[0].reason, "connection refused")
	require.True(t, repo.tempUnschedCalls[0].until.After(before.Add(upstreamTransportErrorTempUnschedDuration-time.Second)))
	require.True(t, repo.tempUnschedCalls[0].until.Before(after.Add(upstreamTransportErrorTempUnschedDuration+time.Second)))
	require.NotNil(t, account.TempUnschedulableUntil)
	require.Equal(t, repo.tempUnschedCalls[0].reason, account.TempUnschedulableReason)
	require.Equal(t, 0, rec.Body.Len(), "the handler must own the response during failover")
}

func TestHandleGatewayUpstreamTransportError_TransientFailsOverWithoutUnscheduling(t *testing.T) {
	repo := &gatewayTransportAccountRepoStub{}
	svc := &GatewayService{accountRepo: repo}
	account := &Account{ID: 99, Name: "local-llm-flaky", Platform: PlatformAnthropic}
	c, rec := newGatewayTransportErrTestContext()

	err := svc.handleGatewayUpstreamTransportError(
		context.Background(),
		c,
		account,
		"http://127.0.0.1:30001/v1/messages",
		errors.New(`Post "http://127.0.0.1:30001/v1/messages": context deadline exceeded`),
		false,
	)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Empty(t, repo.tempUnschedCalls)
	require.Nil(t, account.TempUnschedulableUntil)
	require.Equal(t, 0, rec.Body.Len())
}

func TestHandleGatewayUpstreamTransportError_ContextCanceledDoesNotFailOver(t *testing.T) {
	repo := &gatewayTransportAccountRepoStub{}
	svc := &GatewayService{accountRepo: repo}
	account := &Account{ID: 77, Name: "local-llm-healthy", Platform: PlatformAnthropic}
	c, rec := newGatewayTransportErrTestContext()

	err := svc.handleGatewayUpstreamTransportError(context.Background(), c, account, "http://127.0.0.1:30001", context.Canceled, false)

	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, repo.tempUnschedCalls)
	require.Nil(t, account.TempUnschedulableUntil)
	require.Equal(t, 0, rec.Body.Len())
}
