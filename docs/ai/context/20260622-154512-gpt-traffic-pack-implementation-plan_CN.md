# GPT 流量包实现计划

> 状态：用户已确认产品规则，进入实现阶段。计划遵循 TDD：先写失败测试，再写最小实现。

## 目标

在用户 `/purchase` 页面新增 GPT 流量包购买能力：2 元/5 USD、3 元/10 USD、5 元/20 USD，365 天有效，可用于 GPT 写代码和 GPT 生图；订阅日额度优先，订阅日额度用完后扣流量包；没有订阅但有未过期流量包也可使用 GPT。

## 架构

- 新增独立 GPT 流量包批次账本，不复用 `users.balance` 和 `user_subscriptions`。
- 支付系统新增 `order_type=traffic_pack`，复用现有创建订单、付款弹窗、订单轮询和支付回调流程。
- 扣费事务扩展第三种成本来源 `TrafficPackCost`，按“最早过期优先，同过期时间最早购买优先”扣多个批次。

## 文件结构

- 新增 `backend/ent/schema/traffic_pack.go`：流量包商品 SKU。
- 新增 `backend/ent/schema/user_traffic_credit.go`：用户购买后的批次权益。
- 新增 `backend/ent/schema/traffic_credit_ledger.go`：入账、扣费、过期、退款等流水。
- 新增 `backend/migrations/151_gpt_traffic_packs.sql`：建表并种子三档 GPT 流量包。
- 新增 `backend/internal/service/traffic_pack.go`：领域常量、DTO、服务接口。
- 新增 `backend/internal/repository/traffic_pack_repo.go`：商品列表、批次入账、可用余额查询、FIFO 扣费。
- 修改 `backend/internal/payment/types.go`：新增 `OrderTypeTrafficPack = "traffic_pack"`。
- 修改 `backend/ent/schema/payment_order.go`：订单增加 `traffic_pack_id` 和 `traffic_credit_usd`。
- 修改 `backend/internal/service/payment_order.go`：支持流量包订单校验和订单快照。
- 修改 `backend/internal/service/payment_fulfillment.go`：支付成功后给用户入账流量包批次。
- 修改 `backend/internal/service/usage_billing.go`、`backend/internal/repository/usage_billing_repo.go`、`backend/internal/service/gateway_service.go`、`backend/internal/service/openai_gateway_service.go`：支持流量包扣费。
- 修改 `backend/internal/service/billing_cache_service.go`：订阅额度耗尽但有 GPT 流量包时放行；无订阅但有流量包时允许 GPT 分组请求。
- 修改 `backend/internal/handler/payment_handler.go`：`checkout-info` 返回流量包商品。
- 修改 `backend/internal/service/wire.go` 和生成文件：注入流量包服务/仓库。
- 修改 `frontend/src/types/payment.ts`：增加 `TrafficPack` 类型、`OrderType` 支持 `traffic_pack`。
- 修改 `frontend/src/views/user/PaymentView.vue`：新增三张流量包卡片和购买确认，复用 `createOrder` 与 `PaymentStatusPanel`。
- 新增/修改测试：后端服务/仓库/支付/扣费测试，前端 PaymentView 测试。

## TDD 任务

### 任务 1：商品与 checkout-info

1. 写失败测试：`backend/internal/handler/payment_handler_test.go` 或现有支付 handler 测试中验证 `/payment/checkout-info` 返回三档 `traffic_packs`，字段含 `id/name/price/credit_usd/validity_days/platform/sort_order`。
2. 写失败测试：前端 `PaymentView.spec.ts` 使用带 `traffic_packs` 的 fixture，断言页面出现 2/3/5 元三张流量包卡片。
3. 实现 migration、schema、repo/service 的只读商品列表，并在 `checkout-info` 返回。
4. 前端类型和页面读取 `checkout.traffic_packs` 渲染卡片。

### 任务 2：流量包订单创建与支付复用

1. 写失败测试：创建 `order_type=traffic_pack` 且 `traffic_pack_id=...` 时，订单 amount 等于商品 price，写入 `traffic_pack_id` 和 `traffic_credit_usd`。
2. 写失败测试：前端点击流量包购买按钮时调用 `createOrder`，payload 为 `order_type=traffic_pack`、`traffic_pack_id`、`amount=price`。
3. 实现后端 `CreateOrderRequest` 增加 `TrafficPackID`，校验商品在售。
4. 实现前端流量包购买确认使用现有支付方法选择和 `PaymentStatusPanel`。

### 任务 3：支付成功后批次入账

1. 写失败测试：`ExecuteTrafficPackFulfillment` 或支付通知完成后，为用户创建 `user_traffic_credits` 批次，`expires_at=credited_at+365天`。
2. 写失败测试：重复执行同一订单不重复入账。
3. 实现 `PaymentService` 对 `traffic_pack` 的 fulfillment 分支。
4. 写入 `traffic_credit_ledger`，类型为 `purchase`，关联订单。

### 任务 4：扣费优先级和 FIFO 扣费

1. 写失败测试：用户有活跃订阅且未达日限额时，只增加订阅用量，不扣流量包。
2. 写失败测试：订阅日限额已满且有未过期流量包时，请求资格检查放行，扣最早过期批次。
3. 写失败测试：无订阅但有未过期 GPT 流量包时，GPT 请求资格检查放行并扣流量包。
4. 写失败测试：多个批次跨批次扣费，先扣最早过期；同过期时间按购买时间。
5. 实现 `UsageBillingCommand.TrafficPackCost` 和 repository 事务内扣费。
6. 使用行级锁或事务顺序更新，避免并发超扣。

### 任务 5：用户展示与余额汇总

1. 写失败测试：checkout-info 或新增用户接口返回 `traffic_credit_summary`：总可用额度、最近到期额度、最近到期时间。
2. 前端在流量包区显示总剩余额度和最近到期信息。
3. 不展示后台分组 ID、本地端口或内部路由细节。

### 任务 6：验证

1. 后端单测：`go test -tags=unit ./internal/service ./internal/handler ./internal/repository`。
2. 前端单测：`pnpm test:run src/views/user/__tests__/PaymentView.spec.ts`。
3. 前端类型检查和构建：`pnpm typecheck`、`pnpm build`。
4. 后端编译：`go test ./...` 可按耗时情况执行；若集成测试依赖外部服务，则记录未执行原因。

## 关键约束

- 不把流量包额度写入 `users.balance`。
- 不创建 365 天订阅来模拟流量包。
- 流量包只适用于 GPT/OpenAI 分组；后续扩平台另开规则。
- 前端购买支付流程必须复用订阅的支付弹窗/等待支付流程。
- 已有未提交改动不回滚、不覆盖。
