# 邮箱验证码与忘记密码当前状态

## 结论

- 当前项目支持 SMTP 发信能力，发件配置来自数据库 `settings` 表中的 `smtp_*` 设置。
- 当前运行态未配置 SMTP：`smtp_*` 有效配置数量为 0。
- 当前运行态已开启普通注册：`registration_enabled=true`。
- 当前运行态未开启邮箱验证码：`email_verify_enabled=false`。
- 当前运行态未开启忘记密码：`password_reset_enabled=false`。
- 登录页只有在 `password_reset_enabled=true` 且非 backend mode 时才显示“忘记密码”入口。

## 代码链路

- 邮件服务：`backend/internal/service/email_service.go`
  - `GetSMTPConfig()` 从数据库读取 `smtp_host`、`smtp_port`、`smtp_username`、`smtp_password`、`smtp_from`、`smtp_from_name`、`smtp_use_tls`。
  - `SendVerifyCode()` 发送 6 位邮箱验证码，验证码有效期 15 分钟，冷却 1 分钟，最多尝试 5 次。
  - `SendPasswordResetEmail()` 发送密码重置邮件，当前实现是邮件链接 token，不是用户手输验证码；token 有效期 30 分钟。
- 忘记密码接口：
  - `POST /api/v1/auth/forgot-password`
  - `POST /api/v1/auth/reset-password`
  - 实现位置：`backend/internal/handler/auth_handler.go`、`backend/internal/service/auth_service.go`
- 注册邮箱验证码接口：
  - `POST /api/v1/auth/send-verify-code`
  - 只在注册开启后发送，并受邮件队列与 SMTP 配置影响。
- 用户已登录后的邮箱绑定、通知邮箱、TOTP 相关验证码也复用同一套 SMTP 发信能力。

## 个人邮箱作为发件邮箱

技术上可以使用个人邮箱，只要邮箱服务商提供 SMTP 服务和应用专用密码/授权码，并且项目里正确填写：

- `smtp_host`
- `smtp_port`
- `smtp_username`
- `smtp_password`
- `smtp_from`
- `smtp_from_name`
- `smtp_use_tls`

但不建议长期用普通个人邮箱给用户发生产通知。原因：

- 个人邮箱容易触发频率限制、风控或封号。
- 发件域名、SPF、DKIM、DMARC 不规范时更容易进垃圾箱。
- 用户会看到个人邮箱地址，品牌和信任感较弱。
- 某些个人邮箱要求 `smtp_from` 与登录账号一致，否则会拒发或代发标识异常。

更稳妥做法是使用域名邮箱或专业邮件服务商，至少使用 `noreply@aaccx.pw` 一类的专用发件身份。

## 启用前必须确认

1. 配好 SMTP 并通过后台测试发信。
2. 设置 `frontend_url`，否则忘记密码链接无法生成正确公网地址。
3. 开启 `email_verify_enabled=true`。
4. 需要忘记密码时再开启 `password_reset_enabled=true`。
5. 若启用注册验证码，确认当前 `@phone.com` 迁移用户不会被误导为真实可收信邮箱。
