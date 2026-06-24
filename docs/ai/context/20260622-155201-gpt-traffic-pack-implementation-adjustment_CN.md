# GPT 流量包实现调整记录

## 背景

用户已确认在 `/purchase` 用户购买页新增 GPT 流量包，三档为 `2 元/5 USD`、`3 元/10 USD`、`5 元/20 USD`，有效期 365 天，可用于 GPT 写代码和生图。

## 调整后的实现口径

- 流量包权益独立建账，不复用 `users.balance`，也不伪装成 `user_subscriptions`。
- 不新增 Ent schema，不重新生成大量 Ent 代码；本次采用 `database/sql` 仓库实现，降低改动面。
- 不给 `payment_orders` 新增物理字段；流量包订单通过 `order_type=traffic_pack` 区分，商品快照写入现有 `provider_snapshot` JSON。
- 支付流程复用现有订阅/充值订单创建、支付方法选择、支付弹窗和轮询；支付成功后履约为用户新增一笔独立流量包批次。
- 重复购买按批次独立记账，用户前台只展示可用额度汇总。
- 扣费顺序为：GPT 订阅日额度优先；订阅日额度不足或无订阅时，扣未过期 GPT 流量包；多个流量包按最早过期优先，同过期时间按最早购买优先。

## 新增数据表

- `traffic_packs`：流量包商品 SKU，种子三档商品。
- `user_traffic_credits`：用户购买后的批次权益，`order_id` 唯一保证支付履约幂等。
- `traffic_credit_ledger`：入账和扣费流水，支持后续审计。

## 风险控制

- 只对 `platform=openai` 生效，不影响 Anthropic/Gemini/Antigravity。
- 后扣费在数据库事务中执行，流量包余额不足时不写入半笔扣费。
- 既有订阅、余额充值和手动支付逻辑保持原路径，新增逻辑只挂在 `traffic_pack` 分支。
