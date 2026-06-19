# 用户付款入口、价格表和模型表调查

## 背景

用户反馈当前用户页面不清楚应在哪里付款，也没有明显的价格表和模型表。目标是确认 Sub2API 是否自带这些能力，以及当前运行实例为什么没有暴露。

## 代码侧结论

- Sub2API 自带内置支付系统。
- 用户侧付款入口是 `/purchase`，前端文案为 `nav.buySubscription`，中文/英文含义是充值 / 订阅。
- 用户侧订单页是 `/orders`。
- 用户侧我的订阅页是 `/subscriptions`。
- 管理端套餐配置页是 `/admin/orders/plans`，接口为 `/api/v1/admin/payment/plans`。
- 用户购买页会调用 `/api/v1/payment/checkout-info`，该接口一次返回可见支付方式、支付限额、可售订阅套餐和帮助信息。
- 购买页里的订阅套餐卡片就是项目内置的“价格表”，字段包括 `price`、`original_price`、`validity_days`、`rate_multiplier`、`daily_limit_usd`、`weekly_limit_usd`、`monthly_limit_usd`、`features`。
- 用户侧模型/价格表不是放在购买页，而是 `/available-channels`，前端文案为“可用渠道”。它展示渠道、用户可访问分组、支持模型以及模型定价。
- `/available-channels` 由 `available_channels_enabled` 控制，默认关闭。
- `/purchase` 和 `/orders` 由 `payment_enabled` 控制，当前路由守卫在关闭时会把用户打回 dashboard。

## 当前运行态

通过 `http://127.0.0.1:18080/` 的 `window.__APP_CONFIG__` 以及 `https://api.aaccx.pw/` 的注入配置确认：

- `payment_enabled=false`
- `available_channels_enabled=false`
- `purchase_subscription_enabled=false`
- `purchase_subscription_url=""`
- `custom_menu_items=[]`

因此当前用户页看不到明显付款入口和模型表，根因不是项目没有功能，而是对应功能开关未开启，且信息架构上“套餐价格”和“模型价格”被拆在两个页面。

## 用户应去哪里付款

如果启用内置支付：

- 用户从侧边栏进入 `/purchase`，选择“订阅”页签，然后选择套餐并付款。
- 如果余额充值未禁用，也可在同一页面使用“充值”页签给余额充值。
- 用户可在 `/orders` 查看订单。

如果继续沿用外部购买页：

- 应通过自定义菜单或首页/shop 按钮跳转外部购买页。
- 但这会让 Sub2API 不是完整的购买闭环，后续仍要同步订单和订阅权益。

## 建议

第一阶段按 Sub2API 内置能力走，开启并配置：

1. 管理端开启 `payment_enabled`。
2. 管理端创建可售套餐，对应当前 `codex-pool-19-usd`、`codex-pool-29-usd`、`codex-pool-49-usd`。
3. 配置至少一个支付服务商实例，或先只展示套餐不开放支付。
4. 开启 `available_channels_enabled`，让用户能从 `/available-channels` 看模型和定价。
5. 后续如要更清楚，应把 dashboard 或 `/purchase` 顶部加一个“选择套餐 / 查看模型价格”的直接入口，而不是只依赖侧边栏。

## 注意

- 不要把 yui.web、Sub2API 和 CLIProxyAPI 同时做同一用户 Key 的扣费事实源。
- 如果让 yui.web 继续做购买页，Sub2API 仍必须是 API Key、订阅权益、用量和限额事实源。
- 当前没有修改支付配置和数据库，只完成调查记录。
