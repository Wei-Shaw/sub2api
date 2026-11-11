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

var openCodeCapacityRefusalMarkers = [...]string{
	"ttft_admission_refused",
	"prefill_cap_refused",
	"overloaded",
}

func isOpenCodeCapacityRefused(account *Account, statusCode int, body []byte) bool {
	if account == nil || !account.IsOpenCodeGo() || statusCode != http.StatusTooManyRequests {
		return false
	}
	// 只接受上游错误字段中的容量拒绝信号；配额耗尽、参数错误和回显的请求内容不参与重试分类。
	code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "error.code").String()))
	message := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "error.message").String()))
	// 已确认的另一种 OpenCode 容量响应没有方括号标记；只匹配完整句子及错误类型，避免普通限流被误重试。
	if code == "" && strings.EqualFold(strings.TrimSpace(gjson.GetBytes(body, "error.type").String()), "server_error") &&
		message == "the inference service is overloaded. please try again later." {
		return true
	}
	for _, marker := range openCodeCapacityRefusalMarkers {
		// 明确错误码优先于消息，避免额度错误中回显的容量标记绕过原有冷却。
		if code != "" {
			if code == marker {
				return true
			}
		} else if strings.Contains(message, "["+marker+"]") {
			return true
		}
	}
	return false
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
