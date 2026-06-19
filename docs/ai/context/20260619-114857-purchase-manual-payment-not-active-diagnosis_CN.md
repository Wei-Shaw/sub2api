# /purchase 手动收款码未生效排查记录

## 现象

公网 `https://aaccx.pw/purchase` 中，选择订阅套餐后，“确认支付 ¥39.00”按钮处于禁用态。点击购买没有弹出支付宝/微信个人收款码。

截图特征：

- 页面能正常打开，说明 `/purchase` 路由和静态资源加载已恢复。
- 套餐确认页已展示“39 元订阅池”。
- 没有展示支付方式选择器。
- “确认支付 ¥39.00”按钮是禁用状态。

## 预期逻辑

本地源码中的目标逻辑已经存在：

- `frontend/src/components/payment/ManualPaymentDialog.vue` 存在。
- `frontend/src/views/user/PaymentView.vue` 已引入 `ManualPaymentDialog`。
- `canSubmitSubscription` 中有：

```ts
if (enabledMethods.value.length === 0) return true
```

- `confirmSubscribe()` 中有：

```ts
if (enabledMethods.value.length === 0) {
  showManualPaymentDialog.value = true
  return
}
```

因此，如果运行中的前端是这版代码，当没有支付方式时按钮应该可点击，并打开手动收款码弹窗。

## 实际运行代码

公网 `/purchase` 当前加载的入口资源：

- `/assets/app-index-DUHFzDC1.js`
- `/assets/pkg-vue-DdvVI69T.js`
- `/assets/pkg-i18n-DY-5nrdT.js`
- `/assets/pkg-misc-DJoKcLuU.js`

从公网入口 JS 解析到购买页 chunk：

- `https://aaccx.pw/assets/PaymentView-BELUiT9u.js`

对该运行中 chunk 做关键字检查：

- 未包含 `ManualPaymentDialog`
- 未包含 `manual-payment`
- 未包含 `payment.manual`
- 未包含 `showManualPaymentDialog`
- 未包含 `enabledMethods.value.length === 0`

运行中 chunk 仍是旧逻辑。其订阅按钮可用性等价于：

```ts
m.value !== null && re(m.value.price, E.value) && he.value?.available !== false
```

其中：

- `m.value` 是已选择套餐。
- `E.value` 是当前支付方式。
- 没有支付方式时 `E.value` 为空。
- `re(price, '')` 会因为找不到支付方式限制对象而返回 `false`。

所以无支付方式时按钮会禁用，和截图一致。

## 根因

根因不是点击事件失效，也不是收款码组件内部问题，而是：

**当前公网运行的 Sub2API 前端产物仍是旧版，未包含手动收款码功能。**

源码中有新功能，但运行容器实际返回的是旧 `PaymentView-BELUiT9u.js`。这个旧 chunk 仍要求必须存在可用支付方式，所以在未配置支付服务商时按钮被禁用。

## 支撑证据

1. 源码存在新逻辑：

```bash
rg -n "ManualPaymentDialog|showManualPaymentDialog|enabledMethods\\.value\\.length === 0" frontend/src
```

结果命中：

- `frontend/src/components/payment/ManualPaymentDialog.vue`
- `frontend/src/views/user/PaymentView.vue`

2. 公网运行 chunk 不存在新逻辑：

```bash
curl -skL -o /tmp/public-paymentview.js https://aaccx.pw/assets/PaymentView-BELUiT9u.js
rg -n "ManualPaymentDialog|manual-payment|payment\\.manual|showManualPaymentDialog|enabledMethods\\.value\\.length" /tmp/public-paymentview.js
```

结果无命中。

3. 公网运行 chunk 存在旧按钮逻辑：

```js
bt = computed(() =>
  m.value !== null && re(m.value.price, E.value) && he.value?.available !== false
)
```

4. 当前工作区没有可直接确认的新嵌入式 dist：

```bash
find . -maxdepth 5 -type f \( -name '*PaymentView*.js' -o -name '*manual*.jpg' -o -name 'index.html' \)
```

只看到源码资产：

- `frontend/index.html`
- `frontend/src/assets/payment/manual-alipay.jpg`
- `frontend/src/assets/payment/manual-wxpay.jpg`

未看到已嵌入到 `backend/internal/web/dist` 的新构建产物。

## 可能发生的部署链路问题

之前本地 `pnpm build` 曾通过，并生成过包含收款码资源的前端产物。但后续 Docker 多阶段构建在 Go 编译阶段曾因本机磁盘空间不足出现过临时文件系统错误：

- `read-only file system`
- `input/output error`
- `go: unlinkat /tmp/go-build...`

因此很可能是：

1. 源码改动存在。
2. 本地前端单独 build 曾通过。
3. 新前端没有成功进入最终 Docker 镜像。
4. 当前重启的是旧镜像或旧嵌入式前端产物。

## 当前不要做的事

- 不要去改点击按钮逻辑。源码逻辑已经有手动付款分支。
- 不要临时配置假支付方式绕过按钮禁用。这样会走真实订单创建路径，不符合“手动收款码 + 兑换码”的边界。
- 不要只改 nginx。nginx 当前能正确加载 `/purchase`，问题在应用前端产物版本。

## 后续建议

后续真正修复时，重点不是写新业务代码，而是完成一次可靠部署：

1. 清理本机磁盘空间，至少保证 Docker/Go 编译有足够临时空间。
2. 重新构建前端和最终 Docker 镜像。
3. 确认最终镜像内的公网 chunk 包含：
   - `ManualPaymentDialog`
   - `payment.manual`
   - `showManualPaymentDialog`
4. 重启 `sub2api` 容器。
5. 重新验证公网：

```bash
curl -skL https://aaccx.pw/purchase
curl -skL https://aaccx.pw/assets/<新的 PaymentView chunk>.js | rg "ManualPaymentDialog|payment\\.manual|showManualPaymentDialog"
```

6. 浏览器打开 `/purchase`，选择套餐后确认：
   - 按钮可点击。
   - 弹出微信/支付宝收款码。
   - 点击“我已完成支付”后进入提交成功态。
   - “前往兑换”跳转 `/redeem`。
