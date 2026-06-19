# aaccx.pw dashboard 白屏修复记录

## 现象

`https://aaccx.pw/dashboard` 公网页面只显示空白。公网 HTML 返回 200，`#app` 存在，但前端没有正常挂载。

## 根因

Sub2API 当前嵌入式前端仍会生成 `vendor-*` 入口预加载资源，例如：

- `/assets/vendor-vue-DdvVI69T.js`
- `/assets/vendor-i18n-DY-5nrdT.js`
- `/assets/vendor-misc-DJoKcLuU.js`

Cloudflare 会拦截 `vendor-*` 静态资源，公网请求返回 403。`aaccx.pw` 的 nginx 只把入口脚本 `index-*` 改写成了 `app-index-*`，但没有像 `api.aaccx.pw` 一样把 `vendor-*` 对外改写为 `pkg-*`，导致 Vue runtime 无法加载，页面白屏。

## 修复

修改本机 nginx 配置：

- 文件：`/opt/homebrew/etc/nginx/servers/aaccx-root.conf`
- 在 `aaccx.pw` server 的 Sub2API 资源代理中补齐：
  - `/assets/pkg-*` rewrite 到内部 `/assets/vendor-*`
  - HTML 和 JS/CSS 响应 `sub_filter "vendor-" "pkg-"`
  - 保留现有 `index-* -> app-index-*` 规避旧入口缓存规则

执行：

```bash
nginx -t
nginx -s reload
```

## 验证

修复后：

- `https://aaccx.pw/dashboard` HTML 返回 200。
- HTML 中入口资源变为：
  - `/assets/app-index-DUHFzDC1.js`
  - `/assets/pkg-vue-DdvVI69T.js`
  - `/assets/pkg-i18n-DY-5nrdT.js`
  - `/assets/pkg-misc-DJoKcLuU.js`
- `https://aaccx.pw/assets/pkg-vue-DdvVI69T.js` 返回 200，`content-type: text/javascript`。
- 原始 `https://aaccx.pw/assets/vendor-vue-DdvVI69T.js` 仍被 Cloudflare 403，这是预期；公网页面不应再引用它。

## 后续注意

`aaccx.pw` 和 `api.aaccx.pw` 都需要保持同一套 Sub2API 前端资源改写规则。若后续从 Vite 源头彻底改为 `pkg-*` chunk 命名，可再移除 nginx 中的 `vendor-* -> pkg-*` workaround。
