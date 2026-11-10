# Cloudflare Email Service 邮件发送渠道

Sub2API 原本只能用 SMTP 发信。本功能新增 `email_provider` 设置项，把「用哪条通道发信」从代码里解耦出来，
目前支持 `smtp` 与 `cloudflare` 两个取值，默认 `smtp`（留空即回退）。切换渠道只需要在后台改一个下拉框，
不用重新部署。

> 本文面向两类读者：想直接启用 Cloudflare 发信的管理员（看第 1～4 节），和想阅读/审查实现的开发者（看第 5～8 节）。

---

## 1. 为什么加这个渠道

SMTP 依赖一台能稳定出网的中转（Gmail、SendGrid、自建 Postfix 都要维护凭据和信誉）。
Cloudflare Email Service 走 HTTPS API 发信，不需要维护 SMTP 会话，也不需要在中转服务器上放任何东西，
对于已经在用 Cloudflare 托管 DNS 的部署来说少一层运维。

代价是它**要求 Workers Paid 计划**（见第 4 节），而且**只能发事务性邮件**，不能用来发营销邮件。

---

## 2. 前置条件

| 条件 | 说明 |
| --- | --- |
| Cloudflare 账号 | 需要 Workers Paid 计划才能向任意收件人发信；Workers Free 计划只能发到已验证的目标地址 |
| 域名使用 Cloudflare DNS | 发信域名必须托管在同一个 Cloudflare 账号下，否则无法完成 onboard |
| 发信域名已接入 Email Sending | 见 2.1 |
| API Token | 具备 Email Sending 权限的账号级 Token，见 2.2 |
| Account ID | 账户概览页右侧栏的 **Account ID**，或控制台地址栏 `dash.cloudflare.com/<account_id>/...` 中的那一段 |

> Cloudflare Email Service 目前处于 beta 阶段，API 在正式版发布前仍可能调整。

### 2.1 接入发信域名（Onboard Domain）

1. 登录 Cloudflare 控制台，选择目标账号。
2. 进入 **Compute** > **Email Service** > **Email Sending**。
3. 点击 **Onboard Domain**，选择发信域名。
4. 在确认页可以看到 Cloudflare 即将自动写入的 DNS 记录：
   - **MX** 记录，写在 `cf-bounce.<你的域名>` 子域，用于回收退信；
   - **SPF** TXT，写在 `cf-bounce.<你的域名>`，值为 `v=spf1 include:_spf.mx.cloudflare.net ~all`；
   - **DKIM** TXT，写在 `cf-bounce._domainkey.<你的域名>`，公钥由 Cloudflare 生成；
   - **DMARC** TXT，写在 `_dmarc.<你的域名>`。
5. 点击 **Done**。DNS 通常 5～15 分钟生效，官方说明最长可达 24 小时。

注意两点：

- **这些记录全部挂在 `cf-bounce` 子域上**，不是根域名。根域上的是 Email Routing（收信）的记录，
  两者互不影响，也可以只启用其中一个。
- Email Sending 的记录接入后会被 Cloudflare 锁定，无法在 **DNS** > **Records** 里编辑或删除，
  只有把域名从 Email Sending 移除才会一并删除。

验证：进入 **Compute** > **Email Service** > **Email Sending** > **Settings**，**DNS records** 区块会列出
上面四条记录。状态显示 **Locked** 或 **Unlocked** 都表示配置正确，区别只是该记录是否由 Email Service 托管。

### 2.2 创建 API Token

1. 进入 **My Profile** > **API Tokens** > **Create Token**。
2. 使用 **Custom token**，权限选择 **Account** 范围的 **Email Sending : Edit**
   （部分账号显示为 **Email Sending — Send**，以控制台实际展示为准）。
3. Account Resources 限制到持有发信域名的那个账号。
4. 创建后抄下 Token —— 它只显示一次。

> Token 在 Sub2API 里以密文存储，界面上只回显「已配置 / 未配置」，不会把明文发给前端。

### 2.3 尚未接入域名时的限制

如果发信域名还没 onboard 完成，Cloudflare 只允许向**账号内已验证的目标地址**发信，其它收件人一律拒绝。
这一点在首次配置时最容易踩坑：后台「发送测试邮件」填了自己没验证过的邮箱，会拿到 403。

---

## 3. 在 Sub2API 中启用

后台 **系统设置** > **邮件设置** 选项卡：

1. **邮件发送渠道** 选择 **Cloudflare**。
2. 填写 **API Token**、**Account ID**、**发件人邮箱**、**发件人名称**。
   - 发件人邮箱必须属于已在 2.1 中接入的域名，例如 `welcome@example.com`。
   - 发件人名称可留空，留空时发件人只显示裸地址。
3. 保存。此时 SMTP 配置卡片会自动隐藏，Cloudflare 配置卡片出现。
4. 在 **发送测试邮件** 卡片里填一个收件地址，点击发送。

**切换回 SMTP** 同样只改下拉框即可，SMTP 的历史配置不会被清除。

已保存的 API Token 不会回显。再次保存时留空表示保留原 Token；只有重新输入才会覆盖。

### 相关 API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/admin/settings` | 返回值新增 `email_provider`、`cloudflare_api_token_configured`、`cloudflare_account_id`、`cloudflare_from_email`、`cloudflare_from_name` |
| `PUT` | `/api/admin/settings` | 请求体新增 `email_provider`、`cloudflare_api_token`、`cloudflare_account_id`、`cloudflare_from_email`、`cloudflare_from_name` |
| `POST` | `/api/admin/settings/email/test` | 请求体新增 `provider`、`cloudflare_api_token`、`cloudflare_account_id`、`cloudflare_from_email`、`cloudflare_from_name` |

`email_provider` 只接受 `smtp` 与 `cloudflare`（大小写不敏感、会去空格），留空回退到 `smtp`；
其它值返回 400 `INVALID_EMAIL_PROVIDER`。

测试邮件接口的 Cloudflare 字段留空时会回退到已保存的配置，方便前端只改一处就试发。

---

## 4. 配额与限制

以下均为 Cloudflare 侧的硬限制，Sub2API 不做额外拦截。

| 项目 | 限制 | 说明 |
| --- | --- | --- |
| 收件人（to + cc + bcc 合计） | 50 个/封 | |
| 主题长度 | 998 字符 | 符合 RFC 5322 |
| 邮件总大小 | 5 MiB | 含附件 |
| 邮件总大小 | 25 MiB | 仅限已验证目标地址 |
| 自定义标头总大小 | 16 KB | |
| 每个 zone 的域名数 | 30 | Email Routing + Email Sending 合计 |
| 每账户已验证目标地址 | 200 | 跨域名共享 |

**配额（Workers Paid）**：每月含 3,000 封外发邮件，超出后 $0.35 / 1,000 封。
发往已验证目标地址的邮件免费，且不计入月配额，也不占用每日发送上限。

**每日发送上限**：新账号以一个较保守的日配额起步，随发信行为、投递率和账号信誉自动上调，
没有公开的固定数值。需要更快提额请走 Cloudflare 的
[Limit Increase Request Form](https://forms.gle/eX6pXvit1wBv77Yw5)。

> Sub2API 的验证码、密码重置、订阅提醒都属于低频事务性邮件，正常情况下远低于月配额。
> 如果你打算用它跑通知类批量任务，先确认日配额够用。

---

## 5. 实现说明

### 5.1 分层

```
Callers (验证码 / 密码重置 / 通知 / 订阅提醒)
        │
        ▼
   EmailService.SendEmail(ctx, to, subject, body)
        │  按 email_provider 分发
        ├── smtp        → SendEmailWithConfig(...)         （原有实现，未改动）
        └── cloudflare  → SendEmailWithCloudflareConfig(...)（新增）
```

分发点是 [`EmailService.SendEmail`](../backend/internal/service/email_service.go)，
调用方完全无感：现有代码继续调用同一个方法，由设置决定走哪条路。

### 5.2 新增的设置键

`backend/internal/service/domain_constants.go`：

| 键 | 说明 |
| --- | --- |
| `email_provider` | `smtp` / `cloudflare`，留空回退 `smtp` |
| `cloudflare_api_token` | API Token，加密存储，只写不回显 |
| `cloudflare_account_id` | Cloudflare 账户 ID |
| `cloudflare_from_email` | 发件人地址，须为已接入的域名 |
| `cloudflare_from_name` | 发件人名称，可留空 |

设置项的读写链路（`settings_view.go` / `dto/settings.go` / `setting_parse.go` / `setting_update.go`）
沿用既有的「密文只写」约定：`cloudflare_api_token` 明文永不返回给前端，只返回
`cloudflare_api_token_configured` 布尔值；保存时该字段为空即保留原值。

### 5.3 发送实现

`SendEmailWithCloudflareConfig` 使用官方 Go SDK `github.com/cloudflare/cloudflare-go/v6` 的
`email_sending` 包，调用：

```
POST https://api.cloudflare.com/client/v4/accounts/{account_id}/email/sending/send
```

请求体由 SDK 构造：`from` 传 `{address, name}`（name 为空时退化为裸地址字符串）、`to`、
`subject`、`html`，以及由 HTML 转出的 `text` 纯文本副本——同时带 html 与 text 两个版本对投递率更友好。

发送前会校验 `to` 与 `subject` 中是否含换行符，命中则直接拒绝，避免标头注入。

> 注意：本功能使用 `/email/sending/send`，不是 `/email/routing/send`。
> 后者是收信（Email Routing）的接口，用它发信会失败。

### 5.4 测试

`backend/internal/service/email_service_cloudflare_test.go` 用 `httptest` 起本地服务，
通过 `option.WithBaseURL` 把 SDK 指过去，断言：

- 请求路径为 `/accounts/{account_id}/email/sending/send`；
- `Authorization` 头携带 Token；
- `subject` / `to` / `html` 原样透传，`text` 为 HTML 去标签后的纯文本；
- `from` 在给了名称时是 `{address, name}` 对象，未给名称时是裸地址字符串；
- 含换行的 `to` / `subject` 被拒绝；
- 缺少凭据或配置返回 `ErrEmailNotConfigured`；
- `email_provider` 为空时落到 SMTP 分支，取值非法时返回 `ErrInvalidEmailProvider`。

---

## 6. 排错

先看后端日志里的报错原文，再对照下表（错误码与消息取自 Cloudflare Email Service 的 REST API 文档）：

| HTTP | 错误码 | 消息 | 含义与处理 |
| --- | --- | --- | --- |
| 400 | 10001 | `invalid_request_schema` | 请求体不合法：检查发件人邮箱是否为空、字段格式是否正确 |
| 400 | 10200 | `email.too_big` | 超过大小限制：单封上限 5 MiB（已验证目标地址 25 MiB） |
| 400 | 10202 | `email.invalid` | 邮件内容非法：检查收件人地址格式 |
| 401 | 10101 | `email.auth.missing_or_invalid` | API Token 缺失或无效：重新生成并填写 |
| 401 | 10103 | `email.auth.invalid_token_type` | Token 类型不对：需要账号级 Token，不是用户级或 zone 级 |
| 403 | 10102 | `email.auth.missing_permission` | Token 权限不足：补上 Email Sending 权限 |
| 403 | 10105 | `email.service_not_enabled` | 账号未开通 Email Sending |
| 403 | 10203 | `email.sending_disabled` | 该 zone/账号的发信被禁用：检查域名是否已接入、是否被移除 |
| 429 | 10004 | `email.rate_limited` | 触发限流：降低发送频率，或申请提额 |
| 500 | 10002 | `email.internal_error` | Cloudflare 内部错误：稍后重试 |
| 503 | 10100 | `email.auth.service_unavailable` | 认证服务不可用：稍后重试 |

常见现象与原因：

- **测试邮件报 403，但 Token 看起来没问题** —— 发信域名还没 onboard 完成，此时只能发到已验证的目标地址。
- **`Email service not configured`** —— 选了 Cloudflare 但 Account ID 或发件人邮箱为空；或选了 SMTP 但没配 `smtp_host`。
- **保存报 token is required** —— 首次切换到 Cloudflare 时必须填 Token；已存过 Token 后再保存可以留空。
- **`email provider must be smtp or cloudflare`** —— `email_provider` 传了其它值，或数据库里存在历史脏值。
- **邮件进了垃圾箱** —— 检查 `cf-bounce` 子域上的 SPF/DKIM 记录是否生效，以及 DMARC 策略是否过严。

---

## 7. 验证与回滚

### 验证

```bash
# 后端
cd backend
go build ./...
go vet ./internal/service/... ./internal/handler/...
go test -tags=unit -count=1 -run 'Email|Cloudflare|Provider|Setting|SMTP' ./internal/service/ ./internal/handler/admin/

# 前端
cd frontend
pnpm install --frozen-lockfile
pnpm typecheck
pnpm check:i18n
pnpm build
```

手工验证：切换渠道后保存 → 刷新页面确认单选框与配置卡片回显正确 → 发送测试邮件 → 收到邮件。
再切回 SMTP 并发一封，确认原通道未受影响。

### 回滚

三种粒度，任选：

1. **只停用（推荐，无需发版）**：后台把「邮件发送渠道」改回 SMTP，Cloudflare 配置留在库里不生效。
2. **清空配置**：把 `email_provider` 设为空或 `smtp`，并清空 `cloudflare_api_token`。
3. **代码回滚**：本功能不涉及数据库迁移，回退代码后新设置键留在 `settings` 表里成了孤儿数据，
   不影响启动与运行，需要清理时手动删除即可。

Cloudflare 侧如需停用，进入 **Email Sending** > 选择域名 > **Settings** > **Remove Domain**，
会一并删除 `cf-bounce` 子域上的 MX/SPF/DKIM 与 `_dmarc` 记录（根域名上的 Email Routing 记录不受影响）。

---

## 8. 兼容性说明

- **不影响 SMTP**：`SendEmailWithConfig` 与原有 SMTP 配置链路未做任何改动，未设置 `email_provider` 的存量部署行为完全不变。
- **无数据库迁移**：全部通过 `settings` 表的新键实现，升级不需要跑迁移，回滚也不需要。
- **前端下拉框只渲染两种渠道**：读取设置时会把 `email_provider` 归一到 `smtp` 或 `cloudflare`，
  历史脏值不会让单选框变成「无选中」状态。
- **新增依赖**：`github.com/cloudflare/cloudflare-go/v6 v6.10.0`（`go.mod` / `go.sum`）。
