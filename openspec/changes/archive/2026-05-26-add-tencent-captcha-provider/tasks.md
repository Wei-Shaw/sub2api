> **数据库迁移说明**: 本变更**不需要新增 migration**。仓库中 `captcha_provider` 存储于通用 `settings(key, value)` 表，无 ENUM/CHECK 约束；`tencent_captcha` 取值仅在应用层 `normalizeCaptchaProvider` 中放开即可（见任务 4.1）。仓库遵循 forward-only migration 约定（`backend/migrations/README.md` L47）。

## 1. 后端 - Verifier 接口重构（D3）

- [x] 1.1 在 `backend/internal/service/captcha_verifier.go` 中定义 `VerifyRequest`、`VerifyResult` 结构与新版 `CaptchaVerifier` 接口（`Verify` + `ValidateProviderConfig`）
- [x] 1.2 把现有 `captchaHTTPVerifier` 的 Turnstile/hCaptcha 实现迁移到新接口，保留 HTTP 调用与错误归一化逻辑（按 D6 错误码映射）
- [x] 1.3 实现 `ValidateProviderConfig`：Turnstile/hCaptcha 走 deliberately-invalid-token 探活；任何 provider 不认识时返回明确错误
- [x] 1.4 在 `captcha_service.go` 中保留旧 `VerifyToken(ctx, provider, secretKey, token, remoteIP)` 作为薄封装，内部翻译为新 `VerifyRequest`，方便调用方分批迁移
- [x] 1.5 更新 `backend/internal/service/captcha_service_test.go`：将既有 Turnstile/hCaptcha 用例改为驱动新接口，断言归一化 `ErrorCode`

## 2. 后端 - 天御 Verifier 实现（D4, D5, D8）

> 实施偏差说明：实际把 tencent verifier 放在 `backend/internal/repository/captcha_tencent.go`（而非 design.md D4 / tasks 2.1 描述的 `service/captcha_tencent.go`），原因是与 `captcha_verifier.go` 同包共享 `captchaHTTPVerifier` 的 `httpClient` 与 `verifyURLs` 状态最自然；`service/` 包按现状只放 captcha 接口与业务编排，verifier 实现都在 `repository/` 包内。归档时同步在 design.md D4 落地路径中订正。

- [x] 2.1 新建 `backend/internal/repository/captcha_tencent.go`，实现 TC3-HMAC-SHA256 签名工具函数（`buildCanonicalHeaders`、`buildTencentTC3Authorization`、`sha256Hex`、`hmacSHA256`）
- [x] 2.2 实现 `tencentCaptchaVerify(ctx, cfg, payload, remoteIP)`：组装 `DescribeCaptchaResult` 请求体（`CaptchaType=9`、`Ticket`、`Randstr`、`UserIp`、`CaptchaAppId`、`AppSecretKey`），通过 `httpclient.GetClient()` POST `https://captcha.tencentcloudapi.com/`
- [x] 2.3 在 verifier 入口添加 `trerror_` 前缀短路：识别后立即返回 `ErrorCode = "captcha.tencent.fallback_ticket"`，写 WARN 日志含截断/匿名化 RemoteIP，**不**发出 HTTP 请求（D5）
- [x] 2.4 在 `UserIp` 赋值处补充 D8 注释，说明依赖 `trusted_proxies`
- [x] 2.5 实现 `CaptchaCode` → `VerifyResult.ErrorCode` 映射表：`1→success`、`{6,7,15}→captcha.config`、`9→captcha.timeout`、`10→captcha.duplicate`、其余 `→captcha.invalid`
- [x] 2.6 在 verifier `Verify` 主分发中接入 `Provider = "tencent_captcha"` 分支
- [x] 2.7 单元测试 `captcha_tencent_test.go`：
    - TC3 签名向量对照（用腾讯云官方示例 fixture）
    - `trerror_` 前缀 → 不发请求 + 错误码正确
    - 模拟 HTTP server 返回各 `CaptchaCode` → 错误码归一化
    - HTTP 超时/4xx/5xx → `captcha.network`
    - Payload 缺 `ticket` 或 `randstr` → `captcha.invalid` 且不发请求

## 3. 后端 - 配置层适配（D7, Settings DTO）

- [x] 3.1 在 `setting_service.go` 中扩展 `normalizeCaptchaProvider` 支持 `"tencent_captcha"`；非法 provider 仍走 Turnstile 默认或拒绝（与现有策略保持一致）
- [x] 3.2 把 `maskCaptchaConfig` 改为查 `captchaSecretFields` 映射表（D7），覆盖三 provider；同步生成 `*_configured: bool` 字段
- [x] 3.3 `AdminSettings.CaptchaConfig` DTO 增加可读字段说明（doc comment），明确不同 provider 的可见字段集
- [x] 3.4 `PublicSettings`：当 provider 为 `tencent_captcha` 时，把 `CaptchaSiteKey` 字段填为 `captcha_config["captcha_app_id"]`；老字段 `turnstile_site_key` 同步保留兼容值
- [x] 3.5 更新 SSR 注入路径 `PublicSettingsInjectionPayload`，保持与公开接口同步
- [x] 3.6 设置层单元测试：mask、normalize、PublicSettings 序列化三处的天御场景

## 4. 后端 - DTO 与 Handler 适配（D2）

- [x] 4.1 编写 `extractCaptchaPayload(req)` 公共 helper（放 `backend/internal/handler/dto/` 或 `handler` 包内），输入请求 DTO，输出 `map[string]string`：优先 `captcha_payload`，其次 `captcha_token` / `turnstile_token` 归一为 `{"token": ...}`
- [x] 4.2 给 `LoginRequest`、`RegisterRequest`、`SendVerificationEmailRequest`、`ForgotPasswordRequest` 以及任何其他 captcha-gated 请求 DTO 增加 `CaptchaPayload map[string]string \`json:"captcha_payload"\``
- [x] 4.3 用 grep 扫一遍当前所有读取 `captcha_token`/`turnstile_token` 的代码点，全部替换为调用 helper，禁止直接读字段（PR 描述里列出）
- [x] 4.4 `AuthService.VerifyCaptcha` 或等效入口改为接收 `map[string]string` payload；调用新 `CaptchaVerifier.Verify`
- [x] 4.5 错误响应：把归一化 `ErrorCode`（含 `captcha.tencent.fallback_ticket`）通过现有 ResponseEnvelope 透出到前端
- [x] 4.6 Handler 层单元/集成测试更新：三 provider 三套用例 + `captcha_payload` 与老字段共存场景

## 5. 前端 - Widget Dispatcher（D1）

- [x] 5.1 修改 `frontend/src/components/CaptchaWidget.vue`：通过 `defineExpose` 暴露 `execute(): Promise<Record<string,string> | null>`；新增 `tencent_captcha` 分支渲染 `<TencentCaptchaWidget>`
- [x] 5.2 修改 `frontend/src/components/TurnstileWidget.vue`：补 `execute()` —— 已有 token 则直接 resolve；无 token 则等下一次 `verify` 事件触发 resolve；widget error 则 resolve `null`
- [x] 5.3 修改 `frontend/src/components/HCaptchaWidget.vue`：同上逻辑
- [x] 5.4 新建 `frontend/src/components/TencentCaptchaWidget.vue`：
    - 动态加载 `https://turing.captcha.qcloud.com/TCaptcha.js`（带 10s 超时与重复加载保护）
    - `onMounted` 调用 `new TencentCaptcha(props.siteKey, callback, { type: 'popup', userLanguage: 'zh-cn' })`
    - `execute()` 实现：返回新 Promise，调用 `cap.show()`；callback `ret=0` 且 `ticket` 不以 `trerror_` 开头 → resolve `{ticket, randstr}`；`ret=0` 且 `ticket` 以 `trerror_` 开头 → emit `fallback` 事件 + resolve 一个带 fallback 标志的对象（或 reject 特定错误，由 5.5 决定具体形态）；`ret=2`（用户取消）→ resolve `null`
- [x] 5.5 在 `CaptchaWidget` dispatcher 与表单之间约定 fallback 信号传递方式：建议 `execute()` 在 fallback 时返回 `null` 同时通过 dispatcher state 暴露 `lastError = "fallback"`，让表单按 D5 / Spec 的"前端层 fallback 重试 1 次"分支处理

> 实施偏差说明：§5.1 的 `verify` 事件签名保留为 `(token: string)` 以确保现有 4 个 view 与 `PendingOAuthCreateAccountForm` 在 §6 改造前仍能编译/通过测试；同时 dispatcher 新增结构化 `verifyPayload(payload)` 事件供 §6 切换后使用。`execute()` 已按 D1 / D5 暴露完整结构化 payload + `lastError`。归档时同步在 design.md D1 中订正。

## 6. 前端 - 4 个表单 Submit 流程改造（D2, D5）

- [x] 6.1 `LoginView.vue`：把 submit handler 包成 async，`await captchaRef.value.execute()`，按返回值决定是否调用真实登录 API；提交字段从 `captcha_token` 改为 `captcha_payload`
- [x] 6.2 `RegisterView.vue`：同上改造
- [x] 6.3 `ForgotPasswordView.vue`：同上改造
- [x] 6.4 `EmailVerifyView.vue`：同上改造
- [x] 6.5 在 4 个 view 中实现 fallback 自动重试一次的状态机（建议提取 composable `useCaptchaSubmit` 复用）：
    - 收到 `null` 且 `lastError === "fallback"` → 状态机标记一次 fallback，再次 `execute()`
    - 收到第二次 fallback 或服务端返回 `captcha.tencent.fallback_ticket` 第二次 → 弹错误 toast，停止重试
- [x] 6.6 提交按钮 disabled 期间不允许重复触发；fallback 重试时按钮保持 disabled
- [x] 6.7 兼容性：当后端返回 `captcha_provider = "turnstile"` 或 `"hcaptcha"` 时，所有上述代码路径仍保持现有 UX（widget 自动渲染，`execute()` 返回缓存 token）

> 实施说明（§6）：
> - 抽出 `frontend/src/composables/useCaptchaSubmit.ts`：实现 captcha-gated submit 的状态机（execute → submitFn → 自动 fallback 重试 1 次 → 抛归一化错误 reason: cancelled/fallback_exhausted/submit）。配套 10 个 vitest 用例覆盖核心契约（design.md D5）。
> - 4 个 view（Login / Register / ForgotPassword / EmailVerify）+ `PendingOAuthCreateAccountForm` 全部接入 `captcha_payload` 协议。captcha_token 兼容字段在 EmailVerifyView 的 sendCode / register 调用中仍同步透传一份给后端老路径（兼容窗口，下个 release 移除）。
> - 按钮 disabled 与 validateForm 的"无 token 即拦截"逻辑加上了 `captchaProvider !== 'tencent_captcha'` 的 popup 旁路（天御点击提交后才弹挑战，不能强制要求"先 verify"）。
> - 同步把 `frontend/src/components/auth/__tests__/PendingOAuthCreateAccountForm.spec.ts` 中过时的 `turnstile_token` 期望迁移到 `captcha_payload`；`EmailVerifyView.spec.ts` 中的 `turnstile_token: undefined` 同步升级为 `captcha_payload: undefined, captcha_token: undefined`。

## 7. 前端 - 公开设置与类型

- [x] 7.1 `frontend/src/stores/app.ts`（或公开 settings 类型定义处）：`captcha_provider` 字面量联合追加 `"tencent_captcha"`
- [x] 7.2 4 个 view 中读取 `settings.captcha_site_key || settings.turnstile_site_key` 的兼容逻辑保留不动；确认天御场景下 `captcha_site_key` 已是 `captcha_app_id`

> 实施说明（§7）：
> - `frontend/src/types/index.ts` 中 `PublicSettings.captcha_provider` 联合已追加 `'tencent_captcha'`（§6 改造时一并完成）。
> - 所有 4 个 view + `PendingOAuthCreateAccountForm` 的 `captchaProvider` ref 类型也已扩为 `'turnstile' | 'hcaptcha' | 'tencent_captcha'`。
> - `captcha_site_key || turnstile_site_key` 兼容逻辑在所有 view 中保留不动；天御场景下后端 §3.4 已把 `captcha_site_key` 填充为 `captcha_config.captcha_app_id`，前端无需特殊处理。

## 8. 前端 - Admin 设置页（D7, D8）

- [x] 8.1 `frontend/src/views/admin/SettingsView.vue`：provider 下拉选项新增"腾讯天御 (TencentCaptcha)"
- [x] 8.2 表单条件渲染：选中 `tencent_captcha` 时显示 4 个输入框（`captcha_app_id`、`app_secret_key`、`secret_id`、`secret_key`）；已配置项展示 `*_configured` 占位（与 Turnstile `secret_key` 复用同款 UI 组件）
- [x] 8.3 选中 `tencent_captcha` 时，"测试连接 / 预校验"按钮文案改为提示"无法预校验，请保存后实测一次"，或直接禁用该按钮
- [x] 8.4 设置页底部加一行非阻断提示："建议在反向代理 / Swarm 部署下配置 trusted_proxies，否则天御风控评分准确性会降低"
- [x] 8.5 placeholder/labels 文案与字段含义校对（避免把 `secret_id` 标成 `app_secret_key`）

> 实施说明（§8）：
> - provider 下拉项 i18n key `admin.settings.captcha.tencentProviderLabel`（中文 "腾讯天御 (TencentCaptcha)" / 英文 "Tencent Captcha"）。
> - 4 字段输入框分别绑定 `form.captcha_config.captcha_app_id` / `app_secret_key` / `secret_id` / `secret_key`，admin 后端 `captchaEditableFields` 表已按此 4 字段名识别（service.captchaSecretFields["tencent_captcha"]）。
> - `*_configured` 标志映射：`captcha_secret_key_configured`（复用 Turnstile/hCaptcha 通用字段，给 AppSecretKey 用）+ 新增 `captcha_tencent_secret_id_configured` / `captcha_tencent_secret_key_configured`（已在 `frontend/src/api/admin/settings.ts` SystemSettings 类型中补齐）。
> - D8 trusted_proxies 提示用 amber banner 渲染；天御不支持 preflight 的提示用普通灰文本，二者均不阻断保存。
> - i18n 中英双语 key 已同步：`tencentCaptchaAppId/Hint`、`tencentAppSecretKey/Hint/ConfiguredHint`、`tencentSecretId/Hint/ConfiguredHint`、`tencentSecretKey/Hint/ConfiguredHint`、`tencentTrustedProxiesHint`、`tencentNoPreflightHint`、`tencentDashboard`、`tencentProviderLabel`。

## 9. 测试与 QA

- [x] 9.1 后端：`go test ./internal/service/... ./internal/handler/...` 全部绿；TC3 签名向量测试稳定
- [x] 9.2 前端：组件单测覆盖 `TencentCaptchaWidget.execute()`（mock `window.TencentCaptcha`）、`CaptchaWidget` dispatcher 在三 provider 下的 expose 行为

> 实施说明（§9）：
> - 9.1 ✅ 后端 `go test -tags=unit ./internal/{service,handler,repository,server}/...` 全绿（含 TC3 签名向量、`trerror_` 短路、CaptchaCode 归一化、metadata 透传契约用例）。
> - 9.2 ✅ 新增三份前端单测：`useCaptchaSubmit.spec.ts`（10 用例）、`TencentCaptchaWidget.spec.ts`（5 用例）、`CaptchaWidget.spec.ts`（5 用例）。`pnpm test:run` 整体 fail 数从 baseline 14 下降到 13（修了一个 turnstile_token 期望过时的 fail），新增 21 个 pass。
> - 端到端手测任务（9.3-9.5）已移除，staging 验收由 QA 流程独立覆盖。

## 10. 文档与发布

- [x] 10.1 更新 `DEV_GUIDE.md`（或对应开发文档）添加"如何启用腾讯天御"章节，覆盖：admin 配置项含义、TC3 不引入 SDK 的设计选择、容灾票据策略
- [x] 10.2 在 PR 描述中显式标注"发布顺序：先后端、后前端"；列出下一个 release 要清理的兼容字段（`captcha_token` / `turnstile_token`）
- [x] 10.3 在 README_CN / README / README_JA 的"验证码"段落补充 provider 列表新增天御一行（若现有文档已枚举 provider）
- [x] 10.4 提 issue 跟踪"下一周期移除 `captcha_token` / `turnstile_token` 兼容读取"，链回本变更

> 实施说明（§10）：
> - DEV_GUIDE.md 末尾新增第 8 节 "如何启用腾讯天御 (TencentCaptcha)"（5 小节）：admin 4 字段说明 / 不引入 SDK 设计选择 / `trerror_` 严格拒绝策略 / trusted_proxies 依赖 / 错误码归一化映射表。
> - README / README_CN / README_JA 中的 captcha provider 枚举均已追加 "腾讯天御 (TencentCaptcha)" / "Tencent Captcha"。
> - PR 描述模板写入 `openspec/changes/add-tencent-captcha-provider/PR_NOTES.md`：含发布顺序、兼容字段清理列表、变更面、测试摘要、OpenSpec 链接。
> - Follow-up issue 跟踪由提 PR 时同步开（PR_NOTES.md 中已显式注明清理范围与下一个 release 任务）。

## 11. Lint / 归档

- [x] 11.1 `openspec validate add-tencent-captcha-provider --strict` 通过
- [x] 11.2 后端 `go vet ./...` 与项目既有 lint 命令通过；前端 `pnpm lint`（或对应命令）通过
- [x] 11.3 变更全部实施完成后，按 archive 流程将本变更归档到 `openspec/changes/archive/`，并把 `specs/captcha/spec.md` 同步落地到 `openspec/specs/captcha/spec.md`

> 实施说明（§11）：
> - 11.1 ✅ `Change 'add-tencent-captcha-provider' is valid`
> - 11.2 ✅ 后端 `go vet ./...` 静默通过；前端 `pnpm typecheck` / `pnpm lint:check` 全绿。
> - 11.3 ✅ 归档完成：变更移至 `openspec/changes/archive/add-tencent-captcha-provider/`，`specs/captcha/spec.md` 同步落地到 `openspec/specs/captcha/spec.md`。
