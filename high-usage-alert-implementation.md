# 高消费用户余额预警通知 - 实现方案

## 1. 功能概述

系统每日定时扫描用户使用情况，识别当日消费超过 $500 且当前余额不足 $200 的用户，自动通过 Telegram Bot 或微信（企业微信 Webhook）发送预警通知给管理员。

---

## 2. 触发条件

同时满足以下两个条件时触发通知：

| 条件 | 说明 |
|------|------|
| 当日消费 ≥ $500 | 基于 `usage_logs` 表 `total_cost` 字段，按 `user_id` 聚合当日（UTC 0:00 起）总消费 |
| 当前余额 < $200 | 基于 `users` 表 `balance` 字段 |

阈值可配置（存入系统设置表），默认值：
- `high_usage_alert_daily_threshold`: 500 (USD)
- `high_usage_alert_balance_threshold`: 200 (USD)

---

## 3. 数据查询方案

### 3.1 SQL 查询逻辑

```sql
SELECT 
    u.id AS user_id,
    u.email,
    u.username,
    u.balance,
    COALESCE(SUM(ul.total_cost), 0) AS today_cost
FROM users u
INNER JOIN usage_logs ul ON ul.user_id = u.id
WHERE ul.created_at >= DATE_TRUNC('day', NOW() AT TIME ZONE 'UTC')
  AND u.balance < :balance_threshold
GROUP BY u.id, u.email, u.username, u.balance
HAVING COALESCE(SUM(ul.total_cost), 0) >= :daily_threshold
ORDER BY today_cost DESC;
```

### 3.2 利用已有聚合表优化

项目已有 `DashboardAggregationService` 产出的按小时/按日聚合数据。如果当日聚合数据延迟（增量聚合间隔），可降级为直接查 `usage_logs` 表带索引扫描。

`usage_logs` 已有索引 `(user_id, created_at)`，性能可接受。

---

## 4. 通知渠道实现

### 4.1 Telegram Bot 通知

**实现方式**: 调用 Telegram Bot API `sendMessage`

**所需配置**:
| 配置项 | 说明 |
|--------|------|
| `alert_telegram_enabled` | 是否启用 TG 通知 |
| `alert_telegram_bot_token` | Bot Token（从 @BotFather 获取） |
| `alert_telegram_chat_id` | 目标聊天 ID（群组或个人） |

**API 调用**:
```
POST https://api.telegram.org/bot{token}/sendMessage
Content-Type: application/json

{
  "chat_id": "{chat_id}",
  "parse_mode": "Markdown",
  "text": "⚠️ *高消费余额预警*\n\n以下用户当日消费超过 $500 且余额不足 $200：\n\n| 用户 | 今日消费 | 当前余额 |\n|------|---------|----------|\n| user@example.com | $523.45 | $178.20 |\n| user2@example.com | $612.00 | $95.30 |\n\n📅 检查时间: 2026-07-28 00:05 UTC"
}
```

**消息格式（Markdown）**:
```
⚠️ *高消费余额预警*

以下用户当日消费超过 $500 且余额不足 $200：

👤 user@example.com
   今日消费: $523.45 | 余额: $178.20

👤 user2@example.com
   今日消费: $612.00 | 余额: $95.30

📅 2026-07-28 00:05 UTC
🔗 [管理后台](https://your-domain.com/admin/users)
```

### 4.2 企业微信 Webhook 通知

**实现方式**: 调用企业微信群机器人 Webhook

**所需配置**:
| 配置项 | 说明 |
|--------|------|
| `alert_wechat_enabled` | 是否启用企微通知 |
| `alert_wechat_webhook_url` | Webhook 地址（从企微群机器人获取） |

**API 调用**:
```
POST {webhook_url}
Content-Type: application/json

{
  "msgtype": "markdown",
  "markdown": {
    "content": "## ⚠️ 高消费余额预警\n\n以下用户当日消费超过 $500 且余额不足 $200：\n\n> 👤 user@example.com\n> 今日消费: <font color=\"warning\">$523.45</font> | 余额: <font color=\"warning\">$178.20</font>\n\n> 👤 user2@example.com\n> 今日消费: <font color=\"warning\">$612.00</font> | 余额: <font color=\"warning\">$95.30</font>\n\n检查时间: 2026-07-28 00:05 UTC"
  }
}
```

### 4.3 通知频率控制

- 默认每日检查 2 次（可配置 cron 表达式）：`0 0,12 * * *`（UTC 0:00 和 12:00）
- 同一用户 24 小时内不重复告警（Redis key 去重: `alert:high_usage:{user_id}:{date}`）
- 单次通知最多包含 20 个用户，超出则分批发送

---

## 5. 后端实现细节

### 5.1 新增文件

```
backend/internal/service/high_usage_alert_service.go     # 核心服务
backend/internal/pkg/notify/telegram.go                  # TG Bot 客户端
backend/internal/pkg/notify/wechat_work.go               # 企微 Webhook 客户端
backend/internal/pkg/notify/notify.go                    # 通知接口定义
```

### 5.2 服务结构

```go
// backend/internal/service/high_usage_alert_service.go

type HighUsageAlertService struct {
    usageLogRepo    UsageLogRepository
    userRepo        UserRepository
    settingService  *SettingService
    notifiers       []notify.Notifier
    lockCache       LeaderLockCache
    db              *sql.DB
    instanceID      string

    parentCtx       context.Context
    parentCancel    context.CancelFunc
    wg              sync.WaitGroup
}

// HighUsageAlertSettings 存入系统设置表
type HighUsageAlertSettings struct {
    Enabled              bool    `json:"enabled"`
    CronExpr             string  `json:"cron_expr"`              // 默认 "0 0,12 * * *"
    DailyThresholdUSD    float64 `json:"daily_threshold_usd"`    // 默认 500
    BalanceThresholdUSD  float64 `json:"balance_threshold_usd"`  // 默认 200
    TelegramEnabled      bool    `json:"telegram_enabled"`
    TelegramBotToken     string  `json:"telegram_bot_token"`
    TelegramChatID       string  `json:"telegram_chat_id"`
    WechatEnabled        bool    `json:"wechat_enabled"`
    WechatWebhookURL     string  `json:"wechat_webhook_url"`
}

// AlertUser 扫描结果
type HighUsageAlertUser struct {
    UserID    int64
    Email     string
    Username  string
    Balance   float64
    TodayCost float64
}
```

### 5.3 通知接口抽象

```go
// backend/internal/pkg/notify/notify.go

package notify

type Notifier interface {
    Name() string
    Send(ctx context.Context, message *Message) error
}

type Message struct {
    Title   string
    Content string        // Markdown 格式
    Users   []AlertUser   // 被预警的用户列表
}

type AlertUser struct {
    Email     string
    Username  string
    Balance   float64
    TodayCost float64
}
```

### 5.4 定时任务模式

复用项目已有的 `robfig/cron` + Redis Leader Lock 模式（参考 `OpsScheduledReportService`）：

```go
func (s *HighUsageAlertService) Start() {
    // 1. 启动后台 goroutine，每分钟 tick
    // 2. 解析 cron 表达式，判断是否到达触发时间
    // 3. 获取 Redis 分布式锁，确保多实例只执行一次
    // 4. 执行扫描 + 通知
}

func (s *HighUsageAlertService) runCheck(ctx context.Context) {
    // 1. 读取配置
    // 2. 查询符合条件的用户
    // 3. 过滤 24h 内已通知的用户（Redis 去重）
    // 4. 构造消息，分别调用已启用的 notifier
    // 5. 记录发送结果
}
```

### 5.5 Wire 依赖注入

在 `backend/internal/service/wire.go` 中注册：

```go
func ProvideHighUsageAlertService(
    usageLogRepo UsageLogRepository,
    userRepo UserRepository,
    settingService *SettingService,
    lockCache LeaderLockCache,
    db *sql.DB,
) *HighUsageAlertService {
    svc := NewHighUsageAlertService(usageLogRepo, userRepo, settingService, lockCache, db)
    svc.Start()
    return svc
}
```

---

## 6. 管理员配置界面

### 6.1 入口位置

在现有管理员设置页面 `/admin/settings` 中新增一个 Tab 或 Section：「高消费预警」

### 6.2 配置表单字段

```
┌─────────────────────────────────────────────────┐
│ 高消费余额预警设置                                │
│ ─────────────────────────────────────────────── │
│                                                 │
│ [✓] 启用高消费预警                               │
│                                                 │
│ 触发条件:                                        │
│   当日消费超过: [___500___] USD                   │
│   且余额低于:   [___200___] USD                   │
│                                                 │
│ 检查频率: [0 0,12 * * *] (Cron 表达式)           │
│   提示: 默认每日 0:00 和 12:00 (UTC) 各检查一次   │
│                                                 │
│ ── 通知渠道 ──────────────────────────────────── │
│                                                 │
│ Telegram:                                       │
│   [✓] 启用 Telegram 通知                         │
│   Bot Token: [_________________________]        │
│   Chat ID:   [_________________________]        │
│   [测试发送]                                     │
│                                                 │
│ 企业微信:                                        │
│   [✓] 启用企业微信通知                            │
│   Webhook URL: [___________________________]    │
│   [测试发送]                                     │
│                                                 │
│                              [保存配置]          │
└─────────────────────────────────────────────────┘
```

### 6.3 API 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/admin/settings/high-usage-alert` | 获取预警配置 |
| PUT | `/admin/settings/high-usage-alert` | 更新预警配置 |
| POST | `/admin/settings/high-usage-alert/test` | 测试通知发送 |
| POST | `/admin/settings/high-usage-alert/check-now` | 立即执行一次检查 |

---

## 7. 实现步骤

### Phase 1: 通知基础设施
1. 创建 `backend/internal/pkg/notify/` 包，实现 `Notifier` 接口
2. 实现 `TelegramNotifier`（调用 Bot API sendMessage）
3. 实现 `WechatWorkNotifier`（调用企微 Webhook）
4. 单元测试

### Phase 2: 预警服务
1. 创建 `HighUsageAlertService`，实现定时扫描逻辑
2. 新增 Repository 方法: `GetHighUsageUsers(ctx, dailyThreshold, balanceThreshold) ([]HighUsageAlertUser, error)`
3. 集成 Redis 去重 + Leader Lock
4. 注册到 Wire DI
5. 集成测试

### Phase 3: 管理配置
1. 后端: 新增设置读写 handler + 路由
2. 前端: 在 Settings 页面新增预警配置 Section
3. 「测试发送」按钮调用 `/test` 接口验证通道连通性
4. 「立即检查」按钮调用 `/check-now` 接口手动触发

---

## 8. 关键设计决策

| 决策点 | 选择 | 理由 |
|--------|------|------|
| 定时任务模式 | robfig/cron + Redis Lock | 与项目现有模式一致（OpsScheduledReportService） |
| 通知抽象 | Notifier 接口 | 方便后续扩展（钉钉、Slack 等） |
| 消费统计来源 | 直接查 usage_logs | 聚合表有延迟，实时性更重要；已有索引 `(user_id, created_at)` |
| 去重方式 | Redis SET with TTL 24h | 简单高效，无需额外数据表 |
| 配置存储 | 系统设置表 JSON | 与项目其他设置一致（如 UpstreamBillingProbeSettings） |
| 通知限流 | 同用户 24h 内仅告警一次 | 避免轰炸，管理员只需关注增量 |

---

## 9. 安全考虑

- Telegram Bot Token 和企微 Webhook URL 加密存储，API 返回时脱敏
- 通知内容仅包含邮箱前缀 + 金额，不含敏感个人信息
- 测试发送接口需管理员权限 + 频率限制（1 分钟 1 次）
- Webhook URL 校验为合法 HTTPS 地址

---

## 10. 监控与可观测性

- 每次检查记录日志: 扫描用户数、命中用户数、通知发送状态
- 通知失败时记录错误，连续失败 3 次后暂停该通道并通过邮件通知管理员
- 在 Ops Dashboard 中可看到预警执行历史（复用 audit_log）
