# 自动密钥保护

自动密钥保护在模型请求发出前，将对话中**受支持格式的文本密钥**替换为 `keyx_` 占位符，并在响应返回客户端时还原已授权的占位符。用户仍可照常粘贴密钥，无须逐个登记外部服务凭据、配置服务环境变量或修改提示词格式。功能默认关闭。

本次改动基于实际工作区版本 [`bdb42e22f81fcb633ff0a060961211dd2bcb515b`](https://github.com/Wei-Shaw/sub2api/tree/bdb42e22f81fcb633ff0a060961211dd2bcb515b)。该版本已有日志与审核摘要脱敏；本次新增独立的正文转换、授权映射和客户端响应还原。

## 开启与日常使用

管理员进入 **系统设置 → 安全 → 自动密钥保护**，选择用户或 API 密钥分组、运行模式和还原范围，然后点击该卡片内的“保存密钥保护设置”。卡片独立保存。两项适用名单都为空时适用于所有用户；否则命中用户或其平台 API 密钥所属分组即可生效。总开关关闭或未命中名单的请求沿用原有处理路径。

开启可逆模式前，部署管理员须已配置固定的 `TOTP_ENCRYPTION_KEY`，所有实例使用同一已有平台密钥。未配置时保存会返回 `KEY_PROTECTION_PLATFORM_KEY_REQUIRED`，界面提示部署管理员处理。这是平台统一的加密材料，不要求每位用户配置任何外部服务密钥；仅遮盖模式不需要该映射加密基础。

| 设置 | 默认值 | 含义 |
| --- | --- | --- |
| `enabled` | `false` | 总开关 |
| `user_ids`、`group_ids` | `[]` | 服务端认证主体对应的内部编号；两项为空适用所有人 |
| `mode` | `reversible` | `reversible` 可逆替换；`redact` 仅替换为 `[REDACTED]` |
| `restore_scope` | `text_and_tools` | 回复文本及工具参数；可选 `tools_only` 仅工具参数 |
| `rules` | `[]` | 留空启用全部内置规则；填写时选择对应规则集合 |
| `custom_rules` | `[]` | 管理员附加 RE2 规则，日常用户无须接触 |
| `ttl_seconds` | `3600` | 持久映射有效期，允许 60～2592000 秒 |
| `max_mappings` | `256` | 单份请求/会话映射的密钥条目上限，允许 1～10000 |
| `max_sessions` | `10000` | 每个用户/API 密钥/分组上下文内的有效存储记录上限，允许 1～1000000；会话及响应快照均计数 |

仅遮盖模式不建立可还原映射，生成的代码与工具参数无法自动获得原始密钥。仅工具参数还原模式下，普通代码块仍属于回复文本，不会还原。需要“粘贴后代码也能使用”的体验，应选择可逆替换及“回复文本和工具参数”。

以下配置示例是管理 API 的 JSON 请求体，**不是自动加载的部署配置文件**。示例保持 `enabled: false`；经管理界面开启后无需修改用户客户端。

```json
{
  "enabled": false,
  "user_ids": [],
  "group_ids": [],
  "mode": "reversible",
  "restore_scope": "text_and_tools",
  "rules": [],
  "custom_rules": [],
  "ttl_seconds": 3600,
  "max_mappings": 256,
  "max_sessions": 10000
}
```

```text
GET    /api/v1/admin/settings/key-protection
PUT    /api/v1/admin/settings/key-protection
DELETE /api/v1/admin/settings/key-protection/mappings
```

这些接口使用现有管理员认证与访问控制。配置保存在现有 settings 仓库的 `automatic_key_protection` 项，无新增数据表。PUT 最多接受 64 KiB JSON；错误字段、非法规则、无效模式和超限配置会被拒绝。设置不存在时返回关闭的默认配置；数据库读取失败或配置文档损坏会明确报错，不会悄悄关闭保护。每次请求读取并固定配置，更新不改变已经开始的请求。

## 识别范围

内置规则编号为 `openai`、`anthropic`、`github`、`gitlab`、`google`、`stripe`、`slack`、`huggingface`、`groq`、`npm`。规则结合明确前缀、长度、合法字符和 token 边界匹配，具体格式以 [core.go](../backend/internal/keyprotection/core.go) 为准；例如 GitHub 的 `ghp_` 加 36 位字母数字。它不会把所有 `sk` 开头的单词、网址、UUID 或长随机字符串一概当成密钥，也不会调用外部服务验证候选值。

扫描的是解析后的消息字符串，包括用户、系统和历史消息、工具返回、工具参数、相关工具描述及结构化输出描述。JSON 字符串形式的工具参数会解析内层 JSON 后再安全写回；数字使用 `json.Number`，不经浮点数转换。拒绝重复 JSON 字段、无效 UTF-8、过深结构和无法安全处理的内容。

协议 ID、模型名、工具名称、路由元数据、鉴权请求头和平台模型配置凭据不是正文密钥替换目标。不要把业务密钥放进这些非内容字段期待得到保护。图片、音频、文件和其他不透明媒体数据不执行 OCR 或解码识别；支持协议里的媒体块可保留，媒体中的密钥不在本功能承诺内。未知扩展字段中发现受支持格式的未处理密钥时会拒绝请求。

管理员可在高级设置中增加规则，例如下面的虚构格式。每条规则使用整个匹配作为替换目标，名称只用于分类；不要把凭据本身写进规则或名称。

```json
[
  {"name": "fictional_service", "pattern": "fictional_[A-Za-z0-9]{24}"}
]
```

最多 32 条自定义规则，每个表达式最多 2048 字符；规则名为小写字母开头的字母、数字或下划线，最多 48 字符，不能与内置规则或其他附加规则重名。该版本不支持匹配空字符串的规则。

## 实际数据流

```text
平台 API 密钥认证 / 主体绑定
  → 大小、编码、JSON 结构与支持范围检查
  → 本地识别 + 输入替换 + 必要映射存储
  → 路由、完整提示词快照、审核、正文记录
  → 协议转换、模型调用、重试或失败切换
  → 最终客户端协议的 JSON / SSE 解析
  → 当前授权映射内的完整占位符还原
  → 客户端及工具执行层
```

入口中间件同时替换 `Request.Body`、`Request.GetBody` 和 Gin 的正文缓存，确保审核、重读、重试和转换使用同一份受保护正文。映射不放进模型消息、提示词元数据、审核记录或错误详情。模型侧历史保持占位符形态。输出包装器位于协议转换后的客户端边界，返回的明文不会反馈给内部处理器。

诊断日志只记录运行模式、规则类别命中数量、转换耗时和固定失败原因；不记录明文映射或恢复后的完整正文。受保护请求的 Ops 错误响应捕获跳过正文。没有命中只表示“未发现受支持格式的密钥”，不表示内容不存在其他敏感信息。

输入阶段失败时直接返回保护错误，不会自动降级发送原文。输出阶段失败且尚未发送正文时返回固定错误；SSE 已开始时发送对应协议的失败事件并停止，取消或异常中断时丢弃未检查的缓冲尾部。

## 稳定占位符、会话与存储

生产入口使用 **HMAC-SHA256** 生成 `keyx_` 加 64 位小写十六进制标识，共 69 字符。平台密钥派生出独立的占位符用途密钥，再绑定认证后的 `UserID / APIKeyID / GroupID` 和可选会话标识。它不是直接对密钥作公开 SHA 哈希，也不包含真实密钥的前后缀。

同一安全上下文及会话内，相同密钥产生相同占位符。无会话标识时，同一认证 API 密钥上下文重发完整历史也产生稳定占位符。不同用户、API 密钥、分组或显式会话使用不同上下文。这里有意允许模型在同一上下文内看出“两个位置使用相同密钥”，以降低每轮更换占位符造成的缓存前缀变化。稳定标识不授予还原权限：还原仍必须命中本次授权映射，不会根据哈希遍历其他用户或会话的状态。

| 客户端方式 | 映射处理 |
| --- | --- |
| Chat Completions / Messages，无会话标识，每轮发送完整历史 | 本次映射仅在受控请求内存中保留；下一轮重新扫描历史中的原始密钥或已经恢复过的代码 |
| 提供现有会话标识 | 在认证主体上下文内使用加密会话映射，相同密钥复用占位符；并发更新通过 Redis 乐观事务合并 |
| Responses 普通或 SSE 请求 | 返回响应 ID 前保存该响应的加密映射快照，即使没有会话标识或没有命中密钥也需要 Redis 可用 |
| `previous_response_id` 续接 | 只读取认证上下文下对应响应快照；不存在或过期时拒绝并提示重发完整历史 |
| 只发送无法关联的历史占位符 | 不跨上下文查找；未知占位符保持原样，无法猜测还原 |

可复用的会话来源包括 `X-Sub2API-Session-ID`、`X-Claude-Code-Session-ID`、`session-id`、`session_id`、`conversation_id`、`X-Session-Affinity`、`X-Session-Id`、`X-OpenCode-Session`、`X-Conversation-ID` 请求头，以及已支持的 Claude `metadata.user_id` 会话编码。客户端传入的值只作会话选择，不决定用户身份；最长 512 字符。普通完整历史客户端不必增加任何请求头。

平台加密基础沿用 `totp.encryption_key` / 部署环境变量 `TOTP_ENCRYPTION_KEY`，要求固定的 32 字节十六进制密钥；**已有部署应沿用原有值**。映射加密与占位符使用不同的 HMAC 用途字符串派生密钥，不直接复用 TOTP 密文格式。多实例必须共享同一平台密钥和 Redis。配置为空时，项目原有加载逻辑会在每个进程启动时生成临时材料；自动密钥保护明确拒绝将它用于生产可逆模式，直接修改数据库配置也不能绕过这一运行时限制。只有固定平台密钥保持不变时，才能维持重启后的稳定标识及跨实例还原；不要为了启用本功能随意轮换现有平台密钥，以免同时影响已有 TOTP 数据。

Redis 仅保存 AES-256-GCM 密文及不透明索引，完整存储键作为附加认证数据，复制密文到另一个上下文不能获得映射。会话映射每次成功写入刷新 TTL，响应快照从创建时计时。每份记录最多默认 256 个密钥；会话和响应记录合计占用默认 10000 条的上下文容量。没有显式会话的 Chat/Messages 不需要读取 Redis 映射，但稳定占位符仍依赖平台派生密钥。

单个匹配值最多 1 MiB，每份映射的标识及原值总量最多 8 MiB。自定义规则匹配过大内容也会明确失败，不能绕过条目和内存限制。更换平台主密钥后旧映射不能解密，需要清理旧记录并重发完整历史；本版不提供多版本密钥轮换。

管理员可以清理全部已保存映射。该操作使已有占位符和响应续接失效，客户端需要重发含原始凭据的完整历史。**清理不撤销已经开始的请求内存状态**，在途请求仍可能还原并重新写入记录。需要完整清空时，先关闭保护并停止接收这批受保护流量，等待在途请求结束，再执行清理；不要在此维护窗口继续发送需要保护的原始密钥。清理也不等于在外部服务撤销凭据。

## 协议支持与明确拒绝的路径

下表描述受保护模式；功能关闭时不改变原有通道支持。

| 客户端 HTTP 入口 | 普通 JSON | SSE | 范围 |
| --- | --- | --- | --- |
| `/v1/chat/completions`、`/chat/completions` | 支持 | 支持 | 消息文本、代码、函数/工具参数，按 choice 和工具调用分别处理 |
| `/v1/responses`、`/responses`、`/backend-api/codex/responses` | 支持 | 支持 | 输入/输出内容、函数/自定义工具参数、主体校验后的 `previous_response_id` |
| `/v1/messages`、`/antigravity/v1/messages` | 支持 | 支持 | 消息/系统文本、工具块，按 content block 分别处理 |
| 原生 Gemini、WebSocket、Realtime | 拒绝 | 拒绝 | 返回保护错误；可使用上述已接入的 HTTP 兼容入口，但仍受原有平台路由能力限制 |
| Images、Audio、Video、异步任务、Embeddings、Count Tokens、Responses compact | 拒绝 | 拒绝 | 不允许通过未接入的正文路径透传 |
| GET models、usage、sub2api/billing | 沿用原有行为 | 不适用 | 不处理消息正文的查询 |

协议支持不等于支持协议的每一种扩展内容。签名请求头、带签名的历史思考块、`encrypted_content`、`redacted_thinking`、压缩续接块等不可安全改写的输入会明确拒绝。Responses 的 `background: true` 与 `conversation` 服务端会话引用也不在本版支持范围；使用完整历史或已授权 `previous_response_id`。部分客户端会自动回传带签名/加密历史，因此可能遇到这一明确边界。

SSE 按完整事件解析后定位文本及工具字段，不对任意网络块作全文替换。文本保留由占位符格式决定的有界尾部；不同 choice、内容块和工具调用有独立状态。工具参数按调用缓冲至 JSON 完整再还原，不把不同工具参数拼在一起；无关事件继续输出。完整响应和相应增量内容使用相同映射和还原范围。

入口复用已有的 gzip、zstd、deflate 请求解压与限制机制；输出请求 `Accept-Encoding: identity`，如果最终响应仍带未支持的压缩编码，输出包装器明确拒绝。输入受网关正文大小配置限制；本模块普通响应上限 16 MiB、单个 SSE 事件上限 4 MiB、工具参数累计缓冲上限 8 MiB，逻辑输出流数量也有限制。超限会失败，不会透传未经检查的数据。

## 缓存与计费

稳定 HMAC 标识减少相同历史因占位符变化造成的前缀抖动，但 JSON 重新序列化、协议转换、模型供应商自身缓存策略及完整历史重发仍可能改变缓存结果；不承诺与未启用时相同的缓存命中率。

受保护模式设置 `Cache-Control: no-store, private`，移除客户端响应的旧 `Content-Length` 和 `ETag`，避免把恢复后的明文作为共享结果缓存。现有未按保护上下文隔离的 reasoning 缓存读取及自动兼容续接被禁用；显式授权的 `previous_response_id` 由本模块单独绑定。部署方新增结果缓存时必须保留这条隔离边界。

受保护的 HTTP Responses 转发固定使用 HTTP，不复用原有 WS 连接或 turn-state 缓存。上游不支持等价 HTTP 时会返回错误，不会通过未验证的 WebSocket 通道绕过保护。

上游的 usage、token 和计费数据按原有通道处理；不会根据还原后字符长度重新估算用量。没有增加账号、支付或调度业务功能。

## 虚构密钥的三阶段对照

以下 `ghp_` 加 36 个 `A` 是虚构测试数据，`fake-model` 是模拟模型。示例占位符用单元测试中的公开固定种子（32 字节 `0x42`）计算，便于复现；**该种子不能用于部署**。生产环境由平台密钥及认证上下文派生，因此实际标识不同。模拟上游仅复制占位符，不调用 GitHub 或付费模型。

用户提交：

```json
{
  "model": "fake-model",
  "messages": [{"role": "user", "content": "请用 ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA 查询仓库，并给出代码。"}],
  "stream": false
}
```

模型上游收到（只展示客户端协议的等价请求；具体供应商转换可能调整外层结构）：

```json
{
  "model": "fake-model",
  "messages": [{"role": "user", "content": "请用 keyx_042a631f3210074debd0c896834d5a9024b1202715e5bce903bd673f3095e7a2 查询仓库，并给出代码。"}],
  "stream": false
}
```

模拟上游在回复文本及 `function.arguments` 的 `token` 值中复制上述占位符。最终客户端得到：

```json
{
  "id": "chat_fake",
  "choices": [{
    "index": 0,
    "message": {
      "role": "assistant",
      "content": "token = \"ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\"",
      "tool_calls": [{
        "id": "call_1",
        "type": "function",
        "function": {
          "name": "query_repository",
          "arguments": "{\"token\":\"ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\"}"
        }
      }]
    }
  }],
  "usage": {"prompt_tokens": 33, "completion_tokens": 9}
}
```

还原器只替换映射中的完整标识。模型改动、截断、扩展或编造标识时不作前缀或模糊还原；未知标识保持原样。相同密钥重复出现复用同一个标识，多密钥分别映射。引号、反斜杠、换行通过 JSON 解析和编码处理，而不是直接拼接工具参数。

客户端、代码执行器及工具在还原后仍会接触真实密钥，也仍拥有其使用权限。模型可能要求把占位符发送至错误目标，网关还原后仍可能造成泄露。此功能降低可识别密钥进入模型服务的机会，**不承诺消除凭据滥用**；服务目标限制、命令审核与工具执行授权需要在实际执行层另行实现。

## 改动位置与验证

| 位置 | 改动 |
| --- | --- |
| `backend/internal/keyprotection/core.go`、`json.go`、`store.go`、`response.go` | 配置与本地规则、HMAC 标识、结构化正文转换、Redis 加密映射、JSON 与 SSE 响应还原 |
| `backend/internal/server/middleware/key_protection.go` | 鉴权后统一保护入口、正文重读替换、支持范围检查、最外层客户端输出包装 |
| `backend/internal/server/routes/gateway.go`、`server/router.go` | 核心 HTTP 和其他网关路径接入、共享加密存储、管理员清理接口 |
| `backend/internal/service/setting_key_protection.go`、`handler/admin/setting_handler_key_protection.go` | 默认关闭的持久配置、严格管理接口及安全审计 |
| `backend/internal/handler/ops_error_logger.go` 与 OpenAI 兼容转换服务 | 防止收集已还原错误正文，隔离既有 reasoning 缓存及自动续接路径 |
| `frontend/src/views/admin/settings/KeyProtectionSettings.vue`、管理 API 及中英文文案 | 开关、范围、模式、规则、容量、主动清理和使用边界 |

本地模拟测试直接检查上游接收正文没有虚构原值、含有效占位符；随后检查客户端文本和工具参数恢复原值，审核快照与诊断日志没有原值。网关测试还模拟一次 503 后重试，断言两次上游正文一致；这验证正文保护进入重试路径，而非只测试孤立的字符串函数。

应使用本仓库要求的 Go 1.27.0 和锁定的前端依赖，在本地运行：

```sh
cd backend
go test -tags=unit ./internal/keyprotection ./internal/server/middleware ./internal/server/routes ./internal/handler ./internal/handler/admin ./internal/service ./internal/securityaudit ./internal/pkg/apicompat
go test -tags=unit ./internal/service -run '^TestKeyProtection' -count=1
go test ./internal/keyprotection -run '^ExampleState_ProtectJSON$' -v
go vet ./internal/keyprotection ./internal/server/middleware ./internal/server/routes ./internal/server ./internal/handler ./internal/handler/admin ./internal/service
cd ../frontend
pnpm exec vue-tsc --noEmit
pnpm exec eslint src/api/admin/keyProtection.ts src/views/admin/settings/KeyProtectionSettings.vue src/views/admin/settings/__tests__/KeyProtectionSettings.spec.ts
pnpm exec vitest run src/views/admin/settings/__tests__/KeyProtectionSettings.spec.ts src/views/admin/__tests__/SettingsView.spec.ts src/i18n/__tests__/localeKeyCompleteness.spec.ts src/i18n/__tests__/localesMessageCompile.spec.ts
```

2026-09-13 已执行并通过：上述八个后端包的完整 `-tags=unit` 回归；后续改动的核心、网关、管理接口定向复验；`cmd/server` 编译检查；相关包 `go vet`；前端 Vue 类型检查及新增文件 ESLint；新增设置卡片 5 项、原设置页 38 项、国际化完整性和编译 5 项测试。

实际服务转发测试 `TestKeyProtectionActualGatewayForwarding` 覆盖 Chat 原生转发、Responses→Chat→Responses、Messages→Chat→Messages 三种组合，各含 JSON/SSE 与文本/工具还原，直接断言模拟上游没有虚构原值且保留占位符，并验证上游 usage 的 11/7 token 未被还原长度改变。另有已配置 WS 的账号仍选择受保护 HTTP 转发的测试。中间件闭环还校验真实审核快照、503 重试重读、日志、用户隔离和跨轮响应关联。Redis 测试使用 miniredis，覆盖加密、跨实例重建、并发、篡改、过期、容量及清理；未连接生产 Redis。

Windows 本地未启用 CGO，`go test -race` 明确报告需要 CGO，因此没有宣称通过 race detector。并发逻辑的普通测试已通过，可在具备 CGO 的 CI 环境补跑 `go test -race ./internal/keyprotection`。全部测试使用虚构凭据和本地模拟上游，不调用真实密钥或付费模型。本次没有部署到生产环境；本地模拟验证不代表所有真实客户端与供应商扩展组合已经验证。

## 本地部署与真实客户端验收

在上述代码测试之后，又搭建了包含生产前端的本地 Sub2API 实例，连接独立 PostgreSQL 16.15、真实 Redis 8.10.1，并接入实际 OpenCode 1.18.27。三个核心 HTTP 协议及跨协议转换的真实网关回归 26 组通过，包含 204 个占位符分片请求；Chrome 设置页 10 项通过；OpenCode 三协议各 9 项断言全部通过，确实执行本地写文件/读回工具并续聊，上游每轮均仅收到占位符。

另外通过本地模拟外审实际验证同步及异步审核输入、PostgreSQL 完整提示词快照与日志无虚构原值；真实双网关实例及网关进程重启后，经过授权的响应续接映射仍可还原，HMAC 占位符保持稳定。模型和外审均为本地模拟服务，没有调用真实付费模型。

本地验收使用虚构 GitHub 格式密钥，输入为含原值的普通提示词。模拟上游检查对应位置已变为 `keyx_` 占位符，再让 OpenCode 通过工具写文件、读回并续聊；客户端输出及文件内容恢复原值，工具回显进入下一轮上游前再次替换。三阶段 JSON 对照见本文“虚构密钥的三阶段对照”。这验证了实际客户端和工具执行链路，未验证真实模型能否始终正确复制占位符。

完整本地验收工具、报告与运行产物另行保留，不属于功能 PR；本 PR 保留上述可直接运行的后端、前端回归测试。项目默认关闭，本地验收仅为两个测试用户启用可逆替换。测试 Redis 关闭磁盘持久化，停止 Redis 后旧映射不再保留，须重新发送包含原值的完整历史。

## 参考来源

LiteLLM 企业密钥检测文档演示的是固定 `[REDACTED]` 替换，不能据此推断其已经具备本次需要的跨协议智能体可逆流程。[LiteLLM Secret Detection/Redaction](https://docs.litellm.ai/docs/proxy/guardrails/secret_detection)

LiteLLM 的 Presidio 文档另行介绍 `output_parse_pii`，可将模型回复中的替换标记恢复成用户提交值。本实现参考输入保护与输出还原的阶段划分，采用本地规则及自身映射，不依赖 Presidio 服务。[LiteLLM Presidio Output parsing](https://docs.litellm.ai/docs/proxy/guardrails/pii_masking_v2#output-parsing)
