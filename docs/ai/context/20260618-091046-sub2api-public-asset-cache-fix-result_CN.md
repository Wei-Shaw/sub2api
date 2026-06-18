# Sub2API 公网静态资源缓存修正结果

## 修改内容

已更新 `/opt/homebrew/etc/nginx/servers/cliproxy.conf`：

- 保留公网 `/assets/pkg-*` -> origin `/assets/vendor-*` 的路径改写。
- 保留公网 `/assets/app-index-*` -> origin `/assets/index-*` 的路径改写。
- 新增通用 `/assets/` 处理，统一返回 `Cache-Control: no-store`。
- `/assets/*` 不再返回 `Clear-Site-Data`。
- JS/CSS 静态资源统一执行响应体改写：
  - `vendor-` -> `pkg-`
  - `./index-` -> `./app-index-`
  - `assets/index-` -> `assets/app-index-`
- 只在 `/` 和 `/index.html` 上返回 `Clear-Site-Data: "cache"`，用于清理用户浏览器中的旧入口缓存。

## 验证结果

已执行：

- `nginx -t`：通过，无警告。
- `brew services restart nginx`：已重启。
- `https://api.aaccx.pw/?trace=codex-fix-090750-root`：HTTP 200，包含 `Clear-Site-Data: "cache"`。
- `https://api.aaccx.pw/assets/HomeView-DV-G3zoc.js?trace=codex-fix-090750-homeview`：HTTP 200，`Cache-Control: no-store`，不包含 `Clear-Site-Data`。
- `https://api.aaccx.pw/assets/index-DUHFzDC1.js?trace=codex-fix-090750-index`：HTTP 200，`Cache-Control: no-store`，不包含 `Clear-Site-Data`。
- `https://api.aaccx.pw/assets/app-index-DUHFzDC1.js?trace=codex-fix-090750-app-index`：HTTP 200，`Cache-Control: no-store`，不包含 `Clear-Site-Data`。
- `https://api.aaccx.pw/assets/pkg-vue-DdvVI69T.js?trace=codex-fix-090750-pkg-vue`：HTTP 200，`Cache-Control: no-store`，不包含 `Clear-Site-Data`。
- `https://api.aaccx.pw/assets/index-Um9CggmV.css?trace=codex-fix-090750-index-css`：HTTP 200，`Cache-Control: no-store`。
- `https://api.aaccx.pw/health?trace=codex-fix-090750-health`：HTTP 200。
- 使用 Sub2API 用户 key 请求 `https://api.aaccx.pw/v1/models?trace=codex-fix-090750-models`：HTTP 200。

内容断言：

| 文件 | `vendor-` | `pkg-` | `./index-` | `./app-index-` |
|---|---:|---:|---:|---:|
| 根 HTML | 0 | 4 | 0 | 0 |
| `HomeView-DV-G3zoc.js` | 0 | 3 | 0 | 1 |
| `index-DUHFzDC1.js` | 0 | 9 | 0 | 0 |
| `app-index-DUHFzDC1.js` | 0 | 9 | 0 | 0 |
| `pkg-vue-DdvVI69T.js` | 0 | 0 | 0 | 0 |

## 当前判断

服务端现在不会再从新的 HTML / JS chunk 链路主动请求 `/assets/vendor-*`。如果用户浏览器仍看到 `vendor-*` 403，优先判断为本地页面或 Service Worker / DevTools 缓存仍在执行旧 chunk；访问一次带新查询参数的根页面会触发 `/` 上的 `Clear-Site-Data`。
