# aaccx.pw 管理后台深链接刷新 404 诊断

## 现象

- 用户在 `https://aaccx.pw/admin/redeem` 页面停留较久后刷新，页面返回 404。
- 从已加载的 Sub2API 前端内部跳转到该页面可以访问。

## 证据

- `curl https://aaccx.pw/admin/redeem` 返回 `HTTP/2 404`，HTML 内容是 yui.web 官网的 `404 - Page Not Found`。
- `curl https://aaccx.pw/dashboard` 返回 `HTTP/2 200`，HTML 内容是 Sub2API 前端入口。
- `curl http://127.0.0.1:18080/admin/redeem` 返回 `HTTP/1.1 200 OK`，说明 Sub2API 自身可以正确处理 SPA 深链接刷新。
- `curl -H 'Host: aaccx.pw' http://127.0.0.1:8080/admin/redeem` 返回 `HTTP/1.1 404 Not Found`，说明问题发生在本机 nginx 的 `aaccx.pw` 分流层，不是 Cloudflare 或 Sub2API 后端。

## 根因

`/opt/homebrew/etc/nginx/servers/aaccx-root.conf` 当前只把以下 Sub2API 控制台路由代理到 `127.0.0.1:18080`：

```nginx
location ~ ^/(dashboard|login|register|keys|subscriptions|settings)(/.*)?$
```

`/admin/redeem` 没有命中该规则，继续落到 yui.web 静态站点 fallback。yui.web 没有 `/admin/redeem.html`，所以返回官网 404。

内部跳转能访问，是因为浏览器已加载 Sub2API SPA，路由由 Vue Router 在客户端接管；刷新会重新发起 HTTP 请求，需要 nginx 把该深链接交还给 Sub2API。

## 最小修复计划

1. 将 `admin` 加入 `aaccx.pw` 的 Sub2API 控制台深链接 nginx 匹配。
2. 执行 `nginx -t` 验证配置语法。
3. 重载 nginx。
4. 验证：
   - `http://127.0.0.1:8080/admin/redeem` 返回 Sub2API HTML 200。
   - `https://aaccx.pw/admin/redeem` 返回 Sub2API HTML 200。
   - `https://aaccx.pw/shop` 仍归 yui.web。

## 取舍

- 只加 `/admin` 前缀，不把所有未知路径都转给 Sub2API，避免抢走 yui.web 官网的普通页面。
- `/api/*` 已经单独代理到 Sub2API，`/admin/*` 这里只用于前端历史路由入口，不影响后端 API 路径。
