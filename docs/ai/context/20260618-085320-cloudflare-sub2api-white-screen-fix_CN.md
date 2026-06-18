# Sub2API 公网页面白屏修复记录

## 背景

从 `aaccx.pw/shop` 跳转到 `https://api.aaccx.pw` 后，浏览器显示空白页。

## 根因

`https://api.aaccx.pw/` 的 HTML 能正常返回 Sub2API 前端页面，但页面引用的多个静态资源在 Cloudflare 边缘被拦截：

- `/assets/vendor-vue-*.js`
- `/assets/vendor-i18n-*.js`
- `/assets/vendor-misc-*.js`
- `/assets/vendor-misc-*.css`

这些资源在本机 Sub2API `127.0.0.1:18080` 和本机 nginx `127.0.0.1:8080` 下都是 HTTP 200；只有经过 Cloudflare 后变为 HTTP 403，并返回 Cloudflare 的 “Sorry, you have been blocked” 页面。因此问题不在 Sub2API 域名缺失，也不在本机上游，而是 Cloudflare 安全策略误拦截了 `vendor-*` 静态资源路径。

主入口 JS 里也包含 `vendor-*` 动态 import，因此只改 HTML 不够。

## 本次修复

修改 `/opt/homebrew/etc/nginx/servers/cliproxy.conf`：

- 对公网 HTML / JS / CSS 响应执行 `sub_filter`，把 `vendor-` 替换为 `libs-`。
- 增加 `/assets/libs-*` 入口，在本机 nginx 内部 rewrite 回 `/assets/vendor-*` 后再转发到 Sub2API。
- 增加 `/assets/app-index-*` 入口，绕开 Cloudflare 已缓存的旧 `/assets/index-*` 主入口 JS，再在本机 rewrite 回真实 `/assets/index-*`。
- 对 HTML 中的主入口 `src="/assets/index-*"` 改写为 `src="/assets/app-index-*"`。

修复后公网看到的路径不再包含 `vendor-`，Cloudflare 不再误拦截；本机仍请求 Sub2API 的真实静态文件。

## 验证结果

| 验证项 | 结果 |
| --- | --- |
| `https://api.aaccx.pw/` HTML | HTTP 200，不再引用 `vendor-*`，改为 `libs-*` / `app-index-*` |
| `https://api.aaccx.pw/assets/app-index-DUHFzDC1.js` | HTTP 200，内容中 `vendor-*` 计数为 0，`libs-*` 计数为 9 |
| `https://api.aaccx.pw/assets/libs-vue-DdvVI69T.js` | HTTP 200，`text/javascript` |
| `https://api.aaccx.pw/assets/libs-i18n-DY-5nrdT.js` | HTTP 200，`text/javascript` |
| `https://api.aaccx.pw/assets/libs-misc-DJoKcLuU.js` | HTTP 200，`text/javascript` |
| `https://api.aaccx.pw/assets/libs-misc-DB0Q8XAf.css` | HTTP 200，`text/css` |
| 抽查首页 / 登录页 chunk | HTTP 200，无 Cloudflare 403 |
| `https://api.aaccx.pw/health` | HTTP 200 |
| `https://api.aaccx.pw/v1/models` 使用 Sub2API 用户 Key | HTTP 200 |
| `nginx -t` | 配置语法通过 |

## 后续注意事项

- 如果后续重新构建 Sub2API 前端，入口 hash 可能变化；当前 nginx 规则按 `index-*` / `vendor-*` 前缀匹配，不依赖固定 hash。
- 如果未来在 Cloudflare 后台关闭对应 WAF 误拦截，可以去掉这层路径规避；但在未确认前不要移除。
- 公网 API 路径 `/v1/*` 不受该修复影响，仍由 Sub2API 处理。
