# Gateway Extraction · Decision（v0 - 等用户拍板）

> 状态：**待用户拍板**。
> 入参：`GATEWAY-EXTRACTION-PROPOSAL.md` + `GATEWAY-POC-A-STREAMING.md` + `GATEWAY-POC-B-TOKEN-PROVIDER.md` + `GATEWAY-POC-C-DEPENDENCY-MAP.md`。
> 受众：用户（决策者）。看完即可拍板"启动阶段一 / 暂缓 / 改路线"。

---

## 0. 执行摘要

**整体可行性评级：🟡 Yellow → 🟢 Green**（取决于 PoC-A 实测结果）

3 份 POC 没发现"红线"问题；提案的 §3 双层路由（EndpointTable + ProviderRegistry）+ §6 三阶段路线整体可行。但**提案的 SDK 表面要扩**：
- 提案 §5.2 列了 11 个 host RPC，POC-C 实际盘点是 **22 个**（多 11 个）。
- 提案 §10 Q3 假设"plugin 自己持 OAuth refresh"，POC-B 否决了这个 — 推荐 **token provider 整体留 host**，仅 4 个新 RPC。

**推荐下一步**：
1. **立即启动阶段一 host 内重构**（god object 拆 GatewayProvider 接口，2-3 周）。这一步独立有价值，零跨进程风险。
2. **并行做 PoC-A 流式实验**（按 POC-A 文档 12 步 checklist）。一周左右出数据，决定 P99 增量是否 ≤ 2 ms。
3. **PoC-A 通过后再启动阶段二**（SDK extension + Mediator）。失败则按 POC-A §5 决策树降级。
4. **阶段三抽插件按 OpenAI → Antigravity → Anthropic 顺序**。

总周期估 **14-20 周**（提案 12-18 周 +2 周 buffer 给 22 RPC 的额外工作量）。

---

## 1. POC-A 关键结论：流式可行（待实测确认）

> 完整内容见 `GATEWAY-POC-A-STREAMING.md`。

- **阈值合理**：跨进程 TTFB P99 增量 ≤ 2 ms。基于现状网关 TTFB P99 1.5-3 s，2 ms 占比 ≤ 0.15%，肉眼无感；同时远低于 OpenAI chunk-to-chunk 抖动（30-80 ms）不会被噪音掩盖。
- **预算来源**：gRPC bidi loopback 单向 80-200 μs（双向 0.16-0.4 ms）+ 5× 余量。
- **测试矩阵**：24 组（mode × concurrency × frame_count × payload_size），每组 warm-up 30 s + 采集 60 s + 重复 3 次。
- **回退决策树**：
  - ≤ 2 ms → 阶段二实施
  - 2-3 ms → 方案 A（透明 byte stream，不解码 protobuf）
  - 3-5 ms → 方案 B（plugin 监听 HTTP，host 反向代理，绕过 gRPC）
  - \> 5 ms → 方案 C（owner-only，不抽 provider — 实质放弃整体方案，重审）
- **WebSocket 路径**（OpenAI Realtime）**不在 PoC 内**。理由：SSE 是单向 server-stream，已能外推；WS bidi 已在 `openai_ws_forwarder.go` 验证；分开评估降低 PoC 范围。

**对提案的影响**：无。Q2 由实测回答；目前仍维持 §10 Q2 的"待 PoC"状态。

---

## 2. POC-B 关键结论：token provider 整体留 host（推翻提案 §10 Q3）

> 完整内容见 `GATEWAY-POC-B-TOKEN-PROVIDER.md`。

**反提案 / 关键发现**：

1. **现有并发保护已成熟**：`OAuthRefreshAPI` 已经是"进程内 mutex + Redis 分布式锁 + DB CAS (`_token_version`)" 三层，零修改即可继续工作。把 token provider 拆到 plugin 反而要在 plugin 端重复实现这套保护，且会破坏"invalid_grant 重读 DB 比较版本"的恢复逻辑。
2. **Account.Credentials 是明文 JSON**：DB 里没加密（`backend/internal/repository/account_repo.go:341-441`）。host 唯一的应用层加密（`SecretEncryptionServer`）只用于 plugin Settings/Secret，不覆盖账号凭据。所以"host 加密 / plugin 解密"边界目前不存在。
3. **Bedrock SigV4 与 Vertex Service Account 必须留 host**：长寿 IAM access key + secret + Vertex private key 一旦出 host 进程，泄漏成本极高。host 暴露 `SignBedrockRequest` / `MintVertexAccessToken` RPC，plugin 只拿短期凭据。
4. **OAuth refresh_token 同理**：长期 refresh_token 不应给 plugin；plugin 调 `GetAccountAccessToken`，host 内部跑现有 token provider 返回短期 access_token。

**推荐 SDK 简化（POC-B §4）**：
- `GetAccountAccessToken(account_id, scope)` — 替代 plugin 内 token provider
- `SignBedrockRequest(account_id, method, url, headers, payload_hash)` — Bedrock 专用
- `MintVertexAccessToken(account_id, scope)` — Vertex 专用（与 GetAccountAccessToken 可合并按 type 分发）
- `MarkAccountTempUnschedulable(account_id, until, reason)` — 替代 antigravity 的 SetTempUnschedulable
- `MarkAccountError(account_id, code, message)` — plugin 上报 401 / invalid_grant

**对提案的影响（要应用到 PROPOSAL §5.2 / §10 Q3）**：

| 提案原列表 | 修订 |
|---|---|
| `UpdateAccountCredentials` | **删除**（plugin 不写 credentials） |
| `GetAccount`（含解密 credentials） | **改名 `GetAccountSnapshot`**（不返回 refresh_token / aws_secret / private_key） |
| ❌ 提案没明确的 token RPC | **新增 `GetAccountAccessToken`**（合并 OAuth + Vertex 流程） |
| ❌ 提案没明确的 Bedrock RPC | **新增 `SignBedrockRequest`** |
| ❌ 提案没明确的状态 RPC | **新增 `MarkAccountTempUnschedulable / MarkAccountError`** |

---

## 3. POC-C 关键结论：RecordUsage 留 host + 22 RPC + per-platform 灰度

> 完整内容见 `GATEWAY-POC-C-DEPENDENCY-MAP.md`。

### 3.1 RecordUsage 必须留 host（回答 §10 Q10）

`recordUsageCore` 单次调用扇出 **8 个写入目标**：5 个 DB/Redis 直连（usage_logs / usage_billing tx with RETURNING / users.balance / api_keys redis / quota redis）+ 3 个异步通知。同步部分 6-35 ms，包含事务依赖关系。搬到 plugin 等于把整个 billing 子系统迁过去，得不偿失（370 行代码、新增 4-5 个 RPC、双写期一致性更难保证）。

**结论**：plugin 调单次 `RecordUsage` RPC，把 `ForwardResult + ChannelUsageFields` 送回 host 跑现有 `recordUsageCore`。提案 §5.2 已含此 RPC，**保持不变**。

### 3.2 Stable surface 实际是 22 RPC（提案 §5.2 是 11，要补 11 条）

POC-C §2 的完整表给出每条 RPC 的现有调用点 + 行号。提案 §5.2 漏掉的 11 条主要是横切关注点和小工具：

| 提案 §5.2 漏掉的 RPC | 用途 | 调用频率 |
|---|---|---|
| `IncrementAccountRPM` | Anthropic OAuth RPM 计数 | 每请求 1 |
| `IncrementWait` / `DecrementWait` | 并发 wait 队列 | 每请求 1-2 |
| `UpdateSessionWindow` | 响应头解析后更新限流窗口 | 每成功响应 1 |
| `HandleUpstreamError` | 401/403/429/529 设置封禁 | 错误路径 1 |
| `ResolveErrorPassthroughRule` | 错误透传规则 | 错误路径 1 |
| `ResolveChannelMapping` | 渠道模型映射 | 每请求 1 |
| `ResolveTLSProfile` | TLS 指纹 profile | 每请求 1（轻量，可随 Account 返回） |
| `GetOrCreateFingerprint` | 客户端身份指纹 | 每请求 1 |
| `LookupDigestSession` / `SaveDigestSession` | OpenAI digest 会话 | 每请求 0-1 |
| `CalculateCost` | 模型价格计算（host 持 LiteLLM 数据） | 每请求 1 |
| `IncrementAccountInternal500` | antigravity INTERNAL 500 渐进惩罚 | 错误路径 1 |

5 条参数小、每请求 1-2 次（CalculateCost / ResolveChannelMapping / GetOrCreateFingerprint / ResolveTLSProfile / DigestSession）— 跨进程代价可控；其余 6 条仅错误路径触发，频率低。

### 3.3 双写灰度推荐 per-platform manifest-driven（回答 §10 Q9）

**现状**：仓库无通用 feature flag 系统。

**推荐方案**：阶段二的 `ProviderRegistry` 注册即灰度 — `(platform, protocol)` 没注册 plugin 时 host 自动 fallback 到内置实现。OpenAI plugin 先装上 → 看 1 周 → Antigravity → 最后 Anthropic。**故障即降级**，不需要单独的 toggle。第二阶段补 `groups.gateway_plugin_override` 列做 per-group 灰度。

**避免**：`if cfg.UseGatewayPlugin { ... } else { ... }` 散落在 handler 里 — 违反解耦原则。

### 3.4 跨边界 DTO 警告

POC-C §6 列了 4 个 DTO（ParsedRequest / ForwardResult / ClaudeUsage / OpenAIForwardResult / OpenAIUsage）的字段。两个特殊点：
- `ParsedRequest.OnUpstreamAccepted` 是闭包，**跨进程不可序列化**，必须改成 `host.NotifyUpstreamAccepted(request_id)` 信号 RPC。
- `OpenAIForwardResult.ResponseHeaders` (http.Header) 不应每次跨进程拖完整头表；改为 plugin 主动 `host.UpdateCodexSnapshot(account_id, headers)` 单独上送。

---

## 4. Open Questions 收敛状态

| # | 问题 | 当前答案 | 状态 |
|---|---|---|---|
| Q1 | Manifest 字段命名 `Protocol` + `RequiredAccountPlatform`？ | **YES** — 激进版（用户已拍板） | ✅ 已收敛 |
| Q2 | 流式 P99 增量 ≤ 2 ms？ | POC-A 设计就绪，待 implementer 跑 24 组实验 | ⏳ 待 PoC |
| Q3 | 凭据管理放哪？ | **SDK 提供通用 OAuth + API Key 抽象 + 支持自定义鉴权注册**。Plugin 拿完整 credentials，refresh / 锁 / cache 由 SDK 封装。POC-B 的并发保护结论仍有效，但实现方式从"token 留 host"改为"SDK 内 singleflight + host Redis 锁 RPC"。 | ✅ 已收敛（用户修正） |
| Q4 | OpenAI 端点是否预留 `(antigravity, openai)`？ | 待用户业务判断（antigravity 是否真提供 openai 协议） | ⏳ 用户判断 |
| Q5 | `gateway.endpoint.owner` 是否合并进 `gateway.provider`？ | 倾向合并（提案推荐） | ✅ 倾向定 |
| Q6 | Gemini `/v1beta/*` 的 google-auth 鉴权链？ | POC-C §3.3 没专门处理；建议 endpoint manifest 加 `AuthType="apikey-google"` 并复用现有 `APIKeyAuthWithSubscriptionGoogle` 中间件 | ⏳ 阶段二实施时定 |
| Q7 | `(platform, protocol)` 多 plugin 注册策略？ | 拒启动（提案倾向） | ✅ 倾向定 |
| Q8 | provider plugin 重启时进行中请求？ | 立即 fail（提案倾向） | ✅ 倾向定 |
| Q9 | 双写灰度方案？ | **per-platform manifest-driven**（POC-C §7），fallback 即降级 | ✅ 已收敛 |
| Q10 | RecordUsage 是否留 host？ | **YES**（POC-C §4） | ✅ 已收敛 |

**7/10 已收敛**，1 个等用户判断（Q4 OpenAI 业务判断），2 个等执行（Q2 PoC、Q6 阶段二）。

---

## 5. 提案修订摘要（应用到 PROPOSAL 的补丁清单）

不立刻改 PROPOSAL（避免文档反复）；阶段一启动前一次性合并修订到 v1。要打的补丁：

| 章节 | 修订 |
|---|---|
| §5.2 HostService RPC | 从 11 RPC 扩到 22 RPC；细化 token 相关为 4 件套（GetAccountAccessToken / SignBedrockRequest / MarkAccountTempUnschedulable / MarkAccountError） |
| §5.2 删除 | `UpdateAccountCredentials` 删除（refresh 留 host） |
| §5.2 改名 | `GetAccount` → `GetAccountSnapshot`（不返回长期凭据） |
| §10 Q3 | 状态从"待调研"改"已收敛 / NO" |
| §10 Q9 | 状态从"待调研"改"已收敛 / per-platform manifest-driven" |
| §10 Q10 | 状态从"倾向留 host"改"已收敛 / YES" |
| §6 阶段二 | 加 PoC-A 12 步 checklist 引用 |
| §7 风险表 | "OAuth refresh 跨进程并发刷新" 风险**消除**（token 留 host） |
| §7 风险表 | "Bedrock / Vertex 凭据泄漏" 风险**消除**（host 暴露 sign/mint RPC） |
| §3 ASCII 流程图 | 第 5 步 `HostService.Forward` 改为 plugin 直接 dial provider 的 mediator 模式（POC-A §3.3）|
| 新增 §6.0 | 阶段一前置：跑 PoC-A，达标再进阶段二 |

---

## 6. 推荐的下一步行动

### 立即执行（不需用户决策、低风险）

**A. 启动阶段一：host 内 GatewayProvider 接口重构**

- 工期：2-3 周
- 范围：把三套 forward 抽成 `GatewayProvider` 接口（提案 §6 阶段一），每 platform 一个实现仍在 host 内
- 价值：即便最终不抽 plugin，god object 收敛是必赚（24500 → 4 个 ≤6000 行的清晰文件）
- 风险：零跨进程风险；本质上是单一 commit 重构，可灰度滚动
- 输出：`GatewayProvider` 接口定义 + 三 platform 实现 + 单一 pipeline (`acquire → forward → consume → record`) 替代现有 6 处 if/else
- 任务命名建议：派 Plan agent 写 `GATEWAY-EXTRACTION-PHASE-1-PLAN.md`，再派 implementer agent 实施

**B. 并行执行：PoC-A 流式实验**

- 工期：1 周
- 范围：按 POC-A §6 的 12 步 checklist 执行
- 输出：`docs/plugin-architecture/poc-a-results.md` + 决策树判定
- 风险：PoC 代码不入主干，独立分支 `feat/plugin-poc-streaming`

A 和 B 可以并行（不同分支、不同人 / agent）。

### 等用户拍板（中等决策）

**C. Q1 / Q4 字段 + 业务判断**

需要你回答：
1. **Q1**：Manifest 字段命名取 `Protocol` + `RequiredAccountPlatform`（推荐）还是 `InputProtocol` + `ForceAccountPlatform`（与现有 `middleware.ForcePlatform` 词根呼应）？
2. **Q4**：OpenAI 端点（`/v1/chat/completions` / `/v1/responses`）是否需要预留 `(antigravity, openai)` 注册位（即 manifest 不写 `RequiredAccountPlatform="openai"`）？取决于 antigravity 后续会不会真的暴露 OpenAI 兼容协议。

### 阶段二（PoC-A 通过后）

**D. SDK 扩展 + Mediator 实施**

- 前置：PoC-A 达标
- 工期：3-4 周
- 范围：22 个 host RPC + GatewayProviderExtension proto + ProviderRegistry + Mediator 实现 + gateway-anthropic-stub 集成测试

### 阶段三（按风险递增）

**E. 三 plugin 实施**：OpenAI → Antigravity → Anthropic（每个 4-6 周）

---

## 7. 给用户的判断题（回来时优先回答这 3 个）

1. **是否同意启动阶段一 host 内重构 + 并行 PoC-A**？这两个独立有价值、低风险。等于"先把 god object 拆成接口、同时验证流式延迟假设"。
2. **Q1 字段命名取激进版（`Protocol` + `RequiredAccountPlatform`）还是保守版（`InputProtocol` + `ForceAccountPlatform`）**？前者更优、后者沿用现有词根。
3. **POC-B 推翻的 §10 Q3 决策（token provider 留 host）你同意吗**？这个决定 SDK 形状是否要朝"短期 token 透传 + Bedrock/Vertex sign RPC"演进。如果你倾向"plugin 自己持 OAuth refresh"，需要给充分理由（POC-B §5 列了所有反对论据）。

回答完这 3 个，整体方案就可以进入 Plan / 实施阶段。

---

## 8. 文档导航

- `GATEWAY-EXTRACTION-PROPOSAL.md` — 整体设计（v0 草稿，待按 §5 补丁更新）
- `GATEWAY-POC-A-STREAMING.md` — 流式 PoC 实验设计（207 行）
- `GATEWAY-POC-B-TOKEN-PROVIDER.md` — Token provider 解耦分析（242 行）
- `GATEWAY-POC-C-DEPENDENCY-MAP.md` — 依赖图 + 22 RPC 全表（466 行）
- `GATEWAY-EXTRACTION-DECISION.md` — 本文件（决策建议）

5 份合计约 1500 行 / 90 KB。建议阅读顺序：DECISION → PROPOSAL §10 (Open Questions) → 三份 POC 按需翻。
