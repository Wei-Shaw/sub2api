# Sub2API 公网静态资源缓存修正计划

## 背景

用户侧访问 `https://api.aaccx.pw` 仍然白屏，控制台继续出现：

- `/assets/vendor-i18n-*.js` 403
- `/assets/vendor-vue-*.js` 403
- `/assets/vendor-misc-*.js` 403
- 动态导入 `HomeView-*.js` 失败

当前架构仍保持方案 A：

`Cloudflare Tunnel -> nginx 127.0.0.1:8080 -> Sub2API 127.0.0.1:18080 -> CLIProxyAPI 127.0.0.1:8317`

## 已确认事实

- 公网根 HTML 已被 nginx 改写为 `/assets/app-index-DUHFzDC1.js` 和 `/assets/pkg-*`，不再直接引用 `/assets/vendor-*`。
- Cloudflare 对原始 `/assets/vendor-*` 路径仍直接返回 403，origin nginx 收不到这些请求。
- `/assets/HomeView-DV-G3zoc.js` 当前响应为 200，但响应头仍包含 `Clear-Site-Data: "cache"`，并带上游较长缓存头。
- `/assets/index-DUHFzDC1.js` 当前响应为 200，但仍走通用 `location /`，响应头也包含 `Clear-Site-Data: "cache"` 和较长缓存头。
- `HomeView-DV-G3zoc.js` 内部仍引用 `./index-DUHFzDC1.js`，这会让浏览器有机会命中旧的原始入口缓存。

## 根因判断

问题不是没有域名，而是公网静态资源路径和缓存策略不稳定：

- `vendor-*` 路径被 Cloudflare 拦截，必须从页面和所有 JS chunk 的 import/preload 中彻底消除。
- 只把 HTML 入口改成 `app-index-*` 不够，普通 chunk 仍可能通过 `./index-*` 重新加载旧入口。
- 把 `Clear-Site-Data` 加在所有资源上会让浏览器在加载 chunk 过程中反复清缓存，增加动态导入失败概率。

## 修改计划

1. 在 nginx 中新增 HTML 专用处理，只对 `/` 和 `/index.html` 添加 `Clear-Site-Data: "cache"`。
2. 对 `/assets/pkg-*` 保留路径改写：公网 `pkg-*` -> origin `vendor-*`，响应体继续改写 `vendor-` -> `pkg-`。
3. 对 `/assets/app-index-*` 保留路径改写：公网 `app-index-*` -> origin `index-*`。
4. 对所有 JS/CSS 静态资源统一执行响应体改写：
   - `vendor-` -> `pkg-`
   - `./index-` -> `./app-index-`
   - `assets/index-` -> `assets/app-index-`
5. `/assets/*` 不再返回 `Clear-Site-Data`，并统一返回 `Cache-Control: no-store`，避免旧 chunk 被浏览器或 Cloudflare 长时间持有。

## 验证标准

- `nginx -t` 通过。
- 根 HTML 仍为 200，并包含 `Clear-Site-Data: "cache"`。
- `/assets/HomeView-DV-G3zoc.js` 为 200，不包含 `Clear-Site-Data`，不包含 `vendor-`，不引用 `./index-*`。
- `/assets/index-DUHFzDC1.js` 为 200，不包含 `Clear-Site-Data`，不包含 `vendor-`。
- `/assets/app-index-DUHFzDC1.js` 为 200，不包含 `Clear-Site-Data`，不包含 `vendor-`。
- `/assets/pkg-vue-DdvVI69T.js` 为 200，不包含 `Clear-Site-Data`。
- `/health` 和 `/v1/models` 继续可用。
