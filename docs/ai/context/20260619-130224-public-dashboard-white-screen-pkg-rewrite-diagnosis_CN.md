# 公网 dashboard 白屏排查：pkg 静态资源被旧 nginx rewrite 打坏

## 时间

2026-06-19 13:02 JST

## 用户问题

`https://aaccx.pw/dashboard` 仍然白屏，需要排查原因。

## 结论

当前白屏根因不是 dashboard 业务接口、登录态或 Vue 路由本身，而是公网 nginx 仍保留旧的 `/assets/pkg-* -> /assets/vendor-*` workaround。

现在 Sub2API 本地运行产物已经从 Vite 源头生成 `pkg-*` chunk，真实存在的是：

- `/assets/pkg-vue-DdvVI69T.js`
- `/assets/pkg-i18n-ECmPCSvH.js`
- `/assets/pkg-misc-DRGW1HPS.js`
- `/assets/pkg-chart-DtGmxNIH.js`
- `/assets/pkg-ui-VdeNot8c.js`

但公网 nginx 看到 `/assets/pkg-*` 后仍会重写到 `/assets/vendor-*`。这些 `vendor-*` 在当前运行产物里已经不存在，Sub2API 静态资源 handler 又会对不存在的 SPA 路径 fallback 到 `index.html`，于是浏览器把 HTML 当 JS/CSS 加载，应用无法挂载，表现为白屏。

## 复现证据

### dashboard HTML

`https://aaccx.pw/dashboard?debugts=20260619-1` 返回 200，HTML 引用：

```text
/assets/app-index-IE2dMgjs.js
/assets/pkg-vue-DdvVI69T.js
/assets/pkg-i18n-ECmPCSvH.js
/assets/pkg-misc-DRGW1HPS.js
/assets/pkg-misc-DB0Q8XAf.css
/assets/index-CCGqWopy.css
```

本地直连 `http://127.0.0.1:18080/dashboard?debugts=20260619-1` 返回 200，HTML 引用：

```text
/assets/index-IE2dMgjs.js
/assets/pkg-vue-DdvVI69T.js
/assets/pkg-i18n-ECmPCSvH.js
/assets/pkg-misc-DRGW1HPS.js
/assets/pkg-misc-DB0Q8XAf.css
/assets/index-CCGqWopy.css
```

说明当前运行态已经是 `pkg-*` 产物；公网只额外把入口脚本从 `index-*` 暴露为 `app-index-*`。

### 入口 JS 正常，但 pkg 依赖异常

公网入口脚本正常：

```text
https://aaccx.pw/assets/app-index-IE2dMgjs.js
200 text/javascript
```

从入口 JS 提取 `/dashboard` 懒加载依赖：

```text
assets/DashboardView-kPHRYncl.js
assets/usage-BcU2mu8H.js
assets/AppLayout.vue_vue_type_script_setup_true_lang-DRnN8q5K.js
assets/pkg-chart-DtGmxNIH.js
assets/TokenUsageTrend.vue_vue_type_script_setup_true_lang-DMUIeWO9.js
assets/EmptyState.vue_vue_type_script_setup_true_lang-7NGHOmzf.js
assets/user-BpbrHp8f.js
assets/pkg-ui-VdeNot8c.js
```

公网和本地对比：

```text
PUBLIC /assets/DashboardView-kPHRYncl.js  200 text/javascript
LOCAL  /assets/DashboardView-kPHRYncl.js  200 text/javascript

PUBLIC /assets/pkg-chart-DtGmxNIH.js       200 text/html
LOCAL  /assets/pkg-chart-DtGmxNIH.js       200 text/javascript

PUBLIC /assets/pkg-ui-VdeNot8c.js          200 text/html
LOCAL  /assets/pkg-ui-VdeNot8c.js          200 text/javascript
```

首页预加载资源也同样异常：

```text
PUBLIC /assets/pkg-vue-DdvVI69T.js         200 text/html
LOCAL  /assets/pkg-vue-DdvVI69T.js         200 text/javascript

PUBLIC /assets/pkg-i18n-ECmPCSvH.js        200 text/html
LOCAL  /assets/pkg-i18n-ECmPCSvH.js        200 text/javascript

PUBLIC /assets/pkg-misc-DRGW1HPS.js        200 text/html
LOCAL  /assets/pkg-misc-DRGW1HPS.js        200 text/javascript

PUBLIC /assets/pkg-misc-DB0Q8XAf.css       200 text/html
LOCAL  /assets/pkg-misc-DB0Q8XAf.css       200 text/css
```

浏览器后台打开 `https://aaccx.pw/dashboard?debugts=browser-20260619-1` 时，`#app` 内容为空，页面正文为空，符合前端入口资源加载失败后 Vue 没挂载的表现。

### nginx 配置命中旧 workaround

`/opt/homebrew/etc/nginx/servers/aaccx-root.conf` 当前有：

```nginx
location ^~ /assets/pkg- {
    rewrite ^/assets/pkg-(.*)$ /assets/vendor-$1 break;
    proxy_pass http://127.0.0.1:18080;
    ...
}
```

`/opt/homebrew/etc/nginx/servers/cliproxy.conf` 也有同类规则，所以 `api.aaccx.pw` 同样受影响：

```text
https://api.aaccx.pw/assets/pkg-vue-DdvVI69T.js    200 text/html
https://api.aaccx.pw/assets/pkg-chart-DtGmxNIH.js  200 text/html
```

## 和之前问题的区别

之前 `vendor-*` 会被 Cloudflare 拦截，nginx 增加了 `vendor-* -> pkg-*` 对外绕过；当时的前提是 origin 运行产物仍叫 `vendor-*`。

现在 `frontend/vite.config.ts` 已经从源头把 `manualChunks()` 改成生成 `pkg-*`：

```text
pkg-vue
pkg-ui
pkg-chart
pkg-i18n
pkg-misc
```

因此旧 rewrite 已经过期。继续把 `pkg-*` 反向改回 `vendor-*` 会把真实存在的资源改成不存在的资源。

## 修复方向

后续修复应收敛 nginx workaround：

1. 移除或禁用 `location ^~ /assets/pkg-` 中的 `rewrite ^/assets/pkg-(.*)$ /assets/vendor-$1 break;`。
2. 保留 `/assets/app-index-* -> /assets/index-*` 的入口兼容规则，前提是仍需要规避旧入口缓存。
3. `/assets/` 普通代理仍可保留 `Cache-Control: no-store`。
4. 响应体中的 `vendor- -> pkg-` 也应评估是否可以删除；当前本地运行产物已不应再包含 `vendor-`。
5. 修改后同时验证 `aaccx.pw` 和 `api.aaccx.pw`：
   - `/dashboard` HTML 200。
   - `/assets/pkg-vue-*`、`/assets/pkg-i18n-*`、`/assets/pkg-misc-*`、`/assets/pkg-chart-*`、`/assets/pkg-ui-*` 均返回正确 JS/CSS MIME。
   - 浏览器打开 dashboard 后 `#app` 非空，无 chunk load error。

## 本次未做的事

本次只排查和记录，没有修改源码、没有修改 nginx 配置、没有重启服务。
