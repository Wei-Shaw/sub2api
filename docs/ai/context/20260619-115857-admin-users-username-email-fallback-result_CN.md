# 管理员用户页用户名邮箱兜底修复结果

## 背景

管理员 `/admin/users` 页面中部分用户的“用户名”列显示 `-`。这些用户的 `users.username` 是空字符串，前端用户名列直接显示 `value || '-'`，所以即使“用户”列有邮箱，“用户名”列仍为空。

本次按已确认方案执行：现有数据回填、后端创建兜底、前端显示兜底。

## 改动

- 前端管理员用户页 `UsersView.vue`：用户名列显示改为 `username || email || '-'`。
- 前端测试 `UsersView.spec.ts`：增加空用户名时显示邮箱的覆盖。
- 后端邮箱注册 `AuthService.RegisterWithVerification`：创建用户时写入 `Username: email`。
- 后端管理员创建用户 `AdminService.CreateUser`：`username` trim 后为空时默认使用 email。
- 后端测试：
  - 注册成功时断言 `Username == Email`。
  - 管理员创建用户传空白用户名时断言落库用户名为 email。

## 运行库处理

已回填当前运行库中 active 用户的空用户名：

```sql
UPDATE users
SET username = email, updated_at = NOW()
WHERE deleted_at IS NULL AND username = '';
```

执行结果：

- 回填 5 个用户：`admin@sub2api.local`、`jinzhiduo2850@gmail.com`、`1915474749@qq.com`、`xiaobianfuai@gmail.com`、`tongji_lishouqi@163.com`
- active 用户空用户名数：`0 / 26`

## 部署

已重新构建 `weishaw/sub2api:latest`，并用原 compose 配置重建 `sub2api` 应用容器。Postgres 和 Redis 未重建，数据卷未动。

运行状态：

- `http://127.0.0.1:18080/health` 返回 `{"status":"ok"}`
- `sub2api` 容器状态为 `healthy`
- `http://127.0.0.1:18080/admin/users` 返回 200

## 验证

已执行：

```bash
pnpm test:run src/views/admin/__tests__/UsersView.spec.ts
go test ./internal/service -tags unit -count=1 -run 'TestAuthService_Register_Success|TestAdminService_CreateUser'
docker build -t weishaw/sub2api:latest .
docker compose --env-file deploy/.env.scheme-a.local -f deploy/docker-compose.yml up -d --no-deps --force-recreate sub2api
```

结果：

- 前端定向测试通过：2 个测试通过
- 后端定向测试通过
- Docker 镜像构建通过
- 应用容器重启后健康检查通过

## 后续注意

- 后续用户创建路径应继续保证 `username` 不为空；如果新增其它创建入口，也应复用同一语义：空用户名默认邮箱。
- 不要把管理员用户页的空用户名当成正常状态；如果再次出现，优先检查是否存在绕过 `AuthService` / `AdminService` 直接写 `users` 的脚本或迁移。
