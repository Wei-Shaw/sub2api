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
	"github.com/tidwall/gjson"
)

const capacityShedOverloadedBody = `{"error":{"message":"Our servers are currently overloaded. Please try again later.","type":"server_error","code":"server_is_overloaded"}}`

func capacityShedOverloaded503() *http.Response {
	return &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(capacityShedOverloadedBody)),
	}
}

func capacityShedOverloadedAnswers(n int) []*http.Response {
	answers := make([]*http.Response, 0, n)
	for i := 0; i < n; i++ {
		answers = append(answers, capacityShedOverloaded503())
	}
	return answers
}

func TestCapacityShedAttemptsExhausted(t *testing.T) {
	overloaded := &service.UpstreamFailoverError{
		StatusCode:             http.StatusServiceUnavailable,
		ResponseBody:           []byte(capacityShedOverloadedBody),
		RetryableOnSameAccount: true,
		RequestScopedTransient: true,
	}
	other := &service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, RetryableOnSameAccount: true}

	t.Run("unlimited_by_default", func(t *testing.T) {
		attempts := 0
		for i := 0; i < 100; i++ {
			require.False(t, capacityShedAttemptsExhausted(overloaded, &attempts, 0))
			require.False(t, capacityShedAttemptsExhausted(overloaded, &attempts, -1))
		}
		require.Zero(t, attempts)
	})

	t.Run("configured_limit", func(t *testing.T) {
		attempts := 0
		for i := 1; i < 6; i++ {
			require.False(t, capacityShedAttemptsExhausted(overloaded, &attempts, 6), "attempt %d", i)
		}
		require.True(t, capacityShedAttemptsExhausted(overloaded, &attempts, 6))
		require.Equal(t, 6, attempts)
	})

	t.Run("other_errors_not_counted", func(t *testing.T) {
		attempts := 0
		for i := 0; i < 20; i++ {
			require.False(t, capacityShedAttemptsExhausted(other, &attempts, 2))
		}
		require.False(t, capacityShedAttemptsExhausted(nil, &attempts, 2))
		require.Zero(t, attempts)
	})
}

// 默认不限：上游持续过载时行为与原来一致，两个账号各 1+3 次后因无号可换结束。
func TestOpenAIGatewayHandlerResponses_CapacityShedUnlimitedByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := newAstraProCapturedUpstream(capacityShedOverloadedAnswers(20)...)
	handler := newOpenAIResponsesFailoverTestHandler(t, upstream)
	c, rec := newOpenAIResponsesFailoverTestContext(t, nil)

	handler.Responses(c)

	_, accountIDs, _ := upstream.snapshot()
	require.Equal(t, []int64{1, 1, 1, 1, 2, 2, 2, 2}, accountIDs)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// 上限设为 6：总尝试次数为 6（第一个账号 1+3 次同号重试，第二个账号 2 次），
// 之后不再换号，直接把 503 交还客户端。
func TestOpenAIGatewayHandlerResponses_CapacityShedStopsAtConfiguredAttemptCap(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := newAstraProCapturedUpstream(capacityShedOverloadedAnswers(20)...)
	handler := newOpenAIResponsesFailoverTestHandler(t, upstream)
	handler.cfg.Gateway.MaxCapacityShedAttempts = 6
	c, rec := newOpenAIResponsesFailoverTestContext(t, nil)

	handler.Responses(c)

	_, accountIDs, _ := upstream.snapshot()
	require.Equal(t, []int64{1, 1, 1, 1, 2, 2}, accountIDs)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "server_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
	require.Contains(t, gjson.GetBytes(rec.Body.Bytes(), "error.message").String(), "overloaded")
}

// 上限内恢复不受影响：同号过载 2 次后第 3 次成功。
func TestOpenAIGatewayHandlerResponses_CapacityShedCapDoesNotBlockRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := newAstraProCapturedUpstream(capacityShedOverloaded503(), capacityShedOverloaded503(), astra200())
	handler := newOpenAIResponsesFailoverTestHandler(t, upstream)
	handler.cfg.Gateway.MaxCapacityShedAttempts = 6
	c, rec := newOpenAIResponsesFailoverTestContext(t, nil)

	handler.Responses(c)

	_, accountIDs, _ := upstream.snapshot()
	require.Len(t, accountIDs, 3)
	require.Equal(t, http.StatusOK, rec.Code)
}
