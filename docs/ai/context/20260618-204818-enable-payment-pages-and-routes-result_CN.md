# 开启用户购买页、订单页和订阅页结果

## 背景

用户要求把当前项目的三个用户页面加载出来：

- Sub2API 内置购买页：`/purchase`
- 我的订单：`/orders`
- 我的订阅：`/subscriptions`

同时要求把运行配置中的 false 打开，但先不显示支付方式，只让页面展示出来。

## 修改

### nginx 路由

修改 `/opt/homebrew/etc/nginx/servers/aaccx-root.conf`：

```nginx
location ~ ^/(dashboard|login|register|keys|subscriptions|purchase|orders|settings|admin)(/.*)?$ {
```

原因：

- `api.aaccx.pw` 已经全量代理到 Sub2API，不需要补路由。
- `aaccx.pw` 原来只代理了 `/subscriptions`，缺少 `/purchase` 和 `/orders`，会落到 yui.web 侧。
- 只补明确的 Sub2API 控制台路由，避免把 yui.web 的普通文章/页面误交给 Sub2API。

### 运行库设置

在 `sub2api-postgres` 的 `settings` 表中 upsert：

```text
payment_enabled=true
available_channels_enabled=true
```

说明：

- 这两个 key 原本不存在，false 来自代码默认值。
- 当前没有配置支付服务商实例，所以购买页可以展示套餐，但支付方式可能为空或不可用。

### 服务刷新

- `nginx -t` 通过。
- 已执行 `nginx -s reload`。
- 因 HTML 注入缓存仍显示旧值，已重启 `sub2api` 容器刷新注入配置。

## 验证

本地注入配置：

```json
{
  "payment_enabled": true,
  "available_channels_enabled": true
}
```

公网 `api.aaccx.pw` 注入配置：

```json
{
  "payment_enabled": true,
  "available_channels_enabled": true
}
```

公开设置接口：

```json
{
  "payment_enabled": true,
  "available_channels_enabled": true
}
```

公网页面状态：

- `https://aaccx.pw/purchase` 返回 `200`
- `https://aaccx.pw/orders` 返回 `200`
- `https://aaccx.pw/subscriptions` 返回 `200`
- `https://api.aaccx.pw/purchase` 返回 `200`

`https://aaccx.pw/purchase` HTML 资源检查：

```text
vendor_refs=0
pkg_refs=4
app_index_refs=1
payment_true=1
available_true=1
```

## 后续注意

- 当前只是打开页面入口和可用渠道入口，不代表支付闭环已可收款。
- 要真正付款，还需要在管理端配置支付服务商实例和可见支付方式来源。
- 如果只想展示套餐、不允许用户下单，需要后续在前端或配置层增加“仅展示套餐”的明确模式；当前项目的内置逻辑是支付开启后显示购买入口。
