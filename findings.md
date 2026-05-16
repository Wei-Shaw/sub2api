# Findings: Backend Billing Logic

## Requirements
- 用户要求：整理“后端完整的计费逻辑”，必须准确。
- 产物：一份文档，并在文档中画一个时序图。

## Research Findings
- 请求热路径先做计费资格检查：`BillingCacheService.CheckBillingEligibility` 在非 simple 模式下检查余额模式或订阅模式，再检查 API Key 5h/1d/7d 费用窗口和 RPM。
- 余额模式只要求用户余额缓存/DB 中 `balance > 0`；订阅模式要求订阅 active、未过期、且分组日/周/月美元用量未达上限。这里是准入检查，不预扣。
- 真实扣费发生在上游响应结束并解析 usage 后。Claude/Gemini 走 `GatewayService.recordUsageCore`，OpenAI 走 `OpenAIGatewayService.RecordUsage`，两者都会计算费用、构造 `UsageLog` 和 `UsageBillingCommand`。
- `BillingService.CalculateCostUnified` 支持 token、per_request、image 三种模式。定价解析链是 Channel 配置优先，其次 LiteLLM/本地资源价格，最后硬编码 fallback。token 模式按输入、输出、cache creation、cache read、image output 分项计算。
- `actual_cost = total_cost * rate_multiplier`。rate multiplier 优先级：系统默认 → 分组默认 → 用户-分组专属倍率。图片可以使用独立倍率。
- OpenAI 路径会从 `input_tokens` 中扣掉 `cache_read_input_tokens`，避免缓存读取 token 同时按输入 token 计费；Claude/Gemini 按各自 usage 字段构造 token。
- token 计费还处理 priority/flex service tier、模型长上下文倍率、缓存 5m/1h 明细、渠道区间定价；image/per_request 支持 size tier 或上下文区间匹配。
- 扣费事务由 `UsageBillingRepository.Apply` 完成：先用 `(request_id, api_key_id)` 插入 `usage_billing_dedup`，冲突时比较 request fingerprint，不同 fingerprint 返回冲突，相同则视为已处理。
- 扣费事务效果包括：订阅用量累加 `daily/weekly/monthly_usage_usd`、余额扣减 `users.balance`、API Key quota_used 与状态、API Key 5h/1d/7d usage、账号 quota JSON 计数和调度 outbox。
- 订阅扣的是 `ActualCost`；余额扣的是 `ActualCost`；API Key quota/rate-limit 用 `ActualCost`；账号额度统计用 `TotalCost * account_rate_multiplier`。
- 事务成功后才异步更新 Redis 计费缓存、触发余额低告警和账号额度告警、更新账号 last_used；usage log 是 best-effort 写入，不作为扣费依据。
- simple 模式只记录 usage log，不扣费、不做计费准入限制。
- 旧的 `UsageService.Create` 仍有“写日志并扣余额”的事务逻辑，但网关生产扣费路径使用 `applyUsageBilling` 与 `UsageBillingRepository`，不是依赖 `UsageService.Create`。
- 支付订单创建先读取支付配置，校验启用、金额/套餐、待支付订单数、日限额、用户状态，再生成 `payment_orders`。余额订单的 `amount` 是入账余额 `支付输入金额 * balance_recharge_multiplier`，`pay_amount` 是实际支付金额加手续费。
- 支付成功 webhook 校验 provider、metadata、金额后，把订单从 PENDING/CANCELLED/短宽限 EXPIRED 更新为 PAID，然后进入履约；重复回调会按当前状态跳过或重试失败订单。
- 余额订单履约把订单置 RECHARGING，按订单 `recharge_code` 创建 `RedeemTypeBalance` 兑换码并兑换，兑换会在同一 DB 事务中标记兑换码已用并更新用户余额，成功后订单置 COMPLETED。
- 订阅订单履约校验订阅分组有效，把订单置 RECHARGING，通过 `AssignOrExtendSubscription` 创建或续期用户订阅，再置 COMPLETED；用 `SUBSCRIPTION_SUCCESS` 审计日志防止重试重复续期。
- 兑换码可直接给用户余额、并发、订阅权益；余额/订阅兑换后会失效认证缓存和计费缓存。负数余额兑换码用于扣减但余额最低扣到 0；负数订阅天数会缩短或取消订阅。
- 后台余额调整 `UpdateUserBalance` 支持 set/add/subtract，禁止结果为负，更新后失效缓存，并写一条已使用的 `admin_balance` 兑换码作为历史记录。
- 退款只对符合条件的订单执行。用户自助退款只允许已完成余额订单且 provider instance 允许；管理员退款可选择扣余额或扣订阅天数，再调支付通道退款。网关退款失败会尝试回滚扣减，回滚失败写 `REFUND_ROLLBACK_FAILED`。
- 订单过期任务周期性扫描 PENDING 且超时的订单；过期前先主动查上游是否已支付，已支付则走支付通知履约，否则取消上游订单并标记 EXPIRED。

## Source References
- `backend/internal/service/billing_cache_service.go`
- `backend/internal/service/billing_service.go`
- `backend/internal/service/model_pricing_resolver.go`
- `backend/internal/service/gateway_service.go`
- `backend/internal/service/openai_gateway_service.go`
- `backend/internal/service/usage_billing.go`
- `backend/internal/repository/usage_billing_repo.go`
- `backend/internal/service/payment_order.go`
- `backend/internal/service/payment_fulfillment.go`
- `backend/internal/service/payment_order_lifecycle.go`
- `backend/internal/service/payment_order_expiry_service.go`
- `backend/internal/service/payment_refund.go`
- `backend/internal/service/redeem_service.go`
- `backend/internal/service/subscription_service.go`
- `backend/internal/service/admin_service.go`
- `backend/ent/schema/usage_log.go`
- `backend/ent/schema/payment_order.go`
- `backend/ent/schema/user_subscription.go`
- `backend/ent/schema/user.go`
- `backend/ent/schema/group.go`
- `backend/migrations/071_add_usage_billing_dedup.sql`
- `backend/migrations/073_add_usage_billing_dedup_archive.sql`

## Issues Encountered
| Issue | Resolution |
|-------|------------|
| Existing planning files belonged to an older documentation task | Replaced them with current task-specific planning files. |
