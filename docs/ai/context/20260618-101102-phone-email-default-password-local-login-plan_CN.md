# Sub2API 本地用户入口迁移计划：手机号内部邮箱 + 默认密码

## 目标

- 将 yui.web 现有手机号用户导入当前本地 Sub2API。
- 每个用户使用 `<手机号>@phone.com` 作为临时内部邮箱登录名。
- 所有迁移用户设置默认密码 `123123`，先保证最低等级登录入口可用。
- 用户登录后可在个人资料页自行修改密码。
- 新注册用户走 Sub2API 邮箱注册入口。

## 当前约束

- yui.web 旧密码 hash 为 `scrypt$...`，Sub2API 使用 bcrypt，不能直接复用旧 hash。
- 当前本地 Sub2API 未配置 SMTP，`email_verify_enabled=false`，不能真正给用户邮箱发送验证码。
- 因此本次只打开邮箱/密码注册入口，不打开邮箱验证码。后续配置 SMTP 后再启用邮箱验证和忘记密码。
- `123123` 是 6 位密码，符合后端最小长度 `min=6`；前端修改新密码表单要求至少 8 位，用户改密时需要设置更长的新密码。

## 执行设计

1. 备份 Sub2API PostgreSQL 和 yui.web SQLite。
2. 从 yui.web `users.phone` 读取现有手机号。
3. 生成内部邮箱：`lower(trim(phone)) || '@phone.com'`。
4. 使用 bcrypt 生成默认密码 hash，不在日志中输出 hash。
5. 写入 Sub2API `users`：
   - `email`: `<phone>@phone.com`
   - `username`: `<phone>@phone.com`
   - `role`: `user`
   - `status`: `active`
   - `balance`: `0`
   - `concurrency`: `5`
   - `signup_source`: `email`
   - `notes`: `migrated_from_yuiweb_phone_email_default_password_20260618`
6. 同步创建 `auth_identities` 的 email 身份记录，保持邮箱登录身份链路完整。
7. 设置 `registration_enabled=true`，保留 `email_verify_enabled=false` 与 `password_reset_enabled=false`，直到 SMTP 配置完成。

## 验证

- 统计迁移前后用户数量。
- 使用一个迁移用户通过 `/api/v1/auth/login` 登录。
- 打开本地 Sub2API 页面，进入个人控制中心确认个人资料、改密入口可见。
- 不输出完整手机号、JWT、密码 hash。

