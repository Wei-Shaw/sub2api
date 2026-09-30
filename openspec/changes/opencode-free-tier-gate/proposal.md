## Why

OpenCode Zen 的免费层模型（模型 ID 以 `-free` 结尾，官方目录当前有 11 个，例如 `mimo-v2.6-flash-free`、`deepseek-v4-flash-free`、`ling-3.0-flash-fin-free`）对请求施加了「仅允许官方 OpenCode CLI 发起」的服务端门禁。网关按普通 OpenAI 兼容客户端转发时，上游一律返回 `403 FreeTierError`：

```json
{"type":"error","error":{"type":"FreeTierError","message":"Error from provider (Console): OpenCode's free tier can only be used from within OpenCode"}}
```

实测（2026-09-30，`Authorization: Bearer public`）确认了四项硬性要求，缺一即 403，其中两项与网关现状直接冲突：

| 要求 | 实测证据 | 网关现状 |
| --- | --- | --- |
| `X-Opencode-Session` 必须匹配 `ses_` + 12 位小写 hex + 14 位字母数字（共 30 字符） | UUID → 403；`abc` → 403；规范值 → 200 | 发的是裸 UUID（`openai_opencode_session.go:169`），**不匹配** |
| `User-Agent` 必须为 `opencode/<version>` 且 version ≥ 1.18.0 | `opencode/1.17.9` → `426 UpgradeRequired`；`1.18.0`/`1.18.31`/`1.19.0` → 200 | 常量写死 `opencode/1.0.0`（`openai_opencode_session.go:24`），**直接吃 426** |
| body 的 `tools` 必须**同时**包含 `bash` 与 `read` | 无工具 → 403；仅 `bash` → 403；仅 `read` → 403 | 完全不注入 |
| body 的 `stream` 必须为 `true` | `stream:false` → 403 | 不改写，透传客户端取值 |

另外实测 `X-Opencode-Request` / `X-Opencode-Client` / `X-Opencode-Project` **非必需**（任一缺失仍 200），因此不把它们列为硬要求。

免费层模型的计费现状已实测（干净 HEAD），**本提案不改动它**，仅作背景说明。11 个免费层模型的定价解析结果并不一致：

| 模型 | 既有行为 |
| --- | --- |
| `deepseek-v4-flash-free` | 因含 `deepseek-v4-flash` 子串命中兜底价卡分支，按付费价计费（input 1.5e-07 / output 6e-07） |
| 其余 10 个 | `pricing not found` → 上游照常服务，记零成本 + `openai_usage.pricing_missing_record_zero_cost` 告警 |

两点容易误判，需要澄清：**查不到价不会拒绝请求**（`openai_gateway_usage.go:253-267` 捕获 `ErrModelPricingUnavailable` 后记零成本继续），因此免费层模型不加任何改动也能正常使用；而 `getFallbackPricing` 以 `return nil` 收尾、注释明确「未知型号不回退以避免误计价」，说明**内置价卡刻意没有通用兜底价**。

定价是资金安全敏感的既有子系统，**本提案明确将其列为非目标**：不新增针对 `-free` 的定价特判，`billing_service.go` 保持零 diff。运营者若需统一免费层的计费口径，用分组/渠道定价（支持尾随 `*` 通配）配置，不改代码。

`opencode/1.0.0` 这个 UA 同时影响 `/zen/go` 付费链路——版本下限检查在认证之前触发，属于存量缺陷，需要一并修复。

## What Changes

- 新增账号级凭据 `free_tier_gate`，取值 `auto`（默认）/ `always` / `off`，沿用 `protocol_rules` 的凭据扩展范式，**不新增数据库列、不需要迁移**。
  - `auto`：仅当请求模型 ID 以 `-free` 结尾时启用门禁伪装。
  - `always`：该账号全部 OpenCode 请求都伪装（用于上游规则收紧时的应急开关）。
  - `off`：完全关闭。
- 新增门禁伪装实现：注入规范 `ses_` 会话头、把出站 UA 收敛到满足版本下限的官方客户端身份、强制 `stream:true`、按方言补齐 `bash` + `read` 工具定义。
- 会话 ID 由确定性哈希铸造（`sha256` 前 6 字节转 12 位小写 hex，其后 14 字节对 62 字符表取模得 14 位后缀），保证同一会话跨轮次稳定，从而不破坏上游 prompt cache；沿用现有 `prompt_cache_key` / `metadata.user_id` 解析链作为种子来源。
- 修正 `openCodeUpstreamUserAgent` 常量到满足 `≥ 1.18.0` 的官方客户端 UA 串，覆盖 `go` 与 `zen` 两种模式。
- **不改动定价**：`billing_service.go` 保持零 diff，不新增任何针对 `-free` 的定价特判。免费层模型的计费结果与本提案引入前完全一致。
- OpenCode 内置模型目录补充官方免费层模型 ID，使 `/v1/models`、账号白名单预填与管理端可见。
- 门禁注入点覆盖全部三条原生协议 funnel（Chat Completions / Responses / Anthropic Messages），因为出站 body 没有统一收口点。

## Capabilities

### New Capabilities
- `opencode-free-tier-gate`：OpenCode Zen 免费层门禁的账号级开关、四项硬性要求的伪装实现、会话 ID 的确定性铸造、按方言的工具注入，以及模型目录配套。

### Modified Capabilities
<!-- openspec/specs 目前为空，没有既有能力需要修改。 -->

## Impact

- **数据库**：无。配置存于 `accounts.credentials` JSONB，复用既有列。
- **后端**：新增 `internal/service/openai_opencode_freetier.go`；修改 `openai_opencode_session.go`（UA 常量、会话 ID 规范化）、`opencode_go.go`（凭据归一化、免费模型目录），以及三条出站 body funnel：`openai_gateway_cc_pipeline.go`、`openai_gateway_forward.go`、`openai_gateway_messages_anthropic_native.go`。**`billing_service.go` 与任何定价路径均不修改。**
- **管理端 API**：账号创建/更新路径新增 `free_tier_gate` 凭据校验与归一化；非法取值返回 400。
- **前端**：账号创建与编辑弹窗的 OpenCode 区块新增「免费层门禁伪装」下拉（仅免费模型 / 全部请求 / 关闭），配套 `credentialsBuilder` 的解析与写入助手、中英文 i18n 与 Vitest 用例。缺省 `auto` 不写入凭据，使既有账号行为不变。
- **上游影响**：仅对启用门禁的账号改变出站头与 body；`auto` 模式下付费模型流量完全不受影响。
- **已知取舍**：注入的 `bash` / `read` 工具对模型可见，模型理论上可能发起同名工具调用。免费层模型以 `-free` 为界、且默认只作用于免费模型，风险可控；响应侧过滤留作后续跟进项。
