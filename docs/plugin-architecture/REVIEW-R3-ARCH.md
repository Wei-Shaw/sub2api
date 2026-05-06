# Review Round 3 -- 架构一致性 + 四原则

> 审核范围：`backend/internal/gateway/`（10 文件）、`plugin-sdk/credentials.go` + `credentials_test.go`、`wire_gen.go` gateway 引用、`handler/handler.go` + `handler/wire.go`、`routes/gateway.go` debug endpoint、三份设计文档。
> 审核基准：CLAUDE.md "工程原则（最高优先级）" 章节的复用/解耦/封装/抽象四原则。

---

## 1. 复用（Reuse）

- 问题数：3

### R1. `anthropicResultToForwardResult` 与 `antigravityResultToForwardResult` 是近乎逐字的平行实现

`anthropic_provider.go:55-68` 和 `antigravity_provider.go:80-94` 两个函数体完全相同（都把 `service.ForwardResult` 映射到 `gateway.ForwardResult`），仅函数名不同。这违反"同一概念只在一处实现"。

当 `service.ForwardResult` 新增字段（如已有但未映射的 `ReasoningEffort`）时，改一处漏另一处必然漂移。

### R2. `gateway.ForwardResult` 与 `service.ForwardResult` / `service.OpenAIForwardResult` 存在两份 DTO 定义

`gateway/result.go` 定义的 `ForwardResult` 是 `service.ForwardResult` + `service.OpenAIForwardResult` 的合集子集。目前映射逻辑分散在三个 adapter 文件的三个 `xxxResultToForwardResult` 函数中。虽然 Phase 1 作为 thin wrapper 这是合理的过渡设计，但需要注意以下字段漂移风险：

- `service.ForwardResult.ReasoningEffort` -- 未映射到 `gateway.ForwardResult`
- `service.ForwardResult.CacheCreation5mTokens` / `CacheCreation1hTokens` -- 未映射
- `service.OpenAIForwardResult.BillingModel` / `ServiceTier` / `ReasoningEffort` / `OpenAIWSMode` / `ResponseHeaders` -- 未映射

这些字段在 `recordUsage` 回调中可能需要，如果 handler 的 `RecordUsageFunc` 实现依赖 `gateway.ForwardResult` 而非原始 service result，则数据会丢失。当前 Pipeline 尚未被路由使用（仅 compile-time validation），但字段遗漏会在实际切换时成为 bug。

### R3. `toServiceParsedRequest` 仅在 `anthropic_provider.go` 定义但可能被 antigravity adapter 复用

`anthropic_provider.go:50-59` 的 `toServiceParsedRequest` 构建 `service.ParsedRequest`。AntigravityProvider 没有用它（因为 `AntigravityGatewayService.Forward` 接受不同的参数签名），但如果 Antigravity 未来统一签名，此函数就需要被共享。当前不是 blocker，但值得标注。

---

## 2. 解耦（Decouple）

- 问题数：3

### D1. `ForwardRequest.GinContext *gin.Context` -- web 框架耦合进入 gateway 包 DTO

`request.go:48` 将 `*gin.Context` 作为字段嵌入 `ForwardRequest`。虽然注释明确说"Phase 1 adapter bridge field, Phase 2+ will not use this"，但这意味着 `gateway` 包的核心 DTO 直接 import 了 `gin-gonic/gin`，使 gateway 包无法在非 gin 环境下复用。

注释写得很好，Phase 1 过渡意图明确。但从架构角度看，更干净的做法是让 adapter 自己在 Forward 方法内从 closure/field 获取 `*gin.Context`，而非塞进跨 provider 共享的 DTO。

### D2. `pipeline.go` 直接 import `middleware2 "...server/middleware"` -- gateway 包反向依赖 server 层

`pipeline.go:12` import 了 `backend/internal/server/middleware`，用于 `extractAuthContext` 中调用 `middleware2.GetAPIKeyFromContext` / `GetAuthSubjectFromContext` / `GetSubscriptionFromContext`。这让 gateway 包（理应是核心业务层）依赖了 HTTP server 层的 middleware。

依赖方向应为 handler/server -> gateway -> service，而当前是 gateway -> server/middleware，形成了不健康的循环依赖风险（虽然 Go 编译器不允许真正的循环 import，但逻辑分层被打破了）。

建议：将 `GetAPIKeyFromContext` 等上下文提取逻辑移到 Pipeline 的调用方（handler 层）完成，以参数形式传入 Pipeline。

### D3. `ProvideProviderRegistry` 在 `wire.go` 中硬编码了三个具体 provider 实现

`wire.go:11-18` 的 `ProvideProviderRegistry` 直接构造 `NewAnthropicProvider` / `NewOpenAIProvider` / `NewAntigravityProvider`，使 registry 的 provider 函数依赖所有三个具体 service 类型。Phase 1 这是合理的过渡，但与"ProviderRegistry 不应依赖具体 provider 实现"的目标有冲突。Phase 2 需要改为 plugin manager 动态注册。注释已标注意图，info 级别。

---

## 3. 封装（Encapsulate）

- 问题数：2

### E1. `GatewayPipeline.Registry()` 暴露了内部 ProviderRegistry 实例

`pipeline.go:71` 的 `Registry()` 方法将 pipeline 的内部 `*ProviderRegistry` 直接返回给外部。`routes/gateway.go:186-195` 的 `pipelineHealthHandler` 通过此方法调用 `ForProtocol`。

虽然这只用于诊断端点，但它暴露了可变的内部状态（外部拿到指针后可调用 `Register` / `Unregister` 修改 registry）。更好的封装是在 Pipeline 上提供 `HealthStatus() map[string]int` 方法，而非暴露整个 registry 引用。

### E2. `ForwardRequest` 的 `UpstreamAccepted` 字段既是 provider 写、pipeline 读，又通过 closure 传递

`request.go:72` 的 `UpstreamAccepted` 由 provider 写入，pipeline 在 `handleForwardError` 中读取。同时 `anthropic_provider.go:57` 通过 `OnUpstreamAccepted` closure 修改此字段。这个双写路径（直接写字段 + closure 间接写）增加了并发安全隐患。

当前 Pipeline 是单线程执行（一次只有一个 provider.Forward 在跑），不是并发 bug，但封装上不够干净。建议统一为一种写入方式。

---

## 4. 抽象（Abstract）

- 问题数：3

### A1. GatewayProvider 接口 4 方法精简到合适，但缺少 error path 的差异化抽象

实际接口：`Platform() + Protocols() + Forward() + ShouldFailover()` 共 4 方法。PROPOSAL 阶段一曾提到 6 方法版本（含 `PrepareRequest` / `ParseUsage` / `MapError`），实际实现精简为 4 方法并用 callback（`ParseRequestFunc` / `RecordUsageFunc`）注入差异 -- 这是更好的设计。

但 error path 的差异化处理不足：所有三个 provider 的 `ShouldFailover` 实现完全相同（都是 `errors.As(err, &failoverErr)`），这说明当前的 failover 判定没有平台差异。如果未来真的存在平台差异（如 PROPOSAL 提到的 Antigravity PromptTooLong fallback），当前接口可以支持，但目前三个一模一样的实现是可以提取为默认行为的。

### A2. Pipeline 的 happy path 和 error path 不对称

`selectAndForward` 循环处理了 failover 逻辑，但以下 error path 差异未被抽象：
- Anthropic 的 `PromptTooLong` 可能需要 fallback 到不同 group
- OpenAI 的 `retry-on-same-account` 逻辑与 Anthropic 不同
- 错误透传规则（`ResolveErrorPassthroughRule`）未出现在 Pipeline 中

Pipeline 仅抽象了 happy path 和简单 failover，但 error handling/error response writing 仍留在 provider 内部（provider 通过 `service.Forward` 直接写 response）。这意味着 Pipeline 对 error 路径的控制不完整。Phase 1 过渡这是可以接受的，但需要在 Phase 2 补齐。

### A3. `CredentialManager` 的 3 组方法（OAuth / APIKey / Custom）是否应该拆成多个接口

`plugin-sdk/credentials.go` 的 `CredentialManager` 接口有 4 个方法涵盖 3 种凭证模型。按接口隔离原则，OAuth-only 的 plugin 不需要 `RegisterCustomAuth` / `GetCustomToken`，APIKey-only 的 plugin 不需要 `GetOAuthToken`。

但考虑到：(1) 这是 SDK 面向 plugin 的统一入口；(2) 不需要的方法调用会返回 `ErrCredentialNotFound`，行为清晰；(3) 拆成 3 个小接口会增加 `PluginContext` 的方法数量 -- 保持单一接口是合理的权衡。info 级别，不是 blocker。

---

## 5. 文档一致性

- 问题数：5

### DOC1. PROPOSAL S6 的 GatewayProvider 接口与实际实现不匹配

PROPOSAL S6 阶段一列出的接口有 6 个方法：`Platform()` / `Protocols()` / `PrepareRequest()` / `Forward()` / `ParseUsage()` / `MapError()`。实际实现只有 4 方法（`Platform` / `Protocols` / `Forward` / `ShouldFailover`）。`PrepareRequest` / `ParseUsage` / `MapError` 被精简掉了，`ShouldFailover` 是新增的（替代 `MapError`）。

PROPOSAL 标注为 v0 草稿，DECISION 没有更新接口定义，这导致三份文档描述的接口形状与代码不一致。

### DOC2. DECISION Q3 的答案与 `credentials.go` 的接口设计基本一致但命名有偏差

DECISION Q3 收敛的答案是"SDK 提供通用 OAuth + API Key 抽象 + 支持自定义鉴权注册"。`credentials.go` 的 `CredentialManager` 接口与此方向一致。但 DECISION 提到的 `GetAccountAccessToken`（合并 OAuth + Vertex）在 `credentials.go` 中叫 `GetOAuthToken`，命名有微小偏差。接口设计一致，命名需统一。

### DOC3. PHASE-1-PLAN S6 的文件增删预估与实际不完全匹配

| 预估文件 | 预估行数 | 实际文件 | 实际行数 | 差异 |
|---|---|---|---|---|
| `provider.go` | ~80 | `provider.go` | 18 | 大幅低于预估（接口精简了） |
| `pipeline.go` | ~250 | `pipeline.go` | 388 | 超出预估 55% |
| `request.go` | ~60 | `request.go` | 55 | 接近 |
| `result.go` | ~50 | `result.go` | 29 | 低于预估 |
| `registry.go` | ~50 | `registry.go` | 61 | 接近 |
| `anthropic_provider.go` | ~100 | `anthropic_provider.go` | 75 | 偏低 |
| `antigravity_provider.go` | ~80 | `antigravity_provider.go` | 91 | 接近 |
| `openai_provider.go` | ~100 | `openai_provider.go` | 61 | 偏低 |
| `pipeline_test.go` | ~200 | **不存在** | 0 | **缺失** |
| `registry_test.go` | ~60 | `registry_test.go` | 111 | 超出预估 |
| **未预估** | -- | `wire.go` | 31 | 新增（未在计划中） |

关键发现：`pipeline_test.go` 预估 ~200 行但实际不存在。Pipeline 是整个重构的核心，缺少单元测试是质量风险。

### DOC4. PHASE-1-PLAN S4.1 描述的路由改造未实际执行

PLAN S4.1 展示了 `routes/gateway.go` 从 if/else 改为 `pipeline.Execute(c, "anthropic", ...)` 的改造方案。实际代码中 `routes/gateway.go` 的路由仍然是 if/else 分发到 `h.Gateway.Messages` / `h.OpenAIGateway.Messages`，Pipeline 仅作为 diagnostic（`_debug/pipeline-health` endpoint）。

这与 `handler/handler.go:63-66` 的注释一致："Currently held for compile-time validation only"。不算文档不一致（PLAN 是"就绪"状态不是"已完成"），但说明 Phase 1 Milestone 4 尚未实施。

### DOC5. PROPOSAL S5.2 列出的 HostService RPC 与 pipeline.go 实际调用的 service 方法基本对应

PROPOSAL S5.2 列出 11 个 host RPC，DECISION 扩展到 22 个。pipeline.go 实际调用的 service 方法：

| pipeline 中的调用 | 对应 PROPOSAL RPC |
|---|---|
| `pkghttputil.ReadRequestBodyWithPrealloc` | 无（本地操作） |
| `gatewayService.ResolveChannelMappingAndRestrict` | `ResolveChannelMapping` |
| `concurrency.IncrementWaitCount` / `DecrementWaitCount` | `IncrementWait` / `DecrementWait`（POC-C 补充） |
| `concurrency.AcquireUserSlot` | 未列入 PROPOSAL（host 内部） |
| `billingCache.PrepareBillingCheckForRequest` | `AcquireBillingTicket` |
| `gatewayService.GetCachedSessionAccountID` | `LookupStickySession` |
| `gatewayService.SelectAccountWithLoadAwareness` | `SelectAccount` |
| `concurrency.AcquireAccountSlot` | 未列入 PROPOSAL |
| `billingTicket.Consume` | `ConsumeBillingTicket` |
| `gatewayService.BindStickySession` | `BindStickySession` |

核心 RPC 基本对应，但 concurrency slot 相关的调用（`AcquireUserSlot` / `AcquireAccountSlot`）在 PROPOSAL 中未被列为 RPC -- 因为 Phase 1 它们是 host 内部调用不需要跨进程。一致性 OK，差异合理。

---

## 发现的问题（汇总）

| # | 原则 | 严重度 | 文件 | 问题 | 建议 |
|---|---|---|---|---|---|
| 1 | 复用 | warning | `anthropic_provider.go:55-68` / `antigravity_provider.go:80-94` | `anthropicResultToForwardResult` 与 `antigravityResultToForwardResult` 逐字重复 | 提取为共享的 `serviceResultToForwardResult` |
| 2 | 复用 | warning | `gateway/result.go` | `gateway.ForwardResult` 丢失 `ReasoningEffort`、`CacheCreation5m/1hTokens`、`BillingModel`、`ServiceTier` 等字段 | 补全或确认 RecordUsageFunc 可绕过此 DTO |
| 3 | 复用 | info | `anthropic_provider.go:50-59` | `toServiceParsedRequest` 仅 Anthropic 使用，Antigravity 可能需要复用 | 待统一签名时提取 |
| 4 | 解耦 | warning | `request.go:48` | `GinContext *gin.Context` web 框架耦合进 gateway 包 DTO | adapter 自行持有，不放入共享 DTO |
| 5 | 解耦 | warning | `pipeline.go:12` | `import middleware2` 使 gateway 反向依赖 server 层 | auth context 提取移到 handler 层 |
| 6 | 解耦 | info | `wire.go:11-18` | `ProvideProviderRegistry` 硬编码三个具体实现 | Phase 2 改为动态注册（已标注） |
| 7 | 封装 | warning | `pipeline.go:71` | `Registry()` 暴露可变内部 ProviderRegistry 指针 | 改为 `HealthStatus()` 或只读视图 |
| 8 | 封装 | info | `request.go:72` + `anthropic_provider.go:57` | `UpstreamAccepted` 双写路径 | 统一为一种写入方式 |
| 9 | 抽象 | info | 三个 provider `ShouldFailover` | 三个实现完全相同 | 提供 `DefaultShouldFailover` helper |
| 10 | 抽象 | info | `pipeline.go` | error path 抽象不完整 | Phase 2 补齐 |
| 11 | 抽象 | info | `plugin-sdk/credentials.go` | `CredentialManager` 未拆分小接口 | 当前权衡合理 |
| 12 | 文档 | warning | PROPOSAL S6 vs 代码 | 6 方法接口 vs 实际 4 方法 | 更新 PROPOSAL 或 DECISION |
| 13 | 文档 | blocker | PHASE-1-PLAN S6 vs 代码 | `pipeline_test.go` 预估 ~200 行但实际不存在 | 补充 pipeline_test.go |
| 14 | 文档 | info | PHASE-1-PLAN S6 vs 代码 | `wire.go` 未在预估中 | 更新 PLAN |
| 15 | 文档 | info | DECISION Q3 vs credentials.go | `GetAccountAccessToken` vs `GetOAuthToken` 命名偏差 | 统一命名 |
| 16 | 文档 | info | PHASE-1-PLAN S4.1 vs routes/gateway.go | M4 路由改造尚未实施 | 标注 M4 状态为 TODO |

---

## 整体评价

**PASS WITH WARNINGS**

### 四原则取舍说明

**复用**：Phase 1 作为 thin wrapper 的定位使得引入 `gateway.ForwardResult` 这一"平行 DTO"是不可避免的过渡成本。真正的问题是 `anthropicResultToForwardResult` 与 `antigravityResultToForwardResult` 的逐字重复，以及字段映射的遗漏（`ReasoningEffort` 等）。前者应立即修复，后者需要在 M4 切换前补齐。

**解耦**：`gin.Context` 嵌入 DTO 和 gateway 反向 import server/middleware 是 Phase 1 的最大架构妥协。这两处打破了 gateway 包应有的层级独立性。好消息是注释明确标注了过渡意图，且不影响 Phase 1 的"compile-time validation only"定位。但 Phase 2 必须解决，否则 gateway 包无法作为 plugin SDK 的 stable surface。

**封装**：整体良好。`Pipeline.Execute` 是唯一对外入口，内部步骤全部私有化。`Registry()` 暴露可变指针是小瑕疵。

**抽象**：GatewayProvider 4 方法接口 + ParseRequestFunc / RecordUsageFunc callback 的设计比 PROPOSAL 原始的 6 方法接口更优。Pipeline 的 `selectAndForward` 循环优雅地抽象了 failover 机制。不足之处是 error path 的抽象不完整，以及三个 `ShouldFailover` 实现的无意义重复。

**综合判断**：Phase 1 代码质量高于预期，接口设计精简合理，注释清晰标注了过渡边界。blocker 仅 1 项（缺少 pipeline_test.go），warning 6 项均可在后续 milestone 中逐步修复。建议补齐测试后再进入 M4 路由切换。
