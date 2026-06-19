# 管理员用户页 username 为空兜底计划

## 背景

管理员 `/admin/users` 页面中，部分用户的“用户名”列显示为 `-`。截图中这些用户的“用户”列已有邮箱，但单独 `username` 字段为空字符串。

## 根因

- 数据库 `users.username` 字段默认是空字符串，允许未填写。
- 邮箱注册、OAuth/三方同步或手工新增用户时，部分路径没有显式设置 `username`。
- 管理员用户页直接展示 `row.username || '-'`，因此空用户名显示为 `-`。

## 目标

- 已有空用户名用户：统一回填为邮箱。
- 新增/注册用户：如果没有显式用户名，默认写入邮箱。
- 管理员用户页：显示层保留 `username || email` 兜底，避免异常历史数据继续显示 `-`。

## 实施范围

1. 后端用户创建逻辑：确保 email 注册用户 username 默认等于 email。
2. 管理员创建用户逻辑：如果请求未传 username，则默认等于 email。
3. 管理员用户页前端：用户名列显示 `username || email || '-'`。
4. 数据库运行态回填：`UPDATE users SET username=email WHERE username='' AND deleted_at IS NULL`。
5. 增加/更新测试覆盖空 username 兜底。

## 验证

- 前端单测覆盖管理员页 username 为空时显示邮箱。
- 后端单测覆盖注册用户 username 默认等于 email。
- 数据库查询确认空 username 数量为 0。
- 管理员用户列表 API 确认相关用户 username 已等于邮箱。
