# Design — add-devin-platform

## 上游协议来源与权威顺序

1. **本地 devin-connect 插件**（`~/.pi/agent/extensions/devin-connect/`）是 wire 层权威：Connect 帧格式、proto 字段号、CLI 元数据头、PKCE 参数、目录解码、trajectory/step_index 归属、AssignModel 路由、overflow 重试均按插件实现逐行移植。
2. **WncFht/devin2api**（Go 参考实现，克隆于 `/tmp/devin2api`）是格式转换参考：`internal/llm` IR、`internal/adapter/devin` 请求/响应编解码、`internal/api` 三协议 codec 与 WS 会话机。复制其结构并重命名/裁剪，而非 import。
3. 字段号与 `all-protos.proto`（插件抓包标注 v3000.10.21）交叉验证一致。

## 关键决策

### 入站 = 标准 API 翻译，不是 Connect-RPC 透传

客户端继续发标准 Responses / Chat Completions / Anthropic Messages；`internal/pkg/devin/api/*` codec 把请求译成 `llm.RequestMessages` IR，adapter 再编成 Connect 帧。响应事件流经各协议 encoder 回写对应 SSE 格式。

### 手工 wire codec（不走 protoc）

上游 proto 未公开完整描述符，插件按抓包字段号手编手解。`internal/pkg/devin/proto.go` 实现最小 wire 读写（varint/len-delimited/嵌套 message），字段号常量与插件捕获一一对应。

### 模型目录分组（thinkingLevelMap）

上游 `ListModels` 返回分级 uid（`swe-2-max`、`claude-fable-5-1-xhigh`…）。`GroupModels` 按"族 + variant"聚合为对外 `GroupedModel{ID, ThinkingLevelMap}`：`swe-2` 等族名是对外模型 id，`effort` 请求参数经 ThinkingLevelMap 选 uid。无分级条目（plain）按 uid 原样展开。

### 会话与 step_index

- 会话 id 由请求首条消息内容 sha256 → 确定性 UUID 派生（无服务端会话表）。
- `step_index` 首个请求为 0，后续 2,3,4…（上游标题生成消费 index 1）。
- 助手历史回合合并为单条 prompt（插件行为）；省略 f13/f18/f22 字段。

### Failover 分类

`devin.InvalidRequestError`（`NewInvalidRequest`/`IsInvalidRequest`）包裹 adapter 内所有本地请求形状错误（validate、空 model、图片校验、路由解析、参数构建）；`IsTransientTransportError` 对其返回 false → `DevinFailoverError` 判为 400 `NextAccountStop`。未包裹的非 Connect 错误视为真实传输失败 → 502 failover。

### Responses WebSocket 传输

- 复用 `forward()` 逐回合转发：每个 WS 回合用 `gin.CreateTestContext(devinWSResponseWriter)` 伪造内部 `POST /v1/responses` 请求（透传 Authorization / API Key / 会话头等），直接调用 `h.forward`——计费/并发/failover 路径零分叉。
- `devinWSResponseWriter` 实现 http.ResponseWriter+Flusher：SSE `\n\n` 帧切分为 WS text 帧，`:` 注释转 WS Ping，`message_too_big` → close 1009。
- 会话状态机 `WSSession`（移植 devin2api）：增量 transcript 归并（call_id keep-first、id keep-last 去重）、孤儿 output 配对检查、pending tool call 门控、completed-transcript 检测（含 Codex 压缩摘要前缀）、model/instructions 继承、`stream=true` 强制。
- ingress 上限/首包超时/读限复用 `Gateway.OpenAIWS` 配置与 `AcquireOpenAIWSIngressLease`；不新增 devin 配置项。

### OAuth / token 登录

- `POST /admin/devin/oauth/auth-url`：生成 PKCE verifier/challenge + state，会话存 `AuthSessionStore`（进程内，TTL 10min）。
- `POST /admin/devin/oauth/exchange-code`：code 为 `devin-session-token$…`/JWT/长 hex（`devin.LooksLikeAPIKey`）时**免会话**直接建号；否则校验 session+state，依次尝试 `ExchangePKCEAuthorizationCode` → `ExchangeDevinCLIPKCECode`。
- 不使用 `POST api.devin.ai/auth/cli/token`（插件实测 web-session JWT 会被 server.codeium.com 拒收）。
- 交换后 `GetUserStatus` 拉 email/org/plan 写 extra；失败不阻断建号。
- Devin 无 refresh token（session token 长效）；换绑 = ReAuthAccountModal 重走同一流程或直贴新 token。

### 计费与调度

- 复用 `SelectAccountWithLoadAwareness`（含 `IsModelSupported` 账号映射判定）、`AcquireUserSlotWithWait`、`AcquireAccountSlotWithWaitTimeout`、rateLimitService.HandleUpstreamError、billingCacheService 资格检查、`submitDevinUsage` 异步扣费（`result.Stream` 恒 true——入站一律强制流式）。
- 账号 `model_mapping`：选中账号后 `ResolveMappedModel` 命中即改写上游模型副本；`reqModel`（客户端模型）保留给调度/计费/响应回填。
- 已知与 `GatewayHandler.Messages` 的差距：Devin `forward` 暂无 profit-gate veto、sticky-session 绑定、wait-queue 计数——后续按需要补齐。

## 数据流

```
client (Responses/Chat/Anthropic JSON or WS)
  → routes/gateway.go 按分组平台分派 platform=devin
  → DevinGatewayHandler.{Messages,ChatCompletions,Responses,ResponsesWebSocket,Models}
  → decodeDevinRequest → llm.RequestMessages IR
  → SelectAccountWithLoadAwareness → ResolveMappedModel → devinGateway.Stream
  → adapter: session derive → model routing(AssignModel) → Connect frame encode
  → upstream server.codeium.com DevinService/GetChatMessage (stream)
  → response decode → llm.ResponseEvent → protocol encoder → SSE/WS frames
  → submitDevinUsage（worker pool 异步计费）
```
