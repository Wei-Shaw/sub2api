# 29 元订阅池公网 Key MVP 可用性排查

## 背景

用户反馈部分 29 元套餐用户无法使用，需要确认：

- 29 元套餐每天 19 USD 的限额是否真实生效。
- 当前 29 元套餐用户的 Key 是否能从公网最小请求成功。
- 失败是否来自订阅限额、Key 状态、公网路由、Sub2API 认证，还是上游账号池。

本次未输出完整 API Key，只使用脱敏 preview。

## 结论

29 元套餐本身配置正确，19 USD 日限额真实参与请求前校验。

当前 15 个正式 29 元套餐用户：

- `/v1/models` 公网请求：15/15 成功，HTTP 200。
- `/v1/chat/completions` 最小真实请求：15/15 成功，HTTP 200，返回 `pong`。
- 所有正式用户订阅均为 `active`，未过期。
- 所有正式用户 Key 均为 `active`，未删除。
- 所有正式用户当前 daily usage 均低于 19 USD，未触发套餐日限额。

因此，“29 元套餐用户整体无法使用”不成立。当前最小公网 MVP 可用。

## 套餐配置

Sub2API 中 29 元订阅池对应：

```text
group: codex-pool
subscription_type: subscription
status: active
daily_limit_usd: 19.00000000
plan: 29 元订阅池
price: 29.00
validity_days: 30
```

代码路径：

```text
backend/internal/server/middleware/api_key_auth.go
backend/internal/service/subscription_service.go
backend/internal/service/user_subscription.go
```

请求进入 API Key 中间件后：

1. 根据 Key 找到用户和 group。
2. 如果 group 是 subscription 类型，加载 active subscription。
3. 调用 `ValidateAndCheckLimits(subscription, group)`。
4. 如果 daily/weekly/monthly 超限，返回 `USAGE_LIMIT_EXCEEDED`，HTTP 429。

`codex-pool` 只配置了 daily limit，没有 weekly/monthly limit。因此 weekly/monthly usage 即使超过 19，也不会被拦截。

## 当前 29 元用户订阅状态

正式用户数量：15。

所有用户：

- 有 `codex-pool` active subscription。
- 有 active API Key。
- daily usage 均低于 19 USD。

当前 daily 使用最高的用户：

```text
17371571728@phone.com daily_used=2.807063 / 19
13584052801@phone.com daily_used=0.689302 / 19
18367290091@phone.com daily_used=0.081794 / 19
```

这说明当前没有任何 29 元用户因为 Sub2API 的 19 USD 日限额被挡。

## 公网最小请求验证

验证目标：

```text
https://aaccx.pw/v1/models
https://aaccx.pw/v1/chat/completions
```

验证范围：

```text
codex-pool 下 15 个正式用户 active Key
不包含 sub2api-test-local@example.com
```

`/v1/models` 结果：

```text
total=15
ok=15
failed=0
HTTP 200
modelCount=10
firstModel=gpt-5.5
```

`/v1/chat/completions` 最小真实请求结果：

```text
total=15
ok=15
failed=0
HTTP 200
model=gpt-5.5
content=pong
```

请求后 `usage_logs` 有新增记录，说明真实调用链路和用量写入都可用。

## 近 24 小时错误分类

`ops_error_logs` 近 24 小时统计：

```text
401 api_error/auth: 79
429 rate_limit_error/upstream: 6
502 upstream_error/upstream: 2
```

没有看到：

```text
USAGE_LIMIT_EXCEEDED
DAILY_LIMIT_EXCEEDED
SUBSCRIPTION_NOT_FOUND
SUBSCRIPTION_INVALID
```

这说明近期失败主要不是 29 元套餐日限额导致。

## 真实失败类型

### 1. 无效 Key 或未带 Key

日志中大量 401：

```text
Invalid API key
API key is required in Authorization header
```

代表用户客户端可能：

- 没有配置 Authorization header。
- 仍使用未迁移的旧 Key。
- Key 复制错了。
- 使用了其他平台生成的 Key，而不是 Sub2API 中导入或生成的 Key。

日志中出现的无效 Key prefix 示例：

```text
sk-41a40...
sk-d445e...
```

这些请求没有匹配到 Sub2API 中的 `api_keys`，所以不会进入套餐限额判断。

### 2. 上游池临时限流或不可用

少量 429/502：

```text
Upstream rate limit exceeded, please retry later
Upstream service temporarily unavailable
```

示例：

```text
13584052801@phone.com 429 upstream rate_limit_error
19814722044@phone.com 429 upstream rate_limit_error
15776812883@phone.com 502 upstream_error, requested_model=gpt-5.1-codex
```

这些不是 Sub2API 用户日限额。它们发生在上游账号池或模型路由层。

## 判断

如果用户说“无法使用”，应先让用户提供：

- Base URL。
- 模型名。
- 完整错误码和 HTTP status。
- Key preview 前 8 位和后 6 位，不能发完整 Key 到公开聊天。

按错误码判断：

- `401 API_KEY_REQUIRED`：客户端没带 Key。
- `401 INVALID_API_KEY`：Key 不在 Sub2API 数据库里，通常是复制错、用旧未迁移 Key、或用了其他服务 Key。
- `429 USAGE_LIMIT_EXCEEDED`：才是 Sub2API 套餐限额。
- `429 rate_limit_error / Upstream rate limit exceeded`：上游账号池临时限流。
- `502 upstream_error`：上游服务或模型路由临时不可用。

## 推荐下一步

1. 给用户一条标准配置说明：

```text
Base URL: https://aaccx.pw/v1
Header: Authorization: Bearer <你的 Sub2API API Key>
测试接口: GET https://aaccx.pw/v1/models
```

2. 如果用户报 401，让他发 Key preview，不要发完整 Key。
3. 如果用户报 429，区分是 `USAGE_LIMIT_EXCEEDED` 还是 `Upstream rate limit exceeded`。
4. 对上游 429/502，继续排查 CLIProxyAPI 账号池状态和具体模型路由，不要误判为用户 19 USD 日限额。
5. 可以新增一个管理员排查脚本：输入 Key preview 或手机号，输出 Key 状态、订阅状态、daily remaining、最近错误日志和最近成功 usage。
