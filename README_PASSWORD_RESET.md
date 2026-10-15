# 密码找回指南

## 现有功能说明

本项目已经实现了完整的密码找回功能，包括：

### 1. 自动化密码重置流程（推荐）

**前提条件：**
- ✅ 邮件服务已配置（SMTP）
- ✅ 系统设置中已启用邮件验证功能
- ✅ 系统设置中已启用密码重置功能

**用户操作流程：**
```
1. 访问前端的"忘记密码"页面
2. 输入注册邮箱
3. 收到密码重置邮件（包含重置链接）
4. 点击链接，输入新密码
5. 完成重置
```

**API端点：**
- `POST /api/auth/password/reset/request` - 请求密码重置
- `POST /api/auth/password/reset/confirm` - 确认密码重置

**安全特性：**
- 防用户枚举（无论邮箱是否存在都返回成功）
- Token一次性使用
- Token有过期时间
- 重置后自动撤销所有现有会话

### 2. 手动重置密码（管理员操作）

当自动流程不可用时（邮件服务未配置、用户邮箱无法访问等），管理员可以手动重置：

#### 方法A：使用提供的工具脚本

```bash
# 1. 生成密码Hash
go run reset-password-manual.go "新密码123"

# 2. 复制输出的SQL语句执行
# 示例输出：
# UPDATE users SET password_hash = '$2a$10$...' WHERE email = '用户邮箱';
```

#### 方法B：直接使用SQL

```sql
-- 查找用户
SELECT id, email, role, status FROM users WHERE email = 'user@example.com';

-- 使用工具生成hash后更新
UPDATE users
SET password_hash = '$2a$10$生成的hash值'
WHERE email = 'user@example.com';

-- 可选：强制撤销该用户所有refresh token
-- 需要根据实际的Redis/缓存配置操作
```

### 3. 检查密码重置功能是否启用

**检查配置：**

```sql
-- 查看系统设置
SELECT key, value FROM settings
WHERE key IN (
    'email_verify_enabled',
    'password_reset_enabled',
    'smtp_configured'
);
```

**启用密码重置：**

如果功能未启用，需要在系统设置中配置：

1. **启用邮件验证**
   - 设置 `email_verify_enabled = true`

2. **启用密码重置**
   - 设置 `password_reset_enabled = true`

3. **配置SMTP**
   - 在配置文件或数据库中配置SMTP服务器信息

### 4. 相关代码位置

- **认证服务**：`backend/internal/service/auth_service.go`
  - `RequestPasswordReset()` - 第1567行
  - `RequestPasswordResetAsync()` - 第1591行
  - `ResetPassword()` - 第1615行
  - `IsPasswordResetEnabled()` - 第1520行

- **邮件服务**：`backend/internal/service/email_service.go`
  - 密码重置邮件发送
  - Token生成和验证

## 常见问题

### Q1: 用户说收不到密码重置邮件？

**排查步骤：**
1. 检查SMTP配置是否正确
2. 查看应用日志，搜索 "Password reset email"
3. 检查用户邮箱是否存在且状态为active
4. 检查邮件是否进入垃圾箱
5. 使用异步发送时，检查邮件队列服务是否正常

### Q2: 如何批量重置密码？

```sql
-- 不推荐批量重置，但如果确实需要：
-- 1. 先生成临时密码的hash
-- 2. 批量更新
UPDATE users
SET password_hash = '$2a$10$临时密码hash'
WHERE id IN (用户ID列表);

-- 3. 通知用户使用临时密码登录后立即修改
```

### Q3: 密码重置后为什么其他设备也被登出了？

这是设计的安全特性：
- 密码重置会让所有现有JWT token失效（通过TokenVersion机制）
- 同时撤销所有refresh token
- 确保只有知道新密码的人才能访问账户

### Q4: 如何禁用密码重置功能？

```sql
UPDATE settings
SET value = 'false'
WHERE key = 'password_reset_enabled';
```

或在配置文件中设置相应选项。

## 安全建议

1. ✅ **始终使用HTTPS** - 保护密码重置链接在传输中不被截获
2. ✅ **设置合理的Token过期时间** - 建议15-60分钟
3. ✅ **记录密码重置操作** - 便于审计
4. ✅ **限制重置请求频率** - 防止滥用
5. ✅ **通知用户** - 密码重置后发送通知邮件告知用户

## 使用工具脚本示例

```bash
# 进入项目目录
cd /Users/lifengdem1pro/Documents/students/gptplusch-sub2api

# 为用户生成新密码
go run reset-password-manual.go "TempPass2024!"

# 输出示例：
# =====================================
# 密码Hash生成成功！
# =====================================
# 新密码: TempPass2024!
# Hash值: $2a$10$abcd...xyz
# =====================================
#
# 执行以下SQL更新用户密码:
# UPDATE users SET password_hash = '$2a$10$abcd...xyz' WHERE email = '用户邮箱';

# 然后连接数据库执行SQL
psql -h localhost -U your_user -d your_database
# 粘贴上面的UPDATE语句（替换邮箱地址）
```

## 需要帮助？

如果遇到问题：
1. 检查应用日志：`grep -i "password reset" logs/*.log`
2. 检查邮件服务日志
3. 验证数据库连接和配置
4. 确认系统设置表中的相关配置项

---

**最后更新：** 2026-09-11
