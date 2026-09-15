// ratelimit.go 移植 devin2api 的限流文案解析：上游不给 Retry-After
// 头或 RetryInfo detail，错误消息里的 "reset in N seconds/minutes"
// 是唯一可行动的 hint。
package devin

import (
	"regexp"
	"strconv"
	"strings"
)

// rateLimitResetPattern 匹配上游限流文案里的重试窗口。实测两种单位：
// 剩余不足一分钟时报 "reset in N seconds"，更长时报 "reset in N
// minute(s)"（floor 取整）。
var rateLimitResetPattern = regexp.MustCompile(`(?i)reset in (\d+)\s*(seconds?|minutes?)`)

// RetryAfterSeconds 从上游错误文案解析限流重置秒数；无 hint 返回 false。
// 返回的是上游声明的字面秒数（分钟按 60 折算）。
func RetryAfterSeconds(message string) (int, bool) {
	match := rateLimitResetPattern.FindStringSubmatch(message)
	if len(match) != 3 {
		return 0, false
	}
	seconds, err := strconv.Atoi(match[1])
	if err != nil || seconds <= 0 {
		return 0, false
	}
	if strings.HasPrefix(match[2], "minute") {
		seconds *= 60
	}
	return seconds, true
}
