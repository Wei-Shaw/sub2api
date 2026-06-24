# GPT 次卡购买与返回入口修复结果

## 结论

- 已修复 `/purchase` 中 GPT 次卡在“无在线支付方式、仅手动收款码”运行态下无法继续购买的问题。
- 已为订阅/次卡确认态补充顶部返回入口，并把底部次级操作统一为 `返回`，避免误解成取消订单。

## 根因回顾

- 次卡确认态原先把 `enabledMethods.length === 0` 视为不可提交，直接禁用 `确认支付`。
- 订阅确认态同场景会复用 `ManualPaymentDialog`，因此两者行为不一致。
- 确认态缺少顶部左侧返回入口，用户只能看到底部 `取消`，返回路径不清晰。

## 实施内容

- `frontend/src/views/user/PaymentView.vue`
  - 次卡确认态在无在线支付方式时改为复用手动收款弹窗，不再禁用按钮。
  - 增加顶部左侧返回按钮。
  - 订阅与次卡确认态底部次级按钮统一为 `返回`。
  - 手动收款弹窗目标改为从当前选中的订阅或次卡生成通用商品摘要。
- `frontend/src/components/payment/ManualPaymentDialog.vue`
  - 入参从订阅计划收敛为通用商品摘要 `{ name, price }`，供订阅池和次卡共用。
- `frontend/src/i18n/locales/zh.ts`
- `frontend/src/i18n/locales/en.ts`
  - 新增手动收款弹窗通用商品标签文案。

## 验证

- `pnpm test:run src/views/user/__tests__/PaymentView.spec.ts`
- `pnpm test:run src/components/payment/__tests__/ManualPaymentDialog.spec.ts`
- `pnpm typecheck`
- `pnpm build`

以上均已通过。构建阶段仅有项目原有的 Vite chunk warning，与本次修复无关。
