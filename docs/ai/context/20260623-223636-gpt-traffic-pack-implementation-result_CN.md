# GPT 一次性流量包实现结果

## 已完成

- 新增 GPT/OpenAI 一次性流量包三档：2 元/5 USD、3 元/10 USD、5 元/20 USD，有效期 365 天。
- 新增独立 SQL 表：`traffic_packs`、`user_traffic_credits`、`traffic_credit_ledger`。
- 支付订单支持 `order_type=traffic_pack` 和 `traffic_pack_id`，支付弹窗复用现有支付链路。
- 流量包商品快照写入 `payment_orders.provider_snapshot`，支付成功后按订单快照幂等入账。
- 请求准入支持 OpenAI/GPT 平台在余额或订阅额度不可用时使用未过期流量包。
- 用量后扣费支持 `TrafficPackCost`，生产路径在 `usage_billing_dedup` 同一事务后扣流量包，避免重复扣费。
- 多批次扣费顺序：最早过期优先，同过期时间按最早购买批次优先。
- `/purchase` 前端新增三张流量包卡片和购买确认区，购买时传 `order_type=traffic_pack`、`traffic_pack_id`。

## 验证

- `go test -tags=unit ./internal/service ./internal/repository -run 'TestPlanTrafficCreditDeductions|TestBuildUsageBillingCommand_UsesTrafficPackInsteadOfBalance|TestBillingCacheServiceAllowsOpenAITrafficPackWhenBalanceEmpty|TestBillingCacheServiceDoesNotUseTrafficPackForOtherPlatforms|TestTrafficPackRepository|TestCreateOrderInTx_WritesTrafficPackSnapshot|TestExecuteTrafficPackFulfillment_CreditsBatchAndIsIdempotent'` 通过。
- `go test ./cmd/server -run '^$'` 通过。
- `pnpm test:run src/views/user/__tests__/PaymentView.spec.ts` 通过。
- `pnpm typecheck` 通过。
- `pnpm build` 通过。

## 本地端口状态

- 前端预览 `http://localhost:5173/purchase` 返回 200，当前 Vite 进程会热更新源码页面。
- 本机 `127.0.0.1:18080` 和 `127.0.0.1:18081` 当前由 Docker 进程监听，仍是旧运行后端；新后端接口需要执行 `151_gpt_traffic_packs.sql` 并重启/部署新版后端后才会真实生效。
