package service

import (
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

const (
	openCodeCapacityRetryMax      = 3
	openCodeCapacityRetryMaxDelay = 30 * time.Second
)

var openCodeCapacityRetryHint = regexp.MustCompile(`(?i)\bretry shortly \(after ([0-9]+(?:\.[0-9]+)?) s\)`)

func isOpenCodeCapacityRefused(account *Account, statusCode int, body []byte) bool {
	if account == nil || !account.IsOpenCodeGo() || statusCode != http.StatusTooManyRequests {
		return false
	}
	// 只接受上游错误字段中的容量拒绝信号；配额耗尽、参数错误和回显的请求内容不参与重试分类。
	code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "error.code").String()))
	if code != "" {
		return code == "ttft_admission_refused"
	}
	message := strings.ToLower(gjson.GetBytes(body, "error.message").String())
	return strings.Contains(message, "[ttft_admission_refused]")
}

func openCodeCapacityRetryDelay(headers http.Header, body []byte) (time.Duration, bool) {
	// 同时存在响应头和消息内等待提示时取较长值，避免在上游允许重试前再次请求。
	delay := 500 * time.Millisecond
	type retryValue struct {
		raw  string
		unit time.Duration
	}
	values := []retryValue{
		{strings.TrimSpace(headers.Get("Retry-After")), time.Second},
		{strings.TrimSpace(headers.Get("Retry-After-Ms")), time.Millisecond},
	}
	if match := openCodeCapacityRetryHint.FindStringSubmatch(gjson.GetBytes(body, "error.message").String()); len(match) > 1 {
		values = append(values, retryValue{match[1], time.Second})
	}
	for _, value := range values {
		if value.raw == "" {
			continue
		}
		var candidate time.Duration
		number, parseErr := strconv.ParseFloat(value.raw, 64)
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return 0, false
		}
		if parseErr == nil {
			// 转为 Duration 前校验数值，避免溢出把超长等待变成即时重试。
			if number < 0 || number > float64(openCodeCapacityRetryMaxDelay)/float64(value.unit) {
				return 0, false
			}
			candidate = time.Duration(number * float64(value.unit))
		} else if value.unit == time.Second {
			if resetAt, err := http.ParseTime(value.raw); err == nil {
				candidate = time.Until(resetAt)
			}
		}
		// 超过单次等待上限时交还切号流程，不缩短上游要求的等待时间。
		if candidate > openCodeCapacityRetryMaxDelay {
			return 0, false
		}
		if candidate > delay {
			delay = candidate
		}
	}
	return delay, true
}
