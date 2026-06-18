# Sub2API 公网静态资源最终验证

## 验证时间

2026-06-18 09:12 JST

## 验证命令

- `nginx -t`
- Node ESM 断言脚本，请求公网 `https://api.aaccx.pw`，trace 为 `codex-final-1781741530060`

## 结果

- `nginx -t`：通过。
- 根 HTML：HTTP 200，`Clear-Site-Data: "cache"`，不包含 `vendor-*`，包含 `/assets/app-index-*` 和 `pkg-*`。
- `HomeView-DV-G3zoc.js`：HTTP 200，`Cache-Control: no-store`，不包含 `Clear-Site-Data`，不包含 `vendor-*`，不包含 `./index-*`，包含 `./app-index-*`。
- `index-DUHFzDC1.js`：HTTP 200，`Cache-Control: no-store`，不包含 `Clear-Site-Data`，不包含 `vendor-*`。
- `app-index-DUHFzDC1.js`：HTTP 200，`Cache-Control: no-store`，不包含 `Clear-Site-Data`，不包含 `vendor-*`。
- `pkg-vue-DdvVI69T.js`：HTTP 200，`Cache-Control: no-store`，不包含 `Clear-Site-Data`，不包含 `vendor-*`。
- `index-Um9CggmV.css`：HTTP 200，`Cache-Control: no-store`，不包含 `Clear-Site-Data`。
- `/health`：HTTP 200。
- `/v1/models`：使用 Sub2API 用户 key 请求 HTTP 200，未打印 key。
- `/assets/vendor-vue-DdvVI69T.js`：仍为 Cloudflare 403，符合预期；页面链路已不再引用该路径。

## 用户侧建议

如果浏览器还显示旧 `vendor-*` 请求，先访问：

`https://api.aaccx.pw/?trace=codex-final-1781741530060`

根页面会返回 `Clear-Site-Data: "cache"`，清掉旧入口缓存；随后正常打开 `https://api.aaccx.pw/`。
