//go:build unit

package service

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func openCodeCapacityFixture(seconds string) []byte {
	return []byte(fmt.Sprintf(`{"error":{"type":"server_error","message":"Upstream request failed: [ttft_admission_refused] the cell is at capacity; retry shortly (after %s s)"}}`, seconds))
}

func TestOpenCodeCapacityRetryClassification(t *testing.T) {
	account := &Account{ID: 31, Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey,
		Extra: map[string]any{cnExtraKey(PlatformOpenCodeGo, cnExtraSuffix5hReset): time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}}
	for _, seconds := range []int{2, 9, 15} {
		t.Run(fmt.Sprintf("observed_%ds", seconds), func(t *testing.T) {
			body := openCodeCapacityFixture(fmt.Sprint(seconds))
			svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{}}
			// rateLimitService 没有仓储；若容量拒绝误入配额冷却或临时停调，此处会失败。
			require.False(t, svc.handleOpenAIAccountUpstreamError(context.Background(), account, 429, nil, body))
			err := svc.newOpenAIAccountFailoverError(account, 429, nil, body, "", false, false)
			require.True(t, err.RetryableOnSameAccount)
			require.True(t, err.RequestScopedTransient)
			require.Equal(t, time.Duration(seconds)*time.Second, err.SameAccountRetryDelay)
			require.Equal(t, 3, err.SameAccountRetryMax)
			require.Equal(t, 429, err.StatusCode)
			require.Equal(t, 429, err.ClientStatusCode)
		})
	}
	for _, tt := range []struct {
		name, body string
		delay      time.Duration
	}{
		{"prefill_message", `{"error":{"type":"server_error","message":"Upstream request failed: [prefill_cap_refused] the cell is at capacity; retry shortly (after 5 s)"}}`, 5 * time.Second},
		{"overloaded_message_without_hint", `{"error":{"type":"server_error","message":"Upstream request failed: [overloaded] the server is overloaded; retry shortly"}}`, 500 * time.Millisecond},
		{"prefill_code", `{"error":{"code":"prefill_cap_refused","message":"retry shortly (after 5 s)"}}`, 5 * time.Second},
		{"overloaded_code", `{"error":{"code":"overloaded"}}`, 500 * time.Millisecond},
		{"normalized_code", `{"error":{"code":" OVERLOADED "}}`, 500 * time.Millisecond},
		{"normalized_message", `{"error":{"message":"Upstream request failed: [PREFILL_CAP_REFUSED]; retry shortly (after 5 s)"}}`, 5 * time.Second},
		{"inference_overloaded", `{"error":{"type":"server_error","message":"The inference service is overloaded. Please try again later."}}`, 500 * time.Millisecond},
		{"normalized_inference_overloaded", `{"error":{"type":" SERVER_ERROR ","message":" THE INFERENCE SERVICE IS OVERLOADED. PLEASE TRY AGAIN LATER. "}}`, 500 * time.Millisecond},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{}}
			require.False(t, svc.handleOpenAIAccountUpstreamError(context.Background(), account, 429, nil, body))
			err := svc.newOpenAIAccountFailoverError(account, 429, nil, body, "", false, false)
			require.True(t, err.RetryableOnSameAccount)
			require.True(t, err.RequestScopedTransient)
			require.Equal(t, tt.delay, err.SameAccountRetryDelay)
			require.Equal(t, 3, err.SameAccountRetryMax)
			require.Equal(t, 429, err.StatusCode)
			require.Equal(t, 429, err.ClientStatusCode)
		})
	}
	for _, tt := range []struct {
		name string
		code int
		body string
	}{
		{"quota", 429, `{"error":{"code":"usage_limit_reached","message":"quota exhausted"}}`},
		{"token_limit", 400, `{"error":{"type":"server_error","message":"Param Incorrect","param":"max_tokens is too large"}}`},
		{"developer_role", 422, `{"error":{"message":"unknown variant developer"}}`},
		{"empty_400", 400, `{"error":{"type":"server_error","message":""}}`},
		{"request_echo", 429, `{"error":{"message":"quota exhausted"},"request":{"message":"[ttft_admission_refused]"}}`},
		{"different_code", 429, `{"error":{"code":"usage_limit_reached","message":"[ttft_admission_refused]"}}`},
		{"prefill_quota_code", 429, `{"error":{"code":"usage_limit_reached","message":"[prefill_cap_refused]"}}`},
		{"overloaded_quota_code", 429, `{"error":{"code":"usage_limit_reached","message":"[overloaded]"}}`},
		{"unknown_code", 429, `{"error":{"code":"unknown_error","message":"[overloaded]"}}`},
		{"generic_rate_limit", 429, `{"error":{"type":"rate_limit_error","message":"rate limit exceeded, please retry later"}}`},
		{"marker_in_type", 429, `{"error":{"type":"overloaded","message":"retry shortly"}}`},
		{"new_marker_request_echo", 429, `{"error":{"message":"quota exhausted"},"request":{"message":"[prefill_cap_refused] [overloaded]"}}`},
		{"new_marker_wrong_status", 400, `{"error":{"code":"overloaded","message":"[prefill_cap_refused]"}}`},
		{"inference_overloaded_quota_code", 429, `{"error":{"code":"usage_limit_reached","type":"server_error","message":"The inference service is overloaded. Please try again later."}}`},
		{"inference_overloaded_unknown_code", 429, `{"error":{"code":"unknown_error","type":"server_error","message":"The inference service is overloaded. Please try again later."}}`},
		{"inference_overloaded_wrong_type", 429, `{"error":{"type":"rate_limit_error","message":"The inference service is overloaded. Please try again later."}}`},
		{"inference_overloaded_missing_type", 429, `{"error":{"message":"The inference service is overloaded. Please try again later."}}`},
		{"inference_overloaded_suffix", 429, `{"error":{"type":"server_error","message":"The inference service is overloaded. Please try again later. quota exhausted"}}`},
		{"inference_overloaded_prefix", 429, `{"error":{"type":"server_error","message":"Echo: The inference service is overloaded. Please try again later."}}`},
		{"inference_overloaded_echo", 429, `{"error":{"type":"server_error","message":"quota exhausted"},"request":{"message":"The inference service is overloaded. Please try again later."}}`},
		{"inference_overloaded_wrong_status", 503, `{"error":{"type":"server_error","message":"The inference service is overloaded. Please try again later."}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.False(t, isOpenCodeCapacityRefused(account, tt.code, []byte(tt.body)))
			err := (&OpenAIGatewayService{}).newOpenAIAccountFailoverError(account, tt.code, nil, []byte(tt.body), "", false, false)
			require.False(t, err.RetryableOnSameAccount)
			require.Zero(t, err.SameAccountRetryMax)
		})
	}
	require.False(t, isOpenCodeCapacityRefused(&Account{Platform: PlatformOpenAI}, 429, openCodeCapacityFixture("2")))
	require.True(t, isOpenCodeCapacityRefused(account, 429, []byte(`{"error":{"code":"ttft_admission_refused"}}`)))
	// 既有共享容量策略可单独识别该文案；本次 OpenCode 分类器不扩大到任意过载文本。
	require.False(t, isOpenCodeCapacityRefused(account, 429, []byte(`{"error":{"type":"server_error","message":"the server is overloaded; retry shortly"}}`)))
	require.False(t, isOpenCodeCapacityRefused(nil, 429, []byte(`{"error":{"code":"overloaded"}}`)))
	require.False(t, isOpenCodeCapacityRefused(&Account{Platform: PlatformOpenAI}, 429, []byte(`{"error":{"message":"[prefill_cap_refused] [overloaded]"}}`)))
}

func TestOpenCodeCapacityRetryDelayBounds(t *testing.T) {
	for _, tt := range []struct {
		name    string
		headers http.Header
		seconds string
		want    time.Duration
		allowed bool
	}{
		{"body_15", nil, "15", 15 * time.Second, true},
		{"header_longer", http.Header{"Retry-After": {"20"}}, "2", 20 * time.Second, true},
		{"body_longer", http.Header{"Retry-After": {"2"}}, "15", 15 * time.Second, true},
		{"milliseconds", http.Header{"Retry-After-Ms": {"2500"}}, "2", 2500 * time.Millisecond, true},
		{"fallback", nil, "invalid", 500 * time.Millisecond, true},
		{"body_too_long", nil, "31", 0, false},
		{"header_too_long", http.Header{"Retry-After": {"60"}}, "2", 0, false},
		{"overflow", http.Header{"Retry-After": {"1e300"}}, "2", 0, false},
		{"float_overflow", http.Header{"Retry-After": {"1e999"}}, "2", 0, false},
		{"infinite", http.Header{"Retry-After": {"Inf"}}, "2", 0, false},
		{"nan", http.Header{"Retry-After": {"NaN"}}, "2", 0, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			delay, allowed := openCodeCapacityRetryDelay(tt.headers, openCodeCapacityFixture(tt.seconds))
			require.Equal(t, tt.allowed, allowed)
			require.Equal(t, tt.want, delay)
		})
	}
	header := http.Header{"Retry-After": {time.Now().Add(10 * time.Second).UTC().Format(http.TimeFormat)}}
	delay, allowed := openCodeCapacityRetryDelay(header, openCodeCapacityFixture("2"))
	require.True(t, allowed)
	require.InDelta(t, 10*time.Second, delay, float64(time.Second))

	// 即使池模式原本允许重试，过长的等待提示也不能退化成默认 500ms 抢跑。
	err := (&OpenAIGatewayService{}).newOpenAIAccountFailoverError(
		&Account{Platform: PlatformOpenCodeGo}, 429, http.Header{"Retry-After": {"60"}}, openCodeCapacityFixture("2"), "", false, true)
	require.False(t, err.RetryableOnSameAccount)
}

func TestOpenCodeCapacityRetryPreservesQuotaCooldown(t *testing.T) {
	repo := &oauth429RateLimitRepo{}
	resetAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	account := &Account{ID: 31, Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey,
		Extra: map[string]any{cnExtraKey(PlatformOpenCodeGo, cnExtraSuffix5hReset): resetAt.Format(time.RFC3339)}}
	svc := &OpenAIGatewayService{rateLimitService: NewRateLimitService(repo, nil, nil, nil, nil)}
	body := []byte(`{"error":{"code":"usage_limit_reached","message":"quota exhausted"}}`)
	svc.handleOpenAIAccountUpstreamError(context.Background(), account, 429, nil, body)
	require.Equal(t, 1, repo.setRateLimitedCalls)
	require.Equal(t, resetAt, repo.lastRateLimitedUntil)
}
