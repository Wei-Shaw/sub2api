# yui.web 用户迁移到 Sub2API：手机号内部邮箱与默认密码方案

## 背景

- yui.web 现有用户以手机号作为登录标识，密码 hash 使用 `scrypt$...` 格式。
- Sub2API 现有用户模型以 email / username 为主要身份字段，密码 hash 使用 bcrypt。
- yui.web 的旧密码 hash 不能直接复制到 Sub2API 后无感登录。
- 用户确认迁移后所有用户名统一转为 `手机号@phone.com` 形式的内部邮箱，并给迁移用户设置默认密码。

## Sub2API 现有密码能力

Sub2API 已有登录后修改密码能力：

- 前端组件：`frontend/src/components/user/profile/ProfilePasswordForm.vue`
- 前端 API：`frontend/src/api/user.ts`
- 后端路由：`PUT /api/v1/user/password`
- 后端服务：`backend/internal/service/user_service.go` 的 `ChangePassword`

该能力要求用户先登录，然后提交当前密码和新密码。后端会校验当前密码，写入新密码 hash，并递增 `TokenVersion`，使旧 JWT 失效。

Sub2API 也有忘记密码 / 邮件重置能力：

- `POST /api/v1/auth/forgot-password`
- `POST /api/v1/auth/reset-password`

但该路径依赖 `password_reset_enabled`、邮件队列/邮件服务和 `frontend_url` 配置，不适合作为这次默认密码迁移的主路径。

## 推荐迁移方案

采用一次性用户迁移：

1. 从 yui.web SQLite 读取需要迁移的用户。
2. 将手机号规范化为内部邮箱：`<phone>@phone.com`。
3. Sub2API `email` 写内部邮箱，`username` 可同样写内部邮箱或写脱敏手机号。
4. 为所有迁移用户设置同一个临时默认密码，迁移脚本从环境变量读取，不写入代码、文档或日志。
5. 用户使用内部邮箱和默认密码登录 Sub2API。
6. 用户进入个人资料页自行修改密码。

## 不推荐方案

- 不直接复用 yui.web 的 `scrypt$...` password_hash，因为 Sub2API 不使用该校验格式。
- 不让 Sub2API 实时读取 yui.web SQLite 登录，因为会把两个系统的身份源耦合在一起。
- 不依赖忘记密码邮件作为迁移主流程，因为当前内部邮箱并不一定是真实可收信邮箱，且邮件重置需要额外配置。
- 不开放公开注册来解决迁移问题；注册开关只影响新用户自助注册，不解决旧用户权益、余额和身份映射。

## 待确认

- `username` 是否也使用完整 `手机号@phone.com`，还是使用脱敏值。
- 默认密码的发放渠道和有效期策略。
- 是否需要实现“首次登录强制改密”。当前 Sub2API 有登录后改密，但未确认有强制首次改密状态字段；若需要强制，需新增用户字段和前端拦截逻辑。

