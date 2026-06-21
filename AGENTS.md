# AI 协作记忆

> 压缩记忆全文见 `docs/ai/context/20260619-151920-sub2api-compressed-memory_CN.md`。
> 后续新增长期上下文统一新建到 `docs/ai/context/YYYYMMDD-HHMMSS-*.md`，不要覆写、重命名或删除历史文档。

## 核心定论

- 采用“三项目串联方案 A”：Sub2API 是唯一公网 API 入口，也是唯一用户 Key、计费和用量事实源；CLIProxyAPI 只作为内网账号池、OAuth、协议转换和轮询上游；yui.web/shop 只保留展示、说明和跳转。
- 当前主链路：`Cloudflare Tunnel -> nginx 127.0.0.1:8080 -> Sub2API 127.0.0.1:18080 -> CLIProxyAPI 127.0.0.1:8317`。
- 不要让 Sub2API、yui.web、CLIProxyAPI 同时对同一个用户 Key 做状态判定或扣费。
- 不要在文档、提交或日志中记录完整 API Key、内部 token、HMAC secret。
- 前端 UI 重设计采用 `yui.web` 作品集黑白灰风格，覆盖用户页和管理后台；默认品牌名为「天才程序员小站」，保留 `siteName/siteLogo` 后台配置优先逻辑；信息架构、后端、API、路由和计费逻辑不改。设计稿见 `docs/ai/context/20260620-214101-sub2api-yuiweb-black-white-ui-redesign-design_CN.md`。

## 运行态摘要

- `aaccx.pw/shop` 归 yui.web；`aaccx.pw/v1/*`、`/api/*` 和 Sub2API 控制台路由归 Sub2API；`api.aaccx.pw` 也是 Sub2API 入口。
- CLIProxyAPI 是聚合上游，不是单个静态 OpenAI Key；作为 Sub2API 上游账号时必须启用 `credentials.pool_mode=true`，并让 401/403/429 在同账号内重试。
- 套餐分组显示名为 `codex-pool-19-usd`、`codex-pool-29-usd`、`codex-pool-49-usd`，分别对应每日 19/29/49 USD；`codex-pool-local-unlimited` 是本机自用无限额分组。
- yui.web 旧 Key 已按 `orders` 迁入 Sub2API；旧邀请码和旧 API Key 发放业务应退役为 410/只读历史，不要继续写入。
- `xiaobianfuai@gmail.com` 是管理员和本机 Codex Local Key 所属账号（原 `15951875192@phone.com` 已改名），不要按普通用户删除；如需隐藏，用角色筛选或备注标识。

## 易踩坑

- 前端 chunk 命名已从源头改为 `pkg-*`；不要再把真实 `/assets/pkg-*` 反向 rewrite 到 `/assets/vendor-*`。公网入口兼容只保留 `app-index-* -> index-*` 方向。
- Docker 构建需要复制 `docs/legal/*.md`，否则前端 raw import 会失败。
- 更新 Sub2API account credentials 时要带回 `base_url` 等非敏感字段；后端只保留未提交的敏感字段，非敏感字段会被 incoming credentials map 覆盖。
- 当前 SMTP 已配置为 Gmail 发信，发件邮箱 `xiaobianfuai@gmail.com`，`email_verify_enabled=true`、`password_reset_enabled=true`；忘记密码实现是邮件重置链接 token，不是用户手输验证码。不要在文档或提交中记录 Gmail 应用专用密码；该密码曾在对话中出现，后续建议重新生成并替换。
- `/purchase` 在未配置支付服务商时展示手动收款码，引导用户去 `/redeem`；该路径不创建支付订单、不自动开通订阅、不写账单或用量。
- 当前售卖套餐里 `29 元订阅池` 对应 `subscription_plans.id=1 -> group_id=2 -> codex-pool-19-usd`；`codex-pool-29-usd` 实际对应 `39 元订阅池`，不要按分组名误绑 29 元/月用户。

## 运行记录

- 2026-06-19：`18405650929@phone.com` 用户存在且状态 active，已有 active API Key（掩码 `sk-yui-l...OQjSJH`），绑定 `codex-pool-19-usd`，订阅 active 且到期时间 `2026-07-17 16:06:37.531+08`；使用该 Key 访问 `https://aaccx.pw/v1/models` 和 `https://api.aaccx.pw/v1/models` 均返回 200，模型列表包含 `gpt-5.5`、`gpt-5.4` 等 10 个模型。结果见 `docs/ai/context/20260619-152007-18405650929-api-key-public-models-result_CN.md`。
- 2026-06-19：已为当前运行态配置 Gmail SMTP，发件邮箱为 `xiaobianfuai@gmail.com`，`frontend_url=https://aaccx.pw`，并开启 `email_verify_enabled=true`、`password_reset_enabled=true`；SMTP 登录验证成功，`/api/v1/auth/forgot-password` 已对该邮箱入队并由 worker 发送密码重置邮件。结果见 `docs/ai/context/20260619-220357-gmail-smtp-enabled-result_CN.md`。
- 2026-06-20：邮件重置链接 `https://aaccx.pw/reset-password?...` 404 的根因是 `aaccx.pw` nginx 只代理部分 Sub2API SPA 路由，漏掉 `reset-password`、`forgot-password`、`email-verify` 等认证入口；已补明确白名单并 reload nginx，公网三条路径均返回 200，`/shop` 仍归 yui.web。截图里暴露的旧 Gmail reset token 已从 Redis 删除，并重新发送新的密码重置邮件。结果见 `docs/ai/context/20260620-100611-aaccx-reset-password-route-404-fix-result_CN.md`。
- 2026-06-20：已将手机号迁移用户 `13052071067@phone.com` 的身份字段更新为真实邮箱 `milesyang987@gmail.com`，同步修改 `users.email`、`users.username` 和 `auth_identities.provider_subject`；用户 id、API Key、订阅、余额、用量、密码 hash 均未改动，并已清理该用户 API Key auth cache。结果见 `docs/ai/context/20260620-110315-phone-user-real-email-update-result_CN.md`。
- 2026-06-20：已批量将 6 个手机号迁移用户从 `<手机号>@phone.com` 更新为真实邮箱：`18367290091 -> changjunwang123@gmail.com`、`13052071067 -> milesyang987@gmail.com`、`19520434236 -> xunskyler@gmail.com`、`18405650929 -> xwh1124wcw@163.com`、`13584052801 -> 897858381@qq.com`、`15995436627 -> 15995436627@163.com`；同步修改 `users.email`、`users.username`、`auth_identities.provider_subject`，API Key/订阅/余额/用量/密码 hash 均未改动，并清理这些用户的 API Key auth cache。结果见 `docs/ai/context/20260620-110907-batch-phone-users-real-email-update-result_CN.md`。
- 2026-06-20：已将手机号迁移用户 `15062376174@phone.com` 的身份字段更新为真实邮箱 `313398924@qq.com`，同步修改 `users.email`、`users.username` 和 `auth_identities.provider_subject`；用户 id、API Key、订阅、余额、用量、密码 hash 均未改动，并清理该用户 API Key auth cache。结果见 `docs/ai/context/20260620-111205-phone-user-15062376174-real-email-update-result_CN.md`。
- 2026-06-20：已将手机号迁移用户 `15776812883@phone.com` 的身份字段更新为真实邮箱 `liyutong2883@gmail.com`，同步修改 `users.email`、`users.username` 和 `auth_identities.provider_subject`；用户 id、API Key、订阅、余额、用量、密码 hash 均未改动，并清理该用户 API Key auth cache。结果见 `docs/ai/context/20260620-111404-phone-user-15776812883-real-email-update-result_CN.md`。
- 2026-06-20：`18014503779 -> 1915474749@qq.com` 迁移时发现真实邮箱已存在空壳账号 `users.id=25`；已将其 email identity、user affiliate 和 user platform quota 合并到有权益主账号 `users.id=16`，软删除 `users.id=25`，并把 `users.id=16` 的 `email/username` 更新为 `1915474749@qq.com`；主账号 API Key/订阅/余额/用量/密码 hash 未改动，并清理主账号 API Key auth cache。结果见 `docs/ai/context/20260620-112250-phone-user-18014503779-real-email-merge-result_CN.md`。
- 2026-06-20：已将手机号迁移用户 `17371571728@phone.com` 的身份字段更新为真实邮箱 `2246950894@qq.com`，同步修改 `users.email`、`users.username` 和 `auth_identities.provider_subject`；用户 id、API Key、订阅、余额、用量、密码 hash 均未改动，并清理该用户 API Key auth cache。结果见 `docs/ai/context/20260620-194159-phone-user-17371571728-real-email-update-result_CN.md`。
- 2026-06-20：已直接创建用户 `2316095427@qq.com`，按用户指定初始密码设置 bcrypt 登录密码，绑定 29 元/月套餐 `subscription_plans.id=1` / `codex-pool-19-usd` / `group_id=2`，创建默认 API Key（掩码 `sk-bf709...e7c6fd`）和 4 条默认 `user_platform_quotas`；登录、`/v1/models` 和 `/v1/chat/completions` 公网验证均返回 200。结果见 `docs/ai/context/20260620-180728-add-2316095427-user-29-result_CN.md`。
- 2026-06-20：已将管理员账号 `users.id=13` 从 `15951875192@phone.com` 改为 `xiaobianfuai@gmail.com`，仍保持 `admin`；管理员本机无限额 API Key（掩码 `sk-LOCAL...e28804`）未改动。已软删除占用该邮箱的普通测试账号 `users.id=26`，删除其邮箱 identity，软删除测试 API Key 并写入 `deleted_api_key_audits`，同时清理测试 Key auth cache 和测试账号 refresh token 集合。结果见 `docs/ai/context/20260620-195106-admin-15951875192-renamed-xiaobianfuai-test-user-deleted-result_CN.md`。
- 2026-06-20：已完成 Sub2API 前端 yui.web 黑白灰视觉重设计，默认品牌呈现为「天才程序员小站」，覆盖用户页和管理后台；后端、API、路由、计费逻辑未修改。结果见 `docs/ai/context/20260620-214423-sub2api-yuiweb-black-white-ui-redesign-result_CN.md`。
- 2026-06-21：已修正默认首页展示：未登录态只保留 Hero 区「立即登录」入口，右上角匿名登录按钮移除；中部三张卡片改为 29/39/59 元套餐及每日 24 点刷新说明；支持模型只展示 GPT 5.3、Codex 5.4、GPT 5.5。结果见 `docs/ai/context/20260621-101724-home-login-plans-models-result_CN.md`。
- 2026-06-21：已调整 `/purchase` 前端展示：打开页面默认展示订阅页签，页签顺序为订阅在左、充值在右；支付接口、订单、订阅和计费逻辑未修改。结果见 `docs/ai/context/20260621-110615-purchase-default-subscription-tab-result_CN.md`。
- 2026-06-21：已精简购买页订阅卡片和首页套餐卡片文案：带日限额月度套餐右侧价格改为 `¥xx元`，描述统一为「月度订阅-时间 30天，日限额 x刀，24点刷新」，并移除重复的美元价格、周期、倍率、模型 scope 和 features 展示；后端、支付和计费逻辑未修改。结果见 `docs/ai/context/20260621-111028-subscription-card-copy-price-result_CN.md`。
- 2026-06-21：已在 nginx 公网分流层下线 yui.web 旧 Shop 路由 `/shop/login/`、`/shop/register/`、`/shop/reset-password/`、`/shop/account/`、`/shop/admin/`、`/shop/redeem/`、`/shop/key/`、`/shop/query/`、`/shop/order/`、`/shop/pay/`、`/shop/result/`、`/shop/content/`，公网返回 410；`/shop/` 和 `/shop/guide/` 保留 200，Sub2API `/dashboard`、`/v1/*` 未受影响。结果见 `docs/ai/context/20260621-120456-yuiweb-public-shop-legacy-routes-retired-result_CN.md`。
- 2026-06-21：已将手机号迁移用户 `19814722044@phone.com` 的身份字段更新为真实邮箱 `varmons@proton.me`，同步修改 `users.email`、`users.username` 和 `auth_identities.provider_subject`；用户 id、API Key、订阅、余额、用量、密码 hash、角色和状态均未改动，并已广播该用户 API Key auth cache 失效。结果见 `docs/ai/context/20260621-121301-phone-user-19814722044-real-email-update-result_CN.md`。
- 2026-06-21：已在普通用户面板新增「使用方法」页面 `/usage-guide`，左侧导航仅普通用户可见，管理员页面和管理员侧栏不新增入口；页面只展示 8 个步骤和用户提供的 10 张截图，后端、支付、订阅、兑换码、API Key、计费和公网配置未修改。结果见 `docs/ai/context/20260621-160506-user-usage-guide-result_CN.md`。
- 2026-06-21：已调查当前生图能力与计费：代码支持 `/v1/images/generations`、`/v1/images/edits` 和 `/v1/responses` 的 `image_generation` 工具，但当前三个在售订阅分组 `codex-pool-19-usd`、`codex-pool-29-usd`、`codex-pool-49-usd` 的 `allow_image_generation=false`，因此当前没有在售档位可用生图；若启用则按图片张数、尺寸和倍率消耗订阅日额度。结果见 `docs/ai/context/20260621-155950-image-generation-access-billing-current-state_CN.md`。
- 2026-06-21：已将手机号迁移用户 `15706598243@phone.com` 的身份字段更新为真实邮箱 `15706598243@163.com`，同步修改 `users.email`、`users.username` 和 `auth_identities.provider_subject`；用户 id、API Key、订阅、余额、用量、密码 hash、角色和状态均未改动，并已广播该用户 API Key auth cache 失效。结果见 `docs/ai/context/20260621-155951-phone-user-15706598243-real-email-update-result_CN.md`。
- 2026-06-21：已移除 CLIProxyAPI 运行配置中错误的 `gpt-image-2 -> gemini-3.1-flash-image` Antigravity 别名映射；使用 CLIProxyAPI 当前 API Key 直连 `http://127.0.0.1:8317/v1/images/generations` 真实生图返回 200，得到 1 张 PNG，并视觉确认结果有效。本次未修改 Sub2API 套餐生图开关、价格或计费逻辑。结果见 `docs/ai/context/20260621-161803-cliproxyapi-gpt-image-2-native-image-test-result_CN.md`。
- 2026-06-21：已将当前工作区使用教程前端改动整理到分支 `codex/usage-guide-review-merge-20260621` 并合并回本地 `main`；review 后遮挡教程截图中的浏览器个人信息和用户邮箱，补充 `.gitignore` 防止本地 dump/sqlite/env 进入仓库；验证包含 UsageGuide/AppSidebar 测试、前端 typecheck 和 build。结果见 `docs/ai/context/20260621-161806-usage-guide-branch-review-merge-result_CN.md`。
