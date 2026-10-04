# 设计：OpenCode Zen 免费层门禁伪装

## 1. 问题定义

上游对免费层模型实施客户端身份门禁，判定依据是「这次请求是否由官方 OpenCode CLI 发起」。判定不是单一开关，而是四组独立信号的与：

1. 会话头形状（`X-Opencode-Session` 必须符合 `ses_` 规范）
2. 客户端版本（`User-Agent` 中的 `opencode/<version>` 必须 ≥ 1.18.0）
3. 工具集（body `tools` 必须同时含 `bash` 与 `read`）
4. 流式标志（body `stream` 必须为 `true`）

任一不满足即 `403 FreeTierError`。因此伪装必须是「四者同时满足」，不存在只补一部分的中间态。

## 2. 实测证据（2026-09-30）

对 `https://opencode.ai/zen/v1/chat/completions`，`Authorization: Bearer public`，模型 `mimo-v2.6-flash-free`。

| 编号 | 变量 | 结果 |
| --- | --- | --- |
| A | 仅 `Authorization` + `python-requests` UA | 403 FreeTierError |
| B | 全套头 + 官方 UA + `bash`+`read` + `stream:true` | **200** |
| C | 全套头，**无**工具 | 403 FreeTierError |
| D | 全套头 + 工具，会话用 UUID | 403 FreeTierError |
| E | 全套头 + 工具，UA = `opencode/1.0.0` | **426 UpgradeRequired** |
| F | 官方 UA + 规范会话，无工具无三头 | 403 FreeTierError |
| G | 官方 UA + 工具，无任何 Opencode 头 | 403 FreeTierError |
| I/J/K | 分别缺 `X-Opencode-Request` / `Client` / `Project` | 均 **200** |
| L | UA = `opencode/1.18.0` | **200** |
| M | UA = `opencode/1.17.9` | **426 UpgradeRequired** |
| M2 | UA = `opencode/1.19.0` | **200** |
| P | 全套头 + 工具，`stream:false` | 403 FreeTierError |
| R | 会话头 = `abc` | 403 FreeTierError |
| T | **仅** `X-Opencode-Session`（无 Request/Client/Project） | **200** |
| U | 只有 `bash` 工具 | 403 FreeTierError |
| V | 只有 `read` 工具 | 403 FreeTierError |

结论：

- 版本下限精确落在 **1.18.0**（1.17.9 拒绝、1.18.0 通过）。
- `bash` 与 `read` **必须同时存在**，任一单独都不够。
- `X-Opencode-Request` / `Client` / `Project` **非必需**。实现里仍然发送，是为了与官方客户端流量保持形态一致、降低后续规则收紧时的回归风险，但它们不是验收条件。
- 会话头是**形状**要求而非「存在」要求（`abc` 被拒）。

### 验证边界

- `muse-spark-1.3-contributor-free` 与 `mimo-v2.6-flash-free` 在 `/zen/v1/responses` 上返回 `403 RegionError`（"This model is not available in your country."），属地理封锁而非门禁失败。因此 **Responses 方言的门禁无法从当前网络验证**，其实现按 Chat 方言同构推导，并依赖单元测试覆盖。
- `/zen/go` 付费链路的版本下限无法判定：`public` 密钥在该路径返回 `401 AuthError`，认证先于版本检查失败。改用 ≥ 1.18.0 的 UA 严格优于现状，故按修复处理。

## 3. 关键设计决策

### 3.1 作用域：账号级开关，默认只作用于 `-free`

注入的 `bash` / `read` 对模型可见，模型可能真的发起同名工具调用，因此不能无差别作用于付费流量。

凭据 `free_tier_gate` 三态：

| 取值 | 行为 |
| --- | --- |
| `auto`（缺省） | 仅模型 ID 以 `-free` 结尾时伪装 |
| `always` | 该账号全部 OpenCode 请求伪装（上游规则收紧时的应急开关） |
| `off` | 关闭 |

未配置时等同 `auto`，既有账号行为在付费模型上完全不变，可无风险上线。

### 3.2 配置载体：账号凭据，不新增数据库列

仓库既有范式是 `protocol_rules` 存在 `accounts.credentials` JSONB 中，由 `NormalizeOpenCodeGoProtocolRulesCredentials` 在账号写入路径归一化校验。`free_tier_gate` 完全复用该路径：无迁移、无 ent 重新生成、无前端强依赖。非法取值返回 400 `INVALID_OPENCODE_FREE_TIER_GATE`。

### 3.3 会话 ID：确定性铸造而非随机 UUID

现状是 `uuid.NewString()`（`openai_opencode_session.go:169`），既不符合形状要求，又每轮变化——即使形状修好，随机值也会让上游 prompt cache 永远不命中。

改为确定性铸造，与 CPA 插件 `canonicalID` 算法一致：

```go
sum := sha256.Sum256([]byte(source))
hexPart := hex.EncodeToString(sum[:6])          // 12 位小写 hex
const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
suffix := make([]byte, 14)
for i := 0; i < 14; i++ {
    suffix[i] = alphabet[int(sum[6+i])%len(alphabet)]
}
id := prefix + "_" + hexPart + string(suffix)   // ses_ / msg_
```

种子沿用现有解析优先级（客户端 `X-OpenCode-Session` → `prompt_cache_key` / `metadata.user_id` → 账号覆写），因此同一会话跨轮次稳定。仅当所有稳定标识都缺失时才退回随机值。

**注意**：客户端已经传来的、形状合法的 `ses_` 值必须原样保留，不能被重新哈希——否则会破坏调用方自建的会话连续性。

**不能依赖既有链路补值。** 实现过程中发现：`applyOpenCodeSessionHeader` 只在 `shouldGenerateOpenCodeSession` 为真时生成兜底值，而该条件要求账号为 Go 订阅、或目标路径含 `/zen/go`。**Zen 账号两条都不满足**（Zen 的 base URL 是 `/zen/v1`）。于是「Zen 账号 + 客户端未提供任何会话标识」这一组合下出站请求完全没有会话头，门禁第一项硬要求直接不成立。

因此门禁必须自行派生，顺序为：

| 优先级 | 来源 | 稳定性 |
| --- | --- | --- |
| 1 | 已落地的会话头（含客户端提供的合法 `ses_`） | 客户端决定 |
| 2 | 请求体 `prompt_cache_key` / `metadata.user_id` | 跨轮次稳定 |
| 3 | 模型名 + 首条用户消息（内容派生） | 跨轮次稳定 |
| 4 | 随机值兜底 | 每轮变化，仅保证能过门禁 |

第 3 级与 CPA 插件的 content fallback 同构：首条用户消息在同一会话内不变，因此派生的会话值跨轮次稳定，prompt cache 仍能命中。只有第 4 级会牺牲缓存，属最后兜底。

### 3.4 注入点：三条 funnel，一个共享助手

出站 body 没有统一收口点，三条原生协议各自 finalize：

| 协议 | funnel | body 注入点 |
| --- | --- | --- |
| Chat Completions | `sendCCUpstreamRequest` | `openai_gateway_cc_pipeline.go`，`ensureDeepSeekChatReasoningPlaceholders` 之后、`NewRequest` 之前 |
| Responses | `buildUpstreamRequest` | `openai_gateway_forward.go`，`normalizeDeepSeekResponsesRequestBody` 之后、`NewRequest` 之前 |
| Anthropic Messages | `buildNativeAnthropicUpstreamRequest` | `openai_gateway_messages_anthropic_native.go`，`clampOllamaCloudAnthropicMessagesMaxTokens` 之后、`NewRequest` 之前 |

头部注入复用既有 `applyOpenCodeSessionHeader` 调用点（三条 funnel 各一处）。`openai_gateway_passthrough.go` 对 OpenCode 不可达（要求 `Platform == PlatformOpenAI`），不改。

采用仓库既有惯用法：纯函数 `applyXxx(...) ([]byte, error)`，`gjson` 读、`sjson` 写，形状不符时原样返回而不是报错。

### 3.5 定价：不改动任何既有逻辑

**结论：本能力不引入任何免费层定价特判，`billing_service.go` 保持原样（零 diff）。**

设计过程中曾两次尝试改动定价，均被否决，理由记录如下，以免后续再犯。

**尝试一（否决）**：在 `getFallbackPricing` 顶部加「`-free` 一律零价」分支。

**尝试二（否决）**：改为「剥离 `-free` 后缀后递归走正常定价链，查到则同价、查不到记零价」。

两次尝试的共同问题是**擅自新增了一套原代码没有的定价规则**。定价是资金安全敏感的既有子系统，免费层不是引入新定价语义的正当理由。正确做法是让免费层模型沿用它们本来就会走到的路径。

**为什么不改也不影响功能**：查不到价**不会拒绝请求**。`openai_gateway_usage.go:253-267` 捕获 `ErrModelPricingUnavailable` 后记零成本并打 `openai_usage.pricing_missing_record_zero_cost` 告警，请求照常被上游服务。所以免费层模型在本能力引入前后都能正常使用。

**免费层模型的实际计费结果（既有行为，未改动）**：取决于模型名是否恰好包含某个付费家族子串。`deepseek-v4-flash-free` 含 `deepseek-v4-flash` 子串，命中兜底价卡 DeepSeek 分支，按付费价计费；其余 10 个免费层模型无家族匹配，走「无价可循」，记零成本并告警。这一结果在同一免费层内并不一致，但它是既有行为。**若运营者需要为免费层统一计费或统一免计费，正确手段是分组/渠道定价**（支持精确、大小写不敏感与尾随 `*` 通配，见 `model_pricing_resolver.go:161`），而不是改动定价解析代码。

**关于「未知型号」的既有语义**：`getFallbackPricing` 以 `return nil` 收尾（`billing_service.go:1223`），且注释明确「未知型号不回退以避免误计价」（`:1055-1056`）——内置价卡**刻意没有通用兜底价**。这是仓库的有意设计，不是缺陷，本能力不改动它。

### 3.6 与参考实现对保真度

门禁的验收标准只是「四项硬性要求」，但参考实现 cpa-plugin-opencodezen 的伪装范围更大。对照它补齐了三处保真度差异。

| 项 | 参考实现 | 我们的最小集 | 处理 |
| --- | --- | --- | --- |
| 会话头 | `X-Opencode-Session` + 镜像到 `X-Session-Affinity` / `X-Session-Id` | 只有 `X-Opencode-Session` | **已补齐镜像** |
| 工具集 | 官方 6 个（bash/edit/glob/grep/read/write），官方 schema，按名排序 | 只有 bash+read，最小 `{"type":"object"}` schema | **已补齐** |
| 无工具请求 | 注入全套 + `tool_choice: none` | 注入 bash+read，不设 `tool_choice` | **已补齐** |
| `X-Opencode-Request` / `Client` / `Project` | 全部发送 | 不发 | 保持不发（实测非必需） |

**实测验证**（真实上游，2026-10-04）：

- 6 官方工具 + affinity 头 → 200
- 6 官方工具 + `tool_choice: "none"` → 200
- 只发 `X-Opencode-Session`、不发 affinity → 200（确认 affinity **不是**门禁要求）

**为什么补齐而不是停在最小集**：门禁是上游单方面实现的校验，规则会变。最小集只能保证「今天能过」；与官方客户端形态越接近，上游收紧规则时的回归风险越低。affinity 头与 6 件套都属于这类防御性对齐。

**`tool_choice` 的必要性**：补齐工具集会让原本无工具的请求（压缩/总结、纯聊天客户端）带上工具，模型可能去调用客户端根本无法执行的工具。参考实现用 `tool_choice: none` 解决，本能力照做。Anthropic 方言的 `tool_choice` 是对象而非字符串，用 `{"type":"none"}`。

**关于 `jev-1.13-free`**：参考实现明确把它排除在注册之外（其提交信息为 "unsupported dialect models are not announced, specifically handling the jev-1.13-free model"），因为它走 `/systemone` 方言，而该方言属于独立的 `typesafe` 平台。本能力的目录仍包含它——运营者自行选择使用哪些模型，且实测该模型在 chat 与 responses 两个方言上均返回 500。

## 4. 风险与缓解

| 风险 | 缓解 |
| --- | --- |
| 注入工具导致模型发起 `bash`/`read` 调用 | 默认只作用于 `-free`；`off` 可完全关闭；响应侧过滤列为后续跟进 |
| 免费层计费结果不一致（有的按付费价、有的记零成本） | 既有行为，本能力不改动；运营者用分组/渠道定价（含 `*` 通配）统一口径 |
| 误改定价子系统引发资金风险 | 明确列为非目标；`billing_service.go` 保持零 diff，spec 设「不改动定价逻辑」需求钉住 |
| UA 常量变更影响付费链路 | 新值满足版本下限且为官方客户端形态，严格优于 `opencode/1.0.0`；回归测试覆盖 |
| Responses 方言无法真实联调 | 单元测试按方言断言注入形状；文档记录验证边界 |
| 上游规则再次收紧 | `always` 开关可把伪装扩大到该账号全部请求，无需改代码 |

## 5. 非目标

- **不改动任何定价逻辑**：不新增针对 `-free` 的定价特判，不修改 `getFallbackPricing` / `applyModelSpecificPricingPolicyEx`。免费层模型的计费结果与本能力引入前完全一致；需要调整计费口径时走分组/渠道定价配置。
- 不做上游 `-free` 模型的额度/配额管理。
- 不改动 Responses SSE 重拼与 keep-alive 过滤（CPA 插件的另一项能力，与本门禁正交）。
- 不为其他 provider 引入通用「出站 body 改写」框架。
