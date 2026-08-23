# 如何启用腾讯天御 (TencentCaptcha)

> 本文介绍 sub2api 第三个验证码 provider 腾讯天御的配置与使用。设计文档见
> `openspec/changes/add-tencent-captcha-provider/`（含 proposal / design / specs / tasks）。

## 1. Admin 后台配置

进入 admin 设置页 → "验证码防护"段，将 provider 切换为 **"腾讯天御 (TencentCaptcha)"**。
此 provider 需要 4 个字段：

| 字段 | 用途 | 来源 |
|---|---|---|
| `CaptchaAppId` | 前端 SDK 实例化使用 | [腾讯云验证码控制台](https://console.cloud.tencent.com/captcha) → 创建 App |
| `AppSecretKey` | 前端 / 移动端验证码密钥 | 同上控制台 → App 详情 |
| `SecretId` | 调用 `DescribeCaptchaResult` 服务端校验 | [访问管理 CAM](https://console.cloud.tencent.com/cam/capi) |
| `SecretKey` | 同上，用于 TC3-HMAC-SHA256 签名 | 同 CAM |

> 已配置项保存后会被后端 `maskCaptchaConfig` 剥离，再次进入页面时显示"已配置"占位。
> 留空对应字段提交 → 后端按"未填则保留旧值"语义处理。

## 2. 设计选择：不引入 tencentcloud-sdk-go

后端 `backend/internal/repository/captcha_tencent.go` 手写 TC3-HMAC-SHA256 签名（约 130 行），**不**依赖
`tencentcloud-sdk-go`。原因：

- 整个签名只在一个 endpoint（`DescribeCaptchaResult`）使用，引入完整 SDK 体积/依赖收益不大；
- 手写签名让我们能用项目统一的 `httpclient.GetClient()`（含 SSRF 保护、IP 校验）；
- 签名本身遵循腾讯云 v3 规范，单测里有官方示例 fixture 对照（见 `captcha_tencent_test.go`）。

## 3. 容灾票据策略：trerror_ 严格拒绝

腾讯天御在网络抖动 / 风控降级场景会下发以 `trerror_` 开头的容灾票据。本仓库选择 **严格拒绝**：

- 后端 `tencentCaptchaVerify` 入口先 trim 检测，命中 `trerror_` 直接返回归一化错误码
  `captcha.tencent.fallback_ticket`（**不**发出真实 HTTP 请求，节省一次外呼）。
- 同时打 WARN 日志（含截断 / 匿名化的 RemoteIP），便于运维侧观察异常率突增。
- 前端 `useCaptchaSubmit` 状态机检测到该错误码后**自动重试一次**；二次仍 fallback 即弹错误 toast 不再重试。

## 4. 客户端 IP 来源

`UserIp` 字段直接传入 `c.ClientIP()` 解析的值，依赖框架 `trusted_proxies` 配置正确。
反向代理 / Swarm 部署下务必把真实出口 CIDR 列入 `trusted_proxies`，否则天御风控评分会因为始终拿到代理 IP 而失准。

## 5. 错误码归一化（design.md D6）

后端 `CaptchaCode` → `VerifyResult.ErrorCode` 映射表：

| 腾讯返回 | 归一化 ErrorCode | 前端含义 |
|---|---|---|
| 1 | success | 通过 |
| 6 / 7 / 15 | `captcha.config` | 后端配置错（AppId/Secret 不匹配） |
| 9 | `captcha.timeout` | 票据过期 |
| 10 | `captcha.duplicate` | 票据重放 |
| 其它 | `captcha.invalid` | 票据无效（前端可让用户重试） |
| trerror_ 前缀 | `captcha.tencent.fallback_ticket` | 容灾票据；触发前端 fallback 重试 1 次 |

ErrorCode 通过 `ApplicationError.WithMetadata("captcha_error_code", ...)` 透传到 ResponseEnvelope.metadata，
前端 `useCaptchaSubmit` 据此决定文案 / 重试。
