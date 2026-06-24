# GPT 一次性流量包合并前 Review 记录

## 背景

用户要求将 GPT 一次性流量包功能提交到新的 `codex/` 分支，完成本地 review 后合并到本地 `main`。

## Review 结论

- 后端扣费规则符合设计：订阅日/周/月额度可用时优先扣订阅额度；订阅额度不可用或无订阅时，OpenAI/GPT 平台请求可使用流量包；非 OpenAI 平台不使用流量包。
- 流量包批次不合并，购买后生成独立批次，有效期 365 天；扣费顺序按 `expires_at ASC, credited_at ASC, id ASC`，等价于先到期优先、同到期再按购买顺序。
- 支付链路已覆盖 `traffic_pack` 订单类型、订单快照、微信支付恢复上下文和履约发放。
- 发现并修复一个前端风险：购买页不应在后端返回空 `traffic_packs` 时回退展示硬编码默认流量包，否则后台下架或迁移未完成时用户仍能看到购买入口。修复后以前端接口返回为准，并补充回归测试。

## 验证

- `go test -count=1 -tags=unit ./internal/service ./internal/repository -run 'TestPlanTrafficCreditDeductions|TestBuildUsageBillingCommand_UsesTrafficPackInsteadOfBalance|TestBillingCacheServiceAllowsOpenAITrafficPackWhenBalanceEmpty|TestBillingCacheServiceDoesNotUseTrafficPackForOtherPlatforms|TestTrafficPackRepository|TestCreateOrderInTx_WritesTrafficPackSnapshot|TestExecuteTrafficPackFulfillment_CreditsBatchAndIsIdempotent'`
- `go test -count=1 ./cmd/server -run '^$'`
- `pnpm test:run src/views/user/__tests__/PaymentView.spec.ts`
- `pnpm typecheck`
- `pnpm build`
