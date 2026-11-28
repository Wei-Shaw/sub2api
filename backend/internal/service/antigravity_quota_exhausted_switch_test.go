//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Google 在 5h/周配额窗口耗尽时返回的 429：带 QUOTA_EXHAUSTED 与 quotaResetDelay。
const quotaExhaustedAntigravity429Body = `{"error":{"code":429,"message":"You have exhausted your capacity on this model. Your quota will reset after 2h.","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"QUOTA_EXHAUSTED","metadata":{"quotaResetDelay":"2h0m0s"}}]}}`

// 只在 message 里写 QUOTA_EXHAUSTED、没有 details 与重置时间：同时属于"裸 429"。
const quotaExhaustedMessageOnly429Body = `{"error":{"status":"RESOURCE_EXHAUSTED","message":"QUOTA_EXHAUSTED"}}`

type quotaLoopOutcome struct {
	result    *antigravityRetryLoopResult
	err       error
	calls     int
	cooled    int
	cooledFor int
}

func runQuotaExhaustedLoop(t *testing.T, c *gin.Context, account *Account, body string, reqBody []byte) quotaLoopOutcome {
	t.Helper()
	upstream := &fixed429Upstream{body: body}
	repo := &stubAntigravityAccountRepo{}
	svc := &AntigravityGatewayService{accountRepo: repo}
	var out quotaLoopOutcome
	if reqBody == nil {
		reqBody = []byte(`{"input":"test"}`)
	}
	out.result, out.err = svc.antigravityRetryLoop(antigravityRetryLoopParams{
		prefix:         "[test]",
		ctx:            context.Background(),
		account:        account,
		accessToken:    "token",
		action:         "streamGenerateContent",
		body:           reqBody,
		c:              c,
		httpUpstream:   upstream,
		accountRepo:    repo,
		requestedModel: "gemini-4-argon-high",
		handleError: func(ctx context.Context, prefix string, acc *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			out.cooled++
			out.cooledFor = statusCode
			return nil
		},
	})
	out.calls = upstream.calls
	if out.result != nil && out.result.resp != nil {
		_ = out.result.resp.Body.Close()
	}
	return out
}

// 配额耗尽：只打一次上游，冷却账号并立即返回切换信号，不在同一账号上指数退避重试。
func TestAntigravityRetryLoop_QuotaExhausted_SwitchesWithoutSameAccountRetries(t *testing.T) {
	setupBare429Test(t)
	c := newBare429TestContext()

	out := runQuotaExhaustedLoop(t, c, bare429Account(13), quotaExhaustedAntigravity429Body, nil)

	switchErr, ok := IsAntigravityAccountSwitchError(out.err)
	require.True(t, ok, "quota exhausted must return an account switch signal, got %v", out.err)
	require.Equal(t, int64(13), switchErr.OriginalAccountID)
	require.Equal(t, "gemini-4-argon-high", switchErr.RateLimitedModel)
	require.Nil(t, out.result)
	require.Equal(t, 1, out.calls, "no same-account retries after QUOTA_EXHAUSTED")
	require.Equal(t, 1, out.cooled, "the exhausted account is cooled exactly once")
	require.Equal(t, http.StatusTooManyRequests, out.cooledFor)

	// 切换信号到上层只剩 503，上游 429 的原因必须留在运维事件里。
	raw, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := raw.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, "failover", events[0].Kind)
	require.Equal(t, http.StatusTooManyRequests, events[0].UpstreamStatusCode)
	require.Contains(t, events[0].Message, "exhausted your capacity")
}

// 只有 message 的 QUOTA_EXHAUSTED 同时是裸 429：立即切换，但仍遵守"一个请求最多冷却一个账号"。
func TestAntigravityRetryLoop_MessageOnlyQuotaExhausted_KeepsBare429Scope(t *testing.T) {
	setupBare429Test(t)
	c := newBare429TestContext()

	first := runQuotaExhaustedLoop(t, c, bare429Account(13), quotaExhaustedMessageOnly429Body, nil)
	_, ok := IsAntigravityAccountSwitchError(first.err)
	require.True(t, ok)
	require.Equal(t, 1, first.calls)
	require.Equal(t, 1, first.cooled, "first account is cooled")

	second := runQuotaExhaustedLoop(t, c, bare429Account(6), quotaExhaustedMessageOnly429Body, nil)
	_, ok = IsAntigravityAccountSwitchError(second.err)
	require.True(t, ok, "still switches away immediately")
	require.Equal(t, 1, second.calls)
	require.Zero(t, second.cooled, "same request on a second account: not cooled (bare 429 scope)")
}

// 开启 overages 且积分重试失败：只额外尝试一次积分请求，随后切换，不再逐次重复积分重试。
func TestAntigravityRetryLoop_QuotaExhausted_CreditsRetryFailed_SwitchesOnce(t *testing.T) {
	setupBare429Test(t)
	account := bare429Account(21)
	account.Type = AccountTypeOAuth
	account.Extra = map[string]any{"allow_overages": true}

	out := runQuotaExhaustedLoop(t, newBare429TestContext(), account, quotaExhaustedAntigravity429Body,
		[]byte(`{"model":"gemini-4-argon-high","request":{}}`))

	_, ok := IsAntigravityAccountSwitchError(out.err)
	require.True(t, ok, "got %v", out.err)
	require.Equal(t, 2, out.calls, "original request + one credits retry, no further same-account attempts")
	require.Equal(t, 1, out.cooled)
}

// 不变式：没有配额耗尽关键词的裸 429 仍走默认同号重试，重试用尽后返回 429 响应。
func TestAntigravityRetryLoop_Bare429WithoutQuotaKeyword_StillRetriesSameAccount(t *testing.T) {
	setupBare429Test(t)

	out := runQuotaExhaustedLoop(t, newBare429TestContext(), bare429Account(31), bareAntigravity429Body, nil)

	require.NoError(t, out.err)
	require.NotNil(t, out.result)
	require.Equal(t, http.StatusTooManyRequests, out.result.resp.StatusCode)
	require.Equal(t, antigravityMaxRetries, out.calls)
	require.Equal(t, 1, out.cooled)
}

// 不变式：非 Antigravity 平台账号不进入该分支（与 shouldTriggerAntigravitySmartRetry 的平台口径一致）。
func TestHandleSmartRetry_QuotaExhausted_NonAntigravityAccountContinues(t *testing.T) {
	account := &Account{ID: 41, Name: "acc-41", Platform: PlatformAnthropic}
	respBody := []byte(quotaExhaustedAntigravity429Body)
	resp := &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(respBody))}
	cooled := false
	params := antigravityRetryLoopParams{
		ctx:     context.Background(),
		prefix:  "[test]",
		account: account,
		handleError: func(ctx context.Context, prefix string, acc *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			cooled = true
			return nil
		},
	}

	result := (&AntigravityGatewayService{}).handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, []string{"https://ag-1.test"})

	require.Equal(t, smartRetryActionContinue, result.action)
	require.Nil(t, result.switchError)
	require.False(t, cooled)
}

// Claude 路径的二次重试（签名/预算修正）收到切换信号时，转为 failover，而不是吞掉后返回原始 400。
func TestAntigravityRetrySwitchToFailover(t *testing.T) {
	c := newBare429TestContext()
	account := bare429Account(51)

	switchErr := &AntigravityAccountSwitchError{OriginalAccountID: 51, RateLimitedModel: "m", IsStickySession: true}
	failover := antigravityRetrySwitchToFailover(c, account, fmt.Errorf("wrapped: %w", switchErr))
	require.NotNil(t, failover)
	require.Equal(t, http.StatusServiceUnavailable, failover.StatusCode)
	require.True(t, failover.ForceCacheBilling)

	require.Nil(t, antigravityRetrySwitchToFailover(c, account, errors.New("dial tcp: timeout")))
	require.Nil(t, antigravityRetrySwitchToFailover(c, account, nil))
}
