//go:build unit

package service

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// 固定时钟：解析器是纯函数，所有断言值以 UTC 写死，杜绝时区/时钟竞态。
var resetAtFixedNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

func TestParseResetAtTimestamp_ZWithMillis(t *testing.T) {
	// 带毫秒的 RFC3339 UTC 串
	at, assumed := parseResetAtTimestampInMessage(
		"You've reached your weekly usage limit for your plan. Your limit resets at 2026-10-17T09:17:26.406Z. Please wait for the window to reset.", resetAtFixedNow)
	require.NotNil(t, at)
	require.False(t, assumed)
	require.Equal(t, "2026-10-17T09:17:26.406Z", at.UTC().Format("2006-01-02T15:04:05.000Z"))
}

func TestParseResetAtTimestamp_ZSeconds(t *testing.T) {
	// 秒级 RFC3339 UTC 串；now=12:00 已过 08:03 的那条应被过期护栏忽略
	at, assumed := parseResetAtTimestampInMessage(
		"Coding Plan quota is exhausted and wallet balance is insufficient. Coding Plan quota resets at 2026-10-16T08:03:20Z; this does not replenish your wallet.", resetAtFixedNow)
	require.Nil(t, at, "past reset time must be ignored")
	_ = assumed

	// 换未来时间验证解析本体
	at2, assumed2 := parseResetAtTimestampInMessage(
		"Coding Plan quota resets at 2026-10-16T20:03:20Z; this does not replenish your wallet.", resetAtFixedNow)
	require.NotNil(t, at2)
	require.False(t, assumed2)
	require.Equal(t, "2026-10-16T20:03:20Z", at2.UTC().Format(time.RFC3339))
}

func TestParseResetAtTimestamp_BeijingOffset(t *testing.T) {
	// 带偏移的串必须换算为正确的 UTC 时刻（时区红线：不得出时差）
	at, assumed := parseResetAtTimestampInMessage("Your limit resets at 2026-10-17T17:26:26+08:00.", resetAtFixedNow)
	require.NotNil(t, at)
	require.False(t, assumed)
	require.Equal(t, "2026-10-17T09:26:26Z", at.UTC().Format(time.RFC3339), "17:26+08:00 must equal 09:26 UTC")
}

func TestParseResetAtTimestamp_NaiveAssumedUTC(t *testing.T) {
	// naive 串（防御态）：一律按 UTC（宁晚不早）
	atT, assumedT := parseResetAtTimestampInMessage("resets at 2026-10-17T09:26:26", resetAtFixedNow)
	require.NotNil(t, atT)
	require.True(t, assumedT)
	require.Equal(t, "2026-10-17T09:26:26Z", atT.UTC().Format(time.RFC3339))

	atSpace, assumedSpace := parseResetAtTimestampInMessage("resets at 2026-10-17 09:26:26", resetAtFixedNow)
	require.NotNil(t, atSpace)
	require.True(t, assumedSpace)
	require.Equal(t, "2026-10-17T09:26:26Z", atSpace.UTC().Format(time.RFC3339))
}

func TestParseResetAtTimestamp_TooFarIgnored(t *testing.T) {
	at, _ := parseResetAtTimestampInMessage("resets at 2026-12-01T09:00:00Z", resetAtFixedNow)
	require.Nil(t, at, "reset beyond 35d horizon must be ignored")
}

func TestParseResetAtTimestamp_NoTimestampTextIgnored(t *testing.T) {
	// 无时间文案 / 相对时长 / 钟点写法均不得误匹配
	cases := []string{
		`Upstream rate limit exceeded, please retry later`,
		`Resets in 2h 41m (3:00 PM)`,
		`It resets in 3 days.`,
		`You've reached your 5-hour usage limit. Resets in 2h 41m.`,
	}
	for _, msg := range cases {
		at, _ := parseResetAtTimestampInMessage(msg, resetAtFixedNow)
		require.Nil(t, at, "must not match: %s", msg)
	}
}

func TestParseBodyResetAt_FromMessageField(t *testing.T) {
	body := fmt.Sprintf(`{"error":{"message":"Your limit resets at %s. Please wait.","type":"rate_limit_error"}}`,
		resetAtFixedNow.Add(6*time.Hour).Format(time.RFC3339))
	at, assumed := parseBodyResetAt([]byte(body), resetAtFixedNow)
	require.NotNil(t, at)
	require.False(t, assumed)
	require.WithinDuration(t, resetAtFixedNow.Add(6*time.Hour), *at, time.Second)
}

// ---- 接入点测试（stub + HandleUpstreamError，body 用动态未来时间避免时钟竞态） ----

func newResetAtTestAccount(platform string, id int64) *Account {
	return &Account{ID: id, Platform: platform, Type: AccountTypeAPIKey}
}

func futureRFC3339(d time.Duration) string {
	return time.Now().Add(d).UTC().Format("2006-01-02T15:04:05Z")
}

func runUpstream(service *RateLimitService, account *Account, statusCode int, body string) bool {
	return service.HandleUpstreamError(context.Background(), account, statusCode, http.Header{}, []byte(body))
}

func TestHandleUpstreamError_402WithResetAtTempUnschedulableInsteadOfSetError(t *testing.T) {
	// 402 带明确恢复点 → 临时停调到期自愈，不再永久 SetError
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newResetAtTestAccount(PlatformOpenAI, 1290)
	body := fmt.Sprintf(`{"error":{"message":"[code=insufficient_balance] Coding Plan quota is exhausted and wallet balance is insufficient. Please top up your wallet to continue now. Coding Plan quota resets at %s; this does not replenish your wallet.","type":"insufficient_balance"}}`, futureRFC3339(2*time.Hour))

	shouldDisable := runUpstream(service, account, http.StatusPaymentRequired, body)

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.tempCalls)
	require.Equal(t, 0, repo.setErrorCalls, "402 with explicit reset time must never SetError")
	require.Contains(t, repo.lastTempReason, "Payment required (402)")
	require.WithinDuration(t, time.Now().Add(2*time.Hour).UTC(), repo.lastTempUntil.UTC(), time.Minute)
}

func TestHandleUpstreamError_402WithoutResetAtStillSetError(t *testing.T) {
	// 反向守护：无时间的 402 维持人工介入语义（SetError）不变
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newResetAtTestAccount(PlatformOpenAI, 1291)

	shouldDisable := runUpstream(service, account, http.StatusPaymentRequired, `{"error":{"message":"insufficient balance"}}`)

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.setErrorCalls)
	require.Equal(t, 0, repo.tempCalls)
}

func TestHandleUpstreamError_400UsageLimitWithResetAtTempUnschedulable(t *testing.T) {
	// 上游把窗口限流打成 400 → 停调到文案时间
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newResetAtTestAccount(PlatformOpenAI, 1170)
	body := fmt.Sprintf(`{"error":{"message":"You've reached your weekly usage limit for your plan. Your limit resets at %s. Please wait for the window to reset or upgrade your plan to continue.","type":"invalid_request_error"}}`, futureRFC3339(26*time.Hour))

	shouldDisable := runUpstream(service, account, http.StatusBadRequest, body)

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.tempCalls)
	require.Equal(t, 0, repo.setErrorCalls)
	require.Contains(t, repo.lastTempReason, "Usage window limit (400)")
	require.WithinDuration(t, time.Now().Add(26*time.Hour).UTC(), repo.lastTempUntil.UTC(), time.Minute)
}

func TestHandleUpstreamError_400PlainParamErrorUntouched(t *testing.T) {
	// 反向守护：普通参数 400 零处理（不断言 shouldDisable，400 本就不禁用）
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newResetAtTestAccount(PlatformOpenAI, 1171)

	_ = runUpstream(service, account, http.StatusBadRequest, `{"error":{"message":"Invalid option: expected one of \"off\"|\"low\"|\"medium\"|\"high\"","type":"invalid_request_error"}}`)

	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 0, repo.tempCalls)
	require.Equal(t, 0, repo.rateLimitedCalls)
}

func TestHandleUpstreamError_429MessageISORateLimitedToWindowReset(t *testing.T) {
	// 429 文案带 ISO 时间必须被采用，而非秒级兜底
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newResetAtTestAccount(PlatformOpenAI, 1263)
	body := fmt.Sprintf(`{"error":{"message":"You've reached your weekly usage limit for your plan. Your limit resets at %s. Please wait for the window to reset or upgrade your plan to continue.","type":"rate_limit_error"}}`, futureRFC3339(26*time.Hour))

	shouldDisable := runUpstream(service, account, http.StatusTooManyRequests, body)

	require.False(t, shouldDisable, "429 keeps existing shouldDisable=false semantics")
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.WithinDuration(t, time.Now().Add(26*time.Hour).UTC(), repo.lastRateLimitedAt.UTC(), time.Minute,
		"must cool down to the message reset time, not the seconds fallback")
}

func TestHandleUpstreamError_429WithoutResetTimeStillShortFallback(t *testing.T) {
	// 反向守护：无时间 429 仍走秒级兜底（默认 5s），行为不变
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newResetAtTestAccount(PlatformOpenAI, 1264)

	_ = runUpstream(service, account, http.StatusTooManyRequests, `{"error":{"message":"Upstream rate limit exceeded, please retry later","type":"rate_limit_error"}}`)

	require.Equal(t, 1, repo.rateLimitedCalls)
	require.WithinDuration(t, time.Now().UTC(), repo.lastRateLimitedAt.UTC(), time.Minute,
		"no-timestamp 429 must stay on the short seconds fallback")
}

func TestCommandCodeResetAt_PrefersMessageISOOverRelativeAndBounds(t *testing.T) {
	// 相对时长优先；绝对时刻次之，且受 maxWait 约束
	relativeBody := []byte(`{"error":{"message":"You've reached your 5-hour usage limit. Resets in 2h 41m"}}`)
	at := commandCodeResetAt(relativeBody, resetAtFixedNow, commandCodeMaxWindowCooldown)
	require.NotNil(t, at)
	require.WithinDuration(t, resetAtFixedNow.Add(2*time.Hour+41*time.Minute), *at, time.Second)

	isoBody := []byte(fmt.Sprintf(`{"error":{"message":"You've reached your weekly usage limit for your plan. Your limit resets at %s. Please wait for the window to reset."}}`, resetAtFixedNow.Add(6*time.Hour).Format(time.RFC3339)))
	at2 := commandCodeResetAt(isoBody, resetAtFixedNow, commandCodeMaxWindowCooldown)
	require.NotNil(t, at2)
	require.WithinDuration(t, resetAtFixedNow.Add(6*time.Hour), *at2, time.Second)

	// 超出 maxWait（周窗口上限）的绝对时刻视为异常 → nil（调用方回落快照）
	farBody := []byte(fmt.Sprintf(`{"error":{"message":"resets at %s"}}`, resetAtFixedNow.Add(200*time.Hour).Format(time.RFC3339)))
	require.Nil(t, commandCodeResetAt(farBody, resetAtFixedNow, commandCodeMaxWindowCooldown))
}

func TestHandleUpstreamError_CommandCode400UsageLimitHandledByCommandCodePath(t *testing.T) {
	// command_code 平台 400 窗口限流 → 走 handleCommandCodeUsageError 停调到文案时间
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newResetAtTestAccount(PlatformCommandCode, 1034)
	body := fmt.Sprintf(`{"error":{"message":"You've reached your weekly usage limit for your plan. Your limit resets at %s. Please wait for the window to reset or upgrade your plan to continue.","type":"rate_limit_error"}}`, futureRFC3339(26*time.Hour))

	shouldDisable := runUpstream(service, account, http.StatusBadRequest, body)

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.tempCalls)
	require.Equal(t, 0, repo.setErrorCalls)
	require.WithinDuration(t, time.Now().Add(26*time.Hour).UTC(), repo.lastTempUntil.UTC(), time.Minute)
	require.Contains(t, repo.lastTempReason, commandCodeUsageLimitReason)
}

func TestHandleUpstreamError_CommandCode400PlainParamUntouched(t *testing.T) {
	// 反向守护：command_code 普通 400 参数错零处理
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newResetAtTestAccount(PlatformCommandCode, 1035)

	_ = runUpstream(service, account, http.StatusBadRequest, `{"error":{"message":"Invalid option: expected one of \"off\"|\"low\"","type":"invalid_request_error"}}`)

	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 0, repo.tempCalls)
	require.Equal(t, 0, repo.rateLimitedCalls)
}
