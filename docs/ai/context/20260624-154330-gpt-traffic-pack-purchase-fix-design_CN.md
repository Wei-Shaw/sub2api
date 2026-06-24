# GPT 次卡购买与返回入口修复设计

## 背景

- 最新 `main` 合入 `feat: 增加 GPT 一次性流量包` 后，用户在 `/purchase` 进入 GPT 次卡确认页时，`确认支付` 按钮不可用。
- 同一页面的订阅池在当前运行态可通过手动收款弹窗继续购买，因此用户预期次卡应复用同样流程。
- 次卡确认页只有底部 `取消`，没有明确的顶部返回入口，用户容易误判为无法回到套餐列表。

## 根因

### 1. 次卡与订阅在“无在线支付方式”场景下分支不一致

- 订阅确认态：
  - `canSubmitSubscription` 在 `enabledMethods.length === 0` 时返回 `true`
  - `confirmSubscribe` 不创建订单，而是打开 `ManualPaymentDialog`
- 次卡确认态：
  - `canSubmitTrafficPack` 在 `enabledMethods.length === 0` 时直接返回 `false`
  - `confirmTrafficPack` 在无支付方式时直接报错 `payment.notAvailable`
- 当前运行态正是“未配置在线支付服务商、只展示手动收款码”的模式，所以次卡被前端自己禁掉了。

### 2. 确认态缺少明确返回入口

- 选中 `selectedPlan` 或 `selectedTrafficPack` 后，界面进入单项确认态。
- 当前只有底部 `取消`，没有顶部、靠左、语义明确的“返回上一层”入口。
- 对用户来说，这里不是“取消已创建订单”，而是“回到商品列表重新选”，语义应该更接近 `返回`。

## 备选方案

### 方案 A：直接让次卡复用现有 `ManualPaymentDialog`

- 优点：最小改动，和订阅池保持一致，不引入新的支付分支。
- 缺点：需要把 `ManualPaymentDialog` 的入参从“订阅计划”收敛成更通用的“商品摘要”。

### 方案 B：为次卡单独做一个手动收款弹窗

- 优点：类型最直观。
- 缺点：重复 UI、重复测试、后续维护会再次分叉。

## 选型

- 采用方案 A。
- 把手动收款弹窗抽象为接收 `name + price` 的商品摘要，订阅池和次卡都走同一个弹窗。
- 在确认态顶部增加左侧返回按钮，并把底部第二按钮统一为“返回”，避免把“返回列表”误写成“取消订单”。

## 影响范围

- 仅前端用户购买页：
  - `frontend/src/views/user/PaymentView.vue`
  - `frontend/src/components/payment/ManualPaymentDialog.vue`
  - 对应前端单测
- 不修改后端支付、订单、计费、流量包发放逻辑。
