# 通知 Webhook

> English version: [NOTIFICATION_WEBHOOK.md](NOTIFICATION_WEBHOOK.md)

把系统通知投递到自建 HTTP 端点，交给你自己的服务消费。邮件与 Webhook 是两条独立通道，可以按事件分别开关——包括把邮件完全关掉，只走 Webhook。

配置入口：后台 → 设置 → 邮件 → **通知渠道 / Webhook**。

## 通道解析规则

每个事件的最终通道由三层叠加决定：

1. **默认**：邮件开、Webhook 关。升级后行为不变。
2. **全局开关**：`webhook.enabled` 是**硬关闸**——关闭时所有事件的 Webhook 一律不推，不必逐个取消；但**打开它不会让任何事件自动开始推送**，每个事件仍需显式勾选。
3. **按事件覆盖**：单独指定 `email` / `webhook` 开关，以及可选的独立端点地址和报文模板。这两个端点字段未指定时继承全局值；签名密钥始终用全局那一个。`email` / `webhook` 两个开关未指定时用的是上面第 1 条的默认值（开 / 关），不受全局开关影响。

任一事件的 Webhook 生效还要求端点 URL 非空。

## 默认报文

`POST`，`Content-Type: application/json; charset=utf-8`：

```json
{
  "schema_version": 1,
  "event": "content_moderation.violation_notice",
  "event_label": "Risk control violation notice",
  "category": "risk_control",
  "audience": "user",
  "locale": "zh",
  "site_name": "Sub2API",
  "delivery_id": "9f86d081...",
  "occurred_at": "2026-07-26T06:39:58Z",
  "timestamp": "2026-07-26T06:40:00Z",
  "recipient": {
    "user_id": 1234,
    "username": "alice",
    "email": "alice@example.com"
  },
  "source": { "type": "content_moderation", "id": "99" },
  "data": { "moderation_category": "...", "violation_count": "3" }
}
```

- `audience` 为 `user` 时，`recipient` 供消费方定位到具体用户；为 `admin` 时是运维通知，`recipient.email` 可能为空（管理员邮箱列表清空后仍会推送）。
- `schema_version` 表示信封版本；`event` 是事件级 `data` 结构的判别字段。消费端应使用结构化字段，而不是依赖发送方生成的展示文案。
- 默认报文不含邮件模板、已渲染邮件内容或 HTML。`occurred_at` 是源事件发生时间，`timestamp` 是本次投递报文构建时间。

### `content_moderation.cyber_policy_notice` 的 data

Cyber policy 通知只包含原有通知字段和上游诊断信息，不包含用户原始输入：

```json
{
  "triggered_at": "2026-07-26T06:39:58Z",
  "model": "gpt-5.6-sol",
  "group_name": "default",
  "upstream_message": "This content was flagged ..."
}
```

`triggered_at`、`model`、`group_name` 和 `upstream_message` 来源于原有 Cyber 通知。`upstream_message` 是经过脱敏和长度限制的上游诊断文本，可能包含响应正文或 usage 信息；消费端应将其作为不透明诊断信息处理，不要从中解析用户输入。

### `ops.alert` 的 data

`ops.alert` 的 `data` 直接使用已有的源 DTO，不做邮件字段投影：

```json
{
  "rule": { "id": 45, "name": "错误率过高", "window_minutes": 5 },
  "alert": { "id": 123, "severity": "P1", "status": "firing", "metric_value": 6.91, "threshold_value": 5.0, "dimensions": { "platform": "openai", "group_id": 12 } }
}
```

`source.id` 是告警事件在信封层的标准标识，`data.alert.id` 是同一个 ID 在源对象中的具名字段。`dimensions` 及其中各键都可能缺失；指标值不可用时可为 `null`。告警严重级别使用规则原始词汇（`P0`、`P1` 等），不同于通知最低级别使用的 `critical` / `warning` / `info`。

`data.rule` 和 `data.alert` 是 Ops API 使用的完整 DTO，不只限于示例列出的字段；后续版本可以增加字段。投递时 `alert.email_sent` 为 `false`，因为它只表示邮件是否实际成功；`rule.notify_email` 同样只影响邮件，所以 Webhook 投递时它可以是 `false`。

### `ops.scheduled_report` 的 data

定时报表是运行时聚合产物，所以 `data` 带报告运行元信息和生成它所用的原始聚合结果，绝不带邮件 HTML：

```json
{
  "report": {
    "name": "日报",
    "type": "daily_summary",
    "schedule": "0 9 * * *",
    "start_time": "2026-07-26T09:00:00Z",
    "end_time": "2026-07-27T09:00:00Z"
  },
  "overview": { "request_count_total": 1234, "sla": 0.9995 }
}
```

按 `report.type` 恰好带一个聚合结果：日报/周报用已有的 `OpsDashboardOverview`（`overview`），错误摘要用 `{ "total": 42 }`（`error_digest`），账号健康报表用已有的 `OpsAccountAvailability`（`account_availability`）。`account_availability` 使用 snake_case 的 `group`、`accounts`、`collected_at`，其中 `accounts` 是以账号 ID 的十进制字符串为键的对象。错误摘要不会包含逐请求日志、用户身份或客户端 IP；如需明细，请用 `report.start_time` 和 `report.end_time` 通过 Ops 错误日志 API 查询。数值保持为数值；`report_html`、CSS 显示控制、`"-"` 占位值、带百分号的展示字符串等邮件专属字段不会发送。

## 自定义报文模板

填了模板就用模板，直接产出接收方期望的报文结构：

```json
{"event":"{{event}}","source_id":"{{source_id}}","occurred_at":"{{occurred_at}}","rule":"{{rule_name}}"}
```

- 占位符 = 该事件的邮件占位符 ∪ Webhook 专属占位符：
  `event` `event_label` `event_category` `audience` `locale` `user_id` `source_type` `source_id` `occurred_at` `timestamp`
- 值按 JSON 字符串转义后代入，引号、反斜杠、换行都是安全的。
- 保存时会校验：占位符必须属于该事件，且模板必须渲染成**合法 JSON**。全局模板会对所有事件做一遍校验，所以全局模板只能用各事件共有的占位符。
- 自定义模板是显式替代默认报文的能力，可以引用该事件的模板变量，但不会拿到邮件模板输出或样例预览值。为了兼容已保存配置，`rendered_title`、`rendered_text` 暂时仍可写入模板，但固定渲染为空字符串；请改为使用结构化字段。

## 签名校验

每次投递都会签名。启用 Webhook 时密钥会自动生成，从设置页复制到你的接收端即可。每个请求带：

| 请求头 | 含义 |
| --- | --- |
| `X-Sub2Api-Signature` | `hex(HMAC-SHA256(secret, timestamp + "." + body))` |
| `X-Sub2Api-Timestamp` | Unix 秒 |
| `X-Sub2Api-Event` | 事件名 |
| `X-Sub2Api-Delivery` | 投递 ID，可用于消费侧去重 |

投递固定用 `POST` + JSON body。不提供方法选择，也不支持自定义请求头——签名本身就是鉴权手段。

目标地址要求 `http` 或 `https`。**响应体一律丢弃**：接收方返回的任何内容都不会进入调用方、日志或后台界面。

## 投递语义

- **尽力而为**：后台 goroutine 投递，不阻塞产生通知的请求。
- **重试**：默认 2 次，指数退避（1s、2s）。**显式设为 0 表示不重试**。5xx 和 429 重试；其他 4xx 不重试，同样的报文重发没意义。
- **并发上限**：名额在创建投递协程**之前**以非阻塞方式预留，因此同时在途的协程与 HTTP 请求都不超过 32。名额用尽时立即**丢弃并记 warn 日志**（含 `event`、`delivery_id`、`reason=slots_exhausted`，便于日志聚合告警），不排队、不阻塞产生通知的请求。
- **失败即丢弃**：持久投递标记只在成功后写入，失败**不会**再次调度。`email_sent` 保持原语义：只有邮件实际发送成功才会写入；Webhook 成功或入队都不会改变它。需要不丢消息请自行监控上述日志。
- **管理员事件的重试预算按事件计**：三个管理员 fan-out 场景（运维告警 `ops.alert`、账号配额 `account.quota_alert`、定时报表 `ops.scheduled_report`）都会先独立派发一次 Webhook，随后的邮箱循环是**纯邮件**的。因此一个事件产生的 HTTP 请求数不超过 `max_retries + 1`，与配置了几个管理员邮箱无关，首次派发失败也不会被每个收件人各重启一次。
- **定时报表可以纯 Webhook**：报表的邮件收件人列表为空时不再跳过——只要 `ops.scheduled_report` 的 Webhook 已开启就照常推送。此时没有收件人邮箱可用于解析语言，信封使用默认语言（`en`）；发给各邮箱的邮件仍使用各自的语言。
  > 注意：ops 设置里的「报表启用」（`report.enabled`）是**报表任务总闸**，不是邮件通道开关——关掉它连 Webhook 也不会推。要「只推 Webhook 不发邮件」，请保持它开启，然后在本页把 `ops.scheduled_report` 的邮件通道关掉。
- **去重**：投递键含通道维度，邮件成功不会把 Webhook 标记成已投递。管理员事件按事件去重而非按收件人——配了 3 个管理员邮箱只推 1 条，不会刷屏。用户事件按收件人去重，每人一条。
- **尽力投递，不保证不丢或不重**：上述去重是**单实例内**的（进程内在途锁 + 持久标记）。投递失败或并发名额耗尽时可能丢失，多副本部署时同一事件也可能被重复推送；**接收端必须按 `X-Sub2Api-Delivery` 去重**。
- **不跟随重定向**：3xx 按失败处理。

运维告警的 Webhook **不受邮件收件人列表和邮件发信频率限制的门控**：邮件限额耗尽不会影响 Webhook 推送，两条通道各自独立。

## 事件清单

| 事件 | 受众 | 说明 |
| --- | --- | --- |
| `auth.verify_code` | user | 注册 / 绑定邮箱 / OAuth 补全 / TOTP 校验 |
| `auth.password_reset` | user | 密码重置链接 |
| `notification_email.verify_code` | user | 额外通知邮箱验证 |
| `subscription.purchase_success` | user | 订阅开通 / 续期成功 |
| `subscription.expiry_reminder` | user | 订阅到期提醒（可退订） |
| `balance.low` | user | 余额低于阈值（可退订） |
| `balance.recharge_success` | user | 充值成功 |
| `account.quota_alert` | admin | 上游账号配额触线 |
| `content_moderation.violation_notice` | user | 风控命中告知 |
| `content_moderation.account_disabled` | user | 风控自动封禁告知 |
| `content_moderation.cyber_policy_notice` | user | 网信政策拦截告知 |
| `ops.alert` | admin | 运维告警规则触发 |
| `ops.scheduled_report` | admin | 定时运营报表 |

## 需要注意的语义变化

- **退订对所有通道生效**。用户对某个可选事件退订后，Webhook 也不再推送——退订表达的是「别拿这件事烦我」，不是「别发邮件」。
- **Ops 告警规则的邮件开关仍然只管邮件**。规则的 `notify_email`、告警收件人列表、发信频率限制、`Alert.Enabled` 只影响邮件。中央 `ops.alert` Webhook 开关对所有启用的规则生效，包括 `notify_email=false` 的规则；但仍受统一通知最低级别和静默规则约束。最低级别和静默只阻止投递，不阻止告警事件创建或在历史中展示。
