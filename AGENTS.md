# AI 协作记忆

## 三项目串联方案 A

- 2026-06-17：已决定采用方案 A：Sub2API 作为唯一公网 API 入口和唯一用户 Key / 计费 / 用量事实源；CLIProxyAPI 退到内网，仅作为本地订阅账号池、OAuth、协议转换和多账号轮询服务；yui.web/shop 第一阶段退为展示、说明和跳转入口。
- 详细设计记录见 `docs/ai/context/20260617-223355-sub2api-entry-cliproxy-yuiweb-scheme-a_CN.md`。
- 实施计划见 `docs/ai/context/20260617-223837-sub2api-entry-cliproxy-yuiweb-scheme-a-implementation-plan_CN.md`。
- 后续实施前先做最小链路验证：Sub2API 非 8080 端口启动，接入 `CLIProxyAPI` 的 `127.0.0.1:8317` 上游，创建测试用户和测试 Sub2API Key，确认请求、用量和扣费。
- 不要让 Sub2API、yui.web、CLIProxyAPI 同时对同一个用户 Key 做状态判定或扣费。
- 不要在文档或提交中记录完整 API Key、内部 token、HMAC secret。
- CLIProxyAPI 是聚合上游账号池，不是单个静态 OpenAI Key；作为 Sub2API 上游账号时必须启用 account `credentials.pool_mode=true`，并让 401/403/429 在同账号内重试，避免 CLIProxyAPI 内部某个账号 401 导致整个聚合上游被 Sub2API 永久禁用。
- 更新 Sub2API account credentials 时要带回 `base_url` 等非敏感字段；后端只会保留未提交的敏感字段，非敏感字段会被 incoming credentials map 覆盖。
- 2026-06-18：本机方案 A 最小链路已验证通过，记录见 `docs/ai/context/20260618-082743-sub2api-cliproxy-pool-mode-implementation_CN.md`。
- 2026-06-18：公网入口已从 CLIProxyAPI 切到 Sub2API；当前链路为 `Cloudflare Tunnel -> nginx 127.0.0.1:8080 -> Sub2API 127.0.0.1:18080 -> CLIProxyAPI 127.0.0.1:8317`。记录见 `docs/ai/context/20260618-084509-public-entry-cutover-to-sub2api_CN.md`。
- 2026-06-18：`api.aaccx.pw` 的 Sub2API 前端曾因 Cloudflare 误拦截 `/assets/vendor-*` 静态资源而白屏；nginx 已把公网路径改写为 `/assets/libs-*`，并用 `/assets/app-index-*` 绕过 Cloudflare 旧入口 JS 缓存。记录见 `docs/ai/context/20260618-085320-cloudflare-sub2api-white-screen-fix_CN.md`。
- 2026-06-18：为处理 Chrome 继续使用旧 `vendor-*` 入口缓存，`api.aaccx.pw` 的 nginx 响应已加入 `Clear-Site-Data: "cache"`；若用户侧仍报旧 vendor 403，先访问 `https://api.aaccx.pw/?cacheclear=1` 或强刷。记录见 `docs/ai/context/20260618-085730-sub2api-browser-cache-clear_CN.md`。
- 2026-06-18：`libs-*` 前缀仍可能保留二级 import 里的 `vendor-*`，并可能被 Cloudflare 缓存；当前最终公网资源前缀改为 `/assets/pkg-*`，且 `/assets/pkg-*` 响应也做 `vendor- -> pkg-` 内容改写。记录见 `docs/ai/context/20260618-090246-sub2api-pkg-asset-rewrite_CN.md`。
- 2026-06-18：`Clear-Site-Data` 不应加到 `/assets/*`，否则动态 chunk 加载时可能反复清缓存；当前 nginx 只在 `/` 和 `/index.html` 加 `Clear-Site-Data: "cache"`，所有 `/assets/*` 统一 `Cache-Control: no-store`，并把 chunk 内 `./index-*` 改写为 `./app-index-*`，避免旧入口 JS 缓存继续请求 `vendor-*`。记录见 `docs/ai/context/20260618-091046-sub2api-public-asset-cache-fix-result_CN.md`。
- 2026-06-18：公网仍白屏后，判断前面的 nginx `sub_filter` 是不稳定 workaround；正确方向是先建立本地 `127.0.0.1:18080` 完整验收基准，并从 `frontend/vite.config.ts` 源头把 `manualChunks()` 的 `vendor-*` 命名改为 `pkg-*`，重新构建嵌入式前端后再回切公网。设计见 `docs/ai/context/20260618-091756-sub2api-local-runthrough-design_CN.md`。
- 2026-06-18：本地已按最新设计从源头修复 Vite chunk 命名，`manualChunks()` 改为生成 `pkg-*`，并重建 Docker 镜像后在 `127.0.0.1:18080` 完整跑通前端、浏览器、models、chat、responses 和用量记录；Docker 构建还需复制 `docs/legal/*.md`，否则前端 raw import 会失败。记录见 `docs/ai/context/20260618-093659-sub2api-local-runthrough-source-pkg-chunks_CN.md`。
- 2026-06-18：已确认 `yui.web/shop` 真实库 `data/shop.sqlite` 有 21 个用户、20 个有密码、12 个 active 订阅；yui 密码为 scrypt，Sub2API 密码为 bcrypt，不能直接复制 hash 无感登录。Sub2API 当前注册关闭是因为 `registration_enabled` 设置缺失，代码安全默认关闭。不要实时把 Sub2API 登录接到 yui.web SQLite，推荐一次性迁移用户和权益到 Sub2API。记录见 `docs/ai/context/20260618-095632-yuiweb-users-sub2api-registration-auth-migration_CN.md`。
- 2026-06-18：用户决定旧 API Key 迁移范围按 yui.web `orders` 中 15 个已有 API Key 的用户全部导入 Sub2API，而不是只迁 12 个 active subscription 用户；其中 3 个无 active subscription 的用户需要用 `orders.expires_at` 做人工迁移订阅或单独复核。旧 Key 可用 yui.web 加密字段解密后写入 Sub2API `api_keys.key`，但必须同时补 `user_subscriptions`、group 绑定和当日用量，不能只复制 Key。设计见 `docs/ai/context/20260618-103811-yuiweb-legacy-api-key-import-design_CN.md`。
- 2026-06-18：已将 yui.web `orders` 中 15 个已发 API Key 导入 Sub2API，全部绑定到 `codex-pool`；12 个复用已迁 active subscription，3 个无 active subscription 的旧 Key 已按订单过期时间补人工迁移订阅。迁移脚本见 `scripts/migrate-yuiweb-legacy-api-keys.mjs`，结果见 `docs/ai/context/20260618-105355-yuiweb-legacy-api-key-import-result_CN.md`。
- 2026-06-18：旧 Key 本地验证通过：`/v1/models` 返回 200，最小 `/v1/chat/completions` 返回 200，并新增 `usage_logs`；当前运行代码未写入 `billing_usage_entries`，订阅扣费用量事实体现在 `usage_logs.subscription_id` 与 `user_subscriptions.*_usage_usd` 增量中。
