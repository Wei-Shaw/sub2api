# Sub2API 前端旧缓存清理补充记录

## 背景

修复 Cloudflare 误拦截 `/assets/vendor-*` 后，公网 HTML 与新入口 JS 已经改写为 `/assets/app-index-*` 和 `/assets/libs-*`。但 Chrome 仍可能继续使用旧缓存入口，并请求旧的 `/assets/vendor-vue-DdvVI69T.js`，该旧路径仍会被 Cloudflare 直接返回 403。

## 根因补充

`/assets/vendor-*` 的 403 发生在 Cloudflare 层，本机 nginx 收不到该请求，因此无法在 origin 侧对旧 vendor 路径做 rewrite。必须让浏览器停止使用旧缓存入口。

## 本次补充变更

在 `/opt/homebrew/etc/nginx/servers/cliproxy.conf` 的 `api.aaccx.pw` 通用 location 中增加：

```nginx
add_header Clear-Site-Data "\"cache\"" always;
```

该响应头只清理浏览器对 `api.aaccx.pw` 的缓存，不清理 cookie、localStorage 或登录状态。

## 验证结果

| 验证项 | 结果 |
| --- | --- |
| `https://api.aaccx.pw/?cacheclear=...` | HTTP 200，响应头包含 `Clear-Site-Data: "cache"` |
| 新 HTML | `vendor-*` 计数为 0，`app-index-*` 计数为 1，`libs-*` 计数为 4 |
| `https://api.aaccx.pw/assets/app-index-DUHFzDC1.js?cacheclear=...` | HTTP 200，`vendor-*` 计数为 0，`libs-*` 计数为 9 |
| `https://api.aaccx.pw/assets/vendor-vue-DdvVI69T.js?cacheclear=...` | 仍为 Cloudflare 403，符合预期，说明旧路径必须通过清缓存停止使用 |
| `https://api.aaccx.pw/health` | HTTP 200 |
| `https://api.aaccx.pw/v1/models` 使用 Sub2API 用户 Key | HTTP 200 |
| `nginx -t` | 配置语法通过 |

## 操作提示

用户侧如果仍看到旧 `vendor-*` 403，访问一次 `https://api.aaccx.pw/?cacheclear=1` 或按 `Cmd + Shift + R`，浏览器会拿到 `Clear-Site-Data: "cache"` 并清理旧缓存。
