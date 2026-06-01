## Why

现有验证码体系只支持 Cloudflare Turnstile 与 hCaptcha 两家"声明式渲染 + 单字段 token + 单密钥校验"的协议。腾讯天御（TencentCaptcha）是国内合规与可用性更友好的选择，但它的协议与现有体系存在三处硬不兼容：前端凭证是 `ticket + randstr` 两字段、服务端鉴权是"业务密钥 + 云 API 密钥对"四字段、官方推荐 UX 是命令式 `cap.show()` 而非自动渲染。这三处差异不能通过简单"复用 site_key/secret_key"绕过，必须在 captcha 抽象层做一次有意识的扩展。

## What Changes

- 新增 captcha provider `tencent_captcha`，前端使用嵌入式以外的"提交时弹窗"原生 UX：用户点击表单提交按钮 → 拦截 → `cap.show()` → 拿到 ticket/randstr → 程序化提交
- **BREAKING (内部协议)**: 前后端提交认证类请求的字段从 `captcha_token: string` 统一改为 `captcha_payload: map<string,string>`（`{token?, ticket?, randstr?}`），老字段 `captcha_token` / `turnstile_token` 保留 1 个发布周期做兼容
- **BREAKING (内部接口)**: 重构 `CaptchaVerifier` 接口为对象形式：`Verify(ctx, VerifyRequest{Provider, Config, Payload, RemoteIP}) (*VerifyResult, error)`，老 Turnstile/hCaptcha 内部实现保持 HTTP 调用逻辑不变
- 扩展 `captcha_config` 结构，容纳天御 4 个新字段：`captcha_app_id` / `app_secret_key` / `secret_id` / `secret_key`；`maskCaptchaConfig` 改为按 provider 分支选择脱敏字段
- 公开下发：`PublicSettings.captcha_site_key` 在天御场景下复用为 `captcha_app_id`（保持字段名不变以维持前端兼容）
- 新增 `TencentCaptchaWidget.vue`，与 Turnstile/hCaptcha widget 一起接入 `CaptchaWidget` dispatcher；dispatcher 暴露统一的命令式 `execute(): Promise<Payload | null>` 方法供登录/注册/找回密码/邮箱验证 4 个表单调用
- 服务端新增 `tencentCaptchaHTTPVerifier`：手写 TC3-HMAC-SHA256 签名（不引入 tencentcloud-sdk-go），复用 `httpclient.GetClient` 以保留 SSRF 防护与统一超时
- 严格模式处理天御容灾票据：服务端识别 `trerror_` 前缀直接拒绝；前端在收到该错误码时清票据并允许用户最多自动重试 1 次，第 2 次起降级为普通错误提示
- Admin 设置页表单：天御 provider 显示 4 个字段输入、`ValidateProviderSecretKey` 对天御直接跳过并提示"无法预校验，请实测一次"
- UserIp 取值保持 `c.ClientIP()` 不变，在 tencent verifier 处添加注释说明依赖 `trusted_proxies` 配置；admin 设置页加一行非阻断提示

## Capabilities

### New Capabilities

- `captcha`: 多 provider 验证码抽象（站点公钥下发、服务端校验、provider 配置脱敏、命令式触发协议、统一 payload 字段、天御容灾票据策略）。本项目目前没有任何 OpenSpec capability，此次以"将现有 captcha 行为正式化为 spec"为契机，把 Turnstile / hCaptcha 既有能力与天御新增能力一并写入同一个 capability，后续 captcha 相关变更都收敛到此 spec。

### Modified Capabilities

<!-- 项目 openspec/specs/ 当前为空，无已存在 capability 需要 delta 修改。 -->

## Impact

- 后端代码:
  - `backend/internal/service/captcha_verifier.go`（接口重构 + 新增 tencent verifier）
  - `backend/internal/service/captcha_service.go`、`captcha_service_test.go`（调用方适配）
  - `backend/internal/service/setting_service.go`（`maskCaptchaConfig` 按 provider 分支、`normalizeCaptchaProvider` 接入 `tencent_captcha`）
  - `backend/internal/handler/dto/auth.go` / `settings.go`（DTO 增加 `captcha_payload`，老字段保留兼容期）
  - `backend/internal/handler/auth_handler.go`、`captcha_handler.go`（请求解包改读 payload）
  - `backend/internal/handler/dto/settings.go` 中 `PublicSettings`（天御场景 `captcha_site_key` 填 `captcha_app_id`）
  - **数据库迁移**：无需新增 migration。`captcha_provider` 存储在通用 `settings(key, value)` 表中，无 ENUM/CHECK 约束；`tencent_captcha` 取值仅在应用层 `normalizeCaptchaProvider` 中放开即可
- 前端代码:
  - `frontend/src/components/CaptchaWidget.vue`（dispatcher 新增 `tencent_captcha` 分支 + 暴露 `execute()` ref 方法）
  - `frontend/src/components/TurnstileWidget.vue` / `HCaptchaWidget.vue`（补 `execute()` 为读取已存 token 的 no-op resolver）
  - `frontend/src/components/TencentCaptchaWidget.vue`（**新增**，动态加载 `https://turing.captcha.qcloud.com/TCaptcha.js`、命令式 `new TencentCaptcha + cap.show()`、容灾票据识别）
  - `frontend/src/views/auth/{LoginView,RegisterView,ForgotPasswordView,EmailVerifyView}.vue`（submit 流程改为 `await captchaRef.execute()`；trerror_ 自动重试 1 次的状态机）
  - `frontend/src/views/admin/SettingsView.vue`（provider 下拉新增"腾讯天御"、对应 4 个字段输入与脱敏展示、`trusted_proxies` 软提示）
  - `frontend/src/stores/app.ts` / SSR 注入点（PublicSettings 类型新增 `tencent_captcha` provider 字面量）
- 外部依赖:
  - 不引入新的 Go 模块；TC3 签名在仓库内手写
  - 浏览器端新增加载 `https://turing.captcha.qcloud.com/TCaptcha.js`
- 配置/部署:
  - 管理员需在后台填入 `CaptchaAppId / AppSecretKey / SecretId / SecretKey` 才能启用天御
  - 建议但不强制配置 `trusted_proxies`（影响天御评分准确性，不影响通过率）
- 风险:
  - 老前端在新后端发布后仍可工作（兼容字段保留 1 个版本）；老后端遇到新前端发的 `captcha_payload` 会忽略（验证失败而非崩溃）—— 升级顺序：**先后端，后前端**
  - 严格 trerror_ 策略：天御侧大面积故障时登录链路会受影响，已在 design.md 中给出降级开关方案
