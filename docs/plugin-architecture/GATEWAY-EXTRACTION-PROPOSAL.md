# Gateway Extraction · Proposal（骨架草稿 v0）

> 状态：**草稿**。这是讨论稿，不是定稿。所有 §10 的 Open Question 都还没有最终答案，决策依赖后续 3 份调研报告（POC-A 流式延迟 / POC-B token provider 解耦 / POC-C 依赖图）回归后才能给出。
> 受众：架构 reviewer + 后续 PoC / Implementer agent。
> base：`feat/plugin-system-fixes--upstream-sync-115-121`

---

## 1. 背景

### 1.1 现状代码体量

| 文件 | 行数 | 职责 |
|---|---:|---|
| `backend/internal/service/gateway_service.go` | 9845 | Anthropic + Antigravity 主流程 god object |
| `backend/internal/service/openai_gateway_service.go` | 6345 | OpenAI / Codex 主流程（含 WebSocket 池） |
| `backend/internal/service/antigravity_gateway_service.go` | 4568 | Antigravity forward / 错误映射 / 积分扣减 |
| `backend/internal/handler/gateway_handler.go` | 1979 | Anthropic + Antigravity HTTP 入口 |
| `backend/internal/handler/openai_gateway_handler.go` | 1744 | OpenAI HTTP 入口 |
| **合计核心逻辑** | **~24,481 行** | |

加上 gateway_request / gateway_helper / gateway_tool_rewrite / gateway_websearch_emulation / openai_gateway_chat_completions / openai_gateway_messages 等辅助文件，总盘子接近 30 k 行。

### 1.2 现状的 platform 共享机制

`backend/internal/server/routes/gateway.go` 用 `getGroupPlatform(c)` 在 `OpenAIGateway` / `Gateway` 之间硬 dispatch（重复 6 次 if/else）。

Antigravity 没有独立 handler — 由 Anthropic 链路 + `middleware.ForcePlatform` + GatewayService 内部分支处理：`backend/internal/handler/gateway_handler.go:776` 是 fork 点（`if account.Platform == PlatformAntigravity { antigravityGatewayService.Forward } else { gatewayService.Forward }`）。

混调机制层次：
- **group.platform**：anthropic / openai / gemini / antigravity（4 选 1）
- **`useMixed`**（`gateway_service.go:2279`）：`platform == anthropic || gemini` 且无 ForcePlatform 时启用混调池
- **`account.IsMixedSchedulingEnabled()`**：antigravity 账号级开关，决定它是否被 anthropic group 借用
- **`middleware.ForcePlatform`**：`/antigravity/v1/*` 路径上把 platform 钉死

### 1.3 现有 plugin SDK 已具备的扩展面

| 扩展 | 方向 | 用途 |
|---|---|---|
| `HostService.ResolveModelPricing/CreditBalance/...` | plugin → host | 反向数据查询 |
| `EventsExtension.Subscribe(gateway.model.invoked)` | host → plugin | 网关请求事件推送 |
| `PricingExtension.AdjustCost / ResolveAccountStatsCost` | host → plugin | host 算完成本后让 plugin 改写 |
| `MaintenanceExtension.RunMaintenance` | host → plugin | 巡检任务 |
| `CapabilityHTTPRegisterGateway`（声明可用、尚未启用） | 路由声明 | plugin 在 `/v1/*` 注册 HTTP |
| `RouteTable` 反向代理（`backend/internal/plugin/router_middleware.go`） | host → plugin HTTP | path → ReverseProxy |

**已有的 hook 预埋**：
- `GatewayService.accountStatsResolver` (`gateway_service.go:582`)
- `OpenAIGatewayService.accountStatsResolver`（`openai_gateway_service.go:357`）
- `AntigravityGatewayService.eventPublisher`（`antigravity_gateway_service.go:889`）

---

## 2. 目标 / 非目标

### 2.1 目标

1. 把 24 k+ 行网关代码按"协议 × 上游"维度拆成 3 个独立 plugin：`gateway-anthropic` / `gateway-openai` / `gateway-antigravity`
2. 保住现有所有功能：混调、强制平台、Bedrock / Vertex / Codex 等亚平台、SSE / WebSocket 流式
3. 提供 SDK 入口让第三方写自己的网关插件
4. 拆分后每个 plugin ≤ 4000 行，god object 收敛

### 2.2 非目标

1. **不**重写计费 / 调度 / 限流 / 账号 / Group 数据模型 — 横切关注点必须留 host
2. **不**在第一阶段优化性能；可以接受 ≤ 2ms P99 增量
3. **不**改前端；`/v1/*` URL 表层语义对 client 透明
4. **不**支持运行时热卸载 gateway plugin

---

## 3. 整体架构（双层路由）

### 3.1 两张表

| 表 | key | value | 由谁维护 |
|---|---|---|---|
| **EndpointTable**（已有，扩展即可） | `(method, path)` | owner plugin + 协议守门员配置 | manifest + plugin manager |
| **ProviderRegistry**（新增） | `(account.platform, protocol)` | provider plugin | manifest + plugin manager |

**Endpoint owner** = 协议守门员：解析请求、调度选号、决定 forward 调谁、把响应转回自家协议。
**Provider** = 账号专属上游执行器：拿 (account, prepared_request)，去对应上游 forward，返回响应（已转成 input protocol 格式）。

### 3.2 请求生命周期

```
client → host
        │ EndpointTable.Match → owner plugin + endpoint manifest
        ▼
   owner plugin (gateway-anthropic)
        │ 1. 解析协议 (manifest.Protocol="anthropic")
        │ 2. HostService.SelectAccount(group_id, protocol, required_platform)
        │      → host 调度（含混调）+ ProviderRegistry 过滤
        │      → 选到 account{platform="antigravity"}
        │ 3. HostService.AcquireBillingTicket(account, model)
        │ 4. ProviderRegistry[antigravity, anthropic] → gateway-antigravity
        │ 5. HostService.Forward(account, prepared_request, protocol)
        ▼
   host (GatewayMediator)
        │ 看 ProviderRegistry → 调 gateway-antigravity.Forward bidi stream
        ▼
   provider plugin (gateway-antigravity)
        │ 用 antigravity OAuth token forward 到上游
        │ SSE 帧 → 转成 anthropic 协议 → stream 回传
        ▼
   host → owner (gateway-anthropic)
        │ 透传 SSE 给 client
        │ 流结束后调 HostService.RecordUsage / ConsumeBillingTicket
        ▼
   client
```

**owner == provider 时**（同 plugin 内 `(anthropic, anthropic)`），第 4-5 步直接本进程调用 `s.localProvider.Forward(...)`，不经 host mediator，性能等同现状。

### 3.3 调度池过滤（host 内、复用现有 SchedulerSnapshotService）

```go
// host 伪码
func (s *Service) SelectAccountForEndpoint(ctx, groupID, ep *EndpointDecl) (*Account, error) {
    accounts, useMixed, _ := s.scheduler.ListSchedulableAccounts(ctx, groupID,
        groupPlatform, ep.RequiredAccountPlatform != "")
    out := accounts[:0]
    for _, acc := range accounts {
        if ep.RequiredAccountPlatform != "" && acc.Platform != ep.RequiredAccountPlatform { continue }
        if !s.isAccountAllowedForPlatform(acc, groupPlatform, useMixed) { continue } // 现有
        if !s.providerRegistry.Has(acc.Platform, ep.Protocol)            { continue } // 新增
        out = append(out, acc)
    }
    return s.pickByPolicy(out), nil
}
```

第 (3) 条副作用：禁用 `gateway-antigravity` 后，anthropic group 里的 antigravity 账号自动从 `/v1/messages` 调度池排除 — 不需要人工运维。

---

## 4. Manifest 字段设计

### 4.1 EndpointDecl 新增两个字段

```go
type EndpointDecl struct {
    Path     string
    Methods  []string
    AuthType string

    // 该端点接收的请求协议。owner 解析与协议转换都按它走。
    // 候选值：anthropic / openai / gemini
    Protocol string

    // 强制使用的 account.platform。空 = 走 group 调度（含混调）。
    // 替代现有的 middleware.ForcePlatform。
    RequiredAccountPlatform string
}
```

字段命名理由见 §10 Q1。

### 4.2 Manifest 新增 GatewayProviders 声明

```go
type ProviderDecl struct {
    Platform string
    Protocol string
}

type Manifest struct {
    GatewayProviders []ProviderDecl
}
```

### 4.3 三个 plugin 的最终 manifest 形态（示意）

**`gateway-anthropic`**：
```go
GatewayEndpoints: []EndpointDecl{
    {Path: "/v1/messages",              Protocol: "anthropic"},
    {Path: "/v1/messages/count_tokens", Protocol: "anthropic"},
    {Path: "/v1/models",                Protocol: "anthropic"},
},
GatewayProviders: []ProviderDecl{
    {Platform: "anthropic", Protocol: "anthropic"},
},
```

**`gateway-antigravity`**：
```go
GatewayEndpoints: []EndpointDecl{
    {Path: "/antigravity/v1/messages",  Protocol: "anthropic", RequiredAccountPlatform: "antigravity"},
    {Path: "/antigravity/v1beta/*",     Protocol: "gemini",    RequiredAccountPlatform: "antigravity"},
    {Path: "/v1beta/models/*",          Protocol: "gemini"},
},
GatewayProviders: []ProviderDecl{
    {Platform: "antigravity", Protocol: "anthropic"},
    {Platform: "antigravity", Protocol: "gemini"},
},
```

**`gateway-openai`**：
```go
GatewayEndpoints: []EndpointDecl{
    {Path: "/v1/responses",         Protocol: "openai", RequiredAccountPlatform: "openai"},
    {Path: "/v1/responses/*",       Protocol: "openai", RequiredAccountPlatform: "openai"},
    {Path: "/v1/chat/completions",  Protocol: "openai", RequiredAccountPlatform: "openai"},
    {Path: "/v1/images/generations",Protocol: "openai", RequiredAccountPlatform: "openai"},
    {Path: "/v1/images/edits",      Protocol: "openai", RequiredAccountPlatform: "openai"},
    {Path: "/responses/*",          Protocol: "openai", RequiredAccountPlatform: "openai"},
    {Path: "/chat/completions",     Protocol: "openai", RequiredAccountPlatform: "openai"},
    {Path: "/backend-api/codex/*",  Protocol: "openai", RequiredAccountPlatform: "openai"},
},
GatewayProviders: []ProviderDecl{
    {Platform: "openai", Protocol: "openai"},
},
```

> **不确定**：OpenAI 的端点是否需要 `RequiredAccountPlatform="openai"`？**§10 Q4**。

---

## 5. SDK 表面增量

### 5.1 新增的 plugin extension service

```proto
service GatewayProviderExtension {
  rpc Forward(stream ForwardRequest) returns (stream ForwardChunk);
  rpc ParseUsage(ParseUsageRequest) returns (ParseUsageResponse);
  rpc RefreshCredentials(RefreshCredentialsRequest) returns (RefreshCredentialsResponse);
}

message ForwardInit {
  Account account = 1;
  string  protocol = 2;
  bytes   request_body = 3;
  string  model = 4;
  bool    stream = 5;
  int64   group_id = 6;
  string  session_hash = 7;
  bool    sticky_bound = 8;
  string  request_id = 9;
}
```

### 5.2 新增的 HostService RPC

```proto
service HostService {
  rpc SelectAccount(SelectAccountRequest) returns (SelectAccountResponse);
  rpc AcquireBillingTicket(AcquireTicketRequest) returns (AcquireTicketResponse);
  rpc ConsumeBillingTicket(ConsumeTicketRequest) returns (ConsumeTicketResponse);
  rpc CloseBillingTicket(CloseTicketRequest) returns (google.protobuf.Empty);
  rpc Forward(stream ForwardRequest) returns (stream ForwardChunk);
  rpc RecordUsage(RecordUsageRequest) returns (RecordUsageResponse);
  rpc UpdateAccountCredentials(UpdateCredentialsRequest) returns (UpdateCredentialsResponse);
  rpc GetAccount(GetAccountRequest) returns (GetAccountResponse);
  rpc BindStickySession(BindSessionRequest) returns (google.protobuf.Empty);
  rpc LookupStickySession(LookupSessionRequest) returns (LookupSessionResponse);
}
```

### 5.3 新增的 capability

| capability | tier | 说明 |
|---|---|---|
| `gateway.endpoint.owner` | declare-required | plugin 声明它当 endpoint owner |
| `gateway.provider` | declare-required | plugin 声明它注册了 GatewayProviders |
| `accounts.read` | declare-required | 调 GetAccount |
| `accounts.refresh_credentials` | admin-approve（Phase 2） | 反向写回 OAuth token |
| `billing.ticket` | declare-required | 用 BillingTicket lifecycle |

详见 §10 Q5。

---

## 6. 三阶段路线图

### 阶段一：host 内重构（不引入 plugin，先收敛 god object）

**目标**：把三套 forward 抽成 `GatewayProvider` 接口，每个 platform 一个实现仍在 host 内。

```go
type GatewayProvider interface {
    Platform() string
    Protocols() []string
    PrepareRequest(ctx, *ParsedRequest, *Account) (*UpstreamRequest, error)
    Forward(ctx, http.ResponseWriter, *UpstreamRequest, *Account) (*ForwardResult, error)
    ParseUsage(*ForwardResult) (*UsageTokens, error)
    MapError(*http.Response) error
}
```

`gateway_handler.go` / `openai_gateway_handler.go` 退化成"路由 → 选 provider → 通用 pipeline (acquire → forward → consume → recordUsage)"。

**这一步独立有价值**：即便最终不抽 plugin，god object 收敛也是必赚。预算 2-3 周。

### 阶段二：SDK 扩展面落地（GatewayProviderExtension + ProviderRegistry）

按 §5 的 proto 定义生成代码，host 加 ProviderRegistry，Mediator gRPC bidi stream forward 通路。

落地"假 plugin"做集成测试：写 `gateway-anthropic-stub` 把 host 内 GatewayProvider 实现复制过去，用 Mediator 转发。

预算 3-4 周（含 PoC-A 流式实测）。

### 阶段三：分平台抽离插件（按风险递增）

| 顺序 | 插件 | 风险 | 理由 |
|---|---|---|---|
| 1 | `plugins/gateway-openai` | 低 | OpenAI 已经独立 service，几乎不依赖 GatewayService 内部状态 |
| 2 | `plugins/gateway-anthropic` | 中 | god object 主体，需先完成阶段一拆分 |
| 3 | `plugins/gateway-antigravity` | 中 | 现在贴在 anthropic 链路上，单独抽出后正好独立 |

每个平台 4-6 周。**总周期估**：12-18 周，含 PoC + 双写灰度 + 上线观察。

---

## 7. 已识别风险

| 风险 | 等级 | 缓解 |
|---|---|---|
| 流式 SSE/WS 跨 host mediator 延迟过高 | **高** | POC-A 实测；备选零拷贝 stream proxy |
| OAuth refresh token 跨进程并发刷新（多 instance） | **中** | host 提供分布式锁 + UpdateAccountCredentials CAS |
| Bedrock SigV4 / Vertex Service Account 凭据泄漏到 plugin 进程 | **中** | host 留解密通道；plugin 只拿短期凭据，POC-B 详细分析 |
| god object 拆分期主流程必须双写 | **高** | 阶段一先抽 GatewayProvider 接口，阶段二/三逐 platform feature flag |
| recordUsageCore 跨 RPC 影响每请求 latency | 中 | 维持在 host 进程内；plugin 不接管 RecordUsage |
| sticky session redis 命名空间被 plugin 污染 | 低 | 走 BindStickySession RPC 不开放 redis raw |
| Gemini `/v1beta/*` 鉴权链特殊（Google Auth） | 中 | 端点 manifest 需要支持 AuthType 扩展，未确定 |

---

## 8. 不在本提案范围内的事

- 网关插件的前端 settings 页面设计
- 网关插件的安装 / 升级 / 卸载流程
- gemini 协议端点的 google auth chain（Q6）
- 网关 metrics / tracing 跨进程串联（沿用现有 traceparent）

---

## 10. Open Questions（必须在决策前回答）

| # | 问题 | 答案来源 | 状态 |
|---|------|----------|------|
| Q1 | Manifest 字段叫 `Protocol` + `RequiredAccountPlatform`（推荐）还是 `InputProtocol` + `ForceAccountPlatform`？ | 用户拍板 | 推荐已给 |
| Q2 | 流式跨 host mediator 转发的 P99 延迟增量是否 ≤ 2 ms？ | POC-A 实测 | 待 PoC |
| Q3 | OAuth refresh token 是否能完全在 plugin 进程做？host 留什么 RPC？ | POC-B 分析 | 待调研 |
| Q4 | OpenAI 端点是否预留 `(antigravity, openai)` 注册位？ | 业务判断 | 待用户确认 |
| Q5 | `gateway.endpoint.owner` 是否合并进 `gateway.provider`？ | 设计判断 | 倾向合并 |
| Q6 | `/v1beta/*`（Gemini 原生）的 google-auth 鉴权链如何在 manifest 表达？ | 设计判断 | 待 POC-C |
| Q7 | 同 `(platform, protocol)` 多 plugin 注册时的策略？ | 设计判断 | 倾向拒启动 |
| Q8 | provider plugin 重启时正在 forward 的请求如何处理？ | 设计判断 | 倾向立即 fail |
| Q9 | god object 拆分期的双写灰度方案？ | POC-C 完成后再设计 | 待调研 |
| Q10 | recordUsageCore 跨进程的代价是否可接受？还是必须留 host？ | POC-C 实测 | 倾向留 host |

---

## 11. 后续动作

1. **POC-A**（Agent A）：流式转发 PoC 实验设计 → `GATEWAY-POC-A-STREAMING.md`
2. **POC-B**（Agent B）：Token provider 跨进程解耦分析 → `GATEWAY-POC-B-TOKEN-PROVIDER.md`
3. **POC-C**（Agent C）：GatewayService 主流程依赖图 → `GATEWAY-POC-C-DEPENDENCY-MAP.md`
4. **决策建议**（Leader 整合）：→ `GATEWAY-EXTRACTION-DECISION.md`

完成后用户拍板进入阶段一 / 暂缓 / 改路线。

---

## 附录 A：术语表

- **owner（端点 owner）**：负责协议解析 / 调度选号 / 响应封装的 plugin。一个 endpoint 只有一个 owner。
- **provider（账号 provider）**：负责 (account.platform, protocol) 二元组的实际上游 forward。一个 plugin 可注册多个 provider。
- **混调（mixed scheduling）**：anthropic group 拉 antigravity 账号一起调度的现有机制；账号级 opt-in。
- **stable surface**：host 必须长期保留、plugin 跨进程依赖的 RPC 接口集合。
- **god object**：`gateway_service.go` 的 `GatewayService` 类型（9845 行 / 24+ 依赖注入）。
