# sub2api OpenAI/Codex 目标组路由设计

## 1. 概述

在 sub2api 的 OpenAI/Codex 网关链路中增加请求分类与目标账号组约束，实现：
- 自动将请求分类为"耗尽组"或"非耗尽组"
- 调度时只从目标组中选择账号
- 保留现有 `previous_response_id` / session sticky / load balance 语义
- 支持 `-Sys` 后缀模型别名，自动补充工具调用并路由到耗尽组
- 在 `/v1/models` 端点中为每个模型生成 `-Sys` 别名版本

## 2. 耗尽组判定规则

### 2.1 判定信号

**OAuth 账号**：
- `extra.codex_7d_used_percent >= 100` **或** `extra.codex_primary_used_percent >= 100`
- 两个信号**同等重要**，任意一个 >= 100 即判定为耗尽
- 注意：使用率字段为字符串，需转为 float 比较

**API Key 账号**：
- `account.IsQuotaExceeded()`（检查 `quota_limit`、`quota_daily_limit`、`quota_weekly_limit`）

**明确排除**：
- `RateLimitResetAt`（rate limit 冷却）
- `OverloadUntil`（529 过载冷却）
- `TempUnschedulableUntil`（临时不可调度）

### 2.2 辅助方法

在 `account.go` 中新增：

```go
// IsExhausted 判定账号是否处于配额耗尽状态。
// OAuth 账号检查 Codex 使用率，API Key 账号检查 IsQuotaExceeded。
func (a *Account) IsExhausted() bool {
    if a == nil {
        return false
    }
    switch a.Type {
    case AccountTypeOAuth:
        return a.isOAuthExhausted()
    case AccountTypeAPIKey:
        return a.IsQuotaExceeded()
    default:
        return false
    }
}

func (a *Account) isOAuthExhausted() bool {
    if a.Extra == nil {
        return false
    }
    pct7d, _ := a.Extra["codex_7d_used_percent"].(string)
    primaryPct, _ := a.Extra["codex_primary_used_percent"].(string)
    if parseFloatPct(pct7d) >= 100 {
        return true
    }
    if parseFloatPct(primaryPct) >= 100 {
        return true
    }
    return false
}

func parseFloatPct(s string) float64 {
    f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
    return f
}
```

## 3. 请求分类逻辑

### 3.1 分类规则

在 `openai_gateway_handler.go` 的 `Responses()` 中，解析请求 body 后：

- 检查 `input` 数组中最后一项的 `type` 字段
- 若最后一项为 `"function_call_output"` → 标记为 `TargetGroup: Exhausted`
- 否则（user message / assistant message / tool_call / item_reference 等结尾）→ 标记为 `TargetGroup: Active`

### 3.2 数据结构

在 `openai_account_scheduler.go` 中新增：

```go
type AccountTargetGroup string

const (
    AccountTargetGroupAny       AccountTargetGroup = ""
    AccountTargetGroupActive    AccountTargetGroup = "active"
    AccountTargetGroupExhausted AccountTargetGroup = "exhausted"
)

type OpenAIAccountScheduleRequest struct {
    GroupID            *int64
    SessionHash        string
    StickyAccountID    int64
    PreviousResponseID string
    RequestedModel     string
    RequiredTransport  OpenAIUpstreamTransport
    ExcludedIDs        map[int64]struct{}
    TargetGroup        AccountTargetGroup  // 新增：目标账号组约束
}
```

### 3.3 分类辅助函数

在 `openai_tool_continuation.go` 中新增：

```go
// GetRequestTargetGroup 根据请求 input 最后一项类型判定目标组。
// function_call_output 结尾 → Exhausted，否则 → Active。
func GetRequestTargetGroup(reqBody map[string]any) AccountTargetGroup {
    if reqBody == nil {
        return AccountTargetGroupActive
    }
    input, ok := reqBody["input"].([]any)
    if !ok || len(input) == 0 {
        return AccountTargetGroupActive
    }
    lastItem, ok := input[len(input)-1].(map[string]any)
    if !ok {
        return AccountTargetGroupActive
    }
    itemType, _ := lastItem["type"].(string)
    if itemType == "function_call_output" {
        return AccountTargetGroupExhausted
    }
    return AccountTargetGroupActive
}
```

## 4. `-Sys` 模型别名与自动工具调用

### 4.1 模型别名识别

在 `openai_model_mapping.go` 中新增：

```go
// IsSysModel 判断模型名是否以 -Sys 结尾（大小写不敏感）。
func IsSysModel(model string) bool {
    return strings.HasSuffix(strings.ToLower(model), "-sys")
}

// StripSysSuffix 去掉 -Sys 后缀，返回实际发往上游的模型名。
func StripSysSuffix(model string) string {
    if !IsSysModel(model) {
        return model
    }
    return model[:len(model)-4]
}
```

### 4.2 自动补充工具调用

**触发条件**（全部满足）：
1. 请求模型名以 `-Sys` 结尾
2. 请求 `input` 数组最后一项的 `type == "message"` 且 `role == "user"`

**"非工具结尾"的精确判定**：
- `input` 最后一项 `type == "message"` 且 `role == "user"` → 视为 user message 结尾，触发补全
- `input` 最后一项 `type == "function_call_output"` → 视为工具结尾，不触发补全
- `input` 最后一项 `type == "item_reference"` → 视为非工具结尾，触发补全
- 其他情况 → 不触发补全

**补全行为**：
在 `input` 数组末尾追加一对 `tool_call` + `function_call_output`：
```json
{ "type": "tool_call", "call_id": "sys_dummy", "name": "sys_status", "arguments": "{}" },
{ "type": "function_call_output", "call_id": "sys_dummy", "output": "ready" }
```

补全后，请求变为"工具结尾"，自然被分类到 `TargetGroup: Exhausted`。

### 4.3 上游模型映射

识别到 `-Sys` 模型后：
1. 先做 tool call 补全（若满足条件）
2. 将 `model` 字段替换为 `StripSysSuffix(model)` 的结果
3. 继续走正常调度流程

### 4.4 模型列表 API 扩展

在 `gateway_handler.go` 的 `Models()` 函数中：
- 获取当前返回的模型列表后，为每个模型生成一个带 `-Sys` 后缀的别名版本
- 例如：`gpt-5.4` → 额外返回 `gpt-5.4-Sys`
- 别名模型的 `DisplayName` 加 `(Sys)` 标记
- **去重规则**：若原列表中已存在同名 `-Sys` 模型（大小写不敏感），不重复生成
- **排序**：别名模型紧跟在原模型之后，保持原有模型顺序

## 5. 调度层目标组约束

### 5.1 核心原则

**Rate Limit 放宽**：
当 `TargetGroup == Exhausted` 时，耗尽账号几乎必然带有 `rate_limit_reset_at`（因为 429 会触发 rate limit）。如果严格按 `IsSchedulable()` 过滤，耗尽组永远为空。

因此，在目标组过滤时：
- `TargetGroup == Exhausted`：跳过 `RateLimitResetAt` 检查，保留其他检查
- `TargetGroup == Active`：使用完整的 `IsSchedulable()` 检查
- `TargetGroup == Any`：使用完整的 `IsSchedulable()` 检查

### 5.2 辅助方法

在 `account.go` 中新增：

```go
// IsSchedulableForTargetGroup 按目标组放宽可调度检查。
// Exhausted 组跳过 RateLimitResetAt 检查，避免耗尽账号被 rate limit 排除。
func (a *Account) IsSchedulableForTargetGroup(group AccountTargetGroup) bool {
    if a == nil {
        return false
    }
    if !a.IsActive() {
        return false
    }
    if !a.Schedulable {
        return false
    }
    if a.ExpiresAt != nil && time.Now().After(*a.ExpiresAt) {
        return false
    }
    if a.TempUnschedulableUntil != nil && time.Now().Before(*a.TempUnschedulableUntil) {
        return false
    }
    // Exhausted 组跳过 RateLimitResetAt 检查
    if group != AccountTargetGroupExhausted {
        if a.RateLimitResetAt != nil && time.Now().Before(*a.RateLimitResetAt) {
            return false
        }
    }
    if a.OverloadUntil != nil && time.Now().Before(*a.OverloadUntil) {
        return false
    }
    return true
}

// MatchesTargetGroup 判定账号是否属于目标组。
func (a *Account) MatchesTargetGroup(group AccountTargetGroup) bool {
    if group == AccountTargetGroupAny {
        return true
    }
    isExhausted := a.IsExhausted()
    return (group == AccountTargetGroupActive && !isExhausted) ||
           (group == AccountTargetGroupExhausted && isExhausted)
}
```

### 5.3 三层调度修改

#### 5.3.1 previous_response_id 层

在 `Select()` 方法中，找到粘住的账号后：
1. 检查 `account.MatchesTargetGroup(req.TargetGroup)`
2. 检查 `account.IsSchedulableForTargetGroup(req.TargetGroup)`
3. 检查模型支持、传输兼容
4. 若全部通过 → 返回该账号
5. 若**不匹配目标组** → **不清除** previous_response 缓存，仅跳过 sticky，回退到 session_hash 层
6. 若**不可调度**（expired/temp_unschedulable/overloaded）→ 清除缓存，回退到 session_hash 层

#### 5.3.2 session_hash 层

同样检查目标组匹配和放宽版可调度：
1. 检查 `account.MatchesTargetGroup(req.TargetGroup)`
2. 检查 `account.IsSchedulableForTargetGroup(req.TargetGroup)`
3. 若通过 → 返回该账号
4. 若**不匹配目标组** → **不清除** sticky 缓存，仅跳过 sticky，回退到 load_balance 层
5. 若**不可调度**（expired/temp_unschedulable/overloaded）→ 清除缓存，回退到 load_balance 层

**设计理由**：sticky 不匹配目标组是正常现象（如 Active 请求粘到耗尽账号），不应清除 sticky 标记。只有账号真正不可调度时才清除，避免后续同组请求丢失 sticky。

#### 5.3.3 load_balance 层

在 `selectByLoadBalance()` 的候选过滤中增加：
```go
// 目标组过滤
if req.TargetGroup != AccountTargetGroupAny {
    if !account.MatchesTargetGroup(req.TargetGroup) {
        continue
    }
}
// 放宽版可调度检查
if !account.IsSchedulableForTargetGroup(req.TargetGroup) {
    continue
}
```

### 5.4 无可用账号处理

- 若 `TargetGroup == Exhausted` 但无可用耗尽账号 → 返回 `429 No available accounts in target group`
- 若 `TargetGroup == Active` 但无可用非耗尽账号 → 返回 `503 No available accounts`
- **不回退**到另一组（硬性路由规则）

## 6. 数据流

```
客户端请求
    ↓
openai_gateway_handler.Responses()
    ├─ 解析 body → reqBody
    ├─ 提取 model, stream, previous_response_id
    ├─ 识别 -Sys 后缀？
    │   ├─ 是 → upstreamModel = StripSysSuffix(model)
    │   │       检查是否 user message 结尾？
    │   │       ├─ 是 → 追加 dummy tool_call + function_call_output
    │   │       └─ 否 → 不追加
    │   └─ 否 → upstreamModel = model
    ├─ GetRequestTargetGroup(reqBody) → TargetGroup
    └─ SelectAccountWithScheduler(..., TargetGroup)
            ↓
openai_account_scheduler.Select()
    ├─ previous_response_id 层
    │   └─ MatchesTargetGroup + IsSchedulableForTargetGroup
    │       ├─ 匹配 → 返回
    │       └─ 不匹配 → 清除 sticky，回退
    ├─ session_hash 层
    │   └─ MatchesTargetGroup + IsSchedulableForTargetGroup
    │       ├─ 匹配 → 返回
    │       └─ 不匹配 → 清除 sticky，回退
    └─ load_balance 层
        └─ 过滤：MatchesTargetGroup + IsSchedulableForTargetGroup
            └─ 按现有逻辑选号
            ↓
发往上游 OpenAI/Codex（使用 upstreamModel）
```

## 7. 错误处理与边界情况

### 7.1 目标组内无可用账号
- 不回退到另一组
- `TargetGroup == Exhausted` 无可用账号 → 返回 `429`，错误消息 `No available accounts in target group (exhausted)`
- `TargetGroup == Active` 无可用账号 → 返回 `503`，错误消息 `No available accounts in target group (active)`
- 错误响应格式与现有 OpenAI 兼容错误响应保持一致

### 7.2 sticky 账号不匹配目标组
- previous_response_id 层不匹配目标组 → **不清除** `previous_response_account_cache`，跳过 sticky，回退到 session_hash 层
- session_hash 层不匹配目标组 → **不清除** `session_account_sticky`，跳过 sticky，回退到 load_balance 层
- 只有当 sticky 账号**真正不可调度**（expired/temp_unschedulable/overloaded）时才清除对应缓存
- **设计理由**：sticky 不匹配目标组是正常现象（如 Active 请求粘到耗尽账号），不应清除 sticky 标记。只有账号真正不可调度时才清除，避免后续同组请求丢失 sticky。

**场景示例**：
```
请求1 (Active)    → 选中账号 A，session sticky = A
请求2 (Exhausted) → A 不匹配 Exhausted → 不清除 sticky，跳过 sticky，选耗尽账号 B
请求3 (Exhausted) → 选耗尽账号 B
请求4 (Active)    → sticky 仍是 A → A 匹配 Active → 命中 sticky，返回 A（正确恢复）
```

### 7.3 -Sys 模型但无对应真实模型

> 本节不属于本次设计范围，仅作为实现完成后的部署指引。

修改并测试通过后，将 sub2api 部署到用户 VPS：
- Host: 74.48.17.44
- Port: 22
- User: root
- 环境：Docker + 1Panel

## 8. 修改文件清单

| 文件 | 修改内容 |
|------|---------|
| `internal/service/account.go` | 新增 `IsExhausted()`, `IsSchedulableForTargetGroup()`, `MatchesTargetGroup()` |
| `internal/service/openai_tool_continuation.go` | 新增 `GetRequestTargetGroup()` |
| `internal/service/openai_model_mapping.go` | 新增 `IsSysModel()`, `StripSysSuffix()` |
| `internal/service/openai_account_scheduler.go` | 新增 `AccountTargetGroup` 类型，修改 `OpenAIAccountScheduleRequest`，修改 `Select()`, `selectBySessionHash()`, `selectByLoadBalance()` |
| `internal/service/openai_gateway_service.go` | 修改 `SelectAccountWithScheduler()` 签名，传入 `TargetGroup` |
| `internal/handler/openai_gateway_handler.go` | 新增请求分类逻辑、`-Sys` 模型处理、dummy tool call 补全 |
| `internal/handler/gateway_handler.go` | 修改 `Models()` 函数，增加 `-Sys` 别名模型 |
| `internal/pkg/openai/constants.go` | 可选：增加 `-Sys` 模型常量 |
