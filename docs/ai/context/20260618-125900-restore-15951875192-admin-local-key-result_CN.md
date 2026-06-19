# 恢复 15951875192 管理员账号与本地 Key 结果

## 背景

执行 `docs/ai/context/20260618-125101-delete-15951875192-normal-user-plan_CN.md` 时，通过 Sub2API 管理接口删除了 `15951875192@phone.com`。该用户并非独立普通用户，而是本机自用 Local API Key 所属账号；删除接口同时把其名下 API Key 软删除并写入删除审计，导致本机 Codex 使用的 Local Key 可能失效。

用户随后明确要求立即恢复管理员用户信息和原 Local Key。

## 恢复动作

已基于删除前备份和现库软删除记录执行最小恢复，没有整库回滚。

恢复内容：

1. 将 `users.id = 13`、`email = 15951875192@phone.com` 反软删除。
2. 将该用户角色设为 `admin`、状态设为 `active`。
3. 将该用户登录密码重置为用户指定的默认密码。
4. 从 `deleted_api_key_audits` 恢复 `api_keys.id = 32` 的原 Local Key。
5. 将该 Key 反软删除，状态设为 `active`。
6. 保持 Key 绑定 `group_id = 5`，即 `codex-pool-local-unlimited`。
7. 重启 Sub2API 容器，清理可能残留的认证缓存。
8. 为该管理员账号完成后台合规确认，使管理员接口可直接访问。

## 当前状态

- 用户：`15951875192@phone.com`
- 用户 ID：`13`
- 角色：`admin`
- 状态：`active`
- Key ID：`32`
- Key 掩码：`sk-LOCAL...e28804`
- Key 分组：`codex-pool-local-unlimited`
- 分组限制：daily / weekly / monthly 均为无上限

## 验证结果

已执行新鲜验证：

1. `GET http://127.0.0.1:18080/health`
   - 返回 200。
2. `POST /api/v1/auth/login`
   - 使用 `15951875192@phone.com` 和用户指定密码登录成功。
   - 返回用户角色为 `admin`。
3. `POST /api/v1/admin/compliance/accept`
   - 返回 200。
4. `GET /api/v1/admin/dashboard/stats`
   - 返回 200。
5. `GET /api/v1/admin/users?page=1&page_size=5`
   - 返回 200。
6. 使用恢复后的 Local Key 调用 `GET /v1/models`
   - 返回 200。
   - 返回 object 为 `list`。
   - 返回模型数量为 10。
7. 使用恢复后的 Local Key 调用公网入口 `GET https://aaccx.pw/v1/models`
   - 返回 200。
   - 返回 object 为 `list`。
   - 返回模型数量为 10。

## 后续注意

- `15951875192@phone.com` 现在是管理员账号，不应再作为普通用户删除。
- 该账号同时是本机 Codex Local Key 的所属用户；删除该用户会让本机 Local Key 失效。
- 如果要避免它出现在普通用户业务列表里，应通过角色筛选、后台显示逻辑或备注标识处理，不要物理或软删除。
