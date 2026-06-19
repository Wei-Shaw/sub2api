# aaccx.pw 接入 Sub2API 公网路由结果

## 结论

已把 `aaccx.pw` 的 API 和 Sub2API 控制台入口接到本机 Sub2API：

- `https://aaccx.pw/shop` 继续由 `yui.web` 提供。
- `https://aaccx.pw/shop` 页面“打开 Sub2API”指向 `https://aaccx.pw/dashboard`。
- `https://aaccx.pw/v1/*` 由 Sub2API 提供，走 `Sub2API -> CLIProxyAPI` 链路。
- `https://aaccx.pw/dashboard`、`/login`、`/register`、`/keys`、`/subscriptions`、`/settings` 由 Sub2API 前端提供。
- `https://aaccx.pw/assets/*` 当前统一代理到 Sub2API；`/shop/assets/*` 仍按更高优先级留给 yui.web。

## nginx 路由

修改位置：

```text
/opt/homebrew/etc/nginx/servers/aaccx-root.conf
```

关键边界：

- `/v1/`、`/api/` 代理到 `127.0.0.1:18080`。
- Sub2API 前端路由代理到 `127.0.0.1:18080`。
- `/assets/app-index-*` rewrite 到 Sub2API 内部真实 `/assets/index-*`。
- `/assets/*` 统一 `Cache-Control: no-store`，并对 JS/CSS 做动态 chunk import 改写：
  - `./index-` -> `./app-index-`
  - `assets/index-` -> `assets/app-index-`

这么做的原因是 Cloudflare 或浏览器可能残留旧入口 `index-*`，而 Vue 被双入口加载时可能出现 mount 异常。公网入口使用 `app-index-*` 后，动态 chunk 也必须继续 import `app-index-*`。

## yui.web 跳转

修改位置：

```text
/Users/wujianxiang/CodeSpace/yui.web/server.js
/Users/wujianxiang/CodeSpace/yui.web/shop/index.html
/Users/wujianxiang/CodeSpace/yui.web/.env
/Users/wujianxiang/CodeSpace/yui.web/.env.example
```

当前渲染结果：

```html
href="https://aaccx.pw/dashboard"
```

`shop/index.html` 保留相对路径 `/dashboard` 作为静态兜底，但运行中的 `server.js` 会按 `SUB2API_PUBLIC_URL=https://aaccx.pw/dashboard` 注入公网地址。

## 公网验证

### 页面与资源

- `curl https://aaccx.pw/dashboard?verify=20260618T1148`
  - HTTP 200
  - `Cache-Control: no-store`
  - HTML 主入口为 `/assets/app-index-HOAcWbNE.js`
  - 无 `Clear-Site-Data`
- `curl https://aaccx.pw/shop?verify=20260618T1148`
  - HTTP 200
  - 页面包含 `data-sub2api-link`
  - 链接目标为 `https://aaccx.pw/dashboard`
- `curl https://aaccx.pw/shop/assets/pay/wechat-qr.png?verify=20260618T1148`
  - HTTP 200
  - 返回 PNG，说明 `/shop/assets/*` 未被 Sub2API `/assets/*` 抢走。
- `curl https://aaccx.pw/assets/LoginView-YmLH_fNC.js?verify=20260618T1149`
  - HTTP 200
  - `Cache-Control: no-store`
  - chunk 内引用 `./app-index-HOAcWbNE.js`
- `curl https://aaccx.pw/assets/DashboardView-W5wNMX1X.js?verify=20260618T1149`
  - HTTP 200
  - `Cache-Control: no-store`
  - chunk 内引用 `./app-index-HOAcWbNE.js`

### 浏览器

使用 Codex in-app browser 新标签验证：

- 打开 `https://aaccx.pw/shop?verify=20260618T1150`。
- 点击 “打开 Sub2API”。
- 跳转到 `https://aaccx.pw/dashboard`，随后未登录状态进入 `https://aaccx.pw/login?redirect=/dashboard`。
- 登录页 `#app.children.length = 2`，页面显示邮箱、密码、登录按钮。
- console error 为空。

继续使用 `15951875192@phone.com` 和默认密码登录验证：

- 登录后 URL 为 `https://aaccx.pw/dashboard`。
- 页面标题为 `仪表盘 - Sub2API`。
- 页面显示仪表盘、API 密钥、使用记录、我的订阅和最近使用记录。
- console error 为空。

## API 验证

无 Key：

```text
GET https://aaccx.pw/v1/models
HTTP 401
code=API_KEY_REQUIRED
```

使用本机自用 Key `sk-LOCAL-454...e28804`：

```text
GET https://aaccx.pw/v1/models
HTTP 200
modelCount=10
firstModel=gpt-5.5
```

```text
POST https://aaccx.pw/v1/responses
HTTP 200
responseModel=gpt-5.5
```

数据库确认：

```text
usage_logs api_key_id=32
before=15
after=16
delta=1
latest inbound_endpoint=/v1/responses
```

## 本地验证命令

- `/opt/homebrew/bin/nginx -t`
  - 通过。
- `node --test --test-name-pattern 'Shop 首页使用配置的 Sub2API 公网入口链接' test/shop-flow.test.js`
  - 通过，1/1。
- `node --test test/shop-flow.test.js`
  - 未通过，66 pass / 46 fail。
  - 失败集中在旧邀请码和旧 API key 发放路径返回 `410`，这是当前迁移到 Sub2API 发 Key 后的预期边界变化；本次公网路由和 shop 跳转相关测试已单独通过。

## 剩余风险

- nginx 仍承担 `app-index-*` 入口别名和动态 chunk import 改写；后续更干净的方案是在 Sub2API 构建源头固定入口 chunk 命名，减少公网 sub_filter。
- Cloudflare 边缘缓存可能短暂残留旧 HTML 或旧 JS；当前资源响应已设置 `no-store`，并通过 `app-index-*` 避开旧入口。
- yui.web 全量旧 shop-flow 测试仍有 legacy 发 Key 相关失败；如果后续彻底下线 yui.web 发 Key，应同步清理或重写这些测试。
