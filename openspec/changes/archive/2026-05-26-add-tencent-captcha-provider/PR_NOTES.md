# PR Notes — `add-tencent-captcha-provider`

## 摘要

新增腾讯天御 (TencentCaptcha) 作为 sub2api 的第三个 captcha provider，与现有 Turnstile / hCaptcha 并列。
同时把"前端单 token"假设替换为显式 `captcha_payload` map 协议，让多 provider 抽象更干净。

完整设计见 `openspec/changes/add-tencent-captcha-provider/`（proposal / design / specs / tasks / qa-checklist）。

## 发布顺序（重要）

**先后端、后前端**。

理由：
1. 后端在 §4 引入 `captcha_payload` 协议时**保留** `captcha_token` / `turnstile_token` 兼容字段窗口，老前端打到新后端无影响。
2. 前端 §6 改造完成后**仍同时**透传 `captcha_payload` + `captcha_token` 双字段（在 EmailVerifyView / RegisterView 的 sessionStorage 路径里），向后兼容。
3. 切换 provider 到 `tencent_captcha` 之前必须确保前端已升级，否则 dispatcher 给老 view 的 `verify` 事件只透传 ticket 当 captcha_token 用，后端拿不到 randstr 导致 `captcha.invalid`。
   admin 设置页的 provider 下拉项 "腾讯天御 (TencentCaptcha)" 在前端未升级时**不应被选中**。

## 兼容字段窗口（下个 release 清理）

- `LoginRequest.captcha_token`、`LoginRequest.turnstile_token`
- `RegisterRequest.captcha_token`、`RegisterRequest.turnstile_token`
- `SendVerifyCodeRequest.captcha_token`、`SendVerifyCodeRequest.turnstile_token`
- `ForgotPasswordRequest.captcha_token`、`ForgotPasswordRequest.turnstile_token`

新协议字段：`captcha_payload: Record<string, string>`（turnstile / hcaptcha 走 `{token}`，
天御走 `{ticket, randstr}`）。下个 release（**v0.X.Y+1**，待 follow-up issue 跟踪）移除上述兼容字段。

> 已开 follow-up：`backend/internal/handler/dto/auth.go` `extractCaptchaPayload` helper 中的 fallback 分支与
> 4 个 Request DTO 的兼容字段需在下一个周期一起删除；同时升级 release notes。

## 变更面（高层）

### 后端
- `internal/repository/captcha_verifier.go` + 新建 `captcha_tencent.go`：实现新版 `CaptchaVerifier` 接口（`Verify` + `ValidateProviderConfig`），手写 TC3-HMAC-SHA256 签名（不引入 tencentcloud-sdk-go）。
- `internal/service/captcha_service.go`：`VerifyPayload(ctx, payload, remoteIP)` 新入口；归一化 6 个错误码常量；通过 `ApplicationError.WithMetadata` 透传到 ResponseEnvelope。
- `internal/service/setting_service.go`：`captchaSecretFields` 表 + provider-aware `maskCaptchaConfig`；`normalizeCaptchaProvider` 放开 `tencent_captcha`；天御场景下 `captcha_site_key` = `captcha_app_id`。
- `internal/handler/auth_handler.go` + `auth_oauth_pending_flow.go`：5 个调用点全部改造为 `VerifyCaptchaPayload + extractCaptchaPayload`。

### 前端
- 新建 `components/TencentCaptchaWidget.vue`：动态加载 TCaptcha.js（10s 超时 + 重复加载保护）+ 命令式 `execute()` + `trerror_` 前缀检测。
- 改造 `CaptchaWidget.vue` / `TurnstileWidget.vue` / `HCaptchaWidget.vue`：暴露命令式 `execute()` API；保留向后兼容的 `verify(token)` 事件 + 新增 `verifyPayload(payload)` 事件。
- 新建 `composables/useCaptchaSubmit.ts`：fallback 重试 1 次状态机。
- 4 个 view（Login / Register / ForgotPassword / EmailVerify）+ `PendingOAuthCreateAccountForm` 全部接入 `captcha_payload` 协议 + `useCaptchaSubmit`；按钮 disabled 与 validateForm 加 `captchaProvider !== 'tencent_captcha'` popup 旁路。
- `views/admin/SettingsView.vue`：provider 下拉新增"腾讯天御"；4 字段输入 + `*_configured` 占位 + trusted_proxies 提示 + 无 preflight 提示。

## 测试

- 后端：`go test -tags=unit ./internal/{service,handler,repository,server}/...` 全部绿（含 TC3 签名向量、`trerror_` 短路、CaptchaCode 归一化、metadata 透传契约）。
- 前端：新增 20 个用例（`useCaptchaSubmit.spec.ts` 10 + `TencentCaptchaWidget.spec.ts` 5 + `CaptchaWidget.spec.ts` 5），全部通过；`pnpm typecheck` / `pnpm lint:check` 绿。
- 仓库 baseline 14 fail 已修复 1 个（PendingOAuthCreateAccountForm 中 `turnstile_token` 期望过时），剩余 13 fail 全是与本变更无关的历史遗留。
- Staging 端到端手测见 `openspec/changes/add-tencent-captcha-provider/qa-checklist.md`。

## OpenSpec

- 变更目录：`openspec/changes/add-tencent-captcha-provider/`
- `openspec validate add-tencent-captcha-provider --strict` 通过。
- 实施完毕后按 archive 流程归档到 `openspec/changes/archive/`，并把 `specs/captcha/spec.md` 落地到 `openspec/specs/captcha/spec.md`。
