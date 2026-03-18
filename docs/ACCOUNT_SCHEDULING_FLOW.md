# Account Scheduling Flow（SelectAccountWithLoadAwareness）

本文档对应后端主调度入口 `SelectAccountWithLoadAwareness`，用于说明 Anthropic 网关的账号选择流程、亲和策略与关键过滤规则。

## 代码入口与引用

- 调度主入口：`backend/internal/service/gateway_service.go` 中的 `SelectAccountWithLoadAwareness`
- 亲和详情 API：`backend/internal/handler/admin/account_affinity_handler.go` 中的 `GetAffinityDetails`
- Admin 路由：`backend/internal/server/routes/admin.go`（`/api/v1/admin/accounts/:id/affinity-details`）

## 流程概览（修正版）

1. 解析输入参数  
`sessionHash`、`stickyAccountID`、`affinityClientID(metadata.user_id 提取)`、`affinityUserID(sub2api user id)`。

2. Claude Code 限制与分组降级  
先执行 `checkClaudeCodeRestriction`，必要时替换 `groupID`。

3. Legacy / Load-aware 分支  
若 `concurrencyService == nil || !LoadBatchEnabled` 走传统路径；否则走负载感知路径。

4. Layer 1（模型路由优先）
- 路由候选过滤（排除、平台、模型、配额、窗口费用、RPM）
- 路由范围内 sticky 优先
- 路由内按（优先级 > 负载 > 亲和数 > LRU）尝试获取槽位

5. Layer 1.3（pinned_users 预处理）  
仅做 `UpdateAffinity` 预热，不直接做调度决策。

6. Layer 1.4（客户端亲和调度）  
按亲和记录尝试命中；`allow_switch=false` 且无一票放行时尝试等待计划，否则返回 `ErrAffinityNoSwitch`。

7. Layer 1.5（粘性会话）  
仅在 `!affinityHit && routingAccountIDs==0` 时生效；先过 `shouldClearStickySession`。

8. Layer 2（负载感知选择）  
分层过滤：优先级 -> 亲和区(单维客户端) -> 最低负载 -> 最少亲和客户端 -> LRU。

9. Layer 3（兜底排队）  
按 `FallbackSelectionMode`（`last_used` 或 `random`）排序后返回等待计划。  
注意：此层不是“按最低负载”。

## 关键业务规则（2026-03）

### 无客户端 ID 时过滤亲和 Anthropic 账号

当 `metadata.user_id` 无法提取 `client_id`（即 `affinityClientID == ""`）时：

- 会直接过滤 **Anthropic 平台且开启客户端亲和** 的账号（覆盖 OAuth / SetupToken / API Key / Bedrock 等类型）
- 该规则只看平台与亲和开关，不再限定账号类型

设计目标：避免无客户端标识的请求误用客户端亲和账号。

## 亲和详情 API 契约

`GET /api/v1/admin/accounts/:id/affinity-details` 返回结构（后端已对齐前端）：

- `users[]`：包含 `user_id`、`user_email`、`client_count`、`is_pinned`、`clients[]`
- `total_users`
- `total_clients`
- `pinned_users`
