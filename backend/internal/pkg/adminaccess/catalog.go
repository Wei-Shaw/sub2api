package adminaccess

import "slices"

type Permission struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Category string `json:"category"`
	Scope    string `json:"scope"`
}

type permissionDefinition struct {
	Permission
	Routes []string
}

// 接口按方法和 Gin 完整路径精确登记；新增接口不会继承同目录权限。
// 全站权限与账户规则独立，选中分组绝不会隐式授予全站操作。
var definitions = []permissionDefinition{
	{Permission{"accounts.read", "查看账户（脱敏）", "账户", "account"}, []string{"GET /accounts", "GET /accounts/:id", "GET /accounts/:id/models"}},
	{Permission{"accounts.create", "创建账户", "账户", "account"}, []string{"POST /accounts", "POST /accounts/import/codex-session"}},
	{Permission{"accounts.update", "编辑名称与备注", "账户", "account"}, []string{"PUT /accounts/:id"}},
	{Permission{"accounts.credentials", "写入凭证（不包含读取）", "账户", "account"}, []string{"PUT /accounts/:id"}},
	{Permission{"accounts.models.write", "编辑账户白名单模型 ID 与映射", "账户", "account"}, []string{"PUT /accounts/:id", "POST /accounts/bulk-update"}},
	{Permission{"accounts.network", "修改接入地址、代理与连接选项", "账户", "account"}, []string{"PUT /accounts/:id"}},
	{Permission{"accounts.status", "启用、停用与调度开关", "账户", "account"}, []string{"PUT /accounts/:id", "POST /accounts/:id/schedulable"}},
	{Permission{"accounts.schedule", "修改并发、优先级与有效期", "账户", "account"}, []string{"PUT /accounts/:id"}},
	{Permission{"accounts.refresh", "刷新 OAuth 凭证", "账户", "account"}, []string{"POST /accounts/:id/refresh"}},
	{Permission{"accounts.recover", "清除账户错误与限流状态", "账户", "account"}, []string{"POST /accounts/:id/clear-error"}},
	{Permission{"accounts.delete", "删除账户（影响共享分组）", "账户", "account"}, []string{"DELETE /accounts/:id"}},
	{Permission{"accounts.groups", "调整授权分组关联", "账户", "account"}, []string{"PUT /accounts/:id"}},
	{Permission{"groups.models.read", "读取所选分组模型白名单与候选 ID", "分组模型", "group"}, []string{"GET /groups", "GET /groups/all", "GET /groups/:id", "GET /groups/:id/model-allowlist-candidates"}},
	{Permission{"groups.models.write", "编辑所选分组模型白名单（影响整个分组）", "分组模型", "group"}, []string{"PUT /groups/:id"}},
	{Permission{"proxies.read", "读取代理 IP 候选（分页，不含认证凭证）", "代理", "global"}, []string{"GET /proxies", "GET /proxies/all", "GET /proxies/:id"}},
	{Permission{"models.defaults.read", "读取默认白名单模型 ID（含默认映射）", "模型", "global"}, []string{"GET /accounts/antigravity/default-model-mapping", "GET /groups/:id/model-allowlist-candidates"}},
	{Permission{"dashboard.read", "读取全站经营报表", "报表", "global"}, []string{"GET /dashboard/snapshot-v2", "GET /dashboard/stats", "GET /dashboard/realtime", "GET /dashboard/trend", "GET /dashboard/models", "GET /dashboard/groups", "GET /dashboard/api-keys-trend", "GET /dashboard/users-trend", "GET /dashboard/users-ranking", "POST /dashboard/users-usage", "POST /dashboard/api-keys-usage", "GET /dashboard/user-breakdown"}},
	{Permission{"users.read", "读取全站用户资料（含联系方式）", "用户", "global"}, []string{"GET /users", "GET /users/:id", "GET /users/:id/rpm-status", "GET /users/:id/attributes"}},
	{Permission{"users.balance.read", "读取全站用户余额及流水", "财务", "global"}, []string{"GET /users/:id/balance-history"}},
	{Permission{"users.balance.write", "增减任意用户余额", "财务", "global"}, []string{"POST /users/:id/balance"}},
	{Permission{"users.quotas.read", "读取全站用户平台额度", "用户", "global"}, []string{"GET /users/:id/platform-quotas"}},
	{Permission{"users.quotas.write", "修改与重置全站用户平台额度", "用户", "global"}, []string{"PUT /users/:id/platform-quotas", "POST /users/:id/platform-quotas/reset"}},
	{Permission{"groups.read", "读取所有分组配置", "分组", "global"}, []string{"GET /groups", "GET /groups/all", "GET /groups/:id", "GET /groups/:id/rate-multipliers", "GET /groups/:id/composite-routes", "GET /groups/:id/model-allowlist-candidates"}},
	{Permission{"usage.read", "读取全站用量明细及用户用量", "报表", "global"}, []string{"GET /usage", "GET /usage/stats", "GET /users/:id/usage"}},
	{Permission{"payment.read", "读取全站支付订单与报表", "财务", "global"}, []string{"GET /payment/dashboard", "GET /payment/orders", "GET /payment/orders/:id"}},
	{Permission{"payment.cancel", "取消支付订单", "财务", "global"}, []string{"POST /payment/orders/:id/cancel"}},
	{Permission{"payment.fulfill", "重新履约支付订单", "财务", "global"}, []string{"POST /payment/orders/:id/retry"}},
	{Permission{"payment.refund", "发起退款及确认退款结果", "财务", "global"}, []string{"POST /payment/orders/:id/refund", "POST /payment/orders/:id/refund/query"}},
	{Permission{"payment.plans.read", "读取全部支付套餐", "财务", "global"}, []string{"GET /payment/plans"}},
	{Permission{"payment.plans.write", "创建、修改和删除支付套餐", "财务", "global"}, []string{"POST /payment/plans", "PUT /payment/plans/:id", "DELETE /payment/plans/:id"}},
	{Permission{"subscriptions.read", "读取全站订阅记录", "订阅", "global"}, []string{"GET /subscriptions", "GET /subscriptions/:id", "GET /subscriptions/:id/progress", "GET /groups/:id/subscriptions", "GET /users/:id/subscriptions"}},
	{Permission{"subscriptions.assign", "分配全站用户订阅", "订阅", "global"}, []string{"POST /subscriptions/assign", "POST /subscriptions/bulk-assign"}},
	{Permission{"subscriptions.extend", "延长订阅有效期", "订阅", "global"}, []string{"POST /subscriptions/:id/extend"}},
	{Permission{"subscriptions.revoke", "撤销订阅", "订阅", "global"}, []string{"POST /subscriptions/:id/revoke", "DELETE /subscriptions/:id"}},
	{Permission{"subscriptions.restore", "恢复订阅", "订阅", "global"}, []string{"POST /subscriptions/:id/restore"}},
	{Permission{"subscriptions.reset", "重置订阅额度", "订阅", "global"}, []string{"POST /subscriptions/:id/reset-quota"}},
	{Permission{"announcements.read", "读取所有公告与阅读记录", "内容", "global"}, []string{"GET /announcements", "GET /announcements/:id", "GET /announcements/:id/read-status"}},
	{Permission{"announcements.write", "创建、发布与修改公告", "内容", "global"}, []string{"POST /announcements", "PUT /announcements/:id"}},
	{Permission{"announcements.delete", "删除公告", "内容", "global"}, []string{"DELETE /announcements/:id"}},
	{Permission{"promo.read", "读取优惠码及使用记录", "财务", "global"}, []string{"GET /promo-codes", "GET /promo-codes/:id", "GET /promo-codes/:id/usages"}},
	{Permission{"promo.write", "创建与修改优惠码权益", "财务", "global"}, []string{"POST /promo-codes", "PUT /promo-codes/:id"}},
	{Permission{"promo.delete", "删除优惠码", "财务", "global"}, []string{"DELETE /promo-codes/:id"}},
	{Permission{"ops.read", "读取全站运维指标", "监控", "global"}, []string{"GET /ops/concurrency", "GET /ops/user-concurrency", "GET /ops/account-availability", "GET /ops/realtime-traffic", "GET /ops/ingress-rejections", "GET /ops/ingress-rejections/health", "GET /ops/auth-cache-invalidation/health", "GET /ops/dashboard/snapshot-v2", "GET /ops/dashboard/overview", "GET /ops/dashboard/throughput-trend", "GET /ops/dashboard/latency-histogram", "GET /ops/dashboard/error-trend", "GET /ops/dashboard/error-distribution", "GET /ops/dashboard/openai-token-stats"}},
	{Permission{"ops.errors.read", "读取全站请求错误详情（敏感）", "监控", "global"}, []string{"GET /ops/errors", "GET /ops/errors/:id", "GET /ops/request-errors", "GET /ops/request-errors/:id", "GET /ops/request-errors/:id/upstream-errors", "GET /ops/upstream-errors", "GET /ops/upstream-errors/:id", "GET /ops/requests"}},
	{Permission{"ops.errors.resolve", "标记请求错误处理状态", "监控", "global"}, []string{"PUT /ops/errors/:id/resolve", "PUT /ops/request-errors/:id/resolve", "PUT /ops/upstream-errors/:id/resolve"}},
	{Permission{"ops.alerts.read", "读取全站告警规则与事件", "监控", "global"}, []string{"GET /ops/alert-rules", "GET /ops/alert-events", "GET /ops/alert-events/:id"}},
	{Permission{"ops.alerts.write", "修改告警规则、状态与静默", "监控", "global"}, []string{"POST /ops/alert-rules", "PUT /ops/alert-rules/:id", "DELETE /ops/alert-rules/:id", "PUT /ops/alert-events/:id/status", "POST /ops/alert-silences"}},
	{Permission{"system.read", "读取系统版本与可更新版本", "系统", "global"}, []string{"GET /system/version", "GET /system/check-updates", "GET /system/rollback-versions"}},
}

func PermissionCatalog() []Permission {
	result := make([]Permission, 0, len(definitions))
	for _, definition := range definitions {
		result = append(result, definition.Permission)
	}
	return result
}

// RouteAllowed 只检查动作准入；账户资源及字段必须再由专用处理器检查。
func RouteAllowed(policy Policy, method, path string) bool {
	target := method + " " + path
	for _, definition := range definitions {
		matched := false
		for _, route := range definition.Routes {
			// route 的方法前缀与资源路径固定，不做前缀通配或按 HTTP 方法推断读写。
			for i, ch := range route {
				if ch == ' ' {
					matched = target == route[:i]+" /api/v1/admin"+route[i+1:]
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			continue
		}
		if definition.Scope == "global" && slices.Contains(policy.GlobalPermissions, definition.ID) {
			return true
		}
		if (definition.Scope == "account" || definition.Scope == "group") && len(policy.RulesFor(definition.ID)) > 0 {
			return true
		}
	}
	return false
}
