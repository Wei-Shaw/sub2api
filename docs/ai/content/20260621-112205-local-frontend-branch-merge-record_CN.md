# 本地前端改动分支合并记录

## 背景

根据用户要求，将当前本地未提交的前端改动拆分到不同的新分支提交，再依次合并回本地 `main`。

## 合并边界

- 只处理本次前端页面与前端测试改动。
- 未提交 `.tmp-*`、数据库 dump、sqlite 备份、`deploy/.env.scheme-a.runtime` 等本地临时或环境文件。
- 未拉取远端 `origin/main`，未推送远端；本次只完成本地分支、本地提交和本地合并。

## 分支与提交

1. `codex/home-landing-copy-20260621`
   - 功能提交：`668d65d0 feat: 精简首页登录和套餐展示`
   - 合并提交：`e15e9abd merge: 首页登录和套餐展示`
   - 范围：首页未登录入口、三张套餐卡片文案、支持模型展示、首页测试。

2. `codex/purchase-subscription-default-20260621`
   - 功能提交：`6fa2b2b5 feat: 默认展示订阅购买页签`
   - 合并提交：`9e5bc800 merge: 默认展示订阅购买页签`
   - 范围：`/purchase` 默认展示订阅，页签顺序改为订阅在左、充值在右，并补回归测试。

3. `codex/subscription-card-copy-price-20260621`
   - 功能提交：`f321c9b2 feat: 精简订阅卡片价格文案`
   - 合并提交：`6092f4a6 merge: 精简订阅卡片价格文案`
   - 范围：购买页订阅卡片价格改为 `¥xx元`，描述统一为「月度订阅-时间 30天，日限额 x刀，24点刷新」，隐藏重复美元价格、周期、倍率、模型 scope 和 features。

## 验证记录

- 首页分支提交前运行：`pnpm vitest run src/views/__tests__/HomeView.spec.ts src/__tests__/visualThemeSource.spec.ts`，2 个测试文件通过，4 个测试通过。
- 购买页签分支提交前运行：`pnpm vitest run src/views/user/__tests__/PaymentView.spec.ts`，1 个测试文件通过，8 个测试通过。
- 订阅卡片分支提交前运行：`pnpm vitest run src/components/payment/__tests__/SubscriptionPlanCard.spec.ts`，1 个测试文件通过，3 个测试通过。

## 后续状态

- 本地 `main` 已包含上述三个分支的合并结果。
- 合并记录与 `AGENTS.md` 协作记忆由 `codex/merge-record-20260621` 分支提交并作为最后一步合并回本地 `main`。
