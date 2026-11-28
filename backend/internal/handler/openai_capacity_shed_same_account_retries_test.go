//go:build unit

package handler

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func overloadedResponsesUpstreamAnswer() *http.Response {
	return &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(bytes.NewBufferString(
			`{"error":{"message":"Our servers are currently overloaded. Please try again later.","type":"server_error","code":"server_is_overloaded"}}`)),
	}
}

func overloadedResponsesUpstreamAnswers(n int) []*http.Response {
	out := make([]*http.Response, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, overloadedResponsesUpstreamAnswer())
	}
	return out
}

// 同号重试设为 1：每个账号 2 次（1 次 + 1 次同号重试）后换号。
func TestOpenAIGatewayHandlerResponses_CapacityShedSameAccountRetriesSwitchesEarlier(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := newAstraProCapturedUpstream(overloadedResponsesUpstreamAnswers(20)...)
	handler := newOpenAIResponsesFailoverTestHandler(t, upstream)
	handler.cfg.Gateway.MaxCapacityShedSameAccountRetries = 1
	c, rec := newOpenAIResponsesFailoverTestContext(t, nil)

	handler.Responses(c)

	_, accountIDs, _ := upstream.snapshot()
	require.Equal(t, []int64{1, 1, 2, 2}, accountIDs)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// 同号重试设为 1 时，第二个账号首次即成功。
func TestOpenAIGatewayHandlerResponses_CapacityShedSameAccountRetriesRecoversOnNextAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := newAstraProCapturedUpstream(overloadedResponsesUpstreamAnswer(), overloadedResponsesUpstreamAnswer(), astra200())
	handler := newOpenAIResponsesFailoverTestHandler(t, upstream)
	handler.cfg.Gateway.MaxCapacityShedSameAccountRetries = 1
	c, rec := newOpenAIResponsesFailoverTestContext(t, nil)

	handler.Responses(c)

	_, accountIDs, _ := upstream.snapshot()
	require.Equal(t, []int64{1, 1, 2}, accountIDs)
	require.Equal(t, http.StatusOK, rec.Code)
}

// 账号自身的同号重试次数更小时以账号为准（池模式 0 = 不重试）。
func TestCapacityShedSameAccountRetriesNeverRaisesAccountLimit(t *testing.T) {
	account := &service.Account{
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"pool_mode": true, "pool_mode_retry_count": 0},
	}
	err := &service.UpstreamFailoverError{RetryableOnSameAccount: true, RequestScopedTransient: true, SameAccountRetryMax: 2}
	require.Zero(t, effectiveSameAccountRetryLimit(err, account))
	require.False(t, sameAccountRetryAllowed(err, 0, effectiveSameAccountRetryLimit(err, account)))
}
