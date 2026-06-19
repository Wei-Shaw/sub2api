# 手动收款码支付引导实现结果

## 背景

`/purchase` 已开放给用户，但当前没有配置真实支付宝、微信支付服务商与回调。用户要求先让套餐确认页可付款展示：点击“确认支付”后弹出微信/支付宝个人收款码，用户线下付款后再通过兑换码开通订阅。

## 实现

- 新增 `frontend/src/components/payment/ManualPaymentDialog.vue`。
- 新增静态收款码资产：
  - `frontend/src/assets/payment/manual-wxpay.jpg`
  - `frontend/src/assets/payment/manual-alipay.jpg`
- `/purchase` 订阅确认页在 `enabledMethods.length === 0` 时：
  - 仍允许点击“确认支付”。
  - 不调用 `paymentStore.createOrder()`。
  - 打开手动收款码弹窗。
- 弹窗支持：
  - 展示套餐名与金额。
  - 微信/支付宝 Tab 切换。
  - 点击“我已完成支付”后展示提交成功态。
  - 点击“前往兑换”跳转 `/redeem`。
- 保留真实支付分支：如果后续配置了服务商支付方式，仍走原 `createOrder()` 流程。

## 边界

- 不创建 payment order。
- 不写账单、用量或订阅状态。
- 不自动开通订阅。
- “支付已提交”只表示用户自述已付款；权益必须等待管理员确认到账并发放兑换码后，由用户在 `/redeem` 手动兑换。

## 测试

- 新增 `ManualPaymentDialog` 单测，覆盖套餐金额、默认微信二维码、切换支付宝、提交成功态、兑换事件。
- 新增 `PaymentView` 单测，覆盖无支付方式时选择套餐点击确认不创建订单，并打开手动付款弹窗。

## 后续

- 若未来接入真实支付回调，不要复用这条手动弹窗路径做自动开通；应恢复/新增服务商 payment method 并走订单状态确认链路。
