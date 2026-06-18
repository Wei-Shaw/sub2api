# Sub2API 本地用户入口迁移结果

## 执行结果

- 已从 yui.web SQLite 迁移 21 个手机号用户到当前本地 Sub2API。
- 迁移用户邮箱/用户名格式为 `<手机号>@phone.com`。
- 迁移用户角色为普通用户，状态为 active。
- 迁移用户默认密码为 `123123`。
- 已为 21 个迁移用户补齐 `auth_identities` 的 email 身份记录。
- 已打开注册入口：`registration_enabled=true`。
- 当前仍未打开邮箱验证码和忘记密码：
  - `email_verify_enabled=false`
  - `password_reset_enabled=false`

## 备份

- Sub2API PostgreSQL 迁移前备份：
  - `/Users/wujianxiang/CodeSpace/sub2api/.tmp-sub2api-before-phone-email-migration-20260618-101102.dump`
- yui.web SQLite 迁移前备份：
  - `/Users/wujianxiang/CodeSpace/sub2api/.tmp-yuiweb-shop-before-phone-email-migration-20260618-101102.sqlite`

## 验证

- 当前 Sub2API 活跃用户总数：23。
- 当前 `@phone.com` 迁移用户数：21。
- 当前 `@phone.com` email 身份记录数：21。
- 使用迁移账号和默认密码调用 `/api/v1/auth/login` 成功，返回 access token。
- `/register` 页面返回 HTTP 200。
- 浏览器已登录一个迁移账号并打开 `/profile`，可以看到个人资料、邮箱绑定状态和修改密码入口。

## 后续建议

- 用户真实邮箱确认后，将对应用户的 `users.email`、`users.username` 和 `auth_identities.provider_subject` 同步替换为真实邮箱。
- 如果要让新用户注册时必须收邮箱验证码，需要先配置 SMTP，再打开 `email_verify_enabled=true`。
- 若要启用忘记密码，同样需要 SMTP 可用，然后打开 `password_reset_enabled=true`。

