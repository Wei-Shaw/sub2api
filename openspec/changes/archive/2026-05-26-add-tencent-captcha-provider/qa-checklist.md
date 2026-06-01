# 腾讯天御 (TencentCaptcha) Provider — 手测 / QA Checklist

适用于变更 `add-tencent-captcha-provider` 的人工验收阶段（tasks.md §9.3 / §9.4 / §9.5）。

## 前置条件

- staging 环境已部署本变更对应的后端镜像与前端 build。
- 在 [腾讯云验证码控制台](https://console.cloud.tencent.com/captcha) 创建一个验证码 App，
  拿到 `CaptchaAppId` / `AppSecretKey`（前端用）；以及账号下的 `SecretId` / `SecretKey`（后端用，需有 `captcha:DescribeCaptchaResult` 权限）。
- 反向代理 / Swarm 部署下确认 `trusted_proxies` 已包含真实客户端 IP 的来源（design.md D8）。

## §9.3 端到端：4 个表单 × 4 组场景

> 在 admin 设置页选 provider = "腾讯天御 (TencentCaptcha)"，填入上述 4 字段并保存。

### 场景 A：成功（正常 ticket）

对每个表单（Login / Register / ForgotPassword / EmailVerify resend code）执行：

1. 打开页面 → 点击提交按钮（无需提前 verify）。
2. 弹出腾讯天御挑战 → 完成滑块。
3. 期望：请求体 `captcha_payload` 包含真实 `ticket` + `randstr`，后端返回成功，按钮恢复可用。

### 场景 B：用户取消

1. 点击提交按钮 → 关闭天御弹窗。
2. 期望：前端不发请求，按钮恢复可用，无错误 toast（design.md D5: cancel 静默）。

### 场景 C：模拟 trerror_ 容灾票据（前端层 fallback）

> 触发方式：在浏览器 devtools 的 Network 中拦截 `https://t.captcha.qq.com/cap_union_new_show` 等天御请求并改返回，使 `ticket` 以 `trerror_` 开头；
> 或在断网状态下部分场景天御 SDK 会主动下发 `trerror_` 容灾票据。

1. 点击提交按钮 → 完成挑战 / 模拟容灾。
2. 期望：前端识别 `trerror_` → `lastError = "fallback"` → composable 自动重新触发 `execute()` 一次。
3. 第二次仍是 `trerror_` → 弹错误 toast `auth.captchaFailed`，停止重试。

### 场景 D：服务端 fallback_ticket（后端校验失败 + 自动重试）

> 触发方式：在 admin 设置页填一个错误的 `SecretKey`，或在 staging 后端日志注入测试桩让 `tencentCaptchaVerify` 返回归一化错误码 `captcha.tencent.fallback_ticket`。

1. 点击提交 → 第一次 captcha_payload 校验失败（响应 metadata.captcha_error_code = "captcha.tencent.fallback_ticket"）。
2. 期望：composable 触发 `onFallbackRetry` 一次（清空 token + reset widget），重新 `execute()` → 二次提交。
3. 第二次仍失败 → 弹错误 toast，停止重试。

### 场景 E：票据过期

1. 完成天御挑战，但等待 5 分钟后再点提交（天御票据有效期通常 5 分钟）。
2. 期望：后端返回错误码 `captcha.timeout`，前端 toast 显示对应错误（不自动 fallback 重试）。

## §9.4 Provider 切换 / 兼容窗口

1. admin 后台从 `turnstile` 切到 `tencent_captcha`，保存。
2. 立即在普通用户浏览器登录 → 期望天御弹窗形态、登录成功。
3. 切回 `turnstile`，保存。
4. 再次登录 → 期望 turnstile widget 直接渲染、登录成功。
5. 验证：现有会话（cookie / token）不被 captcha 切换打扰。

## §9.5 老前端 + 新后端 兼容性

1. 在另一个浏览器 / 隐身窗口打开**未升级的旧前端**（不带 `captcha_payload` 字段，仅 `captcha_token`）。
2. 后端切到 `turnstile`（兼容字段必须仍生效）。
3. 期望：登录链路正常通过；后端 `extractCaptchaPayload` helper 在缺失 `captcha_payload` 时回退到 `captcha_token`。
4. 切到 `tencent_captcha` + 老前端 → 已知不被支持（dispatcher 透传 ticket 给老 view 的 captcha_token，后端拿不到 randstr）；admin 切换前必须确保前端已升级。

## 自动化已覆盖（不再需要重复手测）

- TC3 签名向量 / `trerror_` 短路 / CaptchaCode 归一化（`backend/internal/repository/captcha_tencent_test.go`）。
- ApplicationError.Metadata 透传归一化错误码（`backend/internal/service/captcha_service_test.go`）。
- captcha-gated submit fallback 状态机 10 个用例（`frontend/src/composables/__tests__/useCaptchaSubmit.spec.ts`）。
- TencentCaptchaWidget execute() 5 个用例（`frontend/src/components/__tests__/TencentCaptchaWidget.spec.ts`）。
- CaptchaWidget dispatcher 三 provider 转发 + lastError 重置 5 个用例（`frontend/src/components/__tests__/CaptchaWidget.spec.ts`）。

## 通过标准

- §9.3 所有 4 组场景在 4 个表单全部表现符合期望。
- §9.4 切换前后 UX 与会话状态均符合预期。
- §9.5 老前端 + turnstile 后端组合完全无影响。
- 后端日志中可见 `trerror_` 命中时的 WARN 行（design.md D5：含截断/匿名化 RemoteIP）。
