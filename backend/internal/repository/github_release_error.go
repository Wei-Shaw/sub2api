package repository

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// githubAPIResponseError 只读取有限大小的消息用于分类；原文不进入日志或管理接口。
// 参考：https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api
func githubAPIResponseError(resp *http.Response, now time.Time) *service.GitHubAPIError {
	err := &service.GitHubAPIError{StatusCode: resp.StatusCode, Code: "github_http_error"}
	var body struct {
		Message string `json:"message"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 8*1024)).Decode(&body)
	message := strings.ToLower(body.Message)
	limitedStatus := resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests
	remaining := strings.TrimSpace(resp.Header.Get("X-RateLimit-Remaining"))
	secondary := strings.Contains(message, "secondary rate limit") || strings.Contains(message, "abuse detection")
	switch {
	case limitedStatus && remaining == "0":
		err.Code = "github_primary_rate_limit"
	case limitedStatus && secondary:
		err.Code = "github_secondary_rate_limit"
	case limitedStatus && strings.Contains(message, "api rate limit exceeded"):
		err.Code = "github_primary_rate_limit"
	case limitedStatus && resp.Header.Get("Retry-After") != "":
		err.Code = "github_secondary_rate_limit"
	case resp.StatusCode == http.StatusTooManyRequests:
		err.Code = "github_rate_limit"
	case resp.StatusCode == http.StatusUnauthorized:
		err.Code = "github_unauthorized"
	case resp.StatusCode == http.StatusForbidden:
		err.Code = "github_forbidden"
	case resp.StatusCode >= http.StatusInternalServerError:
		err.Code = "github_unavailable"
	}
	if !err.IsRateLimited() {
		return err
	}

	// 使用服务器 Date 计算相对等待，避免本机时钟偏快时提前发起重试。
	serverNow := now
	if date, parseErr := http.ParseTime(resp.Header.Get("Date")); parseErr == nil {
		serverNow = date
	}
	setRetryAfter := func(wait time.Duration) {
		if wait < 0 {
			wait = 0
		}
		at := now.Add(wait).UTC()
		if err.RetryAt == nil || at.After(*err.RetryAt) {
			err.RetryAt = &at
		}
	}
	if remaining == "0" || err.Code == "github_primary_rate_limit" {
		// RFC3339 年份之外的数值无法持久化为 JSON 时间，按缺失期限处理。
		if epoch, parseErr := strconv.ParseInt(strings.TrimSpace(resp.Header.Get("X-RateLimit-Reset")), 10, 64); parseErr == nil && epoch > 0 && epoch <= 253402300799 {
			reset := time.Unix(epoch, 0).UTC()
			err.RateLimitResetAt = &reset
			setRetryAfter(reset.Sub(serverNow))
		}
	}
	if retryAfter := strings.TrimSpace(resp.Header.Get("Retry-After")); retryAfter != "" {
		if seconds, parseErr := strconv.ParseInt(retryAfter, 10, 64); parseErr == nil && seconds >= 0 && seconds <= int64((1<<63-1)/time.Second) {
			setRetryAfter(time.Duration(seconds) * time.Second)
		} else if at, parseErr := http.ParseTime(retryAfter); parseErr == nil {
			setRetryAfter(at.Sub(serverNow))
		}
	}
	return err
}
