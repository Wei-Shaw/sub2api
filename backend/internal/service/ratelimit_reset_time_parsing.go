package service

import (
	"regexp"
	"strings"
	"time"
)

// 上游文案自带的绝对恢复时刻解析。
//
// 部分上游在限流/配额耗尽的报文里直接给出绝对恢复时刻，而不只是相对时长：
//
//   - "Your limit resets at 2026-10-17T09:17:26.406Z"（RFC3339 UTC，带毫秒）
//   - "Coding Plan quota resets at 2026-10-16T08:03:20Z"（RFC3339 UTC，秒）
//
// 这类报文此前无法被解析：commandCodeResetAt 只认 "resets in 3 days" 这类相对
// 时长，解析不出时只能回落到快照选窗或秒级兜底，账号往往被停调得比上游声明的
// 恢复点更久（甚至被永久停用）。
//
// 时区纪律：带 Z / ±hh:mm 标记的串按 RFC3339 直接解析，零时差；无标记的 naive
// 串按 UTC 假设（宁晚不早，避免提前放行），并返回 assumedUTC=true 供调用方
// 打日志，上游出现该形态后可据实改判。

// maxResetAtHorizon 是文案恢复点距当前时刻的上限（周/月窗口最长 32 天），
// 超界视为脏数据忽略，防止账号被异常时间长期睡死。
const maxResetAtHorizon = 35 * 24 * time.Hour

// 时间部分限定 YYYY-MM-DD 开头（日期与时间之间 T 或空格），避免误匹配
// "Resets in 2h 41m (3:00 PM)" 这类旧格式里的钟点写法。
var (
	resetAtWithZonePattern = regexp.MustCompile(`(?i)resets?\s+at\s+(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2}))`)
	resetAtNaivePattern    = regexp.MustCompile(`(?i)resets?\s+at\s+(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?)`)
)

// parseResetAtTimestampInMessage 从文案提取 "resets at <RFC3339>" 的绝对恢复时刻。
// 解析不出、已过期（≤now）或超过 maxResetAtHorizon 时返回 nil。
// 第二个返回值表示是否按 UTC 假设解析了无时区标记的 naive 串。
func parseResetAtTimestampInMessage(message string, now time.Time) (*time.Time, bool) {
	if m := resetAtWithZonePattern.FindStringSubmatch(message); m != nil {
		if t, err := time.Parse(time.RFC3339, normalizeResetAtDateTime(m[1])); err == nil {
			return boundResetAt(t, now), false
		}
		return nil, false
	}
	if m := resetAtNaivePattern.FindStringSubmatch(message); m != nil {
		// 无时区标记：按 UTC 解析（time.Parse 对无 zone 布局返回 UTC 语义的绝对时刻）。
		if t, err := time.Parse("2006-01-02T15:04:05.999999999", normalizeResetAtDateTime(m[1])); err == nil {
			return boundResetAt(t, now), true
		}
	}
	return nil, false
}

// parseBodyResetAt 从上游响应体提取恢复点：先取 error.message 文案，取不到再扫整包 body。
func parseBodyResetAt(body []byte, now time.Time) (*time.Time, bool) {
	if t, assumed := parseResetAtTimestampInMessage(extractUpstreamErrorMessage(body), now); t != nil {
		return t, assumed
	}
	return parseResetAtTimestampInMessage(string(body), now)
}

func normalizeResetAtDateTime(raw string) string {
	return strings.Replace(raw, " ", "T", 1)
}

// usageWindowLimitText 报告响应体是否为用量窗口类文案（与 commandcode 的
// classifyCommandCodeUsageError 窗口判断保持同一口径），供 400 状态码接入点
// 区分「窗口限流被打成 400」与普通参数错误。
func usageWindowLimitText(body []byte) bool {
	text := strings.ToLower(string(body))
	return strings.Contains(text, "usage limit") || strings.Contains(text, "usage exceeded")
}

func boundResetAt(t time.Time, now time.Time) *time.Time {
	if !t.After(now) || t.After(now.Add(maxResetAtHorizon)) {
		return nil
	}
	return &t
}
