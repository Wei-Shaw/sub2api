package repository

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGitHubAPIResponseErrorClassification(t *testing.T) {
	now := time.Date(2026, 10, 1, 14, 0, 0, 0, time.UTC)
	reset := now.Add(15 * time.Minute)
	tests := []struct {
		name, body, remaining, reset, retry, code string
		status                                    int
		wait                                      time.Duration
		wantReset                                 bool
	}{
		{name: "空响应体仍可识别主限流", status: 403, remaining: "0", reset: strconv.FormatInt(reset.Unix(), 10), code: "github_primary_rate_limit", wait: 15 * time.Minute, wantReset: true},
		{name: "两个期限取更晚者", status: 429, remaining: "0", reset: strconv.FormatInt(reset.Unix(), 10), retry: "1200", code: "github_primary_rate_limit", wait: 20 * time.Minute, wantReset: true},
		{name: "次级限流不使用主额度窗口", status: 403, remaining: "42", body: `{"message":"You have exceeded a secondary rate limit."}`, retry: "90", reset: strconv.FormatInt(reset.Unix(), 10), code: "github_secondary_rate_limit", wait: 90 * time.Second},
		{name: "次级限流缺少期限", status: 403, body: `{"message":"You have exceeded a secondary rate limit."}`, code: "github_secondary_rate_limit"},
		{name: "普通权限错误不重试", status: 403, body: `{"message":"forbidden: private-secret"}`, code: "github_forbidden"},
		{name: "认证错误不是额度问题", status: 401, remaining: "0", reset: strconv.FormatInt(reset.Unix(), 10), code: "github_unauthorized"},
		{name: "429 未提供额外信息", status: 429, code: "github_rate_limit"},
		{name: "非法重置时间不臆测", status: 403, remaining: "0", reset: "not-a-time", code: "github_primary_rate_limit"},
		{name: "超出 JSON 时间范围的数值无效", status: 403, remaining: "0", reset: "9223372036854775807", code: "github_primary_rate_limit"},
		{name: "主限流消息也保留已知期限", status: 403, body: `{"message":"API rate limit exceeded."}`, reset: strconv.FormatInt(reset.Unix(), 10), code: "github_primary_rate_limit", wait: 15 * time.Minute, wantReset: true},
		{name: "负数等待无效", status: 429, retry: "-1", code: "github_secondary_rate_limit"},
		{name: "HTTP 日期形式的等待", status: 429, retry: now.Add(2 * time.Minute).Format(http.TimeFormat), code: "github_secondary_rate_limit", wait: 2 * time.Minute},
		{name: "服务故障", status: 503, code: "github_unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tt.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tt.body))}
			resp.Header.Set("X-RateLimit-Remaining", tt.remaining)
			resp.Header.Set("X-RateLimit-Reset", tt.reset)
			resp.Header.Set("Retry-After", tt.retry)
			err := githubAPIResponseError(resp, now)
			require.Equal(t, tt.code, err.Code)
			require.NotContains(t, err.Error(), "private-secret")
			if tt.wait == 0 {
				require.Nil(t, err.RetryAt)
			} else {
				require.Equal(t, now.Add(tt.wait), *err.RetryAt)
			}
			if tt.wantReset {
				require.Equal(t, reset, *err.RateLimitResetAt)
			} else {
				require.Nil(t, err.RateLimitResetAt)
			}
		})
	}
}

func TestGitHubAPIResponseErrorUsesServerClock(t *testing.T) {
	serverNow := time.Date(2026, 10, 1, 14, 0, 0, 0, time.UTC)
	localNow := serverNow.Add(10 * time.Minute)
	reset := serverNow.Add(2 * time.Minute)
	resp := &http.Response{StatusCode: 403, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}
	resp.Header.Set("Date", serverNow.Format(http.TimeFormat))
	resp.Header.Set("X-RateLimit-Remaining", "0")
	resp.Header.Set("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))
	err := githubAPIResponseError(resp, localNow)
	require.Equal(t, reset, *err.RateLimitResetAt, "展示 GitHub 的原始 UTC 期限")
	require.Equal(t, localNow.Add(2*time.Minute), *err.RetryAt, "本机时钟偏快也必须等待完整窗口")
}

func TestGitHubReleaseEndpointsPreserveSafeRateLimitError(t *testing.T) {
	for _, endpoint := range []string{"latest", "recent"} {
		t.Run(endpoint, func(t *testing.T) {
			client := newTestGitHubReleaseClient()
			client.updateGitHubToken = "private-secret"
			client.httpClient.Transport = githubReleaseRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				header := make(http.Header)
				header.Set("X-RateLimit-Remaining", "0")
				header.Set("X-RateLimit-Reset", "1790867547")
				return &http.Response{StatusCode: 403, Header: header, Body: io.NopCloser(strings.NewReader(`{"message":"private-secret"}`)), Request: req}, nil
			})
			var err error
			if endpoint == "latest" {
				_, err = client.FetchLatestRelease(context.Background(), "openai/codex")
			} else {
				_, err = client.FetchRecentReleases(context.Background(), "openai/codex", 30)
			}
			var apiErr *service.GitHubAPIError
			require.ErrorAs(t, err, &apiErr)
			require.True(t, apiErr.IsRateLimited())
			require.Equal(t, int64(1790867547), apiErr.RateLimitResetAt.Unix())
			require.NotContains(t, err.Error(), "private-secret")
		})
	}
}
