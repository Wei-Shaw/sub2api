# Sub2API 压缩记忆

记录时间：2026-06-19 15:19:20
来源：根目录 `AGENTS.md` 流水记忆压缩整理。
原则：根目录只保留高频约束；完整长期上下文统一放在 `docs/ai/context/`；不记录完整 API Key、内部 token、HMAC secret。

## 1. 架构定论

- 已采用“三项目串联方案 A”：
  - Sub2API 是唯一公网 API 入口。
  - Sub2API 是唯一用户 Key、计费和用量事实源。
  - CLIProxyAPI 退到内网，只负责本地订阅账号池、OAuth、协议转换和多账号轮询。
  - yui.web/shop 第一阶段只保留展示、说明和跳转，不再负责 API Key 状态判定或扣费。
- 不允许 Sub2API、yui.web、CLIProxyAPI 同时对同一个用户 Key 做状态判定或扣费。
- 当前主链路：
  - `Cloudflare Tunnel -> nginx 127.0.0.1:8080 -> Sub2API 127.0.0.1:18080 -> CLIProxyAPI 127.0.0.1:8317`
- 关键设计文档：
  - `docs/ai/context/20260617-223355-sub2api-entry-cliproxy-yuiweb-scheme-a_CN.md`
  - `docs/ai/context/20260617-223837-sub2api-entry-cliproxy-yuiweb-scheme-a-implementation-plan_CN.md`
  - `docs/ai/context/20260618-082743-sub2api-cliproxy-pool-mode-implementation_CN.md`
  - `docs/ai/context/20260618-084509-public-entry-cutover-to-sub2api_CN.md`

## 2. 上游账号池和凭据规则

- CLIProxyAPI 是聚合上游账号池，不是单个静态 OpenAI Key。
- CLIProxyAPI 作为 Sub2API 上游账号时必须启用 `account.credentials.pool_mode=true`。
- 401/403/429 应在同账号池内重试，避免 CLIProxyAPI 内部某个账号异常导致整个聚合上游被 Sub2API 永久禁用。
- 更新 Sub2API account credentials 时要带回 `base_url` 等非敏感字段；后端只会保留未提交的敏感字段，非敏感字段会被 incoming credentials map 覆盖。
- `cliproxy-local-openai` 已绑定到 `codex-pool-19-usd`、`codex-pool-29-usd`、`codex-pool-49-usd`，否则新 Key 的 `/v1/chat/completions` 会因无上游账号返回 503。
- 相关记录：
  - `docs/ai/context/20260619-094444-new-api-key-connectivity-test-result_CN.md`

## 3. 套餐、Key 和用户迁移

- 套餐分组显示名：
  - `codex-pool-19-usd`：每日 19 USD，承接 29 元套餐入口分组。
  - `codex-pool-29-usd`：每日 29 USD。
  - `codex-pool-49-usd`：每日 49 USD，承接 59 元套餐。
  - `codex-pool-local-unlimited`：本机自用无限额分组，`daily_limit_usd/weekly_limit_usd/monthly_limit_usd` 均为 `NULL`。
- 分组重命名只改显示名称和迁移脚本映射，不改 group id、订阅、API Key 绑定或限额。
- yui.web/shop 真实库曾确认有 21 个用户、20 个有密码、12 个 active 订阅；yui 密码为 scrypt，Sub2API 密码为 bcrypt，不能直接复制 hash 实现无感登录。
- Sub2API 注册关闭的直接原因是 `registration_enabled` 设置缺失，代码安全默认关闭。
- 不要实时把 Sub2API 登录接到 yui.web SQLite；推荐一次性迁移用户和权益到 Sub2API。
- yui.web `orders` 中 15 个已有 API Key 的用户已导入 Sub2API，而不是只迁 12 个 active subscription 用户。
- 其中 12 个复用已迁 active subscription，3 个无 active subscription 的旧 Key 已按 `orders.expires_at` 补人工迁移订阅。
- 旧 Key 迁移必须同时补 `user_subscriptions`、group 绑定和当日用量，不能只复制 Key。
- 旧 Key 本地和公网验证通过，使用事实体现在 `usage_logs.subscription_id` 与 `user_subscriptions.*_usage_usd` 增量；当前运行代码未写入 `billing_usage_entries`。
- 29 元订阅池 `codex-pool-19-usd` 的 `daily_limit_usd=19` 会在 API Key 中间件通过 `ValidateAndCheckLimits()` 真实校验。正式迁移用户近 24 小时失败主要是 `401 INVALID_API_KEY/API_KEY_REQUIRED` 和少量上游 `429/502`，不是 `USAGE_LIMIT_EXCEEDED`。
- 相关记录：
  - `docs/ai/context/20260618-095632-yuiweb-users-sub2api-registration-auth-migration_CN.md`
  - `docs/ai/context/20260618-103811-yuiweb-legacy-api-key-import-design_CN.md`
  - `docs/ai/context/20260618-105355-yuiweb-legacy-api-key-import-result_CN.md`
  - `scripts/migrate-yuiweb-legacy-api-keys.mjs`
  - `docs/ai/context/20260618-125150-codex-pool-29-plan-public-key-mvp-diagnosis_CN.md`

## 4. 管理员、本机 Key 和特殊用户

- 手机号 `15951875192` 是本机自用 Key 账号，不属于 yui.web `orders` 迁移范围。
- `15951875192@phone.com` 已恢复并设为 Sub2API 管理员账号；其本机自用 Key 已从删除审计恢复，继续绑定 `codex-pool-local-unlimited`。
- 该账号是管理员和本机 Codex Local Key 所属用户，后续不要按普通用户删除。
- 如需在普通用户视图隐藏该账号，应做角色筛选或备注标识，而不是删除。
- 指定 59 元套餐用户已创建并绑定 `codex-pool-49-usd`；登录、公网 `/v1/models`、最小 `/v1/chat/completions` 和用量写入已验证。
- 截图中的测试用户 `sub2api-test-local@example.com` 已软删除，其原 Key 已写入删除审计、软删除并清理 Redis 认证缓存；本地和公网请求均返回 `401 INVALID_API_KEY`。
- 相关记录：
  - `docs/ai/context/20260618-111935-local-key-15951875192-unlimited-result_CN.md`
  - `docs/ai/context/20260618-125900-restore-15951875192-admin-local-key-result_CN.md`
  - `docs/ai/context/20260618-193328-delete-sub2api-test-local-user-result_CN.md`
  - `docs/ai/context/20260619-114313-add-tongji-lishouqi-user-59-plan-result_CN.md`

## 5. 公网路由和前端资源

- `aaccx.pw/shop` 保留 yui.web，入口按钮跳转 `https://aaccx.pw/dashboard`。
- `aaccx.pw/v1/*`、`/api/*` 和 Sub2API 控制台路由由 nginx 代理到 `127.0.0.1:18080`。
- `api.aaccx.pw` 也是 Sub2API 入口。
- `/shop/assets/*` 仍归 yui.web。
- `aaccx.pw/admin/redeem` 刷新 404 的根因曾是 nginx 只代理部分控制台路由，漏掉 `/admin` 前缀；修复时只应补 Sub2API 控制台路由，不要把全部未知路径转给 Sub2API。
- `api.aaccx.pw` 和 `aaccx.pw/dashboard` 曾因 Cloudflare 拦截 `/assets/vendor-*`、浏览器旧缓存和 nginx `sub_filter` workaround 反复白屏。
- 最终正确方向是从 Vite 源头把 `manualChunks()` 命名改为 `pkg-*`，重新构建嵌入式前端；公网入口用 `app-index-*` 规避旧入口缓存。
- 不要再把真实存在的 `/assets/pkg-*` 反向 rewrite 到已不存在的 `/assets/vendor-*`，否则后端 SPA fallback 会返回 HTML 并触发 JS/CSS MIME 错误。
- 当前资产策略：
  - 保留 `app-index-* -> index-*` 入口兼容。
  - 移除 `pkg-* -> vendor-*` 反向 rewrite。
  - `/assets/*` 应避免反复 `Clear-Site-Data`；静态资源更适合 `Cache-Control: no-store`。
- Docker 构建需要复制 `docs/legal/*.md`，否则前端 raw import 会失败。
- 相关记录：
  - `docs/ai/context/20260618-085320-cloudflare-sub2api-white-screen-fix_CN.md`
  - `docs/ai/context/20260618-091756-sub2api-local-runthrough-design_CN.md`
  - `docs/ai/context/20260618-093659-sub2api-local-runthrough-source-pkg-chunks_CN.md`
  - `docs/ai/context/20260618-114829-aaccx-pw-sub2api-public-route-result_CN.md`
  - `docs/ai/context/20260618-194718-aaccx-admin-spa-refresh-404-diagnosis_CN.md`
  - `docs/ai/context/20260619-113614-aaccx-dashboard-white-screen-vendor-asset-fix_CN.md`
  - `docs/ai/context/20260619-130224-public-dashboard-white-screen-pkg-rewrite-diagnosis_CN.md`
  - `docs/ai/context/20260619-131328-aaccx-dashboard-pkg-mime-fix-result_CN.md`

## 6. 支付、兑换码和邮件

- Sub2API 自带内置支付和套餐页：
  - 用户付款入口：`/purchase`
  - 订单页：`/orders`
  - 模型/定价表：`/available-channels`
- 运行态已打开 `payment_enabled=true` 和 `available_channels_enabled=true`，`/purchase`、`/orders`、`/subscriptions` 公网可返回 200。
- 支付服务商实例仍未配置，页面可展示但真实支付方式可能为空。
- `/purchase` 在未配置支付方式时已接入手动收款码弹窗，展示微信/支付宝个人收款码并引导用户去 `/redeem` 输入兑换码。
- 手动收款码路径不创建 payment order、不自动开通订阅、不写账单或用量；权益仍以管理员确认到账后发放的兑换码为准。
- 当前项目支持通过 SMTP 发注册验证码、邮箱绑定/通知邮箱/TOTP 验证码，以及忘记密码邮件。
- 忘记密码当前实现是邮件重置链接 token，不是用户手输验证码。
- 当前运行态未配置 SMTP，`email_verify_enabled=false`、`password_reset_enabled=false`，因此暂不能给真实邮箱发送验证码或重置密码邮件。
- 相关记录：
  - `docs/ai/context/20260618-203016-user-payment-pricing-model-pages-diagnosis_CN.md`
  - `docs/ai/context/20260618-204818-enable-payment-pages-and-routes-result_CN.md`
  - `docs/ai/context/20260619-093511-manual-qr-payment-implementation-result_CN.md`
  - `docs/ai/context/20260619-093831-email-smtp-password-reset-current-state_CN.md`

## 7. 已修复或已验证事项

- 本机自用 Key 已验证 `127.0.0.1:18080`、`https://aaccx.pw`、`https://api.aaccx.pw` 的 `/v1/models` 和最小 `/v1/chat/completions`，模型列表包含 `gpt-5.4`，最小回复为 `pong`。
- 某正式旧 Key 曾验证 `https://aaccx.pw/v1/models`、最小 `https://aaccx.pw/v1/chat/completions` 和 `https://api.aaccx.pw/v1/models` 均可用。
- 测试用户在 `/keys` 页面连续新建 3 个 API Key 时，`/v1/models` 均 200；初始 chat 503 的根因是 `codex-pool-49-usd` 没有上游绑定，已修复。
- 管理员 `/admin/users` 页“用户名”列空值已修复：
  - 历史 active 用户空 `username` 已回填为 email。
  - 前端显示兜底为 `username || email || '-'`。
  - 邮箱注册和管理员创建用户时空用户名默认写入 email。
  - 已重建并重启 `sub2api` 容器，健康检查通过。
- 相关记录：
  - `docs/ai/context/20260618-115936-15776812883-public-api-key-connectivity-result_CN.md`
  - `docs/ai/context/20260618-192934-api-connectivity-model-access-result_CN.md`
  - `docs/ai/context/20260619-094444-new-api-key-connectivity-test-result_CN.md`
  - `docs/ai/context/20260619-115857-admin-users-username-email-fallback-result_CN.md`

## 8. yui.web 旧业务退役原则

- yui.web 旧邀请码和旧 API Key 发放业务不要立即物理删除。
- 推荐节奏：
  - 短期锁死写路径并返回 410。
  - 中期清理 UI 和测试。
  - 长期观察后删除旧写代码，保留历史只读和薄 410 route。
- 相关记录：
  - `docs/ai/context/20260618-120001-yuiweb-legacy-key-business-retirement-design_CN.md`

## 9. 后续操作禁忌

- 不要绕过 Sub2API 去让 yui.web 或 CLIProxyAPI 判定用户 Key 状态、扣费或用量。
- 不要把完整密钥、内部 token 或 HMAC secret 写入文档、提交、日志或截图描述。
- 不要把 CLIProxyAPI 当作单个静态 OpenAI Key。
- 不要删除 `15951875192@phone.com` 或其本机自用 Key。
- 不要为解决公网白屏重新引入 `pkg-* -> vendor-*` 反向 rewrite。
- 不要只复制旧 Key 而不补订阅、分组绑定和用量事实。
- 不要把全部未知公网路径都转给 Sub2API；应按控制台路由明确代理，避免吞掉 yui.web/shop。
