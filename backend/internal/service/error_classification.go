package service

import "strings"

// quotaExhaustedKeywords 用于识别上游"账号配额/积分/余额耗尽"类错误。
// 命中任一关键词即视为账号级配额耗尽，应触发 failover 切换到其他账号。
// 注意：这些关键词覆盖了多种上游（Anthropic 官方、第三方中转、OpenAI 等）的错误文案。
var quotaExhaustedKeywords = []string{
	// 通用 credit/quota 耗尽
	"credit has been exceeded",
	"credit balance",
	"credits exhausted",
	"credit exhausted",
	"insufficient credit",
	"insufficient credits",
	"not enough credit",
	"not enough credits",
	"out of credit",
	"out of credits",
	// quota 类
	"quota has been exceeded",
	"quota exhausted",
	"quota_exhausted",
	"quota exceeded",
	// balance 类
	"insufficient balance",
	"insufficient_balance",
	"balance is too low",
	// max credit 类（第三方中转常见格式）
	"max credit",
	// 通用 payment/billing
	"payment required",
	"billing issue",
	// 容量/资源耗尽（Google/Vertex 风格）
	"resource has been exhausted",
	"exhausted your capacity",
}

// IsQuotaExhaustedMessage 判断上游错误消息是否表示账号配额/积分/余额耗尽。
// msg 应为小写化后的错误消息文本。
func IsQuotaExhaustedMessage(msg string) bool {
	if msg == "" {
		return false
	}
	lower := strings.ToLower(msg)
	for _, keyword := range quotaExhaustedKeywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}
