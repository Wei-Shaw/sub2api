# GPT 一次性流量包后端接线计划

## 目标

补齐 GPT 一次性流量包的真实业务链路：购买页可展示并下单，支付成功后入账，GPT 写代码和生图在订阅日额度耗尽后自动扣流量包；无订阅但有未过期流量包时也允许 GPT 使用。

## 规则

- 商品三档：2 元/5 USD、3 元/10 USD、5 元/20 USD。
- 每个购买批次独立记账，有效期 365 天。
- 扣费优先级：订阅日额度优先，订阅不可用或额度耗尽后使用流量包。
- 多批次扣费顺序：最早过期优先；同过期时间按最早购买批次优先。
- 仅用于 GPT/OpenAI 平台，覆盖写代码和生图。
- 不复用 `users.balance`，不伪装成 `user_subscriptions`，不修改 Ent schema。

## 文件边界

- `backend/internal/service/traffic_pack.go`：流量包领域类型、排序扣费规划。
- `backend/internal/repository/traffic_pack_repo.go`：SQL 表读写、批次入账、扣费事务。
- `backend/internal/handler/payment_handler.go`：checkout 返回商品和用户流量包摘要；下单接收 `traffic_pack_id`。
- `backend/internal/service/payment_*`：创建流量包订单、商品快照、支付履约入账。
- `backend/internal/service/*gateway*` 与 `usage_billing*`：把流量包作为订阅额度耗尽后的后备扣费来源。
- `frontend/src/views/user/PaymentView.vue`：购买页卡片和复用支付弹窗。

## 测试计划

1. 服务层测试：下单校验识别流量包 SKU，创建订单写入商品快照，支付履约幂等入账。
2. 仓储测试：重复购买批次独立、按最早过期/最早购买扣费。
3. 计费测试：订阅账单仍优先；无订阅或订阅不可用时可以构造流量包扣费命令。
4. 前端测试：三张卡片展示，购买时传 `order_type=traffic_pack` 和 `traffic_pack_id`。
5. 编译/构建：`go test` 目标包、前端 typecheck 和 build。
