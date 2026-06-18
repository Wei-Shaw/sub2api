# Sub2API 静态资源二级 import 修复记录

## 背景

用户刷新后仍看到白屏，DevTools 报错：

```text
GET https://api.aaccx.pw/assets/vendor-vue-DdvVI69T.js 403 (Forbidden)
```

新的线索是错误来源来自 `libs-i18n-DY-5nrdT.js`，说明首页 HTML 和入口 JS 已经切到新的资源路径，但被改写后的 `libs-i18n` 文件内部仍引用旧的 `./vendor-vue-DdvVI69T.js`。

## 根因

上一轮修复只对 HTML 和 `/assets/app-index-*` 入口 JS 做了 `vendor- -> libs-` 内容改写，漏掉了 `/assets/libs-*` 这类由真实 vendor 文件改名暴露的资源。

因此：

```text
app-index -> libs-i18n -> vendor-vue
```

第二级 import 仍然回到 Cloudflare 会拦截的 `/assets/vendor-*` 路径。

另外，Cloudflare 已经可能缓存过部分 `/assets/libs-*` 文件，因此继续使用 `libs-*` 前缀风险较高。

## 本次修复

修改 `/opt/homebrew/etc/nginx/servers/cliproxy.conf`：

- 公网新前缀从 `/assets/libs-*` 改为 `/assets/pkg-*`。
- `/assets/pkg-*` 在本机 nginx 中 rewrite 回真实 `/assets/vendor-*`。
- 对 `/assets/pkg-*` 响应也启用 `sub_filter "vendor-" "pkg-"`，覆盖二级 import。
- 对 `/assets/app-index-*` 响应继续启用 `sub_filter "vendor-" "pkg-"`。
- 对相关变换后的 JS/CSS 使用 `Cache-Control: no-store`，避免 Cloudflare 继续缓存中间改写产物。
- HTML 继续把真实入口 `/assets/index-*` 改写为 `/assets/app-index-*`，并把 `vendor-*` 改写为 `pkg-*`。

## 验证结果

| 验证项 | 结果 |
| --- | --- |
| `https://api.aaccx.pw/?trace=...` | HTTP 200，`vendor-*` 计数为 0，`pkg-*` 计数为 4，`app-index-*` 计数为 1 |
| `https://api.aaccx.pw/assets/app-index-DUHFzDC1.js?trace=...` | HTTP 200，`vendor-*` 计数为 0，`libs-*` 计数为 0，`pkg-*` 计数为 9 |
| `https://api.aaccx.pw/assets/pkg-i18n-DY-5nrdT.js?trace=...` | HTTP 200，内部 import 已变为 `./pkg-vue-DdvVI69T.js` |
| `https://api.aaccx.pw/assets/pkg-vue-DdvVI69T.js` | HTTP 200 |
| `https://api.aaccx.pw/assets/pkg-misc-DJoKcLuU.js` | HTTP 200 |
| `https://api.aaccx.pw/assets/pkg-chart-MUBsB-vK.js` | HTTP 200 |
| `https://api.aaccx.pw/assets/pkg-ui-BreCn9F4.js` | HTTP 200 |
| `https://api.aaccx.pw/health` | HTTP 200 |
| `https://api.aaccx.pw/v1/models` 使用 Sub2API 用户 Key | HTTP 200 |
| `nginx -t` | 配置语法通过 |

## 后续注意事项

- 旧路径 `/assets/vendor-*` 仍会被 Cloudflare 403，符合预期；浏览器应改为请求 `/assets/pkg-*`。
- 如果用户侧仍看到 `vendor-*`，说明浏览器还持有旧缓存；访问 `https://api.aaccx.pw/?trace=1` 或强刷后应拿到新 HTML。
- 这仍是 Cloudflare WAF 误拦截的规避方案；更根本的做法是在 Cloudflare 后台给 `api.aaccx.pw/assets/vendor-*` 添加允许规则或降低误拦截规则。
