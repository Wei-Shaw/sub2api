# Gateway Extraction · Phase 1 · 详细实施计划

> 状态：**就绪**。Implementer 看完即可开工。
> 目标：把三套 forward 抽成 GatewayProvider 接口 + 通用 Pipeline，不引入 plugin / 不跨进程。
> 预算：2-3 周（可按 4 个 milestone 递增交付）。
> 分支：在当前 worktree 继续（`feat/plugin-system-fixes--upstream-sync-115-121`）。

---

## 0. 核心发现（来自 POC-C 依赖图）

三条 hot path 共享同一管道结构，仅 **Forward** 步骤分叉：

```
ParseRequest → ChannelMapping → AcquireUserSlot → BillingCheck →
SessionHash → LOOP { SelectAccount → AcquireAccountSlot → BillingConsume →
   ★ provider.Forward(account, request) ★
→ ReleaseAccountSlot → IncrementRPM } → RecordUsage → BillingClose
```

阶段一就是把这个管道提取成 `GatewayPipeline.Execute(provider)`，让 handler 退化为路由层。

---

## 1. Milestone 1 — 接口定义 + 新包骨架

### 1.1 新增 `backend/internal/gateway/` 包

```
backend/internal/gateway/
    provider.go       // GatewayProvider 接口
    pipeline.go       // GatewayPipeline struct + Execute
    request.go        // GatewayRequest（跨 provider 的通用请求上下文）
    result.go         // ForwardResult / Usage 通用 DTO
    registry.go       // ProviderRegistry（platform → provider 注册表）
```

### 1.2 GatewayProvider 接口

```go
package gateway

type GatewayProvider interface {
    // Platform 返回该 provider 服务的平台标识（"anthropic" / "openai" / "antigravity"）
    Platform() string

    // Protocols 返回该 provider 能处理的输入协议列表（"anthropic" / "openai" / "gemini"）
    Protocols() []string

    // Forward 执行上游转发。Pipeline 在 SelectAccount + BillingConsume
    // 之后调用。provider 负责：获取凭据 → 构建上游请求 → 转发 → 流式写响应。
    // 返回的 ForwardResult 包含 usage 用于后续 RecordUsage。
    Forward(ctx context.Context, w http.ResponseWriter, req *GatewayRequest) (*ForwardResult, error)

    // HandleError 在 Forward 返回错误后调用。provider 决定是否该 failover
    // 到下一个账号（返回 true）还是终止（返回 false + 写响应）。
    ShouldFailover(ctx context.Context, w http.ResponseWriter, req *GatewayRequest, err error) bool
}
```

### 1.3 GatewayRequest（跨 provider 的通用请求上下文）

```go
type GatewayRequest struct {
    // 不可变——Pipeline 构建后 provider 只读
    RawBody     []byte
    Model       string
    Stream      bool
    GroupID     *int64
    APIKey      *service.APIKey
    SessionHash string

    // Pipeline 每次 select-account 循环更新
    Account         *service.Account
    ChannelMapping  *service.ChannelMappingResult
    BillingTicket   *service.BillingTicket
    SwitchCount     int

    // provider 可写——Forward 结束后 Pipeline 读取
    UpstreamAccepted bool // provider 在上游接受请求后设 true，禁止 failover
}
```

### 1.4 ProviderRegistry

```go
type ProviderRegistry struct {
    mu        sync.RWMutex
    providers map[string]GatewayProvider // platform → provider
}

func (r *ProviderRegistry) Register(p GatewayProvider)
func (r *ProviderRegistry) Get(platform string) (GatewayProvider, bool)
func (r *ProviderRegistry) ForProtocol(protocol string) []GatewayProvider
```

阶段一：host 启动时硬编码注册三个 provider。阶段二/三：plugin manager 动态注册。

---

## 2. Milestone 2 — Pipeline 通用流程

### 2.1 GatewayPipeline struct

```go
type GatewayPipeline struct {
    registry           *ProviderRegistry
    gatewayService     *service.GatewayService     // 调度 + 会话
    billingCacheService *service.BillingCacheService // 票据
    concurrencyService *service.ConcurrencyService  // 并发槽
    settingService     *service.SettingService
    // ...其他横切 service
}
```

### 2.2 Execute 方法签名

```go
func (p *GatewayPipeline) Execute(
    c *gin.Context,
    protocol string,          // "anthropic" / "openai" / "gemini"
    forcePlatform string,     // 空 = 走 group 调度；非空 = 强制平台
    parseRequest func(body []byte) (*GatewayRequest, error),
    recordUsage func(ctx context.Context, account *service.Account, result *ForwardResult) error,
) error
```

### 2.3 Execute 内部步骤（替代 gateway_handler.go 的 6 处 if/else）

```
1. parseRequest(body) → GatewayRequest
2. gatewayService.ResolveChannelMappingAndRestrict
3. concurrencyService.AcquireUserSlot (+ wait counter)
4. billingCacheService.PrepareBillingCheckForRequest
5. gatewayService.GenerateSessionHash + GetCachedSessionAccountID
6. LOOP (max retries = failover limit):
   a. gatewayService.SelectAccount(groupID, protocol, forcePlatform, excludeIDs)
   b. concurrencyService.AcquireAccountSlot
   c. billingTicket.Consume(channelID, accountID)
   d. provider := registry.Get(account.Platform)
   e. result, err := provider.Forward(ctx, w, req)
   f. if err != nil && provider.ShouldFailover(ctx, w, req, err):
        excludeIDs = append(excludeIDs, account.ID); continue
      else if err != nil: return err (已写响应)
   g. break
7. recordUsage(ctx, account, result)
8. billingTicket.Close() (defer)
```

### 2.4 关键设计点

- **parseRequest / recordUsage 由 handler 注入**：Anthropic 和 OpenAI 的 parse / record 逻辑差异大（gjson vs json.Unmarshal、ClaudeUsage vs OpenAIUsage），不适合抽象成 provider 方法。用回调保持 handler 控制权。
- **SelectAccount 仍在 GatewayService 里**：调度依赖大量内部状态（cache / singleflight / sticky），不适合搬出 god object。Pipeline 只调公共方法。
- **provider.Forward 接管 ResponseWriter**：流式场景 provider 直接写 SSE/WS 到客户端。Pipeline 不缓冲响应。
- **failover 判定由 provider 做**：不同平台的 failover 条件不同（antigravity 的 PromptTooLong 可以 fallback group；OpenAI 的 retry-on-same-account 逻辑不同）。

---

## 3. Milestone 3 — 三个 Platform Adapter

### 3.1 AnthropicProvider（最大的一个）

```
backend/internal/gateway/anthropic_provider.go
```

包装 `GatewayService.Forward` + `GatewayService.GetAccessToken` + 现有所有 Anthropic 特异性逻辑。初始版本是 thin wrapper：

```go
type AnthropicProvider struct {
    gatewayService *service.GatewayService
}

func (p *AnthropicProvider) Platform() string { return "anthropic" }
func (p *AnthropicProvider) Protocols() []string { return []string{"anthropic"} }
func (p *AnthropicProvider) Forward(ctx, w, req) (*ForwardResult, error) {
    // 调现有 gatewayService.Forward，转换 req/result 类型
    parsedReq := toParsedRequest(req)
    result, err := p.gatewayService.Forward(ctx, c, req.Account, parsedReq)
    return toForwardResult(result), err
}
```

**不重写 gatewayService.Forward 内部逻辑**。阶段一只是"加一层接口"，不拆 god object 内部。

### 3.2 AntigravityProvider

```
backend/internal/gateway/antigravity_provider.go
```

包装 `AntigravityGatewayService.Forward` + `AntigravityGatewayService.ForwardGemini`。

```go
func (p *AntigravityProvider) Protocols() []string { return []string{"anthropic", "gemini"} }
```

### 3.3 OpenAIProvider

```
backend/internal/gateway/openai_provider.go
```

包装 `OpenAIGatewayService` 的 ChatCompletions / Responses / Images。OpenAI 的 handler 逻辑与 Anthropic 差异最大（不用 ParseGatewayRequest、有自己的 SelectAccountWithScheduler），但 Pipeline 通过回调注入 parse/record 已经处理了这个差异。

---

## 4. Milestone 4 — Handler 切换 + 测试

### 4.1 routes/gateway.go 改造

```go
// 改前（6 处 if/else）：
gateway.POST("/messages", func(c *gin.Context) {
    if getGroupPlatform(c) == service.PlatformOpenAI {
        h.OpenAIGateway.Messages(c)
        return
    }
    h.Gateway.Messages(c)
})

// 改后（单一入口）：
gateway.POST("/messages", func(c *gin.Context) {
    pipeline.Execute(c, "anthropic", "", parseAnthropicRequest, recordAnthropicUsage)
})
```

`/antigravity/v1/*` 路由：
```go
antigravityV1.POST("/messages", func(c *gin.Context) {
    pipeline.Execute(c, "anthropic", "antigravity", parseAnthropicRequest, recordAnthropicUsage)
})
```

### 4.2 测试策略

| 类型 | 覆盖 | 做法 |
|---|---|---|
| 单元测试 | ProviderRegistry | 注册 / 查找 / protocol 过滤 |
| 单元测试 | Pipeline.Execute | mock provider + mock services，验证步骤顺序和 failover 逻辑 |
| 集成测试 | 现有 gateway_handler_*_test.go | 不改！它们验证端到端行为，阶段一是透明重构，所有现有测试必须继续通过 |
| Smoke | 手动跑 test 环境 | 部署后打一遍 anthropic / openai / antigravity / gemini 流式 + 非流式 |

### 4.3 风险兜底

每个 milestone 完成后跑 `cd backend && go build ./... && make test-unit`。M4 完成后跑 `make test-integration`。任何测试失败 → 回退该 milestone commit。

---

## 5. 不在阶段一范围内的事

- 拆 god object 内部逻辑（9845 行的 GatewayService.Forward 内部不动）
- 新增 SDK proto / gRPC extension
- PoC-A 流式实验（并行线路，独立分支）
- Plugin 抽离

---

## 6. 文件增删预估

| 操作 | 文件 | 行数（估） |
|---|---|---|
| 新增 | `backend/internal/gateway/provider.go` | ~80 |
| 新增 | `backend/internal/gateway/pipeline.go` | ~250 |
| 新增 | `backend/internal/gateway/request.go` | ~60 |
| 新增 | `backend/internal/gateway/result.go` | ~50 |
| 新增 | `backend/internal/gateway/registry.go` | ~50 |
| 新增 | `backend/internal/gateway/anthropic_provider.go` | ~100 |
| 新增 | `backend/internal/gateway/antigravity_provider.go` | ~80 |
| 新增 | `backend/internal/gateway/openai_provider.go` | ~100 |
| 新增 | `backend/internal/gateway/pipeline_test.go` | ~200 |
| 新增 | `backend/internal/gateway/registry_test.go` | ~60 |
| 修改 | `backend/internal/server/routes/gateway.go` | ~-80 / +40 |
| 修改 | `backend/internal/handler/gateway_handler.go` | ~-60 / +20 |
| 修改 | `backend/internal/handler/openai_gateway_handler.go` | ~-40 / +15 |
| 修改 | `backend/cmd/server/wire.go` + `wire_gen.go` | ~+30 |
| **合计新增** | | **~1030 行** |
| **合计删除** | | **~180 行** |
| **净增** | | **~850 行** |
