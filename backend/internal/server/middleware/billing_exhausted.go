package middleware

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const apiKeyQuotaExhaustedMessage = "API key 额度已用完，请停止自动重试；请提高该 Key 的额度上限或重置额度后再试，充值用户余额不会自动提高 Key 限额。"

// markBillingExhausted 将确定性的额度拒绝纳入入口统计和现有访问日志限频，
// 避免每次重试写入 Ops 错误表。不缓存拒绝状态，额度恢复后按最新鉴权结果放行。
func markBillingExhausted(c *gin.Context, reason IngressRejectReason) {
	MarkIngressRejected(c, reason)
	c.Header("Retry-After", "60")
	c.Header("x-should-retry", "false")
}

func billingBalanceExhaustedMessage() string {
	return service.InsufficientBalanceClientMessage + " 请停止自动重试，额度恢复后再发起请求。"
}
