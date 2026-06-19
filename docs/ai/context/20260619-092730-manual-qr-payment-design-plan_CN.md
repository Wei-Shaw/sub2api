# 手动收款码支付引导设计与计划

## 背景

当前 `/purchase` 已能展示套餐，但没有配置支付宝、微信等真实支付服务商回调。用户希望点击“确认支付”后弹出微信/支付宝个人收款码，用户自行扫码付款；付款后页面显示支付完成，并引导用户去 `/redeem` 输入管理员发放的兑换码完成权益开通。

## 边界

- 这是手动支付引导，不是真实支付闭环。
- 不创建 Sub2API payment order。
- 不自动开通订阅。
- 不写 usage、billing 或订阅状态。
- “我已完成支付”只表示用户自述已支付，最终权益仍以兑换码兑换为准。

## 方案

在前端新增 `ManualPaymentDialog.vue`：

- 展示套餐名、金额。
- 展示微信和支付宝两个收款码 Tab。
- 使用本机图片：
  - 微信：`/Users/wujianxiang/Downloads/IMG_8624.JPG`
  - 支付宝：`/Users/wujianxiang/Downloads/IMG_8623.JPG`
- 用户点击“我已完成支付”后显示成功态和说明。
- 成功态提供“前往兑换”按钮，跳转 `/redeem`。
- 提供取消/关闭按钮，不影响当前套餐选择。

接入 `PaymentView.vue`：

- 当订阅确认页没有任何可用支付方式时，仍允许点击“确认支付 ¥xx”。
- 此时不调用 `paymentStore.createOrder()`，只打开手动支付弹窗。
- 如果未来配置了真实支付方式，保持原来的创建订单流程。

## 文件计划

- 新增：`frontend/src/components/payment/ManualPaymentDialog.vue`
- 新增测试：`frontend/src/components/payment/__tests__/ManualPaymentDialog.spec.ts`
- 修改：`frontend/src/views/user/PaymentView.vue`
- 修改测试：`frontend/src/views/user/__tests__/PaymentView.spec.ts`
- 新增资源：
  - `frontend/src/assets/payment/manual-wxpay.jpg`
  - `frontend/src/assets/payment/manual-alipay.jpg`
- 修改 i18n：
  - `frontend/src/i18n/locales/zh.ts`
  - `frontend/src/i18n/locales/en.ts`

## TDD 计划

1. 写 `ManualPaymentDialog` 失败测试：
   - 展示套餐名和金额。
   - 默认展示微信收款码。
   - 切换支付宝后展示支付宝收款码。
   - 点击“我已完成支付”后显示成功态。
   - 点击“前往兑换”发出 redeem 事件。
2. 运行单测确认失败。
3. 实现 `ManualPaymentDialog.vue`。
4. 运行单测确认通过。
5. 写 `PaymentView` 失败测试：
   - 在没有支付方式、有套餐时，选择套餐后按钮不禁用。
   - 点击确认支付不调用 `createOrder`，而是显示手动支付弹窗。
6. 运行单测确认失败。
7. 接入 `PaymentView.vue`。
8. 运行相关单测和前端 build。
9. 重新构建并重启 Sub2API 容器，使公网嵌入式前端生效。
10. 浏览器验证 `/purchase` 点击确认支付能弹出二维码，点击完成后可跳 `/redeem`。

## 风险

- 收款码图片会进入前端静态资源，任何访问网站的人都可看到；这是本需求的预期行为。
- 用户点“已完成支付”不等于到账，页面文案必须明确“请使用兑换码开通”。
- 兑换码仍需管理员线下确认到账后发放。
