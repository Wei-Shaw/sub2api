# aaccx.pw dashboard 白屏修复结果：pkg 静态资源 MIME 恢复

## 时间

2026-06-19 13:13 JST

## 根因

`https://aaccx.pw/dashboard` 白屏的直接原因是公网 nginx 仍保留旧的 `pkg-* -> vendor-*` rewrite。

当前 Sub2API origin 已从 Vite 源头生成 `pkg-*` chunk，真实存在的是 `/assets/pkg-*.js/css`。旧 rewrite 把这些真实资源改成不存在的 `/assets/vendor-*`，后端静态资源找不到后按 SPA fallback 返回 `index.html`，导致浏览器把 `text/html` 当 JS/CSS 加载并报 MIME 错误。

## 修改内容

已修改两个 nginx server 配置：

- `/opt/homebrew/etc/nginx/servers/aaccx-root.conf`
- `/opt/homebrew/etc/nginx/servers/cliproxy.conf`

删除内容：

```nginx
rewrite ^/assets/pkg-(.*)$ /assets/vendor-$1 break;
```

保留内容：

- `/assets/app-index-* -> /assets/index-*` 入口兼容规则。
- `/assets/*` 的 `Cache-Control: no-store`。
- 现有 `vendor- -> pkg-` 响应体改写；当前新产物不依赖它，后续可单独清理。

## 验证

### 配置验证

`nginx -t` 通过：

```text
nginx: the configuration file /opt/homebrew/etc/nginx/nginx.conf syntax is ok
nginx: configuration file /opt/homebrew/etc/nginx/nginx.conf test is successful
```

已执行 `nginx -s reload`。

### 本地验证

`node scripts/verify-local-frontend-assets.mjs` 通过：

```json
{
  "baseUrl": "http://127.0.0.1:18080",
  "htmlAssets": 6,
  "checkedAssets": 125,
  "vendorReferences": 0
}
```

本地 API 完整链路：

```text
local /v1/models: 200, first_model=gpt-5.5
local /v1/chat/completions: 200, content=pong
```

### 公网前端资源验证

以 `https://aaccx.pw/dashboard` 为入口递归检查 125 个前端资源，通过：

```text
htmlStatus=200
checkedAssets=125
vendorReferences=0
```

关键资源 MIME：

```text
/assets/app-index-IE2dMgjs.js       text/javascript
/assets/pkg-vue-DdvVI69T.js         text/javascript
/assets/pkg-i18n-ECmPCSvH.js        text/javascript
/assets/pkg-misc-DRGW1HPS.js        text/javascript
/assets/pkg-misc-DB0Q8XAf.css       text/css
/assets/index-CCGqWopy.css          text/css
```

`https://api.aaccx.pw/` 也递归检查 125 个前端资源，通过，同样没有 `vendor-*` 引用。

### 公网 API 验证

使用本机 active Key 验证，不记录完整 Key：

```text
https://aaccx.pw /v1/models: 200, first_model=gpt-5.5
https://aaccx.pw /v1/chat/completions: 200, content=pong
https://api.aaccx.pw /v1/models: 200, first_model=gpt-5.5
https://api.aaccx.pw /v1/chat/completions: 200, content=pong
```

## 后续注意

不要再把公网 `/assets/pkg-*` 反向 rewrite 到 `/assets/vendor-*`。只有在 origin 产物重新变回 `vendor-*` 时才需要这种兼容，但当前正确方向是保持 Vite 源头输出 `pkg-*`。

如果后续清理 nginx workaround，应分两步做：

1. 先确认公网 HTML、入口 JS、所有动态 import 都不包含 `vendor-`。
2. 再移除无效的 `vendor- -> pkg-` 响应体改写。
